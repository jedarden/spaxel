#!/usr/bin/env bash
# Stage and burn the ESP32-S3 eFuses for Secure Boot V2, key digest
# provisioning, JTAG / download-mode closure and anti-rollback.
#
# Implements the burn order in docs/notes/esp32s3-efuse-provisioning.md:
# every stage that closes a recovery path runs after the node has a
# known-good prod-signed image, and SECURE_BOOT_EN comes last.
#
#   ./scripts/burn-efuses.sh <image>                       # dry-run plan (default)
#   ./scripts/burn-efuses.sh --check-only <image>          # preflight + stage (a), no device
#   ./scripts/burn-efuses.sh --burn --port /dev/ttyACM0 <image>
#
# Modes:
#   (default)   print the ordered plan — every command that would run and what
#               is irreversible about it. No device, no OpenBao fetch, no burns.
#   --check-only  run preflight + stage (a) only: fetch the production PUBLIC
#               key by reference, verify the image against it, exit. Burns
#               nothing, touches no device — the "is this image the one I
#               should provision?" gate.
#   --burn      execute the selected stages in order against --port.
#
# Flags:
#   --port DEV                 serial port (required for --burn)
#   --stage LIST               comma-separated subset of
#                              preflight,a,b,c,d,e (canonical order enforced)
#   --pubkey FILE              use this public PEM instead of fetching
#                              public_key_pem from OpenBao
#   --secure-version N         override the SECURE_VERSION burned in stage (d)
#                              (default: read from the image header)
#   --allow-dis-download-mode  include DIS_DOWNLOAD_MODE in stage (c). This is
#                              the point of no return for flashing: after it a
#                              node updates only by signed OTA or by physical
#                              replacement. Fleet decision, not this script's.
#   --do-not-confirm           pass --do-not-confirm to espefuse.py. Without it
#                              espefuse prompts BURN before every burn, which
#                              also means a non-interactive run aborts — the
#                              safe default.
#   -h|--help                  this text
#
# Exit codes: 0 success (a dry-run plan is a success), 1 failure, 2 usage.
#
# KEY HANDLING — the private key is never fetched by this script. Everything
# it does (signature verification and the key digest) works from the PUBLIC
# half, and espsecure.py's digest_sbv2_public_key accepts a public PEM
# directly. The prod key comes from OpenBao by reference, never by value:
#
#   bao-as openbao-v2 bao kv get -field=public_key_pem \
#       secret/ardenone-cluster/spaxel/firmware-signing/prod
#
# Key material never appears in argv, in a log line, in an error message or in
# this script's output under any flag; the only file it is ever written to is
# a mode-700 directory under /dev/shm, shredded on exit (including on failure).
# espefuse.py's --show-sensitive-info is deliberately never passed.
#
# TOOLS — this script runs espefuse.py / espsecure.py from the pinned ESP-IDF
# 5.2 python environment (~/.espressif/python_env/idf5.2*), the same
# environment docs/notes/firmware-signing-keys.md requires for espsecure.py.
# It deliberately does NOT fall back to whatever is on PATH or to another
# idf version's env: eFuse names, key purposes and the secure_version field
# are version-specific, so a wrong tool version fails loudly here instead of
# burning the wrong thing.
#
# NOT VALIDATED ON HARDWARE. The bench bead spaxel-6c9344e4 is deferred and
# ex44 has no flashing host, so no stage of this script has run against a real
# ESP32-S3. Treat the first bench run as the validation gate: diff
# `espefuse.py summary` before and after each stage and attach both dumps to
# the bench record. See docs/notes/esp32s3-efuse-provisioning.md.

set -euo pipefail

