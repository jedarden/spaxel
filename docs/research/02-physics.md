# Physics of Human-WiFi Interaction

## Human Tissue as a Dielectric at 2.4 GHz

Human tissue is electrically characterised by complex permittivity: `ε* = ε' − jε''`.

At 2450 MHz:

| Tissue | Relative Permittivity (ε') | Conductivity (σ, S/m) | Penetration Depth |
|---|---|---|---|
| Muscle | 52–54 | 1.8–2.4 | ~1.5–2 cm |
| Skin (wet) | 42 | 1.6 | ~2–3 cm |
| Skin (dry) | 37–38 | 1.5 | ~2–3 cm |
| Fat | 5 | 0.1 | ~9–12 cm |
| Blood | 58–60 | 2.5 | ~1.2 cm |
| Whole body (avg) | ~38.5 | ~2.4 | — |

Sources: Gabriel et al. 4-Cole-Cole model; IFAC-CNR Dielectric Properties Database; IT'IS Foundation.

At 2.4 GHz, the free-space wavelength is λ = c/f = 3×10⁸ / 2.4×10⁹ ≈ **12.5 cm**. Inside human tissue, the effective wavelength is shortened by √ε' ≈ 7×, to ~1.7 cm inside muscle. The body is therefore electrically large — many wavelengths in extent — making it an effective scatterer.

---

## Why 2.4 GHz Interacts Strongly with the Body

### 1. Polar Water Absorption

Liquid water's orientational polarisation relaxation frequency falls in the low-GHz range. At 2.4 GHz, water molecules cannot fully follow the rapidly oscillating field, producing dielectric loss (energy absorbed as heat). The human body is approximately 60–70% water by mass (higher in muscle, lower in fat), making it a strong absorber.

### 2. High Ionic Conductivity

Bodily fluids (blood, interstitial fluid, cytoplasm) contain dissolved ions. At 2.4 GHz, σ ≈ 2.4 S/m for average tissue — roughly a million times higher than distilled water. This drives significant conduction current and Ohmic loss.

### 3. Body as a Large Obstacle

Empirical measurements show that human body shadowing causes additional link attenuation of **3–12 dB** depending on orientation and environment. A frontal cross-section presents a cylinder of ~30–50 cm diameter; a side view is ~20–25 cm. Both are multiple wavelengths wide, limiting diffraction around the body — most incident energy is absorbed.

### 4. Penetration Depth (Skin Depth)

`δ = 1/α` where `α = (2πf/c) · Im(√ε*)`.

For muscle at 2.4 GHz: δ ≈ 1.5–2 cm. Signal amplitude falls to 1/e of its surface value within ~2 cm of entering tissue — the body is essentially **opaque** to 2.4 GHz.

---

## Effect on Multipath

Indoor WiFi propagation involves many multipath components: direct LOS, reflections off walls/ceiling/floor/furniture, diffracted paths. The received signal is the superposition of all these.

When a human body enters the environment:

- **New scatter paths are created**: The body surface reflects incident energy in all directions, creating new multipath components that add to (or subtract from) existing ones.
- **Existing paths are perturbed**: Paths passing close to or through the body are attenuated and phase-shifted.
- **LOS is shadowed**: If the body is between TX and RX, the direct path is attenuated by 3–12 dB.
- **Dynamic multipath**: As the body moves, the lengths of all affected paths change, producing time-varying CSI amplitude and phase variations.

Sensitivity is highest when path length changes are on the order of λ/2 ≈ 6.25 cm, which produces ~π radians of phase shift and transitions between constructive and destructive interference.

---

## Fresnel Zone Theory

### Definition

The n-th Fresnel zone is the set of all points P such that:

```
|TX→P| + |P→RX| = |TX→RX| + n·λ/2
```

This defines an ellipsoid with TX and RX at its foci.

### First Fresnel Zone Radius

The maximum radius of the first Fresnel zone at a point distance d₁ from TX and d₂ from RX:

```
r₁ = √(λ · d₁ · d₂ / (d₁ + d₂))
```

At the midpoint of the link (d₁ = d₂ = D/2):

```
r₁_max = (1/2) · √(λ · D)
```

For 2.4 GHz (λ = 0.125 m) at various link distances:

| Link distance D | r₁_max |
|---|---|
| 2 m | 0.25 m |
| 5 m | 0.40 m |
| 10 m | 0.56 m |
| 20 m | 0.79 m |

A human body (diameter ~0.4–0.5 m) occupies a significant fraction of the first Fresnel zone on all typical indoor links of 2–10 m.

### The Odd/Even Zone Effect

Fresnel zones alternate between constructive and destructive contributions:

- **Odd zones** (1st, 3rd, 5th…): contributions arrive roughly in-phase with LOS — constructive.
- **Even zones** (2nd, 4th, 6th…): contributions arrive roughly anti-phase — destructive.

As a person moves through successive Fresnel zone boundaries (each λ/2 of path length change = one zone crossing), CSI amplitude varies sinusoidally.

Key signatures:
- **Person inside FFZ**: Strong amplitude perturbation, sinusoidal variation with movement
- **Person on FFZ boundary**: Maximum rate of change in CSI
- **Person outside FFZ**: Weaker perturbation via diffraction

### Fresnel Zone Localization

In a multi-link system, each TX-RX pair defines a set of Fresnel zone ellipsoids. If a person's presence shifts the CSI in a way indicating they are within the n-th Fresnel zone of a given link, this constrains the person to an ellipsoidal shell.

With multiple links, the constraints intersect:
- 1 link: an ellipsoidal shell (no position constraint, only distance sum constraint)
- 2 links: intersection of 2 ellipsoids → typically 2–4 candidate points (ambiguity)
- 3+ links: sufficient for a unique solution in most geometries

Phase measurement precision determines which Fresnel zone the person is in. A phase resolution of 0.1 rad translates to path length precision of ≈ λ/(4π) × 0.1 ≈ 1 mm — but hardware phase noise is much larger in practice (~0.5–2 rad per sample), requiring averaging.

---

## Implication for Spaxel

The human body as a "saltwater bag" is an accurate physical model:

- High water content (~70%) → strong 2.4 GHz absorption
- High ionic conductivity → opaque to WiFi at skin depth ~2 cm
- Electrically large body (multiple wavelengths) → significant scatter and shadow
- First Fresnel zone radii match human body dimensions at 2–10 m indoor links

This means WiFi CSI can reliably detect the presence of a human-sized conductive mass. What it cannot do is resolve internal structure — limbs, posture, fine geometry. The detectable unit is the blob, not the body.
