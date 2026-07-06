> ⚠️ **Secondary — folded into the consolidated inventory.** Detailed child results of
> `notes/bf-26ta-findings.md` (now secondary). The authoritative JS/TS blob-creation inventory
> is **`notes/bf-1bmg-js-ts.md`** (a blessed child of `notes/bf-1q3m-consolidated.md`, the
> single source of truth). Retained for provenance only.

---

# bf-26ta: JavaScript Blob-Shaped Object Literal Search Results (secondary — see banner above)

## Task Summary
Search all JavaScript files (.js and .jsx) for blob-shaped object literals using the pattern defined in bf-3aij.

## Search Scope
- **Total JavaScript files searched:** 80 (.js and .jsx files)
- **Files excluded:** node_modules, dist, .beads directories
- **Pattern matched:** Object literals with blob structure (position fields, identity fields, tracking fields)

## Search Results Summary

### Blob-Shaped Object Literals Found: **14 instances across 5 files**

> **Re-verified 2026-07-06:** previously reported "25 across 7 files" over-counted
> `quick-actions.test.js` (16 reported, 5 actual literals) and listed only 4 of the 6
> `ambient.test.js` sites. Corrected below.

---

## Detailed Findings

### 1. **dashboard/js/state.js** - Core State Management

#### File: `dashboard/js/state.js:290-295`

**Primary blob creation pattern in state management**

```javascript
// Line 290-295: Default blob initialization in updateBlob()
appState.blobs[id] = {
    id: id,
    personName: undefined,
    assignedColor: undefined,
    identityResolved: undefined
};
```

**Context:** This is the canonical blob object creation pattern in the central state management system. When a blob is first created, it's initialized with identity fields set to undefined, then populated via `Object.assign()` with server-provided data (position, velocity, confidence).

**Pattern Type:** State initialization object literal

---

### 2. **dashboard/js/ambient.test.js** - Ambient Renderer Tests

#### File: `dashboard/js/ambient.test.js`

**Multiple blob test fixtures for 3D ambient rendering tests**

```javascript
// Line 124-134: Full blob with identity fields
blobs: [{
    id: 1,
    x: 2,
    y: 2,
    z: 0,
    confidence: 0.8,
    person: 'Alice',
    personName: undefined,
    assignedColor: undefined,
    identityResolved: undefined
}]

// Line 276-284: Minimal blob with identity fields
blobs: [{
    id: 1,
    x: 2,
    y: 2,
    z: 0,
    personName: undefined,
    assignedColor: undefined,
    identityResolved: undefined
}]

// Line 642-648: Position-only blob with confidence
blobs: [{
    id: 1,
    x: 1,
    y: 1,
    z: 0,
    confidence: 0.8
}]

// Line 659-665: Position blob (different coordinates)
blobs: [{
    id: 1,
    x: 3,
    y: 3,
    z: 0,
    confidence: 0.8
}]
```

- **Line 694-700:** Position blob at (1,1) — lerp source fixture
```javascript
blobs: [{id: 1, x: 1, y: 1, z: 0, confidence: 0.8}]
```

- **Line 708-714:** Position blob at (3,3) — lerp target fixture
```javascript
blobs: [{id: 1, x: 3, y: 3, z: 0, confidence: 0.8}]
```

**Context:** Test fixtures for the 3D ambient floor plan renderer. These blobs represent detected people in 2D floor space for testing the ambient visualization system.

**Pattern Types:**
- Full blob with position + confidence + identity
- Minimal blob with position + identity
- Position-only blob with confidence

---

### 3. **dashboard/js/quick-actions.test.js** - Quick Actions Tests

#### File: `dashboard/js/quick-actions.test.js`

**Blob objects for testing spatial quick actions menu**

