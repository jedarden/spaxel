# Role Assignment and Node Registration Flow

**Research Date:** 2026-08-24  
**Purpose:** Understand role assignment, role locking, and API endpoint divergence for regression test development

---

## Overview

The Spaxel system maintains two distinct role representations:

1. **Registry Roles** (Persistent) - Stored in SQLite, represent operator intent
2. **Computed Roles** (Runtime) - Calculated by the optimiser for fleet health

**Critical Insight:** Role locking prevents the optimiser from overriding operator-set roles (ADR-003 / `bf-4kdww`).

---

## 1. How Nodes Are Created with Pinned/Passive Roles

### Virtual Node Creation (`POST /api/nodes/virtual`)

Virtual nodes (used for planning and coverage simulation) are created directly with a `'virtual'` role:

```go
// mothership/internal/fleet/registry.go:100-108
func (r *Registry) AddVirtualNode(mac, name string, x, y, z float64) error {
    now := time.Now().UnixNano()
    _, err := r.db.Exec(`
        INSERT INTO nodes (mac, name, role, pos_x, pos_y, pos_z, virtual, first_seen_at, last_seen_at)
        VALUES (?, ?, 'virtual', ?, ?, ?, 1, ?, ?)
    `, mac, name, x, y, z, now, now)
    return err
}
```

### Physical Node Role Assignment

Physical nodes get their role through multiple paths:

1. **Hello message** - Node reports its current role on connect
2. **Fleet optimiser** - Calculates optimal role assignment
3. **Operator override** - Manual role set via API
4. **Self-healing** - Reacts to node failures

---

## 2. RoleAssignmentFor: The Chokepoint

**Location:** `mothership/internal/fleet/registry.go:234-242`

This is the **single chokepoint** every role-assignment path must go through before sending a role to a node.

```go
func (r *Registry) RoleAssignmentFor(mac, proposed string) (string, string) {
    role := proposed
    if r.IsNodeRoleLocked(mac) {
        if locked, err := r.GetNodeRole(mac); err == nil && locked != "" {
            role = locked  // Override: use locked role instead of proposed
        }
    }
    return role, r.PassiveBSSIDFor(mac, role)
}
```

### How It Works

1. **Input:** Takes a MAC address and a proposed role
2. **Role Lock Check:** If the node's role is locked (`role_locked=1`), retrieves the stored role
3. **Override:** Returns the locked role instead of the proposed role
4. **Passive BSSID:** Also returns the passive BSSID (for passive nodes) to prevent CSI filter drops

### Callers That Must Route Through RoleAssignmentFor

Every `SendRoleToMAC` call site must use this:

- **Self-healing** (`selfheal.go`)
- **Fleet optimiser** (`optimiser.go`)
- **Healer** (`healer.go`)
- **Reconnect re-push** (grace period handling)
- **Operator override** (`manager.go`)

**Why:** Without this chokepoint, an operator-set `role=passive` would be overwritten by `tx_rx` on the next optimiser cycle (60s thrashing). See ADR-003 / `bf-4kdww`.

---

## 3. Role Locking Mechanism

### Setting the Lock

```go
// mothership/internal/fleet/registry.go:206-213
func (r *Registry) SetNodeRoleLocked(mac string, locked bool) error {
    v := 0
    if locked {
        v = 1
    }
    _, err := r.db.Exec(`UPDATE nodes SET role_locked=? WHERE mac=?`, v, mac)
    return err
}
```

### Checking the Lock

```go
// mothership/internal/fleet/registry.go:217-223
func (r *Registry) IsNodeRoleLocked(mac string) bool {
    var locked int
    if err := r.db.QueryRow(`SELECT role_locked FROM nodes WHERE mac=?`, mac).Scan(&locked); err != nil {
        return false
    }
    return locked == 1
}
```

### Database Schema

