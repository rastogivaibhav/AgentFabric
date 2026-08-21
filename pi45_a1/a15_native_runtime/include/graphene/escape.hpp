#pragma once

#include "graphene/stability_critic.hpp"

#include <chrono>
#include <cstddef>
#include <cstdint>
#include <string>
#include <vector>

namespace graphene {

enum class EscapeAction : uint8_t {
  ExpandMinorityPath,
  SearchContradiction,
  SearchTemporalNeighbour,
  SeekIndependentEvidence,
  GenerateFalsificationQuestion,
  GenerateMissingEvidenceQuery,
  PruneRetrievalNoise,
  VerifySemanticClaim,
  ExploreAnalogy,
  RequestHumanEvidence
};

struct EscapeTask {
  EscapeAction action{EscapeAction::ExpandMinorityPath};
  uint32_t target_node{0};
  std::string reason;
};

struct EscapePlan {
  std::vector<EscapeTask> tasks;
  size_t max_new_nodes{200};
  size_t max_new_edges{512};
  uint32_t max_depth{2};
  std::chrono::milliseconds time_budget{500};
  bool requires_human_evidence{false};
  bool generic_expansion_allowed{false};
};

class CorrectiveEscape {
 public:
  EscapePlan plan(const FiberBundle& bundle,
                  const StabilityAssessment& assessment,
                  QueryMode mode = QueryMode::Balanced) const;
};

}  // namespace graphene
