# Empty-Room Baseline Capture - BLOCKED

**Date:** 2026-08-27 14:32 UTC
**Bead:** spaxel-65c76469 (split-child 2 of bf-3v39)
**Status:** ❌ BLOCKED - Prerequisite not met

## Current State Assessment

### Mothership Status
```json
{
  "blobs": 0,
  "detection_quality": 0,
  "nodes": 0,
  "uptime_s": 4412,
  "version": "dev"
}
```

**Status:** Mothership running normally (uptime: 73 minutes)

### Node Status
```json
[]
```

**Status:** **NO NODES CONNECTED** (0 nodes)

### Baseline Status
```json
[]
```

**Status:** **NO BASELINES RECORDED** (0 links = 0 baselines)

## Blocker Analysis

### Why Baseline Capture Cannot Proceed

The baseline capture API (`POST /api/baseline/capture`) has this logic:

```go
// From mothership/internal/api/baseline.go:140-147
if len(linksToCapture) == 0 {
    writeJSON(w, http.StatusOK, captureResponse{
        OK:            true,
        LinksCaptured: 0,
        Message:       "No links found to capture. Capture will start automatically once links are active.",
    })
    return
}
```

**With 0 nodes connected:**
- No links exist in the system
- No CSI data is being captured
- No baseline can be recorded
- Capture API returns: `"links_captured": 0`

### Prerequisite Dependency

This task is **split-child 2 of bf-3v39** and explicitly **depends on node-connectivity verification (child 1)**.

From task description:
> "Depends on node-connectivity verification (child 1)"

From troubleshooting guide in notes/bf-3v39-baseline.md:
> "If `links_captured: 0`: Nodes are not connected - return to parent bead Step 1"

## Required Actions Before This Task Can Proceed

### Step 1: Complete Node Connectivity Verification (child 1)

The following must be completed first:

1. **Physical Node Setup**
   - Ensure ESP32-S3 nodes are powered on
   - Verify WiFi credentials are configured
   - Confirm nodes can reach mothership network

2. **Node Registration**
   - Nodes should appear in `/api/nodes`
   - Each node should have: `status: "online"`, `role` assigned, position configured

3. **Link Formation**
   - TX→RX pairs establish automatically
   - Links should appear in system once nodes are streaming CSI

4. **Verification**
   - Confirm `/api/nodes` shows `nodes > 0`
   - Confirm `/api/links` (if available) shows active links
   - Confirm `/healthz` shows `nodes_online > 0`

### Step 2: Capture Empty-Room Baseline (this task)

Once nodes are connected, this task can proceed:

1. **Physical Precondition**
   - ✅ Operator confirms: **ROOM IS EMPTY**
   - ✅ No people in detection zone
   - ✅ No moving objects (fans, etc.)
   - ✅ Quiet environment

2. **Start Capture**
   ```bash
   curl -s -X POST https://spaxel.ardenone.com/api/baseline/capture \
     -H "Content-Type: application/json" \
     -d '{"links": []}' | jq .
   ```
   Expected: `"links_captured": <N>` where N > 0

3. **Wait 60s + EMA Stabilization**
   - 60s quiet-room sampling
   - Additional 30-60s for EMA (tau=30s) to stabilize
   - Total: ~90-120s

4. **Verify Baseline Recording**
   ```bash
   curl -s https://spaxel.ardenone.com/api/baseline | jq .
   ```
   Expected: Array of baseline entries with:
   - `link_id` per link
   - `confidence > 0.8`
   - `n_sub: 64`
   - `snapshot_time_ms` recent

5. **Verify deltaRMS ~0.02**
   - Check dashboard or API for deltaRMS per link
   - Expected: ~0.02 ± 0.01 (empty-room noise floor)

## Acceptance Criteria (when unblocked)

All of the following must be documented:

- [ ] **Baseline ID:** Each link has baseline recorded
- [ ] **Timestamp:** Capture time recent (within 2-3 minutes)
- [ ] **Coverage:** All active links have baselines (links_captured > 0)
- [ ] **Confidence:** Each baseline has confidence > 0.8
- [ ] **deltaRMS:** Observed deltaRMS ~0.02 (empty-room noise floor)

## Recommendations

### For Immediate Next Steps

1. **Check Physical Setup**
   - Are ESP32-S3 nodes powered on?
   - Are they connected to the correct WiFi network?
   - Can they reach the mothership at https://spaxel.ardenone.com?

2. **Check Mothership Logs**
   ```bash
   kubectl -n spaxel logs deployment/mothership --tail=100
   ```
   Look for:
   - Node connection attempts
   - WebSocket handshake errors
   - CSI frame reception

3. **Verify Network Connectivity**
   - Mothership is at https://spaxel.ardenone.com
   - Nodes must be able to reach this hostname
   - Check DNS resolution from node network

4. **Complete Child 1 (Node Connectivity)**
   - This is the blocking prerequisite
   - Must be completed before baseline capture can proceed

### For Testing Setup

If this is a development environment without physical nodes:

1. **Use the Simulator**
   ```bash
   spaxel-sim --mothership ws://localhost:8080/ws/node \
     --nodes 4 --walkers 0 --duration 120
   ```

2. **Verify Nodes Appear**
   ```bash
   curl -s http://localhost:8080/api/nodes | jq .
   ```
   Should show 4 nodes

3. **Then Proceed with Baseline Capture**
   - Room will be empty (0 walkers)
   - Baselines should record automatically
   - deltaRMS should stabilize at ~0.02

## Conclusion

**STATUS:** ❌ BLOCKED - Cannot proceed

**REASON:** No nodes connected to mothership (0 nodes). Baseline capture requires active links, which require connected nodes.

**BLOCKER:** This task depends on successful completion of child 1 (node-connectivity verification).

**NEXT STEP:** Complete node connectivity verification first, then retry baseline capture.

---

**Documented:** 2026-08-27 14:32 UTC
**Mothership:** https://spaxel.ardenone.com (or http://localhost:8080 for local testing)
