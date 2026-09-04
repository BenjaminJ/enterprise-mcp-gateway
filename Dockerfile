# Multi-stage build for minimal production image
FROM golang:1.27-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build statically linked binaries
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -extldflags '-static'" -o /bin/mcp-gateway ./cmd/gateway
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w -extldflags '-static'" -o /bin/mockserver ./cmd/mockserver

# Final lightweight production image
FROM alpine:3.20

RUN apk --no-cache add ca-certificates tzdata && \
    addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

COPY --from=builder /bin/mcp-gateway /app/mcp-gateway
COPY --from=builder /bin/mockserver /app/mockserver
COPY examples/ /app/examples/

USER appuser

EXPOSE 8080 8081

ENTRYPOINT ["/app/mcp-gateway"]
CMD ["--config", "/app/examples/config.yaml"]
