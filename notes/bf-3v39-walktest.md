# Walk-Through Presence Test Procedure (bf-3v39-child-3)

**Bead ID:** spaxel-b075c0f3
**Status:** OPERATOR REQUIRED - Physical walk-through test
**Created:** 2026-08-29
**Split Parent:** bf-3v39 (baseline capture blocker)

## Overview

This test validates that the Spaxel motion detection system detects a person walking through the detection area between a node and the home WiFi AP. It measures deltaRMS (signal variation) spikes during the walk and tracks blob formation.

## Test Requirements

### Physical Setup
- **Operator Required:** A human must physically walk through the detection area
- **Detection Area:** Space between the Spaxel node and the home WiFi AP
- **Duration:** 60 seconds minimum (configurable via script parameter)
- **Mothership:** Must be running on port 8088 (default)

### Expected Results
- **Baseline deltaRMS:** ~0.02 (no motion)
- **Spike deltaRMS:** > 0.05 during walk (expected ~0.10+)
- **Blob Formation:** At least 1 tracked blob via `/api/blobs`
- **Timing:** Spike timestamps should correlate with walk timing

## Prerequisites

### 1. Verify Mothership Running
```bash
curl -s http://localhost:8088/healthz | jq .
```
Expected response: `{"status":"ok"}`

### 2. Verify Node Online
```bash
curl -s http://localhost:8088/api/nodes | jq '.[] | select(.went_offline_at=="0001-01-01T00:00:00Z" or .went_offline_at==null)'
```
Should return at least one online node.

### 3. Check WiFi Configuration
Ensure the node has the correct WiFi credentials and can communicate with the AP:
```bash
curl -s http://localhost:8088/api/settings/network | jq .
```

## Test Execution

### Step 1: Start Monitoring
Run the walk-through monitoring script:
```bash
cd /home/coding/spaxel
./scripts/walkthrough_monitor.sh 60
```

The script will:
- Poll `/api/explain/{blobID}` every second for deltaRMS values
- Poll `/api/blobs` for tracked blob count
- Record timestamps for all samples
- Detect spikes exceeding 0.05 threshold
- Generate log file at `data/walkthrough/walkthrough_<timestamp>.log`

### Step 2: Perform Walk
1. Press ENTER when prompted to start monitoring
2. Begin walking through the detection area
3. Walk continuously for the full monitoring period (60s default)
4. Vary your position within the detection area
5. Move at normal walking pace (not too fast, not too slow)

### Step 3: Review Results
The script outputs:
- Real-time console output with timestamps
- Detailed log file with all samples
- Peak deltaRMS and blob count
- Pass/fail evaluation

Example output:
```
TIME ELAPSED | BLOBS | PEAK BLOBS | DELTARMS | PEAK DELTARMS | SPIKE?
-----------|-------|------------|----------|----------------|-------
1s         | 0     | 0          | 0.0000   | 0.0000         | ❌
2s         | 0     | 0          | 0.0000   | 0.0000         | ❌
3s         | 1     | 1          | 0.0234   | 0.0234         | ❌
4s         | 1     | 1          | 0.0876   | 0.0876         | ✅
...
```

## Troubleshooting

### No deltaRMS Spike Detected

If the peak deltaRMS remains below 0.05 threshold:

#### 1. Verify passive_bss NVS Key
The node must have the correct AP BSSID configured:
```bash
# Check current WiFi settings
curl -s http://localhost:8088/api/settings/network | jq '.ssid, .bssid'
```

Compare the BSSID with your actual AP:
```bash
# Get AP BSSID
nmcli -t -f active,bssid dev wifi | grep yes
```

If they don't match, update the NVS key on the node per the operator runbook.

#### 2. Check Node Positioning
- Verify node is positioned correctly relative to AP
- Ensure direct line of sight during walk
- Check for interference sources (metal objects, other APs)

#### 3. Evaluate TX_RX Mode
If passive RX mode fails, consider switching to TX_RX mode for node-to-node CSI:
- This requires two nodes configured as transmitter/receiver pair
- Setup instructions in parent bead description

#### 4. Review System Health
```bash
# Check system status
curl -s http://localhost:8088/api/status | jq .

# Check for CSI frames being received
curl -s http://localhost:8088/api/nodes | jq '.[].csi_frame_rate'
```

### No Blobs Tracked

