# Response message definitions — protobuf survey, step 2

Bead: spaxel-c95b4ab0. Continues [protobuf-survey-free-heap.md](protobuf-survey-free-heap.md)
(the "previous step" catalog).

Task as dispatched: *identify which `.proto` files define response-type messages*,
mapping each response message to its defining file, its message name, and the
RPC(s) that use it.

## Answer to the question as asked

**Zero `.proto` files exist in spaxel, so no `.proto` file defines a response
message.** The protocol has no protobuf layer at all — every exchange is JSON,
either over WebSocket (node↔mothership, dashboard↔mothership) or as REST bodies
served by the chi router.

Evidence, all reproducible from the repo root:

```bash
find . -name '*.proto' -not -path './.git/*'      # no output
grep -rn 'syntax = "proto"' --include='*' .       # no output
git log --all --diff-filter=A --name-only -- '*.proto' | grep -c proto   # 0
grep -ri 'protobuf\|google.golang.org/protobuf' mothership/go.mod firmware/  # no hits
```

The nearest thing to a schema registry is the Go struct set in
`mothership/internal/ingestion/message.go`, which is what the previous step's
catalog documented. This step therefore maps the **response-role** definitions
of that same JSON protocol: for each one, the file, the type name, and the
exchange(s) that serve it.

---

## A. WebSocket messages (node ↔ mothership)

`ParseJSONMessage` (`mothership/internal/ingestion/message.go:145`) is the
authoritative decoder for the upstream direction: its switch accepts exactly
five types, which is proof by construction that these five are the only
message types a node can send. Firmware senders live in
`firmware/main/websocket.c`; mothership senders in
`mothership/internal/ingestion/server.go`.

### Upstream (node → mothership) — the responses/notifications side

| Message | Go definition | Firmware sender | What it responds to |
|---|---|---|---|
| `HelloMessage` | `ingestion/message.go:11` | `websocket.c:484` (`"hello"`) | The WS connection itself — handshake answer to connect. The mothership replies downstream with `RoleMessage` + `ConfigMessage`, or `RejectMessage` on auth failure |
| `HealthMessage` | `ingestion/message.go:39` — **carries `free_heap_bytes` at :43** | `websocket.c:563` (`"health"`) | Periodic (~10 s) health report; unsolicited, no reply |
| `BLEMessage` | `ingestion/message.go:66` | `websocket.c:642` (`"ble"`) | Periodic (~5 s) scan report; unsolicited |
| `MotionHintMessage` | `ingestion/message.go:74` | `websocket.c:690` (`"motion_hint"`) | Event-driven (variance threshold exceeded); unsolicited |
| `OTAStatusMessage` | `ingestion/message.go:82` | `websocket.c:725` (`"ota_status"`) | Progress/error report **answering `OTAMessage`** (`:109`) or `RebootMessage` (`:117`) |

### Downstream (mothership → node) — the request side, for pairing completeness

These are requests/commands, not responses; listed because each upstream
response above is only meaningful against them. All defined in
`ingestion/message.go`, all sent from `ingestion/server.go`:

| Message | Go definition | Mothership sender | Answered by |
|---|---|---|---|
| `RoleMessage` | `message.go:93` | `sendRole`, `server.go:953` | next `HelloMessage`/`HealthMessage` |
| `ConfigMessage` | `message.go:100` | `sendConfig`, `server.go:969` | next `HealthMessage` (reflects new rate) |
| `OTAMessage` | `message.go:109` | `SendOTAToMAC`, `server.go:448` | `OTAStatusMessage` stream |
| `RebootMessage` | `message.go:117` | `SendRebootToMAC`, `server.go:491` | reconnect `HelloMessage` |
| `IdentifyMessage` | `message.go:123` | `SendIdentifyToMAC`, `server.go:465` | none (firmware-side LED blink) |
| `BaselineRequestMessage` | `message.go:129` | — | none — **no distinct response type exists**; baseline data re-arrives through the normal CSI/BLE flow |
| `ShutdownMessage` | `message.go:134` | `Shutdown`, `server.go:1009` | reconnect `HelloMessage` after `reconnect_in_ms` |
| `RejectMessage` | `message.go:140` | `sendReject`, `server.go:946` | terminal — client closes |

**Strictly request/response in shape: `OTAMessage` → `OTAStatusMessage`.**
Everything else is either a periodic report or a command with an implicit
acknowledgement (behaviour change visible in the next `HealthMessage`), not a
typed response message.

