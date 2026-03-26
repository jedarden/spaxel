# Tool Release: Gathering 802.11n Traces with Channel State Information

**Authors:** Daniel Halperin, Wenjun Hu, Anmol Sheth, David Wetherall
**Venue:** ACM SIGCOMM Computer Communication Review, Vol. 41, No. 1, January 2011
**DOI:** [10.1145/1925861.1925870](https://doi.org/10.1145/1925861.1925870)
**PDF:** https://www.halper.in/pubs/halperin_csitool.pdf
**Tool:** https://dhalperi.github.io/linux-80211n-csitool/
**Institution:** University of Washington / Intel Labs Seattle

---

## Citation

```
@article{halperin2011tool,
  title   = {Tool Release: Gathering 802.11n Traces with Channel State Information},
  author  = {Halperin, Daniel and Hu, Wenjun and Sheth, Anmol and Wetherall, David},
  journal = {ACM SIGCOMM Computer Communication Review},
  volume  = {41},
  number  = {1},
  year    = {2011},
  pages   = {53--53},
  doi     = {10.1145/1925861.1925870}
}
```

---

## Description

This tool release paper announces the availability of a platform for recording detailed channel measurements alongside received 802.11n packet traces, running on commodity 802.11n network interface cards with modified firmware. It defines the de facto standard CSI data format used by virtually all subsequent WiFi sensing research.

---

## Technical Specifications

### Hardware

**Intel WiFi Link 5300 (IWL5300)**:
- 3 antenna ports
- 802.11n capable (2.4 GHz and 5 GHz)
- HT20 and HT40 support
- Required hardware for this tool

### CSI Output Format

Channel matrices for **30 subcarrier groups** (approximately 1 group per 2 subcarriers at 20 MHz, 1 per 4 at 40 MHz).

Each matrix entry is a complex number with **signed 8-bit resolution** (int8) for real and imaginary parts, specifying the gain and phase of the signal path between one TX-RX antenna pair.

**Output matrix dimensions:** A × 30, where A = number of active antenna pairs (maximum 3 × 3 = 9 for 3 TX × 3 RX; typically 1 TX × 3 RX = 3 entries per subcarrier group).

### Data Format Detail

Per-packet CSI record:
```
timestamp_low    : 4 bytes  (low 32 bits of NIC timestamp, 1 µs units)
bfee_count       : 2 bytes  (number of beamforming measurements)
Nrx              : 1 byte   (number of receive antennas in use, ≤3)
Ntx              : 1 byte   (number of transmit antennas used, ≤3)
rssi_a/b/c       : 3 bytes  (RSSI measured at each receive antenna, dBm)
noise            : 1 byte   (noise floor, dBm)
agc              : 1 byte   (automatic gain control setting)
antenna_sel      : 1 byte   (which antennas used for Tx/Rx)
len              : 2 bytes  (payload length including this header)
rate             : 2 bytes  (rate at which packet was received)
payload          : 60×Nrx×Ntx bytes  (30 groups × Nrx × Ntx × 2 bytes per complex value)
```

**Complex value encoding:** Each (real, imaginary) pair encoded as two int8 values = 2 bytes. 30 groups × up to 9 antenna pairs × 2 bytes = 540 bytes maximum.

### Software Stack

- Modified Intel closed-source firmware (exposes CSI to driver)
- Open-source `iwlwifi` Linux driver with Intel-provided patch
- Userspace collection tools (`log_to_file`, UDP streaming)
- MATLAB/Octave parsing scripts (`read_bf_file.m`)
- OS: Ubuntu 10.04 LTS, kernel 2.6.36 (original release)

### CSI Rate

One measurement per received 802.11 frame. Achieved rates:
- Passive (monitor mode): rate depends on ambient traffic
- Active (packet injection): up to ~2500 samples/sec with 0.4 ms injection interval

---

## Significance

This tool became the **de facto standard for WiFi CSI research for approximately a decade**. The papers listed below all used this tool or were directly inspired by it:

| Paper | Year | Uses Intel 5300 |
|---|---|---|
| FILA (Wu et al.) | 2012 | Yes |
| WiSee (Pu et al.) | 2013 | No (USRP) |
| SpotFi (Kotaru et al.) | 2015 | Yes |
| CARM (Wang et al.) | 2015 | Yes |
| Widar (Qian et al.) | 2017 | Yes |
| IndoTrack (Li et al.) | 2017 | Yes |
| Widar2.0 (Qian et al.) | 2018 | Yes |

---

## Related Publications from the Same Group

1. **Halperin et al. (2010).** "Predictable 802.11 packet delivery from wireless channel measurements." *ACM SIGCOMM 2010.* The application paper motivating CSI exposure — used CSI to predict packet delivery rates, showing it is more informative than RSSI.

2. **Halperin et al. (2010).** "802.11 with multiple antennas for dummies." *ACM SIGCOMM CCR 2010.* Tutorial introduction to MIMO, beamforming, and spatial multiplexing in 802.11n.

3. **Halperin et al. (2010).** "Investigation into the Doppler component of the IEEE 802.11n channel model." *IEEE GLOBECOM 2010.*

4. **Halperin et al. (2012).** "ParCast: Soft video delivery in MIMO-OFDM WLANs." *ACM MobiCom 2012.*

---

## Limitations

- Only 30 subcarrier groups (not full 64-subcarrier resolution of 802.11n HT20)
- 8-bit quantisation limits dynamic range to 48 dB
- Requires specific Intel 5300 hardware (discontinued, scarce post-2015)
- Maximum 3 antennas limits spatial resolution
- Linux driver support ended; modern distributions require significant patching
- No phase calibration — raw phase contains hardware-specific offsets requiring sanitisation
- Closed firmware is proprietary; not portable to other hardware

---

## Comparison to ESP32-S3

| Property | Intel 5300 CSI Tool | ESP32-S3 |
|---|---|---|
| Subcarriers | 30 groups | 64 (LLTF + HT-LTF) |
| Resolution | int8 | int8 |
| Antennas | Up to 3 | 1 (built-in) |
| Phase access | Yes | Yes |
| Cost | ~$50 NIC + Linux host | ~$5–10 per node |
| Standalone | No (requires PC) | Yes (embedded) |
| Production status | Discontinued | Current |
| Phase coherence across nodes | No | No |

The ESP32-S3 provides **more subcarriers** than the Intel 5300 per sample and operates as a standalone embedded node — the key reasons it is preferred for Spaxel over academic-grade hardware.
