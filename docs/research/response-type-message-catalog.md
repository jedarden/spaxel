# Response-type messages — complete catalog, organized by name pattern

Bead: `spaxel-4cc60ef7`. Fourth step of the protobuf survey chain.

| Step | Bead | Document |
|---|---|---|
| 1 | `spaxel-3278a63f` | [protobuf-survey-free-heap.md](protobuf-survey-free-heap.md) |
| 2 | `spaxel-c95b4ab0` | [protobuf-survey-response-message-definitions.md](protobuf-survey-response-message-definitions.md) |
| 3 | `spaxel-2e4830b7` | [proto-file-inventory.md](proto-file-inventory.md) |
| 4 | `spaxel-4cc60ef7` | **this document** |

## Attribution: what repeats and what is new

Substantial overlap with step 2 (`spaxel-c95b4ab0`, closed, commit `3d6a0cbc`):
both answer "list every response-type message with its source." The two prior
docs already enumerate the same definition sites and the same node-WebSocket
message set, and every anchor they cite for `ingestion/message.go` re-verified
**byte-for-byte identical** in this pass — all 13 type declarations and
`ParseJSONMessage` sit at exactly the lines they were published at.

What this document adds, and the reason it is not a re-publication:

1. **The cut step 2 never made.** The task for this bead asks for messages
   *categorized by name pattern* — `*Response`, `*Reply`, `*Info`, `*Status`,
   `*Result`. Neither prior doc groups that way; both group by transport
   surface and direction. Section 2 is that grouping, complete.
2. **Wire versus non-wire, with evidence.** A name ending in `Response` is not
   evidence of a response message. Of the 63 types this catalog holds, **37 are
   actually serialised to a peer** and **26 are internal domain values that
   merely borrowed the vocabulary** — several carry live
   `json:` tags despite never being marshalled, which is exactly the trap a
   grep-only survey falls into. Both populations are listed, and every wire
   claim is backed by a traced serialisation site.
3. **Three name collisions step 2 recorded only one of.** `health.Response` /
   `doctor.Response` was already documented. There are also **three distinct
   `NodeInfo`** types and **three distinct identity-match types**, plus two
   unrelated `ActionResult`. Section 4.
4. **One dead route registration.** `GET /api/diurnal/status` is registered
   twice and chi silently keeps the second — verified by executing chi, not by
   reading it. Section 5.

---

## 1. Premise, re-verified at dispatch time

The task's step 1 is "read each `.proto` file found in the previous step." The
previous step found none, and that finding still holds:

```bash
find . -name '*.proto' -not -path './.git/*'                                   # 0 files
git log --all --diff-filter=A --name-only --pretty=format: -- '*.proto' \
  | grep -c proto                                                              # 0 — never existed
grep -rln 'syntax = "proto' --exclude-dir=.git .                               # only the 2 survey docs
```

So the "source `.proto` file" column the acceptance criteria ask for has no
possible value. The schema registry this codebase actually has is the set of Go
structs and C `cJSON` builders inventoried by step 3, and the source-file
column below points there. Every response message in the system is a Go struct
or a C string literal; there is no generated layer and never has been.

---

## 2. The catalog, grouped by name pattern

Counts below are non-test `.go` files in `mothership/`, `cmd/`, `test/`,
`tests/`. "Source" is `mothership/`-relative unless marked.

### 2.1 `*Response` — 23 wire, 0 non-wire

Every type whose name ends in `Response` is genuinely serialised. No false
positives in this group — the suffix is used consistently here.

**`internal/api` (15):**

