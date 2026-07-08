#!/usr/bin/env bash
#
# run-sim-ble-match.sh — the BLE identity MATCHER check (bf-1yr1w).
#
# This is the SECOND link of the bf-2m534 identity chain. The first link
# (bf-14wx5, scripts/run-sim-ble-fixture.sh) established the fixture: a registered
# person + the sim's advertised device bound to that person, verified only via
# GET /api/ble/devices (the device<->person binding). That binding is NECESSARY but
# not SUFFICIENT for identity on a blob — the matcher (ble.IdentityMatcher) still
# has to (a) triangulate the device from live RSSI, (b) land it within 2m of a CSI
# blob, and (c) clear MinMatchConfidence (0.6). This script drives exactly that and
# asserts the matcher's GetMatch() returns the seeded person for at least one blob.
#
# Observation surface: GET /api/ble/matches returns identityMatcher.GetAllMatches()
# (cmd/mothership/main.go:~3070) — both real blob matches (blob_id >= 0) and BLE-only
# placeholder tracks (blob_id == -1). A record with person_name == "Alice" and
# blob_id != -1 is direct evidence GetMatch(blobID) returns the seeded person.
#
# It also wires the two preconditions the chain named:
#   1. The fixture (reuse run-sim-ble-fixture.sh's REST sequence): register person
#      "Alice" + bind the sim's advertised AA:BB:CC:DD:EE:00 (walker 0).
#   2. The rssi_dbm key in sim BLE ads (mothership/cmd/sim/main.go:sendBLEMessages):
#      emitting "rssi" left RSSIdBm at zero, collapsing triangulation. The sim is
#      built from the canonical mothership/cmd/sim source which carries this fix.
#
# Scope note: this asserts the MATCHER produces a match. It deliberately does NOT
# gate on /api/blobs identity fields — that is the THIRD link / parent capstone
# (bf-2m534, scripts/run-sim-identity.sh), which depends on this bead's PASS.
#
# No hardware, no Docker, no manual IP, no manual token. Reuses the build + health
# path proven by run-sim-local.sh / run-sim-ble-fixture.sh.
#
# Usage:  ./scripts/run-sim-ble-match.sh
# Env:    SIM_NODES, SIM_WALKERS, SIM_RATE, SIM_DURATION, SIM_SEED, SIM_SPACE,
#         SIM_PORT, SIM_WALKER_MAC, SIM_PERSON_NAME, SIM_PERSON_COLOR,
#         SPAXEL_MOTHERSHIP_BIN, SPAXEL_SIM_BIN
# Exit:   0 if GetMatch() returns the seeded person for >=1 blob (real blob match,
#           not BLE-only); 1 otherwise.
#
# Requires: go, curl, jq.

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MS="${SPAXEL_MOTHERSHIP_BIN:-/tmp/spaxel-mothership}"
SIM="${SPAXEL_SIM_BIN:-/tmp/spaxel-sim}"
PORT="${SIM_PORT:-8088}"

# walkers=1: this fixture registers exactly ONE person (Alice) + ONE device
# (AA:BB:CC:DD:EE:00), so the sim emits exactly ONE walker — Alice's. The earlier
# default of 2 (inherited from run-sim-local.sh's blob-emission stress params) is a
# multi-person confound this single-person identity test does not model: with two
# walkers the CSI blob nearest Alice's triangulated point was the SECOND walker's
# (cornered away from Alice), leaving a ~1.73m gap so f_distance killed matchConf
# (0.143 < gate 0.60) — diagnosis bf-6crd7/bf-6d2ii. With one walker the blob is
# Alice's own, co-located at ~0.6m gap → matchConf ≈ 0.714 ≥ 0.60. NOT a threshold
# loosening: MaxBLEBlobDistance=2.0 and MinMatchConfidence=0.6 are unchanged. The
# rate (30) stays — REQUIRED for deterministic CSI blobs (hardware-free-runtime.md
# gotcha #2); the matcher needs a live blob near the device to match.
SIM_NODES="${SIM_NODES:-4}"
SIM_WALKERS="${SIM_WALKERS:-1}"
SIM_RATE="${SIM_RATE:-30}"
SIM_DURATION="${SIM_DURATION:-25}"
SIM_SEED="${SIM_SEED:-42}"
SIM_SPACE="${SIM_SPACE:-5x5x2.5}"

