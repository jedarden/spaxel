#!/usr/bin/env bash
# bf-ekr9c: prove spaxel-sim connects window-INDEPENDENT of SPAXEL_MIGRATION_WINDOW_HOURS.
# For each of WINDOW=0 (strict), 24 (default), 8760 (large): boot a fresh mothership,
# run spaxel-sim with default --provision, and assert: all nodes connect + receive roles,
# fps>0, and ZERO reject/REJECT/invalid_token/401/403 in BOTH the sim log and mothership stderr.
set -uo pipefail

MS=/tmp/spaxel-mothership
SIM=/tmp/spaxel-sim
PORT=8099
BIND=127.0.0.1:$PORT
WS=ws://127.0.0.1:$PORT/ws/node

run_one() {
  local WINDOW="$1"
  local TAG="$2"
  local DATADIR MSLOG SIMLOG
  DATADIR=$(mktemp -d /tmp/spaxel-win-XXXX)
  MSLOG=$(mktemp /tmp/spaxel-mslog-XXXX)
  SIMLOG=$(mktemp /tmp/spaxel-simlog-XXXX)

  echo "=================================================================="
  echo "[$TAG] WINDOW=$WINDOW  datadir=$DATADIR"
  echo "=================================================================="

  SPAXEL_BIND_ADDR=$BIND \
  SPAXEL_DATA_DIR="$DATADIR" \
  SPAXEL_MIGRATION_WINDOW_HOURS=$WINDOW \
  SPAXEL_LOG_LEVEL=info \
  TZ=UTC \
    "$MS" >"$MSLOG" 2>&1 &
  local MSPID=$!

  # Wait for healthy (budget ~15s)
  local ok=""
  for _ in $(seq 1 50); do
    if curl -fsS "http://127.0.0.1:$PORT/healthz" >/dev/null 2>&1; then ok=1; break; fi
    sleep 0.3
  done
  if [ -z "$ok" ]; then
    echo "[$TAG] FAIL: mothership never became healthy"; echo "--- mothership log ---"; cat "$MSLOG"; kill $MSPID 2>/dev/null; return 1
  fi
  echo "[$TAG] mothership healthy: $(curl -fsS http://127.0.0.1:$PORT/healthz)"

  # Run the sim with default --provision (mints real per-node HMAC tokens via /api/provision)
  "$SIM" --mothership "$WS" --nodes 4 --walkers 1 --rate 20 --duration 12 --seed 42 >"$SIMLOG" 2>&1
  local SIMRC=$?

  echo "[$TAG] sim exit code: $SIMRC"

  # Give graceful shutdown a moment, then stop mothership
  kill -INT $MSPID 2>/dev/null; sleep 1; kill -9 $MSPID 2>/dev/null

  echo "----- [$TAG] SIM LOG (key lines) -----"
  grep -E "\[SIM\] (Provisioning real per-node|Node [0-9]+ \([0-9A-F:]+\): provisioned real token|Node [0-9]+: received role message|Stats:|Frames sent|Average FPS|REJECT message|Exiting due to rejection)" "$SIMLOG" | head -40

  echo "----- [$TAG] MOTHERSHIP stderr (node/auth lines) -----"
  grep -iE "Node connected|Node .* reject|invalid_token|migration window|provisioning:" "$MSLOG" | head -40

  echo "----- [$TAG] NEGATIVE GREP (word-bounded, must be 0) -----"
  echo "[sim] reject/REJECT/invalid_token/<401>/<403> occurrences:"
  grep -cE "reject|REJECT|invalid_token|\b401\b|\b403\b" "$SIMLOG" || true
  echo "[mothership] reject/REJECT/invalid_token/<401>/<403> occurrences:"
  grep -cE "reject|REJECT|invalid_token|\b401\b|\b403\b" "$MSLOG" || true

  # Assertions
  local FAIL=0
  if [ "$SIMRC" -ne 0 ]; then echo "[$TAG] ASSERT FAIL: sim exited non-zero ($SIMRC)"; FAIL=1; fi
  local ROLES
  ROLES=$(grep -cE "\[SIM\] Node [0-9]+: received role message" "$SIMLOG" || true)
  if [ "$ROLES" -lt 4 ]; then echo "[$TAG] ASSERT FAIL: only $ROLES nodes received roles (need 4)"; FAIL=1; fi
  local PROV
  PROV=$(grep -cE "\[SIM\] Node [0-9]+ \([0-9A-F:]+\): provisioned real token" "$SIMLOG" || true)
  if [ "$PROV" -lt 4 ]; then echo "[$TAG] ASSERT FAIL: only $PROV nodes provisioned real tokens via /api/provision (need 4)"; FAIL=1; fi
  local FPSLINE
  FPSLINE=$(grep -oE "Average FPS: [0-9.]+" "$SIMLOG" | tail -1 || true)
  if [ -z "$FPSLINE" ]; then echo "[$TAG] ASSERT FAIL: no FPS reported"; FAIL=1; fi
  local SIMNEG MSNEG
  SIMNEG=$(grep -cE "reject|REJECT|invalid_token|\b401\b|\b403\b" "$SIMLOG" || true)
  MSNEG=$(grep -cE "reject|REJECT|invalid_token|\b401\b|\b403\b" "$MSLOG" || true)
  if [ "$SIMNEG" -ne 0 ]; then echo "[$TAG] ASSERT FAIL: $SIMNEG reject/auth strings in sim log"; FAIL=1; fi
  if [ "$MSNEG" -ne 0 ]; then echo "[$TAG] ASSERT FAIL: $MSNEG reject/auth strings in mothership log"; FAIL=1; fi

  # Persist log paths for evidence harvesting
  echo "$DATADIR $MSLOG $SIMLOG $TAG $WINDOW $SIMRC roles=$ROLES prov=$PROV \"$FPSLINE\" sim-neg=$SIMNEG ms-neg=$MSNEG" >> /tmp/window_test_results.txt

  if [ "$FAIL" -eq 0 ]; then
    echo "[$TAG] RESULT: PASS (roles=$ROLES, $FPSLINE, sim-neg=0, ms-neg=0)"
  else
    echo "[$TAG] RESULT: FAIL"
  fi
  echo
}

rm -f /tmp/window_test_results.txt
run_one 0   "STRICT"   || true
run_one 24  "DEFAULT"  || true
run_one 8760 "LARGE"   || true

echo "=================================================================="
echo "SUMMARY"
echo "=================================================================="
cat /tmp/window_test_results.txt