| Type | Source | Serialised at | Route |
|---|---|---|---|
| `ActiveAlertsResponse` | `internal/api/alerts.go:39` | `handleGetActiveAlerts` | `GET /api/alerts/active` |
| `captureResponse` | `internal/api/baseline.go:95` | `captureBaseline` | `POST /api/baseline/capture` |
| `eventsResponse` | `internal/api/events.go:211` | `listEvents` | `GET /api/events` |
| `integrationSettingsResponse` | `internal/api/integrations.go:121` | `handleGetSettings` | `GET /api/settings/integration` |
| `networkSettingsResponse` | `internal/api/network_settings.go:37` | `response()` | `GET`/`PUT /api/settings/network` |
| `networkSettingsWithPasswordResponse` | `internal/api/network_settings.go:45` | `responseWithPassword()` | `GET /api/settings/network/recovery` |
| `notificationSettingsResponse` | `internal/api/notification_settings.go:87` | `getSettings` `:238` | `GET`/`PUT /api/settings/notifications` |
| `notificationConfigResponse` | `internal/api/notifications.go:321` | `handleGetConfig` | `GET /api/notifications/config` |
| `testNotificationResponse` | `internal/api/notifications.go:381` | `handleSendTest` | `POST /api/notifications/test` (registered twice — see step 2 caveat 3) |
| `SystemModeResponse` | `internal/api/security.go:57` | `writeJSON` `:107`, `:145`, `:162` | `GET`/`POST /api/mode` |
| `GDOPResponse` | `internal/api/simulator.go:496` | `ComputeGDOP` | `POST /api/simulator/compute` |
| `SimulationResponse` | `internal/api/simulator.go:709` | `RunSimulation` | `POST /api/simulator/simulate` |
| `occupancyResponse` | `internal/api/status.go:135` | `getOccupancy` | `GET /api/occupancy` |
| `TriggerResponse` | `internal/api/volume_triggers.go:66` | `toResponse` `:756` | `/api/triggers` CRUD |
| `crossingResponse` | `internal/api/zones.go:80` | `getPortalCrossings` | `GET /api/portals/{id}/crossings` |

**`internal/fleet` (6)** — working-tree line numbers; see §6 for the drift:

| Type | Source | Serialised at | Route |
|---|---|---|---|
| `fleetListResponse` | `internal/fleet/handler.go:182` | `listFleet` | `GET /api/fleet` |
| `systemModeResponse` | `internal/fleet/handler.go:793` | `getSystemMode`/`setSystemMode` | `GET`/`POST /api/mode` |
| `autoAwayConfigResponse` | `internal/fleet/handler.go:799` | embedded in the above | `GET`/`POST /api/mode` |
| `fleetHealthResponse` | `internal/fleet/fleethandler.go:54` | `getFleetHealth` `:155` | `GET /api/fleet/health` |
| `optimiseResponse` | `internal/fleet/fleethandler.go:294` | `triggerOptimise` | `POST /api/fleet/optimise` |
| `simulateResponse` | `internal/fleet/fleethandler.go:315` | `simulateNodeRemoval` | `GET /api/fleet/simulate` |

**Other packages (2):**

| Type | Source | Route |
|---|---|---|
| `health.Response` | `internal/health/health.go:49` | `GET /healthz` |
| `doctor.Response` | `internal/doctor/doctor.go:73` | `GET /api/doctor` |

`SimulationResponse` is **not** the same shape as `simulator.SimulationResult`
(§2.4) — it is an independent struct holding `WalkerPositions`, `LinkActivity`,
`Duration` and `Ticks`. Do not assume one wraps the other.

### 2.2 `*Status` — 3 wire, 2 non-wire

**Wire:**

| Type | Source | Serialised at | Surface |
|---|---|---|---|
| `ingestion.OTAStatusMessage` | `internal/ingestion/message.go:82` | firmware `websocket.c:725` | The **only strictly typed request/response pair in the node protocol** — answers `OTAMessage` (`:109`) |
| `signal.DiurnalLearningStatus` | `internal/signal/processor.go:499` | `writeJSON` `:3693`→dead, live at `api/diurnal.go:74` | `GET /api/diurnal/status` — see §5 |
| `api.SecurityStatus` | `internal/api/security.go:48` | `writeJSON` `:107`, `:145`, `:162` | `GET`/`POST /api/mode` |

**Non-wire:**

| Type | Source | Why excluded |
|---|---|---|
| `sleep.SleepStatus` | `internal/sleep/integration.go:601` | Carries full `json:` tags and `Monitor.GetStatus()` (`:620`) builds one — **but `GetStatus` has no callers**. A response shape built for an endpoint that was never written. |
| `zones.OccupancyStatus` | `internal/zones/manager.go:19` | Not a message at all — a two-value `string` enum (`uncertain`/`reconciled`) that appears as a *field* inside real responses. |

### 2.3 `*Info` — 4 wire, 10 non-wire

**Wire:**

| Type | Source | Serialised at | Surface |
|---|---|---|---|
| `ingestion.NodeInfo` | `internal/ingestion/server.go:1056` | dashboard hub `delta["nodes"]` (`hub.go:128`, `:782`) | Dashboard WebSocket feed |
| `ingestion.LinkInfo` | `internal/ingestion/server.go:1099` | dashboard hub `delta["links"]` (`hub.go:796`) | Dashboard WebSocket feed |
| `ingestion.LinkHealthInfo` | `internal/ingestion/server.go:1111` | `GET /api/links` (`cmd/mothership/main.go:3757`) | REST |
| `api.replayInfo` | `internal/api/replay.go:143` | `listSessions` → `writeJSON` `:183` | `GET /api/replay/sessions` — `replay.go:127`, mounted at `main.go:925` only when a replay store is configured |

