#!/usr/bin/env bash
#
# run-sim-identity.sh — hardware-free reproduce of NON-EMPTY blob identity (bf-2m534 / bf-gdfwx).
#
# Sibling of run-sim-local.sh (the canonical bf-40hc "a blob exists" harness) and
# the capstone of the bf-5h1t identity chain. It drives the SAME hardware-free
# runtime with --ble enabled, seeds the BLE registry with a registered person
# BEFORE the sim starts, then asserts that GET /api/blobs serves a tracked blob
# whose canonical identity field (personName / assignedColor / identityResolved)
# is NON-EMPTY — observed from the live REST surface, with no panic.
#
# This is the THIRD and final link of the chain:
#   matcher-active -> match-found -> written-onto-served-blob -> observable-at-/api/blobs
# The wire fix (Stage 2b write-back) landed in bf-21v71; this script supplies the
# fixture-side preconditions the diagnosis (notes/blob-identity-diagnosis.md)
# named as required for a match to fire: --ble advertisements + a registered
# person. (The third precondition — the sim's `rssi` vs parser's `rssi_dbm`
# field-name mismatch — is fixed at mothership/cmd/sim/main.go so RSSI is no
# longer read as 0 and triangulation does not collapse.)
#
# Ordering (the fix for bf-gdfwx's two prior failures): apply the fixture
# BEFORE starting the sim, exactly as run-sim-ble-match.sh (bf-1yr1w, proven
# PASS) does. With walkers=1 the CSI blob only lives in the early window
# (real t~1-6s; the single walker drifts to a low-signal corner after that),
# so the matcher must have the person binding already in place when the first
# --ble adv + early blob coincide. Registering the fixture after the sim start
# (the prior shape) delayed the poll loop to ~real t=4s and the matcher never
# got person+RSSI+nearby-blob together -> empty /api/blobs identity. Polling is
# also run at 0.5s cadence (not 1s) to catch that narrow early blob window.
#
# No hardware, no Docker, no manual IP, no manual token. Reuses the build +
# health + token path proven by run-sim-local.sh and the fixture from
# run-sim-ble-match.sh.
#
# Usage:  ./scripts/run-sim-identity.sh
# Env:    SIM_NODES, SIM_WALKERS, SIM_RATE, SIM_DURATION, SIM_SEED, SIM_PORT,
#         SIM_SPACE, SIM_WALKER_MAC, SIM_PERSON_NAME, SIM_PERSON_COLOR,
#         SPAXEL_MOTHERSHIP_BIN, SPAXEL_SIM_BIN
# Exit:   0 if a live /api/blobs blob carries non-empty canonical identity, 1 otherwise.
#
# Requires: go, curl, jq.

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MS="${SPAXEL_MOTHERSHIP_BIN:-/tmp/spaxel-mothership}"
SIM="${SPAXEL_SIM_BIN:-/tmp/spaxel-sim}"
PORT="${SIM_PORT:-8088}"

# walkers=1: one registered person (Alice) -> one identity-bearing blob is the
# cleanest evidence. rate=30 is REQUIRED for deterministic blob emission
# (hardware-free-runtime.md gotcha #2). seed=42 pins walker-0 to advertise
# AA:BB:CC:DD:EE:00 (cmd/sim/main.go: fmt AA:BB:CC:DD:EE:%02X, IDs from 0).
SIM_NODES="${SIM_NODES:-4}"
SIM_WALKERS="${SIM_WALKERS:-1}"
SIM_RATE="${SIM_RATE:-30}"
SIM_DURATION="${SIM_DURATION:-25}"
SIM_SEED="${SIM_SEED:-42}"
SIM_SPACE="${SIM_SPACE:-5x5x2.5}"

WALKER_MAC="${SIM_WALKER_MAC:-AA:BB:CC:DD:EE:00}"
PERSON_NAME="${SIM_PERSON_NAME:-Alice}"
PERSON_COLOR="${SIM_PERSON_COLOR:-#4488ff}"

log() { printf '[run-sim-id] %s\n' "$*"; }
die() { printf '[run-sim-id] ERROR: %s\n' "$*" >&2; exit 1; }

for dep in go curl jq; do
  command -v "$dep" >/dev/null 2>&1 || die "missing required dependency: $dep"
done

# --- Build both binaries from source (reuse if present; force sim has --ble) ----
# Same canonical-source rule as run-sim-local.sh: build ONLY mothership/cmd/sim
# (it has --ble/--verify); never the repo-root cmd/sim/ (older, lacks them).
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

DATA_DIR=$(mktemp -d -t spaxel-runsimid-data-XXXXXX)
MS_PID=""
SIM_PID=""

