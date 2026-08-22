#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${1:-/tmp/duck_dialectic_native_helper}"
SRC=/tmp/chess_counterfactual_native_helper.cpp

bash "$ROOT/arc_competition/build_chess_counterfactual_native_helper_p42.sh" /tmp/duck-dmw-base-helper >/tmp/duck-dmw-base-helper.log
python3 - "$SRC" <<'PY'
from pathlib import Path
import sys
p=Path(sys.argv[1]); s=p.read_text()
old='static const std::string prefix = "cf2-ply-";'
new='static const std::string prefix = "duck-dmw-";'
if old not in s: raise SystemExit('Duck DMW retrieval-prefix patch anchor missing')
s=s.replace(old,new,1)
old2='options.dialectic.semantic_candidates = 12;'
if old2 in s:
    s=s.replace(old2,'options.dialectic.semantic_candidates = 1;',1)
p.write_text(s)
PY

grep -q 'static const std::string prefix = "duck-dmw-"' "$SRC"
grep -q 'QueryMode::Theoretical' "$SRC"
grep -q 'options.dialectic.semantic_candidates = 1' "$SRC"
grep -q 'options.dialectic.max_visited_states = 4096' "$SRC"

RUNTIME="$ROOT/pi45_a1/a15_native_runtime"; MODEL_WORLD="$ROOT/pi45_a1/a15_native_model_world"; CORE="$ROOT/pi45_a1/graphenedb_snapshot"
g++ -std=c++20 -O2 -UNDEBUG \
  -I"$RUNTIME/include" -I"$MODEL_WORLD/include" -I"$CORE/include" \
  "$SRC" "$CORE/src/db.cpp" "$CORE/src/platform_posix.cpp" "$MODEL_WORLD/src/model_world.cpp" \
  "$RUNTIME/src/epistemic.cpp" "$RUNTIME/src/dialectic.cpp" "$RUNTIME/src/fiber_bundle.cpp" \
  "$RUNTIME/src/stability_critic.cpp" "$RUNTIME/src/epistemic_control.cpp" "$RUNTIME/src/escape.cpp" \
  "$RUNTIME/src/self_healing.cpp" "$RUNTIME/src/path_verifier.cpp" "$RUNTIME/src/hypokosh_runtime.cpp" \
  -pthread -o "$OUT"

git -C "$ROOT" diff --exit-code -- pi45_a1/a15_native_runtime pi45_a1/graphenedb_snapshot pi45_a1/a15_native_runtime_helper.cpp
printf '%s\n' \
  'duck_dialectic_native_helper=PASS' \
  'query_mode=Theoretical' \
  'retrieval_anchor_prefix=duck-dmw-' \
  'semantic_seed_count=1' \
  'core_graphenedb_modified=false' \
  'core_hypokosh_modified=false'
sha256sum "$SRC" "$OUT"
