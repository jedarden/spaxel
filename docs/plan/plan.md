# Spaxel — Implementation Plan

WiFi CSI-based indoor positioning for self-hosted home environments.

## System Overview

A single Docker container ("the mothership") runs on a home server. It manages a fleet of ESP32-S3 devices that transmit and receive WiFi packets, extract Channel State Information, and stream it back. The mothership fuses CSI from all links to detect and localize people as spatial blobs, rendered on a floor-plan dashboard.

### What Spaxel Can Realistically Achieve

Based on physics and literature (see `docs/research/06-accuracy-and-limits.md`):

- **Presence detection** — reliably, with 2+ nodes on opposite sides of a space
- **Approximate 2D position** — ±0.5–1.0 m with 4+ nodes
- **Motion tracking** — trajectory following of moving people
- **Rough person count** — distinguish 1 vs 2+ people (degrades at 3+)
- **Rough Z-axis** — ±1–2 m with mixed-height node placement
- **Stationary person detection** — via breathing micro-motion (0.1–0.5 Hz), requires stable setup

**Not achievable:** sub-10 cm accuracy, skeletal pose, reliable 5+ person tracking.

---

## Architecture

```
┌───────────────────────────────────────────────────────────────────────────┐
│  Docker Container (Mothership)                                            │
│                                                                           │
│  ┌──────────┐  ┌──────────┐  ┌────────────┐  ┌──────────┐  ┌──────────┐  │
│  │ Ingestion│  │ Signal   │  │ Fusion,    │  │ BLE      │  │Dashboard │  │
│  │ Server   │──│Processing│──│ Localizer &│──│ Identity │──│ (Web UI) │  │
│  │ (WS)     │  │ Pipeline │  │ Biomech UKF│  │ Matcher  │  │          │  │
│  └──────────┘  └──────────┘  └────────────┘  └──────────┘  └──────────┘  │
│  ┌──────────┐  ┌──────────┐  ┌────────────┐  ┌───────────────────────┐   │
│  │ Fleet    │  │ OTA      │  │ Onboarding │  │ Automation Engine     │   │
│  │ Manager  │  │ Server   │  │ (Web Serial│  │ (triggers, fall,      │   │
│  └──────────┘  └──────────┘  │ + Captive) │  │  anomaly, prediction, │   │
│                              └────────────┘  │  sleep, crowd flow)   │   │
│                                              └───────────────────────┘   │
└───────────────────────────────────────────────────────────────────────────┘
         ▲ WebSocket /ws/node (binary CSI + JSON config/BLE, bidirectional)
         │
    ┌─────────┐     ┌─────────┐        ┌─────────┐     ┌─────────┐
    │ ESP32-S3│     │ ESP32-S3│        │ ESP32-S3│     │ WiFi AP │
    │ (RX)    │     │ (TX)    │        │ (TX/RX) │     │ (passive│
    │ WiFi+BLE│     │ WiFi+BLE│        │ WiFi+BLE│     │  radar) │
    └─────────┘     └─────────┘        └─────────┘     └─────────┘
```

### Technology Choices

| Component | Technology | Rationale |
|-----------|-----------|-----------|
| Mothership backend | Go | Low-latency ingestion, single binary, easy Docker packaging |
| Dashboard frontend | Vanilla JS + Three.js | No build toolchain; Three.js provides hardware-accelerated 3D scene with orbit controls, raycasting, and transparent rendering — all needed for spatial visualization |
| ESP32 firmware | ESP-IDF (C) | Full CSI API access, OTA support, NVS for config persistence |
| Node ↔ Mothership transport | WebSocket (single bidirectional connection per node) | Binary frames upstream (CSI data), JSON frames downstream (config, role, OTA triggers). Single HTTP port via Traefik. Connection state = node liveness — no separate heartbeat protocol needed |
| OTA delivery | HTTP (served by mothership) | Standard ESP-IDF OTA mechanism, firmware binaries served from container |
| Onboarding | Web Serial API (browser → USB) | Zero-install provisioning from the dashboard |
| BLE identity | ESP32-S3 BLE (passive scan) | Concurrent with WiFi on second core. Scans for phone/watch/tag advertisements. Enables person identification of anonymous CSI blobs |
| Persistence | SQLite (`modernc.org/sqlite`, pure Go) | Node registry, BLE device registry, floor plans, calibration data, baseline snapshots, CSI recording buffer, Fresnel weights, prediction models, sleep data |
| HA integration | MQTT client (optional) — `github.com/eclipse/paho.mqtt.golang` | Mothership connects as client to user's existing MQTT broker for Home Assistant auto-discovery. No broker runs inside the container |
| Go WebSocket (server) | `github.com/gorilla/websocket` | Mature, widely used. Supports `SetReadDeadline`, binary frame send, and ping/pong handler registration. `conn.SetPingHandler()` used for pong tracking |
| HTTP routing | `net/http` stdlib + `github.com/go-chi/chi` | Chi provides URL parameters (`:mac`, `:id`) without adding a framework. Lightweight, compatible with stdlib handlers |
| Matrix ops (UKF) | `gonum.org/v1/gonum/mat` | Standard Go scientific computing library for UKF sigma point matrix operations |
| Notification image render | `github.com/fogleman/gg` | Pure Go 2D drawing — no cgo required |
| mDNS | `github.com/hashicorp/mdns` | Pure Go, no OS daemon dependency |
| HMAC/auth | `crypto/hmac`, `golang.org/x/crypto/bcrypt` | Standard library HMAC; bcrypt for PIN hashing |

---

## Component Design

### 1. ESP32-S3 Firmware

Single firmware binary that runs on every node. Behavior (TX, RX, or both) is determined by config from the mothership.

**Core responsibilities:**
- Connect to configured WiFi network
- Discover mothership via mDNS (`_spaxel._tcp.local`) — no manual IP configuration required. Falls back to NVS-stored IP if mDNS fails
- Open a single WebSocket connection to mothership (`ws://mothership:8080/ws/node`). This connection carries all communication in both directions — CSI data upstream, config/commands downstream

**Node connection lifecycle (state machine):**

```
BOOT
  └─▶ WiFi connect (exponential backoff: 1s, 2s, 4s, 8s, 16s, 30s, 30s steady)
        │  WiFi connected
        ▼
  MOTHERSHIP_DISCOVERY
     1. Query mdns_query_srv("_spaxel", "_tcp", 5000ms timeout) → get host + port
     2. If mDNS fails: use cached ms_ip NVS key if set
     3. Attempt WebSocket connect to resolved address (5 s timeout)
     4. On success: update ms_ip NVS key with current IP, go to CONNECTED
     5. On fail: retry discovery (retry 1→2→1→2... cycling, 5 s between attempts)
     6. After 10 consecutive discovery failures: go to MOTHERSHIP_UNAVAILABLE
        │  WebSocket connected
        ▼
  CONNECTED — normal operation (CSI streaming, BLE scanning, health reporting)
     - On WebSocket disconnect: go to MOTHERSHIP_DISCOVERY (WiFi may still be fine)
     - On WiFi disconnect: go to WIFI_LOST
        │
  WIFI_LOST — WiFi reconnect loop (exponential backoff, same as above)
     - If WiFi reconnects: go to MOTHERSHIP_DISCOVERY
     - After 10 consecutive WiFi failures: go to CAPTIVE_PORTAL
        │
  MOTHERSHIP_UNAVAILABLE — mothership is unreachable but WiFi is fine
     - Continue operating at last known role (TX/RX/passive) — CSI is not streamed, just discarded
     - BLE scanning continues (results queued locally, max 60 entries)
     - Retry mothership discovery every 30 s indefinitely
     - Dashboard shows node as STALE (not OFFLINE — WiFi is still up)
     - NEVER trigger captive portal — the mothership may simply be rebooting
     - On mothership reconnect: deliver queued BLE results, resume normal operation
        │
  CAPTIVE_PORTAL — WiFi credentials invalid or network gone
     - Start AP: "spaxel-XXXX" (last 4 of MAC)
     - Serve config page at 192.168.4.1 for new WiFi SSID/password + optional mothership IP
     - On credentials saved: write to NVS, reboot → BOOT
```

**Key invariants:**
- Captive portal ONLY triggers on WiFi failure, never on mothership unreachability
- The node is always operational (at last known role) even when disconnected from the mothership
- ms_ip NVS key is auto-updated on every successful connection, making the fallback self-healing
- mDNS and direct IP are both tried on every reconnect (mDNS first, cached IP second)
- Send a registration JSON message on connect: `{type: "hello", mac, firmware_version, capabilities}`
- Listen for role assignment (TX, RX, TX/RX, PASSIVE) and packet rate config as JSON messages from mothership on the same WebSocket
- **TX mode:** Send probe packets at configured rate (default 20 Hz)
- **RX mode:** Enable promiscuous mode, capture CSI from all TX nodes, stream raw I/Q pairs as WebSocket binary frames
- **Passive mode:** RX-only, filtering for the home WiFi AP's BSSID — uses existing router beacon/data frames as the TX source (see Passive Radar Mode)
- **TX/RX mode:** Alternate between transmitting and receiving on a time-division schedule
- **BLE scanning (second core):** Continuously scan for BLE advertisements (iBeacon, Eddystone, generic GAP) on the ESP32-S3's second core. Report per-device RSSI as periodic JSON: `{type: "ble", devices: [{addr: "AA:BB:...", rssi: -62, name: "iPhone"}, ...]}`. Scanning runs concurrently with WiFi CSI — the ESP32-S3's dual-core architecture handles both without contention
- **Adaptive sensing rate:** Support mothership-controlled packet rate changes. Also perform on-device amplitude variance check at low rate (2 Hz) — if local variance exceeds threshold, burst to full rate and notify mothership
- Report health metrics (free heap, WiFi RSSI, uptime, temperature) as periodic JSON messages on the WebSocket (every 10 s)
- Support OTA firmware updates triggered by mothership command on the WebSocket, pulled via HTTP
- Store config in NVS: WiFi credentials, mothership IP (fallback), node ID, last known role

**NVS Layout:**

All keys live in NVS namespace `"spaxel"` (key names max 15 characters per ESP-IDF limit). All multi-key updates call `nvs_commit()` after every individual write AND once after the full batch — ensuring each key is durable even if power is lost mid-write. A `"provisioned"` flag is written last; firmware checks it on boot to determine state.

| NVS Key | Type | Max | Default | Written By | Description |
|---------|------|-----|---------|------------|-------------|
| `schema_ver` | `uint8` | 1 B | 1 | firmware | NVS schema version; firmware migrates if found version < current |
| `provisioned` | `uint8` | 1 B | 0 | provisioning | 0 = not provisioned (captive portal); 1 = provisioned |
| `wifi_ssid` | `str` | 32 B | — | provisioning/captive | WiFi network SSID |
| `wifi_pass` | `str` | 64 B | — | provisioning/captive | WiFi passphrase |
| `node_id` | `str` | 37 B | — | provisioning | UUID4 string assigned by mothership |
| `node_token` | `str` | 65 B | — | provisioning | 64-char hex HMAC-SHA256 token for WebSocket auth |
| `ms_mdns` | `str` | 64 B | `"spaxel"` | provisioning | mDNS service name; full: `<ms_mdns>._spaxel._tcp.local` |
| `ms_ip` | `str` | 46 B | — | runtime/captive | Fallback mothership IP (set by captive portal or runtime push); used if mDNS fails |
| `ms_port` | `uint16` | 2 B | 8080 | provisioning | Mothership HTTP port |
| `passive_bss` | `blob` | 6 B | — | runtime | AP BSSID bytes for passive radar mode; all-zeros = disabled |
| `role` | `uint8` | 1 B | 2 (TX_RX) | runtime | Last assigned role: 0=TX, 1=RX, 2=TX_RX, 3=PASSIVE, 4=IDLE |
| `pkt_rate` | `uint8` | 1 B | 20 | runtime | Current packet rate Hz |
| `ap_mode` | `uint8` | 1 B | 0 | firmware | 1 = force captive portal on next boot |
| `debug` | `uint8` | 1 B | 0 | provisioning | 1 = verbose USB serial logging |

**NVS write sequence during provisioning** (Web Serial → esptool-js → NVS):
1. Erase namespace `"spaxel"` (clean slate)
2. Write `schema_ver`, `wifi_ssid`, `wifi_pass`, `node_id`, `node_token`, `ms_mdns`, `ms_port`, `debug` — each followed by `nvs_commit()`
3. Write `provisioned` = 1 last — only set once all other keys are durable
4. Final `nvs_commit()`

**Schema migration:** On boot, firmware reads `schema_ver`. If less than the compiled-in version, firmware runs migration code (add/rename/remove keys), then updates `schema_ver`. Ensures OTA-updated firmware handles NVS written by older versions.

**CSI packet format (WebSocket binary frame, upstream):**

Each CSI sample is sent as a single WebSocket binary frame on the node's persistent connection. All multi-byte integers are little-endian.

```
Header (fixed 24 bytes):
  node_mac:     6 bytes    — source node MAC (6 uint8)
  peer_mac:     6 bytes    — transmitting node MAC in RX mode; own MAC in TX mode (6 uint8)
  timestamp_us: 8 bytes    — microseconds since node boot, from esp_timer_get_time() (uint64, little-endian)
                            Never wraps in practice (~580,000 year overflow). Monotonic since last boot.
                            Resets to near-zero on reboot — mothership detects reboot by checking if the
                            new timestamp is significantly less than the previous one on the same connection.
  rssi:         1 byte     — signed RSSI in dBm (int8)
  noise_floor:  1 byte     — signed noise floor in dBm (int8)
  channel:      1 byte     — WiFi channel number (uint8)
  n_sub:        1 byte     — number of subcarriers in this frame (uint8, typically 64)

Payload (n_sub * 2 bytes):
  Per subcarrier: int8 I, int8 Q  (in-phase and quadrature, subcarrier index 0..n_sub-1)
```

**Timestamp semantics:**
- Node timestamps are used for: (a) inter-packet interval computation within a localization window, (b) phase synchronization across links from the same TX burst.
- Mothership receive time (`time.Now().UnixNano()`) is stored alongside each CSI frame in the ring buffer and replay store. This is the authoritative time for replay, timeline events, and baseline timestamps. Node timestamp is stored as a secondary field for phase-synchronization use only.
- Inter-link synchronization tolerance: up to 50 ms clock skew between nodes is acceptable (the localization algorithm uses the Fresnel zone geometry, not precise time differences between nodes).

The binary format uses WebSocket binary frames (opcode 0x2) to avoid base64/JSON encoding overhead. Frame size: 24 + n_sub×2 bytes = 152 bytes for 64 subcarriers.

**Recovery layers** (per `docs/notes/recovery-mechanisms.md`):
1. **Automatic:** WiFi reconnect loop with exponential backoff; OTA rollback on boot failure (two-partition scheme)
2. **Captive portal:** After 10 failed WiFi attempts, start AP mode as `spaxel-XXXX`, serve config page for new WiFi credentials / mothership IP
3. **Web Serial:** Dashboard provides browser-based flashing via `esptool-js` for full recovery
4. **USB fallback:** Standard `esptool.py` for manufacturing / batch flashing

### 2. Ingestion Server

**WebSocket endpoint** at `/ws/node` on the mothership's HTTP port. Each ESP32 node maintains a single persistent bidirectional connection.

**Upstream (node → mothership):**
- Binary frames: CSI samples (parsed into `(link_id, timestamp, csi_vector)` tuples)
- JSON frames: registration (`hello`), health metrics, OTA status reports, BLE scan results

**Downstream (mothership → node):**
- JSON frames: role assignment, packet rate config (including adaptive rate commands), OTA commands, reboot commands, identify (blink LED)

**Node↔Mothership JSON Message Schemas:**

All JSON messages include a `"type"` discriminator. Unknown type values are silently ignored by the receiver (forward-compatible). MAC addresses are uppercase colon-separated hex (`"AA:BB:CC:DD:EE:FF"`). Timestamps are Unix milliseconds (uint64). Field naming is snake_case throughout. Maximum JSON frame size: 4 KB (ESP32 heap constraint).

**WebSocket keepalive (ping/pong):**
- **Mothership → Node:** The mothership sends WebSocket ping frames every 30 s on each node connection. Read deadline is set to 60 s (= 2× ping interval). If no data (including pong) is received within the read deadline, the connection is closed and the node is marked OFFLINE.
  - Implementation: `conn.SetReadDeadline(time.Now().Add(60 * time.Second))` reset on every received frame (data or pong). A goroutine sends pings on a 30 s ticker.
- **Node → Mothership:** ESP32 `esp_websocket_client` has a built-in keepalive option: `config.ping_interval_sec = 30`. The ESP32 sends ping frames autonomously; the mothership replies with pong (handled by the Go WebSocket library automatically).
- **Mothership → Dashboard:** Dashboard WebSocket connections use the same 30 s ping / 60 s read deadline. The browser WebSocket API responds to pings automatically.
- **Purpose:** Keeps NAT state tables alive (typical residential NAT timeout = 60–120 s); detects silently-dropped connections (e.g., WiFi power management dropping packets) within 60 s.

```jsonc
// UPSTREAM: hello — first message on every connect
{"type":"hello","mac":"AA:BB:CC:DD:EE:FF","node_id":"f47ac10b-...","firmware_version":"1.2.3",
 "capabilities":["csi","ble","tx","rx"],"chip":"ESP32-S3","flash_mb":16,"uptime_ms":4200}

// UPSTREAM: health — every 10 s
{"type":"health","mac":"AA:BB:CC:DD:EE:FF","timestamp_ms":1711234567890,
 "free_heap_bytes":204800,"wifi_rssi_dbm":-52,"uptime_ms":3600000,
 "temperature_c":42.1,"csi_rate_hz":20,"wifi_channel":6,"ip":"192.168.1.123"}

// UPSTREAM: ble — every 5 s
{"type":"ble","mac":"AA:BB:CC:DD:EE:FF","timestamp_ms":1711234567890,
 "devices":[{"addr":"AA:BB:CC:DD:EE:FF","addr_type":"public","rssi_dbm":-62,
             "name":"iPhone","mfr_id":76,"mfr_data_hex":"0215..."}]}

// UPSTREAM: motion_hint — when on-device variance exceeds threshold
{"type":"motion_hint","mac":"AA:BB:CC:DD:EE:FF","timestamp_ms":1711234567890,"variance":0.043}

// UPSTREAM: ota_status — during OTA progress
{"type":"ota_status","mac":"AA:BB:CC:DD:EE:FF","state":"downloading","progress_pct":45}
// state values: "downloading" | "verifying" | "writing" | "rebooting" | "failed"
// "failed" adds: "error":"sha256_mismatch" | "download_failed" | "write_failed"

// DOWNSTREAM: role — assign operational role
{"type":"role","role":"rx"}
// role values: "tx" | "rx" | "tx_rx" | "passive" | "idle"
// passive adds: "passive_bssid":"AA:BB:CC:DD:EE:FF"

// DOWNSTREAM: config — change operational parameters
{"type":"config","rate_hz":50,"tx_slot_us":5000,"variance_threshold":0.02}
// all fields optional; omit to leave unchanged

// DOWNSTREAM: ota — trigger firmware update
{"type":"ota","url":"http://spaxel.local:8080/firmware/spaxel-1.3.0.bin",
 "sha256":"e3b0c44298fc1c149afb...","version":"1.3.0"}

// DOWNSTREAM: reboot
{"type":"reboot","delay_ms":1000}

// DOWNSTREAM: identify — blink LED
{"type":"identify","duration_ms":5000}

// DOWNSTREAM: baseline_request — node sends a health frame with extra CSI stats
{"type":"baseline_request"}

// DOWNSTREAM: shutdown — mothership is shutting down
{"type":"shutdown","reconnect_in_ms":30000}

// DOWNSTREAM: reject — authentication or policy failure (connection closes after)
{"type":"reject","reason":"invalid_token"}
// reason values: "invalid_token" | "unknown_node" | "rate_limited"
```

**Protocol rules:**
- Node sends `hello` as the first message. Mothership responds with `role` + `config` within 2 s, or `reject` (then closes the connection).
- OTA is two-phase: mothership sends `ota` → node sends `ota_status` frames → mothership monitors until `"rebooting"` or `"failed"`.
- Both sides ignore unknown `type` values.
- Node does not need to ACK non-OTA downstream messages (fire-and-forget). Role and config changes take effect immediately on receipt.
- TCP/WebSocket ordering guarantees in-order delivery; no sequence numbers needed.

**Authentication:**
- On first run, the mothership auto-generates a random 256-bit installation secret (`SPAXEL_INSTALL_SECRET`), stores it in SQLite, and prints it once to stdout: `[SPAXEL] Installation secret: <hex>. Shown once — saved to /data/spaxel.db`.
- If `SPAXEL_INSTALL_SECRET` is set in the environment, it takes precedence (useful for scripted deployments).
- During provisioning, the mothership derives a per-node token: `HMAC-SHA256(install_secret, node_mac)`. This token is embedded in the provisioning NVS payload written via Web Serial.
- On WebSocket connect, the node includes its token as the `X-Spaxel-Token` HTTP header during the upgrade request. The mothership verifies before completing the upgrade. Nodes without a valid token are rejected with HTTP 401 and the connection is closed. An invalid-token counter per IP triggers a 60-second block after 5 consecutive failures.
- Dashboard access is protected by a PIN. On first run, if no PIN is configured, the dashboard shows a one-time setup page to set a PIN (stored as bcrypt hash in SQLite). Subsequent visits require the PIN, which issues a session cookie (secure, HttpOnly, 7-day TTL). If the mothership is behind Traefik with TLS, the cookie is also SameSite=Strict.
- The `/ws/dashboard` endpoint verifies the session cookie before upgrading. Unauthenticated dashboard connections receive HTTP 401.
- **LAN-only binding (defense in depth):** By default, the mothership binds only to the interface associated with the container's LAN-facing network. The `SPAXEL_BIND_ADDR` environment variable (default `0.0.0.0`) can restrict this further. Users are advised not to expose port 8080 directly to the WAN without a reverse proxy with TLS.
- Nodes provisioned before auth was added (e.g., during development) can be re-provisioned via Web Serial to obtain a valid token. The dashboard shows an "Unpaired" badge on nodes connecting without a token during a one-time migration window (configurable, default 24 h after auth is first enabled), then enforces strict rejection thereafter.

**Connection management:**
- One goroutine per connection handles both directions
- Node identity established by the `hello` message on connect (MAC + firmware version)
- Maintain per-link ring buffers (last 256 samples, ~5–12 s at 20–50 Hz)
- Detect new links automatically — no pre-registration required
- Connection state is authoritative: disconnect = node offline (immediate, no timeout). Reconnect = node online, re-send current config
- Write CSI frames to the recording buffer for time-travel replay (see Component 14)
- Pass completed sample windows to the signal processing pipeline

**Binary CSI frame validation (ingestion server):**

Before the frame is enqueued for processing, the following checks are applied. Frames failing any check are silently dropped (not logged at INFO — only at DEBUG to avoid flooding logs at 600 frames/s):

```
Minimum frame length:  24 bytes (header only; n_sub=0 is valid as a header-only probe)
Maximum frame length:  24 + 128×2 = 280 bytes (n_sub max = 128; more = malformed)
                       (ESP32-S3 CSI is 64 subcarriers; 128 is a safety margin for future hardware)

Validation rules (in order):
1. len(frame) < 24:              drop — frame too short to contain header
2. n_sub = frame[23]:            read from byte 23
3. 24 + n_sub×2 != len(frame):  drop — payload length mismatch
4. n_sub > 128:                  drop — implausible subcarrier count
5. rssi (frame[20]) == 0:        allowed (0 = invalid RSSI per firmware spec); flag for AGC skip in pipeline
6. channel (frame[22]) == 0:     drop — channel 0 is invalid
7. channel (frame[22]) > 14:     drop — invalid 2.4 GHz channel

On drop: increment per-connection malformed_frame_count counter.
If malformed_frame_count > 100 within 1 minute: log WARN "Node [mac] sending malformed CSI frames".
If > 1000 within 1 minute: close connection (likely firmware bug or protocol mismatch).
```

### 3. Signal Processing Pipeline

Runs per-link, converting raw I/Q into motion features. Based on `docs/research/04-signal-processing.md`.

**Pipeline stages:**

```
Raw I/Q → Complex CSI → Phase Sanitisation → Feature Extraction → Motion Score
```

1. **Complex CSI computation:** Convert int8 I/Q pairs to float64 complex numbers, compute amplitude and phase per subcarrier

2. **Phase sanitisation** (per CSI frame, per link):

   ```
   Input: n_sub int8 pairs (I_k, Q_k), rssi_dbm int8

   Step 1 — Complex CSI:
     for k in 0..n_sub-1:
       csi[k] = complex(float64(I_k), float64(Q_k))
       amplitude[k] = abs(csi[k])       // = sqrt(I²+Q²)
       phase[k] = atan2(Q_k, I_k)       // radians, range [-π, π]

   Step 2 — RSSI normalization (AGC compensation):
     rssi_ref = -30.0  // dBm
     if rssi_dbm != 0:  // 0 = invalid; skip normalization
       norm = pow(10.0, (rssi_ref - float64(rssi_dbm)) / 20.0)
       amplitude[k] *= norm  for all k

   Step 3 — Spatial phase unwrapping (across subcarriers, per frame):
     // Detect and correct 2π jumps between adjacent subcarrier phases
     unwrapped[0] = phase[0]
     for k in 1..n_sub-1:
       delta = phase[k] - phase[k-1]
       while delta > π:  delta -= 2π
       while delta < -π: delta += 2π
       unwrapped[k] = unwrapped[k-1] + delta

   Step 4 — Linear regression (OLS) over data subcarriers:
     // Fit: unwrapped_phase_k = a·k + b, where k = subcarrier index
     // X axis = subcarrier index k, Y axis = unwrapped_phase[k]
     // Use only data subcarrier indices (not null/guard/pilot)
     // Closed-form OLS:
     n = len(data_indices)
     sum_k = sum(k for k in data_indices)
     sum_kk = sum(k² for k in data_indices)
     sum_y = sum(unwrapped[k] for k in data_indices)
     sum_ky = sum(k·unwrapped[k] for k in data_indices)
     denom = n·sum_kk - sum_k²
     a = (n·sum_ky - sum_k·sum_y) / denom  // STO slope (radians/subcarrier)
     b = (sum_y - a·sum_k) / n             // CFO intercept (radians)

   Step 5 — Residual phase:
     residual[k] = unwrapped[k] - (a·k + b)  for all k

   Output: amplitude[k] (RSSI-normalized), residual[k] (phase)
   Both are float64 arrays of length n_sub.
   The residual phase is the primary input to NBVI selection and feature extraction.
   The raw amplitude (before normalization) is NOT stored — only the normalized version.
   ```

   If any step produces NaN or Inf (e.g., rssi_dbm causes overflow, zero I/Q pair), the frame is skipped and a warning is logged. The regression denominator is checked for near-zero before division.

   **Note on subcarrier spacing:** The STO slope `a` in radians/subcarrier corresponds to a round-trip delay of `a / (2π × Δf)` where `Δf = 312.5 kHz` for HT20. This estimate is not used further — it's removed as a nuisance parameter.

   **Pipeline ordering:** CSI frames are first written to the replay store (raw binary, before any processing), then phase sanitization runs on a copy. This ensures the replay store contains raw I/Q and any algorithm changes can be applied to it later.
3. **Subcarrier selection:**

   **HT20 (802.11n 20 MHz) subcarrier map** (64 total):
   - Null subcarriers (excluded): indices 0 (DC), 1, 63 (guard)
   - Guard band (excluded): indices 27–37 (center guard + upper null carriers)
   - Pilot subcarriers (excluded from NBVI selection): indices 7, 21, 43, 57
   - Data subcarriers (eligible): all remaining = 47 subcarriers

   **NBVI (Normalized Bandwidth Variance Index) selection algorithm:**

   NBVI for subcarrier i over window W: `NBVI_i = Var(amplitude_i) / (Mean(amplitude_i))²`

   This normalizes variance by the square of the mean amplitude, making the metric scale-invariant across subcarriers with different mean gains.

   - Variance and mean computed using **Welford's online algorithm** for numerical stability over a sliding window of W=100 samples (~5 s at 20 Hz)
   - Update period: NBVI scores recalculated every 2 s (every 40 samples at 20 Hz)
   - Minimum samples required before applying selection: 50 samples (~2.5 s). Before that, use all 47 data subcarriers
   - Selection: take the top 16 subcarriers by NBVI score (the 16 with highest normalized variance)
   - Threshold floor: exclude any subcarrier with NBVI < 0.001 even if it would be in the top 16 (indicates a degenerate link)
   - Fallback: if fewer than 8 subcarriers pass the threshold, use all data subcarriers (link quality may be poor)
   - Selected subcarrier indices are stored as a `[64]bool` mask per link in memory (recomputed on restart from buffered samples; no SQLite persistence needed)
   - NBVI is computed on phase-sanitised CSI amplitudes (after STO/CFO removal), not raw amplitudes
   - deltaRMS and phase variance features use only selected subcarriers. Breathing band uses all 47 data subcarriers (the low-frequency signal is spread across all subcarriers; selection would discard useful signal)
   - Diagnostic view shows a per-subcarrier NBVI bar chart with selected subcarriers highlighted in green
