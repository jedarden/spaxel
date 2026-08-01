#!/usr/bin/env bash
# bf-5k1z probe: capture the node-position-vs-blob-state contrast DURING a live
# spaxel-sim run, so the bf-4q5w wiring-gap signature (distinct announced node
# positions + zero tracked blobs) is recorded while the sim is still connected
# (the gated test's CaptureIO6Diagnostics runs at t~=30s, right as the sim exits,
# so /api/nodes can read empty by then).
#
# Mirrors the IO-6 hard-gate config: 4 nodes, 2 walkers, 20 Hz, --ble, --seed 42
# (same as TestIO6HardGate_WalkerProducesTrackedBlob), but runs the sim for 40s
# and polls /api/nodes + /api/blobs + /api/status + detection events every 3s.
set -u

MSHIP=/tmp/spaxel-mothership-test
SIM=/tmp/spaxel-sim-test
API=http://localhost:8080

TMPDIR=$(mktemp -d /tmp/spaxel-bf5k1z-XXXXXX)
echo "data dir: $TMPDIR"

cleanup() {
  [[ -n "${SIM_PID:-}" ]] && kill "$SIM_PID" 2>/dev/null && wait "$SIM_PID" 2>/dev/null
  [[ -n "${MS_PID:-}" ]]   && kill -INT "$MS_PID" 2>/dev/null && wait "$MS_PID" 2>/dev/null
  rm -rf "$TMPDIR"
}
trap cleanup EXIT

# Start mothership (same env as TestHarness.Start)
SPAXEL_BIND_ADDR=127.0.0.1:8080 \
SPAXEL_DATA_DIR="$TMPDIR" \
SPAXEL_LOG_LEVEL=info \
TZ=UTC \
SPAXEL_MIGRATION_WINDOW_HOURS=0 \
"$MSHIP" >"$TMPDIR/mothership.stdout" 2>"$TMPDIR/mothership.stderr" &
MS_PID=$!

# Wait for health (up to 15s)
for i in $(seq 1 30); do
  if curl -sf "$API/healthz" >/dev/null 2>&1; then break; fi
  sleep 0.5
done
if ! curl -sf "$API/healthz" >/dev/null 2>&1; then
  echo "ERROR: mothership never became healthy"
  cat "$TMPDIR/mothership.stderr" | tail -30
  exit 1
fi
echo "mothership healthy (PID $MS_PID)"
echo

# Start sim: 4 nodes, 2 walkers, 20 Hz, 40s, ble, seed 42
"$SIM" --mothership ws://localhost:8080/ws/node \
  --nodes 4 --walkers 2 --rate 20 --duration 40 --ble --seed 42 \
  >"$TMPDIR/sim.stdout" 2>"$TMPDIR/sim.stderr" &
SIM_PID=$!
echo "sim started (PID $SIM_PID) — 4 nodes, 2 walkers, 20 Hz, 40s"
echo

printf "%-4s %-22s %-30s %-12s %-10s\n" "t(s)" "/api/status" "/api/nodes (mac: pos_x,pos_y,pos_z)" "/api/blobs" "detections"
printf "%-4s %-22s %-30s %-12s %-10s\n" "----" "----------------------" "------------------------------" "------------" "----------"

for t in $(seq 3 3 45); do
  sleep 3
  status=$(curl -sf "$API/api/status" 2>/dev/null || echo '{"err":"status"}')
  nodes_json=$(curl -sf "$API/api/nodes" 2>/dev/null || echo '[]')
  blobs_n=$(curl -sf "$API/api/blobs" 2>/dev/null | python3 -c 'import sys,json;print(len(json.load(sys.stdin)))' 2>/dev/null || echo '?')
  det=$(curl -sf "$API/api/events?type=detection&limit=100" 2>/dev/null | python3 -c 'import sys,json;print(len(json.load(sys.stdin).get("events",[])))' 2>/dev/null || echo '?')

  st_summary=$(echo "$status" | python3 -c 'import sys,json
try:
  d=json.load(sys.stdin); print(f"nodes={d.get(\"nodes\")} blobs={d.get(\"blobs\")} q={d.get(\"detection_quality\")}")
except Exception as e: print("parse-err")' 2>/dev/null)

  nodes_summary=$(echo "$nodes_json" | python3 -c 'import sys,json
try:
  arr=json.load(sys.stdin)
  if not arr: print("(no nodes registered)")
  else:
    parts=[]
    for n in arr:
      parts.append(f"{n.get(\"mac\",\"?\")[-8:]}:({n.get(\"pos_x\",0):.2f},{n.get(\"pos_y\",0):.2f},{n.get(\"pos_z\",0):.2f})")
    print(" ".join(parts))
except Exception as e: print("parse-err")' 2>/dev/null)

  printf "%-4s %-22s %-30s %-12s %-10s\n" "$t" "$st_summary" "$nodes_summary" "$blobs_n" "$det"
done

echo
echo "=== sim.stderr (connection / announce evidence) ==="
cat "$TMPDIR/sim.stderr" 2>/dev/null | tail -25
echo
echo "=== mothership stderr tail (node hello / position evidence) ==="
grep -iE "hello|position|node.*online|register|reject|token" "$TMPDIR/mothership.stderr" 2>/dev/null | tail -25
