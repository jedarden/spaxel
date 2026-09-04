# Messages requiring `free_heap_bytes` — verdicts over the full response catalog

**Date:** 2026-09-03
**Task:** For each response-type message in the catalog, decide whether it should
carry `free_heap_bytes`, with per-message justification and an explicit
does-not-need list.
**Inputs:** `docs/research/response-type-message-catalog.md` (63 types, 37 wire /
26 non-wire) and `docs/research/protobuf-survey-free-heap.md` (step 1, which
made a first pass over the 5 upstream node messages only).
**Scope:** analysis only. No code or firmware was changed for this document;
every claim below is read off the tree at the cited line.

## 0. The decision rule

The dispatch's heuristic is *"if a message represents a node or device
reporting its state, it should include free_heap_bytes."* Applied to the
catalog, that resolves into one question and two gates:

> **Whose state is this message reporting?**

A message carries `free_heap_bytes` only if **all three** hold:

1. **Origin** — the payload originates on a device (an ESP32-S3 node), not in a
   mothership process. The field is `esp_get_free_heap_size()`; only the device
   can measure it.
2. **Subject** — the payload's subject is the *reporting device itself*, not
   something the device merely observed. A node reporting BLE advertisements it
   overheard is reporting other devices' state; a node reporting `health` is
   reporting its own.
3. **Survivability** — the message is a state snapshot that plausibly reaches
   the mothership when `health` has not. `health` fires every 10 s per
   connected node and its value is already persisted
   (`internal/fleet/registry.go:159`, `:346`), so any message that can only
   arrive on a healthy, connected node between two `health` ticks adds a value
   the next tick makes redundant. The messages that survive this gate are the
   ones sent **outside** the steady state: connection, and OTA.

Gate 3 is what keeps the answer small: heap on a connected node is never
staler than ~10 s, so per-message copies are only worth their wire cost where
the node is *not* in the steady state.

## 1. Messages that need it

### 1.1 `hello` — needs it, does not have it (gap 1)

| Sender | Site | Heap today |
|---|---|---|
| ESP32-S3 firmware | `firmware/main/websocket.c:478` (`websocket_send_hello`) | none — `free_heap_bytes` appears exactly once in the whole firmware tree, at `websocket.c:571`, inside the health builder |
| in-module simulator | `mothership/cmd/sim/main.go:719` | none |
| standalone simulator | `cmd/sim/main.go:475` | none |

Go side: `internal/ingestion/message.go:11` (`HelloMessage`) has no heap
field.

**Why it qualifies.** `hello` is the handshake *answer to the connection
itself* (catalog §2.6) — origin device, subject device, and by construction the
first observation of a boot. It already carries the other boot-diagnosis
fields for exactly this audience: `safe_mode_active` and `boot_count`
(`message.go:23`-24) exist to explain *why* a node came back wrong. Heap at
hello is the missing member of that set: it is the earliest post-boot reading,
and the reading most likely to explain a boot failure (a heap-starved boot
degrades before it reaches the first 10 s `health` tick). It is also the first
reading after every OTA reboot, which is where the previous firmware version's
heap behaviour gets compared against the new one.

`hello` fires precisely when gate 3 says a per-message copy is worth it: a node
that never gets far enough to send `health` still sends `hello`.

**Wire compatibility is already proven, not assumed.** Unknown JSON fields are
ignored on unmarshal — `internal/ingestion/json_fuzz_test.go:117`
(`{"type":"hello",...,"extra":"ignored"}` round-trips) — and every sender may
add the key before any receiver learns about it. Old firmware keeps working
against a new mothership, and vice versa.

### 1.2 `ota_status` — needs it, does not have it (gap 2)

| Site | Heap today |
|---|---|
| firmware builder `firmware/main/websocket.c:718` (`websocket_send_ota_status`) | none |
| Go struct `internal/ingestion/message.go:82` (`OTAStatusMessage`) | none |

Fields today: `mac`, `state`, `progress_pct`, `error` (`websocket.c:726`-738).

**Why it qualifies.** This is the only strictly typed request/response pair in
the node protocol (catalog §2.2): it answers `OTAMessage`
(`message.go:109`), and it fires during the one operation that *itself*
consumes heap — the download and write buffers of `esp_https_ota`/`esp_ota_begin`,
which live in `websocket.c` (the only file in `firmware/main/` that calls
them). Origin device, subject device, and a non-steady state in which `health`
is the least reliable reporter: an OTA that ends in a reboot or a boot loop
takes the 10 s tick with it.

