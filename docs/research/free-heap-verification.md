# `free_heap_bytes` on the other response messages — verification

**Date:** 2026-09-03
**Bead:** `spaxel-404da1fc` — "Verify other response messages free_heap_bytes"
**Sibling:** [nodeinfo-free-heap.md](nodeinfo-free-heap.md) (`spaxel-527ed77b`, the
`NodeInfo` half of the same question)
**Verified at:** working tree on `main` (HEAD `e55d492a`), `go build`/`go vet`
clean, new fleet tests green.

---

## 1. The premise, carried over

As established by the survey chain and re-confirmed for this bead: **there is
no protobuf in Spaxel.** `find . -name '*.proto'` returns nothing, nothing was
ever added in git history, and no Go module in the repo imports a protobuf
runtime. "Field type" therefore means the Go field's kind behind a json tag,
and "field numbering" maps onto the two hazards JSON has instead of tag
numbers — a **duplicated key** (encoding/json silently drops one of the pair
on unmarshal) and a **case-only collision** (encoding/json matches keys
case-insensitively on unmarshal, so `"Free_Heap_Bytes"` would shadow
`"free_heap_bytes"`).

## 2. What "the other response messages" resolved to

The firmware builds exactly five upstream message types
(`firmware/main/websocket.c` — `hello` :484, `health` :563, `ble` :642,
`motion_hint` :690, `ota_status` :725), which is the same five
`ParseJSONMessage` switches on (`internal/ingestion/message.go:146`). The
survey's gates for *needing* a heap field are: origin is the device, subject
is the reporting device itself, and the message arrives outside steady state
where the 10 s health tick is unreliable. Everything else in the survey's
63-type inventory is either a downstream (mothership→node) message, an
internal type with no serialiser, or a response surface that reports a
node's state — those last ones are included here because they are where a
heap reading actually reaches a consumer.

## 3. The verification

### 3.1 Upstream wire messages

| Message | Heap key | Type | Key shape | Verdict |
|---|---|---|---|---|
| `health` → `HealthMessage` (`message.go:39`) | present :43 | `int64` | exact, **not** `omitempty` — this message exists to carry a reading, so `0` is meaningful and must not vanish | ✅ |
| `hello` → `HelloMessage` (`message.go:11`) | absent | — | — | ⚠️ gap, consistent both sides |
| `ota_status` → `OTAStatusMessage` (`message.go:82`) | absent | — | — | ⚠️ gap, consistent both sides |
| `ble` → `BLEMessage` (`message.go:66`) | correctly absent | — | subject is *other* devices, not the reporting node | ✅ n/a |
| `motion_hint` → `MotionHintMessage` (`message.go:74`) | correctly absent | — | an event, not a health report | ✅ n/a |

**"Consistent both sides" is the important nuance on the two gaps:** the
firmware's single `esp_get_free_heap_size()` call is in the health builder
(`websocket.c:571-572`); neither its `hello` builder nor its `ota_status`
builder emits the key, and neither Go struct declares it. So nothing is
being silently dropped today — these are an enhancement the survey scoped,
not a wire inconsistency. Adding the field is safe in one direction only
(`HelloMessage`/`OTAStatusMessage` unmarshal ignores unknown keys), which is
why the survey ordered it firmware-first.

### 3.2 Response surfaces a consumer actually reads

| Surface | Heap key | Type | Key shape | Verdict |
|---|---|---|---|---|
| `ingestion.NodeInfo` — dashboard WS (`server.go:1061`) | present | `int64` | `omitempty` — correct: no health tick yet means *unknown*, distinct from `0` | ✅ (verified in `spaxel-527ed77b`) |
| `fleet.NodeRecord` — GET /api/nodes/{mac} (`registry.go:44`) | present | `int64` | exact, not `omitempty` | ✅ shape |
| `fleet.FleetNode` — GET /api/fleet (`handler.go:177`) | present :177, projected :244 | `int64` | exact, not `omitempty` | ✅ shape |
| `fleet.fleetNodeEntry` — GET /api/fleet/health (`fleethandler.go:74`) | present :74, projected :122 | `int64` | exact, not `omitempty` | ✅ shape |

Every Go definition site of the field in the repo is in one of these tables
plus one SQL column (§4.2) — a repo-wide grep for `free_heap_bytes` /
`FreeHeapBytes` finds no other definition and no naming variant such as
`free_heap` that would fork the key. Wire-shape assertions for the three
fleet types are pinned in code (§6), so the duplicate-key and case-collision
hazards are mechanically checked rather than eyeballed.

## 4. Findings — the field is defined correctly and reads from nowhere

### 4.1 The write path has never existed

`Registry.UpdateNodeHealth` (`registry.go:343`) is the only writer of
`nodes.free_heap_bytes`, and it is **never called**. Its intended trigger,
`nodeHealthUpdater` in `internal/ingestion/server.go`, is a method on the
ingestion server that no code path invokes, and the setter that would inject
it — `SetNodeHealthUpdater` — has zero call sites in `main.go` and zero in
the whole history (`git log -S "SetNodeHealthUpdater"` finds only the commit
that introduced it, `3d844457`, 2026-08-11; no commit ever added a caller of
`UpdateNodeHealth`).

Consequence: `nodes.free_heap_bytes` sits at its schema default `0` forever,
so **GET /api/nodes/{mac}, GET /api/fleet and GET /api/fleet/health always
serve `free_heap_bytes: 0`** while the dashboard WebSocket's `NodeInfo`
carries the real reading from in-memory `nc.LastHealth`
(`server.go:1077`). The live value exists; none of the three REST surfaces
can see it.

