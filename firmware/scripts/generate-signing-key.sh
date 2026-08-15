#!/usr/bin/env bash
# Spaxel Firmware Signing Key Generator
# Generates RSA-2048 key pair for Secure Boot V2 firmware signing.
#
# SECURITY NOTES:
# - The PRIVATE KEY must be kept secret and never committed to git
# - The PUBLIC KEY is embedded in the bootloader and can be public
# - Store the private key in a secure location (e.g., hardware security module)
# - If the private key is compromised, use Secure Boot V2 key revocation
#
# Usage: ./generate-signing-key.sh [output_dir]
# Default output_dir: ../keys/

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEFAULT_OUTPUT_DIR="${SCRIPT_DIR}/../keys"
OUTPUT_DIR="${1:-$DEFAULT_OUTPUT_DIR}"

# Create output directory
mkdir -p "$OUTPUT_DIR"

echo "Generating Secure Boot V2 signing keys in: $OUTPUT_DIR"
echo ""

# Check if ESP-IDF secure boot signing tool is available
if ! command -v espsecure.py &> /dev/null; then
    echo "ERROR: espsecure.py not found in PATH"
    echo ""
    echo "ESP-IDF secure boot tools are required. Source ESP-IDF environment:"
    echo "  source /path/to/esp-idf/export.sh"
    echo ""
    exit 1
fi

# Generate RSA-2048 key pair for Secure Boot V2
# Key format: PEM (for use with espsecure.py sign_bootloader --version 2)
echo "Generating RSA-2048 key pair..."
espsecure.py generate_signing_key \
    --version 2 \
    --scheme rsa_pss \
    "$OUTPUT_DIR/firmware_signing_key.pem"

if [ $? -eq 0 ]; then
    echo ""
    echo "✓ Key generation successful!"
    echo ""
    echo "Generated files:"
    echo "  - $OUTPUT_DIR/firmware_signing_key.pem (PRIVATE KEY - KEEP SECRET!)"
    echo ""
    echo "Next steps:"
    echo "  1. BACK UP the private key to a secure, offline location"
    echo "  2. Add '$OUTPUT_DIR' to .gitignore"
    echo "  3. Build and sign firmware: idf.py build"
    echo "  4. Flash signed firmware to enable secure boot"
    echo ""
    echo "SECURITY WARNING:"
    echo "  The private key MUST be kept secret. If compromised, an attacker"
    echo "  can sign malicious firmware that nodes will accept. Rotate the key"
    echo "  immediately if exposure is suspected."
    echo ""
else
    echo ""
    echo "✗ Key generation failed"
    rm -f "$OUTPUT_DIR/firmware_signing_key.pem"
    exit 1
fi
