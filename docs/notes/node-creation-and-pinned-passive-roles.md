# Node Creation with Pinned/Passive Roles

**Last updated:** 2026-08-28  
**Context:** Understanding how ESP32 nodes enter the Spaxel system and how roles (especially pinned/passive roles) are assigned and persisted.

## Overview

Nodes are created automatically when they first connect to the mothership via WebSocket. The system supports both automatic role assignment for TX/RX modes and manual "pinned" role assignment for special cases like passive radar mode.

## Node Creation Flow

### 1. Connection and Hello Message

**Entry Point:** `/home/coding/spaxel/mothership/internal/ingestion/server.go`

When an ESP32 node connects via WebSocket to `/ws/node`, it must send a `hello` message as the first JSON frame:

```go
// HelloMessage structure
type HelloMessage struct {
    Type            string   `json:"type"`            // Must be "hello"
    MAC             string   `json:"mac"`             // Node MAC address
    NodeID          string   `json:"node_id"`          // UUID from provisioning
    FirmwareVersion string   `json:"firmware_version"`
    Capabilities    []string `json:"capabilities"`
    Chip            string   `json:"chip"`
    FlashMB         int      `json:"flash_mb"`
    UptimeMS        int64    `json:"uptime_ms"`
    APBSSID         string   `json:"ap_bssid"`        // Optional: router BSSID for passive radar
    APChannel       int      `json:"ap_channel"`       // Optional: router channel
    Token           string   `json:"token"`           // HMAC-SHA256(install_secret, mac)
    PosX *float64 `json:"pos_x,omitempty"`    // Optional: announced 3D position
    PosY *float64 `json:"pos_y,omitempty"`
    PosZ *float64 `json:"pos_z,omitempty"`
}
```

**Key Function:** `handleWebSocket()` in `server.go` (lines 550-650)

The connection handler:
1. Reads and parses the hello message
2. Validates the token (if validator is configured)
3. Creates a `NodeConnection` object
4. Calls `fleetNotifier.OnNodeConnected()` to register the node

### 2. Node Registration in Fleet Manager

**Entry Point:** `/home/coding/spaxel/mothership/internal/fleet/manager.go`

**Key Function:** `OnNodeConnected()` (lines 184-232)

```go
func (m *Manager) OnNodeConnected(mac, firmware, chip string, posX, posY, posZ *float64) {
    // 1. Persist node to registry
    if err := m.registry.UpsertNode(mac, firmware, chip); err != nil {
        log.Printf("[WARN] fleet: upsert node %s: %v", mac, err)
    }

    // 2. Persist 3D position if announced (e.g., by spaxel-sim)
    if posX != nil && posY != nil && posZ != nil {
        if err := m.registry.SetNodePosition(mac, *posX, *posY, *posZ); err != nil {
            log.Printf("[WARN] fleet: set hello position %s: %v", mac, err)
        }
    }

    // 3. Mark node as online
    m.mu.Lock()
    m.online[mac] = struct{}{}
    m.mu.Unlock()

    // 4. Assign role (automatic, based on online node count)
    role := m.assignRole(mac)
    if err := m.registry.SetNodeRole(mac, role); err != nil {
        log.Printf("[WARN] fleet: set role %s: %v", mac, err)
    }

    // 5. Send role and config to node
    m.applyRoleAndConfig(mac, role)

    // 6. Broadcast updated state to dashboard
    m.broadcastRegistry()
}
```

### 3. Automatic Role Assignment

**Key Function:** `assignRole()` (lines 348-374)

The system automatically assigns roles based on the number of connected nodes:

```go
func (m *Manager) assignRole(mac string) string {
    n := len(m.online)
    switch {
    case n <= 1:
        return "tx_rx"  // Single node: both TX and RX
    case n == 2:
        // First node = TX, second = RX (by join order)
        if m.txCount == 0 {
            m.txCount++
            m.txNodes = append(m.txNodes, mac)
            return "tx"
        }
        return "rx"
    default:
        // For 3+ nodes: floor(N/2) TX nodes, rest RX
        targetTX := n / 2
        if len(m.txNodes) < targetTX {
            m.txCount++
            m.txNodes = append(m.txNodes, mac)
            return "tx"
        }
        return "rx"
    }
}
```

**Valid roles:** `tx`, `rx`, `tx_rx`, `passive`, `idle`, `virtual`

## Pinned/Passive Role Assignment

### Manual Role Override via REST API

**Entry Point:** `/home/coding/spaxel/mothership/internal/fleet/handler.go`

**API Endpoint:** `POST /api/nodes/{mac}/role`

**Key Function:** `setNodeRole()` (lines 352-390)

This allows operators to manually set a node's role, including the special `passive` role:

```go
type setRoleRequest struct {
    Role         string `json:"role"`         // Must be valid role
    PassiveBSSID string `json:"passive_bssid"` // Required when Role="passive"
}
```

