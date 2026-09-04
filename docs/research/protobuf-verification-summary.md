# `free_heap_bytes` verification — consolidated summary

**Date:** 2026-09-04
**Bead:** `spaxel-b5dc100d` — "Document verification results summary"
**Verified at:** `origin/main` `20b580d2` (tree HEAD `426340ee`; the only delta is
the `VERSION` auto-bump). All line numbers below are **origin-accurate** unless
marked otherwise.

This is the one document that answers all three of the chain's verification
questions at once: which messages have `free_heap_bytes` correctly defined,
which are missing it, and what issues remain. It consolidates five upstream
documents and two verification beads; links to each are in §6 so no claim here
has to be taken on trust.

---

## 0. The premise every verdict rests on

**Spaxel has no protobuf.** Zero `.proto` files in the tree, zero in all of git
history, and no Go module in the repo imports a protobuf runtime. The wire
format is JSON (a struct's json tags *are* the schema) over WebSocket plus chi
REST, plus two hand-rolled binary CSI frames. The survey chain established this
across five beads and both verification beads re-confirmed it live.

So the dispatch's protobuf vocabulary translates as:

| As written | What it means here |
|---|---|
| message | a Go struct carrying json tags that a traced serialisation site marshals |
| field type | the Go field's kind behind the tag (`int64` vs `uint32` …) |
| field numbering | the two hazards JSON has instead of tag numbers: a **duplicated key** (`encoding/json` silently drops one of a pair on unmarshal) and a **case-only collision** (`encoding/json` matches keys case-insensitively) |

## 1. Verdict per message — pass/fail

Every Go definition site of the field in the repo appears in this table; a
repo-wide grep for `free_heap_bytes` / `FreeHeapBytes` finds no other
definition and no naming variant such as `free_heap` that would fork the key.

### 1.1 PASS — field present, typed `int64`, exact key, no key collision

| Message / surface | Definition | Filled from | Consumer | Verified by |
|---|---|---|---|---|
| `health` → `HealthMessage` | `internal/ingestion/message.go:43` | firmware `esp_get_free_heap_size()`, `websocket.c:571` | ingestion → `LastHealth` | `nodeinfo_test.go` |
| `ingestion.NodeInfo` (dashboard WS) | `internal/ingestion/server.go:1061` (`omitempty` — correct: unknown ≠ 0) | `nc.LastHealth.FreeHeapBytes`, `server.go:1077` | `hub.go:670` snapshot, `:773` delta | `nodeinfo_test.go` |
| `fleet.NodeRecord` | `internal/fleet/registry.go:44` | column `free_heap_bytes`, added `registry.go:159` | GET /api/nodes/{mac} marshals the record directly | `heap_fields_test.go` |
| `fleet.FleetNode` — GET /api/fleet | `internal/fleet/handler.go:146` | projected `:213` | `GET /api/fleet` (`listFleet`, route `handler.go:87`) | `heap_fields_test.go` |
| `fleet.fleetNodeEntry` — GET /api/fleet/health | `internal/fleet/fleethandler.go:74` | projected `:122`/`:148` | `GET /api/fleet/health` | `heap_fields_test.go` |

Shape is right at all five sites: **one width for one measurement** — every
value descends from `HealthMessage.FreeHeapBytes int64`, and no `uint32`
narrowing was introduced anywhere; the exact key `free_heap_bytes` at every
site, matching the firmware sender; no duplicated key and no case-only
collision (asserted mechanically, not eyeballed, by the tests in §6).

> Line-number note: `handler.go:146`/`:213` are the **origin** positions. Two
> sibling docs cite `:177`/`:244` for these same sites — those were read from a
> working tree carrying a concurrent worker's uncommitted `nodeView` edit
> (§4.6), which shifts the file by ~40 lines. Nothing else in this table is
> affected; every other cited file is identical between the tree and origin.

### 1.2 FAIL — should have the field and does not (both sides consistently)

| Message | Go struct | Firmware sender | Why it needs the field |
|---|---|---|---|
| `hello` (gap 1) | `internal/ingestion/message.go:11` — no heap field | `websocket.c:478`, no key emitted | origin device, subject device, and the *only* reading a node sends before its first 10 s `health` tick; already carries the other boot-diagnosis fields (`safe_mode_active`, `boot_count`) for exactly that audience |
| `ota_status` (gap 2) | `internal/ingestion/message.go:82` — no heap field | `websocket.c:718`, no key emitted | fires during the one operation that itself consumes heap; `health` is least reliable precisely here |

"Consistent both sides" is the important nuance: the firmware's single
`esp_get_free_heap_size()` call is at `websocket.c:571`, inside the health
builder only. Neither the `hello` nor the `ota_status` builder emits the key,
and neither Go struct declares it — so **nothing is being silently dropped
today**. These are an enhancement the survey scoped, not a wire
inconsistency. Adding the field is one-direction-safe (unknown JSON keys are
ignored on unmarshal, proven by `json_fuzz_test.go:117`), which is why the
survey ordered it firmware-first.

### 1.3 PASS (n/a) — correctly absent

No action for any of these; the reason each lacks the field is the finding.

- **`ble`** (`message.go:66`) — node-*originated*, but the payload is other
  devices' advertisements. The sharpest case in the set: an origin-only rule
  would wrongly attach the reporter's heap to a list of third parties.
- **`motion_hint`** (`message.go:74`) — a one-number event; heap is at most
  10 s stale from `health` anyway.
- **All 23 mothership-computed `*Response` types** plus `SecurityStatus`,
  `DiurnalLearningStatus`, `LinkInfo`, `LinkHealthInfo`, `replayInfo`,
  `WebhookTestResult`, `ActionResult`, `CheckResult` — origin is the
  mothership, not a device. A mothership reporting its own heap under a field
  named for the device metric would be actively misleading.
- **Downstream commands** `role`/`config`/`ota`/`reboot`/`identify`/
  `baseline_request`/`shutdown`, and **`RejectMessage`** — mothership→node; a
  command is the wrong direction for a device measurement, and the rejected
  peer is by definition unauthenticated.
- **Provisioning acks and captive-portal handlers** — fixed C string literals
  and HTML over serial/`esp_http_server`, not parseable as state reports.
- **26 non-wire catalog types** — no serialiser exists, so there is no message
  to extend. (Includes the `doctor.NodeInfo` and `fleet.NodeInfo` look-alikes —
  three Go types share the name `NodeInfo`; only the `ingestion` one is wire.)

## 2. Field-type and numbering issues found

**None in what exists.** No type mismatch (all `int64`), no duplicate key, no
case collision, no drifted key name. The one *prospective* issue is in the
chain's own task text: `spaxel-3278a63f` says *"Add **uint32** free_heap_bytes
field"* — that would introduce a second width for one measurement.
**Recommendation: `int64`, matching `HealthMessage`**, from which every other
value is copied.

## 3. Actionable remaining issues

Ordered by impact. Items 1–4 are the real defects; 5–8 are housekeeping.

1. **The write path never existed — three REST surfaces serve `free_heap_bytes: 0` forever.** `Registry.UpdateNodeHealth` (`registry.go:343`) is the only
   writer of `nodes.free_heap_bytes` and is **never called**: its trigger
   `nodeHealthUpdater` has no call site and `SetNodeHealthUpdater`
   (`server.go:313`) has zero callers in the whole history (`git log -S` finds
   only its introducing commit `3d844457`). GET /api/nodes/{mac}, GET /api/fleet
   and GET /api/fleet/health therefore always report `0` while the dashboard WS
   `NodeInfo` carries the live reading. **Owner: `spaxel-3b6699a4`** (its task 1
   — the response structs — is already done; task 2 is this population).
2. **When wired as written, the first call errors.** `UpdateNodeHealth`'s
   statement sets `uptime_ms, wifi_rssi_dbm, free_heap_bytes, temperature_c,
   ip, updated_at` — columns that exist in `spaxel.db`'s `nodes` table
   (`internal/db/migrations.go:242-264`) but **not** in the `fleet.db` schema
   this `Registry` owns (`registry.go:95`). Verified live:
   `SQL logic error: no such column: uptime_ms`. The wiring bead has two jobs,
   not one: pick the write shape (slim the UPDATE to `free_heap_bytes` alone,
   or migrate the columns) *and* add the call. Same owner as item 1.
3. **`spaxel.db`'s `nodes.free_heap_bytes` is orphaned** (`internal/db/migrations.go:258`) — no reader and no writer anywhere; the AP
   detector's INSERT does not set it, so it stays NULL. Decide: read it, write
   it, or drop the column.
4. **The plan's OTA heap gate is unimplemented.** `docs/plan/plan.md:733`
   specifies *"free heap ≥ 20 KB before starting (reject … `error:"low_heap"`)"*;
   neither `low_heap` nor any heap reference exists in `firmware/`. Adding
   `free_heap_bytes` to `ota_status` is the only way its absence becomes
   observable in fleet data today. Owner: the `hello`/`ota_status` gap —
   `spaxel-3278a63f` chain.
5. **Four downstream beads are premised on the protobuf layer that does not
   exist.** This bead's own dependents — `spaxel-44ef0ad3` ("verify protobuf
   definitions"), `spaxel-d35b082f` ("regenerate protobuf Go code"),
   `spaxel-4c830849` ("test protobuf compilation") and `spaxel-3278a63f`
   ("add field to protobuf response definitions") — **cannot be executed as
   written**: there is no `.proto` file and no generated code to verify,
   regenerate or compile. Each should be re-scoped to its JSON equivalent
   (per §0's translation table) or closed as premise-false. Left uncorrected,
   each becomes loop fuel. §0 is the re-scope key.
6. **`nodeView` — a sixth heap-bearing surface exists only uncommitted.**
   `mothership/internal/fleet/handler.go` in the working tree carries a
   concurrent worker's **unpublished** `nodeView` type (`:116`, embedding
   `NodeRecord` + derived `Status`, served by `listNodes`/GET /api/nodes). It
   inherits `free_heap_bytes` through the embed. Not this bead's deliverable
   and not on origin; re-check the file before building anything on it.
7. **Unrouted duplicate: `fleethandler.getFleet`.** The handler at
   `fleethandler.go:168` projects heap at `:212`/`:237` but has **no route** —
   `Fleethandler.RegisterRoutes` registers only `/api/fleet/health`,
   `/api/fleet/history`, `/api/fleet/optimise`, `/api/fleet/simulate`. Dead
   code duplicating the live `getFleetHealth` projections; delete or route it.
8. **Stale plan/ADR text.** ADR-008 decision 5 (`plan.md:4636`) says heap
   *"is not surfaced by any API"* — it is surfaced by five (§1.1, §4.6), and
   the firmware citation drifted. Correct when the mTLS resource spike runs.
   Simulators to follow any `hello` decision: `mothership/cmd/sim/main.go:719`
   (its `:957` health fixture is already correct), `cmd/sim/main.go:475`.

## 4. What the fix looks like (for the owning beads)

- `FreeHeapBytes int64 json:"free_heap_bytes,omitempty"` on `HelloMessage`
  and `OTAStatusMessage`; `omitempty` keeps both wire-compatible in both
  directions. **`int64`, not `uint32`** (§2).
- Firmware: `cJSON_AddNumberToObject(root, "free_heap_bytes",
  esp_get_free_heap_size());` in `websocket_send_hello` and
  `websocket_send_ota_status`, mirroring `websocket.c:571`.
- Decide whether `hello`'s value also writes the persisted column —
  recommended yes, so a node that never reaches its first `health` tick still
  leaves a correct last-known heap.
- Then items 1–2: fix `UpdateNodeHealth`'s statement *and* call it.

## 5. Verification chain — which bead produced what

```
survey:  spaxel-3374abd5 (inventory) → catalog → definitions → response messages
         → messages-requiring-free-heap.md   (verdicts over all 63 types)
verify:  spaxel-527ed77b  → nodeinfo-free-heap.md      (PASS: ingestion.NodeInfo)
         spaxel-404da1fc  → free-heap-verification.md  (PASS×4 shape; findings 1-2)
summarize: spaxel-b5dc100d → THIS DOCUMENT
```

Remaining chain (all open): `44ef0ad3 → d35b082f → 4c830849 → 3278a63f →
3b6699a4 → 63ca0e88 → 68359a6a`. See §3.5 before working any of the first
four.

## 6. Evidence index

| Document | Origin commit | Carries |
|---|---|---|
| `proto-file-inventory.md` | `f5e545dd` | the no-protobuf premise |
| `response-type-message-catalog.md` | `bfd669ec` | 63 types, 37 wire / 26 not |
| `protobuf-survey-response-message-definitions.md` | `3d6a0cbc` | per-type definitions |
| `protobuf-survey-response-messages.md` | `2e1988aa` | survey capstone |
| `messages-requiring-free-heap.md` | `2ededbe6` | the three-gate rule, per-message verdicts |
| `nodeinfo-free-heap.md` | `6c93ca6b` | `NodeInfo` PASS + its test |
| `free-heap-verification.md` | `62b0d3c8` | shape tables + findings 1–2 + its tests |

Durable tests (machine-checkable, not prose-only):
`mothership/internal/ingestion/nodeinfo_test.go`,
`mothership/internal/fleet/heap_fields_test.go` (5 tests — they seed the column
directly because `UpdateNodeHealth` cannot execute; see §3.2).

## Reproduce

```bash
# premise
find . -name '*.proto' -not -path './.git/*'
git log --all --diff-filter=A --name-only --pretty=format: -- '*.proto' | grep -c proto

# every definition site of the field
grep -rn "free_heap_bytes\|FreeHeapBytes" --include='*.go' mothership/ | grep -v _test.go

# the firmware's single heap site, and the five message types it builds
grep -n "free_heap\|esp_get_free_heap_size" firmware/main/websocket.c

# the write path does not exist
grep -rn "UpdateNodeHealth\|SetNodeHealthUpdater" --include='*.go' mothership/ | grep -v _test.go
git log --oneline -S "SetNodeHealthUpdater" -- mothership/

# the statement targets the other database's schema
sed -n '95,120p' mothership/internal/fleet/registry.go
sed -n '343,352p' mothership/internal/fleet/registry.go
sed -n '242,264p' mothership/internal/db/migrations.go

# durable tests
cd mothership && go test ./internal/ingestion/ ./internal/fleet/ \
  -run 'TestNodeInfo|TestGetConnectedNodesInfo|TestHeapBearing|TestFreeHeapBytes|TestListFleetServesHeap|TestGetNodeServesHeap|TestGetFleetHealthServesHeap' -v
```