If `/api/blobs` returns empty array:

#### 1. Verify Fusion Engine Running
Check fusion logs for errors or crashes.

#### 2. Check Node Positions
```bash
curl -s http://localhost:8088/api/nodes | jq '.[] | {mac, name, x, y, z}'
```
Nodes should have non-origin positions.

#### 3. Review Link Quality
```bash
# Use explainability API to check link states
curl -s http://localhost:8088/api/explain/1 | jq '.contributing_links[] | {link_id, delta_rms, contributing}'
```

### API Errors

If APIs return errors or timeouts:

#### 1. Restart Mothership
```bash
# Stop existing instance
pkill mothership

# Restart with debug logging
SPAXEL_LOG_LEVEL=debug SPAXEL_BIND_ADDR=127.0.0.1:8088 mothership
```

#### 2. Check Port Availability
```bash
lsof -i :8088
```

## Acceptance Criteria

### ✅ PASS Conditions
- deltaRMS spike > 0.05 during walk (peak > baseline)
- At least 1 tracked blob observed via `/api/blobs`
- Timestamps correlate walk timing with spike detection
- Log file contains complete time series

### ❌ FAIL Conditions (with Documentation)
- No deltaRMS spike detected (peak <= 0.05)
- No blobs tracked during walk
- API errors preventing monitoring

### Documented Negative Result
If test fails, document in bead notes:
1. What was observed (peak deltaRMS, blob count, any anomalies)
2. Troubleshooting steps performed
3. Root cause analysis (if determined)
4. Recommended next actions

## Related Documentation

- **Parent Bead:** bf-3v39 (baseline capture blocker)
- **Baseline Status:** notes/bf-3v39-baseline.md
- **Troubleshooting Runbook:** notes/bf-3v39-troubleshooting-runbook.md
- **API Reference:** See API_IMPLEMENTATION_STATUS.md

## Next Steps (Child 4)

After successful walk-through test:
1. Review `/api/blobs` data captured during walk
2. Analyze blob formation patterns and timing
3. Document blob evidence for child 4 analysis
4. Correlate blob timing with deltaRMS spikes

## System Status (2026-08-29 09:55 UTC)

**Infrastructure Status: READY**
- ✅ Mothership running on port 8088 (PID 845527)
- ✅ Health endpoint responding: `{"status":"ok","db":"ok"}`
- ✅ All APIs operational (`/api/nodes`, `/api/blobs`, `/api/explain`)
- ✅ Monitoring script executable: `scripts/walkthrough_monitor.sh`
- ✅ Data directory prepared: `data/walkthrough/`

**Current Blocker: NO OPERATOR**
- AI agent cannot perform physical walk-through
- Requires human operator to walk through detection area
- This is why parent bf-3v39 failed 5x previously

**Pre-Walk Checklist for Operator:**
1. Verify nodes are online: `curl -s http://localhost:8088/api/nodes`
2. Run monitor: `./scripts/walkthrough_monitor.sh 60`
3. Walk through detection area continuously for 60s
4. Review log: `data/walkthrough/walkthrough_<timestamp>.log`
5. Verify deltaRMS spike >0.05 during walk

**Expected Results:**
- Baseline deltaRMS: ~0.02 (no motion)
- Spike deltaRMS: >0.05 during walk (expected ~0.10+)
- At least 1 tracked blob via `/api/blobs`
- Timestamp correlation between walk and spike

**Troubleshooting (if no spike):**
1. Verify `passive_bss` NVS key holds correct AP BSSID
2. Check node positioning relative to WiFi AP
3. Evaluate switching to TX_RX mode for node-to-node CSI
4. Review system health and CSI frame rates

## Output Files

Results are saved to:
```
data/walkthrough/walkthrough_<YYYYMMDD_HHMMSS>.log
```

The log contains:
- Timestamp for each sample
- Blob count at each interval
- Peak deltaRMS from contributing links
- Spike detection indicators
- Final evaluation and recommendations

## Agent Actions Taken

1. ✅ Started mothership on port 8088 (corrected from default 8080)
2. ✅ Verified all APIs responding correctly
3. ✅ Confirmed monitoring infrastructure operational
4. ✅ Prepared data directory for results
5. ✅ Documented system status and operator requirements

**BLOCKER:** Cannot proceed without physical operator for walk-through test.
