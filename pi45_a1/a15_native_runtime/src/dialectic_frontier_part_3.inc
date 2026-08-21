
  std::map<uint32_t, RootBundle> grouped;
  std::map<uint32_t, std::set<std::vector<uint32_t>>> path_keys;
  size_t total_paths = 0;

  for (const auto& candidate : candidates) {
    if (total_paths >= options.max_paths || output.truncated) break;
    ExpansionState initial;
    initial.node = candidate.node_id;
    initial.anchor = candidate.node_id;
    initial.score = clamp01(candidate.score);
    initial.reverse_nodes.push_back(candidate.node_id);
    std::deque<ExpansionState> queue;
    queue.push_back(std::move(initial));

    while (!queue.empty() && total_paths < options.max_paths) {
      if (output.visited_states >= options.max_visited_states) {
        output.truncated = true;
        output.warnings.push_back("VISIT_BUDGET_EXHAUSTED");
        break;
      }
      ExpansionState current = std::move(queue.front());
      queue.pop_front();
      ++output.visited_states;

      auto current_node = db_.get_node(current.node, output.snapshot_version);
      if (!current_node ||
          !temporal_allowed(current_node->metadata,
                            "node " + std::to_string(current.node))) {
        continue;
      }

      if (current_node->root) {
        RootBundle& root = grouped[current.node];
        root.root_node = current.node;
        if (root.paths.size() < options.max_paths_per_root &&
            path_keys[current.node].insert(current.reverse_edges).second) {
          DialecticPath path;
          path.root_node = current.node;
          path.anchor_node = current.anchor;
          path.nodes.assign(current.reverse_nodes.rbegin(), current.reverse_nodes.rend());
          path.edges.assign(current.reverse_edges.rbegin(), current.reverse_edges.rend());
          path.score = clamp01(current.score);
          path.contains_contradiction = current.contradiction;
          path.contains_hypothetical = current.hypothetical;
          path.temporal_consistent = true;
          path.evidence = std::move(current.evidence);
          path.provenance_findings = std::move(current.findings);
          path.joint_requirements = std::move(current.joint_requirements);
          root.paths.push_back(std::move(path));
          ++total_paths;
        }
        continue;
      }

      if (current.depth >= options.max_hops) continue;
      auto incoming = db_.incoming_edges(current.node, output.snapshot_version);
      std::vector<Edge> ordinary;
      std::map<std::string, std::vector<Edge>> joint_groups;
      for (const auto& edge : incoming) {
        const std::string hyperedge_id =
            metadata_value(edge.metadata, "hyperedge_id");
        if (!hyperedge_id.empty() &&
            metadata_value(edge.metadata, "hyperedge_semantics") ==
                "all_sources") {
          std::string group_id =
              metadata_value(edge.metadata, "hyperedge_group_id");
          if (group_id.empty()) {
            group_id = hyperedge_id + ":" +
                       std::to_string(edge.created_version);
          }
          joint_groups[group_id].push_back(edge);
        } else {
          ordinary.push_back(edge);
        }
      }

      auto enqueue_ordinary = [&](const Edge& edge) {
        if (!edge_allowed(edge, options.mode) ||
            !temporal_allowed(edge.metadata,
                              "edge " + std::to_string(edge.id))) {
          return;
        }
        auto source = db_.get_node(edge.from, output.snapshot_version);
        if (!source ||
            !temporal_allowed(source->metadata,
                              "node " + std::to_string(edge.from))) {
          return;
        }
        if (std::find(current.reverse_nodes.begin(), current.reverse_nodes.end(), edge.from) !=
            current.reverse_nodes.end()) {
          return;
        }

        ExpansionState next = current;
        next.node = edge.from;
        next.reverse_nodes.push_back(edge.from);
        next.reverse_edges.push_back(edge.id);
        ++next.depth;
        next.score *= clamp01(edge.confidence);
        next.contradiction =
            next.contradiction || edge.role == EdgeRole::Contradicts;
        next.hypothetical =
            next.hypothetical || edge.origin == EdgeOrigin::Hypothetical;

        DialecticPath assessment;
        append_provenance(edge, &assessment);
        next.evidence.insert(next.evidence.end(),
                             assessment.evidence.begin(),
                             assessment.evidence.end());
        next.findings.insert(next.findings.end(),
                             assessment.provenance_findings.begin(),
                             assessment.provenance_findings.end());
        queue.push_back(std::move(next));
      };
      for (const auto& edge : ordinary) enqueue_ordinary(edge);

      for (const auto& [group_id, members] : joint_groups) {
        const std::string hyperedge_id =
            metadata_value(members.front().metadata, "hyperedge_id");
        std::vector<uint32_t> required_sources;
        bool valid_group = members.size() >= 2 &&
                           parse_u32_list(metadata_value(
                                              members.front().metadata,
