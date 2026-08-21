#include "graphene/db.hpp"
#include "graphene/hypokosh_runtime.hpp"
#include "graphene/model_world.hpp"

#include <algorithm>
#include <cctype>
#include <filesystem>
#include <iomanip>
#include <iostream>
#include <map>
#include <optional>
#include <sstream>
#include <stdexcept>
#include <string>
#include <vector>

using namespace graphene;

namespace {

constexpr uint32_t kDimension = 64;

struct RequestNode {
  std::string external_id;
  std::string type;
  std::string status;
  std::string origin;
  std::string statement;
};

struct RequestRelation {
  std::string from;
  std::string to;
  std::string role;
  std::string origin;
  double confidence{0.0};
};

struct Request {
  std::string operation;
  std::string game_id;
  int turn{0};
  std::vector<RequestNode> nodes;
  std::vector<RequestRelation> relations;
  // Legacy v1/v2 transport fields are parsed for backwards-compatible native
  // proof fixtures, but A1.5 v3 never uses them to seed dialectic control.
  std::string provisional_hypothesis;
  std::string provisional_goal;
  std::vector<std::string> reopen;
  std::string experiment_id;
  std::string action;
};

int hex_value(char c) {
  if (c >= '0' && c <= '9') return c - '0';
  if (c >= 'a' && c <= 'f') return 10 + (c - 'a');
  if (c >= 'A' && c <= 'F') return 10 + (c - 'A');
  return -1;
}

std::string hex_decode(const std::string& hex) {
  if (hex.size() % 2 != 0) throw std::runtime_error("odd hex field");
  std::string out;
  out.reserve(hex.size() / 2);
  for (size_t i = 0; i < hex.size(); i += 2) {
    const int hi = hex_value(hex[i]);
    const int lo = hex_value(hex[i + 1]);
    if (hi < 0 || lo < 0) throw std::runtime_error("invalid hex field");
    out.push_back(static_cast<char>((hi << 4) | lo));
  }
  return out;
}

std::string json_escape(const std::string& text) {
  std::ostringstream out;
  for (unsigned char c : text) {
    switch (c) {
      case '\\': out << "\\\\"; break;
      case '"': out << "\\\""; break;
      case '\n': out << "\\n"; break;
      case '\r': out << "\\r"; break;
      case '\t': out << "\\t"; break;
      default:
        if (c < 0x20) {
          out << "\\u" << std::hex << std::setw(4) << std::setfill('0')
              << static_cast<int>(c) << std::dec;
        } else {
          out << static_cast<char>(c);
        }
    }
  }
  return out.str();
}

Request parse_request(std::istream& input) {
  Request request;
  std::string line;
  if (!std::getline(input, line) || line != "A15V1") {
    throw std::runtime_error("missing A15V1 protocol header");
  }
  bool ended = false;
  while (std::getline(input, line)) {
    if (line.empty()) continue;
    if (line == "END") { ended = true; break; }
    std::istringstream row(line);
    std::string kind;
    row >> kind;
    if (kind == "OP") {
      std::string value; row >> value; request.operation = hex_decode(value);
    } else if (kind == "GAME") {
      std::string value; row >> value; request.game_id = hex_decode(value);
    } else if (kind == "TURN") {
      row >> request.turn;
    } else if (kind == "NODE") {
      RequestNode node;
      std::string id_hex, statement_hex, metadata_hex;
      if (!(row >> id_hex >> node.type >> node.status >> node.origin >> statement_hex >> metadata_hex))
        throw std::runtime_error("invalid NODE record");
      node.external_id = hex_decode(id_hex);
      node.statement = hex_decode(statement_hex);
      (void)hex_decode(metadata_hex);
      request.nodes.push_back(std::move(node));
    } else if (kind == "REL") {
      RequestRelation relation;
      std::string from_hex, to_hex;
      if (!(row >> from_hex >> to_hex >> relation.role >> relation.origin >> relation.confidence))
        throw std::runtime_error("invalid REL record");
      relation.from = hex_decode(from_hex);
      relation.to = hex_decode(to_hex);
      request.relations.push_back(std::move(relation));
    } else if (kind == "PROVISIONAL_H") {
      std::string value; row >> value; request.provisional_hypothesis = hex_decode(value);
    } else if (kind == "PROVISIONAL_G") {
      std::string value; row >> value; request.provisional_goal = hex_decode(value);
    } else if (kind == "REOPEN") {
      std::string value; row >> value; request.reopen.push_back(hex_decode(value));
    } else if (kind == "EXPERIMENT") {
      std::string value; row >> value; request.experiment_id = hex_decode(value);
    } else if (kind == "ACTION") {
      std::string value; row >> value; request.action = hex_decode(value);
    } else {
      throw std::runtime_error("unknown protocol record: " + kind);
    }
  }
  if (!ended) throw std::runtime_error("protocol END missing");
  if (request.operation.empty() || request.game_id.empty())
    throw std::runtime_error("operation/game_id missing");
  if (request.nodes.empty()) throw std::runtime_error("request has no nodes");
  return request;
}

EdgeOrigin parse_origin(const std::string& value) {
  if (value == "Observed") return EdgeOrigin::Observed;
  if (value == "Discovered") return EdgeOrigin::Discovered;
  if (value == "Inferred") return EdgeOrigin::Inferred;
  if (value == "Reinforced") return EdgeOrigin::Reinforced;
  if (value == "Hypothetical") return EdgeOrigin::Hypothetical;
  throw std::runtime_error("unknown origin: " + value);
}

EdgeRole parse_role(const std::string& value) {
  if (value == "Mechanistic") return EdgeRole::Mechanistic;
  if (value == "Compressed") return EdgeRole::Compressed;
  if (value == "Analogical") return EdgeRole::Analogical;
  if (value == "Predictive") return EdgeRole::Predictive;
  if (value == "Causal") return EdgeRole::Causal;
  if (value == "Contradicts") return EdgeRole::Contradicts;
  if (value == "Supports") return EdgeRole::Supports;
  if (value == "Supersedes") return EdgeRole::Supersedes;
  throw std::runtime_error("unknown role: " + value);
}

ModelWorldNodeType parse_world_type(const std::string& value) {
  if (value == "Fact") return ModelWorldNodeType::Fact;
  if (value == "Concept") return ModelWorldNodeType::Concept;
  if (value == "Hypothesis") return ModelWorldNodeType::Hypothesis;
  if (value == "Contradiction") return ModelWorldNodeType::Contradiction;
  if (value == "Abstraction") return ModelWorldNodeType::Abstraction;
  if (value == "Opposition") return ModelWorldNodeType::Opposition;
  if (value == "Decision") return ModelWorldNodeType::Decision;
  if (value == "Experiment") return ModelWorldNodeType::Experiment;
  if (value == "Implementation") return ModelWorldNodeType::Implementation;
  if (value == "Outcome") return ModelWorldNodeType::Outcome;
  if (value == "Failure") return ModelWorldNodeType::Failure;
  if (value == "Reinforcement") return ModelWorldNodeType::Reinforcement;
  if (value == "Model") return ModelWorldNodeType::Model;
  throw std::runtime_error("unknown model-world type: " + value);
}

ModelWorldStatus parse_world_status(const std::string& value) {
  if (value == "Proposed") return ModelWorldStatus::Proposed;
  if (value == "Active") return ModelWorldStatus::Active;
  if (value == "Contested") return ModelWorldStatus::Contested;
  if (value == "Verified") return ModelWorldStatus::Verified;
  if (value == "Rejected") return ModelWorldStatus::Rejected;
  if (value == "Superseded") return ModelWorldStatus::Superseded;
  throw std::runtime_error("unknown model-world status: " + value);
}

std::vector<float> anchor_vector() {
  return std::vector<float>(kDimension, 0.125f);
}

uint32_t lookup_external_id(const GrapheneDB& db, const std::string& external_id) {
  const auto matches = db.metadata_search("external_id", external_id);
  if (matches.empty()) return 0;
  return matches.back();
}

std::optional<uint64_t> lookup_world_external_id(const ModelWorld& world,
                                                 const std::string& external_id) {
  for (const auto& node : world.nodes()) {
    const auto it = node.metadata.find("external_id");
    if (it != node.metadata.end() && it->second == external_id) return node.id;
  }
  return std::nullopt;
}

std::optional<std::string> hypothesis_external_id_for_graph_node(
    const GrapheneDB& db, const ModelWorld& world, uint32_t graph_node_id) {
  for (const auto& node : world.nodes()) {
    if (node.type != ModelWorldNodeType::Hypothesis) continue;
    const auto it = node.metadata.find("external_id");
    if (it == node.metadata.end()) continue;
    if (lookup_external_id(db, it->second) == graph_node_id) return it->second;
  }
  return std::nullopt;
}

std::vector<std::string> reopened_hypothesis_external_ids(
    const GrapheneDB& db, const ModelWorld& world, const std::vector<uint32_t>& graph_node_ids) {
  std::vector<std::string> out;
  for (const auto graph_node_id : graph_node_ids) {
    const auto external = hypothesis_external_id_for_graph_node(db, world, graph_node_id);
    if (external && std::find(out.begin(), out.end(), *external) == out.end()) {
      out.push_back(*external);
    }
  }
  return out;
}

void require_status(const Status& status, const std::string& where) {
  if (!status) throw std::runtime_error(where + ": " + status.message);
}

void emit_bool(std::ostream& out, const char* key, bool value, bool comma = true) {
  out << '"' << key << "\":" << (value ? "true" : "false");
  if (comma) out << ',';
}

void emit_string_array(std::ostream& out, const std::vector<std::string>& values) {
  out << '[';
  for (size_t i = 0; i < values.size(); ++i) {
    if (i) out << ',';
    out << '"' << json_escape(values[i]) << '"';
  }
  out << ']';
}

} // namespace

