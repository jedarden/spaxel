#!/usr/bin/env bash
# Device-free exercise of burn-efuses.sh's live-flash verify gate.
#
# Drives verify-live-image.sh — the exact functions the gate runs — over
# synthetic whole-partition dumps: a fixture image signed with a THROWAWAY
# key generated here in a mode-700 /dev/shm directory and shredded on exit.
# No device is touched, and neither the production key (OpenBao) nor the
# bench key (firmware/keys/) is ever read. Covers every documented branch:
# verified, wrong-key refusal, unsigned refusal (including a stray 0xE7 byte
# that must not be mistaken for a signature block), non-image refusal, blank
# skip, the dual-signed rotation shape, and partition enumeration.
#
#   ./scripts/test-verify-live-image.sh [fixture-image]
#
# The fixture defaults to the unsigned app image from a firmware build
# (firmware/build/spaxel-firmware.bin); without one, a minimal ESP image is
# synthesized here. Exit 0 when every branch behaves as documented, 1
# otherwise.

set -euo pipefail

usage() {
    sed -n '2,20p' "$0" | sed 's/^# \{0,1\}//'
    exit 2
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

case "${1:-}" in
    -h | --help) usage ;;
esac

LIB="${SCRIPT_DIR}/verify-live-image.sh"
[ -f "$LIB" ] || { echo "FAIL: missing ${LIB}" >&2; exit 1; }
# shellcheck source-path=SCRIPT_DIR source=verify-live-image.sh disable=SC1091
. "$LIB"

# espsecure.py: PATH first (CI container, sourced IDF export.sh), then the
# pinned IDF 5.2 python env, then any installed env — mirroring
# sign-firmware.sh. burn-efuses.sh itself stays pinned-env-only; this test
# needs the wider search so it can run wherever the firmware builds.
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
    echo "FAIL: espsecure.py not found (on PATH or under ~/.espressif/python_env/)." >&2
    echo "The gate's classification needs it; source an ESP-IDF 5.2 environment" >&2
    echo "or install one, then re-run." >&2
    exit 1
fi

# Throwaway key material lives only here: mode-700, RAM-backed, shredded on
# exit — the same hygiene burn-efuses.sh applies to the production public key.
TMPD="$(mktemp -d /dev/shm/verify-live-image-test.XXXXXX)"
chmod 700 "$TMPD"
trap 'shred -u "${TMPD:?}"/* 2>/dev/null; rm -rf -- "${TMPD:?}"' EXIT

pass=0
fail=0
OUT_LAST=""

run_case() { # run_case <name> <expected_rc> <dump> <label> <pubkey>
    local name="$1" want_rc="$2" dump="$3" label="$4" pubkey="$5"
    local rc=0 out
    out="$(verify_live_dump "$dump" "$label" "$pubkey" "$ESPSECURE" 2>&1 >/dev/null)" || rc=$?
    if [ "$rc" -ne "$want_rc" ]; then
        fail=$((fail + 1))
        printf 'FAIL - %s: expected rc=%d, got rc=%d\n' "$name" "$want_rc" "$rc"
        printf '%s\n' "$out" | sed 's/^/       /'
    else
        pass=$((pass + 1))
        printf 'ok - %s (rc=%d)\n' "$name" "$rc"
    fi
    OUT_LAST="$out"
}

assert_phrase() { # assert_phrase <substring the refusal must contain>
    if printf '%s' "$OUT_LAST" | grep -qF "$1"; then
        pass=$((pass + 1))
        printf 'ok - refusal names it: "%s"\n' "$1"
    else
        fail=$((fail + 1))
        printf 'FAIL - refusal message lacks "%s"\n' "$1"
        printf '%s\n' "$OUT_LAST" | sed 's/^/       got: /'
    fi
}

expect_eq() { # expect_eq <name> <actual> <expected>
    if [ "$2" = "$3" ]; then
        pass=$((pass + 1))
        printf 'ok - %s\n' "$1"
    else
        fail=$((fail + 1))
        printf 'FAIL - %s\n' "$1"
        printf '       expected: %s\n       actual:   %s\n' "$3" "$2"
    fi
}