**Corroborating finding: the plan's OTA heap gate is not implemented.**
`docs/plan/plan.md:733` specifies *"Check: free heap ≥ 20 KB before starting
(reject if insufficient, send `ota_status: failed, error:"low_heap"`)"*.
Neither the string `low_heap` nor any heap reference exists anywhere in
`firmware/` — the only heap symbol in the firmware tree is the health builder
at `websocket.c:571`. The gate is specified and absent, and `free_heap_bytes`
on `ota_status` is the only way its absence becomes observable in fleet data
today: a `failed` status carrying the heap at failure time distinguishes
"download failed" from "never had the memory to try".

### 1.3 Messages that already comply

| Message | Device field | Mothership surfaces already carrying it |
|---|---|---|
| `health` (upstream) | `internal/ingestion/message.go:43` (`FreeHeapBytes int64`, required — the only non-`omitempty` heap field) | — |
| `fleetListResponse.FleetNode` | `internal/fleet/handler.go:177`, filled at `:244` | `GET /api/fleet` |
| `fleetHealthResponse.fleetNodeEntry` | `internal/fleet/fleethandler.go:74`, filled at `:122`/`:148` | `GET /api/fleet/health` |
| `NodeRecord` (raw struct as body) | `internal/fleet/registry.go:44`, column added `:159`, UPDATE `:346` | `GET /api/nodes/{mac}` via `getNode` (`internal/fleet/handler.go:358`) marshals the record directly |
| `ingestion.NodeInfo` | `internal/ingestion/server.go:1061`, filled at `:1077` from `nc.LastHealth` | dashboard WebSocket `delta["nodes"]` (`hub.go:128`, `:782`) |

Nothing further to add to any of these — `health` and its four projections are
the compliant baseline the two gaps above close.

*Working-tree disclosure:* `internal/fleet/handler.go` currently also carries an
**uncommitted** `nodeView` edit from a concurrent worker (a list-view wrapper
around `NodeRecord` for `GET /api/nodes`, `+34/-3`). That change adds a fifth
heap-bearing surface and is not part of this bead's deliverable; the line
numbers in this document are read from that working tree, so `GET /api/nodes`
may not show them at `origin/main` until the twin publishes.

## 2. Messages that do not need it

Every type below fails at least one gate of §0. Grouped by the gate that
excludes it, since the *reason* is the deliverable, not the list.

### 2.1 Fails gate 1 (origin is the mothership) — the entire `*Response` family

All 23 wire `*Response` types (catalog §2.1) plus `SecurityStatus`,
`DiurnalLearningStatus`, `LinkInfo`, `LinkHealthInfo`, `replayInfo`,
`WebhookTestResult`, `ActionResult`, `CheckResult` are computed in
`mothership/` from stored state. No Go process can know an ESP32's heap, and a
mothership reporting its *own* heap under a field named for the device metric
would be actively misleading. The most tempting cases, individually:

| Type | Why not |
|---|---|
| `optimiseResponse`, `simulateResponse` (`fleethandler.go:294`, `:315`) | describe a *hypothetical* fleet (coverage before/after removal). Any predicted heap figure would be a guess; the node's next real `health` tick reports the actual consequence within 10 s. |
| `fleetHealthResponse` | already carries per-node heap (§1.3). A fleet-wide min/avg aggregate is a dashboard-side derivation of data already present — not a new protocol field. Step 1 listed this as "optional"; it is already satisfied per-node. |
| `health.Response` (`internal/health/health.go:49`), `doctor.Response` (`internal/doctor/doctor.go:73`) | mothership self-report. The dispatch's rule is scoped to *node or device* responses; mothership memory belongs to a different field name and a different task. |
| alerts, events, occupancy, zones, triggers, replay, settings, simulator, GDOP | subjects are people, zones, sessions, and configuration — none of them devices reporting state. |

### 2.2 Fails gate 2 (device-originated, but the subject is not the device)

| Message | Site | Why not |
|---|---|---|
| `ble` | `websocket.c:635`, `message.go:66` | the sharpest case in the set. It is node-*originated*, which is exactly why a suffix- or origin-only rule gets it wrong: its payload is `[]BLEDevice` — other devices' addresses, RSSI and manufacturer data. Adding the reporter's heap to a report about third parties attaches a per-scan attribute to a device list, where it reads as one of them. |
| `motion_hint` | `websocket.c:676`, `message.go:74` | a one-number event signal (`variance`). Gate 3 also applies: it fires on a healthy connected node, so heap is at most 10 s stale from `health`. |

### 2.3 Fails gate 3 (steady-state redundant) or is not a state report at all

