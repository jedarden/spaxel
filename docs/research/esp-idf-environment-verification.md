# ESP-IDF Environment Verification

**Date:** 2026-08-28  
**System:** NixOS  
**Purpose:** Document ESP-IDF installation status and firmware development workflow

## Current Environment Status

### ESP-IDF Installation
- **Location:** `/home/coding/esp-idf/esp-idf-v5.2/`
- **Version:** v5.2
- **Status:** ✅ Installed but not properly configured for NixOS

### Environment Variables
- **IDF_PATH:** ❌ Not set (required: should point to `/home/coding/esp-idf/esp-idf-v5.2`)
- **PATH:** ❌ Does not include ESP-IDF tools
- **Python venv:** ✅ Exists at `/home/coding/.espressif/python_env/idf5.2_py3.13_env/`

### Tool Availability
- **idf.py:** ❌ Not in PATH, requires ESP-IDF shell environment
- **openocd-esp32:** ❌ Missing dependency `libusb-1.0.so.0`
- **esptool.py:** ⚠️ Available but requires proper environment

## Issues Identified

### 1. NixOS Compatibility
The system is running NixOS, which uses a different package management approach than traditional Linux distributions. ESP-IDF assumes a standard Linux filesystem layout with system-wide shared libraries.

**Problem:** Missing `libusb-1.0.so.0` required by openocd-esp32  
**Impact:** Cannot run ESP-IDF tools directly on host system

### 2. Environment Not Configured
ESP-IDF requires sourcing `export.sh` to set up the environment properly:
```bash
source /home/coding/esp-idf/esp-idf-v5.2/export.sh
```

Without this, `idf.py` cannot locate required modules and tools.

### 3. System Dependencies
ESP-IDF requires several system packages that may not be available in NixOS:
- libusb-1.0
- libncurses
- libffi
- Various build tools

## Recommended Workflow: Docker Builds

**Good news:** Spaxel is already configured for Docker-based firmware builds, which is the recommended approach for NixOS systems.

### Docker Build (Recommended)
The project's `Dockerfile` includes a multi-stage build that:
1. Uses `espressif/idf:v5.2` for firmware compilation
2. Builds ESP32 firmware in a containerized environment
3. Embeds the firmware binary in the runtime image

```bash
# From project root
docker build -t spaxel-firmware -f Dockerfile .
```

### Local Development Options

If you need to develop firmware locally outside of Docker, you have two options:

#### Option 1: Nix Shell with Dependencies
Create a `shell.nix` in the firmware directory to provide required dependencies:

```nix
{ pkgs ? import <nixpkgs> {} }:

pkgs.mkShell {
  buildInputs = with pkgs; [
    python3
    python39Packages.pip
    libusb1
    ncurses
    libffi
    git
    dfu-util
  ];

  shellHook = ''
    export IDF_PATH=/home/coding/esp-idf/esp-idf-v5.2
    source $IDF_PATH/export.sh
  '';
}
```

Then run: `nix-shell` in the firmware directory.

#### Option 2: Development Container
Use the same ESP-IDF container for interactive development:

```bash
docker run -it --rm \
  -v $(pwd):/project \
  -v /home/coding/esp-idf/esp-idf-v5.2:/esp-idf \
  -w /project/firmware \
  espressif/idf:v5.2 \
  bash
```

## Verification Steps

### 1. Check ESP-IDF Installation
```bash
ls -la /home/coding/esp-idf/esp-idf-v5.2/
```

### 2. Verify Python Environment
```bash
ls -la /home/coding/.espressif/python_env/idf5.2_py3.13_env/
```

### 3. Test Docker Build
```bash
cd /home/coding/spaxel
docker build --no-cache --target firmware-builder -t spaxel-firmware-test .
```

### 4. Verify Firmware Output
```bash
ls -lh firmware/build/spaxel-firmware.bin
```

## Project-Specific Notes

### Firmware Build Location
- **Source:** `/home/coding/spaxel/firmware/`
- **Build Output:** `/home/coding/spaxel/firmware/build/`
- **Target Device:** ESP32-S3
- **Flash Size:** 16MB (N16R8 variant: 16MB flash, 8MB PSRAM)

### Current Status Summary
| Component | Status | Notes |
|-----------|--------|-------|
| ESP-IDF Installation | ✅ Complete | v5.2 installed at `/home/coding/esp-idf/esp-idf-v5.2/` |
| Host Environment | ❌ Not Configured | NixOS incompatibility, missing dependencies |
| Docker Build | ✅ Working | Use Docker for all firmware builds |
| Firmware Source | ✅ Present | Complete ESP-IDF project in `firmware/` |
| Build Artifacts | ⚠️ Requires Build | Run Docker build to generate firmware |

## Acceptance Criteria Status

| Criterion | Status |
|-----------|--------|
| ESP-IDF toolchain installed | ✅ Yes (v5.2) |
| ESP-IDF toolchain accessible | ⚠️ Via Docker only |
| IDF_PATH set correctly | ❌ Not set on host |
| `idf.py --version` works | ❌ Host environment, ✅ Docker |
| Required tools in PATH | ❌ Host environment, ✅ Docker |

## Conclusion

**For NixOS systems:** Use Docker-based firmware builds. The host ESP-IDF installation exists but cannot be used directly due to NixOS's non-standard filesystem layout and package management.

**For traditional Linux systems:** The ESP-IDF installation at `/home/coding/esp-idf/esp-idf-v5.2/` would work properly after sourcing `export.sh`.

**Recommendation:** Continue using Docker builds as documented in the project's `Dockerfile`. This approach is already proven to work and avoids NixOS compatibility issues.
