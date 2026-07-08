#!/usr/bin/env bash
#
# run-sim-ble-fixture.sh — the BLE identity FIXTURE (bf-14wx5).
#
# This is the FIRST link of the bf-2m534 identity chain: it establishes the BLE
# fixture a registered person + a sim device bound to that person that the
# identity matcher needs to produce ANY match. It does NOT assert anything about
# /api/blobs identity that is the parent bead's capstone (scripts/run-sim-identity.sh,
# bf-2m534/bf-5h1t). This script's acceptance is narrower and directly verifiable:
#
#   GET /api/ble/devices on a running mothership confirms the sim's advertised
#   device addr is associated to the seeded person (person_id / person_name non-empty).
#
# It also answers the "must the device be seen-live first?" question empirically:
#
#   Phase A registers + binds the device with NO live BLE advertisement flowing
#   (sim not started yet), proving preregister+PUT binds standalone.
#   Phase B then starts sim --ble and re-verifies, proving the binding survives
#   live advertisements (ProcessRelayMessage upserts never touch person_id).
#
# Sim BLE addr convention (mothership/cmd/sim/main.go: sendBLEMessages):
#   each walker advertises addr = fmt.Sprintf("AA:BB:CC:DD:EE:%02X", walker.ID),
#   name = fmt.Sprintf("sim-person-%d", walker.ID), and walker.ID = i (from 0).
#   So --walkers 1 advertises exactly AA:BB:CC:DD:EE:00.
#
# No hardware, no Docker, no manual IP, no manual token. Reuses the build + health
# path proven by run-sim-local.sh. See notes/ble-identity-fixture.md for the full
# reasoning, exact REST calls, and evidence.
#
# Usage:  ./scripts/run-sim-ble-fixture.sh
# Env:    SIM_NODES, SIM_WALKERS, SIM_RATE, SIM_DURATION, SIM_SEED, SIM_PORT,
#         SIM_SPACE, SIM_WALKER_MAC, SIM_PERSON_NAME, SIM_PERSON_COLOR,
#         SPAXEL_MOTHERSHIP_BIN, SPAXEL_SIM_BIN
# Exit:   0 if GET /api/ble/devices confirms the device<->person binding in both
#         phases; 1 otherwise.
#
# Requires: go, curl, jq.

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MS="${SPAXEL_MOTHERSHIP_BIN:-/tmp/spaxel-mothership}"
SIM="${SPAXEL_SIM_BIN:-/tmp/spaxel-sim}"
PORT="${SIM_PORT:-8088}"

# walkers=1: one sim device (AA:BB:CC:DD:EE:00) -> one registered person. rate=30
# mirrors run-sim-local.sh's verified default (REQUIRED for deterministic blob
# emission, though blobs are not this script's gate).
SIM_NODES="${SIM_NODES:-4}"
SIM_WALKERS="${SIM_WALKERS:-1}"
SIM_RATE="${SIM_RATE:-30}"
SIM_DURATION="${SIM_DURATION:-15}"
SIM_SEED="${SIM_SEED:-42}"
SIM_SPACE="${SIM_SPACE:-5x5x2.5}"

# Walker 0 -> BLE addr AA:BB:CC:DD:EE:00 (cmd/sim/main.go: AA:BB:CC:DD:EE:%02X, IDs from 0).
WALKER_MAC="${SIM_WALKER_MAC:-AA:BB:CC:DD:EE:00}"
PERSON_NAME="${SIM_PERSON_NAME:-Alice}"
PERSON_COLOR="${SIM_PERSON_COLOR:-#4488ff}"

log() { printf '[fixture] %s\n' "$*"; }
die() { printf '[fixture] ERROR: %s\n' "$*" >&2; exit 1; }

for dep in go curl jq; do
  command -v "$dep" >/dev/null 2>&1 || die "missing required dependency: $dep"
done

# --- Build both binaries from source (reuse if present; force sim has --ble) ------
# Canonical-source rule (see notes/hardware-free-runtime.md gotcha #1): build ONLY
# mothership/cmd/sim (it has --ble/--verify); never the repo-root cmd/sim/ (older).
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

DATA_DIR=$(mktemp -d -t spaxel-fixture-data-XXXXXX)
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
log "fixture: $WALKER_MAC -> person \"$PERSON_NAME\" ($PERSON_COLOR) [walkers=$SIM_WALKERS]"

# --- Start mothership on an ephemeral data dir (identical env to run-sim-local) --
SPAXEL_BIND_ADDR="127.0.0.1:$PORT" \
SPAXEL_DATA_DIR="$DATA_DIR" \
SPAXEL_LOG_LEVEL=warn \
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

