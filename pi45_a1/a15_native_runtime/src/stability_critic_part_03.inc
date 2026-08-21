        observation.regime = LyapunovRegime::Equilibrium;
      } else if (observation.delta < -targets.descent_epsilon) {
        observation.regime = LyapunovRegime::Descending;
        ++certificate.descending_transitions;
      } else if (observation.delta > targets.increase_epsilon) {
        observation.regime = LyapunovRegime::Diverging;
        ++certificate.diverging_transitions;
        certificate.maximum_energy_increase =
            std::max(certificate.maximum_energy_increase, observation.delta);
      } else {
        observation.regime = LyapunovRegime::Marginal;
        ++certificate.marginal_transitions;
      }

      if (observation.delta > targets.increase_epsilon) {
        certificate.monotonic_nonincreasing = false;
      }
      if (observation.energy > targets.equilibrium_energy &&
          !observation.sufficient_decrease) {
        certificate.strict_or_sufficient_decrease = false;
      }
    }

    if (observation.energy <= targets.equilibrium_energy) {
      ++equilibrium_dwell;
    } else {
      equilibrium_dwell = 0;
    }

    const auto existing = first_hash_iteration.find(observation.bundle_hash);
    if (observation.bundle_hash != 0 && existing != first_hash_iteration.end() &&
        observation.energy > targets.equilibrium_energy &&
        index > existing->second + 1) {
      certificate.limit_cycle_detected = true;
      observation.regime = LyapunovRegime::LimitCycle;
    } else if (observation.bundle_hash != 0) {
      first_hash_iteration.emplace(observation.bundle_hash, index);
    }

    output.observations.push_back(std::move(observation));
  }

  if (deltas.size() >= targets.oscillation_window - 1) {
    const size_t start = deltas.size() - (targets.oscillation_window - 1);
    size_t changes = 0;
    for (size_t index = start + 1; index < deltas.size(); ++index) {
      if (sign_change(deltas[index - 1], deltas[index],
                      targets.stagnation_epsilon)) {
        ++changes;
      }
    }
    if (changes >= 2) {
      certificate.oscillation_detected = true;
      if (!output.observations.empty() &&
          output.observations.back().regime != LyapunovRegime::LimitCycle) {
        output.observations.back().regime = LyapunovRegime::Oscillating;
      }
    }
  }

  if (!contraction_ratios.empty()) {
    certificate.mean_contraction_ratio =
        std::accumulate(contraction_ratios.begin(), contraction_ratios.end(),
                        0.0) /
        static_cast<double>(contraction_ratios.size());
    certificate.worst_contraction_ratio =
        *std::max_element(contraction_ratios.begin(),
                          contraction_ratios.end());
  }
  certificate.final_energy = output.observations.back().energy;
  certificate.goal_reached =
      certificate.final_energy <= targets.equilibrium_energy;
  certificate.equilibrium_dwell_satisfied =
      equilibrium_dwell >= targets.equilibrium_dwell_steps;
  certificate.practical_stability_observed =
      certificate.weights_positive && certificate.state_bounded &&
      certificate.energy_nonnegative && certificate.goal_reached &&
      !certificate.limit_cycle_detected;
  certificate.convergence_observed =
      certificate.practical_stability_observed &&
      certificate.equilibrium_dwell_satisfied &&
      certificate.monotonic_nonincreasing &&
      !certificate.oscillation_detected;

  if (!certificate.weights_positive)
    certificate.violations.push_back("Lyapunov weights must be finite and strictly positive");
  if (!certificate.quadratic_bounds_valid)
    certificate.violations.push_back("positive quadratic Lyapunov bounds could not be established");
  if (!certificate.state_bounded)
    certificate.violations.push_back("one or more Lyapunov error coordinates left the bounded domain");
  if (!certificate.energy_nonnegative)
    certificate.violations.push_back("Lyapunov energy was non-finite or negative");
  if (!certificate.monotonic_nonincreasing)
    certificate.violations.push_back("observed Lyapunov energy increased beyond tolerance");
  if (!certificate.strict_or_sufficient_decrease && !certificate.goal_reached)
    certificate.violations.push_back("energy did not show sufficient decrease outside the goal set");
  if (certificate.oscillation_detected)
    certificate.violations.push_back("alternating Lyapunov drift indicates an oscillatory reasoning trajectory");
  if (certificate.limit_cycle_detected)
    certificate.violations.push_back("a repeated non-equilibrium bundle hash indicates a finite-state limit cycle");
  if (!certificate.goal_reached)
    certificate.violations.push_back("final energy remains outside the practical equilibrium set");
  return output;
}

StabilityAssessment StabilityCriticV0::assess(
    const FiberBundle& bundle,
    QueryMode mode,
    const StabilityWeights& weights,
    const StabilityThresholds& thresholds) const {
  return LyapunovCritic().assess(bundle, mode, weights, thresholds, {}, {});
}

const char* lyapunov_regime_name(LyapunovRegime regime) {
  switch (regime) {
    case LyapunovRegime::InsufficientHistory: return "insufficient_history";
    case LyapunovRegime::Equilibrium: return "equilibrium";
    case LyapunovRegime::Descending: return "descending";
    case LyapunovRegime::Marginal: return "marginal";
    case LyapunovRegime::Diverging: return "diverging";
    case LyapunovRegime::Oscillating: return "oscillating";
    case LyapunovRegime::LimitCycle: return "limit_cycle";
  }
  return "insufficient_history";
}

}  // namespace graphene
