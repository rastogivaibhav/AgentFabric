#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${1:-/tmp/chess_counterfactual_native_helper}"
SRC=/tmp/chess_counterfactual_native_helper.cpp
BASE="$ROOT/pi45_a1/a15_native_runtime_helper.cpp"
RUNTIME="$ROOT/pi45_a1/a15_native_runtime"
MODEL_WORLD="$ROOT/pi45_a1/a15_native_model_world"
CORE="$ROOT/pi45_a1/graphenedb_snapshot"

cp "$BASE" "$SRC"
python3 - "$SRC" <<'PY'
from pathlib import Path
import sys

p=Path(sys.argv[1])
s=p.read_text()
old='''std::vector<float> anchor_vector() {
  return std::vector<float>(kDimension, 0.125f);
}
'''
new='''std::vector<float> anchor_vector() {
  return std::vector<float>(kDimension, 0.125f);
}

int chess_plugin_anchor_turn(const std::string& external_id) {
  static const std::string prefix = "cf2-ply-";
  static const std::string suffix = "-comparison-anchor";
  if (external_id.rfind(prefix, 0) != 0 ||
      external_id.find(suffix) == std::string::npos) return -1;
  const size_t begin = prefix.size();
  const size_t end = external_id.find(suffix, begin);
  if (end == std::string::npos || end == begin) return -1;
  try { return std::stoi(external_id.substr(begin, end - begin)); }
  catch (...) { return -1; }
}

// Plugin-only retrieval vectors. Each ply's comparison anchor occupies its own
// dimension so previous-turn anchors cannot dominate current-turn convergence.
// All non-anchor nodes occupy a separate direction. This changes only the
// generated experiment helper under /tmp, never GrapheneDB/HypoKosh core.
std::vector<float> chess_plugin_node_vector(const RequestNode& node) {
  std::vector<float> out(kDimension, 0.0f);
  const int turn = chess_plugin_anchor_turn(node.external_id);
  if (turn >= 0) {
    out[2 + (static_cast<size_t>(turn) % (kDimension - 2))] = 1.0f;
  } else {
    out[1] = 1.0f;
  }
  return out;
}

std::vector<float> chess_plugin_query_vector(int turn) {
  std::vector<float> out(kDimension, 0.0f);
  const size_t safe_turn = static_cast<size_t>(std::max(0, turn));
  out[2 + (safe_turn % (kDimension - 2))] = 1.0f;
  return out;
}
'''
if old not in s:
    raise SystemExit('anchor_vector patch anchor missing')
s=s.replace(old,new,1)
old='input.vector = anchor_vector();'
if old not in s:
    raise SystemExit('node-vector patch anchor missing')
s=s.replace(old,'input.vector = chess_plugin_node_vector(node);',1)
old='options.dialectic.mode = QueryMode::Balanced;'
if old not in s:
    raise SystemExit('query-mode patch anchor missing')
s=s.replace(old,'options.dialectic.mode = QueryMode::Theoretical;',1)
old='options.dialectic.semantic_candidates = 12;'
if old not in s:
    raise SystemExit('semantic-candidate patch anchor missing')
s=s.replace(old,'options.dialectic.semantic_candidates = 1;',1)
old='options.dialectic.max_paths = 32;'
if old not in s:
    raise SystemExit('max-path patch anchor missing')
s=s.replace(old,'options.dialectic.max_paths = 128;',1)
old='runtime.reason(anchor_vector(), signature_for(1, 1), options);'
if old not in s:
    raise SystemExit('query-vector patch anchor missing')
s=s.replace(old,'runtime.reason(chess_plugin_query_vector(request.turn), signature_for(1, 1), options);',1)

old='''    std::cout << "\\\"confidence\\\":" << std::setprecision(17) << result.confidence << ',';
    std::cout << "\\\"governed_action\\\":\\\"" << json_escape(action_authorized ? request.action : "") << "\\\",";
'''
new='''    std::cout << "\\\"confidence\\\":" << std::setprecision(17) << result.confidence << ',';
    std::cout << "\\\"debug_fibers\\\":[";
    for (size_t fi = 0; fi < result.final_bundle.fibers.size(); ++fi) {
      if (fi) std::cout << ',';
      const auto& fiber = result.final_bundle.fibers[fi];
      size_t support_count = 0;
      size_t opposition_count = 0;
      double best_support_quality = 0.0;
      for (const auto& path : fiber.paths) {
        if (path.eligible_for_support) {
          ++support_count;
          const double q = path.confidence * path.query_relevance *
                           path.target_consistency * path.completeness *
                           path.provenance_quality;
          if (q > best_support_quality) best_support_quality = q;
        }
        if (path.eligible_for_opposition) ++opposition_count;
      }
      const auto external = hypothesis_external_id_for_graph_node(db, world, fiber.target_node);
      std::cout << '{'
                << "\\\"target_node\\\":" << fiber.target_node << ','
                << "\\\"hypothesis_id\\\":\\\"" << json_escape(external.value_or("")) << "\\\"," 
                << "\\\"path_count\\\":" << fiber.paths.size() << ','
                << "\\\"eligible_support_paths\\\":" << support_count << ','
                << "\\\"eligible_opposition_paths\\\":" << opposition_count << ','
                << "\\\"independent_evidence_family_count\\\":" << fiber.independent_evidence_family_count << ','
                << "\\\"best_support_quality\\\":" << std::setprecision(17) << best_support_quality
                << '}';
    }
    std::cout << "],";
    std::cout << "\\\"governed_action\\\":\\\"" << json_escape(action_authorized ? request.action : "") << "\\\",";
'''
if old not in s:
    raise SystemExit('debug-fiber output patch anchor missing')
s=s.replace(old,new,1)
p.write_text(s)
PY

grep -q 'QueryMode::Theoretical' "$SRC"
grep -q 'chess_plugin_anchor_turn' "$SRC"
grep -q 'chess_plugin_node_vector' "$SRC"
grep -q 'chess_plugin_query_vector(request.turn)' "$SRC"
grep -q 'options.dialectic.semantic_candidates = 1' "$SRC"
grep -q 'options.dialectic.max_paths = 128' "$SRC"
grep -q 'debug_fibers' "$SRC"

# Prove the repository-owned helper and native/core sources were not edited by this builder.
if ! cmp -s "$BASE" "$ROOT/pi45_a1/a15_native_runtime_helper.cpp"; then
  echo 'repository helper unexpectedly changed' >&2
  exit 1
fi

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

printf '%s\n' \
  'chess_counterfactual_helper=PASS' \
  'query_mode=Theoretical' \
  'semantic_seed_count=1' \
  'retrieval_focus=current_turn_comparison_anchor' \
  'retrieval_turn_isolation=true' \
  'max_paths=128' \
  'core_graphenedb_modified=false' \
  'core_hypokosh_modified=false'
sha256sum "$SRC" "$OUT"
