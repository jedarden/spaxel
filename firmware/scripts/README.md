# Spaxel Firmware Security Scripts

This directory contains scripts for managing Spaxel firmware security: Secure Boot V2, firmware signing, and anti-rollback protection.

## Overview

Spaxel firmware uses ESP-IDF Secure Boot V2 to prevent unauthorized firmware execution:

- **Signed App Verification**: Every firmware image is signed with an RSA-2048 private key. The bootloader verifies the signature before execution, rejecting any unsigned or incorrectly signed images.
- **Anti-Rollback**: An eFuse-based security version counter prevents downgrade attacks. Once a higher security version is written, the device permanently refuses to boot lower versions.
- **Defense in Depth**: Even if an attacker can intercept HTTP traffic or impersonate the mothership, they cannot execute arbitrary code because the bootloader will reject unsigned firmware.

## Security Model

### Threats Mitigated

- **Mothership Compromise**: An attacker who gains control of the mothership cannot flash malicious firmware to nodes without the private signing key.
- **Network Interception**: An attacker who can intercept OTA traffic cannot serve unsigned firmware to nodes.
- **Downgrade Attacks**: Anti-rollback prevents attackers from flashing old, vulnerable firmware versions.

### Threats NOT Mitigated

- **Physical Access**: An attacker with physical access to a device can still probe debug interfaces (JTAG/USB) unless those are disabled in hardware.
- **Key Compromise**: If the private signing key is exposed, an attacker can sign malicious firmware. Key rotation and revocation are required in this case (see below).

## Scripts

### generate-signing-key.sh

Generates a new RSA-2048 key pair for firmware signing.

```bash
./scripts/generate-signing-key.sh [output_dir]
```

**Output:**
- `firmware_signing_key.pem` - Private key (KEEP SECRET!)
- Public key is derived and embedded in the bootloader

**Security Notes:**
- The private key MUST be backed up to a secure, offline location.
- Add the keys directory to `.gitignore` to prevent accidental commits.
- If the private key is lost, you cannot sign new firmware images.
- If the private key is compromised, immediately rotate to a new key and use Secure Boot V2 key revocation.

### sign-firmware.sh

Signs a firmware binary with the private key.

```bash
./scripts/sign-firmware.sh <binary_path> <output_path> [version]
```

**Parameters:**
- `binary_path` - Path to unsigned firmware `.bin` file
- `output_path` - Path for signed output
- `version` - Optional firmware version string (for anti-rollback)

This script is typically called automatically by the build system (see `CMakeLists.txt`).

## Key Management

### Key Storage

- **Development**: Store keys in `firmware/keys/` (added to `.gitignore`)
- **Production**: Store private keys in a hardware security module (HSM) or offline cold storage
- **Backup**: Maintain secure, offline backups of private keys

### Key Rotation

If the private key is compromised, rotate to a new key:

1. Generate a new signing key: `./scripts/generate-signing-key.sh`
2. Rebuild and sign firmware with the new key
3. Deploy firmware to all nodes
4. Burn the key revocation eFuse to invalidate the old public key (requires physical access or secure bootloader support)

### Key Revocation

Secure Boot V2 supports key revocation via eFuse:

```bash
# Revoke old public key (one-time operation, irreversible!)
espefuse.py --port /dev/ttyUSB0 burn_efuse SECURE_BOOT_KEY_REVOKE0
```

**WARNING**: This operation is irreversible! Only use after verifying all nodes are running firmware signed with the new key.

## Build Integration

Firmware signing is integrated into the CMake build process. See `firmware/CMakeLists.txt` for details.

During `idf.py build`, the build system automatically:
1. Compiles and links the firmware
2. Signs the bootloader and app with the private key
3. Embeds the public key in the bootloader
4. Outputs signed binaries in `build/`

## Verification

To verify a signed firmware image:

```bash
espsecure.py verify_signature \
    --version 2 \
    --keyfile public_key.pem \
    signed_firmware.bin
```

## References

- [ESP-IDF Secure Boot V2 Documentation](https://docs.espressif.com/projects/esp-idf/en/v5.2/esp32s3/security/secure-boot-v2.html)
- [ESP-IDF Anti-Rollback Protection](https://docs.espressif.com/projects/esp-idf/en/v5.2/esp32s3/security/flash-encryption.html#anti-rollback)
- [ADR-004: OTA Security](../../docs/notes/adr-004-ota-security.md)

## License

See project root LICENSE file.
