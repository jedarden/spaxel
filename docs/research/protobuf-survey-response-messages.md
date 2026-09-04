# Protobuf survey — response messages: consolidated findings

**Date:** 2026-09-04
**Bead:** `spaxel-78a5884e` — capstone of the five-step protobuf survey chain.
**Verified at:** HEAD `0221a9d4` (= `origin/main`). Every line number in this
document is a current number on that commit except where a row says otherwise.

This document compiles the chain's findings into one place. The four earlier
steps each answered one slice of the question and carry the full tables; this
one carries the answers, the categorization, and the action items.

| Step | Bead | Document | Status |
|---|---|---|---|
| 1 | `spaxel-3278a63f` | [protobuf-survey-free-heap.md](protobuf-survey-free-heap.md) | closed |
| 2 | `spaxel-c95b4ab0` | [protobuf-survey-response-message-definitions.md](protobuf-survey-response-message-definitions.md) | closed |
| 3 | `spaxel-2e4830b7` | [proto-file-inventory.md](proto-file-inventory.md) | closed |
| 4 | `spaxel-4cc60ef7` | [response-type-message-catalog.md](response-type-message-catalog.md) | closed |
| 5 | `spaxel-9fb6a8a5` | [messages-requiring-free-heap.md](messages-requiring-free-heap.md) | closed |
| — | `spaxel-78a5884e` | **this document** | — |

---

## 1. The answer to the survey question as asked

**Spaxel contains zero `.proto` files.** No Protocol Buffers definitions, no
generated protobuf code, no protobuf-encoded traffic — and there never has
been: the type is absent from the entire git history.

```bash
find . -name '*.proto' -not -path './.git/*'                                      # no output
git log --all --diff-filter=A --name-only --pretty=format: -- '*.proto' \
  | grep -c proto                                                                 # 0 — never existed
grep -rn 'google.golang.org/protobuf' --include='*.go' mothership/ cmd/ test/     # no hits — never imported
grep -rn 'syntax = "proto' --exclude-dir=.git .                                   # only these survey docs
```

Two protobuf-shaped artifacts exist in the dependency graph and neither is
spaxel code, neither reaches the wire:

| Artifact | Where | Why it is not a spaxel protocol |
|---|---|---|
| `google.golang.org/protobuf v1.36.11` | `mothership/go.mod`, `// indirect` | Transitive module requirement. No `.go` file in `mothership/`, `cmd/` or `test/` imports any protobuf package; nothing is generated from it. |
| `protobuf-c` ESP-IDF component | `firmware/build/esp-idf/protobuf-c/` | Pulled in by ESP-IDF's `protocomm`/`wifi_provisioning`/`esp_local_ctrl`, none of which `firmware/main/` references. Definitive: `grep -cE '\bpb_(decode|encode|read|write)' firmware/build/spaxel-firmware.map` → **0** — the linker pulls no objects from `libprotobuf-c.a`. |

**Consequence that shapes everything below:** "update the protobuf response
definitions" is a **JSON struct-tag change in Go plus a matching `cJSON`
change in C**. There is no generated layer to regenerate, and no schema
compiler to run. A future task phrased in protobuf vocabulary should be read
as being about `mothership/internal/ingestion/message.go` and
`firmware/main/websocket.c`.

---

## 2. What stands in for `.proto` — where the schema actually lives

The protocol's schema registry is a set of Go structs and C `cJSON` builders,
spread across six definition sites (full inventory: step 3):

| Surface | Schema definition site | Encoding |
|---|---|---|
| Node ↔ mothership WebSocket (**primary**) | `mothership/internal/ingestion/message.go` (13 types) + `firmware/main/websocket.c` (senders) | JSON, `"type"` discriminator, snake_case, `omitempty`, 4 KB max, unknown `type` ignored |
| CSI samples | `firmware/main/csi.c` (encode) / `mothership/internal/ingestion/frame.go:47` (`ParseFrame`) | WebSocket **binary** frames: 24-byte header + `n_sub`×2 int8 I/Q |
| REST API | named Go structs in `internal/api/*`, `internal/fleet/*`, `internal/health`, `internal/doctor` | JSON response bodies over chi |
| Dashboard WebSocket | `internal/dashboard/hub.go` (snapshot + broadcast envelopes) | JSON push; client never replies with a typed message |
| Provisioning REST | `internal/provisioning/server.go:24` (`Payload`) | JSON |
| Serial provisioning | `firmware/main/provision.c` (string literals) | newline-delimited JSON over UART0 + USB-Serial/JTAG (ADR-002) |

