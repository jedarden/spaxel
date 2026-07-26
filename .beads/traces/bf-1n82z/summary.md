# bf-1n82z — Capture the genuine bf-4q5w RED signature from the gated io6 test

**Bead:** bf-1n82z (split from bf-5k1z; depends on the bf-3xkrb auth-fix child)
**Task type:** diagnostic / evidence capture — closeable while still RED.
**HEAD at capture:** `0b602ab fix(bf-3xkrb): mint real per-node sim tokens`
**Captured:** 2026-07-26, two deterministic runs

## Files in this trace

| File | Contents |
|------|----------|
| `go-test-v.txt`       | Full `go test -tags io6_gate -run TestIO6HardGate_WalkerProducesTrackedBlob ./tests/e2e/... -v` output (run 1) |
| `go-test-v-run2.txt`  | Repeat run — confirms the failure mode is deterministic, not flaky |
| `go-test-v-run3.txt`  | Re-dispatch confirmation run at HEAD — same failure mode (third consecutive) |
| `diagnostic-dump.txt` | Extracted diagnostic dump + node-position-vs-blob-state contrast + honest nuance |

## Command run

```
cd mothership && go test -tags io6_gate -run TestIO6HardGate_WalkerProducesTrackedBlob ./tests/e2e/... -v
```

## Result: RED — FAIL (exit 1), ~35s, both runs identical

```
--- FAIL: TestIO6HardGate_WalkerProducesTrackedBlob (35.47s)
```

## Acceptance criteria — all met

1. **Fails on the blob assertion (not auth/startup/build/timeout)** — YES.
   Trips at `io6_gate_test.go:221`, the dashboard `/ws/dashboard` feed blob assertion
   (`AssertBlobObserved`: 0 blobs across 20 ticks). Auth is proven working
   (`✓ All 4 nodes online within first 5s (elapsed: 1s)`); startup/health/build all pass.
2. **Diagnostic shows distinct announced node positions AND (effectively) zero live
   blobs on the asserted channel** — YES for the contrast: nodes at the 4 distinct
   corners of a 5×5 space (`distinct geometry was announced = true`, only 1/4 at the
   origin) while the dashboard WS feed exposes zero blobs. Post-run `/api/status`
   and `/api/blobs` both report `blobs=0`. See the nuance note below re: detection
   events.
3. **Trace capturing the evidence exists** — YES (this directory).
4. **No change to the assertion** — YES. Zero `.go` files modified; `go vet ./tests/e2e/...`
   clean. This is a diagnostic-only task, identical in shape to the bf-2izfp trace.

## The bf-4q5w signature, captured

After the auth-fix dependency landed, the earlier auth-rejection failure mode (bf-2izfp)
is gone. The gate now reaches the genuine downstream RED state:

- spaxel-sim announces **DISTINCT** corner geometry in `hello`, persisted to the DB
  (`/api/nodes`: `(0,0,2) (5,0,2) (5,5,2) (0,5,2)` — not collapsed to the `(0,0,1)` default).
- yet the dashboard `/ws/dashboard` feed reports **zero** tracked blobs across the run.
- This node-geometry-vs-blob-feed contrast is the bf-4q5w wiring-gap signature.

## Honest nuance (feed back to bf-4q5w / bf-5jeo — assertion stays strict)

The task context predicted a signature of "zero tracked blobs AND zero detection
events." The actual capture is more specific: during the run `/api/blobs` shows peak 2
blobs and `/api/events` shows 100 detection events — so the fusion pipeline is emitting
reachable blobs via `/api/blobs`; only the **dashboard WS feed** is empty. This points
at a dashboard-WS-publishing wiring gap (bf-5jeo) layered on the bf-4q5w fusion wiring,
which is consistent with bf-3xkrb's regression note. The hard-gate assertion is
**not weakened**: it still `t.Fatalf`s on a zero dashboard blob feed, and is intentionally
kept RED until the wiring lands.

## Determinism

Three consecutive runs (two in the original capture, one re-dispatch confirmation at
HEAD=e2a177c) produced the same failure mode (same trip point at the WS-feed assertion,
same peak-2 /api/blobs, same 100 detection events, same distinct-geometry diagnostic).
Minor role-assignment variation between runs (fleet role engine) did not affect the
outcome. Not a flaky 0-blob — see the bf-4q5w notes re: SIM_RATE 20.