### Role Override Flow with BSSID Persistence

**Key Function:** `OverrideRoleWithBSSID()` in `manager.go` (lines 287-326)

When a passive role is set, the BSSID is persisted and the role is "pinned":

```go
func (m *Manager) OverrideRoleWithBSSID(mac, role, passiveBSSID string) error {
    // 1. Validate and store passive BSSID for passive roles
    if role == "passive" {
        if passiveBSSID == "" {
            // Fall back to stored BSSID before rejecting
            stored, err := m.registry.GetNodePassiveBSSID(mac)
            if err != nil {
                return err
            }
            if stored == "" {
                return ErrPassiveBSSIDRequired  // Fail loudly
            }
            passiveBSSID = stored
        }
        if err := m.registry.SetNodePassiveBSSID(mac, passiveBSSID); err != nil {
            return err
        }
    }

    // 2. Set the role in registry
    if err := m.registry.SetNodeRole(mac, role); err != nil {
        return err
    }

    // 3. Pin the role so re-optimisation won't change it
    if err := m.registry.SetNodeRoleLocked(mac, true); err != nil {
        return err
    }

    // 4. Send role to node via WebSocket
    if notifier != nil {
        notifier.SendRoleToMAC(mac, role, passiveBSSID)
    }

    // 5. Broadcast updated state
    m.broadcastRegistry()
    return nil
}
```

**Critical Point:** When a role is manually set via `OverrideRoleWithBSSID()`, it is **pinned** by setting `role_locked=1` in the database. This prevents the fleet optimizer from automatically changing the role later (see ADR-003 / bf-4kdww).

## Role Assignment Resolution

**Key Function:** `RoleAssignmentFor()` in `registry.go` (lines 234-242)

Every role assignment (automatic or manual) goes through this single choke point:

```go
func (r *Registry) RoleAssignmentFor(mac, proposed string) (string, string) {
    role := proposed
    
    // If role is locked, use the locked role instead
    if r.IsNodeRoleLocked(mac) {
        if locked, err := r.GetNodeRole(mac); err == nil && locked != "" {
            role = locked
        }
    }
    
    // Return role + passive BSSID (empty for non-passive roles)
    return role, r.PassiveBSSIDFor(mac, role)
}
```

This ensures:
1. Manual role overrides persist across re-optimizations
2. Passive nodes always get their required BSSID sent with the role message
3. All role assignment paths (optimizer, self-heal, operator) use consistent logic

## Passive BSSID Resolution

**Key Function:** `PassiveBSSIDFor()` in `registry.go` (lines 281-295)

For passive roles, the system must send the router BSSID so the firmware knows which AP to filter CSI on:

```go
func (r *Registry) PassiveBSSIDFor(mac, role string) string {
    if role != "passive" {
        return ""  // Only passive roles need a BSSID
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

**Validation:** If a passive role is requested without a BSSID (and none is stored), the system returns `ErrPassiveBSSIDRequired` and rejects the request with HTTP 400. This prevents silent failures where the node would filter on `00:00:00:00:00:00` and drop all CSI frames.

## Database Schema

**Table:** `nodes` (in `registry.go` lines 95-111)

Key columns for role management:
- `role` TEXT NOT NULL DEFAULT 'rx' - Current role
- `previous_role` TEXT NOT NULL DEFAULT '' - Role before disconnect
- `role_locked` INTEGER NOT NULL DEFAULT 0 - If 1, role is pinned (operator-set)
- `passive_bssid` TEXT NOT NULL DEFAULT '' - Router BSSID for passive mode
- `role_before_disable` TEXT NOT NULL DEFAULT '' - Saved role when node is disabled

**Note:** All three columns (`role_locked`, `passive_bssid`, `role_before_disable`) were added via schema migrations to support ADR-003 requirements.

## Virtual AP Nodes for Passive Radar

**Entry Point:** `/home/coding/spaxel/mothership/internal/apdetector/detector.go`

**Key Function:** `ProcessHello()` (lines 60-110)

When nodes report their AP BSSID in hello messages, the AP detector:
1. Aggregates BSSID reports from all nodes
2. Detects the dominant AP (80%+ agreement)
3. Creates a **virtual node** representing the router

```go
func (d *Detector) ProcessHello(mac, apBSSID string, apChannel int) error {
    if apBSSID == "" {
        return nil
    }

    // Store report
    d.reports[bssid] = append(d.reports[bssid], BSSIDReport{
        NodeMAC:   mac,
        APBSSID:   bssid,
        APChannel: apChannel,
        Timestamp: now,
    })

    // Check for dominant AP
    if newAP := d.detectDominantAP(); newAP != nil {
        // Create or update virtual node
        if err := d.upsertVirtualNode(newAP); err != nil {
            log.Printf("[ERROR] apdetector: Failed to upsert virtual node: %v", err)
        }
    }

    return nil
}
```

The virtual node is inserted into the `nodes` table with:
- `virtual=1`
- `role='ap'` (or equivalent)
- `manufacturer` from OUI lookup (e.g., "ASUS", "TP-Link")
- Position defaults to (0, 0, 0) — user should place it in the 3D editor

## Validation and Constraints

### Role Validation

**Entry Point:** `handler.go` (lines 341-343)

```go
var validRoles = map[string]bool{
    "tx": true, "rx": true, "tx_rx": true, "passive": true, "virtual": true, "idle": true,
}
```

**Constraint:** Invalid roles return HTTP 400 "invalid role"

### Passive BSSID Validation

**Entry Point:** `handler.go` (lines 374-382)

```go
if err := h.mgr.OverrideRoleWithBSSID(mac, req.Role, req.PassiveBSSID); err != nil {
    if errors.Is(err, ErrPassiveBSSIDRequired) {
        // Fail loudly instead of accepting a config that silently yields no CSI
        http.Error(w, "role=passive requires passive_bssid", http.StatusBadRequest)
        return
    }
}
```

**Constraint:** Role="passive" without a BSSID returns HTTP 400. This is intentional (ADR-003 / bf-1p5g8) — accepting the request would enable an all-zeros CSI filter that silently drops 100% of frames.

### Node Existence Check

**Entry Point:** `handler.go` (lines 356-362)

Before setting a role, the system verifies the node exists in the registry:
```go
if _, err := h.mgr.registry.GetNode(mac); errors.Is(err, sql.ErrNoRows) {
    http.Error(w, "node not found", http.StatusNotFound)
    return
}
```

## Key File Paths

**Core files:**
- `/home/coding/spaxel/mothership/internal/fleet/registry.go` - Node persistence, role/BSSID storage, role locking
- `/home/coding/spaxel/mothership/internal/fleet/manager.go` - Fleet management, role assignment, pinning logic
- `/home/coding/spaxel/mothership/internal/fleet/handler.go` - REST API for manual role override
- `/home/coding/spaxel/mothership/internal/ingestion/server.go` - WebSocket connection handling, hello message processing
- `/home/coding/spaxel/mothership/internal/ingestion/message.go` - Message structures (hello, role, config)
- `/home/coding/spaxel/mothership/internal/apdetector/detector.go` - Virtual AP node creation for passive radar

**Key functions by responsibility:**

**Node Creation:**
- `handleWebSocket()` in `server.go` - Accepts WebSocket connection
- `OnNodeConnected()` in `manager.go` - Registers node, assigns initial role
- `UpsertNode()` in `registry.go` - Persists node to database

**Role Assignment:**
- `assignRole()` in `manager.go` - Automatic role assignment algorithm
- `OverrideRoleWithBSSID()` in `manager.go` - Manual role override with BSSID persistence
- `RoleAssignmentFor()` in `registry.go` - Role resolution choke point
- `setNodeRole()` in `registry.go` - Database update

**Role Pinning:**
- `SetNodeRoleLocked()` in `registry.go` - Pins/unpins a role
- `IsNodeRoleLocked()` in `registry.go` - Checks if role is pinned

**Passive Mode:**
- `SetNodePassiveBSSID()` in `registry.go` - Stores router BSSID
- `GetNodePassiveBSSID()` in `registry.go` - Retrieves stored BSSID
- `PassiveBSSIDFor()` in `registry.go` - BSSID resolution for role messages

**Virtual AP Nodes:**
- `ProcessHello()` in `apdetector/detector.go` - Aggregates BSSID reports
- `upsertVirtualNode()` in `apdetector/detector.go` - Creates virtual router node

## WebSocket Message Flow

When a node connects, the following sequence occurs:

1. **Node → Mothership:** `hello` JSON with MAC, firmware, AP BSSID, token
2. **Mothership validates** token (if validator configured and migration window closed)
3. **Mothership calls** `fleetNotifier.OnNodeConnected()`
4. **Fleet Manager:**  
   - Persists node via `UpsertNode()`
   - Assigns role via `assignRole()` (automatic) or uses pinned role
   - Sends role + config via WebSocket
5. **Mothership → Node:** `role` JSON message with optional `passive_bssid` field

## Summary

- **Automatic roles** are assigned based on online node count (≤1: `tx_rx`, 2: alternating TX/RX, 3+: floor(N/2) TX)
- **Pinned roles** are set via REST API `POST /api/nodes/{mac}/role` and persist across restarts
- **Passive mode** requires both `role="passive"` AND a stored BSSID; without BSSID, requests are rejected
- **Virtual AP nodes** are auto-created when nodes report the same router BSSID
- **Role locking** (`role_locked=1`) prevents the optimizer from changing operator-set roles
- All role assignments resolve through `RoleAssignmentFor()` to ensure pinned roles and passive BSSIDs are always included
