# CARM: CSI-Based Activity Recognition and Monitoring

**Authors:** Wei Wang, Alex X. Liu, Muhammad Shahzad, Kang Ling, Sanglu Lu
**Venue:** ACM MobiCom 2015
**DOI:** [10.1145/2789168.2790093](https://doi.org/10.1145/2789168.2790093)
**Institution:** Michigan State University / Nanjing University

---

## Citation

```
@inproceedings{wang2015carm,
  title     = {Understanding and Modeling of WiFi Signal Based Human Activity Recognition},
  author    = {Wang, Wei and Liu, Alex X. and Shahzad, Muhammad and Ling, Kang and Lu, Sanglu},
  booktitle = {Proceedings of the 21st Annual International Conference on Mobile Computing and Networking},
  series    = {MobiCom '15},
  year      = {2015},
  pages     = {65--76},
  doi       = {10.1145/2789168.2790093},
  publisher = {ACM}
}
```

---

## Abstract

> "Some pioneer WiFi signal based human activity recognition systems have been proposed. Their key limitation lies in the lack of a model that can quantitatively correlate CSI dynamics and human activities. In this paper, we propose CARM, a CSI based human Activity Recognition and Monitoring system. CARM has two theoretical underpinnings: a CSI-speed model, which quantifies the correlation between CSI value dynamics and human movement speeds, and a CSI-activity model, which quantifies the correlation between the movement speeds of different human body parts and a specific human activity. By these two models, we quantitatively build the correlation between CSI value dynamics and a specific human activity. CARM uses this correlation as the profiling mechanism and recognizes a given activity by matching it to the best-fit profile. We implemented CARM using commercial WiFi devices and evaluated it in several different environments. Our results show that CARM achieves an average accuracy of greater than 96%."

---

## Problem Statement

Pioneer systems (WiSee, E-eyes, WiHear) lack quantitative models correlating CSI measurements to human activities. Without a model, optimisation is trial-and-error; systems cannot generalise to new environments or users without retraining. CARM is the first system to provide a **closed-form theoretical model** deriving activity information from physical CSI dynamics.

---

## Core Theory

### Why Phase is Unusable

Raw CSI phase drifts up to 50π between consecutive frames due to Carrier Frequency Offset (CFO). CARM measurements show CFO of ~80 kHz, causing 8π phase shift per 50 µs — body movement signals are ≤0.5π and are entirely buried in noise.

**Solution:** Use CSI power `|H(f,t)|²` instead of phase. Power is **invariant to CFO** because the CFO term `e^{−j2πΔft}` cancels out in the magnitude operation.

### CSI-Speed Model

For multipath channel with K dynamic paths, CFR power decomposes as:

```
|H(f,t)|² = Σ_{k∈P_d} 2|H_s(f)·a_k| · cos(2πv_k·t/λ + 2πd_k(0)/λ + φ_{sk})
           + Σ_{k,l∈P_d, k≠l} 2|a_k·a_l| · cos(2π(v_k−v_l)t/λ + ...)
           + Σ_{k∈P_d} |a_k|² + |H_s(f)|²    (Eq. 3)
```

**Key insight:** CFR power = DC offset + sinusoids with frequencies `v_k/λ` (path length change speeds in units of wavelengths/sec). **Measuring the frequency of CSI power oscillation directly measures the speed of the reflecting body part.**

At 5 GHz (λ = 5.15 cm), 300 Hz → 15.45 m/s — well above human movement speeds. The useful band is 0–80 Hz for indoor activities.

### PCA-Based Denoising

CSI streams across subcarriers are correlated time-varying signals with different initial phases (due to multipath path length differences). They share the same time-varying component (body motion) but differ in the static offset:
```
|H_k(f,t)|² ≈ A_k(t) + C_k    (same time variation, different constant)
```

PCA extracts the first principal component (the shared time-varying signal) while suppressing:
- Impulse/burst noise from rate adaptation (affects all subcarriers simultaneously but incoherently with body motion)
- Frequency-selective static offsets

### DWT Feature Extraction

Discrete Wavelet Transform separates activity speed components:

| Activity | Dominant frequency | Physical speed |
|---|---|---|
| Walking (torso) | 35–40 Hz | 0.9–1.0 m/s |
| Walking (legs) | 50–70 Hz | 1.3–1.8 m/s |
| Falling | 40–80 Hz (brief spike) | 1.0–2.0 m/s (acceleration) |
| Sitting down | < 40 Hz | < 1.0 m/s |
| Brushing teeth | < 1 Hz | < 0.025 m/s |

### CSI-Activity Model (HMM)

Each activity = sequence of speed states over time. Hidden Markov Model captures state transitions:
- Fall: {slow → fast-up → sudden-silence}
- Walk: {sustained 35–40 Hz}
- Sit: {brief 20–30 Hz → silence}

Training: DWT speed profile features from **780+ activity samples across 25 volunteers**. The model generalises across users and environments because it is grounded in the physics of body motion speeds, not environment-specific features.

### Activity Detection

In a static environment, the first PCA eigenvector varies randomly (incoherent). During activity, CSI streams become correlated → eigenvector becomes smooth. High-frequency energy of eigenvector vs. adaptive threshold → detect activity start/end boundaries.

---

## Results

| Scenario | Accuracy |
|---|---|
| Same environment and person (training match) | **>96%** |
| New environment (generalisation) | **>80%** |
| New person (generalisation) | **>80%** |

Compared to RSSI-based: 56–72% accuracy. Compared to WiSee (USRP hardware): CARM matches at 96% using **commodity hardware**.

---

## Limitations

- Single-person scenario; multi-person requires blind signal separation (noted as future work)
- Person must be within range of a TX-RX link
- Activities with very similar speed profiles may be confused
- 2D movement assumed (height information not captured)
- HMM training still required per activity class (though not per environment or user)

---

## Relevance to Spaxel

CARM's CSI-speed model is the theoretical justification for why Spaxel's amplitude variance metric works: body movement at speed v produces CSI power oscillations at frequency v/λ. This directly justifies the motion-gated baseline update in Spaxel — when this oscillation frequency is below the stability threshold, the scene is genuinely quiet, not just slow-moving. CARM's PCA denoising approach is a practical technique for extracting the motion signal from multi-subcarrier CSI before feeding the baseline estimator.