`ParseJSONMessage` (`message.go:146`) is the authoritative upstream decoder:
its switch accepts exactly five types, which proves by construction that those
five are the only messages a node can send. `cmd/sim/main.go` builds its
upstream messages as inline `map[string]interface{}` literals with no named
wire types — a protocol field addition must be mirrored there by hand.

---

## 3. All response-type messages found

### 3.1 The complete tally

Step 4 scanned for the five name patterns (`*Response`, `*Reply`, `*Info`,
`*Status`, `*Result`), split wire from non-wire by tracing an actual
serialisation site, then added the response-role types the suffixes miss:

| Group | Wire | Non-wire | Total |
|---|---|---|---|
| `*Response` | 23 | 0 | 23 |
| `*Status` | 3 | 2 | 5 |
| `*Info` | 4 | 10 | 14 |
| `*Result` | 3 | 14 | 17 |
| `*Reply` | 0 | 0 | 0 |
| response-role, no matching suffix | 4 | — | 4 |
| **Total** | **37** | **26** | **63** |

Two properties of that tally matter more than the count:

- **No type ends in `*Reply`.** The pattern is not part of this codebase's
  vocabulary; the node protocol says `OTAStatusMessage`, REST says `*Response`.
- **26 of the 63 are not messages at all** — internal domain values that
  borrowed the vocabulary. Several carry live `json:` tags despite never being
  marshalled (`automation.ActionResult`, `api.SessionInfo`,
  `sleep.SleepStatus`, `analytics.AnomalyResult`, …), which is exactly the
  trap a tag- or name-only scan falls into.

### 3.2 The node WebSocket protocol (the primary surface)

All 13 types live in `mothership/internal/ingestion/message.go`; firmware
senders in `firmware/main/websocket.c`; mothership senders in
`internal/ingestion/server.go`.

**Upstream (node → mothership):**

| Message | Go | Firmware sender | Role |
|---|---|---|---|
| `HelloMessage` | `message.go:11` | `websocket.c:484` (builder `websocket_send_hello`, `:478`) | First message on connect — handshake answer to the connection itself |
| `HealthMessage` | `message.go:39` — **`free_heap_bytes` at `:43`** | `websocket.c:563` | Periodic ~10 s health report; unsolicited |
| `BLEMessage` (+`BLEDevice`) | `message.go:66` | `websocket.c:642` | Periodic ~5 s scan report; unsolicited |
| `MotionHintMessage` | `message.go:74` | `websocket.c:690` | Event-driven variance signal; unsolicited |
| `OTAStatusMessage` | `message.go:82` | `websocket.c:725` (builder `:718`) | **The only strictly typed response** — answers `OTAMessage` |

**Downstream (mothership → node):** `RoleMessage` (`:93`), `ConfigMessage`
(`:100`), `OTAMessage` (`:109`), `RebootMessage` (`:117`),
`IdentifyMessage` (`:123`), `BaselineRequestMessage` (`:129`),
`ShutdownMessage` (`:134`), `RejectMessage` (`:140`).

Strictly request/response in shape: **`OTAMessage` → `OTAStatusMessage`**.
Everything else is a periodic report or a command acknowledged implicitly by
behaviour. Two downstream messages have **no distinct response type at all**:
`BaselineRequestMessage` (data re-arrives via the normal CSI/BLE flow) and
`IdentifyMessage` (firmware-side LED blink). `RejectMessage` is a terminal
response to a failed handshake — the connection closes after it.

### 3.3 REST — the RPC-shaped surface

23 wire `*Response` types, plus `provisioning.Payload` and the two bare-name
`Response` types. Condensed here; the full route/builder tables are steps 2
and 4.

**`internal/fleet` (6, HEAD line numbers):**

| Type | Defined | Serves |
|---|---|---|
| `FleetNode` | `fleet/handler.go:125` (`FreeHeapBytes` `:146`) | element of `GET /api/fleet`'s body |
| `fleetListResponse` | `fleet/handler.go:151` | `GET /api/fleet` |
| `fleetHealthResponse` (+`fleetNodeEntry`, heap at `fleethandler.go:74`) | `fleet/fleethandler.go:54` | `GET /api/fleet/health` |
| `optimiseResponse` | `fleet/fleethandler.go:294` | `POST /api/fleet/optimise` |
| `simulateResponse` | `fleet/fleethandler.go:315` | `GET /api/fleet/simulate` |
| `systemModeResponse` + `autoAwayConfigResponse` | `fleet/handler.go:762`, `:768` | `GET`/`POST /api/mode` |

