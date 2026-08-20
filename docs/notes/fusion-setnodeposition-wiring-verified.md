# Fusion Engine SetNodePosition Wiring — Verified Live

**Bead:** spaxel-1ef81194 (found during bf-3v39 presence-detection investigation)
**Verified:** 2026-08-20
**Status:** The wiring described by the bead is fully present at HEAD — landed
across earlier beads — and the IO-6 hard gate is GREEN. This dispatch is a
verification, not an implementation.

## Symptom the bead was filed against

E2E tests (`TestFullE2EIntegration`, `TestDetectionEvents`) and the IO-6
hard-gate scenario produced **zero** detection events / blobs with 4 nodes +
2 walkers running spaxel-sim. Root cause: `fusion.Engine.SetNodePosition`
(`mothership/internal/fusion/fusion.go`) was defined but never called outside
tests; nodes sat co-located at the nodes-table schema default
`pos=(0,0,1)` (`mothership/internal/db/migrations.go:246-248`), so the
Fresnel excess path length `|P-T|+|P-R|-|T-R|` collapsed to ~0 and no peak
could form.

## What exists at HEAD (all committed)

Every wiring path the bead's "LIKELY FIX" asked for is live:

| Path | Location | Landed by |
|---|---|---|
| Startup seeding from the DB nodes table | `cmd/mothership/main.go` (Phase 6 loop: `GetAllNodes` → `fusionEngine.SetNodePosition`) | bf-3f6q |
| Startup degenerate-geometry assertion (logs WARN if every seeded node is at (0,0,1)) | `cmd/mothership/main.go` (over `fusionEngine.NodePositions()`) | bf-1tsm |
| `PATCH /api/nodes/{mac}/position` → engine | `internal/fleet/handler.go:400-415` (`updateNodePosition` → `ForwardNodePosition`) + `Manager.SetNodePositionSink` wired to the engine in `main.go` | bf-3p6g |
| Node connect/register → engine (incl. hello-announced positions, which spaxel-sim sends) | `internal/fleet/manager.go` `OnNodeConnected` → `ForwardNodePosition` | bf-3p6g, bf-24xp |
| Simulator registry writes → engine | `fleetRegistryAdapter.SetNodePosition`/`AddVirtualNode` forward through `forwardPos` (`cmd/mothership/main.go`) | bf-u7ds |
| spaxel-sim default placement | `cmd/sim/main.go` announces computed corner geometry (`pos_x/y/z` in hello); `internal/simulator/registry_bridge.go` reassigns spread geometry to any node still at the default origin | bf-24xp, bf-18yn/bf-4q5w |
| Hard test assertions (no count ducking) | `tests/e2e/e2e_test.go` (`AssertBlobObserved` + detection-event gate in `TestFullE2EIntegration`), `tests/e2e/io6_gate_test.go` (dual WS + `/api/blobs` gate with data-driven diagnostics) | bf-5jeo/bf-2aqf, bf-2330, bf-48juo |

## Verification evidence (2026-08-20, HEAD 9ee531c, repo VERSION 0.2.55)

All run from `mothership/` with `-count=1` (no cache):

- `go test ./internal/fusion/ ./internal/fleet/ ./internal/simulator/` — pass
- `TestDetectionEvents` — **PASS** (21.5 s; 100 detection events observed)
- `TestFullE2EIntegration` — **PASS** (66.0 s; dashboard WS showed ≥1 blob,
  peak concurrent blobs 2, 100 detection events)
- `go test -tags io6_gate -run TestIO6HardGate_WalkerProducesTrackedBlob` —
  **PASS**: "IO-6 hard-gate PASSED: walker produced a tracked blob
  (dashboard WS peak >=1, /api/blobs peak=2, 100 detection events)"
- `go vet ./...` — clean
- `go test ./...` (full module) — exit 0

The earlier PROGRESS.md chain of re-verifications (bf-3zll/bf-3hji/bf-4ads8
era) that tracked "`blobs: 0` is correctly out of scope (next chain link,
fusion SetNodePosition wiring, bf-4q5w / IO-6 hard-gate)" is resolved by this
evidence: that chain link is closed at HEAD.

## Implication for bf-3v39 (physical presence-detection validation)

The bead noted the eventual hardware validation would show no blobs
"regardless of hardware, until node positions feed the fusion engine." That
blocker is gone: the simulator path now exercises real geometry end to end.
A hardware run still requires the physical nodes to be **placed** (dashboard
positioning or hello-announced positions), but the engine consumes those
positions on every admission path.
