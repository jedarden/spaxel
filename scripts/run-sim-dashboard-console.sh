#!/usr/bin/env bash
#
# run-sim-dashboard-console.sh — bf-4do5y: live-browser backward-compat check.
#
# Final live-app acceptance for parent bf-2gmx ("Verify backward compatibility
# for blob identity fields"). It is the live-browser twin of bf-20cl7 (the jsdom
# renderer proof) and the NOT-resolved twin of bf-15oi (the identity-RESOLVED
# mothership log): it serves the REAL dashboard against a hardware-free sim and
# captures the browser console to prove identity-less blobs render with NO
# console errors (no personName/assignedColor/identityResolved undefined access),
# then repeats with --ble + a registered person to prove identity-resolved blobs
# still render WITH their identity.
#
# Two phases, two live browser captures:
#   1. identity-less : SIM_BLE=0  -> /api/blobs carries NO identity fields.
#                      Dashboard (ambient + live) must load, render blobs with
#                      the fallback color (#6b7280), and keep the console clean.
#   2. identity      : SIM_BLE=1 + registered person "Alice".
#                      /api/blobs carries personName/assignedColor/identityResolved.
#                      Dashboard must render WITH identity and keep the console clean.
#
# No hardware, no Docker, no manual IP/token. Reuses build + health + token path
# from run-sim-local.sh. Writes everything under CAPTURE_DIR (default
# docs/notes/bf-4do5y-runtime-capture) for the evidence trace.
#
# Usage:  CAPTURE_DIR=... ./scripts/run-sim-dashboard-console.sh
# Env:    SIM_PORT, SIM_NODES, SIM_WALKERS, SIM_RATE, SIM_DURATION, SIM_SEED,
#         SIM_SPACE, SPAXEL_MOTHERSHIP_BIN, SPAXEL_SIM_BIN
# Exit:   0 if BOTH phases have a clean console (0 identity-related hits) and
#         identity-less blobs render; 1 otherwise.

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MS="${SPAXEL_MOTHERSHIP_BIN:-/tmp/spaxel-mothership}"
SIM="${SPAXEL_SIM_BIN:-/tmp/spaxel-sim}"
PORT="${SIM_PORT:-8088}"
CAPTURE_DIR="${CAPTURE_DIR:-$ROOT/docs/notes/bf-4do5y-runtime-capture}"

SIM_NODES="${SIM_NODES:-4}"
SIM_WALKERS="${SIM_WALKERS:-2}"
SIM_RATE="${SIM_RATE:-30}"
SIM_DURATION="${SIM_DURATION:-40}"   # long enough to keep blobs live during the browser capture
SIM_SEED="${SIM_SEED:-42}"
SIM_SPACE="${SIM_SPACE:-5x5x2.5}"

# identity-resolved phase fixture (same defaults as run-sim-identity.sh)
WALKER_MAC="${SIM_WALKER_MAC:-AA:BB:CC:DD:EE:00}"
PERSON_NAME="${SIM_PERSON_NAME:-Alice}"
PERSON_COLOR="${SIM_PERSON_COLOR:-#4488ff}"

# Playwright capture target pages (ambient = Canvas 2D hardened path; live = viz3d/WebGL).
PAGES="${PAGES:-/ambient,/live}"

log() { printf '[dash-console] %s\n' "$*"; }
die() { printf '[dash-console] ERROR: %s\n' "$*" >&2; exit 1; }

for dep in go curl jq node; do
  command -v "$dep" >/dev/null 2>&1 || die "missing required dependency: $dep"
done

mkdir -p "$CAPTURE_DIR"

# --- Build both binaries from source (idempotent: reuse if present) -------------
log "building canonical binaries from source (reuse if present)..."
need_build=0
[ -x "$MS" ]  || need_build=1
[ -x "$SIM" ] || need_build=1
if [ -x "$SIM" ] && ! "$SIM" --help 2>&1 | grep -q -- '--verify'; then
  log "existing $SIM lacks --verify (stale/standalone build); rebuilding..."
  rm -f "$SIM"; need_build=1
