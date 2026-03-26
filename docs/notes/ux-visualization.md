# Spaxel Dashboard: UX and Visualization Design

---

## Overview

The Spaxel dashboard serves three purposes:
1. **Live presence visualization** — real-time blob positions overlaid on a floor plan
2. **Device management** — node list, role assignment, OTA status, health
3. **Setup / commissioning** — floor plan editor, node placement, coordinate anchoring

The frontend is a single-page app served by the mothership at `/`. No separate build host needed — assets are baked into the Docker image. Technology choice: **vanilla JS + Canvas 2D** for the visualization layer, HTML/CSS for the management panels. Avoids a heavy framework dependency for something that needs to work on low-powered LAN devices (Raspberry Pi, old mini-PC).

---

## 1. Floor Plan Layer

### How the Floor Plan Gets In

Three options, in order of user-friendliness:

**Option A: Image upload**
User uploads a PNG/JPG/PDF floor plan image. The dashboard scales and positions it behind the canvas. Most accessible for non-technical users — take a phone photo of a paper floor plan.

**Option B: Simple polygon editor**
User draws room outlines by clicking corner points. Stored as a list of `[x, y]` vertices (real-world metres). Rendered as a filled SVG polygon. Good for apartments with simple rectangular rooms.

**Option C: No floor plan**
Dashboard shows an empty grid. Node positions and blob trails render on the grid without a background image. Functional for initial testing.

**Recommended for v1:** Support A and C. Option B can be added later.

### Coordinate System

The floor plan uses a real-world coordinate system in metres, not pixels. Node positions are stored in metres (the `x`, `y`, `z` fields in the device registry). The canvas scales from metres to pixels based on viewport size.

When an image is uploaded, the user calibrates by clicking two known points on the image and entering the real distance between them. This gives a pixels-per-metre scale and an origin offset.

```javascript
// Calibration: user clicks point A, then point B, enters distance in metres
function calibrate(pxA, pxB, distanceMetres) {
    const pxDist = Math.hypot(pxB.x - pxA.x, pxB.y - pxA.y);
    state.pxPerMetre = pxDist / distanceMetres;
    state.originPx = pxA;  // treat click A as real-world (0,0)
}

function metresTopx(mx, my) {
    return {
        x: state.originPx.x + mx * state.pxPerMetre,
        y: state.originPx.y + my * state.pxPerMetre,
    };
}
```

---

## 2. Live Blob Visualization

### Data Flow

```
ESP32 nodes
    │  UDP :4210 (CSI packets)
    ▼
Mothership (Go)
    │  positioning engine → blob list
    ▼
WebSocket /ws (JSON broadcast)
    │  {"blobs": [{"x": 2.1, "y": 3.4, "z": 0.8, "confidence": 0.87}, ...]}
    ▼
Dashboard (Canvas 2D)
    │  requestAnimationFrame loop
    ▼
Floor plan overlay
```

### Blob Rendering

Each blob is drawn as:
- A filled circle, radius proportional to confidence (e.g., `r = 0.3 + confidence * 0.4` metres)
- Opacity also scaled by confidence: `alpha = 0.3 + confidence * 0.5`
- Colour: white-to-red gradient based on confidence (low = cool blue, high = warm red)
- Crosshair at centre

```javascript
function drawBlob(ctx, blob, pxPerMetre) {
    const px = metresTopx(blob.x, blob.y);
    const radiusPx = (0.3 + blob.confidence * 0.4) * pxPerMetre;
    const alpha = 0.3 + blob.confidence * 0.5;

    // Radial gradient
    const grad = ctx.createRadialGradient(px.x, px.y, 0, px.x, px.y, radiusPx);
    grad.addColorStop(0, `rgba(255, 80, 80, ${alpha})`);
    grad.addColorStop(1, `rgba(80, 120, 255, 0)`);

    ctx.beginPath();
    ctx.arc(px.x, px.y, radiusPx, 0, 2 * Math.PI);
    ctx.fillStyle = grad;
    ctx.fill();
}
```

