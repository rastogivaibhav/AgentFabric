  std::vector<double> confidences;
  for (const auto& path : primary.paths) {
    temporal += path.temporal_consistency;
    provenance += path.provenance_quality;
    contradictory += path.contains_contradiction ? 1.0 : 0.0;
    if (path.source_lineage.empty() && !path.edges.empty()) ++missing_source_paths;
    confidences.push_back(path.confidence);
  }
  const double count = static_cast<double>(primary.paths.size());
  output.temporal_consistency = clamp01(temporal / count);
  output.provenance_score = clamp01(provenance / count);
  output.contradiction_score = clamp01(contradictory / count);
  output.degeneracy_score = clamp01(primary.degeneracy);
  output.missing_evidence_penalty = clamp01(
      static_cast<double>(missing_source_paths) / count +
      (bundle.truncated ? 0.20 : 0.0));

  if (primary.paths.size() == 1) {
    output.path_diversity = 0.0;
  } else {
    double diversity_sum = 0.0;
    size_t pairs = 0;
    for (size_t i = 0; i < primary.paths.size(); ++i) {
      for (size_t j = i + 1; j < primary.paths.size(); ++j) {
        diversity_sum += jaccard_distance(primary.paths[i].edges,
                                          primary.paths[j].edges);
        ++pairs;
      }
    }
    output.path_diversity = pairs == 0
                                ? 0.0
                                : clamp01(diversity_sum /
                                          static_cast<double>(pairs));
  }

  const double confidence_sum =
      std::accumulate(confidences.begin(), confidences.end(), 0.0);
  const double top_share = confidence_sum <= 0.0
                               ? 1.0
                               : *std::max_element(confidences.begin(),
                                                   confidences.end()) /
                                     confidence_sum;
  const double normalised_entropy = entropy(confidences);
  const double source_diversity = clamp01(
      static_cast<double>(primary.independent_path_count) /
      static_cast<double>(std::max<size_t>(1, primary.paths.size())));
  output.pattern_lock_score = clamp01(
      0.45 * top_share + 0.35 * (1.0 - normalised_entropy) +
      0.20 * (1.0 - source_diversity));

  const StabilityWeights weights =
      adjusted_stability_weights(requested_weights, mode);
  const double positive_weight = std::max(
      kEpsilon, weights.temporal + weights.diversity + weights.degeneracy +
                    weights.provenance);
  const double negative_weight = std::max(
      kEpsilon, weights.contradiction + weights.pattern_lock +
                    weights.missing_evidence);
  const double positive =
      (weights.temporal * output.temporal_consistency +
       weights.diversity * output.path_diversity +
       weights.degeneracy * output.degeneracy_score +
       weights.provenance * output.provenance_score) /
      positive_weight;
  const double negative =
      (weights.contradiction * output.contradiction_score +
       weights.pattern_lock * output.pattern_lock_score +
       weights.missing_evidence * output.missing_evidence_penalty) /
      negative_weight;
  output.total_score = clamp01(0.75 * positive + 0.25 * (1.0 - negative));

  const LyapunovTargets effective_targets =
      adjusted_targets(requested_lyapunov_targets, mode);
  const LyapunovState error_state =
      state(output, mode, requested_lyapunov_targets);
  output.lyapunov_state_norm = std::sqrt(error_state.norm_squared);
  output.lyapunov_energy =
      energy(error_state, mode, requested_lyapunov_weights);
  output.lyapunov_goal_reached =
      output.lyapunov_energy <= effective_targets.equilibrium_energy;

  output.stable = output.total_score >= thresholds.stable_score &&
                  output.contradiction_score < 0.25 &&
                  output.missing_evidence_penalty < 0.50 &&
                  output.lyapunov_goal_reached;
  output.requires_escape =
      output.total_score < thresholds.escape_score ||
      output.lyapunov_energy > thresholds.lyapunov_escape_energy ||
      output.pattern_lock_score > 0.70 ||
      output.missing_evidence_penalty > 0.25;
  output.requires_opposition =
      output.contradiction_score >= thresholds.opposition_score ||
      output.pattern_lock_score > 0.55 || bundle.fibers.size() > 1;
  output.requires_abstention =
      output.total_score < thresholds.abstention_score ||
      output.lyapunov_energy > thresholds.lyapunov_abstention_energy ||
      output.provenance_score < 0.25 || output.temporal_consistency < 0.25;

  if (output.path_diversity < 0.25)
    output.reasons.push_back("path diversity is low");
  if (output.degeneracy_score < 0.50)
    output.reasons.push_back("independent support is weak");
  if (output.contradiction_score > 0.0)
    output.reasons.push_back("contradictory paths are present");
  if (output.pattern_lock_score > 0.55)
    output.reasons.push_back("the bundle shows premature pattern lock");
  if (output.missing_evidence_penalty > 0.0)
    output.reasons.push_back(
        "some paths lack source evidence or expansion was truncated");
  if (output.lyapunov_energy > thresholds.lyapunov_stable_energy)
    output.reasons.push_back("Lyapunov energy remains outside the practical stability set");
  if (!output.stable && output.reasons.empty())
    output.reasons.push_back("stability score is below threshold");
  return output;
}

LyapunovState LyapunovCritic::state(
    const StabilityAssessment& assessment,
    QueryMode mode,
    const LyapunovTargets& requested_targets) const {
  const LyapunovTargets targets = adjusted_targets(requested_targets, mode);
  LyapunovState output;
  output.temporal_deficit =
      lower_deficit(assessment.temporal_consistency, targets.temporal_min);
  output.diversity_deficit =
      lower_deficit(assessment.path_diversity, targets.diversity_min);
  output.degeneracy_deficit =
      lower_deficit(assessment.degeneracy_score, targets.degeneracy_min);
  output.provenance_deficit =
      lower_deficit(assessment.provenance_score, targets.provenance_min);
  output.contradiction_excess =
      upper_excess(assessment.contradiction_score, targets.contradiction_max);
  output.pattern_lock_excess =
