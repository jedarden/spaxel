# Walk-Through Presence Test - BF-3V39 Child 3

## Task Context
- **Bead ID**: spaxel-b075c0f3 (split-child 3 of bf-3v39)
- **Parent Task**: BF-3V39 (baseline capture blockers verification)
- **Dependency**: Baseline capture (child 2) must be complete

## Physical Precondition ⚠️
**OPERATOR REQUIRED**: A person must physically walk through the detection area between the spaxel node and the home WiFi AP. The agent cannot perform this action.

## Technical Requirements
- Mothership URL: http://localhost:8088 (local) or https://spaxel.ardenone.com (remote)
- API Endpoints:
  - GET /api/status - Check node connectivity and deltaRMS
  - GET /api/blobs - Check blob tracking evidence
  - GET /api/explain/{blob_id} - Get deltaRMS from contributing links
- Expected Behavior: deltaRMS spike from baseline (~0.02) to > 0.05 during walk
- Test Duration: 60 seconds minimum walk-through period

## System Status Assessment (2026-08-29)

### Remote Mothership
- **URL**: https://spaxel.ardenone.com
- **Status**: ✅ Healthy (uptime: 14+ days, version: 0.2.24)
- **Nodes Online**: ❌ **0 nodes**
- **Authentication**: Google OAuth Required

### Local Mothership
- **URL**: http://localhost:8088
- **Status**: ✅ Running (uptime: 422s, version: dev)
- **Nodes Online**: ❌ **0 nodes**
- **Blobs Tracked**: 0
- **Detection Quality**: 0

### Critical Blocker
**❌ NO PHYSICAL NODES DEPLOYED**

The fundamental blocker is that **no spaxel nodes are deployed or connected**. Without nodes:
- No CSI data is being captured
- No WiFi links exist for presence detection
- No baseline can be established (child 2 confirmed: 0 nodes = 0 links = impossible)
- No walk-through test can be performed
- No deltaRMS measurements can be taken

## Monitoring Infrastructure

### Script Available
**`scripts/walkthrough_monitor.sh`** — Comprehensive monitoring script ready for use

Features:
- Polls deltaRMS via `/api/explain/{blob_id}` every 1 second
- Tracks blob count via `/api/blobs`
- Records timestamps for correlation with walk timing
- Evaluates spike detection against 0.05 threshold
- Generates timestamped log files in `data/walkthrough/`

Usage:
```bash
./scripts/walkthrough_monitor.sh [duration_seconds]
# Default: 60 seconds
# Output: data/walkthrough/walkthrough_YYYYMMDD_HHMMSS.log
```

### API Polling Pattern
```bash
# Poll blob count and blob IDs
curl -s http://localhost:8088/api/blobs | jq '.[].id'

# For each blob, get deltaRMS from contributing links
curl -s http://localhost:8088/api/explain/{blob_id} | jq '.contributing_links[].delta_rms'

# Check system status
curl -s http://localhost:8088/api/status | jq '{nodes: .nodes, blobs: .blobs, quality: .detection_quality}'
```

## Expected Walk-Through Results

### Success Criteria
- **deltaRMS spike**: Peak value > 0.05 during walk (baseline ~0.02)
- **Blob tracking**: At least 1 blob observed during walk
- **Timestamp correlation**: Spike timing matches walk-through period
- **Post-walk return**: deltaRMS returns to baseline after walk completes

### Example Expected Output
```
TIME ELAPSED | BLOBS | PEAK BLOBS | DELTARMS | PEAK DELTARMS | SPIKE?
-----------|-------|------------|----------|----------------|-------
10s         | 1     | 1          | 0.0834   | 0.0834         | ✅
11s         | 1     | 1          | 0.0921   | 0.0921         | ✅
12s         | 1     | 1          | 0.0658   | 0.0921         | ✅
...
30s         | 0     | 1          | 0.0198   | 0.0921         | ❌

=== EVALUATION ===
✅ PASS: deltaRMS spike detected (0.0921 > 0.05)
✅ PASS: Tracked blobs observed (peak: 1)
Motion detection confirmed during walk-through
```

## Failure Mode Troubleshooting

### If deltaRMS Does NOT Spike

**Possible Causes:**
1. **Node not in passive monitor mode** — Node may be in AP mode instead of STA+monitor mode
2. **Wrong AP BSSID** — `passive_bss` NVS key doesn't match the home WiFi AP
3. **Insufficient CSI signal** — Node too far from AP or WiFi signal too weak
4. **TX_RX mode required** — Node-to-node CSI may be needed instead of node-to-AP

**Troubleshooting Steps:**

#### Step 1: Verify Passive BSS Configuration
```bash
# Check node's passive_bss NVS key (operator action)
# Node must be monitoring the correct AP BSSID

# Expected: passive_bss = home WiFi AP's MAC address
# Format: aa:bb:cc:dd:ee:ff
```

