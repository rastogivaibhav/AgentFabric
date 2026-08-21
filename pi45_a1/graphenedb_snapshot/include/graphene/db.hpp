#pragma once
#include "graphene/types.hpp"
#include <filesystem>
#include <memory>
#include <shared_mutex>

namespace graphene {

class GrapheneDB {
public:
  GrapheneDB();
  ~GrapheneDB();
  GrapheneDB(const GrapheneDB&) = delete;
  GrapheneDB& operator=(const GrapheneDB&) = delete;

  Status open(const std::filesystem::path& path, const DBOptions& options);
  Status close();
  bool is_open() const;

  Status put_node(const NodeInput& input, uint32_t* out_id = nullptr);
  Status put_edge(const EdgeInput& input, uint32_t* out_id = nullptr);
  Status put_batch(const BatchInput& input, BatchResult* out = nullptr);
  Status put_extraction(const ExtractionInput& input, ExtractionResult* out = nullptr);
  Status delete_node(uint32_t id);

  uint64_t snapshot() const;
  uint32_t dimension() const;
  size_t node_count(uint64_t snapshot_version = kInfVersion) const;
  size_t edge_count(uint64_t snapshot_version = kInfVersion) const;

  std::vector<SearchResult> vector_search(const std::vector<float>& query, size_t k, uint64_t snapshot_version = kInfVersion) const;
  MemoryBundle causal_search(const std::vector<float>& query, uint64_t query_signature, QueryMode mode = QueryMode::Empirical, uint64_t snapshot_version = kInfVersion) const;
  std::vector<uint32_t> metadata_search(const std::string& key, const std::string& value, uint64_t snapshot_version = kInfVersion) const;
  std::vector<uint32_t> lattice_neighbors(uint32_t node_id, uint32_t max_hops = 1, uint64_t snapshot_version = kInfVersion) const;
  std::vector<Edge> incoming_edges(uint32_t node_id, uint64_t snapshot_version = kInfVersion) const;
  std::vector<Edge> outgoing_edges(uint32_t node_id, uint64_t snapshot_version = kInfVersion) const;

  std::optional<Node> get_node(uint32_t id, uint64_t snapshot_version = kInfVersion) const;
  std::optional<Edge> get_edge(uint32_t id, uint64_t snapshot_version = kInfVersion) const;

  Status compact();
  Status backup(const std::filesystem::path& destination_dir) const;
  Status inspect(std::string* out) const;
  Status validate(std::string* out_report = nullptr) const;

private:
  struct Impl;
  std::unique_ptr<Impl> impl_;
};

} // namespace graphene
