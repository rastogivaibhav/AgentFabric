#include "graphene/db.hpp"
#include "graphene/faiss_index.hpp"
#include "graphene/lattice_placement.hpp"
#include "graphene/platform.hpp"
#include "graphene/vector_index.hpp"
#include "graphene/kdtree_index.hpp"
#include <algorithm>
#include <array>
#include <bit>
#include <cerrno>
#include <chrono>
#include <cmath>
#include <cstdlib>
#include <cstdio>
#include <cstring>
#include <fstream>
#include <iomanip>
#include <iostream>
#include <iterator>
#include <map>
#include <mutex>
#include <numeric>
#include <set>
#include <sstream>
#include <unordered_map>
#include <unordered_set>

namespace fs = std::filesystem;

namespace graphene {

namespace {

uint64_t fnv1a(const std::string& s) {
  uint64_t h = 1469598103934665603ull;
  for (unsigned char c : s) {
    h ^= c;
    h *= 1099511628211ull;
  }
  return h;
}

std::string hex_encode(const std::string& input) {
  static const char* hex = "0123456789abcdef";
  std::string out;
  out.reserve(input.size() * 2);
  for (unsigned char c : input) {
    out.push_back(hex[c >> 4]);
    out.push_back(hex[c & 0x0f]);
  }
  return out;
}

bool hex_decode(const std::string& input, std::string* out) {
  if (input.size() % 2 != 0) return false;
  auto val = [](char c) -> int {
    if (c >= '0' && c <= '9') return c - '0';
    if (c >= 'a' && c <= 'f') return c - 'a' + 10;
    if (c >= 'A' && c <= 'F') return c - 'A' + 10;
    return -1;
  };
  out->clear();
  out->reserve(input.size() / 2);
  for (size_t i = 0; i < input.size(); i += 2) {
    int hi = val(input[i]);
    int lo = val(input[i + 1]);
    if (hi < 0 || lo < 0) return false;
    out->push_back(static_cast<char>((hi << 4) | lo));
  }
  return true;
}

std::string serialize_vector(const std::vector<float>& v) {
  std::ostringstream os;
  os << std::setprecision(9);
  for (size_t i = 0; i < v.size(); ++i) {
    if (i) os << ',';
    os << v[i];
  }
  return os.str();
}

bool parse_vector(const std::string& s, std::vector<float>* out) {
  out->clear();
  if (s.empty()) return true;
  std::stringstream ss(s);
  std::string item;
  while (std::getline(ss, item, ',')) {
    try {
      size_t pos = 0;
      float f = std::stof(item, &pos);
      if (pos != item.size() || !std::isfinite(f)) return false;
      out->push_back(f);
    } catch (...) {
      return false;
    }
  }
  return true;
}

std::string serialize_metadata(const std::map<std::string, std::string>& m) {
  std::ostringstream os;
  bool first = true;
  for (const auto& kv : m) {
    if (!first) os << ';';
    first = false;
    os << hex_encode(kv.first) << '=' << hex_encode(kv.second);
  }
  return os.str();
}

bool parse_metadata(const std::string& s, std::map<std::string, std::string>* out) {
  out->clear();
  if (s.empty()) return true;
  std::stringstream ss(s);
  std::string item;
  while (std::getline(ss, item, ';')) {
    auto pos = item.find('=');
    if (pos == std::string::npos) return false;
    std::string k, v;
    if (!hex_decode(item.substr(0, pos), &k)) return false;
    if (!hex_decode(item.substr(pos + 1), &v)) return false;
    (*out)[k] = v;
  }
  return true;
}

std::vector<std::string> split_tab(const std::string& s) {
  std::vector<std::string> parts;
  size_t start = 0;
  while (true) {
    size_t pos = s.find('\t', start);
    if (pos == std::string::npos) {
      parts.push_back(s.substr(start));
      break;
    }
    parts.push_back(s.substr(start, pos - start));
    start = pos + 1;
  }
  return parts;
}

std::string make_frame(const std::string& payload) {
  return std::to_string(payload.size()) + "|" + std::to_string(fnv1a(payload)) + "|" + payload + "\n";
}

bool parse_frame_line(const std::string& line, std::string* payload) {
  auto p1 = line.find('|');
  if (p1 == std::string::npos) return false;
  auto p2 = line.find('|', p1 + 1);
  if (p2 == std::string::npos) return false;
  size_t len = 0;
  uint64_t checksum = 0;
  try {
    len = static_cast<size_t>(std::stoull(line.substr(0, p1)));
    checksum = std::stoull(line.substr(p1 + 1, p2 - p1 - 1));
  } catch (...) {
    return false;
  }
  std::string data = line.substr(p2 + 1);
  if (data.size() != len) return false;
  if (fnv1a(data) != checksum) return false;
  *payload = std::move(data);
  return true;
}

bool visible_node(const Node& n, uint64_t snap) {
  return n.created_version <= snap && snap < n.deleted_version;
}

bool visible_edge(const Edge& e, const std::vector<Node>& nodes, uint64_t snap) {
  if (!(e.created_version <= snap && snap < e.deleted_version)) return false;
  if (e.from >= nodes.size() || e.to >= nodes.size()) return false;
  return visible_node(nodes[e.from], snap) && visible_node(nodes[e.to], snap);
}

int popcount64(uint64_t x) { return static_cast<int>(std::popcount(x)); }

bool edge_allowed_for_mode(const Edge& edge, QueryMode mode) {
  if (mode == QueryMode::Empirical) {
    return (edge.origin == EdgeOrigin::Observed || edge.origin == EdgeOrigin::Discovered) &&
           edge.role != EdgeRole::Analogical;
  }
  if (mode == QueryMode::Balanced) return edge.origin != EdgeOrigin::Hypothetical;
  return true;
}

double cosine(const std::vector<float>& a, const std::vector<float>& b) {
  if (a.size() != b.size() || a.empty()) return -1.0;
  double dot = 0.0, na = 0.0, nb = 0.0;
  for (size_t i = 0; i < a.size(); ++i) {
    dot += static_cast<double>(a[i]) * b[i];
    na += static_cast<double>(a[i]) * a[i];
    nb += static_cast<double>(b[i]) * b[i];
  }
  if (na == 0.0 || nb == 0.0) return -1.0;
  return dot / (std::sqrt(na) * std::sqrt(nb));
}

std::string bool_s(bool b) { return b ? "1" : "0"; }

bool env_enabled(const char* name) {
  const char* v = std::getenv(name);
  return v && std::string(v) == "1";
}

std::string serialize_lattice(const std::optional<LatticeCoord>& coord) {
  if (!coord) return "none";
  return std::to_string(coord->q) + "," + std::to_string(coord->r) + "," + std::to_string(coord->layer);
}

bool parse_lattice(const std::string& s, std::optional<LatticeCoord>* out) {
  if (s == "none" || s.empty()) {
    *out = std::nullopt;
    return true;
  }
  std::stringstream ss(s);
  std::string q, r, layer;
  if (!std::getline(ss, q, ',')) return false;
  if (!std::getline(ss, r, ',')) return false;
  if (!std::getline(ss, layer, ',')) return false;
  std::string extra;
  if (std::getline(ss, extra, ',')) return false;
  try {
    *out = LatticeCoord{std::stoi(q), std::stoi(r), std::stoi(layer)};
    return true;
  } catch (...) {
    return false;
  }
}

bool are_same_layer_hex_neighbors(const LatticeCoord& a, const LatticeCoord& b) {
  if (a.layer != b.layer) return false;
  const int dq = b.q - a.q;
  const int dr = b.r - a.r;
  return (dq == 1 && dr == 0) || (dq == 1 && dr == -1) ||
         (dq == 0 && dr == -1) || (dq == -1 && dr == 0) ||
         (dq == -1 && dr == 1) || (dq == 0 && dr == 1);
}

bool are_cross_layer_neighbors(const LatticeCoord& a, const LatticeCoord& b, LayerCoupling coupling) {
  if (a.layer == b.layer) return false;
  if (coupling == LayerCoupling::None || coupling == LayerCoupling::SameLayer) return false;
  const int dq = std::abs(b.q - a.q);
  const int dr = std::abs(b.r - a.r);
  return dq <= 1 && dr <= 1;
}

bool is_lattice_bond(BondType bond) {
  return bond == BondType::Sigma || bond == BondType::Pi || bond == BondType::VanDerWaals ||
         bond == BondType::Defect || bond == BondType::Synthetic;
}

const char* vector_index_kind_name(VectorIndexKind kind) {
  switch (kind) {
    case VectorIndexKind::Auto: return "auto";
    case VectorIndexKind::Flat: return "flat";
    case VectorIndexKind::KDTree: return "kdtree";
    case VectorIndexKind::Faiss: return "faiss";
  }
  return "unknown";
}

} // namespace

struct GrapheneDB::Impl {
  struct LatticeCoordHash {
    size_t operator()(const LatticeCoord& c) const {
      uint64_t h = 1469598103934665603ull;
      auto mix = [&](int32_t v) {
        uint32_t u = static_cast<uint32_t>(v);
        for (int i = 0; i < 4; ++i) {
          h ^= static_cast<unsigned char>((u >> (i * 8)) & 0xff);
          h *= 1099511628211ull;
        }
      };
      mix(c.q);
      mix(c.r);
      mix(c.layer);
      return static_cast<size_t>(h);
    }
  };

  mutable std::shared_mutex mu;
  bool open{false};
  std::string maintenance_warning;
  fs::path dir, wal_path, data_path, manifest_path, physical_lattice_path, physical_lattice_bin_path, nodeidx_path, lock_path;
  DBOptions opt;
  platform::FileHandle wal_fd{platform::kInvalidFile};
  uint64_t version{1};
  uint64_t txid{1};
  uint32_t next_node_id{0};
  uint32_t next_edge_id{0};
  size_t live_node_count{0};
  size_t live_edge_count{0};
  std::unique_ptr<VectorIndex> vector_index;
  std::vector<Node> nodes;
  std::vector<Edge> edges;
  std::unordered_map<uint32_t, std::vector<uint32_t>> in_edges;
  std::unordered_map<uint32_t, std::vector<uint32_t>> out_edges;
  std::unordered_map<uint64_t, std::vector<uint32_t>> planes;
  std::unordered_map<std::string, std::vector<uint32_t>> metadata_index;
  std::unordered_map<std::string, std::vector<uint32_t>> relation_index;
  std::unordered_map<LatticeCoord, uint32_t, LatticeCoordHash> lattice_nodes;
  std::unordered_map<uint32_t, std::vector<uint32_t>> lattice_edges;

  // Dense physical hex-lattice index. Cells are sorted by layer/ring/coord;
  // each cell owns fixed six-neighbour slots so lattice traversal can avoid
  // generic edge-vector lookup when physical_lattice_storage is enabled.
  struct DenseLatticeCell {
    LatticeCoord coord;
    uint32_t node_id{UINT32_MAX};
    std::array<uint32_t, 6> neighbor_nodes{UINT32_MAX, UINT32_MAX, UINT32_MAX, UINT32_MAX, UINT32_MAX, UINT32_MAX};
    std::vector<uint32_t> lattice_edge_ids;
  };
  std::vector<DenseLatticeCell> dense_lattice_cells;
  std::unordered_map<uint32_t, uint32_t> dense_lattice_ordinal_by_node;

  struct PhysicalCellRecord {
    int32_t q{0};
    int32_t r{0};
    int32_t layer{0};
    uint32_t node_id{UINT32_MAX};
    uint32_t neighbor_nodes[6]{UINT32_MAX, UINT32_MAX, UINT32_MAX, UINT32_MAX, UINT32_MAX, UINT32_MAX};
    uint64_t vector_dim{0};
    uint64_t content_bytes{0};
    uint32_t flags{0};
    uint32_t checksum{0};
  };

  struct PhysicalLatticeHeader {
    char magic[8];
    uint32_t version;
    uint32_t radius;
    uint64_t cells_per_layer;
    uint64_t visible_cells;
    uint64_t snapshot;
  };

  static uint32_t fnv32_cell(const PhysicalCellRecord& rec) {
    const unsigned char* p = reinterpret_cast<const unsigned char*>(&rec);
    size_t n = sizeof(PhysicalCellRecord) - sizeof(uint32_t);
    uint32_t h = 2166136261u;
    for (size_t i = 0; i < n; ++i) { h ^= p[i]; h *= 16777619u; }
    return h;
  }

  static bool axial_disk_ordinal(int32_t q, int32_t r, uint32_t radius, uint64_t* out) {
    const int32_t R = static_cast<int32_t>(radius);
    const int32_t s = -q - r;
    if (std::max({std::abs(q), std::abs(r), std::abs(s)}) > R) return false;
    uint64_t idx = 0;
    for (int32_t qq = -R; qq < q; ++qq) {
      const int32_t rmin = std::max(-R, -qq - R);
      const int32_t rmax = std::min(R, -qq + R);
      idx += static_cast<uint64_t>(rmax - rmin + 1);
    }
    const int32_t rmin = std::max(-R, -q - R);
    idx += static_cast<uint64_t>(r - rmin);
    *out = idx;
    return true;
  }

  static uint64_t cells_per_layer(uint32_t radius) {
    return 1ull + 3ull * radius * (static_cast<uint64_t>(radius) + 1ull);
  }

  VectorIndexKind resolve_vector_index_kind() const {
    VectorIndexKind kind = opt.vector_index_kind;
    if (kind == VectorIndexKind::Auto) {
      kind = opt.dimension <= 32 ? VectorIndexKind::KDTree : VectorIndexKind::Flat;
    }
    return kind;
  }

  Status validate_vector_index_support() const {
    VectorIndexKind kind = resolve_vector_index_kind();
    if (kind != VectorIndexKind::Faiss) return Status::ok();
#ifdef GRAPHENEDB_HAS_FAISS
    return Status::ok();
#else
    return Status::error(
      ErrorCode::UnsupportedMode,
      "FAISS vector index requested but GrapheneDB was built without FAISS support");
#endif
  }

  std::unique_ptr<VectorIndex> make_vector_index() const {
    VectorIndexKind kind = resolve_vector_index_kind();
    if (kind == VectorIndexKind::Faiss) {
#ifdef GRAPHENEDB_HAS_FAISS
      return std::make_unique<FaissVectorIndex>(opt.dimension);
#else
      return std::make_unique<FlatVectorIndex>(opt.dimension);
#endif
    }
    if (kind == VectorIndexKind::KDTree) return std::make_unique<KDTreeVectorIndex>(opt.dimension);
    return std::make_unique<FlatVectorIndex>(opt.dimension);
  }

  void ensure_node_slot(uint32_t id) {
    if (nodes.size() <= id) {
      size_t old = nodes.size();
      nodes.resize(static_cast<size_t>(id) + 1);
      for (size_t i = old; i < nodes.size(); ++i) {
        nodes[i].id = static_cast<uint32_t>(i);
        nodes[i].created_version = kInfVersion;
        nodes[i].deleted_version = kInfVersion;
      }
    }
  }

  void ensure_edge_slot(uint32_t id) {
    if (edges.size() <= id) {
      size_t old = edges.size();
      edges.resize(static_cast<size_t>(id) + 1);
      for (size_t i = old; i < edges.size(); ++i) {
        edges[i].id = static_cast<uint32_t>(i);
        edges[i].created_version = kInfVersion;
        edges[i].deleted_version = kInfVersion;
      }
    }
  }

  void index_node_metadata(const Node& n) {
    for (const auto& kv : n.metadata) {
      metadata_index[kv.first + "\x1f" + kv.second].push_back(n.id);
    }
  }

