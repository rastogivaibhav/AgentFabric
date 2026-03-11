FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git ca-certificates

WORKDIR /build
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" \
    -o /agentfabric-gateway ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates wget
RUN adduser -D -u 10001 appuser
COPY --from=builder /agentfabric-gateway /agentfabric-gateway

USER appuser
EXPOSE 8080

ENTRYPOINT ["/agentfabric-gateway"]
