FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git ca-certificates

WORKDIR /build/api-gateway
COPY api-gateway/go.mod api-gateway/go.sum ./
RUN go mod download
COPY api-gateway/. ./
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" \
    -o /govagn-gateway ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates wget netcat-openbsd
RUN adduser -D -u 10001 appuser

WORKDIR /app
COPY --from=builder /govagn-gateway /govagn-gateway
COPY deploy/migrations /app/deploy/migrations
COPY deploy/seed /app/deploy/seed
COPY docs/openapi.yaml /app/docs/openapi.yaml

USER appuser
EXPOSE 10000

ENTRYPOINT ["/govagn-gateway"]
