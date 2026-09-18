# Spaxel Mothership — REST / WebSocket API Contract

Canonical description of the HTTP and WebSocket contracts the mothership
(`mothership/cmd/mothership`) actually serves today. Contract tests live in
`mothership/tests/contract/` and pin everything documented here; when this
document and the code disagree, the code wins and both should be fixed in the
same change.

Related: `docs/notes/api-implementation-status.md` (endpoint inventory),
`docs/plan/plan.md` §8 (aspirational spec — see §10 for divergences),
`docs/notes/adr/005-*` (fleet WiFi settings), `docs/notes/adr/006-*`
(firmware download auth).

## 1. Conventions

- **Transport:** plain HTTP/1.1 + WebSocket (gorilla/websocket). The container
  listens on `:8080` by default. There is no versioned path prefix.
- **JSON encoding:** `encoding/json`. Success bodies are `application/json`.
  Two error shapes exist in the wild (see below) — the per-endpoint tables
  name which one applies.
  - **JSON error shape A** (`api.writeJSONError`): `{"error": "<message>"}`
    with `Content-Type: application/json`.
  - **Error shape B** (`http.Error`): plain-text message with
    `Content-Type: text/plain; charset=utf-8`.
- **Method not matched** → chi's default `405 Method Not Allowed` (plain text).
- **Unknown path** → `404` (plain text).
- **Timestamps** are ISO-8601/RFC-3339 strings; numeric durations are seconds
  (`*_s`) or epoch milliseconds (`*_ms`).
- **Demo mode** (`SPAXEL_DEMO_MODE=1`): `auth.DemoModeMiddleware` rejects every
  `POST`/`PUT`/`PATCH`/`DELETE` **before routing** with
  `403` shape-A body `{"error":"demo mode active","message":"Spaxel is running
  in demo mode - mutating operations are disabled"}`. GET/HEAD/OPTIONS pass
  through.

### Authentication model

There is **no per-request auth on the API surface**. In production, cluster
authentication is enforced at the Traefik layer (the mothership is not
internet-facing). In-process:

- A dashboard PIN (bcrypt, cost 12) plus a `spaxel_session` cookie guard the
  PIN-management endpoints only: `POST /api/auth/change-pin` and
  `GET /api/doctor` are wrapped in `RequireAuth`; everything else is
  reachable without a session (deliberate — see bead spaxel-d223bfdd; the wide
  session middleware was uninstalled on purpose).
- Node-originated requests authenticate per-endpoint where it matters:
  firmware downloads (`X-Spaxel-*` headers, §7) and node WebSocket ingestion
  (`/ws/node`, HMAC token on hello). `GET /metrics` is deliberately
  unauthenticated for scraping.

## 2. Status & health

### `GET /healthz` → 200 | 503

Container liveness for orchestrators. Always `application/json`.

| Status | Body |
|---|---|
| 200 | `{"status":"ok","uptime_s":<int>,"version":"<ver>","nodes_online":<int>,"db":"ok","shedding_level":<0-3>}` |
| 503 | same shape, `"status":"degraded"`, plus `"reason":"..."` (omitted when ok), `db:"failing"` or shed level > 0 |

`db` is probed with `SELECT 1` (100 ms timeout). `shedding_level` is the
load-shedder level 0–3 (0 when no shedder is wired).

### `GET /api/status` → 200

Summary snapshot:

```json
{"version":"x","nodes":2,"blobs":1,"uptime_s":123,"detection_quality":42}
```

`nodes` = online node count, `blobs` = currently tracked blob count,
`detection_quality` = system health × 100 truncated to int (0–100).

### `GET /api/occupancy` → 200

Map of zone name → `{"count":<int>,"people":["<person label>",...]}`.
Empty object `{}` when no zones are defined (or no zones manager is wired).

## 3. Authentication endpoints (`/api/auth/*`)

All bodies are JSON; error responses here use **shape B** (plain text).