  void apply_committed_node(const Node& n) {
    ensure_node_slot(n.id);
    nodes[n.id] = n;
    planes[n.signature].push_back(n.id);
    index_node_metadata(n);
    if (n.lattice) lattice_nodes[*n.lattice] = n.id;
    ++live_node_count;
    if (vector_index) (void)vector_index->add(n.id, n.vector);
    add_dense_lattice_node_unlocked(n, version ? version - 1 : 0);
  }

  void apply_committed_edge(const Edge& e) {
    ensure_edge_slot(e.id);
    edges[e.id] = e;
    out_edges[e.from].push_back(e.id);
    in_edges[e.to].push_back(e.id);
    const auto relation = e.metadata.find("graphene_relation_key");
    if (relation != e.metadata.end()) {
      relation_index[relation->second].push_back(e.id);
    }
    if (is_lattice_bond(e.bond_type)) {
      lattice_edges[e.from].push_back(e.id);
      lattice_edges[e.to].push_back(e.id);
    }
    ++live_edge_count;
  }

  static int32_t hex_ring_distance(const LatticeCoord& c) {
    const int32_t s = -c.q - c.r;
    return std::max({std::abs(c.q), std::abs(c.r), std::abs(s)});
  }

  static std::array<LatticeCoord, 6> hex_neighbor_coords(const LatticeCoord& c) {
    return {{{c.q + 1, c.r, c.layer}, {c.q + 1, c.r - 1, c.layer}, {c.q, c.r - 1, c.layer},
             {c.q - 1, c.r, c.layer}, {c.q - 1, c.r + 1, c.layer}, {c.q, c.r + 1, c.layer}}};
  }

  static int opposite_hex_dir(int i) { return (i + 3) % 6; }

  void add_dense_lattice_node_unlocked(const Node& n, uint64_t snap) {
    if (!opt.physical_lattice_storage || !n.lattice || !visible_node(n, snap)) return;
    if (dense_lattice_ordinal_by_node.find(n.id) != dense_lattice_ordinal_by_node.end()) return;
    DenseLatticeCell cell;
    cell.coord = *n.lattice;
    cell.node_id = n.id;
    auto dirs = hex_neighbor_coords(*n.lattice);
    for (size_t i = 0; i < dirs.size(); ++i) {
      auto it = lattice_nodes.find(dirs[i]);
      if (it != lattice_nodes.end() && it->second < nodes.size() && visible_node(nodes[it->second], snap)) {
        cell.neighbor_nodes[i] = it->second;
      }
    }
    uint32_t ordinal = static_cast<uint32_t>(dense_lattice_cells.size());
    dense_lattice_cells.push_back(cell);
    dense_lattice_ordinal_by_node[n.id] = ordinal;
    for (size_t i = 0; i < cell.neighbor_nodes.size(); ++i) {
      uint32_t nb = cell.neighbor_nodes[i];
      if (nb == UINT32_MAX) continue;
      auto oit = dense_lattice_ordinal_by_node.find(nb);
      if (oit == dense_lattice_ordinal_by_node.end()) continue;
      dense_lattice_cells[oit->second].neighbor_nodes[opposite_hex_dir(static_cast<int>(i))] = n.id;
    }
  }

  PhysicalLatticeHeader make_physical_header_unlocked(uint64_t visible) const {
    const uint32_t radius = opt.physical_lattice_radius == 0 ? 256 : opt.physical_lattice_radius;
    return PhysicalLatticeHeader{{'G','D','B','H','E','X','1','\0'}, 1u, radius, cells_per_layer(radius), visible, version ? version - 1 : 0};
  }

  Status physical_absolute_ordinal_unlocked(const LatticeCoord& coord, uint64_t* out) const {
    if (coord.layer < 0) {
      return Status::error(ErrorCode::InvalidInput, "physical lattice primary requires a non-negative layer");
    }
    const uint32_t radius = opt.physical_lattice_radius == 0 ? 256 : opt.physical_lattice_radius;
    uint64_t ordinal = 0;
    if (!axial_disk_ordinal(coord.q, coord.r, radius, &ordinal)) {
      return Status::error(ErrorCode::InvalidInput, "lattice coord outside physical lattice radius");
    }
    *out = static_cast<uint64_t>(coord.layer) * cells_per_layer(radius) + ordinal;
    return Status::ok();
  }

  void note_maintenance_failure(const char* phase, const Status& status) {
    if (status) return;
    if (!maintenance_warning.empty()) maintenance_warning += "; ";
    maintenance_warning += std::string(phase) + ": " + status.message;
  }

  Status ensure_physical_lattice_binary_file_unlocked() {
    if (!opt.physical_lattice_primary) return Status::ok();
    std::error_code ec;
    if (fs::exists(physical_lattice_bin_path, ec)) return Status::ok();
    std::ofstream out(physical_lattice_bin_path, std::ios::binary | std::ios::trunc);
    if (!out) return Status::error(ErrorCode::IoError, "cannot create physical lattice binary");
    auto header = make_physical_header_unlocked(live_node_count);
    out.write(reinterpret_cast<const char*>(&header), sizeof(header));
    out.flush();
    return out ? Status::ok() : Status::error(ErrorCode::IoError, "failed writing physical lattice binary header");
  }

  Status update_physical_header_unlocked() {
    if (!opt.physical_lattice_primary) return Status::ok();
    auto st = ensure_physical_lattice_binary_file_unlocked();
    if (!st) return st;
    std::fstream io(physical_lattice_bin_path, std::ios::binary | std::ios::in | std::ios::out);
    if (!io) return Status::error(ErrorCode::IoError, "cannot open physical lattice binary header for update");
    auto header = make_physical_header_unlocked(live_node_count);
    io.seekp(0);
    io.write(reinterpret_cast<const char*>(&header), sizeof(header));
    io.flush();
    return io ? Status::ok() : Status::error(ErrorCode::IoError, "failed updating physical lattice binary header");
  }

  Status write_physical_cell_record_unlocked(const DenseLatticeCell& c) {
    if (!opt.physical_lattice_primary) return Status::ok();
    auto st = ensure_physical_lattice_binary_file_unlocked();
    if (!st) return st;
    uint64_t absolute = 0;
    st = physical_absolute_ordinal_unlocked(c.coord, &absolute);
    if (!st) return st;
    PhysicalCellRecord rec;
    rec.q = c.coord.q; rec.r = c.coord.r; rec.layer = c.coord.layer; rec.node_id = c.node_id;
    for (size_t i = 0; i < 6; ++i) rec.neighbor_nodes[i] = c.neighbor_nodes[i];
    if (c.node_id < nodes.size() && visible_node(nodes[c.node_id], version ? version - 1 : 0)) {
      rec.vector_dim = nodes[c.node_id].vector.size();
      rec.content_bytes = nodes[c.node_id].content.size();
    }
    rec.flags = 1u; rec.checksum = 0; rec.checksum = fnv32_cell(rec);
    std::fstream io(physical_lattice_bin_path, std::ios::binary | std::ios::in | std::ios::out);
    if (!io) return Status::error(ErrorCode::IoError, "cannot open physical lattice binary for cell update");
    io.seekp(static_cast<std::streamoff>(sizeof(PhysicalLatticeHeader) + absolute * sizeof(PhysicalCellRecord)));
    io.write(reinterpret_cast<const char*>(&rec), sizeof(rec));
    io.flush();
    if (!io) return Status::error(ErrorCode::IoError, "failed writing physical cell record");
    return Status::ok();
  }

  Status append_nodeidx_unlocked(const DenseLatticeCell& c) {
    if (!opt.physical_lattice_primary) return Status::ok();
    const bool exists = fs::exists(nodeidx_path);
    std::ofstream idx(nodeidx_path, std::ios::app);
    if (!idx) return Status::error(ErrorCode::IoError, "cannot append nodeidx");
    if (!exists) idx << "# node_id\tlayer\tq\tr\tordinal\n";
    uint64_t absolute = 0;
    auto st = physical_absolute_ordinal_unlocked(c.coord, &absolute);
    if (!st) return st;
    idx << c.node_id << '\t' << c.coord.layer << '\t' << c.coord.q << '\t' << c.coord.r << '\t' << absolute << '\n';
    idx.flush();
    return idx ? Status::ok() : Status::error(ErrorCode::IoError, "failed appending nodeidx");
  }

  Status write_incremental_physical_lattice_node_unlocked(const Node& n) {
    if (!opt.physical_lattice_primary || !n.lattice) return Status::ok();
    auto oit = dense_lattice_ordinal_by_node.find(n.id);
    if (oit == dense_lattice_ordinal_by_node.end()) return Status::error(ErrorCode::InvalidInput, "node missing from dense lattice index");
    const auto& cell = dense_lattice_cells[oit->second];
    auto st = write_physical_cell_record_unlocked(cell);
    if (!st) return st;
    for (uint32_t nb : cell.neighbor_nodes) {
      if (nb == UINT32_MAX) continue;
      auto nit = dense_lattice_ordinal_by_node.find(nb);
      if (nit != dense_lattice_ordinal_by_node.end()) {
        st = write_physical_cell_record_unlocked(dense_lattice_cells[nit->second]);
        if (!st) return st;
      }
    }
    st = append_nodeidx_unlocked(cell);
    if (!st) return st;
    return update_physical_header_unlocked();
  }

  void rebuild_dense_lattice_index_unlocked(uint64_t snap) {
    dense_lattice_cells.clear();
    dense_lattice_ordinal_by_node.clear();
    if (!opt.physical_lattice_storage) return;
    dense_lattice_cells.reserve(lattice_nodes.size());
    for (const auto& n : nodes) {
      if (!visible_node(n, snap) || !n.lattice) continue;
      DenseLatticeCell c;
      c.coord = *n.lattice;
      c.node_id = n.id;
      auto dirs = hex_neighbor_coords(*n.lattice);
      for (size_t i = 0; i < dirs.size(); ++i) {
        auto it = lattice_nodes.find(dirs[i]);
        if (it != lattice_nodes.end() && it->second < nodes.size() && visible_node(nodes[it->second], snap)) {
          c.neighbor_nodes[i] = it->second;
        }
      }
      auto eit = lattice_edges.find(n.id);
      if (eit != lattice_edges.end()) {
        for (uint32_t eid : eit->second) {
          if (eid < edges.size() && visible_edge(edges[eid], nodes, snap)) c.lattice_edge_ids.push_back(eid);
        }
        std::sort(c.lattice_edge_ids.begin(), c.lattice_edge_ids.end());
        c.lattice_edge_ids.erase(std::unique(c.lattice_edge_ids.begin(), c.lattice_edge_ids.end()), c.lattice_edge_ids.end());
      }
      dense_lattice_cells.push_back(std::move(c));
    }
    std::sort(dense_lattice_cells.begin(), dense_lattice_cells.end(), [](const DenseLatticeCell& a, const DenseLatticeCell& b) {
      if (a.coord.layer != b.coord.layer) return a.coord.layer < b.coord.layer;
      const auto ar = hex_ring_distance(a.coord), br = hex_ring_distance(b.coord);
      if (ar != br) return ar < br;
      if (a.coord.q != b.coord.q) return a.coord.q < b.coord.q;
      if (a.coord.r != b.coord.r) return a.coord.r < b.coord.r;
      return a.node_id < b.node_id;
    });
    for (uint32_t i = 0; i < dense_lattice_cells.size(); ++i) dense_lattice_ordinal_by_node[dense_lattice_cells[i].node_id] = i;
  }

  bool dense_lattice_enabled() const {
    return opt.physical_lattice_storage && !dense_lattice_cells.empty();
  }

  Status write_physical_lattice_unlocked() {
    if (!opt.physical_lattice_storage) return Status::ok();
    const uint64_t snap = version ? version - 1 : 0;
    rebuild_dense_lattice_index_unlocked(snap);
    const auto& cells = dense_lattice_cells;
    fs::path tmp = physical_lattice_path.string() + ".tmp";
    std::ofstream out(tmp, std::ios::trunc);
    if (!out) return Status::error(ErrorCode::IoError, "cannot write physical lattice tmp");
    out << make_frame("LATTICE_BEGIN\t1\t" + std::to_string(snap) + "\t" + std::to_string(cells.size()));
    for (const auto& c : cells) {
      std::ostringstream payload;
      payload << "CELL\t" << c.coord.layer << '\t' << c.coord.q << '\t' << c.coord.r << '\t' << c.node_id;
      for (uint32_t n : c.neighbor_nodes) payload << '\t' << (n == UINT32_MAX ? -1 : static_cast<int64_t>(n));
      payload << '\t';
      for (size_t i = 0; i < c.lattice_edge_ids.size(); ++i) {
        if (i) payload << ',';
        payload << c.lattice_edge_ids[i];
      }
      out << make_frame(payload.str());
    }
    out << make_frame("LATTICE_END\t" + std::to_string(fnv1a(std::to_string(snap) + ":" + std::to_string(cells.size()))));
    out.flush();
    if (!out) return Status::error(ErrorCode::IoError, "failed flushing physical lattice tmp");
    out.close();
    auto replace_status = platform::durable_replace(tmp, physical_lattice_path);
    if (!replace_status) return Status::error(replace_status.code, "physical lattice replace failed: " + replace_status.message);
    if (opt.physical_lattice_primary) {
      auto bst = write_physical_lattice_binary_unlocked(cells);
      if (!bst) return bst;
    }
    return Status::ok();
  }

  Status write_physical_lattice_binary_unlocked(const std::vector<DenseLatticeCell>& cells) {
    const uint32_t radius = opt.physical_lattice_radius == 0 ? 256 : opt.physical_lattice_radius;
    PhysicalLatticeHeader header{{'G','D','B','H','E','X','1','\0'}, 1u, radius, cells_per_layer(radius), static_cast<uint64_t>(cells.size()), version ? version - 1 : 0};

    fs::path tmp = physical_lattice_bin_path.string() + ".tmp";
    std::ofstream out(tmp, std::ios::binary | std::ios::trunc);
    if (!out) return Status::error(ErrorCode::IoError, "cannot write physical lattice binary tmp");
    out.write(reinterpret_cast<const char*>(&header), sizeof(header));

    std::map<uint64_t, PhysicalCellRecord> records;
    for (const auto& c : cells) {
      uint64_t ordinal = 0;
      if (!axial_disk_ordinal(c.coord.q, c.coord.r, radius, &ordinal)) {
        return Status::error(ErrorCode::InvalidInput, "lattice coord outside physical lattice radius");
      }
      if (c.coord.layer < 0) {
        return Status::error(ErrorCode::InvalidInput, "physical lattice primary requires a non-negative layer");
      }
      uint64_t absolute = static_cast<uint64_t>(c.coord.layer) * header.cells_per_layer + ordinal;
      PhysicalCellRecord rec;
      rec.q = c.coord.q; rec.r = c.coord.r; rec.layer = c.coord.layer; rec.node_id = c.node_id;
      for (size_t i = 0; i < 6; ++i) rec.neighbor_nodes[i] = c.neighbor_nodes[i];
      if (c.node_id < nodes.size() && visible_node(nodes[c.node_id], version ? version - 1 : 0)) {
        rec.vector_dim = nodes[c.node_id].vector.size();
        rec.content_bytes = nodes[c.node_id].content.size();
      }
      rec.flags = 1u;
      rec.checksum = 0; rec.checksum = fnv32_cell(rec);
      records[absolute] = rec;
    }
    for (const auto& kv : records) {
      const uint64_t offset = sizeof(header) + kv.first * sizeof(PhysicalCellRecord);
      out.seekp(static_cast<std::streamoff>(offset));
      out.write(reinterpret_cast<const char*>(&kv.second), sizeof(PhysicalCellRecord));
    }
    out.flush(); out.close();
    if (!out) return Status::error(ErrorCode::IoError, "failed flushing physical lattice binary tmp");
    auto replace_status = platform::durable_replace(tmp, physical_lattice_bin_path);
    if (!replace_status) return Status::error(replace_status.code, "physical lattice binary replace failed: " + replace_status.message);

    fs::path itmp = nodeidx_path.string() + ".tmp";
    std::ofstream idx(itmp, std::ios::trunc);
    if (!idx) return Status::error(ErrorCode::IoError, "cannot write nodeidx tmp");
    idx << "# node_id\tlayer\tq\tr\tordinal\n";
    for (const auto& c : cells) {
      uint64_t ordinal = 0;
      (void)axial_disk_ordinal(c.coord.q, c.coord.r, radius, &ordinal);
      if (c.coord.layer < 0) {
        return Status::error(ErrorCode::InvalidInput, "physical lattice primary requires a non-negative layer");
      }
      uint64_t absolute = static_cast<uint64_t>(c.coord.layer) * header.cells_per_layer + ordinal;
      idx << c.node_id << '\t' << c.coord.layer << '\t' << c.coord.q << '\t' << c.coord.r << '\t' << absolute << '\n';
    }
    idx.flush(); idx.close();
    replace_status = platform::durable_replace(itmp, nodeidx_path);
    if (!replace_status) return Status::error(replace_status.code, "nodeidx replace failed: " + replace_status.message);
    return Status::ok();
  }

