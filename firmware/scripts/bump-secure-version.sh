#!/usr/bin/env bash
# Bump the firmware anti-rollback secure version.
#
#   ./scripts/bump-secure-version.sh                       # show current state
#   ./scripts/bump-secure-version.sh --reason "why"        # bump by one
#   ./scripts/bump-secure-version.sh --to 4 --reason "why" # bump to a value
#
# The value lives in firmware/SECURE_VERSION and every bump is recorded in
# firmware/SECURE_VERSION_LOG.md. Decreasing is refused outright: nodes burn
# the value into the one-way SECURE_VERSION eFuse field, so a number that
# ever ships is permanent for every node that accepted it. Lowering it by
# hand in the file is possible for a bump that never reached a node, but the
# ledger entry for it has to be written by hand too, on purpose.
#
# Run this from the firmware/ directory or anywhere inside the repository.
# See docs/notes/anti-rollback-secure-version.md for the bump policy.

set -euo pipefail

usage() {
    sed -n '2,12p' "$0" | sed 's/^# \{0,1\}//'
    exit 2
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FIRMWARE_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
VERSION_FILE="${FIRMWARE_DIR}/SECURE_VERSION"
LEDGER_FILE="${FIRMWARE_DIR}/SECURE_VERSION_LOG.md"

REASON=""
TARGET=""

while [ $# -gt 0 ]; do
    case "$1" in
        --reason)
            [ $# -ge 2 ] || usage
            REASON="$2"
            shift 2
            ;;
        --reason=*)
            REASON="${1#--reason=}"
            shift
            ;;
        --to)
            [ $# -ge 2 ] || usage
            TARGET="$2"
            shift 2
            ;;
        --to=*)
            TARGET="${1#--to=}"
            shift
            ;;
        -h|--help)
            usage
            ;;
        *)
            echo "unknown argument: $1" >&2
            usage
            ;;
    esac
done

[ -f "${VERSION_FILE}" ] || { echo "missing ${VERSION_FILE}" >&2; exit 1; }
[ -f "${LEDGER_FILE}" ] || { echo "missing ${LEDGER_FILE}" >&2; exit 1; }

# Strip the policy comments; the remaining text must be one integer.
CURRENT="$(sed 's/#.*//' "${VERSION_FILE}" | tr -d '[:space:]')"

case "${CURRENT}" in
    ''|*[!0-9]*)
        echo "${VERSION_FILE} does not hold a single non-negative integer (got '${CURRENT}')" >&2
        exit 1
        ;;
esac

show() {
    echo "current secure version: ${CURRENT}"
    echo "ledger:                 ${LEDGER_FILE}"
    echo "last entry:             $(grep -E '^\| [0-9]{4}-' "${LEDGER_FILE}" | tail -1 || echo '(none)')"
    exit 0
}

if [ -z "${REASON}" ] && [ -z "${TARGET}" ]; then
    show
fi

if [ -z "${REASON}" ]; then
    echo "--reason is required: the ledger entry is what makes a bump auditable" >&2
    exit 1
fi

if [ -z "${TARGET}" ]; then
    TARGET="$((CURRENT + 1))"
fi

case "${TARGET}" in
    ''|*[!0-9]*)
        echo "--to must be a non-negative integer (got '${TARGET}')" >&2
        exit 1
        ;;
esac

if [ "${TARGET}" -gt 65535 ]; then
    echo "${TARGET} does not fit the 16-bit SECURE_VERSION eFuse field" >&2
    exit 1
fi

if [ "${TARGET}" -lt "${CURRENT}" ]; then
    echo "refusing to lower the secure version (${CURRENT} -> ${TARGET})." >&2
    echo "Nodes burn this value into the SECURE_VERSION eFuse field, which is" >&2
    echo "one-way; a shipped value is permanent for every node that accepted it." >&2
    echo "If this bump never reached a node, edit ${VERSION_FILE} and the ledger" >&2
    echo "by hand and say so in the entry." >&2
    exit 1
fi

if [ "${TARGET}" -eq "${CURRENT}" ]; then
    echo "secure version is already ${CURRENT}; nothing to do" >&2
    exit 1
fi

AUTHOR="$(git -C "${FIRMWARE_DIR}" config user.name 2>/dev/null || echo unknown)"
COMMIT="$(git -C "${FIRMWARE_DIR}" rev-parse --short HEAD 2>/dev/null || echo unknown)"
DATE="$(date -u +%Y-%m-%d)"

# Replace the value line and keep the policy comments above it: the whole
# point of them living in the file is that they travel with the number.
trap 'rm -f "${VERSION_FILE}.tmp"' EXIT
awk -v new="${TARGET}" '
    /^[[:space:]]*(#|$)/ { print; next }
    !replaced { print new; replaced = 1; next }
    { print }
' "${VERSION_FILE}" > "${VERSION_FILE}.tmp"
mv "${VERSION_FILE}.tmp" "${VERSION_FILE}"

# Confirm the file still parses to exactly the value we meant to write.
WRITTEN="$(sed 's/#.*//' "${VERSION_FILE}" | tr -d '[:space:]')"
if [ "${WRITTEN}" != "${TARGET}" ]; then
    echo "${VERSION_FILE} now parses to '${WRITTEN}', expected '${TARGET}'" >&2
    exit 1
fi

printf '| %s | %s | %s | %s | %s | %s |\n' \
    "${DATE}" "${CURRENT}" "${TARGET}" "${REASON}" "${AUTHOR}" "${COMMIT}" >> "${LEDGER_FILE}"

echo "secure version: ${CURRENT} -> ${TARGET}"
echo "recorded in ${LEDGER_FILE}"
echo
echo "Commit both files together, then verify the produced image before it ships:"
echo "  git add firmware/SECURE_VERSION firmware/SECURE_VERSION_LOG.md"
echo "  ./scripts/inspect-firmware-header.py <image>.bin --expect ${TARGET}"
echo
echo "Note: firmware/SECURE_VERSION is not in the CI build trigger set, so this"
echo "change ships with the next push that does trigger a firmware build."