usage() {
    sed -n '2,67p' "$0" | sed 's/^# \{0,1\}//'
    exit 2
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REFERENCE_DOC="docs/notes/esp32s3-efuse-provisioning.md"
BAO_PATH="secret/ardenone-cluster/spaxel/firmware-signing/prod"
BAO_FETCH=(bao-as openbao-v2 bao kv get -field=public_key_pem "$BAO_PATH")

MODE="plan"
PORT=""
IMAGE=""
PUBKEY_FILE=""
SECURE_VERSION=""
STAGES="preflight,a,b,c,d,e"
ALLOW_DIS_DOWNLOAD_MODE=0
DO_NOT_CONFIRM=0

while [ $# -gt 0 ]; do
    case "$1" in
        --burn) MODE="burn"; shift ;;
        --check-only) MODE="check"; shift ;;
        --port)
            [ $# -ge 2 ] || usage
            PORT="$2"; shift 2
            ;;
        --port=*) PORT="${1#--port=}"; shift ;;
        --stage)
            [ $# -ge 2 ] || usage
            STAGES="$2"; shift 2
            ;;
        --stage=*) STAGES="${1#--stage=}"; shift ;;
        --pubkey)
            [ $# -ge 2 ] || usage
            PUBKEY_FILE="$2"; shift 2
            ;;
        --pubkey=*) PUBKEY_FILE="${1#--pubkey=}"; shift ;;
        --secure-version)
            [ $# -ge 2 ] || usage
            SECURE_VERSION="$2"; shift 2
            ;;
        --secure-version=*) SECURE_VERSION="${1#--secure-version=}"; shift ;;
        --allow-dis-download-mode) ALLOW_DIS_DOWNLOAD_MODE=1; shift ;;
        --do-not-confirm) DO_NOT_CONFIRM=1; shift ;;
        -h|--help) usage ;;
        --*) echo "unknown option: $1" >&2; usage ;;
        *)
            if [ -z "$IMAGE" ]; then
                IMAGE="$1"
            else
                echo "unexpected argument: $1" >&2
                usage
            fi
            shift
            ;;
    esac
done

[ -n "$IMAGE" ] || { echo "an <image> argument is required" >&2; usage; }
[ -f "$IMAGE" ] || { echo "ERROR: image not found: $IMAGE" >&2; exit 1; }

# --stage is a subset selector, never a reordering: the canonical order is the
# safety property, so a subset is always executed in canonical order.
ALL_STAGES=(preflight a b c d e)
WANT=()
for wanted in $(printf '%s' "$STAGES" | tr ',' ' '); do
    found=""
    for s in "${ALL_STAGES[@]}"; do
        [ "$wanted" = "$s" ] && found=1 && break
    done
    [ -n "$found" ] || { echo "ERROR: unknown stage '$wanted' (known: ${ALL_STAGES[*]})" >&2; exit 2; }
    duplicate=""
    for s in "${WANT[@]:-}"; do [ "$wanted" = "$s" ] && duplicate=1 && break; done
    [ -z "$duplicate" ] || { echo "ERROR: stage '$wanted' listed twice" >&2; exit 2; }
    WANT+=("$wanted")
done
[ "${#WANT[@]}" -gt 0 ] || { echo "ERROR: --stage selected nothing" >&2; exit 2; }

wants() {
    local s
    for s in "${WANT[@]}"; do [ "$s" = "$1" ] && return 0; done
    return 1
}

if [ "$MODE" = "burn" ]; then
    [ -n "$PORT" ] || { echo "ERROR: --burn requires --port DEV" >&2; exit 2; }
    [ -e "$PORT" ] || { echo "ERROR: serial port not found: $PORT" >&2; exit 1; }
fi
# --check-only has no device, so the espefuse stages cannot run in it.
if [ "$MODE" = "check" ]; then
    for s in b c d e; do
        if wants "$s"; then
            echo "ERROR: --check-only runs preflight and stage (a) only; stage '$s' needs --burn" >&2
            exit 2
        fi
    done
fi

# ---------------------------------------------------------------------------
# Tools — pinned ESP-IDF 5.2 environment only, no PATH fallback (see header).

find_idf5_2_tool() {
    local name="$1" candidate
    for candidate in "${HOME}"/.espressif/python_env/idf5.2*/bin/"$name"; do
        if [ -x "$candidate" ]; then
            printf '%s\n' "$candidate"
            return 0
        fi
    done
    return 1
}

