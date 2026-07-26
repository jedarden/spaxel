#!/usr/bin/env bash
# run-ws-capture.sh — bf-16tsv: start identity-less sim, capture RAW /ws/dashboard
# frames, tear down. Self-contained lifecycle. Diagnosis-only throwaway.
set -uo pipefail
ROOT=/home/coding/spaxel
MS=/tmp/spaxel-mothership
SIM=/tmp/spaxel-sim
PORT=8088
CAP="$ROOT/docs/notes/bf-4do5y-runtime-capture"
DATA=$(mktemp -d -t spaxel-wscap-XXXXXX)
MS_PID=""; SIM_PID=""

cleanup() {
  echo "[run] cleanup"
  kill "$SIM_PID" 2>/dev/null || true
  kill -INT "$MS_PID" 2>/dev/null || true
  sleep 1
  kill -9 "$MS_PID" 2>/dev/null || true
  wait 2>/dev/null || true
}
trap cleanup EXIT

echo "[run] starting mothership on $PORT (data=$DATA)"
SPAXEL_BIND_ADDR="127.0.0.1:$PORT" SPAXEL_DATA_DIR="$DATA" SPAXEL_LOG_LEVEL=warn \
  SPAXEL_MDNS_ENABLED=false TZ=UTC \
  "$MS" > "$DATA/ms.log" 2>&1 &
MS_PID=$!
ok=0
for i in $(seq 1 60); do
  if curl -s --max-time 2 "http://localhost:$PORT/healthz" 2>/dev/null | jq -e '.status=="ok"' >/dev/null 2>&1; then ok=1; break; fi
  sleep 0.3
done
if [ "$ok" -ne 1 ]; then echo "[run] FAIL: mothership never healthy"; echo "--- ms.log ---"; tail -20 "$DATA/ms.log"; exit 2; fi
echo "[run] mothership healthy (pid $MS_PID)"

echo "[run] starting identity-less sim (4 nodes, 3 walkers, rate 30, 120s)"
"$SIM" --mothership "ws://localhost:$PORT/ws/node" \
  --nodes 4 --walkers 3 --rate 30 --space 5x5x2.5 --duration 120 --seed 42 \
  > "$DATA/sim.log" 2>&1 &
SIM_PID=$!

peak=0
for i in $(seq 1 40); do
  n=$(curl -s --max-time 2 "http://localhost:$PORT/api/blobs" 2>/dev/null | jq 'length' 2>/dev/null || echo 0)
  [ "$n" -gt "$peak" ] && peak=$n
  if [ "$n" -gt 0 ]; then echo "[run] blobs flowing (api/blobs=$n)"; break; fi
  sleep 0.5
done
echo "[run] peak blobs so far: $peak"

echo "[run] running raw-WS capture (12s)..."
node "$CAP/capture-ws-frames.mjs" --port "$PORT" --duration 12000 2>&1
echo "[run] capture exit=$?"
echo "[run] DONE"
