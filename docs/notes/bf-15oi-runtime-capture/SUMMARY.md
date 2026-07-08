# bf-15oi — End-to-end runtime log capture (identity blob)

**Parent:** bf-f841 (final acceptance step)
**Goal:** Run the full identity-blob scenario end-to-end and capture a runtime
log proving there are **no** runtime errors from undefined/missing identity
fields — the parent's last acceptance criterion.

## How it was produced

```
CAPTURE_DIR=docs/notes/bf-15oi-runtime-capture ./scripts/run-sim-identity.sh
```

`run-sim-identity.sh` (with the `CAPTURE_DIR` hook added for this bead) drives
the hardware-free runtime: it builds the mothership + `spaxel-sim` from source,
starts the mothership on an ephemeral data dir, pre-registers the person fixture
(`AA:BB:CC:DD:EE:00` → "Alice"), runs `spaxel-sim --ble --nodes 4 --walkers 1
--rate 30 --seed 42 --duration 25`, and polls `GET /api/blobs` for a blob
carrying non-empty canonical identity. On exit the `CAPTURE_DIR` hook copies the
mothership/sim logs and the captured evidence blob here before the temp dir is
removed.

**Run parameters:** nodes=4 walkers=1 rate=30Hz duration=25s seed=42 space=5x5x2.5 ble=on
**Script exit code:** 0 (PASS — a live `/api/blobs` blob carried non-empty identity)

## Artifacts

| File | What it is |
|------|------------|
| `mothership.log` | Full mothership runtime log (2414 lines) — startup phases → fusion → identity matching → graceful shutdown. **The artifact this bead captures.** |
| `sim.log` | `spaxel-sim` console output (43 lines) — 4 nodes connect, receive roles/config, stream CSI+BLE. |
| `identity_blob.json` | The `/api/blobs` blob that carried non-empty canonical identity (the PASS evidence). |

## Evidence — blob carried non-empty identity

`identity_blob.json` (blob ID 7):

```json
{
  "ID": 7, "X": 2.5, "Y": 2.3, "Z": 1.9,
  "person_id": "576aab22-0f69-4018-8f6e-8daccbea0928",
  "person_label": "Alice",
  "person_color": "#f97316",
  "identity_confidence": 0.7139,
  "identity_source": "ble",
  "personName": "Alice",
  "assignedColor": "#f97316",
  "identityResolved": true
}
```

Every canonical identity field is populated: `personName`, `assignedColor`,
`person_id`, `person_label`, `person_color`, `identity_confidence`,
`identity_source`, `identityResolved=true`.

## Error scan (the parent's acceptance criterion)

Scan of `mothership.log` + `sim.log` for runtime errors related to identity
fields — **all zero**:

| Pattern searched | Matches |
|------------------|---------|
| `panic` | **0** |
| `nil pointer dereference` | **0** |
| `runtime error` / `fatal error` / `SIGSEGV` | **0** |
| `undefined` / `missing…field` / `no such field` | **0** |
| `[ERROR]` / `[FATAL]` log lines | **0** |
| identity lines that are also warn/error/fail | **0** |

Log-level breakdown of `mothership.log`: 2340 `[INFO]`, 10 `[DEBUG]`, 10 `[WARN]`,
**0 `[ERROR]`, 0 `[FATAL]`**. The 10 `[WARN]` lines are all benign/expected for a
hardware-free run (missing dashboard dir, an early health-check race before bind,
the 4 sim nodes connecting on the open token-migration window, and the fleet
degraded-mode notices as the sim nodes disconnect at shutdown).

## Ran to completion without crashing

The mothership executed all 7 startup phases, then on SIGINT ran the full
graceful shutdown sequence:

```
[INFO] Received signal interrupt, initiating graceful shutdown
[INFO] [SHUTDOWN] Initiating graceful shutdown sequence (30s deadline)
[INFO] [SHUTDOWN] Step 1/10 … Step 4/10 …
[INFO] Saved learned weights on shutdown
[INFO] Saved spatial weights on shutdown
```

No panic, no unclean exit — the process shut down through its 10-step sequence.

## Conclusion

The identity-blob scenario runs end-to-end, produces a tracked blob with fully
populated identity, and the captured runtime log contains **zero** panic /
nil-deref / undefined-field errors related to identity fields. This satisfies
parent bf-f841's final acceptance criterion.
