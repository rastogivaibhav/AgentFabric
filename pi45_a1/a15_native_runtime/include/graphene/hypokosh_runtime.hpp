#pragma once

#include "graphene/epistemic_control.hpp"
#include "graphene/escape.hpp"
#include "graphene/model_world.hpp"
#include "graphene/path_verifier.hpp"
#include "graphene/self_healing.hpp"

#include <cstdint>
#include <string>
#include <vector>

namespace graphene {

enum class GovernedEpistemicStatus : uint8_t {
  Resolved,
  ProvisionallyResolved,
  Contested,
  EvidenceRequired,
  Abstain,
  Speculative
};

struct RuntimeOptions {
  DialecticOptions dialectic;
  StabilityWeights stability_weights;
  StabilityThresholds stability_thresholds;
  LyapunovWeights lyapunov_weights;
  LyapunovTargets lyapunov_targets;
  const PathVerifier* path_verifier{nullptr};
  uint32_t max_recursive_cycles{2};
  // Recovery recursion remains available for incomplete evidence. Opposition-
  // only secondary research is opt-in so an adequate first answer is not
  // automatically expanded again.
  bool enable_opposition_research{false};
  // A completed FiberBundle can remain unchanged while the search frontier
  // advances through intermediate nodes. Permit bounded patience for that case.
  uint32_t unchanged_recovery_patience{2};
  bool update_model_world{true};
};

struct ReasoningReceipt {
  uint64_t snapshot_version{0};
  uint64_t initial_bundle_hash{0};
  uint64_t final_bundle_hash{0};
  uint64_t model_world_event_hash{0};
  uint32_t expansion_rounds{0};
  uint32_t frontier_progress_rounds{0};
  uint32_t unchanged_bundle_rounds{0};
  bool stopped_for_no_progress{false};
  bool opposition_research_enabled{false};
  bool graphene_executed{false};
  bool path_verifier_executed{false};
  bool fiber_bundle_built{false};
  bool fiber_bundle_authoritative{false};
  bool stability_critic_executed{false};
  bool epistemic_admissibility_executed{false};
  bool lyapunov_trajectory_executed{false};
  bool lyapunov_certificate_valid{false};
  bool lyapunov_goal_reached{false};
  bool semantic_verification_required{true};
  bool escape_considered{false};
  bool convergence_executed{false};
  bool opposition_executed{false};
  bool governed_projection_executed{false};
  bool no_silent_promotion{true};
};

struct HypoKoshRuntimeResult {
  FiberBundle initial_bundle;
  StabilityAssessment initial_stability;
  EpistemicAdmissibility initial_admissibility;
  EscapePlan initial_escape;
  ConvergedAnswer initial_convergence;
  OppositionReport initial_opposition;
  SelfHealingPlan initial_self_healing;

  FiberBundle final_bundle;
  StabilityAssessment final_stability;
  EpistemicAdmissibility final_admissibility;
  ConvergedAnswer final_convergence;
  OppositionReport final_opposition;
  SelfHealingPlan final_self_healing;
  LyapunovTrajectory lyapunov;

  GovernedEpistemicStatus status{GovernedEpistemicStatus::Abstain};
  uint32_t primary_node{0};
  double confidence{0.0};
  std::vector<uint32_t> evidence_edges;
  std::vector<std::string> residual_uncertainty;
  ReasoningReceipt receipt;
};

class CompleteHypoKoshRuntime {
 public:
  explicit CompleteHypoKoshRuntime(const GrapheneDB& db,
                                   ModelWorld* model_world = nullptr);

  HypoKoshRuntimeResult reason(const std::vector<float>& query,
                               uint64_t query_signature,
                               const RuntimeOptions& options = {},
                               uint64_t snapshot_version = kInfVersion) const;

 private:
  const GrapheneDB& db_;
  ModelWorld* model_world_{nullptr};
};

const char* governed_status_name(GovernedEpistemicStatus status);

}  // namespace graphene
