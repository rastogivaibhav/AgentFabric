#include "graphene/model_world.hpp"

#include <filesystem>
#include <iomanip>
#include <iostream>
#include <sstream>
#include <string>

using namespace graphene;

namespace {
std::string esc(const std::string& s) {
  std::ostringstream out;
  for (unsigned char c : s) {
    switch (c) {
      case '\\': out << "\\\\"; break;
      case '"': out << "\\\""; break;
      case '\n': out << "\\n"; break;
      case '\r': out << "\\r"; break;
      case '\t': out << "\\t"; break;
      default:
        if (c < 0x20) out << "\\u" << std::hex << std::setw(4) << std::setfill('0') << static_cast<int>(c) << std::dec;
        else out << static_cast<char>(c);
    }
  }
  return out.str();
}

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

bool useful(ModelWorldNodeType t) {
  return t == ModelWorldNodeType::Hypothesis || t == ModelWorldNodeType::Decision ||
         t == ModelWorldNodeType::Opposition || t == ModelWorldNodeType::Experiment ||
         t == ModelWorldNodeType::Outcome;
}
}

int main(int argc, char** argv) {
  if (argc != 2) {
    std::cerr << "usage: a15_model_world_dump MODELWORLD_PATH\n";
    return 2;
  }
  ModelWorld world;
  const Status loaded = world.load(std::filesystem::path(argv[1]));
  if (!loaded) {
    std::cerr << loaded.message << "\n";
    return 3;
  }
  std::cout << "{\"event_log_hash\":" << world.event_log_hash() << ",\"nodes\":[";
  bool first = true;
  for (const auto& node : world.nodes()) {
    if (!useful(node.type)) continue;
    if (!first) std::cout << ',';
    first = false;
    std::string external_id;
    auto it = node.metadata.find("external_id");
    if (it != node.metadata.end()) external_id = it->second;
    std::cout << "{\"id\":" << node.id
              << ",\"external_id\":\"" << esc(external_id) << "\""
              << ",\"type\":\"" << type_name(node.type) << "\""
              << ",\"status\":\"" << status_name(node.status) << "\""
              << ",\"statement\":\"" << esc(node.statement) << "\"}";
  }
  std::cout << "]}" << std::endl;
  return 0;
}
