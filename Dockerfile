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
FROM gcr.io/distroless/static-debian12:nonroot

# Copy the binary
COPY --from=builder /app/spaxel /spaxel

# Copy dashboard static files (served from filesystem at runtime)
COPY dashboard/ /dashboard/

# Create firmware directory (users should mount their own firmware volume)
# The container will serve firmware binaries for OTA from /firmware/
VOLUME ["/data", "/firmware"]

# Expose HTTP/WebSocket port
EXPOSE 8080

# Health check — distroless has no shell or wget, so remove container-level check.
# K8s liveness/readiness probes handle health checking instead.

# Run as non-root (distroless default is UID 65532)
ENTRYPOINT ["/spaxel"]
