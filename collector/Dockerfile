# ─── Build stage ─────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git ca-certificates

WORKDIR /build
COPY go.mod ./
RUN go mod tidy

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s -X main.Version=1.0.0" \
    -o /agentfabric-collector ./cmd/collector

# ─── Runtime stage ────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static:nonroot
COPY --from=builder /agentfabric-collector /agentfabric-collector
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

USER nonroot:nonroot
EXPOSE 4317 4318 9090

ENTRYPOINT ["/agentfabric-collector"]
