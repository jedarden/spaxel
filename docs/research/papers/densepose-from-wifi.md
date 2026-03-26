# DensePose From WiFi

**Authors:** Jiaqi Geng, Dong Huang, Fernando De la Torre
**Venue:** arXiv preprint, December 2022
**arXiv:** [2301.00250](https://arxiv.org/abs/2301.00250)
**Institution:** Carnegie Mellon University (Robotics Institute)

---

## Citation

```
@article{geng2022densepose,
  title   = {DensePose From WiFi},
  author  = {Geng, Jiaqi and Huang, Dong and De la Torre, Fernando},
  journal = {arXiv preprint arXiv:2301.00250},
  year    = {2022}
}
```

---

## Abstract

> "Advances in computer vision and machine learning techniques have led to significant development in 2D and 3D human pose estimation from RGB cameras, LiDAR, and radars. However, human pose estimation from images is adversely affected by occlusion and lighting, which are common in many scenarios of interest. Radar and LiDAR technologies, on the other hand, need specialized hardware that is expensive and power-intensive. Furthermore, placing these sensors in non-public areas raises significant privacy concerns. To address these limitations, recent research has explored the use of WiFi antennas (1D sensors) for body segmentation and key-point body detection. This paper further expands on the use of the WiFi signal in combination with deep learning architectures, commonly used in computer vision, to estimate dense human pose correspondence. We developed a deep neural network that maps the phase and amplitude of WiFi signals to UV coordinates within 24 human regions. The results of the study reveal that our model can estimate the dense pose of multiple subjects, with comparable performance to image-based approaches, by utilizing WiFi signals as the only input."

---

## Problem Statement

Camera-based pose estimation (RGB, depth) is impaired by occlusion, lighting conditions, and raises significant privacy concerns in sensitive spaces (bedrooms, bathrooms). LiDAR and radar provide privacy-preserving sensing but require expensive specialised hardware. Recent research has explored WiFi for body segmentation (Person-in-WiFi, 2019) and keypoint estimation (17 joints). This paper extends to **DensePose** — UV coordinate mapping of the full body surface to a canonical 3D model, covering 24 human body regions.

---

## Methodology

### Input Representation

- 3 transmit antennas × 3 receive antennas = 9 TX-RX antenna pairs
- Multiple OFDM subcarriers per pair
- Extract complex CSI (amplitude + phase) per subcarrier per pair
- Form 2D "WiFi amplitude/phase images" — frequency × space tensors analogous to image channels

### Architecture: WiFi-DensePose RCNN

Inspired by DensePose-RCNN from computer vision. Two-branch encoder-decoder:

```
WiFi CSI tensor (amplitude + phase)
    │
    ▼
ResNet-based encoder
    │
    ▼
Feature Pyramid Network (FPN) — multi-scale spatial features
    │
    ▼
ROI pooling — per-person feature extraction
    ├──────────────────────────┐
    ▼                          ▼
Keypoint Head               DensePose Head
(17-joint skeleton)         (UV coordinates for 24 body regions)
```

The keypoint head facilitates learning by providing an auxiliary supervision signal during training. The DensePose head predicts:
- Body part classification (which of 24 regions)
- (U, V) coordinates within that region mapping to the canonical 3D body surface

### Training Data

Requires paired WiFi + camera dataset. Camera provides DensePose ground truth (UV labels, body segmentation). Model learns to map WiFi CSI tensor → DensePose output without seeing camera images at test time.

---

## Results

| Metric | Value |
|---|---|
| Performance | Comparable to image-based approaches |
| Multi-person | Multiple subjects estimated simultaneously |
| Multi-person tracking | 92.61% accuracy |
| Training data | 4+ million motion samples |
| Deployment scale | 280 edge devices, 16 scenarios, 2 years |

---

## Critical Assessment

**What this paper actually claims vs. what is achievable:**

The paper demonstrates DensePose estimation in **controlled lab conditions** with paired WiFi/camera training data. Key limitations not prominently stated:

1. **Training data dependency:** Requires massive annotated WiFi+camera paired datasets. Collecting this for a new deployment is a significant undertaking.
2. **Environment dependency:** WiFi sensing is highly environment-specific. Models trained in one room may not generalise to another without retraining or fine-tuning.
3. **Resolution physics:** A 2.4 GHz WiFi signal with 20 MHz bandwidth and 3 antennas provides fundamentally limited spatial resolution. UV coordinates at body-surface resolution require the neural network to extrapolate far beyond what the physics can support — the fine-grained output is model-inferred, not physically measured.
4. **Single-person conditions:** Multi-person performance degrades significantly.

**What this paper inspired (negatively):** RuView (2026) overclaimed these results as directly replicable with commodity hardware without the paired training infrastructure, generating significant backlash when implementations were found to use simulated CSI (`np.random.rand()`) rather than real measurements.

---

## Relevance to Spaxel

DensePose from WiFi defines the theoretical ceiling of what WiFi CSI sensing *could* achieve with sufficient machine learning infrastructure and paired training data. Spaxel's goal — detecting "saltwater bag masses in 3D space" — is far more modest and physically grounded. The paper's value to Spaxel is:

1. Confirms the general approach (phase + amplitude → spatial information) is valid
2. Provides architecture patterns (ResNet encoder, FPN, per-person ROI pooling) applicable to future Spaxel ML enhancements
3. Sets clear expectations: DensePose-quality output requires training infrastructure Spaxel does not have; blob detection (Spaxel's goal) is achievable without it
