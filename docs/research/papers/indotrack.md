# IndoTrack: Device-Free Indoor Human Tracking with Commodity Wi-Fi

**Authors:** Xiang Li, Daqing Zhang, Qin Lv, Jie Xiong, Shengjie Li, Yue Zhang, Hong Mei
**Venue:** ACM IMWUT (UbiComp) 2017, Vol. 1, No. 3
**DOI:** [10.1145/3130940](https://doi.org/10.1145/3130940)
**Institution:** Peking University / University of Massachusetts Amherst

---

## Citation

```
@article{li2017indotrack,
  title     = {IndoTrack: Device-Free Indoor Human Tracking with Commodity Wi-Fi},
  author    = {Li, Xiang and Zhang, Daqing and Lv, Qin and Xiong, Jie and Li, Shengjie and Zhang, Yue and Mei, Hong},
  journal   = {Proc. ACM Interact. Mob. Wearable Ubiquitous Technol.},
  volume    = {1},
  number    = {3},
  year      = {2017},
  pages     = {1--22},
  doi       = {10.1145/3130940},
  publisher = {ACM}
}
```

---

## Abstract

IndoTrack is a device-free indoor human tracking system using commodity Wi-Fi. It addresses the fundamental challenge that device-free tracking relies on reflection signals orders of magnitude weaker than direct signals, while commodity Wi-Fi has limited antenna count, small bandwidth, and significant hardware noise.

Two core innovations:
1. **Doppler-MUSIC**: Extracts accurate Doppler velocity information from noisy CSI via a super-resolution algorithm applied to the Doppler frequency domain
2. **Doppler-AoA**: Determines absolute trajectory by jointly estimating target velocity and location via probabilistic co-modelling of spatial-temporal Doppler and AoA information

---

## Problem Statement

Device-free tracking — localising targets without body-worn hardware — is essential for applications where instrumentation is impractical: eldercare, intruder detection, infant monitoring. The challenge:

- Reflected signal strength is orders of magnitude weaker than direct signal
- Human body Doppler shifts are small: at 2.4 GHz, walking at 1 m/s → 16 Hz Doppler
- Commodity Wi-Fi has only 3 antennas and 30 CSI subgroups (Intel 5300)
- Hardware noise dominates the weak reflection signal
- Prior tracking systems require ≥3 dedicated links or specialised hardware

---

## Methodology

### Phase Noise Removal

Uses conjugate multiplication between antenna pairs (same technique as Widar2.0) to cancel CFO and hardware phase noise:
```
C(m) = H̃(m) × H̃*(m₀)
```
This preserves AoA and DFS structure in the product while cancelling common-mode hardware errors.

### Doppler-MUSIC

Extends MUSIC from the spatial domain to the **Doppler frequency domain**:

1. Construct a Doppler covariance matrix from CSI time series (analogous to the spatial covariance matrix in classic AoA-MUSIC)
2. Eigendecompose into signal and noise subspaces
3. Doppler pseudo-spectrum: `P_D(f) = 1 / (d(f)^H · U_n · U_n^H · d(f))`
   where `d(f)` is the Doppler steering vector
4. Peaks of `P_D(f)` = Doppler frequencies of individual reflectors (torso, arms, legs)

Super-resolution in frequency domain: resolves Doppler components separated by less than the FFT frequency resolution limit. This is the key improvement over standard STFT-based DFS extraction.

### Doppler-AoA Joint Estimation

The Doppler shift of a reflected path depends on both the target velocity and the geometry (AoA from TX and RX). For a target at angle θ_t (from TX) and θ_r (from RX), moving at velocity v:

```
f_D = (v/λ) · (cos(θ_t) + cos(θ_r))
```

This creates a coupling between Doppler and AoA — a unique velocity vector produces a specific Doppler signature *per link*, and that Doppler signature depends on the AoA. IndoTrack exploits this coupling by:

1. Estimating Doppler shifts across multiple links (TX-RX pairs)
2. Probabilistically combining per-link Doppler-AoA joint distributions
3. Finding the (position, velocity) pair that is most consistent with all observations

### Trajectory Reconstruction

Starting from the estimated position-velocity state, IndoTrack integrates velocity over time to produce a trajectory estimate. A particle filter propagates and reweights trajectory hypotheses based on new Doppler-AoA observations.

### Hardware Setup

- 1 transmitter (802.11n AP) + 2 receiver laptops
- Each receiver: 3 antennas → 3 links from one transmitter
- Total: 3 links used for Doppler-AoA fusion
- Intel 5300 NIC for CSI extraction
- Packet injection rate: ~1000 packets/sec

---

## Results

| Metric | Value |
|---|---|
| Median trajectory error | **35 cm** |
| 80th percentile error | ~70 cm |
| Tracking range | 6 m |
| Number of links required | 3 |

Outperforms Widar v1 (38 cm, but requires initial position). Does not require initial position. Tested in office and lab environments.

---

## Limitations

- Requires 3 Wi-Fi links (1 Tx, 2 Rx)
- Single-target scenario primarily evaluated
- Accuracy degrades when target walks along the LoS path between TX and RX (minimal Doppler observed)
- 2D tracking only
- Doppler-MUSIC performance degrades when multiple people are present simultaneously

---

## Relevance to Spaxel

IndoTrack's Doppler-MUSIC provides the best commodity-hardware accuracy for moving target tracking. For Spaxel, the key takeaway is that 3 well-placed nodes give ~35 cm accuracy for a single moving person — this sets the performance ceiling for the geometric Fresnel zone approach and is a target for future enhancement. The particle filter trajectory reconstruction is directly applicable to Spaxel's Kalman smoother.