**`internal/api` (15):** `ActiveAlertsResponse` (`alerts.go:39`),
`captureResponse` (`baseline.go:95`), `crossingResponse` (`zones.go:80`),
`eventsResponse` (`events.go:211`), `GDOPResponse` (`simulator.go:496`),
`SimulationResponse` (`simulator.go:709`), `SystemModeResponse`
(`security.go:57`), `integrationSettingsResponse` (`integrations.go:121`),
`networkSettingsResponse` / `networkSettingsWithPasswordResponse`
(`network_settings.go:37`/`:45`), `notificationConfigResponse`
(`notifications.go:321`), `notificationSettingsResponse`
(`notification_settings.go:87`), `testNotificationResponse`
(`notifications.go:381`), `occupancyResponse` (`status.go:135`),
`TriggerResponse` (`volume_triggers.go:66`).

**Other packages:** `health.Response` (`internal/health/health.go:49`,
`GET /healthz`) and `doctor.Response` (`internal/doctor/doctor.go:73`,
`GET /api/doctor`) — unrelated types sharing the bare name `Response`, always
read them package-qualified. `provisioning.Payload`
(`internal/provisioning/server.go:24`) is `POST /api/provision`'s body.

**Not inventoriable:** the ~82 inline handlers in `cmd/mothership/main.go`
mostly write `map[string]interface{}` or ad-hoc literals — no named type
exists beyond the route. Test-only helpers never leave the test process and
are excluded.

### 3.4 Name collisions — always package-qualify

| Bare name | Distinct types |
|---|---|
| `Response` | `health.Response` · `doctor.Response` |
| `NodeInfo` | `ingestion.NodeInfo` (wire, dashboard WS) · `doctor.NodeInfo` (internal) · `fleet.NodeInfo` (optimiser input) |
| `ActionResult` | `api.ActionResult` (**wire**) · `automation.ActionResult` (internal, carries `json:` tags but is event-log only) |
| `IdentityMatch` | `api.IdentityMatch` (internal, read-only) · `ble.IdentityMatch` (domain model) |
| `ZoneInfo` | `simulator.ZoneInfo` · `guidedtroubleshoot.ZoneInfo` |

---

## 4. Categorization: which messages need `free_heap_bytes`

Step 5 turned the dispatch's heuristic ("if a message represents a node or
device reporting its state, it should include `free_heap_bytes`") into one
question and three gates:

> **Whose state is this message reporting?**

A message carries `free_heap_bytes` only if **all three** hold:

1. **Origin** — the payload originates on a device (an ESP32-S3 node). Only
   the device can call `esp_get_free_heap_size()`.
2. **Subject** — the payload's subject is the *reporting device itself*, not
   something it merely observed.
3. **Survivability** — the message plausibly reaches the mothership when
   `health` has not. `health` fires every 10 s per connected node and its
   value is already persisted, so a message that can only arrive between two
   `health` ticks adds a value the next tick makes redundant. The messages
   that survive are the ones sent **outside** the steady state: connection,
   and OTA.

Gate 3 is what keeps the answer small.

### 4.1 Needs it — the two gaps

| # | Message | Go | Firmware sender | Heap today |
|---|---|---|---|---|
| gap 1 | `HelloMessage` | `message.go:11` | `websocket.c:478` | **none** |
| gap 2 | `OTAStatusMessage` | `message.go:82` | `websocket.c:718` | **none** |

Verified current: `FreeHeapBytes` appears exactly once in `message.go` (`:43`,
inside `HealthMessage`), and `free_heap_bytes`/`esp_get_free_heap_size()`
appear exactly once in the whole firmware tree (`websocket.c:571-572`, the
health builder). Both gaps are **unimplemented as of this writing**.

**Why `hello`:** origin device, subject device, and by construction the first
observation of a boot. It already carries the other boot-diagnosis fields for
exactly this audience — `safe_mode_active` and `boot_count`
(`message.go:23`-24) exist to explain *why* a node came back wrong; heap is
the missing member of that set, and the reading most likely to explain a boot
failure (a heap-starved boot degrades before it reaches the first 10 s
`health` tick). It is also the first reading after every OTA reboot, which is
where the previous firmware version's heap behaviour gets compared against the
new one. It fires precisely when gate 3 says a per-message copy is worth it: a
node that never gets far enough to send `health` still sends `hello`.

