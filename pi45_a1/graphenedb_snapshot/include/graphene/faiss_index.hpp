#pragma once

#include "graphene/vector_index.hpp"

#ifdef GRAPHENEDB_HAS_FAISS

#include <faiss/IndexHNSW.h>
#include <faiss/IndexIDMap.h>

#include <atomic>
#include <cmath>
#include <cstdint>
#include <memory>
#include <mutex>
#include <unordered_map>
#include <vector>

namespace graphene {

class FaissVectorIndex final : public VectorIndex {
public:
  explicit FaissVectorIndex(uint32_t dim) : dim_(dim) {}

  Status add(uint32_t id, const std::vector<float>& vec) override {
    if (vec.size() != dim_) {
      return Status::error(ErrorCode::DimensionMismatch, "FaissVectorIndex dimension mismatch");
    }
    raw_[id] = vec;
    dirty_.store(true, std::memory_order_release);
    return Status::ok();
  }

  Status remove(uint32_t id) override {
    raw_.erase(id);
    dirty_.store(true, std::memory_order_release);
    return Status::ok();
  }

  std::vector<SearchResult> search(const std::vector<float>& query, size_t k) const override {
    if (query.size() != dim_ || k == 0 || raw_.empty()) return {};

    if (dirty_.load(std::memory_order_acquire)) {
      std::lock_guard<std::mutex> g(build_mu_);
      if (dirty_.load(std::memory_order_relaxed)) {
        const_cast<FaissVectorIndex*>(this)->rebuild();
      }
    }

    if (!index_ || raw_.empty()) return {};

    auto q = unit(query);
    std::vector<float> distances(k, 0.0f);
    std::vector<faiss::idx_t> labels(k, -1);
    index_->search(1, q.data(), static_cast<faiss::idx_t>(k), distances.data(), labels.data());

    std::vector<SearchResult> out;
    out.reserve(k);
    for (size_t i = 0; i < k; ++i) {
      if (labels[i] < 0) continue;
      out.push_back({static_cast<uint32_t>(labels[i]), static_cast<double>(distances[i])});
    }
    return out;
  }

  const char* name() const override { return "faiss"; }

private:
  static constexpr int kHnswM = 32;
  static constexpr int kEfConstruction = 128;
  static constexpr int kEfSearch = 192;

  uint32_t dim_;
  std::unordered_map<uint32_t, std::vector<float>> raw_;
  mutable std::atomic<bool> dirty_{true};
  mutable std::mutex build_mu_;
  std::unique_ptr<faiss::Index> index_;

  std::vector<float> unit(const std::vector<float>& v) const {
    double norm = 0.0;
    for (float f : v) norm += static_cast<double>(f) * f;
    norm = std::sqrt(norm);
    if (norm < 1e-12) return v;
    std::vector<float> out(v.size());
    for (size_t i = 0; i < v.size(); ++i) {
      out[i] = static_cast<float>(v[i] / norm);
    }
    return out;
  }

  void rebuild() {
    auto base = std::make_unique<faiss::IndexHNSWFlat>(
      static_cast<int>(dim_), kHnswM, faiss::METRIC_INNER_PRODUCT);
    base->hnsw.efConstruction = kEfConstruction;
    base->hnsw.efSearch = kEfSearch;

    auto idmap = std::make_unique<faiss::IndexIDMap2>(base.release());
    if (!raw_.empty()) {
      std::vector<float> vectors;
      std::vector<faiss::idx_t> ids;
      vectors.reserve(raw_.size() * static_cast<size_t>(dim_));
      ids.reserve(raw_.size());
      for (const auto& kv : raw_) {
        auto normalized = unit(kv.second);
        vectors.insert(vectors.end(), normalized.begin(), normalized.end());
        ids.push_back(static_cast<faiss::idx_t>(kv.first));
      }
      idmap->add_with_ids(static_cast<faiss::idx_t>(ids.size()), vectors.data(), ids.data());
    }

    index_ = std::move(idmap);
    dirty_.store(false, std::memory_order_release);
  }
};

}  // namespace graphene

#endif