---

## B. REST response types (the RPC-shaped surface)

These are the true analogues of "response messages used by RPCs": named Go
types whose only purpose is to be serialised as an HTTP response body. Route
registrations are on the shared chi router (`cmd/mothership/main.go:748`)
or in each package's `RegisterRoutes`.

### Fleet (`internal/fleet`)

> Line numbers for `internal/fleet/handler.go` are cited **at git HEAD**.
> The working tree carries an in-flight +31-line third-party edit in that
> file (hunk at `@@ -109,16 +109,47 @@`); everything at or below ~109 shifts
> by +31 until it lands or is reverted. The other files are unaffected.

| Response type | Defined at | Built by | RPC(s) that use it |
|---|---|---|---|
| `fleet.Handler.FleetNode` | `fleet/handler.go:125` (`FreeHeapBytes` at :146) | `listFleet`, `handler.go:159` | `GET /api/fleet` — `handler.go:87` (element type of `fleetListResponse`, `:151`) |
| `fleetListResponse` | `fleet/handler.go:151` | `listFleet`, `handler.go:159` | `GET /api/fleet` — `handler.go:87` |
| `fleetHealthResponse` | `fleet/fleethandler.go:54` | `getFleetHealth`, `fleethandler.go:155` | `GET /api/fleet/health` — `fleethandler.go:47` (per-node entry `fleetNodeEntry`, `:61`, carries `FreeHeapBytes` at `:74`) |
| `optimiseResponse` | `fleet/fleethandler.go:294` | `triggerOptimise`, `fleethandler.go:301` | `POST /api/fleet/optimise` — `fleethandler.go:49` |
| `simulateResponse` | `fleet/fleethandler.go:315` | `simulateNodeRemoval`, `fleethandler.go:322` | `GET /api/fleet/simulate` — `fleethandler.go:50` |
| `systemModeResponse` | `fleet/handler.go:762` | `getSystemMode` `:774` / `setSystemMode` `:794` | `GET /api/mode` `:105`, `POST /api/mode` `:106` |
| `autoAwayConfigResponse` | `fleet/handler.go:768` | `getSystemMode`/`setSystemMode` (embedded) | `GET`/`POST /api/mode` |

### API package (`internal/api`)

| Response type | Defined at | Built by | RPC(s) that use it |
|---|---|---|---|
| `ActiveAlertsResponse` | `api/alerts.go:39` | `handleGetActiveAlerts` | `GET /api/alerts/active` — `alerts.go:72` |
| `GDOPResponse` | `api/simulator.go:496` | `ComputeGDOP` | `POST /api/simulator/compute` — `simulator.go:97` |
| `SimulationResponse` | `api/simulator.go:709` | `RunSimulation` | `POST /api/simulator/simulate` — `simulator.go:106` |
| `api.SystemModeResponse` | `api/security.go:57` | mode handlers | `GET`/`POST /api/mode` — `security.go:43-44` |
| `captureResponse` | `api/baseline.go:95` | `captureBaseline` | `POST /api/baseline/capture` — `baseline.go:31` |
| `crossingResponse` | `api/zones.go:80` | `getPortalCrossings` | `GET /api/portals/{id}/crossings` — `zones.go:301` |
| `eventsResponse` | `api/events.go:211` | `listEvents` | `GET /api/events` — `events.go:205` |
| `notificationConfigResponse` | `api/notifications.go:321` | `handleGetConfig` | `GET /api/notifications/config` — `notifications.go:314` |
| `testNotificationResponse` | `api/notifications.go:381` | `handleSendTest` | `POST /api/notifications/test` — **registered twice**: `notifications.go:316` (conditional) and `api/notification_settings.go:83` |
| `occupancyResponse` | `api/status.go:135` | `getOccupancy` | `GET /api/occupancy` — `status.go:88` |
| `integrationSettingsResponse` | `api/integrations.go:121` | `handleGetSettings` | `GET /api/settings/integration` — `integrations.go:115` |
| `TriggerResponse` | `api/volume_triggers.go:66` | `toResponse`, `volume_triggers.go:756` | `/api/triggers` CRUD + test routes — `volume_triggers.go:272+` (called from `listTriggers` `:307`, `getTrigger` `:327`, `createTrigger` `:372`, `updateTrigger` `:462`) |
| `networkSettingsResponse` | `api/network_settings.go:37` | `response()` | `GET`/`PUT /api/settings/network` — `network_settings.go:87-88` |
| `networkSettingsWithPasswordResponse` | `api/network_settings.go:45` | `responseWithPassword()` | `GET /api/settings/network/recovery` — `network_settings.go:89` |
| `notificationSettingsResponse` | `api/notification_settings.go:87` | `getSettings`, `:238` | `GET`/`PUT /api/settings/notifications` — `notification_settings.go:81-82` |

