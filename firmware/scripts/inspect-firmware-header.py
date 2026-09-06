#!/usr/bin/env python3
"""Inspect (and optionally verify) the anti-rollback fields of a firmware image.

Reads two things out of an ESP-IDF app image without needing the IDF
toolchain installed:

  * ``secure_version`` — byte 19 of ``esp_image_header_t``, written by
    esptool from ``CONFIG_BOOTLOADER_APP_SECURE_VERSION`` and the value the
    ESP32-S3 bootloader compares against the SECURE_VERSION eFuse field.
  * the app descriptor's own ``secure_version`` — a u32 at offset 4 of
    ``esp_app_desc_t``, which Kconfig copies at compile time.

They are cross-checked: they can only disagree if the build wired one path
but not the other, which is precisely the silent failure this project shipped
with until 2026-09-05 (every image carried 0).

Works on both a bare app image (``spaxel-firmware.bin``, app header at offset
0) and a merged flash image (``*-merged.bin``, bootloader first), because the
app image is located by its signature rather than at a fixed offset: a byte
0xE9 at the candidate offset plus the 0xabcd5432 magic word 0x20 bytes later.

Exit status 0 means the image matches ``--expect`` (or that no expectation
was given), 1 means it does not or could not be parsed — so it is safe to use
as a release gate.
"""

import argparse
import json
import struct
import sys

IMAGE_MAGIC = 0xE9
APP_DESC_MAGIC = 0xABCD5432
# esp_image_header_t is 24 bytes and the app descriptor is the first thing in
# the image's first segment, so its magic word sits 0x20 bytes into the image.
APP_DESC_OFFSET = 0x20
SCAN_STRIDE = 0x1000
SCAN_LIMIT = 0x400000


def find_app_offset(data, forced):
    """Return the offset of the app image header inside ``data``."""
    if forced is not None:
        return forced
    for off in range(0, min(len(data), SCAN_LIMIT), SCAN_STRIDE):
        if off + APP_DESC_OFFSET + 4 > len(data):
            break
        if data[off] != IMAGE_MAGIC:
            continue
        if struct.unpack_from("<I", data, off + APP_DESC_OFFSET)[0] != APP_DESC_MAGIC:
            continue
        return off
    return None


def parse_image(data, app_offset):
    header = data[app_offset:app_offset + 24]
    desc = data[app_offset + APP_DESC_OFFSET:app_offset + APP_DESC_OFFSET + 256]
    (
        _magic,
        segments,
        _spi_mode,
        _spi_speed,
        entry,
        _wp_pin,
        _pin_drv,
        chip_id,
        _min_rev,
        _min_full,
        _max_full,
        header_secure_version,
        _reserved,
        hash_appended,
    ) = struct.unpack("<BBBBIB3sHBHHB3sB", header)

    (
        desc_magic,
        desc_secure_version,
    ) = struct.unpack_from("<II", desc, 0)

    version = desc[16:48].split(b"\x00")[0].decode("utf-8", "replace")
    project = desc[48:80].split(b"\x00")[0].decode("utf-8", "replace")
    idf_ver = desc[112:144].split(b"\x00")[0].decode("utf-8", "replace")

    return {
        "app_offset": app_offset,
        "image_magic": IMAGE_MAGIC,
        "segments": segments,
        "chip_id": chip_id,
        "entry": entry,
        "header_secure_version": header_secure_version,
        "app_desc_secure_version": desc_secure_version,
        "hash_appended": bool(hash_appended),
        "app_version": version,
        "project_name": project,
        "idf_version": idf_ver,
        "desc_magic_ok": desc_magic == APP_DESC_MAGIC,
    }


def main():
    parser = argparse.ArgumentParser(
        description="Read the anti-rollback secure version out of a firmware image."
    )
    parser.add_argument("image", help="path to the .bin (bare or merged)")
    parser.add_argument(
        "--expect",
        type=int,
        help="fail unless the image carries this secure version",
    )
    parser.add_argument(
        "--app-offset",
        type=lambda v: int(v, 0),
        default=None,
        help="skip auto-detection and read the app header here (hex or decimal)",
    )
    parser.add_argument("--json", action="store_true", help="machine-readable output")
    args = parser.parse_args()

    try:
        with open(args.image, "rb") as handle:
            data = handle.read()
    except OSError as exc:
        print(f"cannot read {args.image}: {exc}", file=sys.stderr)
        return 1

    app_offset = find_app_offset(data, args.app_offset)
    if app_offset is None:
        print(f"no app image found in {args.image}", file=sys.stderr)
        return 1

    info = parse_image(data, app_offset)
    if not info["desc_magic_ok"]:
        print(f"app descriptor magic word missing at offset {app_offset:#x}", file=sys.stderr)
        return 1

    if info["header_secure_version"] != info["app_desc_secure_version"]:
        print(
            "secure version mismatch between image header "
            f"({info['header_secure_version']}) and app descriptor "
            f"({info['app_desc_secure_version']}) — the build is wiring one "
            "path but not the other",
            file=sys.stderr,
        )
        return 1

    if args.json:
        print(json.dumps(info, indent=2))
    else:
        print(f"image:               {args.image}")
        print(f"app image at:        {info['app_offset']:#x}")
        print(f"project:             {info['project_name']}")
        print(f"app version:         {info['app_version']}")
        print(f"idf version:         {info['idf_version']}")
        print(f"secure_version:      {info['header_secure_version']}")
        print(f"app_desc sec_ver:    {info['app_desc_secure_version']}")
        print(f"segments:            {info['segments']}")

    if args.expect is not None and info["header_secure_version"] != args.expect:
        print(
            f"expected secure_version {args.expect}, image carries "
            f"{info['header_secure_version']}",
            file=sys.stderr,
        )
        return 1

    return 0


if __name__ == "__main__":
    sys.exit(main())
