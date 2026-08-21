#!/usr/bin/env bash
set -euo pipefail

sudo apt-get update
sudo apt-get install -y python3.12 python3.12-venv g++ curl git
python3.12 -m venv /tmp/pi45venv
/tmp/pi45venv/bin/pip install --upgrade pip
/tmp/pi45venv/bin/pip install 'arc-agi==0.9.9'
/tmp/pi45venv/bin/python - <<'PY'
import importlib.metadata
assert importlib.metadata.version('arc-agi') == '0.9.9'
print('arc_agi_version=0.9.9')
PY

# Verify vendored GrapheneDB source identity. No model weights or llama.cpp are
# downloaded for API-backed proposer experiments.
GDB="$GITHUB_WORKSPACE/pi45_a1/graphenedb_snapshot"
MANIFEST="$GDB/SOURCE_MANIFEST.txt"
test -f "$MANIFEST"
grep -qx 'pinned_commit=9dfe3681c315008377710a0731eab78ebba798a9' "$MANIFEST"
grep -qx 'source_mode=vendored-byte-identical-core' "$MANIFEST"
while read -r rel expected; do
  case "$rel" in
    include/*|src/*)
      actual="$(git hash-object "$GDB/$rel")"
      test "$actual" = "$expected" || {
        echo "GrapheneDB source identity mismatch: $rel expected=$expected actual=$actual" >&2
        exit 1
      }
      ;;
  esac
done < "$MANIFEST"
printf '%s\n' '9dfe3681c315008377710a0731eab78ebba798a9' > /tmp/graphenedb_commit.txt
sha256sum "$MANIFEST" > /tmp/graphenedb_source_manifest_SHA256SUMS.txt

echo 'a15_prepare_arc_api=PASS'