echo "== throwaway bench keys (shredded on exit; never firmware/keys/, never OpenBao) =="
"$ESPSECURE" generate_signing_key --version 2 --scheme rsa3072 "${TMPD}/bench1.pem" >/dev/null 2>&1
"$ESPSECURE" generate_signing_key --version 2 --scheme rsa3072 "${TMPD}/bench2.pem" >/dev/null 2>&1
"$ESPSECURE" extract_public_key --version 2 --keyfile "${TMPD}/bench1.pem" "${TMPD}/pub1.pem" >/dev/null 2>&1
"$ESPSECURE" extract_public_key --version 2 --keyfile "${TMPD}/bench2.pem" "${TMPD}/pub2.pem" >/dev/null 2>&1

FIXTURE="${1:-}"
if [ -z "$FIXTURE" ] && [ -f "${REPO_ROOT}/firmware/build/spaxel-firmware.bin" ]; then
    FIXTURE="${REPO_ROOT}/firmware/build/spaxel-firmware.bin"
fi
if [ -n "$FIXTURE" ]; then
    cp "$FIXTURE" "${TMPD}/app-unsigned.bin"
    echo "fixture image: ${FIXTURE}"
else
    echo "no build artifact found — synthesizing a minimal ESP image as the fixture"
    python3 - "${TMPD}/app-unsigned.bin" <<'PY'
import struct
import sys

seg = b"SPAXEL-MINIMAL-FIXTURE" + b"\x00" * 10
img = bytes([0xE9, 0x01, 0x01, 0x02]) + struct.pack("<I", 0x40000000)
img += bytes([0xEE]) + b"\x00" * 15  # extended header, wp_pin disabled
img += struct.pack("<II", 0x3FC00000, len(seg)) + seg
img += b"\x00" * ((16 - len(img) % 16) % 16)
img += bytes([sum(img) & 0xFF])
img += b"\x00" * ((16 - len(img) % 16) % 16)
with open(sys.argv[1], "wb") as f:
    f.write(img)
PY
fi

"$ESPSECURE" sign_data --version 2 --keyfile "${TMPD}/bench1.pem" \
    --output "${TMPD}/app-signed1.bin" "${TMPD}/app-unsigned.bin" >/dev/null 2>&1
"$ESPSECURE" sign_data --version 2 --keyfile "${TMPD}/bench2.pem" \
    --output "${TMPD}/app-signed2.bin" "${TMPD}/app-unsigned.bin" >/dev/null 2>&1
# The rotation shape — one signature block per key. Repeated --keyfile does
# NOT produce it (espsecure v4.12 keeps only the last key): --append_signatures
# on a previously signed image is the real two-block form.
"$ESPSECURE" sign_data --version 2 --keyfile "${TMPD}/bench1.pem" \
    --output "${TMPD}/app-dual-base.bin" "${TMPD}/app-unsigned.bin" >/dev/null 2>&1
"$ESPSECURE" sign_data --version 2 --keyfile "${TMPD}/bench2.pem" --append_signatures \
    --output "${TMPD}/app-dual.bin" "${TMPD}/app-dual-base.bin" >/dev/null 2>&1
# Bootloader-shaped fixture: a signed bootloader image padded to the S3
# bootloader region size (0x8000). The build's bootloader when it exists,
# the app fixture otherwise — the branch under test is the dump shape, not
# which image it came from.
BL_UNSIGNED="${REPO_ROOT}/firmware/build/bootloader/bootloader.bin"
[ -f "$BL_UNSIGNED" ] || BL_UNSIGNED="${TMPD}/app-unsigned.bin"
"$ESPSECURE" sign_data --version 2 --keyfile "${TMPD}/bench1.pem" \
    --output "${TMPD}/bl-signed1.bin" "$BL_UNSIGNED" >/dev/null 2>&1

pad_to() { # pad_to <src> <dst> <size> — build a whole-partition dump shape
    python3 - "$1" "$2" "$3" <<'PY'
import sys

src, dst, size = sys.argv[1], sys.argv[2], int(sys.argv[3], 0)
with open(src, "rb") as f:
    data = f.read()
assert len(data) <= size, (src, len(data), size)
with open(dst, "wb") as f:
    f.write(data + b"\xff" * (size - len(data)))
PY
}
pad_to "${TMPD}/app-signed1.bin" "${TMPD}/dump-ok-app.bin" 0x1F0000
pad_to "${TMPD}/bl-signed1.bin" "${TMPD}/dump-ok-bl.bin" 0x8000
pad_to "${TMPD}/app-signed2.bin" "${TMPD}/dump-wrongkey.bin" 0x1F0000
pad_to "${TMPD}/app-unsigned.bin" "${TMPD}/dump-unsigned.bin" 0x1F0000
pad_to "${TMPD}/app-dual.bin" "${TMPD}/dump-dual.bin" 0x1F0000

