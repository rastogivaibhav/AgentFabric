#include "graphene/db.hpp"

#include <filesystem>
#include <iostream>
#include <string>
#include <vector>

using namespace graphene;

int main(int argc, char** argv) {
  if (argc != 2) {
    std::cerr << "usage: a15_native_runtime_bootstrap DB_PATH\n";
    return 2;
  }
  GrapheneDB db;
  DBOptions options;
  options.dimension = 64;
  options.create_if_missing = true;
  options.fsync_on_commit = true;
  const Status opened = db.open(std::filesystem::path(argv[1]), options);
  if (!opened) {
    std::cerr << opened.message << '\n';
    return 2;
  }
  const auto existing = db.metadata_search("a15_internal_sentinel", "true");
  if (existing.empty()) {
    NodeInput node;
    node.content = "A1.5 internal zero-id reservation; excluded from epistemic reasoning.";
    node.vector = std::vector<float>(64, 0.0f);
    node.signature = signature_for(15, 15);
    node.metadata["a15_internal_sentinel"] = "true";
    node.metadata["reasoning_excluded"] = "true";
    uint32_t id = 999;
    const Status written = db.put_node(node, &id);
    if (!written) {
      std::cerr << written.message << '\n';
      return 2;
    }
    if (id != 0) {
      std::cerr << "fresh A1.5 DB did not allocate reserved sentinel at id 0; got " << id << '\n';
      return 2;
    }
  } else if (existing.front() != 0) {
    std::cerr << "A1.5 sentinel exists but is not node 0\n";
    return 2;
  }
  std::string report;
  const Status valid = db.validate(&report);
  if (!valid) {
    std::cerr << valid.message << '\n';
    return 2;
  }
  std::cout << "a15_zero_id_reserved=true\n";
  std::cout << "graphenedb_validation=" << report << '\n';
  db.close();
  return 0;
}
