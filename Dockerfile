# Spaxel Mothership Dockerfile
# Multi-stage build: ESP32 firmware (amd64 only) → Go binary → minimal runtime image
# Build arguments for multi-platform support
ARG TARGETPLATFORM=linux/amd64
ARG TARGETARCH=amd64

# Stage 1: Build ESP32-S3 firmware (amd64 only - ESP-IDF is x86_64)
FROM espressif/idf:v5.2 AS firmware-builder
ARG TARGETPLATFORM

# Create build directory
RUN mkdir -p /project/build

# Handle amd64-only firmware build: skip on arm64, build on amd64
RUN if [ "$TARGETPLATFORM" != "linux/amd64" ]; then \
        echo "# Firmware not available on $TARGETPLATFORM (ESP-IDF is amd64-only)" > /project/build/spaxel-firmware-merged.bin && \
        echo "Firmware build skipped - placeholder created"; \
    fi

# Only copy firmware source and build on amd64 (placeholder already created on arm64)
RUN if [ "$TARGETPLATFORM" = "linux/amd64" ]; then \
        cd /project && \
        echo "Building ESP32 firmware for $TARGETPLATFORM"; \
    else \
        exit 0; \
    fi

# Bust Kaniko layer cache when flash size config changes (sdkconfig.defaults).
# Without this ARG, Kaniko can serve a cached firmware layer that was built with
# the old 16MB config even after sdkconfig.defaults is updated to 4MB.
ARG FIRMWARE_CACHE_BUST=2026-06-03

WORKDIR /project
COPY firmware/ ./

# Remove any stale generated sdkconfig so set-target regenerates it from
# sdkconfig.defaults (which specifies CONFIG_ESPTOOLPY_FLASHSIZE_4MB=y).
RUN rm -f sdkconfig sdkconfig.old

# Source export.sh to activate IDF toolchain (entrypoint is not called in build stages).
# set-target must be run explicitly before build even when CONFIG_IDF_TARGET is in sdkconfig.defaults.
# idf.py build produces build/spaxel-firmware.bin
SHELL ["/bin/bash", "-c"]
RUN . $IDF_PATH/export.sh && idf.py set-target esp32s3 && idf.py build && \
    python -m esptool --chip esp32s3 merge_bin \
        --flash_mode dio --flash_freq 80m --flash_size 4MB \
        --output build/spaxel-firmware-merged.bin \
        0x0     build/bootloader/bootloader.bin \
        0x8000  build/partition_table/partition-table.bin \
        0x10000 build/spaxel-firmware.bin

# Stage 2: Build the Go binary (cross-platform)
FROM golang:1.25-bookworm AS builder

WORKDIR /app

# Copy Go module files first for better caching
COPY mothership/go.mod mothership/go.sum ./
RUN go mod download

# Copy source code
COPY mothership/ ./

# Copy dashboard files into the mothership cmd/mothership directory for go:embed
# The go:embed directive in cmd/mothership/main.go references the local dashboard directory
COPY dashboard/ ./cmd/mothership/dashboard/

# Build the binary. CI builds amd64 only (ESP-IDF firmware is x86_64-only),
# so GOOS/GOARCH are pinned to linux/amd64.
# CGO_ENABLED=0 because we use pure-Go SQLite (modernc.org/sqlite)
# -tags=embed enables dashboard embedding via go:embed
ARG VERSION=dev
ARG TARGETPLATFORM
RUN CGO_ENABLED=0 \
    GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -tags=embed \
    -o spaxel ./cmd/mothership

# Also build the CSI simulator so the same image can run synthetic-node load
# against a deployed mothership (used by the in-cluster simulator workload).
RUN CGO_ENABLED=0 \
    GOOS=linux GOARCH=amd64 \
    go build \
    -ldflags="-s -w" \
    -o spaxel-sim ./cmd/sim

# Stage 3: Minimal runtime image - distroless nonroot
# Dashboard is embedded in the Go binary via go:embed, not copied as files
FROM gcr.io/distroless/static-debian12:nonroot
ARG TARGETARCH=amd64

# Copy the binary (dashboard is embedded via go:embed)
COPY --from=builder /app/spaxel /spaxel

# CSI simulator binary — invoked via an explicit command override in the
# simulator workload; the default ENTRYPOINT still runs the mothership.
COPY --from=builder /app/spaxel-sim /spaxel-sim

# Bake ESP32 firmware into the image so the mothership can seed it on first run.
# The mothership copies /firmware/*.bin → /data/firmware/ at startup if not present.
# Firmware is only included on amd64 builds (ESP-IDF is x86_64-only).
# For non-amd64 builds, the placeholder from firmware-builder stage is included.
COPY --from=firmware-builder /project/build/spaxel-firmware-merged.bin /firmware/spaxel-firmware.bin

VOLUME ["/data"]

# Expose HTTP/WebSocket port
EXPOSE 8080

# Run as non-root
ENTRYPOINT ["/spaxel"]