cleanup() {
  [ -n "$SIM_PID" ] && kill "$SIM_PID" 2>/dev/null || true
  [ -n "$MS_PID" ]  && kill -INT "$MS_PID" 2>/dev/null || true
  sleep 1
  [ -n "$MS_PID" ]  && kill -9 "$MS_PID" 2>/dev/null || true
  wait 2>/dev/null || true
  # CAPTURE_DIR hook (bf-15oi): if set, persist the text log artifacts
  # (mothership.log, sim.log, identity_blob.json) to a durable location before
  # the temp dir is removed, so the run can be scanned/referenced after exit.
  # Only the human-readable logs/evidence are copied — NOT the data dir's
  # binaries (csi_replay.bin can be hundreds of MB; the *.db shards are
  # regenerable) — so a capture run never drops a giant blob into the repo.
  # Purely additive — unset (the default) preserves the prior cleanup behaviour.
  if [ -n "${CAPTURE_DIR:-}" ]; then
    mkdir -p "$CAPTURE_DIR"
    for f in mothership.log sim.log identity_blob.json; do
      [ -f "$DATA_DIR/$f" ] && cp -a "$DATA_DIR/$f" "$CAPTURE_DIR/" 2>/dev/null || true
    done
  fi
  rm -rf "$DATA_DIR"
}
trap cleanup EXIT INT TERM

log "mothership: $MS"
log "sim:        $SIM"
log "params: nodes=$SIM_NODES walkers=$SIM_WALKERS rate=$SIM_RATE duration=${SIM_DURATION}s seed=$SIM_SEED space=$SIM_SPACE port=$PORT ble=on"
log "identity fixture: $WALKER_MAC -> person \"$PERSON_NAME\" ($PERSON_COLOR)"

# --- Start mothership on an ephemeral data dir (identical env to run-sim-ble-match) --
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
log "mothership healthy"

# --- Apply the fixture BEFORE starting the sim (bf-gdfwx fix; mirrors ----------
# run-sim-ble-match.sh's proven seed_fixture). Register the person + bind the
# sim's advertised device addr while the mothership is up but the sim is not yet
# streaming, so the device is bound standalone and the binding is in place when
# the first --ble adv arrives.
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

log "applying fixture (register \"$PERSON_NAME\" + bind $WALKER_MAC) BEFORE sim..."
SEED_PERSON_ID=""
seed_fixture
log "fixture bound: $WALKER_MAC -> \"$PERSON_NAME\" (id=$SEED_PERSON_ID)"

# --- Start sim WITH --ble (the fixture precondition run-sim-local.sh omits) ------
"$SIM" \
  --mothership "ws://localhost:$PORT/ws/node" \
  --nodes "$SIM_NODES" --walkers "$SIM_WALKERS" --rate "$SIM_RATE" \
  --space "$SIM_SPACE" \
  --duration "$SIM_DURATION" --seed "$SIM_SEED" --ble \
  > "$DATA_DIR/sim.log" 2>&1 &
SIM_PID=$!
log "sim started (pid $SIM_PID, --ble enabled)"

# --- Observe /api/blobs for non-empty canonical identity over the run window ----
# Identity is written by the Stage 2b write-back each fusion tick; the matcher
# triangulates from cached RSSI (~5s BLE cadence). Poll at 0.5s cadence over the
# full window (real t=0..duration) so the narrow early blob window is not missed.
# Capture the first blob that carries identityResolved==true OR personName != ""
# as the canonical evidence; also snapshot /api/ble/matches as corroboration.
peak_blobs=0
identity_blob=""        # first /api/blobs blob JSON with non-empty personName
identity_blob_t=""
match_json=""
sample_count=0
ticks=$(( SIM_DURATION * 2 ))    # 0.5s cadence
for i in $(seq 1 "$ticks"); do
  sleep 0.5
  sample_count=$((sample_count+1))
  t_ms=$(( i * 500 ))
  raw=$(curl -s --max-time 2 "http://localhost:$PORT/api/blobs" 2>/dev/null)
  n=$(printf '%s' "$raw" | jq 'length' 2>/dev/null || echo 0)
  [ "$n" -gt "$peak_blobs" ] && peak_blobs=$n

  # identityResolved==true OR personName != ""  => resolved canonical identity
  id_blob=$(printf '%s' "$raw" | jq -c \
    '[.[] | select((.identityResolved==true) or (.personName // "" | length > 0))][0]' 2>/dev/null)
  if [ "$id_blob" != "null" ] && [ -n "$id_blob" ]; then
    identity_blob="$id_blob"
    [ -z "$identity_blob_t" ] && identity_blob_t="${t_ms}ms"
    [ -z "$match_json" ] && match_json=$(curl -s --max-time 2 "http://localhost:$PORT/api/ble/matches" 2>/dev/null)
    break
  fi

  if [ $(( i % 2 )) -eq 0 ]; then
    resolved=$(printf '%s' "$raw" | jq '[.[]|select(.identityResolved==true)]|length' 2>/dev/null || echo 0)
    printf '  t=%2ds  blobs=%s  peak=%s  identityResolved_blobs=%s\n' "$((i/2))" "$n" "$peak_blobs" "$resolved"
  fi
