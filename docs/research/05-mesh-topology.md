# Multi-Node Mesh Topology

## Link Count

A network of N nodes creates N×(N−1) directed links (each ordered TX→RX pair). Every bidirectional link is two directed links.

| Nodes (N) | Directed links | Unique bidirectional links |
|---|---|---|
| 3 | 6 | 3 |
| 4 | 12 | 6 |
| 5 | 20 | 10 |
| 6 | 30 | 15 |
| 8 | 56 | 28 |

Each link is a bistatic radar with the two nodes as TX and RX. A person's position creates an ellipsoidal constraint for each link. More independent links → tighter constraint intersection → better localization.

---

## Why Link Diversity Matters

Each link's first Fresnel zone ellipsoid has a different orientation and coverage area. A person at position P may be:
- Inside the FFZ of links whose paths cross through P
- Outside the FFZ of tangential links

Using links with many different orientations means some will always be sensitive to any given person location. This reduces blind spots — positions where no single link would show perturbation.

Localization accuracy improvement scales roughly as √N_links for independent measurements (analogous to averaging independent noise sources).

### Coverage per Link

One link's effective sensing volume is approximately its first Fresnel zone: for a 5 m link at 2.4 GHz, this is an ellipsoid with semi-major axis 2.5 m and semi-minor axis ~0.4 m at the midpoint.

Rule of thumb: **one node per 50–70 m²** for room-scale presence detection. For sub-metre localization: 4–8 nodes per sensing region.

---

## Optimal Node Placement

### Principles

- **Non-collinear**: All nodes on one line → degenerate geometry; poor localization of targets along the line
- **Distributed perimeter**: Nodes at room corners/walls create links crossing the interior from many angles
- **Angular diversity**: Prefer a uniform distribution of link directions (0°, 45°, 90°, 135°…)
- **Avoid clustering**: Two nodes close together provide nearly identical angular information

### Height Variation

Nodes at different heights are essential for meaningful Z-axis detection:
- Most indoor links are horizontal (nodes mounted at similar heights) → poor elevation angle diversity
- Deploying some nodes high (2.0–2.5 m) and some low (0.3–1.0 m) creates vertical Fresnel zone components
- A person standing vs. crouching will show differently across high/low node pairs
- Z-accuracy with mixed heights: ~0.5–1.5 m (significantly worse than XY at ~0.5 m)

### Minimum Counts by Task

| Task | Minimum nodes | Recommended |
|---|---|---|
| Presence detection (binary) | 1 link (2 nodes) | 2–3 links |
| Coarse 2D localization | 3 nodes | 4–6 nodes |
| Sub-metre 2D localization | 4–6 nodes | 6–8 nodes |
| 3D localization | 6+ nodes | 8+ nodes |
| Multiple people tracking | 4 per person | 6–12 nodes per person |

### Geometric Dilution of Precision (GDOP)

Borrowed from GPS — quantifies how geometry affects localization accuracy for a given target location:
- GDOP < 2: good geometry, accurate localization
- GDOP > 5: poor geometry, avoid this target location if possible
- GDOP varies across the room; worst near corners (far from all nodes) and along node-to-node lines

---

## TX/RX Role Assignment

### Multistatic Mesh Modes

| Mode | Description |
|---|---|
| **Pure TX** | Node transmits probe packets; does not capture CSI |
| **Pure RX** | Node captures CSI from TX nodes; does not transmit probes |
| **Both** | Alternates TX and RX; used when node count is low |

The mothership assigns roles dynamically via MQTT config push:

```json
{
  "role": "rx",
  "listen_macs": ["aa:bb:cc:dd:ee:ff", "11:22:33:44:55:66"]
}
```

### Probe Packet Strategy

TX nodes send probe requests or null data packets on a schedule. Rate: 20–50 Hz is sufficient for motion detection; higher rates improve DFS resolution but increase channel contention.

Coordination: the mothership can stagger TX schedules across nodes to prevent simultaneous transmissions (which would corrupt CSI at the RX nodes).

---

## Node Self-Positioning via MDS

If node positions are not known a priori, Multidimensional Scaling (MDS) can recover relative coordinates from pairwise distance estimates.

### MDS-MAP Algorithm

1. Measure pairwise ToF (or RSSI-based ranging) between all node pairs → distance matrix D_ij
2. Construct double-centred Gram matrix: `B = −½ · H · D² · H`, where `H = I − (1/N)·11^T`
3. Eigendecompose B; take top 2 (or 3) eigenvectors × √eigenvalues as relative coordinates
4. Anchor to ≥3 reference points with known positions to get absolute coordinates

### Ranging Accuracy on ESP32

| Method | Range resolution |
|---|---|
| RSSI path loss model | ~1–3 m (very noisy) |
| ToF from CIR peak (20 MHz BW) | c/(2B) = 7.5 m — too coarse |
| ToF from CIR peak (40 MHz BW) | c/(2B) = 3.75 m — still coarse |
| ToF + DFS refinement (Widar2.0) | ~0.05 m — requires post-processing |

**Practical recommendation**: Measure node positions manually with a tape measure for initial deployment. MDS self-positioning is a future enhancement once the system is validated. Use the mothership floor plan editor to pin nodes to known coordinates.

---

## Apartment Perimeter Deployment

For a typical apartment deployment with nodes at perimeter walls and mixed heights:

```
Top-down view (8 nodes):         Side view (one wall):

  N1 ──── N2 ──── N3              2.0m  N1
  │                │
  │    interior    │              1.0m
  │                │
  N8              N4              0.3m        N3
  │                │
  N7 ──── N6 ──── N5
```

- 4 corner nodes at 2.0 m height → good horizontal coverage of interior
- 4 mid-wall nodes at 0.3–1.0 m → elevation diversity, improves Z detection
- 56 directed links from 8 nodes → excellent interior coverage from all angles
- Blind spots: corners very close to nodes (over-perturbation, hard to localise precisely)
- Coverage: reliable detection throughout the interior; sub-metre 2D localisation feasible
