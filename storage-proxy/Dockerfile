# Multi-stage build for storage-proxy
# Stage 1: Build the binary
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make protobuf-dev protoc gcc musl-dev

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Generate protobuf code
RUN go install google.golang.org/protobuf/cmd/protoc-gen-go@latest && \
    go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

RUN make proto

# Build binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /storage-proxy \
    -ldflags="-w -s" \
    ./cmd/storage-proxy

# Stage 2: Create minimal runtime image
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 storage && \
    adduser -D -u 1000 -G storage storage

# Create directories
RUN mkdir -p /var/lib/storage-proxy/cache && \
    mkdir -p /var/log/storage-proxy && \
    chown -R storage:storage /var/lib/storage-proxy /var/log/storage-proxy

# Copy binary from builder
COPY --from=builder /storage-proxy /usr/local/bin/storage-proxy
RUN chmod +x /usr/local/bin/storage-proxy

# Switch to non-root user
USER storage

# Expose ports
EXPOSE 8080 8081 9090

# Set default environment variables
ENV GRPC_PORT=8080 \
    HTTP_PORT=8081 \
    METRICS_PORT=9090 \
    LOG_LEVEL=info \
    CACHE_ROOT=/var/lib/storage-proxy/cache \
    AUDIT_FILE=/var/log/storage-proxy/audit.log

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8081/health || exit 1

# Run the binary
ENTRYPOINT ["/usr/local/bin/storage-proxy"]

