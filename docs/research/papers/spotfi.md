# SpotFi: Decimeter Level Localization Using WiFi

**Authors:** Manikanta Kotaru, Kiran Joshi, Dinesh Bharadia, Sachin Katti
**Venue:** ACM SIGCOMM 2015
**DOI:** [10.1145/2785956.2787487](https://doi.org/10.1145/2785956.2787487)
**PDF:** https://web.stanford.edu/~skatti/pubs/sigcomm15-spotfi.pdf
**Institution:** Stanford University

---

## Citation

```
@inproceedings{kotaru2015spotfi,
  title     = {SpotFi: Decimeter Level Localization Using WiFi},
  author    = {Kotaru, Manikanta and Joshi, Kiran and Bharadia, Dinesh and Katti, Sachin},
  booktitle = {Proceedings of the 2015 ACM Conference on Special Interest Group on Data Communication},
  series    = {SIGCOMM '15},
  year      = {2015},
  pages     = {269--282},
  doi       = {10.1145/2785956.2787487},
  publisher = {ACM}
}
```

---

## Abstract

> "This paper presents the design and implementation of SpotFi, an accurate indoor localization system that can be deployed on commodity WiFi infrastructure. SpotFi only uses information that is already exposed by WiFi chips and does not require any hardware or firmware changes, yet achieves the same accuracy as state-of-the-art localization systems. SpotFi makes two key technical contributions. First, SpotFi incorporates super-resolution algorithms that can accurately compute the angle of arrival (AoA) of multipath components even when the access point (AP) has only three antennas. Second, it incorporates novel filtering and estimation techniques to identify AoA of direct path between the localization target and AP by assigning values for each path depending on how likely the particular path is the direct path. Our experiments in a multipath rich indoor environment show that SpotFi achieves a median accuracy of 40 cm and is robust to indoor hindrances such as obstacles and multipath."

---

## Problem Statement

Indoor localization must satisfy three simultaneous requirements:
1. **Deployable** — on existing commodity WiFi infrastructure, no hardware/firmware changes
2. **Universal** — localise any target with only a commodity WiFi chip
3. **Accurate** — match best systems (ArrayTrack, Ubicarse) at 30–50 cm median error

Existing systems fail at least one: RSSI-based systems achieve only 2–4 m median accuracy. AoA-based systems (ArrayTrack) require 6–8 antennas. Time-based systems require synchronised clocks. None satisfy all three simultaneously.

---

## Key Technical Contributions

### 1. Super-Resolution AoA and ToF via Modified MUSIC

Standard MUSIC with M=3 antennas fails in typical indoor environments with 6–8 significant propagation paths (D < M required). SpotFi's insight: expand the effective sensor array by treating each **(antenna, subcarrier) pair as a virtual sensor element**.

For the k-th propagation path with AoA θ and ToF τ, the steering vector across M×N virtual sensors (M antennas × N subcarriers) is:

```
a(θ, τ) = [1, …, Ω^(N−1)_τ, Φ_θ, …, Ω^(N−1)_τ Φ_θ, …, Φ^(M−1)_θ, …, Ω^(N−1)_τ Φ^(M−1)_θ]^T
```

Where:
- `Φ(θ) = e^(−j2π·d·sin(θ)·f/c)` — phase shift at adjacent antenna due to AoA
- `Ω(τ) = e^(−j2π·f_δ·τ)` — phase shift at adjacent subcarrier due to ToF

**CSI matrix** (Intel 5300: 3 antennas × 30 subcarriers):
```
CSI_matrix = [csi_{m,n}]_{3×30}
```

**Received signal model:**
```
x = A · Γ    (Eq. 3)
```
where A is the M×L steering matrix (L = number of paths), Γ is the vector of complex attenuations.

**CSI smoothing:** SpotFi constructs a 30×30 smoothed CSI matrix by taking overlapping subarrays (each subarray = 15 subcarriers × 2 antennas). This expands from 3 effective sensors to 30, enabling resolution of up to 30 multipath components.

### 2. ToF Sanitisation

Sampling Time Offset (STO) adds a constant phase ramp `−2πf_δ(n−1)τ_s` to all subcarriers.

Removal algorithm:
```
τ̂_s,i = argmin_ρ  Σ_{m,n} (ψ_i(m,n) + 2πf_δ(n−1)ρ + β)²
ψ̂_i(m,n) = ψ_i(m,n) + 2πf_δ(n−1)τ̂_s,i
```

### 3. Direct Path Identification

Likelihood metric for path k being the direct (LoS) path:
```
likelihood_k = exp(w_C·C̄_k − w_θ·σ̄_{θk} − w_τ·σ̄_{τk} − w_s·τ̄_k)
```
Where:
- `C̄_k` = cluster size (higher → more consistent, more likely LoS)
- `σ̄_{θk}, σ̄_{τk}` = variance of AoA/ToF estimates across packets (lower → more likely LoS)
- `τ̄_k` = mean ToF (lower → shorter path, more likely LoS)

### 4. Localization

Combines direct-path AoA from multiple APs with RSSI-based distance, weighted by likelihood, finding:
```
l* = argmax_l  Σ_i  likelihood_i · P(AoA_i | l, AP_i) · P(dist_i | l, AP_i)
```

---

## Results

| Metric | Value |
|---|---|
| Median localization error | **40 cm** |
| 80th percentile error | 1.8 m |
| Packets required per fix | 10 |
| APs required | 3 |
| Antenna count per AP | 3 |

Comparable to ArrayTrack (40 cm, but 6–8 antennas/AP) and Ubicarse (40 cm, but requires gyroscope). RSSI baseline: 2–4 m.

---

## Limitations

- ToF estimates are relative (no clock synchronisation) — only path length *differences* reliable
- Requires APs at known positions (one-time offline survey)
- Tested with Intel 5300 NICs only
- Direct path may be fully obstructed — degrades under heavy occlusion
- Fixed assumption: ≤5 significant paths per AP (5 Gaussian clusters)
- Designed for static localisation, not tracking moving targets

---

## Relevance to Spaxel

SpotFi's core idea — treating (antenna, subcarrier) pairs as virtual array elements — is the algorithmic foundation for single-device multi-path resolution. With multiple ESP32-S3 nodes in a mesh, each directed link approximates one AP's contribution; the Fresnel zone intersection method in Spaxel is a simplified geometric alternative to SpotFi's MUSIC that avoids the multi-antenna coherence requirement.
