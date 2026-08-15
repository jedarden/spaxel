# OTA Security Hardening Implementation

**Date:** 2026-08-15
**Bead:** spaxel-8aa9703c
**ADR:** ADR-004 / bf-1447x

## Problem Statement

The OTA firmware update path had a critical security vulnerability:

- **Unsigned firmware**: OTA images were not cryptographically signed
- **Plain HTTP downloads**: Firmware fetched over unencrypted HTTP (CONFIG_ESP_HTTP_CLIENT_ENABLE_HTTPS=n)
- **SHA-256 only**: Integrity verification against corruption, but no authenticity check
- **No anti-rollback**: Downgrade attacks possible

**Consequence**: Anyone who could impersonate the mothership on the node's network, or serve the firmware URL, could flash arbitrary code onto every node — and the nodes are physically distributed around a home.

## Solution

Implemented **defense-in-depth** through ESP-IDF Secure Boot V2, signed app verification, and anti-rollback protection.

### 1. Secure Boot V2 (Signed App Verification)

**Configuration Changes:** `firmware/sdkconfig.defaults`

```ini
# Secure Boot V2 — signed app verification
CONFIG_SECURE_BOOT_V2=y
CONFIG_SECURE_BOOT_V2_RSA_SUPPORTED=y
CONFIG_SECURE_BOOT_V2_PREFERRED=y
CONFIG_SECURE_BOOT_V2_RSA_MODE_2048=y
CONFIG_SECURE_SIGNED_APPS_RSA_SCHEME="scheme_rsa_pss"
CONFIG_SECURE_BOOT_V2_ALLOW_EFUSE_DISABLE=y
```

**How it works:**
- A RSA-2048 private key signs every firmware image during build
- The public key is embedded in the bootloader
- At boot time, the bootloader verifies the RSA-PSS signature before executing the app
- **Unsigned or incorrectly signed images are rejected**

**Threats mitigated:**
- ✅ Mothership compromise: Attacker cannot flash malicious firmware without the private key
- ✅ Network interception: Attacker cannot serve unsigned firmware via MITM
- ✅ Direct flash attacks: Physical or debug interface attacks still possible (use Secure Boot + hardware disable)

**Threats NOT mitigated:**
- ❌ Physical access with JTAG/USB (requires hardware debug disable)
- ❌ Private key compromise (requires key rotation and revocation)

### 2. Anti-Rollback Protection

**Configuration Changes:** `firmware/sdkconfig.defaults`

```ini
# OTA — anti-rollback protection (prevents downgrade attacks)
CONFIG_APP_ANTI_ROLLBACK=y
CONFIG_BOOTLOADER_APP_ANTI_ROLLBACK_EFuse=y
CONFIG_BOOTLOADER_APP_ANTI_ROLLBACK_EFuse_WP_DISABLE=y
```

**How it works:**
- An eFuse stores a security version counter (starts at 0)
- Each firmware image includes a security version in its app descriptor
- The bootloader refuses to boot any image with a security version lower than the eFuse value
- When a higher version boots successfully, the eFuse is permanently updated

**Threats mitigated:**
- ✅ Downgrade attacks: Attacker cannot flash old, vulnerable firmware versions
- ✅ Version rollback protection: Even with mothership compromise, attackers cannot revert to known-buggy versions

**Note:** The security version is currently tied to the firmware version (`PROJECT_VER`). For production, you may want a separate security version that increments on security fixes only, independent of feature releases.

### 3. HTTPS Downloads (Future Work)

**Current state:** `CONFIG_ESP_HTTP_CLIENT_ENABLE_HTTPS=n` (plain HTTP)

**Why not enabled yet:**
- The task notes that HTTPS has flash/RAM cost and interacts with the 4MB budget (bf-436bh)
- TLS handshake requires additional heap (~1.4 KB for cert bundle)
- Flash footprint increase for mbedTLS + cert bundle

**Future work:**
- Enable HTTPS in sdkconfig.defaults: `CONFIG_ESP_HTTP_CLIENT_ENABLE_HTTPS=y`
- The code already supports HTTPS via `.crt_bundle_attach` (see `websocket.c:876`)
- Measure flash/RAM impact and adjust partition sizes if needed
- Consider HTTP for local/bench deployments, HTTPS for internet-facing

## Implementation Details

### Signing Key Generation

New script: `firmware/scripts/generate-signing-key.sh`

