# Spaxel Firmware Build Instructions

## Quick Start (Recommended)

### Docker Build
The project uses Docker-based firmware builds, which work reliably on all systems including NixOS:

```bash
cd /home/coding/spaxel
docker build --target firmware-builder -t spaxel-firmware .
```

The firmware binary will be embedded in the Docker image at `/firmware/spaxel-firmware.bin`.

## Local Development

### Prerequisites
- **Target:** ESP32-S3 (N16R8 variant recommended: 16MB flash, 8MB PSRAM)
- **ESP-IDF:** v5.2.x
- **System:** Linux (Docker recommended for NixOS)

### ESP-IDF Installation Status
ESP-IDF v5.2 is installed at `/home/coding/esp-idf/esp-idf-v5.2/` but requires proper environment setup.

**Note for NixOS users:** Direct ESP-IDF usage on host system is not recommended due to library compatibility issues. Use Docker builds instead.

### Building Firmware (via Docker)
```bash
# Full build with Docker
cd /home/coding/spaxel
docker buildx build --platform linux/amd64 \
  -t spaxel-firmware:$(cat VERSION) \
  -f Dockerfile .

# Extract firmware from image
docker run --rm -v $(pwd)/firmware:/output \
  spaxel-firmware:$(cat VERSION) \
  cp /firmware/spaxel-firmware.bin /output/
```

### Building Firmware (Local - Non-NixOS Only)
If you're on a traditional Linux distribution with proper ESP-IDF setup:

```bash
cd firmware
. $HOME/esp-idf/esp-idf-v5.2/export.sh  # Source ESP-IDF environment
idf.py set-target esp32s3
idf.py build
idf.py flash  # Requires connected ESP32-S3
```

## Project Structure
```
firmware/
├── main/           # Main application source
├── CMakeLists.txt  # Build configuration
├── partitions.csv  # Flash partition layout
├── sdkconfig.defaults  # Default project configuration
└── build/          # Build output (generated)
```

## Hardware Requirements
- **MCU:** ESP32-S3
- **Flash:** 4MB minimum (16MB recommended for OTA)
- **PSRAM:** 8MB optional but recommended
- **Antenna:** External 2.4GHz antenna recommended

## Troubleshooting

### "Cannot import module esp_idf_monitor"
**Cause:** ESP-IDF environment not properly sourced  
**Solution:** Use Docker builds or run: `. $HOME/esp-idf/esp-idf-v5.2/export.sh`

### "error while loading shared libraries: libusb-1.0.so.0"
**Cause:** Missing system dependencies (common on NixOS)  
**Solution:** Use Docker builds - this is the recommended approach

### Permission denied on /dev/ttyUSB0
**Solution:** Add user to dialout group:
```bash
sudo usermod -a -G dialout $USER
```

## Verification
To verify the build environment, check the documentation:
- [ESP-IDF Environment Verification](../research/esp-idf-environment-verification.md) - Detailed system compatibility report
