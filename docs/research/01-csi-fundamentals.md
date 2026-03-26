# WiFi CSI Fundamentals

## What CSI Is

Channel State Information (CSI) describes the propagation process of a wireless signal at the physical layer, with per-subcarrier granularity. It characterises how a signal is attenuated, phase-shifted, and dispersed by the environment as it travels from transmitter to receiver.

CSI is extracted from OFDM (Orthogonal Frequency Division Multiplexing) frames during the Long Training Field (LTF) preamble, which the receiver uses for channel equalisation. This same channel estimate — normally discarded after equalisation — is exposed via vendor APIs.

Mathematically, CSI is the Channel Frequency Response (CFR), expressed as a vector of complex numbers across subcarriers:

```
H = { H(f_j) | j ∈ [1, J] }
H(f_j) = |H(f_j)| · e^(j∠H(f_j))
```

where `|H(f_j)|` is amplitude and `∠H(f_j)` is phase for subcarrier j.

---

## CSI vs. RSSI

| Property | RSSI | CSI |
|---|---|---|
| Granularity | Single scalar per packet | Complex vector (one value per subcarrier) |
| Frequency content | Aggregate bandwidth-averaged power | Per-subcarrier amplitude and phase |
| Multipath sensitivity | Cannot distinguish multipath components | Each subcarrier encodes a different multipath mix |
| Phase information | None | Full phase per subcarrier |
| Noise resilience | Single measurement; unreliable if faded | Other subcarriers remain informative when some fade |
| Temporal stability | Low (random fading) | Higher (multipath structure is more stable) |

RSSI is the sum of power across all subcarriers, discarding all frequency-selective fading information. CSI exposes how amplitude and phase vary across frequency — this frequency-domain signature encodes geometry.

---

## 802.11n / HT20 Subcarrier Structure

Under 802.11n (Wi-Fi 4) with HT20 (20 MHz channel):

| Parameter | Value |
|---|---|
| Total OFDM subcarriers | 64 (indices −32 to +31) |
| Data subcarriers | 52 |
| Pilot subcarriers | 4 (at indices −21, −7, +7, +21) |
| Null/guard subcarriers | 8 (DC + edge guards) |
| Subcarrier spacing | 312.5 kHz |
| Symbol duration | 3.2 µs + 0.8 µs guard interval |

For HT40 (40 MHz): 114 usable subcarriers (108 data + 6 pilots).

---

## ESP32-S3 CSI API

### Key Calls

```c
// 1. Enable in menuconfig: Component Config → Wi-Fi → Wi-Fi CSI
// 2. Register callback
esp_wifi_set_csi_rx_cb(my_csi_callback, NULL);
// 3. Configure which LTF fields to capture
wifi_csi_config_t csi_cfg = {
    .lltf_en          = true,   // Legacy LTF (all modes)
    .htltf_en         = true,   // HT LTF (802.11n)
    .stbc_htltf2_en   = true,   // STBC HT LTF
    .channel_filter_en = false, // disable for independent subcarriers
    .manu_scale       = false,
    .shift            = 0,
};
esp_wifi_set_csi_config(&csi_cfg);
// 4. Enable
esp_wifi_set_csi(true);
```

### wifi_csi_info_t Fields

| Field | Type | Description |
|---|---|---|
| `rx_ctrl` | struct | RSSI, noise floor, channel BW, signal mode, STBC flag, timestamp, antenna index |
| `mac` | uint8[6] | Sender MAC address |
| `len` | uint16 | Byte count of CSI buffer |
| `buf` | int8* | Raw CSI: (Im, Re) int8 pairs per subcarrier |
| `first_word_invalid` | bool | First 4 bytes may be invalid (hardware bug on original ESP32) |

### CSI Data Format

Each subcarrier is encoded as two consecutive signed 8-bit integers:
- `buf[2k]` = imaginary part (Im) for subcarrier k
- `buf[2k+1]` = real part (Re) for subcarrier k

Order: LLTF subcarriers, then HT-LTF, then STBC-HT-LTF (whichever are enabled).

### Subcarrier Counts and Byte Totals

| Packet type | Channel | LTF fields enabled | Subcarriers | Bytes |
|---|---|---|---|---|
| Legacy (non-HT) | HT20 | LLTF only | 64 | 128 |
| HT20 | 20 MHz | LLTF + HT-LTF | 128 | 256 |
| HT40 | 40 MHz | LLTF + HT-LTF | 192 | 384 |
| HT40 w/ STBC | 40 MHz | All 3 LTFs | 320+ | 640+ |

### Packet Rate

CSI is event-driven — one sample per received WiFi packet:

| Mode | Practical rate |
|---|---|
| Passive beacon sniffing | ~10 Hz (beacon interval = 100 ms) |
| Active ICMP ping loop | 50–100 Hz |
| Typical sensing deployments | 20–22 Hz |

---

## ESP32-S3 vs. Other Hardware

| Tool | Chip | Subcarriers (HT20) | Phase access | Cost |
|---|---|---|---|---|
| Intel 5300 CSI Tool | IWL5300 | 30 groups | Yes | ~$50 NIC + Linux host |
| Atheros CSI Tool | AR9390/AR9580 | 56 | Yes | Moderate |
| Nexmon CSI | BCM4339/BCM4366 | 256 | Yes | RPi / Android |
| **ESP32-S3** | ESP32-S3 | 64 (LLTF+HT-LTF) | Yes | ~$5–10 per node |
| ESPARGOS | 8× ESP32 array | Phase-coherent 8-antenna | Coherent | Research device |

ESP32 advantages: more subcarriers than Intel 5300 per sample, standalone embedded operation, no host PC required, ~$5 per node.

ESP32 disadvantages: single antenna (no spatial MIMO diversity), lower phase stability, int8 limits dynamic range, no Linux userspace driver ecosystem.

**ESPARGOS** (University of Stuttgart) is an 8-antenna phase-coherent 2×4 array of ESP32 chips sharing a single 40 MHz reference clock. It supports MUSIC-based AoA estimation and is designed for research requiring true array processing.

### Quality Ranking (Espressif's own)

> ESP32-C5 > ESP32-C6 > ESP32-C3 ≈ ESP32-S3 > ESP32 (original)
