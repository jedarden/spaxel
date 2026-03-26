# Detection and Localization Algorithms

## Background Subtraction

### Exponential Moving Average (EMA)

```
H_bg(t) = α · H_bg(t−1) + (1−α) · H(t)
H_diff(t) = H(t) − H_bg(t)
```

α ≈ 0.9–0.99. Simple, real-time capable. Fails for slowly drifting environments; adapts to stationary persons over time — the primary *stillness problem*.

### Robust PCA (RPCA)

Decomposes a CSI matrix M = L + S, where L is low-rank (static background) and S is sparse (motion events). Applied to a matrix with rows = time samples, columns = subcarriers:

- L captures stable multipath environment
- S captures transient changes due to human motion

Solved via Principal Component Pursuit:

```
minimise  ||L||_* + λ||S||_1   subject to  L + S = M
```

More robust than EMA but computationally intensive. Typically applied in batch windows rather than fully online.

### Gaussian Mixture Models (GMM)

Model each subcarrier's CSI amplitude distribution as a mixture of K Gaussians over time. Background is the dominant component. Foreground (person present) is detected when the observation probability under the background GMM falls below a threshold. Can track multiple states (empty / occupied / different activity levels).

---

## Fresnel Zone Weighted Localization

For multi-link systems without per-link AoA:

1. For each link (TX_i, RX_j), compute the RMS delta from baseline across subcarriers.
2. For each candidate grid point P, compute the Fresnel influence weight:
   ```
   w(P, i, j) = deltaRMS × (1 − excess/r₁)   if excess < r₁, else 0
   ```
   where `excess = (d₁ + d₂) − direct_distance` and `r₁` is the first Fresnel zone radius at P.
3. Sum influence weights across all links → spatial probability field.
4. Find peaks via non-maximum suppression → blob positions.

Model-based and geometric — no fingerprint training required. Accuracy depends on link count, geometry, and delta precision.

---

## MUSIC (MUltiple SIgnal Classification)

Subspace-based super-resolution algorithm for estimating directions of arrival.

**Setup**: M antennas receive signals from D sources (D < M). Received signal vector: `x(t) = A·s(t) + n(t)`.

**Steps**:
1. Estimate spatial covariance matrix: `R = E[x·x^H]`
2. Eigendecompose R into signal subspace (U_s) and noise subspace (U_n)
3. MUSIC pseudospectrum: `P_MUSIC(θ) = 1 / (a(θ)^H · U_n · U_n^H · a(θ))`
4. Peaks of P_MUSIC(θ) are the estimated AoAs

### SpotFi Extension (Joint AoA-ToF)

SpotFi (Kotaru et al., SIGCOMM 2015) treats each (antenna, subcarrier) pair as a virtual element. For M antennas and J subcarriers, this gives M×J virtual elements. The 2D steering vector becomes:

```
a(θ, τ) = [e^(j·2π·d·sin(θ)/λ₁), ..., e^(j·2π·d·sin(θ)/λ_J)]
         × [e^(−j·2π·f₁·τ), ..., e^(−j·2π·f_J·τ)]
```

This enables joint AoA + Time-of-Flight estimation from a single antenna with multiple subcarriers. SpotFi achieves **40 cm median localization error** in tested environments.

**Spatial smoothing for coherent signals**: When multipath signals are correlated, spatial smoothing divides the virtual array into overlapping subarrays and averages their covariance matrices, de-correlating coherent components at the cost of reduced effective aperture.

**Single-antenna limitation**: MUSIC directly requires multiple spatially separated antennas. A single-antenna ESP32-S3 cannot apply MUSIC for AoA unless signals from multiple nodes are combined coherently (requires phase-synchronised hardware or the ESPARGOS platform).

---

## ESPRIT

Estimation of Signal Parameters via Rotational Invariance Techniques. Exploits rotational invariance between two identical subarrays displaced by a known distance d. Avoids the grid search of MUSIC by solving a generalised eigenvalue problem. Computationally cheaper than MUSIC but requires a specific two-subarray structure. Same spatial smoothing requirements as MUSIC.

