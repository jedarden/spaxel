#!/usr/bin/env bash
# Live-flash image verification helpers for burn-efuses.sh.
#
# Sourced, never executed: no side effects at the top level. Two functions:
#
#   live_flash_partitions [partitions.csv]
#       Emit "<offset> <length> <name>" lines for the flash regions that must
#       hold a prod-signed image: every app partition (parsed from the repo's
#       partitions.csv, so the gate follows the real layout) and the
#       bootloader region, which is fixed on the S3 (0x0..0x8000; the
#       partition table follows at 0x8000). If the CSV is missing or carries
#       no app rows, the documented ota_0/ota_1 defaults are used and a note
#       goes to stderr.
#
#   verify_live_dump <dump> <label> <pubkey> <espsecure>
#       Classify and verify one partition dump — the exact bytes
#       `esptool.py read_flash` returns for a whole partition.
#       rc 0 = verified against <pubkey>; rc 3 = blank (erased), nothing to
#       verify; rc 1 = refused (reason printed to stderr). Every refusal
#       names the failure — unsigned, wrong key, unanalysable — and none of
#       them prints key material.
#
# How verification works (measured behaviour of the pinned ESP-IDF 5.2
# espsecure.py v4.12, 2026-09-13; every branch is exercised device-free by
# test-verify-live-image.sh in this directory):
#
#   * espsecure.py verify_signature / signature_info_v2 accept only datafiles
#     whose size is a non-zero multiple of 4096, and they anchor their
#     signature-block search at (file end - 4096). A whole-partition dump is
#     image + 0xFF padding, so the tools cannot find the block in the raw
#     dump: it has to be trimmed to the image first.
#   * A Secure Boot v2 signature block starts with magic 0xE7, version 0x02,
#     and carries at +4..+35 the SHA-256 of ALL image content before it. The
#     block sits at a 4096-aligned offset — signing pads the content to a
#     sector boundary before appending the signature sector. Those properties
#     locate the real block without trusting anything: a stray 0xE7 in
#     compiled code (about one per MB of firmware) fails the digest check and
#     is ignored, so an unsigned image classifies as unsigned and not as
#     "wrong key".
#   * verify_signature stays the final arbiter: the dump is trimmed to the
#     located block's sector end and handed to it with the public key. It
#     walks block 0 and any blocks chained after it (stride 1216 bytes, the
#     dual/triple-signed rotation shape), so a transitional release passes
#     when the production key is any of its signature blocks.

live_flash_partitions() {
    local csv="${1:-}"
    local name type offset length found=0
    if [ -n "$csv" ] && [ -r "$csv" ]; then
        while IFS=, read -r name type _subtype offset length _rest; do
            name="$(printf '%s' "$name" | tr -d '[:space:]')"
            type="$(printf '%s' "$type" | tr -d '[:space:]')"
            offset="$(printf '%s' "$offset" | tr -d '[:space:]')"
            length="$(printf '%s' "$length" | tr -d '[:space:]')"
            case "$name" in '' | \#*) continue ;; esac
            [ "$type" = "app" ] || continue
            if [[ "$offset" =~ ^0x[0-9a-fA-F]+$ && "$length" =~ ^0x[0-9a-fA-F]+$ ]]; then
                printf '%s %s %s\n' "$offset" "$length" "$name"
                found=1
            fi
        done <"$csv"
    fi
    if [ "$found" -eq 0 ]; then
        if [ -n "$csv" ]; then
            echo "NOTE: no app partitions parsed from $csv — using the documented" >&2
            echo "defaults below; check firmware/partitions.csv." >&2
        fi
        printf '%s\n' "0x20000 0x1F0000 ota_0" "0x210000 0x1F0000 ota_1"
    fi
    # Listed last but verified like the apps: the bootloader is the first
    # thing the ROM verifies, and a blank bootloader makes every later burn a
    # guaranteed brick, so the caller refuses on it rather than skipping.
    printf '%s\n' "0x0 0x8000 bootloader"
}

verify_live_dump() {
    [ $# -eq 4 ] || {
        echo "internal: verify_live_dump expects <dump> <label> <pubkey> <espsecure>" >&2
        return 1
    }
    local dump="$1" label="$2" pubkey="$3" espsecure="$4"
    local probe block_offset trim

    probe="$(python3 - "$dump" <<'LIVE_PROBE_PY'
import hashlib
import sys

with open(sys.argv[1], "rb") as f:
    data = f.read()

# A blank (erased) partition is all 0xFF, or all 0x00 if it was wiped with
# zeros. Nothing to verify in either case.
if not data or set(data) <= {0x00, 0xFF}:
    print("blank")
    raise SystemExit(0)

# Both a bootloader and an app image start with the ESP image magic 0xE9.
if data[:1] != b"\xe9":
    print("noimage")
    raise SystemExit(0)

# Locate the Secure Boot v2 signature block: magic 0xE7, version 0x02, and
# the SHA-256 of all content before it at +4..+35, at a 4096-aligned offset.
# The digest check is what tells a real block apart from a stray 0xE7 byte
# inside compiled code.
for off in range(0, len(data) - 36, 4096):
    if (
        data[off] == 0xE7
        and data[off + 1] == 0x02
        and hashlib.sha256(data[:off]).digest() == data[off + 4 : off + 36]
    ):
        print("block %d" % off)
        raise SystemExit(0)
print("noblock")
LIVE_PROBE_PY
    )" || {
        echo "ERROR: $label: could not analyse the flash dump at $dump" >&2
        return 1
    }

    case "$probe" in
        blank)
            echo "-- $label: empty (erased) — nothing to verify"
            return 3
            ;;
        noimage)
            echo "ERROR: $label: does not start with an ESP image header (0xE9)." >&2
            echo "The bytes in flash are not a firmware image (unsigned or corrupt)," >&2
            echo "so there is nothing to verify. This is not burnable." >&2
            return 1
            ;;
        "block "*)
            block_offset="${probe#"block "}"
            trim="$(mktemp "${dump}.trim.XXXXXX")"
            head -c "$((block_offset + 4096))" "$dump" >"$trim"
            if "$espsecure" verify_signature --version 2 --keyfile "$pubkey" "$trim" >/dev/null 2>&1; then
                rm -f "$trim"
                echo "-- $label: verified against the production key (signature block at 0x$(printf '%x' "$block_offset"))"
                return 0
            fi
            rm -f "$trim"
            echo "ERROR: $label: carries a Secure Boot v2 signature block (at" >&2
            echo "0x$(printf '%x' "$block_offset")) but it does NOT verify against the production" >&2
            echo "key — the image in flash was signed by a different key, or its" >&2
            echo "signature is corrupt. This is not burnable." >&2
            return 1
            ;;
        noblock)
            echo "ERROR: $label: unsigned — no Secure Boot v2 signature block covers" >&2
            echo "the image in flash. This is the expected state until images ship" >&2
            echo "through the production signing pipeline" >&2
            echo "(docs/notes/firmware-signing-keys.md); it is not burnable." >&2
            return 1
            ;;
        *)
            echo "ERROR: $label: unexpected probe output '$probe'" >&2
            return 1
            ;;
    esac
}