# Walker 0 -> BLE addr AA:BB:CC:DD:EE:00 (cmd/sim/main.go: AA:BB:CC:DD:EE:%02X, IDs from 0).
WALKER_MAC="${SIM_WALKER_MAC:-AA:BB:CC:DD:EE:00}"
PERSON_NAME="${SIM_PERSON_NAME:-Alice}"
PERSON_COLOR="${SIM_PERSON_COLOR:-#4488ff}"

log() { printf '[match] %s\n' "$*"; }
die() { printf '[match] ERROR: %s\n' "$*" >&2; exit 1; }

for dep in go curl jq; do
  command -v "$dep" >/dev/null 2>&1 || die "missing required dependency: $dep"
done

# --- Build both binaries from source (canonical sim has --ble; force if stale) -----
log "building canonical binaries from source (reuse if present)..."
need_build=0
[ -x "$MS" ]  || need_build=1
[ -x "$SIM" ] || need_build=1
if [ -x "$SIM" ] && ! "$SIM" --help 2>&1 | grep -q -- '--ble'; then
  log "existing $SIM lacks --ble (stale/standalone build); rebuilding..."
  rm -f "$SIM"; need_build=1
fi
if [ "$need_build" -eq 1 ]; then
  ( cd "$ROOT/mothership" || exit 1
    go build -o "$MS"  ./cmd/mothership || die "building mothership"
    go build -o "$SIM" ./cmd/sim         || die "building sim"
  ) || exit 1
fi

DATA_DIR=$(mktemp -d -t spaxel-match-data-XXXXXX)
MS_PID=""
SIM_PID=""

cleanup() {
  [ -n "$SIM_PID" ] && kill "$SIM_PID" 2>/dev/null || true
  [ -n "$MS_PID" ]  && kill -INT "$MS_PID" 2>/dev/null || true
  sleep 1
  [ -n "$MS_PID" ]  && kill -9 "$MS_PID" 2>/dev/null || true
  wait 2>/dev/null || true
  rm -rf "$DATA_DIR"
}
trap cleanup EXIT INT TERM

log "mothership: $MS"
log "sim:        $SIM"
log "fixture: $WALKER_MAC -> person \"$PERSON_NAME\" ($PERSON_COLOR)"
log "params: nodes=$SIM_NODES walkers=$SIM_WALKERS rate=$SIM_RATE duration=${SIM_DURATION}s seed=$SIM_SEED space=$SIM_SPACE port=$PORT"

# --- Start mothership on an ephemeral data dir (identical env to run-sim-local) ----
# SPAXEL_LOG_LEVEL=info surfaces matcher/fleet INFO lines for evidence capture.
SPAXEL_BIND_ADDR="127.0.0.1:$PORT" \
SPAXEL_DATA_DIR="$DATA_DIR" \
SPAXEL_LOG_LEVEL=info \
SPAXEL_MDNS_ENABLED=false \
TZ=UTC \
  "$MS" > "$DATA_DIR/mothership.log" 2>&1 &
MS_PID=$!

ok=""
for _ in $(seq 1 50); do
  if resp=$(curl -s --max-time 2 "http://localhost:$PORT/healthz" 2>/dev/null) && \
     [ "$(printf '%s' "$resp" | jq -r '.status // empty' 2>/dev/null)" = "ok" ]; then
    ok=1; break
  fi
  sleep 0.3
done
[ -n "$ok" ] || { log "mothership never became healthy"; tail -30 "$DATA_DIR/mothership.log"; exit 1; }
log "mothership healthy (port $PORT)"

