# ESP32 Node Troubleshooting Runbook

## Problem: First ESP32 Node Not Connecting to Mothership

**Status:** Critical - No nodes currently connected to https://spaxel.ardenone.com

## Pre-flight Checks

### 1. Verify Physical Hardware
- [ ] ESP32 has power (LED indicators, serial console output)
- [ ] USB cable connected and recognized by host system
- [ ] Device boots successfully (watch serial console)

### 2. Check Network Infrastructure
- [ ] Home WiFi AP is broadcasting
- [ ] Other devices can connect to the AP
- [ ] AP BSSID is stable (not changing)

## Step 1: Connect Serial Console

```bash
# Find the serial port
ls /dev/ttyUSB* /dev/ttyACM*

# Connect to serial console (115200 baud, 8N1)
screen /dev/ttyUSB0 115200
# OR
minicom -D /dev/ttyUSB0 -b 115200
```

**What to look for:**
- Boot messages showing firmware version
- WiFi connection attempts
- Mothership connection attempts
- Error messages or crash indicators

## Step 2: Verify NVS Configuration

The ESP32 must have the correct home AP BSSID in the `spaxel` NVS namespace,
key `passive_bss`. It is a 6-byte blob, not a string. The similarly named
`passive_bssid` is the JSON field used in a runtime `role` message; it is not
the persisted NVS key.

### Check Current Configuration

1. Connect the node's USB-Serial/JTAG console at 115200 baud and capture the
   boot log.
2. If a raw flash dump is needed, read the NVS partition using the actual board
   port and the offsets in `firmware/partitions.csv`:

   ```bash
   esptool.py --port /dev/ttyACM0 read_flash 0x9000 0x6000 nvs.bin
   ```

3. Decode `nvs.bin` with an ESP-IDF version-matched NVS inspection tool. In
   namespace `spaxel`, expect `passive_bss` to contain six bytes formatted as
   the home AP BSSID, for example `AA:BB:CC:DD:EE:FF`.

The shipped firmware does not provide a supported `nvs_dump` or `nvs_set`
serial shell command. Do not substitute the old `passive_bssid` spelling when
inspecting the NVS key.

### Correct a Missing or Incorrect BSSID

For immediate runtime recovery, use the authenticated dashboard role control
with the JSON field `passive_bssid`:

```json
{"role":"passive","passive_bssid":"AA:BB:CC:DD:EE:FF"}
```

Then reboot only after confirming the serial log shows the expected passive
BSSID and CSI activity. The current firmware's provisioning writer does not
write `passive_bss`, and the runtime role handler updates the in-memory BSSID;
therefore verify persistence after a reboot and treat a missing key as a
firmware/provisioning follow-up rather than assuming the runtime command was
durable.

If the BSSID is wrong, obtain the value from the router's 2.4 GHz radio (not a
client MAC), apply the runtime role update, and repeat the persistence check.
Avoid erasing NVS unless the operator has a replacement provisioning payload:
that partition also contains WiFi credentials, node identity, token, and
mothership settings.

## Step 3: Verify WiFi Connection

Watch the serial console for WiFi connection messages:

**Expected:**
```
I (1234) wifi:Connecting to AA:BB:CC:DD:EE:FF
I (2345) wifi:Connected
I (2346) wifi:Got IP: 192.168.1.100
```

