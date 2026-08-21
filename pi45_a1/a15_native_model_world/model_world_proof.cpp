#include "graphene/model_world.hpp"

#include <cassert>
#include <filesystem>
#include <iostream>
#include <string>

using namespace graphene;

int main(int argc, char** argv) {
  const std::filesystem::path out = argc > 1 ? argv[1] : "/tmp/a15_model_world.mw";
  std::filesystem::remove(out);
  std::filesystem::remove(out.string() + ".tmp");

  ModelWorld world;

  ModelWorldNode observation;
  observation.type = ModelWorldNodeType::Fact;
  observation.status = ModelWorldStatus::Active;
  observation.statement = "A persistent state change was observed after an experiment.";
  observation.origin = EdgeOrigin::Observed;
  observation.evidence_edges = {101};
  observation.metadata["turn"] = "0";
  const auto observation_id = world.add(observation, "ARC observation ingested");

  ModelWorldNode hypothesis;
  hypothesis.type = ModelWorldNodeType::Hypothesis;
  hypothesis.status = ModelWorldStatus::Active;
  hypothesis.statement = "The probed element may be interactable.";
  hypothesis.origin = EdgeOrigin::Hypothetical;
  hypothesis.evidence_edges = {101};
  hypothesis.related_nodes = {observation_id};
  hypothesis.metadata["turn"] = "0";
  const auto hypothesis_id = world.add(hypothesis, "LLM proposal preserved as hypothesis");

  ModelWorldNode alternative;
  alternative.type = ModelWorldNodeType::Hypothesis;
  alternative.status = ModelWorldStatus::Contested;
  alternative.statement = "The probed element may be inert in this context.";
  alternative.origin = EdgeOrigin::Hypothetical;
  alternative.evidence_edges = {101};
  alternative.related_nodes = {observation_id};
  const auto alternative_id = world.add(alternative, "Competing hypothesis preserved");

  ModelWorldNode goal;
  goal.type = ModelWorldNodeType::Decision;
  goal.status = ModelWorldStatus::Active;
  goal.statement = "Determine which interaction hypothesis better explains the observed transition.";
  goal.origin = EdgeOrigin::Hypothetical;
  goal.evidence_edges = {101};
  goal.related_nodes = {hypothesis_id, alternative_id};
  const auto goal_id = world.add(goal, "Candidate goal derived from competing hypotheses");

  ModelWorldNode opposition;
  opposition.type = ModelWorldNodeType::Opposition;
  opposition.status = ModelWorldStatus::Active;
  opposition.statement = "What result would falsify the interactable interpretation?";
  opposition.origin = EdgeOrigin::Hypothetical;
  opposition.related_nodes = {hypothesis_id, alternative_id};
  const auto opposition_id = world.add(opposition, "Opposition retained before closure");

  ModelWorldNode experiment;
  experiment.type = ModelWorldNodeType::Experiment;
  experiment.status = ModelWorldStatus::Active;
  experiment.statement = "Probe an available action to discriminate the competing hypotheses.";
  experiment.origin = EdgeOrigin::Hypothetical;
  experiment.related_nodes = {hypothesis_id, alternative_id, goal_id};
  const auto experiment_id = world.add(experiment, "Action represented as experiment before execution");

  ModelWorldNode outcome;
  outcome.type = ModelWorldNodeType::Outcome;
  outcome.status = ModelWorldStatus::Active;
  outcome.statement = "The environment changed persistently after the action.";
  outcome.origin = EdgeOrigin::Observed;
  outcome.evidence_edges = {202};
  outcome.related_nodes = {experiment_id, hypothesis_id, alternative_id};
  const auto outcome_id = world.add(outcome, "Observed environment outcome");

  // Core no-silent-promotion invariant: a hypothetical node cannot become
  // Verified merely because the proposal system wants it to.
  const bool illegal_promotion = world.update_status(
      hypothesis_id, ModelWorldStatus::Verified, "attempted silent hypothesis promotion");
  assert(!illegal_promotion);

  // An externally discovered/observed proposition with evidence can be verified.
  ModelWorldNode discovered;
  discovered.type = ModelWorldNodeType::Fact;
  discovered.status = ModelWorldStatus::Active;
  discovered.statement = "The executed action produced a persistent state delta.";
  discovered.origin = EdgeOrigin::Discovered;
  discovered.evidence_edges = {202};
  discovered.related_nodes = {outcome_id};
  const auto discovered_id = world.add(discovered, "Outcome-derived discovered fact");
  const bool legal_promotion = world.update_status(
      discovered_id, ModelWorldStatus::Verified, "verified from observed transition evidence");
  assert(legal_promotion);

  const auto findings = world.audit();
  for (const auto& f : findings) {
    assert(f.code != "SILENT_TRUTH_PROMOTION");
    assert(f.code != "EVENT_CHECKSUM_MISMATCH");
  }

  const auto hash_before = world.event_log_hash();
  const auto nodes_before = world.nodes().size();
  const auto events_before = world.events().size();
  assert(nodes_before == 8);
  assert(events_before == 9); // 8 adds + one legal status transition.
  assert(world.save(out));

  ModelWorld restored;
  assert(restored.load(out));
  assert(restored.nodes().size() == nodes_before);
  assert(restored.events().size() == events_before);
  assert(restored.event_log_hash() == hash_before);

  ModelWorldScheduler scheduler;
  const auto report = scheduler.run(restored);
  for (const auto& f : report.findings) {
    assert(f.code != "SILENT_TRUTH_PROMOTION");
    assert(f.code != "EVENT_CHECKSUM_MISMATCH");
  }

  std::cout << "a15_native_model_world=PASS\n";
  std::cout << "nodes=" << restored.nodes().size() << "\n";
  std::cout << "events=" << restored.events().size() << "\n";
  std::cout << "event_log_hash=" << restored.event_log_hash() << "\n";
  std::cout << "illegal_hypothesis_promotion_blocked=" << (!illegal_promotion ? 1 : 0) << "\n";
  std::cout << "evidence_backed_fact_promotion_allowed=" << (legal_promotion ? 1 : 0) << "\n";
  std::cout << "opposition_node_id=" << opposition_id << "\n";
  std::cout << "experiment_node_id=" << experiment_id << "\n";
  std::cout << "outcome_node_id=" << outcome_id << "\n";
  std::cout << "audit_findings=" << report.findings.size() << "\n";
  return 0;
}
