# bf-54cx2: CSI Rate Reporting Fix

## Issue
Node reported configured CSI rate (20 Hz) instead of measured rate. A node emitting zero CSI frames still claimed `csi_rate_hz=20` in health messages, making dead sensing paths appear healthy.

## Root Cause
`websocket.c` health message reported `g_state.packet_rate` (the configured target from NVS) instead of actual measured CSI frame rate.

## Solution Implemented
The fix was already implemented in the codebase:

1. **csi.c** (lines 313-338): Added `csi_measured_rate_hz()` function that calculates the actual measured rate from `s_stats.frames_sent` delta over time intervals.

2. **csi.h** (lines 59-67): Declared the function with documentation explaining the reasoning (referencing bf-54cx2).

3. **websocket.c** (lines 498-502):
   - Changed `csi_rate_hz` to report `csi_measured_rate_hz()` (the measured rate)
   - Added new field `csi_rate_configured_hz` to report `g_state.packet_rate` (the configured target)

4. **websocket.c** (lines 104, 714): OTA validation now uses `csi_measured_rate_hz() > 0` to confirm CSI is actually flowing.

## Implementation Details
The `csi_measured_rate_hz()` function:
- Tracks `s_stats.frames_sent` (incremented only on successful `websocket_send_csi` in csi.c:180)
- Uses static state variables to compute delta between calls
- Returns rate in Hz calculated from `(delta_frames * 1000000) / delta_time_us`
- Returns 0 on first call (establishes baseline)

## Verification
The fix ensures:
- A node with zero CSI emissions reports `csi_rate_hz=0`
- Operators can trust `csi_rate_hz` as the actual sensing health metric
- The configured target is still visible in `csi_rate_configured_hz` for debugging

## Status
Already implemented in codebase. This note documents the completed work.