**Why `ota_status`:** the only strictly typed request/response pair in the
node protocol, firing during the one operation that *itself* consumes heap —
the download and write buffers of `esp_https_ota`/`esp_ota_begin`. An OTA that
ends in a reboot or boot loop takes the 10 s `health` tick with it, so this is
the non-steady state in which `health` is the least reliable reporter.

**Corroborating finding: the plan's OTA heap gate is not implemented.**
`docs/plan/plan.md:733` specifies *"Check: free heap ≥ 20 KB before starting
(reject if insufficient, send `ota_status: failed, error:"low_heap"`)"*. The
string `low_heap` and any heap reference are absent from `firmware/` — the
only heap symbol in the tree is the health builder. The gate is specified and
absent, and `free_heap_bytes` on `ota_status` is the only way its absence
becomes observable in fleet data today.

### 4.2 Already compliant — no action

`health` and its four projections are the compliant baseline the two gaps
close. Nothing further to add to any of these:

| Surface | Where |
|---|---|
| `HealthMessage` (upstream WS) | `ingestion/message.go:43` — the only **required** (non-`omitempty`) heap field |
| `fleetListResponse.FleetNode` | `fleet/handler.go:146` (HEAD), filled `:213` → `GET /api/fleet` |
| `fleetHealthResponse.fleetNodeEntry` | `fleet/fleethandler.go:74`, filled `:122`/`:148` → `GET /api/fleet/health` |
| `NodeRecord` (persisted + raw body) | `fleet/registry.go:28` (heap field `:44`), column added `:159`, UPDATE `:346`; marshalled directly by `GET /api/nodes/{mac}` |
| `ingestion.NodeInfo` (dashboard WS) | `internal/ingestion/server.go:1056`, filled `:1071` from `nc.LastHealth` |

### 4.3 Does not need it — grouped by the gate that excludes it

**Fails gate 1 (origin is the mothership).** All 23 wire `*Response` types
plus `SecurityStatus`, `DiurnalLearningStatus`, `LinkInfo`,
`LinkHealthInfo`, `replayInfo`, `WebhookTestResult`, `ActionResult`,
`CheckResult` are computed in `mothership/` from stored state. No Go process
can know an ESP32's heap, and a mothership reporting its *own* heap under a
field named for the device metric would be actively misleading. The
most tempting cases individually: `optimiseResponse`/`simulateResponse`
describe a *hypothetical* fleet, so any predicted heap figure would be a
guess; `fleetHealthResponse` already carries per-node heap, so a fleet-wide
min/avg is a dashboard-side derivation of present data, not a new protocol
field (step 1 had listed this as merely "optional" — it is already satisfied
per-node); `health.Response`/`doctor.Response` are mothership self-reports, a
different field name and a different task.

**Fails gate 2 (device-originated, but the subject is not the device).**
`ble` (`websocket.c:642`) is the sharpest case: node-*originated*, which is
exactly why an origin-only rule gets it wrong, but its payload is
`[]BLEDevice` — other devices' addresses, RSSI and manufacturer data. Adding
the reporter's heap to a report about third parties attaches a per-scan
attribute to a device list, where it reads as one of them. `motion_hint`
(`websocket.c:690`) is a one-number event signal; gate 3 also applies.

**Fails gate 3 (steady-state redundant, or not a state report).** All eight
downstream commands (`role`, `config`, `ota`, `reboot`, `identify`,
`baseline_request`, `shutdown`) — mothership→node, the wrong direction for a
device measurement, acknowledged by behaviour. `RejectMessage` — the rejected
peer is by definition untrusted, and also the one case where attaching heap
would leak device internals to a peer that just failed auth. The serial
provisioning `{"ok":...}` literals (`provision.c:138`-`206`) and captive
portal handlers (`wifi.c:373`-550) — emitted with no AP association and no
WebSocket; provisioning UI, not device telemetry. The 26 non-wire catalog
types have no serialiser at all, so there is no message to extend.

**Simulators** follow whatever the node protocol decides. Both send `hello`
(`mothership/cmd/sim/main.go:719`, `cmd/sim/main.go:475`), neither carries
heap; the in-module one also sends `health` hardcoded `200000`
(`mothership/cmd/sim/main.go:957`, already correct). The standalone `cmd/sim`
sends no `health` at all. If `hello` gains the field, the two sim senders
should follow so test traffic matches production traffic.

---

## 5. Action items

### 5.1 The implementation the chain was building toward

