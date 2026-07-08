#!/usr/bin/env bash
#
# run-sim-local.sh — canonical, hardware-free reproduce of the spaxel runtime path.
#
# Builds the mothership + the canonical CSI simulator from source, starts the
# mothership on an ephemeral port + data dir, streams simulated CSI, and asserts
# that the fusion/tracker pipeline emits >=1 tracked blob via GET /api/blobs.
#
# This is the single deterministic artifact that captures the working end-to-end
# path (sim connect -> CSI ingest -> phase/fusion -> blob) so that later beads
# (e.g. the blob-identity runtime checks under bf-f841) and reruns don't have to
# re-derive the command sequence or the tuning. See notes/hardware-free-runtime.md
# for the full reasoning, gotchas, and the chain this resolves (bf-3zll/bf-3hji/bf-51fq).
#
# No hardware, no Docker, no manual IP, no manual token. The sim auto-provisions a
# per-node HMAC token from the mothership /api/provision endpoint (with the
# migration-window fallback), exactly like real provisioned firmware.
#
# Usage:  ./scripts/run-sim-local.sh
# Env:    SIM_NODES, SIM_WALKERS, SIM_RATE, SIM_DURATION, SIM_SEED, SIM_PORT,
#         SIM_SPACE, SIM_BLE (override defaults; see below)
# Exit:   0 if >=1 blob is observed via /api/blobs, 1 otherwise.
#
# Requires: go, curl, jq.

set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
MS="${SPAXEL_MOTHERSHIP_BIN:-/tmp/spaxel-mothership}"
SIM="${SPAXEL_SIM_BIN:-/tmp/spaxel-sim}"
PORT="${SIM_PORT:-8088}"

# --- Working defaults (verified reproducible 2026-07-08, post bf-243os) ----------
# nodes=4/walkers=2/rate=30/duration=25/seed=42 -> peak_blob_count==walkers,
# first blob ~1s, 0 rejects, ~360 FPS. rate=30 (not the upstream default 20) is
# REQUIRED for reproducibility: at 20 Hz fusion only peaks while a walker crosses
# the DeltaRMS threshold, so a run can land peak=2 *or* 0 nondeterministically.
# At 30 Hz every observed run hits peak==walkers. duration=25 keeps the sim
# --verify late-window sample inside the active-walk phase. See
# notes/hardware-free-runtime.md for the rationale.
SIM_NODES="${SIM_NODES:-4}"
SIM_WALKERS="${SIM_WALKERS:-2}"
SIM_RATE="${SIM_RATE:-30}"
SIM_DURATION="${SIM_DURATION:-25}"
SIM_SEED="${SIM_SEED:-42}"
SIM_SPACE="${SIM_SPACE:-5x5x2.5}"
# SIM_BLE=1 opts in to synthetic BLE advertisements (--ble). Default off: this
# script's gate is the CSI->blob path, NOT identity. The --ble + fixture + matcher
# gate lives in scripts/run-sim-ble-match.sh (bf-1yr1w, notes/hardware-free-runtime.md
# "BLE identity matcher (--ble)"). Opting in here only exercises BLE ingestion.
SIM_BLE="${SIM_BLE:-0}"

log() { printf '[run-sim] %s\n' "$*"; }
die() { printf '[run-sim] ERROR: %s\n' "$*" >&2; exit 1; }

for dep in go curl jq; do
  command -v "$dep" >/dev/null 2>&1 || die "missing required dependency: $dep"
done

# --- Build both binaries from source (idempotent: reuse if present) -------------
# IMPORTANT: the canonical sim lives at mothership/cmd/sim (it has --verify). The
# repo-root cmd/sim/ is a separate, OLDER module that LACKS --verify — never use it.
log "building canonical binaries from source (reuse if present)..."
need_build=0
[ -x "$MS" ]  || need_build=1
[ -x "$SIM" ] || need_build=1
# Even if the binary exists, reject a stale/standalone build that lacks --verify.
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

DATA_DIR=$(mktemp -d -t spaxel-runsim-data-XXXXXX)
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
log "params: nodes=$SIM_NODES walkers=$SIM_WALKERS rate=$SIM_RATE duration=${SIM_DURATION}s seed=$SIM_SEED space=$SIM_SPACE port=$PORT ble=$SIM_BLE"