**If not connecting:**
- Verify BSSID matches exactly (case-sensitive)
- Check AP is on 2.4GHz (ESP32 doesn't support 5GHz)
- Verify AP security settings (WPA2-PSK recommended)
- Check for MAC filtering on AP (add ESP32 MAC address)

## Step 4: Verify Mothership Connection

Once WiFi is connected, watch for mothership connection:

**Expected:**
```
I (3456) mothership:Connecting to spaxel.ardenone.com
I (4567) mothership:Connected
I (4568) mothership:Node registered: AA:BB:CC:DD:EE:FF
I (5678) csi:Streaming CSI frames
```

**Common Errors:**

### "DNS lookup failed"
- Check internet connectivity
- Verify DNS settings on AP
- Try using IP address directly

### "Connection refused"
- Verify mothership is reachable (ping spaxel.ardenone.com)
- Check firewall rules
- Verify TLS/certificate is valid

### "Authentication failed"
- Verify node token derivation is correct
- Check install secret matches mothership database

## Step 5: Check Mothership Logs

On the mothership server, check for connection attempts:

```bash
# Check application logs
docker logs spaxel-mothership --tail 100

# Or if running natively
journalctl -u spaxel-mothership -n 100

# Look for:
# - Node registration events
# - WebSocket connection attempts
# - CSI frame reception
# - Authentication failures
```

**What to look for:**
```
# Successful connection:
[INFO] Node AA:BB:CC:DD:EE:FF connected from 192.168.1.100
[INFO] WebSocket established for node AA:BB:CC:DD:EE:FF
[INFO] Receiving CSI frames from AA:BB:CC:DD:EE:FF

# Connection failures:
[ERROR] WebSocket handshake failed for AA:BB:CC:DD:EE:FF
[ERROR] Invalid token from node AA:BB:CC:DD:EE:FF
[WARN] Node AA:BB:CC:DD:EE:FF connection timeout
```

## Step 6: Verify CSI Streaming

Once connected, verify CSI frames are arriving:

### Via Mothership API (requires OAuth)
```bash
# After OAuth login
curl -s https://spaxel.ardenone.com/api/nodes | jq '.nodes[0]'

# Look for:
# - "status": "CONNECTED"
# - "last_csi_timestamp": <recent ISO8601 timestamp>
# - "csi_frame_rate": <frames per second>
```

### Via Serial Console
```
# Should see periodic messages like:
I (6789) csi:Sent CSI frame, 128 subcarriers
I (6890) csi:Sent CSI frame, 124 subcarriers
I (6991) csi:Sent CSI frame, 132 subcarriers
```

### Expected CSI Frame Rate
- **Target:** ~10-20 frames per second per node
- **Minimum:** ~5 fps for viable presence detection
- **Below 5 fps:** Unreliable detection

## Common Issues and Solutions

### Issue: ESP32 Boots But No WiFi

**Symptoms:** Device boots, serial console shows code running, but no WiFi connection

**Possible Causes:**
1. Wrong BSSID in NVS
2. AP not reachable (distance, interference)
3. Wrong WiFi password
4. AP on 5GHz band

**Solutions:**
1. Verify BSSID with `nvs_list`
2. Move ESP32 closer to AP
3. Check WiFi credentials
4. Force AP to 2.4GHz

### Issue: WiFi Connects But No Mothership

**Symptoms:** WiFi successful, but mothership connection fails

**Possible Causes:**
1. Mothership DNS unreachable
2. TLS certificate validation failure
3. Network firewall blocking WebSocket
4. Wrong mothership URL

**Solutions:**
1. Ping spaxel.ardenone.com from another device
2. Check system time (TLS requires correct time)
3. Disable firewall temporarily to test
4. Verify URL configuration in firmware

### Issue: Node Registers But No CSI

**Symptoms:** Node shows as connected, but CSI frames not arriving

**Possible Causes:**
1. CSI capture not enabled in firmware
2. ESP32 CSI hardware not supported
3. Insufficient RAM/cpu for CSI processing
4. Network bandwidth insufficient

**Solutions:**
1. Verify CSI compilation flags in firmware
2. Check ESP32 model supports CSI (ESP32-WROOM-32 does)
3. Reduce CSI frame rate
4. Check network for packet loss

### Issue: Intermittent Disconnections

**Symptoms:** Node connects, then disconnects repeatedly

**Possible Causes:**
1. Weak WiFi signal
2. AP power saving mode
3. ESP32 power supply insufficient
4. Firmware watchdog timeout

**Solutions:**
1. Improve WiFi signal or move closer
2. Disable AP power saving
3. Use powered USB hub or adequate supply
4. Check firmware for blocking operations

## Verification Checklist

Once the node is connected, verify the full pipeline:

- [ ] Node appears in `/healthz` with `nodes_online > 0`
- [ ] Node status in `/api/nodes` shows "CONNECTED"
- [ ] `last_csi_timestamp` is recent (within last 10 seconds)
- [ ] `csi_frame_rate` is > 5 fps
- [ ] Presence detection triggers on movement
- [ ] Mothership logs show consistent CSI reception

## Emergency Recovery

If all else fails, perform a factory reset:

```bash
# Erase NVS partition
esptool.py --port /dev/ttyUSB0 erase_region 0x9000 0x6000

# Reflash firmware
make flash

# Re-provision via serial or web
```

## Next Steps After Success

Once the first node is connected and streaming:

1. **Validate CSI Quality**: Check frame rate and subcarrier count
2. **Test Presence Detection**: Verify motion detection works
3. **Deploy Additional Nodes**: Repeat process for remaining ESP32s
4. **Monitor Stability**: Watch for disconnections over 24-48 hours

## Contact Information

If issues persist after this runbook:

1. Check mothership source code: `/home/coding/spaxel/mothership/`
2. Review firmware source: `/home/coding/spaxel/firmware/`
3. Check parent bead documentation for deployment specifics

---

**Last Updated:** 2026-08-26 UTC
**Bead Reference:** bf-3v39, spaxel-9c1a4858
