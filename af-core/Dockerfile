# ─── Build ───────────────────────────────────────────────────────────────────
FROM rust:latest AS builder

RUN apt-get update && apt-get install -y \
    pkg-config libssl-dev libsasl2-dev cmake \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build
COPY Cargo.toml ./
# Cache dependencies layer
RUN mkdir src && echo "fn main(){}" > src/main.rs && cargo build --release && rm -rf src

COPY src ./src
COPY migrations ./migrations
RUN touch src/main.rs && cargo build --release

# ─── Runtime ─────────────────────────────────────────────────────────────────
FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates libssl3 libsasl2-2 curl \
    && rm -rf /var/lib/apt/lists/*

RUN useradd -r -u 10002 afcore
COPY --from=builder /build/target/release/af-core /usr/local/bin/af-core
RUN chown afcore:afcore /usr/local/bin/af-core

USER afcore
EXPOSE 50051 8889

# Orchestrator health gate: polls :8889/health (af-core HTTP management port).
# curl -sf exits non-zero on HTTP errors (4xx/5xx) or connection failure.
HEALTHCHECK --interval=10s --timeout=5s --start-period=15s --retries=3 \
  CMD curl -sf http://127.0.0.1:8889/health || exit 1

ENTRYPOINT ["/usr/local/bin/af-core"]
