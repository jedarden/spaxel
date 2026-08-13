# OTA WiFi Reconnection Race Condition - Quick Test Guide

**Quick reference for testing the OTA vs WiFi reconnection race condition fix.**

## One-Minute Test

**Trigger the race:**
1. Start OTA update on any node
2. Immediately power-cycle the WiFi AP (or move node out of range)
3. Watch serial output for `[RESTART-SAFE-GUARD]` messages
4. Verify clean reboot and reconnection

**Expected result:**
```
[RESTART-SAFE-GUARD] Skipping WiFi connection - restart flag is set
[RESTART-SAFE-GUARD] This is a guard-triggered skip, NOT an error
[OTA] Calling esp_restart() NOW
```

## Pass/Fail (30 seconds)

**PASS ✅** (if you see):
- `[RESTART-SAFE-GUARD]` messages
- Clean reboot (no abort)
- Node reconnects with new firmware

**FAIL ❌** (if you see):
- `ESP_ERROR_CHECK` abort
- Guru Meditation Error
- No guard messages
- Node doesn't reconnect

## What This Tests

**The Bug:** OTA completing during WiFi reconnect caused ESP_ERROR_CHECK aborts  
**The Fix:** Restart-safe guard skips WiFi ops when `restarting=true`  
**The Race:** 5000ms window when OTA can finish during reconnect delay

## Full Documentation

See: `docs/research/ota-wifi-reconnection-race-condition-testing.md`

## Automated Tests

Run all tests:
```bash
cd /home/coding/spaxel/firmware/test
make test
```

All 26 tests should pass ✅
