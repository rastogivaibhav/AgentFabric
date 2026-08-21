
GovernedEpistemicStatus project_status(
    const ConvergedAnswer& convergence,
    const OppositionReport& opposition,
    const StabilityAssessment& stability,
    const EpistemicAdmissibility& admissibility,
    QueryMode mode) {
  if (!convergence.has_answer) {
    return stability.missing_evidence_penalty > 0.0
               ? GovernedEpistemicStatus::EvidenceRequired
               : GovernedEpistemicStatus::Abstain;
  }
  if (admissibility.semantic_verification ==
      SemanticVerificationStatus::Contradicted) {
    return GovernedEpistemicStatus::Contested;
  }
  if (admissibility.contradiction_blocks_resolution ||
      opposition.opposition_score >= 0.50) {
    return GovernedEpistemicStatus::Contested;
  }
  if (!admissibility.evidence_admissible) {
    return stability.completeness_score < 0.50 ||
                   stability.provenance_score < 0.25
               ? GovernedEpistemicStatus::EvidenceRequired
               : GovernedEpistemicStatus::Abstain;
  }
  if (mode == QueryMode::Theoretical &&
      admissibility.semantic_verification ==
          SemanticVerificationStatus::Unverified) {
    return GovernedEpistemicStatus::Speculative;
  }
  if (stability.stable &&
      admissibility.sufficient_independent_support &&
      admissibility.semantic_verification ==
          SemanticVerificationStatus::Verified &&
      opposition.opposition_score == 0.0) {
    return GovernedEpistemicStatus::Resolved;
  }
  return GovernedEpistemicStatus::ProvisionallyResolved;
}

}  // namespace

CompleteHypoKoshRuntime::CompleteHypoKoshRuntime(
    const GrapheneDB& db,
    ModelWorld* model_world)
    : db_(db), model_world_(model_world) {}

HypoKoshRuntimeResult CompleteHypoKoshRuntime::reason(
    const std::vector<float>& query,
    uint64_t query_signature,
    const RuntimeOptions& requested_options,
    uint64_t snapshot_version) const {
  RuntimeOptions options = requested_options;
  options.max_recursive_cycles =
      std::min<uint32_t>(options.max_recursive_cycles, 3);
  options.unchanged_recovery_patience =
      std::min<uint32_t>(options.unchanged_recovery_patience, 3);
  const uint64_t resolved_snapshot =
      snapshot_version == kInfVersion ? db_.snapshot() : snapshot_version;

  HypoKoshRuntimeResult result;
  result.receipt.opposition_research_enabled =
      options.enable_opposition_research;
  DialecticEngine dialectic(db_);
  FiberBundleBuilder bundle_builder;
  LyapunovCritic critic;
  EpistemicController controller;
  CorrectiveEscape escape;
  RecursiveSelfHealingController self_healing;

  BundleSet expansion = dialectic.expand(
      query, query_signature, options.dialectic, resolved_snapshot);
  if (options.path_verifier) {
    apply_path_verifier(&expansion, *options.path_verifier, query,
                        query_signature, options.dialectic.mode);
  }
  result.initial_bundle = bundle_builder.build(expansion);
  result.initial_stability = critic.assess(
      result.initial_bundle, options.dialectic.mode,
      options.stability_weights, options.stability_thresholds,
      options.lyapunov_weights, options.lyapunov_targets);
  result.initial_admissibility = controller.assess(
      result.initial_bundle, result.initial_stability,
      options.dialectic.mode);
  result.initial_escape = escape.plan(
      result.initial_bundle, result.initial_stability,
      options.dialectic.mode);
  result.initial_convergence = controller.converge(
      result.initial_bundle, result.initial_admissibility,
      result.initial_stability, options.dialectic);
  result.initial_opposition = controller.oppose(
      result.initial_bundle, result.initial_convergence,
      result.initial_admissibility, result.initial_stability,
      options.dialectic);
  result.initial_self_healing = self_healing.plan(
      result.initial_stability, result.initial_opposition,
      result.initial_escape);

  result.final_bundle = result.initial_bundle;
  result.final_stability = result.initial_stability;
  result.final_admissibility = result.initial_admissibility;
  result.final_convergence = result.initial_convergence;
  result.final_opposition = result.initial_opposition;
  result.final_self_healing = result.initial_self_healing;

  std::vector<LyapunovSample> samples;
  samples.push_back(
      {reasoning_state_hash(result.initial_bundle, options.dialectic),
       result.initial_stability});

  DialecticOptions current_options = options.dialectic;
  EscapePlan current_escape = result.initial_escape;
  uint32_t consecutive_unchanged_bundles = 0;
  for (uint32_t round = 0; round < options.max_recursive_cycles; ++round) {
    const bool recovery_search_requested =
        result.final_stability.requires_escape &&
        has_searchable_action(current_escape);
    const bool opposition_search_requested =
        options.enable_opposition_research &&
        result.final_opposition.requests_reexpansion;
    if (!recovery_search_requested && !opposition_search_requested) {
      break;
    }