**Non-wire** — the first row is the one a tag-based scan gets wrong:

| Type | Source | Why excluded |
|---|---|---|
| `api.SessionInfo` | `internal/api/replay.go:47` | Carries `json:` tags and a public accessor, `ReplayHandler.GetSessions() []SessionInfo` (`:845`) — but `GetSessions` has **zero callers**. A response shape built for a replay-listing endpoint that was never written. Do not re-add it to the wire column without writing that endpoint first. |
| `doctor.NodeInfo` | `internal/doctor/doctor.go:46` | No `json` tags; a 1-field (`MAC`) input to a token-consistency *check*, not a response |
| `fleet.NodeInfo` (optimiser) | `internal/fleet/optimiser.go:26` | No `json` tags; optimiser input model |
| `apdetector.APInfo` | `internal/apdetector/detector.go:32` | No `json` tags; internal consensus accumulator |
| `simulator.ZoneInfo` | `internal/simulator/engine.go:61` | `json` tags present, but consumed only inside the simulation engine |
| `api.IdentityMatch` | `internal/api/status.go:45` | `json` tags, but `getOccupancy` only reads `PersonName`/`DeviceName` off it (`status.go:191-200`) — never marshalled |
| `guidedtroubleshoot.ZoneInfo` | `internal/guidedtroubleshoot/quality.go:142` | No `json` tags |
| `tracker.IdentityInfo` | `internal/tracker/identity.go:22` | No `json` tags |
| `automation.PredictionInfo` | `internal/automation/engine.go:226` | No `json` tags |
| `volume.PredictionInfo` | `internal/volume/shape.go:167` | No `json` tags |

### 2.4 `*Result` — 3 wire, 14 non-wire

This is the group where the suffix misleads most: only 3 of 17 are wire types.
(`automation.PredictionInfo` also matches a `*Result`-adjacent role but is an
`*Info` type; it is counted once, in §2.3.)

**Wire:**

| Type | Source | Serialised at | Route |
|---|---|---|---|
| `api.WebhookTestResult` | `internal/api/volume_triggers.go:86` | `writeJSON` `:630` | `POST /api/triggers/{id}/test` |
| `api.ActionResult` | `internal/api/volume_triggers.go:94` | nested in `WebhookTestResult.Actions` | same |
| `doctor.CheckResult` | `internal/doctor/doctor.go:66` | nested in `doctor.Response.Checks` | `GET /api/doctor` |

**Non-wire:**

| Type | Source | Why excluded |
|---|---|---|
| `automation.ActionResult` | `internal/automation/engine.go:147` | `json` tags, but only passed to `logActionResults` (`:969`) → the event log. **Collides by name with the wire type `api.ActionResult`** (§4) |
| `analytics.AnomalyResult` | `internal/analytics/patterns.go:277` | `json` tags, yet zero references outside `patterns.go`. An event-log payload shape, never a response body |
| `simulator.SimulationResult` | `internal/simulator/engine.go:68` | `json` tags, never marshalled. `api.SimulationResponse` is the wire type and has a different shape |
| `simulator.BlobResult` | `internal/simulator/engine.go:80` | `json` tags, embedded only in the above |
| `simulator.GDOPResult` | `internal/simulator/gdop.go:10` | No tags. `api.GDOPResponse` is the wire type |
| `simulator.RayResult` | `internal/simulator/propagation.go:36` | No tags; physics intermediate |
| `fleet.OptimiseResult` | `internal/fleet/optimiser.go:92` | No tags. `fleet.optimiseResponse` is the wire type |
| `fleet.CoverageResult` | `internal/fleet/healer.go:26` | No tags; internal coverage analysis |
| `signal.ProcessResult` | `internal/signal/processor.go:38` | No tags; per-frame pipeline output |
| `signal.FFTBreathingResult` | `internal/signal/breathing.go:387` | No tags; DSP intermediate |
| `recording.BenchmarkResult` | `internal/recording/benchmark.go:35` | No tags |
| `localization.FusionResult` | `internal/localization/fusion.go:26` | No tags |
| `fusion.Result` | `internal/fusion/fusion.go:42` | No tags |
| `explainability.FusionResultSnapshot` | `internal/explainability/handler.go:88` | No tags |

