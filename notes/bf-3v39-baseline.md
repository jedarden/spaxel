# Empty-Room Baseline Capture for bf-3v39 (presence-detection)

**Date:** 2026-08-29
**Mothership:** https://spaxel.ardenone.com
**Bead:** spaxel-65c76469 (split-child 2 of bf-3v39)
**Status:** ❌ BLOCKED - No nodes connected

## Current State (2026-08-29 09:06 UTC)

### Mothership Health
```json
{
  "status": "ok",
  "uptime_s": 1222969,
  "version": "0.2.24",
  "nodes_online": 0,
  "db": "ok",
  "shedding_level": 0,
  "reason": "no nodes connected"
}
```

**Analysis:**
- ✅ Mothership running normally (14.1 days uptime)
- ✅ Database operational
- ❌ **Zero nodes connected** - blocker remains active
- ❌ No links = no CSI data = no baseline possible

### Blocker Summary

**Cannot proceed:** No nodes connected to mothership
**Blocking dependency:** Child 1 (spaxel-082135bc - node connectivity verification)
**Last node seen:** 2026-08-07 11:50:18 EDT (22 days ago)

## Task Requirements (for when nodes are online)

### PHYSICAL PRECONDITION ⚠️

**The room must be completely EMPTY during the entire 60-second capture window:**
- **NO PEOPLE** in the detection zone
- **NO MOVING OBJECTS** (fans, swinging decorations, etc.)
- **QUIET ENVIRONMENT** - stationary ambient conditions only

**Operator must confirm:** Room is empty and will remain empty for the full 60-second capture.

### Step 1: Start Baseline Capture

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
  "message": "Baseline capture started. Keep the room clear for 60 seconds for best results."
}
```

**If `links_captured: 0`:**
- No links are active - nodes may not be connected
- Return to parent bead Step 1 (node connectivity verification)

### Step 2: Wait 60 Seconds + EMA Stabilization

- **Capture duration:** 60 seconds
- **EMA stabilization (tau=30s):** 30-60 seconds additional
- **Total wait:** ~90-120 seconds from capture start

### Step 3: Verify Baseline Recording

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
- `confidence` > 0.8 (high-quality quiet-room capture)
- `n_sub` matches ESP32 CSI config (typically 64)
- `snapshot_time_ms` is recent (within last 2 minutes)

### Step 4: Verify EMA Stabilization (deltaRMS)

**Expected deltaRMS:** ~0.02 (empty-room noise floor with WiFi ambient noise only)

**How to check:**
1. Monitor dashboard at https://spaxel.ardenone.com
2. Look at "Link Status" or "CSI Metrics" panel
3. Find deltaRMS metric for each link
4. Confirm values ~0.02 ± 0.01

## Acceptance Criteria

The task is complete when ALL of the following are documented:

1. ✅ **Baseline ID:** Each link has a baseline recorded (`link_id` from Step 3)
2. ✅ **Timestamp:** Capture time is recent (within last 2-3 minutes)
3. ✅ **Coverage:** All active links have baselines (`links_captured` > 0)
4. ✅ **Confidence:** Each baseline has `confidence > 0.8`
5. ✅ **deltaRMS:** Observed deltaRMS ~0.02 (empty-room noise floor)

## Required Actions to Unblock

### Immediate: Restore Node Connectivity

**Physical verification required:**

1. **Check physical node status:**
   - Is ESP32-S3 powered on?
   - Is it connected to the correct WiFi network?
   - Can it reach https://spaxel.ardenone.com?

2. **Verify mothership logs:**
   ```bash
   kubectl -n spaxel logs deployment/mothership --tail=100 | grep -E "hello|node|WS|CSI"
   ```
   Look for:
   - Node `hello` messages with MAC addresses
   - WebSocket connection attempts
   - CSI frame reception

3. **Re-provision if needed:**
   - Use Web Serial from dashboard to re-provision
   - Verify WiFi credentials in NVS
   - Confirm mDNS can reach mothership
   - Check for firmware update (0.2.19 → 0.2.24)

4. **Verify node appears in system:**
   ```bash
   curl -s https://spaxel.ardenone.com/api/nodes | jq .
   ```
   Should show at least one node with `status: "online"`

### Then Proceed with Baseline Capture

Once nodes are connected and links are active:

#### Physical Precondition
- ✅ **Operator confirms: ROOM IS EMPTY**
- ✅ No people in detection zone
- ✅ No moving objects (fans, etc.)
- ✅ Quiet environment

#### Capture Process
```bash
# 1. Start 60s capture
curl -s -X POST https://spaxel.ardenone.com/api/baseline/capture \
  -H "Content-Type: application/json" \
  -d '{}' | jq .
```
Expected: `"links_captured": <N>` where N > 0

```bash
# 2. Wait 60s + EMA stabilization (tau=30s)
# Total: ~90-120 seconds
```

```bash
# 3. Verify baseline recorded
curl -s https://spaxel.ardenone.com/api/baseline | jq .
```
Expected: Array of baseline entries with:
- `link_id` per link
- `confidence > 0.8`
- `n_sub: 64`
- `snapshot_time_ms` recent

```bash
# 4. Verify deltaRMS ~0.02 (empty-room noise floor)
# Check dashboard or metrics API
```
Expected: deltaRMS ~0.02 ± 0.01

## Dependency Chain

This task is **split-child 2 of bf-3v39** with explicit dependencies:

1. **Child 1:** spaxel-082135bc - "Node connectivity verification"
   - **Status:** Open, BLOCKED (node offline since Aug 7)

2. **Child 2:** spaxel-65c76469 - **THIS TASK** (empty-room baseline capture via API)
   - **Status:** InProgress, BLOCKED (depends on child 1)

3. **Child 3:** spaxel-b075c0f3 - "Run walk-through presence test and record deltaRMS spike"
   - **Status:** Open (blocked by child 1)

4. **Child 4:** spaxel-2ce98275 - "Verify presence blob in dashboard and document final verification record"
   - **Status:** Open (blocked by child 3)

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

---

**Last Updated:** 2026-08-29 09:06 UTC
**Mothership:** https://spaxel.ardenone.com
**Status:** BLOCKED - Awaiting node connectivity (Child 1)