### Trail Rendering

Keep a circular buffer of the last N blob positions (N=60 at 10 fps = 6 seconds of history). Draw the trail as a fading polyline before drawing the current blob position.

```javascript
const TRAIL_LEN = 60;
let trails = {};  // keyed by blob ID (assigned by mothership based on proximity matching)

function updateTrails(blobs) {
    blobs.forEach(blob => {
        if (!trails[blob.id]) trails[blob.id] = [];
        trails[blob.id].push({ x: blob.x, y: blob.y });
        if (trails[blob.id].length > TRAIL_LEN) trails[blob.id].shift();
    });
    // Remove trails for blobs not seen this frame
    const seen = new Set(blobs.map(b => b.id));
    Object.keys(trails).forEach(id => { if (!seen.has(id)) delete trails[id]; });
}
```

The trail is drawn as a series of line segments with opacity decreasing from head to tail:
```javascript
function drawTrail(ctx, trail) {
    trail.forEach((pt, i) => {
        const alpha = (i / trail.length) * 0.4;
        const px = metresTopx(pt.x, pt.y);
        ctx.lineTo(px.x, px.y);
        ctx.strokeStyle = `rgba(255, 200, 100, ${alpha})`;
    });
}
```

### Blob ID Assignment

The mothership needs to track blob identity across frames so the dashboard can maintain trails. Use nearest-neighbour matching — each blob in the new frame is matched to the closest blob in the previous frame within a 1 m threshold. Unmatched blobs get new IDs; unmatched previous blobs are dropped.

This belongs in `positioning/fresnel.go` as a `trackBlobs()` function that runs after `extractBlobs()`.

---

## 3. WebSocket Protocol

Messages from mothership to dashboard:

```json
{
  "type": "blobs",
  "ts": 1712345678901,
  "blobs": [
    { "id": "b1", "x": 2.1, "y": 3.4, "z": 0.8, "confidence": 0.87 }
  ]
}
```

```json
{
  "type": "nodes",
  "nodes": [
    { "mac": "AA:BB:CC:DD:EE:FF", "name": "living-nw", "online": true,
      "x": 0.0, "y": 0.0, "z": 2.4, "role": "tx", "version": "0.2.1",
      "rssi": -62, "last_seen": 1712345678000 }
  ]
}
```

The dashboard subscribes to both. Node positions are used to draw the sensor node overlay on the floor plan. RSSI and last_seen drive health indicators.

---

## 4. Node Overlay

Each node is drawn as a labelled icon on the floor plan at its configured `(x, y)` coordinates:
- Icon: WiFi symbol or antenna SVG, colour-coded green/yellow/red by online status + RSSI
- Label: node name below the icon
- On hover: tooltip showing MAC, firmware version, RSSI, last seen, current role (TX/RX)
- On click: opens device detail panel on the right side

Draw node links (TX→RX pairs) as thin dotted lines connecting nodes. Line opacity scaled by link quality (variance of delta amplitude over last 1 s). This gives a visual sense of which links are "active" vs. saturated/noisy.

---

## 5. Device Management Panel

Side panel (collapsible on mobile) with tabs:

### Nodes Tab

Table of all registered nodes:
| Icon | Name | Role | Version | RSSI | Status | Actions |
|------|------|------|---------|------|--------|---------|
| 🟢 | living-nw | TX | 0.2.1 | -58 | Online | Rename / Config |

- **Rename**: inline edit, PUT /api/devices/{mac}/config `{"node_name": "..."}`
- **Config**: modal showing mothership IP, node name, position (x/y/z input)
- **OTA badge**: shows "Update available" if node.version < latest; click triggers per-node OTA

### Fleet Tab

- Latest firmware version available
- Button: "Update All" — sends OTA command to all nodes via MQTT
- OTA progress bars per node (driven by MQTT events from nodes during flash)

### Links Tab

