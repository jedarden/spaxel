# Spaxel Firmware Security Scripts

This directory contains scripts for managing Spaxel firmware security: Secure Boot V2, firmware signing, and anti-rollback protection.

## Overview

Spaxel firmware uses ESP-IDF Secure Boot V2 to prevent unauthorized firmware execution:

- **Signed App Verification**: Every firmware image is signed with an RSA-3072 private key (the only RSA size Secure Boot V2 supports). The bootloader verifies the signature before execution, rejecting any unsigned or incorrectly signed images.
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

Generates a new RSA-3072 key pair for firmware signing.

```bash
./scripts/generate-signing-key.sh [output_dir]              # dev: self-serve local key
./scripts/generate-signing-key.sh --stdout                  # prod: PEM on stdout only
./scripts/generate-signing-key.sh --stdout --pubout FILE    # also extract the public half
```

**Output:**
- `firmware_signing_key.pem` - Private key (KEEP SECRET!)
- Public key is derived and embedded in the bootloader

**Security Notes:**
- The private key MUST be backed up to a secure, offline location.
- Add the keys directory to `.gitignore` to prevent accidental commits.
- If the private key is lost, you cannot sign new firmware images.
- If the private key is compromised, immediately rotate to a new key and use Secure Boot V2 key revocation.
- **Development keys** land in `firmware/keys/` and are for bench boards only.
  **The production key is generated once via `--stdout`, piped into OpenBao,
  and never stored in a repo path or displayed on a terminal** — the full
  ceremony is in [`docs/notes/firmware-signing-keys.md`](../../docs/notes/firmware-signing-keys.md).

### sign-firmware.sh

Signs a firmware binary with one or more Secure Boot V2 private keys.

```bash
./scripts/sign-firmware.sh <binary_path> <output_path> [version]   # dev key from ../keys/
./scripts/sign-firmware.sh --keyfile K.pem <binary> <output>       # explicit key file
./scripts/sign-firmware.sh --key-fd 0 <binary> <output>            # key read from a file descriptor
```

**Parameters:**
- `binary_path` - Path to unsigned firmware `.bin` file
- `output_path` - Path for signed output
- `version` - Accepted for compatibility and echoed only; the anti-rollback value is stamped into the image by the build from `../SECURE_VERSION`
- `--keyfile PATH` - Repeatable, up to 3 keys total; one signature block per key (how a transitional rotation release is signed)
- `--key-fd FD` - Repeatable, same as `--keyfile /dev/fd/FD` — lets the key be piped in without touching disk

The production signing pipeline, with the key never landing on disk:

```bash
bao-as openbao-v2 bao kv get -field=key_pem \
    secret/ardenone-cluster/spaxel/firmware-signing/prod \
  | ./scripts/sign-firmware.sh --key-fd 0 build/spaxel.bin build/spaxel-signed.bin
```

The build does **not** invoke this script — `idf.py` signs internally when
secure boot is enabled (see `firmware/CMakeLists.txt`). Use it for
out-of-band signing of an existing image.

### bump-secure-version.sh

Increments the anti-rollback secure version and records the reason. Unlike the
app version (`../VERSION`), this counter moves only when a release changes the
firmware's security posture, because every node burns it into the one-way
`SECURE_VERSION` eFuse field.

```bash
./scripts/bump-secure-version.sh                        # show current state
./scripts/bump-secure-version.sh --reason "why"         # bump by one
./scripts/bump-secure-version.sh --to 4 --reason "why"  # bump to a value
```

Decreasing is refused outright, and every bump appends a row to
`../SECURE_VERSION_LOG.md`. Policy, the eFuse one-way behaviour and the
rollback interaction are documented in
`../../docs/notes/anti-rollback-secure-version.md`.

### inspect-firmware-header.py

Reads the anti-rollback fields back out of a built image — no IDF toolchain
required — and cross-checks the secure version in the image header against the
one in the app descriptor. Works on both the bare app image and the merged
flash image.

```bash
./scripts/inspect-firmware-header.py build/spaxel-firmware.bin --expect 1
```

Exit status is non-zero when the image does not carry the expected value, so
it is safe to use as a release gate. This is the check that would have caught
every image before 2026-09-05 shipping with secure version 0.

## Key Management

### Key Storage

- **Development**: Store keys in `firmware/keys/` (added to `.gitignore`)
- **Production**: OpenBao —
  `secret/ardenone-cluster/spaxel/firmware-signing/prod` on instance
  `openbao-v2`, written via the key ceremony in
  [`docs/notes/firmware-signing-keys.md`](../../docs/notes/firmware-signing-keys.md)
- **Backup**: an operator-held offline escrow copy (cold storage). Agents
  never create key copies outside OpenBao.

### Key Rotation

There is no calendar rotation — each rotation is a physical event (dual-signed
transitional release plus an irreversible eFuse burn per node). Rotate only on
compromise, suspected compromise, or a custody change:

1. Generate and store a new key as a new version at the same OpenBao path (via the ceremony)
2. Ship a dual-signed transitional release: `./scripts/sign-firmware.sh --keyfile NEW.pem --key-fd 0 <binary> <output>` (one signature block per key)
3. Deploy firmware to all nodes
4. Burn the key revocation eFuse for the retired key (requires physical access)

Full mechanics, and the eFuse budget (three trusted digests, ever), are in
[`docs/notes/firmware-signing-keys.md`](../../docs/notes/firmware-signing-keys.md).

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