# Bind a helper that asserts GET /api/ble/devices shows the device<->person binding.
# Echoes "OK <json>" on success (person_id and person_name non-empty for $WALKER_MAC).
verify_binding() {
  local label="$1"
  local rec
  rec=$(curl -s --max-time 2 "http://localhost:$PORT/api/ble/devices?registered=true" \
    | jq -c --arg mac "$WALKER_MAC" '[.devices[]|select(.mac==$mac)][0]' 2>/dev/null)
  if [ "$rec" = "null" ] || [ -z "$rec" ]; then
    log "  [$label] $WALKER_MAC NOT in /api/ble/devices?registered=true"
    return 1
  fi
  local pid pname
  pid=$(printf '%s' "$rec" | jq -r '.person_id // empty')
  pname=$(printf '%s' "$rec" | jq -r '.person_name // empty')
  if [ -z "$pid" ] || [ -z "$pname" ]; then
    log "  [$label] $WALKER_MAC present but unbound (person_id=\"$pid\" person_name=\"$pname\")"
    return 1
  fi
  log "  [$label] OK: $WALKER_MAC -> person_id=$pid person_name=\"$pname\""
  printf '%s\n' "$rec" > "$DATA_DIR/${label}.json"
  return 0
}

# --- The repeatable REST fixture sequence (register person + bind device) --------
# POST /api/ble/devices/preregister -> create the device row (sets last_seen_at=now,
#   so the device shows in GET /api/ble/devices even with NO live advertisement).
# POST /api/people                 -> create the person, capture its id.
# PUT  /api/ble/devices/{mac}      -> assign device -> person (sets person_id).
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
}

# ================= Phase A: bind with NO live advertisement ======================
# Proves preregister+PUT binds standalone — the device does NOT need to be seen
# live first. This directly answers bf-14wx5 acceptance criterion #4.
log "Phase A: seed fixture BEFORE any sim BLE advertisement (no live adv)..."
SEED_PERSON_ID=""
seed_fixture
log "registered: $WALKER_MAC -> person \"$PERSON_NAME\" (id=$SEED_PERSON_ID)"
phaseA=1
verify_binding "phaseA" || phaseA=0

# ================= Phase B: start sim --ble, re-verify ===========================
# Proves the binding survives live advertisements (ProcessRelayMessage upserts
# rssi/last_seen but never person_id) and the device is now seen live (rssi set).
log "Phase B: starting sim --ble (walkers=$SIM_WALKERS, seed=$SIM_SEED)..."
"$SIM" \
  --mothership "ws://localhost:$PORT/ws/node" \
  --nodes "$SIM_NODES" --walkers "$SIM_WALKERS" --rate "$SIM_RATE" \
  --space "$SIM_SPACE" \
  --duration "$SIM_DURATION" --seed "$SIM_SEED" --ble \
  > "$DATA_DIR/sim.log" 2>&1 &
SIM_PID=$!
# Let at least one BLE adv relay land (~5s BLE cadence in the sim).
log "waiting 7s for sim BLE advertisements to land..."
sleep 7
phaseB=1
verify_binding "phaseB" || phaseB=0

# Capture the full live device record (incl. rssi, now populated by live advs) and
# the people view as corroboration for the note.
curl -s --max-time 2 "http://localhost:$PORT/api/ble/devices" \
  | jq -c --arg mac "$WALKER_MAC" '[.devices[]|select(.mac==$mac)][0]' > "$DATA_DIR/phaseB_full.json" 2>/dev/null
curl -s --max-time 2 "http://localhost:$PORT/api/people" > "$DATA_DIR/people.json" 2>/dev/null

# Stop the sim so the run is bounded.
kill "$SIM_PID" 2>/dev/null || true
wait "$SIM_PID" 2>/dev/null || true
SIM_PID=""
reject_count=$(grep -ciE 'reject|invalid_token|\b401\b|\b403\b' "$DATA_DIR/sim.log" 2>/dev/null)

echo ""
log "---- RESULT ----"
log "phaseA (preregister+PUT, NO live adv): $([ "$phaseA" = 1 ] && echo PASS || echo FAIL)"
log "phaseB (live sim --ble adv, binding survives): $([ "$phaseB" = 1 ] && echo PASS || echo FAIL)"
log "seeded person: \"$PERSON_NAME\" (id=$SEED_PERSON_ID)  device: $WALKER_MAC"
log "reject_in_sim_log=${reject_count:-0}"
echo ""
log "evidence: /api/ble/devices?registered=true record after live adv (phaseB_full):"
if [ -s "$DATA_DIR/phaseB_full.json" ] && [ "$(cat "$DATA_DIR/phaseB_full.json")" != "null" ]; then
  jq '{mac,label,person_id,person_name,rssi_avg,last_seen_node,last_seen_at}' "$DATA_DIR/phaseB_full.json" 2>/dev/null \
    || cat "$DATA_DIR/phaseB_full.json"
else
  log "  (no record)"
fi
echo ""

# --- Acceptance gate: GET /api/ble/devices confirms the binding in BOTH phases ----
if [ "$phaseA" != 1 ] || [ "$phaseB" != 1 ]; then
  log "FAIL: binding not confirmed via GET /api/ble/devices (phaseA=$phaseA phaseB=$phaseB)"
  log "      (mothership.log tail for diagnostics:)"
  tail -20 "$DATA_DIR/mothership.log" >&2 || true
  exit 1
fi
log "PASS: GET /api/ble/devices confirms $WALKER_MAC <-> \"$PERSON_NAME\" in both phases"
log "      (preregister+PUT binds with NO live adv; binding survives live sim --ble advs)"
exit 0
