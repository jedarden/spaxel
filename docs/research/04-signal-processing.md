# Signal Processing Pipeline

Raw int8 I/Q pairs from the ESP32 require several processing steps before they are usable for sensing.

---

## Step 1: Acquire Raw I/Q Pairs

From `wifi_csi_info_t.buf` (int8 array). Separate LLTF and HT-LTF portions based on `len` and the enabled field flags in `wifi_csi_config_t`. Skip null/guard subcarriers per the ESP-IDF subcarrier index tables.

---

## Step 2: Compute Complex CSI

```python
# Raw buffer: alternating Im, Re pairs
im = csi_buf[0::2].astype(float)   # imaginary
re = csi_buf[1::2].astype(float)   # real

amplitude = np.sqrt(re**2 + im**2)
phase_raw  = np.arctan2(im, re)    # range [−π, π], contaminated
```

---

## Step 3: Phase Sanitisation

Raw phase is contaminated by several hardware error sources.

### a. Sampling Frequency Offset (SFO) / Packet Detection Delay (PDD)

Creates a linear phase slope across subcarriers: `φ(k) = φ_true(k) + a·k + b`.

Mitigation — linear regression across subcarrier index k:

```python
k_idx = np.arange(len(phase))
phase_unwrapped = np.unwrap(phase_raw)
slope, intercept = np.polyfit(k_idx, phase_unwrapped, 1)
phase_corrected  = phase_unwrapped − (slope * k_idx + intercept)
```

### b. Phase Unwrapping

Raw arctan2 output wraps at ±π. Unwrap before linear fitting:

```python
phase_unwrapped = np.unwrap(phase_raw)
```

### c. IQ Imbalance

Hardware imperfection causing M-shaped amplitude and S-shaped phase distortion as a function of subcarrier index. Stable across time but varies between TX-RX pairs. Calibrate with a reference measurement (cable-connected TX/RX); subtract the template from operational data.

### d. AGC (Automatic Gain Control) Uncertainty

Each packet's CSI amplitude is scaled by a random gain factor β. Options:
- Compensate using reported RSSI and noise floor from `rx_ctrl.rssi`
- Disable AGC in driver (impacts packet decoding reliability — not recommended for production)

### e. Central Frequency Offset (CFO)

Random frequency shift between TX and RX clocks → time-varying phase bias. Estimated using the known 4 µs spacing between consecutive HT-LTFs within a single packet (phase difference between LTF1 and LTF2 encodes CFO directly).

### f. Cross-Node Phase Calibration

Phases are not coherent across independent nodes — each has its own hardware oscillator. For multi-node fusion:
- **Reference-link normalisation**: divide each link's CSI by the CSI of a known reference path
- **Conjugate multiplication**: between antenna pairs on the same device to cancel common hardware noise
- **Full coherence**: only achievable with shared reference clock hardware (ESPARGOS platform)

---

## Step 4: Subcarrier Selection

Not all subcarriers are equally informative.

**Exclude**:
- Guard/null subcarriers (indices ±28 to ±32, DC at 0)
- Pilot subcarriers (carry constant-phase pilots, not data — though they can serve as phase references)

**Prefer**:
- Mid-band subcarriers (edge subcarriers suffer more from the M-shaped hardware response)
- Subcarriers with higher temporal variance (carry more motion information)

**NBVI algorithm** (Normalised Band Variance Index — from ESPectre): selects 12 non-consecutive subcarriers that maximise information while avoiding correlation between adjacent subcarriers.

---

## Step 5: Feature Extraction

### Motion Detection

Variance or standard deviation of CSI amplitude over a sliding window:

```python
motion_score = np.var(amplitude_window, axis=0).mean()
```

Compare to baseline variance. Threshold crossing = motion event.

### Velocity / DFS

Short-Time Fourier Transform (STFT) of complex CSI over time. The Doppler frequency shift:

```
f_d = 2 · v · cos(θ) / λ
```

For walking at 1 m/s at 2.4 GHz: f_d ≈ 16 Hz.

```python
from scipy.signal import stft
freqs, times, Zxx = stft(csi_complex, fs=20.0, nperseg=64)
doppler_spectrum = np.abs(Zxx)
```

### Localization (AoA-ToF)

Channel Impulse Response via IFFT across subcarriers:

```python
cir = np.fft.ifft(H_complex, n=256)
tof_est = np.argmax(np.abs(cir)) / (256 * subcarrier_spacing)  # seconds
```

Apply 2D MUSIC across subcarriers (virtual array) for joint AoA-ToF per SpotFi.

### Breathing Rate

```python
from scipy.signal import butter, filtfilt
b, a = butter(4, [0.1, 0.5], btype='bandpass', fs=20.0)
breathing_signal = filtfilt(b, a, amplitude_ts)
fft_spectrum = np.abs(np.fft.rfft(breathing_signal))
breathing_rate_hz = np.argmax(fft_spectrum) / len(breathing_signal) * 20.0
```

---

## Step 6: Multi-Link Fusion

Per-link features are combined in the mothership positioning engine:

1. For each link, compute deltaRMS = RMS(current_CSI − baseline)
2. Accumulate Fresnel zone influence weights across the voxel grid
3. Decay grid over time (factor ~0.95 per cycle) to allow blobs to move or disappear
4. Find peaks via non-maximum suppression
5. Apply Kalman/UKF to smooth peak positions over time

---

## Timing Summary

| Stage | Where | Rate |
|---|---|---|
| CSI capture | ESP32 (csi_callback) | 10–100 Hz per link |
| I/Q → amplitude | ESP32 (optional) or mothership | Per packet |
| UDP send | ESP32 (transport.c) | Per packet |
| Baseline update | Mothership (baseline/ema.go) | Per received packet |
| Fresnel accumulation | Mothership (positioning/fresnel.go) | Per received packet |
| Blob extraction | Mothership | Per accumulation cycle |
| WebSocket publish | Mothership (hub/hub.go) | Per blob extraction |
| Dashboard render | Browser | ~10 Hz (rAF throttled) |

---

## Key Practical Notes

- **Phase is more sensitive than amplitude** for detecting fine motion (breathing, small displacements) but requires sanitisation.
- **Amplitude is more robust** for gross motion and presence detection — use it for the primary pipeline.
- **Subcarrier diversity is the main advantage of CSI over RSSI** — some subcarriers may fade while others remain informative; averaging across subcarriers reduces noise.
- **int8 dynamic range** (−128 to +127) limits the detectable signal range per subcarrier. Objects very close to a node will saturate.
- **channel_filter_en = false** in `wifi_csi_config_t` gives independent per-subcarrier data; setting it true smooths adjacent subcarriers and reduces per-subcarrier noise at the cost of spatial frequency resolution.