# --- Apply the child-1 fixture BEFORE starting the sim ----------------------------
# Register the person + bind the sim's advertised device addr, exactly per
# notes/ble-identity-fixture.md. Done while the mothership is up but the sim is not
# yet streaming, so the device is bound standalone (PreregisterDevice sets
# last_seen_at=now). The binding then survives the live --ble advertisements.
seed_fixture() {
  curl -s -X POST "http://localhost:$PORT/api/ble/devices/preregister" \
    -H 'Content-Type: application/json' \
    -d "{\"mac\":\"$WALKER_MAC\",\"label\":\"$PERSON_NAME\"}" >/dev/null
  person_id=$(curl -s -X POST "http://localhost:$PORT/api/people" \
    -H 'Content-Type: application/json' \
    -d "{\"name\":\"$PERSON_NAME\",\"color\":\"$PERSON_COLOR\"}" \
    | jq -r '.id // empty' 2>/dev/null)
  [ -n "$person_id" ] || die "could not create person \"$PERSON_NAME\" via /api/people"
  curl -s -X PUT "http://localhost:$PORT/api/ble/devices/$WALKER_MAC" \
    -H 'Content-Type: application/json' \
    -d "{\"person_id\":\"$person_id\",\"label\":\"$PERSON_NAME\"}" >/dev/null
  SEED_PERSON_ID="$person_id"

  # Sanity: confirm the binding took (reuses the child-1 gate).
  rec=$(curl -s --max-time 2 "http://localhost:$PORT/api/ble/devices?registered=true" \
    | jq -c --arg mac "$WALKER_MAC" '[.devices[]|select(.mac==$mac)][0]' 2>/dev/null)
  pid=$(printf '%s' "$rec" | jq -r '.person_id // empty' 2>/dev/null)
  [ -n "$pid" ] || { log "fixture FAILED: $WALKER_MAC not bound (child-1 regression)"; exit 1; }
}

log "applying child-1 fixture (register \"$PERSON_NAME\" + bind $WALKER_MAC)..."
SEED_PERSON_ID=""
seed_fixture
log "fixture bound: $WALKER_MAC -> \"$PERSON_NAME\" (id=$SEED_PERSON_ID)"

# --- Start the sim with --ble (canonical blob-emission params from run-sim-local) --
"$SIM" \
  --mothership "ws://localhost:$PORT/ws/node" \
  --nodes "$SIM_NODES" --walkers "$SIM_WALKERS" --rate "$SIM_RATE" \
  --space "$SIM_SPACE" \
  --duration "$SIM_DURATION" --seed "$SIM_SEED" --ble \
  > "$DATA_DIR/sim.log" 2>&1 &
SIM_PID=$!
log "sim --ble started (pid $SIM_PID)"

# --- Poll /api/ble/matches for a REAL blob match for the seeded person -----------
# GET /api/ble/matches -> identityMatcher.GetAllMatches(). A real blob match has
# blob_id != -1 (BLE-only placeholder tracks use blob_id == -1). We want a match
# whose person_name == "Alice" attached to a live CSI blob — that is GetMatch()
# returning the seeded person. We also record peak blob count and whether a BLE-only
# track appeared (diagnostic: BLE-only-with-no-blob-match narrows a miss to the
# distance/confidence gate, not the triangulation step).
peak_blobs=0
match_blob_id=""
match_conf=""
match_device=""
got_bleonly=0
first_match_t=""
for t in $(seq 1 "$SIM_DURATION"); do
  sleep 1
  nblobs=$(curl -s --max-time 2 "http://localhost:$PORT/api/blobs" 2>/dev/null | jq 'length' 2>/dev/null || echo 0)
  [ "$nblobs" -gt "$peak_blobs" ] && peak_blobs=$nblobs

  matches_json=$(curl -s --max-time 2 "http://localhost:$PORT/api/ble/matches" 2>/dev/null)
  # Real blob match for the seeded person: blob_id != -1 AND person_name == Alice.
  rec=$(printf '%s' "$matches_json" \
    | jq -c --arg name "$PERSON_NAME" \
        '[.[] | select((.blob_id // -1) != -1 and .person_name == $name)][0]' 2>/dev/null)
  if [ "$rec" != "null" ] && [ -n "$rec" ]; then
    match_blob_id=$(printf '%s' "$rec" | jq -r '.blob_id')
    match_conf=$(printf '%s' "$rec" | jq -r '.confidence')
    match_device=$(printf '%s' "$rec" | jq -r '.device_addr')
    first_match_t="${t}s"
    break
  fi
  # Diagnostic: did a BLE-only track (triangulated device, no nearby blob) appear?
  bo=$(printf '%s' "$matches_json" \
    | jq -c --arg name "$PERSON_NAME" \
        '[.[] | select((.blob_id // -1) == -1 and .person_name == $name)][0]' 2>/dev/null)
  [ "$bo" != "null" ] && [ -n "$bo" ] && got_bleonly=1

  printf '  t=%2ds  blobs=%s  peak_blobs=%s  alice_blob_match=no%s\n' \
    "$t" "$nblobs" "$peak_blobs" "$([ "$got_bleonly" = 1 ] && echo ' (alice BLE-only track seen)')"