```sql
CREATE TABLE nodes (
    mac              TEXT PRIMARY KEY,
    role             TEXT    NOT NULL DEFAULT 'rx',
    role_locked      INTEGER NOT NULL DEFAULT 0,  -- Operator-set protection
    passive_bssid    TEXT    NOT NULL DEFAULT '',  -- AP BSSID for passive nodes
    previous_role    TEXT    NOT NULL DEFAULT '',  -- For reconnect grace period
    -- ... other fields ...
);
```

---

## 4. How Manager.OverrideRoleWithBSSID Pins Operator Roles

**Location:** `mothership/internal/fleet/manager.go`

```go
func (m *Manager) OverrideRoleWithBSSID(mac, role, passiveBSSID string) error {
    // Store the passive BSSID first (for passive nodes)
    if role == "passive" && passiveBSSID != "" {
        if err := m.registry.SetNodePassiveBSSID(mac, passiveBSSID); err != nil {
            return err
        }
    }

    // Set the role
    if err := m.registry.SetNodeRole(mac, role); err != nil {
        return err
    }

    // CRITICAL: Pin it, or the next optimiser cycle reverts it. See bf-4kdww.
    if err := m.registry.SetNodeRoleLocked(mac, true); err != nil {
        return err
    }

    // Send to node via WebSocket
    notifier.SendRoleToMAC(mac, role, passiveBSSID)
    return nil
}
```

### Why Both SetNodeRole AND SetNodeRoleLocked?

1. **SetNodeRole** - Stores the role value in the database
2. **SetNodeRoleLocked** - Protects that value from optimiser overrides

Without the lock, the optimiser would recalculate and re-assign its preferred role on the next cycle (every 60s by default).

---

## 5. API Endpoint Behavior

### `/api/nodes` - List All Nodes

**Handler:** `mothership/internal/fleet/handler.go:112-122`

```go
func (h *Handler) listNodes(w http.ResponseWriter, r *http.Request) {
    nodes, err := h.mgr.registry.GetAllNodes()
    if err != nil {
        http.Error(w, "internal error", http.StatusInternalServerError)
        return
    }
    if nodes == nil {
        nodes = []NodeRecord{}
    }
    writeJSON(w, nodes)
}
```

**Returns:** Raw `NodeRecord` objects directly from the registry (SQLite)

### `/api/nodes/{mac}` - Get Single Node

**Handler:** `mothership/internal/fleet/handler.go` (similar to listNodes)

**Returns:** Single `NodeRecord` from registry

**Response fields:**
- `role` - The role stored in the database (may be locked or computed)
- `role_locked` - Boolean indicating if operator-set protection is active
- `passive_bssid` - AP BSSID for passive nodes

### `/api/fleet` - Extended Fleet Data

**Handler:** `mothership/internal/fleet/handler.go` (has computed fields)

**Returns:** `FleetNode` with both registry fields AND computed fields:

**Registry fields:**
- `role` - From database
- `role_locked` - From database
- `passive_bssid` - From database

**Computed fields:**
- `status` - "online", "offline", "updating", "unpaired" (runtime state)
- `last_seen_ms` - Computed from `last_seen_at` timestamp
- `uptime_seconds` - From node health messages
- `packet_rate` - Current TX/RX rate from config
- `configured_rate` - Target rate from mothership
- `temperature` - Latest health report
- `free_heap_bytes` - Latest health report
- `ota_in_progress` - OTA state from OTA manager

---

## 6. Passive BSSID Handling

### Why It Matters (ADR-003 / `bf-6auk5`, `bf-303qg`)

When a node is assigned `role=passive`, it filters CSI frames by the router's BSSID. If the BSSID is empty or incorrectly sent as `""`, the firmware filters on `00:00:00:00:00:00` and **silently drops 100% of CSI**.

### PassiveBSSIDFor Function

**Location:** `mothership/internal/fleet/registry.go:281-295`

```go
func (r *Registry) PassiveBSSIDFor(mac, role string) string {
    if role != "passive" {
        return ""
    }
    bssid, err := r.GetNodePassiveBSSID(mac)
    if err != nil {
        log.Printf("[WARN] fleet: cannot read passive BSSID for %s: %v", mac, err)
        return ""
    }
    if bssid == "" {
        log.Printf("[WARN] fleet: node %s assigned role=passive with no stored BSSID; "+
            "it will filter on 00:00:00:00:00:00 and report no CSI", mac)
    }
    return bssid
}
```

