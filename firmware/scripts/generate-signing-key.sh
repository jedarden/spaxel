#!/usr/bin/env bash
# Generate a Secure Boot V2 firmware signing key (RSA-3072 — the only RSA size
# the scheme supports; earlier revisions of this script said RSA-2048, which is
# wrong).
#
#   ./generate-signing-key.sh [output_dir]              # dev: self-serve local key
#   ./generate-signing-key.sh --stdout                  # prod: PEM on stdout, nothing else
#   ./generate-signing-key.sh --stdout --pubout FILE    # also extract the public half
#
# DEV (default): writes firmware_signing_key.pem into output_dir (default
# ../keys/, gitignored). Only ever used for bench boards.
#
# PRODUCTION: the key must never touch a terminal, a repo path, argv or a log.
# Generate to stdout and pipe it straight into OpenBao, from a RAM-backed
# working directory, under the write-only provisioning identity:
#
#   ./generate-signing-key.sh --stdout --pubout "$TMPD/pub.pem" > "$TMPD/key.pem"
#   bao-as openbao-v2-provision bao kv put -cas=<N> \
#       secret/ardenone-cluster/spaxel/firmware-signing/prod @"$TMPD/payload.json"
#
# The full ceremony (payload assembly, CAS, verify-by-metadata) is documented
# in docs/notes/firmware-signing-keys.md.
#
# Diagnostics go to stderr in --stdout mode so the stream stays a clean PEM.

set -euo pipefail

usage() {
    sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
    exit 2
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEFAULT_OUTPUT_DIR="${SCRIPT_DIR}/../keys"

STDOUT_MODE=0
PUBOUT=""
OUTPUT_DIR="$DEFAULT_OUTPUT_DIR"

while [ $# -gt 0 ]; do
    case "$1" in
        --stdout)
            STDOUT_MODE=1
            shift
            ;;
        --pubout)
            [ $# -ge 2 ] || usage
            PUBOUT="$2"
            shift 2
            ;;
        --pubout=*)
            PUBOUT="${1#--pubout=}"
            shift
            ;;
        -h|--help)
            usage
            ;;
        *)
            if [ "$STDOUT_MODE" -eq 1 ]; then
                echo "--stdout takes no positional argument: $1" >&2
                usage
            fi
            OUTPUT_DIR="$1"
            shift
            ;;
    esac
done

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

# Private keys are created 0600 everywhere this script writes.
umask 077

# Version 2, RSA scheme: the key is RSA-3072 (Secure Boot V2's fixed RSA size).
GEN_ARGS=(generate_signing_key --version 2 --scheme rsa3072)

if [ "$STDOUT_MODE" -eq 1 ]; then
    # Emit the private key on stdout only. espsecure's own log output is
    # redirected to stderr regardless of where it prints, so the PEM stream
    # cannot be polluted. The key exists only in a temp file that is shredded
    # on exit.
    TMPD="$(mktemp -d "${TMPDIR:-/tmp}/spaxel-signkey.XXXXXX")"
    chmod 700 "$TMPD"
    trap 'shred -u "${TMPD}"/* 2>/dev/null; rm -rf "${TMPD}"' EXIT

    KEYFILE="${TMPD}/signing_key.pem"
    "$ESPSECURE" "${GEN_ARGS[@]}" "$KEYFILE" >&2

    if [ -n "$PUBOUT" ]; then
        "$ESPSECURE" extract_public_key --version 2 \
            --keyfile "$KEYFILE" "$PUBOUT" >&2
        echo "public key written to: $PUBOUT" >&2
    fi

    cat "$KEYFILE"
    exit 0
fi

# Dev mode: self-serve local key.
mkdir -p "$OUTPUT_DIR"

KEYFILE="${OUTPUT_DIR}/firmware_signing_key.pem"
echo "Generating Secure Boot V2 signing key (RSA-3072) in: $OUTPUT_DIR"
"$ESPSECURE" "${GEN_ARGS[@]}" "$KEYFILE"

if [ -n "$PUBOUT" ]; then
    "$ESPSECURE" extract_public_key --version 2 --keyfile "$KEYFILE" "$PUBOUT"
    echo "public key written to: $PUBOUT"
fi

echo ""
echo "Key generated: $KEYFILE (PRIVATE - KEEP SECRET)"
echo ""
echo "This is a DEVELOPMENT key. For the production key see the ceremony in"
echo "docs/notes/firmware-signing-keys.md — it is generated once, piped into"
echo "OpenBao, and never stored in a repo path or on a terminal."
echo ""
echo "Bench-board signing:"
echo "  idf.py build    (signs automatically when CONFIG_SECURE_BOOT is enabled)"
echo "  ./scripts/sign-firmware.sh <unsigned.bin> <signed.bin>"
