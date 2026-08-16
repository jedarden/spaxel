# Spaxel ESP32-S3 Firmware

## Overview

This firmware runs on ESP32-S3 devices and implements:
- WiFi connectivity with exponential backoff reconnection
- CSI (Channel State Information) capture and streaming
- WebSocket communication with mothership
- BLE scanning for device identification
- OTA firmware updates
- Captive portal for provisioning

## Architecture

### FreeRTOS Tasks

| Task | Core | Priority | Stack | Responsibility |
|------|------|----------|-------|----------------|
| `app_main` | 1 | 1 | 8 KB | Startup sequencing, WiFi/WS lifecycle |
| `ws_task` | 1 | 5 | 8 KB | WebSocket send/receive loop |
| `csi_task` | 1 | 10 | 4 KB | CSI callback → binary frame serialization |
| `ble_scan_task` | 0 | 3 | 4 KB | BLE passive scan, advertisement parsing |
| `health_task` | 0 | 2 | 2 KB | Periodic health JSON assembly (every 10 s) |

### State Machine

See `firmware/main/main.c` for the complete state machine implementation. States include:
- `BOOT` - Initial startup and WiFi connection
- `MOTHERSHIP_DISCOVERY` - mDNS query and WebSocket connect
- `CONNECTED` - Normal operation (CSI streaming, BLE scanning)
- `WIFI_LOST` - WiFi reconnect loop with exponential backoff
- `MOTHERSHIP_UNAVAILABLE` - Mothership unreachable but WiFi OK
- `CAPTIVE_PORTAL` - WiFi credentials invalid, serving AP for re-provisioning

## Error Handling Patterns

**CRITICAL:** This firmware follows specific error handling patterns to prevent race conditions and abort loops. **Read this before modifying any code:**

### 1. Never use ESP_ERROR_CHECK in application code

❌ **WRONG:**
```c
ESP_ERROR_CHECK(some_function());  // Will abort on any error
```

✅ **CORRECT:**
```c
esp_err_t err = some_function();
if (err != ESP_OK) {
    ESP_LOGE(TAG, "Operation failed: %s", esp_err_to_name(err));
    return err;  // Let caller decide what to do
}
```

**Why:** `ESP_ERROR_CHECK` aborts the system immediately, creating abort loops for transient errors.

### 2. Check restart flags before long-running operations

✅ **REQUIRED PATTERN:**
```c
if (g_state.restarting) {
    ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] Skipping operation - restart imminent");
    return ESP_OK;  // Graceful skip, NOT an error
}

// Now perform the operation
esp_err_t err = some_long_operation();
```

**Why:** Prevents races between your operation and imminent system restart (OTA, reboot command, timeout).

**Required flags:**
- `g_state.restarting` - Set when system is about to reboot (6 trigger points)
- `g_state.ota_in_progress` - Set during OTA download

### 3. State machine handles errors, not individual functions

Functions return error codes. The state machine decides what to do:

```c
esp_err_t my_operation(void) {
    if (g_state.restarting) {
        return ESP_OK;  // Guard triggered
    }
    
    esp_err_t err = do_something();
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed: %s", esp_err_to_name(err));
        return err;
    }
    return ESP_OK;
}
```

**State machine:**
```c
case SOME_STATE:
    esp_err_t err = my_operation();
    if (err != ESP_OK) {
        if (g_state.restarting) {
            ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] Expected during restart");
        } else {
            ESP_LOGE(TAG, "Real error, will retry");
            // State machine will retry with exponential backoff
        }
    }
    break;
```

## Complete Documentation

For detailed error handling patterns and race condition prevention, see:

### Primary Documentation
- **[Error Handling Patterns](../docs/notes/error-handling-patterns.md)** - Complete guide to error handling philosophy
- **[ADR-010: Error Handling Patterns](../docs/notes/adr-010-error-handling-patterns.md)** - Architectural decision record

### Race Condition Analysis
- **[OTA WiFi Reconnection Race Summary](../docs/notes/ota-wifi-reconnection-race-summary.md)** - Analysis of OTA vs WiFi race
- **[OTA WiFi Race Investigation](../docs/notes/ota-wifi-race-investigation.md)** - Detailed investigation with call sequences

### Testing
- **[Manual OTA Test Procedure](../docs/tests/manual-ota-during-wifi-reconnect-test.md)** - Hardware testing guide
- **`firmware/test/test_ota_during_wifi_reconnect.c`** - Automated test for OTA guard
- **`firmware/test/test_all_restart_trigger_points.c`** - Test all restart trigger points
- **`firmware/test/test_restart_flag_propagation.c`** - Test flag visibility across tasks

## Key Components

