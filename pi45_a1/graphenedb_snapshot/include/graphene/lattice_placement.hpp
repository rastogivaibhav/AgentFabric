#pragma once
#include "graphene/types.hpp"

namespace graphene {

enum class PlacementStrategy : uint8_t {
  InputOrder = 0,
  SemanticGroups = 1
};

struct LatticePlacementOptions {
  uint32_t base_node_id{0};
  int32_t layer{0};
  uint32_t incident{0};
  uint64_t signature{0};
  PlacementStrategy strategy{PlacementStrategy::InputOrder};
  BondType default_bond{BondType::Sigma};
  double default_bond_strength{0.85};
};

struct LatticePlacementResult {
  BatchInput batch;
};

LatticeCoord hex_spiral_coord(uint32_t index, int32_t layer = 0);
Status assign_lattice_batch(std::vector<NodeInput> nodes, const LatticePlacementOptions& options, LatticePlacementResult* out);

} // namespace graphene
