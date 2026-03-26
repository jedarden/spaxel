# FILA: Fine-Grained Indoor Localization

**Authors:** Kaishun Wu, Jiang Xiao, Youwen Yi, Min Gao, Lionel M. Ni
**Venue:** IEEE INFOCOM 2012
**DOI:** [10.1109/INFCOM.2012.6195606](https://doi.org/10.1109/INFCOM.2012.6195606)
**Institution:** Hong Kong University of Science and Technology

---

## Citation

```
@inproceedings{wu2012fila,
  title     = {FILA: Fine-grained indoor localization},
  author    = {Wu, Kaishun and Xiao, Jiang and Yi, Youwen and Gao, Min and Ni, Lionel M.},
  booktitle = {Proceedings of the 31st Annual IEEE International Conference on Computer Communications},
  series    = {INFOCOM '12},
  year      = {2012},
  pages     = {2210--2218},
  doi       = {10.1109/INFCOM.2012.6195606},
  publisher = {IEEE}
}
```

---

## Abstract

> "Indoor positioning systems have received increasing attention for supporting location-based services in indoor environments. WiFi-based indoor localization has been attractive due to its open access and low cost properties. However, the distance estimation based on received signal strength indicator (RSSI) is easily affected by the temporal and spatial variance due to the multipath effect, which contributes to most of the estimation errors in current systems. In this paper, we propose FILA, a Fine-grained Indoor Localization Algorithm, which leverages the Channel State Information (CSI) from OFDM systems to alleviate the impact of multipath effect via frequency diversity."

---

## Problem Statement

RSSI-based indoor localisation is limited by multipath fading: RSSI variance in static environments can be up to **5 dB in one minute**, yielding 2–4 m typical accuracy. Standard fingerprinting systems (Horus, RADAR) inherit this instability.

FILA is **claimed as the first system to apply CSI to indoor localisation**. By exploiting frequency-selective fading across OFDM subcarriers, CSI provides a far more stable distance metric than RSSI.

---

## Key Concept: Frequency Diversity

Multipath causes frequency-selective fading in OFDM channels. Different subcarriers experience different fading states. Averaging across subcarriers reduces variance because:
- When subcarrier k is in a deep fade (low amplitude), adjacent subcarriers k±1 likely are not
- The aggregate across 30 subcarriers is far more stable than any individual subcarrier or the RSSI aggregate

This is the core advantage of CSI over RSSI: **CSI exposes the fading structure; RSSI averages over it**.

---

## Methodology

### Effective CSI Computation

Aggregate across subcarriers to produce a stable scalar distance metric:

```
CSI_eff = (1/K) · Σ_{k ∈ (−15, 15)} (f_k / f_0 × |H_k|)
```

Where:
- K = number of subcarriers (30 for Intel 5300)
- `f_k / f_0` = frequency ratio for normalisation across subcarriers
- `|H_k|` = CSI amplitude at subcarrier k

### Refined Propagation Model

Distance estimation from `CSI_eff`:

```
d = (1/4π) × (c / (f_0 × |CSI_eff|))^(1/n) × σ
```

Where:
- n = path loss exponent (environment-specific, 2–4 for indoor)
- σ = environment calibration constant
- c = speed of light
- f_0 = centre frequency

### Two Localisation Approaches

1. **Model-based trilateration:** Use the above distance formula from ≥3 APs; trilaterate to get 2D position. Achieves sub-metre accuracy in favourable environments.

2. **CSI fingerprinting:** Use the full per-subcarrier CSI amplitude vector as a fingerprint. k-NN matching against site-surveyed database. More robust to heterogeneous environments.

---

## Results

| Environment | Approach | Accuracy |
|---|---|---|
| Research laboratory | Model + trilateration | 0.45 m median |
| Lecture theatre | Fingerprinting | 1.8 m (90th percentile) |
| Multi-room corridor | Fingerprinting | 1.2 m median |

- ~25% improvement over Horus RSSI fingerprinting
- Distance estimation: ~3× more accurate than RSSI-based scheme
- System latency: **~0.01 s** (vs. 2–3 s for RSSI tracking)

---

## Limitations

- Sub-metre accuracy only achievable in favourable environments
- Propagation model requires per-environment calibration of n and σ
- Limited to 30 CSI subcarrier groups from Intel 5300
- Fingerprinting approach still requires a site survey
- Multi-person scenarios not addressed

---

## Relevance to Spaxel

FILA establishes that CSI amplitude aggregated across subcarriers provides a significantly more stable distance metric than RSSI. The `CSI_eff` formulation is directly applicable to Spaxel's per-link signal delta computation: weighting subcarriers by their frequency ratio reduces edge-subcarrier bias in the amplitude measurement. FILA's 3× distance accuracy improvement over RSSI validates the choice of CSI over simpler signal metrics.
