#!/usr/bin/env bash
# check-fusion-timing-thresholds.sh — reconcile the 15/30/40 ms fusion timing
# thresholds between this repo and the spaxel-build timing gate.
#
# The thresholds live in two places that can drift:
#
#   repo side : the canonical marker in docs/ci-benchmark-integration.md and
#               the constants in mothership/internal/localizer/fusion/
#               timing_budget_test.go. Agreement between those two is enforced
#               automatically by TestThresholdsSingleSourced
#               (ci_gate_contract_test.go) in the go-test CI leg.
#   gate side : the ci_threshold=/hard_limit= assignments in the
#               timing-benchmark step of the spaxel-build WorkflowTemplate
#               (jedarden/declarative-config, live on iad-ci).
#
# This script is the template-vs-repo half of the reconciliation: it extracts
# the gate's assignments and compares them to the repo's marker, failing
# loudly (exit 1) on a one-sided threshold change instead of leaving the
# drift to a review. Run it after changing a threshold on either side.
#
# Usage:
#   scripts/check-fusion-timing-thresholds.sh               # vs the live template on iad-ci (read-only get)
#   scripts/check-fusion-timing-thresholds.sh <manifest>    # vs a declarative-config checkout file
#
# Exit codes: 0 = agree, 1 = DRIFT (or the marker convention itself is
# broken), 2 = could not check (missing kubectl/cluster/file — distinct from
# a checked-and-failed result so callers can tell the two apart).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DOC="$REPO_ROOT/docs/ci-benchmark-integration.md"
KUBECTL_SERVER="${KUBECTL_SERVER:-http://traefik-iad-ci:8001}"
TEMPLATE_NAME="${TEMPLATE_NAME:-spaxel-build}"
TEMPLATE_NAMESPACE="${TEMPLATE_NAMESPACE:-argo-workflows}"
# The WorkflowTemplate object (TEMPLATE_NAME) contains several step templates;
# the gate's assignments live in the one named STEP_NAME.
STEP_NAME="${STEP_NAME:-timing-benchmark}"

MARKER_RE='spaxel-fusion-timing-thresholds:[[:space:]]+production_ms=[0-9]+[[:space:]]+ci_ms=[0-9]+[[:space:]]+hard_ms=[0-9]+'

die()  { echo "FAIL: $*" >&2; exit 1; }
cant() { echo "CANNOT CHECK: $*" >&2; exit 2; }

# --- repo side: the canonical marker -------------------------------------
[ -f "$DOC" ] || cant "doc not found: $DOC"
marker_lines=$(grep -Ec "$MARKER_RE" "$DOC" || true)
[ "$marker_lines" -eq 1 ] ||
	die "expected exactly one canonical threshold marker line ('spaxel-fusion-timing-thresholds: production_ms=<n> ci_ms=<n> hard_ms=<n>') in docs/ci-benchmark-integration.md, found '${marker_lines}' — the single-sourcing convention itself is broken"
marker=$(grep -Eo "$MARKER_RE" "$DOC")
prod_ms=$(sed -E 's/.*production_ms=([0-9]+).*/\1/' <<<"$marker")
ci_ms=$(sed -E 's/.*ci_ms=([0-9]+).*/\1/' <<<"$marker")
hard_ms=$(sed -E 's/.*hard_ms=([0-9]+).*/\1/' <<<"$marker")

# --- gate side: ci_threshold= / hard_limit= in the timing-benchmark step --
if [ $# -ge 1 ]; then
	[ -f "$1" ] || cant "manifest not found: $1"
	gate_body="$(cat "$1")"
	gate_src="manifest $1"
else
	command -v kubectl >/dev/null 2>&1 ||
		cant "kubectl not on PATH and no manifest argument given"
	gate_json="$(kubectl --server="$KUBECTL_SERVER" get workflowtemplate "$TEMPLATE_NAME" -n "$TEMPLATE_NAMESPACE" -o json 2>/dev/null)" ||
		cant "could not fetch live template ${TEMPLATE_NAME} from ${KUBECTL_SERVER}"
	gate_src="live template ${TEMPLATE_NAME} (${KUBECTL_SERVER})"
	# Structural extraction, scoped to the gate step. Grepping the raw JSON is
	# ambiguous: kubectl's last-applied-configuration annotation embeds a
	# second copy of the whole manifest. Walk the body-bearing paths
	# (container.args, container.command, script.source) instead — a miss on
	# one path is normal for container- vs script-shaped steps, so collect
	# from all of them.
	gate_body="$(printf '%s' "$gate_json" | STEP_NAME="$STEP_NAME" python3 -c '
import json, os, sys
name = os.environ["STEP_NAME"]
w = json.load(sys.stdin)
chunks = []
for t in w.get("spec", {}).get("templates", []):
    if t.get("name") != name:
        continue
    c = t.get("container") or {}
    chunks.extend(a for a in (c.get("args") or []) if isinstance(a, str))
    if c.get("command"):
        chunks.append(" ".join(x for x in c["command"] if isinstance(x, str)))
    if t.get("script"):
        chunks.append(t["script"].get("source") or "")
print("\n".join(chunks))
')" || cant "could not parse the live template JSON"
	[ -n "$gate_body" ] ||
		cant "no ${TEMPLATE_NAME}/${STEP_NAME} body found in ${gate_src} — dump the object and walk every body-bearing path (container.args, script.source) before believing it is missing"
fi

ci_gate_lines=$(printf '%s\n' "$gate_body" | grep -Ec 'ci_threshold=[0-9]+' || true)
hard_gate_lines=$(printf '%s\n' "$gate_body" | grep -Ec 'hard_limit=[0-9]+' || true)
[ "$ci_gate_lines" -eq 1 ] && [ "$hard_gate_lines" -eq 1 ] ||
	die "expected exactly one ci_threshold= and one hard_limit= assignment in the ${TEMPLATE_NAME}/${STEP_NAME} step (${gate_src}), found ci=${ci_gate_lines} hard=${hard_gate_lines} — the gate-side half of the convention is broken"
ci_gate=$(printf '%s\n' "$gate_body" | grep -Eo 'ci_threshold=[0-9]+' | grep -Eo '[0-9]+')
hard_gate=$(printf '%s\n' "$gate_body" | grep -Eo 'hard_limit=[0-9]+' | grep -Eo '[0-9]+')

# --- compare ---------------------------------------------------------------
status=0
[ "$ci_gate" = "$ci_ms" ] ||
	{ echo "FAIL: threshold drift — repo marker ci_ms=${ci_ms} != gate ci_threshold=${ci_gate} (${gate_src})"; status=1; }
[ "$hard_gate" = "$hard_ms" ] ||
	{ echo "FAIL: threshold drift — repo marker hard_ms=${hard_ms} != gate hard_limit=${hard_gate} (${gate_src})"; status=1; }
if [ "$status" -eq 1 ]; then
	echo "One side changed its timing thresholds without the other. Update both to the same values (see docs/ci-benchmark-integration.md, 'Single-sourcing the thresholds')." >&2
	exit 1
fi
echo "PASS: fusion timing thresholds agree — repo marker (ci_ms=${ci_ms} hard_ms=${hard_ms}) == ${gate_src} (ci_threshold=${ci_gate} hard_limit=${hard_gate})"
echo "      (marker production_ms=${prod_ms} is repo-only: the gate does not enforce the production target)"
