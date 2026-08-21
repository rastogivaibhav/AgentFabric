#include "graphene/path_verifier.hpp"

#include <algorithm>
#include <set>

namespace graphene {
namespace {

SemanticVerificationStatus combine_status(
    SemanticVerificationStatus existing,
    SemanticVerificationStatus requested) {
  if (existing == SemanticVerificationStatus::Contradicted ||
      requested == SemanticVerificationStatus::Contradicted) {
    return SemanticVerificationStatus::Contradicted;
  }
  if (existing == SemanticVerificationStatus::Unverified) return requested;
  if (requested == SemanticVerificationStatus::Unverified) return existing;
  if (existing == requested) return existing;
  return SemanticVerificationStatus::Unverified;
}

}  // namespace

void apply_path_verifier(BundleSet* bundles,
                         const PathVerifier& verifier,
                         const std::vector<float>& query_vector,
                         uint64_t query_signature,
                         QueryMode mode) {
  if (!bundles) return;
  std::set<std::string> versions;
  for (auto& root : bundles->roots) {
    for (auto& path : root.paths) {
      const PathVerificationContext context{
          &query_vector,
          query_signature,
          bundles->snapshot_version,
          root.root_node,
          mode};
      const PathVerificationResult result = verifier.verify(path, context);
      path.query_relevance = std::min(
          std::clamp(path.query_relevance, 0.0, 1.0),
          std::clamp(result.query_relevance, 0.0, 1.0));
      path.target_consistency = std::min(
          std::clamp(path.target_consistency, 0.0, 1.0),
          std::clamp(result.target_consistency, 0.0, 1.0));
      path.completeness = std::min(
          std::clamp(path.completeness, 0.0, 1.0),
          std::clamp(result.completeness, 0.0, 1.0));
      if (result.role != PathRoleHint::Auto) path.role_hint = result.role;
      path.semantic_verification = combine_status(
          path.semantic_verification, result.semantic_verification);
      if (!result.verifier_version.empty()) {
        path.verifier_version = result.verifier_version;
        versions.insert(result.verifier_version);
      }
      path.verification_findings.insert(
          path.verification_findings.end(),
          result.findings.begin(), result.findings.end());
      std::sort(path.verification_findings.begin(),
                path.verification_findings.end());
      path.verification_findings.erase(
          std::unique(path.verification_findings.begin(),
                      path.verification_findings.end()),
          path.verification_findings.end());
    }
  }
  for (const auto& version : versions) {
    bundles->warnings.push_back("PATH_VERIFIER_VERSION:" + version);
  }
  std::sort(bundles->warnings.begin(), bundles->warnings.end());
  bundles->warnings.erase(
      std::unique(bundles->warnings.begin(), bundles->warnings.end()),
      bundles->warnings.end());
}

}  // namespace graphene
