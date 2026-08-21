#include "graphene/fiber_bundle.hpp"

#include <algorithm>
#include <cmath>
#include <map>
#include <numeric>
#include <set>
#include <sstream>

namespace graphene {
namespace {

constexpr double kEligibilityThreshold = 0.50;

uint64_t append_hash(uint64_t hash, const std::string& text) {
  for (unsigned char value : text) { hash ^= value; hash *= 1099511628211ULL; }
  return hash;
}

std::string join(const std::vector<std::string>& values, char separator) {
  std::ostringstream output;
  for (size_t index = 0; index < values.size(); ++index) { if (index) output << separator; output << values[index]; }
  return output.str();
}

std::vector<std::string> sources(const DialecticPath& path) {
  std::set<std::string> unique;
  for (const auto& evidence : path.evidence) if (!evidence.source_id.empty()) unique.insert(evidence.source_id);
  return {unique.begin(), unique.end()};
}

std::vector<std::string> families(const DialecticPath& path) {
  std::set<std::string> unique;
  for (const auto& evidence : path.evidence) {
    if (!evidence.evidence_family_id.empty()) unique.insert("family:" + evidence.evidence_family_id);
    else if (!evidence.content_hash.empty()) unique.insert("content:" + evidence.content_hash);
    else if (!evidence.derivation_id.empty()) unique.insert("derivation:" + evidence.derivation_id);
    else if (!evidence.source_id.empty()) unique.insert("source:" + evidence.source_id);
  }
  return {unique.begin(), unique.end()};
}

std::vector<std::string> derivations(const DialecticPath& path) {
  std::set<std::string> unique;
  for (const auto& evidence : path.evidence) if (!evidence.derivation_id.empty()) unique.insert(evidence.derivation_id);
  return {unique.begin(), unique.end()};
}

FiberPathRole classify(const DialecticPath& path) {
  if (path.role_hint == PathRoleHint::Noise || path.query_relevance < kEligibilityThreshold || path.target_consistency < kEligibilityThreshold) return FiberPathRole::Noise;
  if (path.role_hint == PathRoleHint::Opposition || path.contains_contradiction || path.semantic_verification == SemanticVerificationStatus::Contradicted) return FiberPathRole::Opposition;
  if (path.role_hint == PathRoleHint::Support || path.role_hint == PathRoleHint::Auto) return FiberPathRole::Support;
  return FiberPathRole::Unknown;
}

std::string route_signature(const FiberPath& path) {
  std::ostringstream output;
  output << path.target_node << ':' << path.anchor_node << ':';
  for (uint32_t node : path.nodes) output << 'n' << node << ',';
  output << '|';
  for (uint32_t edge : path.edges) output << 'e' << edge << ',';
  return output.str();
}

uint64_t path_hash(const FiberPath& path) {
  uint64_t hash = 1469598103934665603ULL;
  hash = append_hash(hash, path.route_signature);
  hash = append_hash(hash, path.evidence_lineage_signature);
  return append_hash(hash, std::to_string(static_cast<int>(path.role)));
}

bool overlaps(const std::vector<std::string>& left, const std::vector<std::string>& right) {
  size_t i = 0, j = 0;
  while (i < left.size() && j < right.size()) {
    if (left[i] == right[j]) return true;
    if (left[i] < right[j]) ++i; else ++j;
  }
  return false;
}

void merge_strings(std::vector<std::string>* target, const std::vector<std::string>& source) {
  target->insert(target->end(), source.begin(), source.end());
  std::sort(target->begin(), target->end());
  target->erase(std::unique(target->begin(), target->end()), target->end());
}

SemanticVerificationStatus conservative_semantic_status(SemanticVerificationStatus left, SemanticVerificationStatus right) {
  if (left == SemanticVerificationStatus::Contradicted || right == SemanticVerificationStatus::Contradicted) return SemanticVerificationStatus::Contradicted;
  if (left == SemanticVerificationStatus::Verified && right == SemanticVerificationStatus::Verified) return SemanticVerificationStatus::Verified;
  if (left == SemanticVerificationStatus::NotApplicable && right == SemanticVerificationStatus::NotApplicable) return SemanticVerificationStatus::NotApplicable;
  return SemanticVerificationStatus::Unverified;
}

void merge_exact_path(FiberPath* retained, const FiberPath& duplicate) {
  retained->confidence = std::min(retained->confidence, duplicate.confidence);
  retained->query_relevance = std::min(retained->query_relevance, duplicate.query_relevance);
  retained->target_consistency = std::min(retained->target_consistency, duplicate.target_consistency);
  retained->completeness = std::min(retained->completeness, duplicate.completeness);
  retained->temporal_consistency = std::min(retained->temporal_consistency, duplicate.temporal_consistency);
  retained->provenance_quality = std::min(retained->provenance_quality, duplicate.provenance_quality);
  retained->semantic_verification = conservative_semantic_status(retained->semantic_verification, duplicate.semantic_verification);
  retained->eligible_for_support = retained->eligible_for_support && duplicate.eligible_for_support;
  retained->eligible_for_opposition = retained->eligible_for_opposition && duplicate.eligible_for_opposition;
  retained->irrelevant = retained->irrelevant || duplicate.irrelevant;
  retained->contains_contradiction = retained->contains_contradiction || duplicate.contains_contradiction;
  retained->contains_hypothetical = retained->contains_hypothetical || duplicate.contains_hypothetical;
  retained->validity.graph_continuous = retained->validity.graph_continuous && duplicate.validity.graph_continuous;
  retained->validity.reaches_target = retained->validity.reaches_target && duplicate.validity.reaches_target;
  retained->validity.satisfies_joint_requirements = retained->validity.satisfies_joint_requirements && duplicate.validity.satisfies_joint_requirements;
  retained->validity.relation_types_valid = retained->validity.relation_types_valid && duplicate.validity.relation_types_valid;
  retained->validity.every_critical_edge_has_evidence = retained->validity.every_critical_edge_has_evidence && duplicate.validity.every_critical_edge_has_evidence;
  retained->validity.temporal_windows_overlap = retained->validity.temporal_windows_overlap && duplicate.validity.temporal_windows_overlap;
  retained->validity.critical_edge_coverage = std::min(retained->validity.critical_edge_coverage, duplicate.validity.critical_edge_coverage);
  retained->validity.completeness_score = std::min(retained->validity.completeness_score, duplicate.validity.completeness_score);
  merge_strings(&retained->source_lineage, duplicate.source_lineage);
  merge_strings(&retained->evidence_family_lineage, duplicate.evidence_family_lineage);
  merge_strings(&retained->derivation_lineage, duplicate.derivation_lineage);
  merge_strings(&retained->verification_findings, duplicate.verification_findings);
  merge_strings(&retained->validity.findings, duplicate.validity.findings);
  if (retained->verifier_version != duplicate.verifier_version) {
    retained->verifier_version = "mixed";
    retained->verification_findings.push_back("exact duplicate received results from different verifier versions");
    std::sort(retained->verification_findings.begin(), retained->verification_findings.end());
    retained->verification_findings.erase(std::unique(retained->verification_findings.begin(), retained->verification_findings.end()), retained->verification_findings.end());
  }
}

bool contains_edge(const std::vector<uint32_t>& edges, uint32_t edge) { return std::find(edges.begin(), edges.end(), edge) != edges.end(); }

PathValidityAssessment assess_validity(const DialecticPath& path, uint32_t target_node) {
  PathValidityAssessment result;
  result.reaches_target = !path.nodes.empty() && path.nodes.front() == target_node;
  if (!result.reaches_target) result.findings.push_back("path does not begin at the asserted target");
  const bool anchor_matches = !path.nodes.empty() && path.nodes.back() == path.anchor_node;
  const bool adjacent_node_cycle = std::adjacent_find(path.nodes.begin(), path.nodes.end()) != path.nodes.end();
  result.graph_continuous = result.reaches_target && anchor_matches && path.nodes.size() >= 2 && !path.edges.empty() && !adjacent_node_cycle;
  if (!result.graph_continuous) result.findings.push_back("path chain is empty, cyclic, or disconnected from its anchor");
  result.satisfies_joint_requirements = true;
  for (const auto& requirement : path.joint_requirements) {
    bool complete = requirement.all_sources_present && !requirement.source_nodes.empty() && !requirement.member_edges.empty();
    for (uint32_t member_edge : requirement.member_edges) complete = complete && contains_edge(path.edges, member_edge);
    if (!complete) { result.satisfies_joint_requirements = false; result.findings.push_back("incomplete joint requirement: " + requirement.hyperedge_id); }
  }
  std::set<uint32_t> unique_edges(path.edges.begin(), path.edges.end());
  std::set<uint32_t> missing_evidence_edges;
  result.relation_types_valid = true;
  for (const auto& finding : path.provenance_findings) {
    if (finding.code == "MISSING_EVIDENCE") missing_evidence_edges.insert(finding.edge_id);
    if (finding.code == "RELATION_TYPE_INVALID" || finding.code == "RELATION_DOMAIN_INVALID" || finding.code == "RELATION_RANGE_INVALID") result.relation_types_valid = false;
  }
  if (!result.relation_types_valid) result.findings.push_back("relation ontology validation failed");
  if (unique_edges.empty()) result.critical_edge_coverage = 0.0;
  else {
    size_t missing = 0;
    for (uint32_t edge : missing_evidence_edges) if (unique_edges.count(edge) != 0) ++missing;
    result.critical_edge_coverage = std::clamp(1.0 - static_cast<double>(missing) / static_cast<double>(unique_edges.size()), 0.0, 1.0);
  }
  result.every_critical_edge_has_evidence = !unique_edges.empty() && missing_evidence_edges.empty() && !path.evidence.empty();
  if (!result.every_critical_edge_has_evidence) result.findings.push_back("one or more critical edges lack source evidence");
  result.temporal_windows_overlap = path.temporal_consistent;
  if (!result.temporal_windows_overlap) result.findings.push_back("path evidence is not valid in one common temporal window");
  result.completeness_score = std::min(std::clamp(path.completeness, 0.0, 1.0), result.critical_edge_coverage);
  if (!result.graph_continuous || !result.reaches_target || !result.satisfies_joint_requirements || !result.relation_types_valid || !result.temporal_windows_overlap) result.completeness_score = 0.0;
  return result;
}

struct DisjointSet {
  explicit DisjointSet(size_t size) : parent(size), rank(size, 0) { std::iota(parent.begin(), parent.end(), 0); }
  size_t find(size_t value) { if (parent[value] != value) parent[value] = find(parent[value]); return parent[value]; }
  void unite(size_t left, size_t right) { left = find(left); right = find(right); if (left == right) return; if (rank[left] < rank[right]) std::swap(left, right); parent[right] = left; if (rank[left] == rank[right]) ++rank[left]; }
  std::vector<size_t> parent;
  std::vector<uint8_t> rank;
};

double quality(const FiberPath& path) { return std::clamp(path.confidence * path.query_relevance * path.target_consistency * path.completeness * path.provenance_quality, 0.0, 1.0); }

double jaccard_distance(const std::vector<uint32_t>& left, const std::vector<uint32_t>& right) {
  std::set<uint32_t> a(left.begin(), left.end()), b(right.begin(), right.end());
  size_t intersection = 0;
  for (uint32_t value : a) if (b.count(value)) ++intersection;
  const size_t union_size = a.size() + b.size() - intersection;
  return union_size == 0 ? 0.0 : 1.0 - static_cast<double>(intersection) / static_cast<double>(union_size);
}

void append_path_state_hash(uint64_t* hash, const FiberPath& path) {
  *hash = append_hash(*hash, "q" + std::to_string(path.query_relevance));
  *hash = append_hash(*hash, "t" + std::to_string(path.target_consistency));
  *hash = append_hash(*hash, "c" + std::to_string(path.completeness));
  *hash = append_hash(*hash, "p" + std::to_string(path.provenance_quality));
  *hash = append_hash(*hash, "v" + std::to_string(static_cast<int>(path.semantic_verification)));
  *hash = append_hash(*hash, "vv" + path.verifier_version);
  for (const auto& finding : path.verification_findings) *hash = append_hash(*hash, "vf" + finding);
  *hash = append_hash(*hash, path.validity.graph_continuous ? "gc1" : "gc0");
  *hash = append_hash(*hash, path.validity.reaches_target ? "rt1" : "rt0");
  *hash = append_hash(*hash, path.validity.satisfies_joint_requirements ? "jr1" : "jr0");
  *hash = append_hash(*hash, path.validity.every_critical_edge_has_evidence ? "ev1" : "ev0");
  *hash = append_hash(*hash, path.validity.temporal_windows_overlap ? "tw1" : "tw0");
  *hash = append_hash(*hash, "ec" + std::to_string(path.validity.critical_edge_coverage));
}

}  // namespace

const char* fiber_path_role_name(FiberPathRole role) {
  switch (role) { case FiberPathRole::Support: return "support"; case FiberPathRole::Opposition: return "opposition"; case FiberPathRole::Noise: return "noise"; case FiberPathRole::Unknown: return "unknown"; }
  return "unknown";
}

FiberBundle FiberBundleBuilder::build(const BundleSet& input) const {
  FiberBundle bundle;
  bundle.snapshot_version = input.snapshot_version;
  bundle.warnings = input.warnings;
  bundle.visited_states = input.visited_states;
  bundle.truncated = input.truncated;
  std::vector<RootBundle> roots = input.roots;
  std::sort(roots.begin(), roots.end(), [](const RootBundle& left, const RootBundle& right) { return left.root_node < right.root_node; });
  for (const RootBundle& root : roots) {
    TargetFiber fiber;
    fiber.target_node = root.root_node;
    fiber.raw_path_count = root.paths.size();
    std::map<std::string, FiberPath> exact_paths;
    for (const DialecticPath& path : root.paths) {
      FiberPath converted;
      converted.target_node = root.root_node; converted.anchor_node = path.anchor_node; converted.nodes = path.nodes; converted.edges = path.edges; converted.evidence = path.evidence;
      converted.source_lineage = sources(path); converted.evidence_family_lineage = families(path); converted.derivation_lineage = derivations(path); converted.verifier_version = path.verifier_version; converted.verification_findings = path.verification_findings;
      converted.confidence = std::clamp(path.score, 0.0, 1.0); converted.query_relevance = std::clamp(path.query_relevance, 0.0, 1.0); converted.target_consistency = std::clamp(path.target_consistency, 0.0, 1.0); converted.temporal_consistency = path.temporal_consistent ? 1.0 : 0.0;
      converted.validity = assess_validity(path, root.root_node);
      converted.validity.findings.insert(converted.validity.findings.end(), converted.verification_findings.begin(), converted.verification_findings.end());
      std::sort(converted.validity.findings.begin(), converted.validity.findings.end()); converted.validity.findings.erase(std::unique(converted.validity.findings.begin(), converted.validity.findings.end()), converted.validity.findings.end());
      converted.completeness = converted.validity.completeness_score;
      const double finding_quality = path.edges.empty() ? 0.0 : std::clamp(1.0 - static_cast<double>(path.provenance_findings.size()) / static_cast<double>(path.edges.size()), 0.0, 1.0);
      converted.provenance_quality = std::min(finding_quality, converted.validity.critical_edge_coverage);
      converted.contains_contradiction = path.contains_contradiction; converted.contains_hypothetical = path.contains_hypothetical; converted.semantic_verification = path.semantic_verification; converted.role = classify(path); converted.irrelevant = converted.role == FiberPathRole::Noise;
      const bool structurally_valid = converted.validity.graph_continuous && converted.validity.reaches_target && converted.validity.satisfies_joint_requirements && converted.validity.relation_types_valid && converted.validity.every_critical_edge_has_evidence && converted.validity.temporal_windows_overlap;
      converted.eligible_for_support = converted.role == FiberPathRole::Support && structurally_valid && converted.query_relevance >= kEligibilityThreshold && converted.target_consistency >= kEligibilityThreshold && converted.completeness >= kEligibilityThreshold && converted.provenance_quality > 0.0;
      converted.eligible_for_opposition = converted.role == FiberPathRole::Opposition && structurally_valid && converted.query_relevance >= kEligibilityThreshold && converted.target_consistency >= kEligibilityThreshold && converted.provenance_quality > 0.0;
      converted.route_signature = route_signature(converted); converted.evidence_lineage_signature = join(converted.evidence_family_lineage, '\x1f'); converted.causal_ancestry_signature = join(converted.derivation_lineage, '\x1f'); converted.id = path_hash(converted);
      const std::string exact = converted.route_signature + '|' + converted.evidence_lineage_signature + '|' + std::to_string(static_cast<int>(converted.role));
      auto [existing, inserted] = exact_paths.emplace(exact, converted); if (!inserted) merge_exact_path(&existing->second, converted);
    }
    for (auto& entry : exact_paths) fiber.paths.push_back(std::move(entry.second));
    std::sort(fiber.paths.begin(), fiber.paths.end(), [](const FiberPath& left, const FiberPath& right) { return left.id != right.id ? left.id < right.id : left.edges < right.edges; });
    fiber.unique_route_count = fiber.paths.size();
    for (const FiberPath& path : fiber.paths) { if (path.role == FiberPathRole::Noise) ++fiber.noise_path_count; else { ++fiber.relevant_path_count; if (!path.eligible_for_support && !path.eligible_for_opposition) ++fiber.invalid_path_count; } }
    fiber.retrieval_noise_ratio = fiber.paths.empty() ? 0.0 : static_cast<double>(fiber.noise_path_count) / static_cast<double>(fiber.paths.size());
    fiber.invalid_path_ratio = fiber.relevant_path_count == 0 ? 0.0 : static_cast<double>(fiber.invalid_path_count) / static_cast<double>(fiber.relevant_path_count);
    DisjointSet dsu(fiber.paths.size());
    for (size_t left = 0; left < fiber.paths.size(); ++left) { if (fiber.paths[left].role == FiberPathRole::Noise) continue; for (size_t right = left + 1; right < fiber.paths.size(); ++right) { if (fiber.paths[right].role == FiberPathRole::Noise || fiber.paths[left].role != fiber.paths[right].role) continue; if (overlaps(fiber.paths[left].source_lineage, fiber.paths[right].source_lineage) || overlaps(fiber.paths[left].evidence_family_lineage, fiber.paths[right].evidence_family_lineage) || overlaps(fiber.paths[left].derivation_lineage, fiber.paths[right].derivation_lineage)) dsu.unite(left, right); } }
    std::map<size_t, std::vector<size_t>> components;
    for (size_t index = 0; index < fiber.paths.size(); ++index) if (fiber.paths[index].role != FiberPathRole::Noise) components[dsu.find(index)].push_back(index);
    std::vector<const FiberPath*> representatives; double completeness = 0.0, evidence_coverage = 0.0;
    for (const auto& entry : components) {
      EvidenceCorrelationGroup group; std::set<std::string> family_ids, derivation_ids; const FiberPath* best = nullptr;
      for (size_t index : entry.second) { const FiberPath& path = fiber.paths[index]; group.path_ids.push_back(path.id); family_ids.insert(path.evidence_family_lineage.begin(), path.evidence_family_lineage.end()); derivation_ids.insert(path.derivation_lineage.begin(), path.derivation_lineage.end()); if (!best || quality(path) > quality(*best) || (quality(path) == quality(*best) && path.id < best->id)) best = &path; }
      group.evidence_family_ids.assign(family_ids.begin(), family_ids.end()); group.derivation_ids.assign(derivation_ids.begin(), derivation_ids.end());
      if (best) { group.representative_path_id = best->id; group.role = best->role; uint64_t group_hash = 1469598103934665603ULL; group_hash = append_hash(group_hash, join(group.evidence_family_ids, '\x1f')); group_hash = append_hash(group_hash, join(group.derivation_ids, '\x1f')); group.id = append_hash(group_hash, std::to_string(static_cast<int>(group.role))); group.independent_support = best->eligible_for_support && !group.evidence_family_ids.empty(); if (group.independent_support) { ++fiber.independent_evidence_family_count; representatives.push_back(best); completeness += best->completeness; evidence_coverage += best->validity.critical_edge_coverage; } if (best->eligible_for_opposition) fiber.contradiction_mass = std::max(fiber.contradiction_mass, quality(*best)); }
      std::sort(group.path_ids.begin(), group.path_ids.end()); fiber.correlation_groups.push_back(std::move(group));
    }
    std::sort(fiber.correlation_groups.begin(), fiber.correlation_groups.end(), [](const EvidenceCorrelationGroup& left, const EvidenceCorrelationGroup& right) { return left.id < right.id; });
    fiber.independent_path_count = fiber.independent_evidence_family_count; fiber.independent_support_score = 1.0 - std::exp(-0.7 * static_cast<double>(fiber.independent_evidence_family_count)); fiber.degeneracy = fiber.independent_support_score; fiber.completeness_score = representatives.empty() ? 0.0 : completeness / static_cast<double>(representatives.size()); fiber.evidence_coverage = representatives.empty() ? 0.0 : evidence_coverage / static_cast<double>(representatives.size());
    if (representatives.size() > 1) { double sum = 0.0; size_t pairs = 0; for (size_t left = 0; left < representatives.size(); ++left) for (size_t right = left + 1; right < representatives.size(); ++right) { sum += jaccard_distance(representatives[left]->edges, representatives[right]->edges); ++pairs; } fiber.relevant_route_diversity = pairs == 0 ? 0.0 : std::clamp(sum / static_cast<double>(pairs), 0.0, 1.0); }
    bundle.fibers.push_back(std::move(fiber));
  }
  bundle.immutable_hash = hash(bundle); return bundle;
}

uint64_t FiberBundleBuilder::hash(const FiberBundle& bundle) {
  uint64_t value = 1469598103934665603ULL;
  value = append_hash(value, std::to_string(bundle.schema_version)); value = append_hash(value, std::to_string(bundle.snapshot_version)); value = append_hash(value, bundle.truncated ? "1" : "0");
  for (const TargetFiber& fiber : bundle.fibers) { value = append_hash(value, "t" + std::to_string(fiber.target_node)); value = append_hash(value, "i" + std::to_string(fiber.independent_evidence_family_count)); value = append_hash(value, "n" + std::to_string(fiber.noise_path_count)); value = append_hash(value, "x" + std::to_string(fiber.invalid_path_count)); for (const FiberPath& path : fiber.paths) { value = append_hash(value, "p" + std::to_string(path.id)); value = append_hash(value, "r" + std::to_string(static_cast<int>(path.role))); append_path_state_hash(&value, path); } for (const EvidenceCorrelationGroup& group : fiber.correlation_groups) value = append_hash(value, "g" + std::to_string(group.id)); }
  for (const std::string& warning : bundle.warnings) value = append_hash(value, "w" + warning);
  return value;
}

}  // namespace graphene
