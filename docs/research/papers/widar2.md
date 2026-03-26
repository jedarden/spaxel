# Widar2.0: Passive Human Tracking with a Single Wi-Fi Link

**Authors:** Kun Qian, Chenshu Wu, Yi Zhang, Guidong Zhang, Zheng Yang, Yunhao Liu
**Venue:** ACM MobiSys 2018
**DOI:** [10.1145/3210240.3210314](https://doi.org/10.1145/3210240.3210314)
**PDF:** https://www.cswu.me/papers/mobisys18_widar2.0_paper.pdf
**Institution:** Tsinghua University (TNS Lab)

---

## Citation

```
@inproceedings{qian2018widar2,
  title     = {Widar2.0: Passive Human Tracking with a Single Wi-Fi Link},
  author    = {Qian, Kun and Wu, Chenshu and Zhang, Yi and Zhang, Guidong and Yang, Zheng and Liu, Yunhao},
  booktitle = {Proceedings of the 16th Annual International Conference on Mobile Systems, Applications, and Services},
  series    = {MobiSys '18},
  year      = {2018},
  pages     = {350--361},
  doi       = {10.1145/3210240.3210314},
  publisher = {ACM}
}
```

---

## Abstract

> "This paper presents Widar2.0, the first WiFi-based system that enables passive human localization and tracking using a single link on commodity off-the-shelf devices. Previous works based on either specialized or commercial hardware all require multiple links, preventing their wide adoption in scenarios like homes where typically only one single AP is installed. The key insight underlying Widar2.0 to circumvent the use of multiple links is to leverage multi-dimensional signal parameters from one single link. To this end, we build a unified model accounting for Angle-of-Arrival, Time-of-Flight, and Doppler shifts together and devise an efficient algorithm for their joint estimation. We then design a pipeline to translate the erroneous raw parameters into precise locations, which first finds parameters corresponding to the reflections of interests, then refines range estimates, and ultimately outputs target locations. Our implementation and evaluation on commodity WiFi devices demonstrate that Widar2.0 achieves better or comparable performance to state-of-the-art localization systems, which either use specialized hardware or require 2 to 40 Wi-Fi links."

---

## Problem Statement

Device-free passive localisation is needed for eldercare, security monitoring, and retail analytics without body-worn hardware. Prior systems require 2–40 Wi-Fi links:

| System | Technique | Links | Accuracy |
|---|---|---|---|
| WiTrack | FMCW radar | 2 | 0.3 m |
| WiDeo | Full-duplex Wi-Fi | 1 (specialised) | 0.7 m |
| Widar (v1) | COTS Wi-Fi | 6 | 0.35 m |
| DynamicMusic | COTS Wi-Fi | 4 | 0.6 m |
| IndoTrack | COTS Wi-Fi | 3 | 0.48 m |
| LiFS | COTS Wi-Fi | 40 | 0.7 m |
| **Widar2.0** | **COTS Wi-Fi** | **1** | **0.75 m** |

Most homes have only one AP. Challenge: reflected signals are orders of magnitude weaker than direct signal; 20 MHz bandwidth limits ToF resolution to 0.3 m.

---

## Core Signal Model

Channel at time t, frequency f, sensor s:
```
H(t,f,s) = Σ_{l=1}^{L} α_l(t,f,s) · e^{−j2πf·τ_l(t,f,s)} + N(t,f,s)    (Eq. 1)
```

Phase of l-th path in discrete measurement H(i,j,k):
```
f·τ_l(i,j,k) ≈ f_c·τ_l + Δf_j·τ_l + f_c·Δs_k·φ_l − f_{Dl}·Δt_i    (Eq. 2)
```
Where:
- `f_c` = carrier frequency
- `Δt_i / Δf_j / Δs_k` = time / frequency / space differences from reference
- `τ_l / φ_l / f_{Dl}` = ToF / AoA / DFS of l-th path

---

## Methodology

### Step 1: Phase Noise Removal — Conjugate Multiplication

Raw CSI contains phase noise from timing offset `ε_ti` and carrier frequency offset `ε_f`:
```
H̃(m) = H(m) · e^{2π(Δf_j·ε_{ti} + Δt_i·ε_f) + ζ_{sk}}    (Eq. 8)
```

**Solution:** Conjugate multiply adjacent antenna k against reference k₀:
```
C(m) = H̃(m) × H̃*(m₀) = H(m) × H*(m₀)    (Eq. 9)
```

Phase noise is identical on co-located antennas and cancels out. The product retains AoA and DFS structure but makes ToF *relative* (τ_l − τ_LoS) — this is actually an advantage because it removes the unknown LoS propagation time.

### Step 2: Joint AoA-ToF-DFS Estimation via SAGE

Maximum likelihood estimation:
```
Θ̂_ML = argmax_Θ { −Σ_m |h(m) − Σ_{l=1}^{L} P_l(m; θ_l)|² }    (Eqs. 3–4)
```

SAGE (Space-Alternating Generalised EM) algorithm:

**E-step:**
```
P̂_l(m; Θ̂') = P_l(m; θ̂'_l) + β_l · (h(m) − Σ_{l'} P_{l'}(m; θ̂'_{l'}))    (Eq. 5)
```

**M-step (sequential for each path):**
```
τ̂''_l   = argmax_τ       |z(τ, φ̂'_l, f̂'_{Dl}; P̂_l)|
φ̂''_l   = argmax_φ       |z(τ̂''_l, φ, f̂'_{Dl}; P̂_l)|
f̂''_{Dl} = argmax_{f_D}  |z(τ̂''_l, φ̂''_l, f_D; P̂_l)|
α̂''_l   = z(τ̂''_l, φ̂''_l, f̂''_{Dl}; P̂_l) / TFA          (Eq. 6)
```

where `z(τ, φ, f_D; P_l)` is the cross-ambiguity function integrating time, frequency, and space:
```
z = Σ_m e^{j2πΔf_j·τ} · e^{j2πf_c·Δs_k·φ} · e^{−j2πf_D·Δt_i} · P_l(m)    (Eq. 7)
```

### Step 3: Path Matching via N-Partite Graph

Build N-partite weighted graph where each node `v_{ij}` = parameters of j-th path in i-th time window. Edge weight:
```
w_{i1j1}^{i2j2} = ||c^T(θ_{i1j1} − θ_{i2j2})||    (Eq. 13)
```

Minimise total edge weight subject to forming L complete N-order subgraphs. Solved as binary integer programme over rolling windows of N=6 estimations.

### Step 4: Range Refinement (0.3 m → 0.05 m)

ToF-based range resolution: `c / (2B)` = 7.5 m at 20 MHz — too coarse.

DFS provides fine-grained range change rate: `v = −f_D · λ` with ~0.05 m/s resolution.

**Kalman Smoother fuses both:**
```
f_D = −v / λ    (Eq. 19)
```

Cumulative DFS integration refines range estimates to 0.05 m resolution — a 6× improvement over ToF alone.

### Step 5: Localization (Semi-Ellipse Intersection)

Target at `l = (x, y)` from transmitter `o = (0,0)` and receiver `l_r = (x_r, y_r)`:

System of equations:
```
{ x² + y² + √((x−x_r)² + (y−y_r)²) = d_{Tar}
{ (y−y_r)cos(ψ_r−φ_{Tar}) = (x−x_r)sin(ψ_r−φ_{Tar})    (Eq. 20)
```

Closed-form solution (intersection of semi-ellipse and semi-line):
```
x = ½ · (d²_{Tar} + 2s_r·d_{Tar}·x_r·secφ + x²_r·sec²φ − (x_r·tanφ − y_r)²)
      / (x_r + y_r·tanφ + s_r·d_{Tar}·secφ)
y = tanφ · (x − x_r) + y_r    (Eq. 21)
```

---

## Results

| Environment | Size | Median Error |
|---|---|---|
| Classroom | 5 m × 6 m | ~0.75 m |
| Office | 4 m × 2.5 m | ~0.70 m |
| Corridor | 8 m long | ~0.82 m |

- Hardware: Intel 5300 NICs, 5.825 GHz (channel 165), 1000 Hz packet rate
- ToF resolution improved: 0.3 m → **0.05 m** via DFS-Kalman fusion
- Comparable to IndoTrack (0.48 m, 3 links) using only **1 link**
- Better than DynamicMusic (0.6 m, 4 links) and LiFS (0.7 m, 40 links)

---

## Limitations

- Single-link accuracy (0.75 m) lower than multi-link systems (0.35–0.48 m)
- Assumes target on one side of the link (unique geometric solution requires this)
- Range estimation fundamentally limited by 20 MHz bandwidth
- Tracking range: ~8 m
- Processing requires high-performance CPU — not real-time on embedded hardware
- 2D only (no height estimation)

---

## Relevance to Spaxel

Widar2.0's range refinement technique (fusing ToF with cumulative DFS integration) is directly applicable to improving Spaxel's positioning accuracy beyond the raw Fresnel zone granularity. The SAGE joint parameter estimation is the gold standard for multi-dimensional CSI parameter extraction — Spaxel's voxel accumulator is a simpler approximation that trades accuracy for computational tractability on the mothership.
