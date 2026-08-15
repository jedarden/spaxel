#!/usr/bin/env bash
# Spaxel Firmware Signing Script
# Signs firmware binaries with the Secure Boot V2 private key.
#
# This script is called automatically during the build process via CMake.
#
# Usage: ./sign-firmware.sh <binary_path> <output_path> <version>
#   binary_path: Path to unsigned firmware binary (.bin)
#   output_path: Path for signed output binary
#   version: Firmware version string (for anti-rollback)

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
KEYS_DIR="${SCRIPT_DIR}/../keys"
PRIVATE_KEY="${KEYS_DIR}/firmware_signing_key.pem"

BINARY_PATH="$1"
OUTPUT_PATH="$2"
VERSION="$3"

if [ -z "$BINARY_PATH" ] || [ -z "$OUTPUT_PATH" ]; then
    echo "Usage: $0 <binary_path> <output_path> [version]"
    exit 1
fi

if [ ! -f "$PRIVATE_KEY" ]; then
    echo "ERROR: Signing key not found: $PRIVATE_KEY"
    echo ""
    echo "Generate signing key first:"
    echo "  ./scripts/generate-signing-key.sh"
    echo ""
    exit 1
fi

echo "Signing firmware: $BINARY_PATH"
echo "Output: $OUTPUT_PATH"
echo "Version: ${VERSION:-<not specified>}"

# Sign the firmware binary with Secure Boot V2
espsecure.py sign_data \
    --version 2 \
    --scheme rsa_pss \
    --keyfile "$PRIVATE_KEY" \
    --output "$OUTPUT_PATH" \
    "$BINARY_PATH"

if [ $? -eq 0 ]; then
    echo "✓ Firmware signed successfully"
else
    echo "✗ Firmware signing failed"
    exit 1
fi