### 4.2 And when it is wired, it will error on the first call

`UpdateNodeHealth`'s statement names five columns that the table it targets
does not have:

```sql
UPDATE nodes
SET uptime_ms=?, wifi_rssi_dbm=?, free_heap_bytes=?, temperature_c=?, ip=?, updated_at=...
```

The `nodes` table this `Registry` owns (`fleet.db`, schema at
`registry.go:95`) has **none** of `uptime_ms`, `wifi_rssi_dbm`,
`temperature_c`, `ip`, `updated_at` — its only health-related column is
`free_heap_bytes` itself, added by migration at `registry.go:159`. Those five
columns belong to the *other* database's `nodes` table (`spaxel.db`,
`internal/db/migrations.go:242-264`, which also carries `status`,
`node_id` and `capabilities`). The statement was evidently written against
that schema and dropped into the wrong registry. Any wiring that calls it as
written fails with `SQL logic error: no such column: uptime_ms` — verified
directly, by calling it in a test against `NewRegistry(":memory:")`.

So the wiring bead has two jobs, not one: pick the write shape (either slim
the UPDATE to `free_heap_bytes` alone, or migrate the missing columns into
`fleet.db`) *and* add the call. Confirmed there is no third writer to
coordinate with: `spaxel.db`'s `nodes.free_heap_bytes`
(`internal/db/migrations.go:258`) has no reader and no writer anywhere —
the AP detector's INSERT into that table (`internal/apdetector/detector.go:242`)
does not set it, so it stays NULL.

### 4.3 Scope call

This is a verification bead; the dispatch's acceptance criteria are that the
messages are checked and missing/incorrect definitions documented. The fix
involves a real design choice (which schema carries the health columns) plus
a wiring decision, and it is already owned by the open implementation bead
`spaxel-3b6699a4` ("Update API handlers to populate free_heap_bytes" — its
task 1, the response structs, is done; task 2 is the population) inside the
chain rooted at `spaxel-3278a63f`. The findings above are written up so that
bead starts from the schema error rather than discovering it in CI. The
`HelloMessage`/`OTAStatusMessage` gaps stay with the same chain as the survey
assigned them.

## 5. What is correct and needs no change

- All five Go wire surfaces carry `int64` — one width for one measurement,
  matching `HealthMessage.FreeHeapBytes`, from which every other value is
  copied. No `uint32` narrowing was introduced anywhere.
- The exact key `free_heap_bytes` is used at every definition site, matching
  the firmware sender; no surface forks the name.
- Required-vs-`omitempty` is right at each site: required on `HealthMessage`
  (the reading's origin), `omitempty` on `NodeInfo` (unknown ≠ 0), plain
  non-`omitempty` `int64` on the three REST surfaces where the DB column is
  `NOT NULL DEFAULT 0` — where "0" currently means "never written" rather
  than "no heap left", which is exactly the ambiguity §4.1 creates and the
  wiring fix resolves.

## 6. Durable artifact

`mothership/internal/fleet/heap_fields_test.go` — table-driven, package
`fleet`, five tests:

- `TestHeapBearingResponseTypesWireShape` — for `NodeRecord`, `FleetNode` and
  `fleetNodeEntry`: exactly one field marshals the exact key
  `free_heap_bytes`, its kind is `int64`, no sibling key collides with it
  case-insensitively, and the marshalled object emits the key exactly once.
- `TestFreeHeapBytesColumnPersistsIntoNodeRecord` — the column round-trips
  through `GetNode`/`GetAllNodes`, including the explicit-zero and the
  never-reported (default `0`) cases.
- `TestListFleetServesHeapFromRegistry`, `TestGetNodeServesHeapFromRegistry`,
  `TestGetFleetHealthServesHeapFromRegistry` — end to end through each of the
  three REST surfaces: a stored reading reaches the response's
  `free_heap_bytes` key, and an unreported node serves `0`.

These tests seed the column with a direct `UPDATE` rather than calling
`Registry.UpdateNodeHealth`, because that method cannot execute today
(§4.2); the helper is commented accordingly so the reason survives.

## Reproduce

```bash
# the premise
find . -name '*.proto' -not -path './.git/*'

# every definition site of the field
grep -rn "free_heap_bytes\|FreeHeapBytes" --include='*.go' mothership/ | grep -v _test.go

# the only firmware sender, and the five message types it builds
grep -n "free_heap\|esp_get_free_heap_size" firmware/main/websocket.c
grep -n 'cJSON_AddStringToObject(root, "type"' firmware/main/websocket.c

# the write path does not exist: no caller, ever
grep -rn "UpdateNodeHealth\|SetNodeHealthUpdater" --include='*.go' mothership/ | grep -v _test.go
git log --oneline -S "SetNodeHealthUpdater" -- mothership/

# the statement targets columns the fleet schema never created
sed -n '95,120p' mothership/internal/fleet/registry.go   # CREATE TABLE nodes
sed -n '343,352p' mothership/internal/fleet/registry.go  # UpdateNodeHealth
sed -n '242,264p' mothership/internal/db/migrations.go   # the schema it was written for

# the durable tests
cd mothership && go test ./internal/fleet/ -run 'TestHeapBearing|TestFreeHeapBytes|TestListFleetServesHeap|TestGetNodeServesHeap|TestGetFleetHealthServesHeap' -v
```
