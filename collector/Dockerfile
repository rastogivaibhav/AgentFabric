# ─── Build stage ─────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git ca-certificates

WORKDIR /build
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s -X main.Version=1.0.0" \
    -o /govagn-collector ./cmd/collector

# Static TCP health-check binary — dials :4318 to confirm the HTTP listener is up.
# Compiled here and copied into the distroless runtime (no shell available there).
RUN echo 'package main' > /tmp/hc.go && \
    echo 'import ("net"; "os"; "time")' >> /tmp/hc.go && \
    echo 'func main() {' >> /tmp/hc.go && \
    echo '  c, e := net.DialTimeout("tcp", "127.0.0.1:4318", 3*time.Second)' >> /tmp/hc.go && \
    echo '  if e != nil { os.Exit(1) }' >> /tmp/hc.go && \
    echo '  c.Close()' >> /tmp/hc.go && \
    echo '}' >> /tmp/hc.go && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /healthcheck /tmp/hc.go

# ─── Runtime stage ────────────────────────────────────────────────────────────
FROM gcr.io/distroless/static:nonroot
COPY --from=builder /govagn-collector /govagn-collector
COPY --from=builder /healthcheck /healthcheck
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

USER nonroot:nonroot
EXPOSE 4317 4318 9090

# Orchestrator health gate: dials :4318 (OTLP HTTP).
# Uses the static /healthcheck binary — no shell in distroless image.
HEALTHCHECK --interval=10s --timeout=5s --start-period=10s --retries=3 \
  CMD ["/healthcheck"]

ENTRYPOINT ["/govagn-collector"]
