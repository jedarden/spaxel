# Spaxel Mothership Dockerfile
# Multi-stage build: Go binary → minimal runtime image (ESP32 firmware fetched from CI artifact)
# Build arguments for multi-platform support (auto-populated by buildx --platform)
ARG TARGETPLATFORM
ARG TARGETARCH

# Stage 1: Fetch prebuilt ESP32-S3 firmware from GitHub Releases
# Firmware is built once in CI (amd64-only) and published as a versioned artifact.
# This stage downloads the appropriate firmware binary for all platforms.
FROM alpine:3.20 AS firmware-fetcher
ARG VERSION=dev

# Install dependencies
RUN apk add --no-cache curl

# Fetch firmware from GitHub Releases
# The firmware-build CI step uploads spaxel-firmware-${VERSION}-merged.bin to releases
WORKDIR /firmware
RUN curl -fsSL \
    "https://github.com/jedarden/spaxel/releases/download/v${VERSION}/spaxel-firmware-${VERSION}-merged.bin" \
    -o spaxel-firmware-merged.bin && \
    curl -fsSL \
    "https://github.com/jedarden/spaxel/releases/download/v${VERSION}/spaxel-firmware.bin" \
    -o spaxel-firmware.bin && \
    echo "=== Firmware binaries downloaded ===" && \
    ls -lh

# Stage 2: Build the Go binary (cross-platform)
# Same empty-under-kaniko risk as TARGETPLATFORM above -- default to the
# CI builder's actual platform so `--platform=$BUILDPLATFORM` below never
# resolves to an empty, unparsable value.
ARG BUILDPLATFORM=linux/amd64
FROM --platform=$BUILDPLATFORM golang:1.25-bookworm AS builder

WORKDIR /app

# Copy Go module files first for better caching
COPY mothership/go.mod mothership/go.sum ./
RUN go mod download

# Copy source code
COPY mothership/ ./

# Copy dashboard files into the mothership cmd/mothership directory for go:embed
# The go:embed directive in cmd/mothership/main.go references the local dashboard directory
COPY dashboard/ ./cmd/mothership/dashboard/

# Build the binary for the target platform (set by buildx --platform).
# Builds native binaries per-architecture (amd64, arm64) using --platform=$BUILDPLATFORM.
# ESP32 firmware is built once on amd64 and copied into all platform images.
# CGO_ENABLED=0 because we use pure-Go SQLite (modernc.org/sqlite)
# -tags=embed enables dashboard embedding via go:embed
ARG VERSION=dev
ARG TARGETPLATFORM=linux/amd64
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 \
    GOOS=linux GOARCH=$TARGETARCH \
    go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -tags=embed \
    -o spaxel ./cmd/mothership

# Also build the CSI simulator so the same image can run synthetic-node load
# against a deployed mothership (used by the in-cluster simulator workload).
RUN CGO_ENABLED=0 \
    GOOS=linux GOARCH=$TARGETARCH \
    go build \
    -ldflags="-s -w" \
    -o spaxel-sim ./cmd/sim

# Stage 3: Minimal runtime image - distroless nonroot
# Dashboard is embedded in the Go binary via go:embed, not copied as files
FROM gcr.io/distroless/static-debian12:nonroot
ARG TARGETARCH=amd64
ARG VERSION=dev

# Copy the binary (dashboard is embedded via go:embed)
COPY --from=builder /app/spaxel /spaxel

# CSI simulator binary — invoked via an explicit command override in the
# simulator workload; the default ENTRYPOINT still runs the mothership.
COPY --from=builder /app/spaxel-sim /spaxel-sim

# OTA writes directly into an app partition, so seed only the app image at the
# top level. The semver-bearing filename is also the OTA store's version source.
COPY --from=firmware-fetcher /firmware/spaxel-firmware.bin /firmware/spaxel-firmware-${VERSION}.bin

# Keep the merged offset-0 image for first-flash serial provisioning, isolated
# in a subdirectory that seedFirmwareDir deliberately does not copy into the OTA
# store. A merged image must never be written into an OTA app partition.
COPY --from=firmware-fetcher /firmware/spaxel-firmware-merged.bin /firmware/serial/spaxel-firmware-${VERSION}-merged.bin

VOLUME ["/data"]

# Expose HTTP/WebSocket port
EXPOSE 8080

# Run as non-root
ENTRYPOINT ["/spaxel"]