  Status checkpoint_unlocked() {
    uint64_t snap = version ? version - 1 : 0;
    fs::path tmp = data_path.string() + ".tmp";
    {
      std::ofstream out(tmp, std::ios::trunc);
      if (!out) return Status::error(ErrorCode::IoError, "cannot write data tmp");
      for (const auto& n : nodes) if (visible_node(n, snap)) out << make_frame(node_payload(n, "DATA_NODE"));
      for (const auto& e : edges) if (visible_edge(e, nodes, snap)) out << make_frame(edge_payload(e, "DATA_EDGE"));
      if (env_enabled("GRAPHENEDB_TEST_FAIL_CHECKPOINT_WRITE")) {
        return Status::error(ErrorCode::IoError, "injected checkpoint write failure");
      }
      out.flush();
      if (!out) return Status::error(ErrorCode::IoError, "failed flushing data tmp");
    }
    if (env_enabled("GRAPHENEDB_TEST_FAIL_CHECKPOINT_RENAME")) {
      return Status::error(ErrorCode::IoError, "injected checkpoint rename failure");
    }
    auto replace_status = platform::durable_replace(tmp, data_path);
    if (!replace_status) return Status::error(replace_status.code, "checkpoint replace failed: " + replace_status.message);
    close_wal_fd_unlocked();
    auto truncate_status = platform::truncate_and_flush(wal_path);
    if (!truncate_status) {
      (void)open_wal_fd_unlocked();
      return Status::error(truncate_status.code, "cannot durably truncate WAL: " + truncate_status.message);
    }
    auto ost = open_wal_fd_unlocked();
    if (!ost) return ost;
    auto lst = write_physical_lattice_unlocked();
    if (!lst) return lst;
    auto manifest_status = write_manifest();
    if (manifest_status) maintenance_warning.clear();
    return manifest_status;
  }

  Status maybe_rotate_wal_unlocked() {
    if (opt.wal_rotate_bytes == 0) return Status::ok();
    std::error_code ec;
    auto sz = fs::exists(wal_path, ec) ? fs::file_size(wal_path, ec) : 0;
    if (ec) return Status::error(ErrorCode::IoError, "cannot stat WAL: " + ec.message());
    if (sz >= opt.wal_rotate_bytes) return checkpoint_unlocked();
    return Status::ok();
  }

  Status open_wal_fd_unlocked() {
    if (wal_fd != platform::kInvalidFile) return Status::ok();
    auto st = platform::open_append(wal_path, &wal_fd);
    if (!st) return Status::error(st.code, "open WAL fd failed: " + st.message);
    return Status::ok();
  }

  void close_wal_fd_unlocked() {
    if (wal_fd != platform::kInvalidFile) { platform::close_file(wal_fd); wal_fd = platform::kInvalidFile; }
  }

  Status rollback_wal_append_unlocked(uint64_t size_before) {
    close_wal_fd_unlocked();
    std::error_code ec;
    fs::resize_file(wal_path, size_before, ec);
    if (ec) return Status::error(ErrorCode::IoError, "rollback WAL append failed: " + ec.message());
    return open_wal_fd_unlocked();
  }

  Status append_wal_unlocked(const std::vector<std::string>& payloads) {
    if (env_enabled("GRAPHENEDB_TEST_FAIL_WAL_APPEND")) {
      return Status::error(ErrorCode::IoError, "injected WAL append failure");
    }
    auto ost = open_wal_fd_unlocked();
    if (!ost) return ost;
    std::error_code ec;
    uint64_t size_before = fs::exists(wal_path, ec) ? fs::file_size(wal_path, ec) : 0;
    if (ec) return Status::error(ErrorCode::IoError, "cannot stat WAL before append: " + ec.message());
    std::string blob;
    for (const auto& p : payloads) blob += make_frame(p);
    if (env_enabled("GRAPHENEDB_TEST_FAIL_WAL_WRITE")) {
      return Status::error(ErrorCode::IoError, "injected WAL write failure");
    }
    auto wst = platform::write_all(wal_fd, blob.data(), blob.size());
    if (!wst) {
      auto rst = rollback_wal_append_unlocked(size_before);
      if (!rst) return rst;
      return Status::error(wst.code, "write WAL failed: " + wst.message);
    }
    if (opt.fsync_on_commit) {
      if (env_enabled("GRAPHENEDB_TEST_FAIL_WAL_FSYNC")) {
        auto rst = rollback_wal_append_unlocked(size_before);
        if (!rst) return rst;
        return Status::error(ErrorCode::IoError, "injected WAL fsync failure");
      }
      auto fst = platform::flush(wal_fd);
      if (!fst) {
        auto rst = rollback_wal_append_unlocked(size_before);
        if (!rst) return rst;
        return Status::error(fst.code, "fsync WAL failed: " + fst.message);
      }
    }
    return Status::ok();
  }

  Status validate_lattice_node_input(const NodeInput& input) const {
    if (opt.require_lattice && !input.lattice) {
      return Status::error(ErrorCode::InvalidInput, "lattice coordinate required");
    }
    if (input.lattice) {
      auto it = lattice_nodes.find(*input.lattice);
      if (it != lattice_nodes.end()) {
        return Status::error(ErrorCode::InvalidInput, "duplicate lattice coordinate");
      }
      if (opt.physical_lattice_primary) {
        uint64_t ignored = 0;
        auto st = physical_absolute_ordinal_unlocked(*input.lattice, &ignored);
        if (!st) return st;
      }
    }
    return Status::ok();
  }

  Status validate_lattice_edge_input(const EdgeInput& input, uint64_t snap) const {
    if (!std::isfinite(input.bond_strength) || input.bond_strength < 0.0 || input.bond_strength > 1.0) {
      return Status::error(ErrorCode::InvalidInput, "bond_strength must be between 0 and 1");
    }
    if (!is_lattice_bond(input.bond_type)) return Status::ok();
    if (input.from >= nodes.size() || input.to >= nodes.size() || !visible_node(nodes[input.from], snap) || !visible_node(nodes[input.to], snap)) {
      return Status::error(ErrorCode::EdgeInvalid, "lattice edge endpoints must exist and be visible");
    }
    const auto& from = nodes[input.from];
    const auto& to = nodes[input.to];
    if (!from.lattice || !to.lattice) {
      return Status::error(ErrorCode::EdgeInvalid, "lattice bond requires lattice coordinates on both endpoints");
    }
    if (input.bond_type == BondType::Defect || input.bond_type == BondType::Synthetic) return Status::ok();
    if (input.bond_type == BondType::VanDerWaals) {
      if (!are_cross_layer_neighbors(*from.lattice, *to.lattice, input.layer_coupling)) {
        return Status::error(ErrorCode::EdgeInvalid, "VanDerWaals bond requires valid cross-layer neighbor and coupling");
      }
      return Status::ok();
    }
    if (!are_same_layer_hex_neighbors(*from.lattice, *to.lattice)) {
      return Status::error(ErrorCode::EdgeInvalid, "Sigma/Pi bond requires same-layer hex-neighbor coordinates");
    }
    if (input.layer_coupling != LayerCoupling::None && input.layer_coupling != LayerCoupling::SameLayer) {
      return Status::error(ErrorCode::EdgeInvalid, "same-layer lattice bond cannot use cross-layer coupling");
    }
    return Status::ok();
  }

  void rebuild_indexes() {
    in_edges.clear(); out_edges.clear(); planes.clear(); metadata_index.clear(); relation_index.clear(); lattice_nodes.clear(); lattice_edges.clear();
    vector_index = make_vector_index();
    live_node_count = 0; live_edge_count = 0;
    next_node_id = 0; next_edge_id = 0; version = std::max<uint64_t>(version, 1);
    uint64_t current_snap = 0;
    for (const auto& n : nodes) {
      if (n.created_version == kInfVersion) continue;
      current_snap = std::max(current_snap, n.created_version);
      if (n.deleted_version != kInfVersion) current_snap = std::max(current_snap, n.deleted_version);
    }
    for (const auto& e : edges) {
      if (e.created_version == kInfVersion) continue;
      current_snap = std::max(current_snap, e.created_version);
      if (e.deleted_version != kInfVersion) current_snap = std::max(current_snap, e.deleted_version);
    }
    for (auto& n : nodes) {
      if (n.created_version == kInfVersion) continue;
      next_node_id = std::max(next_node_id, n.id + 1);
      version = std::max(version, n.created_version + 1);
      if (n.deleted_version != kInfVersion) version = std::max(version, n.deleted_version + 1);
      planes[n.signature].push_back(n.id);
      index_node_metadata(n);
      if (n.lattice && visible_node(n, current_snap)) lattice_nodes[*n.lattice] = n.id;
      if (visible_node(n, current_snap)) {
        ++live_node_count;
        if (vector_index) (void)vector_index->add(n.id, n.vector);
      }
    }
    for (auto& e : edges) {
      if (e.created_version == kInfVersion) continue;
      next_edge_id = std::max(next_edge_id, e.id + 1);
      version = std::max(version, e.created_version + 1);
      if (e.deleted_version != kInfVersion) version = std::max(version, e.deleted_version + 1);
      out_edges[e.from].push_back(e.id);
      in_edges[e.to].push_back(e.id);
      const auto relation = e.metadata.find("graphene_relation_key");
      if (relation != e.metadata.end()) {
        relation_index[relation->second].push_back(e.id);
      }
      if (visible_edge(e, nodes, current_snap)) {
        ++live_edge_count;
        if (is_lattice_bond(e.bond_type)) {
          lattice_edges[e.from].push_back(e.id);
          lattice_edges[e.to].push_back(e.id);
        }
      }
    }
    rebuild_dense_lattice_index_unlocked(current_snap);
    txid = std::max(txid, version + 1);
  }

  bool vector_valid(const std::vector<float>& v, std::string* reason = nullptr) const {
    if (opt.dimension == 0) {
      if (reason) *reason = "database dimension is zero";
      return false;
    }
    if (v.size() != opt.dimension) {
      if (reason) *reason = "expected dimension " + std::to_string(opt.dimension) + ", got " + std::to_string(v.size());
      return false;
    }
    for (float f : v) {
      if (!std::isfinite(f)) {
        if (reason) *reason = "vector contains NaN or infinity";
        return false;
      }
    }
    return true;
  }

  std::string node_payload(const Node& n, const std::string& op) const {
    return op + "\t" + std::to_string(n.id) + "\t" + std::to_string(n.created_version) + "\t" +
           std::to_string(n.deleted_version) + "\t" + std::to_string(n.signature) + "\t" +
           std::to_string(n.incident) + "\t" + bool_s(n.root) + "\t" + bool_s(n.symptom) + "\t" +
           bool_s(n.impact) + "\t" + hex_encode(n.content) + "\t" + serialize_vector(n.vector) + "\t" +
           serialize_lattice(n.lattice) + "\t" + std::to_string(static_cast<int>(n.defect_type)) + "\t" +
           serialize_metadata(n.metadata);
  }

  std::string edge_payload(const Edge& e, const std::string& op) const {
    return op + "\t" + std::to_string(e.id) + "\t" + std::to_string(e.from) + "\t" +
           std::to_string(e.to) + "\t" + std::to_string(static_cast<int>(e.origin)) + "\t" +
           std::to_string(static_cast<int>(e.role)) + "\t" + std::to_string(e.confidence) + "\t" +
           std::to_string(e.created_version) + "\t" + std::to_string(e.deleted_version) + "\t" +
           std::to_string(static_cast<int>(e.bond_type)) + "\t" +
           std::to_string(static_cast<int>(e.defect_type)) + "\t" +
           std::to_string(static_cast<int>(e.layer_coupling)) + "\t" +
           std::to_string(e.bond_strength) + "\t" +
           serialize_metadata(e.metadata);
  }

