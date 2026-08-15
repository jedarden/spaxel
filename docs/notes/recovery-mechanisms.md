# ESP32 Recovery Mechanisms

An ESP32-S3 can end up in several "bricked" states, each with a different recovery path. The goal is to ensure no device is permanently unrecoverable without soldering or specialist tools.

---

## Failure Modes and Recovery Matrix

| Failure Mode | Symptoms | Recovery Method | Physical Access Needed? |
|---|---|---|---|
| Bad firmware (boot loop) | Resets every few seconds | Web Serial full erase + reflash | USB cable |
| Wrong WiFi credentials | Can't reach mothership; captive portal appears after 3 failures | Connect to `spaxel-XXXX` AP, reconfigure | WiFi |
| Corrupted OTA partition | Stays on old firmware / won't update | Rollback automatic; manual reflash if both partitions corrupt | USB cable |
| Corrupted NVS | Factory-resets to provisioning mode | Captive portal restores config | WiFi |
| Bootloader corrupt | No serial response at all | GPIO0 hold + full chip erase | USB cable + BOOT button |
| Fully unresponsive | No serial, no WiFi AP | Physical reset (RST button or power cycle) first, then try above | Physical button |

---

## Layer 1: Automatic Recovery (No User Action)

### WiFi Reconnect Loop

In `main.c`, the WiFi event handler automatically reconnects on disconnect:
```c
if (base == WIFI_EVENT && id == WIFI_EVENT_STA_DISCONNECTED) {
    esp_wifi_connect();  // immediate retry
}
```

After 10 consecutive failed connection attempts (not currently implemented — worth adding), the node should fall back to provisioning mode rather than retrying indefinitely.

**Recommended enhancement** — add failure counter to NVS:
```c
static int s_connect_failures = 0;

void wifi_event_handler(...) {
    if (id == WIFI_EVENT_STA_DISCONNECTED) {
        s_connect_failures++;
        if (s_connect_failures >= 10) {
            // Reset credentials and go to provisioning
            nvs_erase_key(nvs, NVS_KEY_WIFI_SSID);
            esp_restart();
        }
        esp_wifi_connect();
    }
    if (id == IP_EVENT_STA_GOT_IP) {
        s_connect_failures = 0;
    }
}
```

### OTA Rollback

ESP-IDF's `CONFIG_BOOTLOADER_APP_ROLLBACK_ENABLE=y` (set in `sdkconfig.defaults`) provides automatic rollback:

1. After flashing new firmware to `ota_1`, bootloader marks it as `ESP_OTA_IMG_PENDING_VERIFY`
2. New firmware boots; must call `esp_ota_mark_app_valid_cancel_rollback()` within a watchdog timeout
3. If it doesn't (crash, hang, or WDT timeout), bootloader rolls back to previous partition on next boot
4. Call the validation after WiFi connects and mothership is reachable:

```c
void on_mothership_reachable(void) {
    esp_ota_mark_app_valid_cancel_rollback();
    ESP_LOGI(TAG, "OTA validated — rollback cancelled");
}
```

This means a bad firmware update can never permanently brick a node — worst case is one failed boot followed by automatic rollback.

---

## Layer 2: Network Recovery (WiFi, No USB)

### Captive Portal Fallback

Trigger conditions for entering provisioning mode:
- No credentials in NVS (first boot / after NVS erase)
- WiFi connection failure count exceeds threshold
- Provisioning button held on boot (optional hardware feature)

When active, the node broadcasts SSID `spaxel-AABBCC` (last 3 bytes of MAC) and serves the config form at `http://192.168.4.1/`. Any device connected to the AP can reconfigure the node without USB access.

### Remote Config via MQTT

If the node is reachable on the network (wrong mothership IP, not wrong WiFi), the mothership can push a new config via MQTT:

```
Topic: spaxel/devices/{mac}/config
Payload: {"mothership": "192.168.1.20", "node_name": "kitchen-ne"}
```