| Endpoint | Request | Success | Failures |
|---|---|---|---|
| `GET /api/auth/status` | — | 200 `{"pin_configured":<bool>,"demo_mode":<bool>}` | — |
| `GET /api/auth/install-secret` | — | 200 `{"install_secret":"<64 hex>"}` | 401 `install secret requires authentication` when a PIN is configured and no valid session |
| `POST /api/auth/setup` | `{"pin":"<4-8 digits>"}` | 200 `{"ok":"true"}` + `Set-Cookie: spaxel_session=...` | 409 `PIN already configured`; 400 `PIN must be 4-8 digits` / non-digit PIN |
| `POST /api/auth/login` | `{"pin":"..."}` | 200 `{"ok":"true"}` + session cookie | 404 `PIN not configured`; 401 `Invalid PIN` |
| `POST /api/auth/logout` | — | 200 `{"ok":"true"}` (clears cookie) | — |
| `POST /api/auth/change-pin` | `{"old_pin":"...","new_pin":"..."}` **+ session cookie** | 200 `{"ok":"true"}` (rotates the session) | 401 (missing/invalid session or wrong old PIN); 400 new-PIN validation errors |

Notes:
- The `ok` field is the string `"true"`, not a boolean.
- The session cookie is `spaxel_session`, `HttpOnly`, `Path=/`, `SameSite=Lax`
  (no `Secure` — the dashboard terminates on the tailnet).
- When no PIN is configured, `login` returns **404** (not 401) so a fresh
  install can distinguish "needs setup" from "wrong PIN".
- `GET /api/auth/install-secret` is the dashboard's escape hatch for
  Web-Serial provisioning of a node **before** any cluster trust exists; it is
  the only endpoint that ever returns the install secret.

## 4. Tracked blobs

### `GET /api/blobs` → 200

Raw fusion output for external integrations and tests. Registered as a closure
over the `ProcessorManager` in `main.go` — there is no dedicated handler type.

Response is a **bare JSON array** (no envelope), `Content-Type:
application/json`. Elements are `signal.TrackedBlob` values, whose unexported
JSON contract is **capitalized keys for the kinematic core** (the struct has no
tags on those fields) and snake_case/camelCase for identity enrichment:

```json
[{"ID":3,"X":1.25,"Y":0,"Z":-2.4,"VX":0.1,"VY":0,"VZ":0,"Weight":0.87,
  "person_id":"…","person_label":"…","person_color":"#aabbcc",
  "identity_confidence":0.9,"identity_source":"ble_triangulation",
  "posture":1,"personName":"…","assignedColor":"…","identityResolved":true}]
```

- Enrichment fields are `omitempty`: absent unless set.
- `identityResolved` is a tri-state `*bool`: key absent = never attempted.
- **Quirk:** when no blobs are tracked the handler encodes a nil slice, so the
  body is `null` — **not** `[]`. Consumers must null-check.
- This endpoint never returns an error status (always 200 once the process is
  up).

The dashboard does **not** consume this endpoint; it receives `loc_update`
frames over `/ws/dashboard` (§8) whose `blobJSON` uses lowercase keys.

## 5. Provisioning

### `POST /api/provision` → 200

Generates the NVS provisioning blob the Web-Serial onboarding wizard writes to
a node. Body is **entirely optional** (an empty POST provisions with defaults);
bad JSON → **400 shape B** (`invalid JSON body`); install secret not loadable →
**503 shape B** (`provisioning not ready (no install secret)`).

Request (all fields optional):

```json
{"wifi_ssid":"…","wifi_pass":"…","mac":"AA:BB:CC:DD:EE:FF","ms_ip":"10.0.0.5","debug":false}
```

Response 200 `application/json`:

```json
{"version":1,"wifi_ssid":"…","wifi_pass":"…","node_id":"<uuid4>",
 "node_token":"<64 hex>","ms_mdns":"spaxel","ms_ip":"10.0.0.5",
 "ms_port":8080,"ntp_server":"pool.ntp.org","debug":false}
```

- `ms_ip` is `omitempty` — absent unless the request overrode it.
- **ADR-005 defaults:** whitespace-only request WiFi fields are treated as
  empty and fall back to the fleet-wide `network_wifi_ssid` /
  `network_wifi_password` settings (§6). A request value wins over settings.
  Empty WiFi credentials are legal (captive-portal onboarding) and only log a
  warning.
- `node_id` is a fresh UUID v4 per call.
- `node_token`:
  - with `mac`: `hex(HMAC-SHA256(installSecret, upper(mac-without-colons)))`
    — deterministic; re-provisioning the same MAC yields the same token;
  - without `mac`: a random 64-hex placeholder. The ingestion server grants
    tokenless hellos a **120-second grace window** (node sends its MAC on
    hello; the real token is derived then).