### 2.5 `*Reply` — zero, in both groups

**No type in the repository ends in `Reply`.** Verified by the same mechanical
scan that produced every table above. The pattern listed in the task is simply
not part of this codebase's vocabulary — the node protocol expresses the same
concept with `OTAStatusMessage`, and REST uses `*Response`. Nothing to
inventory, and no risk of having missed one.

### 2.6 Response-role messages with no matching suffix

These are genuine responses that the five patterns do not name. Together with
the three role-matched, name-mismatched types noted in §7, they are why a
suffix scan alone under-counts by 7:

| Message | Source | Surface | Note |
|---|---|---|---|
| `RejectMessage` | `internal/ingestion/message.go:140` | Node WS, mothership→node | Terminal response to a failed handshake; closes the connection |
| `HelloMessage` | `internal/ingestion/message.go:11` | Node WS, node→mothership | The handshake *answer* to the connection itself |
| `provisioning.Payload` | `internal/provisioning/server.go:24` | REST, `POST /api/provision` | Response body; no `-Response` suffix |
| `{"ok":true/false,...}` | `firmware/main/provision.c:166`, `:138`, `:147`, `:155`, `:173`, `:202` | Serial provisioning | C string literals, not structs — the only response messages in the tree with no Go type behind them |

Everything else in the node-WebSocket protocol is a periodic report or a
command acknowledged implicitly by behaviour; that pairing analysis is step 2's
§A and is unchanged.

---

## 3. Tally

| Group | Wire | Non-wire | Total |
|---|---|---|---|
| `*Response` | 23 | 0 | 23 |
| `*Status` | 3 | 2 | 5 |
| `*Info` | 4 | 10 | 14 |
| `*Result` | 3 | 14 | 17 |
| `*Reply` | 0 | 0 | 0 |
| no suffix, still a response | 4 | — | 4 |
| **Total** | **37** | **26** | **63** |

---

## 4. Name collisions — always package-qualify

Step 2 documented the `health.Response` / `doctor.Response` pair. Three more
exist, and one of them puts a wire type and a non-wire type on the same bare
name:

| Bare name | Distinct types | Hazard |
|---|---|---|
| `NodeInfo` | `ingestion.NodeInfo` (wire, dashboard WS) · `doctor.NodeInfo` (internal, 1 field) · `fleet.NodeInfo` (internal, optimiser) | A grep for "the NodeInfo response" returns three unrelated structs. Only the `ingestion` one is on the wire |
| `ActionResult` | `api.ActionResult` (**wire**, per-action test outcome) · `automation.ActionResult` (internal, event log) | The internal one carries `json:` tags, so a tag-based scan reports it as wire. It is not |
| `IdentityMatch` | `api.IdentityMatch` (internal, read-only) · `ble.IdentityMatch` (domain model, `internal/ble/identity.go:38`) | `api.IdentityMatch` exists *only* to avoid importing the `ble` type; the doc comment at `status.go:44` says so |
| `Response` | `health.Response` · `doctor.Response` | Already recorded in step 2, caveat 2 |
| `ZoneInfo` | `simulator.ZoneInfo` · `guidedtroubleshoot.ZoneInfo` | Both internal, but unrelated |

---

## 5. New finding: `GET /api/diurnal/status` is registered twice, and the first registration is dead

Both registrations live in `func main()` (`cmd/mothership/main.go:703`):

1. `cmd/mothership/main.go:3693` — inline closure, `writeJSON(w, statuses)`
2. `internal/api/diurnal.go:64` — `DiurnalHandler.RegisterRoutes`, reached from
   `main.go:4622`

**Chi does not reject a duplicate `GET` on the same pattern — it silently
discards the first.** Verified by executing it, not by reading source: a probe
registering two handlers on one `chi.NewRouter()` for the same path returns 200
with the **second** handler's body and never panics. Since registration 1 runs
before registration 2 (both in `main()`, lines in ascending order), the inline
closure at `main.go:3693` is dead code and
`DiurnalHandler.getDiurnalStatus` (`api/diurnal.go:72`, registered at `:66`) is
what actually
serves the route.

Consequence for this catalog: `signal.DiurnalLearningStatus`'s "serialised at"
site is `api/diurnal.go:74`, **not** `main.go:3694`. The inline closure also
registers `/api/diurnal/status/{linkID}` at `main.go:3697`, which the
`DiurnalHandler` does *not* register — that one survives.

