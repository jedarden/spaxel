# Ambient Traffic Measurement — ADR-003 Validation

**Purpose:** Measure whether beacon-only ambient traffic from a quiet WiFi AP produces enough CSI frames to clear motion detection thresholds (deltaRMS, NBVI).

**Context:** This is part of ADR-003 / bead `bf-4j97b`. Ambient CSI rate is bounded by how much the sensed AP actually transmits. A quiet AP emits beacons at roughly 10 Hz. The rate controller targets 2 Hz idle / 50 Hz active. Whether beacon-only ambient traffic produces enough frames to clear the motion-detection thresholds is **unverified** and must be measured before ambient sensing is called done.

If beacon-only traffic is insufficient, a deliberate traffic source on the sensed link becomes a **deployment REQUIREMENT**, not an optimisation.

## Measurement Protocol

### 1. Frame Rate Measurement API

Two new endpoints expose per-link CSI frame statistics:

**GET /api/framestats/all**
- Returns frame statistics for all active links
- Response:
  ```json
  {
    "AA:BB:CC:DD:EE:FF:11:22:33:44": {
      "frame_rate": 10.5,
      "frame_count": 623,
      "last_frame": "2026-08-20T21:05:32Z"
    },
    ...
  }
  ```

**GET /api/framestats/link/{linkID}**
- Returns statistics for a single link
- `linkID` format: `"AA:BB:CC:DD:EE:FF:11:22:33:44"` (canonical TX:RX form)

### 2. Measurement Scenarios

#### Scenario A: Idle AP (Beacon-Only Traffic)
- **Setup:** AP with no associated clients, only beacon frames
- **Expected beacon rate:** ~10 Hz (typical beacon interval: 100 ms)
- **Measurement procedure:**
  1. Deploy 1+ ESP32 nodes in passive mode (no dedicated TX)
  2. Ensure no other traffic on the network (no phones, laptops, IoT devices)
  3. Wait 60 seconds for frame rate statistics to stabilize
  4. Call `GET /api/framestats/all`
  5. Record `frame_rate` for each link
  6. Check whether `motionDetected` is ever true via motion state API

**Expected outcome:** If beacon-only traffic is sufficient, frame_rate ≈ 10 Hz and motion detection should work with reduced sensitivity.

#### Scenario B: AP Under Light Traffic
- **Setup:** Normal home network with 1-2 connected devices (phones in standby, occasional background sync)
- **Expected traffic pattern:** Beacons (10 Hz) + occasional data frames (variable rate)
- **Measurement procedure:**
  1. Same passive node setup
  2. Generate light traffic: device checks email, syncs photos, sends periodic keep-alive
  3. Wait 60 seconds for statistics
  4. Record frame rates and motion detection state

**Expected outcome:** Frame rate should be higher than beacon-only (15-30 Hz range), motion detection more reliable.

### 3. Detection Thresholds

From `mothership/internal/signal/features.go`:

- **deltaRMS threshold:** `DefaultDeltaRMSThreshold = 0.02`
  - Motion detected when: `smooth_deltaRMS > 0.02`
  - Typical values: ~0.02 empty room, ~0.10 walking

- **NBVI (subcarrier selection):**
  - Requires `NBVIMinSamples = 50` samples before selection activates
  - At 10 Hz beacon rate: 5 seconds to reach minimum samples
  - At 2 Hz idle rate: 25 seconds to reach minimum samples

## Critical Question

**Does 10 Hz beacon-only traffic produce enough CSI frames to:**

1. **Clear the NBVI minimum sample threshold?** (50 samples = 5 seconds at 10 Hz)
2. **Generate meaningful deltaRMS values?** (needs baseline + ongoing samples)
3. **Trigger motion detection?** (smooth_deltaRMS > 0.02)

## Success Criteria

Beacon-only ambient traffic is **sufficient** if:
- Frame rate stabilizes at ≥10 Hz
- NBVI reaches minimum sample threshold within 10 seconds
- Motion detection triggers reliably when a person walks through the Fresnel zone

Beacon-only traffic is **insufficient** if:
- Frame rate < 2 Hz consistently
- NBVI never reaches minimum sample threshold
- Motion detection fails to trigger despite visible movement

**If insufficient, the product requirements change:** A deliberate traffic source becomes a deployment requirement for single-node setups, not an optimisation. This affects the "just plug in one node" value proposition.

## Implementation Status

- ✅ FrameTracker implemented (`mothership/internal/ingestion/frametracker.go`)
- ✅ Tracking integrated into CSI ingestion path
- ✅ API endpoints registered (`/api/framestats/*`)
- ⏳ **NOT YET TESTED ON HARDWARE** — requires:
  1. ESP32-S3 node in passive mode
  2. AP with measurable traffic characteristics
  3. Mothership deployment with these changes
  4. Controlled walk-through / idle period

## Next Steps

1. Deploy this code to a test environment
2. Measure Scenario A (idle AP) — document actual frame rates
3. Measure Scenario B (light traffic) — document improvement
4. Update ADR-003 findings in `docs/plan/plan.md`
5. If insufficient, update deployment documentation to state traffic source requirement

---

**Bead:** spaxel-76601bae  
**Status:** Implementation complete, hardware validation pending
