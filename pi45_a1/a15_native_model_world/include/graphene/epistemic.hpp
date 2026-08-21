#pragma once

#include "graphene/types.hpp"

#include <cstdint>
#include <map>
#include <optional>
#include <string>
#include <utility>
#include <vector>

namespace graphene {

struct EvidenceRef {
  EvidenceRef() = default;
  EvidenceRef(std::string source,
              std::string evidence_span = {},
              std::string observation_time = {},
              std::string family = {},
              std::string derivation = {},
              std::string hash = {})
      : source_id(std::move(source)),
        span(std::move(evidence_span)),
        observed_at(std::move(observation_time)),
        evidence_family_id(std::move(family)),
        derivation_id(std::move(derivation)),
        content_hash(std::move(hash)) {}

  std::string source_id;
  std::string span;
  std::string observed_at;

  // Optional v2 lineage identities. Older callers can continue supplying only
  // source_id/span/observed_at. When evidence_family_id is absent the bundle
  // builder conservatively falls back to source_id.
  std::string evidence_family_id;
  std::string derivation_id;
  std::string content_hash;
};

struct ProvenanceFinding {
  uint32_t edge_id{0};
  std::string code;
  std::string detail;
};

struct Rfc3339Instant {
  int64_t unix_seconds{0};
  uint32_t nanosecond{0};
};

struct TemporalValidity {
  std::optional<Rfc3339Instant> valid_from;
  std::optional<Rfc3339Instant> valid_until;
};

struct EdgeProvenance {
  std::vector<EvidenceRef> evidence;
  std::vector<ProvenanceFinding> findings;
};

Status parse_rfc3339(const std::string& text, Rfc3339Instant* out);
Status parse_temporal_validity(
    const std::map<std::string, std::string>& metadata,
    TemporalValidity* out);
bool valid_at(const TemporalValidity& validity, const Rfc3339Instant& instant);
EdgeProvenance assess_edge_provenance(const Edge& edge);

} // namespace graphene
