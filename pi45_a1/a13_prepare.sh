#!/usr/bin/env bash
set -euo pipefail

cat pi45_a1/payload.part.00 pi45_a1/payload.part.01 pi45_a1/payload.part.02 > /tmp/pi45a1_payload.tgz.b64
test "$(wc -c < /tmp/pi45a1_payload.tgz.b64)" -eq 15112
echo '6e23baa3a1c6b9855c1cdfd38f6ce367d95b524ee59a97be7113be08555b0fc7  /tmp/pi45a1_payload.tgz.b64' | sha256sum -c -
base64 -d /tmp/pi45a1_payload.tgz.b64 > /tmp/pi45a1_payload.tgz
echo '0e10bc6e81feab2071d1e75b1dc398656e666801ae0a608edf747b25af5857d6  /tmp/pi45a1_payload.tgz' | sha256sum -c -
rm -rf /tmp/pi45a1
mkdir -p /tmp/pi45a1
tar -xzf /tmp/pi45a1_payload.tgz -C /tmp/pi45a1
cd /tmp/pi45a1
echo '63f205b3dbbad0d580dc8a262edbf8dd432c3ce6df173e0f888ac2188eeb78d1  arc_a1_graphene_episodic.py' | sha256sum -c -
echo '0c517b8c25b118bb85924ca5446f7b6cdb16b895c464e928a45ca532b3f5db8b  pi45_graphene_memory.cpp' | sha256sum -c -
echo '1fe625d28b1edda6a034bb442d50de8a901f8579fea658d8b073928239e0da54  CMakeLists.txt' | sha256sum -c -
echo '66a24a5b1632a386c6a6f5a5ce91bdda207149ebab8389535bb5c00110b38a86  test_arc_a1_graphene_episodic.py' | sha256sum -c -
python3 -m py_compile arc_a1_graphene_episodic.py
python3 -m py_compile "$GITHUB_WORKSPACE/pi45_a1/transport_safe_chat_proxy.py"
python3 -m py_compile "$GITHUB_WORKSPACE/pi45_a1/transport_safe_runner.py"
! grep -q 'causal_search(' arc_a1_graphene_episodic.py
! grep -q 'lattice_neighbors(' arc_a1_graphene_episodic.py
! grep -q 'FiberBundle' arc_a1_graphene_episodic.py

sudo apt-get update
sudo apt-get install -y cmake ninja-build curl git python3.12 python3.12-venv
python3.12 -m venv /tmp/pi45venv
/tmp/pi45venv/bin/pip install --upgrade pip
/tmp/pi45venv/bin/pip install 'arc-agi==0.9.9' pytest
/tmp/pi45venv/bin/python - <<'PY'
import importlib.metadata
assert importlib.metadata.version('arc-agi') == '0.9.9'
print('arc_agi_version=0.9.9')
PY
sudo mkdir -p /mnt/data/pi45a1 /mnt/data/pi45a0
sudo chown -R "$USER":"$USER" /mnt/data/pi45a1 /mnt/data/pi45a0
cp /tmp/pi45a1/arc_a1_graphene_episodic.py /mnt/data/pi45a1/arc_a1_graphene_episodic.py
base64 -d "$GITHUB_WORKSPACE/pi45_arc/arc_a0_baseline.py.gz.b64" | gzip -d > /mnt/data/pi45a0/arc_a0_baseline.py
echo '8c251f9bd3241aa1bdaac988ce462b08ab570fb7bcb89ee21c168dc7cf728e62  /mnt/data/pi45a0/arc_a0_baseline.py' | sha256sum -c -
cd /tmp/pi45a1
/tmp/pi45venv/bin/pytest -q test_arc_a1_graphene_episodic.py

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
cmake -S /tmp/pi45a1 -B /tmp/pi45a1/build -G Ninja -DCMAKE_BUILD_TYPE=Release -DGRAPHENEDB_SOURCE_DIR="$GDB"
cmake --build /tmp/pi45a1/build --target pi45_graphene_memory -j 2
test -x /tmp/pi45a1/build/pi45_graphene_memory
printf '%s\n' '9dfe3681c315008377710a0731eab78ebba798a9' > /tmp/graphenedb_commit.txt
printf '%s\n' 'vendored-byte-identical-core' > /tmp/graphenedb_source_mode.txt
sha256sum "$MANIFEST" > /tmp/graphenedb_source_manifest_SHA256SUMS.txt

rm -rf /tmp/llama.cpp
git clone https://github.com/ggerganov/llama.cpp.git /tmp/llama.cpp
git -C /tmp/llama.cpp checkout 9ee9fc04c136ef2ae729bfc60d18961b23c13ddf
test "$(git -C /tmp/llama.cpp rev-parse HEAD)" = '9ee9fc04c136ef2ae729bfc60d18961b23c13ddf'
cmake -S /tmp/llama.cpp -B /tmp/llama.cpp/build -G Ninja -DCMAKE_BUILD_TYPE=Release -DGGML_NATIVE=OFF -DLLAMA_CURL=OFF
cmake --build /tmp/llama.cpp/build --target llama-server -j 2

MODEL=/tmp/qwen2.5-1.5b-instruct-q4_k_m.gguf
curl -fL --retry 5 --retry-delay 3 --connect-timeout 30 \
  'https://huggingface.co/Qwen/Qwen2.5-1.5B-Instruct-GGUF/resolve/main/qwen2.5-1.5b-instruct-q4_k_m.gguf?download=true' \
  -o "$MODEL"
echo '6a1a2eb6d15622bf3c96857206351ba97e1af16c30d7a74ee38970e434e9407e  /tmp/qwen2.5-1.5b-instruct-q4_k_m.gguf' | sha256sum -c -
sha256sum "$MODEL" > /tmp/model_SHA256SUMS.txt

echo 'a13_prepare=PASS'