**Firmware update** — add MQTT config handler to `main.c`:
```c
void mqtt_event_handler(...) {
    if (event->event_id == MQTT_EVENT_DATA) {
        if (strstr(event->topic, "/config")) {
            cJSON *json = cJSON_Parse(event->data);
            // Update NVS keys from JSON fields
            // Restart to apply
        }
    }
}
```

---

## Layer 3: Web Serial Recovery (USB Required)

### Standard Recovery (Bad Firmware)

The `recovery.html` page in the installer walks the user through:

1. Hold **BOOT** button on ESP32-S3
2. While holding BOOT, press and release **RESET**
3. Release BOOT — device enters download mode (ROM bootloader, no user code runs)
4. Click "Erase & Reflash" → Web Serial connects → `esptool-js` runs full chip erase + flash

The recovery manifest (`manifest-recovery.json`) must set `erase_before_install: true`:
```json
{
  "name": "Spaxel Node (Recovery)",
  "version": "latest",
  "new_install_erase_before_install": true,
  "builds": [
    {
      "chipFamily": "ESP32-S3",
      "parts": [
        {"path": "/firmware/bootloader.bin",        "offset": 0},
        {"path": "/firmware/partition-table.bin",   "offset": 32768},
        {"path": "/firmware/ota_data_initial.bin",  "offset": 57344},
        {"path": "/firmware/firmware.bin",           "offset": 65536}
      ]
    }
  ]
}
```

### Bootloader Recovery (Worst Case)

If even the ROM bootloader is unreachable (extremely rare — requires physical damage or severe flash corruption):

1. GPIO0 must be held LOW during power-on to force ROM bootloader
2. On most ESP32-S3 dev boards this is the BOOT button
3. If BOOT button is inaccessible, short GPIO0 to GND with a wire
4. `esptool.py --chip esp32s3 --port /dev/ttyUSB0 erase_flash` from the recovery page's advanced mode (or user's own terminal)
5. Full reflash follows

The recovery page should expose an **Advanced Mode** toggle that reveals the manual esptool.py commands for users comfortable with a terminal.

---

## Layer 3.5: USB-Serial/JTAG Wedge Recovery (Critical)

### The Wedge Condition (bf-4z6wh, bf-26pa)

**Symptoms:**
- USB device remains enumerated (`303a:1001`, `/dev/ttyACM0` present)
- No serial output, no WiFi association, no application runs
- **Every** esptool invocation fails with `Write timeout`
- DTR/RTS toggle via `stty hupcl` does **not** recover it
- Only a physical USB replug restores normal operation

**Triggers:**
- `esptool read-flash` operations aborted with `A serial exception error occurred: Write timeout`
- Any esptool operation that leaves the chip in a bad USB state
- Can occur during both read and write operations

**Root Cause:**
The ESP32-S3's native USB-Serial/JTAG peripheral enters an unrecoverable state where:
- The USB PHY remains enumerated and visible to the host
- The chip-side peripheral is wedged and cannot process commands
- esptool's timeout logic cannot detect or recover from this state
- USB-level resets (DTR/RTS) do not reach the peripheral

**Why This Matters:**
This is **not** just a bench convenience issue. The wedge means:
- Physical recovery is **not dependable** even on a cabled node
- Raises the bar for OTA correctness (cannot rely on USB fallback)
- Affects emergency recovery, manufacturing, and field service
- Each wedge costs a physical site visit or bench intervention

---

### Current Recovery Procedures

#### Method 1: Physical USB Replug (Only Guaranteed Recovery)

```bash
# 1. Unplug the USB cable
# 2. Wait 2-3 seconds
# 3. Replug the cable
# 4. Verify device re-enumerates:
ls /dev/ttyACM*  # Should show /dev/ttyACM0

# 5. Test connection:
esptool --chip esp32s3 --port /dev/ttyACM0 chip-id
```

**This is the only 100% reliable recovery method currently known.**