### WiFi Management (`wifi.c`, `wifi.h`)
- `wifi_start_connect()` - Connect to stored credentials with exponential backoff
- `wifi_discover_mothership()` - mDNS query for mothership service
- Restart-safe guard prevents races with imminent restart

### CSI Capture (`csi.c`, `csi.h`)
- Promiscuous mode CSI capture from WiFi
- Binary frame serialization for WebSocket transmission
- Amplitude/phase extraction per subcarrier

### WebSocket Client (`ws.c`, `ws.h`)
- Bidirectional WebSocket communication with mothership
- Upstream: Binary CSI frames
- Downstream: JSON config, role, OTA commands
- Handles OTA download and verification

### BLE Scanning (`ble.c`, `ble.h`)
- Passive BLE advertisement scanning
- Device advertisement parsing (iBeacon, Eddystone, generic)
- RSSI aggregation and rotation heuristics

### OTA Updates (`ws.c`, `ws.h`)
- HTTPS-capable firmware download with SHA-256 integrity verification
- **Secure Boot V2** signed app verification (authenticity, not just integrity)
- **Anti-rollback protection** prevents downgrade attacks via eFuse-based security version
- Dual-partition scheme with automatic rollback on validation failure
- `ota_in_progress` flag prevents WiFi reconnection during download

#### Security Model

✅ **Signed App Verification**: Every firmware image is RSA-2048 signed. The bootloader verifies the signature before execution, rejecting any unsigned or incorrectly signed images.

✅ **Anti-Rollback**: An eFuse-based security version counter prevents downgrade attacks. Once a higher version boots, the device permanently refuses lower versions.

✅ **Defense in Depth**: Even if an attacker can intercept HTTP traffic or impersonate the mothership, they cannot execute arbitrary code because the bootloader will reject unsigned firmware.

See `docs/notes/ota-security-hardening-2026-08-15.md` for complete security architecture.

### State Machine (`main.c`)
- FreeRTOS task driving node lifecycle
- Event-driven state transitions
- Exponential backoff retry logic

## Testing

### Host-based Tests (gcc harness)
```bash
make -C firmware/test test
```
Tests NVS migration, binary frame serialization, and provisioning JSON parser without hardware.

### Hardware Tests (requires ESP32-S3)
See `firmware/test/` directory for test programs that validate:
- OTA vs WiFi reconnection race condition
- All `esp_restart()` trigger points
- Restart flag propagation across tasks
- Binary frame format validation

## Build Commands

The shipped ESP32-S3 configuration uses the chip's native USB-Serial/JTAG
peripheral as the primary console. On a native-USB-only board this is normally
the `/dev/ttyACM0` device. UART0 is on GPIO43/44 and is only visible through a
USB-UART bridge (normally `/dev/ttyUSB0`). The secondary-console option is not a
fallback for this: the primary console must match the port connected to the
host.

### Default native-USB build

For a clean build, remove only the generated ESP-IDF configuration and build
output, then let `sdkconfig.defaults` select USB-Serial/JTAG. No `menuconfig`
edits are required:

```bash
cd firmware
rm -f sdkconfig sdkconfig.old
rm -rf build
idf.py set-target esp32s3
idf.py build

# Confirm the generated selection before flashing
./scripts/verify-console-config.sh sdkconfig usb

# Flash and monitor on the native USB CDC device
idf.py -p /dev/ttyACM0 flash
idf.py -p /dev/ttyACM0 monitor
```

The generated `sdkconfig` should contain `CONFIG_ESP_CONSOLE_USB_SERIAL_JTAG=y`,
`# CONFIG_ESP_CONSOLE_UART_DEFAULT is not set`,
`CONFIG_ESP_CONSOLE_UART_NUM=-1`, and
`CONFIG_ESP_SYSTEM_PANIC_PRINT_REBOOT=y`. The boot log should continue past the
bootloader into `app_main` on `/dev/ttyACM0`.

### UART0 override for bridge-equipped boards

For a board with a CP210x, CH340, FTDI, or another USB-UART bridge wired to
GPIO43/44, layer `sdkconfig.uart-console` over the normal defaults. Start from
a clean generated configuration so the defaults are actually re-applied:

```bash
cd firmware
rm -f sdkconfig sdkconfig.old
rm -rf build
idf.py -D SDKCONFIG_DEFAULTS="sdkconfig.defaults;sdkconfig.uart-console" set-target esp32s3
idf.py -D SDKCONFIG_DEFAULTS="sdkconfig.defaults;sdkconfig.uart-console" build

# Confirm that the layered defaults selected UART0 before flashing
./scripts/verify-console-config.sh sdkconfig uart

# The UART bridge is normally ttyUSB0, not the native CDC device.
idf.py -p /dev/ttyUSB0 flash
idf.py -p /dev/ttyUSB0 monitor
```