ESPSECURE=""
ESPEFUSE=""
if [ "$MODE" = "plan" ]; then
    # The plan still names the exact binaries it would use, and says so when
    # they are absent, but absence is only fatal when something would run.
    ESPSECURE="$(find_idf5_2_tool espsecure.py || true)"
    ESPEFUSE="$(find_idf5_2_tool espefuse.py || true)"
else
    if ! ESPSECURE="$(find_idf5_2_tool espsecure.py)"; then
        echo "ERROR: espsecure.py not found in the pinned ESP-IDF 5.2 environment" >&2
        echo "" >&2
        echo "Expected under ~/.espressif/python_env/idf5.2*/bin/ — the same" >&2
        echo "environment docs/notes/firmware-signing-keys.md requires. This" >&2
        echo "script will not use a tool from PATH or from another idf version's" >&2
        echo "environment: eFuse names, key purposes and the secure_version field" >&2
        echo "are version-specific." >&2
        exit 1
    fi
    if [ "$MODE" = "burn" ] && ! ESPEFUSE="$(find_idf5_2_tool espefuse.py)"; then
        echo "ERROR: espefuse.py not found in the pinned ESP-IDF 5.2 environment" >&2
        echo "" >&2
        echo "Expected under ~/.espressif/python_env/idf5.2*/bin/. Burning eFuses" >&2
        echo "with a mismatched espefuse version risks resolving the wrong eFuse" >&2
        echo "names or bit positions for this chip." >&2
        exit 1
    fi
fi

# The header inspector is stdlib-only and deliberately runs on the system
# python3, not the IDF env (see its module docstring).
INSP="${SCRIPT_DIR}/inspect-firmware-header.py"
[ -f "$INSP" ] || { echo "ERROR: missing ${SCRIPT_DIR}/inspect-firmware-header.py" >&2; exit 1; }

ESPEFUSE_GLOBALS=(--chip esp32s3)
# Not the "[ ] && " form: with set -e a false test there exits the script.
if [ -n "$PORT" ]; then
    ESPEFUSE_GLOBALS+=(--port "$PORT")
fi
if [ "$DO_NOT_CONFIRM" -eq 1 ]; then
    ESPEFUSE_GLOBALS+=(--do-not-confirm)
fi

announce() { printf '\n=== stage %s ===\n%s\n' "$1" "$2"; }

stage_note_a() {
    cat <<'EOF'
  Would verify the target image against the production public key:
    espsecure.py verify_signature --version 2 --keyfile <public_key_pem> <image>
  and print the image's signature blocks and secure_version.
  Nothing is burned in this stage. A failed verification aborts the run
  before any eFuse is touched.
EOF
}

stage_note_b() {
    cat <<'EOF'
  Would provision the production key digest into a key block:
    espefuse.py --chip esp32s3 --port <port> burn_key_digest BLOCK_KEY0 <public_key_pem> SECURE_BOOT_DIGEST0
  IRREVERSIBLE: consumes 1 of the 3 trusted-digest slots the chip will ever
  have (SECURE_BOOT_DIGEST0/1/2), and write-protects BLOCK_KEY0 and
  KEY_PURPOSE_0 as part of the burn. The digest must be in place before any
  CONFIG_SECURE_BOOT=y image boots on this node.
EOF
}

stage_note_c() {
    cat <<'EOF'
  Would close JTAG and the download modes:
    espefuse.py --chip esp32s3 --port <port> burn_efuse DIS_PAD_JTAG
    espefuse.py --chip esp32s3 --port <port> burn_efuse DIS_USB_JTAG
    espefuse.py --chip esp32s3 --port <port> burn_efuse DIS_USB_SERIAL_JTAG_DOWNLOAD_MODE
    espefuse.py --chip esp32s3 --port <port> burn_efuse DIS_USB_OTG_DOWNLOAD_MODE
    espefuse.py --chip esp32s3 --port <port> burn_efuse DIS_DIRECT_BOOT
  IRREVERSIBLE: every bit above is write-once. DIS_USB_SERIAL_JTAG is
  deliberately NOT in this set — the fleet's only console is USB-Serial-JTAG
  (verify-console-config.sh usb), and burning that bit destroys it.
EOF
    if [ "$ALLOW_DIS_DOWNLOAD_MODE" -eq 1 ]; then
        cat <<'EOF'
  --allow-dis-download-mode given, so it would also burn:
    espefuse.py --chip esp32s3 --port <port> burn_efuse DIS_DOWNLOAD_MODE
  IRREVERSIBLE and the point of no return for flashing: after it the node
  updates only by a signed image it already trusts (OTA) or by physical
  replacement.
EOF
    else
        cat <<'EOF'
  DIS_DOWNLOAD_MODE is NOT included (pass --allow-dis-download-mode to add
  it). It is the bit that closes the 2026-08-07 extraction path for good,
  and the fleet decision about it belongs outside this script.
EOF
    fi
}