| Message | Site | Why not |
|---|---|---|
| downstream commands `role`, `config`, `ota`, `reboot`, `identify`, `baseline_request`, `shutdown` | `message.go:93`-137 | mothership→node. A command is the wrong direction for a device measurement; these are acknowledged by behaviour, not by a body. |
| `RejectMessage` | `message.go:140` | mothership→node terminal response to a failed handshake. The rejected peer is by definition untrusted/unauthenticated — it is also the one case where attaching heap would leak device internals to a peer that just failed auth. |
| provisioning responses `{"ok":true/false,...}` | `firmware/main/provision.c:166`, `:138`, `:147`, `:155`, `:173`, `:202` | fixed C string literals acking a config write over serial, emitted while the node has no AP association and no WebSocket. Not a state report; also not parseable as one. |
| captive portal handlers | `firmware/main/wifi.c:373`-550 | HTML/text forms via `esp_http_server`. Provisioning UI, not device telemetry. |
| 26 non-wire catalog types | catalog §2.2-2.4 non-wire tables | no serialiser exists, so there is no message to extend. Noted here only to bound the list; they need no per-type rationale. |

### 2.4 Simulators

Both simulators send `hello` (and the in-module one sends `health`, hardcoded
`200000` at `mothership/cmd/sim/main.go:957`). They follow whatever the node
protocol decides — if `hello` gains the field (§1.1), the two sim senders
should follow so test traffic matches production traffic; if it does not, they
need nothing. The standalone `cmd/sim` sends no `health` at all (hello +
binary CSI only, `cmd/sim/main.go:475`), so it has no other surface in this
question.

## 3. Corrections to prior steps in this chain

Found while verifying against live code; recorded here so the next step does
not implement from a stale premise.

1. **ADR-008 decision 5's premise is stale.** `docs/plan/plan.md:4636` states
   the node reports `free_heap_bytes` but *"it simply is not surfaced by any
   API"*, and cites `websocket.c:494`. Both halves are now wrong: it is
   surfaced by four APIs (§1.3), and the firmware site drifted to
   `websocket.c:571`. An mTLS resource-spike go/no-go needs only a *measurement*
   on hardware — no new plumbing. The plan's ADR text should be updated when
   that spike runs.
2. **Step 1 said "no migration needed — heap is not persisted separately."**
   It is: `nodes.free_heap_bytes` has existed since
   `internal/fleet/registry.go:159`, written by the health UPDATE at `:346`.
   Adding the field to `hello`/`ota_status` therefore needs **no** migration —
   but for the opposite reason step 1 gave. Whether `hello`'s value should
   *update the persisted column* is a real implementation decision the next
   step must make (§4).
3. **Step 1's `fleetHealthResponse` "optional" item is already implemented**
   (per-node, §1.3), and its `HealthMessage` citation (`message.go:41`) has
   drifted to `:39`/`:43`. Its two struct snippets are otherwise still accurate.

## 4. Implementation notes for the next step (not done here)

- Add `FreeHeapBytes int64 \`json:"free_heap_bytes,omitempty"\`` to
  `HelloMessage` (`message.go:11`) and `OTAStatusMessage` (`message.go:82`);
  `omitempty` keeps both wire-compatible in both directions (proof: §1.1).
- Firmware: `cJSON_AddNumberToObject(root, "free_heap_bytes",
  esp_get_free_heap_size());` in `websocket_send_hello` and
  `websocket_send_ota_status`, mirroring `websocket.c:571`-572.
- Decide whether `hello`'s value should also write the persisted
  `nodes.free_heap_bytes` column. Recommendation: yes — it makes a node's last
  known heap correct even when it boots into a state where it never reaches the
  first `health` tick, which is the exact case §1.1 exists for.
- Sim senders to update for parity: `mothership/cmd/sim/main.go:719` (and its
  existing `:957` health fixture is already correct), `cmd/sim/main.go:475`;
  fixtures `internal/ingestion/message_test.go:9`,
  `mothership/tests/e2e/e2e_test.go:738`.

## Reproduce

```bash
# heap appears exactly once in the firmware, inside the health builder
grep -rn "free_heap_bytes\|esp_get_free_heap_size" firmware/

# the plan's OTA heap gate is unimplemented (no hits expected)
grep -rn "low_heap" firmware/ mothership/

# hello senders, none carrying heap
grep -n '"type"' firmware/main/websocket.c mothership/cmd/sim/main.go cmd/sim/main.go | grep -i hello

# every mothership surface already carrying heap
grep -rn "FreeHeapBytes" mothership/internal/ --include=*.go | grep -v _test

# hello tolerates unknown fields (wire-compat proof)
grep -n 'extra":"ignored"' mothership/internal/ingestion/json_fuzz_test.go
```
