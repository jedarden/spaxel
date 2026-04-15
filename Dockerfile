# Spaxel Mothership Dockerfile
# Multi-stage build: ESP32 firmware → Go binary → minimal runtime image

# Stage 1: Build ESP32-S3 firmware
FROM espressif/idf:v5.2 AS firmware-builder

WORKDIR /project
COPY firmware/ ./

# Source export.sh to activate IDF toolchain (entrypoint is not called in build stages).
# set-target must be run explicitly before build even when CONFIG_IDF_TARGET is in sdkconfig.defaults.
# idf.py build produces build/spaxel-firmware.bin
SHELL ["/bin/bash", "-c"]
RUN . $IDF_PATH/export.sh && idf.py set-target esp32s3 && idf.py build && \
    python -m esptool --chip esp32s3 merge_bin \
        --output build/spaxel-firmware-merged.bin \
        0x0      build/bootloader/bootloader.bin \
        0x8000   build/partition_table/partition-table.bin \
        0x10000  build/spaxel-firmware.bin \
        0xc10000 build/ota_data_initial.bin

# Stage 2: Build the Go binary
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

# Bake ESP32 firmware into the image so the mothership can seed it on first run.
# The mothership copies /firmware/*.bin → /data/firmware/ at startup if not present.
COPY --from=firmware-builder /project/build/spaxel-firmware-merged.bin /firmware/spaxel-firmware.bin

VOLUME ["/data"]

# Expose HTTP/WebSocket port
EXPOSE 8080

# Health check — verifies service responds with status=ok
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://localhost:8080/healthz | grep -q '"status":"ok"' || exit 1

# Run as non-root
ENTRYPOINT ["/spaxel"]
