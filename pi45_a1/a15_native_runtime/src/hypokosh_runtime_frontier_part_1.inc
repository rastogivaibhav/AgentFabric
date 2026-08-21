#include "graphene/hypokosh_runtime.hpp"

#include <algorithm>
#include <set>

namespace graphene {
namespace {

bool has_action(const EscapePlan& plan, EscapeAction action) {
  return std::any_of(plan.tasks.begin(), plan.tasks.end(),
                     [&](const EscapeTask& task) {
    return task.action == action;
  });
}

bool has_searchable_action(const EscapePlan& plan) {
  return has_action(plan, EscapeAction::GenerateMissingEvidenceQuery) ||
         has_action(plan, EscapeAction::SearchTemporalNeighbour) ||
         has_action(plan, EscapeAction::SeekIndependentEvidence) ||
         has_action(plan, EscapeAction::SearchContradiction) ||
         has_action(plan, EscapeAction::ExpandMinorityPath) ||
         has_action(plan, EscapeAction::PruneRetrievalNoise) ||
         has_action(plan, EscapeAction::ExploreAnalogy);
}

bool depth_repair_active(const EscapePlan& plan) {
  return has_action(plan, EscapeAction::GenerateMissingEvidenceQuery) ||
         has_action(plan, EscapeAction::SearchTemporalNeighbour);
}

DialecticOptions targeted_options(DialecticOptions options,
                                  const EscapePlan& plan) {
  if (has_action(plan, EscapeAction::GenerateMissingEvidenceQuery) ||
      has_action(plan, EscapeAction::SearchTemporalNeighbour)) {
    options.max_hops = std::min<uint32_t>(16, options.max_hops + 1);
  }
  if (has_action(plan, EscapeAction::SeekIndependentEvidence)) {
    options.semantic_candidates =
        std::min<size_t>(64, options.semantic_candidates + 4);
    options.max_paths =
        std::min<size_t>(256, std::max<size_t>(2, options.max_paths * 2));
  }
  if (has_action(plan, EscapeAction::SearchContradiction) ||
      has_action(plan, EscapeAction::ExpandMinorityPath)) {
    options.max_paths_per_root =
        std::min<size_t>(64, options.max_paths_per_root + 4);
    options.max_paths =
        std::min<size_t>(256, std::max<size_t>(2, options.max_paths * 2));
  }
  if (has_action(plan, EscapeAction::PruneRetrievalNoise)) {
    options.minimum_confidence =
        std::min(0.95, options.minimum_confidence + 0.05);
  }
  if (has_searchable_action(plan)) {
    options.max_visited_states =
        std::min<size_t>(200000,
                         std::max<size_t>(2, options.max_visited_states * 2));
  }
  return options;
}

DialecticOptions opposition_options(DialecticOptions options,
                                    const OppositionReport& opposition) {
  if (!opposition.requests_reexpansion || opposition.reopen_nodes.empty()) {
    return options;
  }
  options.reopen_nodes = opposition.reopen_nodes;
  options.semantic_candidates = std::min<size_t>(
      64, options.semantic_candidates +
              std::min<size_t>(4, opposition.reopen_nodes.size()));
  options.max_paths =
      std::min<size_t>(256, std::max<size_t>(2, options.max_paths * 2));
  options.max_paths_per_root = std::min<size_t>(
      64, options.max_paths_per_root +
              std::min<size_t>(4, opposition.reopen_nodes.size()));
  options.max_visited_states =
      std::min<size_t>(200000,
                       std::max<size_t>(2, options.max_visited_states * 2));
  return options;
}

bool options_changed(const DialecticOptions& left,
                     const DialecticOptions& right) {
  return left.semantic_candidates != right.semantic_candidates ||
         left.max_hops != right.max_hops ||
         left.max_paths != right.max_paths ||
         left.max_paths_per_root != right.max_paths_per_root ||
         left.max_visited_states != right.max_visited_states ||
         left.minimum_confidence != right.minimum_confidence ||
         left.reopen_nodes != right.reopen_nodes;
}

uint64_t append_state_hash(uint64_t hash, uint64_t value) {
  for (int shift = 0; shift < 8; ++shift) {
    hash ^= static_cast<unsigned char>((value >> (shift * 8)) & 0xffU);
    hash *= 1099511628211ULL;
  }
  return hash;
}

uint64_t reasoning_state_hash(const FiberBundle& bundle,
                              const DialecticOptions& options) {
  uint64_t hash = bundle.immutable_hash;
  hash = append_state_hash(hash, bundle.visited_states);
  hash = append_state_hash(hash, bundle.truncated ? 1U : 0U);
  hash = append_state_hash(hash, options.semantic_candidates);
  hash = append_state_hash(hash, options.max_hops);
  hash = append_state_hash(hash, options.max_paths);
  hash = append_state_hash(hash, options.max_paths_per_root);
  hash = append_state_hash(hash, options.max_visited_states);
  hash = append_state_hash(
      hash, static_cast<uint64_t>(options.minimum_confidence * 1000000.0));
  for (uint32_t node : options.reopen_nodes) {
    hash = append_state_hash(hash, node);
  }
  return hash;
}

std::vector<uint32_t> all_evidence_edges(
    const ConvergedAnswer& convergence) {
  std::vector<uint32_t> values = convergence.evidence_edges;
  std::sort(values.begin(), values.end());
  values.erase(std::unique(values.begin(), values.end()), values.end());
  return values;
}
