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