fi
if [ "$need_build" -eq 1 ]; then
  ( cd "$ROOT/mothership" || exit 1
    go build -o "$MS"  ./cmd/mothership || die "building mothership"
    go build -o "$SIM" ./cmd/sim         || die "building sim"
  ) || exit 1
fi

DATA_DIR=$(mktemp -d -t spaxel-dashconsole-data-XXXXXX)
MS_PID=""
SIM_PID=""
PHASE=""

cleanup() {
  log "cleanup: stopping sim+mothership (phase=${PHASE:-none})"
  [ -n "$SIM_PID" ] && kill "$SIM_PID" 2>/dev/null || true
  [ -n "$MS_PID" ]  && kill -INT "$MS_PID" 2>/dev/null || true
  sleep 1
  [ -n "$MS_PID" ]  && kill -9 "$MS_PID" 2>/dev/null || true
  wait 2>/dev/null || true
  # Preserve logs into the capture dir; remove the temp data dir.
  [ -n "$PHASE" ] && cp -f "$DATA_DIR/mothership.log" "$CAPTURE_DIR/${PHASE}.mothership.log" 2>/dev/null || true
  [ -n "$PHASE" ] && cp -f "$DATA_DIR/sim.log"        "$CAPTURE_DIR/${PHASE}.sim.log"        2>/dev/null || true
  rm -rf "$DATA_DIR"
}
trap cleanup EXIT INT TERM

start_mothership() {
  log "starting mothership on $PORT (data=$DATA_DIR)..."
  SPAXEL_BIND_ADDR="127.0.0.1:$PORT" \
  SPAXEL_DATA_DIR="$DATA_DIR" \
  SPAXEL_LOG_LEVEL=warn \
  SPAXEL_MDNS_ENABLED=false \
  TZ=UTC \
    "$MS" > "$DATA_DIR/mothership.log" 2>&1 &
  MS_PID=$!
  for _ in $(seq 1 50); do
    if resp=$(curl -s --max-time 2 "http://localhost:$PORT/healthz" 2>/dev/null) && \
       [ "$(printf '%s' "$resp" | jq -r '.status // empty' 2>/dev/null)" = "ok" ]; then
      log "mothership healthy"; return 0
    fi
    sleep 0.3
  done
  die "mothership never became healthy"
}

start_sim() {
  # $1 = "noble" (no --ble) | "ble"
  local mode="$1"
  local args=(--mothership "ws://localhost:$PORT/ws/node"
    --nodes "$SIM_NODES" --walkers "$SIM_WALKERS" --rate "$SIM_RATE"
    --space "$SIM_SPACE" --duration "$SIM_DURATION" --seed "$SIM_SEED")
  [ "$mode" = "ble" ] && args+=(--ble)
  "$SIM" "${args[@]}" > "$DATA_DIR/sim.log" 2>&1 &
  SIM_PID=$!
  log "sim started (pid $SIM_PID, mode=$mode)"
}

wait_for_blobs() {
  # Poll /api/blobs for >=1 live blob; print the first blob JSON to $1.
  local out="$1"
  local peak=0 first=""
  for _ in $(seq 1 30); do
    n=$(curl -s --max-time 2 "http://localhost:$PORT/api/blobs" 2>/dev/null | jq 'length' 2>/dev/null || echo 0)
    [ "$n" -gt "$peak" ] && peak=$n
    if [ "$n" -gt 0 ] && [ -z "$first" ]; then
      first=1
      curl -s --max-time 2 "http://localhost:$PORT/api/blobs" 2>/dev/null \
        | jq -c '.[0]' > "$out" 2>/dev/null || true
    fi
    [ "$peak" -gt 0 ] && { echo "$peak"; return 0; }
    sleep 0.5
  done
  echo "$peak"
}

