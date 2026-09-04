# `.proto` file inventory — complete survey (final)

Bead: `spaxel-2e4830b7`. Third and final step of the protobuf survey chain:

1. [protobuf-survey-free-heap.md](protobuf-survey-free-heap.md) — message catalog, `free_heap_bytes` gap analysis
2. [protobuf-survey-response-message-definitions.md](protobuf-survey-response-message-definitions.md) — response-role messages mapped to defining files and RPCs
3. **this document** — the consolidated inventory

Line numbers are current working-tree numbers except where a table says
otherwise. Everything below was re-derived at HEAD `3d6a0cbc`, not carried
forward from the earlier steps — the step-2 note warned that `internal/ingestion/server.go`
had already drifted ~7 lines between step 1 and step 2, and it had drifted
again by the time this was written.

---

## 1. The answer: zero `.proto` files

Spaxel contains **no Protocol Buffers definition files, no generated protobuf
code, and no protobuf-encoded traffic**. Every wire format in the system is
either JSON or a hand-rolled binary frame. Reproduce from the repo root:

```bash
find . -name '*.proto' -not -path './.git/*'                       # no output
grep -rn 'syntax = "proto' --exclude-dir=.git .                    # 1 hit: this doc chain quoting itself
git log --all --diff-filter=A --name-only --pretty=format: -- '*.proto' | grep -c proto   # 0 — never existed in history
grep -rn 'google.golang.org/protobuf' --include='*.go' mothership/ cmd/ test/   # no hits — never imported
grep -rn 'cJSON\|json:' firmware/main/                             # the real protocol lives here
```

The single `syntax = "proto"` hit in the tree is
`docs/research/protobuf-survey-response-message-definitions.md:21`, which
quotes the grep command as evidence. There is no `.proto` file anywhere in
the repository or its history.

### Near-miss protobuf artifacts — documented so nobody re-chases them

Two protobuf-shaped things exist in the dependency graph. Neither is spaxel
code, neither reaches the wire, and neither implies a `.proto` file to find:

| Artifact | Where | Why it is not a spaxel protocol |
|---|---|---|
| `google.golang.org/protobuf v1.36.11` | `mothership/go.mod`, marked `// indirect` | Transitive module requirement pulled in by a dependency. **No `.go` file in `mothership/`, `cmd/`, or `test/` imports any protobuf package** — it appears only in `go.mod`/`go.sum`. Nothing is generated from it and no message type in this inventory derives from it. |
| `protobuf-c` ESP-IDF component | `firmware/build/esp-idf/protobuf-c/`, `project_description.json` | ESP-IDF's `protocomm`, `wifi_provisioning` and `esp_local_ctrl` components `priv_require` it, so it is *compiled* into the build tree. Spaxel's own `firmware/main/` never references any of the three (zero grep hits in `CMakeLists.txt`, `*.c`, `*.h`). Definitive check: `grep -cE '\bpb_(decode|encode|read|write)' firmware/build/spaxel-firmware.map` → **0**. `libprotobuf-c.a` is LOADed in the map but the linker pulls no objects from it — the shipped image contains no protobuf messages. |

Consequence for any future task phrased as "update the protobuf definitions":
there is no generated layer to regenerate. The edit is a JSON struct tag
change in `mothership/internal/ingestion/message.go` plus the matching
`cJSON` sender in `firmware/main/websocket.c`, as laid out in step 1.

---

## 2. What stands in for `.proto` — the complete JSON inventory

With no protobuf layer, the protocol's schema registry is a set of Go structs
and C `cJSON` builders. This section is the complete inventory of those
definition sites. For each: the file, what plays the role of "package"
(the Go package or firmware module), the "syntax" (encoding rules), and every
message definition in it.

### 2.1 Node ↔ mothership WebSocket — the primary protocol

**File:** `mothership/internal/ingestion/message.go` (Go side, all 13 types)
**Package:** `ingestion` · **Syntax:** JSON objects with a `"type"` discriminator; snake_case field names; `omitempty` on optional fields; max frame 4 KB; unknown `type` values ignored by both sides.

`ParseJSONMessage` (`message.go:146`) is the authoritative upstream decoder:
its switch accepts exactly five types, which proves by construction that
those five are the only messages a node can send.

**Upstream (node → mothership):**

