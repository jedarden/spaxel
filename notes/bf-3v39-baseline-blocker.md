# Empty-Room Baseline Capture - BLOCKED

**Date:** 2026-08-27 18:45 UTC  
**Bead:** spaxel-65c76469 (split-child 2 of bf-3v39)  
**Status:** ❌ BLOCKED - Dependency Unmet  

## Blocker Summary

**Cannot proceed:** No nodes connected to mothership  
**Blocking dependency:** Child 1 (spaxel-082135bc - node connectivity verification)  

## Current State

### Mothership Status
```json
{
  "status": "ok",
  "uptime_s": 1070306,
  "version": "0.2.24",
  "nodes_online": 0,
  "db": "ok",
  "shedding_level": 0,
  "reason": "no nodes connected"
}
```
- ✅ Mothership running normally (12.4 days uptime)
- ❌ Zero nodes connected

### Node Status (from child 1 bead)
- **Node MAC:** Unknown (sole persisted ESP32)
- **Status:** Offline
- **Last seen:** 2026-08-07 11:50:18 EDT (20 days ago)
- **Firmware:** 0.2.19
- **Role:** tx_rx

### Baseline System State
```json
[]
```
- No links exist
- No baseline snapshots stored
- No CSI data streaming

## Why This Task Cannot Proceed

The baseline capture API (`POST /api/baseline/capture`) requires active links:

```go
// From mothership/internal/api/baseline.go:120-147
// Get all unique link_ids from the baselines table
rows, err := h.db.Query("SELECT DISTINCT link_id FROM baselines")
if err != nil {
    // returns error
}
// If no links found:
if len(linksToCapture) == 0 {
    writeJSON(w, http.StatusOK, captureResponse{
        OK:            true,
        LinksCaptured: 0,
        Message:       "No links found to capture. Capture will start automatically once links are active.",
    })
    return
}
```

**With 0 nodes:**
- No TX→RX links exist
- No CSI data is being captured
- No baseline can be recorded
- API will return `"links_captured": 0`

## Dependency Chain

This task is **split-child 2 of bf-3v39** with explicit dependencies:

1. **Child 1:** spaxel-082135bc - "Capture baseline and verify presence detection with first node"
   - **Status:** Open, BLOCKED (node offline since Aug 7)
   
2. **Child 2:** spaxel-65c76469 - **THIS TASK** (empty-room baseline capture via API)
   - **Status:** InProgress, BLOCKED (depends on child 1)
   
3. **Child 3:** spaxel-b075c0f3 - "Run walk-through presence test and record deltaRMS spike"
   - **Status:** Open (blocked by child 1)

4. **Child 4:** spaxel-2ce98275 - "Verify presence blob in dashboard and document final verification record"
   - **Status:** Open (blocked by child 3)

## Required Actions to Unblock

### Step 1: Restore Node Connectivity (Child 1 Prerequisite)

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

### Step 2: Verify Link Formation

Once node is online:
- Check `/api/links` (if available) or dashboard
- Confirm CSI streaming is active
- Verify role assignment (tx_rx, rx, or passive)

### Step 3: Proceed with Baseline Capture (This Task)

**Once nodes are connected and links are active:**

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

## Acceptance Criteria

All of the following must be documented:

- [ ] **Baseline ID:** Each link has baseline recorded
- [ ] **Timestamp:** Capture time recent (within 2-3 minutes)
- [ ] **Coverage:** All active links have baselines (links_captured > 0)
- [ ] **Confidence:** Each baseline has confidence > 0.8
- [ ] **deltaRMS:** Observed deltaRMS ~0.02 (empty-room noise floor)
- [ ] **Documentation:** Evidence recorded to notes/bf-3v39-baseline.md
- [ ] **Comment:** Summary posted on this bead

## Current Status

**STATUS:** ❌ BLOCKED - Cannot proceed  
**REASON:** No nodes connected to mothership (0 nodes). Baseline capture requires active links.  
**BLOCKER:** Child 1 (node connectivity verification) must complete first.  
**NEXT STEP:** Restore node connectivity, then retry baseline capture.

---

**Documented:** 2026-08-27 18:45 UTC  
**Mothership:** https://spaxel.ardenone.com  
**Node Status:** Offline since 2026-08-07 11:50:18 EDT (20 days ago)