capture_console() {
  # $1 = label (identity-less | identity)
  local label="$1"
  log "phase=$label: launching headless dashboard capture ($PAGES)..."
  node "$ROOT/scripts/capture-dashboard-console.mjs" \
    --base "http://localhost:$PORT" \
    --pages "$PAGES" \
    --outdir "$CAPTURE_DIR" \
    --label "$label" \
    --blob-timeout 15000 --settle 2000 \
    || die "playwright capture harness failed for phase=$label"
  log "phase=$label: summary:"
  cat "$CAPTURE_DIR/${label}.summary.txt" | sed 's/^/  /'
}

# Identify which identity-related fields are present in a captured blob JSON.
identity_field_report() {
  local f="$1"
  if [ ! -s "$f" ]; then echo "(no blob captured)"; return; fi
  local has_name has_color has_resolved
  has_name=$(jq -r '
    (has("personName") or has("person_name") or has("person_label") or has("personLabel") or has("person")) // false
    | tostring' "$f" 2>/dev/null)
  has_color=$(jq -r '
    (has("assignedColor") or has("assigned_color") or has("personColor") or has("person_color")) // false
    | tostring' "$f" 2>/dev/null)
  has_resolved=$(jq -r '(has("identityResolved") or has("identity_resolved")) // false | tostring' "$f" 2>/dev/null)
  printf 'personName-ish=%s  assignedColor-ish=%s  identityResolved-ish=%s' "$has_name" "$has_color" "$has_resolved"
}

count_identity_hits() {
  # Sum identityHits counts across the per-page JSONs for a label.
  local label="$1" total=0 n
  for jf in "$CAPTURE_DIR/${label}."*.json; do
    [ -e "$jf" ] || continue
    n=$(jq '.identityHits | length' "$jf" 2>/dev/null || echo 0)
    total=$((total + n))
  done
  echo "$total"
}

result_identity_less=""
result_identity=""

# ============================================================================
# PHASE 1 — identity-less (no --ble): /api/blobs serves blobs with NO identity
# ============================================================================
PHASE="identity-less"
log "================ PHASE 1: identity-less (SIM_BLE=0) ================"
start_mothership
start_sim noble
BLOB_LESS="$CAPTURE_DIR/identity-less.blob.json"
peak=$(wait_for_blobs "$BLOB_LESS")
log "phase=identity-less: peak_blobs=$peak  identity-fields: $(identity_field_report "$BLOB_LESS")"
[ "$peak" -gt 0 ] || { log "FAIL: no identity-less blob observed at /api/blobs"; result_identity_less="FAIL-no-blob"; }

# Snapshot the raw /api/blobs array (evidence it carries NO identity fields).
curl -s --max-time 2 "http://localhost:$PORT/api/blobs" > "$CAPTURE_DIR/identity-less.api-blobs.json" 2>/dev/null || true

capture_console "identity-less"

# Stop the identity-less sim but keep the mothership up for phase 2.
kill "$SIM_PID" 2>/dev/null || true
wait "$SIM_PID" 2>/dev/null || true
SIM_PID=""

hits_less=$(count_identity_hits "identity-less")
log "phase=identity-less: identity-related console hits = $hits_less"

# ============================================================================
# PHASE 2 — identity-resolved (--ble + registered person)
# ============================================================================
PHASE="identity"
log "================ PHASE 2: identity (--ble + person \"$PERSON_NAME\") ================"
# Register the person fixture BEFORE the sim starts (proven ordering, bf-gdfwx).
log "applying fixture (register \"$PERSON_NAME\" + bind $WALKER_MAC)..."
curl -s -X POST "http://localhost:$PORT/api/ble/devices/preregister" \
     -H 'Content-Type: application/json' \
     -d "{\"mac\":\"$WALKER_MAC\",\"label\":\"$PERSON_NAME\"}" >/dev/null
person_id=$(curl -s -X POST "http://localhost:$PORT/api/people" \
     -H 'Content-Type: application/json' \
     -d "{\"name\":\"$PERSON_NAME\",\"color\":\"$PERSON_COLOR\"}" \
     | jq -r '.id // .person_id // empty' 2>/dev/null)
if [ -z "$person_id" ]; then
  log "warn: /api/people did not return an id (continuing; identity may not resolve)"
else
  curl -s -X PUT "http://localhost:$PORT/api/ble/devices/$WALKER_MAC" \
       -H 'Content-Type: application/json' \
       -d "{\"person_id\":\"$person_id\",\"label\":\"$PERSON_NAME\"}" >/dev/null
  log "fixture bound: $WALKER_MAC -> \"$PERSON_NAME\" (id=$person_id)"
fi

start_sim ble
BLOB_ID="$CAPTURE_DIR/identity.blob.json"
# For the identity phase poll at 0.5s and accept any blob; capture the one that
# carries identity if present (else the first blob).
peak_id=0
for _ in $(seq 1 40); do
  arr=$(curl -s --max-time 2 "http://localhost:$PORT/api/blobs" 2>/dev/null || echo "[]")
  n=$(printf '%s' "$arr" | jq 'length' 2>/dev/null || echo 0)
  [ "$n" -gt "$peak_id" ] && peak_id=$n
  # Prefer a blob with identity; fall back to the first blob.
  chosen=$(printf '%s' "$arr" | jq -c '[.[] | select((.personName//"")!="" or (.person_label//"")!="" or .identityResolved==true)][0] // .[0]' 2>/dev/null)
  if [ -n "$chosen" ] && [ "$chosen" != "null" ]; then
    printf '%s' "$chosen" > "$BLOB_ID"
  fi
  has_id=$(printf '%s' "$chosen" | jq -r '(((.personName//"")!="") or ((.person_label//"")!="") or .identityResolved==true) // false | tostring' 2>/dev/null)
  if [ "$has_id" = "true" ]; then break; fi
  sleep 0.5
done
log "phase=identity: peak_blobs=$peak_id  identity-fields: $(identity_field_report "$BLOB_ID")"

curl -s --max-time 2 "http://localhost:$PORT/api/blobs" > "$CAPTURE_DIR/identity.api-blobs.json" 2>/dev/null || true

capture_console "identity"

kill "$SIM_PID" 2>/dev/null || true
wait "$SIM_PID" 2>/dev/null || true
SIM_PID=""

hits_id=$(count_identity_hits "identity")
log "phase=identity: identity-related console hits = $hits_id"

# Stop mothership so cleanup can flush logs.
kill -INT "$MS_PID" 2>/dev/null || true
sleep 1
kill -9 "$MS_PID" 2>/dev/null || true
MS_PID=""
PHASE=""

# ============================================================================
# VERDICT
# ============================================================================
echo ""
log "================ VERDICT ================"
log "phase identity-less : console identity-hits=$hits_less  blob-peak=$peak"
log "phase identity      : console identity-hits=$hits_id   blob-peak=$peak_id"
log "identity-less blob fields: $(identity_field_report "$BLOB_LESS")"
log "identity      blob fields: $(identity_field_report "$BLOB_ID")"

fail=0
if [ "$hits_less" -ne 0 ]; then log "FAIL: identity-less console had $hits_less identity-related hit(s)"; fail=1; fi
if [ "$hits_id"  -ne 0 ]; then log "FAIL: identity console had $hits_id identity-related hit(s)";         fail=1; fi
if [ "${peak:-0}" -le 0 ]; then log "FAIL: identity-less phase produced no blob";                          fail=1; fi

if [ "$fail" -eq 0 ]; then
  log "PASS: live dashboard console clean in BOTH phases (0 identity-related hits); identity-less blobs rendered."
  exit 0
fi
log "FAIL: see $CAPTURE_DIR for captured consoles."
exit 1
