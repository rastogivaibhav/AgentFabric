#pragma once

#include "graphene/db.hpp"
#include "graphene/epistemic.hpp"

#include <cstddef>
#include <cstdint>
#include <string>
#include <vector>

namespace graphene {

struct JointRequirement {
  std::string hyperedge_id;
  uint32_t target_node{0};
  std::vector<uint32_t> source_nodes;
  std::vector<uint32_t> member_edges;
  bool all_sources_present{false};
};

enum class PathRoleHint : uint8_t {
  Auto,
  Support,
  Opposition,
  Noise
};

enum class SemanticVerificationStatus : uint8_t {
  Unverified,
  Verified,
  Contradicted,
  NotApplicable
};

struct DialecticPath {
  uint32_t root_node{0};
  uint32_t anchor_node{0};
  std::vector<uint32_t> nodes;
  std::vector<uint32_t> edges;
  double score{0.0};
  bool contains_contradiction{false};
  bool contains_hypothetical{false};
  bool temporal_consistent{true};
  std::vector<EvidenceRef> evidence;
  std::vector<ProvenanceFinding> provenance_findings;
  std::vector<JointRequirement> joint_requirements;

  // Path-verifier outputs. The graph expander defaults to an admissible but
  // unverified path. A domain verifier may only tighten these coordinates.
  double query_relevance{1.0};
  double target_consistency{1.0};
  double completeness{1.0};
  PathRoleHint role_hint{PathRoleHint::Auto};
  SemanticVerificationStatus semantic_verification{
      SemanticVerificationStatus::Unverified};
  std::string verifier_version;
  std::vector<std::string> verification_findings;
};

struct RootBundle {
  uint32_t root_node{0};
  std::vector<DialecticPath> paths;
  double confidence{0.0};
  double degeneracy{0.0};
  double diversity{0.0};
  double contradiction_ratio{0.0};
  double evidence_coverage{0.0};
  double provenance_risk{0.0};
};

struct BundleSet {
  uint64_t snapshot_version{0};
  std::vector<uint32_t> semantic_candidates;
  std::vector<RootBundle> roots;
  size_t visited_states{0};
  bool truncated{false};
  std::vector<std::string> warnings;
};

struct PathReference {
  uint32_t root_node{0};
  size_t path_index{0};
  std::string reason;
};

struct ConvergedAnswer {
  bool has_answer{false};
  uint32_t primary_node{0};
  double confidence{0.0};
  double false_promotion_risk{0.0};
  std::vector<PathReference> selected_paths;
  std::vector<PathReference> discarded_paths;
  std::vector<uint32_t> evidence_edges;
  std::vector<std::string> residual_uncertainty;
};

struct OppositionReport {
  std::vector<std::string> challenged_claims;
  std::vector<std::string> falsification_questions;
  std::vector<uint32_t> reopen_nodes;
  double opposition_score{0.0};
  bool requests_reexpansion{false};
};

struct DialecticSynthesis {
  bool has_answer{false};
  uint32_t primary_node{0};
  double confidence{0.0};
  std::string epistemic_status{"abstain"};
  std::vector<uint32_t> evidence_edges;
  std::vector<std::string> residual_uncertainty;
};

struct DialecticOptions {
  QueryMode mode{QueryMode::Balanced};
  size_t semantic_candidates{12};
  uint32_t max_hops{6};
  size_t max_paths{32};
  size_t max_paths_per_root{8};
  size_t max_visited_states{20000};
  size_t max_selected_paths{4};
  uint32_t max_opposition_rounds{1};
  double minimum_confidence{0.45};
  double reexpansion_threshold{0.25};
  // Opposition and escape can request specific hypotheses to be reopened. The
  // expander turns root nodes into downstream anchor candidates instead of
  // treating the root itself as fresh corroborating evidence.
  std::vector<uint32_t> reopen_nodes;
  std::string as_of;
};

struct DialecticResult {
  BundleSet initial_bundle;
  ConvergedAnswer initial_convergence;
  OppositionReport initial_opposition;
  BundleSet reopened_bundle;
  bool has_reopened_bundle{false};
  ConvergedAnswer final_convergence;
  OppositionReport final_opposition;
  DialecticSynthesis synthesis;
  uint32_t rounds{0};
  bool durable_writes{false};
};

class DialecticEngine {
 public:
  explicit DialecticEngine(const GrapheneDB& db);

  BundleSet expand(const std::vector<float>& query,
                   uint64_t query_signature,
                   const DialecticOptions& options = {},
                   uint64_t snapshot_version = kInfVersion) const;

  // Legacy BundleSet methods remain for source compatibility. Complete
  // HypoKosh runtime convergence is performed over FiberBundle v2 by the
  // EpistemicController.
  ConvergedAnswer converge(const BundleSet& bundles,
                           const DialecticOptions& options = {}) const;

  OppositionReport oppose(const BundleSet& bundles,
                          const ConvergedAnswer& converged,
                          const DialecticOptions& options = {}) const;

  DialecticResult reason(const std::vector<float>& query,
                         uint64_t query_signature,
                         const DialecticOptions& options = {},
                         uint64_t snapshot_version = kInfVersion) const;

 private:
  const GrapheneDB& db_;
};

} // namespace graphene