| Message | Go definition | Firmware sender (`firmware/main/websocket.c`) | Role |
|---|---|---|---|
| `HelloMessage` | `message.go:11` | `:484` (`"hello"`; `firmware_version` now from `SPAXEL_FIRMWARE_VERSION` at `:500`, no longer a hardcoded literal) | First message on connect; handshake answer to the connection itself |
| `HealthMessage` | `message.go:39` — **`free_heap_bytes` at `:43`** | `:563` (`"health"`; heap at `:571` via `esp_get_free_heap_size()`) | Periodic (~10 s); unsolicited, no reply |
| `BLEMessage` (+`BLEDevice`, `message.go:56`) | `message.go:66` | `:642` (`"ble"`), device array built in `firmware/main/ble.c:202-217` | Periodic (~5 s); unsolicited |
| `MotionHintMessage` | `message.go:74` | `:690` (`"motion_hint"`) | Event-driven (on-device variance threshold); unsolicited |
| `OTAStatusMessage` | `message.go:82` | `:725` (`"ota_status"`) | The only strictly-typed **response**: answers `OTAMessage` |

**Downstream (mothership → node):** all defined in `message.go`, all sent
from `mothership/internal/ingestion/server.go`.

| Message | Go definition | Mothership sender (`server.go`) | Answered by |
|---|---|---|---|
| `RoleMessage` | `message.go:93` | `sendRole` `:952`; public `SendRoleToMAC` `:429` | Next `HelloMessage`/`HealthMessage` |
| `ConfigMessage` | `message.go:100` | `sendConfig` `:968`; `SendConfigToMAC` `:406`, `SendNTPServerToMAC` `:417` | Next `HealthMessage` |
| `OTAMessage` | `message.go:109` | `SendOTAToMAC` `:441` | `OTAStatusMessage` stream |
| `RebootMessage` | `message.go:117` | `SendRebootToMAC` `:484` | Reconnect `HelloMessage` |
| `IdentifyMessage` | `message.go:123` | `SendIdentifyToMAC` `:458` | None (firmware-side LED blink) |
| `BaselineRequestMessage` | `message.go:129` | — | None — no distinct response type exists; data re-arrives via the normal CSI/BLE flow |
| `ShutdownMessage` | `message.go:134` | `Shutdown` `:1005` | Reconnect `HelloMessage` after `reconnect_in_ms` |
| `RejectMessage` | `message.go:140` | `sendReject` `:945` | Terminal — client closes |

Strictly request/response in shape: **`OTAMessage` → `OTAStatusMessage`**.
Everything else is a periodic report or a command acknowledged implicitly by
a behaviour change in the next `HealthMessage`.

**Second Go-side construction site:** `cmd/sim/main.go` (the `spaxel-sim`
fixture) builds its upstream messages as inline `map[string]interface{}`
literals — `hello` at `:474`, `ble` at `:900` — with **no named wire types**.
There is no struct to inventory; a protocol field addition must be mirrored
in those literals by hand.

**CSI binary frames** are the one non-JSON part of this transport; see §4.

### 2.2 REST response types — the RPC-shaped surface

These are the true analogues of "response messages used by RPCs": named Go
types whose only purpose is to be serialised as an HTTP response body.
Route registrations live on the shared chi router (`cmd/mothership/main.go:748`)
or in each package's `RegisterRoutes`.

#### `internal/fleet`

> Line numbers in this sub-table are **`git show HEAD`** numbers. The working
> tree carries an uncommitted third-party edit to `fleet/handler.go`
> (+37 lines, hunk `@@ -109,16 +109,47 @@`, adding a `nodeView` type for
> `/api/nodes` — see the caveat at the end). Every other file in this
> document is current working-tree numbers.