done

# Stop the sim so the run is bounded, then tally diagnostics.
kill "$SIM_PID" 2>/dev/null || true
wait "$SIM_PID" 2>/dev/null; SIM_EXIT=$?
SIM_PID=""
reject_count=$(grep -ciE 'reject|invalid_token|\b401\b|\b403\b' "$DATA_DIR/sim.log" 2>/dev/null)

# If identity was observed mid-run, take one more stable sample (matcher persists
# after sim stop via identity persistence) for a clean evidence capture.
if [ -n "$identity_blob" ] && [ "$identity_blob" != "null" ]; then
  raw=$(curl -s --max-time 2 "http://localhost:$PORT/api/blobs" 2>/dev/null)
  fresh=$(printf '%s' "$raw" | jq -c \
    '[.[] | select((.identityResolved==true) or (.personName // "" | length > 0))][0]' 2>/dev/null)
  [ "$fresh" != "null" ] && [ -n "$fresh" ] && identity_blob="$fresh"
fi
# Final matcher surface snapshot (corroboration).
final_matches=$(curl -s --max-time 2 "http://localhost:$PORT/api/ble/matches" 2>/dev/null)
final_summary=$(printf '%s' "$final_matches" \
  | jq -c '[.[] | {blob_id, person_name, device_addr, confidence, is_ble_only}]' 2>/dev/null)

echo ""
log "---- RESULT ----"
log "seeded person: \"$PERSON_NAME\" (id=$SEED_PERSON_ID)  device: $WALKER_MAC"
log "peak_blob_count=$peak_blobs"
log "identity_blob_at=${identity_blob_t:-never}"
log "samples=$sample_count (0.5s cadence)  nodes=$SIM_NODES walkers=$SIM_WALKERS rate=$SIM_RATE duration=${SIM_DURATION}s seed=$SIM_SEED ble=on"
log "sim_exit=$SIM_EXIT  reject_in_sim_log=${reject_count:-0}"
echo ""
log "evidence: /api/blobs blob with non-empty canonical identity:"
if [ -n "$identity_blob" ] && [ "$identity_blob" != "null" ]; then
  printf '%s\n' "$identity_blob" | jq '.' 2>/dev/null || printf '%s\n' "$identity_blob"
else
  log "  (none observed)"
fi
echo ""
log "corroboration: /api/ble/matches (runtime matcher surface):"
if [ -n "$final_summary" ] && [ "$final_summary" != "null" ]; then
  printf '%s\n' "$final_summary" | head -40
else
  log "  (none observed)"
fi
echo ""
# Persist the raw evidence for the note / audit.
printf '%s\n' "$identity_blob" > "$DATA_DIR/identity_blob.json"
log "mothership matcher-related log lines (INFO/WARN):"
grep -iE 'ble:|identity|triangulat|match' "$DATA_DIR/mothership.log" 2>/dev/null | tail -15 || log "  (none)"

# --- Acceptance gate: a live blob carried non-empty canonical identity ----------
if [ -z "$identity_blob" ] || [ "$identity_blob" = "null" ]; then
  log "FAIL: no /api/blobs blob carried non-empty canonical identity over the run window"
  log "      peak_blobs=$peak_blobs final /api/ble/matches: ${final_summary:-[]}"
  log "      (mothership.log tail for diagnostics:)"
  tail -20 "$DATA_DIR/mothership.log" >&2 || true
  exit 1
fi
person_val=$(printf '%s' "$identity_blob" | jq -r '.personName // empty' 2>/dev/null)
color_val=$(printf '%s' "$identity_blob" | jq -r '.assignedColor // empty' 2>/dev/null)
resolved_val=$(printf '%s' "$identity_blob" | jq -r '.identityResolved // empty' 2>/dev/null)
log "PASS: live /api/blobs blob has non-empty identity — personName=\"$person_val\" assignedColor=\"$color_val\" identityResolved=$resolved_val"
exit 0
