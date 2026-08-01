#!/usr/bin/env bash
# bf-5k1z — node-position-vs-blob-state contrast, captured DURING a live run.
#
# Mirrors the IO-6 hard-gate test harness EXACTLY:
#   - mothership env: SPAXEL_BIND_ADDR=127.0.0.1:8080, SPAXEL_MIGRATION_WINDOW_HOURS=0
#     (strict auth), SPAXEL_LOG_LEVEL=info, TZ=UTC, fresh ephemeral data dir
#   - sim flags: --mothership ws://127.0.0.1:8080/ws/node --nodes 4 --walkers 2
#     --rate 20 --duration 45 --ble --seed 42   (provisioning ON by default)
#
# The gated test's CaptureIO6Diagnostics runs at t~=30s, right as the sim exits, so
# /api/status (online-only) reads nodes=0 and /api/nodes may read empty. This script
# polls EVERY 3s DURING the run so the bf-4q5w wiring-gap signature — distinct
# announced node positions in /api/nodes vs ZERO tracked blobs in /api/blobs — is
# captured while the sim is still connected. It also captures mothership stderr to
# prove admission (hello -> online) vs rejection (invalid_token).
set -u

MSHIP=/tmp/spaxel-mothership-test
SIM=/tmp/spaxel-sim-test
API=http://127.0.0.1:8080
OUT=/home/coding/spaxel/.beads/traces/bf-5k1z

if [[ ! -x "$MSHIP" || ! -x "$SIM" ]]; then
  echo "FATAL: fresh test binaries missing at /tmp (run the gated test first so the harness builds them)"; exit 2
fi

TMPDIR=$(mktemp -d /tmp/spaxel-bf5k1z-XXXXXX)
echo "data dir: $TMPDIR"
echo "mothership binary: $MSHIP"; echo "sim binary: $SIM"

cleanup() {
  [[ -n "${SIM_PID:-}" ]] && kill "$SIM_PID" 2>/dev/null && wait "$SIM_PID" 2>/dev/null
  [[ -n "${MS_PID:-}" ]]   && kill -INT "$MS_PID" 2>/dev/null && wait "$MS_PID" 2>/dev/null
}
trap cleanup EXIT

# --- start mothership (exact harness env) ---
SPAXEL_BIND_ADDR=127.0.0.1:8080 \
SPAXEL_DATA_DIR="$TMPDIR" \
SPAXEL_LOG_LEVEL=info \
TZ=UTC \
SPAXEL_MIGRATION_WINDOW_HOURS=0 \
"$MSHIP" >"$TMPDIR/mothership.stdout" 2>"$TMPDIR/mothership.stderr" &
MS_PID=$!

for i in $(seq 1 40); do
  curl -sf "$API/healthz" >/dev/null 2>&1 && break
  sleep 0.5
done
if ! curl -sf "$API/healthz" >/dev/null 2>&1; then
  echo "ERROR: mothership never became healthy"; tail -30 "$TMPDIR/mothership.stderr"; exit 1
fi
echo "mothership healthy (PID $MS_PID)"
echo

# --- start sim (exact harness flags, duration 45 for polling headroom) ---
"$SIM" --mothership ws://127.0.0.1:8080/ws/node \
  --nodes 4 --walkers 2 --rate 20 --duration 45 --ble --seed 42 \
  >"$TMPDIR/sim.stdout" 2>"$TMPDIR/sim.stderr" &
SIM_PID=$!
echo "sim started (PID $SIM_PID) — 4 nodes, 2 walkers, 20 Hz, 45s, ble, seed 42"
echo

# --- poll during the run ---
{
printf "%-4s | %-26s | %-10s | %-10s\n" "t(s)" "/api/nodes (mac[-8:]=x,y,z)" "/api/blobs" "detections"
printf '%s\n' "-----+----------------------------+------------+------------"
for t in 3 6 9 12 15 18 21 24 27 30 33 36 39 42 45 48; do
  sleep 3
  nodes_json=$(curl -sf "$API/api/nodes" 2>/dev/null || echo '[]')
  blobs_n=$(curl -sf "$API/api/blobs" 2>/dev/null | python3 -c 'import sys,json;print(len(json.load(sys.stdin)))' 2>/dev/null || echo '?')
  det=$(curl -sf "$API/api/events?type=detection&limit=100" 2>/dev/null | python3 -c 'import sys,json;print(len(json.load(sys.stdin).get("events",[])))' 2>/dev/null || echo '?')
  nodes_summary=$(printf '%s' "$nodes_json" | python3 -c 'import sys,json
try:
  arr=json.load(sys.stdin)
  if not arr: print("(0 nodes registered)")
  else:
    print(" ".join(f"{n.get(\"mac\",\"?\")[-8:]}=({n.get(\"pos_x\",0):.1f},{n.get(\"pos_y\",0):.1f},{n.get(\"pos_z\",0):.1f})" for n in arr))
except Exception: print("parse-err")' 2>/dev/null)
  printf "%-4s | %-26s | %-10s | %-10s\n" "$t" "$nodes_summary" "$blobs_n" "$det"
done
} | tee "$OUT/node-vs-blob-contrast.txt"
echo

echo "=== sim.stderr (provisioning + connection evidence) ===" | tee -a "$OUT/node-vs-blob-contrast.txt"
tail -20 "$TMPDIR/sim.stderr" | tee -a "$OUT/node-vs-blob-contrast.txt"
echo
echo "=== mothership stderr: admission vs rejection ===" | tee -a "$OUT/node-vs-blob-contrast.txt"
grep -iE "hello|provision|node.*(online|reject|invalid|admit)|SetNodePosition|fusion engine" "$TMPDIR/mothership.stderr" | tail -40 | tee -a "$OUT/node-vs-blob-contrast.txt"

# keep raw logs for the trace
cp "$TMPDIR/sim.stderr" "$OUT/sim.stderr.txt"
cp "$TMPDIR/mothership.stderr" "$OUT/mothership.stderr.txt"
echo
echo "saved: $OUT/node-vs-blob-contrast.txt, sim.stderr.txt, mothership.stderr.txt"