#### Method 2: esptool Watchdog Reset (Preventative, Not Recovery)

**Use during normal flashing to avoid wedges:**

```bash
# GOOD: Leaves chip in runnable state
esptool --chip esp32s3 --port "$PORT" \
  --before usb-reset --after watchdog-reset \
  write-flash 0x20000 firmware.bin

# AVOID: Leaves chip in download mode (appears dead)
esptool --chip esp32s3 --port "$PORT" \
  --before usb-reset --after no-reset \
  write-flash 0x20000 firmware.bin
```

**Why `watchdog-reset` works:**
- `--after hard-reset` is a **NO-OP** on native USB ESP32-S3 (no bridge wiring RTS/EN)
- `--after watchdog-reset` triggers the chip's watchdog, causing a proper reboot
- USB device re-enumerates after reset, confirming recovery

**Current script status:**
`scripts/flash-esp32s3.sh` uses `--after no-reset`, which leaves the chip in download mode after each chunk. This is safe for multi-chunk flashes but requires a final `watchdog-reset` to boot the application.

#### Method 3: Boot Window Retry Loop (Emergency Workaround)

**When wedge occurs and physical replug is impossible:**

```bash
# Attempt to catch the chip during its ~15s boot window
for i in {1..30}; do
  echo "Attempt $i..."
  if esptool --chip esp32s3 --port /dev/ttyACM0 \
    --before usb-reset --after no-reset \
    chip-id 2>/dev/null; then
    echo "Recovered! Immediately flash:"
    esptool --chip esp32s3 --port /dev/ttyACM0 \
      --before usb-reset --after watchdog-reset \
      write-flash 0x20000 firmware.bin
    exit 0
  fi
  sleep 1
done
echo "Recovery failed - physical replug required"
```

**Success rate:** ~30-50% (depends on timing and wedge severity)

**Documented in:** `docs/plan/plan.md` - required sequence for eFuse enrollment phases

---

### Prevention Strategies

#### Flag Combinations to Avoid

```bash
# AVOID: Leaves chip in download mode
--after no-reset

# AVOID: Does nothing on native USB
--after hard-reset

# AVOID: Can trigger wedge on read operations
esptool read-flash 0x9000 0x6000  # Known wedge trigger
```

#### Recommended Practices

```bash
# DO: Use watchdog-reset for final operation
--after watchdog-reset

# DO: Use by-id symlinks (device path changes on reset)
PORT="/dev/serial/by-id/usb-Espressif_USB_JTAG_serial_debug_unit_XX:XX:XX:XX:XX:XX-if00"

# DO: Split large operations into chunks (prevents Guru Meditation)
# See scripts/flash-esp32s3.sh for 32KB chunking implementation

# DO: Verify connection before expensive operations
esptool --chip esp32s3 --port "$PORT" chip-id || exit 1
```

#### Flash Script Update Needed

**Current issue:** `scripts/flash-esp32s3.sh` ends with `--after no-reset`

**Recommended fix:** Add final reset after successful flash:

```bash
# After all chunks verified:
if [ "$fail" -eq 0 ]; then
  echo "All chunks verified. Booting application..."
  esptool --chip esp32s3 --port "$PORT" \
    --before usb-reset --after watchdog-reset \
    chip-id
fi
```

---

### Future Solutions (Not Yet Implemented)

#### USB Port Power Cycle

**Tool:** `uhubctl` (USB hub power control)

**Status:** Not available on current development system

**Implementation (if available):**

```bash
# Install: sudo apt install uhubctl
# Find hub port:
uhubctl -l

# Power cycle the port:
uhubctl -l -p 2 -a cycle  # Power cycle port 2

# Then immediately reconnect:
esptool --chip esp32s3 --port /dev/ttyACM0 chip-id
```

**Advantages:**
- No physical intervention required
- Can be automated in recovery scripts
- Works over remote serial connections