int main(int argc, char** argv) {
  try {
    std::filesystem::path db_path;
    for (int i = 1; i < argc; ++i) {
      const std::string arg = argv[i];
      if (arg == "--db" && i + 1 < argc) db_path = argv[++i];
      else throw std::runtime_error("usage: a15_native_runtime_helper --db PATH");
    }
    if (db_path.empty()) throw std::runtime_error("--db is required");

    const Request request = parse_request(std::cin);

    DBOptions db_options;
    db_options.dimension = kDimension;
    db_options.create_if_missing = true;
    db_options.fsync_on_commit = true;
    GrapheneDB db;
    require_status(db.open(db_path, db_options), "open GrapheneDB");

    ModelWorld world;
    const auto world_path = std::filesystem::path(db_path.string() + ".modelworld");
    if (std::filesystem::exists(world_path)) {
      require_status(world.load(world_path), "load ModelWorld");
    }

    std::map<std::string, uint32_t> ids;
    for (const auto& node : request.nodes) {
      uint32_t id = lookup_external_id(db, node.external_id);
      if (id == 0) {
        NodeInput input;
        input.content = node.statement;
        input.vector = anchor_vector();
        input.signature = signature_for(1, 1);
        input.incident = static_cast<uint32_t>(std::max(0, request.turn));
        input.root = node.type == "Hypothesis";
        input.symptom = node.type == "Decision" || node.type == "Outcome";
        input.impact = node.type == "Outcome";
        input.metadata["external_id"] = node.external_id;
        input.metadata["game_id"] = request.game_id;
        input.metadata["turn"] = std::to_string(request.turn);
        input.metadata["model_world_type"] = node.type;
        input.metadata["epistemic_origin"] = node.origin;
        require_status(db.put_node(input, &id), "put GrapheneDB node");
      }
      ids[node.external_id] = id;
    }

    std::map<std::string, std::vector<uint32_t>> incoming_evidence;
    for (const auto& relation : request.relations) {
      uint32_t from = ids.count(relation.from) ? ids[relation.from] : lookup_external_id(db, relation.from);
      uint32_t to = ids.count(relation.to) ? ids[relation.to] : lookup_external_id(db, relation.to);
      if (from == 0 || to == 0) {
        throw std::runtime_error("relation references unknown external id: " + relation.from + " -> " + relation.to);
      }
      EdgeInput edge;
      edge.from = from;
      edge.to = to;
      edge.origin = parse_origin(relation.origin);
      edge.role = parse_role(relation.role);
      edge.confidence = relation.confidence;
      edge.metadata["source_id"] = request.game_id + ":turn:" + std::to_string(request.turn);
      edge.metadata["from_external_id"] = relation.from;
      edge.metadata["to_external_id"] = relation.to;
      uint32_t edge_id = 0;
      require_status(db.put_edge(edge, &edge_id), "put GrapheneDB edge");
      incoming_evidence[relation.to].push_back(edge_id);
    }

    for (const auto& node : request.nodes) {
      if (lookup_world_external_id(world, node.external_id)) continue;
      ModelWorldNode world_node;
      world_node.type = parse_world_type(node.type);
      world_node.status = parse_world_status(node.status);
      world_node.statement = node.statement;
      world_node.origin = parse_origin(node.origin);
      world_node.evidence_edges = incoming_evidence[node.external_id];
      world_node.metadata["external_id"] = node.external_id;
      world_node.metadata["game_id"] = request.game_id;
      world_node.metadata["turn"] = std::to_string(request.turn);
      if (node.type == "Outcome" && world_node.origin == EdgeOrigin::Observed &&
          !world_node.evidence_edges.empty()) {
        world_node.status = ModelWorldStatus::Verified;
      }
      world.add(std::move(world_node), "AgentFabric A1.5 native ingest");
    }

    if (request.operation == "apply_outcome_and_reason") {
      for (const auto& relation : request.relations) {
        const auto target_world_id = lookup_world_external_id(world, relation.to);
        if (!target_world_id) continue;
        if (relation.role == "Contradicts") {
          world.update_status(*target_world_id, ModelWorldStatus::Contested,
                              "observed A1.5 outcome contradicts hypothesis");
        } else if (relation.role == "Supports") {
          const auto existing = world.get(*target_world_id);
          if (existing && existing->type == ModelWorldNodeType::Hypothesis &&
              existing->status == ModelWorldStatus::Proposed) {
            world.update_status(*target_world_id, ModelWorldStatus::Active,
                                "observed A1.5 outcome supports hypothesis");
          }
        }
      }
    }

    RuntimeOptions options;
    options.dialectic.mode = QueryMode::Balanced;
    options.dialectic.semantic_candidates = 12;
    options.dialectic.max_hops = 6;
    options.dialectic.max_paths = 32;
    options.dialectic.max_paths_per_root = 8;
    options.dialectic.minimum_confidence = 0.0;
    options.dialectic.max_opposition_rounds = 1;
    options.dialectic.reexpansion_threshold = 0.25;
    // A1.5 v3 intentionally does NOT copy request.reopen into options. The
    // DialecticEngine derives reopen_nodes from its own convergence/opposition.
    options.max_recursive_cycles = 2;
    options.enable_opposition_research = true;
    options.update_model_world = true;

    CompleteHypoKoshRuntime runtime(db, &world);
    const auto result = runtime.reason(anchor_vector(), signature_for(1, 1), options);

    const auto audit = world.audit();
    require_status(world.save(world_path), "save ModelWorld");
    std::string validation;
    require_status(db.validate(&validation), "validate GrapheneDB");

    const bool action_authorized = request.operation == "ingest_and_reason" &&
                                   !request.action.empty() &&
                                   result.receipt.governed_projection_executed &&
                                   result.receipt.no_silent_promotion;
    std::string regime = "insufficient_history";
    if (!result.lyapunov.observations.empty()) {
      regime = lyapunov_regime_name(result.lyapunov.observations.back().regime);
    }

    const auto primary_hypothesis =
        hypothesis_external_id_for_graph_node(db, world, result.primary_node);
    const auto reopened_hypotheses = reopened_hypothesis_external_ids(
        db, world, result.final_opposition.reopen_nodes);

    std::cout << '{';
    std::cout << "\"protocol\":\"agentfabric-a15-native-v3\",";
    std::cout << "\"operation\":\"" << json_escape(request.operation) << "\",";
    std::cout << "\"epistemic_status\":\"" << governed_status_name(result.status) << "\",";
    std::cout << "\"primary_node\":" << result.primary_node << ',';
    std::cout << "\"primary_hypothesis_id\":\""
              << json_escape(primary_hypothesis.value_or("")) << "\",";
    std::cout << "\"confidence\":" << std::setprecision(17) << result.confidence << ',';
    std::cout << "\"governed_action\":\"" << json_escape(action_authorized ? request.action : "") << "\",";
    emit_bool(std::cout, "action_authorized", action_authorized);
    std::cout << "\"action_policy\":\"exploratory experiment permitted only after native governed projection\",";
    std::cout << "\"lyapunov_regime\":\"" << json_escape(regime) << "\",";
    std::cout << "\"lyapunov_final_energy\":" << result.lyapunov.certificate.final_energy << ',';
    std::cout << "\"opposition_score\":" << result.final_opposition.opposition_score << ',';
    std::cout << "\"reopen_nodes\":[";
    for (size_t i = 0; i < result.final_opposition.reopen_nodes.size(); ++i) {
      if (i) std::cout << ',';
      std::cout << result.final_opposition.reopen_nodes[i];
    }
    std::cout << "],";
    std::cout << "\"reopened_hypothesis_ids\":";
    emit_string_array(std::cout, reopened_hypotheses);
    std::cout << ',';
    std::cout << "\"challenged_claims\":";
    emit_string_array(std::cout, result.final_opposition.challenged_claims);
    std::cout << ',';
    std::cout << "\"native_falsification_questions\":";
    emit_string_array(std::cout, result.final_opposition.falsification_questions);
    std::cout << ',';
    std::cout << "\"native_reopen_decision_source\":\"CompleteHypoKoshRuntime.final_opposition\",";
    emit_bool(std::cout, "legacy_reopen_seed_ignored", !request.reopen.empty());
    std::cout << "\"model_world_nodes\":" << world.nodes().size() << ',';
    std::cout << "\"model_world_events\":" << world.events().size() << ',';
    std::cout << "\"model_world_audit_findings\":" << audit.size() << ',';
    std::cout << "\"graphenedb_validation\":\"" << json_escape(validation) << "\",";
    std::cout << "\"reasoning_receipt\":{";
    emit_bool(std::cout, "graphene_executed", result.receipt.graphene_executed);
    emit_bool(std::cout, "fiber_bundle_built", result.receipt.fiber_bundle_built);
    emit_bool(std::cout, "fiber_bundle_authoritative", result.receipt.fiber_bundle_authoritative);
    emit_bool(std::cout, "stability_critic_executed", result.receipt.stability_critic_executed);
    emit_bool(std::cout, "epistemic_admissibility_executed", result.receipt.epistemic_admissibility_executed);
    emit_bool(std::cout, "lyapunov_trajectory_executed", result.receipt.lyapunov_trajectory_executed);
    emit_bool(std::cout, "lyapunov_certificate_valid", result.receipt.lyapunov_certificate_valid);
    emit_bool(std::cout, "lyapunov_goal_reached", result.receipt.lyapunov_goal_reached);
    emit_bool(std::cout, "escape_considered", result.receipt.escape_considered);
    emit_bool(std::cout, "convergence_executed", result.receipt.convergence_executed);
    emit_bool(std::cout, "opposition_executed", result.receipt.opposition_executed);
    emit_bool(std::cout, "governed_projection_executed", result.receipt.governed_projection_executed);
    emit_bool(std::cout, "no_silent_promotion", result.receipt.no_silent_promotion);
    std::cout << "\"final_bundle_hash\":" << result.receipt.final_bundle_hash << ',';
    std::cout << "\"model_world_event_hash\":" << world.event_log_hash();
    std::cout << "},";
    std::cout << "\"residual_uncertainty\":[";
    for (size_t i = 0; i < result.residual_uncertainty.size(); ++i) {
      if (i) std::cout << ',';
      std::cout << '"' << json_escape(result.residual_uncertainty[i]) << '"';
    }
    std::cout << "]}" << std::endl;

    require_status(db.close(), "close GrapheneDB");
    return 0;
  } catch (const std::exception& error) {
    std::cerr << "a15_native_runtime_helper: " << error.what() << '\n';
    return 2;
  }
}