done

# Stop the sim so the run is bounded, then tally diagnostics.
kill "$SIM_PID" 2>/dev/null || true
wait "$SIM_PID" 2>/dev/null || true
SIM_PID=""
reject_count=$(grep -ciE 'reject|invalid_token|\b401\b|\b403\b' "$DATA_DIR/sim.log" 2>/dev/null)

# Final matcher state snapshot (may still carry a persistent match after sim stop).
final_matches=$(curl -s --max-time 2 "http://localhost:$PORT/api/ble/matches" 2>/dev/null)
final_summary=$(printf '%s' "$final_matches" \
  | jq -c '[.[] | {blob_id, person_name, device_addr, confidence, is_ble_only}]' 2>/dev/null)

echo ""
log "---- RESULT ----"
log "seeded person: \"$PERSON_NAME\" (id=$SEED_PERSON_ID)  device: $WALKER_MAC"
log "peak_blobs (via /api/blobs): $peak_blobs"
log "alice_blob_match: ${match_blob_id:+blob_id=$match_blob_id conf=$match_conf device=$match_device (at $first_match_t)}"
log "alice_blob_match: ${match_blob_id:-NONE}"
log "alice_bleonly_track_seen: $([ "$got_bleonly" = 1 ] && echo yes || echo no)"
log "reject_in_sim_log=${reject_count:-0}"
echo ""
log "final /api/ble/matches summary:"
printf '%s\n' "${final_summary:-[]}" | head -40
echo ""
log "mothership matcher-related log lines (INFO/WARN):"
grep -iE 'ble:|identity|triangulat|match' "$DATA_DIR/mothership.log" 2>/dev/null | tail -20 || log "  (none)"

# --- Acceptance gate: GetMatch() returns the seeded person for >=1 real blob -------
if [ -n "$match_blob_id" ]; then
  log "PASS: GetMatch() returns \"$PERSON_NAME\" for blob_id=$match_blob_id (conf=$match_conf) via /api/ble/matches"
  log "      — matcher triangulated $match_device within range of a CSI blob and cleared MinMatchConfidence."
  exit 0
fi

# Explicit failure with evidence (acceptance criterion 4): never leave a miss silent.
log "FAIL: no real blob match for \"$PERSON_NAME\" via /api/ble/matches over the ${SIM_DURATION}s window."
log "      peak_blobs=$peak_blobs (so CSI blobs WERE present), alice_bleonly_track=$got_bleonly"
if [ "$peak_blobs" -le 0 ]; then
  log "      -> NO CSI blob at all: the prerequisite blob path (run-sim-local) regressed. Re-run at SIM_RATE=30 first."
elif [ "$got_bleonly" = 1 ]; then
  log "      -> A BLE-only track appeared (device WAS triangulated + has a person) but never attached to a blob:"
  log "         the matcher's distance (<2m, X/Z plane) or confidence (>=0.6) gate rejected the blob assignment."
  log "         Likely cause: the sim RSSI model (-50-20log10(d)) differs from the matcher's rssiToDistance"
  log "         (ref -65 dBm @ 1m, n=2.5), so triangulation lands the device off the walker -> off the CSI blob."
else
  log "      -> No triangulated device for \"$PERSON_NAME\" appeared at all: triangulation produced <0.1 confidence"
  log "         (too few nodes / node positions unset / RSSI not landing). Check sim rssi_dbm + node positions."
fi
exit 1
