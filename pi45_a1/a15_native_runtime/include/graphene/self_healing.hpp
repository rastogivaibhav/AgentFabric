#pragma once

#include "graphene/escape.hpp"

#include <cstdint>
#include <string>
#include <vector>

namespace graphene {

enum class SafeRepairAction : uint8_t {
  PreserveMinorityPath,
  RequestSourceEvidence,
  MarkContested,
  DemoteUnsafeShortcut,
  SearchTemporalWindow,
  GenerateIndependentTest,
  PruneRetrievalNoise,
  VerifySemanticClaim,
  StopAndAbstain
};

struct SafeRepairProposal {
  SafeRepairAction action{SafeRepairAction::PreserveMinorityPath};
  std::string reason;
  bool mutates_truth_status{false};
};

struct SelfHealingPlan {
  std::vector<std::string> observations;
  std::vector<std::string> critiques;
  std::vector<SafeRepairProposal> repairs;
  std::vector<std::string> discovery_questions;
  bool rerun_required{false};
  bool human_review_required{false};
};

class RecursiveSelfHealingController {
 public:
  SelfHealingPlan plan(const StabilityAssessment& stability,
                       const OppositionReport& opposition,
                       const EscapePlan& escape) const;
};

}  // namespace graphene
