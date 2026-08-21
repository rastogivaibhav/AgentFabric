#include "graphene/model_world.hpp"
#include "graphene/platform.hpp"

#include <algorithm>
#include <fstream>
#include <iomanip>
#include <sstream>

namespace graphene {
namespace {

uint64_t fnv1a(uint64_t hash, const std::string& value) {
  for (unsigned char ch : value) {
    hash ^= ch;
    hash *= 1099511628211ULL;
  }
  return hash;
}

uint64_t event_checksum(const ModelWorldEvent& event) {
  uint64_t hash = 1469598103934665603ULL;
  hash = fnv1a(hash, std::to_string(event.sequence));
  hash = fnv1a(hash, std::to_string(event.node_id));
  hash = fnv1a(hash, event.action);
  hash = fnv1a(hash, std::to_string(static_cast<int>(event.before)));
  hash = fnv1a(hash, std::to_string(static_cast<int>(event.after)));
  hash = fnv1a(hash, event.reason);
  return hash;
}

}  // namespace

uint64_t ModelWorld::add(ModelWorldNode node, const std::string& reason) {
  node.id = next_id_++;
  nodes_.push_back(node);
  ModelWorldEvent event;
  event.sequence = events_.size() + 1;
  event.node_id = node.id;
  event.action = "add";
  event.before = ModelWorldStatus::Proposed;
  event.after = node.status;
  event.reason = reason;
  event.checksum = event_checksum(event);
  events_.push_back(std::move(event));
  return node.id;
}

bool ModelWorld::update_status(uint64_t node_id, ModelWorldStatus status,
                               const std::string& reason) {
  auto it = std::find_if(nodes_.begin(), nodes_.end(),
                         [&](const ModelWorldNode& node) { return node.id == node_id; });
  if (it == nodes_.end()) return false;
  const ModelWorldStatus before = it->status;
  // Hypothetical or inferred model-world nodes cannot become verified without
  // evidence edges and an explicit external/discovered origin.
  if (status == ModelWorldStatus::Verified &&
      (it->origin == EdgeOrigin::Hypothetical || it->origin == EdgeOrigin::Inferred ||
       it->evidence_edges.empty())) {
    return false;
  }
  it->status = status;
  ModelWorldEvent event;
  event.sequence = events_.size() + 1;
  event.node_id = node_id;
  event.action = "status";
  event.before = before;
  event.after = status;
  event.reason = reason;
  event.checksum = event_checksum(event);
  events_.push_back(std::move(event));
  return true;
}

std::optional<ModelWorldNode> ModelWorld::get(uint64_t node_id) const {
  const auto it = std::find_if(nodes_.begin(), nodes_.end(),
                               [&](const ModelWorldNode& node) { return node.id == node_id; });
  if (it == nodes_.end()) return std::nullopt;
  return *it;
}

std::vector<ModelWorldAuditFinding> ModelWorld::audit() const {
  std::vector<ModelWorldAuditFinding> findings;
  for (const auto& node : nodes_) {
    if (node.status == ModelWorldStatus::Verified && node.evidence_edges.empty()) {
      findings.push_back({node.id, "VERIFIED_WITHOUT_EVIDENCE",
                          "verified model-world nodes require source evidence"});
    }
    if ((node.origin == EdgeOrigin::Hypothetical || node.origin == EdgeOrigin::Inferred) &&
        node.status == ModelWorldStatus::Verified) {
      findings.push_back({node.id, "SILENT_TRUTH_PROMOTION",
                          "hypothetical or inferred nodes cannot be silently verified"});
    }
    if (node.type == ModelWorldNodeType::Hypothesis &&
        node.status == ModelWorldStatus::Active && node.evidence_edges.empty()) {
      findings.push_back({node.id, "ACTIVE_HYPOTHESIS_WITHOUT_EVIDENCE",
                          "active hypotheses should retain their supporting evidence edges"});
    }
  }
  for (const auto& event : events_) {
    if (event.checksum != event_checksum(event)) {
      findings.push_back({event.node_id, "EVENT_CHECKSUM_MISMATCH",
                          "the append-only model-world event log is corrupted"});
    }
  }
  return findings;
}

uint64_t ModelWorld::event_log_hash() const {
  uint64_t hash = 1469598103934665603ULL;
  for (const auto& event : events_) hash = fnv1a(hash, std::to_string(event.checksum));
  return hash;
}

Status ModelWorld::save(const std::filesystem::path& path) const {
  std::error_code ec;
  if (!path.parent_path().empty()) std::filesystem::create_directories(path.parent_path(), ec);
  if (ec) return Status::error(ErrorCode::IoError, "cannot create model-world directory: " + ec.message());
  const std::filesystem::path temporary = path.string() + ".tmp";
  std::ofstream output(temporary, std::ios::trunc);
  if (!output) return Status::error(ErrorCode::IoError, "cannot write model-world snapshot");
  output << "MWV1\n";
  for (const auto& node : nodes_) {
    output << "NODE " << node.id << ' ' << static_cast<int>(node.type) << ' '
           << static_cast<int>(node.status) << ' ' << static_cast<int>(node.origin) << ' '
           << std::quoted(node.statement) << ' ' << node.evidence_edges.size();
    for (uint32_t edge : node.evidence_edges) output << ' ' << edge;
    output << ' ' << node.related_nodes.size();
    for (uint64_t related : node.related_nodes) output << ' ' << related;
    output << ' ' << node.metadata.size();
    for (const auto& [key, value] : node.metadata) output << ' ' << std::quoted(key) << ' ' << std::quoted(value);
    output << '\n';
  }
  for (const auto& event : events_) {
    output << "EVENT " << event.sequence << ' ' << event.node_id << ' '
           << std::quoted(event.action) << ' ' << static_cast<int>(event.before) << ' '
           << static_cast<int>(event.after) << ' ' << std::quoted(event.reason) << ' '
           << event.checksum << '\n';
  }
  output.flush();
  if (!output) return Status::error(ErrorCode::IoError, "failed flushing model-world snapshot");
  output.close();
  return platform::durable_replace(temporary, path);
}

Status ModelWorld::load(const std::filesystem::path& path) {
  std::ifstream input(path);
  if (!input) return Status::error(ErrorCode::IoError, "cannot read model-world snapshot");
  std::string header;
  std::getline(input, header);
  if (header != "MWV1") return Status::error(ErrorCode::DataCorrupt, "unsupported model-world format");
  std::vector<ModelWorldNode> nodes;
  std::vector<ModelWorldEvent> events;
  uint64_t max_id = 0;
  std::string line;
  while (std::getline(input, line)) {
    if (line.empty()) continue;
    std::istringstream row(line);
    std::string kind;
    row >> kind;
    if (kind == "NODE") {
      ModelWorldNode node;
      int type = 0, status = 0, origin = 0;
      size_t evidence_count = 0, related_count = 0, metadata_count = 0;
      if (!(row >> node.id >> type >> status >> origin >> std::quoted(node.statement) >> evidence_count))
        return Status::error(ErrorCode::DataCorrupt, "invalid model-world node record");
      node.type = static_cast<ModelWorldNodeType>(type);
      node.status = static_cast<ModelWorldStatus>(status);
      node.origin = static_cast<EdgeOrigin>(origin);
      for (size_t i = 0; i < evidence_count; ++i) { uint32_t value = 0; if (!(row >> value)) return Status::error(ErrorCode::DataCorrupt, "invalid evidence edge"); node.evidence_edges.push_back(value); }
      if (!(row >> related_count)) return Status::error(ErrorCode::DataCorrupt, "invalid related-node count");
      for (size_t i = 0; i < related_count; ++i) { uint64_t value = 0; if (!(row >> value)) return Status::error(ErrorCode::DataCorrupt, "invalid related node"); node.related_nodes.push_back(value); }
      if (!(row >> metadata_count)) return Status::error(ErrorCode::DataCorrupt, "invalid metadata count");
      for (size_t i = 0; i < metadata_count; ++i) { std::string key, value; if (!(row >> std::quoted(key) >> std::quoted(value))) return Status::error(ErrorCode::DataCorrupt, "invalid metadata"); node.metadata[key] = value; }
      max_id = std::max(max_id, node.id);
      nodes.push_back(std::move(node));
    } else if (kind == "EVENT") {
      ModelWorldEvent event;
      int before = 0, after = 0;
      if (!(row >> event.sequence >> event.node_id >> std::quoted(event.action) >> before >> after >> std::quoted(event.reason) >> event.checksum))
        return Status::error(ErrorCode::DataCorrupt, "invalid model-world event record");
      event.before = static_cast<ModelWorldStatus>(before);
      event.after = static_cast<ModelWorldStatus>(after);
      if (event.checksum != event_checksum(event)) return Status::error(ErrorCode::DataCorrupt, "model-world event checksum mismatch");
      events.push_back(std::move(event));
    } else {
      return Status::error(ErrorCode::DataCorrupt, "unknown model-world record");
    }
  }
  if (!input.eof()) return Status::error(ErrorCode::IoError, "failed reading model-world snapshot");
  nodes_ = std::move(nodes);
  events_ = std::move(events);
  next_id_ = max_id + 1;
  return Status::ok();
}

}  // namespace graphene

namespace graphene {

ModelWorldSchedulerReport ModelWorldScheduler::run(const ModelWorld& world,
                                                   size_t max_findings) const {
  ModelWorldSchedulerReport report;
  report.jobs_run = {
      ModelWorldAuditJob::ContradictionAudit,
      ModelWorldAuditJob::HypothesisReview,
      ModelWorldAuditJob::FalsePromotionAudit,
      ModelWorldAuditJob::EvidenceIntegrityAudit,
      ModelWorldAuditJob::EventLogIntegrityAudit};
  const auto findings = world.audit();
  const size_t limit = std::min(max_findings, findings.size());
  report.findings.assign(findings.begin(), findings.begin() + limit);
  report.bounded = findings.size() <= max_findings;
  return report;
}

}  // namespace graphene
