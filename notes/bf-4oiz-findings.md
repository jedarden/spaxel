# Registry Bridge Position Wiring — Audit (bf-4oiz, re-verified bf-4pqj)

## Summary

**Prior claim (STALE):** The original bf-4oiz notes asserted the registry-bridge
position wiring was *missing*.

**Current reality (verified 2026-07-06):** The periodic sync **is wired in
source** — `cmd/mothership/main.go` constructs the `VirtualNodeStore`, wraps it in
a `FleetRegistryBridge`, and runs a 30 s ticker that calls `SyncToRegistry`. The
`registry_bridge.go` logic itself is correct and is backed by
`registry_bridge_test.go`.

⚠ **HOWEVER — compile blocker discovered during this audit:** `cmd/mothership`
does **not** currently build. The wiring call at `main.go:4217` passes
`fleetReg` (`*fleet.Registry`) to `SyncToRegistry`, but `*fleet.Registry` does
**not** satisfy `simulator.RegistryNodeAdapter` (see [§ Build Blocker](#build-blocker)).
So the sync is present in source but the binary does not compile — it is **not
actually running**. This must be fixed (likely a separate concern from bf-5dpu,
which introduced the call) before the periodic sync executes at all.

---

## Current Data Flow (source-level, correct by inspection + unit tests)

```
simulator (REST API / sim CLI)
        │  CreateVirtualNode(id, name, Point{X,Y,Z})      virtual_state.go:130
        │  UpdateNodePosition(id, Point{X,Y,Z})           virtual_state.go:202
        ▼
VirtualNodeStore  (persistent; DefaultSpace)              virtual_state.go
        │  ListNodes() []*VirtualNodeState{ .Position }   registry_bridge.go:147
        ▼
FleetRegistryBridge.SyncToRegistry(adapter)               registry_bridge.go:142
        │  effectivePositions(nodes)  → spread / preserve registry_bridge.go:51
        │  adapter.AddVirtualNode(mac, name, x, y, z)     registry_bridge.go:161
        │  adapter.SetNodePosition(mac, x, y, z)          registry_bridge.go:177
        ▼
fleet.Registry (RegistryNodeAdapter impl)                 internal/fleet/registry.go
        │  periodic, every 30 s                           main.go:4209-4222
```

### Wiring site — `cmd/mothership/main.go`

(Line numbers as of HEAD; the task brief cited 4269-4301 but the block has
since shifted to **4190-4226**.)

```go
// main.go:4195  — store is the position source
virtualNodeStore, err = simulator.NewVirtualNodeStore(simulator.StoreConfig{...})

// main.go:4206  — bridge wraps the store
registryBridge = simulator.NewFleetRegistryBridge(virtualNodeStore)

// main.go:4209-4222 — 30 s periodic sync
go func() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            if err := registryBridge.SyncToRegistry(fleetReg); err != nil { // <-- BLOCKER: type mismatch
                log.Printf("[WARN] Failed to sync virtual nodes to fleet registry: %v", err)
            }
        }
    }
}()
```

### Position formatting — CONFIRMED CORRECT

`RegistryNodeAdapter` (registry_bridge.go:113-121) uses `float64` XYZ throughout:

```go
type RegistryNodeAdapter interface {
    AddVirtualNode(mac, name string, x, y, z float64) error
    SetNodePosition(mac string, x, y, z float64) error
    SetNodeRole(mac, role string) error
    DeleteNode(mac string) error
    GetNode(mac string) (*NodeRecord, error)
    GetAllNodes() ([]NodeRecord, error)
}
```

`Point` is `struct{ X, Y, Z float64 }` (space.go:51), so the `pos.X/pos.Y/pos.Z`
spreads at registry_bridge.go:161-166 and :177-180 are type-correct.

`registry_bridge_test.go` backs this with a `fakeRegistry` that asserts
positions round-trip exactly:

- `TestSyncToRegistry_ExplicitPositionsPreserved` — `{1,1,1.5}` and `{4,4,2}`
  survive unchanged through `AddVirtualNode`.
- `TestSyncOneNode_ExplicitPreserved` — `{2,3,1.2}` survives `SetNodePosition`.
- `TestSyncToRegistry_OriginNodesGetSpreadGeometry` — unset (origin) nodes get
  distinct, non-degenerate spread geometry.
- `TestSyncOneNode_MatchesFullSync` / `TestSyncToRegistry_Idempotent` —
  deterministic, stable slot assignment.

`go test ./internal/simulator/` → **ok** (the bridge logic is sound).

---

## Build Blocker (discovered by this audit)

`go build ./cmd/mothership/` and `go vet ./...` both **fail**:

```
cmd/mothership/main.go:4217:46: cannot use fleetReg (variable of type
*fleet.Registry) as simulator.RegistryNodeAdapter value in argument to
registryBridge.SyncToRegistry: *fleet.Registry does not implement
simulator.RegistryNodeAdapter (wrong type for method GetAllNodes)
        have GetAllNodes() ([]fleet.NodeRecord, error)
        want GetAllNodes() ([]simulator.NodeRecord, error)
```

`*fleet.Registry` (`internal/fleet/registry.go`) defines every method the
interface needs, but two return a **different named type**:

| Adapter method     | interface wants (`simulator`) | `*fleet.Registry` has (`fleet`) |
|--------------------|-------------------------------|---------------------------------|
| `GetNode`          | `*simulator.NodeRecord`       | `*fleet.NodeRecord`             |
| `GetAllNodes`      | `[]simulator.NodeRecord`      | `[]fleet.NodeRecord`            |

The two structs are also structurally incompatible (not just differently-named):
`fleet.NodeRecord` has `PreviousRole`, `WentOfflineAt`, `Manufacturer`,
`FirstSeenAt`, `LastSeenAt`, `FirmwareVersion`, `ChipModel`, `HealthScore` and
lacks `Enabled`; `simulator.NodeRecord` is a minimal `{MAC,Name,Role,PosX,PosY,
PosZ,Virtual,Enabled}`.

**Origin:** introduced by commit `e92d5bc` ("feat(simulator): wire registry
bridge position data flow (bf-5dpu)"), which added the `SyncToRegistry(fleetReg)`
call without making `*fleet.Registry` satisfy the adapter. Before that commit the
call site did not exist, so the mismatch was not exercised.

**Second build failure (same commit):** `internal/api/simulator_test.go:19`
calls `NewSimulatorHandler()` with no arguments, but `e92d5bc` changed the
signature to `NewSimulatorHandler(store, bridge)` (see main.go:4226). The test
was not updated:

```
internal/api/simulator_test.go:19:33: not enough arguments in call to
NewSimulatorHandler
        have ()
        want (*simulator.VirtualNodeStore, *simulator.FleetRegistryBridge)
```

Net effect of `go vet ./...` / `go test ./...`: **two packages fail to build**
(`cmd/mothership`, `internal/api`). The `internal/simulator` package itself —
which holds `registry_bridge.go` and its tests — builds and tests **green**.

**Implication:** The "periodic sync IS wired" premise of bf-4pqj is true **only
at the source-text level**. Because `cmd/mothership` does not compile, the
container binary cannot be built and the 30 s sync is not executing in any
running system. The `registry_bridge.go` logic and its unit tests are correct in
isolation; the breakage is purely the main.go → fleet adapter glue.

### Suggested fix (NOT applied — this bead is verification-only)

Pick one (a small, self-contained change; belongs to the wiring owner, not this
audit):

1. **Add a thin adapter** in `cmd/mothership` that wraps `*fleet.Registry` and
   converts `*fleet.NodeRecord` → `*simulator.NodeRecord` for `GetNode` /
   `GetAllNodes`. Smallest blast radius; no fleet-package churn.
2. **Align the type** — have `simulator.RegistryNodeAdapter` reference
   `fleet.NodeRecord` (or a shared record type), and drop the duplicate
   `simulator.NodeRecord`. Bigger change; touches the bridge tests' `fakeRegistry`.

Either way the `Enabled` field semantics should be reconciled (simulator's record
carries it; fleet's does not).

---

## Conclusion / acceptance-criteria mapping

| Criterion (bf-4pqj) | Status |
|---|---|
| registry_bridge receives position from simulator via store — documented | ✅ Confirmed (store.ListNodes → .Position). |
| Position formatting correct, backed by registry_bridge_test.go | ✅ float64 XYZ; tests assert exact round-trip; `go test ./internal/simulator/` passes. |
| bf-4oiz-findings.md 'Current Data Flow' corrected (periodic sync wired) | ✅ This document. |
| No production code changes | ✅ None made. |
| `go vet ./...` green before close | ❌ **Fails** — see Build Blocker. Pre-existing, introduced by bf-5dpu (e92d5bc); out of scope for this verification bead. |

**Net:** the bridge logic and tests are verified correct; the stale "wiring
missing" claim is corrected (wiring exists in source). The remaining gate is the
compile-time adapter mismatch at main.go:4217, which must be fixed (a wiring
concern, not a bridge-logic concern) before the periodic sync actually runs.
