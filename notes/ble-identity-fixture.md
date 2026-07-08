# BLE Identity Fixture — Registered Person + Sim Device (bf-14wx5)

**Canonical script:** `scripts/run-sim-ble-fixture.sh`
**Verified:** 2026-07-08 — HEAD `36b9729` (post bf-21v71 Stage 2b write-back).
**Resolves:** bf-14wx5 — the FIRST link of the bf-2m534 identity chain. Establishes the
BLE fixture (a registered person + the sim's advertised device bound to that person) that
the identity matcher needs to produce ANY match. Without it the matcher has nothing to
match and identity stays empty regardless of the wire fix.

This note records the exact REST sequence, the observed sim BLE address convention, and
the answer to "must the device be seen-live first?" so that later beads (the matcher /
`/api/blobs` identity capstone under bf-2m534, and `scripts/run-sim-identity.sh`) can
**reuse, not re-derive.**

---

## TL;DR

```bash
./scripts/run-sim-ble-fixture.sh
# expects:
#   [fixture] phaseA (preregister+PUT, NO live adv): PASS
#   [fixture] phaseB (live sim --ble adv, binding survives): PASS
#   [fixture] PASS: GET /api/ble/devices confirms AA:BB:CC:DD:EE:00 <-> "Alice" in both phases
```

The sim advertises each walker as device addr `AA:BB:CC:DD:EE:%02X` (walker.ID, from 0),
name `sim-person-%d`. With `--walkers 1` the advertised addr is exactly
`AA:BB:CC:DD:EE:00`. Bind it to a person with three REST calls, then `GET /api/ble/devices`
confirms `person_id` / `person_name` are non-empty.

---

## The repeatable REST sequence (the fixture)

Against a running mothership (start one with `scripts/run-sim-local.sh`'s env, or any
fresh-data-dir instance — `SPAXEL_DATA_DIR=$(mktemp -d)`):

```bash
PORT=8088
MAC="AA:BB:CC:DD:EE:00"        # sim walker 0 (see convention below)
NAME="Alice"
COLOR="#4488ff"

# 1. Create the device row for the known MAC. Sets last_seen_at=now, so the device
#    appears in GET /api/ble/devices even with NO live advertisement (see "seen-live").
curl -s -X POST http://localhost:$PORT/api/ble/devices/preregister \
  -H 'Content-Type: application/json' -d "{\"mac\":\"$MAC\",\"label\":\"$NAME\"}"

# 2. Create the person; capture its id.
PERSON_ID=$(curl -s -X POST http://localhost:$PORT/api/people \
  -H 'Content-Type: application/json' \
  -d "{\"name\":\"$NAME\",\"color\":\"$COLOR\"}" | jq -r .id)

# 3. Assign the device to the person (sets ble_devices.person_id).
curl -s -X PUT http://localhost:$PORT/api/ble/devices/$MAC \
  -H 'Content-Type: application/json' \
  -d "{\"person_id\":\"$PERSON_ID\",\"label\":\"$NAME\"}"

# 4. Verify: device is associated to the seeded person.
curl -s "http://localhost:$PORT/api/ble/devices?registered=true" \
  | jq '.devices[]|select(.mac=="AA:BB:CC:DD:EE:00")|{mac,label,person_id,person_name}'
```

Expected (verified 2026-07-08):

```json
{ "mac": "AA:BB:CC:DD:EE:00", "label": "Alice",
  "person_id": "<uuid>", "person_name": "Alice" }
```

Endpoint reference: `mothership/internal/ble/handler.go`
(`POST /api/ble/devices/preregister`, `POST /api/people`, `PUT /api/ble/devices/{mac}`,
`GET /api/ble/devices`).

---

## Sim BLE address convention (observed)

Source: `mothership/cmd/sim/main.go`, `sendBLEMessages` (line ~1125). For each walker:

```go
"addr":     fmt.Sprintf("AA:BB:CC:DD:EE:%02X", walker.ID),
"rssi_dbm": int(rssi),                       // -50 - 20*log10(dist), floored at -90
"name":     fmt.Sprintf("sim-person-%d", walker.ID),
```

`walker.ID = i` (the loop index, from 0 — `mothership/cmd/sim/main.go` walker constructors
`ID: i`). So:

| `--walkers N` | Advertised addrs (one per walker) |
|---------------|-----------------------------------|
| 1 | `AA:BB:CC:DD:EE:00` |
| 2 | `AA:BB:CC:DD:EE:00`, `AA:BB:CC:DD:EE:01` |
| k | `AA:BB:CC:DD:EE:00` … `AA:BB:CC:DD:EE:%02X` (k−1) |

BLE cadence: the sim relays one `{type:"ble",...}` message per node every 5 s
(`if *flagBLE && time.Since(lastBLETime) > 5*time.Second`). Each node reports every
walker, so a 4-node fleet yields 4 RSSI samples per walker per 5 s window — enough for
the matcher's triangulation.

> **Field-name note (bf-21v71):** the sim previously sent `"rssi"`; the mothership's BLE
> parser reads `rssi_dbm`, so RSSI was read as 0 and triangulation collapsed. The sim now
> sends `"rssi_dbm"` (uncommitted working-tree change at `mothership/cmd/sim/main.go`).
> This is the third precondition the diagnosis `notes/blob-identity-diagnosis.md` named.

---

## "Must the device be seen-live first?" — NO (with evidence)

**Answer: `preregister` + `PUT` binds the sim device addr standalone; the device does NOT
need to be seen live first.** Evidence (bf-14wx5 Phase A, `scripts/run-sim-ble-fixture.sh`):

The fixture was seeded with the sim **not running** (no live advertisement), and
`GET /api/ble/devices?registered=true` still returned the device fully bound:

```
[fixture] Phase A: seed fixture BEFORE any sim BLE advertisement (no live adv)...
[fixture]   [phaseA] OK: AA:BB:CC:DD:EE:00 -> person_id=345dcfeb-… person_name="Alice"
```

**Why it works** (code, `mothership/internal/ble/registry.go`):

- `PreregisterDevice` (`INSERT … VALUES (… last_seen_at=now …)`) sets `last_seen_at` to
  the current time on insert.
- `GetDevicesSeenInHours` (the query behind `GET /api/ble/devices`) filters
  `WHERE d.last_seen_at >= <now − hours>`. Because preregister sets `last_seen_at=now`,
  the device falls inside the default 24 h window and is served — no live sighting needed.

**The binding also survives live advertisements.** Phase B of the same run started
`sim --ble` after the fixture was seeded, let advertisements flow, and re-verified:

```
[fixture]   [phaseB] OK: AA:BB:CC:DD:EE:00 -> person_id=345dcfeb-… person_name="Alice"
…
{ "mac": "AA:BB:CC:DD:EE:00", "label": "Alice", "person_id": "345dcfeb-…",
  "person_name": "Alice", "rssi_avg": -60, "last_seen_node": "AA:BB:CC:00:00:02" }
```

`ProcessRelayMessage` (the live-adv path) upserts with `ON CONFLICT(mac) DO UPDATE` that
touches only `device_type`, `device_name`, `manufacturer`, mfr fields, rssi stats,
`last_seen_at`, `last_seen_node`, `is_wearable` — **it never writes `person_id`**, so a
live sighting cannot clobber the assignment. (Conversely, registering a device that is
already seen-live works identically — that is the realistic onboarding order exercised by
`scripts/run-sim-identity.sh`.)

> **Caveat on re-running `preregister`:** the `ON CONFLICT(mac) DO UPDATE` clause updates
> only `name`/`label`, **not** `last_seen_at`. So a second `preregister` on an already-seen
> device will not refresh its visibility window. For the fixture this is irrelevant (one
> preregister on a fresh data dir), but if you ever need to re-surface a stale device, send
> a live advertisement or bump `last_seen_at` directly — don't rely on re-preregistering.

---

## What this fixture enables (for later beads)

The identity matcher (`mothership/internal/ble`) only considers devices with a non-null
`person_id` (e.g. `GetAllPersonDevices` / `GetAllPersonDevicesWithAliases` filter
`WHERE person_id IS NOT NULL`). So this fixture — a person + a bound device — is the
**precondition** for any identity match to fire at all. Downstream:

- **bf-2m534 / `scripts/run-sim-identity.sh`** (the parent capstone): pairs this fixture
  with `sim --ble` and the Stage 2b write-back (bf-21v71) to assert a tracked blob served
  at `/api/blobs` carries non-empty canonical identity (`personName`/`assignedColor`).
- **bf-m1ynp diagnosis** (`notes/blob-identity-diagnosis.md`): named "no person
  registered" as a required fix; this fixture is that fix, factored out as the first link.

This script deliberately does NOT gate on `/api/blobs` identity — that is the parent
bead's concern. It gates only on `GET /api/ble/devices`, keeping the fixture link
independently verifiable.

---

## Reuse, don't re-derive

- **Start from `scripts/run-sim-ble-fixture.sh`.** It builds both binaries from
  `mothership/cmd/{mothership,sim}` (the canonical sim with `--ble`; never the repo-root
  `cmd/sim/` — see `notes/hardware-free-runtime.md` gotcha #1), starts a fresh-data-dir
  mothership, and runs both phases.
- **All params overridable:** `SIM_PORT`, `SIM_WALKER_MAC`, `SIM_PERSON_NAME`,
  `SIM_PERSON_COLOR`, `SIM_NODES`, `SIM_WALKERS`, `SIM_RATE`, `SIM_SEED`, `SIM_SPACE`.
- **Determinism:** `--seed` pins walker paths; walker 0 always advertises
  `AA:BB:CC:DD:EE:00`. The only residual nondeterminism is the ~5 s BLE relay cadence,
  absorbed by the 7 s settle before the Phase B re-verify.

## Keeping this honest

If `scripts/run-sim-ble-fixture.sh` ever fails Phase A: the `PreregisterDevice` →
`last_seen_at=now` invariant (or the `GetDevicesSeenInHours` window) changed; re-read
`registry.go` before assuming the fixture is wrong. If it fails Phase B only:
`ProcessRelayMessage`'s upsert started touching `person_id`, or the sim stopped emitting
`rssi_dbm` — check `mothership/cmd/sim/main.go` `sendBLEMessages` first.