- `mac` case/separator-insensitive on the request side (`aa:bb:…` works).

## 6. Network settings (fleet WiFi, ADR-005)

Backed by the shared settings table (`network_wifi_ssid`,
`network_wifi_password`) so the provisioning server sees writes immediately.

### `GET /api/settings/network` → 200

```json
{"wifi_ssid":"home-net","configured":true}
```

`configured` = SSID set **and** non-empty password present. The password is
**never** echoed here.

### `PUT /api/settings/network` → 200

Partial update — pointer fields distinguish "omitted" from "set to empty".
Body `{"wifi_ssid":"…","wifi_password":"…"}`; response is the same shape as
GET. Errors are **shape A** `{"error":"..."}`:

| Code | Condition / message |
|---|---|
| 400 | `invalid request body: …` (malformed JSON) |
| 400 | `wifi_ssid: must not be empty` |
| 400 | `wifi_ssid: must be 32 characters or fewer` |
| 400 | `wifi_password: must be at least 8 characters (WPA2 minimum) or empty for an open network` |
| 500 | `failed to save network settings` |

Validation applies to trimmed SSID; SSID ≤ 32 chars, password ≥ 8 chars or
empty (open network).

### `GET /api/settings/network/recovery` → 200

Same as GET but **includes `wifi_password`** — captive-portal recovery path
for an operator who locked themselves out. Reachable without a session
in-process; network-level protection is expected at Traefik.

## 7. Firmware

Firmware directory (default `/data/firmware`, `SPAXEL_FIRMWARE_DIR` /
`SPAXEL_SEED_FIRMWARE_DIR` env overrides) is re-scanned on demand.

### `GET /api/firmware` → 200

JSON array of scanned `.bin` files (newest semantic version first is the
caller's concern; the array is in scan order with `is_latest` marking the
single highest `major.minor.patch`):

```json
[{"filename":"spaxel-v1.2.4.bin","version":"1.2.4","sha256":"<64 hex>",
  "size_bytes":1234567,"is_latest":true,"uploaded_at":"2026-09-18T00:00:00Z"}]
```

`version` = first `\d+\.\d+\.\d+` match in the filename, else the filename
minus `.bin`. Files without a semver never get `is_latest:true`. Non-`.bin`
files are not listed.

### `GET /firmware/{filename}` — node OTA download

ADR-006 auth. Request headers:

| Header | Meaning |
|---|---|
| `X-Spaxel-MAC` | node MAC, e.g. `AA:BB:CC:DD:EE:FF` |
| `X-Spaxel-Token` | HMAC node token (as issued by /api/provision) |

| Status | Condition |
|---|---|
| 200 | valid token; body is the raw image, `Content-Type: application/octet-stream`, response headers `X-SHA256` and `X-Firmware-Version` |
| 404 | filename missing, has no `.bin` suffix, or **token invalid/mismatched** — 404 rather than 401 so probes can't enumerate firmware names |
| 404 | tokenless request outside the migration window (grace deadline passed; tokenless legacy nodes rejected) |

Path traversal is neutralized (`filepath.Base`); `filename` is matched against
the scanned set (a miss triggers a re-scan once, then 404).

### `POST /api/firmware/upload`

Multipart upload from the dashboard; the served contract above is the
load-bearing part, upload is out of scope for the contract tests.

## 8. WebSocket endpoints

### `/ws/node` — node ingestion (inbound CSI)

Upgrade: standard `websocket` handshake (origin check disabled). Node
protocol: binary length-prefixed frames of CSI samples; first message may be a
JSON `hello` carrying `mac` + `token`, validated with constant-time compare
against the HMAC install secret (120 s tokenless grace for freshly
provisioned nodes). Server→node messages are sparse (config pushes). This is
the **only** high-rate ingress; see `mothership/internal/ingest/`.

### `/ws/dashboard` — dashboard event stream

Upgrade: standard handshake; **origin check disabled** (`CheckOrigin` returns
true — tailnet-internal). Message frames are JSON text.

**Lifecycle (order is contractual):**

