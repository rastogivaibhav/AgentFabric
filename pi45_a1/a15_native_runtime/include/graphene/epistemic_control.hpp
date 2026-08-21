#pragma once

#include "graphene/stability_critic.hpp"

#include <string>
#include <vector>

namespace graphene {

struct EpistemicAdmissibility {
  double relevance{0.0};
  double target_consistency{0.0};
  double completeness{0.0};
  double provenance{0.0};
  double independent_support{0.0};
  double retrieval_noise{0.0};
  double unresolved_contradiction{0.0};

  bool evidence_admissible{false};
  bool contradiction_blocks_resolution{false};
  bool sufficient_independent_support{false};
  bool requires_external_verification{true};
  SemanticVerificationStatus semantic_verification{
      SemanticVerificationStatus::Unverified};
  std::vector<std::string> reasons;
};

class EpistemicController {
 public:
  EpistemicAdmissibility assess(
      const FiberBundle& bundle,
      const StabilityAssessment& stability,
      QueryMode mode = QueryMode::Balanced) const;

  ConvergedAnswer converge(
      const FiberBundle& bundle,
      const EpistemicAdmissibility& admissibility,
      const StabilityAssessment& stability,
      const DialecticOptions& options = {}) const;

  OppositionReport oppose(
      const FiberBundle& bundle,
      const ConvergedAnswer& answer,
      const EpistemicAdmissibility& admissibility,
      const StabilityAssessment& stability,
      const DialecticOptions& options = {}) const;
};

}  // namespace graphene