```javascript
// Line 208-214: SpaxelState blob assignment
window.SpaxelState.set('blobs', 123, {
    id: 123,
    person: 'Alice',
    x: 2,
    y: 0,
    z: 3
});

// Line 316-322: Minimal blob for menu testing
const blob = {
    id: 123,
    person: 'Alice',
    x: 2,
    y: 0,
    z: 3
};

// Line 365-373: Zone object (NOT a blob - has dimensions)
const zone = {
    id: 1,
    name: 'Kitchen',
    x: 0,
    y: 0,
    z: 0,
    w: 4,
    d: 3,
    h: 2.5
};

// Line 470-476: Blob for camera follow testing
const blob = {
    id: 123,
    person: 'Alice',
    x: 2,
    y: 0,
    z: 3
};

// Line 513-519: Blob for follow indicator testing
const blob = {
    id: 123,
    person: 'Alice',
    x: 2,
    y: 0,
    z: 3
};

// Line 678: Minimal inline blob (compressed format)
const blob = { id: 123, x: 2, y: 0, z: 3, personName: undefined, assignedColor: undefined, identityResolved: undefined };
```

**Context:** Test fixtures for spatial quick actions (context menu when clicking on blob elements in the UI). Tests menu items, camera follow mode, and identity assignment features.

**Pattern Types:**
- Minimal blob with id + position + person
- Identity field variations (personName, assignedColor, identityResolved)
- **Note:** Zone object included for comparison (not a blob due to dimensions w, d, h)

---

### 4. **dashboard/js/replay.test.js** - Session Replay Tests

#### File: `dashboard/js/replay.test.js:101-113`

**Extended blob with velocity and posture fields**

```javascript
blobs: [
    {
        id: 1,
        x: 2.5,
        y: 1.3,
        z: 0.8,
        vx: 0.1,           // Velocity X
        vy: 0.0,           // Velocity Y
        vz: 0.0,           // Velocity Z
        weight: 0.85,     // Detection confidence
        posture: 'standing' // Body posture
    }
]
```

**Context:** Mock API response for session replay functionality. This represents the most complete blob structure with velocity vectors and posture information for historical replay scenarios.

**Pattern Type:** Extended blob with velocity + posture fields

---

### 5. **dashboard/js/websocket.js:167-173** - WebSocket Message Handler

**Blob state capture for position extrapolation**

```javascript
// Line 167-173: Capture blob position and velocity
_blobStates.set(b.id, {
    x: b.x,
    z: b.z,
    vx: b.vx || 0,
    vz: b.vz || 0,
    ts: Date.now()
});
```

**Context:** This captures minimal blob state (position + velocity + timestamp) for dead reckoning extrapolation when WebSocket connection is lost. The system extrapolates blob positions based on last known velocity.

**Pattern Type:** Derived blob state object (minimal extrapolation state)

---

### 6. **dashboard/js/ambient_renderer.js:624-647** - Blob Rendering Logic

**Blob field access for rendering (not object creation)**

```javascript
// Line 624-626: Confidence-based radius calculation
const confidence = blob.confidence || 0.5;
const radius = 10 + (confidence * 8); // 10-18px

// Line 632-633: Person name field fallback chain
const personName = blob.personName || blob.person_label || blob.person || null;

// Line 636-640: Color assignment from blob fields
if (blob.assignedColor) {
    blobColor = blob.assignedColor;
} else if (personName) {
    blobColor = getPersonColor(personName);
}

// Line 645-646: Identity resolution check
} else if (blob.identityResolved === false) {
    displayName = '?'; // Explicitly unresolved
}
```

**Context:** These are blob field access patterns (not object creation), showing how the renderer consumes blob objects for 3D visualization.

**Pattern Type:** Blob field consumption (not creation)

---

## Blob Field Usage Summary

### Core Fields (Always Present)
- **id** - Numeric/string identifier for the blob
- **x, y, z** - 3D world-space position (metres)

### Extended Fields (Commonly Used)
- **confidence/weight** - Detection confidence [0-1]
- **person/personName** - Associated person identity
- **assignedColor** - Hex color for dashboard rendering
- **identityResolved** - Boolean flag for identity confirmation

### Velocity Fields (Optional)
- **vx, vy, vz** - Velocity components (m/s) for extrapolation

### Posture Field (Rare)
- **posture** - Body posture state ('standing', 'walking', 'seated', 'lying')

### Legacy/Deprecated Fields
- **person_label** - Superseded by personName
- **personColor** - Superseded by assignedColor

---

## Blob Creation Patterns by Category

