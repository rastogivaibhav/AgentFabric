#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${1:-/tmp/generic_hidden_rule_native_helper_p6b1}"
SRC=/tmp/chess_counterfactual_native_helper.cpp

# Generate proven Theoretical helper with expanded traversal budget.
bash "$ROOT/arc_competition/build_chess_counterfactual_native_helper_p42.sh" /tmp/p6b1-base-helper >/tmp/p6b1-base-helper.log

# Retarget only the generated /tmp helper to the P6B.1 comparison-anchor namespace.
python3 - "$SRC" <<'PY'
from pathlib import Path
import sys
p=Path(sys.argv[1]); s=p.read_text()
old='static const std::string prefix = "cf2-ply-";'
new='static const std::string prefix = "hidden-v2-";'
if old not in s:
    raise SystemExit('P6B.1 retrieval-prefix patch anchor missing')
s=s.replace(old,new,1)
p.write_text(s)
PY

grep -q 'static const std::string prefix = "hidden-v2-"' "$SRC"
grep -q 'QueryMode::Theoretical' "$SRC"
grep -q 'options.dialectic.semantic_candidates = 1' "$SRC"
grep -q 'options.dialectic.max_visited_states = 4096' "$SRC"

RUNTIME="$ROOT/pi45_a1/a15_native_runtime"
MODEL_WORLD="$ROOT/pi45_a1/a15_native_model_world"
CORE="$ROOT/pi45_a1/graphenedb_snapshot"
g++ -std=c++20 -O2 -UNDEBUG \
  -I"$RUNTIME/include" -I"$MODEL_WORLD/include" -I"$CORE/include" \
  "$SRC" \
  "$CORE/src/db.cpp" "$CORE/src/platform_posix.cpp" \
  "$MODEL_WORLD/src/model_world.cpp" \
  "$RUNTIME/src/epistemic.cpp" "$RUNTIME/src/dialectic.cpp" \
  "$RUNTIME/src/fiber_bundle.cpp" "$RUNTIME/src/stability_critic.cpp" \
  "$RUNTIME/src/epistemic_control.cpp" "$RUNTIME/src/escape.cpp" \
  "$RUNTIME/src/self_healing.cpp" "$RUNTIME/src/path_verifier.cpp" \
  "$RUNTIME/src/hypokosh_runtime.cpp" -pthread -o "$OUT"

git -C "$ROOT" diff --exit-code -- \
  pi45_a1/a15_native_runtime \
  pi45_a1/graphenedb_snapshot \
  pi45_a1/a15_native_runtime_helper.cpp

printf '%s\n' \
  'generic_hidden_rule_p6b1_helper=PASS' \
  'query_mode=Theoretical' \
  'retrieval_anchor_prefix=hidden-v2-' \
  'semantic_seed_count=1' \
  'max_visited_states=4096' \
  'core_graphenedb_modified=false' \
  'core_hypokosh_modified=false'
sha256sum "$SRC" "$OUT"