### Persistence

```go
// mothership/internal/fleet/registry.go:299-302
func (r *Registry) SetNodePassiveBSSID(mac, bssid string) error {
    _, err := r.db.Exec(`UPDATE nodes SET passive_bssid=? WHERE mac=?`, bssid, mac)
    return err
}
```

---

## 7. Registry vs Computed Roles: The Divergence

### Registry Roles (Persistent)

**Source:** SQLite database (`nodes.role` column)

**When it changes:**
- Node hello message (role reported by firmware)
- Operator override (POST `/api/nodes/{mac}/role`)
- Fleet optimiser (if not locked)
- Self-healing re-optimisation

**Characteristics:**
- Survives mothership restart
- Can be protected by `role_locked` flag
- Always retrievable via `GetNodeRole(mac)`

### Computed Roles (Runtime)

**Source:** Fleet optimiser (`RoleOptimiser.Optimise()`)

**When it changes:**
- Every optimiser cycle (default 60s)
- Triggered by:
  - Node offline event
  - Manual optimisation request (POST `/api/fleet/optimise`)
  - Health score degradation

**Characteristics:**
- Not persisted to database
- Re-calculated each cycle based on:
  - GDOP (Geometric Dilution of Precision)
  - Health scores
  - Angular separation between nodes
  - Co-location detection
- Respects `role_locked` nodes (excluded from optimisation)

### Where Divergence Happens

**Critical code path:** `mothership/internal/fleet/selfheal.go:422-438`

```go
// Get locked nodes to exclude from optimisation
lockedNodes, err := shm.registry.GetLockedNodes()

// Filter out locked nodes from optimisation
unlockedNodes := make([]NodeInfo, 0, len(nodes))
for _, node := range nodes {
    if !lockedNodes[node.MAC] {
        unlockedNodes = append(unlockedNodes, node)
    }
}

// Run optimisation only on unlocked nodes
result := shm.optimiser.Optimise(unlockedNodes, triggerReason)
```

**The divergence:**
1. **Locked nodes** keep their registry role (operator intent preserved)
2. **Unlocked nodes** get computed roles (optimiser result)
3. **Registry** stores the computed result for unlocked nodes
4. **Next cycle:** Optimiser re-calculates for unlocked nodes again

### Role Resolution Flow

```
Optimiser proposes role
         ↓
SelfHealManager filters out locked nodes
         ↓
RoleAssignmentFor(mac, proposed)
         ↓
    If role_locked?
         ↓ Yes
    Use locked role (registry)
         ↓ No
    Use proposed role (computed)
         ↓
Return (role, passive_bssid)
         ↓
SendRoleToMAC sends to node
```

---

## 8. API Endpoints and Role Sources

### `/api/nodes` and `/api/nodes/{mac}`

**Role source:** Registry (SQLite database)

**Returns:**
- `role` - Current role from `nodes.role` column
- `role_locked` - Boolean protection flag
- `passive_bssid` - AP BSSID for passive nodes

**No computation** - Direct database query

### `/api/fleet`

**Role source:** Registry (same as `/api/nodes`)

**Plus:** Computed status fields
- `status` - Runtime connection state
- `last_seen_ms` - Computed from timestamp
- `health_score` - From registry (updated by health messages)
- `uptime_seconds` - From registry (updated by health messages)

**Role itself:** Still from registry, not computed

### `/api/fleet/health`

**Role source:** Not directly role-focused

**Returns:**
- Overall fleet health metrics
- Node counts by status
- Mean GDOP
- Coverage percentage

**Roles included:** Node count breakdown may include roles, but as reported from registry

### `/api/coverage`

**Role source:** Computed (for coverage calculation)

**Returns:**
- GDOP values per grid cell
- Coverage percentage
- Link quality metrics

**Uses:** Current role assignments (from registry) to compute coverage

