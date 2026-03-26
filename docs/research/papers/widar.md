# Widar: Decimeter-Level Passive Tracking via Velocity Monitoring with Commodity Wi-Fi

**Authors:** Kun Qian, Chenshu Wu, Zheng Yang, Yunhao Liu, Kyle Jamieson
**Venue:** ACM MobiHoc 2017
**DOI:** [10.1145/3084041.3084067](https://doi.org/10.1145/3084041.3084067)
**Institution:** Tsinghua University (TNS Lab) / Princeton University

---

## Citation

```
@inproceedings{qian2017widar,
  title     = {Widar: Decimeter-Level Passive Tracking via Velocity Monitoring with Commodity Wi-Fi},
  author    = {Qian, Kun and Wu, Chenshu and Yang, Zheng and Liu, Yunhao and Jamieson, Kyle},
  booktitle = {Proceedings of the 18th ACM International Symposium on Mobile Ad Hoc Networking and Computing},
  series    = {MobiHoc '17},
  year      = {2017},
  doi       = {10.1145/3084041.3084067},
  publisher = {ACM}
}
```

---

## Abstract

> "Various pioneering approaches have been proposed for Wi-Fi-based sensing, which usually employ learning-based techniques to seek appropriate statistical features, yet do not support precise tracking without prior training. Thus to advance passive sensing, the ability to track fine-grained human mobility information acts as a key enabler. In this paper, we propose Widar, a Wi-Fi-based tracking system that simultaneously estimates a human's moving velocity (both speed and direction) and location at a decimeter level. Instead of applying statistical learning techniques, Widar builds a theoretical model that geometrically quantifies the relationships between CSI dynamics and the user's location and velocity. On this basis, we propose novel techniques to identify frequency components related to human motion from noisy CSI readings and then derive a user's location in addition to velocity. We implement Widar on commercial Wi-Fi devices and validate its performance in real environments. Our results show that Widar achieves decimeter-level accuracy, with a median location error of 25 cm given initial positions and 38 cm without them and a median relative velocity error of 13%."

---

## Problem Statement

Pioneer WiFi sensing systems use statistical learning to recognise pre-defined activities and gestures but require prior training and cannot track continuous fine-grained human mobility. No system provided a physics-based model directly mapping CSI measurements to instantaneous velocity and location without environment-specific training. Widar bridges this gap.

---

## Core Signal Model

Channel model:
```
H(f,t) = Σ_{k=1}^{K} α_k(t) · e^{−j2πf·τ_k(t)}    (Eq. 1)
```

**Path Length Change Rate (PLCR):** Doppler frequency shift of reflected signal:
```
f_D(t) = −(1/λ) · d/dt[d(t)] = −f · d/dt[τ(t)]    (Eq. 2)
```
PLCR `r ≜ d/dt[d(t)]` — the rate of change of path length in metres/second.

CSI with static and dynamic paths:
```
H(f,t) = (H_s(f) + Σ_{k∈P_d} α_k(t) · e^{j2π∫f_{Dk}(u)du}) · e^{−jκ(f,t)}    (Eq. 3)
```

---

## CSI-Mobility Model (Geometric)

For transmitter `l_t^(i)`, receiver `l_r^(i)`, target position `l_h = (x_h, y_h)^T`, velocity `v = (v_x, v_y)^T`:

```
a_x^(i) · v_x + a_y^(i) · v_y = r^(i)    (Eq. 4)
```

Where:
```
a_x^(i) = (x_h − x_t^(i))/||l_h − l_t^(i)|| + (x_h − x_r^(i))/||l_h − l_r^(i)||
a_y^(i) = (y_h − y_t^(i))/||l_h − l_t^(i)|| + (y_h − y_r^(i))/||l_h − l_r^(i)||
```

Physical interpretation: `a_x^(i)` and `a_y^(i)` are sums of unit vector x-components from target toward transmitter and receiver. Only the **radial velocity** component (toward/away from each link's foci) contributes to PLCR.

Aggregating L links:
```
A · v = r    (Eq. 6)
```

Least-squares velocity estimate:
```
v_opt = (A^T·A)^{−1} · A^T · r    (Eq. 7)
```

Target velocity is **uniquely determined** from ≥2 links with non-parallel radial directions.

---

## PLCR Extraction Pipeline

1. **Bandpass filter** (2–80 Hz Butterworth) on CSI power `|H(f,t)|²`
2. **Subcarrier selection:** Choose 20 subcarriers with highest correlation increase during motion vs. static; PCA on selected subcarriers → first principal component captures motion signal
3. **STFT** with 0.125 s Gaussian window, 0.5 s zero-padding → 1 Hz frequency resolution spectrogram
4. **Dynamic programming** to extract PLCR curve from spectrogram, enforcing physical acceleration constraint

---

## Direction Ambiguity Resolution

Without the sign of PLCR, 2L possible velocity combinations exist. Two methods:

**WiDir integration:** Uses subcarrier time-lag patterns to infer sign. When target moves toward link, larger-wavelength subcarriers experience constructive variations earlier → positive time lag. Sign inferred from majority vote of time lags across subcarrier pairs.

**Continuity constraint:** Consecutive velocities must be similar (human acceleration bounded). Error function:
```
s_{k,opt} = argmin_{s_k ∈ {−1,+1}^L} (err_{l,k} + β · err_{v,k})
err_{l,k} = ||A_k · v_{k,opt} − R_k · s_k||
err_{v,k} = ||A_k · v_{k,opt} − A_{k−1} · v_{k−1,opt}||    (Eq. 10)
```

---

## Results

| Condition | Median Location Error |
|---|---|
| Initial position known | **25 cm** |
| No initial position | **38 cm** |
| Relative velocity error | 13% |

Hardware: COTS Wi-Fi, 1 transmitter + 2 receivers (each with 3 antennas, treated as 6 separate links). Office room 4 m × 5 m.

---

## Limitations

- Requires 6 effective links (3 per receiver × 2 receivers) — higher than claimed "commodity" setup
- Sign of PLCR cannot always be determined (WiDir fails when motion is parallel to link)
- Mixed reflections from arms/legs alongside torso create additional PLCR components
- 2D tracking only (no height estimation)
- Point reflector assumption: human body is extended, not a point

---

## Relevance to Spaxel

Widar's CSI-mobility model is the theoretical foundation for understanding how node geometry and target velocity relate to measured CSI changes. The A·v = r formulation directly informs how Spaxel should weight per-link Fresnel zone contributions based on the relative geometry of nodes and target.
