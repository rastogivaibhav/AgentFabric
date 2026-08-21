  options.max_selected_paths = std::clamp<size_t>(options.max_selected_paths, 1, 16);
  options.max_opposition_rounds = std::min<uint32_t>(options.max_opposition_rounds, 2);
  options.minimum_confidence = clamp01(options.minimum_confidence);
  options.reexpansion_threshold = clamp01(options.reexpansion_threshold);
  std::sort(options.reopen_nodes.begin(), options.reopen_nodes.end());
  options.reopen_nodes.erase(
      std::unique(options.reopen_nodes.begin(), options.reopen_nodes.end()),
      options.reopen_nodes.end());
  if (options.reopen_nodes.size() > 16) options.reopen_nodes.resize(16);
  return options;
}

} // namespace

DialecticEngine::DialecticEngine(const GrapheneDB& db) : db_(db) {}

BundleSet DialecticEngine::expand(const std::vector<float>& query,
                                  uint64_t query_signature,
                                  const DialecticOptions& requested_options,
                                  uint64_t snapshot_version) const {
  const DialecticOptions options = bounded_options(requested_options);
  BundleSet output;
  output.snapshot_version =
      snapshot_version == kInfVersion ? db_.snapshot() : snapshot_version;

  std::optional<Rfc3339Instant> as_of;
  if (!options.as_of.empty()) {
    Rfc3339Instant parsed_as_of;
    const Status status = parse_rfc3339(options.as_of, &parsed_as_of);
    if (!status) {
      output.warnings.push_back("INVALID_AS_OF: " + status.message);
      return output;
    }
    as_of = parsed_as_of;
  }
  auto temporal_allowed =
      [&](const std::map<std::string, std::string>& metadata,
          const std::string& object) {
        TemporalValidity validity;
        const Status status = parse_temporal_validity(metadata, &validity);
        if (!status) {
          const std::string warning =
              "INVALID_TEMPORAL_METADATA: " + object + ": " + status.message;
          if (std::find(output.warnings.begin(), output.warnings.end(), warning) ==
              output.warnings.end()) {
            output.warnings.push_back(warning);
          }
          return false;
        }
        return !as_of || valid_at(validity, *as_of);
      };

  const size_t search_count = std::min<size_t>(64, options.semantic_candidates * 4);
  const auto raw_candidates =
      db_.vector_search(query, search_count, output.snapshot_version);
  if (raw_candidates.empty() && options.reopen_nodes.empty()) {
    output.warnings.push_back("NO_SEMANTIC_CANDIDATES");
    return output;
  }

  std::map<uint32_t, double> raw_scores;
  for (const auto& candidate : raw_candidates) {
    raw_scores[candidate.node_id] = candidate.score;
  }

  std::vector<SearchResult> candidates;
  std::unordered_set<uint32_t> selected_ids;
  auto append_candidate = [&](uint32_t node_id, double fallback_score) {
    if (candidates.size() >= options.semantic_candidates ||
        selected_ids.count(node_id) != 0) {
      return;
    }
    auto node = db_.get_node(node_id, output.snapshot_version);
    if (!node ||
        !temporal_allowed(node->metadata,
                          "node " + std::to_string(node_id)) ||
        !signature_matches(node->signature, query_signature)) {
      return;
    }
    const auto raw = raw_scores.find(node_id);
    candidates.push_back(
        {node_id, raw == raw_scores.end() ? fallback_score : raw->second});
    selected_ids.insert(node_id);
  };

  for (uint32_t reopen_node : options.reopen_nodes) {
    auto node = db_.get_node(reopen_node, output.snapshot_version);
    if (!node ||
        !temporal_allowed(node->metadata,
                          "node " + std::to_string(reopen_node))) {
      continue;
    }
    bool seeded_downstream = false;
    for (const auto& edge :
         db_.outgoing_edges(reopen_node, output.snapshot_version)) {
      if (!edge_allowed(edge, options.mode) ||
          !temporal_allowed(edge.metadata,
                            "edge " + std::to_string(edge.id))) {
        continue;
      }
      const size_t before = candidates.size();
      append_candidate(edge.to, std::max(0.50, options.minimum_confidence));
      seeded_downstream = seeded_downstream || candidates.size() > before;
      if (candidates.size() >= options.semantic_candidates) break;
    }
    if (!node->root || !seeded_downstream) {
      append_candidate(reopen_node,
                       std::max(0.50, options.minimum_confidence));
    }
    if (candidates.size() >= options.semantic_candidates) break;
  }

  for (const auto& candidate : raw_candidates) {
    append_candidate(candidate.node_id, candidate.score);
    if (candidates.size() >= options.semantic_candidates) break;
  }
  for (const auto& candidate : raw_candidates) {
    if (candidates.size() >= options.semantic_candidates) break;
    append_candidate(candidate.node_id, candidate.score);
  }
  for (const auto& candidate : candidates) {
    output.semantic_candidates.push_back(candidate.node_id);
  }
