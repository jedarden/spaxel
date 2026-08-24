# Empty-Room Baseline Capture for bf-3v39 (presence-detection)

**Date:** 2026-08-24
**Mothership:** https://spaxel.ardenone.com
**Bead:** spaxel-65c76469 (split-child 2 of bf-3v39)

## Task

Capture an empty-room baseline via API and verify that EMA stabilization reaches the expected noise floor (~0.02 deltaRMS).

## PHYSICAL PRECONDITION ⚠️

**The room must be completely EMPTY during the entire 60-second capture window.**

- **NO PEOPLE** in the detection zone
- **NO MOVING OBJECTS** (fans, swinging decorations, etc.)
- **QUIET ENVIRONMENT** - stationary ambient conditions only

**Operator must confirm:** Room is empty and will remain empty for the full 60-second capture.

## API Authentication

All baseline endpoints require **Google OAuth** authentication. You must be logged into your Google account via the browser to access these endpoints.

## Step 1: Start Baseline Capture

Execute this curl command (authenticated browser session required):

```bash
# Start 60-second quiet-room capture for all active links
curl -s -X POST https://spaxel.ardenone.com/api/baseline/capture \
  -H "Content-Type: application/json" \
  -d '{"links": []}' | jq .
```

**Expected Response:**
```json
{
  "ok": true,
  "links_captured": <N>,
  "links": ["link-A-B", "link-A-C", ...],
  "message": "Started baseline capture for N links"
}
```

**If `links_captured: 0`:**
- No links are active - nodes may not be connected
- Return to Step 1 of parent bead (node connectivity verification)

## Step 2: Wait 60 Seconds + EMA Stabilization

The baseline capture takes **60 seconds** of quiet-room sampling.

After capture completes, wait an additional **30-60 seconds** for the EMA (Exponential Moving Average) to stabilize with `tau=30s`.

**Total wait time:** ~90-120 seconds from capture start

## Step 3: Verify Baseline Recording

Check that baselines are recorded for each link/subcarrier:

```bash
curl -s https://spaxel.ardenone.com/api/baseline | jq .
```

**Expected Output:**
```json
[
  {
    "link_id": "AA:BB:CC:DD:EE:FF-11:22:33:44:55:66",
    "snapshot_time_ms": 1724498751000,
    "confidence": 0.95,
    "n_sub": 64
  },
  ...
]
```

**What to check:**
- Each link should have a baseline entry
- `confidence` should be > 0.8 (high-quality quiet-room capture)
- `n_sub` should match your ESP32 CSI configuration (typically 64)
- `snapshot_time_ms` should be recent (within last 2 minutes)

## Step 4: Verify EMA Stabilization (deltaRMS)

The critical verification is that deltaRMS settles to the **empty-room noise floor**.

**Expected deltaRMS:** ~0.02 (for truly empty room with WiFi ambient noise only)

**How to check:**
1. Monitor the spaxel dashboard at https://spaxel.ardenone.com
2. Look at the "Link Status" or "CSI Metrics" panel
3. Find the deltaRMS metric for each link
4. Confirm values are ~0.02 ± 0.01

**Alternative: API query (if available):**
```bash
# If there's a metrics endpoint for deltaRMS
curl -s https://spaxel.ardenone.com/api/links | jq '.[].delta_rms'
```

## Acceptance Criteria

The task is complete when ALL of the following are documented:

1. ✅ **Baseline ID:** Each link has a baseline recorded (`link_id` from Step 3)
2. ✅ **Timestamp:** Capture time is recent (within last 2-3 minutes)
3. ✅ **Coverage:** All active links have baselines (`links_captured` > 0)
4. ✅ **Confidence:** Each baseline has `confidence > 0.8`
5. ✅ **deltaRMS:** Observed deltaRMS ~0.02 (empty-room noise floor)

## Operator Actions

**Please perform these steps and report back with:**

1. **Confirmation:** Room is empty and will remain empty for 60 seconds
2. **Capture output:** Copy the full JSON response from Step 1
3. **Baseline list:** Copy the full JSON response from Step 3
4. **deltaRMS values:** Report the observed deltaRMS for each link (from dashboard or API)
5. **Timestamp:** Note the approximate time of capture (e.g., "2026-08-24 10:15 AM")

## Evidence to Collect

After successful capture, share:

```bash
# Capture response
{
  "ok": true,
  "links_captured": 2,
  "links": ["link1", "link2"]
}

# Baseline list
[
  {"link_id": "...", "confidence": 0.92, "n_sub": 64, "snapshot_time_ms": ...}
]

# deltaRMS observations
link1: 0.019
link2: 0.021
```

## Troubleshooting

### No links captured (`links_captured: 0`)
- Nodes are not connected - return to parent bead Step 1
- Verify `/healthz` shows `nodes_online > 0`
- Check node power and WiFi connection

### Low confidence (< 0.8)
- Room may not have been truly empty during capture
- Re-run capture with stricter empty-room conditions
- Check for interference (microwaves, other WiFi networks)

### High deltaRMS (> 0.05)
- Room may have had movement during capture
- Check for environmental noise (fans, HVAC, moving objects)
- Verify nodes are mounted securely (no vibration)

### Baseline not appearing in list
- Wait 60 seconds after capture starts
- Check mothership logs: `kubectl -n spaxel logs deployment/mothership`
- Verify database is writable: check SQLite file permissions

## Parent Bead Reference

This verification is split-child 2 of **bf-3v39** (presence-detection verification).
Depends on successful node connectivity verification from child 1.
