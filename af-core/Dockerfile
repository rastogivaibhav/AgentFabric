# ── Stage 1: Build ────────────────────────────────────────────────────────────
# rust:1.76-slim-bookworm gives a lean base with cargo/rustc pre-installed.
# cmake + gcc are required by the rdkafka cmake-build feature which compiles
# librdkafka from source and statically links it — no runtime C library needed.
FROM rust:1.76-slim-bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    cmake \
    gcc \
    g++ \
    pkg-config \
    libssl-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# ── Dependency cache layer ─────────────────────────────────────────────────────
# Copy manifests only first so cargo can cache the full dependency compile.
# Rebuilds are fast unless Cargo.toml changes.
COPY Cargo.toml ./
RUN mkdir -p src && echo 'fn main() {}' > src/main.rs
RUN cargo build --release
# Remove the stub binary so the real source triggers a fresh link step
RUN rm -f target/release/af-core target/release/deps/af_core*

# ── Full source build ──────────────────────────────────────────────────────────
COPY src ./src
RUN cargo build --release

# ── Stage 2: Runtime ──────────────────────────────────────────────────────────
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

RUN adduser --disabled-password --gecos "" --uid 10001 appuser
WORKDIR /app

COPY --from=builder /app/target/release/af-core ./af-core

USER appuser

# :8889 — HTTP management (health + metrics)
# :50051 — gRPC (reserved for future policy query API)
EXPOSE 8889 50051

HEALTHCHECK --interval=10s --timeout=5s --retries=5 --start-period=15s \
    CMD curl -sf http://127.0.0.1:8889/health || exit 1

CMD ["./af-core"]
