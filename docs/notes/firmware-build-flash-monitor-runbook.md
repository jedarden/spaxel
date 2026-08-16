# ESP32-S3 Firmware Build, Flash, and Monitor Runbook

**Purpose:** Complete procedure for building and flashing `firmware/` to real ESP32-S3 hardware, including workarounds for esptool USB-JTAG instability discovered during 2026-07-28 bring-up.

**Target Hardware:** ESP32-S3 with native USB-Serial/JTAG interface (appears as `/dev/ttyACM0` on Linux)

## Overview

This runbook covers:
1. **Build:** ESP-IDF v5.2 compilation in Docker
2. **Flash:** Chunked flashing workaround for USB-JTAG instability
3. **Monitor:** Serial console monitoring for boot logs and runtime debugging
4. **Backup:** Pre-flash backup recommendations

---

## Part 1: Build Firmware

### Prerequisites

- Docker with ESP-IDF v5.2 environment
- ESP32-S3 device with native USB-Serial/JTAG interface
- Host machine with USB access

### Build Process

```bash
cd /path/to/spaxel/firmware

# Clean build environment
rm -rf build sdkconfig sdkconfig.old

# Set target and build
idf.py set-target esp32s3
idf.py build
```

**Expected Output:**
- Binary size: ~1.6 MB (1,631,552 bytes typical)
- Partition fit: Factory partition at 0x1F0000 with ~20% free space
- Build artifacts in `build/` directory

**Important Docker Considerations:**
- If Docker daemon uses userns-remap, running container as host UID fails with bind-mount permission errors
- Solution: Run container with default UID (maps to host user automatically)

**Generated Files:**
- `build/spaxel-firmware.bin` - Main application binary
- `build/bootloader/bootloader.bin` - ESP-IDF bootloader
- `build/partition_table/partition-table.bin` - Partition table
- `build/ota_data_initial.bin` - OTA data partition

---

## Part 2: Flash Firmware (with Chunked-Flash Workaround)

### The USB-JTAG Instability Problem

**Symptom:** Large `write-flash` operations fail with misleading chip-side panic messages:

```
A fatal error occurred: Guru Meditation Error detected (StoreProhibited)
A fatal error occurred: Guru Meditation Error detected (IllegalInstruction)
A fatal error occurred: The chip stopped responding
```

**Critical Insight:** These messages are **transport artifacts**, not firmware crashes. The firmware boots cleanly once fully written. Before diagnosing a "crashing" ESP32, verify the flash is complete.

**Observed Behavior:**
- Writes ≤20 KB succeed reliably
- Writes ≥64 KB fail consistently
- Boundary is not a clean cutoff (32 KB succeeded, 24 KB failed once)
- `erase-flash` and `erase-region` fail similarly
- `--before usb-reset` required for reliable download mode entry
- `--after no-reset` avoids teardown failures in `soft_reset()`

### Solution: Chunked Flashing

The workaround: split the app image into 32 KB chunks and write each separately with retries. This approach was validated during 2026-07-28 bring-up with 50 chunks, all verified successfully.

### Using the Chunked-Flash Script

**Location:** `scripts/flash-esp32s3.sh` (committed in repo)

**Fresh Chip Flash** (bootloader + partition table + app):
```bash
cd /path/to/spaxel
./scripts/flash-esp32s3.sh /dev/ttyACM0 \
  0x0:firmware/build/bootloader/bootloader.bin \
  0x8000:firmware/build/partition_table/partition-table.bin \
  0x10000:firmware/build/ota_data_initial.bin \
  0x20000:firmware/build/spaxel-firmware.bin
```

**App-Only Update** (preserves NVS/WiFi credentials, no erase):
```bash
./scripts/flash-esp32s3.sh /dev/ttyACM0 \
  0x20000:firmware/build/spaxel-firmware.bin
```