### Health / diagnostics (two distinct types that share the bare name `Response`)

These two are unrelated and **must be package-qualified** — a name-only
collision collapses them into one.

| Response type | Defined at | Built by | RPC(s) that use it |
|---|---|---|---|
| `health.Response` | `internal/health/health.go:49` | `Checker.check`, `health.go:77` | `GET /healthz` — `cmd/mothership/main.go:800` (via `Checker.Handler`, `health.go:60`) |
| `doctor.Response` | `internal/doctor/doctor.go:73` | `Check` | `GET /api/doctor` — `cmd/mothership/main.go:5053` |

### Not covered by the table

- **Anonymous responses.** The ~82 inline handlers registered directly in
  `cmd/mothership/main.go` mostly write `map[string]interface{}` or ad-hoc
  struct literals with no named response type, so there is nothing to
  inventory beyond the route itself. Named types above are the complete set.
- **Test-only response helpers**, excluded from the inventory as they never
  leave the test process: `blobsResponse`
  (`test/acceptance/test_helpers.go:82`), `StatusResponse`
  (`tests/e2e/io6_gate_test.go:53`), `HealthResponse` / `EventsResponse`
  (`tests/e2e/e2e_test.go:269` / `:421`).

---

## C. Dashboard WebSocket feed — push, not response

`internal/dashboard/hub.go` defines `PortalSnapshot` (`:83`),
`ZoneSnapshot` (`:106`) and `ZoneOccupancySnapshot` (`:121`). These are
broadcast snapshots: the dashboard client never replies with a typed
response message, so they are excluded from the response inventory but
recorded here to keep the two WS transports distinct.

---

## `free_heap_bytes` status across response-role messages

This is the actionable output for the open free-heap chain in the bead store:

| Bead | Title |
|---|---|
| `spaxel-3278a63f` | Add free_heap_bytes to protobuf response definitions |
| `spaxel-68359a6a` | Surface free heap in mothership API |
| `spaxel-3b6699a4` | Update API handlers to populate free_heap_bytes |
| `spaxel-63ca0e88` | Verify free_heap_bytes in API responses |

Their titles say "protobuf response definitions"; given the finding above,
the field addition they actually describe is a plain JSON struct field in
`ingestion/message.go` plus the matching firmware sender in
`firmware/main/websocket.c` — there is no generated protobuf layer to
regenerate:

| Carries it today | Where |
|---|---|
| `HealthMessage` | `ingestion/message.go:43` (upstream WS) |
| `fleet.Handler.FleetNode` | `fleet/handler.go:146` (REST `GET /api/fleet`) |
| `fleetNodeEntry` | `fleet/fleethandler.go:74` (REST `GET /api/fleet/health`) |
| `NodeRecord` (persisted) | `fleet/registry.go:44`, column added by migration at `registry.go:159` |

| Does **not** carry it (the gap that chain targets) | Where |
|---|---|
| `HelloMessage` | `ingestion/message.go:11` — so a node's first report has no heap figure |
| `OTAStatusMessage` | `ingestion/message.go:82` — so OOM-during-OTA cannot be correlated with heap |

---

## Caveats

1. **`fleet/handler.go` line numbers are at git HEAD** (`9e45a111`). An
   uncommitted third-party edit (+31 lines, hunk `@@ -109,16 +109,47 @@`)
   shifts all later lines in the working tree. Numbers for every other file
   are current working-tree numbers.
2. **`health.Response` and `doctor.Response` collide on the bare name** —
   always read them package-qualified.
3. **`POST /api/notifications/test` is registered twice**
   (`notifications.go:316` and `notification_settings.go:83`); the first is
   conditionally mounted, so which handler answers depends on construction
   order. `testNotificationResponse` therefore has two possible builders.
4. Route/builder pairs were hand-verified by reading each `RegisterRoutes`
   and the builder function, not taken from a mechanical scan; a mechanical
   route scan alone mis-attributes because most routes live in per-package
   `RegisterRoutes` methods rather than on the single root router.
