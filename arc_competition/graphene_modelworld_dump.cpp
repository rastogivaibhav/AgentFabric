#include "graphene/model_world.hpp"

#include <filesystem>
#include <iostream>
#include <string>

using namespace graphene;

namespace {
const char* type_name(ModelWorldNodeType t) {
  switch (t) {
    case ModelWorldNodeType::Fact: return "Fact";
    case ModelWorldNodeType::Concept: return "Concept";
    case ModelWorldNodeType::Hypothesis: return "Hypothesis";
    case ModelWorldNodeType::Contradiction: return "Contradiction";
    case ModelWorldNodeType::Abstraction: return "Abstraction";
    case ModelWorldNodeType::Opposition: return "Opposition";
    case ModelWorldNodeType::Decision: return "Decision";
    case ModelWorldNodeType::Experiment: return "Experiment";
    case ModelWorldNodeType::Implementation: return "Implementation";
    case ModelWorldNodeType::Outcome: return "Outcome";
    case ModelWorldNodeType::Failure: return "Failure";
    case ModelWorldNodeType::Reinforcement: return "Reinforcement";
    case ModelWorldNodeType::Model: return "Model";
  }
  return "Unknown";
}
const char* status_name(ModelWorldStatus s) {
  switch (s) {
    case ModelWorldStatus::Proposed: return "Proposed";
    case ModelWorldStatus::Active: return "Active";
    case ModelWorldStatus::Contested: return "Contested";
    case ModelWorldStatus::Verified: return "Verified";
    case ModelWorldStatus::Rejected: return "Rejected";
    case ModelWorldStatus::Superseded: return "Superseded";
  }
  return "Unknown";
}
std::string esc(const std::string& s) {
  std::string o;
  for (char c : s) {
    switch (c) {
      case '\\': o += "\\\\"; break;
      case '"': o += "\\\""; break;
      case '\n': o += "\\n"; break;
      case '\r': o += "\\r"; break;
      case '\t': o += "\\t"; break;
      default: o += c;
    }
  }
  return o;
}
}

int main(int argc, char** argv) {
  if (argc != 2) {
    std::cerr << "usage: graphene_modelworld_dump MODELWORLD_PATH\n";
    return 2;
  }
  ModelWorld world;
  auto st = world.load(std::filesystem::path(argv[1]));
  if (!st) {
    std::cerr << "load failed: " << st.message << "\n";
    return 2;
  }
  const auto findings = world.audit();
  std::cout << "{\n  \"event_log_hash\": " << world.event_log_hash() << ",\n";
  std::cout << "  \"audit_findings\": " << findings.size() << ",\n";
  std::cout << "  \"nodes\": [\n";
  const auto& nodes = world.nodes();
  for (size_t i = 0; i < nodes.size(); ++i) {
    const auto& n = nodes[i];
    std::string external;
    auto it = n.metadata.find("external_id");
    if (it != n.metadata.end()) external = it->second;
    std::cout << "    {\"id\":" << n.id
              << ",\"external_id\":\"" << esc(external)
              << "\",\"type\":\"" << type_name(n.type)
              << "\",\"status\":\"" << status_name(n.status)
              << "\",\"statement\":\"" << esc(n.statement)
              << "\",\"evidence_edge_count\":" << n.evidence_edges.size()
              << ",\"related_node_count\":" << n.related_nodes.size() << "}";
    if (i + 1 < nodes.size()) std::cout << ',';
    std::cout << '\n';
  }
  std::cout << "  ],\n  \"events\": [\n";
  const auto& events = world.events();
  for (size_t i = 0; i < events.size(); ++i) {
    const auto& e = events[i];
    std::cout << "    {\"sequence\":" << e.sequence
              << ",\"node_id\":" << e.node_id
              << ",\"action\":\"" << esc(e.action)
              << "\",\"before\":\"" << status_name(e.before)
              << "\",\"after\":\"" << status_name(e.after)
              << "\",\"reason\":\"" << esc(e.reason)
              << "\",\"checksum\":" << e.checksum << "}";
    if (i + 1 < events.size()) std::cout << ',';
    std::cout << '\n';
  }
  std::cout << "  ]\n}\n";
  return 0;
}
