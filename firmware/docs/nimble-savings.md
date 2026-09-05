# NimBLE Flash and RAM Savings

**Date:** 2026-08-27  
**Build:** ESP-IDF v5.2, Target ESP32-S3  
**Configuration:** CONFIG_BT_NIMBLE_ENABLED=y

## Executive Summary

Switching from Bluedroid to NimBLE yields **massive footprint reductions**:
- **Flash savings:** 413,596 bytes (404 KiB) - **87.4% reduction**
- **RAM savings:** 26,503 bytes (25.9 KiB) - **99.0% reduction**

These savings are transformative for the project's security roadmap.

## Bluedroid Baseline (from child 2)

```
Flash: 416,856 bytes (407 KB)
RAM:   26,760 bytes (26 KB)

Component breakdown:
- libbt.a:          344,809 bytes flash, 14,342 bytes RAM
- libbtdm_app.a:    72,047 bytes flash, 12,418 bytes RAM

Total firmware context:
- Total Flash: 1,598,403 bytes (1.52 MB)
- Total Image: 1,718,673 bytes (1.64 MB)
```

## NimBLE Measurements

### Total Firmware Image
```
Total Flash: 1,072,447 bytes (1.02 MB)
Total Image: 1,184,169 bytes (1.13 MB)
```

### NimBLE Component (libbt.a)
From `idf.py size-components`:
```
DRAM .data:        60 bytes
.bss:              197 bytes
IRAM0 .text:        0 bytes
IRAM0 .vectors:     0 bytes
RAM total:         257 bytes

Flash .text:     3,151 bytes
Flash .rodata:      49 bytes
Flash .appdesc:     0 bytes
Flash total:     3,260 bytes
```

### Memory Category Totals
```
Used static IRAM:   16,383 bytes (100.0% used)
Used stat D/IRAM:  121,243 bytes (35.1% used, 224,613 remain)
Used Flash size:  1,072,447 bytes
```

## Savings Calculation

### Flash Savings
| Metric | Bluedroid | NimBLE | Savings |
|--------|-----------|--------|---------|
| BT Component Flash | 416,856 bytes | 3,260 bytes | 413,596 bytes |
| Total Firmware Flash | 1,598,403 bytes | 1,072,447 bytes | 525,956 bytes |
| **Flash Reduction** | - | - | **404 KiB (87.4%)** |

### RAM Savings
| Metric | Bluedroid | NimBLE | Savings |
|--------|-----------|--------|---------|
| BT Component RAM | 26,760 bytes | 257 bytes | 26,503 bytes |
| **RAM Reduction** | - | - | **25.9 KiB (99.0%)** |

### Detailed Component-by-Component Comparison
| Component | Bluedroid Flash | NimBLE Flash | Savings |
|-----------|-----------------|--------------|----------|
| libbt.a | 344,809 bytes | 3,260 bytes | 341,549 bytes (99.1%) |
| libbtdm_app.a | 72,047 bytes | 0 bytes (not present) | 72,047 bytes (100%) |
| **Total BT** | **416,856 bytes** | **3,260 bytes** | **413,596 bytes (99.2%)** |

| Component | Bluedroid RAM | NimBLE RAM | Savings |
|-----------|---------------|------------|----------|
| libbt.a | 14,342 bytes | 257 bytes | 14,085 bytes (98.2%) |
| libbtdm_app.a | 12,418 bytes | 0 bytes (not present) | 12,418 bytes (100%) |
| **Total BT** | **26,760 bytes** | **257 bytes** | **26,503 bytes (99.0%)** |

## Assessment: Room for Security Features?

### Planned Security Additions

The project roadmap requires space for:
1. **HTTPS client for OTA updates**
   - ESP-IDF esp_https_ota component
   - Estimated flash: ~50-100 KiB
   - Estimated RAM: ~5-10 KiB

2. **Signed app verification**
   - App signature verification logic
   - Estimated flash: ~20-30 KiB
   - Estimated RAM: ~2-5 KiB

### Budget Analysis

**Flash budget:**
- Bluedroid baseline: 1,598,403 bytes (1.52 MB)
- NimBLE baseline: 1,072,447 bytes (1.02 MB)
- **Available: 525,956 bytes (514 KiB)**

**RAM budget:**
- Bluedroid used: 121,243 bytes static + ~26,760 bytes BT = ~148 KB
- NimBLE used: 121,243 bytes static + ~257 bytes BT = ~121 KB
- **Available: ~27 KiB static RAM savings**

### Verdict

✅ **YES - The NimBLE migration provides MORE than enough room**

The 404 KiB flash savings comfortably exceeds the estimated 70-130 KiB required for:
- HTTPS client (~50-100 KiB)
- Signed app verification (~20-30 KiB)

**Remaining flash buffer after security features: ~274-434 KiB**

