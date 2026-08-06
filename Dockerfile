# Spaxel Mothership Dockerfile
# Multi-stage build: ESP32 firmware (amd64 only) → Go binary → minimal runtime image
# Build arguments for multi-platform support (auto-populated by buildx
# --platform; kaniko, which is what actually runs this in CI, does NOT
# auto-populate these -- see the default-value note below).
ARG TARGETPLATFORM
ARG TARGETARCH

# Stage 1: Build ESP32-S3 firmware (amd64 only - ESP-IDF is x86_64)
FROM espressif/idf:v5.2 AS firmware-builder
# The CI kaniko invocation (see spaxel-build-workflowtemplate.yml) passes no
# --customPlatform and no --build-arg for TARGETPLATFORM, so unlike buildx it
# never auto-populates this ARG -- with no default it resolved to an empty
# string. "" != "linux/amd64" is always true, so the "skip on non-amd64"
# branch below ALWAYS ran, unconditionally writing a placeholder
# spaxel-firmware-merged.bin into /project/build before idf.py ever touched
# it -- 100% deterministically, on every single build regardless of layer
# caching. idf.py set-target's implicit fullclean action then refused to
# clean that stray non-CMake file and failed the build every time.
#
# Three earlier fix attempts (see git history, bf-38dbu, 2026-08-02) treated
# this as a kaniko cache-poisoning bug and tried to force a cache miss --
# that was a red herring. The layers really were identical on every build,
# because the underlying command was genuinely deterministic. Defaulting the
# ARG to linux/amd64 fixes it for kaniko while still letting buildx override
# it per-platform for local multi-arch builds.
ARG TARGETPLATFORM=linux/amd64

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

# Remove any stale generated sdkconfig so set-target regenerates it from
# sdkconfig.defaults (which specifies CONFIG_ESPTOOLPY_FLASHSIZE_4MB=y).
RUN rm -f sdkconfig sdkconfig.old

# Firmware host-test gate: run the gcc host unit tests (nvs schema migration,
# CSI binary-frame serialization, serial_prov JSON parser fuzz) BEFORE the
# expensive ESP-IDF build so a logic/format-contract regression fails the image
# build fast. Pure gcc — no IDF toolchain needed; gcc + GNU make ship in this
# espressif/idf image. `make` propagates the suite's non-zero exit code on any
# assertion failure, failing the build. This is the gcc harness, NOT idf.py
# --target linux — see firmware/test/Makefile and the decision record
# docs/notes/firmware-host-test-approach.md (firmware/main cannot be host-linked).
RUN make -C test test

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
        0x10000 build/ota_data_initial.bin \
        0x20000 build/spaxel-firmware.bin

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
