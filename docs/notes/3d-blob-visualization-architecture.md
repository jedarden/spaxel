# 3D Blob Visualization Architecture Survey

**Date:** 2026-08-20  
**Purpose:** Document current blob rendering pipeline and BLE identity flow for 3D identity extension work

## Overview

The Spaxel dashboard uses Three.js for real-time 3D visualization of detected persons (blobs) in indoor spaces. The system receives blob positions via WebSocket (10 Hz) and renders them as either generic markers or humanoid figures depending on identity resolution.

## Core Architecture

### 1. Scene Setup

**File:** `dashboard/js/app.js` (Lines 90-310)

- **Renderer:** WebGLRenderer with mobile optimizations (pixel ratio capping, conditional MSAA)
- **Camera:** PerspectiveCamera (60° FOV) at (8, 8, 8) with OrbitControls
- **Scene:** Dark blue background (0x1a1a2e), ambient + directional lighting
- **Grid:** 10×10 meter room with 1m grid divisions

### 2. Blob Rendering Pipeline

**File:** `dashboard/js/viz3d.js` (Lines 712-998)

**Creation Flow:**
```
WebSocket receives blob data → resolveIdentity() → _createBlobObj() → add to scene
```

**Two Representation Modes:**

1. **Unresolved Identity** → Generic marker
   - `SphereGeometry(0.28, 16, 12)` with `MeshPhongMaterial`
   - Color from `BLOB_COLORS` array (6 predefined colors)
   - No animation, static pose

2. **Resolved Identity** → Humanoid figure
   - `SkinnedMesh` with 13 bones (root, pelvis, spine, chest, head, shoulders, elbows, hips, knees)
   - Per-person color from BLE registry or name hash
   - Animated posture: standing/walking/seated/lying
   - Movement timeScale scales with velocity (up to 2.5x)

**Blob Object Structure:**
```javascript
{
  group: THREE.Group,          // Parent container
  humanoid: SkinnedMesh|null,  // Animated figure (if resolved)
  marker: THREE.Mesh|null,     // Sphere (if unresolved)
  rep: 'humanoid'|'marker',    // Active representation
  trail: THREE.Line,           // Footprint trail (60 points)
  pillar: THREE.Line,          // Vertical anchor line to ground
  blobId: number,
  personName: string|null,
  assignedColor: string|null,
  identityResolved: boolean
}
```

### 3. BLE Identity Data Flow

**File:** `dashboard/js/blob-identity.js` (Lines 1-290)

**Data Sources:**
- WebSocket `ble_scan` messages (5 Hz) → update `window.spaxelGetState().bleDevices`
- WebSocket `loc_update` messages (10 Hz) → blob positions with optional `personName` field

**Resolution Precedence:**
1. **Server-resolved identity** (`blob.personName`, `blob.assignedColor`) - highest priority
2. **BLE registry match** via `blob.ble_device` or reverse lookup (`device.blob_id`)
3. **Unmatched** → `identityResolved=false`, renders as gray marker

**Color Assignment:**
- Resolved person: Use `assignedColor` from registry, or hash-based HSL from name
- Unresolved: Neutral gray (`#888888`)
- Applied via `_applyBlobMeshColor()` every frame

**Person Identity Data Structure:**
```javascript
// From WebSocket ble_scan message
{
  addr: "AA:BB:CC:DD:EE:FF",
  name: "iPhone",
  rssi: -62,
  blob_id: 2  // Linked to blob by backend matcher
}

// From WebSocket loc_update message
{
  id: 2,
  x: 3.2, y: 1.1, z: 0.8,
  personName: "Alice",       // Server-resolved identity
  assignedColor: "#4488ff",    // Person's registry color
  ble_device: "AA:BB:CC:DD:EE:FF"  // Optional device reference
}
```

### 4. WebSocket Message Types

**File:** `dashboard/js/websocket.js` (Lines 1-406) + `app.js` (Lines 736-1377)