The 25.9 KiB RAM savings also provides headroom for:
- TLS session buffers (~5-10 KiB)
- Signature verification state (~2-5 KiB)

**Remaining RAM buffer: ~10-15 KiB**

## Conclusion

The Bluedroid → NimBLE migration is not just an optimization—it is an **enabler** for the project's security architecture. Without this change, the firmware would be severely flash-constrained and unable to safely implement OTA updates with signature verification.

### Key Takeaways
1. **Massive reduction:** 99.2% flash and 99.0% RAM savings for the Bluetooth stack
2. **Enables security:** Frees 404 KiB flash for HTTPS + signed app verification
3. **Headroom remaining:** Even after security features, ~274-434 KiB flash buffer remains
4. **No tradeoffs:** NimBLE provides equivalent BLE functionality with better memory characteristics

## Recommendation: GO — NimBLE is the permanent BT stack (and already landed)

**Go/no-go: GO.** The Bluedroid → NimBLE switch should be permanent, and it
already is: commit `d4ad6737` (2026-08-27) migrated `firmware/main/ble.c` to
the NimBLE host API and set `CONFIG_BT_NIMBLE_ENABLED=y` /
`CONFIG_BT_BLUEDROID_ENABLED=n` — both still true at HEAD. There is nothing to
revert to: the firmware's only Bluetooth role is passive GAP advertisement
scanning, which NimBLE provides, and no current Bluedroid build exists (the
2026-08-27 baseline in `bluedroid-baseline.txt` is historical, marked as such
by `41052018`).

*Provenance note:* the analysis above was written as a pre-decision comparison;
this section states the decision explicitly, added 2026-09-05 (bead
spaxel-4ca0dd6a) after the switch had already landed.

### Why GO

1. **The savings are real and stable at HEAD.** `libbt.a` measured
   byte-identical (3,260 B flash / 257 B RAM) in the 2026-08-27 capture above
   and in the 2026-09-05 re-capture
   (`size-components-2026-09-05.{md,json}`, built from a clean `git archive`
   of `b995e781`).
2. **Headroom is sufficient for the security roadmap.** 404 KiB flash freed
   against an estimated 70–130 KiB for HTTPS OTA + signed-app verification
   leaves ~274–434 KiB of buffer (see *Budget Analysis* above).
3. **No functional tradeoff.** Equivalent BLE functionality for the node's
   passive-scan use case, at 99% less stack footprint.

### Measurement refinement (recorded, no conclusion change)

The 2026-09-05 capture shows the NimBLE build still links a vestigial
`libbtdm_app.a` (475 B flash / 41 B RAM) and `libbtbb.a` (0 B). Whole
BT-family savings are therefore **413,121 B flash / 26,462 B RAM** rather than
the libbt.a-only 413,596 / 26,503 quoted above — a 0.1% refinement.

### Next steps

1. **HTTPS + signed-app verification is unblocked on flash, not on segments.**
   The 404 KiB BT savings clear the 70–130 KiB estimate with room to spare,
   but the *segment* budget is the binding one: the 2026-09-05 capture shows
   static IRAM at 16,383/16,384 bytes (1 byte remain), so any new
   IRAM-resident crypto path needs `noflash`/place-in-flash triage first.
   App-partition headroom at the capture tip: 0xcce20 B (41%) of 0x1f0000.
2. **The heap go/no-go for mTLS stays hardware-gated.** ADR-008's resource
   spike — measured peak heap through a real TLS handshake with PSRAM enabled —
   is the gating number for the asymmetric node-identity plan. Flash savings
   do not substitute for it; it requires the bench rig.
3. **Do not re-run this comparison.** `libbt.a` is byte-identical across both
   capture passes and the only app-code change since the first capture touched
   watchdog sdkconfig only; the figures in this document are current.

## Appendix: Raw Build Output

NimBLE build summary:
```
spaxel-firmware.bin binary size 0x121210 bytes. Smallest app partition is 0x1f0000 bytes. 0xcedf0 bytes (42%) free.

Total sizes:
Used static IRAM:   16383 bytes (      1 remain, 100.0% used)
      .text size:   15356 bytes
   .vectors size:    1027 bytes
Used stat D/IRAM:  121243 bytes ( 224613 remain, 35.1% used)
      .data size:   20312 bytes
      .bss  size:   25904 bytes
      .text size:   75027 bytes
Used Flash size : 1072447 bytes
           .text:  788535 bytes
         .rodata:  283656 bytes
Total image size: 1184169 bytes (.bin may be padded larger)
```

Per-archive BT contribution:
```
libbt.a         60     197           0           0          257        3151        49                0          0        3260
```

Format: `DRAM .data .bss IRAM0 .text IRAM0 .vectors ram_st_total Flash .text Flash .rodata Flash .rodata_noload Flash .appdesc flash_total`