**What the Script Does:**
1. Splits each image into 32 KB chunks
2. Writes each chunk with up to 6 retries
3. Verifies each write with `Hash of data verified`
4. Reports OK/FAIL status per chunk
5. Boots application with `watchdog-reset` (avoids USB wedge)

**Expected Output:**
```
OK   bootloader.bin @ 0x0 (c_0000)
OK   bootloader.bin @ 0x8000 (c_0001)
...
OK   spaxel-firmware.bin @ 0x1f0000 (c_0049)
All chunks verified. Booting application...
Application booted successfully.
```

**Troubleshooting Flash Failures:**

If chunks fail after retries:
1. Check USB cable connection (use short, high-quality cable)
2. Verify port is correct (`ls /dev/ttyACM*`)
3. Try physical reset: disconnect USB, reconnect, retry
4. If persistent, the device may have a hardware issue

---

## Part 3: Monitor Serial Console

### Console Routing

The ESP32-S3 console appears on the native USB-Serial/JTAG interface (`/dev/ttyACM0`). For boards with secondary UART bridges, UART0 is on GPIO43/44 (`/dev/ttyUSB0`).

**Console is required for:**
- Boot diagnostics
- Runtime debugging
- State machine observation
- Error message visibility

### Monitoring Methods

**Method 1: Direct serial cat**
```bash
# Open port before reset to catch full boot sequence
cat /dev/ttyACM0

# Then trigger reset (Ctrl+T not applicable, use physical reset button)
```

**Method 2: idf.py monitor (ESP-IDF native)**
```bash
cd firmware
idf.py -p /dev/ttyACM0 monitor
```

**Method 3: screen/minicom**
```bash
screen /dev/ttyACM0 115200
# or
minicom -D /dev/ttyACM0 -b 115200
```

### Expected Boot Sequence

Successful boot shows:
```
ESP-ROM:esp32s3-rcocup-20230508
...
I (123) boot: ESP-IDF v5.2-second
I (145) boot: Chip revision: 0
...
I (1234) main: [BOOT] Starting spaxel firmware
I (1256) wifi: WiFi credentials found
I (1345) main: [CONNECTED] Mothership connection established
```

### Common Boot Patterns

**Normal Boot:**
- Bootloader logs → partition table → app startup
- WiFi connection → mothership discovery → WebSocket connect
- Transition to `CONNECTED` state

**Provisioning Mode (Captive Portal):**
- `[BOOT] No WiFi credentials found`
- `[CAPTIVE_PORTAL] Starting AP for provisioning`
- Connect to `spaxel-setup` WiFi network

**WiFi Connection Issues:**
- `[WIFI_LOST] WiFi disconnected, retrying...`
- Exponential backoff retry logs
- Check credentials in NVS

---

## Part 4: Backup and Recovery

### Pre-Flash Backup (Critical!)

Before first flash of a new board, **always back up the factory flash contents**. This preserves the original firmware for recovery if needed.

**Backup Procedure:**
```bash
# Read entire flash (8 MB for ESP32-S3 with 4MB flash)
esptool --chip esp32s3 --port /dev/ttyACM0 --no-stub \
  --before usb-reset --after no-reset \
  read_flash 0x0 0x800000 \
  /path/to/backups/board-original-$(date +%Y%m%d).bin

# Verify backup integrity
esptool --chip esp32s3 --port /dev/ttyACM0 --no-stub \
  --before usb-reset --after no-reset \
  verify_flash 0x0 /path/to/backups/board-original-$(date +%Y%m%d).bin
```

**Original Board Flash Backup Location:**
- Store in secure, off-repo location: `~/backups/esp32s3-boards/`
- Naming convention: `<serial-or-date>-original-flash.bin`
- Include README with board details (model, date, purpose)

### Recovery

If flashing fails or board becomes unresponsive:
1. Restore original backup using reverse of backup procedure
2. Use manual download mode: hold BOOT button, tap RESET, release BOOT
3. Retry chunked flash with fresh USB connection

---