---

## 9. Key Architectural Decisions

### ADR-003 References

| Bead | Issue | Fix |
|------|-------|-----|
| `bf-4kdww` | Operator-set roles must survive fleet re-optimization | Added `role_locked` flag and `RoleAssignmentFor` chokepoint |
| `bf-6auk5` | Passive BSSID must be sent with passive role | `PassiveBSSIDFor` ensures BSSID accompanies every passive role |
| `bf-303qg` | Passive BSSID persistence | `passive_bssid` column in database |

### Why RoleAssignmentFor is Mandatory

**Problem:** Before ADR-003, operator-set `role=passive` would flip back to `tx_rx` within 60s when the optimiser ran.

**Root cause:** Multiple code paths sent roles without checking if the node was locked:
- Self-healing re-optimisation
- Fleet optimiser
- Healer (GDOP-based optimisation)
- Reconnect grace period handling

**Solution:** Single chokepoint that **all** `SendRoleToMAC` call sites must use:
```go
role, bssid := registry.RoleAssignmentFor(mac, proposedRole)
SendRoleToMAC(mac, role, bssid)
```

### Why Passive BSSID Cannot Be Empty

**Problem:** Empty `passive_bssid` sent to firmware → filters on `00:00:00:00:00:00` → 100% CSI drop.

**Solution:** `PassiveBSSIDFor` warns and returns empty string (but logs the problem):
```go
if bssid == "" {
    log.Printf("[WARN] fleet: node %s assigned role=passive with no stored BSSID; "+
        "it will filter on 00:00:00:00:00:00 and report no CSI", mac)
}
```

**Critical:** Every role-assignment path must call `PassiveBSSIDFor` instead of passing `""` directly.

---

## 10. Regression Test Implications

### What to Test

1. **Role lock persistence:**
   - Set `role=passive` with lock
   - Trigger optimiser cycle
   - Verify role stays `passive`

2. **Passive BSSID propagation:**
   - Set `role=passive` with BSSID
   - Trigger self-healing
   - Verify BSSID still sent to node

3. **API endpoint consistency:**
   - `/api/nodes/{mac}` returns locked role
   - `/api/fleet` returns same role
   - Both include `role_locked` and `passive_bssid`

4. **Computed vs registry:**
   - Unlocked node role changes after optimisation
   - Locked node role never changes
   - Registry reflects both correctly

5. **Chokepoint coverage:**
   - Verify all `SendRoleToMAC` call sites use `RoleAssignmentFor`
   - Verify no direct `SendRoleToMAC(mac, role, "")` calls bypass it

### Critical Test Cases

| Test | Setup | Expected |
|------|-------|----------|
| Lock prevents optimiser override | Set `role=passive` + lock, trigger optimiser | Role stays `passive` |
| BSSID persists through self-heal | Set BSSID, trigger self-heal | BSSID still sent to node |
| API returns registry role | Set `role=tx_rx` (locked), query `/api/nodes/{mac}` | Returns `tx_rx` |
| Unlocked node accepts optimiser | Set `role=rx` (unlocked), optimise | Role may change |
| Passive role without BSSID warns | Set `role=passive` with empty BSSID | Log warning, send empty BSSID |

---

## Summary

**Role assignment flow:**
1. Registry stores roles (persistent)
2. Optimiser computes optimal roles (runtime)
3. `RoleAssignmentFor` is the chokepoint that reconciles both
4. Locked nodes keep registry roles
5. Unlocked nodes get computed roles
6. `PassiveBSSIDFor` ensures passive nodes have correct BSSID

**API endpoints:**
- `/api/nodes` and `/api/nodes/{mac}` → Registry roles (direct DB read)
- `/api/fleet` → Registry roles + computed status fields
- `/api/fleet/health` → Health metrics (not role-focused)
- `/api/coverage` → Uses registry roles to compute coverage

**Divergence:** Happens at the optimiser cycle, where locked nodes are excluded from computation and keep their registry roles, while unlocked nodes receive newly computed roles.
