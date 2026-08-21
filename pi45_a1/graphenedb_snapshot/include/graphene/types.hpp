#pragma once
#include <cstddef>
#include <cstdint>
#include <map>
#include <optional>
#include <string>
#include <unordered_map>
#include <vector>

namespace graphene {

static constexpr uint64_t kInfVersion = UINT64_MAX;
static constexpr uint32_t kManifestFormatVersion = 1;
static constexpr uint32_t kStorageFormatVersion = 2;
static constexpr uint32_t kWalFrameFormatVersion = 1;
static constexpr uint32_t kLatticeFormatVersion = 1;
static constexpr uint32_t kExtractionFormatVersion = 1;

enum class ErrorCode {
  Ok = 0,
  NotOpen,
  InvalidOption,
  IoError,
  WalCorrupt,
  DataCorrupt,
  TransactionIncomplete,
  DimensionMismatch,
  NodeNotFound,
  EdgeInvalid,
  InvalidInput,
  LockBusy,
  UnsupportedMode
};

struct Status {
  ErrorCode code{ErrorCode::Ok};
  std::string message{};
  static Status ok() { return {}; }
  static Status error(ErrorCode c, std::string m) { return {c, std::move(m)}; }
  explicit operator bool() const { return code == ErrorCode::Ok; }
};

enum class EdgeOrigin : uint8_t {
  Observed = 0,
  Discovered = 1,
  Inferred = 2,
  Reinforced = 3,
  Hypothetical = 4
};

enum class EdgeRole : uint8_t {
  Mechanistic = 0,
  Compressed = 1,
  Analogical = 2,
  Predictive = 3,
  Causal = 4,
  Contradicts = 5,
  Supports = 6,
  Supersedes = 7
};

enum class QueryMode : uint8_t {
  Empirical = 0,
  Balanced = 1,
  Theoretical = 2
};

enum class VectorIndexKind : uint8_t {
  Auto = 0,
  Flat = 1,
  KDTree = 2,
  Faiss = 3
};

struct LatticeCoord {
  int32_t q{0};
  int32_t r{0};
  int32_t layer{0};
};

inline bool operator==(const LatticeCoord& a, const LatticeCoord& b) {
  return a.q == b.q && a.r == b.r && a.layer == b.layer;
}

enum class BondType : uint8_t {
  None = 0,
  Sigma = 1,
  Pi = 2,
  VanDerWaals = 3,
  Defect = 4,
  Synthetic = 5
};

enum class DefectType : uint8_t {
  None = 0,
  Vacancy = 1,
  Substitution = 2,
  StoneWales = 3,
  Strain = 4,
  Doped = 5,
  Boundary = 6
};

enum class LayerCoupling : uint8_t {
  None = 0,
  SameLayer = 1,
  VanDerWaals = 2,
  BernalStacked = 3,
  Twisted = 4,
  Synthetic = 5
};

struct RetrievalTuning {
  // RC defaults. These are explicit tuning values, not scientific constants.
  double min_anchor_score{0.42};
  double high_confidence_anchor_score{0.97};
  double min_incident_concentration{0.34};
  double min_bundle_confidence{0.45};
  double symptom_or_impact_boost{0.15};
  size_t max_reverse_bfs_visits{20000};
  size_t min_candidate_floor{64};
  size_t max_anchor_inspect{12};
  bool enable_lattice_retrieval{false};
  uint32_t lattice_max_hops{3};
  double lattice_decay{0.65};
  double lattice_defect_penalty{0.4};
  double lattice_cross_layer_penalty{0.25};
  double lattice_weight{0.25};
};

struct DBOptions {
  uint32_t dimension{0};
  bool create_if_missing{true};
  bool fsync_on_commit{true};
  // If >0, committed WAL bytes above this threshold trigger a checkpoint/rotation.
  uint64_t wal_rotate_bytes{0};
  // If true, a LOCK file owned by a dead process is removed on open.
  bool recover_stale_lock{true};
  // If true, every inserted node must include a lattice coordinate and lattice
  // bonds are topology-validated.
  bool require_lattice{false};
  // If true, GrapheneDB materializes a physical lattice sidecar file
  // (graphene.lattice) where visible lattice nodes are stored in deterministic
  // hex-cell order with coordinate-neighbor slots. The canonical WAL/data files
  // remain the recovery source of truth in this RC, but traversal can be backed
  // by a disk-shaped lattice layout rather than coordinates as metadata only.
  bool physical_lattice_storage{false};
  // If true, graphene.lattice.bin becomes a fixed-offset physical hex lattice index.
  // The framed WAL remains the commit log, while node placement is mirrored into
  // ordinal-addressed CellRecords for direct seek/mmap-style retrieval.
  bool physical_lattice_primary{false};
  // Radius of the fixed axial disk per layer used by the physical index.
  // Capacity per layer = 1 + 3R(R + 1).
  uint32_t physical_lattice_radius{256};
  // Auto currently selects KDTree for low-dimensional vectors and Flat for
  // higher-dimensional embeddings where KDTree pruning degrades. Faiss is an
  // explicit opt-in backend when GrapheneDB is compiled with FAISS support.
  VectorIndexKind vector_index_kind{VectorIndexKind::Auto};
  RetrievalTuning tuning{};
};

struct NodeInput {
  std::string content;
  std::vector<float> vector;
  uint64_t signature{0};
  uint32_t incident{0};
  bool root{false};
  bool symptom{false};
  bool impact{false};
  std::map<std::string, std::string> metadata;
  std::optional<LatticeCoord> lattice;
  DefectType defect_type{DefectType::None};
};

struct Node {
  uint32_t id{0};
  std::string content;
  std::vector<float> vector;
  uint64_t signature{0};
  uint32_t incident{0};
  bool root{false};
  bool symptom{false};
  bool impact{false};
  std::map<std::string, std::string> metadata;
  std::optional<LatticeCoord> lattice;
  DefectType defect_type{DefectType::None};
  uint64_t created_version{0};
  uint64_t deleted_version{kInfVersion};
};

struct EdgeInput {
  uint32_t from{0};
  uint32_t to{0};
  EdgeOrigin origin{EdgeOrigin::Observed};
  EdgeRole role{EdgeRole::Causal};
  double confidence{0.9};
  std::map<std::string, std::string> metadata;
  BondType bond_type{BondType::None};
  DefectType defect_type{DefectType::None};
  LayerCoupling layer_coupling{LayerCoupling::None};
  double bond_strength{1.0};
};

struct Edge {
  uint32_t id{0};
  uint32_t from{0};
  uint32_t to{0};
  EdgeOrigin origin{EdgeOrigin::Observed};
  EdgeRole role{EdgeRole::Causal};
  double confidence{0.9};
  std::map<std::string, std::string> metadata;
  BondType bond_type{BondType::None};
  DefectType defect_type{DefectType::None};
  LayerCoupling layer_coupling{LayerCoupling::None};
  double bond_strength{1.0};
  uint64_t created_version{0};
  uint64_t deleted_version{kInfVersion};
};

struct Path {
  std::vector<uint32_t> nodes;
  std::vector<uint32_t> edges;
  double score{1.0};
  bool contains_hypothetical{false};
  bool contains_contradiction{false};
};

struct MemoryBundle {
  bool abstain{false};
  std::string reason;
  uint32_t target_node{0};
  uint64_t snapshot_version{0};
  std::vector<Path> paths;
  std::vector<uint32_t> semantic_candidates;
  std::vector<std::string> why_retrieved;
  double confidence{0.0};
  double degeneracy{0.0};
  double diversity{0.0};
  double contradiction{0.0};
  double lattice_score{0.0};
  std::vector<uint32_t> lattice_neighbors;
  std::vector<std::string> lattice_explanation;
};

struct SearchResult {
  uint32_t node_id{0};
  double score{0.0};
};

struct BatchInput {
  std::vector<NodeInput> nodes;
  std::vector<EdgeInput> edges;
};

struct BatchResult {
  std::vector<uint32_t> node_ids;
  std::vector<uint32_t> edge_ids;
};

enum class ExtractionRole : uint8_t {
  Node = 0,
  Root = 1,
  Symptom = 2,
  Impact = 3
};

static constexpr uint32_t kExtractionSchemaVersion = 1;

struct ExtractionNode {
  std::string external_id;
  std::string content;
  std::vector<float> vector;
  uint64_t signature{0};
  uint32_t incident{0};
  ExtractionRole role{ExtractionRole::Node};
  std::map<std::string, std::string> metadata;
  std::optional<LatticeCoord> lattice;
  DefectType defect_type{DefectType::None};
};

struct ExtractionRelation {
  std::string from_external_id;
  std::string to_external_id;
  EdgeOrigin origin{EdgeOrigin::Observed};
  EdgeRole role{EdgeRole::Supports};
  double confidence{0.9};
  std::string evidence_id;
  std::string evidence_uri;
  std::string evidence_text;
  std::map<std::string, std::string> metadata;
  BondType bond_type{BondType::Sigma};
  DefectType defect_type{DefectType::None};
  LayerCoupling layer_coupling{LayerCoupling::SameLayer};
  double bond_strength{0.85};
};

struct ExtractionInput {
  uint32_t schema_version{kExtractionSchemaVersion};
  std::string source_id;
  std::string source_uri;
  std::string extraction_run_id;
  int32_t layer{0};
  uint32_t incident{0};
  uint64_t signature{0};
  bool place_missing_lattice{true};
  bool idempotent{true};
  std::vector<ExtractionNode> nodes;
  std::vector<ExtractionRelation> relations;
};

struct ExtractionResult {
  std::unordered_map<std::string, uint32_t> external_to_node_id;
  std::vector<uint32_t> inserted_node_ids;
  std::vector<uint32_t> existing_node_ids;
  std::vector<uint32_t> inserted_edge_ids;
};

inline uint64_t signature_for(uint32_t service, uint32_t symptom) {
  return (1ull << (service % 16)) | (1ull << (16 + (symptom % 16)));
}

} // namespace graphene