  Status apply_payload(const std::string& payload, bool from_replay, std::unordered_map<uint64_t, std::vector<std::string>>* pending = nullptr, std::unordered_set<uint64_t>* committed = nullptr) {
    (void)from_replay;
    auto p = split_tab(payload);
    if (p.empty()) return Status::error(ErrorCode::DataCorrupt, "empty payload");
    const auto& op = p[0];
    try {
      if (op == "BEGIN") {
        if (p.size() != 2 || !pending) return Status::error(ErrorCode::WalCorrupt, "bad BEGIN");
        uint64_t t = std::stoull(p[1]);
        (*pending)[t] = {};
        return Status::ok();
      }
      if (op == "COMMIT") {
        if (p.size() != 2 || !pending || !committed) return Status::error(ErrorCode::WalCorrupt, "bad COMMIT");
        uint64_t t = std::stoull(p[1]);
        auto it = pending->find(t);
        if (it == pending->end()) return Status::error(ErrorCode::TransactionIncomplete, "commit without begin");
        for (const auto& rec : it->second) {
          auto st = apply_payload(rec, true, nullptr, nullptr);
          if (!st) return st;
        }
        committed->insert(t);
        pending->erase(it);
        return Status::ok();
      }
      if (pending && op != "BEGIN" && op != "COMMIT") {
        if (p.size() < 2) return Status::error(ErrorCode::WalCorrupt, "transactional record missing txid");
        uint64_t t = std::stoull(p[1]);
        auto it = pending->find(t);
        if (it == pending->end()) return Status::error(ErrorCode::TransactionIncomplete, "record without begin");
        std::string without_tx;
        auto first_tab = payload.find('\t');
        auto second_tab = payload.find('\t', first_tab + 1);
        if (second_tab == std::string::npos) return Status::error(ErrorCode::WalCorrupt, "bad transactional record");
        without_tx = payload.substr(0, first_tab) + payload.substr(second_tab);
        it->second.push_back(without_tx);
        return Status::ok();
      }
      if (op == "PUT_NODE" || op == "DATA_NODE") {
        if (p.size() != 12 && p.size() != 14) return Status::error(ErrorCode::DataCorrupt, "bad node record field count");
        Node n;
        n.id = static_cast<uint32_t>(std::stoul(p[1]));
        n.created_version = std::stoull(p[2]);
        n.deleted_version = std::stoull(p[3]);
        n.signature = std::stoull(p[4]);
        n.incident = static_cast<uint32_t>(std::stoul(p[5]));
        n.root = p[6] == "1";
        n.symptom = p[7] == "1";
        n.impact = p[8] == "1";
        if (!hex_decode(p[9], &n.content)) return Status::error(ErrorCode::DataCorrupt, "bad content encoding");
        if (!parse_vector(p[10], &n.vector)) return Status::error(ErrorCode::DataCorrupt, "bad vector encoding");
        size_t metadata_field = 11;
        if (p.size() == 14) {
          if (!parse_lattice(p[11], &n.lattice)) return Status::error(ErrorCode::DataCorrupt, "bad lattice encoding");
          n.defect_type = static_cast<DefectType>(std::stoi(p[12]));
          metadata_field = 13;
        }
        if (!parse_metadata(p[metadata_field], &n.metadata)) return Status::error(ErrorCode::DataCorrupt, "bad metadata encoding");
        std::string reason;
        if (!vector_valid(n.vector, &reason)) return Status::error(ErrorCode::DimensionMismatch, reason);
        ensure_node_slot(n.id);
        nodes[n.id] = std::move(n);
        return Status::ok();
      }
      if (op == "PUT_EDGE" || op == "DATA_EDGE") {
        if (p.size() != 10 && p.size() != 14) return Status::error(ErrorCode::DataCorrupt, "bad edge record field count");
        Edge e;
        e.id = static_cast<uint32_t>(std::stoul(p[1]));
        e.from = static_cast<uint32_t>(std::stoul(p[2]));
        e.to = static_cast<uint32_t>(std::stoul(p[3]));
        e.origin = static_cast<EdgeOrigin>(std::stoi(p[4]));
        e.role = static_cast<EdgeRole>(std::stoi(p[5]));
        e.confidence = std::stod(p[6]);
        e.created_version = std::stoull(p[7]);
        e.deleted_version = std::stoull(p[8]);
        size_t metadata_field = 9;
        if (p.size() == 14) {
          e.bond_type = static_cast<BondType>(std::stoi(p[9]));
          e.defect_type = static_cast<DefectType>(std::stoi(p[10]));
          e.layer_coupling = static_cast<LayerCoupling>(std::stoi(p[11]));
          e.bond_strength = std::stod(p[12]);
          metadata_field = 13;
        }
        if (!parse_metadata(p[metadata_field], &e.metadata)) return Status::error(ErrorCode::DataCorrupt, "bad edge metadata");
        if (e.from >= nodes.size() || e.to >= nodes.size()) return Status::error(ErrorCode::EdgeInvalid, "edge endpoint missing during replay");
        ensure_edge_slot(e.id);
        edges[e.id] = std::move(e);
        return Status::ok();
      }
      if (op == "DELETE_NODE") {
        if (p.size() != 3) return Status::error(ErrorCode::DataCorrupt, "bad delete node record");
        uint32_t id = static_cast<uint32_t>(std::stoul(p[1]));
        uint64_t v = std::stoull(p[2]);
        if (id >= nodes.size()) return Status::error(ErrorCode::NodeNotFound, "delete missing node during replay");
        nodes[id].deleted_version = v;
        return Status::ok();
      }
      return Status::error(ErrorCode::DataCorrupt, "unknown op: " + op);
    } catch (const std::exception& e) {
      return Status::error(ErrorCode::DataCorrupt, std::string("parse error: ") + e.what());
    }
  }

  Status load_framed_file(const fs::path& path, bool transactional, bool stop_on_bad_frame) {
    if (!fs::exists(path)) return Status::ok();
    std::ifstream in(path, std::ios::binary);
    if (!in) return Status::error(ErrorCode::IoError, "cannot read " + path.string());
    const std::string bytes((std::istreambuf_iterator<char>(in)), std::istreambuf_iterator<char>());
    std::unordered_map<uint64_t, std::vector<std::string>> pending;
    std::unordered_set<uint64_t> committed;
    size_t offset = 0;
    size_t last_valid_offset = 0;
    while (offset < bytes.size()) {
      const size_t newline = bytes.find('\n', offset);
      const bool terminated = newline != std::string::npos;
      const size_t end = terminated ? newline : bytes.size();
      const bool final_fragment = !terminated;
      std::string line = bytes.substr(offset, end - offset);
      offset = terminated ? newline + 1 : bytes.size();
      // Checkpoint/data files written through a Windows text stream use CRLF;
      // the frame format itself defines the terminator, not a payload byte.
      if (terminated && !line.empty() && line.back() == '\r') line.pop_back();
      if (line.empty()) continue;
      if (stop_on_bad_frame && final_fragment) {
        std::error_code ec;
        fs::resize_file(path, last_valid_offset, ec);
        if (ec) return Status::error(ErrorCode::IoError, "cannot truncate torn WAL tail: " + ec.message());
        auto flush_status = platform::flush_path(path);
        if (!flush_status) return Status::error(flush_status.code, "cannot flush repaired WAL: " + flush_status.message);
        break;
      }
      std::string payload;
      if (!parse_frame_line(line, &payload)) {
        return Status::error(
          transactional ? ErrorCode::WalCorrupt : ErrorCode::DataCorrupt,
          "corrupt complete frame in " + path.string());
      }
      Status st = transactional ? apply_payload(payload, true, &pending, &committed)
                                : apply_payload(payload, true, nullptr, nullptr);
      if (!st) return st;
      last_valid_offset = offset;
    }
    // Pending transactions are intentionally ignored: no COMMIT means no durability.
    return Status::ok();
  }

  Status write_manifest() const {
    std::ostringstream os;
    os << "graphenedb_manifest_v" << kManifestFormatVersion << "\n";
    os << "storage_format=" << kStorageFormatVersion << "\n";
    os << "wal_frame_format=" << kWalFrameFormatVersion << "\n";
    os << "lattice_format=" << kLatticeFormatVersion << "\n";
    os << "physical_lattice_storage=" << (opt.physical_lattice_storage ? 1 : 0) << "\n";
    os << "physical_lattice_primary=" << (opt.physical_lattice_primary ? 1 : 0) << "\n";
    os << "physical_lattice_radius=" << opt.physical_lattice_radius << "\n";
    os << "extraction_format=" << kExtractionFormatVersion << "\n";
    os << "dimension=" << opt.dimension << "\n";
    os << "version=" << version << "\n";
    os << "next_node_id=" << next_node_id << "\n";
    os << "next_edge_id=" << next_edge_id << "\n";
    os << "next_txid=" << txid << "\n";
    std::string body = os.str();
    body += "checksum=" + std::to_string(fnv1a(body)) + "\n";
    fs::path tmp = manifest_path.string() + ".tmp";
    std::ofstream out(tmp, std::ios::trunc);
    if (!out) return Status::error(ErrorCode::IoError, "cannot write manifest tmp");
    out << body;
    out.flush();
    if (!out) return Status::error(ErrorCode::IoError, "cannot flush manifest tmp");
    out.close();
    if (env_enabled("GRAPHENEDB_TEST_FAIL_MANIFEST_RENAME")) {
      return Status::error(ErrorCode::IoError, "injected manifest rename failure");
    }
    auto replace_status = platform::durable_replace(tmp, manifest_path);
    if (!replace_status) return Status::error(replace_status.code, "manifest replace failed: " + replace_status.message);
    return replace_status;
  }

  Status read_manifest(uint32_t* dim, uint64_t* next_txid) const {
    if (!fs::exists(manifest_path)) return Status::ok();
    std::ifstream in(manifest_path);
    if (!in) return Status::error(ErrorCode::IoError, "cannot read manifest");
    std::string line;
    std::string body;
    std::string checksum_text;
    bool saw_header = false;
    bool saw_dimension = false;
    bool saw_next_txid = false;
    auto parse_u64 = [](const std::string& text, uint64_t* out) {
      size_t pos = 0;
      uint64_t v = std::stoull(text, &pos);
      if (pos != text.size()) throw std::invalid_argument("trailing characters");
      *out = v;
    };
    while (std::getline(in, line)) {
      if (line.empty()) continue;
      if (line.rfind("checksum=", 0) == 0) {
        checksum_text = line.substr(9);
        continue;
      }
      body += line + "\n";
      if (!saw_header) {
        if (line != "graphenedb_manifest_v" + std::to_string(kManifestFormatVersion)) {
          return Status::error(ErrorCode::DataCorrupt, "bad manifest header");
        }
        saw_header = true;
        continue;
      }
      try {
        if (line.rfind("storage_format=", 0) == 0) {
          uint64_t v = 0; parse_u64(line.substr(15), &v);
          if (v > kStorageFormatVersion) return Status::error(ErrorCode::UnsupportedMode, "unsupported storage format");
        } else if (line.rfind("wal_frame_format=", 0) == 0) {
          uint64_t v = 0; parse_u64(line.substr(17), &v);
          if (v > kWalFrameFormatVersion) return Status::error(ErrorCode::UnsupportedMode, "unsupported WAL frame format");
        } else if (line.rfind("lattice_format=", 0) == 0) {
          uint64_t v = 0; parse_u64(line.substr(15), &v);
          if (v > kLatticeFormatVersion) return Status::error(ErrorCode::UnsupportedMode, "unsupported lattice format");
        } else if (line.rfind("physical_lattice_storage=", 0) == 0) {
          uint64_t ignored = 0; parse_u64(line.substr(25), &ignored);
        } else if (line.rfind("extraction_format=", 0) == 0) {
          uint64_t v = 0; parse_u64(line.substr(18), &v);
          if (v > kExtractionFormatVersion) return Status::error(ErrorCode::UnsupportedMode, "unsupported extraction format");
        } else if (line.rfind("dimension=", 0) == 0) {
          uint64_t v = 0; parse_u64(line.substr(10), &v);
          if (v > UINT32_MAX) return Status::error(ErrorCode::DataCorrupt, "manifest dimension out of range");
          *dim = static_cast<uint32_t>(v);
          saw_dimension = true;
        } else if (line.rfind("next_txid=", 0) == 0) {
          uint64_t v = 0; parse_u64(line.substr(10), &v);
          if (next_txid) *next_txid = v;
          saw_next_txid = true;
        } else if (line.rfind("version=", 0) == 0 || line.rfind("next_node_id=", 0) == 0 || line.rfind("next_edge_id=", 0) == 0) {
          uint64_t ignored = 0;
          parse_u64(line.substr(line.find('=') + 1), &ignored);
        }
      } catch (const std::exception& e) {
        return Status::error(ErrorCode::DataCorrupt, std::string("bad manifest numeric field: ") + e.what());
      }
    }
    if (!saw_header) return Status::error(ErrorCode::DataCorrupt, "missing manifest header");
    if (checksum_text.empty()) return Status::error(ErrorCode::DataCorrupt, "missing manifest checksum");
    try {
      uint64_t expected = 0;
      parse_u64(checksum_text, &expected);
      if (expected != fnv1a(body)) return Status::error(ErrorCode::DataCorrupt, "manifest checksum mismatch");
    } catch (const std::exception& e) {
      return Status::error(ErrorCode::DataCorrupt, std::string("bad manifest checksum: ") + e.what());
    }
    if (!saw_dimension) return Status::error(ErrorCode::DataCorrupt, "missing manifest dimension");
    if (!saw_next_txid) return Status::error(ErrorCode::DataCorrupt, "missing manifest next_txid");
    return Status::ok();
  }

  Status acquire_lock() {
    std::error_code ec;
    if (fs::exists(lock_path, ec)) {
      bool removed_stale = false;
      if (opt.recover_stale_lock) {
        uint64_t pid = 0;
        {
          std::ifstream in(lock_path);
          in >> pid;
        }
        bool stale_same_pid_restart = false;
        if (pid == platform::current_pid()) {
          const auto started_at = platform::current_process_start_time();
          const auto lock_mtime = fs::last_write_time(lock_path, ec);
          if (!ec && started_at.has_value()) {
            const auto lock_sys = std::chrono::time_point_cast<std::chrono::system_clock::duration>(
                lock_mtime - fs::file_time_type::clock::now() + std::chrono::system_clock::now());
            stale_same_pid_restart = lock_sys < *started_at;
          }
        }
        if (stale_same_pid_restart || (pid > 1 && !platform::process_is_alive(pid))) {
          fs::remove(lock_path, ec);
          removed_stale = !fs::exists(lock_path, ec);
        }
      }
      if (!removed_stale && fs::exists(lock_path, ec)) {
        return Status::error(ErrorCode::LockBusy, "database lock exists: " + lock_path.string());
      }
    }
    auto create_status = platform::create_file_exclusive(
      lock_path, std::to_string(platform::current_pid()) + "\n");
    if (!create_status && create_status.code == ErrorCode::LockBusy) {
      return Status::error(ErrorCode::LockBusy, "database lock exists: " + lock_path.string());
    }
    return create_status;
  }

  void release_lock() {
    if (!lock_path.empty()) {
      std::error_code ec;
      fs::remove(lock_path, ec);
    }
  }

  Path reverse_root(uint32_t anchor, QueryMode mode, uint64_t snap) const {
    Path path;
    if (anchor >= nodes.size() || !visible_node(nodes[anchor], snap)) return path;
    std::vector<uint32_t> queue{anchor};
    std::unordered_map<uint32_t, uint32_t> parent_node;
    std::unordered_map<uint32_t, uint32_t> parent_edge;
    std::unordered_set<uint32_t> seen{anchor};
    uint32_t root = UINT32_MAX;
    for (size_t i = 0; i < queue.size() && i < opt.tuning.max_reverse_bfs_visits; ++i) {
      uint32_t cur = queue[i];
      if (nodes[cur].root) { root = cur; break; }
      auto it = in_edges.find(cur);
      if (it == in_edges.end()) continue;
      for (uint32_t eid : it->second) {
        if (eid >= edges.size() || !visible_edge(edges[eid], nodes, snap)) continue;
        const auto& e = edges[eid];
        if (!edge_allowed_for_mode(e, mode)) continue;
        if (!seen.count(e.from)) {
          seen.insert(e.from);
          parent_node[e.from] = cur;
          parent_edge[e.from] = eid;
          queue.push_back(e.from);
        }
      }
    }
    if (root == UINT32_MAX) return path;
    uint32_t cur = root;
    path.nodes.push_back(cur);
    while (cur != anchor) {
      uint32_t eid = parent_edge.at(cur);
      path.edges.push_back(eid);
      const auto& e = edges[eid];
      path.contains_hypothetical |= (e.origin == EdgeOrigin::Hypothetical);
      path.contains_contradiction |= (e.role == EdgeRole::Contradicts);
      path.score *= std::max(0.0, std::min(1.0, e.confidence));
      cur = parent_node.at(cur);
      path.nodes.push_back(cur);
    }
    return path;
  }

