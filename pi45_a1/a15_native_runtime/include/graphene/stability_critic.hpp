#pragma once

#include "graphene/fiber_bundle.hpp"

#include <cstddef>
#include <cstdint>
#include <string>
#include <vector>

namespace graphene {

struct StabilityWeights {
  double temporal{0.14};
  double diversity{0.08};
  double degeneracy{0.10};
  double provenance{0.14};
  double contradiction{0.16};
  double pattern_lock{0.08};
  double missing_evidence{0.08};
  double relevance{0.08};
  double target_consistency{0.06};
  double completeness{0.08};
  double retrieval_noise{0.08};
};

struct StabilityThresholds {
  double stable_score{0.62};
  double escape_score{0.48};
  double opposition_score{0.15};
  double abstention_score{0.25};
  double lyapunov_stable_energy{0.08};
  double lyapunov_escape_energy{0.25};
  double lyapunov_abstention_energy{0.70};
  double minimum_relevance{0.50};
  double minimum_target_consistency{0.50};
  double minimum_completeness{0.50};
  double material_contradiction{0.15};
  double maximum_noise{0.40};
};

struct StabilityAssessment {
  double temporal_consistency{0.0};
  double path_diversity{0.0};
  double degeneracy_score{0.0};
  double provenance_score{0.0};
  double contradiction_score{0.0};
  double pattern_lock_score{0.0};
  double missing_evidence_penalty{0.0};
  double relevance_score{1.0};
  double target_consistency_score{1.0};
  double completeness_score{1.0};
  double independent_support_score{1.0};
  double retrieval_noise_penalty{0.0};
  double material_contradiction{0.0};
  double total_score{0.0};
  double lyapunov_energy{1.0};
  double lyapunov_state_norm{1.0};
  bool lyapunov_goal_reached{false};
  bool evidence_admissible{false};
  bool contradiction_blocks_resolution{false};
  bool requires_external_verification{true};
  bool stable{false};
  bool requires_escape{false};
  bool requires_opposition{false};
  bool requires_abstention{false};
  std::vector<std::string> reasons;
};

struct LyapunovState {
  double temporal_deficit{0.0};
  double diversity_deficit{0.0};
  double degeneracy_deficit{0.0};
  double provenance_deficit{0.0};
  double contradiction_excess{0.0};
  double pattern_lock_excess{0.0};
  double missing_evidence_excess{0.0};
  double relevance_deficit{0.0};
  double target_consistency_deficit{0.0};
  double completeness_deficit{0.0};
  double retrieval_noise_excess{0.0};
  double norm_squared{0.0};
};

struct LyapunovWeights {
  double temporal{0.12};
  double diversity{0.06};
  double degeneracy{0.08};
  double provenance{0.12};
  double contradiction{0.16};
  double pattern_lock{0.08};
  double missing_evidence{0.08};
  double relevance{0.08};
  double target_consistency{0.06};
  double completeness{0.08};
  double retrieval_noise{0.08};
};

struct LyapunovTargets {
  double temporal_min{0.90};
  double diversity_min{0.25};
  double degeneracy_min{0.50};
  double provenance_min{0.75};
  double contradiction_max{0.15};
  double pattern_lock_max{0.55};
  double missing_evidence_max{0.10};
  double relevance_min{0.60};
  double target_consistency_min{0.60};
  double completeness_min{0.75};
  double retrieval_noise_max{0.10};
  double equilibrium_energy{0.08};
  double descent_epsilon{0.005};
  double increase_epsilon{0.005};
  double stagnation_epsilon{0.002};
  double sufficient_decrease_rate{0.02};
  size_t equilibrium_dwell_steps{2};
  size_t oscillation_window{4};
};

enum class LyapunovRegime : uint8_t {
  InsufficientHistory,
  Equilibrium,
  Descending,
  Marginal,
  Diverging,
  Oscillating,
  LimitCycle
};

struct LyapunovObservation {
  size_t iteration{0};
  uint64_t bundle_hash{0};
  LyapunovState state;
  double energy{0.0};
  double delta{0.0};
  double relative_delta{0.0};
  bool sufficient_decrease{false};
  LyapunovRegime regime{LyapunovRegime::InsufficientHistory};
};

struct LyapunovCertificate {
  bool weights_positive{false};
  bool state_bounded{false};
  bool energy_nonnegative{false};
  bool quadratic_bounds_valid{false};
  bool monotonic_nonincreasing{false};
  bool strict_or_sufficient_decrease{false};
  bool goal_reached{false};
  bool equilibrium_dwell_satisfied{false};
  bool practical_stability_observed{false};
  bool convergence_observed{false};
  bool oscillation_detected{false};
  bool limit_cycle_detected{false};
  double initial_energy{0.0};
  double final_energy{0.0};
  double lower_quadratic_coefficient{0.0};
  double upper_quadratic_coefficient{0.0};
  double mean_contraction_ratio{1.0};
  double worst_contraction_ratio{1.0};
  double maximum_energy_increase{0.0};
  size_t descending_transitions{0};
  size_t marginal_transitions{0};
  size_t diverging_transitions{0};
  std::vector<std::string> violations;
};

struct LyapunovTrajectory {
  std::vector<LyapunovObservation> observations;
  LyapunovCertificate certificate;
};

struct LyapunovSample {
  uint64_t bundle_hash{0};
  StabilityAssessment assessment;
};

class LyapunovCritic {
 public:
  StabilityAssessment assess(const FiberBundle& bundle,
                             QueryMode mode = QueryMode::Balanced,
                             const StabilityWeights& weights = {},
                             const StabilityThresholds& thresholds = {},
                             const LyapunovWeights& lyapunov_weights = {},
                             const LyapunovTargets& lyapunov_targets = {}) const;

  LyapunovState state(const StabilityAssessment& assessment,
                      QueryMode mode = QueryMode::Balanced,
                      const LyapunovTargets& targets = {}) const;

  double energy(const LyapunovState& state,
                QueryMode mode = QueryMode::Balanced,
                const LyapunovWeights& weights = {}) const;

  LyapunovTrajectory analyse(const std::vector<LyapunovSample>& samples,
                             QueryMode mode = QueryMode::Balanced,
                             const LyapunovWeights& weights = {},
                             const LyapunovTargets& targets = {}) const;
};

class StabilityCriticV0 {
 public:
  StabilityAssessment assess(const FiberBundle& bundle,
                             QueryMode mode = QueryMode::Balanced,
                             const StabilityWeights& weights = {},
                             const StabilityThresholds& thresholds = {}) const;
};

const char* lyapunov_regime_name(LyapunovRegime regime);

}  // namespace graphene
