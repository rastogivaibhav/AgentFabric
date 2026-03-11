# ─── Build ───────────────────────────────────────────────────────────────────
FROM rust:1.78-slim AS builder

RUN apt-get update && apt-get install -y \
    pkg-config libssl-dev libsasl2-dev cmake \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /build
COPY Cargo.toml Cargo.lock* ./
# Cache dependencies layer
RUN mkdir src && echo "fn main(){}" > src/main.rs && cargo build --release && rm -rf src

COPY src ./src
RUN touch src/main.rs && cargo build --release

# ─── Runtime ─────────────────────────────────────────────────────────────────
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y \
    ca-certificates libssl3 libsasl2-2 \
    && rm -rf /var/lib/apt/lists/*

RUN useradd -r -u 10002 afcore
COPY --from=builder /build/target/release/af-core /usr/local/bin/af-core
RUN chown afcore:afcore /usr/local/bin/af-core

USER afcore
EXPOSE 50051 8889
ENTRYPOINT ["/usr/local/bin/af-core"]
