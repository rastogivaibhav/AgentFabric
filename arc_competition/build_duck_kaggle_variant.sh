#!/usr/bin/env bash
set -euo pipefail

MODE="${1:?usage: build_duck_kaggle_variant.sh off|evidence [duck-root]}"
DUCK_ROOT="${2:-/tmp/duck-harness}"
ROOT="${GITHUB_WORKSPACE:-$(pwd)}"
PINNED_DUCK_COMMIT='7652836056c59e044f093e3c13ed7438c814169e'

case "$MODE" in
  off|evidence) ;;
  dialectic)
    echo 'dialectic Kaggle packaging is intentionally blocked until native CompleteHypoKoshRuntime invocation is wired into the Duck action seam.' >&2
    exit 64
    ;;
  *)
    echo "unsupported mode: $MODE" >&2
    exit 64
    ;;
esac

if [ ! -d "$DUCK_ROOT/.git" ]; then
  git clone https://github.com/Tufalabs/duck-harness.git "$DUCK_ROOT"
fi
git -C "$DUCK_ROOT" fetch origin "$PINNED_DUCK_COMMIT" --depth=1
git -C "$DUCK_ROOT" checkout --detach "$PINNED_DUCK_COMMIT"
test "$(git -C "$DUCK_ROOT" rev-parse HEAD)" = "$PINNED_DUCK_COMMIT"

git -C "$DUCK_ROOT" reset --hard "$PINNED_DUCK_COMMIT"
git -C "$DUCK_ROOT" clean -fd

PATCH_MANIFEST="$ROOT/results/arc-paper/package-$MODE/duck_patch_manifest.json"
mkdir -p "$(dirname "$PATCH_MANIFEST")"
python3 "$ROOT/arc_competition/apply_duck_graphene_patch.py" "$DUCK_ROOT" \
  --bridge "$ROOT/arc_competition/graphene_dmw_duck_bridge.py" \
  --manifest-out "$PATCH_MANIFEST"

python3 -m py_compile \
  "$DUCK_ROOT/ARC3-Inference/inference/agent/tool_agent.py" \
  "$DUCK_ROOT/ARC3-Inference/inference/agent/graphene_dmw_bridge.py" \
  "$DUCK_ROOT/ARC3-Inference/inference/framework/solver.py"

# Duck's own deployment target creates both a source dataset and Kaggle notebook.
# Dry-run means no Kaggle credentials or upload are required here. The produced
# notebook keeps internet disabled and targets the competition RTX Pro 6000.
cd "$DUCK_ROOT/ARC3-Inference"
GRAPHENE_DMW_MODE="$MODE" \
KAGGLE_DRY_RUN=true \
KAGGLE_ENABLE_INTERNET=false \
KAGGLE_ACCELERATOR=NvidiaRtxPro6000 \
KAGGLE_MAKE_SHARE_VERSION=true \
KAGGLE_PUBLIC=true \
KAGGLE_USERNAME="${KAGGLE_USERNAME:-graphene-dmw-dryrun}" \
make kaggle-duck

# Locate newest dry-run Kaggle staging output and copy an immutable evidence map.
RUNS_DIR="$DUCK_ROOT/ARC3-Inference/runs"
STAGED="$(find "$RUNS_DIR" -type d -path '*/kaggle/source-dataset' -printf '%T@ %p\n' 2>/dev/null | sort -nr | head -n1 | cut -d' ' -f2-)"
test -n "$STAGED" && test -d "$STAGED"
KERNEL_DIR="$(dirname "$STAGED")/kernel"
test -d "$KERNEL_DIR"

OUT="$ROOT/results/arc-paper/package-$MODE"
printf '%s\n' \
  "mode=$MODE" \
  "duck_commit=$PINNED_DUCK_COMMIT" \
  'model=vrfai/Qwen3.6-27B-FP8' \
  'accelerator=NvidiaRtxPro6000' \
  'internet=false' \
  'kaggle_dry_run=true' \
  > "$OUT/package_identity.txt"
find "$STAGED" -maxdepth 2 -type f -printf 'source/%P\n' | sort > "$OUT/source_bundle_files.txt"
find "$KERNEL_DIR" -maxdepth 2 -type f -printf 'kernel/%P\n' | sort > "$OUT/kernel_bundle_files.txt"
sha256sum "$ROOT/arc_competition/graphene_dmw_duck_bridge.py" "$ROOT/arc_competition/apply_duck_graphene_patch.py" > "$OUT/integration_SHA256SUMS.txt"

echo "duck_kaggle_variant_package=PASS mode=$MODE"
echo "source_dataset=$STAGED"
echo "kernel_bundle=$KERNEL_DIR"
