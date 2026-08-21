      upper_excess(assessment.pattern_lock_score, targets.pattern_lock_max);
  output.missing_evidence_excess = upper_excess(
      assessment.missing_evidence_penalty, targets.missing_evidence_max);
  output.norm_squared =
      output.temporal_deficit * output.temporal_deficit +
      output.diversity_deficit * output.diversity_deficit +
      output.degeneracy_deficit * output.degeneracy_deficit +
      output.provenance_deficit * output.provenance_deficit +
      output.contradiction_excess * output.contradiction_excess +
      output.pattern_lock_excess * output.pattern_lock_excess +
      output.missing_evidence_excess * output.missing_evidence_excess;
  return output;
}

double LyapunovCritic::energy(const LyapunovState& state_value,
                              QueryMode mode,
                              const LyapunovWeights& requested_weights) const {
  const LyapunovWeights weights =
      adjusted_lyapunov_weights(requested_weights, mode);
  if (!all_weights_positive(weights) || !state_bounded(state_value)) {
    return std::numeric_limits<double>::infinity();
  }
  const double total_weight =
      weights.temporal + weights.diversity + weights.degeneracy +
      weights.provenance + weights.contradiction + weights.pattern_lock +
      weights.missing_evidence;
  const double weighted =
      weights.temporal * state_value.temporal_deficit *
          state_value.temporal_deficit +
      weights.diversity * state_value.diversity_deficit *
          state_value.diversity_deficit +
      weights.degeneracy * state_value.degeneracy_deficit *
          state_value.degeneracy_deficit +
      weights.provenance * state_value.provenance_deficit *
          state_value.provenance_deficit +
      weights.contradiction * state_value.contradiction_excess *
          state_value.contradiction_excess +
      weights.pattern_lock * state_value.pattern_lock_excess *
          state_value.pattern_lock_excess +
      weights.missing_evidence * state_value.missing_evidence_excess *
          state_value.missing_evidence_excess;
  return clamp01(weighted / std::max(kEpsilon, total_weight));
}

LyapunovTrajectory LyapunovCritic::analyse(
    const std::vector<LyapunovSample>& samples,
    QueryMode mode,
    const LyapunovWeights& requested_weights,
    const LyapunovTargets& requested_targets) const {
  LyapunovTrajectory output;
  LyapunovCertificate& certificate = output.certificate;
  const LyapunovWeights weights =
      adjusted_lyapunov_weights(requested_weights, mode);
  const LyapunovTargets targets = adjusted_targets(requested_targets, mode);

  certificate.weights_positive = all_weights_positive(weights);
  const double weight_sum = weights.temporal + weights.diversity +
                            weights.degeneracy + weights.provenance +
                            weights.contradiction + weights.pattern_lock +
                            weights.missing_evidence;
  const double minimum_weight = std::min(
      {weights.temporal, weights.diversity, weights.degeneracy,
       weights.provenance, weights.contradiction, weights.pattern_lock,
       weights.missing_evidence});
  const double maximum_weight = std::max(
      {weights.temporal, weights.diversity, weights.degeneracy,
       weights.provenance, weights.contradiction, weights.pattern_lock,
       weights.missing_evidence});
  if (certificate.weights_positive && std::isfinite(weight_sum) &&
      weight_sum > kEpsilon) {
    certificate.lower_quadratic_coefficient = minimum_weight / weight_sum;
    certificate.upper_quadratic_coefficient = maximum_weight / weight_sum;
    certificate.quadratic_bounds_valid =
        certificate.lower_quadratic_coefficient > 0.0 &&
        certificate.upper_quadratic_coefficient >=
            certificate.lower_quadratic_coefficient;
  }
  certificate.state_bounded = true;
  certificate.energy_nonnegative = true;
  certificate.monotonic_nonincreasing = true;
  certificate.strict_or_sufficient_decrease = true;

  if (samples.empty()) {
    certificate.state_bounded = false;
    certificate.energy_nonnegative = false;
    certificate.monotonic_nonincreasing = false;
    certificate.strict_or_sufficient_decrease = false;
    certificate.violations.push_back("no Lyapunov samples were provided");
    return output;
  }

  std::unordered_map<uint64_t, size_t> first_hash_iteration;
  std::vector<double> deltas;
  std::vector<double> contraction_ratios;
  deltas.reserve(samples.size() > 0 ? samples.size() - 1 : 0);
  contraction_ratios.reserve(samples.size() > 0 ? samples.size() - 1 : 0);
  size_t equilibrium_dwell = 0;

  for (size_t index = 0; index < samples.size(); ++index) {
    LyapunovObservation observation;
    observation.iteration = index;
    observation.bundle_hash = samples[index].bundle_hash;
    observation.state =
        state(samples[index].assessment, mode, requested_targets);
    observation.energy =
        energy(observation.state, mode, requested_weights);
    certificate.state_bounded =
        certificate.state_bounded && state_bounded(observation.state);
    certificate.energy_nonnegative =
        certificate.energy_nonnegative && std::isfinite(observation.energy) &&
        observation.energy >= -kEpsilon;

    if (index == 0) {
      observation.regime = observation.energy <= targets.equilibrium_energy
                               ? LyapunovRegime::Equilibrium
                               : LyapunovRegime::InsufficientHistory;
      certificate.initial_energy = observation.energy;
    } else {
      const double previous = output.observations.back().energy;
      observation.delta = observation.energy - previous;
      observation.relative_delta =
          observation.delta / std::max(kEpsilon, previous);
      observation.sufficient_decrease =
          observation.delta <=
          -targets.sufficient_decrease_rate *
              output.observations.back().state.norm_squared;
      deltas.push_back(observation.delta);
      if (previous > kEpsilon && previous > targets.equilibrium_energy) {
        contraction_ratios.push_back(observation.energy / previous);
      }

      if (observation.energy <= targets.equilibrium_energy) {