## Part 5: Complete End-to-End Example

**From clean checkout to booting node:**

```bash
# 1. Build firmware
cd /path/to/spaxel/firmware
rm -rf build sdkconfig sdkconfig.old
idf.py set-target esp32s3
idf.py build

# 2. Backup original flash (first time only)
esptool --chip esp32s3 --port /dev/ttyACM0 --no-stub \
  --before usb-reset --after no-reset \
  read_flash 0x0 0x800000 \
  ~/backups/esp32s3-boards/$(date +%Y%m%d)-original.bin

# 3. Flash firmware with chunked workaround
cd /path/to/spaxel
./scripts/flash-esp32s3.sh /dev/ttyACM0 \
  0x0:firmware/build/bootloader/bootloader.bin \
  0x8000:firmware/build/partition_table/partition-table.bin \
  0x10000:firmware/build/ota_data_initial.bin \
  0x20000:firmware/build/spaxel-firmware.bin

# 4. Monitor console
cd /path/to/spaxel/firmware
idf.py -p /dev/ttyACM0 monitor

# 5. Provision via captive portal (first boot only)
# Connect to "spaxel-setup" WiFi, follow web UI
```

---

## Part 6: Troubleshooting Guide

### Flash-Related Issues

**"Hash of data verified" missing from output:**
- Chunk write failed - check USB cable and connection
- Retry the specific chunk manually

**"A fatal error occurred" during flash:**
- Normal for large writes - this is why chunked flash exists
- Use the chunked script instead of direct esptool calls

**Chunks fail repeatedly:**
- Try different USB port
- Replace USB cable (use short, high-quality cable)
- Check device is properly powered

### Boot-Related Issues

**No console output at all:**
- Verify correct port device (`/dev/ttyACM0` vs `/dev/ttyUSB0`)
- Check USB-Serial/JTAG is enabled in `sdkconfig`
- For UART boards, verify console routing configuration

**Bootloader loops endlessly:**
- Flash may be incomplete - retry chunked flash
- Check partition table matches expected layout

**Panic on boot:**
- Verify app binary size fits partition
- Check NVS partition integrity
- Review panic message for specific error

### Runtime Issues

**WiFi won't connect:**
- Verify credentials in NVS
- Check for captive portal mode
- Review WiFi logs for specific failure reason

**Mothership unreachable:**
- Verify mDNS is working
- Check network connectivity
- Review WebSocket connection logs

---

## Appendix: Technical Details

### Partition Layout

```
0x0          - Bootloader (28 KB)
0x8000       - Partition table (4 KB)
0x10000      - OTA data (8 KB)
0x20000      - Factory app (1.9 MB)
0x1F0000     - NVS (default config, may vary)
0x210000     - Phy init (4 KB)
0x220000     - Storage (varies)
```

### esptool Parameters Explained

- `--no-stub` - Bypass stub flasher (more reliable for native USB)
- `--before usb-reset` - Reset USB device before operation
- `--after no-reset` - Don't reset after write (avoids soft_reset issues)
- `--after watchdog-reset` - Use watchdog for clean reboot

### Chunk Size Rationale

32 KB chunks were chosen empirically:
- Small enough to succeed reliably
- Large enough to keep total chunk count manageable (50 for 1.6 MB app)
- Balanced against retry overhead

---

## References

- **Firmware README:** `firmware/README.md` - Architecture and error handling patterns
- **Error Handling:** `docs/notes/error-handling-patterns.md` - Anti-abort patterns
- **Console Routing:** `docs/notes/` - See console-routing sibling bead for USB-Serial/JTAG details
- **Script Source:** `scripts/flash-esp32s3.sh` - Chunked flash implementation

---

## Change History

- **2026-07-28:** Initial bring-up, USB-JTAG instability discovered
- **2026-08-16:** Documented runbook with chunked-flash workaround
- **Future:** Add UART bridge support for `/dev/ttyUSB0` boards