Verify the override before flashing: `sdkconfig` should show
`CONFIG_ESP_CONSOLE_UART_DEFAULT=y`,
`# CONFIG_ESP_CONSOLE_USB_SERIAL_JTAG is not set`, and
`CONFIG_ESP_CONSOLE_UART_NUM=0`. Do not edit the shared defaults or copy a
generated `sdkconfig` between board types.

### Console panic check

Both console variants explicitly use `CONFIG_ESP_SYSTEM_PANIC_PRINT_REBOOT=y`,
so an application panic prints the panic reason and backtrace before rebooting
on the selected primary console. After the first clean flash, deliberately
perform one controlled panic check on the bench: add a temporary
`esp_system_abort("console panic probe")` immediately after the first
`app_main` log line, rebuild and flash, and leave the matching `idf.py monitor`
running. Confirm that the capture contains `Guru Meditation Error` (or the
equivalent panic reason), `Backtrace:`, and decoded PC entries. Remove the probe
and rebuild the normal image immediately afterward; never ship the probe.

## NVS Layout

All keys in namespace `"spaxel"`:

| Key | Type | Max | Description |
|-----|------|-----|-------------|
| `schema_ver` | uint8 | 1 B | NVS schema version for migration |
| `provisioned` | uint8 | 1 B | 0 = not provisioned, 1 = provisioned |
| `wifi_ssid` | str | 32 B | WiFi network SSID |
| `wifi_pass` | str | 64 B | WiFi passphrase |
| `node_id` | str | 37 B | UUID4 string assigned by mothership |
| `node_token` | str | 65 B | HMAC-SHA256 token for auth |
| `ms_mdns` | str | 64 B | mDNS service name |
| `ms_ip` | str | 46 B | Fallback mothership IP |
| `ms_port` | uint16 | 2 B | Mothership HTTP port |
| `passive_bss` | blob | 6 B | AP BSSID for passive radar mode |
| `role` | uint8 | 1 B | Last assigned role (0=TX, 1=RX, 2=TX_RX, 3=PASSIVE, 4=IDLE) |
| `pkt_rate` | uint8 | 1 B | Current packet rate Hz |
| `ap_mode` | uint8 | 1 B | Force captive portal on next boot |
| `debug` | uint8 | 1 B | Verbose USB serial logging |

## Common Patterns

### Adding a New Long-Running Operation

1. **Check restart guard at entry:**
   ```c
   if (g_state.restarting) {
       ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] Skipping - restart imminent");
       return ESP_OK;
   }
   ```

2. **Use explicit error checking:**
   ```c
   esp_err_t err = some_api();
   if (err != ESP_OK) {
       ESP_LOGE(TAG, "Failed: %s", esp_err_to_name(err));
       return err;
   }
   ```

3. **Return error codes, never abort:**
   ```c
   // NEVER: ESP_ERROR_CHECK(foo());
   // ALWAYS: esp_err_t err = foo(); if (err != ESP_OK) { ... }
   ```

### Adding a New Restart Trigger Point

If you're adding code that calls `esp_restart()`:

1. **Set the flag well before restart:**
   ```c
   g_state.restarting = true;
   // ... cleanup code ...
   esp_restart();
   ```

2. **Add test coverage:**
   - Add test case to `test_all_restart_trigger_points.c`
   - Validate flag propagation to relevant tasks

3. **Document the trigger point:**
   - Update `error-handling-patterns.md` with the new location
   - Add logging with clear reason

## Debugging

### Enable Verbose Logging

Set via NVS during provisioning:
```c
nvs_set_u8("debug", 1);
```

### Common Log Patterns

- `[RESTART-SAFE-GUARD]` - Restart guard triggered (expected, not an error)
- `[OTA]` - OTA task lifecycle events
- `[wifi_start_connect] Step N:` - WiFi connection progress
- `WIFI_EVENT_STA_DISCONNECTED` - WiFi lost (will trigger reconnection)

### Known Race Conditions

All race conditions have been eliminated through flag coordination:

✅ **OTA vs WiFi reconnection** - `ota_in_progress` flag (see ota-wifi-reconnection-race-summary.md)  
✅ **Restart vs WiFi operations** - `restarting` flag (this pattern)  
✅ **Restart vs OTA completion** - Coordinated flag sequencing (ADR-004)

## Contributing

When contributing to this firmware:

1. **Read the error handling patterns** (see above)
2. **Add restart guards** before any long-running operation
3. **Use explicit error checking**, never `ESP_ERROR_CHECK`
4. **Add test coverage** for new functionality
5. **Update documentation** for new patterns or states

## License

See project root LICENSE file.