| Response type | Defined at | Built by | RPC(s) |
|---|---|---|---|
| `Handler.FleetNode` (`FreeHeapBytes` at `:146`) | `fleet/handler.go:125` | `listFleet` `:159` | `GET /api/fleet` — `handler.go:87` (element of `fleetListResponse`, `:151`) |
| `fleetListResponse` | `fleet/handler.go:151` | `listFleet` `:159` | `GET /api/fleet` — `handler.go:87` |
| `fleetHealthResponse` | `fleet/fleethandler.go:54` | `getFleetHealth` `:78` (builds it at `:155`) | `GET /api/fleet/health` — `fleethandler.go:47` (per-node entry `fleetNodeEntry`, `:61`, carries `FreeHeapBytes` at `:74`) |
| `optimiseResponse` | `fleet/fleethandler.go:294` | `triggerOptimise` `:301` | `POST /api/fleet/optimise` — `fleethandler.go:49` |
| `simulateResponse` | `fleet/fleethandler.go:315` | `simulateNodeRemoval` `:322` | `GET /api/fleet/simulate` — `fleethandler.go:50` |
| `systemModeResponse` | `fleet/handler.go:762` | `getSystemMode` `:774` / `setSystemMode` `:794` | `GET`/`POST /api/mode` — `handler.go:105-106` |
| `autoAwayConfigResponse` | `fleet/handler.go:768` | embedded in `systemModeResponse` | `GET`/`POST /api/mode` |
| *(in flight, not at HEAD)* `nodeView` | working-tree `fleet/handler.go` | `listNodes` | `GET /api/nodes` — `handler.go:88`. At HEAD the route marshals `[]NodeRecord` directly, so there is no named response type yet |

Related persisted record: `NodeRecord` — `fleet/registry.go:28`.

#### `internal/api`

| Response type | Defined at | Built by | RPC(s) |
|---|---|---|---|
| `ActiveAlertsResponse` | `api/alerts.go:39` | `handleGetActiveAlerts` | `GET /api/alerts/active` — `alerts.go:72` |
| `GDOPResponse` | `api/simulator.go:496` | `ComputeGDOP` | `POST /api/simulator/compute` — `simulator.go:97` |
| `SimulationResponse` | `api/simulator.go:709` | `RunSimulation` | `POST /api/simulator/simulate` — `simulator.go:106` |
| `SystemModeResponse` | `api/security.go:57` | mode handlers | `GET`/`POST /api/mode` — `security.go:43-44` |
| `captureResponse` | `api/baseline.go:95` | `captureBaseline` | `POST /api/baseline/capture` — `baseline.go:31` |
| `crossingResponse` | `api/zones.go:80` | `getPortalCrossings` | `GET /api/portals/{id}/crossings` — `zones.go:301` |
| `eventsResponse` | `api/events.go:211` | `listEvents` | `GET /api/events` — `events.go:205` |
| `notificationConfigResponse` | `api/notifications.go:321` | `handleGetConfig` | `GET /api/notifications/config` — `notifications.go:314` |
| `testNotificationResponse` | `api/notifications.go:381` | `handleSendTest` | `POST /api/notifications/test` — **registered twice**: `notifications.go:316` (conditional) and `notification_settings.go:83` |
| `occupancyResponse` | `api/status.go:135` | `getOccupancy` | `GET /api/occupancy` — `status.go:88` |
| `integrationSettingsResponse` | `api/integrations.go:121` | `handleGetSettings` | `GET /api/settings/integration` — `integrations.go:115` |
| `TriggerResponse` | `api/volume_triggers.go:66` | `toResponse` `:756` | `/api/triggers` CRUD — called from `listTriggers` `:307`, `getTrigger` `:327`, `createTrigger` `:372`, `updateTrigger` `:462` |
| `networkSettingsResponse` | `api/network_settings.go:37` | `response()` | `GET`/`PUT /api/settings/network` — `network_settings.go:87-88` |
| `networkSettingsWithPasswordResponse` | `api/network_settings.go:45` | `responseWithPassword()` | `GET /api/settings/network/recovery` — `network_settings.go:89` |
| `notificationSettingsResponse` | `api/notification_settings.go:87` | `getSettings` `:238` | `GET`/`PUT /api/settings/notifications` — `notification_settings.go:81-82` |

**Name collision.** `health.Response` (`internal/health/health.go:49`, served
at `GET /healthz` — `cmd/mothership/main.go:800`) and `doctor.Response`
(`internal/doctor/doctor.go:73`, served at `GET /api/doctor` — `main.go:5053`)
are unrelated types sharing the bare name `Response`. Always read them
package-qualified.

**Not covered by the tables.** The ~82 inline handlers registered directly in
`cmd/mothership/main.go` mostly write `map[string]interface{}` or ad-hoc
struct literals with no named response type, so there is nothing to inventory
beyond the route. Test-only helpers (`blobsResponse`, `StatusResponse`,
`HealthResponse`, `EventsResponse` — locations in step 2) never leave the
test process and are excluded.