**Limitations:**
- Requires USB hub (not direct port)
- Hub must support per-port power switching
- Not available on all systems

#### Python USB Control

**Tool:** PyUSB (`usb.core`)

**Status:** Not installed on current system

**Implementation concept:**

```python
import usb.core
import usb.util

dev = usb.core.find(idVendor=0x303a, idProduct=0x1001)
if dev is None:
    raise ValueError('Device not found')

# Power cycle by resetting device
dev.reset()

# Re-enumerate and reconnect
```

**Advantages:**
- Can be integrated into recovery tools
- More precise control than hub-level power cycle

**Limitations:**
- Requires Python USB libraries
- May still trigger wedge if peripheral is wedged at hardware level

#### Kernel USB Device Reset

**Tool:** Linux USB device sysfs interface

**Implementation:**

```bash
# Find USB device path:
ls /sys/bus/usb/devices/*/idVendor | xargs grep -l 303a

# Reset device:
echo "1-1" > /sys/bus/usb/drivers/usb/unbind
sleep 1
echo "1-1" > /sys/bus/usb/drivers/usb/bind
```

**Status:** Untested - may or may not recover wedged peripheral

---

### Impact on OTA Strategy

**The wedge directly affects OTA reliability planning:**

1. **Cannot rely on USB fallback** - Physical recovery is not guaranteed
2. **OTA correctness is critical** - No easy recovery from bad firmware
3. **Safe mode requirements** - Must boot without networking for factory reset
4. **Watchdog timeouts** - Must auto-recover from hangs without USB
5. **Rollback enforcement** - Every update must validate within watchdog window

**See also:**
- `bf-1447x` - OTA correctness requirements
- `bf-1xywb` - Safe mode implementation
- `bf-2tgcx` - Watchdog configuration
- `docs/plan/plan.md` - Risk analysis for eFuse enrollment phases

---

### Recovery Checklist (When Wedge Occurs)

1. **Confirm wedge condition:**
   - Device visible in `/dev/ttyACM0` or `/dev/serial/by-id/`
   - esptool fails with `Write timeout` on **every** command
   - No serial output when opening port

2. **Attempt boot window recovery (if physical replug delayed):**
   - Run retry loop for 30 iterations
   - If successful, immediately flash with `--after watchdog-reset`
   - If failed, proceed to physical replug

3. **Physical replug (if on-site):**
   - Unplug USB cable
   - Wait 2-3 seconds
   - Replug cable
   - Verify re-enumeration with `ls /dev/ttyACM*`

4. **Post-recovery verification:**
   - Test connection: `esptool chip-id`
   - If flashing, end with `--after watchdog-reset`
   - Verify application boots (serial output, WiFi association)

5. **Document incident:**
   - Record operation that triggered wedge (usually `read-flash`)
   - Note recovery method used
   - Update `bf-4z6wh` if new patterns discovered

---

## Layer 4: Manufacturing / Mass Recovery

For recovering a batch of nodes simultaneously:

```bash
# Flash multiple nodes in parallel using esptool.py
for port in /dev/ttyUSB*; do
  esptool.py --chip esp32s3 --port $port \
    --baud 921600 write_flash \
    0x0      node/build/bootloader/bootloader.bin \
    0x8000   node/build/partition_table/partition-table.bin \
    0xe000   node/build/ota_data_initial.bin \
    0x10000  node/build/spaxel-node.bin &
done
wait
```

Or using Espressif's **Flash Download Tool** (Windows GUI) for batch flashing without command line.

---

## Recovery Page Enhancements (Future)

- **Diagnostics mode**: connect via Web Serial without flashing — read NVS keys, show current firmware version, show WiFi scan results
- **Selective NVS reset**: erase only WiFi credentials (not node name, position) for easier network migration
- **Firmware downgrade**: allow flashing a specific older version from a version picker
- **Serial console**: embedded terminal in the recovery page (xterm.js + Web Serial) for real-time ESP32 log output
