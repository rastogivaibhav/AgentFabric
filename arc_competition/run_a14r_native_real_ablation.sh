#!/usr/bin/env bash
set -euo pipefail

DUCK_ROOT=${DUCK_ROOT:-/tmp/duck-harness}
ENV_ROOT=${ENV_ROOT:-/tmp/arc-public-envs}
OUT=${OUT:-results/arc-paper/a14r-native-real}
GAME=${GAME:-ft09-0d8bbf25}
MAX_ACTIONS=${MAX_ACTIONS:-12}
MAX_RUNTIME_MINUTES=${MAX_RUNTIME_MINUTES:-8}

: "${OPENAI_API_KEY:?OPENAI_API_KEY is required for this diagnostic run}"
: "${GRAPHENE_DMW_NATIVE_HELPER:=/tmp/duck_dialectic_native_helper}"
: "${GRAPHENE_DMW_BOOTSTRAP:=/tmp/a15_graphene_bootstrap}"
export GRAPHENE_DMW_NATIVE_HELPER GRAPHENE_DMW_BOOTSTRAP

test -d "$DUCK_ROOT/ARC3-Inference"
test -x "$GRAPHENE_DMW_NATIVE_HELPER"
test -x "$GRAPHENE_DMW_BOOTSTRAP"

rm -rf "$ENV_ROOT"
git clone --depth=1 https://huggingface.co/datasets/zarczynski/arc-agi-3-public "$ENV_ROOT"
test -d "$ENV_ROOT/environment_files/ft09/0d8bbf25"

python3 - "$DUCK_ROOT" "$ENV_ROOT" <<'PY'
import json, sys
from pathlib import Path
duck=Path(sys.argv[1]); env=Path(sys.argv[2])
p=duck/'ARC3-Inference/configs/inference.json'
c=json.loads(p.read_text())
c['shared'].update({'model_name':'gpt-5.6-sol','base_url':'https://api.openai.com/v1','provider':'openai','context_window':32768})
c['environment'].update({'games':['ft09-0d8bbf25'],'include_tags':[],'environments_dir':str(env/'environment_files'),'n_passes':1,'concurrent_jobs':1,'max_steps':12,'max_runtime_minutes':8})
c['deployment']['target']='inline'
c['analyzer'].update({'model_id':'gpt-5.6-sol','base_url':'https://api.openai.com/v1','provider':'openai','context_window':32768,'tool_steps':6,'timeout':120,'save_request_logs':True})
c['multimodal']['context']=''
Path('/tmp/a14r-native-real.json').write_text(json.dumps(c,indent=2,sort_keys=True))
PY

python3 -m pip install --quiet uv
cd "$DUCK_ROOT/ARC3-Inference"
uv venv --python 3.12.12 .venv
uv sync --locked --python .venv/bin/python
uv pip install --python .venv/bin/python --no-deps -e ../tufa-arc-agi-framework

run_arm() {
  local name=$1
  local goal_flag=$2
  local scope=$3
  local dir="/tmp/${name}"
  rm -rf "$dir"
  GRAPHENE_DMW_MODE=dialectic \
  GRAPHENE_DMW_GOAL_DISCOVERY="$goal_flag" \
  GRAPHENE_DMW_WORLD_SCOPE="$scope" \
  CONFIG_PATH=/tmp/a14r-native-real.json EXPERIMENT_DIR="$dir" \
  GAME="$GAME" GAME_TAGS='[]' ENVIRONMENTS_DIR="$ENV_ROOT/environment_files" \
  N_PASSES=1 CONCURRENT_JOBS=1 MAX_ACTIONS="$MAX_ACTIONS" MAX_RUNTIME_MINUTES="$MAX_RUNTIME_MINUTES" \
  SIMULATE_COMPETITION_ARCADE=true make interactive
  CONFIG_PATH=/tmp/a14r-native-real.json make score_run \
    SCORE_RUN_DIR="$dir" SCORE_OUTPUT_PATH="$dir/evaluation.json"
}

run_arm a14-native off ft09-a14-native-control
run_arm a14r-native true ft09-a14r-native-goal-discovery

cd "$GITHUB_WORKSPACE"
mkdir -p "$OUT"
cp /tmp/a14-native/evaluation.json "$OUT/a14-evaluation.json"
cp /tmp/a14r-native/evaluation.json "$OUT/a14r-evaluation.json"
find /tmp/a14-native -type f -name graphene_dmw_state.json -exec cp {} "$OUT/a14-state.json" \; || true
find /tmp/a14r-native -type f -name graphene_dmw_state.json -exec cp {} "$OUT/a14r-state.json" \; || true

python3 - "$OUT" "$GAME" <<'PY'
import json, sys
from pathlib import Path
out=Path(sys.argv[1]); game=sys.argv[2]
def metric(name):
    x=json.loads((out/f'{name}-evaluation.json').read_text())
    row=x['games'][game]
    return {
        'overall_score':float(x.get('score') or 0),
        'game_score':float(row.get('score') or 0),
        'trial_count':int(row.get('trial_count') or 0),
        'trial_scores':row.get('trial_scores') or {},
    }
a14=metric('a14'); a14r=metric('a14r')
state={}
p=out/'a14r-state.json'
if p.exists(): state=json.loads(p.read_text())
obs=[x for x in state.get('crawler_records',[]) if x.get('kind')=='CrawlerObservation']
events=state.get('dialectic_events') or []
summary={
    'experiment':'A1.4 vs A1.4R native dialectical goal discovery',
    'game':game,
    'model':'gpt-5.6-sol',
    'competition_valid_model':False,
    'action_cap':12,
    'only_intended_treatment_difference':'GRAPHENE_DMW_GOAL_DISCOVERY off vs true',
    'a14':a14,
    'a14r':a14r,
    'score_delta':a14r['game_score']-a14['game_score'],
    'crawler_observations':len(obs),
    'dialectic_events':len(events),
    'pass_gameplay_improvement':a14r['game_score']>a14['game_score'],
}
(out/'summary.json').write_text(json.dumps(summary,indent=2,sort_keys=True))
print(json.dumps(summary,indent=2,sort_keys=True))
PY

find "$OUT" -type f ! -name SHA256SUMS.txt -print0 | sort -z | xargs -0 sha256sum > "$OUT/SHA256SUMS.txt"