Four beads remain **open** and carry this work. Their titles say "protobuf
response definitions"; given the §1 finding, what they actually describe is
the JSON field below plus its firmware sender — there is no protobuf layer to
regenerate.

| Bead | Title |
|---|---|
| `spaxel-3278a63f` | Add free_heap_bytes to protobuf response definitions |
| `spaxel-68359a6a` | Surface free heap in mothership API |
| `spaxel-3b6699a4` | Update API handlers to populate free_heap_bytes |
| `spaxel-63ca0e88` | Verify free_heap_bytes in API responses |

The concrete edits, consolidated from steps 1 and 5:

1. **Go — `mothership/internal/ingestion/message.go`:** add
   `FreeHeapBytes int64 \`json:"free_heap_bytes,omitempty"\`` to
   `HelloMessage` (`:11`) and `OTAStatusMessage` (`:82`). `int64` to match the
   existing `:43` field; `omitempty` keeps both directions wire-compatible.
   Wire compatibility is **proven, not assumed** — unknown JSON fields are
   ignored on unmarshal (`internal/ingestion/json_fuzz_test.go:117` round-trips
   `{"type":"hello",...,"extra":"ignored"}`) — so either side may add the key
   before the other learns about it.
2. **Firmware — `firmware/main/websocket.c`:** add
   `cJSON_AddNumberToObject(root, "free_heap_bytes", esp_get_free_heap_size());`
   to `websocket_send_hello` (`:478`) and `websocket_send_ota_status` (`:718`),
   mirroring the existing health builder at `:571`-`572`.
3. **Decide whether `hello`'s value writes the persisted
   `nodes.free_heap_bytes` column** (`registry.go:159`, currently updated only
   by the health UPDATE at `:346`). Step 5's recommendation: **yes** — it makes
   a node's last known heap correct even when it boots into a state where it
   never reaches the first `health` tick, which is the exact case gap 1 exists
   for. **No schema migration is needed** — the column already exists.
4. **Simulator parity:** `mothership/cmd/sim/main.go:719` and
   `cmd/sim/main.go:475` (both send `hello`).
5. **Test fixtures:** `internal/ingestion/message_test.go:9`,
   `mothership/tests/e2e/e2e_test.go:738`. Per repo rules: table-driven tests
   alongside the change, plus `cd mothership && go test ./...` and
   `go vet ./...` before closing.

Result once landed: heap visible at **three points across the node
lifecycle** — initial connection (`hello`), steady state (`health`, already
present), and firmware update (`ota_status`) — with no noise added to
event-type messages.

### 5.2 Adjacent findings surfaced by the survey — need their own beads

These were found and recorded during the chain but deliberately **not** fixed
by it, which was documentation-only:

| Finding | Detail | Why not fixed here |
|---|---|---|
| **OTA `low_heap` gate unimplemented** | `plan.md:733` specifies a free-heap ≥ 20 KB pre-flight check emitting `ota_status: failed, error:"low_heap"`; absent from `firmware/` entirely | Firmware behaviour change; belongs with the `ota_status` field work so the gate becomes observable |
| **`GET /api/diurnal/status` registered twice, first registration dead** | `cmd/mothership/main.go:3693` (inline closure) and `internal/api/diurnal.go:66`. Chi does not reject a duplicate `GET` on the same pattern — verified by executing it — and silently keeps the **second**, so `DiurnalHandler.getDiurnalStatus` (`api/diurnal.go:72`) is what actually serves the route. The inline closure's `/api/diurnal/status/{linkID}` at `main.go:3697` is *not* duplicated and survives | Behaviour-adjacent; needs its own bead with its own test |
| **`POST /api/notifications/test` registered twice (both live)** | `notifications.go:316` (conditional) and `notification_settings.go:83` (unconditional); which handler answers depends on construction order | Same |
| **ADR-008 decision 5's premise is stale** | `plan.md:4636` says node `free_heap_bytes` is *"not surfaced by any API"* and cites `websocket.c:494`. It is surfaced by four APIs (§4.2) and the firmware site is now `:571`. An mTLS resource-spike go/no-go needs only a *measurement* on hardware — no new plumbing | Plan-documentation edit; update when the spike runs |
| **`GET /api/nodes` has no named response type at HEAD** | At HEAD the route marshals `[]NodeRecord` directly. An in-flight uncommitted edit adds a `nodeView` wrapper (see §6) | Owned by the concurrent worker editing that file |

---

## 6. Corrections step 5 made to step 1 — do not implement from the stale premise

