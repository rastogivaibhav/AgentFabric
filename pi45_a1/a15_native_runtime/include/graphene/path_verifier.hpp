#pragma once

#include "graphene/dialectic.hpp"

#include <cstdint>
#include <string>
#include <vector>

namespace graphene {

struct PathVerificationContext {
  const std::vector<float>* query_vector{nullptr};
  uint64_t query_signature{0};
  uint64_t snapshot_version{0};
  uint32_t target_node{0};
  QueryMode mode{QueryMode::Balanced};
};

struct PathVerificationResult {
  double query_relevance{1.0};
  double target_consistency{1.0};
  double completeness{1.0};
  PathRoleHint role{PathRoleHint::Auto};
  SemanticVerificationStatus semantic_verification{
      SemanticVerificationStatus::Unverified};
  std::string verifier_version;
  std::vector<std::string> findings;
};

class PathVerifier {
 public:
  virtual ~PathVerifier() = default;

  virtual PathVerificationResult verify(
      const DialecticPath& path,
      const PathVerificationContext& context) const = 0;
};

void apply_path_verifier(BundleSet* bundles,
                         const PathVerifier& verifier,
                         const std::vector<float>& query_vector,
                         uint64_t query_signature,
                         QueryMode mode);

}  // namespace graphene
