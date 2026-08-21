#!/usr/bin/env bash
set -euo pipefail
ROOT="${GITHUB_WORKSPACE:-$(pwd)}"
SLICE="$ROOT/pi45_a1/a15_native_model_world"
CORE="$ROOT/pi45_a1/graphenedb_snapshot"
OUT="${1:-/tmp/a15-native-model-world-evidence}"
mkdir -p "$OUT"

# Prove the vendored slice is byte-identical to the pinned upstream Git blobs.
grep -E '^(include/|src/)' "$SLICE/SOURCE_MANIFEST.txt" | while read -r rel expected; do
  actual="$(git hash-object "$SLICE/$rel")"
  test "$actual" = "$expected" || {
    echo "A1.5 native source identity mismatch: $rel expected=$expected actual=$actual" >&2
    exit 1
  }
done

g++ -std=c++20 -O2 -pthread \
  -I"$SLICE/include" \
  -I"$CORE/include" \
  "$SLICE/src/model_world.cpp" \
  "$CORE/src/platform_posix.cpp" \
  "$SLICE/model_world_proof.cpp" \
  -o "$OUT/a15_model_world_proof"

"$OUT/a15_model_world_proof" "$OUT/model_world.snapshot" | tee "$OUT/model_world_proof.txt"
grep -qx 'a15_native_model_world=PASS' "$OUT/model_world_proof.txt"
grep -qx 'nodes=8' "$OUT/model_world_proof.txt"
grep -qx 'events=9' "$OUT/model_world_proof.txt"
grep -qx 'illegal_hypothesis_promotion_blocked=1' "$OUT/model_world_proof.txt"
grep -qx 'evidence_backed_fact_promotion_allowed=1' "$OUT/model_world_proof.txt"

cp "$SLICE/SOURCE_MANIFEST.txt" "$OUT/SOURCE_MANIFEST.txt"
sha256sum \
  "$SLICE/SOURCE_MANIFEST.txt" \
  "$SLICE/include/graphene/epistemic.hpp" \
  "$SLICE/include/graphene/dialectic.hpp" \
  "$SLICE/include/graphene/fiber_bundle.hpp" \
  "$SLICE/include/graphene/model_world.hpp" \
  "$SLICE/src/model_world.cpp" \
  "$SLICE/model_world_proof.cpp" \
  "$OUT/model_world.snapshot" \
  > "$OUT/SHA256SUMS.txt"

echo 'a15_native_model_world_proof=PASS'
