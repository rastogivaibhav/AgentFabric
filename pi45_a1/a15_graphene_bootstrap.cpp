#include "graphene/db.hpp"

#include <filesystem>
#include <iostream>
#include <string>
#include <vector>

using namespace graphene;

int main(int argc, char** argv) {
  if (argc != 2) {
    std::cerr << "usage: a15_graphene_bootstrap DB_PATH\n";
    return 2;
  }
  DBOptions options;
  options.dimension = 64;
  options.create_if_missing = true;
  options.fsync_on_commit = true;

  GrapheneDB db;
  const Status opened = db.open(std::filesystem::path(argv[1]), options);
  if (!opened) {
    std::cerr << opened.message << "\n";
    return 3;
  }

  if (!db.metadata_search("a15_internal", "zero_id_sentinel").empty()) {
    std::cout << "a15_zero_id_sentinel=EXISTS\n";
    return 0;
  }

  NodeInput node;
  node.content = "A1.5 internal zero-id reservation; excluded from ModelWorld.";
  node.vector = std::vector<float>(64, 0.0f);
  node.signature = (uint64_t{1} << 63);
  node.root = false;
  node.symptom = false;
  node.impact = false;
  node.metadata["a15_internal"] = "zero_id_sentinel";
  node.metadata["epistemic_excluded"] = "true";

  uint32_t id = 999;
  const Status inserted = db.put_node(node, &id);
  if (!inserted) {
    std::cerr << inserted.message << "\n";
    return 4;
  }
  if (id != 0) {
    std::cerr << "expected first reserved GrapheneDB id 0, got " << id << "\n";
    return 5;
  }
  std::cout << "a15_zero_id_sentinel=PASS id=0\n";
  return 0;
}