Step 1 was written first and against the five upstream node messages only.
Step 5 verified every claim against live code and corrected three things that
this summary adopts in place of step 1's originals:

1. **"No migration needed" — right answer, wrong reason.** Step 1 said heap is
   not persisted separately. It is: `nodes.free_heap_bytes` has existed since
   `internal/fleet/registry.go:159`, written by the health UPDATE at `:346`.
   No migration is needed for the same reason the recommendation in §5.1(3)
   exists — the column is already there, and the open question is whether
   `hello` should *write* it.
2. **`fleetHealthResponse` is not "optional".** Step 1 listed a fleet-wide
   heap aggregate as an optional addition. Per-node heap already flows through
   `fleetNodeEntry` (`fleethandler.go:74`), so the aggregate is a
   dashboard-side derivation of present data, not a protocol gap.
3. **Stale citations.** Step 1's `HealthMessage` at `message.go:41` is now
   `:39` with the field at `:43`. Its two struct snippets are otherwise still
   accurate.

## 7. Line-number drift to expect

`internal/fleet/handler.go` carries an **uncommitted** concurrent-worker edit
in the working tree (`nodeView` for `GET /api/nodes`, +31/37 lines, hunk
`@@ -109,16 +109,47 @@`) that shifts every line below ~109. It was **not** at
HEAD `0221a9d4` / `origin/main` when this document was written and verified,
so every `fleet/handler.go` number above is a **HEAD** number
(`FleetNode:125`, `fleetListResponse:151`, `systemModeResponse:762`). If that
edit lands, re-derive those four rows before citing them — and it adds a fifth
heap-bearing response surface for `GET /api/nodes` once it does.

`internal/ingestion/server.go` drifts often (~7 lines between steps 2 and 3).
Re-derive sender line numbers rather than trusting any document in this chain,
including this one. The `ingestion/message.go`, `firmware/main/websocket.c`
and `fleet/fleethandler.go` numbers above were re-verified at HEAD at the top
of this document.

## 8. Methodology

Three techniques, in descending order of strength — the same evidence
hierarchy steps 3–5 used, so every wire claim in this chain is backed by one
of them:

1. **A traced serialisation site.** A `writeJSON`/`json.NewEncoder` call whose
   argument is the type (or a struct embedding it), or a traced marshal into a
   WebSocket frame. Route/builder pairs were read by hand from each
   `RegisterRoutes` and each builder — a mechanical route scan mis-attributes,
   because most routes live in per-package `RegisterRoutes` methods rather
   than on a single root router.
2. **Proof by construction.** `ParseJSONMessage` (`message.go:146`) switches
   on exactly five upstream types, so nothing outside them can be a node
   response; that is what bounds §3.2 without trusting a naming convention.
3. **Absence of all three**, plus a repo-wide grep for the type name excluding
   its own defining file, for every non-wire row — which is how the
   `json:`-tagged-but-never-marshalled types were kept out of the wire counts.

The mechanical scan that seeded the catalog:

```bash
grep -rnE '^type [A-Za-z0-9_]*(Response|Reply|Info|Status|Result)\b' \
  mothership/ cmd/ test/ tests/ --include='*.go' \
  | grep -v '_test.go' | grep -v 'test_helpers.go'   # 56 types
```

Note the `*` after the character class: a `+` there silently drops the three
bare-name declarations (`health.Response`, `doctor.Response`,
`fusion.Result`). The scan yields 56, not the 59 suffix-named types, because a
name scan under-counts in three further ways — `ingestion.OTAStatusMessage`
(role-matched, `…Message` suffix), `api.IdentityMatch`, and
`explainability.FusionResultSnapshot` — each of which was carried in by
reading the response builders rather than the pattern.

## Reproduce

```bash
# the zero-.proto premise
find . -name '*.proto' -not -path './.git/*'
git log --all --diff-filter=A --name-only --pretty=format: -- '*.proto' | grep -c proto

# heap appears exactly once in message.go and once in the firmware tree
grep -n "FreeHeapBytes" mothership/internal/ingestion/message.go
grep -rn "free_heap_bytes\|esp_get_free_heap_size" firmware/main/

# the plan's OTA heap gate is unimplemented (no hits expected)
grep -rn "low_heap" firmware/main/ mothership/

# every mothership surface already carrying heap
grep -rn "FreeHeapBytes" mothership/internal/ --include=*.go | grep -v _test

# hello tolerates unknown fields — the wire-compat proof
grep -n 'extra":"ignored"' mothership/internal/ingestion/json_fuzz_test.go
```