  std::unordered_map<uint32_t, double> propagate_lattice_activation(const std::vector<SearchResult>& anchors, size_t inspect, QueryMode mode, uint64_t snap) const {
    std::unordered_map<uint32_t, double> activation;
    if (!opt.tuning.enable_lattice_retrieval || opt.tuning.lattice_max_hops == 0) return activation;
    struct State { uint32_t node; uint32_t depth; double score; };
    std::vector<State> queue;
    for (size_t i = 0; i < inspect && i < anchors.size(); ++i) {
      if (anchors[i].node_id >= nodes.size() || !visible_node(nodes[anchors[i].node_id], snap)) continue;
      activation[anchors[i].node_id] = std::max(activation[anchors[i].node_id], anchors[i].score);
      queue.push_back({anchors[i].node_id, 0, anchors[i].score});
    }
    for (size_t qi = 0; qi < queue.size(); ++qi) {
      const auto cur = queue[qi];
      if (cur.depth >= opt.tuning.lattice_max_hops) continue;
      if (dense_lattice_enabled()) {
        auto ord_it = dense_lattice_ordinal_by_node.find(cur.node);
        if (ord_it == dense_lattice_ordinal_by_node.end()) continue;
        const auto& cell = dense_lattice_cells[ord_it->second];
        for (uint32_t next : cell.neighbor_nodes) {
          if (next == UINT32_MAX || next >= nodes.size() || !visible_node(nodes[next], snap)) continue;
          double best_strength = 0.85; // Physical adjacency path when no explicit bond exists.
          bool blocked_by_mode = false;
          for (uint32_t eid : cell.lattice_edge_ids) {
            if (eid >= edges.size() || !visible_edge(edges[eid], nodes, snap)) continue;
            const auto& e = edges[eid];
            if (!((e.from == cur.node && e.to == next) || (e.to == cur.node && e.from == next))) continue;
            if (!edge_allowed_for_mode(e, mode)) { blocked_by_mode = true; continue; }
            best_strength = std::max(best_strength, e.bond_strength);
          }
          if (blocked_by_mode && best_strength <= 0.85) continue;
          double score = cur.score * opt.tuning.lattice_decay * best_strength;
          if (nodes[cur.node].lattice && nodes[next].lattice && nodes[cur.node].lattice->layer != nodes[next].lattice->layer) score *= opt.tuning.lattice_cross_layer_penalty;
          if (score <= 0.0) continue;
          auto prev = activation.find(next);
          if (prev == activation.end() || score > prev->second) {
            activation[next] = score;
            queue.push_back({next, cur.depth + 1, score});
          }
        }
        continue;
      }
      auto it = lattice_edges.find(cur.node);
      if (it == lattice_edges.end()) continue;
      for (uint32_t eid : it->second) {
        if (eid >= edges.size() || !visible_edge(edges[eid], nodes, snap)) continue;
        const auto& e = edges[eid];
        if (!edge_allowed_for_mode(e, mode)) continue;
        uint32_t next = e.from == cur.node ? e.to : e.from;
        double penalty = 1.0;
        if (e.bond_type == BondType::Defect || e.defect_type != DefectType::None) penalty *= opt.tuning.lattice_defect_penalty;
        if (nodes[e.from].lattice && nodes[e.to].lattice && nodes[e.from].lattice->layer != nodes[e.to].lattice->layer) penalty *= opt.tuning.lattice_cross_layer_penalty;
        double score = cur.score * opt.tuning.lattice_decay * e.bond_strength * penalty;
        if (score <= 0.0) continue;
        auto prev = activation.find(next);
        if (prev == activation.end() || score > prev->second) {
          activation[next] = score;
          queue.push_back({next, cur.depth + 1, score});
        }
      }
    }
    return activation;
  }
};

GrapheneDB::GrapheneDB() : impl_(new Impl()) {}
GrapheneDB::~GrapheneDB() { close(); }

Status GrapheneDB::open(const fs::path& path, const DBOptions& options) {
  std::unique_lock lock(impl_->mu);
  if (impl_->open) return Status::ok();
  if (options.dimension == 0) return Status::error(ErrorCode::InvalidOption, "dimension must be greater than zero");
  impl_->dir = path;
  impl_->wal_path = path / "graphene.wal";
  impl_->data_path = path / "graphene.data";
  impl_->manifest_path = path / "MANIFEST";
  impl_->physical_lattice_path = path / "graphene.lattice";
  impl_->physical_lattice_bin_path = path / "graphene.lattice.bin";
  impl_->nodeidx_path = path / "graphene.nodeidx";
  impl_->lock_path = path / "LOCK";
  impl_->opt = options;
  auto vst = impl_->validate_vector_index_support();
  if (!vst) return vst;
  std::error_code ec;
  const bool directory_exists = fs::exists(path, ec);
  if (ec) return Status::error(ErrorCode::IoError, "cannot inspect DB dir: " + ec.message());
  if (!directory_exists && !options.create_if_missing) {
    return Status::error(ErrorCode::InvalidOption, "database directory does not exist and create_if_missing is false");
  }
  if (directory_exists && !fs::is_directory(path, ec)) {
    return Status::error(ErrorCode::InvalidOption, "database path is not a directory");
  }
  if (!directory_exists) {
    fs::create_directories(path, ec);
    if (ec) return Status::error(ErrorCode::IoError, "cannot create DB dir: " + ec.message());
  }
  uint32_t manifest_dim = 0;
  uint64_t manifest_txid = 1;
  auto mst = impl_->read_manifest(&manifest_dim, &manifest_txid);
  if (!mst) return mst;
  if (manifest_dim != 0 && manifest_dim != options.dimension) {
    return Status::error(ErrorCode::DimensionMismatch, "existing DB dimension " + std::to_string(manifest_dim) + " does not match requested " + std::to_string(options.dimension));
  }
  auto lst = impl_->acquire_lock();
  if (!lst) return lst;
  // Interrupted checkpoints may leave complete or partial temporary files.
  // They are never authoritative. Quarantine them after taking the exclusive
  // database lock so operators can inspect them without risking reuse.
  {
    const std::vector<fs::path> stale_temps = {
      fs::path(impl_->data_path.string() + ".tmp"),
      fs::path(impl_->physical_lattice_path.string() + ".tmp"),
      fs::path(impl_->physical_lattice_bin_path.string() + ".tmp"),
      fs::path(impl_->nodeidx_path.string() + ".tmp"),
      fs::path(impl_->manifest_path.string() + ".tmp")
    };
    fs::path quarantine = path / "recovery";
    for (const auto& tmp : stale_temps) {
      std::error_code tec;
      if (!fs::exists(tmp, tec) || tec) continue;
      fs::create_directories(quarantine, tec);
      if (tec) { impl_->release_lock(); return Status::error(ErrorCode::IoError, "cannot create recovery quarantine: " + tec.message()); }
      const auto stamp = std::to_string(platform::current_pid()) + "-" + std::to_string(std::chrono::steady_clock::now().time_since_epoch().count());
      fs::path dest = quarantine / (tmp.filename().string() + ".orphan-" + stamp);
      fs::rename(tmp, dest, tec);
      if (tec) { impl_->release_lock(); return Status::error(ErrorCode::IoError, "cannot quarantine stale temp file " + tmp.string() + ": " + tec.message()); }
    }
  }
  impl_->nodes.clear(); impl_->edges.clear(); impl_->version = 1; impl_->txid = std::max<uint64_t>(1, manifest_txid); impl_->live_node_count = 0; impl_->live_edge_count = 0; impl_->vector_index.reset(); impl_->maintenance_warning.clear();
  auto dst = impl_->load_framed_file(impl_->data_path, false, false);
  if (!dst) { impl_->release_lock(); return dst; }
  auto wst = impl_->load_framed_file(impl_->wal_path, true, true);
  if (!wst) { impl_->release_lock(); return wst; }
  impl_->rebuild_indexes();
  if (impl_->opt.physical_lattice_primary) {
    auto pst = impl_->write_physical_lattice_binary_unlocked(impl_->dense_lattice_cells);
    if (!pst) { impl_->release_lock(); return pst; }
  } else if (impl_->opt.physical_lattice_storage) {
    auto pst = impl_->write_physical_lattice_unlocked();
    if (!pst) { impl_->release_lock(); return pst; }
  }
  auto ow = impl_->open_wal_fd_unlocked();
  if (!ow) { impl_->release_lock(); return ow; }
  auto wm = impl_->write_manifest();
  if (!wm) { impl_->close_wal_fd_unlocked(); impl_->release_lock(); return wm; }
  impl_->open = true;
  return Status::ok();
}

Status GrapheneDB::close() {
  std::unique_lock lock(impl_->mu);
  if (!impl_->open) return Status::ok();
  auto st = impl_->write_manifest();
  impl_->close_wal_fd_unlocked();
  impl_->open = false;
  impl_->release_lock();
  return st;
}

bool GrapheneDB::is_open() const { std::shared_lock lock(impl_->mu); return impl_->open; }
uint64_t GrapheneDB::snapshot() const { std::shared_lock lock(impl_->mu); return impl_->version ? impl_->version - 1 : 0; }
uint32_t GrapheneDB::dimension() const { std::shared_lock lock(impl_->mu); return impl_->opt.dimension; }

Status GrapheneDB::put_node(const NodeInput& input, uint32_t* out_id) {
  std::unique_lock lock(impl_->mu);
  if (!impl_->open) return Status::error(ErrorCode::NotOpen, "database is not open");
  std::string reason;
  if (!impl_->vector_valid(input.vector, &reason)) return Status::error(ErrorCode::DimensionMismatch, reason);
  auto lst = impl_->validate_lattice_node_input(input);
  if (!lst) return lst;
  if (input.content.size() > 16 * 1024 * 1024) return Status::error(ErrorCode::InvalidInput, "content too large");
  Node n;
  n.id = impl_->next_node_id;
  n.content = input.content;
  n.vector = input.vector;
  n.signature = input.signature;
  n.incident = input.incident;
  n.root = input.root; n.symptom = input.symptom; n.impact = input.impact;
  n.lattice = input.lattice;
  n.defect_type = input.defect_type;
  n.created_version = impl_->version;
  n.metadata = input.metadata;
  uint64_t tx = impl_->txid;
  std::vector<std::string> frames = {
    "BEGIN\t" + std::to_string(tx),
    "PUT_NODE\t" + std::to_string(tx) + "\t" + impl_->node_payload(n, "PUT_NODE").substr(std::string("PUT_NODE\t").size()),
    "COMMIT\t" + std::to_string(tx)
  };
  auto st = impl_->append_wal_unlocked(frames);
  if (!st) return st;
  ++impl_->next_node_id;
  ++impl_->version;
  ++impl_->txid;
  impl_->apply_committed_node(n);
  if (out_id) *out_id = n.id;
  Status plst = impl_->opt.physical_lattice_primary ? impl_->write_incremental_physical_lattice_node_unlocked(n) : impl_->write_physical_lattice_unlocked();
  if (!plst) impl_->note_maintenance_failure("physical lattice maintenance", plst);
  auto rst = impl_->maybe_rotate_wal_unlocked();
  if (!rst) impl_->note_maintenance_failure("automatic WAL rotation", rst);
  return Status::ok();
}

Status GrapheneDB::put_edge(const EdgeInput& input, uint32_t* out_id) {
  std::unique_lock lock(impl_->mu);
  if (!impl_->open) return Status::error(ErrorCode::NotOpen, "database is not open");
  uint64_t snap = impl_->version - 1;
  if (input.from >= impl_->nodes.size() || input.to >= impl_->nodes.size() || !visible_node(impl_->nodes[input.from], snap) || !visible_node(impl_->nodes[input.to], snap)) {
    return Status::error(ErrorCode::EdgeInvalid, "edge endpoints must exist and be visible");
  }
  if (!std::isfinite(input.confidence) || input.confidence < 0.0 || input.confidence > 1.0) return Status::error(ErrorCode::InvalidInput, "confidence must be between 0 and 1");
  auto lst = impl_->validate_lattice_edge_input(input, snap);
  if (!lst) return lst;
  Edge e;
  e.id = impl_->next_edge_id;
  e.from = input.from; e.to = input.to; e.origin = input.origin; e.role = input.role; e.confidence = input.confidence;
  e.bond_type = input.bond_type; e.defect_type = input.defect_type; e.layer_coupling = input.layer_coupling; e.bond_strength = input.bond_strength;
  e.created_version = impl_->version;
  e.metadata = input.metadata;
  uint64_t tx = impl_->txid;
  std::vector<std::string> frames = {"BEGIN\t" + std::to_string(tx), "PUT_EDGE\t" + std::to_string(tx) + "\t" + impl_->edge_payload(e, "PUT_EDGE").substr(std::string("PUT_EDGE\t").size()), "COMMIT\t" + std::to_string(tx)};
  auto st = impl_->append_wal_unlocked(frames);
  if (!st) return st;
  ++impl_->next_edge_id;
  ++impl_->version;
  ++impl_->txid;
  impl_->apply_committed_edge(e);
  if (out_id) *out_id = e.id;
  auto plst = impl_->write_physical_lattice_unlocked();
  if (!plst) impl_->note_maintenance_failure("physical lattice maintenance", plst);
  auto rst = impl_->maybe_rotate_wal_unlocked();
  if (!rst) impl_->note_maintenance_failure("automatic WAL rotation", rst);
  return Status::ok();
}

Status GrapheneDB::put_batch(const BatchInput& input, BatchResult* out) {
  std::unique_lock lock(impl_->mu);
  if (!impl_->open) return Status::error(ErrorCode::NotOpen, "database is not open");
  if (input.nodes.empty() && input.edges.empty()) {
    if (out) *out = {};
    return Status::ok();
  }

  std::vector<Node> new_nodes;
  std::vector<Edge> new_edges;
  new_nodes.reserve(input.nodes.size());
  new_edges.reserve(input.edges.size());
  std::unordered_set<uint32_t> batch_node_ids;
  std::unordered_set<std::string> batch_lattice_keys;

  auto lattice_key = [](const LatticeCoord& c) {
    return std::to_string(c.q) + "," + std::to_string(c.r) + "," + std::to_string(c.layer);
  };

  uint32_t next_node = impl_->next_node_id;
  uint64_t next_version = impl_->version;
  for (const auto& ni : input.nodes) {
    std::string reason;
    if (!impl_->vector_valid(ni.vector, &reason)) return Status::error(ErrorCode::DimensionMismatch, reason);
    if (ni.content.size() > 16 * 1024 * 1024) return Status::error(ErrorCode::InvalidInput, "content too large");
    if (impl_->opt.require_lattice && !ni.lattice) return Status::error(ErrorCode::InvalidInput, "lattice coordinate required");
    if (ni.lattice) {
      if (impl_->lattice_nodes.find(*ni.lattice) != impl_->lattice_nodes.end()) return Status::error(ErrorCode::InvalidInput, "duplicate lattice coordinate");
      if (!batch_lattice_keys.insert(lattice_key(*ni.lattice)).second) return Status::error(ErrorCode::InvalidInput, "duplicate lattice coordinate in batch");
      if (impl_->opt.physical_lattice_primary) {
        uint64_t ignored = 0;
        auto pst = impl_->physical_absolute_ordinal_unlocked(*ni.lattice, &ignored);
        if (!pst) return pst;
      }
    }
    Node n;
    n.id = next_node++;
    n.content = ni.content;
    n.vector = ni.vector;
    n.signature = ni.signature;
    n.incident = ni.incident;
    n.root = ni.root; n.symptom = ni.symptom; n.impact = ni.impact;
    n.metadata = ni.metadata;
    n.lattice = ni.lattice;
    n.defect_type = ni.defect_type;
    n.created_version = next_version++;
    new_nodes.push_back(n);
    batch_node_ids.insert(n.id);
  }

  auto node_visible_in_batch = [&](uint32_t id) {
    uint64_t snap = impl_->version - 1;
    if (id < impl_->nodes.size() && visible_node(impl_->nodes[id], snap)) return true;
    return batch_node_ids.count(id) > 0;
  };
  auto node_for_batch = [&](uint32_t id) -> const Node* {
    if (id < impl_->nodes.size()) {
      uint64_t snap = impl_->version - 1;
      if (visible_node(impl_->nodes[id], snap)) return &impl_->nodes[id];
    }
    for (const auto& n : new_nodes) if (n.id == id) return &n;
    return nullptr;
  };

  uint32_t next_edge = impl_->next_edge_id;
  for (const auto& ei : input.edges) {
    if (!node_visible_in_batch(ei.from) || !node_visible_in_batch(ei.to)) {
      return Status::error(ErrorCode::EdgeInvalid, "edge endpoints must exist and be visible");
    }
    if (!std::isfinite(ei.confidence) || ei.confidence < 0.0 || ei.confidence > 1.0) return Status::error(ErrorCode::InvalidInput, "confidence must be between 0 and 1");
    if (!std::isfinite(ei.bond_strength) || ei.bond_strength < 0.0 || ei.bond_strength > 1.0) return Status::error(ErrorCode::InvalidInput, "bond_strength must be between 0 and 1");
    if (is_lattice_bond(ei.bond_type)) {
      const Node* from = node_for_batch(ei.from);
      const Node* to = node_for_batch(ei.to);
      if (!from || !to || !from->lattice || !to->lattice) return Status::error(ErrorCode::EdgeInvalid, "lattice bond requires lattice coordinates on both endpoints");
      if (ei.bond_type == BondType::VanDerWaals) {
        if (!are_cross_layer_neighbors(*from->lattice, *to->lattice, ei.layer_coupling)) return Status::error(ErrorCode::EdgeInvalid, "VanDerWaals bond requires valid cross-layer neighbor and coupling");
      } else if (ei.bond_type != BondType::Defect && ei.bond_type != BondType::Synthetic) {
        if (!are_same_layer_hex_neighbors(*from->lattice, *to->lattice)) return Status::error(ErrorCode::EdgeInvalid, "Sigma/Pi bond requires same-layer hex-neighbor coordinates");
      }
    }
    Edge e;
    e.id = next_edge++;
    e.from = ei.from; e.to = ei.to; e.origin = ei.origin; e.role = ei.role; e.confidence = ei.confidence;
    e.metadata = ei.metadata;
    e.bond_type = ei.bond_type; e.defect_type = ei.defect_type; e.layer_coupling = ei.layer_coupling; e.bond_strength = ei.bond_strength;
    e.created_version = next_version++;
    new_edges.push_back(e);
  }

  uint64_t tx = impl_->txid;
  std::vector<std::string> frames;
  frames.reserve(2 + new_nodes.size() + new_edges.size());
  frames.push_back("BEGIN\t" + std::to_string(tx));
  for (const auto& n : new_nodes) frames.push_back("PUT_NODE\t" + std::to_string(tx) + "\t" + impl_->node_payload(n, "PUT_NODE").substr(std::string("PUT_NODE\t").size()));
  for (const auto& e : new_edges) frames.push_back("PUT_EDGE\t" + std::to_string(tx) + "\t" + impl_->edge_payload(e, "PUT_EDGE").substr(std::string("PUT_EDGE\t").size()));
  frames.push_back("COMMIT\t" + std::to_string(tx));
  auto st = impl_->append_wal_unlocked(frames);
  if (!st) return st;

  ++impl_->txid;
  impl_->next_node_id = next_node;
  impl_->next_edge_id = next_edge;
  impl_->version = next_version;
  for (const auto& n : new_nodes) impl_->apply_committed_node(n);
  for (const auto& e : new_edges) impl_->apply_committed_edge(e);
  if (out) {
    out->node_ids.clear();
    out->edge_ids.clear();
    for (const auto& n : new_nodes) out->node_ids.push_back(n.id);
    for (const auto& e : new_edges) out->edge_ids.push_back(e.id);
  }
  auto plst = impl_->write_physical_lattice_unlocked();
  if (!plst) impl_->note_maintenance_failure("physical lattice maintenance", plst);
  auto rst = impl_->maybe_rotate_wal_unlocked();
  if (!rst) impl_->note_maintenance_failure("automatic WAL rotation", rst);
  return Status::ok();
}

Status GrapheneDB::put_extraction(const ExtractionInput& input, ExtractionResult* out) {
  std::unique_lock lock(impl_->mu);
  if (!impl_->open) return Status::error(ErrorCode::NotOpen, "database is not open");
  if (input.schema_version != kExtractionSchemaVersion) return Status::error(ErrorCode::UnsupportedMode, "unsupported extraction schema version");
  if (input.source_id.empty()) return Status::error(ErrorCode::InvalidInput, "source_id is required");
  if (input.nodes.empty() && input.relations.empty()) {
    if (out) *out = {};
    return Status::ok();
  }

  auto scoped_external = [&](const std::string& external_id) {
    return input.source_id + "\x1e" + external_id;
  };
  auto metadata_key = [](const std::string& key, const std::string& value) {
    return key + "\x1f" + value;
  };
  uint64_t snap = impl_->version - 1;
  auto find_existing_external = [&](const std::string& scoped) -> std::optional<uint32_t> {
    auto it = impl_->metadata_index.find(metadata_key("graphene_scoped_external_id", scoped));
    if (it == impl_->metadata_index.end()) return std::nullopt;
    for (uint32_t id : it->second) {
      if (id < impl_->nodes.size() && visible_node(impl_->nodes[id], snap)) return id;
    }
    return std::nullopt;
  };
  auto user_metadata_matches =
      [](const std::map<std::string, std::string>& stored,
         const std::map<std::string, std::string>& requested) {
        size_t stored_user_entries = 0;
        for (const auto& [key, value] : stored) {
          if (key.rfind("graphene_", 0) == 0) continue;
          ++stored_user_entries;
          const auto requested_it = requested.find(key);
          if (requested_it == requested.end() ||
              requested_it->second != value) {
            return false;
          }
        }
        return stored_user_entries == requested.size();
      };
  auto metadata_optional_matches =
      [](const std::map<std::string, std::string>& stored,
         const std::string& key, const std::string& requested) {
        const auto it = stored.find(key);
        if (requested.empty()) return it == stored.end();
        return it != stored.end() && it->second == requested;
      };

  std::unordered_set<std::string> input_ids;
  std::unordered_map<std::string, uint32_t> external_to_node_id;
  std::vector<uint32_t> existing_node_ids;
  std::vector<Node> new_nodes;
  std::vector<Edge> new_edges;
  std::unordered_set<std::string> batch_lattice_keys;
  std::unordered_map<std::string, const ExtractionRelation*> batch_relations;

  auto lattice_key = [](const LatticeCoord& c) {
    return std::to_string(c.q) + "," + std::to_string(c.r) + "," + std::to_string(c.layer);
  };
  auto lattice_occupied = [&](const LatticeCoord& c) {
    return impl_->lattice_nodes.find(c) != impl_->lattice_nodes.end() || batch_lattice_keys.count(lattice_key(c)) > 0;
  };
  uint32_t placement_probe = 0;
  auto next_open_lattice = [&]() {
    while (true) {
      LatticeCoord c{static_cast<int32_t>(placement_probe++), 0, input.layer};
      if (!lattice_occupied(c)) return c;
    }
  };

  uint32_t next_node = impl_->next_node_id;
  uint64_t next_version = impl_->version;
  for (uint32_t i = 0; i < input.nodes.size(); ++i) {
    const auto& en = input.nodes[i];
    if (en.external_id.empty()) return Status::error(ErrorCode::InvalidInput, "extraction node external_id is required");
    if (!input_ids.insert(en.external_id).second) return Status::error(ErrorCode::InvalidInput, "duplicate extraction external_id");
    std::string scoped = scoped_external(en.external_id);
    auto existing = find_existing_external(scoped);
    if (existing) {
      if (!input.idempotent) return Status::error(ErrorCode::InvalidInput, "extraction external_id already exists");
      const auto& stored = impl_->nodes[*existing];
      const uint64_t expected_signature =
          en.signature ? en.signature : input.signature;
      const uint32_t expected_incident =
          en.incident ? en.incident : input.incident;
      const bool role_matches =
          stored.root == (en.role == ExtractionRole::Root) &&
          stored.symptom == (en.role == ExtractionRole::Symptom) &&
          stored.impact == (en.role == ExtractionRole::Impact);
      if (stored.content != en.content || stored.vector != en.vector ||
          stored.signature != expected_signature ||
          stored.incident != expected_incident || !role_matches ||
          stored.defect_type != en.defect_type ||
          !metadata_optional_matches(stored.metadata, "graphene_source_uri",
                                     input.source_uri) ||
          !user_metadata_matches(stored.metadata, en.metadata)) {
        return Status::error(
            ErrorCode::InvalidInput,
            "extraction node idempotency conflict for external_id '" +
                en.external_id + "'");
      }
      external_to_node_id[en.external_id] = *existing;
      existing_node_ids.push_back(*existing);
      continue;
    }

    NodeInput ni;
    ni.content = en.content;
    ni.vector = en.vector;
    ni.signature = en.signature ? en.signature : input.signature;
    ni.incident = en.incident ? en.incident : input.incident;
    ni.root = en.role == ExtractionRole::Root;
    ni.symptom = en.role == ExtractionRole::Symptom;
    ni.impact = en.role == ExtractionRole::Impact;
    ni.metadata = en.metadata;
    ni.metadata["graphene_source_id"] = input.source_id;
    ni.metadata["graphene_external_id"] = en.external_id;
    ni.metadata["graphene_scoped_external_id"] = scoped;
    ni.metadata["graphene_ingest"] = "extraction-v1";
    ni.metadata["graphene_extraction_schema"] = std::to_string(input.schema_version);
    if (!input.source_uri.empty()) ni.metadata["graphene_source_uri"] = input.source_uri;
    if (!input.extraction_run_id.empty()) ni.metadata["graphene_extraction_run_id"] = input.extraction_run_id;
    ni.lattice = en.lattice;
    if (!ni.lattice && input.place_missing_lattice) ni.lattice = next_open_lattice();
    ni.defect_type = en.defect_type;

    std::string reason;
    if (!impl_->vector_valid(ni.vector, &reason)) return Status::error(ErrorCode::DimensionMismatch, reason);
    if (ni.content.size() > 16 * 1024 * 1024) return Status::error(ErrorCode::InvalidInput, "content too large");
    if (impl_->opt.require_lattice && !ni.lattice) return Status::error(ErrorCode::InvalidInput, "lattice coordinate required");
    if (ni.lattice) {
      if (impl_->lattice_nodes.find(*ni.lattice) != impl_->lattice_nodes.end()) return Status::error(ErrorCode::InvalidInput, "duplicate lattice coordinate");
      if (!batch_lattice_keys.insert(lattice_key(*ni.lattice)).second) return Status::error(ErrorCode::InvalidInput, "duplicate lattice coordinate in extraction");
      if (impl_->opt.physical_lattice_primary) {
        uint64_t ignored = 0;
        auto pst = impl_->physical_absolute_ordinal_unlocked(*ni.lattice, &ignored);
        if (!pst) return pst;
      }
    }

    Node n;
    n.id = next_node++;
    n.content = ni.content;
    n.vector = ni.vector;
    n.signature = ni.signature;
    n.incident = ni.incident;
    n.root = ni.root; n.symptom = ni.symptom; n.impact = ni.impact;
    n.metadata = ni.metadata;
    n.lattice = ni.lattice;
    n.defect_type = ni.defect_type;
    n.created_version = next_version++;
    external_to_node_id[en.external_id] = n.id;
    new_nodes.push_back(n);
  }

  for (const auto& rel : input.relations) {
    if (rel.from_external_id.empty() || rel.to_external_id.empty()) return Status::error(ErrorCode::InvalidInput, "relation endpoints require external IDs");
    if (external_to_node_id.find(rel.from_external_id) == external_to_node_id.end()) {
      auto existing = find_existing_external(scoped_external(rel.from_external_id));
      if (existing) {
        external_to_node_id.emplace(rel.from_external_id, *existing);
      }
    }
    if (external_to_node_id.find(rel.to_external_id) == external_to_node_id.end()) {
      auto existing = find_existing_external(scoped_external(rel.to_external_id));
      if (existing) {
        external_to_node_id.emplace(rel.to_external_id, *existing);
      }
    }
    auto from_it = external_to_node_id.find(rel.from_external_id);
    auto to_it = external_to_node_id.find(rel.to_external_id);
    if (from_it == external_to_node_id.end() || to_it == external_to_node_id.end()) return Status::error(ErrorCode::EdgeInvalid, "relation endpoint external_id not found");

    std::string relation_key = input.source_id + "\x1e" + rel.from_external_id + "\x1e" + rel.to_external_id + "\x1e" + std::to_string(static_cast<int>(rel.role));
    if (input.idempotent) {
      const auto [batch_it, inserted] =
          batch_relations.emplace(relation_key, &rel);
      if (!inserted) {
        const ExtractionRelation& prior = *batch_it->second;
        if (prior.origin != rel.origin || prior.confidence != rel.confidence ||
            prior.evidence_id != rel.evidence_id ||
            prior.evidence_uri != rel.evidence_uri ||
            prior.evidence_text != rel.evidence_text ||
            prior.metadata != rel.metadata ||
            prior.bond_type != rel.bond_type ||
            prior.defect_type != rel.defect_type ||
            prior.layer_coupling != rel.layer_coupling ||
            prior.bond_strength != rel.bond_strength) {
          return Status::error(
              ErrorCode::InvalidInput,
              "extraction relation idempotency conflict within request for '" +
                  rel.from_external_id + "' -> '" + rel.to_external_id +
                  "'");
        }
        continue;
      }
      const Edge* existing_relation = nullptr;
      const auto relation_it = impl_->relation_index.find(relation_key);
      if (relation_it != impl_->relation_index.end()) {
        for (uint32_t edge_id : relation_it->second) {
          if (edge_id < impl_->edges.size() &&
              visible_edge(impl_->edges[edge_id], impl_->nodes, snap)) {
            existing_relation = &impl_->edges[edge_id];
            break;
          }
        }
      }
      if (existing_relation) {
        const bool evidence_matches =
            metadata_optional_matches(existing_relation->metadata,
                                      "graphene_evidence_id",
                                      rel.evidence_id) &&
            metadata_optional_matches(existing_relation->metadata,
                                      "graphene_evidence_uri",
                                      rel.evidence_uri) &&
            metadata_optional_matches(existing_relation->metadata,
                                      "graphene_evidence_text",
                                      rel.evidence_text);
        if (existing_relation->from != from_it->second ||
            existing_relation->to != to_it->second ||
            existing_relation->origin != rel.origin ||
            existing_relation->role != rel.role ||
            existing_relation->confidence != rel.confidence ||
            existing_relation->bond_type != rel.bond_type ||
            existing_relation->defect_type != rel.defect_type ||
            existing_relation->layer_coupling != rel.layer_coupling ||
            existing_relation->bond_strength != rel.bond_strength ||
            !metadata_optional_matches(existing_relation->metadata,
                                       "graphene_source_uri",
                                       input.source_uri) ||
            !evidence_matches ||
            !user_metadata_matches(existing_relation->metadata,
                                   rel.metadata)) {
          return Status::error(
              ErrorCode::InvalidInput,
              "extraction relation idempotency conflict for '" +
                  rel.from_external_id + "' -> '" + rel.to_external_id +
                  "'");
        }
        continue;
      }
    }

    auto node_for_batch = [&](uint32_t id) -> const Node* {
      if (id < impl_->nodes.size() && visible_node(impl_->nodes[id], snap)) return &impl_->nodes[id];
      for (const auto& n : new_nodes) if (n.id == id) return &n;
      return nullptr;
    };

    EdgeInput ei;
    ei.from = from_it->second;
    ei.to = to_it->second;
    ei.origin = rel.origin;
    ei.role = rel.role;
    ei.confidence = rel.confidence;
    ei.metadata = rel.metadata;
    ei.metadata["graphene_source_id"] = input.source_id;
    ei.metadata["graphene_relation_key"] = relation_key;
    ei.metadata["graphene_ingest"] = "extraction-v1";
    ei.metadata["graphene_extraction_schema"] = std::to_string(input.schema_version);
    if (!input.source_uri.empty()) ei.metadata["graphene_source_uri"] = input.source_uri;
    if (!input.extraction_run_id.empty()) ei.metadata["graphene_extraction_run_id"] = input.extraction_run_id;
    if (!rel.evidence_id.empty()) ei.metadata["graphene_evidence_id"] = rel.evidence_id;
    if (!rel.evidence_uri.empty()) ei.metadata["graphene_evidence_uri"] = rel.evidence_uri;
    if (!rel.evidence_text.empty()) ei.metadata["graphene_evidence_text"] = rel.evidence_text;
    ei.bond_type = rel.bond_type;
    ei.defect_type = rel.defect_type;
    ei.layer_coupling = rel.layer_coupling;
    ei.bond_strength = rel.bond_strength;

    if (!std::isfinite(ei.confidence) || ei.confidence < 0.0 || ei.confidence > 1.0) return Status::error(ErrorCode::InvalidInput, "confidence must be between 0 and 1");
    if (!std::isfinite(ei.bond_strength) || ei.bond_strength < 0.0 || ei.bond_strength > 1.0) return Status::error(ErrorCode::InvalidInput, "bond_strength must be between 0 and 1");
    if (is_lattice_bond(ei.bond_type)) {
      const Node* from = node_for_batch(ei.from);
      const Node* to = node_for_batch(ei.to);
      if (!from || !to || !from->lattice || !to->lattice) return Status::error(ErrorCode::EdgeInvalid, "lattice bond requires lattice coordinates on both endpoints");
      if (ei.bond_type == BondType::VanDerWaals) {
        if (!are_cross_layer_neighbors(*from->lattice, *to->lattice, ei.layer_coupling)) return Status::error(ErrorCode::EdgeInvalid, "VanDerWaals bond requires valid cross-layer neighbor and coupling");
      } else if (ei.bond_type != BondType::Defect && ei.bond_type != BondType::Synthetic) {
        if (!are_same_layer_hex_neighbors(*from->lattice, *to->lattice)) return Status::error(ErrorCode::EdgeInvalid, "Sigma/Pi bond requires same-layer hex-neighbor coordinates");
      }
    }

    Edge e;
    e.id = impl_->next_edge_id + static_cast<uint32_t>(new_edges.size());
    e.from = ei.from; e.to = ei.to; e.origin = ei.origin; e.role = ei.role; e.confidence = ei.confidence;
    e.metadata = ei.metadata;
    e.bond_type = ei.bond_type; e.defect_type = ei.defect_type; e.layer_coupling = ei.layer_coupling; e.bond_strength = ei.bond_strength;
    e.created_version = next_version++;
    new_edges.push_back(e);
  }

  if (new_nodes.empty() && new_edges.empty()) {
    if (out) {
      out->external_to_node_id = std::move(external_to_node_id);
      out->existing_node_ids = std::move(existing_node_ids);
    }
    return Status::ok();
  }

  uint64_t tx = impl_->txid;
  std::vector<std::string> frames;
  frames.reserve(2 + new_nodes.size() + new_edges.size());
  frames.push_back("BEGIN\t" + std::to_string(tx));
  for (const auto& n : new_nodes) frames.push_back("PUT_NODE\t" + std::to_string(tx) + "\t" + impl_->node_payload(n, "PUT_NODE").substr(std::string("PUT_NODE\t").size()));
  for (const auto& e : new_edges) frames.push_back("PUT_EDGE\t" + std::to_string(tx) + "\t" + impl_->edge_payload(e, "PUT_EDGE").substr(std::string("PUT_EDGE\t").size()));
  frames.push_back("COMMIT\t" + std::to_string(tx));
  auto st = impl_->append_wal_unlocked(frames);
  if (!st) return st;

  ++impl_->txid;
  impl_->next_node_id = next_node;
  impl_->next_edge_id += static_cast<uint32_t>(new_edges.size());
  impl_->version = next_version;
  for (const auto& n : new_nodes) impl_->apply_committed_node(n);
  for (const auto& e : new_edges) impl_->apply_committed_edge(e);
  if (out) {
    out->external_to_node_id = std::move(external_to_node_id);
    out->existing_node_ids = std::move(existing_node_ids);
    out->inserted_node_ids.clear();
    out->inserted_edge_ids.clear();
    for (const auto& n : new_nodes) out->inserted_node_ids.push_back(n.id);
    for (const auto& e : new_edges) out->inserted_edge_ids.push_back(e.id);
  }
  auto plst = impl_->write_physical_lattice_unlocked();
  if (!plst) impl_->note_maintenance_failure("physical lattice maintenance", plst);
  auto rst = impl_->maybe_rotate_wal_unlocked();
  if (!rst) impl_->note_maintenance_failure("automatic WAL rotation", rst);
  return Status::ok();
}

Status GrapheneDB::delete_node(uint32_t id) {
  std::unique_lock lock(impl_->mu);
  if (!impl_->open) return Status::error(ErrorCode::NotOpen, "database is not open");
  uint64_t snap = impl_->version - 1;
  if (id >= impl_->nodes.size() || !visible_node(impl_->nodes[id], snap)) return Status::error(ErrorCode::NodeNotFound, "node not found or already deleted");
  uint64_t delver = impl_->version;
  uint64_t tx = impl_->txid;
  std::vector<std::string> frames = {"BEGIN\t" + std::to_string(tx), "DELETE_NODE\t" + std::to_string(tx) + "\t" + std::to_string(id) + "\t" + std::to_string(delver), "COMMIT\t" + std::to_string(tx)};
  auto st = impl_->append_wal_unlocked(frames);
  if (!st) return st;
  ++impl_->version;
  ++impl_->txid;
  std::unordered_set<uint32_t> affected_edges;
  auto in_it = impl_->in_edges.find(id);
  if (in_it != impl_->in_edges.end()) affected_edges.insert(in_it->second.begin(), in_it->second.end());
  auto out_it = impl_->out_edges.find(id);
  if (out_it != impl_->out_edges.end()) affected_edges.insert(out_it->second.begin(), out_it->second.end());
  uint64_t before_snap = delver - 1;
  size_t hidden_edges = 0;
  for (uint32_t eid : affected_edges) {
    if (eid < impl_->edges.size() && visible_edge(impl_->edges[eid], impl_->nodes, before_snap)) ++hidden_edges;
  }
  impl_->nodes[id].deleted_version = delver;
  if (impl_->nodes[id].lattice) impl_->lattice_nodes.erase(*impl_->nodes[id].lattice);
  if (impl_->live_node_count > 0) --impl_->live_node_count;
  impl_->live_edge_count = hidden_edges > impl_->live_edge_count ? 0 : impl_->live_edge_count - hidden_edges;
  if (impl_->vector_index) (void)impl_->vector_index->remove(id);
  auto plst = impl_->write_physical_lattice_unlocked();
  if (!plst) impl_->note_maintenance_failure("physical lattice maintenance", plst);
  auto rst = impl_->maybe_rotate_wal_unlocked();
  if (!rst) impl_->note_maintenance_failure("automatic WAL rotation", rst);
  return Status::ok();
}

size_t GrapheneDB::node_count(uint64_t snap) const {
  std::shared_lock lock(impl_->mu);
  uint64_t current = impl_->version - 1;
  if (snap == kInfVersion || snap == current) return impl_->live_node_count;
  size_t c = 0; for (const auto& n : impl_->nodes) if (visible_node(n, snap)) ++c; return c;
}

size_t GrapheneDB::edge_count(uint64_t snap) const {
  std::shared_lock lock(impl_->mu);
  uint64_t current = impl_->version - 1;
  if (snap == kInfVersion || snap == current) return impl_->live_edge_count;
  size_t c = 0; for (const auto& e : impl_->edges) if (visible_edge(e, impl_->nodes, snap)) ++c; return c;
}

std::vector<uint32_t> GrapheneDB::metadata_search(const std::string& key, const std::string& value, uint64_t snap) const {
  std::shared_lock lock(impl_->mu);
  if (snap == kInfVersion) snap = impl_->version - 1;
  std::vector<uint32_t> out;
  auto it = impl_->metadata_index.find(key + "\x1f" + value);
  if (it == impl_->metadata_index.end()) return out;
  std::unordered_set<uint32_t> seen;
  for (uint32_t id : it->second) {
    if (seen.insert(id).second && id < impl_->nodes.size() && visible_node(impl_->nodes[id], snap)) out.push_back(id);
  }
  return out;
}

std::vector<uint32_t> GrapheneDB::lattice_neighbors(uint32_t node_id, uint32_t max_hops, uint64_t snap) const {
  std::shared_lock lock(impl_->mu);
  if (snap == kInfVersion) snap = impl_->version - 1;
  std::vector<uint32_t> out;
  if (max_hops == 0 || node_id >= impl_->nodes.size() || !visible_node(impl_->nodes[node_id], snap)) return out;
  std::vector<std::pair<uint32_t, uint32_t>> queue{{node_id, 0}};
  std::unordered_set<uint32_t> seen{node_id};
  for (size_t qi = 0; qi < queue.size(); ++qi) {
    auto [cur, depth] = queue[qi];
    if (depth >= max_hops) continue;
    if (impl_->dense_lattice_enabled()) {
      auto oit = impl_->dense_lattice_ordinal_by_node.find(cur);
      if (oit == impl_->dense_lattice_ordinal_by_node.end()) continue;
      const auto& cell = impl_->dense_lattice_cells[oit->second];
      for (uint32_t next : cell.neighbor_nodes) {
        if (next == UINT32_MAX || next >= impl_->nodes.size() || !visible_node(impl_->nodes[next], snap)) continue;
        if (seen.insert(next).second) {
          out.push_back(next);
          queue.push_back({next, depth + 1});
        }
      }
      continue;
    }
    auto it = impl_->lattice_edges.find(cur);
    if (it == impl_->lattice_edges.end()) continue;
    for (uint32_t eid : it->second) {
      if (eid >= impl_->edges.size() || !visible_edge(impl_->edges[eid], impl_->nodes, snap)) continue;
      const auto& e = impl_->edges[eid];
      uint32_t next = e.from == cur ? e.to : e.from;
      if (seen.insert(next).second) {
        out.push_back(next);
        queue.push_back({next, depth + 1});
      }
    }
  }
  std::sort(out.begin(), out.end());
  return out;
}

std::vector<Edge> GrapheneDB::incoming_edges(uint32_t node_id, uint64_t snap) const {
  std::shared_lock lock(impl_->mu);
  if (snap == kInfVersion) snap = impl_->version - 1;
  std::vector<Edge> out;
  if (node_id >= impl_->nodes.size() || !visible_node(impl_->nodes[node_id], snap)) return out;
  auto it = impl_->in_edges.find(node_id);
  if (it == impl_->in_edges.end()) return out;
  out.reserve(it->second.size());
  for (uint32_t edge_id : it->second) {
    if (edge_id < impl_->edges.size() && visible_edge(impl_->edges[edge_id], impl_->nodes, snap)) {
      out.push_back(impl_->edges[edge_id]);
    }
  }
  std::sort(out.begin(), out.end(), [](const Edge& a, const Edge& b) { return a.id < b.id; });
  return out;
}

std::vector<Edge> GrapheneDB::outgoing_edges(uint32_t node_id, uint64_t snap) const {
  std::shared_lock lock(impl_->mu);
  if (snap == kInfVersion) snap = impl_->version - 1;
  std::vector<Edge> out;
  if (node_id >= impl_->nodes.size() || !visible_node(impl_->nodes[node_id], snap)) return out;
  auto it = impl_->out_edges.find(node_id);
  if (it == impl_->out_edges.end()) return out;
  out.reserve(it->second.size());
  for (uint32_t edge_id : it->second) {
    if (edge_id < impl_->edges.size() && visible_edge(impl_->edges[edge_id], impl_->nodes, snap)) {
      out.push_back(impl_->edges[edge_id]);
    }
  }
  std::sort(out.begin(), out.end(), [](const Edge& a, const Edge& b) { return a.id < b.id; });
  return out;
}

std::optional<Node> GrapheneDB::get_node(uint32_t id, uint64_t snap) const {
  std::shared_lock lock(impl_->mu);
  if (snap == kInfVersion) snap = impl_->version - 1;
  if (id >= impl_->nodes.size() || !visible_node(impl_->nodes[id], snap)) return std::nullopt;
  return impl_->nodes[id];
}

std::optional<Edge> GrapheneDB::get_edge(uint32_t id, uint64_t snap) const {
  std::shared_lock lock(impl_->mu);
  if (snap == kInfVersion) snap = impl_->version - 1;
  if (id >= impl_->edges.size() || !visible_edge(impl_->edges[id], impl_->nodes, snap)) return std::nullopt;
  return impl_->edges[id];
}

std::vector<SearchResult> GrapheneDB::vector_search(const std::vector<float>& query, size_t k, uint64_t snap) const {
  std::shared_lock lock(impl_->mu);
  std::string reason;
  if (!impl_->vector_valid(query, &reason) || k == 0) return {};
  uint64_t current = impl_->version - 1;
  if (snap == kInfVersion) snap = current;
  if (snap == current && impl_->vector_index) {
    return impl_->vector_index->search(query, k);
  }
  std::vector<SearchResult> scored;
  for (const auto& n : impl_->nodes) {
    if (!visible_node(n, snap)) continue;
    double s = cosine(query, n.vector);
    if (s >= -0.5) scored.push_back({n.id, s});
  }
  size_t kk = std::min(k, scored.size());
  std::partial_sort(scored.begin(), scored.begin() + kk, scored.end(), [](auto& a, auto& b) { return a.score > b.score; });
  scored.resize(kk);
  return scored;
}

MemoryBundle GrapheneDB::causal_search(const std::vector<float>& query, uint64_t query_signature, QueryMode mode, uint64_t snap) const {
  std::shared_lock lock(impl_->mu);
  MemoryBundle out;
  std::string reason;
  if (!impl_->vector_valid(query, &reason)) { out.abstain = true; out.reason = "VECTOR_DIMENSION_MISMATCH: " + reason; return out; }
  if (snap == kInfVersion) snap = impl_->version - 1;
  out.snapshot_version = snap;
  std::vector<uint32_t> candidates;
  for (const auto& kv : impl_->planes) {
    int overlap = popcount64(kv.first & query_signature);
    int required = std::max(1, popcount64(query_signature) - 1);
    if (overlap >= required) {
      for (uint32_t id : kv.second) if (id < impl_->nodes.size() && visible_node(impl_->nodes[id], snap)) candidates.push_back(id);
    }
  }
  if (candidates.size() < impl_->opt.tuning.min_candidate_floor) {
    std::vector<SearchResult> fallback;
    for (const auto& n : impl_->nodes) if (visible_node(n, snap)) fallback.push_back({n.id, cosine(query, n.vector)});
    std::sort(fallback.begin(), fallback.end(), [](auto& a, auto& b) { return a.score > b.score; });
    for (size_t i = 0; i < std::min<size_t>(impl_->opt.tuning.min_candidate_floor, fallback.size()); ++i) candidates.push_back(fallback[i].node_id);
  }
  std::sort(candidates.begin(), candidates.end());
  candidates.erase(std::unique(candidates.begin(), candidates.end()), candidates.end());
  std::vector<SearchResult> anchors;
  for (uint32_t id : candidates) {
    const auto& n = impl_->nodes[id];
    double s = cosine(query, n.vector) + ((n.symptom || n.impact) ? impl_->opt.tuning.symptom_or_impact_boost : 0.0);
    anchors.push_back({id, s});
  }
  std::sort(anchors.begin(), anchors.end(), [](auto& a, auto& b) { return a.score > b.score; });
  if (anchors.empty() || anchors.front().score < impl_->opt.tuning.min_anchor_score) { out.abstain = true; out.reason = "NO_STABLE_ANCHOR"; return out; }
  size_t inspect = std::min<size_t>(impl_->opt.tuning.max_anchor_inspect, anchors.size());
  std::map<uint32_t, int> incidents;
  for (size_t i = 0; i < inspect; ++i) incidents[impl_->nodes[anchors[i].node_id].incident]++;
  int max_inc = 0; for (auto& kv : incidents) max_inc = std::max(max_inc, kv.second);
  if (anchors.front().score < impl_->opt.tuning.high_confidence_anchor_score && static_cast<double>(max_inc) / inspect < impl_->opt.tuning.min_incident_concentration && mode != QueryMode::Theoretical) {
    out.abstain = true; out.reason = "AMBIGUOUS"; return out;
  }
  auto lattice_activation = impl_->propagate_lattice_activation(anchors, inspect, mode, snap);
  std::map<uint32_t, MemoryBundle> by_root;
  for (size_t i = 0; i < inspect; ++i) {
    Path p = impl_->reverse_root(anchors[i].node_id, mode, snap);
    if (p.nodes.empty()) continue;
    p.score *= anchors[i].score;
    uint32_t root = p.nodes.front();
    auto& b = by_root[root];
    b.target_node = root;
    b.paths.push_back(p);
    b.semantic_candidates.push_back(anchors[i].node_id);
    for (uint32_t nid : p.nodes) {
      auto lit = lattice_activation.find(nid);
      if (lit != lattice_activation.end()) b.lattice_score = std::max(b.lattice_score, lit->second);
    }
  }
  if (by_root.empty()) { out.abstain = true; out.reason = "NO_PATH"; return out; }
  double best_score = -1.0;
  for (auto& kv : by_root) {
    auto& b = kv.second;
    std::set<uint32_t> unique_edges;
    int contradictions = 0;
    double sum = 0.0;
    for (const auto& p : b.paths) {
      unique_edges.insert(p.edges.begin(), p.edges.end());
      contradictions += p.contains_contradiction ? 1 : 0;
      sum += p.score;
    }
    b.degeneracy = static_cast<double>(b.paths.size());
    b.diversity = b.paths.empty() ? 0.0 : static_cast<double>(unique_edges.size()) / std::max<size_t>(1, b.paths.size());
    b.contradiction = b.paths.empty() ? 0.0 : static_cast<double>(contradictions) / b.paths.size();
    b.confidence = 0.35 * std::min(1.0, b.degeneracy / 3.0) + 0.25 * std::min(1.0, b.diversity / 4.0) - 0.25 * b.contradiction + sum / std::max<size_t>(1, b.paths.size());
    if (impl_->opt.tuning.enable_lattice_retrieval && b.lattice_score > 0.0) {
      b.confidence += impl_->opt.tuning.lattice_weight * std::min(1.0, b.lattice_score);
      std::set<uint32_t> seen_neighbors;
      for (const auto& p : b.paths) {
        for (uint32_t nid : p.nodes) {
          auto it = lattice_activation.find(nid);
          if (it != lattice_activation.end() && seen_neighbors.insert(nid).second) b.lattice_neighbors.push_back(nid);
        }
      }
      b.lattice_explanation.push_back("activation propagated through graphene-inspired lattice bonds");
    }
    if (b.confidence > best_score) { best_score = b.confidence; out = b; }
  }
  out.confidence = std::clamp(out.confidence, 0.0, 1.0);
  out.snapshot_version = snap;
  if (out.confidence < impl_->opt.tuning.min_bundle_confidence && mode != QueryMode::Theoretical) { out.abstain = true; out.reason = "LOW_STABILITY"; }
  if (!out.abstain) {
    out.why_retrieved.push_back("semantic similarity to candidate memories");
    out.why_retrieved.push_back("signature-plane candidate reduction");
    out.why_retrieved.push_back("causal path from root memory to anchor memory");
    if (out.lattice_score > 0.0) out.why_retrieved.push_back("lattice propagation through graphene-inspired neighbor bonds");
    if (out.contradiction > 0.0) out.why_retrieved.push_back("contains contradiction path; confidence reduced");
  }
  return out;
}

Status GrapheneDB::compact() {
  std::unique_lock lock(impl_->mu);
  if (!impl_->open) return Status::error(ErrorCode::NotOpen, "database is not open");
  return impl_->checkpoint_unlocked();
}

Status GrapheneDB::backup(const fs::path& destination_dir) const {
  std::shared_lock lock(impl_->mu);
  if (!impl_->open) return Status::error(ErrorCode::NotOpen, "database is not open");
  std::error_code ec;
  fs::create_directories(destination_dir, ec);
  if (ec) return Status::error(ErrorCode::IoError, "cannot create backup dir: " + ec.message());
  for (const auto& src : {impl_->data_path, impl_->wal_path, impl_->manifest_path, impl_->physical_lattice_path, impl_->physical_lattice_bin_path, impl_->nodeidx_path}) {
    if (env_enabled("GRAPHENEDB_TEST_FAIL_BACKUP_COPY")) {
      return Status::error(ErrorCode::IoError, "injected backup copy failure");
    }
    if (fs::exists(src)) fs::copy_file(src, destination_dir / src.filename(), fs::copy_options::overwrite_existing, ec);
    if (ec) return Status::error(ErrorCode::IoError, "backup failed: " + ec.message());
  }
  return Status::ok();
}

Status GrapheneDB::inspect(std::string* out) const {
  std::shared_lock lock(impl_->mu);
  if (!out) return Status::error(ErrorCode::InvalidInput, "out cannot be null");
  std::ostringstream os;
  uint64_t snap = impl_->version - 1;
  os << "GrapheneDB v1\n";
  os << "path=" << impl_->dir.string() << "\n";
  os << "manifest_format=" << kManifestFormatVersion << "\n";
  os << "storage_format=" << kStorageFormatVersion << "\n";
  os << "wal_frame_format=" << kWalFrameFormatVersion << "\n";
  os << "lattice_format=" << kLatticeFormatVersion << "\n";
  os << "extraction_format=" << kExtractionFormatVersion << "\n";
  os << "dimension=" << impl_->opt.dimension << "\n";
  os << "version=" << snap << "\n";
  os << "nodes_visible=" << impl_->live_node_count << "\n";
  os << "edges_visible=" << impl_->live_edge_count << "\n";
  std::error_code ec;
  uint64_t wal_bytes = fs::exists(impl_->wal_path, ec) ? fs::file_size(impl_->wal_path, ec) : 0;
  if (ec) wal_bytes = 0;
  ec.clear();
  uint64_t data_bytes = fs::exists(impl_->data_path, ec) ? fs::file_size(impl_->data_path, ec) : 0;
  if (ec) data_bytes = 0;
  os << "wal_rotate_bytes=" << impl_->opt.wal_rotate_bytes << "\n";
  os << "wal_bytes=" << wal_bytes << "\n";
  os << "data_bytes=" << data_bytes << "\n";
  os << "vector_index_requested=" << vector_index_kind_name(impl_->opt.vector_index_kind) << "\n";
  os << "vector_index=" << (impl_->vector_index ? impl_->vector_index->name() : "none") << "\n";
  os << "maintenance_required=" << (impl_->maintenance_warning.empty() ? "false" : "true") << "\n";
  os << "maintenance_warning=" << impl_->maintenance_warning << "\n";
  os << "planes=" << impl_->planes.size() << "\n";
  os << "lattice_required=" << (impl_->opt.require_lattice ? "true" : "false") << "\n";
  os << "lattice_retrieval=" << (impl_->opt.tuning.enable_lattice_retrieval ? "true" : "false") << "\n";
  os << "physical_lattice_storage=" << (impl_->opt.physical_lattice_storage ? "true" : "false") << "\n";
  os << "physical_lattice_primary=" << (impl_->opt.physical_lattice_primary ? "true" : "false") << "\n";
  os << "physical_lattice_radius=" << impl_->opt.physical_lattice_radius << "\n";
  std::error_code lec;
  uint64_t lattice_bytes = fs::exists(impl_->physical_lattice_path, lec) ? fs::file_size(impl_->physical_lattice_path, lec) : 0;
  uint64_t lattice_bin_bytes = fs::exists(impl_->physical_lattice_bin_path, lec) ? fs::file_size(impl_->physical_lattice_bin_path, lec) : 0;
  if (lec) lattice_bytes = 0;
  os << "physical_lattice_file=" << impl_->physical_lattice_path.string() << "\n";
  os << "physical_lattice_bytes=" << lattice_bytes << "\n";
  os << "physical_lattice_bin_file=" << impl_->physical_lattice_bin_path.string() << "\n";
  os << "physical_lattice_bin_bytes=" << lattice_bin_bytes << "\n";
  os << "lattice_nodes=" << impl_->lattice_nodes.size() << "\n";
  os << "dense_lattice_cells=" << impl_->dense_lattice_cells.size() << "\n";
  os << "dense_lattice_index=" << (impl_->dense_lattice_enabled() ? "true" : "false") << "\n";
  size_t lattice_edge_count = 0;
  for (const auto& kv : impl_->lattice_edges) lattice_edge_count += kv.second.size();
  os << "lattice_edge_refs=" << lattice_edge_count << "\n";
  *out = os.str();
  return Status::ok();
}

Status GrapheneDB::validate(std::string* report) const {
  std::shared_lock lock(impl_->mu);
  std::ostringstream os;
  bool ok = true;
  if (impl_->opt.dimension == 0) { ok = false; os << "FAIL dimension zero\n"; }
  for (const auto& n : impl_->nodes) {
    if (n.created_version == kInfVersion) continue;
    if (n.vector.size() != impl_->opt.dimension) { ok = false; os << "FAIL node " << n.id << " dimension mismatch\n"; }
    for (float f : n.vector) if (!std::isfinite(f)) { ok = false; os << "FAIL node " << n.id << " non-finite vector\n"; }
    if (impl_->opt.require_lattice && visible_node(n, impl_->version - 1) && !n.lattice) { ok = false; os << "FAIL node " << n.id << " missing lattice coordinate\n"; }
  }
  std::unordered_map<LatticeCoord, uint32_t, Impl::LatticeCoordHash> coords;
  for (const auto& n : impl_->nodes) {
    if (n.created_version == kInfVersion || !visible_node(n, impl_->version - 1) || !n.lattice) continue;
    auto inserted = coords.emplace(*n.lattice, n.id);
    if (!inserted.second) { ok = false; os << "FAIL duplicate lattice coordinate on nodes " << inserted.first->second << " and " << n.id << "\n"; }
  }
  if (impl_->opt.physical_lattice_storage) {
    if (impl_->opt.physical_lattice_primary) {
      if (!fs::exists(impl_->physical_lattice_bin_path)) {
        ok = false; os << "FAIL physical lattice binary file missing\n";
      } else {
        std::ifstream bin(impl_->physical_lattice_bin_path, std::ios::binary);
        Impl::PhysicalLatticeHeader header{};
        bin.read(reinterpret_cast<char*>(&header), sizeof(header));
        if (!bin || std::string(header.magic, header.magic + 6) != "GDBHEX") {
          ok = false; os << "FAIL physical lattice binary header corrupt\n";
        } else {
          if (header.visible_cells != coords.size()) {
            ok = false; os << "FAIL physical lattice binary visible cells " << header.visible_cells << " != lattice node count " << coords.size() << "\n";
          }
          for (const auto& kv : coords) {
            uint64_t absolute = 0;
            auto ost = impl_->physical_absolute_ordinal_unlocked(kv.first, &absolute);
            if (!ost) { ok = false; os << "FAIL physical lattice coord outside radius for node " << kv.second << "\n"; continue; }
            Impl::PhysicalCellRecord rec{};
            bin.clear();
            bin.seekg(static_cast<std::streamoff>(sizeof(Impl::PhysicalLatticeHeader) + absolute * sizeof(Impl::PhysicalCellRecord)));
            bin.read(reinterpret_cast<char*>(&rec), sizeof(rec));
            if (!bin || rec.node_id != kv.second) {
              ok = false; os << "FAIL physical lattice binary cell mismatch for node " << kv.second << "\n";
              break;
            }
            uint32_t crc = rec.checksum; rec.checksum = 0;
            if (crc != Impl::fnv32_cell(rec)) { ok = false; os << "FAIL physical lattice binary checksum for node " << kv.second << "\n"; break; }
          }
        }
      }
      if (!fs::exists(impl_->nodeidx_path)) { ok = false; os << "FAIL nodeidx file missing\n"; }
    } else if (!fs::exists(impl_->physical_lattice_path)) {
      ok = false; os << "FAIL physical lattice file missing\n";
    } else {
      std::ifstream lin(impl_->physical_lattice_path);
      std::string line;
      size_t cell_count = 0;
      while (std::getline(lin, line)) {
        if (line.empty()) continue;
        std::string payload;
        if (!parse_frame_line(line, &payload)) { ok = false; os << "FAIL physical lattice corrupt frame\n"; break; }
        if (payload.rfind("CELL\t", 0) == 0) ++cell_count;
      }
      if (cell_count != coords.size()) {
        ok = false; os << "FAIL physical lattice cell count " << cell_count << " != lattice node count " << coords.size() << "\n";
      }
    }
  }
  for (const auto& e : impl_->edges) {
    if (e.created_version == kInfVersion) continue;
    if (e.from >= impl_->nodes.size() || e.to >= impl_->nodes.size()) { ok = false; os << "FAIL edge " << e.id << " missing endpoint\n"; }
    if (!visible_edge(e, impl_->nodes, impl_->version - 1) || !is_lattice_bond(e.bond_type)) continue;
    if (!std::isfinite(e.bond_strength) || e.bond_strength < 0.0 || e.bond_strength > 1.0) { ok = false; os << "FAIL edge " << e.id << " invalid bond strength\n"; }
    if (e.from >= impl_->nodes.size() || e.to >= impl_->nodes.size() || !impl_->nodes[e.from].lattice || !impl_->nodes[e.to].lattice) {
      ok = false; os << "FAIL edge " << e.id << " lattice bond missing endpoint coordinate\n"; continue;
    }
    if ((e.bond_type == BondType::Sigma || e.bond_type == BondType::Pi) && !are_same_layer_hex_neighbors(*impl_->nodes[e.from].lattice, *impl_->nodes[e.to].lattice)) {
      ok = false; os << "FAIL edge " << e.id << " invalid same-layer lattice bond\n";
    }
    if (e.bond_type == BondType::VanDerWaals && !are_cross_layer_neighbors(*impl_->nodes[e.from].lattice, *impl_->nodes[e.to].lattice, e.layer_coupling)) {
      ok = false; os << "FAIL edge " << e.id << " invalid cross-layer lattice bond\n";
    }
  }
  if (ok) os << "OK\n";
  if (report) *report = os.str();
  return ok ? Status::ok() : Status::error(ErrorCode::DataCorrupt, "validation failed");
}

} // namespace graphene
