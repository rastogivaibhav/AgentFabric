#pragma once

#include "graphene/fiber_bundle.hpp"

#include <cstddef>
#include <cstdint>
#include <filesystem>
#include <map>
#include <optional>
#include <string>
#include <vector>

namespace graphene {

enum class ModelWorldNodeType : uint8_t {
  Fact,
  Concept,
  Hypothesis,
  Contradiction,
  Abstraction,
  Opposition,
  Decision,
  Experiment,
  Implementation,
  Outcome,
  Failure,
  Reinforcement,
  Model
};

enum class ModelWorldStatus : uint8_t {
  Proposed,
  Active,
  Contested,
  Verified,
  Rejected,
  Superseded
};

struct ModelWorldNode {
  uint64_t id{0};
  ModelWorldNodeType type{ModelWorldNodeType::Fact};
  ModelWorldStatus status{ModelWorldStatus::Proposed};
  std::string statement;
  EdgeOrigin origin{EdgeOrigin::Hypothetical};
  std::vector<uint32_t> evidence_edges;
  std::vector<uint64_t> related_nodes;
  std::map<std::string, std::string> metadata;
};

struct ModelWorldEvent {
  uint64_t sequence{0};
  uint64_t node_id{0};
  std::string action;
  ModelWorldStatus before{ModelWorldStatus::Proposed};
  ModelWorldStatus after{ModelWorldStatus::Proposed};
  std::string reason;
  uint64_t checksum{0};
};

struct ModelWorldAuditFinding {
  uint64_t node_id{0};
  std::string code;
  std::string detail;
};

class ModelWorld {
 public:
  uint64_t add(ModelWorldNode node, const std::string& reason);
  bool update_status(uint64_t node_id, ModelWorldStatus status,
                     const std::string& reason);
  std::optional<ModelWorldNode> get(uint64_t node_id) const;
  const std::vector<ModelWorldNode>& nodes() const { return nodes_; }
  const std::vector<ModelWorldEvent>& events() const { return events_; }
  std::vector<ModelWorldAuditFinding> audit() const;
  uint64_t event_log_hash() const;
  Status save(const std::filesystem::path& path) const;
  Status load(const std::filesystem::path& path);

 private:
  std::vector<ModelWorldNode> nodes_;
  std::vector<ModelWorldEvent> events_;
  uint64_t next_id_{1};
};

enum class ModelWorldAuditJob : uint8_t {
  ContradictionAudit,
  HypothesisReview,
  FalsePromotionAudit,
  EvidenceIntegrityAudit,
  EventLogIntegrityAudit
};

struct ModelWorldSchedulerReport {
  std::vector<ModelWorldAuditJob> jobs_run;
  std::vector<ModelWorldAuditFinding> findings;
  bool bounded{true};
};

class ModelWorldScheduler {
 public:
  ModelWorldSchedulerReport run(const ModelWorld& world, size_t max_findings = 256) const;
};

}  // namespace graphene