**Connection:** `/ws/dashboard` endpoint with session cookie auth

**Message Types:**
- `snapshot` - Full state dump on connect/reconnect (blobs, nodes, zones, portals, triggers)
- `loc_update` - Real-time blob positions (10 Hz)
- `ble_scan` - BLE device scan results (5 Hz)
- `zone_change` - Zone occupancy updates
- `portal_change` - Portal crossing events

**Reconnection:** Exponential backoff (1s → 10s max) with jitter. Brief disconnects (<2s) use velocity extrapolation for smooth blob motion.

### 5. Scene Object Management

**File:** `dashboard/js/viz3d.js` (Lines 10-50)

**Maps for Efficient Lookup:**
- `_nodeMeshes` - `Map(mac → THREE.Mesh)` - ESP32 nodes
- `_linkLines` - `Map(id → THREE.Line)` - TX→RX links
- `_blobs3D` - `Map(blobId → blobObj)` - Main blob objects
- `_zoneMeshes` - `Map(zoneID → {mesh, label, occupantsLabel})`
- `_portalMeshes` - `Map(portalID → {mesh, label, flashEndTime})`

### 6. Label System

**File:** `dashboard/js/viz3d.js` (Lines 776-871)

**Floating Name Labels:**
- Sprite-based, positioned 2.0m above humanoid head
- Canvas-rendered with rounded background and person-colored border
- Format: `"{personName} is in {zone}"` or `"{personName}"`
- Auto-scaled font size (36px → 18px range based on text length)
- DepthTest disabled + renderOrder 999 ensures visibility

### 7. Animation System

**File:** `dashboard/js/viz3d.js` (Lines 473-710)

**Animation Clips:**
- `standing` - Static standing pose
- `walking` - 1.2s loop with limb swing (timeScale scaled by velocity)
- `seated` - Hips flexed, knees bent
- `lying` - Full horizontal pose

**Posture Selection:**
- Velocity > 0.25 m/s → `walking`
- Velocity ≤ 0.25 m/s → `standing`

## Extension Points

For adding 3D identity features (distinctive appearance per person), the key extension points are:

### 1. **Humanoid Mesh Customization** (`viz3d.js` Lines 712-998)
   - `_createBlobObj()` - Entry point for blob creation
   - `_createHumanoidGeometry()` - SkinnedMesh bone construction
   - Extension: Add body type variations (height, build) based on person registry

### 2. **Color System Enhancement** (`blob-identity.js` Lines 200-290)
   - `meshColor(resolved)` - Returns color for resolved person
   - `colorForName(name)` - Hash-based HSL generation
   - Extension: Add skin tone mapping, clothing color patterns

### 3. **Label Augmentation** (`viz3d.js` Lines 776-871)
   - `_createBlobLabel(text, color)` - Sprite label creation
   - Extension: Add dynamic labels (emotion, activity), multi-line labels

### 4. **Identity Resolution Pipeline** (`blob-identity.js` Lines 50-150)
   - `resolve(blob, bleDevices)` - Main identity matcher
   - Extension: Add biometric特征 mapping, gait analysis integration

### 5. **WebSocket Data Handler** (`app.js` Lines 1100-1200)
   - `handleLocUpdate(data)` - Processes blob position updates
   - Extension: Add custom fields (pose, emotion, activity) from backend

## Data Flow Summary

```
Mothership (Go)
  ↓ WebSocket (10 Hz)
Dashboard (JS)
  ↓ handleLocUpdate()
viz3d.js
  ↓ updateBlob3D()
Three.js Scene
  ↓ Render Loop
Browser (60 fps)
```

**Key Files:**
- `dashboard/js/app.js` - Scene setup, WebSocket handler
- `dashboard/js/viz3d.js` - 3D rendering, blob creation, animation
- `dashboard/js/blob-identity.js` - Identity resolution, color assignment
- `dashboard/js/websocket.js` - WebSocket connection manager
