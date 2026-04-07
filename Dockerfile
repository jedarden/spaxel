# Spaxel Mothership Dockerfile
# Multi-stage build for minimal production image

# Stage 1: Build the Go binary
FROM golang:1.25-bookworm AS builder

WORKDIR /app

# Copy Go module files first for better caching
COPY mothership/go.mod mothership/go.sum ./
RUN go mod download

# Copy source code
COPY mothership/ ./

# Build the binary
# CGO_ENABLED=0 because we use pure-Go SQLite (modernc.org/sqlite)
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -o spaxel ./cmd/mothership

# Stage 2: Minimal runtime image
FROM debian:12-slim

# Install wget for health check
RUN apt-get update && apt-get install -y --no-install-recommends wget ca-certificates && rm -rf /var/lib/apt/lists/*

# Copy the binary
COPY --from=builder /app/spaxel /spaxel

# Copy dashboard static files (served from filesystem at runtime)
COPY dashboard/ /dashboard/

# Create firmware directory (users should mount their own firmware volume)
# The container will serve firmware binaries for OTA from /firmware/
VOLUME ["/data", "/firmware"]

# Expose HTTP/WebSocket port
EXPOSE 8080

# Health check — verifies service responds with status=ok
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://localhost:8080/healthz | grep -q '"status":"ok"' || exit 1

# Run as non-root
ENTRYPOINT ["/spaxel"]
