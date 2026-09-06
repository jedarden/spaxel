#!/usr/bin/env bash
# Sign a firmware image with Secure Boot V2 key(s).
#
#   ./sign-firmware.sh <binary> <output> [version]        # dev: key from ../keys/
#   ./sign-firmware.sh --keyfile K.pem <binary> <output>  # explicit key file
#   ./sign-firmware.sh --key-fd 0 <binary> <output>       # key read from a file descriptor
#
# --keyfile and --key-fd may be given more than once (up to 3 keys total): the
# image then carries one signature block per key, which is how a rotation
# release is signed while old and new keys are both still trusted. Repeating a
# flag needs no --append_signatures — espsecure writes one block per keyfile in
# a single pass.
#
# PRODUCTION — fetch the key from OpenBao straight into the signer's stdin so
# it never lands on disk, in argv, in a log or in a transcript:
#
#   bao-as openbao-v2 bao kv get -field=key_pem \
#       secret/ardenone-cluster/spaxel/firmware-signing/prod \
#     | ./sign-firmware.sh --key-fd 0 build/spaxel.bin build/spaxel-signed.bin
#
# The version argument is accepted for compatibility and only echoed: the
# anti-rollback value is stamped into the image by the build from
# firmware/SECURE_VERSION, not by this script (see bump-secure-version.sh).
#
# The build does not invoke this script — idf.py signs internally when secure
# boot is enabled. Use it for out-of-band signing of an existing image.

set -euo pipefail

usage() {
    sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'
    exit 2
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEV_KEY="${SCRIPT_DIR}/../keys/firmware_signing_key.pem"

KEYS=()
BINARY_PATH=""
OUTPUT_PATH=""

while [ $# -gt 0 ]; do
    case "$1" in
        --keyfile|-k)
            [ $# -ge 2 ] || usage
            KEYS+=("$2")
            shift 2
            ;;
        --keyfile=*)
            KEYS+=("${1#--keyfile=}")
            shift
            ;;
        --key-fd)
            [ $# -ge 2 ] || usage
            KEYS+=("/dev/fd/$2")
            shift 2
            ;;
        --key-fd=*)
            KEYS+=("/dev/fd/${1#--key-fd=}")
            shift
            ;;
        -h|--help)
            usage
            ;;
        --*)
            echo "unknown option: $1" >&2
            usage
            ;;
        *)
            if [ -z "$BINARY_PATH" ]; then
                BINARY_PATH="$1"
            elif [ -z "$OUTPUT_PATH" ]; then
                OUTPUT_PATH="$1"
            else
                VERSION="$1"
                shift
                continue
            fi
            shift
            ;;
    esac
done

if [ -z "$BINARY_PATH" ] || [ -z "$OUTPUT_PATH" ]; then
    echo "both <binary> and <output> are required" >&2
    usage
fi

if [ ! -f "$BINARY_PATH" ]; then
    echo "ERROR: input image not found: $BINARY_PATH" >&2
    exit 1
fi

if [ "${#KEYS[@]}" -eq 0 ]; then
    if [ ! -f "$DEV_KEY" ]; then
        echo "ERROR: no key given and dev key not found: $DEV_KEY" >&2
        echo "" >&2
        echo "Generate one first (./scripts/generate-signing-key.sh) or pass an" >&2
        echo "explicit --keyfile / --key-fd. The production key comes from OpenBao:" >&2
        echo "  bao-as openbao-v2 bao kv get -field=key_pem \\" >&2
        echo "      secret/ardenone-cluster/spaxel/firmware-signing/prod \\" >&2
        echo "    | $0 --key-fd 0 <binary> <output>" >&2
        exit 1
    fi
    KEYS+=("$DEV_KEY")
    echo "NOTE: signing with the development key (scripts must stay off the prod" >&2
    echo "path): $DEV_KEY" >&2
fi

if [ "${#KEYS[@]}" -gt 3 ]; then
    echo "ERROR: Secure Boot V2 supports at most 3 signature blocks (got ${#KEYS[@]} keys)" >&2
    exit 1
fi

# Locate espsecure.py: on PATH first (CI container, sourced IDF export.sh),
# then the pinned IDF 5.2 python env, then any installed env.
find_espsecure() {
    if command -v espsecure.py >/dev/null 2>&1; then
        printf 'espsecure.py\n'
        return 0
    fi
    local candidate
    for candidate in "${HOME}"/.espressif/python_env/idf5.2*/bin/espsecure.py \
                     "${HOME}"/.espressif/python_env/*/bin/espsecure.py; do
        if [ -x "$candidate" ]; then
            printf '%s\n' "$candidate"
            return 0
        fi
    done
    return 1
}

if ! ESPSECURE="$(find_espsecure)"; then
    echo "ERROR: espsecure.py not found" >&2
    echo "" >&2
    echo "ESP-IDF secure boot tools are required. Either source an ESP-IDF" >&2
    echo "environment (source /path/to/esp-idf/export.sh) or install one under" >&2
    echo "~/.espressif/python_env/." >&2
    exit 1
fi

echo "Signing image: $BINARY_PATH"
echo "         into: $OUTPUT_PATH"
echo "       key(s): ${#KEYS[@]} (${KEYS[*]})"
echo "      version: ${VERSION:-<not specified — stamped by the build>}"

# Secure Boot V2, RSA scheme. sign_data has no --scheme flag: the key itself
# determines the scheme (RSA keys are always RSA-3072).
"$ESPSECURE" sign_data \
    --version 2 \
    --keyfile "${KEYS[@]}" \
    --output "$OUTPUT_PATH" \
    "$BINARY_PATH"

echo "Signed successfully: $OUTPUT_PATH"