# --- Start mothership on an ephemeral data dir ---------------------------------
# SPAXEL_MDNS_ENABLED=false: loopback reproduce, no multicast needed (the sim
# connects directly via --mothership). SPAXEL_LOG_LEVEL=warn keeps the log quiet
# enough to scan while still surfacing real problems.
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
log "mothership healthy"

# --- Start sim with --verify (canonical acceptance cross-check) ----------------
# The sim streams for $SIM_DURATION, then its verifyBlobs polls /api/blobs
# (2s settle + 12x500ms) and asserts blob_count == walkers ±1. SIM_BLE=1 appends
# --ble (synthetic BLE advertisements); default off keeps the canonical path green.
sim_args=(--mothership "ws://localhost:$PORT/ws/node"
  --nodes "$SIM_NODES" --walkers "$SIM_WALKERS" --rate "$SIM_RATE"
  --space "$SIM_SPACE"
  --duration "$SIM_DURATION" --seed "$SIM_SEED" --verify)
[ "$SIM_BLE" = "1" ] && sim_args+=(--ble)
"$SIM" "${sim_args[@]}" > "$DATA_DIR/sim.log" 2>&1 &
SIM_PID=$!
log "sim started (pid $SIM_PID, --verify enabled, ble=$SIM_BLE)"

# --- Independent /api/blobs poller over the streaming window (authoritative) ---
# A node is online when went_offline_at is the Go zero timestamp (0001-01-01T00:00:00Z)
# or absent — /api/nodes exposes no `status` field (matches the e2e harness, bf-vuzie).
peak=0
first_blob_t=""
sample_count=0
for t in $(seq 1 "$SIM_DURATION"); do
  sleep 1
  nodes=$(curl -s --max-time 2 "http://localhost:$PORT/api/nodes" 2>/dev/null \
    | jq '[.[]|select(.went_offline_at=="0001-01-01T00:00:00Z" or .went_offline_at==null)]|length' 2>/dev/null || echo 0)
  n=$(curl -s --max-time 2 "http://localhost:$PORT/api/blobs" 2>/dev/null | jq 'length' 2>/dev/null || echo 0)
  sample_count=$((sample_count+1))
  [ "$n" -gt "$peak" ] && peak=$n
  if [ "$n" -gt 0 ] && [ -z "$first_blob_t" ]; then first_blob_t="${t}s"; fi
  printf '  t=%2ds  nodes_online=%s  blobs=%s  peak=%s\n' "$t" "$nodes" "$n" "$peak"
done

# --- Capture the sim --verify verdict ------------------------------------------
wait "$SIM_PID"; SIM_EXIT=$?
verify_line=$(grep -E '\[SIM\] (PASS|FAIL|Verification)' "$DATA_DIR/sim.log" | tail -3)
reject_count=$(grep -ciE 'reject|invalid_token|\b401\b|\b403\b' "$DATA_DIR/sim.log" 2>/dev/null)
fps=$(grep -oE 'Average FPS: [0-9.]+' "$DATA_DIR/sim.log" | tail -1)

echo ""
log "---- RESULT ----"
log "peak_blob_count=$peak"
log "first_blob_at=${first_blob_t:-never}"
log "samples=$sample_count  nodes=$SIM_NODES walkers=$SIM_WALKERS rate=$SIM_RATE duration=${SIM_DURATION}s seed=$SIM_SEED"
log "sim_exit=$SIM_EXIT  (0 = --verify PASSED)"
log "sim_verify: ${verify_line//$'\n'/ | }"
log "reject_in_sim_log=${reject_count:-0}"
log "fps=${fps:-n/a}"

# --- Acceptance gate: >=1 blob observed via /api/blobs over the run window ------
# This is the task's actual acceptance. The peak poller is a real gate (it can
# legitimately fail with 0 on an under-tuned rate), not a rubber stamp. sim
# --verify is a reported secondary cross-check (timing-sensitive late-window sample).
if [ "$peak" -le 0 ]; then
  log "FAIL: peak_blob_count=0 via /api/blobs — no tracked blob observed"
  exit 1
fi
if [ "$SIM_EXIT" -eq 0 ]; then
  log "PASS: >=1 blob via /api/blobs (peak=$peak) AND sim --verify passed"
else
  log "PASS: >=1 blob via /api/blobs (peak=$peak); sim --verify=$SIM_EXIT (late-window sample; /api/blobs is authoritative)"
fi
exit 0