stage_note_d() {
    cat <<'EOF'
  Would burn the anti-rollback counter:
    espefuse.py --chip esp32s3 --port <port> burn_efuse SECURE_VERSION <N>
  where N is the secure_version the flashed image carries. The field is
  16 bits and only moves upward. IRREVERSIBLE in the sense that it can never
  be lowered: an N above the fleet's images bricks the nodes that receive
  them. SECURE_VERSION is deliberately NOT write-protected — the OTA install
  path must stay able to advance it (see anti-rollback-secure-version.md).
EOF
}

stage_note_e() {
    cat <<'EOF'
  Would enable Secure Boot, last of all:
    espefuse.py --chip esp32s3 --port <port> burn_efuse SECURE_BOOT_EN
  IRREVERSIBLE and final: from this boot the ROM verifies the bootloader and
  the bootloader verifies every app, against the digest provisioned in stage
  (b). There is no un-burn. A node in this state that rejects its own image
  does not boot, and cannot be re-flashed if stage (c) also landed. That is
  the protection working.
EOF
}

# ---------------------------------------------------------------------------
# Key by reference.

fetch_public_key() {
    # PUBKEY_OUT is always a file inside TMPD (mode 700, /dev/shm), shredded
    # by the exit trap. The private half (key_pem) is never fetched: signature
    # verification and the key digest both work from the public half.
    if [ -n "$PUBKEY_FILE" ]; then
        [ -f "$PUBKEY_FILE" ] || { echo "ERROR: --pubkey file not found: $PUBKEY_FILE" >&2; exit 1; }
        cp -- "$PUBKEY_FILE" "$PUBKEY_OUT"
    else
        echo "Fetching the production PUBLIC key from OpenBao (path only, value never displayed):"
        printf '  %s\n' "${BAO_FETCH[*]}"
        if ! "${BAO_FETCH[@]}" > "$PUBKEY_OUT"; then
            echo "ERROR: could not fetch public_key_pem from OpenBao ($BAO_PATH)." >&2
            echo "Check bao-as openbao-v2 works in this shell (see the workspace secrets" >&2
            echo "policy). Nothing was burned." >&2
            exit 1
        fi
    fi
    if [ ! -s "$PUBKEY_OUT" ]; then
        echo "ERROR: fetched key material is empty — refusing to continue." >&2
        exit 1
    fi
    # Property check, never a read-back: does it parse as a public key?
    if ! openssl pkey -pubin -in "$PUBKEY_OUT" -noout >/dev/null 2>&1; then
        echo "ERROR: fetched material does not parse as a public key — refusing to" >&2
        echo "continue. Nothing was burned." >&2
        exit 1
    fi
}

# ---------------------------------------------------------------------------
# TMPD — the only place key material is ever written, and only in /dev/shm.