---

## DFS-Based Tracking (Widar Family)

Doppler Frequency Shift from a moving body: `f_d = 2v·cos(θ) / λ`

For walking at 1 m/s at 2.4 GHz: f_d ≈ 2×1×1/0.125 = **16 Hz**.

### Widar (2017)

Extracts DFS from CSI time series to estimate velocity vector. Decimeter-level passive tracking from velocity monitoring alone.

### Widar2.0 (2018)

First single-link passive localization. Jointly estimates AoA, ToF, and DFS from one link using a unified signal model. Key insight: range refinement from 0.3 m to 0.05 m by combining ToF with cumulative DFS-derived path length changes.

Accuracy: **0.75 m median error** (1 link), **0.63 m** (2 links).

---

## IndoTrack (Doppler-MUSIC)

Applies MUSIC to the DFS domain rather than the spatial domain — resolves velocity components of multiple people simultaneously. Combined with probabilistic AoA estimation for trajectory reconstruction.

Requires minimum 3 antennas. Achieves **35 cm median trajectory error** with commodity hardware.

---

## Fingerprinting

**Training phase**: Collect CSI at known grid positions throughout the space. Store as a map.

**Runtime**: Match incoming CSI to nearest reference via k-NN, SVM, or deep neural network.

**Pros**: Works in complex multipath environments where geometric models fail. Can exploit full CSI as a high-dimensional feature vector.

**Cons**: Training burden scales with area × spatial resolution. Sensitive to environmental changes — moving furniture invalidates the map. Must be periodically recollected.

Accuracy: 0.5–2 m in controlled settings. Deep learning approaches (ResNet on CSI images) reach ~0.5–1 m without explicit geometric models.

---

## Kalman Filter for Trajectory Smoothing

State vector: `[x, y, ẋ, ẏ]` (2D) or `[x, y, z, ẋ, ẏ, ż]` (3D).

Constant velocity motion model:
```
F = [[1, 0, dt, 0],
     [0, 1,  0, dt],
     [0, 0,  1,  0],
     [0, 0,  0,  1]]
```

Predict-correct cycle:
```
Predict:  x̂⁻ = F·x̂,   P⁻ = F·P·F^T + Q
Update:   K = P⁻·H^T·(H·P⁻·H^T + R)⁻¹
          x̂ = x̂⁻ + K·(z − H·x̂⁻)
```

The **Unscented Kalman Filter (UKF)** is preferred for nonlinear sensing models (e.g., when measurements are bistatic ranges rather than direct positions). Recent work combines UKF with self-attention mechanisms for WiFi CSI tracking.

---

## Breathing and Stillness Detection

A stationary person still breathes at 0.1–0.5 Hz (12–20 breaths/min).

Chest displacement ~5 mm at 2.4 GHz:
```
phase change ≈ 2π × 2 × 0.005 / 0.125 ≈ 0.5 rad
```

This is detectable above noise with 10–30 s averaging. Processing:
1. Apply bandpass filter to CSI amplitude time series at 0.1–0.5 Hz
2. FFT of filtered signal — peak frequency = breathing rate
3. Presence of breathing peak = person present but stationary

Heartbeat (~1–2 Hz, ~1–2 mm displacement) is marginally detectable on commodity hardware and requires very stable mounting and extended averaging windows.

---

## Algorithm Selection for Spaxel

| Condition | Recommended algorithm |
|---|---|
| Moving target, N nodes | Fresnel zone weighted intersection + Kalman |
| Moving target, need high accuracy | Widar2.0-style joint AoA-ToF-DFS (single link) |
| Stationary target | Breathing bandpass detection |
| Multiple people | IndoTrack-style Doppler-MUSIC (requires ≥3 antennas) |
| Post-calibration fingerprinting | k-NN or ResNet on CSI feature vectors |
| Trajectory smoothing | UKF with constant velocity model |
