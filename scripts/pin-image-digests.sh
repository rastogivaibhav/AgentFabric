#!/usr/bin/env bash
# scripts/pin-image-digests.sh
#
# Resolves the sha256 digest for every first-party image at the given tag
# and writes them into deploy/helm/values.yaml.
#
# Called by CI after images are pushed to GHCR:
#   ./scripts/pin-image-digests.sh 1.0.0
#
# Requirements: docker (logged into ghcr.io), yq ≥ 4.x

set -euo pipefail

TAG="${1:-1.0.0}"
REGISTRY="ghcr.io/agentfabric"
VALUES="deploy/helm/values.yaml"

IMAGES=(
  "collector:.collector.image.digest"
  "af-core:.afCore.image.digest"
  "api:.api.image.digest"
  "portal:.portal.image.digest"
)

echo "Pinning image digests for tag ${TAG} …"

for entry in "${IMAGES[@]}"; do
  name="${entry%%:*}"
  yq_path="${entry##*:}"
  ref="${REGISTRY}/${name}:${TAG}"

  echo -n "  ${ref} → "
  docker pull --quiet "${ref}" >/dev/null
  digest=$(docker inspect --format='{{index .RepoDigests 0}}' "${ref}" | cut -d@ -f2)
  echo "${digest}"

  # Write digest into values.yaml (uncomments the line if present as comment)
  # yq edit-in-place:
  yq e "${yq_path} = \"${digest}\"" -i "${VALUES}"
done

echo "Done. Commit the updated ${VALUES}."
