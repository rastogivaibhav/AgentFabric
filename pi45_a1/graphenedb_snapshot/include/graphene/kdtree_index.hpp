#pragma once
#include "graphene/vector_index.hpp"
#include <algorithm>
#include <atomic>
#include <cmath>
#include <memory>
#include <mutex>
#include <queue>
#include <unordered_map>
#include <unordered_set>
#include <vector>

namespace graphene {

// K-d tree vector index using cosine similarity on L2-normalized vectors.
//
// Best for: low dimensions (d <= ~20). The 3D/4D examples in this repo are
// ideal. For d=768 real embeddings it degenerates toward O(n) — use HNSW there.
//
// Thread safety: GrapheneDB's outer shared_mutex ensures add()/remove() are
// called under the exclusive (write) lock and search() under the shared (read)
// lock. Multiple concurrent readers can both reach the lazy-rebuild path, so we
// guard the rebuild with an internal mutex + atomic dirty flag using
// double-checked locking.
//
// Pruning (unit-vector geometry): for L2-normalized vectors,
//   cosine(q, c) = dot(q, c) = 1 - L2²(q,c) / 2
// The minimum L2² from q to any point on the far side of a split plane is
// diff² (the perpendicular split-plane distance in that dimension). Therefore:
//   max cosine reachable in far subtree  ≤  1 - diff² / 2
// We prune when that bound cannot beat the current k-th best score.
class KDTreeVectorIndex final : public VectorIndex {
public:
    explicit KDTreeVectorIndex(uint32_t dim) : dim_(dim) {}

    Status add(uint32_t id, const std::vector<float>& vec) override {
        if (vec.size() != dim_)
            return Status::error(ErrorCode::DimensionMismatch, "KDTreeVectorIndex dimension mismatch");
        raw_[id] = vec;
        removed_.erase(id);
        dirty_.store(true, std::memory_order_release);
        return Status::ok();
    }

    Status remove(uint32_t id) override {
        removed_.insert(id);
        raw_.erase(id);
        dirty_.store(true, std::memory_order_release);
        return Status::ok();
    }

    std::vector<SearchResult> search(const std::vector<float>& query, size_t k) const override {
        if (query.size() != dim_ || k == 0 || raw_.empty()) return {};

        // Double-checked locking: multiple concurrent readers may both arrive
        // here with dirty_=true. Only one will rebuild; the rest wait and reuse.
        if (dirty_.load(std::memory_order_acquire)) {
            std::lock_guard<std::mutex> g(build_mu_);
            if (dirty_.load(std::memory_order_relaxed))
                const_cast<KDTreeVectorIndex*>(this)->rebuild();
        }

        if (!root_) return {};

        auto q = unit(query);
        using Pair = std::pair<double, uint32_t>;
        // Min-heap of size k: top() is always the *lowest* score in the set.
        std::priority_queue<Pair, std::vector<Pair>, std::greater<Pair>> heap;
        search_node(root_.get(), q, k, heap);

        std::vector<SearchResult> out;
        out.reserve(heap.size());
        while (!heap.empty()) {
            out.push_back({heap.top().second, heap.top().first});
            heap.pop();
        }
        std::sort(out.begin(), out.end(), [](const auto& a, const auto& b) {
            return a.score > b.score;
        });
        return out;
    }

    const char* name() const override { return "kdtree"; }

private:
    struct Node {
        uint32_t id;
        uint32_t split_dim;
        std::vector<float> vec; // L2-normalized
        std::unique_ptr<Node> left, right;
    };

    uint32_t dim_;
    std::unordered_map<uint32_t, std::vector<float>> raw_;
    std::unordered_set<uint32_t> removed_;
    std::unique_ptr<Node> root_;
    mutable std::atomic<bool> dirty_{true};
    mutable std::mutex build_mu_;

    std::vector<float> unit(const std::vector<float>& v) const {
        double norm = 0.0;
        for (float f : v) norm += static_cast<double>(f) * f;
        norm = std::sqrt(norm);
        if (norm < 1e-12) return v;
        std::vector<float> out(v.size());
        for (size_t i = 0; i < v.size(); ++i) out[i] = static_cast<float>(v[i] / norm);
        return out;
    }

    // Build a balanced k-d tree splitting on cycling dimensions.
    std::unique_ptr<Node> build(
        std::vector<std::pair<uint32_t, std::vector<float>>>& pts,
        int lo, int hi, uint32_t depth
    ) {
        if (lo >= hi) return nullptr;
        uint32_t sd = depth % dim_;
        int mid = lo + (hi - lo) / 2;
        std::nth_element(
            pts.begin() + lo, pts.begin() + mid, pts.begin() + hi,
            [sd](const auto& a, const auto& b) { return a.second[sd] < b.second[sd]; }
        );
        auto node       = std::make_unique<Node>();
        node->id        = pts[mid].first;
        node->split_dim = sd;
        node->vec       = pts[mid].second; // already normalized
        node->left  = build(pts, lo,      mid,      depth + 1);
        node->right = build(pts, mid + 1, hi,       depth + 1);
        return node;
    }

    void rebuild() {
        std::vector<std::pair<uint32_t, std::vector<float>>> pts;
        pts.reserve(raw_.size());
        for (const auto& kv : raw_) {
            if (!removed_.count(kv.first))
                pts.push_back({kv.first, unit(kv.second)});
        }
        root_ = build(pts, 0, static_cast<int>(pts.size()), 0);
        dirty_.store(false, std::memory_order_release);
    }

    using Pair    = std::pair<double, uint32_t>;
    using MinHeap = std::priority_queue<Pair, std::vector<Pair>, std::greater<Pair>>;

    void search_node(const Node* node, const std::vector<float>& q,
                     size_t k, MinHeap& heap) const {
        if (!node) return;

        // Cosine = dot product of unit vectors.
        double dot = 0.0;
        for (size_t i = 0; i < dim_; ++i)
            dot += static_cast<double>(q[i]) * node->vec[i];

        if (heap.size() < k || dot > heap.top().first) {
            heap.push({dot, node->id});
            if (heap.size() > k) heap.pop();
        }

        // Distance from query to the split hyperplane in that dimension.
        double diff = static_cast<double>(q[node->split_dim]) - node->vec[node->split_dim];

        // Near subtree first (more likely to contain the NN).
        const Node* near = diff <= 0.0 ? node->left.get() : node->right.get();
        const Node* far  = diff <= 0.0 ? node->right.get() : node->left.get();
        search_node(near, q, k, heap);

        // Prune: max cosine reachable in far subtree  <=  1 - diff²/2.
        double max_far = 1.0 - (diff * diff) / 2.0;
        if (heap.size() < k || max_far > heap.top().first)
            search_node(far, q, k, heap);
    }
};

} // namespace graphene