### 1. State Initialization (Most Common)
**Location:** `dashboard/js/state.js:290`
```javascript
appState.blobs[id] = {
    id: id,
    personName: undefined,
    assignedColor: undefined,
    identityResolved: undefined
};
```
**Purpose:** Initialize new blob state entry before populating with server data

### 2. Test Fixtures (High Variance)
**Locations:** `dashboard/js/ambient.test.js`, `dashboard/js/quick-actions.test.js`
- Minimal: `{id, x, y, z}`
- Extended: `{id, x, y, z, confidence, person, personName, assignedColor, identityResolved}`
- Velocity: `{id, x, y, z, vx, vy, vz, weight, posture}`

**Purpose:** Test data for rendering, menu interactions, and replay features

### 3. API Mocks
**Location:** `dashboard/js/replay.test.js:101`
```javascript
{
    id: 1,
    x: 2.5,
    y: 1.3,
    z: 0.8,
    vx: 0.1,
    vy: 0.0,
    vz: 0.0,
    weight: 0.85,
    posture: 'standing'
}
```
**Purpose:** Simulate server blob responses for session replay API

### 4. Extrapolation State (Derived)
**Location:** `dashboard/js/websocket.js:167`
```javascript
{
    x: b.x,
    z: b.z,
    vx: b.vx || 0,
    vz: b.vz || 0,
    ts: Date.now()
}
```
**Purpose:** Capture minimal state for dead reckoning during disconnection

---

## Comparison with Related Structures

### Zone Objects (Similar but Different)
**Example from `quick-actions.test.js:365`:**
```javascript
const zone = {
    id: 1,
    name: 'Kitchen',
    x: 0,
    y: 0,
    z: 0,
    w: 4,    // Width
    d: 3,    // Depth
    h: 2.5   // Height
};
```

**Distinguishing Features:**
- Zones have **dimensions** (w, d, h) - blobs don't
- Zones have **name** field - blobs have **person/personName**
- Zones represent **static spaces** - blobs represent **moving people**

### Node Objects
**Pattern:** Use `mac` instead of `id` for identification

### Event Objects
**Pattern:** Include `timestamp_ms`, `type`, `severity` instead of position fields

---

## Files Summary

| File | Blob Count | Pattern Type | Context |
|------|------------|--------------|---------|
| `dashboard/js/state.js` | 1 | State initialization | Central state management |
| `dashboard/js/ambient.test.js` | 6 | Test fixtures | 3D ambient rendering tests |
| `dashboard/js/quick-actions.test.js` | 5 | Test fixtures | Spatial menu tests |
| `dashboard/js/replay.test.js` | 1 | API mock | Session replay tests |
| `dashboard/js/websocket.js` | 1 | Derived state | Position extrapolation |
| **Total** | **14** | **Multiple patterns** | **5 files** |

---

## Key Findings

### Blob Creation Hotspots
1. **State management** (`state.js:290`) - Single canonical creation pattern
2. **Test files** (`ambient.test.js`, `quick-actions.test.js`) - High variance for coverage
3. **API mocks** (`replay.test.js`) - Full blob structure simulation

### Field Usage Patterns
- **Position fields** (x, y, z) are mandatory in all blob objects
- **Identity fields** (person, personName, assignedColor, identityResolved) appear in ~60% of instances
- **Velocity fields** (vx, vy, vz) appear only in replay scenarios
- **Confidence/weight** appears in ~40% of instances

### Architecture Insights
1. **Centralized creation:** Blob state is managed centrally in `state.js`
2. **Test diversity:** Tests cover minimal to full blob structures
3. **Field fallbacks:** Renderers use fallback chains (personName → person_label → person)
4. **Extrapolation support:** Minimal blob state used for dead reckoning during disconnections

---

## Conclusion

The JavaScript codebase contains **14 blob-shaped object literals** across 5 files, with patterns ranging from minimal state initialization to comprehensive test fixtures. The canonical creation pattern is in `state.js:290`, while test files provide diverse blob structures for comprehensive coverage of rendering, interaction, and replay functionality.

**Blob detection pattern:** Look for objects with `{id, x, y, z}` core structure, optionally extended with identity, velocity, or confidence fields.
