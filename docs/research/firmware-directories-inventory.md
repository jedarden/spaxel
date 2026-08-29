# Firmware Directories Inventory - Spaxel Project

**Generated:** 2026-08-29  
**Workspace:** `/home/coding/spaxel`  
**Hardware Platform:** ESP32-S3 (Espressif Systems)

## Summary

- **Total Firmware Directories:** 14 directories
- **Primary Hardware:** ESP32-S3 microcontroller (Xtensa LX7 dual-core 32-bit)
- **Development Framework:** ESP-IDF v5.2
- **Primary Application:** WiFi CSI-based indoor positioning system

## Complete Firmware Directory Listing

### 1. Main Firmware Project
- **Path:** `/home/coding/spaxel/firmware/`
- **Purpose:** Root directory for the complete ESP32-S3 firmware project
- **Notes:** Complete ESP-IDF project with build system, source code, tests, and documentation

### 2. Main Application Source
- **Path:** `/home/coding/spaxel/firmware/main/`
- **Purpose:** Primary application source code
- **Contents:** C implementation files for WiFi, CSI capture, BLE, WebSocket communication, captive portal, and OTA updates

### 3. Firmware Documentation
- **Path:** `/home/coding/spaxel/firmware/docs/`
- **Purpose:** Technical documentation for firmware
- **Notes:** Contains architecture, design, and usage documentation

### 4. Test Suite
- **Path:** `/home/coding/spaxel/firmware/test/`
- **Purpose:** Test files and test builds
- **Notes:** Contains unit tests and integration test infrastructure

### 5. Build Scripts
- **Path:** `/home/coding/spaxel/firmware/scripts/`
- **Purpose:** Build automation and utility scripts
- **Contents:** Build scripts, flash utilities, and development tools

### 6. Build Artifacts (Root)
- **Path:** `/home/coding/spaxel/firmware/build/`
- **Purpose:** Complete build output directory
- **Notes:** Contains all compiled binaries, object files, and build metadata

### 7. Bootloader Binary
- **Path:** `/home/coding/spaxel/firmware/build/bootloader/`
- **Purpose:** ESP32 bootloader binary
- **Notes:** First-stage bootloader for firmware startup and secure boot verification

### 8. ESP-IDF Framework Components
- **Path:** `/home/coding/spaxel/firmware/build/esp-idf/`
- **Purpose:** ESP-IDF framework compiled components
- **Contents:** Pre-built Espressif IoT Development Framework libraries and components

### 9. Main Firmware Build Artifacts
- **Path:** `/home/coding/spaxel/firmware/build/spaxel-firmware/`
- **Purpose:** Primary firmware compiled output
- **Contents:** `spaxel-firmware.bin` (flashable binary), `spaxel-firmware.elf` (debug symbol file), object files

### 10. Flash Partition Table
- **Path:** `/home/coding/spaxel/firmware/build/partition_table/`
- **Purpose:** Flash memory partition layout
- **Notes:** Defines memory layout for app, bootloader, OTA, and NVS storage

### 11. Managed Components (Root)
- **Path:** `/home/coding/spaxel/firmware/managed_components/`
- **Purpose:** ESP-IDF Component Manager dependencies
- **Notes:** Third-party libraries managed by ESP-IDF component system

### 12. WebSocket Client Library
- **Path:** `/home/coding/spaxel/firmware/managed_components/espressif__esp_websocket_client/`
- **Purpose:** WebSocket protocol client implementation
- **Notes:** Espressif official WebSocket client library for real-time CSI data streaming

### 13. mDNS Library
- **Path:** `/home/coding/spaxel/firmware/managed_components/espressif__mdns/`
- **Purpose:** multicast DNS (mDNS) responder
- **Notes:** Zero-configuration networking for local device discovery

### 14. Data Storage Directory
- **Path:** `/home/coding/spaxel/data/firmware/`
- **Purpose:** Firmware-related data storage
- **Status:** Empty - reserved for future use

## Firmware Architecture Overview

### Hardware Platform
- **Microcontroller:** ESP32-S3 (Xtensa LX7 dual-core 32-bit processor)
- **Wireless:** WiFi 802.11b/g/n, Bluetooth 5.0 LE
- **Key Features:** Hardware acceleration for cryptography, secure boot, flash encryption

### Software Stack
- **Framework:** ESP-IDF v5.2 (Espressif IoT Development Framework)
- **RTOS:** FreeRTOS-based multitasking
- **Language:** C (with C++ compatibility)

### Core Capabilities
1. **WiFi CSI Capture:** Channel State Information extraction for indoor positioning
2. **WebSocket Communication:** Real-time data streaming to backend servers
3. **BLE Scanning:** Bluetooth device identification and tracking
4. **OTA Updates:** Secure over-the-air firmware updates with anti-rollback protection
5. **Captive Portal:** WiFi provisioning interface for initial setup
6. **Secure Boot:** Hardware-based firmware verification and anti-rollback

## Build System

### Build Artifacts
- `spaxel-firmware.bin` - Flashable firmware binary
- `spaxel-firmware.elf` - Debug symbols and crash analysis
- `bootloader.bin` - First-stage bootloader
- `partition_table.bin` - Flash memory layout

### Security Features
- Secure boot verification
- Flash encryption support
- Anti-rollback protection for OTA updates
- Hardware-backed cryptographic acceleration

## Verification Notes

### Completeness Check
✅ All 14 firmware directories identified and documented  
✅ Build artifacts directory structure verified  
✅ Component manager dependencies catalogued  
✅ Hardware platform and framework version confirmed

### Key Observations
- Single, well-structured ESP32-S3 firmware project
- Comprehensive build infrastructure with proper ESP-IDF integration
- Secure firmware architecture with OTA update capability
- Modern WebSocket-based communication protocol
- Professional-grade embedded systems implementation

## Related Documentation

- ESP32-S3 Technical Reference Manual: [Espressif Documentation](https://www.espressif.com/en/products/socs/esp32-s3)
- ESP-IDF Programming Guide: [ESP-IDF v5.2 Documentation](https://docs.espressif.com/projects/esp-idf/en/v5.2/esp32s3/)
- CSI Analysis Papers: See `/home/coding/spaxel/docs/research/` for WiFi CSI positioning research

---

**Document Status:** Complete  
**Verification Date:** 2026-08-29  
**Total Directories:** 14 firmware directories identified and documented
