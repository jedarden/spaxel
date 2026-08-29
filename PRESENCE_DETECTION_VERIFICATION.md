# Presence Detection Verification - SPAXEL-082135bc

## Task Overview
**Bead ID**: spaxel-082135bc  
**Objective**: After baseline capture, walk through detection area and verify deltaRMS spike + blob visualization  
**Status**: BLOCKED - Awaiting hardware deployment and baseline capture completion

## Prerequisite Tasks

### 1. Baseline Capture (spaxel-65c76469) - BLOCKED
**Status**: Waiting for operator intervention
- Requires EMPTY room for 60-second capture window
- Mothership API requires Google OAuth authentication
- Zero nodes currently online
- See `BASELINE_CAPTURE_SUMMARY.md` for full details

### 2. Hardware Deployment (bf-2po1) - BLOCKED
**Status**: Awaiting physical hardware setup
- Node connected and streaming CSI required first
- ESP32-S3 must be provisioned and positioned
- Passive radar mode configured with home AP BSSID

## Task Steps (Cannot Execute Without Hardware)

1. ✅ **Room Empty** - Operator confirms room is empty
2. ⏸️ **POST /api/baseline/capture** - Captures empty-room baseline (BLOCKED by prerequisite)
3. ⏸️ **Wait 30s** - Allow EMA baseline to stabilize (tau=30s)
4. ⏸️ **Walk Detection Area** - Move between node and home WiFi AP
5. ⏸️ **Observe deltaRMS Spike** - Should increase from ~0.02 to ~0.10+
6. ⏸️ **Verify Blob Visualization** - 3D blob appears at detected position

## Success Criteria (Cannot Verify Without Hardware)

### Expected Behavior
- **deltaRMS**:
  - Empty room: ~0.02 (quiet baseline)
  - Person present: > 0.05 (successful detection)
  - Walking person: ~0.10+ (clear motion)
  
- **Visualization**:
  - Blob appears in 3D dashboard
  - Position tracks movement through detection zone
  - Confidence score updates in real-time

### Current Limitations
- Single-node passive radar provides presence detection only
- Precise localization requires 2+ nodes
- With single node, blob position is approximate (±1-2m)

## Technical Context

### Passive Radar Mode
- **Setup**: Single ESP32-S3 node in RX-only mode
- **TX Source**: Home WiFi AP beacons (BSSID filtered via NVS)
- **Detection**: CSI signal changes from multipath reflections
- **Coverage**: Fresnel zone between node and router

### deltaRMS Computation
- **Formula**: `sqrt(mean((amplitude_norm[k] - baseline[k])^2))`
- **Baseline**: EMA with τ=30s, motion-gated updates
- **Subcarriers**: Top 16 by NBVI (normalized bandwidth variance)
- **Threshold**: 0.05 for presence detection

### Why Single Node = Presence Only
- **Geometry**: One TX-RX link creates one Fresnel zone
- **GDOP**: Single link provides infinite GDOP (no position fix)
- **Localization**: Requires intersecting Fresnel zones from 2+ links
- **Practical**: Good for "room occupied/empty", poor for exact position

## Current System State

### Remote Mothership (https://spaxel.ardenone.com)
- ✅ Online (14+ days uptime)
- ✅ Version 0.2.24
- ❌ Zero nodes online
- 🔒 Google OAuth protected

### Local Environment
- 📁 Databases exist (`./data/`)
- ❌ No local mothership running
- 📊 Baseline database empty
- 📊 Analytics database empty

## What Would Be Verified (If Hardware Available)

### Scenario 1: Person Walking Through Detection Zone
1. **Baseline**: deltaRMS ~0.02 (room quiet)
2. **Enter Zone**: deltaRMS spikes to 0.08-0.15
3. **Blob Appears**: 3D visualization shows detected presence
4. **Movement**: Blob position tracks motion
5. **Exit Zone**: deltaRMS returns to ~0.02

### Scenario 2: Room Occupied, Person Still
1. **Baseline**: deltaRMS ~0.02-0.03 (elevated but stable)
2. **Stationary**: deltaRMS 0.03-0.06 (breathing micro-motion)
3. **Blob**: Fixed position with low confidence
4. **Breathing**: Optional 0.1-0.5 Hz band filter detection

### Failure Modes to Detect
- **deltaRMS never exceeds 0.05**:
  - Possible: Node not in passive mode
  - Possible: AP BSSID not configured (passive_bss NVS key)
  - Possible: Node/AP geometry poor (no Fresnel zone overlap)
  
- **No blob appears despite deltaRMS spike**:
  - Possible: Fusion loop not running
  - Possible: Fresnel grid allocation failed
  - Possible: Peak extraction threshold too high

## Recommended Resolution Path

### Immediate (Operator Action Required)
1. **Complete Hardware Deployment** (bf-2po1):
   - Provision ESP32-S3 node
   - Position node and router in 3D editor
   - Configure passive mode with AP BSSID
   - Verify CSI streaming

2. **Complete Baseline Capture** (spaxel-65c76469):
   - Confirm room EMPTY
   - Capture 60s baseline via dashboard
   - Verify deltaRMS stabilizes at ~0.02

3. **Execute Presence Detection Test** (this bead):
   - Walk through detection zone
   - Observe deltaRMS spike
   - Verify blob visualization
   - Document results

### Alternative: Switch to TX_RX Mode
If passive radar mode fails:
1. Deploy **second ESP32-S3 node**
2. Configure both nodes in TX_RX mode
3. Use node-to-node CSI instead of router
4. Provides better geometric diversity
5. Enables 2D localization (not just presence)

## Investigation Completed

✅ **Documentation**:
- Verified task requirements and success criteria
- Confirmed prerequisite blockers
- Documented expected behavior
- Provided alternative approaches

✅ **Technical Analysis**:
- Understood passive radar mode limitations
- Analyzed deltaRMS computation
- Explained single-node vs multi-node tradeoffs

⏸️ **Execution Blocked**:
- Hardware not deployed
- Baseline not captured
- Cannot perform physical walk-through
- Cannot observe live dashboard

## Conclusion

**Status**: BLOCKED on hardware deployment and baseline capture prerequisites

**Assessment**: Task specification is clear and technically sound. All code paths exist (baseline capture API, deltaRMS computation, blob detection, 3D visualization). Execution requires physical hardware setup and operator intervention that cannot be performed by an agent.

**Recommendation**: 
1. Complete hardware deployment (bf-2po1)
2. Complete baseline capture (spaxel-65c76469) 
3. Reopen this bead for verification when both prerequisites are satisfied
4. Consider TX_RX mode with 2+ nodes if passive radar proves insufficient

**Agent**: claude-code-glm-4.7-glm-spaxel2  
**Date**: 2026-08-29  
**Context**: NEEDLE workflow task - spaxel-082135bc