### 2.3 Dashboard WebSocket — push, not request/response

**File:** `mothership/internal/dashboard/hub.go` · **Package:** `dashboard` ·
**Syntax:** JSON; the first message on every connection is a typed
`snapshot` envelope, later messages are typed broadcast events
(`node_connected`, `link_active`, `blob_explain`, …). The client never
replies with a typed message, so nothing here is a response type.

| Wire type | Defined at | Carries |
|---|---|---|
| `PortalSnapshot` | `hub.go:83` | Portal list for the snapshot |
| `ZoneSnapshot` | `hub.go:106` | Zone occupancy for the snapshot |
| `ZoneOccupancySnapshot` | `hub.go:121` | Zone occupancy change broadcasts |
| `nodeJSON` | `hub.go:499` | Per-node registry entry on the wire |
| `roomJSON` | `hub.go:511` | Room dimensions/origin |
| `blobJSON` | `hub.go:553` | Per-blob feed entry. Carries both snake_case identity fields and the camelCase canonical trio (`personName`, `assignedColor`, `identityResolved`) added by bf-5151; the snake_case pair is retained as a deprecated alias |

The full first-message envelope is assembled by `buildSnapshot`
(`hub.go:654`), not by a single named struct.

### 2.4 Provisioning JSON

**File:** `mothership/internal/provisioning/server.go` · **Package:** `provisioning`

| Type | Defined at | Direction |
|---|---|---|
| `Payload` | `server.go:24` | **Response body** of `POST /api/provision` — WiFi credentials, node ID, node token, mDNS name, port |
| `provisionRequest` | `server.go:38` | Request body (optional MAC hint) |

### 2.5 MQTT payloads — Home Assistant auto-discovery

**File:** `mothership/internal/mqtt/client.go` · **Package:** `mqtt` ·
**Syntax:** JSON published to `{prefix}/...` topics and
`homeassistant/{component}/spaxel_{entity}/config` discovery topics;
retained, QoS 1. Publish-only from the mothership — there is no typed
response, only the `command/security_mode` and `command/rebaseline`
subscription topics whose payloads are bare strings.

| Type | Defined at | Wire role |
|---|---|---|
| `HomeAssistantDevice` | `client.go:42` | `device` block of every discovery config |
| `HADiscoveryConfig` | `client.go:51` | Envelope of a discovery config message |
| `EntityConfig` | `client.go:68` | One entity's discovery payload |
| `ZoneConfig` | `client.go:581` | Zone discovery payload |
| `PersonConfig` | `client.go:587` | Per-person discovery payload |
| `Config` | `client.go:18` | Connection config (not a wire type; listed for completeness) |

### 2.6 Firmware serial provisioning protocol

**File:** `firmware/main/provision.c` · **Package:** none (C, no namespace) ·
**Syntax:** newline-delimited JSON over UART0 **and** USB-Serial/JTAG
concurrently (ADR-002), during the first 10 s after boot.

| Message | Direction | Location |
|---|---|---|
| `{"provision": {...}}` | host → device | parsed at `provision.c:152`/`:216`; accepted keys read at `:264-308`: `wifi_ssid`, `wifi_pass`, `node_id`, `node_token`, `ms_mdns`, `ms_ip`, `ms_port`, `debug`, `ntp_server` |
| `{"ok":true,"mac":"..."}` | device → host | `provision.c:166` |
| `{"ok":false,"error":"..."}` | device → host | `provision.c:138` (`already_provisioned`), `:147` (`invalid_json`), `:155` (`missing_provision_key`), `:173` (`nvs_write_failed`), and the USB-transport repeats from `:202` |

These are string literals, not structs — the parser is the bounded
recursive-descent decoder fuzz-tested by `firmware/test/test_serial_prov.c`.

---

## 3. Response messages, consolidated

Answering the chain's original question in one place. **No `.proto` file
defines a response message because no `.proto` file exists.** The complete
set of response-role definitions and their locations:

| Surface | Response type | Location |
|---|---|---|
| Node WS | `OTAStatusMessage` — the only typed response | `ingestion/message.go:82` |
| Node WS | implicit acks (next `HealthMessage`, reconnect `HelloMessage`) | `ingestion/message.go:39`, `:11` |
| REST `/api/fleet` | `fleetListResponse` + `FleetNode` | `fleet/handler.go:151`, `:125` |
| REST `/api/fleet/health` | `fleetHealthResponse` + `fleetNodeEntry` | `fleet/fleethandler.go:54`, `:61` |
| REST `/api/fleet/optimise` | `optimiseResponse` | `fleet/fleethandler.go:294` |
| REST `/api/fleet/simulate` | `simulateResponse` | `fleet/fleethandler.go:315` |
| REST `/api/mode` | `systemModeResponse` + `autoAwayConfigResponse` | `fleet/handler.go:762`, `:768` |
| REST `/api/nodes` | *(none at HEAD; `nodeView` in flight)* | `fleet/handler.go:88` |
| REST `internal/api` (15 types) | see §2.2 table | `internal/api/*.go` |
| REST `/healthz` | `health.Response` | `internal/health/health.go:49` |
| REST `/api/doctor` | `doctor.Response` | `internal/doctor/doctor.go:73` |
| Provisioning | `Payload` | `provisioning/server.go:24` |
| Serial provisioning | `{"ok":...}` literals | `firmware/main/provision.c:138-206` |

### `free_heap_bytes` across response-role messages (the chain's actionable output)

Carries it today:

| Type | Location |
|---|---|
| `HealthMessage` | `ingestion/message.go:43` (upstream WS) |
| `fleet.Handler.FleetNode` | `fleet/handler.go:146` (REST `GET /api/fleet`) |
| `fleetNodeEntry` | `fleet/fleethandler.go:74` (REST `GET /api/fleet/health`) |
| `NodeRecord` (persisted) | `fleet/registry.go:28`; column added by migration at `registry.go:159` |

Still missing it — the gap the `spaxel-3278a63f` / `spaxel-68359a6a` /
`spaxel-3b6699a4` / `spaxel-63ca0e88` chain targets, **unimplemented as of
this writing**:

| Type | Location | Consequence |
|---|---|---|
| `HelloMessage` | `ingestion/message.go:11` | A node's first report carries no heap figure |
| `OTAStatusMessage` | `ingestion/message.go:82` | OOM-during-OTA cannot be correlated with heap |

Neither field exists yet (verified: `FreeHeapBytes` appears exactly once in
`message.go`, at `:43`). Adding them is a JSON struct field plus the matching
`cJSON` sender in `firmware/main/websocket.c` — there is no generated layer
to regenerate.

---

## 4. Related wire formats that are not protobuf and not JSON

The CSI sample frame is the one remaining wire format in the system, recorded
here so a future "message inventory" is genuinely complete:

- **CSI binary frame** — 24-byte header + `n_sub`×2 bytes of int8 I/Q, sent
  as WebSocket binary frames (opcode `0x2`). Firmware encoder:
  `firmware/main/csi.c`. Mothership decoder: `ParseFrame`,
  `mothership/internal/ingestion/frame.go:47` (dispatched from
  `server.go:719`). Validated host-side by `firmware/test/test_csi.c`.
- **CSI replay store** (`csi_replay.bin`) — 64-byte header + framed records,
  written by `internal/replay`. Binary, append-only, versioned by magic.

---

## Caveats

1. **`internal/fleet/handler.go` numbers are `git show HEAD` numbers.** An
   uncommitted third-party edit (+37 lines, `@@ -109,16 +109,47 @@`, adding
   `nodeView` for `/api/nodes`) shifts every line after ~109 in the working
   tree, and adds one response type that is not at HEAD. If that edit lands,
   re-derive the four `fleet/handler.go` rows before citing them.
2. **`internal/ingestion/server.go` drifts often** — it moved ~7 lines between
   step 2 and this document. Re-derive sender line numbers rather than
   trusting any of the three survey docs.
3. **`health.Response` / `doctor.Response`** collide on the bare name;
   package-qualify.
4. **`POST /api/notifications/test` is registered twice**
   (`notifications.go:316` conditionally, `notification_settings.go:83`
   unconditionally), so `testNotificationResponse` has two possible builders
   depending on construction order.
5. Route/builder pairs were hand-verified by reading each `RegisterRoutes`
   and each builder, not taken from a mechanical scan — most routes live in
   per-package `RegisterRoutes` methods rather than on the root router, so a
   mechanical scan mis-attributes.