```bash
cd firmware
./scripts/generate-signing-key.sh
```

**Output:**
- `keys/firmware_signing_key.pem` - RSA-2048 private key (SECRET!)

**Security requirements:**
- ❌ **NEVER** commit the private key to git (already in `.gitignore`)
- ✅ **DO** back up the key to secure, offline storage
- ✅ **DO** store production keys in a hardware security module (HSM)
- ✅ **DO** rotate keys immediately if compromise is suspected

### Firmware Signing

New script: `firmware/scripts/sign-firmware.sh`

The build system (`CMakeLists.txt`) automatically signs firmware when Secure Boot V2 is enabled. No manual signing required during normal builds.

```bash
cd firmware
idf.py build  # Automatically signs bootloader and app
```

### Build Integration

Updated `firmware/CMakeLists.txt` to:
- Detect Secure Boot V2 configuration
- Locate signing key at `keys/firmware_signing_key.pem`
- Fail build with clear error if key is missing
- Log security version for anti-rollback

## Deployment Considerations

### Development Boards

During development, Secure Boot can be disabled via eFuse for flexibility:

```bash
# Disable secure boot (development only!)
espefuse.py --port /dev/ttyUSB0 burn_efuse SECURE_BOOT_DISABLE
```

**WARNING:** This is one-time, irreversible! Only use on development boards.

### Production Devices

For production deployment:
1. Generate a production signing key (separate from development key)
2. Enable Secure Boot V2 in production firmware builds
3. Flash signed firmware to devices
4. Secure Boot is automatically enabled when the first signed app boots
5. (Optional) Burn `DISABL_PAD_JTAG` and `DISABLE_USB_DOWNLOAD_MODE` efuses for physical security

### Key Rotation

If the private key is compromised:

1. Generate a new signing key: `./scripts/generate-signing-key.sh`
2. Rebuild and sign firmware with the new key
3. Deploy firmware to all devices
4. Burn key revocation eFuse to invalidate the old public key:

```bash
espefuse.py --port /dev/ttyUSB0 burn_efuse SECURE_BOOT_KEY_REVOKE0
```

**WARNING:** Key revocation is irreversible! Ensure all devices are running firmware signed with the new key before revoking.

## Verification

### Verify Signed Firmware

```bash
espsecure.py verify_signature \
    --version 2 \
    --keyfile <public_key.pem> \
    <signed_firmware.bin>
```

### Check Secure Boot Status on Device

Monitor boot logs for:
```
I (xxx) secure_boot: Signature verified
I (xxx) secure_boot: Bootloader image is valid
```

### Test Anti-Rollback

Attempt to flash an older firmware version (lower security version). The bootloader should refuse to boot it and roll back to the previous partition.

## References

- [ESP-IDF Secure Boot V2 Documentation](https://docs.espressif.com/projects/esp-idf/en/v5.2/esp32s3/security/secure-boot-v2.html)
- [ESP-IDF Anti-Rollback Protection](https://docs.espressif.com/projects/esp-idf/en/v5.2/esp32s3/security/flash-encryption.html#anti-rollback)
- [Circuit Labs: OTA Update Security for ESP32](https://circuitlabs.net/ota-update-security-for-esp32/)
- [MDPI: ESP32-Based Hardware Key for Software Application Protection](https://www.mdpi.com/2076-3417/16/9/4251)

## Future Work

1. **HTTPS enablement**: Enable CONFIG_ESP_HTTP_CLIENT_ENABLE_HTTPS=y after measuring flash/RAM impact
2. **Partition sizing**: Adjust partition sizes if needed for TLS overhead
3. **Security versioning**: Implement separate security version counter independent of firmware version
4. **Hardware security**: Burn eFuses to disable JTAG/USB for physical attack resistance
5. **Key management**: Integrate with HSM for production signing key storage

## Summary

| Feature | Status | Threat Mitigated |
|---------|--------|------------------|
| Signed App Verification | ✅ Enabled | Mothership compromise, network interception |
| Anti-Rollback | ✅ Enabled | Downgrade attacks |
| HTTPS Downloads | ⏳ Future (bf-436bh) | Network interception (defense-in-depth) |
| Physical Security | ⏳ Future | Physical access with debug interfaces |

**Overall security posture**: Significantly improved. The primary OTA attack vector (unsigned firmware over HTTP) is now closed by Secure Boot V2 signature verification, even before HTTPS is enabled.
