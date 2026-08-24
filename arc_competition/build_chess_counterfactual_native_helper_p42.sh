#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${1:-/tmp/chess_counterfactual_native_helper_p42}"
SRC=/tmp/chess_counterfactual_native_helper.cpp

# First generate the existing plugin-only Theoretical helper. This leaves the
# generated C++ source in /tmp while proving repository core remains unchanged.
bash "$ROOT/arc_competition/build_chess_counterfactual_native_helper.sh" /tmp/chess_counterfactual_native_helper_p42_base >/tmp/chess_counterfactual_native_helper_p42_base.log

python3 - "$SRC" <<'PY'
from pathlib import Path
import sys
p=Path(sys.argv[1])
s=p.read_text()
old='options.dialectic.max_paths = 128;'
new='''options.dialectic.max_paths = 512;
    options.dialectic.max_visited_states = 4096;
    options.dialectic.max_paths_per_root = 64;'''
if old not in s:
    raise SystemExit('P4.2 traversal-budget patch anchor missing')
s=s.replace(old,new,1)
p.write_text(s)
PY

grep -q 'options.dialectic.max_paths = 512' "$SRC"
grep -q 'options.dialectic.max_visited_states = 4096' "$SRC"
grep -q 'options.dialectic.max_paths_per_root = 64' "$SRC"

RUNTIME="$ROOT/pi45_a1/a15_native_runtime"
MODEL_WORLD="$ROOT/pi45_a1/a15_native_model_world"
CORE="$ROOT/pi45_a1/graphenedb_snapshot"

g++ -std=c++20 -O2 -UNDEBUG \
  -I"$RUNTIME/include" -I"$MODEL_WORLD/include" -I"$CORE/include" \
  "$SRC" \
  "$CORE/src/db.cpp" "$CORE/src/platform_posix.cpp" \
  "$MODEL_WORLD/src/model_world.cpp" \
  "$RUNTIME/src/epistemic.cpp" \
  "$RUNTIME/src/dialectic.cpp" \
  "$RUNTIME/src/fiber_bundle.cpp" \
  "$RUNTIME/src/stability_critic.cpp" \
  "$RUNTIME/src/epistemic_control.cpp" \
  "$RUNTIME/src/escape.cpp" \
  "$RUNTIME/src/self_healing.cpp" \
  "$RUNTIME/src/path_verifier.cpp" \
  "$RUNTIME/src/hypokosh_runtime.cpp" \
  -pthread -o "$OUT"

# Core/source isolation guard.
git -C "$ROOT" diff --exit-code -- \
  pi45_a1/a15_native_runtime \
  pi45_a1/graphenedb_snapshot \
  pi45_a1/a15_native_runtime_helper.cpp

printf '%s\n' \
  'chess_counterfactual_p42_helper=PASS' \
  'query_mode=Theoretical' \
  'max_paths=512' \
  'max_visited_states=4096' \
  'max_paths_per_root=64' \
  'core_graphenedb_modified=false' \
  'core_hypokosh_modified=false'
sha256sum "$SRC" "$OUT"
