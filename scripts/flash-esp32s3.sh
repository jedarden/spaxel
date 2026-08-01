#!/usr/bin/env bash
# Flash an ESP32-S3 over its native USB-Serial/JTAG interface.
#
# esptool's large write-flash operations are unreliable on this interface
# (writes above ~20-32KB fail with chip-side Guru Meditation panics that are
# actually transport artifacts, not firmware crashes -- see docs/notes for
# the full writeup, bf-26pa). The workaround is to split every image into
# 32KB chunks and write each in its own esptool invocation with retries.
#
# Usage:
#   scripts/flash-esp32s3.sh <port> <offset>:<file> [<offset>:<file> ...]
#
# Example (fresh chip -- bootloader + partition table + otadata + app):
#   scripts/flash-esp32s3.sh /dev/ttyACM0 \
#     0x0:build/bootloader/bootloader.bin \
#     0x8000:build/partition_table/partition-table.bin \
#     0x10000:build/ota_data_initial.bin \
#     0x20000:build/spaxel-firmware.bin
#
# Example (app-only update, preserves NVS/WiFi credentials -- no erase):
#   scripts/flash-esp32s3.sh /dev/ttyACM0 0x20000:build/spaxel-firmware.bin

set -euo pipefail

CHUNK_SIZE=32768
RETRIES=6

if [ $# -lt 2 ]; then
    echo "Usage: $0 <port> <offset>:<file> [<offset>:<file> ...]" >&2
    exit 1
fi

PORT="$1"
shift

WORKDIR=$(mktemp -d)
trap 'rm -rf "$WORKDIR"' EXIT

fail=0

for arg in "$@"; do
    offset="${arg%%:*}"
    file="${arg#*:}"

    if [ ! -f "$file" ]; then
        echo "FATAL: $file does not exist" >&2
        exit 1
    fi

    base=$(printf "%d" "$offset")
    chunkdir="$WORKDIR/$(basename "$file")"
    mkdir -p "$chunkdir"
    split -b "$CHUNK_SIZE" -d -a 4 "$file" "$chunkdir/c_"

    for chunk in "$chunkdir"/c_*; do
        n=$((10#$(basename "$chunk" | sed 's/^c_//')))
        chunk_off=$((base + n * CHUNK_SIZE))
        hex=$(printf "0x%x" "$chunk_off")

        ok=0
        for try in $(seq 1 "$RETRIES"); do
            if esptool --chip esp32s3 --port "$PORT" --no-stub -b 460800 \
                --before usb-reset --after no-reset \
                write-flash --flash-mode dio --flash-size 4MB --flash-freq 80m \
                "$hex" "$chunk" 2>&1 | grep -q "Hash of data verified"; then
                ok=1
                break
            fi
            sleep 1
        done

        if [ "$ok" -eq 1 ]; then
            echo "OK   $file @ $hex ($(basename "$chunk"))"
        else
            echo "FAIL $file @ $hex ($(basename "$chunk")) after $RETRIES tries" >&2
            fail=1
        fi
    done
done

if [ "$fail" -ne 0 ]; then
    echo "One or more chunks failed to verify -- flash is INCOMPLETE." >&2
    exit 1
fi

echo "All chunks verified."
