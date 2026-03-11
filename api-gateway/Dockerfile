FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git ca-certificates

WORKDIR /build
COPY . .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-w -s" \
    -o /agentfabric-gateway ./cmd/server

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /agentfabric-gateway /agentfabric-gateway
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/agentfabric-gateway"]