TMPD=""
PUBKEY_OUT=""
cleanup() {
    if [ -n "$TMPD" ] && [ -d "$TMPD" ]; then
        shred -u "${TMPD:?}"/* 2>/dev/null
        rm -rf -- "${TMPD:?}"
    fi
}
trap cleanup EXIT

if [ "$MODE" != "plan" ]; then
    TMPD="$(mktemp -d /dev/shm/burn-efuses.XXXXXX)"
    chmod 700 "$TMPD"
    PUBKEY_OUT="${TMPD}/prod-pub.pem"
fi

echo "burn-efuses.sh — mode: $MODE, stages: ${WANT[*]}"
echo "image: $IMAGE"
echo ""
echo "NOT VALIDATED ON HARDWARE: bench spaxel-6c9344e4 is deferred and ex44 has"
echo "no flashing host, so this script has never run against a real ESP32-S3."
echo "Treat the first bench run as the validation gate ($REFERENCE_DOC)."
echo "The burn order below is a safety property: SECURE_BOOT_EN comes last."

# ---------------------------------------------------------------------------
# preflight

if wants preflight; then
    announce preflight "Read-only checks. Burns nothing."
    echo "espsecure.py: ${ESPSECURE:-<pinned ESP-IDF 5.2 environment not found>}"
    echo "espefuse.py:  ${ESPEFUSE:-<pinned ESP-IDF 5.2 environment not found>}"
    if [ "$MODE" = "plan" ]; then
        echo "Would fetch the production public key by reference:"
        printf '  %s\n' "${BAO_FETCH[*]}"
        echo "  -> into a mode-700 directory under /dev/shm, shredded on exit."
    else
        fetch_public_key
    fi
    echo ""
    echo "Signature blocks in the image (read-only):"
    if [ -n "$ESPSECURE" ]; then
        "$ESPSECURE" signature_info_v2 "$IMAGE" || {
            echo "ERROR: could not read signature blocks from $IMAGE — is it a" >&2
            echo "signed app image (sign-firmware.sh output), not a merged flash" >&2
            echo "image?" >&2
            exit 1
        }
    else
        echo "  <skipped: espsecure.py not found>"
    fi
    echo ""
    echo "Anti-rollback fields in the image (read-only):"
    "$INSP" "$IMAGE"
    if [ "$MODE" = "burn" ]; then
        echo ""
        echo "Baseline eFuse state — save this output with the node's record:"
        "$ESPEFUSE" "${ESPEFUSE_GLOBALS[@]}" summary
    fi
fi

# ---------------------------------------------------------------------------
# stage (a) — the image is signed by the production key

if wants a; then
    announce a "Verify the target image is prod-signed. Burns nothing."
    if [ "$MODE" = "plan" ]; then
        stage_note_a
    else
        if [ -z "$PUBKEY_OUT" ]; then
            fetch_public_key
        fi
        if ! "$ESPSECURE" verify_signature --version 2 --keyfile "$PUBKEY_OUT" "$IMAGE"; then
            echo "" >&2
            echo "ERROR: $IMAGE is NOT signed by the production key in $BAO_PATH." >&2
            echo "Refusing to continue: provisioning the digest of a key that did not" >&2
            echo "sign the running image strands the node on an image it cannot trust." >&2
            echo "Nothing was burned." >&2
            exit 1
        fi
        echo "Verified: $IMAGE carries a signature from the production key."
    fi
fi

# ---------------------------------------------------------------------------
# stage (b) — key digest provisioning

if wants b; then
    announce b "Provision the production key digest (BLOCK_KEY0 / SECURE_BOOT_DIGEST0)."
    stage_note_b
    if [ "$MODE" = "burn" ]; then
        [ -n "$PUBKEY_OUT" ] || fetch_public_key
        "$ESPEFUSE" "${ESPEFUSE_GLOBALS[@]}" burn_key_digest BLOCK_KEY0 "$PUBKEY_OUT" SECURE_BOOT_DIGEST0
    fi
fi

# ---------------------------------------------------------------------------
# stage (c) — JTAG and download-mode closure

if wants c; then
    announce c "Close JTAG and the download modes."
    stage_note_c
    if [ "$MODE" = "burn" ]; then
        for efuse in DIS_PAD_JTAG DIS_USB_JTAG DIS_USB_SERIAL_JTAG_DOWNLOAD_MODE \
                     DIS_USB_OTG_DOWNLOAD_MODE DIS_DIRECT_BOOT; do
            echo "-- burning $efuse"
            "$ESPEFUSE" "${ESPEFUSE_GLOBALS[@]}" burn_efuse "$efuse"
        done
        if [ "$ALLOW_DIS_DOWNLOAD_MODE" -eq 1 ]; then
            echo "-- burning DIS_DOWNLOAD_MODE (point of no return for flashing)"
            "$ESPEFUSE" "${ESPEFUSE_GLOBALS[@]}" burn_efuse DIS_DOWNLOAD_MODE
        else
            echo "-- DIS_DOWNLOAD_MODE skipped (--allow-dis-download-mode not given)"
        fi
    fi
fi

# ---------------------------------------------------------------------------
# stage (d) — anti-rollback SECURE_VERSION

if wants d; then
    announce d "Burn the anti-rollback counter (SECURE_VERSION)."
    stage_note_d
    SEC_VER="$SECURE_VERSION"
    if [ -z "$SEC_VER" ]; then
        SEC_VER="$("$INSP" --json "$IMAGE" | python3 -c 'import json,sys; print(json.load(sys.stdin)["header_secure_version"])')"
        echo "Image carries secure_version: $SEC_VER"
    fi
    case "$SEC_VER" in
        ''|*[!0-9]*)
            echo "ERROR: SECURE_VERSION must be an integer (got '$SEC_VER'). Use" >&2
            echo "--secure-version N to set it explicitly." >&2
            exit 1
            ;;
    esac
    if [ "$SEC_VER" -eq 0 ]; then
        echo "ERROR: refusing to burn SECURE_VERSION 0 — the field starts at 0, and" >&2
        echo "0 is exactly what this fleet's images silently carried until" >&2
        echo "2026-09-05 (see anti-rollback-secure-version.md). Pass --secure-version" >&2
        echo "N with the value the image actually carries." >&2
        exit 1
    fi
    echo "Would burn (or burning): SECURE_VERSION = $SEC_VER"
    if [ "$MODE" = "burn" ]; then
        "$ESPEFUSE" "${ESPEFUSE_GLOBALS[@]}" burn_efuse SECURE_VERSION "$SEC_VER"
    fi
fi

# ---------------------------------------------------------------------------
# stage (e) — SECURE_BOOT_EN, last

if wants e; then
    announce e "Enable Secure Boot (SECURE_BOOT_EN) — last, irreversible, final."
    stage_note_e
    if [ "$MODE" = "burn" ]; then
        if wants b; then
            echo "Digest was provisioned in this run (stage b)."
        else
            echo "Stage (b) is not in this run — checking the chip for a provisioned" >&2
            echo "digest before the point of no return:" >&2
            if ! "$ESPEFUSE" "${ESPEFUSE_GLOBALS[@]}" summary | grep -q SECURE_BOOT_DIGEST; then
                echo "" >&2
                echo "ERROR: no SECURE_BOOT_DIGEST purpose is visible in the output of" >&2
                echo "'espefuse.py summary' — the production key digest is not provisioned" >&2
                echo "node. Burning SECURE_BOOT_EN now produces a node that verifies" >&2
                echo "against nothing and does not boot. Refusing. Run stage (b) first" >&2
                echo "(or include it in the same --stage list as (e))." >&2
                exit 1
            fi
            echo "Digest found on the chip — continuing."
        fi
        "$ESPEFUSE" "${ESPEFUSE_GLOBALS[@]}" burn_efuse SECURE_BOOT_EN
        echo ""
        echo "Post-burn state — save this with the node's record:"
        "$ESPEFUSE" "${ESPEFUSE_GLOBALS[@]}" summary
    fi
fi

echo ""
case "$MODE" in
    plan)
        echo "DRY RUN — nothing was fetched, nothing was burned."
        ;;
    check)
        echo "CHECK ONLY — preflight and stage (a) ran; no device was touched, no"
        echo "eFuse was burned."
        ;;
    burn)
        echo "BURN COMPLETE — diff the final 'espefuse.py summary' output against the"
        echo "baseline printed in preflight and attach both to the node's record."
        ;;
esac
echo "Hardware validation is still pending (bench spaxel-6c9344e4, deferred): a"
echo "green run here is not a validated provisioning."