#### Step 2: Check Node Mode and Positioning
```bash
# Verify node is in monitor mode (not AP mode)
# Check node has clear line-of-sight to WiFi AP
# Ensure node is within WiFi range (RSSI > -70 dBm)
```

#### Step 3: Consider TX_RX Mode
```bash
# If node-to-AP CSI is insufficient, switch to node-to-node
# Set both nodes to TX_RX mode for bidirectional CSI
# See docs/gdop-function-signature.md for configuration
```

#### Step 4: Consult Troubleshooting Runbook
```bash
# Reference: notes/bf-3v39-troubleshooting-runbook.md
# (If runbook doesn't exist, create it with detailed steps)
```

## Test Execution Results (2026-08-29)

### Attempt Status: ❌ BLOCKED - Hardware Deployment Required

**Attempted**: Walk-through presence test execution
**Result**: Cannot proceed - no physical nodes deployed
**Reason**: Zero nodes connected = zero CSI links = zero deltaRMS data

#### System State at Test Time

**Local Mothership (http://localhost:8088)**:
```json
{
  "nodes": 0,
  "blobs": 0,
  "quality": 0,
  "version": "dev"
}
```

**Remote Mothership (https://spaxel.ardenone.com)**:
- Status: Online (14+ days uptime, v0.2.24)
- Nodes: Unknown (requires Google OAuth)
- Authentication: Google OAuth required (cannot access programmatically)

#### Test Execution Attempt

1. **Pre-test checks**:
   - ✅ Monitoring script verified: `scripts/walkthrough_monitor.sh` ready
   - ✅ Local mothership running on port 8088
   - ❌ **NODE CHECK FAILED**: 0 nodes online

2. **Cannot execute walk-through**:
   - No CSI data stream exists
   - No WiFi links for motion detection
   - No deltaRMS values to measure
   - No blob tracking possible

3. **DeltaRMS measurement**:
   - API returns: `0.0000` (no nodes = no data)
   - Expected baseline: ~0.02
   - Expected spike: > 0.05
   - **Actual: Cannot measure - no links established**

### Documented Negative Result

**Finding**: The walk-through test cannot be executed without physical nodes deployed.

**Root Cause Analysis**:
1. Spaxel nodes (ESP32-S3 hardware) are not physically deployed
2. No nodes = no CSI capture = no motion detection capability
3. This is a hard prerequisite for ALL CSI-based features:
   - Baseline capture (bf-3v39 child-2)
   - Walk-through test (bf-3v39 child-3)
   - Pattern matching (bf-3v39 child-4)

**Troubleshooting Analysis**:

#### Attempted Diagnostics
1. **Node connectivity check**:
   ```bash
   curl -s http://localhost:8088/api/status
   # Result: {"nodes": 0, "blobs": 0, "quality": 0}
   ```
   - Confirmed: 0 nodes connected to mothership
   - Expected: ≥1 node for CSI capture

2. **Remote mothership check**:
   ```bash
   curl -s https://spaxel.ardenone.com/api/status
   # Result: Redirect to Google OAuth
   ```
   - Remote mothership requires authentication
   - Cannot programmatically verify remote node status
   - Local mothership is the authoritative test environment

3. **CSI link verification**:
   - WiFi links require node-to-AP CSI or node-to-node CSI
   - With 0 nodes, there are zero links to measure
   - deltaRMS is undefined without links (returns 0.0000)

#### Why Troubleshooting Cannot Proceed Without Hardware

The troubleshooting branch in the task description says:
> "If deltaRMS does NOT spike: execute the troubleshooting branch — verify passive_bss NVS key holds the AP BSSID (operator runbook), or evaluate switching the node to TX_RX mode for node-to-node CSI"

**However**, this troubleshooting presumes nodes ARE deployed and connected. When `nodes = 0`:

- **Cannot verify `passive_bss` NVS key**: No nodes to query
- **Cannot check node positioning**: No hardware exists to position
- **Cannot evaluate TX_RX mode**: No nodes to configure
- **Cannot measure CSI signal strength**: No links to measure

**The "no deltaRMS spike" failure mode requires nodes first.** The troubleshooting applies when:
- Nodes are online AND
- deltaRMS is being captured BUT
- Spike does NOT exceed threshold > 0.05

Our current state is upstream of troubleshooting: **no hardware = no CSI pipeline = nothing to troubleshoot.**

#### Resolution Path Forward

**Immediate action required** (operator):
1. **Deploy ESP32-S3 spaxel nodes** to physical environment
2. **Provision nodes to WiFi** via dashboard "Add Node" (Chrome Web Serial API)
3. **Verify connectivity**: `curl /api/status` shows `nodes > 0`
4. **Execute baseline capture** (bf-3v39 child-2) to establish deltaRMS ~0.02
5. **Re-run walk-through test** with nodes deployed

**Once nodes are deployed**, the troubleshooting branch applies:
- If deltaRMS baseline > 0.05 → Check passive_bss NVS key
- If no spike during walk → Consider TX_RX mode for node-to-node CSI
- If signal weak → Reposition nodes within WiFi range

**Hardware deployment is the unblocking prerequisite**, not a configuration issue.

**Hardware Deployment Requirements** (from notes/bf-3v39-baseline.md):
1. Obtain ESP32-S3 spaxel node hardware
2. Provision nodes to home WiFi via dashboard "Add Node" (Chrome Web Serial)
3. Verify nodes appear in mothership `/api/status` (nodes > 0)
4. Confirm baseline capture can execute (child-2)
5. THEN execute walk-through test (this task)

### Monitoring Infrastructure Verification

**✅ Script Ready**: `walkthrough_monitor.sh` is production-ready
- Polls deltaRMS via `/api/explain/{blob_id}` every 1 second
- Captures blob count via `/api/blobs`
- Evaluates spike detection against 0.05 threshold
- Generates timestamped logs in `data/walkthrough/`

**Test when unblocked**:
```bash
./scripts/walkthrough_monitor.sh 60
# Requires: nodes > 0, operator walks through detection area
```

## Current Status (2026-08-29 09:45 UTC)

### Blockers
1. **❌ Physical Deployment Required**: No spaxel nodes are deployed or connected
2. **❌ Operator Action Required**: Physical walk-through cannot be performed by agent
3. **❌ Dependency Blocked**: Child 2 (baseline capture) also blocked on 0 nodes

### What IS Ready
- ✅ Monitoring script: `scripts/walkthrough_monitor.sh`
- ✅ Local mothership: http://localhost:8088 (running)
- ✅ Remote mothership: https://spaxel.ardenone.com (healthy)
- ✅ API endpoints documented and accessible
- ❌ **Zero nodes online = zero CSI data = no testing possible**

### Available Infrastructure
- ✅ Monitoring script ready: `scripts/walkthrough_monitor.sh`
- ✅ Local mothership running: http://localhost:8088
- ✅ Remote mothership healthy: https://spaxel.ardenone.com
- ✅ API endpoints documented and accessible
- ❌ **Zero nodes online = zero links = zero CSI data**

## Resolution Path

### Immediate Requirements
1. **Deploy physical spaxel nodes** (ESP32-S3 hardware)
2. **Provision nodes to home WiFi** via dashboard "Add Node" (Chrome Web Serial)
3. **Verify node connectivity** — mothership shows nodes online
4. **Confirm baseline capture** — child 2 completes with deltaRMS ~0.02
5. **Schedule operator walk-through** — physical presence required

### Once Nodes Are Deployed
1. Start monitoring script: `./scripts/walkthrough_monitor.sh 60`
2. Operator walks through detection area between node and WiFi AP
3. Script captures deltaRMS time series and blob evidence
4. Review results in generated log file
5. Document findings to this file
6. Post comment on bead spaxel-b075c0f3

### Acceptance Criteria (Once Unblocked)
- [ ] Physical nodes deployed and connected
- [ ] Baseline capture complete (child 2 success)
- [ ] Walk-through monitoring executed
- [ ] deltaRMS time series recorded with timestamps
- [ ] Spike > 0.05 observed during walk OR troubleshooting analysis documented
- [ ] Blob evidence captured via /api/blobs
- [ ] Results documented in notes/bf-3v39-walktest.md
- [ ] Bead comment posted with findings

## Next Steps

### Blocked On
- **Physical hardware deployment** — ESP32-S3 nodes must be provisioned and connected
- **Operator availability** — Person required to walk through detection area
- **Child 2 completion** — Baseline capture must succeed first

### When Unblocked
1. Verify child 2 (baseline capture) is complete
2. Confirm nodes are online and streaming CSI
3. Execute walk-through monitoring script during operator walk
4. Analyze deltaRMS time series for spike detection
5. Document results and close bead

## Related Documentation
- [Baseline Capture Results](notes/bf-3v39-baseline.md) — Child 2 findings
- [Monitoring Script](scripts/walkthrough_monitor.sh) — Walk-through automation
- [WiFi Configuration](docs/deployment/wifi-configuration.md) — Node provisioning
- [System Architecture](docs/plan/plan.md) — CSI pipeline and detection design

---

**Last Updated**: 2026-08-29 by agent (claude-code-glm-4.7-glm-acb:spaxel-b075c0f3)
**Status**: ❌ BLOCKED — No physical nodes deployed; operator action required
