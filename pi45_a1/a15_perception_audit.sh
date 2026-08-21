#!/usr/bin/env bash
set -euo pipefail
OUT="${1:-/tmp/a15-perception-audit}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
rm -rf "$OUT" /tmp/pi45a1 /tmp/pi45venv
mkdir -p "$OUT" /tmp/pi45a1

cat "$ROOT/pi45_a1/payload.part.00" "$ROOT/pi45_a1/payload.part.01" "$ROOT/pi45_a1/payload.part.02" > /tmp/pi45a1_payload.tgz.b64
base64 -d /tmp/pi45a1_payload.tgz.b64 > /tmp/pi45a1_payload.tgz
tar -xzf /tmp/pi45a1_payload.tgz -C /tmp/pi45a1
cp /tmp/pi45a1/arc_a1_graphene_episodic.py "$OUT/frozen_agent_source.py"
sha256sum "$OUT/frozen_agent_source.py" > "$OUT/frozen_agent_source_SHA256SUMS.txt"

python3.12 -m venv /tmp/pi45venv
/tmp/pi45venv/bin/pip -q install --upgrade pip
/tmp/pi45venv/bin/pip -q install 'arc-agi==0.9.9' pytest
/tmp/pi45venv/bin/python - <<'PY' > "$OUT/arc_agi_package.txt"
import arc_agi, inspect, pathlib, importlib.metadata
print('version=' + importlib.metadata.version('arc-agi'))
print('module=' + str(pathlib.Path(inspect.getfile(arc_agi)).resolve()))
print('exports=' + ','.join(sorted(x for x in dir(arc_agi) if not x.startswith('_'))))
PY

# Preserve the package source inventory for interface review without relying on docs.
/tmp/pi45venv/bin/python - <<'PY' > "$OUT/arc_agi_source_inventory.txt"
import arc_agi, inspect, pathlib
root=pathlib.Path(inspect.getfile(arc_agi)).resolve().parent
for p in sorted(root.rglob('*.py')):
    print(p.relative_to(root))
PY

# Extract all frozen-agent source lines that touch environment perception/action semantics.
/tmp/pi45venv/bin/python - <<'PY' > "$OUT/frozen_agent_interface_lines.txt"
from pathlib import Path
p=Path('/tmp/pi45a1/arc_a1_graphene_episodic.py')
terms=('arc_agi','grid','available','action','levels','score','state','observation','frame','reset','environment')
for i,line in enumerate(p.read_text().splitlines(),1):
    low=line.lower()
    if any(t in low for t in terms):
        print(f'{i:04d}: {line}')
PY

# Build only the already-pinned GrapheneDB helper needed by the frozen A1 agent.
GDB="$ROOT/pi45_a1/graphenedb_snapshot"
cmake -S /tmp/pi45a1 -B /tmp/pi45a1/build -G Ninja -DCMAKE_BUILD_TYPE=Release -DGRAPHENEDB_SOURCE_DIR="$GDB" >/dev/null
cmake --build /tmp/pi45a1/build --target pi45_graphene_memory -j 2 >/dev/null

python3 "$ROOT/pi45_a1/a15_capture_llm_server.py" --listen 9090 --out "$OUT/exact_llm_request.json" > "$OUT/capture_server.log" 2>&1 &
SERVER_PID=$!
trap 'kill "$SERVER_PID" 2>/dev/null || true' EXIT
for _ in $(seq 1 50); do curl -fsS http://127.0.0.1:9090/health >/dev/null 2>&1 && break; sleep .1; done

set +e
/tmp/pi45venv/bin/python "$ROOT/pi45_a1/transport_safe_runner.py" \
  --client-timeout-floor 30 \
  --script /tmp/pi45a1/arc_a1_graphene_episodic.py \
  --games ft09 --max-actions 1 --memory-window 1 \
  --graphene-helper /tmp/pi45a1/build/pi45_graphene_memory --out "$OUT/run" \
  > "$OUT/agent_stdout.log" 2> "$OUT/agent_stderr.log"
RC=$?
set -e
printf '%s\n' "$RC" > "$OUT/agent_exit_code.txt"
test -f "$OUT/exact_llm_request.json"

python3 - "$OUT" <<'PY'
import json, pathlib, re, sys, hashlib
out=pathlib.Path(sys.argv[1])
req=json.loads((out/'exact_llm_request.json').read_text())
messages=req.get('messages') or []
text='\n'.join(str(m.get('content','')) for m in messages)
(out/'exact_model_context.txt').write_text(text)

# Conservative lexical leakage inventory. Presence is evidence to review, not automatic failure.
patterns={
 'game_id': r'\bft09\b',
 'levels': r'\blevels?\b|0/6|6 levels',
 'score': r'\bscore\b',
 'semantic_goal': r'\bgoal[_ -]?(position|cell|location)|target[_ -]?(position|cell|location)',
 'semantic_objects': r'\b(player|enemy|key|door|collectible|hazard)[_ -]?(position|location|state)?\b',
 'meaningful_actions': r'\b(move[_ -]?(up|down|left|right)|pick[_ -]?up|open[_ -]?door|jump|shoot)\b',
 'opaque_actions': r'\bACTION[0-9]+\b',
 'grid': r'\bgrid\b',
}
findings={k: bool(re.search(v,text,re.I)) for k,v in patterns.items()}
summary={
 'messages': len(messages),
 'request_chars': len(json.dumps(req,sort_keys=True)),
 'context_chars': len(text),
 'findings': findings,
 'model_request_sha256': hashlib.sha256(json.dumps(req,sort_keys=True,separators=(',',':')).encode()).hexdigest(),
 'context_sha256': hashlib.sha256(text.encode()).hexdigest(),
 'visual_pixels_supplied': bool(re.search(r'(?i)(image_url|data:image|pixel|screenshot|frame_buffer)', json.dumps(req))),
}
json.dump(summary, open(out/'perception_contract_summary.json','w'), indent=2, sort_keys=True)
print(json.dumps(summary,indent=2,sort_keys=True))
PY

sha256sum "$OUT/exact_llm_request.json" "$OUT/exact_model_context.txt" "$OUT/perception_contract_summary.json" > "$OUT/evidence_SHA256SUMS.txt"
echo 'a15_perception_audit=PASS_CAPTURED'
