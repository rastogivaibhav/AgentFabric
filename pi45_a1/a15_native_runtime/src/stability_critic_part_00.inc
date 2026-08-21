#include "graphene/stability_critic.hpp"

#include <algorithm>
#include <cmath>
#include <limits>
#include <numeric>
#include <set>
#include <unordered_map>

namespace graphene {
namespace {

constexpr double kEpsilon = 1e-12;

double clamp01(double value) { return std::clamp(value, 0.0, 1.0); }

bool finite01(double value) {
  return std::isfinite(value) && value >= -kEpsilon && value <= 1.0 + kEpsilon;
}

double lower_deficit(double actual, double target) {
  if (!std::isfinite(actual) || !std::isfinite(target) || target <= kEpsilon) return 1.0;
  return clamp01((target - actual) / target);
}

double upper_excess(double actual, double maximum) {
  if (!std::isfinite(actual) || !std::isfinite(maximum)) return 1.0;
  if (actual <= maximum) return 0.0;
  const double span = std::max(kEpsilon, 1.0 - maximum);
  return clamp01((actual - maximum) / span);
}

double jaccard_distance(const std::vector<uint32_t>& left,
                        const std::vector<uint32_t>& right) {
  std::set<uint32_t> a(left.begin(), left.end());
  std::set<uint32_t> b(right.begin(), right.end());
  size_t intersection = 0;
  for (uint32_t value : a) {
    if (b.count(value)) ++intersection;
  }
  const size_t union_size = a.size() + b.size() - intersection;
  return union_size == 0 ? 0.0
                         : 1.0 - static_cast<double>(intersection) /
                                     static_cast<double>(union_size);
}

double entropy(const std::vector<double>& values) {
  double total = 0.0;
  for (double value : values) total += std::max(0.0, value);
  if (total <= 0.0 || values.size() <= 1) return 0.0;
  double h = 0.0;
  for (double value : values) {
    const double p = std::max(0.0, value) / total;
    if (p > 0.0) h -= p * std::log(p);
  }
  return clamp01(h / std::log(static_cast<double>(values.size())));
}

StabilityWeights adjusted_stability_weights(StabilityWeights weights,
                                             QueryMode mode) {
  if (mode == QueryMode::Empirical) {
    weights.provenance += 0.08;
    weights.temporal += 0.05;
    weights.contradiction += 0.05;
  } else if (mode == QueryMode::Theoretical) {
    weights.diversity += 0.08;
    weights.pattern_lock += 0.05;
    weights.missing_evidence = std::max(0.01, weights.missing_evidence - 0.02);
  }
  return weights;
}

LyapunovWeights adjusted_lyapunov_weights(LyapunovWeights weights,
                                          QueryMode mode) {
  if (mode == QueryMode::Empirical) {
    weights.temporal += 0.05;
    weights.provenance += 0.08;
    weights.contradiction += 0.05;
  } else if (mode == QueryMode::Theoretical) {
    weights.diversity += 0.08;
    weights.pattern_lock += 0.05;
    weights.missing_evidence = std::max(0.02, weights.missing_evidence - 0.03);
  }
  return weights;
}

LyapunovTargets adjusted_targets(LyapunovTargets targets, QueryMode mode) {
  if (mode == QueryMode::Empirical) {
    targets.temporal_min = std::max(targets.temporal_min, 0.95);
    targets.provenance_min = std::max(targets.provenance_min, 0.85);
    targets.contradiction_max = std::min(targets.contradiction_max, 0.15);
    targets.missing_evidence_max = std::min(targets.missing_evidence_max, 0.05);
  } else if (mode == QueryMode::Theoretical) {
    targets.diversity_min = std::max(targets.diversity_min, 0.40);
    targets.degeneracy_min = std::max(targets.degeneracy_min, 0.55);
    targets.provenance_min = std::min(targets.provenance_min, 0.60);
    targets.missing_evidence_max = std::max(targets.missing_evidence_max, 0.20);
  }
  targets.equilibrium_dwell_steps = std::max<size_t>(1, targets.equilibrium_dwell_steps);
  targets.oscillation_window = std::max<size_t>(3, targets.oscillation_window);
  return targets;
}

bool all_weights_positive(const LyapunovWeights& weights) {
  return std::isfinite(weights.temporal) && weights.temporal > 0.0 &&
         std::isfinite(weights.diversity) && weights.diversity > 0.0 &&
         std::isfinite(weights.degeneracy) && weights.degeneracy > 0.0 &&
         std::isfinite(weights.provenance) && weights.provenance > 0.0 &&
         std::isfinite(weights.contradiction) && weights.contradiction > 0.0 &&
         std::isfinite(weights.pattern_lock) && weights.pattern_lock > 0.0 &&
         std::isfinite(weights.missing_evidence) && weights.missing_evidence > 0.0;
}

bool state_bounded(const LyapunovState& state) {
  return finite01(state.temporal_deficit) && finite01(state.diversity_deficit) &&
         finite01(state.degeneracy_deficit) && finite01(state.provenance_deficit) &&
         finite01(state.contradiction_excess) && finite01(state.pattern_lock_excess) &&
         finite01(state.missing_evidence_excess) &&
         std::isfinite(state.norm_squared) && state.norm_squared >= -kEpsilon &&
         state.norm_squared <= 7.0 + kEpsilon;
}

bool sign_change(double left, double right, double epsilon) {
  if (std::abs(left) <= epsilon || std::abs(right) <= epsilon) return false;
  return (left < 0.0 && right > 0.0) || (left > 0.0 && right < 0.0);
}

}  // namespace

StabilityAssessment LyapunovCritic::assess(
    const FiberBundle& bundle,
    QueryMode mode,
    const StabilityWeights& requested_weights,
    const StabilityThresholds& thresholds,
    const LyapunovWeights& requested_lyapunov_weights,
    const LyapunovTargets& requested_lyapunov_targets) const {
  StabilityAssessment output;
  if (bundle.fibers.empty()) {
    output.missing_evidence_penalty = 1.0;
    output.lyapunov_energy = 1.0;
    output.lyapunov_state_norm = 1.0;
    output.requires_escape = true;
    output.requires_abstention = true;
    output.reasons.push_back("no causal fibers were produced");
    return output;
  }

  const TargetFiber& primary = bundle.fibers.front();
  if (primary.paths.empty()) {
    output.missing_evidence_penalty = 1.0;
    output.lyapunov_energy = 1.0;
    output.lyapunov_state_norm = 1.0;
    output.requires_escape = true;
    output.requires_abstention = true;
    output.reasons.push_back("the primary fiber has no reasoning paths");
    return output;
  }

  double temporal = 0.0;
  double provenance = 0.0;
  double contradictory = 0.0;
  size_t missing_source_paths = 0;
