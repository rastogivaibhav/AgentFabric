    append_unique(&report.falsification_questions,
                  "What additional evidence would establish a stable causal root?");
    for (const auto& root : bundles.roots) append_unique(&report.reopen_nodes, root.root_node);
  }

  const RootBundle* primary = find_root(bundles, converged.primary_node);
  if (primary) {
    if (primary->contradiction_ratio > 0.0) {
      append_unique(&report.challenged_claims,
                    "the converged answer contains a contradiction path");
      append_unique(&report.falsification_questions,
                    "Which observation resolves the contradiction affecting the selected root?");
      append_unique(&report.reopen_nodes, primary->root_node);
    }
    if (primary->provenance_risk > 0.0 || primary->evidence_coverage < 1.0) {
      append_unique(&report.challenged_claims,
                    "the converged answer relies on incomplete or unsafe provenance");
      append_unique(&report.falsification_questions,
                    "Can each selected causal edge be tied to evidence or a derivation chain?");
      append_unique(&report.reopen_nodes, primary->root_node);
    }
    if (primary->degeneracy < 2.0) {
      append_unique(&report.challenged_claims,
                    "the selected root is supported by only one retrieved path");
      append_unique(&report.falsification_questions,
                    "Is there an independent path that supports or contradicts this root?");
      append_unique(&report.reopen_nodes, primary->root_node);
    }
  }

  if (bundles.roots.size() > 1) {
    append_unique(&report.challenged_claims,
                  "convergence selected one root while competing roots remain");
    append_unique(&report.falsification_questions,
                  "What evidence discriminates the selected root from its strongest alternative?");
    for (size_t index = 1; index < bundles.roots.size(); ++index) {
      append_unique(&report.reopen_nodes, bundles.roots[index].root_node);
    }
  }
  if (bundles.truncated) {
    append_unique(&report.challenged_claims,
                  "the expansion budget may have hidden additional paths");
    append_unique(&report.falsification_questions,
                  "Does a bounded re-expansion reveal a materially different explanation?");
    if (primary) append_unique(&report.reopen_nodes, primary->root_node);
  }

  report.opposition_score = clamp01(
      0.18 * static_cast<double>(report.challenged_claims.size()) +
      0.04 * static_cast<double>(report.reopen_nodes.size()));
  report.requests_reexpansion =
      report.opposition_score >= options.reexpansion_threshold &&
      !report.reopen_nodes.empty();
  return report;
}

DialecticResult DialecticEngine::reason(
    const std::vector<float>& query,
    uint64_t query_signature,
    const DialecticOptions& requested_options,
    uint64_t snapshot_version) const {
  const DialecticOptions options = bounded_options(requested_options);
  DialecticResult result;
  const uint64_t resolved_snapshot =
      snapshot_version == kInfVersion ? db_.snapshot() : snapshot_version;
  result.initial_bundle =
      expand(query, query_signature, options, resolved_snapshot);
  result.initial_convergence = converge(result.initial_bundle, options);
  result.initial_opposition =
      oppose(result.initial_bundle, result.initial_convergence, options);

  result.final_convergence = result.initial_convergence;
  result.final_opposition = result.initial_opposition;

  DialecticOptions expanded_options = options;
  for (uint32_t round = 0;
       round < options.max_opposition_rounds &&
       result.final_opposition.requests_reexpansion;
       ++round) {
    expanded_options.max_hops =
        std::min<uint32_t>(16, expanded_options.max_hops + 1);
    expanded_options.max_paths =
        std::min<size_t>(256, expanded_options.max_paths * 2);
    expanded_options.max_paths_per_root =
        std::min<size_t>(64, expanded_options.max_paths_per_root * 2);
    expanded_options.max_visited_states =
        std::min<size_t>(200000, expanded_options.max_visited_states * 2);
    expanded_options.semantic_candidates =
        std::min<size_t>(64, expanded_options.semantic_candidates + 4);
    expanded_options.reopen_nodes =
        result.final_opposition.reopen_nodes;

    result.reopened_bundle =
        expand(query, query_signature, expanded_options, resolved_snapshot);
    result.has_reopened_bundle = true;
    result.final_convergence =
        converge(result.reopened_bundle, expanded_options);
    result.final_opposition =
        oppose(result.reopened_bundle, result.final_convergence, expanded_options);
    ++result.rounds;
  }

  result.synthesis.has_answer = result.final_convergence.has_answer;
  result.synthesis.primary_node = result.final_convergence.primary_node;
  result.synthesis.confidence = result.final_convergence.confidence;
  result.synthesis.evidence_edges = result.final_convergence.evidence_edges;
  result.synthesis.residual_uncertainty =
      result.final_convergence.residual_uncertainty;

  if (!result.synthesis.has_answer) {
    result.synthesis.epistemic_status = "abstain";
  } else if (result.final_opposition.opposition_score >= 0.50) {
    result.synthesis.epistemic_status = "contested";
  } else if (!result.synthesis.residual_uncertainty.empty() ||
             result.final_opposition.opposition_score > 0.0) {
    result.synthesis.epistemic_status = "provisional";
  } else {
    result.synthesis.epistemic_status = "supported";
  }
  result.durable_writes = false;
  return result;
}

} // namespace graphene
