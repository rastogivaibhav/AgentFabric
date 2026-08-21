#include "graphene/epistemic.hpp"

#include <algorithm>
#include <cctype>
#include <limits>

namespace graphene {
namespace {

bool digits(const std::string& text, size_t offset, size_t length, int* out) {
  if (offset + length > text.size()) return false;
  int value = 0;
  for (size_t i = 0; i < length; ++i) {
    const unsigned char ch = static_cast<unsigned char>(text[offset + i]);
    if (!std::isdigit(ch)) return false;
    value = value * 10 + (ch - '0');
  }
  *out = value;
  return true;
}

bool leap_year(int year) {
  return year % 4 == 0 && (year % 100 != 0 || year % 400 == 0);
}

int days_in_month(int year, int month) {
  static constexpr int kDays[] = {
      31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31};
  if (month == 2 && leap_year(year)) return 29;
  return kDays[month - 1];
}

int64_t days_from_civil(int year, unsigned month, unsigned day) {
  year -= month <= 2;
  const int era = (year >= 0 ? year : year - 399) / 400;
  const unsigned yoe = static_cast<unsigned>(year - era * 400);
  const unsigned adjusted_month = month > 2 ? month - 3 : month + 9;
  const unsigned doy = (153 * adjusted_month + 2) / 5 + day - 1;
  const unsigned doe = yoe * 365 + yoe / 4 - yoe / 100 + doy;
  return static_cast<int64_t>(era) * 146097 + static_cast<int64_t>(doe) - 719468;
}

int compare(const Rfc3339Instant& left, const Rfc3339Instant& right) {
  if (left.unix_seconds != right.unix_seconds) return left.unix_seconds < right.unix_seconds ? -1 : 1;
  if (left.nanosecond != right.nanosecond) return left.nanosecond < right.nanosecond ? -1 : 1;
  return 0;
}

std::string metadata_value(const std::map<std::string, std::string>& metadata,
                           const std::string& key) {
  const auto it = metadata.find(key);
  return it == metadata.end() ? std::string{} : it->second;
}

std::string first_metadata_value(
    const std::map<std::string, std::string>& metadata,
    const std::initializer_list<const char*>& keys) {
  for (const char* key : keys) {
    const std::string value = metadata_value(metadata, key);
    if (!value.empty()) return value;
  }
  return {};
}

} // namespace

Status parse_rfc3339(const std::string& text, Rfc3339Instant* out) {
  if (!out) return Status::error(ErrorCode::InvalidInput, "RFC3339 output pointer is null");
  if (text.size() < 20 || text[4] != '-' || text[7] != '-' ||
      (text[10] != 'T' && text[10] != 't') || text[13] != ':' ||
      text[16] != ':') {
    return Status::error(ErrorCode::InvalidInput, "timestamp is not RFC3339");
  }

  int year = 0, month = 0, day = 0, hour = 0, minute = 0, second = 0;
  if (!digits(text, 0, 4, &year) || !digits(text, 5, 2, &month) ||
      !digits(text, 8, 2, &day) || !digits(text, 11, 2, &hour) ||
      !digits(text, 14, 2, &minute) || !digits(text, 17, 2, &second) ||
      year == 0 || month < 1 || month > 12 || day < 1 ||
      day > days_in_month(year, month) || hour > 23 || minute > 59 || second > 59) {
    return Status::error(ErrorCode::InvalidInput, "timestamp contains an invalid date or time");
  }

  size_t cursor = 19;
  uint32_t nanosecond = 0;
  if (cursor < text.size() && text[cursor] == '.') {
    ++cursor;
    const size_t fraction_start = cursor;
    size_t fraction_digits = 0;
    while (cursor < text.size() && std::isdigit(static_cast<unsigned char>(text[cursor]))) {
      if (fraction_digits >= 9) return Status::error(ErrorCode::InvalidInput, "timestamp precision exceeds nanoseconds");
      nanosecond = nanosecond * 10 + static_cast<uint32_t>(text[cursor] - '0');
      ++cursor;
      ++fraction_digits;
    }
    if (cursor == fraction_start) return Status::error(ErrorCode::InvalidInput, "timestamp has an empty fractional second");
    for (; fraction_digits < 9; ++fraction_digits) nanosecond *= 10;
  }

  int offset_seconds = 0;
  if (cursor < text.size() && (text[cursor] == 'Z' || text[cursor] == 'z')) {
    ++cursor;
  } else if (cursor + 6 == text.size() && (text[cursor] == '+' || text[cursor] == '-') && text[cursor + 3] == ':') {
    int offset_hour = 0, offset_minute = 0;
    if (!digits(text, cursor + 1, 2, &offset_hour) || !digits(text, cursor + 4, 2, &offset_minute) ||
        offset_hour > 23 || offset_minute > 59) {
      return Status::error(ErrorCode::InvalidInput, "timestamp contains an invalid UTC offset");
    }
    offset_seconds = offset_hour * 3600 + offset_minute * 60;
    if (text[cursor] == '-') offset_seconds = -offset_seconds;
    cursor += 6;
  } else {
    return Status::error(ErrorCode::InvalidInput, "timestamp must include Z or a UTC offset");
  }
  if (cursor != text.size()) return Status::error(ErrorCode::InvalidInput, "timestamp has trailing characters");

  const int64_t local_seconds =
      days_from_civil(year, static_cast<unsigned>(month), static_cast<unsigned>(day)) * 86400 +
      hour * 3600 + minute * 60 + second;
  out->unix_seconds = local_seconds - offset_seconds;
  out->nanosecond = nanosecond;
  return Status::ok();
}

Status parse_temporal_validity(const std::map<std::string, std::string>& metadata,
                               TemporalValidity* out) {
  if (!out) return Status::error(ErrorCode::InvalidInput, "temporal validity output pointer is null");
  TemporalValidity parsed;
  const std::string from = metadata_value(metadata, "valid_from");
  const std::string until = metadata_value(metadata, "valid_until");
  if (!from.empty()) {
    Rfc3339Instant instant;
    const Status status = parse_rfc3339(from, &instant);
    if (!status) return Status::error(ErrorCode::InvalidInput, "invalid valid_from: " + status.message);
    parsed.valid_from = instant;
  }
  if (!until.empty()) {
    Rfc3339Instant instant;
    const Status status = parse_rfc3339(until, &instant);
    if (!status) return Status::error(ErrorCode::InvalidInput, "invalid valid_until: " + status.message);
    parsed.valid_until = instant;
  }
  if (parsed.valid_from && parsed.valid_until && compare(*parsed.valid_from, *parsed.valid_until) > 0)
    return Status::error(ErrorCode::InvalidInput, "valid_from is later than valid_until");
  *out = parsed;
  return Status::ok();
}

bool valid_at(const TemporalValidity& validity, const Rfc3339Instant& instant) {
  if (validity.valid_from && compare(instant, *validity.valid_from) < 0) return false;
  if (validity.valid_until && compare(instant, *validity.valid_until) > 0) return false;
  return true;
}

EdgeProvenance assess_edge_provenance(const Edge& edge) {
  EdgeProvenance result;
  EvidenceRef evidence;
  evidence.source_id = first_metadata_value(
      edge.metadata,
      {"source_id", "source", "graphene_source_id", "graphene_source_uri",
       "evidence_ref", "graphene_evidence_id", "graphene_evidence_uri"});
  evidence.span = first_metadata_value(edge.metadata, {"span", "graphene_evidence_text"});
  evidence.observed_at = metadata_value(edge.metadata, "observed_at");
  evidence.evidence_family_id = first_metadata_value(edge.metadata, {"evidence_family_id", "graphene_evidence_family_id"});
  evidence.derivation_id = first_metadata_value(edge.metadata, {"derivation_id", "derived_from"});
  evidence.content_hash = first_metadata_value(edge.metadata, {"evidence_content_hash", "graphene_evidence_hash", "content_hash"});
  if (!evidence.source_id.empty()) result.evidence.push_back(evidence);

  if ((edge.origin == EdgeOrigin::Observed || edge.origin == EdgeOrigin::Discovered) && evidence.source_id.empty())
    result.findings.push_back({edge.id, "MISSING_EVIDENCE", "observed/discovered edge has no source evidence"});
  if (!evidence.observed_at.empty()) {
    Rfc3339Instant ignored;
    const Status status = parse_rfc3339(evidence.observed_at, &ignored);
    if (!status) result.findings.push_back({edge.id, "INVALID_OBSERVED_AT", status.message});
  }
  if (edge.origin == EdgeOrigin::Inferred && evidence.derivation_id.empty())
    result.findings.push_back({edge.id, "INFERRED_WITHOUT_DERIVATION", "inferred edge has no derived_from reference"});
  if (edge.origin == EdgeOrigin::Reinforced && metadata_value(edge.metadata, "promotion_status") == "discovered")
    result.findings.push_back({edge.id, "REINFORCED_TRUTH_PROMOTION", "reinforcement cannot promote an edge to discovered truth"});
  if (edge.role == EdgeRole::Compressed && evidence.derivation_id.empty())
    result.findings.push_back({edge.id, "COMPRESSED_WITHOUT_MECHANISM", "compressed shortcut has no mechanistic derivation reference"});
  TemporalValidity validity;
  const Status temporal_status = parse_temporal_validity(edge.metadata, &validity);
  if (!temporal_status) result.findings.push_back({edge.id, "INVALID_TEMPORAL_METADATA", temporal_status.message});
  return result;
}

} // namespace graphene