This is reported, not fixed: this is a documentation bead, and removing the
dead registration is a behaviour-adjacent change that belongs in its own bead
with its own test.

The same shape as step 2's `POST /api/notifications/test` double registration,
which *is* live because the conditional one is mounted first there.

---

## 6. Line-number drift since step 2/3

`internal/fleet/handler.go` carries an **uncommitted third-party edit** in the
working tree (`+37` lines, hunk `@@ -109,16 +109,47 @@`, adding a `nodeView`
type for `GET /api/nodes`). It shifts everything below ~109:

| Symbol | Step 2/3 cited (at HEAD) | This document (working tree) |
|---|---|---|
| `fleetListResponse` | `fleet/handler.go:151` | `fleet/handler.go:182` |
| `systemModeResponse` | `fleet/handler.go:762` | `fleet/handler.go:793` |
| `autoAwayConfigResponse` | `fleet/handler.go:768` | `fleet/handler.go:799` |

`internal/fleet/fleethandler.go`, `internal/api/*`, `internal/ingestion/*`,
`internal/health`, `internal/doctor` and `internal/signal` all re-verified
**unchanged** from the numbers step 2 and 3 published. The `nodeView` type that
edit adds is a response type in its own right for `GET /api/nodes`, which step
2 recorded as "no named response type at HEAD"; it is omitted from §2.1 because
it is still uncommitted, and lands in the `*Response` count only when its
commit does.

---

## 7. Method — how each wire claim was established

Every row in the wire columns is backed by one of exactly three kinds of
evidence, in descending order of strength:

1. **A traced `writeJSON`/`json.NewEncoder` call site** whose argument is that
   type or a struct embedding it (all `internal/api` `*Response` types,
   `SecurityStatus`, `WebhookTestResult`, `LinkHealthInfo`,
   `DiurnalLearningStatus`, `doctor.Response`, `health.Response`).
2. **A traced marshal into a WebSocket frame** (dashboard hub
   `delta["nodes"]`/`delta["links"]` for `ingestion.NodeInfo`/`LinkInfo`).
3. **Proof by construction for the node protocol**: `ParseJSONMessage`
   (`ingestion/message.go:146`) switches on exactly five upstream types, so
   nothing outside them can be a node response. `OTAStatusMessage` is the only
   one answering a specific request.

Non-wire rows are backed by the **absence** of all three, plus a grep for the
type name across `mothership/`, `cmd/` and `test/` excluding its own defining
file. Where a non-wire type nevertheless carries `json:` tags, that is called
out explicitly in its row — those are the rows a tag-based scan gets wrong.

The mechanical scan that seeded both populations:

```bash
grep -rnE '^type [A-Za-z0-9_]*(Response|Reply|Info|Status|Result)\b' \
  mothership/ cmd/ test/ tests/ --include='*.go' \
  | grep -v '_test.go' | grep -v 'test_helpers.go'   # 56 types
```

Note the `*` after the character class: a `+` there silently drops the three
bare-name declarations (`health.Response`, `doctor.Response`,
`fusion.Result`), which is exactly the class of miss this catalog exists to
avoid.

**56, not the 59 suffix-named types catalogued above**, because a name scan
under-counts in three ways, and only one of them is §2.6:

| Missed type | Actual name ends in | Why it belongs in the catalog |
|---|---|---|
| `ingestion.OTAStatusMessage` | `…Message` | Its *role* is the node protocol's only status response |
| `api.IdentityMatch` | `…Match` | Occupancy-response input (§2.3) |
| `explainability.FusionResultSnapshot` | `…Snapshot` | Fusion-result payload (§2.4) |

Each of those was carried in by reading the response builders, not by the
pattern. Route/builder pairs were likewise read by hand from each
`RegisterRoutes` and each handler, not inferred — per step 2 caveat 4, most
routes live in per-package `RegisterRoutes` methods, so a mechanical route
scan mis-attributes.

### Reproduce

```bash
# every type matching the five patterns, with its defining file
grep -rnE '^type [A-Za-z0-9_]*(Response|Reply|Info|Status|Result)\b' \
  mothership/ cmd/ test/ tests/ --include='*.go' \
  | grep -v '_test.go' | grep -v 'test_helpers.go'   # 56

# the zero-.proto premise
find . -name '*.proto' -not -path './.git/*'

# the dead diurnal registration (two hits, one route)
grep -rn '"/api/diurnal/status"' mothership/cmd/mothership/main.go \
  mothership/internal/api/diurnal.go
```
