#include "graphene/dialectic.hpp"

#include <algorithm>
#include <bit>
#include <charconv>
#include <cmath>
#include <deque>
#include <limits>
#include <map>
#include <set>
#include <unordered_set>

namespace graphene {
namespace {

double clamp01(double value) {
  return std::clamp(value, 0.0, 1.0);
}

bool edge_allowed(const Edge& edge, QueryMode mode) {
  if (mode == QueryMode::Empirical) {
    if (edge.origin != EdgeOrigin::Observed &&
        edge.origin != EdgeOrigin::Discovered) {
      return false;
    }
    if (edge.role == EdgeRole::Analogical) return false;
    if (edge.role == EdgeRole::Compressed) {
      const auto derivation = edge.metadata.find("derived_from");
      if (derivation == edge.metadata.end() || derivation->second.empty()) {
        return false;
      }
    }
    return true;
  }
  if (mode == QueryMode::Balanced) return edge.origin != EdgeOrigin::Hypothetical;
  return true;
}

bool signature_matches(uint64_t node_signature, uint64_t query_signature) {
  if (query_signature == 0 || node_signature == 0) return true;
  const int query_bits = static_cast<int>(std::popcount(query_signature));
  const int required = std::max(1, query_bits - 1);
  return static_cast<int>(std::popcount(node_signature & query_signature)) >= required;
}

std::string metadata_value(const std::map<std::string, std::string>& metadata,
                           const std::string& key) {
  const auto it = metadata.find(key);
  return it == metadata.end() ? std::string{} : it->second;
}

bool parse_u32_list(const std::string& encoded, std::vector<uint32_t>* out) {
  if (!out || encoded.empty()) return false;
  std::vector<uint32_t> values;
  size_t start = 0;
  while (start < encoded.size()) {
    const size_t end = encoded.find(',', start);
    const size_t length =
        end == std::string::npos ? encoded.size() - start : end - start;
    if (length == 0) return false;
    uint32_t value = 0;
    const char* first = encoded.data() + start;
    const char* last = first + length;
    const auto parsed = std::from_chars(first, last, value);
    if (parsed.ec != std::errc{} || parsed.ptr != last) return false;
    values.push_back(value);
    if (end == std::string::npos) break;
    start = end + 1;
  }
  if (values.empty()) return false;
  std::sort(values.begin(), values.end());
  if (std::adjacent_find(values.begin(), values.end()) != values.end()) {
    return false;
  }
  *out = std::move(values);
  return true;
}

void append_provenance(const Edge& edge, DialecticPath* path) {
  const EdgeProvenance provenance = assess_edge_provenance(edge);
  path->evidence.insert(path->evidence.end(), provenance.evidence.begin(),
                        provenance.evidence.end());
  path->provenance_findings.insert(path->provenance_findings.end(),
                                   provenance.findings.begin(),
                                   provenance.findings.end());
}

struct ExpansionState {
  uint32_t node{0};
  uint32_t anchor{0};
  double score{0.0};
  std::vector<uint32_t> reverse_nodes;
  std::vector<uint32_t> reverse_edges;
  bool contradiction{false};
  bool hypothetical{false};
  std::vector<EvidenceRef> evidence;
  std::vector<ProvenanceFinding> findings;
  std::vector<JointRequirement> joint_requirements;
  uint32_t depth{0};
};

const RootBundle* find_root(const BundleSet& bundles, uint32_t root_node) {
  for (const auto& root : bundles.roots) {
    if (root.root_node == root_node) return &root;
  }
  return nullptr;
}

void append_unique(std::vector<std::string>* target, std::string value) {
  if (std::find(target->begin(), target->end(), value) == target->end()) {
    target->push_back(std::move(value));
  }
}

void append_unique(std::vector<uint32_t>* target, uint32_t value) {
  if (std::find(target->begin(), target->end(), value) == target->end()) {
    target->push_back(value);
  }
}

DialecticOptions bounded_options(DialecticOptions options) {
  options.semantic_candidates = std::clamp<size_t>(options.semantic_candidates, 1, 64);
  options.max_hops = std::clamp<uint32_t>(options.max_hops, 1, 16);
  options.max_paths = std::clamp<size_t>(options.max_paths, 1, 256);
  options.max_paths_per_root = std::clamp<size_t>(options.max_paths_per_root, 1, 64);
  options.max_visited_states = std::clamp<size_t>(options.max_visited_states, 1, 200000);
