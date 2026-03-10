#!/usr/bin/env bash
set -euo pipefail

echo "╔══════════════════════════════════════════════════════╗"
echo "║   AgentFabric - Local Development Setup             ║"
echo "╚══════════════════════════════════════════════════════╝"

# Deps check
for cmd in docker docker-compose go node; do
  if ! command -v $cmd &>/dev/null; then
    echo "ERROR: $cmd is required but not installed."
    exit 1
  fi
done

echo ""
echo "→ Generating JWT secret..."
JWT_SECRET=$(openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | base64 | tr -d '=+/' | head -c 32)

cat > .env << EOF
AF_JWT_SECRET=${JWT_SECRET}
POSTGRES_PASSWORD=fabricdev
PORTAL_API_URL=http://localhost:8080
EOF
echo "  .env created with JWT_SECRET=${JWT_SECRET:0:8}..."

echo ""
echo "→ Starting infrastructure..."
cd deploy/docker
docker-compose up -d postgres redis
echo "  Waiting for postgres..."
sleep 5

echo ""
echo "→ Building and starting services..."
docker-compose up -d --build

echo ""
echo "→ Portal dependencies..."
cd ../../portal
npm install --silent

echo ""
echo "════════════════════════════════════════════════════════"
echo "  AgentFabric is running!"
echo ""
echo "  Portal:         http://localhost:3000"
echo "  API Gateway:    http://localhost:8080"
echo "  OTLP (gRPC):    localhost:4317"
echo "  OTLP (HTTP):    localhost:4318"
echo "  Metrics:        http://localhost:9090/metrics"
echo ""
echo "  Quick test (send a test span):"
echo "  curl -X POST http://localhost:4318/v1/traces \\"
echo "    -H 'Content-Type: application/json' \\"
echo "    -d '{\"resourceSpans\":[]}'"
echo ""
echo "  Stop: docker-compose -f deploy/docker/docker-compose.yml down"
echo "════════════════════════════════════════════════════════"