Table of active TX→RX pairs with link quality metrics:
- Mean delta amplitude (current - baseline)
- Variance (rolling)
- Sample rate (packets/sec)

Useful for debugging node placement — a link with near-zero delta variance may be blocked.

---

## 6. Floor Plan Editor (Setup Mode)

Accessed via a "Edit Layout" button. Steps:

1. **Upload floor plan image** (or skip)
2. **Calibrate**: click two points, enter distance
3. **Place nodes**: drag node icons to their physical positions on the floor plan
4. Click a node icon → enter height (z) in a field
5. **Save**: positions stored via PUT /api/devices/{mac}/config `{"x": ..., "y": ..., "z": ...}`

The editor reuses the same canvas as the live view, but with draggable handles instead of blob animation. A `mode` flag switches between `"live"` and `"edit"` rendering.

```javascript
let mode = 'live';  // or 'edit'

canvas.addEventListener('mousedown', e => {
    if (mode === 'edit') {
        const hit = findNodeAtPx(e.offsetX, e.offsetY);
        if (hit) startDrag(hit);
    }
});
```

---

## 7. Canvas vs. WebGL Decision

**Canvas 2D** is the right choice for v1:
- Zero dependencies
- Sufficient performance for < 20 blobs at 10 fps
- Easy to reason about coordinate transforms
- Works on all browsers including mobile Safari

**When to consider WebGL / Three.js:**
- 3D visualization (showing blob height, Z axis)
- More than 50 simultaneous blobs
- Heat map texture (tens of thousands of voxels rendered per frame)

A future 3D view using Three.js would render the voxel grid as a semi-transparent point cloud, with blobs as glowing spheres at their Z height. The existing floor plan image maps as a texture on the ground plane at Z=0.

---

## 8. Responsive Layout

```
┌──────────────────────────────────────────────────────┐
│  Spaxel   [Edit Layout]  [Settings]           ●Live  │
├─────────────────────────────────┬────────────────────┤
│                                 │                    │
│                                 │  Nodes (3 online)  │
│     Floor Plan Canvas           │  ● living-nw  TX   │
│        (blob overlay)           │  ● kitchen-ne RX   │
│                                 │  ● hall-sw    RX   │
│                                 │                    │
│                                 │  [Update All]      │
│                                 │                    │
└─────────────────────────────────┴────────────────────┘
```

On narrow screens (< 768 px), the panel collapses behind a hamburger. The canvas fills the full viewport. A floating status bar at the bottom shows node count and blob count.

---

## 9. Implementation Sequence

1. **Static skeleton** — `index.html` with canvas + side panel, no real data
2. **WebSocket plumbing** — connect to `/ws`, parse JSON, log to console
3. **Blob rendering** — draw circles from real WebSocket data
4. **Node overlay** — draw node positions from `/api/devices`
5. **Floor plan image** — upload + calibration + coordinate transform
6. **Trails + blob IDs** — requires mothership to add blob tracking
7. **Edit mode** — drag-to-place nodes, save positions
8. **Device management panel** — rename, OTA, link quality table
9. **Mobile polish** — collapsed panel, touch drag support

---

## 10. Open Questions

- **3D vs. 2D**: Current visualisation is top-down (XY plane only). The Z coordinate (height) is used for positioning but not yet visualised. A future enhancement is a side-view panel showing blob height distribution — useful for distinguishing a person standing from a person sitting.
- **Heatmap mode**: Instead of blob circles, render the raw voxel grid as a 2D heatmap (hot colours = high weight). More useful for debugging the algorithm than for end-user display, but could be a toggleable view.
- **Presence counter**: A simple integer in the top corner ("2 people detected") may be more useful to end users than the spatial blob view. Display confidence-filtered blob count: `blobs.filter(b => b.confidence > 0.6).length`.
- **Alert webhooks**: A setting to POST to a URL when blob count crosses a threshold (e.g., 0 → 1 = room occupied). Useful for home automation integration without requiring full MQTT on the client side.
