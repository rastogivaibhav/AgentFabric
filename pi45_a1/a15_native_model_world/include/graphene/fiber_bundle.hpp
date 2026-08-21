#pragma once

#include "graphene/dialectic.hpp"

#include <cstddef>
#include <cstdint>
#include <string>
#include <vector>

namespace graphene {

enum class FiberPathRole : uint8_t {
  Support,
  Opposition,
  Noise,
  Unknown
};

struct PathValidityAssessment {
  bool graph_continuous{false};
  bool reaches_target{false};
  bool satisfies_joint_requirements{false};
  bool relation_types_valid{true};
  bool every_critical_edge_has_evidence{false};
  bool temporal_windows_overlap{false};
  double critical_edge_coverage{0.0};
  double completeness_score{0.0};
  std::vector<std::string> findings;
};

struct FiberPath {
  uint64_t id{0};
  uint32_t target_node{0};
  uint32_t anchor_node{0};
  std::vector<uint32_t> nodes;
  std::vector<uint32_t> edges;
  std::vector<EvidenceRef> evidence;
  std::vector<std::string> source_lineage;
  std::vector<std::string> evidence_family_lineage;
  std::vector<std::string> derivation_lineage;

  std::string route_signature;
  std::string evidence_lineage_signature;
  std::string causal_ancestry_signature;
  std::string verifier_version;
  std::vector<std::string> verification_findings;

  double confidence{0.0};
  double query_relevance{1.0};
  double target_consistency{1.0};
  double completeness{1.0};
  double temporal_consistency{0.0};
  double provenance_quality{0.0};
  PathValidityAssessment validity;

  FiberPathRole role{FiberPathRole::Unknown};
  SemanticVerificationStatus semantic_verification{
      SemanticVerificationStatus::Unverified};
  bool eligible_for_support{false};
  bool eligible_for_opposition{false};
  bool irrelevant{false};
  bool contains_contradiction{false};
  bool contains_hypothetical{false};
};

struct EvidenceCorrelationGroup {
  uint64_t id{0};
  FiberPathRole role{FiberPathRole::Unknown};
  std::vector<uint64_t> path_ids;
  std::vector<std::string> evidence_family_ids;
  std::vector<std::string> derivation_ids;
  uint64_t representative_path_id{0};
  bool independent_support{false};
};

struct TargetFiber {
  uint32_t target_node{0};
  std::vector<FiberPath> paths;
  std::vector<EvidenceCorrelationGroup> correlation_groups;

  size_t raw_path_count{0};
  size_t unique_route_count{0};
  size_t relevant_path_count{0};
  size_t noise_path_count{0};
  size_t invalid_path_count{0};
  size_t independent_evidence_family_count{0};

  // Backward-compatible alias for independent_evidence_family_count.
  size_t independent_path_count{0};
  double degeneracy{0.0};

  double relevant_route_diversity{0.0};
  double independent_support_score{0.0};
  double contradiction_mass{0.0};
  double retrieval_noise_ratio{0.0};
  double invalid_path_ratio{0.0};
  double completeness_score{0.0};
  double evidence_coverage{0.0};
};

struct FiberBundle {
  uint32_t schema_version{2};
  uint64_t snapshot_version{0};
  std::vector<TargetFiber> fibers;
  std::vector<std::string> warnings;
  size_t visited_states{0};
  bool truncated{false};
  uint64_t immutable_hash{0};
};

class FiberBundleBuilder {
 public:
  FiberBundle build(const BundleSet& bundles) const;
  static uint64_t hash(const FiberBundle& bundle);
};

const char* fiber_path_role_name(FiberPathRole role);

}  // namespace graphene
