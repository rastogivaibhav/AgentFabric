    const DialecticOptions previous_options = current_options;
    const EscapePlan active_escape = current_escape;
    DialecticOptions next_options =
        targeted_options(current_options, current_escape);
    if (opposition_search_requested) {
      next_options =
          opposition_options(std::move(next_options), result.final_opposition);
    }
    if (!current_escape.generic_expansion_allowed &&
        !opposition_search_requested &&
        !options_changed(current_options, next_options)) {
      break;
    }
    current_options = std::move(next_options);

    const uint64_t previous_bundle_hash =
        result.final_bundle.immutable_hash;
    const size_t previous_visited_states =
        result.final_bundle.visited_states;
    const bool previous_truncated = result.final_bundle.truncated;

    BundleSet reopened = dialectic.expand(
        query, query_signature, current_options, resolved_snapshot);
    if (options.path_verifier) {
      apply_path_verifier(&reopened, *options.path_verifier, query,
                          query_signature, current_options.mode);
    }
    FiberBundle reopened_bundle = bundle_builder.build(reopened);
    StabilityAssessment reopened_stability = critic.assess(
        reopened_bundle, current_options.mode,
        options.stability_weights, options.stability_thresholds,
        options.lyapunov_weights, options.lyapunov_targets);
    EpistemicAdmissibility reopened_admissibility = controller.assess(
        reopened_bundle, reopened_stability, current_options.mode);
    ConvergedAnswer reopened_convergence = controller.converge(
        reopened_bundle, reopened_admissibility,
        reopened_stability, current_options);
    OppositionReport reopened_opposition = controller.oppose(
        reopened_bundle, reopened_convergence,
        reopened_admissibility, reopened_stability,
        current_options);
    EscapePlan reopened_escape = escape.plan(
        reopened_bundle, reopened_stability, current_options.mode);

    const bool bundle_changed =
        reopened_bundle.immutable_hash != previous_bundle_hash;
    const bool frontier_progress =
        reopened_bundle.visited_states > previous_visited_states ||
        (reopened_bundle.truncated && !previous_truncated) ||
        (depth_repair_active(active_escape) &&
         current_options.max_hops > previous_options.max_hops);
    consecutive_unchanged_bundles =
        bundle_changed ? 0 : consecutive_unchanged_bundles + 1;
    if (frontier_progress) ++result.receipt.frontier_progress_rounds;
    result.receipt.unchanged_bundle_rounds =
        consecutive_unchanged_bundles;

    result.final_bundle = std::move(reopened_bundle);
    result.final_stability = std::move(reopened_stability);
    result.final_admissibility = std::move(reopened_admissibility);
    result.final_convergence = std::move(reopened_convergence);
    result.final_opposition = std::move(reopened_opposition);
    result.final_self_healing = self_healing.plan(
        result.final_stability, result.final_opposition,
        reopened_escape);
    current_escape = std::move(reopened_escape);
    ++result.receipt.expansion_rounds;
    samples.push_back(
        {reasoning_state_hash(result.final_bundle, current_options),
         result.final_stability});

    const LyapunovTrajectory partial = critic.analyse(
        samples, current_options.mode,
        options.lyapunov_weights, options.lyapunov_targets);
    if (partial.certificate.limit_cycle_detected ||
        partial.certificate.oscillation_detected) {
      break;
    }

    const bool bounded_depth_patience =
        depth_repair_active(active_escape) &&
        consecutive_unchanged_bundles <=
            options.unchanged_recovery_patience;
    if (!bundle_changed && !frontier_progress &&
        !bounded_depth_patience) {
      result.receipt.stopped_for_no_progress = true;
      break;
    }
  }

  result.lyapunov = critic.analyse(
      samples, options.dialectic.mode,
      options.lyapunov_weights, options.lyapunov_targets);
  result.status = project_status(
      result.final_convergence, result.final_opposition,
      result.final_stability, result.final_admissibility,
      options.dialectic.mode);
  if (result.lyapunov.certificate.limit_cycle_detected ||
      result.lyapunov.certificate.oscillation_detected) {
    result.status = result.final_convergence.has_answer
                        ? GovernedEpistemicStatus::Contested
                        : GovernedEpistemicStatus::Abstain;
  } else if (result.status == GovernedEpistemicStatus::Resolved &&
             !result.lyapunov.certificate.practical_stability_observed) {
    result.status = GovernedEpistemicStatus::ProvisionallyResolved;
  }

  result.primary_node = result.final_convergence.primary_node;
  result.confidence = result.final_convergence.confidence;
  result.evidence_edges =
      all_evidence_edges(result.final_convergence);
  result.residual_uncertainty =
      result.final_convergence.residual_uncertainty;
  result.residual_uncertainty.insert(
      result.residual_uncertainty.end(),
      result.final_stability.reasons.begin(),
      result.final_stability.reasons.end());
  result.residual_uncertainty.insert(
      result.residual_uncertainty.end(),
      result.final_admissibility.reasons.begin(),
      result.final_admissibility.reasons.end());
  result.residual_uncertainty.insert(
      result.residual_uncertainty.end(),
      result.lyapunov.certificate.violations.begin(),
      result.lyapunov.certificate.violations.end());
