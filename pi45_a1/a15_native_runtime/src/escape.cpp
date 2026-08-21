#include "graphene/escape.hpp"

#include <set>

namespace graphene {

EscapePlan CorrectiveEscape::plan(const FiberBundle& bundle,
                                  const StabilityAssessment& assessment,
                                  QueryMode mode) const {
  EscapePlan output;
  std::set<EscapeAction> added;
  const uint32_t target =
      bundle.fibers.empty() ? 0 : bundle.fibers.front().target_node;
  auto add = [&](EscapeAction action, const std::string& reason) {
    if (added.insert(action).second)
      output.tasks.push_back({action, target, reason});
  };

  if (assessment.retrieval_noise_penalty > 0.0) {
    add(EscapeAction::PruneRetrievalNoise,
        "quarantine irrelevant paths before any wider search");
  }
  if (assessment.material_contradiction > 0.0) {
    add(EscapeAction::SearchContradiction,
        "seek discriminating evidence for the strongest material opposition");
    add(EscapeAction::GenerateFalsificationQuestion,
        "define a test that can resolve the competing explanations");
  }
  if (assessment.completeness_score < 0.75 ||
      assessment.missing_evidence_penalty > 0.0) {
    add(EscapeAction::GenerateMissingEvidenceQuery,
        "retrieve the missing critical edge rather than widening all paths");
  }
  if (assessment.independent_support_score < 0.75) {
    add(EscapeAction::SeekIndependentEvidence,
        "search for a new evidence family with independent ancestry");
  }
  if (assessment.temporal_consistency < 1.0) {
    add(EscapeAction::SearchTemporalNeighbour,
        "find evidence valid at the requested query time");
  }
  if (assessment.pattern_lock_score > 0.55 &&
      assessment.retrieval_noise_penalty == 0.0) {
    add(EscapeAction::ExpandMinorityPath,
        "reopen a relevant minority explanation without rewarding noise");
  }
  if (assessment.requires_external_verification) {
    add(EscapeAction::VerifySemanticClaim,
        "obtain an external verifier result before final resolution");
  }
  if (mode == QueryMode::Theoretical &&
      assessment.path_diversity < 0.50) {
    add(EscapeAction::ExploreAnalogy,
        "explore a labelled analogy without treating it as empirical support");
  }
  if (assessment.requires_abstention || assessment.lyapunov_energy > 0.70) {
    add(EscapeAction::RequestHumanEvidence,
        "available evidence remains outside the governed admissibility set");
    output.requires_human_evidence = true;
  }

  output.generic_expansion_allowed = output.tasks.empty();
  return output;
}

}  // namespace graphene
