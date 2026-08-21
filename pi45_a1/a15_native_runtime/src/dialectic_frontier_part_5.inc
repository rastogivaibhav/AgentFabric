              ? path.edges.size() - missing_evidence_edges.size()
              : 0;
    }
    root.degeneracy = static_cast<double>(root.paths.size());
    root.diversity =
        root.paths.empty()
            ? 0.0
            : static_cast<double>(unique_edges.size()) /
                  static_cast<double>(root.paths.size());
    root.contradiction_ratio =
        root.paths.empty()
            ? 0.0
            : static_cast<double>(contradictory_paths) /
                  static_cast<double>(root.paths.size());
    root.evidence_coverage =
        edge_instances == 0
            ? 1.0
            : static_cast<double>(evidence_covered_edges) /
                  static_cast<double>(edge_instances);
    root.provenance_risk =
        edge_instances == 0
            ? 0.0
            : clamp01(static_cast<double>(finding_count) /
                      static_cast<double>(edge_instances));
    const double average_score =
        root.paths.empty() ? 0.0 : score_sum / static_cast<double>(root.paths.size());
    root.confidence = clamp01(
        0.60 * average_score +
        0.15 * std::min(1.0, root.degeneracy / 3.0) +
        0.10 * std::min(1.0, root.diversity / 3.0) +
        0.10 * root.evidence_coverage -
        0.20 * root.contradiction_ratio -
        0.15 * root.provenance_risk);
    output.roots.push_back(std::move(root));
  }

  std::sort(output.roots.begin(), output.roots.end(),
            [](const RootBundle& a, const RootBundle& b) {
              if (std::abs(a.confidence - b.confidence) > 1e-12) {
                return a.confidence > b.confidence;
              }
              return a.root_node < b.root_node;
            });
  if (output.roots.empty()) output.warnings.push_back("NO_CAUSAL_ROOT");
  if (total_paths >= options.max_paths) {
    output.truncated = true;
    output.warnings.push_back("PATH_BUDGET_EXHAUSTED");
  }
  return output;
}

ConvergedAnswer DialecticEngine::converge(
    const BundleSet& bundles,
    const DialecticOptions& requested_options) const {
  const DialecticOptions options = bounded_options(requested_options);
  ConvergedAnswer answer;
  if (bundles.roots.empty()) {
    answer.residual_uncertainty.push_back("no causal root reached");
    return answer;
  }

  const RootBundle& primary = bundles.roots.front();
  answer.primary_node = primary.root_node;
  answer.confidence = primary.confidence;
  answer.false_promotion_risk = primary.provenance_risk;

  std::vector<size_t> path_order(primary.paths.size());
  for (size_t index = 0; index < path_order.size(); ++index) path_order[index] = index;
  std::sort(path_order.begin(), path_order.end(), [&](size_t left, size_t right) {
    if (std::abs(primary.paths[left].score - primary.paths[right].score) > 1e-12) {
      return primary.paths[left].score > primary.paths[right].score;
    }
    return primary.paths[left].edges < primary.paths[right].edges;
  });

  std::set<uint32_t> evidence_edges;
  for (size_t rank = 0; rank < path_order.size(); ++rank) {
    const size_t path_index = path_order[rank];
    if (rank < options.max_selected_paths) {
      answer.selected_paths.push_back(
          {primary.root_node, path_index, "selected by bounded convergence"});
      evidence_edges.insert(primary.paths[path_index].edges.begin(),
                            primary.paths[path_index].edges.end());
    } else {
      answer.discarded_paths.push_back(
          {primary.root_node, path_index, "lower-ranked path retained in immutable bundle"});
    }
  }
  for (size_t root_index = 1; root_index < bundles.roots.size(); ++root_index) {
    const auto& root = bundles.roots[root_index];
    for (size_t path_index = 0; path_index < root.paths.size(); ++path_index) {
      answer.discarded_paths.push_back(
          {root.root_node, path_index, "competing causal root retained for opposition"});
    }
  }
  answer.evidence_edges.assign(evidence_edges.begin(), evidence_edges.end());

  if (primary.contradiction_ratio > 0.0) {
    answer.residual_uncertainty.push_back("selected root contains contradictory paths");
  }
  if (primary.provenance_risk > 0.0) {
    answer.residual_uncertainty.push_back("selected root contains provenance risks");
  }
  if (bundles.roots.size() > 1) {
    answer.residual_uncertainty.push_back("competing causal roots remain");
  }
  if (bundles.truncated) {
    answer.residual_uncertainty.push_back("expansion stopped at a configured budget");
  }
  answer.has_answer = answer.confidence >= options.minimum_confidence;
  if (!answer.has_answer) {
    answer.residual_uncertainty.push_back("confidence is below the convergence threshold");
  }
  return answer;
}

OppositionReport DialecticEngine::oppose(
    const BundleSet& bundles,
    const ConvergedAnswer& converged,
    const DialecticOptions& requested_options) const {
  const DialecticOptions options = bounded_options(requested_options);
  OppositionReport report;

  if (!converged.has_answer) {
    append_unique(&report.challenged_claims,
                  "no answer satisfies the convergence threshold");
