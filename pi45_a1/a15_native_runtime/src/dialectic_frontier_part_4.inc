                                              "hyperedge_sources"),
                                          &required_sources);
        std::vector<uint32_t> actual_sources;
        std::vector<uint32_t> member_edges;
        double confidence_product = 1.0;
        bool contradiction = false;
        bool hypothetical = false;

        for (const auto& member : members) {
          actual_sources.push_back(member.from);
          member_edges.push_back(member.id);
          if (metadata_value(member.metadata, "hyperedge_id") != hyperedge_id ||
              metadata_value(member.metadata, "hyperedge_semantics") !=
                  "all_sources" ||
              (!metadata_value(member.metadata, "hyperedge_group_id").empty() &&
               metadata_value(member.metadata, "hyperedge_group_id") !=
                   group_id) ||
              metadata_value(member.metadata, "hyperedge_sources") !=
                  metadata_value(members.front().metadata,
                                 "hyperedge_sources") ||
              !edge_allowed(member, options.mode) ||
              !temporal_allowed(member.metadata,
                                "edge " + std::to_string(member.id))) {
            valid_group = false;
          }
          confidence_product *= clamp01(member.confidence);
          contradiction =
              contradiction || member.role == EdgeRole::Contradicts;
          hypothetical =
              hypothetical || member.origin == EdgeOrigin::Hypothetical;
        }
        std::sort(actual_sources.begin(), actual_sources.end());
        std::sort(member_edges.begin(), member_edges.end());
        if (actual_sources != required_sources) valid_group = false;
        const std::string arity_text =
            metadata_value(members.front().metadata, "hyperedge_arity");
        uint32_t arity = 0;
        const auto arity_parse =
            std::from_chars(arity_text.data(),
                            arity_text.data() + arity_text.size(), arity);
        if (arity_text.empty() || arity_parse.ec != std::errc{} ||
            arity_parse.ptr != arity_text.data() + arity_text.size() ||
            arity != required_sources.size()) {
          valid_group = false;
        }

        for (uint32_t source_id : required_sources) {
          auto source = db_.get_node(source_id, output.snapshot_version);
          if (!source ||
              !temporal_allowed(source->metadata,
                                "node " + std::to_string(source_id)) ||
              std::find(current.reverse_nodes.begin(),
                        current.reverse_nodes.end(),
                        source_id) != current.reverse_nodes.end()) {
            valid_group = false;
          }
        }
        if (!valid_group) {
          const std::string warning = "INCOMPLETE_HYPEREDGE: " + hyperedge_id;
          if (std::find(output.warnings.begin(), output.warnings.end(),
                        warning) == output.warnings.end()) {
            output.warnings.push_back(warning);
          }
          continue;
        }

        JointRequirement requirement;
        requirement.hyperedge_id = hyperedge_id;
        requirement.target_node = current.node;
        requirement.source_nodes = required_sources;
        requirement.member_edges = member_edges;
        requirement.all_sources_present = true;
        const double joint_confidence =
            std::pow(confidence_product,
                     1.0 / static_cast<double>(members.size()));

        for (const auto& member : members) {
          ExpansionState next = current;
          next.node = member.from;
          next.reverse_nodes.push_back(member.from);
          next.reverse_edges.insert(next.reverse_edges.end(),
                                    member_edges.begin(), member_edges.end());
          next.joint_requirements.push_back(requirement);
          ++next.depth;
          next.score *= joint_confidence;
          next.contradiction = next.contradiction || contradiction;
          next.hypothetical = next.hypothetical || hypothetical;
          for (const auto& provenance_member : members) {
            DialecticPath assessment;
            append_provenance(provenance_member, &assessment);
            next.evidence.insert(next.evidence.end(),
                                 assessment.evidence.begin(),
                                 assessment.evidence.end());
            next.findings.insert(
                next.findings.end(),
                assessment.provenance_findings.begin(),
                assessment.provenance_findings.end());
          }
          queue.push_back(std::move(next));
        }
      }
    }
  }

  for (auto& entry : grouped) {
    RootBundle root = std::move(entry.second);
    std::set<uint32_t> unique_edges;
    size_t contradictory_paths = 0;
    size_t edge_instances = 0;
    size_t finding_count = 0;
    size_t evidence_covered_edges = 0;
    double score_sum = 0.0;
    for (const auto& path : root.paths) {
      score_sum += path.score;
      if (path.contains_contradiction) ++contradictory_paths;
      unique_edges.insert(path.edges.begin(), path.edges.end());
      edge_instances += path.edges.size();
      finding_count += path.provenance_findings.size();
      std::set<uint32_t> missing_evidence_edges;
      for (const auto& finding : path.provenance_findings) {
        if (finding.code == "MISSING_EVIDENCE") {
          missing_evidence_edges.insert(finding.edge_id);
        }
      }
      evidence_covered_edges +=
          path.edges.size() >= missing_evidence_edges.size()