4. **Feature extraction** (computed per fusion tick, 10 Hz, on a window of recent samples from the link's ring buffer):

   **deltaRMS** (primary motion indicator):
   ```
   selected = NBVI-selected subcarrier indices (up to 16)
   deltaRMS = sqrt( mean( (amplitude_norm[k] - baseline[k])^2 for k in selected ) )
   ```
   - `amplitude_norm[k]`: RSSI-normalized amplitude from phase sanitization
   - `baseline[k]`: current EMA baseline for subcarrier k (see baseline management below)
   - Result is dimensionless. Typical values: ~0.02 (empty room), ~0.10 (walking), ~0.30 (vigorous motion)
   - A 5-sample exponential smoothing (α=0.3) is applied to deltaRMS before thresholding: `smooth_deltaRMS = 0.3·deltaRMS + 0.7·prev_smooth`

   **Phase variance** (sub-wavelength displacement indicator):
   ```
   phase_variance = variance( residual_phase[k] for k in selected )
   ```
   - Computed per-frame over selected subcarriers. High variance = person at non-null position in Fresnel zone.
   - Reported in diagnostics panel. Not directly used in Fresnel accumulation (deltaRMS is the primary).

   **Breathing band** (stationary person detection):
   - IIR Butterworth bandpass filter, order 4, passband 0.1–0.5 Hz, sampling rate 20 Hz (at active rate)
   - Filter applied to the time series of `mean(residual_phase[k])` over all 47 data subcarriers
   - Implemented as two cascaded biquad sections (standard Butterworth IIR representation). Biquad coefficients precomputed at build time for Fs=20 Hz using the bilinear transform; embedded as constants.
   - Filter state (4 state variables per biquad section = 8 floats) maintained per link in memory
   - `breathing_rms = sqrt( mean( filtered_phase[t]^2 over last 60 s = 1200 samples ) )`
   - Detection threshold: breathing_rms > 0.005 radians (sustained for >30 s) → stationary person present
   - **Breathing rate estimation:** FFT over 512-sample window (25.6 s at 20 Hz), zero-padded to 1024 points. Frequency resolution: 20/1024 ≈ 0.02 Hz. Find dominant peak in 0.1–0.5 Hz bin range. Convert bin to Hz to bpm. Apply 60-second EMA smoothing to the rate estimate.
   - Only computed when `smooth_deltaRMS < 0.03` (person is still). When in motion, breathing detection is disabled.

5. **Baseline management:**
   - EMA baseline per link per subcarrier with time constant τ = 30 s (configurable)
   - Update rule: `α = dt / (τ + dt)` where dt = 1/fusion_rate_hz ≈ 0.1 s → α ≈ 0.0033
   - `baseline[k] = α·amplitude_norm[k] + (1-α)·baseline[k]`
   - **Motion-gated updates:** Only update when `smooth_deltaRMS < motion_threshold (default 0.05)` — prevents adapting to a stationary person
   - **Initialization:** On first data for a link, baseline = first amplitude sample. On restart, restored from the most recent SQLite baseline snapshot.
   - If the loaded snapshot is older than 7 days: use it as the starting point but mark baseline confidence as 0.3 (show as low in the diagnostic view). The confidence increases as the EMA accumulates new quiet-room samples.
   - Baseline snapshots stored to SQLite every 60 s and on graceful shutdown. Also stored on demand (manual re-baseline, node position change).
   - Re-baseline triggered on: node position change, manual request from dashboard, significant environment change detected (drift >2σ from calibration snapshot)

### 4. Fusion & Localizer

Combines per-link motion scores into spatial blob positions. Based on Fresnel zone geometry (`docs/research/03-algorithms.md`).

**Algorithm — Fresnel Zone Weighted Localization:**

**Physical constants:**
- WiFi wavelength: λ = c/f = 3×10⁸ / 2.437×10⁹ ≈ 0.123 m (for 2.437 GHz, channel 6)
- Half-wavelength excess per zone: λ/2 ≈ 0.0615 m

**Full algorithm:**

1. Divide the floor plan into a 3D grid:
   - XY resolution: `SPAXEL_GRID_CELL_M` (default 0.20 m). XY extent: derived from zone bounds union (the bounding box of all defined zones + 0.5 m margin on each side)
   - Z resolution: 0.10 m (fixed; not configurable — Z accuracy is already limited by node height diversity). Z extent: 0 to `max(zone.z + zone.h for all zones)` (default 0 to 3.0 m if no zones defined yet)
   - Grid origin: (min_x - 0.5, min_y - 0.5, 0) where min_x/min_y are the minimum zone coordinates
   - Grid is reallocated (zeroed and recreated) whenever zone bounds change or SPAXEL_GRID_CELL_M changes
   - Maximum grid size: 100×100×30 cells = 300,000 cells (prevents memory explosion; warn if zone bounds exceed this at current cell size)

2. **Per-link zone number cache** (computed once per link, cached in memory, invalidated when any node is repositioned):
   - For link (TX at position T, RX at position R) and grid cell center P:
   - `ΔL = |P-T| + |P-R| - |T-R|` (path length excess over direct path, in meters)
   - `zone_number = ceil(ΔL / (λ/2))` (zone 1 if ΔL < λ/2, zone 2 if ΔL < λ, etc.)
   - Cells with `zone_number > 5` get `zone_number = OUTSIDE` (weight = 0, excluded from accumulation)
   - Cache format: sparse map `{link_id: [(cell_idx, zone_number), ...]}`  — only cells inside zone 5 are stored

3. **Per-frame accumulation:**
   - For each active link (deltaRMS > threshold, default 0.02):
   - `cell_weight = deltaRMS × link_weight[link_id] × zone_decay(zone_number)`
   - Accumulate into a `float64` 3D grid (same dimensions as the zone cache)

4. **Zone decay function** (parameterizable via time-travel tuning slider "Fresnel weight decay rate"):
   - `zone_decay(n) = 1.0 / pow(float64(n), decay_rate)`
   - Default `decay_rate = 2.0` (inverse square, consistent with ~10 dB sensitivity gradient between zones)
   - Slider range: 1.0 (flat/no decay) to 4.0 (strong decay, zone 1 dominates completely)
   - Zone 1: weight = 1.0; Zone 2: 0.25; Zone 3: 0.11; Zone 4: 0.0625; Zone 5: 0.04 (at default decay_rate=2)

5. **Combined cell weight:** `accumulated[cell] += deltaRMS × link_weight[link_id] × zone_decay(zone_number)`
   - `link_weight[link_id]` is the per-link learned weight (Component 22), initialized to 1.0

6. Extract peaks from the accumulated grid (3D local maxima using 6-connected neighborhood)
   - Minimum peak height threshold: configurable, default 0.10 (in accumulated weight units)
   - Non-maximum suppression: only keep cells with value > all 6 direct neighbors

7. Each peak becomes a blob: `{x, y, z, confidence}` where:
   ```
   max_possible_weight = sum(deltaRMS[link] × link_weight[link] for all active links with deltaRMS > threshold)
   confidence = min(1.0, peak_value / max_possible_weight)
   ```
   This normalizes confidence so that a cell at zone-1 intersection of all active links would get confidence ≈ 1.0.
   If `max_possible_weight = 0` (no active links), no peaks are emitted (confidence would be undefined).

**Active link threshold:** `deltaRMS > 0.02` (configurable). Links below threshold contribute zero weight to the grid (no partial contribution — prevents noise accumulation).

**Grid reset:** The accumulated grid is zeroed at the start of every fusion loop iteration (10 Hz). It is not persisted between frames.
6. **Blob tracking:** Assign persistent IDs via greedy nearest-neighbor matching against previous frame's tracked blobs. Association threshold: 1.0 m (if the nearest old blob is > 1.0 m away, the new peak is treated as a new blob).

   **Assignment algorithm (greedy, O(N×M)):**
   ```
   Input: new_peaks = list of {x,y,z,confidence} from Fresnel grid extraction
          old_blobs = list of active UKF-tracked blobs with predicted positions at current time

   1. Build distance matrix D[i][j] = |new_peaks[i].pos - old_blobs[j].predicted_pos|
   2. Sort all (i, j) pairs by D[i][j] ascending
   3. Greedy assignment: for each (i, j) in sorted order:
        if new_peaks[i] not yet assigned AND old_blobs[j] not yet matched AND D[i][j] <= 1.0m:
          assign new_peaks[i] → old_blobs[j] (update UKF with new_peaks[i] as measurement)
   4. Unmatched new_peaks: create new blob with fresh ID (monotonically increasing uint64 counter)
   5. Unmatched old_blobs: run predict-only UKF step (no measurement); decay confidence

   Tie-breaking: if two new peaks are equidistant from an old blob, the one with higher confidence
   wins. The other gets its own new ID.
   ```

   **Blob ID lifecycle:**
   - IDs are assigned at creation and never reused within a mothership session
   - ID counter is in-memory only (resets to 1 on restart); dashboard handles re-mapping on reconnect
   - Maximum concurrent tracked blobs: 20 (configurable via settings `"max_tracked_blobs"`); additional peaks above this limit are discarded (log WARN if exceeded). This prevents runaway blob proliferation from noise spikes.

   New peaks get new IDs; unmatched old blobs decay over 3 s before removal
7. **Biomechanical UKF:** Per-blob Unscented Kalman Filter (UKF). Library: `gonum/floats` + `gonum/mat` for matrix operations. All UKF state is in memory per blob (not persisted to SQLite).

   **State vector** (n=6): `x = [px, py, pz, vx, vy, vz]` (position in meters, velocity in m/s)

   **Process model** (constant velocity with noise, dt = 0.1 s at 10 Hz):
   ```
   px' = px + vx·dt
   py' = py + vy·dt
   pz' = pz + vz·dt
   vx' = vx   (+ process noise)
   vy' = vy   (+ process noise)
   vz' = vz   (+ process noise)
   ```

   **Process noise covariance Q** (diagonal, tuned empirically):
   - Position: σ_p = 0.01 m² (= (0.1 m)²) — small, position changes are smooth
   - Velocity: σ_v = 0.25 m²/s² (= (0.5 m/s)²) — allows velocity changes of ±0.5 m/s per step
   - `Q = diag([σ_p, σ_p, σ_p, σ_v, σ_v, σ_v·0.25])` — Z velocity noise halved (humans are more constrained vertically)

   **Measurement model**: observation z_obs = [px, py, pz] (position only from Fresnel peak)

   **Measurement noise covariance R** (3×3 diagonal, adaptive):
   - Base: σ_obs = 0.3 m (= (0.3 m)²) — typical Fresnel grid accuracy
   - Adaptive: `R = diag([σ_obs/confidence, σ_obs/confidence, σ_obs_z/confidence])` where `confidence` is the Fresnel peak confidence (0–1) and `σ_obs_z = 1.0 m` (Z is less accurate)

   **Initial state** (new blob from Fresnel peak):
   - State: `[peak_x, peak_y, peak_z, 0, 0, 0]` (zero initial velocity)
   - `P0 = diag([1.0, 1.0, 1.0, 4.0, 4.0, 4.0])` (large initial uncertainty)

   **UKF sigma point parameters** (Wan & van der Merwe 2000):
   - `alpha = 0.001`, `beta = 2.0`, `kappa = 0`, `lambda = alpha²·(n+kappa) - n`
   - 2n+1 = 13 sigma points

   **Biomechanical constraint application** (applied to each sigma point after prediction, before covariance computation):
   - Maximum XY velocity: clamp `sqrt(vx²+vy²)` to 2.0 m/s (scale both components proportionally)
   - Maximum acceleration: if |dv/dt| > 3.0 m/s², scale back the velocity delta
   - Minimum turning radius: if velocity direction changes by >30° in one step, limit the angular change
   - Z velocity: clamp to [-9.8·dt, +1.5·dt] m/step (gravity floor, biological ceiling)
   - Collision avoidance soft repulsion: if two blobs are within 0.4 m, add a repulsion delta to each sigma point's XY position (force = 0.1·(0.4-d)/0.4 m per step)

   **Persistence** (no Fresnel peak within 1.0 m association threshold):
   - Run predict-only (no measurement update)
   - Confidence decays: `confidence *= exp(-dt/τ)` where `τ = 1.0 s`
   - After 3.0 s of no association (confidence < 0.05), mark blob for removal
   - On removal: log a `blob_disappeared` event; if BLE-identified, log a `person_left_detection` event

   **Warm start**: if a blob is re-associated after a brief gap (<3 s, predict-only), it resumes with its last predicted state (not reset). This prevents ID-swapping when two blobs are close and briefly merge in the grid.
8. **BLE identity matching (see Component 21):** Fuse BLE RSSI reports with blob positions to assign person/device labels to tracked blobs
9. **Self-improving weights (see Component 22):** Use BLE proximity as continuous ground truth to refine per-link Fresnel zone weights over time
10. Publish blob list (with identity, posture, velocity) to dashboard via WebSocket at 10 Hz

**Multi-person handling:**
- Works naturally: multiple people create multiple peaks in the Fresnel accumulation grid
- Degrades gracefully: overlapping Fresnel zones merge blobs when people are close together
- Practical limit: 2–3 people reliably, 4+ increasingly unreliable

**Z-axis:**
- Requires nodes at mixed heights (e.g., 0.3 m and 2.0 m)
- Fresnel zones computed in full 3D — blob Z-coordinates are first-class, rendered as true vertical positions in the 3D scene with pillar anchors to the ground plane
- Resolution ±1–2 m — enough for "standing vs. lying down", and critical for fall detection (Component 16)

### 5. Fleet Manager

Manages the lifecycle and role assignment of all ESP32 nodes.

**Node registry (SQLite):**
- MAC address (primary key)
- Friendly name (user-assigned)
- Position (x, y, z in floor plan coordinates — set during onboarding)
- Current role (TX / RX / TX_RX / PASSIVE / IDLE)
- Firmware version
- Last heartbeat timestamp
- Status (ONLINE / STALE / OFFLINE)

**Role assignment engine:**

The mothership decides which nodes transmit, receive, or do both. Goals:
- Maximize spatial coverage (link diversity across the floor plan)
- Minimize RF contention (too many simultaneous TXs cause packet collisions)
- Ensure every node participates in at least one link

**Strategy:**
- With ≤4 nodes: All nodes TX/RX (time-division). Every pair forms a bidirectional link
- With 5–8 nodes: Select ~40% as dedicated TX, rest as RX. Optimize TX selection for angular diversity and GDOP minimization
- With 9+ nodes: Cluster nodes by room/zone. Within each zone, apply the 5–8 strategy. Cross-zone links use perimeter nodes
- **Passive radar option:** If a home WiFi AP BSSID is configured, all nodes default to PASSIVE (RX-only using router traffic). Dedicated TX nodes can be added for higher resolution but aren't required
- Role changes pushed as JSON messages on the node's existing WebSocket connection

**Stagger schedule:**

TX nodes transmit in time-division slots to avoid packet collisions. WiFi CSMA/CA handles residual collisions transparently, but staggering reduces collision probability dramatically.

**Slot computation:**
- Period: `period_us = 1,000,000 / packet_rate_hz` (e.g., 50,000 µs at 20 Hz)
- Slot width: 40% of period (e.g., 20,000 µs at 20 Hz) — guard time is 60% to tolerate clock drift
- Node i (of N TX nodes) is assigned: `tx_slot_offset_us = i × period_us / N`
- Sent to each TX node as `tx_slot_us` in the `config` downstream message

**Clock synchronization:**
- Each ESP32 runs `esp_sntp_init()` on boot, syncing to `pool.ntp.org` (default) or a configurable server
- NTP server configurable via: (a) `SPAXEL_NTP_SERVER` env var on the mothership → embedded in the provisioning payload; (b) `config` downstream message field `ntp_server: "192.168.1.1"`
- NTP sync is attempted for up to 10 s on boot. If it fails, the node transmits at the configured rate without stagger (relies on CSMA/CA for collision avoidance), and logs a warning in the health message
- ESP32 crystal accuracy: ±20 ppm → ±1.2 ms drift per minute. With 20,000 µs slot widths, drift is negligible within a 10-minute window between NTP resync
- Resync: nodes resync NTP every 10 minutes (configurable)

**Slot timer implementation (firmware):**
- On receiving `tx_slot_us` in a `config` message: cancel any existing TX timer; compute `next_tx_us = unix_us_now + (tx_slot_offset_us - (unix_us_now % period_us)); if next_tx_us < now: next_tx_us += period_us`
- Create an `esp_timer` recurring timer that fires at `next_tx_us` and then every `period_us`
- On timer fire: send one probe packet burst (the configured probe sequence for CSI capture)

**Collision detection (mothership):**
- If CSI frames from two different TX nodes arrive within 3 ms of each other, the mothership logs a "possible slot collision" metric for that link pair
- If collision rate > 5% over a 60-second window, the mothership re-randomizes the stagger assignments (shifts one node's slot by half a slot width) and pushes updated `config` messages

**In passive radar mode:** No stagger is needed — the router is the only TX. The router's beacon interval (configurable by the user, typically 100 ms = 10 Hz) is the natural clock.

**Health monitoring:**
- Node WebSocket connection state is authoritative: connected = ONLINE, disconnected = OFFLINE
- Nodes send health JSON (heap, RSSI, uptime, temperature) every 10 s on their WebSocket
- Dashboard shows node status with color coding (green/yellow/red)

**Self-healing (see Component 12):** When a node goes offline, the fleet manager automatically re-optimizes roles among remaining nodes to maintain best possible coverage.

### 6. OTA Update System

Firmware updates pushed from mothership to fleet.

**Update flow:**
1. New firmware binary placed in mothership's `/firmware/` volume (or uploaded via dashboard)
2. Mothership computes SHA-256 hash, stores version metadata
3. Dashboard shows "Update available" badge on nodes with older firmware
4. Update initiated: manually (single node or "Update All") or automatically (see below)
5. Mothership sends OTA command on the node's WebSocket: `{type: "ota", url: "http://mothership:8080/firmware/latest.bin", sha256: "...", version: "..."}`
6. Node downloads firmware via HTTP, verifies SHA-256, writes to inactive OTA partition
7. Node reboots, reconnects WebSocket, sends `hello` with new firmware version — mothership confirms upgrade
8. If new firmware fails to connect within 60 s, ESP-IDF rollback mechanism reverts to previous partition

**Auto-update mode (configurable, default: off):**
- When enabled, the mothership automatically begins a rolling update when new firmware is detected in `/firmware/`
- **Canary strategy:** First, update a single node (the one with the lowest coverage impact if lost). Monitor its detection quality contribution for 10 minutes against the fleet baseline. If quality holds (no degradation >5%), proceed with rolling update of remaining nodes. If quality degrades, automatically roll back the canary and alert the user: "Auto-update paused: canary node showed degraded performance. Review before retrying."
- **Scheduling:** Auto-updates only run during a configurable quiet window (default: 02:00–05:00 local time) to minimize disruption. If no quiet window is configured, updates run when all zones have been vacant for >10 minutes
- **Dashboard settings:** Toggle auto-update on/off, set quiet window, set canary duration, view update history
- **Notifications:** Timeline event + push notification on auto-update start, canary result, completion, or failure

**Firmware format and partition layout:**
- Firmware binaries are raw ESP-IDF OTA application images (the output of `idf.py build` → `build/spaxel.bin`)
- The version string is embedded in the binary via `esp_app_desc_t` (set via `CONFIG_APP_PROJECT_VER` in sdkconfig). The mothership reads the version from the firmware metadata at upload time by parsing the `esp_app_desc_t` structure at offset 32 bytes from the image start.
- Partition layout (`partitions.csv`): `factory` (4 MB), `ota_0` (4 MB), `ota_1` (4 MB), `nvs` (24 KB), `otadata` (8 KB). The dual OTA partition scheme (ota_0 + ota_1) is required for rollback.
- Maximum firmware image size: 4 MB (limited by OTA partition size). Firmware must not exceed 3.8 MB to leave a safety margin.
- Firmware file naming: `spaxel-<semver>.bin` (e.g., `spaxel-1.2.3.bin`). The `is_latest` flag in the firmware table marks the newest uploaded version.

**Node OTA procedure (firmware side):**
1. Receive `{type:"ota", url, sha256, version}` on WebSocket
2. Check: if current firmware version equals `version`, skip and reply with `ota_status: rebooting` is NOT sent — instead, reply with health message (already on latest)
3. Check: free heap ≥ 20 KB before starting (reject if insufficient, send `ota_status: failed, error:"low_heap"`)
4. Open HTTP connection to the OTA URL (using `esp_http_client`); 30 s connect timeout
5. Simultaneously: feed received chunks to `esp_ota_write()` (4 KB chunks) AND feed to SHA-256 running hash
6. Send `ota_status: downloading, progress_pct: N` every 10% of download
7. On download complete: finalize SHA-256 hash, compare to expected. On mismatch: abort, send `ota_status: failed, error:"sha256_mismatch"`, do NOT reboot
8. If SHA-256 matches: call `esp_ota_end()` and `esp_ota_set_boot_partition()` to make the new partition active
9. Send `ota_status: rebooting` — then `esp_restart()` after 1 s
10. On boot from new partition: firmware calls `esp_ota_mark_app_valid_cancel_rollback()` ONLY after successfully sending `hello` AND receiving a `role` message from the mothership (confirms connectivity)
11. If the new firmware fails to connect and mark itself valid within 60 s of boot: ESP-IDF automatically rolls back to the previous partition on next reset

**Rollback detection:** When a node reconnects with the same firmware version it had before an OTA attempt, the mothership checks if it was expecting a new version. If so, it marks the node's OTA status as ROLLBACK_OCCURRED in the dashboard (amber badge) and logs the event.

**Safeguards:**
- Never update all nodes simultaneously — rolling update with 30 s gap between nodes
- Canary node monitored before fleet-wide rollout (auto-update mode)
- If >50% of fleet goes OFFLINE during a rolling update, halt and alert
- Dashboard shows update progress per node (PENDING → CANARY → DOWNLOADING → REBOOTING → VERIFIED / FAILED / ROLLBACK)
- OTA URL does not require session authentication (it's served locally; IP-restricted to the container network)
- Old firmware versions are retained in `/firmware/` (not auto-deleted); configurable retention count (default 3)

### 7. Onboarding Flow

Adding a new ESP32-S3 node to the fleet.

**Zero-config first run (Web Serial — requires Chrome/Edge):**
1. User connects ESP32-S3 via USB to the machine running the dashboard
2. Dashboard's "Add Node" page uses Web Serial API to connect to the device
3. Mothership generates a provisioning payload: WiFi SSID, WiFi password, unique node ID. **No mothership IP needed** — firmware discovers mothership via mDNS (`_spaxel._tcp.local`)
4. Dashboard flashes firmware + writes provisioning config to NVS via `esptool-js`
5. Device reboots, connects to WiFi, discovers mothership via mDNS, opens WebSocket, sends `hello` registration
6. Mothership auto-detects the home WiFi AP's BSSID and begins passive radar mode — **presence detection is working within 30 seconds with zero additional configuration**
7. Dashboard shows guided wizard: "New node detected! I'm already seeing signal from your router. Walk around your space so I can start detecting presence."
8. After 60 s of user walking: "Great, I can see you. Let me help you place this node optimally." — Coverage painting activates, user drags node to position
9. User sets the node's Z-height (dropdown: floor level / desk height / ceiling mount, or manual entry)
10. "Want better accuracy? Plug in another ESP32." — Repeat from step 1

**Working presence detection in under 5 minutes with zero manual network configuration.**

**Subsequent re-provisioning:**
- If a node loses WiFi, captive portal at `spaxel-XXXX` allows re-entering credentials
- If mDNS fails (some networks block it), captive portal allows manual mothership IP entry
- Full reflash available via Web Serial from the dashboard

**Payload generation and Web Serial protocol:**

`POST /api/provision` (no auth required — called by the dashboard Web Serial flow) returns a JSON provisioning payload:

```jsonc
{
  "version": 1,                      // provisioning format version
  "wifi_ssid": "MyNetwork",          // WiFi SSID
  "wifi_pass": "secret",             // WiFi passphrase
  "node_id": "f47ac10b-...",         // UUID4 generated by mothership
  "node_token": "a1b2c3...",         // 64-char hex HMAC-SHA256(install_secret, node_mac)
  "ms_mdns": "spaxel",              // mDNS service name
  "ms_port": 8080,
  "debug": false
}
```

The `mac` parameter in the POST request body is optional. If provided, `node_token` is derived from that MAC. If absent, the mothership generates a UUID4 `node_id` and a placeholder token; the token is finalized when the node sends its first `hello` with its actual MAC (the token is recomputed and validated then, with a 120-second grace window for the node to connect after provisioning).

**Web Serial provisioning protocol:**

The firmware includes a serial provisioning listener active for the **first 10 seconds after boot** (or until provisioning completes). During this window, the firmware reads `\n`-terminated JSON from UART at 115200 baud:

```
Firmware listens for: {"provision": <provisioning_json>}\n
Firmware responds: {"ok": true, "mac": "AA:BB:CC:DD:EE:FF"}\n  (on success)
                   {"ok": false, "error": "..."}\n              (on failure)
```

**esptool-js integration:**

- **Version:** Pin to `esptool-js@0.4.x` (latest stable as of IDF 5.2 era). Loaded as an ES module from the mothership's static files (not CDN — bundled in the dashboard to avoid external dependencies):
  `<script type="module">import { ESPLoader, Transport } from "/static/esptool-js/bundle.js"</script>`
- The `bundle.js` is built from the `esptool-js` npm package at Docker image build time: `npx esbuild esptool-js/bundle.js --bundle --outfile=dashboard/static/esptool-js/bundle.js`
- Firmware binary `spaxel-X.Y.Z.bin` is fetched from `GET /firmware/<filename>` during the flash step (no auth required — uses the existing no-auth OTA serving path).

The dashboard's Web Serial flow (using `esptool-js` for firmware flashing + the Web Serial API for provisioning):
1. Flash firmware binary to the device (erase + write to factory partition, 0x10000 offset):
   ```js
   const transport = new Transport(serialPort);  // Web Serial API port
   const loader = new ESPLoader({transport, baudrate: 921600, terminal: {...}});
   await loader.main_fn();   // connect + detect chip
   await loader.write_flash({  // erase + write
     fileArray: [{data: firmwareArrayBuffer, address: 0x10000}],
     flashSize: "keep",
     eraseAll: false,
     compress: true
   });
   await loader.after = "hard_reset";  // reboot into firmware
   ```
2. Open a serial port at 115200 baud (using the Web Serial API directly, not esptool)
3. Wait for the device to reboot and enter the provisioning window (wait for "SPAXEL READY\n" prompt or 3 s timeout)
4. Send: `{"provision": <JSON from POST /api/provision>}\n`
5. Read the response. On `{"ok": true}`, extract the MAC and immediately call `POST /api/provision` with the confirmed MAC to finalize the node token.

**Fallback when Web Serial is not available** (Firefox, Safari, or non-HTTPS context): Show a download link for `spaxel-X.Y.Z.bin` and instructions for `esptool.py --port /dev/ttyUSB0 write_flash 0x10000 spaxel-X.Y.Z.bin`. The provisioning JSON is shown as text for manual entry via the captive portal.
6. Dashboard shows "Node provisioned! Waiting for it to connect..." and polls for a `hello` from the node with the confirmed MAC.

**Firmware partition offsets** (matching `partitions.csv`):
- Factory app: 0x10000 (4 MB)
- OTA_0: 0x410000
- OTA_1: 0x810000
- NVS: 0x9000 (24 KB)
- OTA data: 0xE000 (8 KB)

**Provisioning window timeout:** The 10-second window prevents normal operations from being accidentally interrupted by serial noise. If no valid provisioning JSON is received in 10 s, the firmware proceeds with its normal boot sequence (using NVS credentials if provisioned, or entering captive portal mode if not).

### 8. Dashboard (Web UI)

Single-page application served by the mothership's HTTP server. Built on Three.js for full 3D spatial visualization. Five modes of interaction: Live View (3D), Fleet Status, Setup/Calibration, Simple Mode, and Ambient Mode. Cross-cutting UX systems: Activity Timeline (Component 27), Detection Explainability (28), Feedback Loop (29), Context Notifications (30), Quick Actions (32), Command Palette (34), Morning Briefing (35), and Guided Troubleshooting (36).

**8a. Live View (default)**

Full-screen 3D scene showing the monitored space with real-time blob visualization.

**3D Scene:**
- **Renderer:** Three.js WebGLRenderer, full viewport, adaptive pixel ratio
- **Camera:** PerspectiveCamera with OrbitControls — mouse drag to rotate, scroll to zoom, right-drag to pan. Touch support for mobile (pinch zoom, two-finger pan)
- **Default view:** Isometric-ish angle looking down at ~45° onto the space. One-click preset buttons: Top (plan view), Front, Side, Perspective
- **Ground plane:** Gridded floor at Y=0 with metric scale markings. Optional floor plan image mapped as a texture on the ground plane (user-uploaded PNG/JPG, calibrated via two-point distance measurement)
- **Room bounds:** Translucent wireframe box showing the defined space extents. Walls rendered as semi-transparent planes so interior is always visible

**Blob rendering (3D) — Humanoid figures:**
- Each detected person rendered as a simplified humanoid figure (`SkinnedMesh` with 4-5 blend poses), deliberately abstract to respect privacy but immediately readable:
  - **Z > 1.4m + velocity > 0.3 m/s:** Standing, walking animation (leg/arm swing via `AnimationMixer`)
  - **Z > 1.4m + velocity < 0.3 m/s:** Standing idle, subtle breathing sway
  - **Z ~ 0.8–1.2m:** Seated posture (bent legs, upright torso)
  - **Z < 0.5m:** Lying down (horizontal figure)
  - **Z drops rapidly:** Falling animation (ties into fall detection alert)
- Color per person: when BLE identity is assigned, each figure gets a distinct color (user-configurable). Unidentified blobs use a neutral gray
- Vertical pillar: thin cylinder from ground plane to figure base — anchors XY position visually from non-top angles
- **Trails:** Last 60 positions rendered as fading footprint dots (small circles on ground plane) with per-vertex opacity. Color matches person color
- **Person label:** CSS2DRenderer overlay showing person name (or "Unknown #N") and zone. On hover: Z-height, velocity, confidence, BLE device name

**Node overlay (3D):**
- Each ESP32 rendered as a small box or custom mesh at its registered (x, y, z) position
- Color = status: green (ONLINE), yellow (STALE), red (OFFLINE)
- **Links:** `LineSegments` between TX→RX pairs, dashed material, opacity proportional to link quality. Toggle-able via toolbar
- **Fresnel zones (debug toggle):** Render first Fresnel zone ellipsoids as wireframe meshes between active link pairs — helps users understand coverage geometry
- Raycaster hover → tooltip with MAC, firmware version, RSSI, role. Click → detail panel slides in

**Controls toolbar (floating):**
- View presets: Top | Front | Side | Perspective
- Toggle layers: Links | Fresnel zones | Trails | Floor plan image | Crowd flow | Coverage quality
- Status bar: Node count (online/total), active blob count (with names if BLE-identified), system uptime, Detection Quality gauge
- Cmd+K shortcut hint (subtle, dismissible after first use)

**Activity timeline sidebar (Component 27):**
- Collapsible sidebar on the right edge of the 3D view
- Every event in one scrollable stream. Tap any event → 3D view jumps to that moment
- Inline thumbs up/down on every detection event (Component 29)
- "Why?" button on every detection to open explainability (Component 28)
- Search bar with natural language filtering

**Spatial quick actions (Component 32):**
- Right-click (desktop) or long-press (mobile) on any 3D element for context-sensitive actions
- Available on: blobs, nodes, empty space, zone labels, portals, trigger volumes

**WebSocket feed:** Mothership pushes updates at 10 Hz via `/ws/dashboard`. Blob IDs are in-memory integers; they are stable across the mothership's lifetime but reset on restart. The dashboard must handle a new set of blob IDs gracefully on reconnect.

The **first message** on every new WebSocket connection is always a `snapshot` message (type field = `"snapshot"`), sent within 100 ms of connection establishment. Subsequent messages omit the `type` field (treated as incremental updates). This enables instant dashboard usability on first connect and seamless reconnect.

```jsonc
// Snapshot message (first message on every connect/reconnect):
{
  "type": "snapshot",
  "blobs": [{"id": 1, "x": 3.2, "y": 1.1, "z": 0.8, "confidence": 0.85,
             "vx": 0.3, "vy": -0.1, "vz": 0.0, "posture": "walking",
             "person": "Alice", "ble_device": "iPhone (AB:CD:...)"}],
  "nodes": [{"mac": "AA:BB:CC:DD:EE:FF", "status": "online", "role": "rx", "rssi": -45,
             "name": "Kitchen North", "pos_x": 1.2, "pos_y": 0.5, "pos_z": 2.1,
             "firmware_version": "1.2.3", "virtual": false}],
  "zones": [{"id": 1, "name": "Kitchen", "count": 1, "people": ["Alice"],
             "x": 0, "y": 0, "z": 0, "w": 4, "d": 3, "h": 2.5}],
  "portals": [{"id": 1, "name": "Kitchen Door", "zone_a": "Hallway", "zone_b": "Kitchen"}],
  "triggers": [{"id": 1, "name": "Couch Dwell", "state": "active", "elapsed": 142, "enabled": true}],
  "confidence": 87,
  "security_mode": false,
  "predictions": [{"person": "Alice", "zone": "Kitchen", "probability": 0.87, "horizon_min": 15}],
  "uptime_s": 3600
}

// Incremental update message (10 Hz after snapshot):
{
  "blobs": [...],
  "nodes": [...],     // only nodes whose status changed since last frame
  "zones": [...],     // only zones whose occupancy changed since last frame
  "triggers": [...],  // only triggers that fired or changed state
  "confidence": 87,
  "events": [...]     // new events since last frame (empty array if none)
}
```

**WebSocket connection state management (dashboard client-side):**
- **Connection indicator:** A small colored dot in the toolbar status bar: green (connected), amber (reconnecting), red (disconnected for >30 s).
- **Brief disconnect (<5 s):** 3D scene retains last known state. Blob positions are extrapolated using last known velocity. No visual indicator changes.
- **Reconnecting (5–30 s):** Scene dims slightly (50% opacity overlay). "Reconnecting..." spinner in status bar. On successful reconnect: full snapshot received, scene returns to normal immediately.
- **Disconnected (>30 s):** "Connection lost" modal appears with "Reload page" button. The modal is non-blocking — user can still view the last known state.
- **Reconnect backoff:** 1 s, 2 s, 4 s, 8 s, max 10 s. Jitter: ±500 ms on each attempt to prevent thundering herd.
- **Post-reconnect:** The 3D scene rebuilds from the snapshot. Blob trail history is cleared (trails only show post-reconnect positions). Timeline events fetched separately via REST API to restore history.

**Performance:**
- Scene updates driven by `requestAnimationFrame`, decoupled from WebSocket rate
- Blob positions interpolated between WebSocket frames for smooth 60 fps motion
- `InstancedMesh` for trail segments if blob count × trail length exceeds ~500 objects
- LOD: reduce trail length and disable Fresnel zone rendering when >8 blobs active

**8b. Fleet Status**

Table view of all registered nodes. Overlaid as a slide-out panel over the 3D view.

| Column | Content |
|--------|---------|
| Name | User-assigned friendly name |
| MAC | Hardware address |
| Role | TX / RX / TX_RX — editable dropdown |
| Position | (x, y, z) — click to highlight node in 3D view and fly camera to it |
| Firmware | Version string + "Update available" badge |
| RSSI | Last reported WiFi signal strength |
| Status | ONLINE / STALE / OFFLINE with colored indicator |
| Uptime | Time since last boot |
| Actions | Restart, Update, Remove, Identify (blink LED) |

Global actions: Update All, Re-baseline All, Export Config, Import Config.

**8c. Setup / Calibration**

Space definition and node placement, all within the 3D view using TransformControls.

- **Space definition:** Set room dimensions (width × depth × height) numerically or by dragging corner handles in the 3D scene. Multi-room: add adjacent boxes, each with its own dimensions
- **Floor plan image:** Upload PNG/JPG, set two calibration points (click on image, enter real-world distance), image mapped as ground plane texture at correct scale.

  **Pixel-to-meter calibration transform:**
  ```
  Given:
    Point A: image pixel (pax, pay), real-world floor plan coords (ax, ay) m  [user places A at a known corner]
    Point B: image pixel (pbx, pby), real-world floor plan coords: derived from A + distance
    real_distance_m: known real-world distance |A−B| in meters

  Step 1 — Compute pixel scale:
    pixel_distance = sqrt((pbx-pax)² + (pby-pay)²)   [pixels]
    meters_per_pixel = real_distance_m / pixel_distance

  Step 2 — Compute rotation angle (image may not be axis-aligned with floor plan):
    angle_rad = atan2(pby-pay, pbx-pax) - atan2(by-ay, bx-ax)
    (by, bx): real-world coords of B. User sets A=(0,0) and B=(real_distance_m, 0).
    This reduces to: angle_rad = atan2(pby-pay, pbx-pax) (since B is along the +x axis)

  Step 3 — Convert any pixel (px, py) to floor plan meters:
    // Translate to A-relative pixel coords
    dx = px - pax;  dy = py - pay
    // Rotate to align with floor plan orientation
    mx = dx × cos(-angle_rad) - dy × sin(-angle_rad)
    my = dx × sin(-angle_rad) + dy × cos(-angle_rad)
    // Scale to meters and add A's floor plan offset
    floor_x = ax + mx × meters_per_pixel
    floor_y = ay + my × meters_per_pixel

  The calibration is stored as {cal_ax, cal_ay, cal_bx, cal_by, cal_distance_m} in the floorplan table.
  meters_per_pixel and angle_rad are computed at load time and cached (not stored).

  Three.js mapping: the floor plan image is applied as a THREE.PlaneGeometry texture in the XZ plane
  (Y=0). The image is scaled so that image_width × meters_per_pixel = geometry width.
  ```
- **Node placement:** Drag-and-drop nodes in 3D using TransformControls (translate mode). Snap-to-grid optional. Set position numerically via the fleet table as fallback
- **Baseline management:** View current baseline state per link. Manual re-baseline trigger. History of baseline snapshots
- **Environment change detection:** Alert when link characteristics shift significantly from baseline (suggests re-calibration)
- **Diagnostic view:** Raw CSI amplitude/phase plots per link (2D chart overlay). Useful for debugging node placement

**8d. Simple Mode (Progressive Disclosure)**

A clean, card-based interface for household members who don't need the full 3D engineering view.

- **No 3D scene.** Replaces the WebGL canvas with a responsive card layout
- **Room cards:** One per defined zone. Shows occupancy count, person names (if BLE-identified), and status color (green = empty, blue = occupied, red = alert). Tap to expand activity history for that zone
- **Activity feed:** Chronological list of events: "Alice entered Kitchen (2 min ago)", "Living Room vacant (15 min ago)", "Fall alert dismissed (1 hour ago)"
- **Alert banner:** Fall detection, anomaly alerts, system warnings — prominent but not overwhelming
- **Quick actions:** Arm/disarm security mode, trigger re-baseline, silence alerts
- **Sleep summary card:** Morning card showing last night's sleep data (if sleep monitoring is configured)
- **Mobile-first:** Designed as the primary mobile experience. Touch-friendly, no gestures required
- **Switching:** Toggle button in toolbar. Per-user default stored in browser `localStorage`. Optionally: simple mode requires no auth, expert mode requires a PIN

### 9. Baseline & Calibration System

The baseline represents the "empty room" state of each link. All motion detection is relative to this baseline.

**Establishing baseline:**
1. User triggers "Calibrate" from dashboard (or auto-triggered on first boot with all nodes online)
2. Mothership collects 60 s of CSI data on all links with no people present
3. Computes per-link baseline: mean amplitude and phase per subcarrier
4. Stores baseline snapshot to SQLite with timestamp

**Baseline drift handling:**
- EMA continuously adapts baseline with long time constant (τ = 30 s)
- Motion-gated: EMA update paused when motion detected on the link — prevents adapting to a stationary person
- **Environment change detection:** If baseline drifts more than 2σ from the calibration snapshot across multiple links, dashboard shows "Environment changed — consider re-calibrating" alert

**Triggers for re-calibration:**
- Node physically moved (user indicates via dashboard)
- Significant furniture rearrangement
- Seasonal changes (temperature/humidity affect propagation)
- Manual request from dashboard
- Automatic suggestion when detection quality degrades (high false-positive rate or low confidence scores)

**Baseline is per-link, not global.** Moving one node only requires re-baselining links involving that node, not the entire fleet.

### 10. Passive Radar Mode (Router-as-TX)

Every home WiFi AP broadcasts beacon frames at ~10 Hz plus regular data traffic. In passive radar mode, ESP32 nodes operate as receive-only — they capture CSI from the existing router's transmissions without any dedicated TX nodes in the fleet.

**How it works:**
- The AP's BSSID is **auto-detected** during provisioning (no manual entry required). Each ESP32 node calls `esp_wifi_sta_get_ap_info()` on boot to get the BSSID and channel of its connected AP, and includes this in the `hello` message as `"ap_bssid": "AA:BB:CC:DD:EE:FF"` and `"ap_channel": 6`.
- Mothership collects `ap_bssid` from all connected nodes. If all nodes report the same BSSID (or ≥80% agreement for mesh networks), the AP is auto-confirmed. If multiple BSSIDs appear (mesh network with different BSSIDs per satellite), each unique BSSID is registered as a separate virtual node.
- The onboarding wizard shows: "I detected your router (AA:BB:CC:DD:EE:FF — ASUS Router). Using it as a signal source." The user confirms with one tap. If the auto-detected BSSID seems wrong, the user can enter it manually via a text field.
- Mothership creates a **virtual node** entry in the `nodes` table for the AP: same schema as a real node, but with a `virtual=1` flag and `role='ap'`. The virtual node participates in Fresnel zone computation at its placed position.
- The AP virtual node appears in the 3D editor as a router icon (distinct from the ESP32 box icon). It starts at position (0, 0, 0) — the user repositions it in the 3D editor to match the physical router's location.
- All ESP32 nodes are assigned PASSIVE role — promiscuous mode, filtering for the AP's BSSID
- Each node extracts CSI from every beacon/data frame received from the AP
- CSI streams to mothership as normal binary frames, with `peer_mac` set to the AP's BSSID
- If the AP BSSID changes (router replacement, MAC address rotation on some routers), the dashboard shows an alert: "No CSI received from passive BSSID for >5 minutes. Your router's MAC address may have changed. [Re-detect BSSID]". Re-detection triggers fresh `hello` reporting from all nodes.
- OUI lookup: the first 3 bytes of the BSSID are looked up against an embedded OUI table (bundled in the Go binary at build time from the IEEE OUI registry) to show a friendly router manufacturer name.
  - **Source:** `https://standards-oui.ieee.org/oui/oui.txt` (download at build time in a `go generate` step)
  - **Format:** A sorted text file `internal/oui/oui.txt` with lines: `AA-BB-CC   (hex)   Manufacturer Name`. The `go generate` step transforms it to a compact Go source file `oui_data.go` with a `var ouiMap = map[uint32]string{0xAABBCC: "ASUS", ...}` (3 bytes packed as uint32, big-endian).
  - **Lookup:** `func LookupOUI(mac []byte) string { key := uint32(mac[0])<<16 | uint32(mac[1])<<8 | uint32(mac[2]); if name, ok := ouiMap[key]; ok { return name }; return "" }`
  - **Embedding:** `//go:generate` tag in `internal/oui/gen.go`; the generated `oui_data.go` is committed to the repo (not re-generated on every build — only when manually updating the OUI list).

**Adding virtual_node column to the nodes table:**

```sql
ALTER TABLE nodes ADD COLUMN virtual INTEGER NOT NULL DEFAULT 0;
ALTER TABLE nodes ADD COLUMN node_type TEXT NOT NULL DEFAULT 'esp32'
    CHECK (node_type IN ('esp32','ap'));
ALTER TABLE nodes ADD COLUMN ap_bssid TEXT;  -- for ap-type nodes: the BSSID being filtered
ALTER TABLE nodes ADD COLUMN ap_channel INTEGER;  -- for ap-type nodes
```

**Advantages:**
- Minimum deployment drops to **2 ESP32 nodes + existing router** — no dedicated TX hardware needed
- No TX stagger scheduling, no collision management
- Router transmits constantly and reliably — more stable than ESP32 probe packets
- Users can add dedicated TX nodes later for higher resolution, mixing passive + active links

**Limitations:**
- Router position is fixed — less geometric diversity than distributed TX nodes
- Beacon rate (~10 Hz) is lower than dedicated TX (20–50 Hz) — lower temporal resolution
- Some routers use beamforming that varies CSI per-frame — may need filtering

**Firmware:** Single `passive_bssid` NVS config field. When set, the CSI callback filters `peer_mac == passive_bssid` instead of accepting all peers.

### 11. Live Coverage Painting & GDOP Overlay

When placing or repositioning nodes in the 3D setup view, the ground plane dynamically displays a color-coded coverage quality map. As a node is dragged, the visualization updates in real-time.

**GDOP (Geometric Dilution of Precision) computation:**

GDOP quantifies how well a set of link geometries can localize a point. For CSI-based localization, the relevant metric is the angular diversity of links covering a cell — links from different directions provide more independent information.

**2D GDOP formula per cell:**
```
For a cell at position P:
1. Collect all links (TX_i → RX_i) where P is within the first 3 Fresnel zones of that link
   (i.e., ΔL = |P-TX_i| + |P-RX_i| - |TX_i - RX_i| ≤ 3·λ/2)
2. If fewer than 2 qualifying links: GDOP = Infinity (gray cell, no coverage)
3. For each qualifying link i: θ_i = atan2(RX_i.y - TX_i.y, RX_i.x - TX_i.x)  (projected to floor plane)
4. Build the 2×2 Fisher information matrix:
   F = Σ_i [ [cos²(θ_i),       cos(θ_i)·sin(θ_i)],
              [cos(θ_i)·sin(θ_i), sin²(θ_i)      ] ]
5. det_F = F[0][0]·F[1][1] - F[0][1]·F[1][0]
6. If det_F ≤ 1e-6: GDOP = Infinity (collinear links — degenerate geometry)
7. trace_Finv = (F[0][0] + F[1][1]) / det_F  (trace of F^-1 using 2×2 inverse formula)
8. GDOP = sqrt(trace_Finv)
```

Thresholds: GDOP < 2 = excellent (green), 2–4 = good (yellow), >4 = poor (red), Infinity = gray.

**Coverage score:** Fraction of floor cells with GDOP < 4, expressed as a percentage (0–100%).

**Implementation (Web Worker):**
- Input: `{grid: {width, height, cell_m: 0.2, origin: [x, y]}, links: [{tx: [x,y,z], rx: [x,y,z]}, ...], lambda: 0.123}`
- Output: `Float32Array` of GDOP values indexed as `[col + row × width]`. Infinity encoded as 9999.
- Computation: nested loop over grid cells × links, O(cells × links). For 50×50 cells and 28 links: ~70,000 iterations = <2 ms.
- Main thread creates `THREE.DataTexture(output, width, height, THREE.RedFormat, THREE.FloatType)` and applies a shader with color mapping (green→yellow→red gradient, gray for GDOP=9999).
- Update trigger: send message to Worker on every `requestAnimationFrame` during node drag. Worker responds within one frame. No throttling needed given <2 ms compute time.

**Coverage painting during node placement:**
- When a node is being dragged via TransformControls, the link geometry changes on every frame. The Web Worker recomputes GDOP on every animation frame during drag.
- The GDOP overlay updates live — dead zones visibly shrink as the node moves into a good position
- A "coverage score" percentage is displayed in a HUD element: "Coverage: 78% ↑3%" (arrow shows improvement since drag started)
- Color legend shown in corner: green excellent / yellow good / red poor / gray no coverage

**Virtual node planning:**
- "Add Virtual Node" button places a phantom node (wireframe, dashed links) that participates in GDOP computation but doesn't correspond to real hardware
- User drags the virtual node around to find the optimal position for their next purchase
- Virtual nodes are visually distinct (translucent, pulsing outline) and can be converted to real nodes via onboarding

### 12. Self-Healing Fleet

When a node goes offline, the fleet manager automatically re-optimizes roles among remaining nodes to maintain the best possible coverage, rather than simply degrading silently.

**Healing sequence:**
1. Node WebSocket disconnects → fleet manager marks it OFFLINE
2. Recompute GDOP across the floor grid with the reduced node set
3. Select optimal TX/RX role assignments among remaining nodes to minimize worst-case GDOP
4. If passive radar mode is active, check if remaining RX nodes still have adequate geometric diversity against the AP
5. Adjust packet rates upward on remaining TX nodes to compensate for lost link density
6. Push new role configs to remaining nodes via their WebSocket connections
7. Dashboard shows a before/after coverage comparison overlay: "Node kitchen-ceiling went offline. Coverage in kitchen corridor degraded from excellent to fair. 4 links lost, 8 remaining."

**Recovery:**
- When a node reconnects (WebSocket `hello`), roles are re-optimized again to restore full coverage
- Dashboard shows: "Node kitchen-ceiling back online. Full coverage restored."
- Node automatically receives its new role assignment on reconnect

**Graceful degradation guarantees:**
- 1 node lost from a 6-node fleet: coverage degrades but system remains functional
- 2 nodes lost: system warns "significant coverage gaps" with affected areas highlighted in red in the 3D view
- >50% fleet offline: system enters degraded mode, disables spatial localization, falls back to per-link presence detection only

### 13. Room Transition Portals

Portals are vertical planes drawn across doorways in the 3D editor. They track directional blob crossings to maintain per-room occupancy counts.

**Portal definition:**
- In setup mode, user draws a vertical rectangle across a doorway by clicking two floor points (the portal spans floor to ceiling)
- Each portal connects two named zones (e.g., "Hallway" ↔ "Kitchen")
- Portals are rendered as translucent colored rectangles in the 3D view

**Portal plane representation:** Each portal is stored as two floor points `[P1, P2]`. The portal plane's normal vector `n` = normalize(cross([P2-P1, 0, 0], [0, 1, 0]))` (horizontal normal, pointing from zone_a to zone_b). The plane equation: `f(P) = dot(P - portal_midpoint, n)`.

**Crossing detection (two-phase, committed-only):**

Phase 1 — Tentative crossing:
- For each tracked blob, evaluate `f(blob_pos)` every fusion tick (10 Hz)
- Sign change detected: `prev_sign != sign(f(blob_pos))`
- Minimum velocity check: `dot(blob_velocity, n)` must be > 0.1 m/s in the crossing direction (prevents jitter near portal from static blob repositioning)
- On sign change + velocity check: record a tentative crossing in memory (direction, timestamp, blob_id). Do NOT yet update occupancy counts.

Phase 2 — Committed crossing:
- Committed when blob is > 0.3 m past the portal plane on the new side for >2 s (dwell confirmation)
- On commit: insert into `portal_crossings` table; update zone occupancy counts
- Reversal window: if blob returns through the portal within 5 s of a tentative crossing (before commit), cancel the tentative crossing. Log: "Blob #N passed tentatively through [portal] but returned."
- Counts are bounded: minimum 0 (committed crossing out of an empty room sets count to 0, not -1)

**Occupancy tracking and restart reconciliation:**
- Per-zone occupancy count maintained in memory: `{zone_id: count}`
- Persisted to `zones.last_known_occupancy` column in SQLite every 60 s and on graceful shutdown
- On mothership restart:
  1. Load `last_known_occupancy` as the starting value for each zone (marked "uncertain")
  2. Compute the net portal crossings since midnight from the `portal_crossings` table: `SELECT direction, zone_a_id, zone_b_id FROM portal_crossings WHERE timestamp_ms > midnight_ms ORDER BY timestamp_ms`
  3. Apply net crossings to the loaded starting values → reconstructed occupancy
  4. Mark occupancy as "reconciled" after 60 s of live operation
- Dashboard shows "Occupancy estimates restored after restart (may be stale)" until next reconciliation
- 60-second reconciliation: every 60 s, compare portal-based occupancy with blob-count-per-zone. If they differ by >1 for 2 consecutive checks, apply the blob-count-per-zone as ground truth and log the discrepancy
- Dashboard shows zone labels in the 3D view with occupancy badges: "Kitchen: 2", "Bedroom: 0"
- Zone occupancy published via the dashboard WebSocket and exposed via REST API

**Portal flash animation:** When a crossing is detected, the portal rectangle briefly flashes and an arrow appears showing the direction of travel.

**Home Assistant integration:** Zone occupancy exposed as sensor entities via optional MQTT client auto-discovery: `sensor.spaxel_kitchen_occupancy`.

### 14. Time-Travel Debugging

The mothership continuously records raw CSI frames to a circular buffer, enabling historical replay with adjustable algorithm parameters.

**Recording:**
- All incoming CSI binary frames are written to a recording store (append-only file or SQLite blob table) with mothership timestamps
- Default retention: 48 hours. Configurable via dashboard settings
- Storage estimate: ~150 bytes/frame × 30 Hz × 20 links = ~7.5 MB/hour = ~360 MB/48h for an 8-node fleet
- Oldest data evicted automatically when retention limit reached

**Replay engine architecture:**

Replay runs as a **separate, isolated pipeline instance** — it does not interfere with live operation. The live fusion loop continues at 10 Hz regardless of replay state.

When `POST /api/replay/start {from_iso8601, to_iso8601}` is called:
1. A new `replay_sessions` row is created with `state='paused'`
2. A dedicated goroutine (`replayWorker`) is spawned for this session
3. The worker seeks to `from_ms` in `csi_replay.bin` by binary-searching `recv_time_ms` fields (linear scan forward from `oldest_pos`; replay seeks are infrequent enough that O(N) is acceptable for the initial seek; subsequent seeks within a session are O(1) for forward seeks and O(N/2) average for backward seeks)

**Seek algorithm in `csi_replay.bin`:**
```
To seek to target_ms:
  1. Start from oldest_pos (guaranteed to exist)
  2. Read frames sequentially, comparing recv_time_ms to target_ms
  3. When recv_time_ms >= target_ms: stop; current file position is the replay cursor
  4. Store cursor in replay_sessions.current_ms
  5. For frame-by-frame mode: advance cursor by exactly one frame
```

**Playback at N× speed:**
- The `replayWorker` reads frames from the file in recv_time_ms order
- Real-time delta between consecutive frames: `real_dt = frame[i+1].recv_time_ms - frame[i].recv_time_ms`
- Worker sleeps `real_dt / speed` ms between pushing each frame through the replay pipeline
- The replay pipeline is a copy of the live signal processing pipeline with the session's `params_json` applied (or live params if `params_json = NULL`)
- Output: blob list from the replay pipeline is pushed to `replay_results` channel (in-memory, not SQLite)
- Dashboard receives replay blobs via the dashboard WebSocket: when a session is playing, the mothership interleaves `{"replay":true, "blobs":[...], "timestamp_ms":N}` frames into the dashboard feed alongside live frames. The frontend distinguishes replay frames by the `replay:true` flag and renders them in the 3D scene's replay layer

**Seek (`POST /api/replay/seek {session_id, timestamp_iso8601}`):**
- Pauses playback; re-runs the seek algorithm; updates `replay_sessions.current_ms`; sends a single frame to the dashboard at the seeked timestamp (one-shot replay tick)

**`PATCH /api/replay/params` (parameter change):**
- Updates `params_json` in the session row
- Triggers a replay "batch re-run": replays the current 60-second window around `current_ms` at maximum speed (no real-time delay), computes blobs, sends results to the dashboard as a burst. This gives the "instant preview" effect.

**Dashboard toolbar: "Pause Live":**
- Clicking "Pause Live" calls `POST /api/replay/start` with `from=now-60s` and `to=now`. The dashboard freezes the live blob render and switches to replay mode. The 3D scene shows the state 60 seconds ago. The user can then scrub backward via the timeline.

- Dashboard toolbar: "Pause Live" button freezes the 3D view and reveals a timeline scrubber
- Scrub backward/forward through recorded history. Playback at 1×, 2×, 5×, or frame-by-frame
- The 3D scene renders blobs exactly as they were detected at that point in time, including trails

**Parameter tuning overlay:**
- While in replay mode, a tuning panel exposes key pipeline parameters as sliders:
  - Detection threshold (deltaRMS)
  - Baseline time constant (τ)
  - Fresnel weight decay rate
  - Subcarrier selection count
  - Breathing band sensitivity
- Adjusting any slider re-runs the pipeline on the recorded CSI data with the new parameters
- The 3D view immediately shows how detection would have differed — missed blobs appear, false positives disappear
- "Apply to Live" button writes the tuned parameters to the running pipeline

**Use cases:**
- "Why did it miss me standing in the kitchen at 2pm?" — scrub back, lower stillness threshold, see the blob appear
- "Why do I get false positives at 3am?" — scrub to the event, raise the detection threshold until it disappears, check if real events are still caught
- Debug new node placements by replaying the first hour of data with different parameters

### 15. Diurnal Adaptive Baseline

Instead of a single EMA baseline per link, maintain a 24-slot circular buffer — one baseline vector per hour of day. This captures predictable environmental periodicity that a simple EMA cannot distinguish from human presence.

**Sources of diurnal variation:**
- HVAC cycling (on/off at scheduled times)
- Sunlight heating window glass and walls (changes propagation characteristics)
- Appliance EMI patterns (refrigerator compressor, washing machine)
- Household RF environment (neighbor's devices, microwave ovens)

**Learning phase:**
- For the first 7 days, the system builds per-hour baselines by accumulating motion-free CSI data into hourly slots
- Dashboard shows a "baseline confidence" indicator per link that fills up as each hourly slot accumulates sufficient samples (minimum 300 samples = 5 minutes of quiet data per slot)
- Slots that haven't been calibrated fall back to the global EMA baseline

**Steady state:**

On each fusion tick (10 Hz), the active baseline for a link is computed as a weighted blend:

```
hour = current_hour_of_day (0–23, in configured TZ)
diurnal_slot = diurnal_baselines[link_id][hour]

// Use diurnal slot only if it has enough samples
if diurnal_slot.sample_count >= 300:
  // Crossfade: blend over the first 15 minutes of the new hour
  // minute_of_hour in [0, 60), crossfade completes at minute 15
  t = min(1.0, minute_of_hour / 15.0)  // linear 0→1 over 15 min
  active_baseline[k] = (1-t) × ema_baseline[k] + t × diurnal_slot.amplitude[k]
else:
  // Slot not ready — use global EMA
  active_baseline[k] = ema_baseline[k]
```

The `active_baseline[k]` computed above replaces `baseline[k]` in the deltaRMS formula during this tick. The global EMA baseline continues to update in parallel (motion-gated, τ = 30 s) regardless of whether the diurnal slot is active — it serves as the fallback for any uncalibrated slot.

When crossfade weight `t` reaches 1.0 (after 15 minutes), the diurnal slot becomes the sole baseline for the current hour. Motion-gated EMA updates during this hour also update `diurnal_slot.amplitude[k]` (in addition to the global EMA), improving the slot over time.

- Motion-gated updates continue within each hourly slot — the diurnal baseline improves over time
- Dramatically reduces false positives from predictable environmental changes

**Storage:** 24 slots × N_subcarrier complex values × N_links. For a typical 8-node fleet with 28 links: 24 × 64 × 2 × 28 = ~86 KB per link set. Negligible.

**Dashboard visualization:** A 24-hour polar chart per link showing baseline amplitude variance by hour — spikes indicate noisy hours. Helps users understand their environment.

### 16. Fall Detection

Detects falls by monitoring blob Z-axis trajectory and post-fall stillness. Designed for elderly or at-risk household members.

**Detection algorithm:**
1. Track blob Z-coordinate velocity via the Kalman filter's state estimate
2. **Trigger condition:** Z velocity exceeds −1.5 m/s (rapid descent) AND blob Z drops below 0.5 m within 1 second
3. **Confirmation:** Blob remains below 0.5 m with low motion (deltaRMS below stillness threshold) for >10 seconds — person hasn't gotten back up
4. **Alert fired** after confirmation window

**Alert chain (configurable):**
1. Dashboard alarm — 3D view highlights the blob in red with pulsing animation, audible alert
2. Webhook — POST to configurable URL (e.g., Home Assistant automation)
3. Push notification — via Ntfy, Pushover, or Gotify (user configures endpoint)
4. Escalation — if no manual dismissal within 5 minutes, fire a secondary webhook (e.g., send SMS via Twilio, notify emergency contact)

**False positive management:**
- Requiring the combination of rapid descent + sustained stillness + low Z is physiologically specific to falls
- "Lying on the couch" doesn't trigger because there's no rapid descent event
- "Picking something up from the floor" doesn't trigger because the person rises within the 10 s confirmation window
- User can dismiss an alert from the dashboard, which logs the event for tuning
- Sensitivity adjustable: confirmation window, Z thresholds, velocity threshold

**False negative cases (accepted limitations):**
- Falling onto a mattress directly on the floor (Z_surface ≈ 0) — the Z drop may be less than the velocity threshold if the person was already seated or crouching
- Falling in a zone with no nodes above 1.5 m — Z resolution is insufficient to detect the rapid descent
- Very slow falls (elderly person slowly sliding down a wall) — velocity may be below the 1.5 m/s threshold
- Falling in a chair (landing height ~0.5 m) — may not clear the Z < 0.5 m threshold

**Mitigation for false negatives:**
- **Zone type metadata:** Each zone can be marked with a type (default: `general`; options: `bedroom`, `bathroom`, `living`, `exercise`, `kitchen`). Bedroom zones automatically suppress fall alerts during typical sleep hours (21:00–07:00) to avoid waking-up-from-bed false positives. Non-bedroom zones do not suppress.
- **At-risk mode:** A per-zone option reduces the velocity threshold to −0.8 m/s and the confirmation window to 5 s. Intended for zones where an at-risk person spends most of their time.
- **Peak velocity detection:** The algorithm examines the Kalman filter's estimated Z velocity at each time step for the 3 seconds leading up to the low-Z event. If the peak downward velocity in this window exceeds the threshold (even if instantaneous velocity at detection time is lower), it's treated as a fall trigger.
- **Manual report:** The dashboard "I fell" button allows users to manually report a missed fall, which is logged and used to tune thresholds.
- **Hardware advisory:** The system checks if fewer than 2 nodes are placed above 1.5 m in the zone where fall detection is enabled. If so, it shows a persistent warning: "Fall detection in this zone requires at least 2 nodes above 1.5 m for reliable Z-axis resolution."

**Zone type stored in the zones table:** Add `zone_type TEXT NOT NULL DEFAULT 'general' CHECK (zone_type IN ('general','bedroom','bathroom','living','exercise','kitchen','office','entry'))` column to the `zones` table.

**Requirements:** Mixed-height node placement is essential for Z-axis resolution. Minimum 2 nodes at >1.5 m height and 2 at <0.5 m for reliable fall detection. The dashboard warns when this requirement is not met in zones where fall detection is enabled.

### 17. Pre-Deployment Simulator

Before purchasing hardware, users can define their space in the 3D editor, place virtual nodes, and run a physics-based simulation to see expected detection quality.

**Space definition:**
- Same 3D editor used for real setup — draw room boxes, set dimensions, add doorways
- Place virtual nodes (visually distinct ghost meshes) at candidate positions
- Optionally add wall segments with material properties (drywall, concrete, glass) that affect signal attenuation

**Simulation engine:**
- Simplified ray-based propagation model: direct path + first-order reflections off walls and floor/ceiling
- Compute expected CSI amplitude and phase for each virtual link
- Apply the same Fresnel zone localization algorithm used in live mode
- Generate synthetic "walkers" — virtual people that move along user-defined paths or random walk patterns

**Visualization:**
- GDOP overlay shows expected detection quality across the floor
- Simulated blobs track the virtual walkers, showing expected accuracy at each position
- Coverage gaps highlighted in red
- "Add another node here" suggestions based on worst-GDOP positions

**Outputs:**
- Minimum node count recommendation for the defined space
- Optimal positions for N nodes (greedy GDOP optimization)
- Expected accuracy estimate at each point in the space
- "Shopping list" — how many ESP32-S3 boards to buy

**Propagation model (quantified):**

The simulator computes expected received signal power for each TX→RX link at each walker position using a two-ray model (direct + single-bounce) in 2D.

```
Path loss model (log-distance):
  PL(d) = PL_0 + 10·n·log10(d/d_0)   [dB]
  PL_0 = 40 dB at d_0 = 1 m (free-space reference)
  n = 2.0 (free space, no walls between TX and RX)

Wall penetration loss (additive, per wall crossed on the direct path):
  Material         Loss (dB)
  Drywall / wood   3
  Brick / concrete 10
  Glass            2
  Metal            20
Default material when none specified: drywall (3 dB)

First-order reflection (single bounce off a flat wall segment):
  Reflection coefficient: R = 0.3 (power; dimensionless)
  Reflected path length: d_refl = |TX-P_reflect| + |P_reflect-RX|
  where P_reflect is the specular reflection point on the wall segment
  Power of reflected ray: P_refl = P_direct × R × PL(d_refl) / PL(|TX-RX|)
  Only the strongest reflected ray is retained (weakest wall absorption material first)

Combined signal amplitude at walker position W:
  amplitude(W) = sqrt(P_direct(W) + P_refl(W))   (coherent sum approximation)

Simulated CSI phase at W:
  phase_k(W) = 2π × k × Δf × (d_direct(W) / c)  for subcarrier k
  where Δf = 312.5 kHz (HT20 subcarrier spacing), c = 3×10⁸ m/s
  (single subcarrier phase model; sufficient for GDOP and presence simulation)

deltaRMS_sim(W) = |amplitude(W) - amplitude(empty_room)| / amplitude(empty_room)
  (simulated signal change from "walker present" vs "empty room")
```

Walker motion model:
- Path-following mode: user draws a polyline in the 3D editor; walker follows at constant speed (default 1.0 m/s)
- Random-walk mode: walker moves with Gaussian velocity updates (σ = 0.5 m/s per axis per step), reflected off room walls
- Step interval: 100 ms (matches live 10 Hz fusion rate)
- When multiple walkers are present: each walker's amplitude contribution is summed (incoherent power addition)

**Implementation:** Reuses the same Fresnel/GDOP math from coverage painting (Component 11) and the same localization algorithm from the fusion engine (Component 4). The propagation model is the only new code — a simplified 2D ray tracer with the wall-penetration table above.

### 18. Spatial Automation Builder

Visual automation system where trigger conditions are defined as 3D volumes in the scene, wired to actions.

**Trigger volumes:**
- In setup mode, user draws 3D boxes (or cylinders) in the scene using TransformControls
- Each volume is named and assigned a condition:
  - **Enter:** blob crosses into the volume
  - **Leave:** blob exits the volume
  - **Dwell:** blob remains inside for ≥ N seconds (configurable)
  - **Vacant:** no blobs inside for ≥ N seconds
  - **Count:** number of blobs inside crosses a threshold (e.g., ≥ 2 people in living room)
- Optional time constraint: "only between 22:00 and 06:00"

**Actions:**
- **Webhook:** POST/GET to configurable URL with JSON payload containing event details
- **MQTT publish:** To user's external broker (e.g., Home Assistant)
- **Internal:** Trigger re-baseline, change node roles, enable/disable fall detection for a zone

**Visual feedback:**
- Trigger volumes rendered as translucent colored shapes in the 3D live view
- When a condition is active, the volume pulses or changes color (e.g., green idle → amber triggered)
- Event log sidebar: "14:32:05 — Blob #2 entered 'Living Room Couch' zone, dwell timer started (30s)"

**Example automations:**
- "Dwell in hallway entrance for 0s → fire `person_home` webhook" (arrival detection)
- "Vacant in all zones for 10 min → fire `house_empty`" (departure detection)
- "Enter bedroom + time 22:00–06:00 → fire `goodnight` scene"
- "Count ≥ 2 in dining room + dwell 5 min → fire `dinner_started`"

**Point-in-volume test:**
- Box volume: `inside = (x ≥ v.x AND x < v.x+v.w) AND (y ≥ v.y AND y < v.y+v.d) AND (z ≥ v.z AND z < v.z+v.h)` (axis-aligned bounding box; all comparisons in meters)
- Cylinder volume: `inside = sqrt((x-v.cx)²+(y-v.cy)²) < v.r AND z ≥ v.z AND z < v.z+v.h`
- `shape_json` fields: box = `{type:"box",x,y,z,w,d,h}`; cylinder = `{type:"cylinder",cx,cy,z,r,h}`

**Per-trigger state machine (evaluated at 10 Hz):**

```
For each enabled trigger T, for each tracked blob B (filtered by T.person if set):
  inside = point_in_volume(B.pos, T.shape)
  prev_inside = last tick's inside value for (T.id, B.id)

  ENTER  condition: fires once on transition prev_inside=false → inside=true
  LEAVE  condition: fires once on transition prev_inside=true  → inside=false
  DWELL  condition: inside=true for ≥ T.duration_s continuously (timer per (T.id, B.id))
           - timer starts when blob enters; resets when blob exits
           - fires exactly once per entry; re-fires after blob leaves and re-enters
  VACANT condition: no blob inside for ≥ T.duration_s
           - timer starts when the last blob exits; fires when timer expires
           - cancelled if any blob enters before the timer expires
  COUNT  condition: fires when blob count inside crosses T.count_threshold
           - fires on rising edge only (count was < threshold, now ≥ threshold)
```

**Time constraint check:** If `T.time_constraint_json` is set (`{from:"22:00",to:"06:00"}`), the trigger only fires when `current_local_time` is within the range. Overnight ranges (from > to) are handled correctly.

**Fire rate limiting:** Each trigger (T.id, B.id, condition) has a `last_fired` timestamp. Minimum re-fire interval: ENTER/LEAVE = 5 s; DWELL = 60 s (after firing, must exit and re-enter before firing again); VACANT = 60 s. This prevents double-fires from jitter at zone boundaries.

**Webhook action payload (POST to `actions_json[i].url`):**
```jsonc
{
  "trigger_id": 42,
  "trigger_name": "Couch Dwell",
  "condition": "dwell",
  "fired_at": "2024-03-15T14:32:05Z",
  "blob_id": 2,
  "person": "Alice",          // null if unidentified
  "position": {"x": 2.1, "y": 3.4, "z": 0.9},
  "zone": "Living Room",      // zone whose bounds contain the trigger volume centroid; null if none
  "dwell_s": 34               // for dwell condition: elapsed seconds; omitted for other conditions
}
```
HTTP timeout: 5 s. On timeout or 5xx: log warning, do not retry (fire-and-forget). On 4xx: log error, disable the trigger and show dashboard warning: "Webhook returned [status] — trigger disabled. Fix the URL and re-enable."

**MQTT action:** Publishes to `T.actions_json[i].topic` with the same JSON payload as the webhook (as a string). QoS 0. Requires `SPAXEL_MQTT_BROKER` to be configured.

**Evaluation:** Trigger conditions are evaluated in the mothership's fusion loop at 10 Hz — point-in-volume tests on already-tracked blob positions. Negligible computational cost.

### 19. Ambient Confidence Score & Link Weather

Continuous system-wide health monitoring that makes the RF environment legible to non-technical users.

**Per-link health metrics and composite score:**

**Link ID format:** `"TX_MAC:RX_MAC"` using uppercase colon-separated hex. For passive links (router as TX): `"AP_BSSID:NODE_MAC"`. Links are directional — TX→RX1 and TX→RX2 are separate link IDs.

**Link ID normalization (canonical form for storage):** For symmetrical links in TX/RX or TX_RX mode (where both nodes can be either TX or RX depending on role), the link_weights and link health tables use a canonical non-directional form to avoid duplicating weights for A→B and B→A. Canonical form: `min(MAC_a, MAC_b) + ":" + max(MAC_a, MAC_b)` (lexicographic sort of the two MACs). For passive links (AP as TX), the AP BSSID is always the first component (AP cannot be RX). The function `CanonicalLinkID(mac1, mac2 string) string` applies this rule consistently throughout the codebase. In-memory CSI frames use the raw `peer_mac:node_mac` (directional) form for signal processing; lookup into `link_weights` always calls `CanonicalLinkID`.

**Metric 1 — Packet Delivery Rate (PDR):**
- For active TX nodes: `PDR = received_count / (configured_rate_hz × window_s)` over a 30-second rolling window
- For passive nodes: on first connect, measure the empirical beacon arrival rate over 60 s (called "warmup"). Use the measured rate as `expected_rate`. Typical: ~10 Hz. During warmup, PDR is shown as "measuring..." in the UI.
- Gap detection: if no frames arrive for >5× expected interval, the link is immediately marked DEGRADED (zero PDR) even within the window
- PDR is reset and re-measured after a node reconnects (30-second warmup window)

**Metric 2 — SNR (Signal-to-Noise Ratio, 0–1):**
- `SNR = mean(amplitude[k]) / std(amplitude[k])` over a 10-second window, averaged over selected subcarriers
- Normalized: `SNR_norm = min(1.0, SNR / 20.0)` (SNR of 20 maps to 1.0 — a good link)

**Metric 3 — Phase Stability (0–1):**
- `phase_variance = variance(residual_phase[k])` over a 10-second window, averaged over selected subcarriers
- `phase_stability = max(0, 1.0 - phase_variance / 0.5)` (variance of 0.5 rad² maps to 0.0)

**Metric 4 — Baseline Drift (0–1, where 1 = no drift):**
- `drift = L2_distance(current_baseline_amplitude, calibration_baseline_amplitude) / num_subcarriers`
- `drift_score = max(0, 1.0 - drift / 5.0)` (drift of 5.0 amplitude units maps to 0.0)

**Composite link quality score (0–100):**
```
quality = 100 × (0.35 × PDR + 0.30 × SNR_norm + 0.25 × phase_stability + 0.10 × drift_score)
```

The dashboard shows the composite score as the link's health indicator. Hovering over a link reveals a tooltip with all 4 component bars.

**System-wide Detection Quality metric:** `mean(quality[link]) over all active links` — simple unweighted mean. Active links are those with PDR > 0 and at least one frame in the last 30 s.

**System-wide confidence:**
- Aggregate all link quality scores into a single "Detection Quality" metric: 0–100%
- Displayed as a prominent gauge/ring in the dashboard toolbar
- Thresholds: 80–100% Excellent, 60–80% Good, 40–60% Fair, <40% Poor

**3D visualization:**
- Links rendered with thickness and color proportional to their health: thick green = strong, thin red = struggling
- Nodes with all links healthy: bright green. Nodes with degraded links: amber border
- Optional "link weather map" overlay: ground-plane heatmap showing detection confidence at each point, derived from link health of links whose Fresnel zones cross that point

**Diagnostics and advice:**
- When quality drops, the system diagnoses why: "Link kitchen↔hallway degraded — possible obstruction change" or "3 links showing correlated phase drift — environmental change detected"
- Links that have been below threshold >40% of the time over the past week are flagged with specific advice: "These nodes may be too far apart or have too many walls between them. Consider adding a relay node at [highlighted position in 3D]"
- Quality trends graphed over time (24h / 7d / 30d) to identify patterns

**Anomaly detection integration (see Component 20):** Sudden quality drops across multiple links outside of normal diurnal patterns can indicate significant environmental changes and trigger re-calibration suggestions.

### 20. Anomaly Detection & Security Mode

Learns normal occupancy patterns over time and alerts on deviations. Privacy-preserving intrusion detection with no cameras.

**Pattern learning:**
- After 7+ days of operation, the system builds a statistical model of typical occupancy:
  - Per-zone, per-hour-of-day: expected occupancy (mean and variance)
  - Per-zone, per-day-of-week: weekend vs. weekday patterns
  - Typical first-detection time (morning wake-up) and last-detection time (bedtime)
  - Common transition patterns (e.g., bedroom → bathroom → kitchen in morning)
- Model stored in SQLite, updated continuously with exponential decay (recent behavior weighted more)

**Anomaly scoring:**

Each detection event is scored at event time using the learned statistical model. Score is in [0, 1]. Three component scores are combined:

```
Z-score helper: z_score(observed, mean, std) = (observed - mean) / max(std, 0.5)
  (floor std at 0.5 prevents division by zero and reduces sensitivity for small-sample slots)

[0,1] mapping: normalize(z) = min(1.0, max(0.0, (|z| - 1.0) / 3.0))
  (0 below 1σ deviation, rises to 1.0 at 4σ deviation)

Time score: Fraction of historical observations in this zone-hour-day slot that had ANY detection.
  time_score = normalize(z_score(is_active, slot_mean_active, sqrt(slot_mean_active * (1 - slot_mean_active))))
  where is_active = 1 if detected, 0 if not; slot_mean_active is the historical fraction.

Zone count score: How unusual is the blob count vs. historical?
  count_score = normalize(z_score(observed_count, slot_mean_count, sqrt(slot_variance_count)))

Zone score: Detection in a zone that is atypically occupied at this time.
  (For the primary zone of the detected blob, same formula as time_score but for that specific zone)

Composite: anomaly_score = max(0.4×time_score + 0.4×count_score + 0.2×zone_score, max(time_score, count_score))
  (takes the larger of the weighted sum and the max component — ensures individual extreme components are not hidden)
```

- Alert threshold: anomaly_score > 0.85 → fire alert (yellow if 0.6–0.84, red if ≥ 0.85). Configurable.
- Security mode: overrides scoring — any detection = score 1.0 (all motion is suspicious)
- "Vacation mode" toggle: suppresses anomaly alerts but doesn't disable monitoring or learning

**Model update rule (Welford's online algorithm applied to `anomaly_patterns` table):**

Once per hour, the mothership records an observation for each zone × day_of_week: `observed_count = blob_count_in_zone_during_this_hour`. The `anomaly_patterns` row is updated:

```
// Welford's online update for running mean and variance
// Fields: mean_count, variance (= M2/n, Bessel-corrected), sample_count

n_new = row.sample_count + 1
delta = observed_count - row.mean_count
mean_new = row.mean_count + delta / n_new
delta2 = observed_count - mean_new                    // second delta after mean update
M2_old = row.variance × row.sample_count              // un-normalize variance
M2_new = M2_old + delta × delta2                      // Welford M2 accumulator
variance_new = M2_new / n_new                         // population variance (not sample)

UPDATE anomaly_patterns SET
    mean_count = mean_new,
    variance = variance_new,
    sample_count = n_new,
    updated_at = now_ms
WHERE zone_id = ? AND hour_of_day = ? AND day_of_week = ?
```

For the `slot_mean_active` used in `time_score`: stored separately as `mean_count` (treat observed_count as binary 1/0 — was zone ever occupied during this hour). The variance column encodes `variance_count` for the count_score formula.

Update period: once per hour (not per fusion tick — avoids massive SQLite write load). The update for a given zone-hour-day slot happens at the end of each calendar hour.

- Cold start: model marked as "not ready" for the first 7 days. No anomaly alerts until the model has at least 50 observations per active slot.
- Outlier protection: model is only updated when `anomaly_score < 0.5` for the event (prevents learning from anomalous events themselves)
- Anomaly score stored in `events.detail_json` for later retrieval in explainability overlay

**Security mode:**
- User-activatable from dashboard or via automation trigger (e.g., "vacant in all zones for 10 min → enable security mode")
- In security mode, ANY detection event fires an alert (no anomaly threshold — all motion is suspicious)
- Alert chain: same as fall detection (dashboard alarm → webhook → push notification → escalation)
- "Away" mode can be automatically activated when correlated with phone geofencing (user configures via HA integration)

**Dashboard visualization:**
- Timeline view showing expected vs. actual occupancy patterns
- Anomaly events highlighted with severity color (yellow = unusual, red = highly anomalous)
- "Normal pattern" overlay in 3D view: faint blob trails showing typical movement patterns for the current hour

**Privacy:** No personally identifiable data is stored — only statistical occupancy counts and zone transition frequencies. No recording of who is where, only that someone is/isn't.

### 21. BLE Beacon Scanning & Device Registry

The ESP32-S3 has a BLE radio that runs concurrently with WiFi on the second core. Each node passively scans for BLE advertisements and reports them to the mothership, enabling person/device identification of tracked blobs.

**BLE scanning (firmware):**
- Passive BLE scan runs continuously on Core 0 (WiFi CSI runs on Core 1)
- Captures: device address, address type (public/random), RSSI, device name (if advertised), manufacturer data
- Handles rotating random addresses via heuristic matching (see below) — passive scanning cannot resolve IRK without pairing
- Reports every 5 s as JSON on the WebSocket: `{type: "ble", devices: [{addr, rssi, name, type}, ...]}`

**BLE Address Rotation Handling:**

Modern smartphones rotate their BLE random address every 15–30 minutes (iOS: ~15 min; Android: varies, often longer). Since Spaxel uses passive scanning (no pairing), IRK-based address resolution is not possible. The following heuristic algorithm is used:

*Rotation detection heuristics (applied in the mothership on received BLE reports):*

1. **Manufacturer data fingerprint:** The first 4 bytes of manufacturer data (after company ID) form a device fingerprint for Apple and Google devices. For Apple Continuity (company ID 0x004C, type 0x0F): extract the 2-byte proximity UUID. This UUID is stable across rotations for the same device in the same pairing context. Match new addresses with this fingerprint.

2. **Time + signal proximity:** When a known address disappears and a new unknown address appears at the same node within 90 seconds with similar RSSI (within 10 dBm), a rotation match is scored. Score = `0.5×manufacturer_match + 0.35×rssi_proximity + 0.15×time_gap_factor`. If score > 0.7, the new address is tentatively linked to the old device.

3. **Position continuity:** If a new BLE address appears at a node, and the closest blob is already associated with a known registered device, the new address is tentatively linked to that device's label.

4. **Merge confirmation:** After 3 consecutive reports under the new address matching the above criteria, the device is updated to the new address in the registry.

*Multi-address registry:* A `ble_device_aliases` table stores historical addresses per device:
```sql
CREATE TABLE IF NOT EXISTS ble_device_aliases (
    addr        TEXT NOT NULL,   -- the alias/rotated address
    canonical_addr TEXT NOT NULL REFERENCES ble_devices(addr) ON DELETE CASCADE,
    first_seen  INTEGER NOT NULL DEFAULT (unixepoch() * 1000),
    last_seen   INTEGER NOT NULL,
    PRIMARY KEY (addr)
);
```
This allows the mothership to recognize any historical address for a registered device, even after rotation.

*Graceful fallback when rotation is unresolved:* If no rotation match is found within 5 minutes of the known address disappearing, the blob that was associated with that device retains its identity label for an additional 5 minutes (estimated persistence) before reverting to "Unknown". This prevents a 15-second lapse in identity during normal rotation.

*Practical recommendation:* For the most reliable person identification, use a dedicated BLE tracker tag (e.g., Tile, Samsung SmartTag, or an AirTag-compatible tag) rather than a phone. Tracker tags typically have stable addresses with rotation periods measured in hours or not at all.

*User-facing indicator:* Devices with the `auto_rotate` flag set show a rotation icon in the BLE device registry. Hovering shows: "This device uses rotating addresses. Identity may lapse briefly (~60s) during rotation."

**Device registry (SQLite):**

| Column | Content |
|--------|---------|
| BLE address | Hardware or resolved address (primary key) |
| Label | User-assigned name: "Alice", "Bob's Watch", "Dog Tracker", "Car Keys Tag" |
| Type | `person` / `pet` / `object` — affects how the blob is rendered and tracked |
| Color | User-chosen color for the 3D figure/marker |
| Icon | Optional icon (for simple mode cards) |
| First seen | Timestamp |
| Last seen | Timestamp |
| Auto-rotate | Whether this device uses rotating addresses (detected automatically) |

**Discovery & registration flow:**
1. Dashboard shows a "People & Devices" panel listing all BLE devices seen by any node in the fleet
2. Unregistered devices appear in a "Discovered" list, sorted by frequency of sighting (household devices will be near the top)
3. User taps a device → assigns a label, type, and color → device is registered
4. Common devices are identified automatically: "iPhone", "Apple Watch", "Fitbit", "Tile" from manufacturer data
5. User can also pre-register by BLE address if they know it (e.g., from a tracker tag's settings app)

**Blob-to-device matching:**

Run once per fusion tick (10 Hz) for each registered BLE device currently visible (seen in the last 10 s by at least one node).

```
For each registered device D (addr or alias in ble_devices / ble_device_aliases):
  1. Collect RSSI reports: {node_i → rssi_i} from the last BLE scan batch (≤5 s old per node)

  2. Estimate device position using the BLE centroid formula (Component 22):
     pos_ble = RSSI-weighted centroid of reporting nodes
     ble_confidence = min(1.0, (K-1)/3.0) where K = reporting node count

  3. If ble_confidence < 0.33 (only 1 node reporting): assign to the nearest blob
     to that single node's position, IF distance < 3.0 m. Otherwise: no match.

  4. If ble_confidence >= 0.33: find the blob nearest to pos_ble:
     nearest_blob = argmin_blob { |blob.pos - pos_ble| }
     match_distance = |nearest_blob.pos - pos_ble|

  5. Match threshold: accept match if match_distance < max(1.5, 2.0 × ble_confidence_m)
     where ble_confidence_m = estimated BLE position uncertainty in meters:
       ble_confidence_m = 3.0 / (K × 0.5)  (heuristic: improves with more nodes)
     Typical: K=2 → accept within 3.0 m; K=4 → accept within 1.5 m

  6. Conflict resolution (two devices match the same blob):
     - Compute match_score = (1.0 - match_distance/3.0) × ble_confidence for each device
     - The device with the higher match_score wins; the other is unmatched this tick
     - Unmatched device retains its previous blob assignment for up to 5 s (identity persistence)

  7. On successful match: assign device.label, device.color, device.type to blob
     If device.type == 'person' AND multiple devices for same person:
       Use the device with the highest match_score for that person
```

**Identity persistence:** When a blob's BLE device was matched in the last tick but is not matched this tick (device went quiet or rotated address), the identity label is retained for up to 5 s (50 ticks). After 5 s without a fresh match, the label is cleared.

- Multiple devices can map to the same person (Alice's phone + Alice's watch both resolve to "Alice")

**Privacy considerations:**
- BLE scanning is local only — no data leaves the mothership
- Users control which devices are tracked — unregistered devices are ignored for identity matching
- "Visitor" devices (seen briefly, never registered) are not stored beyond the 5 s scan window

### 22. Self-Improving Localization via BLE Ground Truth

BLE person identification (Component 21) creates continuous, automatic ground truth that drives a feedback loop to improve CSI localization accuracy over time.

**The feedback loop:**
1. BLE RSSI from multiple nodes estimates a device's approximate position (RSSI-weighted centroid — see formula below)
2. The CSI localizer independently estimates the blob's position via Fresnel zone fusion
3. The discrepancy between BLE-estimated position and CSI-estimated position is the error signal
4. A gradient update adjusts per-link Fresnel zone weights: links whose Fresnel zones correctly predicted the BLE-confirmed position get reinforced, misleading links get dampened

**BLE position estimation formula:**

For a registered device currently seen by K nodes (K ≥ 2), with each node i reporting RSSI `rssi_i` (dBm):

```
// Convert RSSI to linear power weight (avoids negative weights from dBm)
// Shift so that the best possible RSSI (−30 dBm) maps to weight 1.0
weight_i = pow(10.0, (rssi_i - (-30.0)) / 10.0)  // = 10^((rssi_i+30)/10)

// Example: rssi=-50 → weight=0.01; rssi=-70 → weight=0.0001
// Clamp minimum weight: if weight_i < 1e-6, exclude that node from the sum

pos_ble.x = sum(weight_i × node_i.pos_x) / sum(weight_i)
pos_ble.y = sum(weight_i × node_i.pos_y) / sum(weight_i)
// Z-axis: use average of K nodes' Z positions weighted by weight_i (coarser estimate)
pos_ble.z = sum(weight_i × node_i.pos_z) / sum(weight_i)

// Confidence of BLE estimate:
ble_confidence = min(1.0, (K - 1) / 3.0)  // 0 for K=1, 0.33 for K=2, 0.67 for K=3, 1.0 for K≥4
```

The BLE position estimate is only used as ground truth when `ble_confidence ≥ 0.33` (at least 2 reporting nodes) AND `|pos_ble - pos_csi| < 2.0 m` (outlier rejection — large discrepancies indicate a BLE-to-blob mismatch, not a localization error). When either condition fails, the frame is skipped (no weight update).

**Weight update rule:**
- Per-link weight vector `w[link]` (initialized to 1.0, range [0.1, 3.0])
- Each frame where BLE estimate is valid: for each link active in this fusion tick (deltaRMS > threshold):

```
  pos_error = |pos_csi - pos_ble|  // Euclidean distance in meters
  // fresnel_contribution = fraction of the blob's Fresnel accumulation weight from this link
  fresnel_contribution[link] = (link.deltaRMS × zone_decay) / total_accumulated_weight

  // positive update if prediction is correct (small error), negative if not
  error_signal = 1.0 - min(1.0, pos_error / 1.0)  // 1.0 at 0m error, 0.0 at ≥1m error
  w[link] += α × (error_signal - 0.5) × fresnel_contribution[link]
  // note: (error_signal - 0.5) is positive when error < 0.5m, negative when error > 0.5m
```

- α = 0.001 (very slow learning rate to prevent instability)
- Weights clamped to [0.1, 3.0] range
- Stored in SQLite, restored on restart
- Update runs at most once per second (throttled) regardless of fusion rate

**What improves:**
- Links obscured by unexpected reflections get dampened automatically
- Links with clean Fresnel geometry get amplified
- The system adapts to the specific RF environment (wall materials, furniture layout) without manual tuning
- After 2-4 weeks of BLE-carrying occupants, localization accuracy measurably improves

**Dashboard:**
- "Accuracy Trend" graph showing median localization error (BLE-vs-CSI discrepancy) over time — should show a downward curve
- Per-link weight visualization: link thickness in 3D view reflects learned weight (thicker = more trusted)
- Reset button to reinitialize all weights to 1.0 (useful after major furniture changes)

### 23. Presence Prediction & Pre-emptive Automation

Learns per-person temporal patterns (requires BLE person identification) and predicts zone transitions 5–30 minutes in advance.

**Pattern model:**

For each person × zone × time_slot × day_type, the model stores `probability` = P(person is in this zone during this time slot on this day type). This is a marginal presence probability, not a transition probability — simpler to compute and sufficient for the 15-minute horizon predictions used.

- **Time slot index:** `slot = (hour × 60 + minute_of_day) / 15`, range 0–95 (96 per day)
- **Day type:** `weekday` (Mon–Fri) or `weekend` (Sat–Sun), determined using the `TZ` timezone
- **Model state:** stored in `prediction_models` table: `probability`, `sample_count` per (person, zone_id, slot, day_type)

**Model update rule:**
- Observation: every 5 minutes, for each person with a known current zone, record an observation: `obs[zone] = 1.0`, `obs[other_zones] = 0.0` for the current time slot and day_type
- EMA update: `p_new = p_old + α × (obs - p_old)` where `α = 0.03` (≈ 1/33 observations ≈ 14-day half-life at ~2.5 observations/day/slot)
- `sample_count += 1` (used for cold-start gating)
- Updates are processed in a background goroutine every 5 minutes (not in the fusion loop)
- Also applied retroactively on restart: the last 24h of zone events from the events table are replayed to recover model state lost during downtime

**Cold start:**
- A slot is "ready" when `sample_count ≥ 3` (at least 3 observations across ≥ 3 different days)
- "Days complete" for the dashboard progress indicator: count distinct calendar dates with observations for this person
- Before 7 days / 3 observations per slot: dashboard shows "Learning Alice's patterns... 4/7 days complete"; no `predicted_enter` triggers fire

**Prediction engine:**
- Every 60 s: for each person with a current zone assignment, compute predictions for horizons 5, 15, 30 min
- Look up `probability[zone_id][current_slot + H/15][day_type]` for each zone
- Normalize probabilities over all zones to sum to 1.0
- Only output predictions for zones with probability > 0.5 (suppress low-confidence outputs)
- "Alice: 87% Kitchen by 7:20, 12% Bathroom, 1% other" — only Kitchen (0.87) and Bathroom (0.12) shown; "other" is the remainder
- Predictions recalculated every 60 s

**`predicted_enter` trigger:**
- Fires when `P(person in zone Z at T+H) > 0.6` AND previous computation had `P < 0.6` (rising edge only)
- Suppressed: once fired, does not re-fire for the same (person, zone, slot) within 60 minutes
- Configurable: threshold (default 0.6), horizon H in minutes (5, 15, or 30)

**Accuracy tracking:**
- Every actual zone entry is compared against the prediction for that (person, zone, slot) made 15 minutes earlier
- Hit: actual zone matched the predicted top-1 zone. Miss: it didn't.
- Rolling 30-day accuracy: `hits / (hits + misses)`
- Displayed in dashboard: "Alice's predictions: 78% accurate at 15-min horizon (last 30 days)"

**Exposed as:**
- **Dashboard widget:** "Predicted Next 30 min" panel showing per-person expected zone with confidence bars
- **REST API:** `GET /api/predictions?person=alice&horizon=30m` → JSON probability distribution
- **Automation triggers (Component 18 extension):** New trigger type `predicted_enter` — fires N minutes *before* the predicted zone entry. Example: "5 min before Alice's predicted Kitchen entry → POST to kettle webhook"
- **Home Assistant sensors:** `sensor.spaxel_alice_predicted_zone`, `sensor.spaxel_alice_prediction_confidence`

**Cold start:** Predictions require 7+ days of data per person. During the learning phase, the dashboard shows "Learning Alice's patterns... 4/7 days complete" with a progress indicator.

### 24. Adaptive Sensing Rate with Edge Filtering

Dynamically adjusts CSI capture rate per link based on activity, reducing bandwidth by 90%+ during idle periods.

**Two-tier sensing:**

| State | CSI Rate | Bandwidth | Behavior |
|-------|----------|-----------|----------|
| Idle | 2 Hz | ~600 B/s per link | On-device amplitude variance check. If variance > threshold → switch to Active |
| Active | 20–50 Hz | ~6–15 KB/s per link | Full CSI streaming to mothership. If no motion for 10 s → switch to Idle |

**Mothership-controlled rate changes:**
- Mothership sends `{type: "config", rate: 50}` or `{type: "config", rate: 2}` on the WebSocket
- Rate decisions based on: per-link deltaRMS, adjacent-zone activity (if motion in kitchen, preemptively ramp hallway links), prediction engine output (ramp before predicted arrivals)

**On-device edge filtering (ESP32):**
- At idle rate (2 Hz), firmware computes amplitude variance over last 5 samples (~20 lines of C)
- If variance exceeds a configurable threshold: immediately ramp to full rate, send a `{type: "motion_hint"}` JSON to the mothership
- Mothership uses motion hints to ramp adjacent links preemptively

**Fleet-level coordination:**
- When all zones are idle, designate one "sentinel" link per zone at 5 Hz; all others drop to 1 Hz
- When activity detected in one zone, ramp that zone to full rate + adjacent zones to 5 Hz
- Prediction engine can preemptively ramp zones before predicted arrivals

**Benefits:**
- 8-node fleet idle: ~4.8 KB/s total (vs ~120 KB/s at full rate)
- Battery-powered nodes become viable (deep sleep between 2 Hz samples)
- Mothership CPU load drops proportionally during quiet hours
- Scales to larger fleets without linear bandwidth growth

### 25. Sleep Quality Monitoring

Analyzes breathing band and motion data in bedroom zones during nighttime hours to produce a daily sleep quality report.

**Activation:** Automatic when a bedroom zone (user-designated) has been occupied with low motion for >15 minutes during configured nighttime hours (default 21:00–09:00).

**Sleep state machine (per-zone, per-person-or-zone-occupant):**

```
INACTIVE
  └─▶ (zone first becomes occupied during nighttime window)
        └─▶ IN_BED
              bed_time = now
              onset_latency timer starts

IN_BED
  └─▶ (smooth_deltaRMS < 0.03 for 5 consecutive minutes)
        └─▶ ASLEEP
              onset_latency_min = now - bed_time
              restless_event_count = 0; breathing_rate_samples = []

ASLEEP
  ├─▶ (smooth_deltaRMS > 0.08 for > 30 s but zone is still occupied)
  │     └─▶ RESTLESS (transient)
  │           restless_event_count += 1
  │           duration of restless episode tracked (< 5 min: returns to ASLEEP)
  │
  ├─▶ (smooth_deltaRMS > 0.08 for > 5 min, zone still occupied)
  │     └─▶ AWAKE_IN_BED (night waking)
  │           short_wake_count += 1
  │           Returns to ASLEEP if motion stops within 15 min
  │
  └─▶ (zone becomes unoccupied or nighttime window ends)
        └─▶ FINAL (record committed to sleep_records)
              wake_time = now
```

**Metric definitions:**

- **Time in bed:** `wake_time - bed_time` (minutes)
- **Sleep onset latency:** `onset_latency_min` = time from IN_BED to first ASLEEP transition
- **Wake time:** timestamp of final zone-exit or 09:00 if still in zone
- **Restlessness index:** `min(5.0, restless_event_count / (time_in_bed_h))` — events per hour, capped at 5.0
  - A "restless event" = smooth_deltaRMS > 0.08 for > 30 s while in ASLEEP state
- **Breathing rate:** For each 30-minute window during ASLEEP state: compute FFT over the last 600 samples (30 s at 20 Hz), find dominant peak in 0.1–0.5 Hz band, convert to bpm. Append to breathing_rate_samples. `breathing_rate_avg = mean(breathing_rate_samples)`.
- **Breathing regularity:** `coefficient_of_variation = std(breathing_rate_samples) / mean(breathing_rate_samples)`. Low CV (< 0.10) = regular; high CV (> 0.25) = irregular.
- **summary_json:** Array of 30-minute bucket objects `[{"t":"23:00","state":"asleep","bpm":14.2,"restless":false}, ...]` for the weekly-trends chart.

**Multi-person bedroom edge case:** If two blobs are tracked in a bedroom zone simultaneously, the system assigns the sleep record to the BLE-matched person if available, otherwise creates two separate `zone-based` records (one per occupant slot). Breathing analysis uses the blob with the strongest stationary signal (lowest smooth_deltaRMS).

**Dashboard:**
- **Morning summary card (simple mode):** "Sleep 11:23pm – 7:02am (7h 39m). Restlessness: Low. Breathing: Regular."
- **Weekly trends (expert mode):** Charts of sleep duration, onset time, restlessness index, breathing rate over 7/30 days
- **Anomaly flagging:** "Breathing rate elevated last night (22 bpm vs. 16 bpm average)" — could indicate illness, stress, or environmental change

**Per-person tracking:** When BLE identifies who is sleeping, the report is per-person. If BLE is not configured, the report is per-zone (assuming single occupancy).

**Privacy:** No spatial tracking data is stored for sleep analysis — only aggregate motion/breathing statistics. A bedroom zone can have spatial privacy enabled while still collecting sleep metrics.

**Storage:** ~200 bytes per night per person. Negligible.

### 26. Crowd Flow Visualization

Aggregates blob trajectories over time into a directional flow map showing how a space is actually used.

**Data accumulation:**

Every fusion tick (10 Hz), for each tracked blob with confidence > 0.3:

```
cell_x = floor(blob.x / SPAXEL_GRID_CELL_M)
cell_y = floor(blob.y / SPAXEL_GRID_CELL_M)

For each active bucket_type in {'hour', 'day', 'week'}:
  bucket_ms = floor(now_ms / bucket_duration_ms) × bucket_duration_ms
    where bucket_duration_ms = 3_600_000 (hour) | 86_400_000 (day) | 604_800_000 (week)

  UPSERT crowd_flow (bucket_ms, bucket_type, cell_x, cell_y):
    entry_count += 1
    vx_sum      += blob.vx   (m/s; can be negative)
    vy_sum      += blob.vy
    dwell_ms    += 100        (one 10 Hz tick = 100 ms)
```

`entry_count` counts tick-frames (not transitions into a cell). To display "average velocity direction" for a cell: `vx_avg = vx_sum / entry_count`, `vy_avg = vy_sum / entry_count`. Arrow rendered only when `sqrt(vx_avg²+vy_avg²) > 0.05 m/s` (suppress stationary dwell cells from arrow layer).

Memory accumulator: an in-memory `map[bucketKey]CrowdCell` is flushed to SQLite every 60 s (batch UPSERT). `bucketKey = (bucket_ms, bucket_type, cell_x, cell_y)`. Stale in-memory entries (bucket_ms older than the current bucket boundary) are flushed and evicted.

- Accumulated into configurable time buckets: 1 hour, 1 day, 1 week
- Storage: ~30 KB per time bucket for a 50×50 grid

**3D rendering (toggle-able layer):**
- **Flow arrows:** Animated arrows along major movement corridors. `TubeGeometry` along spline paths fitted to high-traffic cell sequences. Width proportional to traffic volume. Color by average speed (blue = slow, red = fast). Arrows animate in the direction of travel
- **Dwell hotspots:** Cells with high dwell time rendered as warm-colored pools on the ground plane (the couch, the desk, the kitchen counter). Intensity proportional to total dwell hours
- **Time filter:** Slider or dropdown to show flow for specific periods: "Morning routine (6–9am)", "Evening (6–10pm)", "Last 24 hours", "Last 7 days"
- **Per-person filter:** When BLE identity is available, show flow for a specific person: "Alice's typical paths" vs "Bob's typical paths"

**Use cases:**
- Understand furniture layout effectiveness ("everyone walks around the coffee table — move it")
- Identify most/least used areas for node placement optimization
- Commercial applications: retail foot traffic, office utilization
- Visualize the household "desire paths" — patterns emerge over days

**Implementation:** A 2D histogram accumulator updated per frame from blob positions. Arrow rendering uses Three.js `TubeGeometry` along spline paths. The accumulator runs in the mothership's fusion loop with negligible cost (one grid-cell increment per blob per frame).

### 27. Activity Timeline (Universal Navigation)

A single scrollable, filterable, searchable timeline that contains every event the system has ever observed. This replaces scattered views and separate log pages with one unified stream that serves as the primary way to navigate both time and space.

**Event types (all in one stream):**
- Detections: blob appeared, blob disappeared, blob entered/left zone
- Person events: "Alice entered Kitchen", "Bob left the house"
- Zone transitions: portal crossings with direction
- Automation triggers: trigger fired, condition met/unmet
- Alerts: fall detection, anomaly, security mode events
- System events: node online/offline, OTA updates, baseline changes, self-improving weight updates
- Learning milestones: "Prediction model for Alice reached 80% confidence", "Diurnal baseline for hour 14 fully calibrated"

**Interactions:**
- **Tap any event:** The 3D view jumps to that exact moment via time-travel. The scene shows the state at that point — blobs where they were, nodes in their state. The timeline becomes a spatial remote control
- **Inline actions per event:** Mark correct/incorrect (thumbs up/down), "Why?" (open explainability), create automation from this event, share
- **Filters:** By person, by zone, by event type, by time range. Combinable: "Alice + Kitchen + after midnight"
- **Search:** Natural language queries: "kitchen occupied after midnight last week" → filters to matching events
- **Scroll up = go back in time.** Open the dashboard after being away → scroll up to see everything that happened since last visit

**Layout:**
- Expert mode: timeline as a collapsible sidebar alongside the 3D view. Clicking events controls the 3D view
- Simple mode: timeline IS the main view (as the activity feed), with room cards above it
- Ambient mode: no timeline visible

**Implementation:** Events stored in SQLite with indexed timestamp, type, zone, person fields. WebSocket pushes new events in real-time. Frontend renders as a virtualized list (only DOM nodes for visible events). Search implemented as SQL queries on the events table.

### 28. Detection Explainability ("Why Is This Here?")

Every detection, alert, and automation trigger can be inspected to reveal exactly why the system made that decision. This is a first-class interaction, not a hidden debug tool.

**Activation:** Tap/click a humanoid figure in the 3D view → "Why?" button, or tap "Why?" on any timeline event.

**X-ray overlay (3D view):**
- All non-contributing visual elements dim (room bounds, other blobs, floor plan go to 20% opacity)
- Links that contributed to this detection glow, with brightness proportional to their deltaRMS contribution
- Fresnel zone ellipsoids appear for active links, showing WHY the system placed the blob at this intersection
- If BLE contributed: a dotted line from the matched BLE device's strongest node to the blob, labeled with RSSI

**Detail sidebar:**
- Per-link contribution table: link name, deltaRMS value, threshold, Fresnel zone number at blob position, learned weight
- BLE match details: device name, per-node RSSI values, match confidence
- Confidence breakdown: "Spatial confidence: 78% (from 5 contributing links). Identity confidence: 92% (iPhone RSSI −48 at kitchen-north)"
- For alerts: the specific conditions that triggered and their values vs thresholds

**For false positives:** The explainability view makes the cause obvious. "Link kitchen↔hallway spiked because of HVAC" → user marks as incorrect → system adjusts. Understanding replaces frustration.

**For automations:** "Trigger 'Couch Dwell' fired because Blob #2 (Alice) has been inside the trigger volume for 34 seconds (threshold: 30s). Action: webhook POST to http://ha.local/api/..."

### 29. Detection Feedback Loop (Thumbs Up/Down)

Every detection has two small buttons: correct (thumbs up) or incorrect (thumbs down). A third action — "I was here but you missed me" — allows the user to tap a location in the 3D view to mark a missed detection.

**Available everywhere:**
- On humanoid figures in the 3D view (small overlay buttons on hover/tap)
- On every detection event in the timeline
- On push notifications (inline action buttons)

**What happens on feedback:**

**Thumbs up (correct):**
- Contributing links' Fresnel weights get a small positive nudge (+0.001 per link)
- Reinforces the current detection parameters for these links
- Logged to the accuracy tracking table

**Thumbs down (incorrect / false positive):**
- Contributing links' Fresnel weights get a small negative nudge (−0.002 per link, slightly stronger than positive to prioritize eliminating false positives)
- Detection threshold for contributing links is microscopically raised (+0.5% per feedback)
- Event logged with timestamp and contributing links — if false positives cluster at specific times of day, the diurnal baseline for those links adjusts during the next hourly crossfade
- System responds in the timeline: "Got it. I've slightly raised the detection threshold for the contributing links. If this keeps happening at this time of day, my hourly baseline will adapt within a few days."

**"I was here" (missed detection):**
- User taps a location in the 3D view → a ground-truth point is recorded at that position and time
- Contributing links' detection thresholds are microscopically lowered (−0.5%)
- The self-improving weight system gets a positive sample: "a person was HERE, so links whose Fresnel zones cover this point should be more trusted"

**Accuracy tracking:**
- Dashboard shows an "Accuracy" trend card: "You've provided 47 corrections. Detection accuracy has improved 12% since installation."
- Weekly accuracy report in the morning briefing
- The trend graph shows the cumulative effect of user feedback over weeks — creating a visible reward for providing corrections

### 30. Spatial Context Notifications

Push notifications include a rendered mini floor-plan thumbnail and natural language text, so the user understands the spatial context without opening the app.

**Notification format:**
- **Image:** A small (400×300 px) top-down floor plan rendering with the relevant blob/zone highlighted. Generated server-side as PNG by the Go backend. Works on every platform: iOS, Android, desktop, email, Slack, Discord.

  **Renderer specification:**
  - Library: `github.com/fogleman/gg` (pure Go, no cgo, 2D drawing API). Embedded font: Roboto Regular at 10pt for labels, 8pt for names (embedded as `//go:embed` binary in the Go binary).
  - Coordinate mapping: floor plan meters → image pixels. Scale = `min(380/room_width_m, 280/room_depth_m)`; origin at (10, 10) px for 10px margin.
  - Render pipeline (in order): (1) dark gray background fill (#1a1a1a); (2) if floor plan image exists: draw as background, resized to fit; (3) room zone outlines as white 1px rectangles; (4) highlighted zone: filled with 40% opacity red (alert) or zone color; (5) person circles: 8px radius for BLE-identified (person color), 6px radius for unknown (#888); (6) person name labels above each circle; (7) small scale bar (bottom-right corner, shows 5m reference); (8) optional text overlay (event title, bottom-left).
  - Background layer is cached: the static floor plan (room outlines + uploaded image) is pre-rendered and cached as an in-memory PNG. Cache is invalidated when room bounds or floor plan image changes. Per-notification rendering only re-draws layers 4–8 on top of the cached background.
  - Thread safety: each render gets its own `gg.Context` — no shared mutable state.
  - Render time target: <50 ms for up to 10 people on a typical floor plan (measured on a Pi 4).
  - Error handling: if any step fails (e.g., font load error, nil geometry), the notification is sent as text-only (no image) with a log warning.
  - Test endpoint: `GET /api/notifications/preview?type=fall&person=Alice` returns a rendered test image for UI development and QA.
  - Output: in-memory `[]byte` PNG (not written to disk); delivered directly in the HTTP notification payload or as a URL reference for push services that require URL-based images.
- **Title:** Short, natural language: "Motion in Kitchen (2:34am)" or "Fall Detected: Alice"
- **Body:** One sentence of context: "Someone entered from the hallway. Security mode is active." or "Alice hasn't moved for 15 seconds."
- **Actions:** Platform-native action buttons where supported: [Open Dashboard] [Dismiss] or for falls: [I'm Fine] [Call Help]

**Notification types and their language:**

| Event | Title | Body |
|-------|-------|------|
| Zone entry | "Alice entered Kitchen" | "Coming from the hallway. Bob is in the living room." |
| Security motion | "Motion detected (2:34am)" | "Kitchen, from hallway direction. Security mode active." |
| Fall alert | "Fall detected: Alice" | "Hallway. No movement for 15s. [I'm Fine] [Call Help]" |
| Anomaly | "Unusual activity" | "Motion in kitchen at 3am — normally vacant at this hour." |
| System | "Node offline: kitchen-north" | "Coverage in kitchen reduced. 5/6 nodes online." |
| Daily summary | "Daily summary" | "Home occupied 14h. Alice: 9h, Bob: 6h. All systems healthy." |

**Smart batching:**
- Multiple zone transitions within 30 seconds are batched: "Alice moved through hallway → kitchen → dining room" instead of three separate notifications
- Repeated events are collapsed: "Motion in kitchen (3 times in 10 min)" instead of three alerts
- Quiet hours suppress non-critical notifications (configurable per user)

**Delivery channels (configurable per event type):**
- Push notification (via Ntfy, Pushover, Gotify)
- Webhook (for Home Assistant, Slack, Discord, email)
- Dashboard only (default for low-priority events)

### 31. Ambient Dashboard Mode

A dedicated display mode for wall-mounted tablets or always-on screens. Served at `/ambient` — a separate lightweight route optimized for low-power devices.

**Visual design:**
- Simplified, stylized top-down floor plan — clean lines, soft rounded corners, no UI chrome
- People appear as softly glowing colored circles (BLE-identified) or neutral dots (unknown), with names in a gentle sans-serif font
- Room labels show subtle occupancy: "Kitchen · Alice" or "Bedroom · Empty"
- Smooth, calm animations: dots drift with interpolated positions, no jitter, no snapping
- No toolbar, no buttons, no panels — just the floor plan, the people, and a small status line

**Time-of-day awareness:**
- Morning (6–10am): bright, cool palette, cheerful
- Day (10am–6pm): neutral, clean
- Evening (6–10pm): warm amber tones, slightly dimmed
- Night (10pm–6am): very dim, minimal elements, just "All secure" centered. Screen brightness at 10%

**Adaptive behavior:**
- House empty for 30+ min: screen goes fully dark (OLED-safe), "All secure" in tiny text
- Someone arrives: gentle fade-in, dot appears with name
- Alert event: entire display transitions to alert mode — pulsing red border, large text, action buttons ("Dismiss" / "Call Help"). Returns to ambient after dismissal with a smooth crossfade

**Morning briefing integration:** When the first person is detected in the morning, the ambient display briefly shows the morning briefing text (sleep summary, overnight events, today's predictions) before fading to the normal ambient view.

**Implementation:** Separate `/ambient` route serving a lightweight HTML page. No Three.js — uses Canvas 2D or SVG for minimal resource usage on older tablets. WebSocket receives the same dashboard feed but only uses blob positions, zone counts, and alerts. Typically <30 MB RAM, <5% CPU on a 2018 iPad.

### 32. Spatial Quick Actions (Context Menus)

Right-click (desktop) or long-press (mobile) anywhere in the 3D view to get context-sensitive actions based on what's under the cursor.

**On a person/blob:**
- "Who is this?" → opens BLE device assignment if unidentified
- "Why is this here?" → opens explainability overlay (Component 28)
- "Follow" → camera smoothly tracks this person, auto-orbiting to keep them centered
- "Create automation here" → pre-fills a trigger volume at this location with this person's filter
- "Mark incorrect" → thumbs-down feedback (Component 29)
- "Track history" → filters timeline to this person's events

**On a node:**
- "Diagnostics" → inline CSI amplitude/phase plot for this node's links (2D overlay)
- "Blink LED" → sends identify command via WebSocket
- "Reposition" → enters TransformControls for this node
- "Update firmware" → triggers OTA if update available
- "Show links" → highlights all links involving this node
- "Disable" / "Enable" → takes node out of / returns to active fleet

**On empty floor space:**
- "What happened here?" → filters timeline to events within 1 m of this point
- "Add trigger zone" → creates a trigger volume centered here
- "Add virtual node" → places a virtual node for coverage planning
- "Coverage quality" → shows GDOP value at this point with contributing link breakdown

**On a zone label:**
- "Zone history" → occupancy chart (24h / 7d) for this zone
- "Edit zone" → resize/rename/delete
- "Create automation" → pre-fills zone-based trigger
- "Crowd flow" → shows flow data filtered to this zone

**On a portal:**
- "Crossing log" → recent directional crossings with timestamps and person names
- "Edit portal" → reposition or rename
- "Reverse direction" → swap the zone labels

**On a trigger volume:**
- "Edit trigger" → open automation config for this trigger
- "Test" → simulate a trigger fire to verify webhook/action
- "View log" → filter timeline to this trigger's events
- "Disable" / "Enable"

**Implementation:** Three.js Raycaster determines what's under the cursor. A single context menu component renders the appropriate options. Each action dispatches to existing dashboard functions (no new backend endpoints needed — just UI wiring).

### 33. Interactive Onboarding (Teach by Doing)

The onboarding wizard responds to live sensor data, teaching CSI physics through direct experience rather than documentation.

**Sequence (runs after first node connects):**

**Step 1 — "Walk around" (30 s):**
- Dashboard shows a real-time CSI amplitude chart alongside the 3D view
- "I'm listening to your WiFi router's signal through your new node. Walk across the room."
- As the user walks, the waveform visibly distorts. Amplitude spikes are highlighted in real-time
- "See that? Your body just changed the WiFi signal between your router and the node. That's how I detect you."

**Step 2 — "Stand still" (10 s):**
- "Now stand still for 10 seconds."
- The waveform stabilizes. A green "baseline" line fades in on the chart
- "This is your room's baseline — the signal when nothing is moving. Any change from this means someone is here."
- Baseline is automatically captured during this step (replaces the manual calibration trigger)

**Step 3 — "Walk through the detection zone" (15 s):**
- The Fresnel zone ellipsoid between the node and the router lights up in the 3D view as a translucent green volume
- "Walk between your node and the router — through the green zone."
- As user crosses it, the Fresnel zone pulses brighter and the amplitude chart shows a strong peak
- "That's the Fresnel zone — I'm most sensitive along this path. The more nodes you add, the more zones I have."

**Step 4 — "Let me find you" (15 s):**
- "Walk somewhere and stop. I'll try to locate you."
- A humanoid figure appears at the estimated position. A dotted circle shows the accuracy radius
- "Found you! I estimate you're about here. My accuracy is ±1 meter with this setup. Adding more nodes tightens this."

**Step 5 — "Place your node" (interactive):**
- Coverage painting activates on the ground plane
- "Now drag your node to where it actually is in the room. Watch the green coverage change — put it where it helps most."
- After placement: "Nice! Your coverage score is 62%. Want to add another node to improve it?"

**Total duration:** ~2 minutes. No jargon ("CSI", "Fresnel", "deltaRMS" never appear). User finishes with intuitive understanding of: how detection works, what a baseline is, where coverage is strong, and why more nodes help.

**Skip option:** "Skip tutorial" link visible throughout for users who know what they're doing.

### 34. Command Palette

Ctrl+K (Cmd+K on Mac) opens a universal search and command interface. Invisible to casual users, indispensable for power users.

**Search:**
- "kitchen" → Kitchen zone, kitchen nodes, kitchen automations, recent kitchen events
- "alice" → Alice's current location, today's timeline, sleep report, BLE devices
- "node 3" → Node details, diagnostics, link health

**Navigate time:**
- "last night 2am" → timeline jumps there, 3D view shows that moment
- "yesterday kitchen" → filters timeline to kitchen events yesterday
- "this morning" → jumps to first detection today

**Execute commands:**
- "update all nodes" → confirms and triggers fleet OTA
- "re-baseline kitchen" → triggers re-baseline for kitchen links
- "add node" → opens Web Serial onboarding
- "arm security" / "disarm security" → toggles security mode
- "dark mode" / "light mode" → toggles theme
- "export config" → downloads system configuration
- "restart node kitchen-north" → sends reboot command

**Get help:**
- "help fall detection" → opens contextual help about fall detection settings
- "why false positive" → opens explainability for the most recent incorrect detection
- "troubleshoot kitchen" → starts guided troubleshooting for the kitchen zone
- "how does prediction work" → inline help text

**Behavior:**
- Fuzzy matching: "flr pln" matches "Floor Plan settings." "brth" matches "Breathing band sensitivity"
- Recently used commands appear first
- Results show keyboard shortcut hints where applicable
- Escape closes, Enter executes top result
- Works in expert mode only (not in simple or ambient mode)

**Implementation:** Frontend-only component. Command registry maps keywords to actions. Search runs against: zone names, person names, node names, setting names, help topics. No backend endpoint needed — all dispatch is client-side.

### 35. Morning Briefing

When the user first opens the dashboard each day (or at a configured notification time), a brief, warm summary appears.

**Content (generated from existing data):**

```
Good morning, Alice. You slept 7h 39m — 12 minutes more than your average.
Breathing was regular.

Bob left at 8:15am. The house has been empty since 8:22am.

Last night: One unusual event at 2:34am — motion in the kitchen for
30 seconds. No BLE match, low-confidence blob. Likely environmental.

System health: Excellent (94%). All 6 nodes online.
Accuracy improved 2% this week thanks to your 8 corrections.

Today's forecast: Based on your Wednesday pattern, you usually return
around 5:45pm. Security mode will auto-activate when you leave.
```

**Display:**
- In expert mode: card overlay that appears on first dashboard open of the day, dismissible with a tap or "Got it" button. Slides away after 10 seconds if not interacted with
- In simple mode: the morning card is the first card in the layout, stays visible until dismissed
- In ambient mode: text fades in over the ambient display when first person detected in the morning, stays for 30 seconds

**Adaptive length:**
- Nothing interesting happened: "All quiet last night. All systems healthy." (one line)
- Something notable: leads with the notable event, then other details
- Something urgent: leads with the alert and actions needed

**Delivery channels:**
- Dashboard (default)
- Push notification at configured time (e.g., 7am)
- Webhook to Slack/Discord channel

**"What happened while I was away" variant:** When the user opens the dashboard after being away for >4 hours, a similar summary covers the entire absence period instead of just overnight.

**Generation algorithm (Go function `GenerateBriefing(date string, person string) string`):**

The briefing is assembled in priority order. Each section is a conditional block; sections with no data are omitted entirely.

```
Inputs (all queried for the prior night: 18:00 yesterday → now):
  sleep  = SELECT * FROM sleep_records WHERE person=? AND date=<yesterday>
  events = SELECT * FROM events WHERE timestamp_ms BETWEEN night_start AND now ORDER BY timestamp_ms
  anomalies = events WHERE type='anomaly' AND severity IN ('warning','alert','critical')
  nodes  = SELECT COUNT(*) FROM nodes WHERE status='online' / total
  quality = current detection_quality
  feedback_this_week = SELECT COUNT(*) FROM feedback WHERE timestamp_ms > now-7d
  accuracy_delta = accuracy this week vs last week (from feedback table)
  predictions = GET /api/predictions for person, horizon=60m

Priority assembly (render first non-empty block as the lead paragraph):

  BLOCK 1 — Critical alerts (if any fall_alert or security_alert in events):
    "⚠ [alert description, zone, time]."

  BLOCK 2 — Sleep summary (if sleep record exists):
    Base: "You slept [duration]h [duration_m]m"
    + deviation: " — [N] minutes [more|less] than your average." (if |delta| > 10 min)
    + restlessness: " Restlessness: [Low|Moderate|High]." (Low < 1/h, Moderate 1–3/h, High > 3/h)
    + breathing: " Breathing: [Regular|Irregular]." (regular if CV < 0.15)
    + anomaly: " Breathing rate elevated ([N] bpm vs [avg] bpm average)." (if bpm > avg×1.25)

  BLOCK 3 — Who is home (current state):
    "Bob left at [time]. The house has been empty since [time]." (if no one home)
    OR "Alice is home. Bob left at [time]."

  BLOCK 4 — Overnight anomalies (if any in events and not already in BLOCK 1):
    "Last night: [first anomaly description]. [Low|Medium|High]-confidence."
    (if multiple: "Last night: [N] unusual events. Most notable: [highest anomaly_score event]")
    "Likely environmental." appended if anomaly_score < 0.7

  BLOCK 5 — System health (if not excellent):
    Skip if quality >= 90 and all nodes online.
    "System health: [Excellent|Good|Fair|Poor] ([quality]%). [N]/[total] nodes online."

  BLOCK 6 — Prediction hint (if prediction exists and confidence > 0.7):
    "Today's forecast: Based on your [weekday] pattern, you usually [first predicted_enter action]."

  BLOCK 7 — Learning progress (if feedback_this_week > 0):
    "Accuracy improved [delta]% this week thanks to your [N] corrections." (if delta > 0)
    OR: "You provided [N] corrections this week." (if delta = 0)

DEGENERATE CASE (all blocks empty = nothing happened):
  "All quiet last night. All systems healthy."

"What happened while I was away" variant: identical algorithm but
  night_start = SELECT MAX(last_seen_at) FROM sessions WHERE last_seen_at < now - 4h
                (= the most recent session activity before the current gap; falls back to 4 h ago if no prior session)
  night_end = now
  BLOCK 2 (sleep) included only if period covers ≥ 4 h of nighttime hours
```

**Storage:** Briefing is generated once per day (at first open or at configured push time). The rendered text is stored in the `briefings` table. Subsequent dashboard opens the same day retrieve the stored record rather than re-generating.

**Stored as a daily record in SQLite so it can be retrieved later.**

### 36. Guided Troubleshooting

When the system detects that the user might be struggling or that detection quality has degraded, it proactively offers contextual help — but never when things are working well.

**Trigger conditions and responses:**

**Detection quality drops:**
- Condition: Zone-level detection quality below 60% for >24 hours
- Banner in timeline and 3D view: "Detection in the kitchen has been less reliable this week. Want me to help diagnose?"
- Guided flow: Check node connectivity → show link health with explainability → suggest node repositioning using coverage painting → offer re-baseline → "Still not right? Try adding a node here [highlighted optimal position]"

**Repeated setting changes:**
- Condition: The same settings key (from `/api/settings` PATCH requests) is modified 3 or more times within a 60-minute sliding window. Qualifying settings keys: `delta_rms_threshold`, `breathing_sensitivity`, `tau_s`, `fresnel_decay`, `n_subcarriers`. Keys that do not qualify: display preferences (theme, layout), notification config, MQTT config.
- Tracking: the server increments a per-key edit counter in memory (not SQLite — ephemeral). Counter resets after 60 minutes of inactivity on that key.
- Trigger: when the counter for any qualifying key reaches 3 within the window, set a `hint_pending` flag. The flag is consumed and cleared when the next dashboard page load or next `/api/settings` response includes `"repeated_edit_hint": true` in the JSON body.
- Frontend behavior: on receipt of `repeated_edit_hint: true`, show a non-intrusive banner (not modal): "You've adjusted the detection threshold several times. Would you like me to show you what the system is seeing?" with a [Show me] button and an [×] dismiss button.
- [Show me] action: opens time-travel to the most recent detection event before the first edit in the window, with the explainability overlay pre-activated, so the user can visually tune thresholds against real data.
- Cooldown: after the hint is shown (displayed or dismissed), do not re-trigger the same hint for 24 hours regardless of further edits.
- The hint is stored in `localStorage` (not server-side) — the server only sets the flag; the client remembers the 24-hour cooldown.

**Node offline:**
- Condition: Any node offline for >2 hours
- Timeline event with expandable troubleshooting steps: "Node kitchen-north has been offline since 3:15pm." → 1) "Is it powered? Check the USB connection." 2) "Can it reach WiFi? Look for the captive portal AP: spaxel-XXXX." 3) "Try reflashing from the dashboard: [Open Web Serial]." Each step has a one-click action where possible

**First-time feature discovery:**
- Condition: User opens a feature panel for the first time
- Brief, non-intrusive tooltip (not a modal): "Draw a box around an area, then choose what happens when someone enters or leaves. [Got it]"
- Shown once, never repeated. Dismissed on click anywhere

**After false positive feedback:**
- Condition: User marks a detection as incorrect
- Inline response in timeline: "Got it. I've slightly raised the detection threshold for the contributing links. If this keeps happening at this time of day, my hourly baseline will adapt within a few days. You can also adjust sensitivity manually → [Open Settings]."

**After successful calibration:**
- Positive reinforcement: "Re-baseline complete. Detection quality in the kitchen improved from 64% to 89%."

**Design principles:**
- **Reactive, not proactive:** Help appears only when something seems wrong or when the user is clearly exploring
- **Dismissible in one tap:** Never blocks the UI
- **Never repeats** after dismissal (stored in localStorage)
- **Always explains what will happen next:** "I'll adjust X, which should improve Y within Z days"
- **Never condescending:** Assumes the user is intelligent but may not know CSI physics

---

## Home Automation Integration (MQTT)

The mothership acts as an MQTT publisher-only client connecting to the user's existing broker (e.g., Mosquitto bundled with Home Assistant). No MQTT broker runs inside the container. MQTT is optional — all features work without it; it's an integration layer only.

**Configuration** (via environment or settings API):
- `SPAXEL_MQTT_BROKER` — broker URL, e.g., `mqtt://homeassistant.local:1883` or `mqtts://...`
- `SPAXEL_MQTT_USERNAME` / `SPAXEL_MQTT_PASSWORD` — optional credentials
- `SPAXEL_MQTT_PREFIX` — topic prefix (default: `spaxel`)
- `SPAXEL_MQTT_CLIENT_ID` — client ID (default: `spaxel-<installation_id>`)

**Connection management:**
- Connects on startup if configured. Reconnects with exponential backoff (1s, 2s, 4s... up to 5 min cap) on broker unavailability.
- LWT (Last Will and Testament): `{prefix}/availability` → payload `"offline"` (retained, QoS 1)
- On successful connect: publish `{prefix}/availability` → `"online"` (retained, QoS 1)
- MQTT v3.1.1 for maximum HA compatibility (paho.mqtt.golang library)

**Topic hierarchy:**

```
{prefix}/                            — e.g., "spaxel/"
  availability                       — "online" | "offline" (LWT; retained)
  system/detection_quality           — integer 0-100 (published on change)
  system/nodes_online                — integer (published on change)

  zone/{zone_name}/occupancy         — integer count (published on change only)
  zone/{zone_name}/people            — JSON array of names e.g. ["Alice","Bob"] (published on change)

  person/{person_name}/present       — "home" | "not_home" (published on change)
  person/{person_name}/zone          — zone name string or "unknown" (published on change)
  person/{person_name}/predicted_zone — JSON {"zone":"Kitchen","confidence":0.87} (published every 5 min)

  alert/fall                         — JSON {"person":"Alice","zone":"Hallway","timestamp_ms":N} (event-fired)
  alert/anomaly                      — JSON {"zone":"Kitchen","score":0.92,"message":"..."} (event-fired)
  alert/security                     — JSON {"zone":"Hallway","timestamp_ms":N} (event-fired, security mode only)

  node/{mac}/status                  — "online" | "stale" | "offline" (published on change)
  node/{mac}/rssi                    — integer dBm (published every 30 s)

  command/security_mode              — subscribes to: "arm" | "disarm" (HA can control security mode)
  command/rebaseline                 — subscribes to: zone name or "all" (HA can trigger re-baseline)
```

**Home Assistant auto-discovery** (published once on connect, retained, QoS 1):

HA auto-discovery topic pattern: `homeassistant/{component}/spaxel_{entity_id}/config`

```jsonc
// Zone occupancy sensor (one per zone)
// Topic: homeassistant/sensor/spaxel_zone_kitchen_occupancy/config
{
  "name": "Kitchen Occupancy",
  "unique_id": "spaxel_zone_kitchen_occupancy",
  "state_topic": "spaxel/zone/Kitchen/occupancy",
  "availability_topic": "spaxel/availability",
  "device_class": "occupancy",       // HA recognizes integer occupancy
  "state_class": "measurement",
  "unit_of_measurement": "people",
  "device": {"identifiers":["spaxel"],"name":"Spaxel","manufacturer":"Spaxel","model":"1.0"}
}

// Per-person presence binary sensor (one per registered BLE person)
// Topic: homeassistant/binary_sensor/spaxel_person_alice_presence/config
{
  "name": "Alice Present",
  "unique_id": "spaxel_person_alice_presence",
  "state_topic": "spaxel/person/Alice/present",
  "payload_on": "home",
  "payload_off": "not_home",
  "availability_topic": "spaxel/availability",
  "device_class": "presence",
  "device": {"identifiers":["spaxel"],"name":"Spaxel"}
}

// Fall detection binary sensor
// Topic: homeassistant/binary_sensor/spaxel_alert_fall/config
{
  "name": "Fall Detected",
  "unique_id": "spaxel_alert_fall",
  "state_topic": "spaxel/alert/fall",
  "value_template": "{% if value_json.person is defined %}ON{% else %}OFF{% endif %}",
  "availability_topic": "spaxel/availability",
  "device_class": "safety",
  "device": {"identifiers":["spaxel"],"name":"Spaxel"}
}

// System detection quality sensor
// Topic: homeassistant/sensor/spaxel_system_quality/config
{
  "name": "Spaxel Detection Quality",
  "unique_id": "spaxel_system_quality",
  "state_topic": "spaxel/system/detection_quality",
  "unit_of_measurement": "%",
  "state_class": "measurement",
  "availability_topic": "spaxel/availability",
  "device": {"identifiers":["spaxel"],"name":"Spaxel"}
}
```

**Auto-discovery lifecycle:**
- Auto-discovery configs are published with `retain=true` on first connect and whenever zones/persons are added or renamed.
- When a zone or person is deleted in the dashboard, the mothership publishes an empty retained payload to the corresponding auto-discovery topic to remove the entity from HA.
- Entity `unique_id` is derived from the installation ID + entity type + name, ensuring stability across restarts.

**Publish policy (avoiding floods):**
- Zone occupancy and person presence: publish only on state change, not at 10 Hz.
- System metrics: publish every 30 s or on significant change (>5% quality change).
- Alerts: publish immediately on event fire, with no deduplication (each alert is a distinct event).
- MQTT publish queue is bounded at 500 messages; oldest are dropped if the broker is slow.

**Bidirectional commands (subscriptions):**
- Mothership subscribes to `{prefix}/command/security_mode` and `{prefix}/command/rebaseline` after connecting.
- This allows HA automations to arm/disarm security mode or trigger re-baseline without opening the dashboard.

---

## Data Flow Summary

```
ESP32 Node                    Mothership                         Browser
    │                             │                                 │
    │── WS /ws/node ────────────▶│                                 │
    │   binary: CSI frames        │── Parse + buffer ──▶ Ring buf   │
    │   json: hello, health,      │── Record ──▶ CSI replay store   │
    │         BLE scan results    │── Phase sanitise ──▶ Clean CSI  │
    │                             │── Feature extract ──▶ deltaRMS  │
    │◀── WS /ws/node ────────────│── Fresnel accumulate ──▶ Grid   │
    │   json: config, role,       │── Peak extract ──▶ Blobs        │
    │         rate, OTA           │── Biomech Kalman ──▶ Tracked    │
    │                             │── BLE match ──▶ Identified      │
    │                             │── Weight update ──▶ Self-improve │
    │                             │── Flow accumulate ──▶ Crowd map │
    │                             │── Trigger eval ──▶ Automations  │
    │                             │── Predict ──▶ Pre-emptive acts  │
    │                             │── Anomaly check ──▶ Alerts      │
    │                             │── Sleep analysis ──▶ Reports    │
    │                             │── Event log ──▶ Timeline store  │
    │                             │── Notification render ──▶ PNG   │
    │                             │                                 │
    │                             │── WS /ws/dashboard ────────────▶│
    │                             │   {blobs+identity, nodes,       │
    │                             │    zones, links, triggers,      │
    │                             │    confidence, predictions,     │
    │                             │    sleep, flow}         10 Hz   │
    │                             │                                 │
    │                             │◀── HTTP API ────────────────────│
    │                             │   (UI, param tuning, feedback,  │
    │                             │    BLE registration, commands)  │
    │                             │                                 │
    │                             │──▶ External MQTT broker ────────│
    │                             │   (optional HA auto-discovery)  │
```

All traffic uses a single HTTP port (8080) — WebSocket upgrades for node connections and dashboard, REST for API. Entire stack sits behind Traefik with no additional ports.

---

## REST API Specification

All endpoints are under the single HTTP server on port 8080. WebSocket endpoints use the `Upgrade: websocket` mechanism. All REST endpoints return `Content-Type: application/json`. Errors follow `{"error": "<human message>", "code": "<snake_case_code>"}`. Authentication: session cookie required on all `/api/*` endpoints (except `/healthz` and `/api/provision`).

### WebSocket Endpoints

| Endpoint | Direction | Description |
|----------|-----------|-------------|
| `GET /ws/node` | bidirectional | Node connection. Requires `X-Spaxel-Token` header. Binary frames upstream (CSI), JSON frames downstream (config/commands) |
| `GET /ws/dashboard` | server→client | Dashboard live feed at 10 Hz. Requires session cookie. JSON frames: `{blobs, nodes, zones, links, triggers, confidence, predictions, sleep, flow, events}` |

### System

| Method | Path | Request | Response |
|--------|------|---------|----------|
| `GET` | `/healthz` | — | `{"status":"ok","uptime_s":N,"nodes_online":N,"db":"ok"}` |
| `GET` | `/api/status` | — | `{"version":"1.0.0","nodes":N,"blobs":N,"uptime_s":N,"detection_quality":87}` |
| `GET` | `/api/settings` | — | All user-configurable settings as flat JSON object. If `repeated_edit_hint` is pending: includes `"repeated_edit_hint":true` (consumed on delivery — cleared after one response) |
| `PATCH` | `/api/settings` | Partial settings object | Updated settings object |
| `GET` | `/api/export` | — | Full config dump as JSON (nodes, zones, portals, trigger volumes, BLE registry, settings). See schema below. |
| `POST` | `/api/import` | Config JSON (same schema as export) | `{"ok":true,"imported":{nodes:N,zones:N,...}}` or `{"error":"...","code":"schema_mismatch"}` |

**Export/Import JSON schema:**
```jsonc
{
  "version": 1,                    // export format version (not app version)
  "exported_at": "2024-03-15T...", // ISO8601
  "nodes": [                       // all rows from nodes table
    {"mac":"AA:BB:CC:DD:EE:FF","name":"Kitchen North","pos_x":1.2,"pos_y":0.5,"pos_z":2.1,
     "role":"rx","node_id":"f47ac10b-..."}
    // firmware_version, status, last_seen omitted (runtime state, not config)
  ],
  "zones": [{"name":"Kitchen","x":0,"y":0,"z":0,"w":4,"d":3,"h":2.5,"zone_type":"kitchen"}],
  "portals": [{"name":"Kitchen Door","zone_a":"Kitchen","zone_b":"Hallway",
               "points":[[1.2,3.0],[1.2,0.0]]}],
  "triggers": [{"name":"Couch Dwell","shape":{"type":"box","x":1,"y":2,"z":0,"w":1,"d":1,"h":1.5},
                "condition":"dwell","condition_params":{"duration_s":30},
                "actions":[{"type":"webhook","url":"http://ha.local/api/..."}],"enabled":true}],
  "ble_devices": [{"addr":"AA:BB:CC:DD:EE:FF","label":"Alice","type":"person","color":"#4488ff"}],
  "floorplan": {"image_url":null,"cal_ax":0,"cal_ay":0,"cal_bx":200,"cal_by":0,
                "cal_distance_m":5.0},
  "settings": {"fusion_rate_hz":10,"grid_cell_m":0.2,"delta_rms_threshold":0.02,...}
}
```

Import behavior:
- All existing nodes, zones, portals, triggers, BLE devices, and settings are **replaced** by the import (full replace, not merge).
- The floorplan image itself is NOT exported/imported via this endpoint (only calibration metadata). Re-upload the image separately via `POST /api/floorplan/image` if needed.
- `auth` (install_secret, PIN) is excluded from export/import — these are installation-specific.
- Learning data (baselines, anomaly patterns, prediction models, link weights) is excluded — these are derived data, not config.
- On validation failure: return `{"error":"schema mismatch","code":"schema_mismatch"}` without modifying any data.
| `GET` | `/api/backup` | — | ZIP archive (binary stream, `Content-Type: application/zip`). Archive contains: `spaxel.db` (SQLite Online Backup API snapshot), `floorplan/` directory, `briefings.json` (last 30 days). **No auth bypass** — requires valid session cookie. The SQLite backup uses the Online Backup API (`sqlite3_backup_*`) so no WAL-mode data is lost even under concurrent writes. Backup is streamed directly to the HTTP response without writing a temp file to disk. Filename hint: `Content-Disposition: attachment; filename="spaxel-backup-<YYYY-MM-DD>.zip"`. Max response time: 5 s (warn in logs if exceeded). |

### Authentication

| Method | Path | Request | Response |
|--------|------|---------|----------|
| `GET` | `/api/auth/setup` | — | `{"pin_configured":false}` — used to detect first-run |
| `POST` | `/api/auth/setup` | `{"pin":"1234"}` | `{"ok":true}` — sets PIN on first run only |
| `POST` | `/api/auth/login` | `{"pin":"1234"}` | Sets `spaxel_session` cookie; `{"ok":true}` or HTTP 401 |
| `POST` | `/api/auth/logout` | — | Clears cookie; `{"ok":true}` |
| `POST` | `/api/auth/change-pin` | `{"old_pin":"...","new_pin":"..."}` | `{"ok":true}` or HTTP 403 |

### Provisioning

| Method | Path | Request | Response |
|--------|------|---------|----------|
| `POST` | `/api/provision` | `{"mac":"AA:BB:CC:DD:EE:FF"}` (optional hint) | Binary NVS blob (WiFi creds + node token). No auth required — called by Web Serial onboarding |

### Nodes

| Method | Path | Request | Response |
|--------|------|---------|----------|
| `GET` | `/api/nodes` | — | `[{mac, name, role, position, firmware_version, status, rssi, uptime_s, last_seen}]` |
| `GET` | `/api/nodes/:mac` | — | Single node object |
| `PATCH` | `/api/nodes/:mac` | `{name?, position?, role?}` | Updated node object |
| `DELETE` | `/api/nodes/:mac` | — | `{"ok":true}` — removes node from registry (does not affect physical device) |
| `POST` | `/api/nodes/:mac/reboot` | — | `{"ok":true}` — sends reboot command over WebSocket |
| `POST` | `/api/nodes/:mac/identify` | — | `{"ok":true}` — blink LED for 5 s |
| `POST` | `/api/nodes/:mac/update` | — | `{"ok":true}` — triggers OTA on single node |
| `POST` | `/api/nodes/update-all` | — | `{"ok":true,"count":N}` — rolling OTA across fleet |
| `POST` | `/api/nodes/:mac/rebaseline` | — | `{"ok":true}` |
| `POST` | `/api/nodes/rebaseline-all` | — | `{"ok":true,"count":N}` |
| `POST` | `/api/nodes/:mac/disable` | — | Sets role to IDLE |
| `POST` | `/api/nodes/:mac/enable` | — | Restores prior role |

### Firmware

| Method | Path | Request | Response |
|--------|------|---------|----------|
| `GET` | `/api/firmware` | — | `[{filename, version, sha256, size_bytes, uploaded_at}]` |
| `POST` | `/api/firmware` | Multipart form: `file=<binary>`, `version=<string>` | `{"ok":true,"sha256":"..."}` |
| `DELETE` | `/api/firmware/:filename` | — | `{"ok":true}` |
| `GET` | `/firmware/:filename` | — | Raw binary (served to ESP32 during OTA; no auth required — URL contains SHA256 for integrity) |

### Zones, Portals, Trigger Volumes

| Method | Path | Request | Response |
|--------|------|---------|----------|
| `GET` | `/api/zones` | — | `[{id, name, bounds:{x,y,z,w,d,h}, occupancy, people:[]}]` |
| `POST` | `/api/zones` | `{name, bounds}` | Created zone object |
| `PATCH` | `/api/zones/:id` | Partial zone | Updated zone |
| `DELETE` | `/api/zones/:id` | — | `{"ok":true}` |
| `GET` | `/api/zones/:id/history` | `?period=24h` | `[{timestamp, count, people:[]}]` hourly buckets |
| `GET` | `/api/portals` | — | `[{id, name, zone_a, zone_b, plane:{points:[...]}}]` |
| `POST` | `/api/portals` | Portal geometry | Created portal |
| `PATCH` | `/api/portals/:id` | Partial | Updated portal |
| `DELETE` | `/api/portals/:id` | — | `{"ok":true}` |
| `GET` | `/api/portals/:id/crossings` | `?limit=50&before=<cursor>` | `[{timestamp, direction, person, blob_id}]` |
| `GET` | `/api/triggers` | — | `[{id, name, shape, condition, actions, enabled, last_fired}]` |
| `POST` | `/api/triggers` | Trigger object | Created trigger |
| `PATCH` | `/api/triggers/:id` | Partial | Updated trigger |
| `DELETE` | `/api/triggers/:id` | — | `{"ok":true}` |
| `POST` | `/api/triggers/:id/test` | — | Fires trigger action once with synthetic event |
| `POST` | `/api/triggers/:id/enable` | — | `{"ok":true}` |
| `POST` | `/api/triggers/:id/disable` | — | `{"ok":true}` |

### BLE Device Registry

| Method | Path | Request | Response |
|--------|------|---------|----------|
| `GET` | `/api/ble/devices` | `?registered=true` or `?discovered=true` | `[{addr, label, type, color, last_rssi, last_seen, auto_rotate}]` |
| `POST` | `/api/ble/devices` | `{addr, label, type, color, icon?}` | Created device |
| `PATCH` | `/api/ble/devices/:addr` | Partial | Updated device |
| `DELETE` | `/api/ble/devices/:addr` | — | `{"ok":true}` |

### Floor Plan

| Method | Path | Request | Response |
|--------|------|---------|----------|
| `GET` | `/api/floorplan` | — | `{image_url?, calibration:{point_a,point_b,real_distance_m}, room_bounds}` |
| `POST` | `/api/floorplan/image` | Multipart: `file=<PNG/JPG>` | `{"ok":true,"image_url":"/floorplan/image.png"}` |
| `PATCH` | `/api/floorplan/calibration` | `{point_a:{x,y},point_b:{x,y},real_distance_m}` | Updated calibration |
| `GET` | `/floorplan/image.png` | — | Raw image file |

### Events & Timeline

| Method | Path | Request | Response |
|--------|------|---------|----------|
| `GET` | `/api/events` | `?limit=50&before=<cursor>&type=<type>&zone=<name>&person=<name>&after=<iso8601>&q=<text>` | `{"events":[...],"cursor":"<next>","total":N}` |
| `GET` | `/api/events/:id` | — | Single event with full detail |

### Security Mode

| Method | Path | Request | Response |
|--------|------|---------|----------|
| `POST` | `/api/security/arm` | — | `{"ok":true,"security_mode":true}` — enables security mode; any detection = alert |
| `POST` | `/api/security/disarm` | — | `{"ok":true,"security_mode":false}` |
| `GET` | `/api/security` | — | `{"security_mode":bool,"armed_at":iso8601_or_null}` |

Security mode state is stored in the `settings` table as key `"security_mode"` (boolean JSON). The `armed_at` timestamp is stored as `"security_mode_armed_at"` (ISO8601 string). Both are cleared on disarm.

When security mode is armed via the MQTT `command/security_mode` subscription, it calls the same internal arm/disarm function as the REST endpoints.

### Localization & Predictions

| Method | Path | Request | Response |
|--------|------|---------|----------|
| `GET` | `/api/blobs` | — | Current blob list (snapshot of live state) |
| `GET` | `/api/predictions` | `?person=<name>&horizon=30m` | `[{zone, probability, horizon_min}]` |
| `GET` | `/api/occupancy` | — | `{zones:{<name>:{count, people:[]}}}` |

### Sleep & Analytics

| Method | Path | Request | Response |
|--------|------|---------|----------|
| `GET` | `/api/sleep` | `?person=<name>&limit=30` | `[{date, duration_m, onset_latency_m, restlessness_index, breathing_rate_avg, breathing_regularity}]` |
| `GET` | `/api/sleep/summary` | `?person=<name>` | Today's / last-night's summary |
| `GET` | `/api/flow` | `?period=24h&person=<name>` | `{cells:[{x,y,count,vx,vy,dwell_s}]}` |
| `GET` | `/api/localization/weights` | — | `[{link_id, weight}]` |
| `POST` | `/api/localization/weights/reset` | — | `{"ok":true}` |

### Feedback

| Method | Path | Request | Response |
|--------|------|---------|----------|
| `POST` | `/api/feedback` | `{type:"correct"\|"incorrect"\|"missed", blob_id?, position?:{x,y,z}, timestamp}` | `{"ok":true}` |

### Calibration / Baseline

| Method | Path | Request | Response |
|--------|------|---------|----------|
| `GET` | `/api/baseline` | — | `[{link_id, snapshot_time, confidence}]` |
| `POST` | `/api/baseline/capture` | `{links?:[link_id,...]}` | `{"ok":true,"links_captured":N}` — starts 60 s quiet-room capture |

### CSI Replay

| Method | Path | Request | Response |
|--------|------|---------|----------|
| `POST` | `/api/replay/start` | `{from_iso8601, to_iso8601}` | `{"session_id":"..."}` |
| `POST` | `/api/replay/seek` | `{session_id, timestamp_iso8601}` | `{"ok":true}` |
| `POST` | `/api/replay/play` | `{session_id, speed:1\|2\|5}` | `{"ok":true}` |
| `POST` | `/api/replay/pause` | `{session_id}` | `{"ok":true}` |
| `POST` | `/api/replay/stop` | `{session_id}` | `{"ok":true}` |
| `PATCH` | `/api/replay/params` | `{session_id, delta_rms_threshold?, tau_s?, fresnel_decay?, n_subcarriers?, breathing_sensitivity?}` | Re-runs pipeline; `{"ok":true}` |
| `POST` | `/api/replay/apply-params` | `{session_id}` | Copies tuned params to live pipeline |

### Notifications & Integrations

| Method | Path | Request | Response |
|--------|------|---------|----------|
| `GET` | `/api/notifications/channels` | — | `[{type, enabled, config}]` |
| `PATCH` | `/api/notifications/channels/:type` | Config object | Updated channel |
| `POST` | `/api/notifications/test` | `{channel_type}` | Sends test notification; `{"ok":true}` |

---

## SQLite Schema

All tables reside in a single `spaxel.db` file. Schema version is tracked in the `schema_migrations` table. Migrations are applied in order on startup. All timestamps are Unix milliseconds (INTEGER). STRICT mode enforced where SQLite version ≥ 3.37. Foreign keys are enabled (`PRAGMA foreign_keys = ON`).

```sql
-- Schema version tracking
CREATE TABLE IF NOT EXISTS schema_migrations (
    version     INTEGER PRIMARY KEY,
    applied_at  INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
);

-- System settings (key-value with typed values)
CREATE TABLE IF NOT EXISTS settings (
    key         TEXT PRIMARY KEY,
    value_json  TEXT NOT NULL,  -- JSON-encoded value (string, number, bool, array)
    updated_at  INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
);

-- Installation secrets and auth
CREATE TABLE IF NOT EXISTS auth (
    id              INTEGER PRIMARY KEY CHECK (id = 1),  -- singleton row
    install_secret  BLOB NOT NULL,    -- 32 bytes, random on first run
    pin_bcrypt      TEXT,             -- bcrypt hash of dashboard PIN; NULL = not set
    updated_at      INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
);

-- Dashboard sessions
-- Sessions are server-side records bound to the `spaxel_session` HTTP cookie.
-- Cookie value = session_id (32-byte random hex, 64 chars). The server validates
-- by looking up session_id here; if not found or expired, HTTP 401 is returned.
CREATE TABLE IF NOT EXISTS sessions (
    session_id  TEXT PRIMARY KEY,  -- 64-char hex (crypto/rand 32 bytes)
    created_at  INTEGER NOT NULL DEFAULT (unixepoch() * 1000),
    expires_at  INTEGER NOT NULL,  -- Unix ms; = created_at + 7 days (7×86400×1000)
    last_seen_at INTEGER NOT NULL DEFAULT (unixepoch() * 1000)  -- updated on every authenticated request
);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);
-- Expired sessions are purged in the background once per hour:
--   DELETE FROM sessions WHERE expires_at < unixepoch() * 1000
-- Session sliding window: last_seen_at is updated on every request.
-- If last_seen_at > expires_at - 1 day: extend expires_at by 7 more days (rolling session).
-- Cookie attributes: HttpOnly=true, SameSite=Strict (if TLS), Path=/, Max-Age=604800 (7 days)

-- Node registry
CREATE TABLE IF NOT EXISTS nodes (
    mac             TEXT PRIMARY KEY,  -- "AA:BB:CC:DD:EE:FF"
    node_id         TEXT UNIQUE,       -- UUID4 assigned at provisioning
    name            TEXT NOT NULL DEFAULT '',
    pos_x           REAL NOT NULL DEFAULT 0,  -- meters in floor plan coordinates
    pos_y           REAL NOT NULL DEFAULT 0,
    pos_z           REAL NOT NULL DEFAULT 1,
    role            TEXT NOT NULL DEFAULT 'tx_rx' CHECK (role IN ('tx','rx','tx_rx','passive','idle')),
    firmware_version TEXT,
    chip            TEXT,
    flash_mb        INTEGER,
    capabilities    TEXT,             -- JSON array of strings
    status          TEXT NOT NULL DEFAULT 'offline' CHECK (status IN ('online','stale','offline')),
    last_seen_ms    INTEGER,
    uptime_ms       INTEGER,
    wifi_rssi_dbm   INTEGER,
    free_heap_bytes INTEGER,
    temperature_c   REAL,
    ip              TEXT,
    created_at      INTEGER NOT NULL DEFAULT (unixepoch() * 1000),
    updated_at      INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
);

-- Per-link Fresnel zone weights (self-improving localization)
CREATE TABLE IF NOT EXISTS link_weights (
    link_id     TEXT PRIMARY KEY,  -- canonical form: min(MAC_a,MAC_b)+":"+max(MAC_a,MAC_b) for symmetric links; "AP_BSSID:NODE_MAC" for passive. Use CanonicalLinkID() to construct.
    weight      REAL NOT NULL DEFAULT 1.0,
    sample_count INTEGER NOT NULL DEFAULT 0,
    updated_at  INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
);

-- Baseline snapshots (per-link, per calibration event)
CREATE TABLE IF NOT EXISTS baselines (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    link_id     TEXT NOT NULL,
    captured_at INTEGER NOT NULL DEFAULT (unixepoch() * 1000),
    n_sub       INTEGER NOT NULL,
    amplitude   BLOB NOT NULL,  -- REAL[n_sub] little-endian float32 array
    phase       BLOB NOT NULL,  -- REAL[n_sub] little-endian float32 array
    confidence  REAL NOT NULL DEFAULT 0  -- 0.0–1.0; builds up as samples accumulate
);
CREATE INDEX IF NOT EXISTS idx_baselines_link ON baselines(link_id, captured_at DESC);

-- Diurnal baselines (24 hourly slots per link)
CREATE TABLE IF NOT EXISTS diurnal_baselines (
    link_id         TEXT NOT NULL,
    hour_of_day     INTEGER NOT NULL CHECK (hour_of_day BETWEEN 0 AND 23),
    n_sub           INTEGER NOT NULL,
    amplitude       BLOB NOT NULL,   -- REAL[n_sub] float32
    phase           BLOB NOT NULL,   -- REAL[n_sub] float32
    sample_count    INTEGER NOT NULL DEFAULT 0,
    confidence      REAL NOT NULL DEFAULT 0,
    updated_at      INTEGER NOT NULL DEFAULT (unixepoch() * 1000),
    PRIMARY KEY (link_id, hour_of_day)
);

-- BLE device registry
CREATE TABLE IF NOT EXISTS ble_devices (
    addr        TEXT PRIMARY KEY,  -- "AA:BB:CC:DD:EE:FF"
    label       TEXT NOT NULL DEFAULT '',
    type        TEXT NOT NULL DEFAULT 'person' CHECK (type IN ('person','pet','object')),
    color       TEXT NOT NULL DEFAULT '#888888',  -- CSS hex color
    icon        TEXT,
    auto_rotate INTEGER NOT NULL DEFAULT 0,  -- boolean: uses rotating addresses
    first_seen  INTEGER NOT NULL DEFAULT (unixepoch() * 1000),
    last_seen   INTEGER NOT NULL DEFAULT (unixepoch() * 1000),
    last_rssi   INTEGER,
    created_at  INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
);

-- Floor plan definition
CREATE TABLE IF NOT EXISTS floorplan (
    id              INTEGER PRIMARY KEY CHECK (id = 1),  -- singleton row
    image_path      TEXT,             -- relative to /data/ ; NULL = no image
    cal_ax          REAL, cal_ay REAL,  -- calibration point A (image pixel coords)
    cal_bx          REAL, cal_by REAL,  -- calibration point B
    cal_distance_m  REAL,             -- real-world distance between A and B
    room_bounds_json TEXT,            -- JSON: [{name, x, y, z, w, d, h}]
    updated_at      INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
);

-- Zones
CREATE TABLE IF NOT EXISTS zones (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL UNIQUE,
    x REAL, y REAL, z REAL,    -- origin corner (meters)
    w REAL, d REAL, h REAL,    -- width, depth, height
    zone_type   TEXT NOT NULL DEFAULT 'general'
                CHECK (zone_type IN ('general','bedroom','bathroom','living','exercise','kitchen','office','entry')),
    last_known_occupancy INTEGER NOT NULL DEFAULT 0,  -- persisted every 60 s and on shutdown; used for restart reconciliation
    occupancy_updated_at INTEGER,  -- Unix ms of last occupancy persistence; NULL = never persisted
    created_at  INTEGER NOT NULL DEFAULT (unixepoch() * 1000),
    updated_at  INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
);

-- Portals (doorway crossing detectors)
CREATE TABLE IF NOT EXISTS portals (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    zone_a_id   INTEGER REFERENCES zones(id) ON DELETE SET NULL,
    zone_b_id   INTEGER REFERENCES zones(id) ON DELETE SET NULL,
    points_json TEXT NOT NULL,  -- JSON: two floor points [[x1,y1],[x2,y2]]
    created_at  INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
);

-- Portal crossing log
CREATE TABLE IF NOT EXISTS portal_crossings (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    portal_id   INTEGER NOT NULL REFERENCES portals(id) ON DELETE CASCADE,
    timestamp_ms INTEGER NOT NULL,
    direction   TEXT NOT NULL CHECK (direction IN ('a_to_b','b_to_a')),
    blob_id     INTEGER,
    person      TEXT   -- resolved BLE label at time of crossing; NULL if unidentified
);
CREATE INDEX IF NOT EXISTS idx_crossings_portal ON portal_crossings(portal_id, timestamp_ms DESC);
CREATE INDEX IF NOT EXISTS idx_crossings_time ON portal_crossings(timestamp_ms DESC);

-- Trigger volumes (spatial automation)
CREATE TABLE IF NOT EXISTS triggers (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    name        TEXT NOT NULL,
    shape_json  TEXT NOT NULL,   -- JSON: {type:"box"|"cylinder", x,y,z, w,d,h | r}
    condition   TEXT NOT NULL CHECK (condition IN ('enter','leave','dwell','vacant','count')),
    condition_params_json TEXT,  -- JSON: {duration_s?, count_threshold?, person?}
    time_constraint_json TEXT,   -- JSON: {from:"22:00", to:"06:00"} or null
    actions_json TEXT NOT NULL,  -- JSON: [{type:"webhook"|"mqtt"|"internal", ...}]
    enabled     INTEGER NOT NULL DEFAULT 1,
    last_fired  INTEGER,
    created_at  INTEGER NOT NULL DEFAULT (unixepoch() * 1000),
    updated_at  INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
);

-- Events (unified timeline)
CREATE TABLE IF NOT EXISTS events (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp_ms INTEGER NOT NULL,
    type        TEXT NOT NULL,   -- 'detection','zone_entry','zone_exit','portal_crossing',
                                 -- 'trigger_fired','fall_alert','anomaly','security_alert',
                                 -- 'node_online','node_offline','ota_update','baseline_changed',
                                 -- 'system','learning_milestone'
    zone        TEXT,
    person      TEXT,
    blob_id     INTEGER,
    detail_json TEXT,            -- event-specific payload
    severity    TEXT NOT NULL DEFAULT 'info' CHECK (severity IN ('info','warning','alert','critical'))
);
CREATE INDEX IF NOT EXISTS idx_events_time ON events(timestamp_ms DESC);
CREATE INDEX IF NOT EXISTS idx_events_zone ON events(zone, timestamp_ms DESC);
CREATE INDEX IF NOT EXISTS idx_events_person ON events(person, timestamp_ms DESC);
CREATE INDEX IF NOT EXISTS idx_events_type ON events(type, timestamp_ms DESC);

-- Events archive (same schema as events; holds events older than 90 days)
-- Auto-archive runs nightly (02:00 local time) via a background goroutine:
--   INSERT INTO events_archive SELECT * FROM events WHERE timestamp_ms < (now_ms - 90d_ms)
--   DELETE FROM events WHERE timestamp_ms < (now_ms - 90d_ms)
-- Retention period configurable via settings key "events_archive_days" (default 90).
-- The /api/events endpoint queries ONLY the events table (not archive) for performance.
-- A separate endpoint GET /api/events/archive (same params) queries events_archive.
-- events_archive does NOT have an FTS5 index (archive search is slower but acceptable).
CREATE TABLE IF NOT EXISTS events_archive (
    id          INTEGER PRIMARY KEY,  -- preserved from original events.id
    timestamp_ms INTEGER NOT NULL,
    type        TEXT NOT NULL,
    zone        TEXT,
    person      TEXT,
    blob_id     INTEGER,
    detail_json TEXT,
    severity    TEXT NOT NULL DEFAULT 'info'
);
CREATE INDEX IF NOT EXISTS idx_events_archive_time ON events_archive(timestamp_ms DESC);

-- FTS5 index for natural-language search across event detail
CREATE VIRTUAL TABLE IF NOT EXISTS events_fts USING fts5(
    type, zone, person, detail_json,
    content='events', content_rowid='id'
);
-- Triggers to keep events_fts in sync with the events table
-- (required for content FTS5 tables per SQLite documentation)
CREATE TRIGGER IF NOT EXISTS events_fts_insert AFTER INSERT ON events BEGIN
    INSERT INTO events_fts(rowid, type, zone, person, detail_json)
    VALUES (new.id, new.type, new.zone, new.person, new.detail_json);
END;
CREATE TRIGGER IF NOT EXISTS events_fts_delete AFTER DELETE ON events BEGIN
    INSERT INTO events_fts(events_fts, rowid, type, zone, person, detail_json)
    VALUES ('delete', old.id, old.type, old.zone, old.person, old.detail_json);
END;
CREATE TRIGGER IF NOT EXISTS events_fts_update AFTER UPDATE ON events BEGIN
    INSERT INTO events_fts(events_fts, rowid, type, zone, person, detail_json)
    VALUES ('delete', old.id, old.type, old.zone, old.person, old.detail_json);
    INSERT INTO events_fts(rowid, type, zone, person, detail_json)
    VALUES (new.id, new.type, new.zone, new.person, new.detail_json);
END;
-- On startup, if events_fts is empty but events has rows (e.g., after a schema re-creation),
-- rebuild with: INSERT INTO events_fts(events_fts) VALUES ('rebuild');
-- This is checked in Phase 3/7 of startup by comparing COUNT(*) on both tables.

-- Detection feedback
CREATE TABLE IF NOT EXISTS feedback (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp_ms INTEGER NOT NULL,
    type        TEXT NOT NULL CHECK (type IN ('correct','incorrect','missed')),
    blob_id     INTEGER,
    position_json TEXT,          -- JSON {x,y,z} for 'missed' type
    links_json  TEXT,            -- JSON [link_id, ...] of contributing links at feedback time
    event_id    INTEGER REFERENCES events(id) ON DELETE SET NULL
);
CREATE INDEX IF NOT EXISTS idx_feedback_time ON feedback(timestamp_ms DESC);

-- Sleep records
CREATE TABLE IF NOT EXISTS sleep_records (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    person          TEXT,                  -- NULL = zone-based (no BLE identity)
    zone_id         INTEGER REFERENCES zones(id) ON DELETE SET NULL,
    date            TEXT NOT NULL,         -- "YYYY-MM-DD" (night start date)
    bed_time_ms     INTEGER,
    wake_time_ms    INTEGER,
    duration_min    INTEGER,
    onset_latency_min INTEGER,
    restlessness    REAL,                  -- 0.0–5.0
    breathing_rate_avg REAL,              -- breaths/min
    breathing_regularity REAL,            -- coefficient of variation
    summary_json    TEXT                  -- 30-min bucket breakdown as JSON array
);
CREATE INDEX IF NOT EXISTS idx_sleep_person ON sleep_records(person, date DESC);

-- Presence prediction models (per-person, per-zone, per-time-slot, per-day-type)
CREATE TABLE IF NOT EXISTS prediction_models (
    person      TEXT NOT NULL,
    zone_id     INTEGER NOT NULL REFERENCES zones(id) ON DELETE CASCADE,
    time_slot   INTEGER NOT NULL,      -- 0..95 (15-min buckets, 96 per day)
    day_type    TEXT NOT NULL CHECK (day_type IN ('weekday','weekend')),
    probability REAL NOT NULL DEFAULT 0,
    sample_count INTEGER NOT NULL DEFAULT 0,
    updated_at  INTEGER NOT NULL DEFAULT (unixepoch() * 1000),
    PRIMARY KEY (person, zone_id, time_slot, day_type)
);

-- Anomaly detection pattern model
CREATE TABLE IF NOT EXISTS anomaly_patterns (
    zone_id     INTEGER NOT NULL REFERENCES zones(id) ON DELETE CASCADE,
    hour_of_day INTEGER NOT NULL CHECK (hour_of_day BETWEEN 0 AND 23),
    day_of_week INTEGER NOT NULL CHECK (day_of_week BETWEEN 0 AND 6),
    mean_count  REAL NOT NULL DEFAULT 0,
    variance    REAL NOT NULL DEFAULT 0,
    sample_count INTEGER NOT NULL DEFAULT 0,
    updated_at  INTEGER NOT NULL DEFAULT (unixepoch() * 1000),
    PRIMARY KEY (zone_id, hour_of_day, day_of_week)
);

-- Crowd flow accumulator (per time bucket per grid cell)
CREATE TABLE IF NOT EXISTS crowd_flow (
    bucket_ms   INTEGER NOT NULL,   -- rounded to bucket boundary (1h or 1d)
    bucket_type TEXT NOT NULL CHECK (bucket_type IN ('hour','day','week')),
    cell_x      INTEGER NOT NULL,   -- grid cell x index
    cell_y      INTEGER NOT NULL,   -- grid cell y index
    entry_count INTEGER NOT NULL DEFAULT 0,
    vx_sum      REAL NOT NULL DEFAULT 0,  -- sum of velocity x components for average
    vy_sum      REAL NOT NULL DEFAULT 0,
    dwell_ms    INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (bucket_ms, bucket_type, cell_x, cell_y)
);

-- OTA firmware metadata
CREATE TABLE IF NOT EXISTS firmware (
    filename        TEXT PRIMARY KEY,
    version         TEXT NOT NULL,
    sha256          TEXT NOT NULL,
    size_bytes      INTEGER NOT NULL,
    uploaded_at     INTEGER NOT NULL DEFAULT (unixepoch() * 1000),
    is_latest       INTEGER NOT NULL DEFAULT 0  -- boolean; only one row has 1
);

-- Morning briefing records (one per day)
CREATE TABLE IF NOT EXISTS briefings (
    date        TEXT PRIMARY KEY,   -- "YYYY-MM-DD"
    content     TEXT NOT NULL,      -- rendered text
    generated_at INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
);

-- Notification channel config
CREATE TABLE IF NOT EXISTS notification_channels (
    type        TEXT PRIMARY KEY,  -- 'ntfy','pushover','gotify','webhook','mqtt'
    enabled     INTEGER NOT NULL DEFAULT 0,
    config_json TEXT NOT NULL DEFAULT '{}'  -- channel-specific config (URLs, tokens); see below
);

/*
  config_json schemas per channel type:

  ntfy:
    {"url":"https://ntfy.sh/my-topic", "token":"tk_..."}  -- token optional (for private topics)
    HTTP call: POST <url>
      Headers: Authorization: Bearer <token> (if set), Title: <title>, Priority: urgent|high|default,
               X-Attach: <image_url_or_base64>   (for floor plan thumbnail)
      Body: <text>

  pushover:
    {"app_token":"aXXXXXX...","user_key":"uXXXXXX..."}
    HTTP call: POST https://api.pushover.net/1/messages.json
      Body (form-encoded): token=<app_token>&user=<user_key>&title=<title>&message=<text>
                           &attachment=<base64_png>   (for floor plan thumbnail, max 2.5 MB)
                           &priority=1 (high) or 2 (emergency — requires retry+expire)

  gotify:
    {"url":"https://gotify.example.com","token":"Aq7mXXXX"}
    HTTP call: POST <url>/message?token=<token>
      Body (JSON): {"title":"<title>","message":"<text>","priority":7}
      Note: Gotify does not support image attachments natively; thumbnail is omitted.

  webhook:
    {"url":"https://example.com/hook","method":"POST","headers":{"X-Secret":"abc"}}
    HTTP call: POST/GET <url> with optional headers
      Body (JSON): same payload as trigger webhook (see Spatial Automation Builder)
      Plus: "event_type":"fall_alert"|"anomaly"|"zone_entry"|...

  mqtt:
    (uses the global MQTT connection from SPAXEL_MQTT_BROKER; no separate config)
    No config_json fields required; this channel is automatically enabled when MQTT is configured.
*/

-- CSI replay session state (ephemeral; cleared on restart)
CREATE TABLE IF NOT EXISTS replay_sessions (
    session_id  TEXT PRIMARY KEY,
    from_ms     INTEGER NOT NULL,
    to_ms       INTEGER NOT NULL,
    current_ms  INTEGER NOT NULL,
    speed       INTEGER NOT NULL DEFAULT 1,
    state       TEXT NOT NULL DEFAULT 'paused' CHECK (state IN ('playing','paused','stopped')),
    params_json TEXT,              -- tuned pipeline params; NULL = use live params
    created_at  INTEGER NOT NULL DEFAULT (unixepoch() * 1000)
);
```

**CSI Replay Store (append-only file, not SQLite):**

The CSI replay buffer is stored as an append-only binary file at `/data/csi_replay.bin` because SQLite's row-per-frame model would be too slow for the write rate (~30 Hz × 20 links = 600 frames/s).

```
File header (64 bytes):
  magic:       8 bytes  — 0x535041584C525000 ("SPAXLRP\0")
  version:     4 bytes  — uint32, currently 1
  write_pos:   8 bytes  — current write position (byte offset past last complete frame)
  oldest_pos:  8 bytes  — byte offset of oldest retained frame (for ring-buffer eviction)
  reserved:    36 bytes — zeroed, reserved for future use

Per-frame record (variable length):
  recv_time_ms:  8 bytes  — mothership receive time (Unix ms, int64)
  frame_len:     2 bytes  — length of the CSI WebSocket binary frame that follows (uint16)
  frame_data:    N bytes  — raw CSI binary frame (same format as the WebSocket binary frame)
```

On startup, `write_pos` is read from the header to resume appending. If the file header has a mismatched magic or truncated write, the file is truncated to the last complete frame (detected by scanning backward from `write_pos`). Eviction: when the file grows beyond the configured size limit (default 360 MB for 48 h retention), `oldest_pos` advances past the oldest frames in a block-eviction loop.

---

## Resource Limits & Performance Budgets

### Minimum Host Requirements

| Fleet Size | Nodes | Links | Min RAM | Min CPU | Disk (48h CSI + DB) |
|------------|-------|-------|---------|---------|---------------------|
| Minimal | 2 | 2 | 256 MB | 1 core (any) | 50 MB |
| Small | 4 | 6 | 512 MB | 1 core (Pi 4 class) | 150 MB |
| Medium | 8 | 28 | 512 MB | 2 cores | 420 MB |
| Large | 16 | 120 | 1 GB | 4 cores | 1.5 GB |

Tested minimum: Raspberry Pi 4 (4 GB RAM) running a 6-node fleet at 20 Hz with <10% CPU idle.

### Docker Compose Resource Limits

Add to the compose service definition for production deployments:

```yaml
    deploy:
      resources:
        limits:
          memory: 512m      # Increase to 1g for 16+ node fleets
          cpus: "2.0"       # Adjust to leave headroom for the host OS
        reservations:
          memory: 128m
          cpus: "0.5"
```

Memory breakdown (8-node fleet):
- Ring buffers: 28 links × 256 samples × 152 bytes/frame = ~1.1 MB
- SQLite page cache: ~20 MB (default)
- Go runtime + binary: ~30 MB
- Crowd flow accumulator: ~5 MB
- Dashboard WebSocket state (per client): ~1 MB
- Total: ~60 MB baseline, peaks to ~150 MB during full-rate operation

### Pipeline Timing Budgets

The fusion loop runs at 10 Hz (100 ms budget per iteration). Per-stage targets:

| Stage | Target | Hard Limit |
|-------|--------|------------|
| Phase sanitization (per link) | <1 ms | 3 ms |
| Feature extraction (per link) | <0.5 ms | 2 ms |
| Fresnel grid accumulation (all links) | <5 ms | 15 ms |
| Peak extraction | <2 ms | 5 ms |
| UKF update (per blob) | <1 ms | 3 ms |
| BLE matching | <1 ms | 3 ms |
| Trigger evaluation | <1 ms | 3 ms |
| Dashboard WebSocket publish | <2 ms | 5 ms |
| Total per fusion iteration | <15 ms | 40 ms |

If any stage exceeds its hard limit, a warning is logged. If the total iteration time exceeds 80 ms (cutting into the next iteration budget), the system enters **load-shedding mode**.

### Load Shedding Policy

When the pipeline is running consistently >80 ms per 100 ms iteration (measured as a 5-iteration rolling average):

1. **Level 1 (>80ms):** Suspend crowd flow accumulation (saves ~3 ms/iter). Log warning.
2. **Level 2 (>90ms):** Suspend CSI replay buffer writes (saves ~2 ms/iter). Alert in dashboard.
3. **Level 3 (>95ms):** Drop CSI frames that arrive when the processing channel is >50% full. Reduce all node rates to 10 Hz via config push. Alert in dashboard: "System under load — CSI rate reduced."
4. **Recovery:** When load drops below 60ms for 10 consecutive iterations, restore normal operation in reverse order.

Load-shedding status is visible in the `/api/health` response and the dashboard status bar.

### Bounded Resource Invariants

These bounds hold regardless of fleet size or runtime:
- Per-link ring buffer: capped at 256 samples (~152 bytes × 256 = 38 KB per link)
- Dashboard WebSocket send queue per client: capped at 50 frames (~500 KB); oldest dropped if client is slow
- Concurrent OTA updates: maximum 3 nodes simultaneously (controlled by rolling update logic)
- SQLite WAL file: checkpoint triggered when WAL exceeds 1000 pages (~4 MB); forced checkpoint on shutdown
- Events table: auto-archive (move to `events_archive` table) events older than 90 days; configurable via settings
- CSI replay store: default 360 MB / 48 h; configurable via `SPAXEL_REPLAY_MAX_MB` env var
- Maximum concurrent dashboard clients: 10 (configurable; beyond this, new connections are queued)

### Disk Full Handling

When the `/data` filesystem has less than 100 MB free:
1. Stop CSI replay buffer writes (highest volume I/O)
2. Emit a system alert event and dashboard warning
3. If disk drops below 20 MB free: also pause crowd flow accumulation writes and prediction model updates
4. Detection and localization continue normally regardless of disk state
5. Dashboard shows a "Disk space low" banner with current usage breakdown

---

## Upgrade Path

### Versioning Policy

Spaxel follows semantic versioning (MAJOR.MINOR.PATCH):
- **PATCH:** Bug fixes only. No schema changes. Safe to apply without any migration steps.
- **MINOR:** New features. Schema changes are additive only (new nullable columns, new tables). Migration runs automatically on startup. All existing data is preserved. Nodes running firmware from the same MAJOR version continue to work.
- **MAJOR:** May include breaking schema changes or protocol changes. A migration guide is published with each major release. Firmware must be updated to the same MAJOR version within 30 days (mothership logs a warning for nodes on a previous major version's firmware).

### Firmware–Mothership Compatibility

| Mothership Version | Compatible Firmware Versions |
|--------------------|------------------------------|
| 1.x | 1.x (any minor) |
| 2.x | 2.x, 1.x (read-only degraded mode) |

Compatibility check: on `hello`, the mothership compares firmware major version to its own. If incompatible (future major version firmware connecting to older mothership), the node is accepted but its role is set to IDLE and a warning is shown in the dashboard. If a previous major version's firmware connects, it continues to work with a deprecation warning.

### Mothership Upgrade Procedure

```bash
# 1. Pre-upgrade backup (automatic on startup, but also do manually):
docker exec spaxel wget -qO- http://localhost:8080/api/backup > spaxel-backup-$(date +%Y%m%d).zip

# 2. Pull new image
docker compose pull

# 3. Restart (migrations run automatically on startup)
docker compose up -d

# 4. Verify upgrade
docker compose logs spaxel --tail=50
# Look for: "Schema migration applied: version X → Y" and "All systems ready"
```

**Automatic pre-migration backup:** On startup, if the schema version in SQLite differs from the compiled-in version, the mothership automatically creates a backup at `/data/backups/pre-upgrade-v<old>-to-v<new>-<timestamp>.sqlite` before running any migrations. This backup is a full SQLite copy (using the SQLite Online Backup API). Backups older than 90 days are automatically pruned.

### Rollback Procedure

```bash
# Stop the new version
docker compose stop

# Restore the pre-upgrade database backup
cp /var/lib/docker/volumes/spaxel-data/_data/backups/pre-upgrade-*.sqlite \
   /var/lib/docker/volumes/spaxel-data/_data/spaxel.db

# Restart with the previous image tag
# (edit docker-compose.yml to pin: image: ghcr.io/spaxel/spaxel:1.2.3)
docker compose up -d
```

Note: The CSI replay store (`csi_replay.bin`) format is append-only and forward-compatible across all versions. It does not need to be restored during rollback.

### Schema Migration Framework

Each migration is a numbered Go function registered in a migrations slice:

```go
// Migrations applied in order. Each migration is idempotent.
// The schema_migrations table tracks which have been applied.
var migrations = []Migration{
    {Version: 1, Up: migration_001_initial_schema},
    {Version: 2, Up: migration_002_add_diurnal_baselines},
    // ...
}
```

Each `Up` function runs in a SQLite transaction. If it fails, the transaction is rolled back, the pre-migration backup is preserved, and the mothership exits with a clear error message. A failed migration never leaves the database in a partially-migrated state.

---

## Phase Plan

### Phase 1 — Foundation

Goal: Bare-minimum loop from ESP32 to browser. Zero-config with passive radar and mDNS from day one.

1. **ESP32 firmware skeleton** — WiFi connect, mDNS mothership discovery, CSI capture in promiscuous mode, single WebSocket connection to mothership (`/ws/node`) carrying binary CSI frames upstream and JSON config downstream
2. **Passive radar support** — Firmware accepts `passive_bssid` config to filter CSI from existing WiFi AP. Auto-detected during guided first run
3. **BLE scanning** — Passive BLE advertisement scanning on Core 0, concurrent with WiFi. Report device list as JSON on the WebSocket every 5 s
4. **Mothership WebSocket ingestion** — Go service with `/ws/node` endpoint that accepts bidirectional node connections, parses binary/JSON frames, mDNS service advertisement (`_spaxel._tcp.local`)
5. **Dashboard skeleton** — Static HTML/JS + Three.js served by mothership. 3D scene with ground grid, OrbitControls (pan/zoom/rotate), `/ws/dashboard` WebSocket connection. Render raw amplitude bar chart as a 2D overlay for a single link
6. **Docker packaging** — Single Dockerfile, `docker-compose.yml` with single port mapping (8080 HTTP/WS). Traefik labels included

**Exit criteria:** Flash firmware via Web Serial → plug in → node auto-discovers mothership → passive radar CSI streaming → amplitude bars visible in browser. Under 5 minutes, zero manual network config.

### Phase 2 — Signal Processing & Detection

Goal: Detect presence on a single link.

7. **Phase sanitisation** — Implement in Go: unwrap, linear regression, STO/CFO removal
8. **Baseline system** — EMA baseline with motion-gated updates, SQLite persistence
9. **Motion detection** — deltaRMS computation, threshold-based presence flag per link
10. **Dashboard presence indicator** — Simple per-link "motion detected" / "clear" display with amplitude time series plot
11. **CSI recording buffer** — Append incoming CSI frames to disk-backed circular buffer (48 h default). Foundation for time-travel replay
12. **Adaptive sensing rate** — Mothership-controlled rate changes (idle 2 Hz ↔ active 50 Hz) per link. On-device amplitude variance check for local burst-to-active. Motion hints from ESP32 to preemptively ramp adjacent links

**Exit criteria:** Dashboard reliably shows motion detected / clear for a single link with one person walking through. Idle links automatically drop to 2 Hz.

### Phase 3 — Multi-Node & Localization

Goal: Spatial positioning with 4+ nodes. Humanoid blob rendering from the start.

13. **Bidirectional node protocol** — Registration (`hello`), health reporting, BLE scan relay, role/config/rate push, OTA commands — all over the existing WebSocket connection
14. **Fleet manager** — Node registry in SQLite, role assignment engine (including passive radar virtual node), stagger scheduling, self-healing role reassignment on node loss
15. **Multi-link fusion** — Fresnel zone weighted localization on a 3D grid
16. **Biomechanical blob tracking** — Peak extraction, ID assignment, UKF with human motion constraints (max velocity, acceleration, turning radius, gravity-consistent Z, collision avoidance, persistence through brief association gaps)
17. **3D spatial visualization** — Room bounds, floor plan texture, humanoid figures (standing/walking/seated/lying postures via `SkinnedMesh` + `AnimationMixer`), vertical pillar anchors, footprint trails, node meshes, link lines, view presets
18. **Node placement UI** — TransformControls for dragging nodes in 3D, space dimension editor
19. **Live coverage painting** — GDOP overlay on ground plane, updates in real-time during node drag. Virtual node support for planning

**Exit criteria:** 4+ nodes produce a 3D view with humanoid figures tracking a walking person at ±1 m accuracy. Figures animate between postures. User can orbit, pan, and zoom. Coverage overlay shows detection quality.

### Phase 4 — Onboarding & OTA

Goal: Non-technical users can add and update nodes. Interactive guided wizard that teaches by doing.

20. **Interactive onboarding wizard (Component 33)** — Flash firmware via Web Serial → node auto-discovers mothership via mDNS → wizard responds to live sensor data: "Walk around" (see CSI waveform react), "Stand still" (capture baseline), "Walk through the detection zone" (see Fresnel zone light up), "Let me find you" (blob appears), "Place your node" (coverage painting guides optimal position). 2-minute hands-on tutorial, no jargon
21. **Provisioning payload** — Mothership generates config blob (WiFi creds + node ID, no IP needed), firmware writes to NVS
22. **OTA system** — HTTP firmware serving, WebSocket-triggered updates, rolling update logic with 30 s gaps, automatic rollback
23. **Captive portal recovery** — AP fallback mode on WiFi failure, config page for re-provisioning (WiFi creds + optional manual mothership IP)
24. **Guided troubleshooting foundation (Component 36)** — First-time feature discovery tooltips. Node-offline troubleshooting steps in timeline. Post-calibration positive reinforcement messages

**Exit criteria:** A new ESP32-S3 can go from unboxed to streaming CSI in under 5 minutes with the user understanding HOW detection works. Firmware can be updated OTA without physical access.

### Phase 5 — Reliability & Intelligence

Goal: Production-quality detection for daily home use.

24. **Diurnal adaptive baseline** — 24-slot hourly baseline vectors, 7-day learning period, automatic crossfade. Baseline confidence indicator per link in dashboard
25. **Stationary person detection** — Breathing band extraction (0.1–0.5 Hz), long-dwell logic
26. **Ambient confidence score** — Per-link health metrics (SNR, phase stability, packet rate, drift), composite system-wide "Detection Quality" gauge. Link thickness/color in 3D view reflects health
27. **Self-healing fleet** — Automatic role re-optimization on node loss/recovery, before/after coverage comparison, graceful degradation warnings
28. **Link weather diagnostics** — Root-cause suggestions for degraded links, weekly reliability trends, node repositioning advice with highlighted positions in 3D

**Exit criteria:** System runs unattended for 7+ days with <5% false positive rate, surviving node reboots, WiFi blips, and diurnal environmental changes.

### Phase 6 — Identity & Spatial Automation

Goal: Named presence, actionable automations, and safety features. Natural language notifications from day one.

29. **BLE device registry** — "People & Devices" dashboard panel. Discovered BLE devices listed with auto-detected type (iPhone, Watch, Tile, etc.). User assigns labels ("Alice", "Dog Tracker", "Car Keys"), type (person/pet/object), and color. Multiple devices can map to one person
30. **BLE-to-blob identity matching** — Multi-node RSSI triangulation matched to nearest CSI blob. Humanoid figures gain per-person color and name label. Dashboard shows "Alice is in Kitchen" instead of "Blob #2"
31. **Room transition portals** — Doorway planes in 3D editor, directional crossing detection, per-zone occupancy counters with person names. Zone labels in 3D view: "Kitchen: Alice, Bob"
32. **Spatial automation builder** — 3D trigger volumes with conditions (enter/leave/dwell/vacant/count + optional person filter: "when Alice enters..."). Webhook and MQTT actions. Visual feedback when triggers fire
33. **Fall detection** — Z-axis rapid descent + sustained stillness. Alert chain: dashboard alarm → webhook → push notification → escalation. Person-identified alerts when BLE available: "Fall detected: Alice in Hallway"
34. **Spatial context notifications (Component 30)** — Push notifications with rendered mini floor-plan thumbnails (PNG, server-side 2D renderer) and natural language text. Smart batching (collapse rapid-fire events). Quiet hours. Configurable delivery channels (Ntfy/Pushover/webhook)
35. **Home automation integration** — Optional MQTT client for HA auto-discovery (per-person presence sensors, zone occupancy, fall alerts). Webhook support for non-MQTT setups

**Exit criteria:** BLE-identified blobs show correct person names. Notifications include floor-plan thumbnails with person names. Room transition counts match manual observation within ±1. Fall detection fires on simulated falls with <10% false positive rate.

### Phase 7 — Learning & Analytics

Goal: The system gets smarter over time. User feedback drives improvement.

36. **Detection feedback loop (Component 29)** — Thumbs up/down on every detection (3D view, timeline, notifications). "I was here" missed-detection marking. Feedback adjusts Fresnel weights and detection thresholds. Accuracy trend tracking: "You've provided 47 corrections. Accuracy improved 12%"
37. **Self-improving localization** — BLE proximity as continuous ground truth drives per-link Fresnel weight refinement. Accuracy trend graph in dashboard. Weights persist in SQLite
38. **Presence prediction** — Per-person, per-zone, per-time-slot transition probabilities learned over 7+ days. Dashboard predictions widget. REST API. New `predicted_enter` automation trigger type. HA prediction sensors
39. **Sleep quality monitoring** — Breathing analysis + motion scoring in bedroom zones. Morning summary card, weekly trends, anomaly flagging. Per-person when BLE available
40. **Crowd flow visualization** — Trajectory accumulation into directional flow map. Animated arrows for corridors, dwell hotspot pools. Time and person filters. Toggle-able 3D layer
41. **Anomaly detection & security mode** — 7-day pattern learning, anomaly scoring, security mode with full alert chain, "Away" auto-activation

**Exit criteria:** Accuracy trend graph shows measurable improvement over 4 weeks. User feedback visibly improves detection within 48 hours. Presence predictions achieve >75% accuracy at 15-minute horizon.

### Phase 8 — Analysis & Developer Tools

Goal: Deep debugging, system tuning, and detection explainability.

42. **Activity timeline (Component 27)** — Universal event stream: detections, transitions, alerts, system events, learning milestones. Tap any event → 3D view jumps to that moment. Inline feedback buttons. Search and filter. Timeline sidebar in expert mode, activity feed in simple mode
43. **Detection explainability (Component 28)** — "Why is this here?" on any blob/alert: X-ray overlay dims non-contributing elements, glows contributing links with Fresnel zone intersection, shows BLE match details and confidence breakdown
44. **Time-travel debugging** — Pause live view, scrub timeline, replay 3D scene from recorded CSI. Parameter tuning overlay with live re-processing. "Apply to Live" button. Integrated with activity timeline for navigation
45. **Pre-deployment simulator** — Virtual space + virtual nodes + synthetic walkers. GDOP overlay, accuracy estimates, minimum node recommendation, shopping list output
46. **CSI simulator** — Go CLI tool (`cmd/sim/main.go`) that opens WebSocket connections as virtual nodes and sends synthetic CSI binary frames (with optional simulated BLE) for development/testing without hardware.

   **Command-line interface:**
   ```
   spaxel-sim \
     --mothership ws://localhost:8080/ws/node \
     --token <node_token>               \  # HMAC from install_secret + mac
     --nodes 4                          \  # number of virtual nodes to simulate
     --walkers 1                        \  # number of walking persons to simulate
     --rate 20                          \  # CSI Hz per node
     --duration 60s                     \  # run for N seconds (0 = forever)
     --ble                              \  # also send simulated BLE advertisements
     --seed 42                          \  # random seed for reproducible runs
     --space "6x5x2.5"                  \  # room dimensions in meters (WxDxH)
   ```

   **Synthetic CSI frame generation:**
   - Each virtual node has a fixed position in the simulated space (placed at corners, evenly distributed)
   - Each walker follows a random walk: Gaussian velocity updates (σ = 0.3 m/s per axis per 50 ms step), reflected at room walls
   - For each TX→RX link pair at each tick, compute `amplitude` and `phase` using the same propagation model as the pre-deployment simulator (Component 17: path-loss + wall penetration + first-order reflection)
   - Inject Gaussian noise: `amplitude_noisy[k] = amplitude × (1 + N(0, 0.05))`, `phase_noisy[k] = phase + N(0, 0.1)`
   - Serialize into the 24-byte binary frame format with `n_sub = 64`, populating all fields. `rssi = clamp(-30 - path_loss_dB, -90, -30)`. `noise_floor = -95`
   - `timestamp_us` increments at the configured rate starting from 1000 (simulates ~1 ms boot time)

   **Simulated BLE:** When `--ble` is set, one virtual node per 5 s sends a `{type:"ble", devices:[{addr:"AA:BB:CC:DD:EE:FF", rssi: -60 + N(0,5), name:"SimPerson"}]}` JSON frame. The BLE address matches the walker's simulated phone. No address rotation in simulation mode.

   **Verification:** The simulator exits non-zero if it receives a `{type:"reject"}` downstream message (authentication or rate limiting). It prints per-second frame counts and the mothership's blob count (from a parallel `GET /api/blobs` poll) to stdout for integration test assertions.

   **Integration test usage:**
   ```bash
   # Start mothership
   docker run -d -p 8080:8080 --name spaxel-test ghcr.io/spaxel/spaxel:latest
   # Run simulator for 30 s
   spaxel-sim --mothership ws://localhost:8080/ws/node --nodes 4 --walkers 1 --duration 30s
   # Assert blob count > 0
   curl -s http://localhost:8080/api/blobs | jq '.| length > 0'
   ```
47. **Fresnel zone debug overlay** — Toggle wireframe ellipsoids between active links in the 3D scene

**Exit criteria:** Tapping "Why?" on any detection shows a clear visual explanation of contributing links. Time-travel replay successfully replays 24 h of data. Simulator produces realistic synthetic data.

### Phase 9 — UX Polish & Accessibility

Goal: Accessible to every household member. Power user efficiency. Always-on ambient display.

48. **Simple mode (progressive disclosure)** — Card-based mobile-first UI with room occupancy cards, activity feed (from timeline), alert banner, sleep summary, morning briefing card. No 3D scene. Toggle between simple/expert mode. Optional PIN for expert mode
49. **Ambient dashboard mode (Component 31)** — `/ambient` route for wall-mounted tablets. Simplified top-down floor plan with colored dots and names. Time-of-day palette. Auto-dim when empty. Alert mode breaks the calm. Morning briefing on first detection. Lightweight Canvas 2D renderer
50. **Spatial quick actions (Component 32)** — Right-click / long-press context menus on every 3D element. Actions on blobs, nodes, empty space, zones, portals, trigger volumes. "Follow" camera mode on people
51. **Command palette (Component 34)** — Ctrl+K / Cmd+K universal search and command interface. Search zones/people/nodes/events. Navigate time. Execute commands. Get help. Fuzzy matching. Expert mode only
52. **Morning briefing (Component 35)** — Daily summary card on first dashboard open: sleep report, overnight events, system health, today's predictions. Also deliverable as push notification or webhook
53. **Guided troubleshooting (Component 36)** — Proactive contextual help when detection quality drops, settings are repeatedly changed, or nodes go offline. Post-feedback explanations. First-time feature tooltips. Never blocks, never repeats, never condescends
54. **Mobile-responsive expert mode** — Touch orbit/pan/zoom, hamburger menu for panels
55. **Fleet status page** — Full table view with all node metrics, bulk actions, camera fly-to on click

**Exit criteria:** Non-technical household member can use simple mode to check occupancy without training. Ambient mode runs unattended on a wall-mounted tablet for 7+ days. Command palette reaches any feature in ≤3 keystrokes. Morning briefing accurately summarizes overnight activity.

---

## Startup Sequencing & Graceful Shutdown

### Startup Phases

The mothership starts in strict sequential phases. Each phase logs its completion at INFO level. If any phase fails, the process exits non-zero with a clear error message.

```
Phase 1/7 — Data directory: verify /data is writable; acquire flock on /data/.lock to prevent duplicate instances
Phase 2/7 — SQLite: open database with PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL; PRAGMA foreign_keys=ON
             On corrupt DB detected: move aside to /data/spaxel.db.corrupt.<timestamp>, start fresh, log warning
             Run PRAGMA integrity_check on every start; on failure, move aside and start fresh
Phase 3/7 — Schema migration: apply pending migrations in order; rollback on failure
Phase 4/7 — Config & secrets: load/generate SPAXEL_INSTALL_SECRET; validate all env vars against schema
Phase 5/7 — Subsystems: start ingestion server, signal pipeline, fleet manager, fusion engine — in that order
             Each subsystem reports ready or fatal within 5 s
Phase 6/7 — HTTP server: bind to :8080; register all routes. mDNS advertisement starts only after bind succeeds.
             mDNS library: github.com/hashicorp/mdns (pure Go, no cgo, no OS mDNS daemon dependency).
             Service registration:
               mdns.NewMDNSService(
                 instance = SPAXEL_MDNS_NAME,   // default "spaxel"
                 service  = "_spaxel._tcp",
                 domain   = "local.",
                 hostName = "",                 // use system hostname
                 port     = 8080,
                 ips      = nil,                // all non-loopback interfaces
                 txt      = ["version=1","ws=/ws/node","api=/api"],
               )
             The TXT records allow future nodes to auto-discover the WS path and API prefix.
             mdns.NewServer(config) starts the responder goroutine. On shutdown: server.Shutdown().
Phase 7/7 — Health: POST /healthz returns 200 JSON {"status":"ok","nodes":N}. Announce readiness to stdout
```

Startup timeout: if phases 1–6 don't complete within 30 s, the process exits with a clear error. This prevents a zombie container that's bound but not functional.

### Graceful Shutdown

SIGTERM triggers an ordered shutdown with a 30-second hard deadline:

```
1. Stop accepting new node WebSocket connections (return HTTP 503 on upgrade attempts)
2. Send {type: "shutdown", reconnect_in: 30} to all connected dashboard WebSocket clients
3. Stop the fusion loop — no new blobs published
4. Drain the signal processing pipeline — process all frames already in the channel buffer
5. Flush all in-memory baselines to SQLite (atomic transaction)
6. Flush the CSI recording write buffer to disk
7. Close all node WebSocket connections (nodes will auto-reconnect after restart)
8. Write a "system_stopped" event to the SQLite events table
9. Run PRAGMA wal_checkpoint(FULL) to collapse WAL into main DB file
10. Close SQLite; release flock; exit 0
```

If any step exceeds the 30-second total deadline, the process force-exits (exit 1). Docker's `stop_grace_period: 35s` in compose gives the full 30 s.

### SQLite Durability

- WAL mode: crash-safe writes; readers don't block writers
- Per-baseline-snapshot writes use SQLite transactions (BEGIN → INSERT/REPLACE → COMMIT)
- Baseline snapshots are persisted every 60 s in addition to on shutdown (prevents losing up to 60 s of learning on crash)
- CSI recording buffer: append-only file with a write cursor. On restart, the cursor is recovered from the file header. An incomplete final write is truncated on open
- Atomic file writes (temp + rename) used for any non-SQLite persistent files (floor plan images, firmware metadata)

### Health & Observability

- `GET /healthz` — returns `{"status":"ok","uptime_s":N,"nodes_online":N,"db":"ok"}` or `{"status":"degraded","reason":"..."}`. HTTP 200 on healthy, 503 on degraded. Used by Docker `HEALTHCHECK` and optional Traefik health routing
- All subsystems use Go's `errgroup` for goroutine lifecycle. Panics in subsystem goroutines are recovered, logged, and the subsystem is marked DEGRADED in the health response
- Process logs include version string, data directory, and listening port on startup for support diagnostics

---

## Deployment

### Environment Variables

All environment variables are optional unless marked (required on production). Unset = use default.

| Variable | Default | Description |
|----------|---------|-------------|
| `SPAXEL_BIND_ADDR` | `0.0.0.0:8080` | Listen address. Set to `127.0.0.1:8080` to restrict to localhost (e.g., when behind a local reverse proxy) |
| `SPAXEL_INSTALL_SECRET` | *(auto-generated)* | 64-char hex installation secret. Auto-generated on first run and stored in SQLite. Override for scripted deployments |
| `SPAXEL_DATA_DIR` | `/data` | Path to the persistent data directory (SQLite, floor plans, CSI replay buffer, firmware uploads) |
| `SPAXEL_FIRMWARE_DIR` | `/firmware` | Path to the firmware binaries directory for OTA |
| `SPAXEL_MQTT_BROKER` | *(disabled)* | MQTT broker URL: `mqtt://host:1883` or `mqtts://host:8883`. If unset, MQTT integration is disabled |
| `SPAXEL_MQTT_USERNAME` | *(none)* | MQTT broker username |
| `SPAXEL_MQTT_PASSWORD` | *(none)* | MQTT broker password |
| `SPAXEL_MQTT_PREFIX` | `spaxel` | MQTT topic prefix |
| `SPAXEL_MQTT_CLIENT_ID` | `spaxel-<install_id>` | MQTT client ID |
| `TZ` | `UTC` | Timezone for diurnal baselines, morning briefings, quiet hours, auto-update scheduling. Use IANA tz names (e.g., `America/New_York`, `Europe/London`) |
| `SPAXEL_REPLAY_MAX_MB` | `360` | Maximum size of the CSI replay buffer in MB (48h at 8 nodes / 20 Hz) |
| `SPAXEL_REPLAY_RETAIN_H` | `48` | CSI replay retention in hours. Eviction is size-based (`REPLAY_MAX_MB`), this is advisory |
| `SPAXEL_MAX_DASHBOARD_CLIENTS` | `10` | Maximum concurrent dashboard WebSocket clients |
| `SPAXEL_NODE_STALE_S` | `15` | Seconds since last health report before a connected node is marked STALE |
| `SPAXEL_LOG_LEVEL` | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `SPAXEL_SKIP_MIGRATIONS` | `false` | Set to `true` to skip automatic schema migrations (advanced; for manual migration management) |
| `SPAXEL_FUSION_RATE_HZ` | `10` | Fusion loop rate in Hz. Reduce for lower CPU use; increase for smoother tracking (max 20) |
| `SPAXEL_GRID_CELL_M` | `0.2` | Fresnel zone accumulation grid cell size in meters |
| `SPAXEL_MDNS_NAME` | `spaxel` | mDNS service name advertised to nodes. Must match firmware `ms_mdns` NVS key |
| `SPAXEL_NTP_SERVER` | `pool.ntp.org` | NTP server hostname embedded in the provisioning payload. Nodes use this for clock synchronization for TX stagger slots. Set to a local NTP server (e.g., router IP) for networks without internet access |
| `SPAXEL_MDNS_ENABLED` | `true` | Set to `false` to disable mDNS advertisement (e.g., when using Docker bridge networking instead of `network_mode: host`). Nodes must then use the cached `ms_ip` NVS key or captive portal IP entry for mothership discovery |

### Dockerfile

Multi-stage build. SQLite is accessed via the pure-Go `modernc.org/sqlite` driver (no CGO, no `gcc` needed in the final image). This keeps the image small and enables `linux/amd64` + `linux/arm64` builds without cross-compilation complexity.

```dockerfile
# Stage 1: Build the Go binary
FROM golang:1.23-bookworm AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO_ENABLED=0 because modernc.org/sqlite is pure Go
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=$(cat VERSION)" \
    -o spaxel ./cmd/mothership

# Stage 2: Minimal runtime image
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /app/spaxel /spaxel
# Embed the dashboard static files at build time
COPY --from=builder /app/dashboard /dashboard
# Include a bundled firmware binary (users can override with a volume mount)
COPY --from=builder /app/firmware/dist/*.bin /firmware/

EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["/spaxel"]
```

**Multi-arch build (CI):**
```bash
docker buildx build --platform linux/amd64,linux/arm64 \
  -t ghcr.io/spaxel/spaxel:$(cat VERSION) \
  -t ghcr.io/spaxel/spaxel:latest \
  --push .
```

**Key design decisions:**
- `distroless/static-debian12:nonroot` — no shell, no package manager, runs as non-root (UID 65532). Minimal attack surface.
- `modernc.org/sqlite` — pure Go SQLite; avoids CGO complexities for multi-arch cross-compilation. Performance is ~20% slower than cgo/mattn but fully adequate for this workload.
- `/dashboard` — the entire dashboard (HTML, JS, Three.js, CSS) is embedded in the binary via `//go:embed dashboard/*`. No volume mount needed for the UI. Updating the UI requires a new Docker image.
- `/firmware` is a COPY from the build stage (bundled default) but is overridable by the user's volume mount (volume takes precedence over COPY content via Docker overlay semantics — actually requires mounting the firmware dir).

**Note on SQLite driver:** `modernc.org/sqlite` maps to the `sqlite3` database/sql driver name. All `sql.Open()` calls use `"sqlite"` (not `"sqlite3"`). Replace with `mattn/go-sqlite3` if CGO performance becomes necessary (requires build-stage `apt-get install gcc`).

### Docker Compose

**Quickstart (single command, no Traefik):**
```bash
docker run -d --name spaxel \
  -p 8080:8080 \
  -v spaxel-data:/data \
  -v ./firmware:/firmware \
  -e TZ=America/New_York \
  ghcr.io/spaxel/spaxel:latest
# Then open http://<server-ip>:8080 — PIN setup page appears on first run
```

**Production docker-compose.yml:**
```yaml
services:
  spaxel:
    image: ghcr.io/spaxel/spaxel:latest   # pin to a specific version in production
    # IMPORTANT: network_mode: host is REQUIRED for mDNS to work.
    # mDNS uses multicast address 224.0.0.251 (link-local), which Docker bridge networking blocks.
    # With host networking, the container shares the host's network interfaces and mDNS multicasts
    # reach the LAN where ESP32 nodes can receive them.
    # Side effect: 'ports' mapping is ignored in host mode — the port 8080 is directly exposed.
    network_mode: host
    # ports:                             # Not used with network_mode: host
    #   - "8080:8080"
    #
    # Alternative (if host mode is not desired): disable mDNS and require nodes to use
    # the cached ms_ip NVS key (manual IP entry during captive portal provisioning).
    # Set SPAXEL_MDNS_ENABLED=false to skip the mDNS advertisement entirely.
    volumes:
      - spaxel-data:/data        # SQLite, baselines, floor plans, CSI recording buffer
      - ./firmware:/firmware     # Firmware binaries for OTA (pre-populate before first run)
    environment:
      TZ: America/New_York       # Required for correct diurnal baseline hours and briefing times
      SPAXEL_MQTT_BROKER: mqtt://homeassistant.local:1883  # Optional; remove line if no MQTT
      # SPAXEL_MQTT_USERNAME: mosquitto
      # SPAXEL_MQTT_PASSWORD: secret
      # SPAXEL_REPLAY_MAX_MB: "720"  # 96h replay for larger installs
      # SPAXEL_LOG_LEVEL: debug      # Uncomment for troubleshooting
    restart: unless-stopped
    stop_grace_period: 35s       # Allows full 30s graceful shutdown
    ulimits:
      nofile:
        soft: 4096               # One fd per node connection + SQLite handles
        hard: 8192
    healthcheck:
      test: ["CMD", "wget", "-q", "-O-", "http://localhost:8080/healthz"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 30s
    deploy:
      resources:
        limits:
          memory: 512m           # Increase to 1g for 16+ node fleets
          cpus: "2.0"
        reservations:
          memory: 128m
          cpus: "0.5"
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.spaxel.rule=Host(`spaxel.example.com`)"
      - "traefik.http.routers.spaxel.entrypoints=websecure"
      - "traefik.http.routers.spaxel.tls.certresolver=letsencrypt"
      - "traefik.http.services.spaxel.loadbalancer.server.port=8080"
      # Extend Traefik timeout for long-lived node WebSocket connections:
      - "traefik.http.routers.spaxel.middlewares=spaxel-ws-timeout"
      - "traefik.http.middlewares.spaxel-ws-timeout.headers.respondingTimeouts.readTimeout=3600s"

volumes:
  spaxel-data:
    driver: local
```

**First-run steps:**
1. Create the firmware directory and copy the initial firmware binary: `mkdir -p ./firmware && cp path/to/spaxel-1.0.0.bin ./firmware/`
2. Start: `docker compose up -d`
3. Open `http://<server-ip>:8080` — the PIN setup page appears (no auth required for this first-run step only)
4. Set your dashboard PIN → redirected to the onboarding wizard
5. Connect an ESP32-S3 via USB, click "Add Node" — Web Serial provisioning begins

**Data backup:**
```bash
# Manual backup before upgrade or for offsite storage
docker exec spaxel wget -qO- http://localhost:8080/api/backup > spaxel-backup-$(date +%Y%m%d).zip

# Or directly from volume:
docker run --rm -v spaxel-data:/data alpine \
  tar czf - /data/spaxel.db /data/floorplan > spaxel-db-backup-$(date +%Y%m%d).tar.gz
```

**Traefik WebSocket notes:**
- Traefik supports WebSocket natively — no special middleware needed. It detects the `Upgrade: websocket` header and proxies the connection transparently
- The `respondingTimeouts.readTimeout` middleware label above extends the default Traefik read timeout so long-lived node WebSocket connections (which may be quiet during idle periods) are not killed
- ESP32 nodes connect to `ws://spaxel.example.com/ws/node` (or `wss://` with TLS) — Traefik routes to the container

### Firmware Build System

**ESP-IDF version:** 5.2.x (stable). Do not use 5.0 or 5.1 — the CSI callback API changed at 5.2. Pin the version in CI: `idf_version: "5.2.3"`.

**Project structure:**
```
firmware/
  main/
    main.c               — app_main(), startup sequencing, task creation
    wifi.c / wifi.h      — WiFi station connect, mDNS, captive portal AP
    csi.c / csi.h        — promiscuous mode, CSI callback, binary frame serialization
    ws.c / ws.h          — WebSocket client (esp_websocket_client), JSON/binary framing
    ble.c / ble.h        — BLE passive scan (esp_bt), advertisement parsing, rotation heuristic
    ota.c / ota.h        — OTA download, SHA-256 verification, esp_ota_ops
    nvs.c / nvs.h        — NVS read/write helpers, schema migration, provisioning
    serial_prov.c        — 10-second serial provisioning window, UART JSON handler
    sntp.c / sntp.h      — SNTP init, sync wait, resync timer
    led.c / led.h        — LED control (identify blink, OTA progress, status)
    CMakeLists.txt
  CMakeLists.txt
  partitions.csv         — factory(4MB) + ota_0(4MB) + ota_1(4MB) + nvs(24KB) + otadata(8KB)
  sdkconfig.defaults     — project-specific sdkconfig overrides (committed to repo)
```

**Required sdkconfig.defaults settings:**
```
# WiFi
CONFIG_ESP32S3_SPIRAM_SUPPORT=y            # enable PSRAM for ring buffer headroom
CONFIG_ESP_WIFI_PROMISCUOUS_FILTER=y       # required for CSI capture
CONFIG_ESP_WIFI_CSI_ENABLED=y             # enable CSI API
CONFIG_ESP_WIFI_STATIC_RX_BUFFER_NUM=16   # increase RX buffers for high CSI rate
CONFIG_ESP_WIFI_DYNAMIC_TX_BUFFER_NUM=32

# BLE (Bluetooth)
CONFIG_BT_ENABLED=y
CONFIG_BT_BLE_ENABLED=y
CONFIG_BT_BLE_42_FEATURES_SUPPORTED=y
CONFIG_ESP_COEX_SW_COEXIST_ENABLE=y       # WiFi+BLE coexistence (mandatory for dual-radio)
CONFIG_ESP_COEX_POWER_MANAGEMENT=y

# OTA
CONFIG_BOOTLOADER_APP_ROLLBACK_ENABLE=y   # dual-partition rollback
CONFIG_OTA_ALLOW_HTTP=y                   # allow HTTP OTA URLs (not HTTPS-only)

# NVS encryption: disabled by default (home use; users can enable manually)
CONFIG_NVS_ENCRYPTION=n

# Flash & partition
CONFIG_ESPTOOLPY_FLASHSIZE_16MB=y
CONFIG_PARTITION_TABLE_CUSTOM=y
CONFIG_PARTITION_TABLE_CUSTOM_FILENAME="partitions.csv"

# App version (set per release)
CONFIG_APP_PROJECT_VER="1.0.0"
CONFIG_APP_PROJECT_VER_FROM_CONFIG=y

# Stack sizes (CSI callback runs in a high-priority task)
CONFIG_ESP_MAIN_TASK_STACK_SIZE=8192
CONFIG_PTHREAD_TASK_STACK_SIZE_DEFAULT=4096

# Logging (INFO in release builds, DEBUG when debug NVS key = 1)
CONFIG_LOG_DEFAULT_LEVEL_INFO=y
CONFIG_LOG_MAXIMUM_LEVEL_DEBUG=y
```

**Task architecture (FreeRTOS):**

| Task | Core | Priority | Stack | Responsibility |
|------|------|----------|-------|----------------|
| `app_main` | 1 | 1 | 8 KB | Startup sequencing, WiFi/WS lifecycle |
| `ws_task` | 1 | 5 | 8 KB | WebSocket send/receive loop |
| `csi_task` | 1 | 10 | 4 KB | CSI callback → binary frame serialization → WS queue |
| `ble_scan_task` | 0 | 3 | 4 KB | BLE passive scan, advertisement parsing, RSSI aggregation |
| `health_task` | 0 | 2 | 2 KB | Periodic health JSON assembly and queuing (every 10 s) |

CSI callback fires at up to 50 Hz; it serializes the frame into a binary buffer and posts to the `ws_send_queue` (depth 32) without blocking. The `ws_task` drains the queue. If the queue is full, the frame is silently dropped (hardware-rate CSI is best-effort).

**Build & flash commands:**
```bash
# One-time setup
. $IDF_PATH/export.sh

# Build
idf.py -C firmware build

# Flash (manufacturing / initial install to factory partition)
idf.py -C firmware -p /dev/ttyUSB0 flash

# Or via esptool directly (used by esptool-js in the dashboard):
esptool.py --port /dev/ttyUSB0 --baud 921600 write_flash \
  0x10000 firmware/build/spaxel.bin

# Generate release binary (same as OTA artifact):
cp firmware/build/spaxel.bin spaxel-$(cat firmware/VERSION).bin
```

**CI/CD:** GitHub Actions workflow builds `spaxel.bin` and attaches it to a GitHub Release. The mothership Docker image includes a `COPY firmware/spaxel-*.bin /firmware/` step so the latest firmware is bundled in the container image (users can override with their own `/firmware/` volume mount).

### Node Hardware

- **Recommended:** ESP32-S3-DevKitC-1 (N16R8 variant — 16 MB flash for OTA dual-partition, 8 MB PSRAM)
- **Minimum:** Any ESP32-S3 board with external antenna connector
- **Antenna:** External 2.4 GHz antenna recommended for consistent CSI (onboard PCB antenna works but with higher variance)
- **Power:** USB-C (5V) — standard phone charger. Consider PoE splitters for ceiling-mounted nodes
- **Enclosure:** 3D-printed or off-the-shelf project box. Mount with adhesive or screws

### Recommended Deployment

- **Quickstart (passive radar):** 2 ESP32 nodes + existing WiFi router. Nodes in RX-only mode. Presence detection in the area between nodes and router
- **Minimum viable:** 4 nodes in a single room, corners at mixed heights (2 high, 2 low). Can mix passive radar + dedicated TX
- **Good coverage:** 6–8 nodes across an apartment, perimeter placement, angular diversity
- **Node density:** ~1 per 50–70 m² for presence, ~1 per 15–25 m² for localization
- **Placement rules:** Non-collinear, avoid all-same-height, keep LoS between at least some pairs

---

## Testing Strategy

### Go Unit Tests

Each algorithmic module has a companion `_test.go` file. Tests are table-driven and use only the standard library (`testing` package). No external test framework required.

**Modules with mandatory unit tests:**

| Package | Test file | What to test |
|---------|-----------|--------------|
| `pipeline/phase` | `phase_test.go` | Phase sanitization: given known I/Q pairs, verify unwrapping produces expected residual. Test NaN/Inf handling. Test near-zero denominator in OLS regression. |
| `pipeline/nbvi` | `nbvi_test.go` | Welford update: verify online variance matches batch variance to 1e-9. Test NBVI threshold fallback (< 8 subcarriers passing). |
| `pipeline/feature` | `feature_test.go` | deltaRMS: given known baseline and amplitude, verify result. EMA baseline update: verify motion-gating (no update when deltaRMS > threshold). |
| `localizer/fresnel` | `fresnel_test.go` | Zone number computation: for known TX/RX/cell geometry, verify ceil(ΔL/(λ/2)). Zone decay: verify zone_decay(n) = 1/n^2 for decay_rate=2. |
| `localizer/ukf` | `ukf_test.go` | Constant-velocity prediction: verify predict-only step matches analytical solution. Measurement update: verify state converges toward known position. Biomechanical clamp: verify XY speed is clamped to 2.0 m/s. |
| `localizer/gdop` | `gdop_test.go` | Fisher matrix: given 2 orthogonal links, verify GDOP = sqrt(2). Collinear links: verify GDOP = Infinity. |
| `portal` | `portal_test.go` | Crossing detection: verify sign-change + velocity threshold logic. Velocity-too-low: verify no tentative crossing registered. Count floor: verify count cannot go below 0. |
| `ble` | `ble_test.go` | BLE centroid: given known node positions and RSSI values, verify pos_ble within 0.01 m of analytical centroid. Address rotation scoring: verify score > 0.7 for matching mfr data + same RSSI node. |
| `anomaly` | `anomaly_test.go` | Welford update: after N identical observations, verify mean = observation and variance = 0. z_score + normalize: verify correct [0,1] mapping at 1σ, 2σ, 4σ. |
| `replay` | `replay_test.go` | File header read/write round-trip. Seek to known timestamp: verify returned frame has recv_time_ms ≥ target. Corruption recovery: truncated final frame → truncated cleanly. |
| `auth` | `auth_test.go` | HMAC token derivation: same inputs produce same token. Session creation/expiry. bcrypt round-trip for PIN. |

**Test data strategy:** All numerical tests use deterministic synthetic data (no random seeds in test paths). The Fresnel zone and UKF tests use hard-coded 2D geometries with analytically known answers.

### Integration Tests (using CSI simulator)

Located in `test/integration/`. Each test:
1. Starts a mothership in a Docker container (or in-process for unit-level integration)
2. Runs `spaxel-sim` with specific walker configurations
3. Polls `GET /api/blobs` and `/api/events` to assert outcomes

**Mandatory integration test scenarios:**

| Scenario | Simulator config | Assertion |
|----------|-----------------|-----------|
| Single node, single walker | 2 nodes, 1 walker, 60 s | blob count > 0 for > 80% of time |
| Multi-node localization | 4 nodes, 1 walker, 60 s | blob position within 1.5 m of walker position |
| Idle-to-active rate change | 4 nodes, 0 walkers → 1 walker after 10 s | node rate increases after walker appears |
| Node disconnect + reconnect | 4 nodes, disconnect one mid-test | system continues producing blobs; node returns to fleet |
| Portal crossing | 2 nodes, walker crosses portal | `portal_crossings` table has 1 row |
| OTA rollback | Push invalid firmware | node reconnects with original version |
| Auth rejection | Connect without token | connection closed with HTTP 401 |

### Firmware Tests (host-based unit tests)

ESP-IDF supports host-based testing via `idf.py test --target linux`. The following firmware modules have host tests:

- `nvs` — NVS schema migration: simulate schema_ver=0→1 upgrade
- `csi` — Binary frame serialization: verify frame header fields and little-endian encoding
- `serial_prov` — Provisioning JSON parser: verify valid JSON parsed correctly; invalid JSON returns `{"ok":false}`

## Open Questions

- **5 GHz support:** ESP32-S3 is 2.4 GHz only. Future ESP32-C6 or C5 may add 5 GHz with different CSI characteristics. Design the pipeline to be frequency-agnostic where possible
- **Node self-positioning:** MDS-MAP from pairwise ToF could eliminate manual position entry. Feasibility with ESP32 ToF resolution (~7.5 m) is questionable — defer to a future phase
- **IEEE 802.11bf:** The new sensing standard (approved May 2025) will eventually provide standardized sensing frames. Monitor for ESP32 support — could replace promiscuous mode CSI capture
- **Multi-installation coordination:** Could multiple Spaxel instances in adjacent apartments share boundary link data to improve wall-adjacent detection? Deferred — privacy and network topology implications need thought