python3 - "$TMPD" <<'PY'
import sys

tmpd = sys.argv[1]
size = 0x1F0000
# all-erased and all-zero partitions
open(f"{tmpd}/dump-blank-ff.bin", "wb").write(b"\xff" * size)
open(f"{tmpd}/dump-blank-00.bin", "wb").write(b"\x00" * size)
# bytes that are not a firmware image at all
open(f"{tmpd}/dump-noimage.bin", "wb").write(b"\x5a" * size)
# an unsigned image with a stray 0xE7 0x02 planted at a 4096-aligned offset —
# real firmware carries about one such byte per MB, and the digest check is
# what keeps it from being misread as a signature block
with open(f"{tmpd}/dump-unsigned.bin", "r+b") as f:
    data = bytearray(f.read())
    data[0x1000] = 0xE7
    data[0x1001] = 0x02
    f.seek(0)
    f.write(data)
open(f"{tmpd}/dump-stray.bin", "wb").write(bytes(data))
PY

echo
echo "== gate branches =="
run_case "signed app verifies against its key" 0 "${TMPD}/dump-ok-app.bin" "ota_0 (offset 0x20000)" "${TMPD}/pub1.pem"
run_case "bootloader-shaped dump verifies" 0 "${TMPD}/dump-ok-bl.bin" "bootloader (offset 0x0)" "${TMPD}/pub1.pem"
run_case "dual-signed: production key is block 1" 0 "${TMPD}/dump-dual.bin" "ota_0 (offset 0x20000)" "${TMPD}/pub1.pem"
run_case "dual-signed: production key is block 2" 0 "${TMPD}/dump-dual.bin" "ota_0 (offset 0x20000)" "${TMPD}/pub2.pem"
run_case "wrong key refused" 1 "${TMPD}/dump-wrongkey.bin" "ota_0 (offset 0x20000)" "${TMPD}/pub1.pem"
assert_phrase "does NOT verify against the production"
assert_phrase "different key"
run_case "unsigned refused" 1 "${TMPD}/dump-unsigned.bin" "ota_0 (offset 0x20000)" "${TMPD}/pub1.pem"
assert_phrase "unsigned"
run_case "stray 0xE7 not mistaken for a block" 1 "${TMPD}/dump-stray.bin" "ota_0 (offset 0x20000)" "${TMPD}/pub1.pem"
assert_phrase "unsigned"
run_case "non-image bytes refused" 1 "${TMPD}/dump-noimage.bin" "ota_0 (offset 0x20000)" "${TMPD}/pub1.pem"
assert_phrase "ESP image header"
run_case "all-0xFF partition skipped" 3 "${TMPD}/dump-blank-ff.bin" "ota_1 (offset 0x210000)" "${TMPD}/pub1.pem"
run_case "all-0x00 partition skipped" 3 "${TMPD}/dump-blank-00.bin" "ota_1 (offset 0x210000)" "${TMPD}/pub1.pem"

echo
echo "== partition enumeration =="
expected_parts="0x0 0x8000 bootloader
0x20000 0x1F0000 ota_0
0x210000 0x1F0000 ota_1"
actual_parts="$(live_flash_partitions "${REPO_ROOT}/firmware/partitions.csv" | sort)"
expect_eq "repo partitions.csv parses to bootloader + ota_0 + ota_1" \
    "$actual_parts" "$(printf '%s\n' "$expected_parts" | sort)"

fallback_parts="$(live_flash_partitions "${TMPD}/no-such.csv" 2>/dev/null | sort)"
expect_eq "missing csv falls back to documented defaults" \
    "$fallback_parts" "$(printf '%s\n' "$expected_parts" | sort)"

echo
if [ "$fail" -eq 0 ]; then
    echo "PASS: ${pass} checks — every documented verify_live_dump / live_flash_partitions branch behaves as specified"
    exit 0
fi
echo "FAIL: ${fail} of $((pass + fail)) checks failed"
exit 1
