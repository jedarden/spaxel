# Realistic Accuracy Bounds and Limitations

## Localization Accuracy by System Type

| System type | Typical accuracy | Conditions |
|---|---|---|
| RSSI fingerprinting | 2–5 m | Calibrated environment |
| CSI fingerprinting | 0.5–2 m | Controlled environment |
| CSI model-based (geometric) | 0.5–1.5 m | Good geometry, single moving person |
| SpotFi (MUSIC + subcarrier extension) | ~40 cm median | 3-antenna AP, calibrated |
| IndoTrack (Doppler-MUSIC) | ~35 cm median | 3 antennas, commodity hardware |
| Widar2.0 (single link) | ~75 cm median | Moving target |
| Widar2.0 (two links) | ~63 cm median | Moving target |
| Multi-link geometric (4–8 nodes) | 0.5–1.0 m | Typical indoor room |

### 3D (Z-axis) Accuracy

- Z-axis accuracy is typically worse than XY: **1–3 m** with geometric approaches
- Machine learning approaches have achieved 0.4 m vertical RMSE in some studies
- Fundamental limit from wavelength: 2.4 GHz → λ/4 = 3.1 cm minimum theoretical resolution per dimension (unreachable with commodity hardware noise)
- Z accuracy improves significantly with nodes at mixed heights

---

## The Stillness Problem

This is the most significant practical limitation. When a person is motionless:

- **No Doppler shift** → DFS-based methods (Widar, IndoTrack) fail entirely
- **EMA baseline adapts** → person gradually absorbed into background; presence fades
- **Variance-based detectors** see near-zero variance → interpret as empty room

### Mitigations

| Mitigation | Mechanism | Limitation |
|---|---|---|
| Long EMA time constant (α = 0.999) | Slows background adaptation | Eventually still absorbs stationary person |
| Motion-gated baseline update | Only update when variance < threshold | Requires quiet periods to capture baseline |
| Breathing detection (0.1–0.5 Hz) | Chest movement ~5 mm still detectable | Needs 10–30 s averaging; fails with noisy environment |
| Entry/exit hysteresis | Require exit event before marking absent | Doesn't detect initial entry of already-still person |
| Static CSI fingerprint comparison | Empty vs. occupied room differ by 3–12 dB on affected links | Requires clean empty-room reference |

**Breathing at 2.4 GHz**: chest displacement ~5 mm → phase change ≈ 2π × 2 × 0.005 / 0.125 ≈ **0.5 rad**. Detectable above noise with sufficient averaging.

---

## Multiple Person Degradation

From WiMANS dataset and related work:

| People | Accuracy degradation |
|---|---|
| 1 | Baseline (e.g. 35 cm for IndoTrack) |
| 2 | ~2× increase in error; ambiguity from two reflection sources |
| 3 | ~3–4× error increase |
| 5 | Localization error +15.4%, activity recognition −25.7% vs. single person |

Multi-person tracking is an open research problem. Current approaches:
- Track as many simultaneous targets as there are resolvable Doppler sources (limited by angular/frequency resolution)
- Use multiple antennas with MUSIC to spatially separate sources
- Deep learning end-to-end approaches show promise but require large training datasets

---

## Environmental Sensitivity

| Change | Effect | Recovery |
|---|---|---|
| Furniture moved | Static multipath changes → fingerprint invalid | Forced baseline reset + slow EMA re-adaptation |
| Temperature change (AC on/off) | Slight material property and phase shifts | Slow EMA absorbs over hours |
| High-activity elsewhere in building | Background dynamic CSI from other rooms | Narrow sensing band; spatial filtering |
| Humidity / rain | Affects building material dielectric properties | Slow EMA absorbs |
| New large objects added | New permanent multipath components | Forced baseline reset |

---

## ESP32-Specific Limitations

| Limitation | Impact | Workaround |
|---|---|---|
| Single antenna | No direct AoA from one node | Multi-node mesh provides angular diversity |
| int8 dynamic range | Saturates near nodes; low SNR far from nodes | Place nodes at 2–5 m from sensing area |
| No phase coherence across nodes | Cannot directly apply array MUSIC to multi-node data | Use geometric Fresnel method instead |
| No 5 GHz | Limited to 2.4 GHz (12.5 cm wavelength) | Adequate for body-scale detection |
| ToF resolution at 20 MHz | c/(2B) = 7.5 m — useless for ranging | Manual node position measurement |
| Packet rate ~20 Hz | Limits DFS resolution to ±10 Hz | Adequate for walking (16 Hz DFS) |

---

## What Spaxel Can Realistically Achieve

### Conservative (safe to claim)

- **Presence detection** (someone in the room vs. empty): reliable with 2+ nodes on opposite sides
- **Approximate 2D position** (±0.5–1.0 m): reliable with 4+ well-placed nodes for a moving person
- **Motion detection and tracking**: reliable with 4+ nodes
- **Rough count** (0 vs. 1 vs. 2+ people): works in practice, degrades with 3+

### Possible with Good Conditions

- **Rough 3D position** (±1–2 m Z): with nodes at mixed heights
- **Stationary person detection**: via breathing detection on stable setup
- **Velocity estimation**: via DFS analysis

### Not Achievable with Commodity ESP32-S3

- Sub-10 cm accuracy
- Reliable skeletal pose estimation
- Fine-grained limb position
- Reliable tracking of 5+ simultaneous people
- Through-floor sensing (different frequency needed)
