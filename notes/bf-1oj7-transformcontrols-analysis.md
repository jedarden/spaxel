# TransformControls for Live Node Dragging - Implementation Analysis

## Finding

TransformControls functionality for dragging live registered nodes in the 3D dashboard is **already fully implemented** in the live dashboard via the Placement module.

## Implementation Details

### 1. Placement Module (dashboard/js/placement.js)

The Placement module provides TransformControls for ALL nodes (both live and virtual):

- **Initialization** (lines 238-275): Creates TransformControls instance
- **Node Selection** (lines 279-311): `selectNode(mac)` attaches TransformControls to any node mesh
- **Pointer Events** (lines 315-354): Click-to-select functionality via raycasting
- **Position Persistence** (lines 419-427): `saveNodePosition(mac, x, y, z)` persists to API
- **Room Bounds Clamping** (lines 261-274): Constrains dragging to room dimensions

### 2. Live Dashboard (dashboard/js/app.js)

The live dashboard properly initializes and uses Placement:

- **Initialization** (line 285-287): `Placement.init(scene, camera, renderer, controls)`
- **Registry State** (line 1034): `Placement.handleRegistryState(msg)` on WebSocket messages
- **UI Selection** (line 1575): `Placement.selectNode(el.dataset.mac)` when clicking node list items

### 3. Viz3D Module (dashboard/js/viz3d.js)

Viz3D creates and manages node meshes:

- **Node Registry** (lines 263-294): `applyNodeRegistry(nodes)` creates meshes for all nodes
- **Position Setting** (line 291): Sets mesh positions from registry_state data (pos_x, pos_y, pos_z)
- **Mesh Access** (line 3354): `getNodeMesh(mac)` returns mesh by MAC address

### 4. Data Flow

1. WebSocket receives `registry_state` message with node positions
2. `Viz3D.handleRegistryState()` creates/updates node meshes
3. `Placement.handleRegistryState()` stores node MACs in `_nodeMACs` array
4. User clicks on node mesh → `Placement.onPointerUp()` raycasting finds mesh
5. `Placement.selectNode()` attaches TransformControls to the mesh
6. User drags node → TransformControls moves the mesh
7. On drag end → `Placement.saveNodePosition()` persists new position to API

## Differences from Simulator

The pre-deployment simulator (simulate.js) creates its own separate TransformControls instance, which may have created the impression that the live dashboard lacked this functionality. However, the live dashboard uses Placement's TransformControls, which is a more integrated and feature-rich implementation that includes:

- GDOP coverage overlay integration
- Room bounds enforcement
- API persistence
- Support for both live and virtual nodes

## Conclusion

The functionality described in bead bf-1oj7 is **already fully implemented and operational**. The TransformControls for dragging live registered nodes in the operational dashboard has been available since the Placement module was integrated into the live dashboard.

## Verification

To verify this is working:
1. Open the live dashboard at `/live`
2. Click on any node in the 3D scene (RGB transform controls should appear)
3. Drag the node using the RGB arrows
4. Position is automatically persisted to the mothership API

No additional implementation work is required for this bead.
