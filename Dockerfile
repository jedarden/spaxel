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

WORKDIR /project
COPY firmware/ ./

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

# Build the binary with cross-platform support
# CGO_ENABLED=0 because we use pure-Go SQLite (modernc.org/sqlite)
# GOOS/GOARCH derived from TARGETPLATFORM for multi-arch builds
# -tags=embed enables dashboard embedding via go:embed
ARG VERSION=dev
ARG TARGETPLATFORM
RUN CGO_ENABLED=0 \
    GOOS=$(echo $TARGETPLATFORM | cut -d'/' -f2) \
    GOARCH=$(echo $TARGETPLATFORM | cut -d'/' -f3) \
    go build \
    -ldflags="-s -w -X main.version=${VERSION}" \
    -tags=embed \
    -o spaxel ./cmd/mothership

# Stage 3: Minimal runtime image - distroless nonroot
# Dashboard is embedded in the Go binary via go:embed, not copied as files
FROM gcr.io/distroless/static-debian12:nonroot
ARG TARGETARCH=amd64

# Copy the binary (dashboard is embedded via go:embed)
COPY --from=builder /app/spaxel /spaxel

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
