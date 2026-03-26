# Spaxel Implementation — Marathon Instruction

## Context

You are implementing Spaxel, a WiFi CSI-based indoor positioning system for
self-hosted home environments. The full implementation plan is at
`/home/coding/spaxel/docs/plan/plan.md` (~1400 lines). Read it before writing
a line of code.

## Working Directory

`/home/coding/spaxel`

## This Iteration

Each iteration, do the following:

1. **Read the plan** at `docs/plan/plan.md` to understand the full architecture,
   component design, and phase requirements. It is the authoritative source of
   truth — follow it exactly.

2. **Assess current state**: check what code exists, what tests pass, what's
   been built so far. Read PROGRESS.md if it exists. Run `ls -la` on key
   directories. Run any existing tests.

3. **Identify the next piece of work**: find the highest-priority unfinished
   item. Work through the phases sequentially — Phase 1 first, then Phase 2,
   etc. Do not skip ahead. Within a phase, complete items in the order listed.

4. **Implement one coherent unit of work**: a single module, a set of related
   functions, a firmware component, or a configuration file. Keep each
   iteration focused — one deliverable at a time. Do not implement Phase 2
   features (signal processing, baseline, deltaRMS) until Phase 1 is complete.

5. **Write tests** for what you build (where applicable — Go code always gets
   tests; ESP-IDF C firmware may use simulator stubs). Run tests and fix
   failures before finishing.

6. **Commit and push BEFORE the iteration ends.** This is mandatory. Every
   iteration MUST end with `git add`, `git commit`, and `git push`. The commit
   message MUST follow this format:

   ```
   <type>(<scope>): <short summary>

   - <specific decision and why>
   - <specific decision and why>
   - <constants, API choices, deviations from plan if any>

   Complete: <what this commit finishes>
   Remaining: <what is still outstanding in this component>
   ```

   Example:
   ```
   feat(mothership): WebSocket ingestion server with binary/JSON frame parsing

   - /ws/node endpoint: one goroutine per connection, bidirectional
   - Binary frames: 20-byte header → CSIFrame struct; payload as []int8 pairs
   - JSON frames: dispatched by "type" field (hello, health, ble, ota_status)
   - Per-link ring buffer: 256-sample circular, keyed by (node_mac, peer_mac)
   - Node identity from first "hello" — no pre-registration required
   - Used nhooyr.io/websocket: context-aware, no global state vs gorilla
   - mDNS via github.com/grandcat/zeroconf at _spaxel._tcp.local:8080

   Complete: frame parsing, ring buffers, mDNS, hello/health/ble dispatch
   Remaining: OTA command dispatch, /ws/dashboard publisher (next loop)
   ```

7. **Update PROGRESS.md**: update the progress file at the repo root before
   committing. Create it on the first iteration if it doesn't exist.

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Mothership backend | Go (single binary, `mothership/` module) |
| Dashboard frontend | Vanilla JS + Three.js (no build toolchain, `dashboard/`) |
| ESP32 firmware | ESP-IDF (C, `firmware/` ESP-IDF project) |
| Node ↔ Mothership | WebSocket — binary frames upstream, JSON downstream |
| Persistence | SQLite (`/data/spaxel.db` in container) |
| Container | Single Docker container, one exposed port (8080) |

## Binary CSI Frame Format (firmware and mothership must agree exactly)

```
Header (fixed 20 bytes):
  node_mac:     6 bytes  — source node MAC
  peer_mac:     6 bytes  — transmitting peer MAC
  timestamp_us: 4 bytes  — uint32, microseconds (wrapping OK)
  rssi:         1 byte   — int8, dBm
  noise_floor:  1 byte   — int8, dBm
  channel:      1 byte   — uint8, WiFi channel
  n_sub:        1 byte   — uint8, subcarrier count (typically 64)

Payload (n_sub × 2 bytes):
  Per subcarrier: int8 I, int8 Q
```

## Repository Structure to Create

```
spaxel/
├── firmware/           # ESP-IDF project
│   ├── main/
│   │   ├── main.c
│   │   ├── wifi.c / wifi.h
│   │   ├── websocket.c / websocket.h
│   │   ├── csi.c / csi.h
│   │   ├── ble.c / ble.h
│   │   └── CMakeLists.txt
│   ├── CMakeLists.txt
│   └── sdkconfig.defaults
├── mothership/         # Go module
│   ├── cmd/mothership/main.go
│   ├── internal/
│   │   ├── ingestion/   # /ws/node WS server, frame parsing, ring buffers
│   │   ├── fleet/       # Node registry (SQLite)
│   │   └── dashboard/   # /ws/dashboard state publisher
│   └── go.mod
├── dashboard/          # Static files served by mothership
│   ├── index.html
│   └── js/
├── Dockerfile
├── docker-compose.yml
├── PROGRESS.md
└── docs/               # Already exists — do not modify
```

## Guidelines

- **Follow the plan**: implement what `docs/plan/plan.md` says, in the order it
  says. Do not add features not in the plan. Do not refactor prematurely.
- **Quality**: production-quality Go code — handle errors, no panics in library
  code, structured logging. ESP-IDF C should match ESP-IDF coding conventions.
- **Tests**: unit tests for every Go module. Run them. Fix failures.
- **Dependencies**: use well-known maintained libraries, pin versions in go.mod.
- **No stubs**: implement each component fully before moving on. No `// TODO`
  placeholders left in committed code.
- **Commit granularity**: one meaningful unit of work per commit. Never batch
  two separate components into one commit. Never end an iteration without
  committing.

## Files to Reference

- `docs/plan/plan.md` — The full implementation plan (source of truth)
- `docs/research/` — Research documents (physics, algorithms, signal processing)
- `docs/notes/` — Design notes (recovery mechanisms, simulation testing, UX)
- `PROGRESS.md` — Running log of what's done (create if missing)