1. Server → client: **`snapshot`** as the *first* frame, built and sent
   *before* the client is added to the broadcast set (so no delta can precede
   the initial state). Missing/empty subsystems produce absent keys, not
   `null`/`[]`. The hub caches the latest `loc_update` blobs and folds them
   into new clients' snapshots, so a blob broadcast before a client connects
   shows up in its snapshot instead of being replayed as a delta:

   ```json
   {"type":"snapshot","timestamp_ms":1726000000000,
    "nodes":[…],"links":[…],"motion_states":[…],"ble_devices":[…],
    "triggers":[…],"zones":[…],"portals":[…],"blobs":[…]}
   ```

2. Server → client: broadcast events (below), fanned out from a 256-message
   per-client queue; **a slow client's queue overflow drops messages** (no
   back-pressure, no disconnect).
3. Server pings every 30 s; client must pong within the 60 s read deadline or
   is dropped.
4. Client → server commands are JSON `{"type":...}` frames (below); unknown
   types are ignored silently.

**Server → client event catalog** (all `{"type":"<name>",...}`):

| type | payload notes |
|---|---|
| `snapshot` | initial state, first frame only |
| `loc_update` | `{"type","timestamp_ms","blobs":[{id,x,z,vx,vz,weight,trail,posture?,person_id?,person_label?,person_color?,identity_confidence?,identity_source?,personName?,assignedColor?,identityResolved?}]}` — lowercase keys; emitted per fusion tick |
| `coverage_map` | `{cols,rows,cell_size,origin_x,origin_z,data}` grid |
| `node_connected` | `{mac,firmware_version,chip,unpaired}` |
| `node_disconnected` | `{mac}` |
| `link_active` / `link_inactive` | link-id payload |
| `motion_state` | motion detector transitions |
| `registry_state`, `fleet_change`, `fleet_health`, `system_health` | periodic/edge status |
| `blob_explain` | `{blob_id,snapshot}` reply to `request_explain` |
| `event`, `alert`, `quality_drop` | notification stream |
| `ble_scan` | BLE device sightings |
| `trigger_state` | automation trigger state |
| `zone_change`, `portal_change` | geometry edits |
| `morning_summary`, `morning_briefing` | daily digest |
| `replay_update` | replay-mode progress |

**Client → server commands:**

| type | fields | effect |
|---|---|---|
| `replay_seek` | `timestamp_iso8601` | seek replay session |
| `replay_play` | `speed` | start playback |
| `replay_pause` | — | pause playback |
| `replay_set_speed` | `speed` | change playback rate |
| `replay_set_params` | any of `delta_rms_threshold`,`tau_s`,`fresnel_decay`,`n_subcarriers`,`breathing_sensitivity` | tune pipeline mid-replay |
| `replay_apply_to_live` | — | promote replay params to live |
| `request_explain` | `blob_id` | asks for a `blob_explain` frame (consumable via the hub's explain-request queue) |

Frames whose first byte is not `{` are treated as binary and ignored by the
dashboard path (the node protocol reuses the same handler shape).

## 9. Contract tests

`mothership/tests/contract/` wires the **real handlers** (chi router, sqlite
settings store, real `ProcessorManager`, real provisioning/OTA/dashboard
servers — the same wiring as `main.go`) and pins this document:

- `contract_test.go` — status/health, auth, blobs, provisioning, network
  settings, firmware download/download-auth (table-driven).
- `dashboard_ws_test.go` — snapshot-first frame ordering, `loc_update`
  broadcast shape, `request_explain` command round-trip, unknown-command
  tolerance.

Run: `cd mothership && go test ./tests/contract/ ./internal/...`.

## 10. Divergences from `docs/plan/plan.md` §8

| plan.md says | actual contract |
|---|---|
| `GET /api/blobs` → `{"blobs":[...]}` envelope, camelCase | bare array; **capitalized** kinematic keys; `null` when empty |
| Session auth on `/api/*` | only `/api/auth/change-pin` and `/api/doctor` enforce it in-process; cluster auth is at Traefik |
| Uniform JSON error envelope | two shapes coexist: `{"error"}` (shape A) and plain text (shape B) — per-endpoint tables name them |
| Firmware download 401 on bad auth | **404** (ADR-006: no filename enumeration) |
| Provisioning requires WiFi credentials | optional (ADR-005 fallback to fleet settings; captive-portal onboarding legal) |
