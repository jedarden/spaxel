# Localization Capability → Simulator Acceptance Map

**Status:** design + recorded results — AS-8, AS-9 and AS-3-ext are implemented and measured (see §8); AS-10 is assigned with its simulator fixture landed, acceptance test pending. Child 1 of 5 of the auto-split of `spaxel-23902254`
**Created:** 2026-09-18
**Scope:** maps every localization capability advertised in `README.md` (lines 13–20) to a
concrete deterministic scenario design for the simulator-driven acceptance suite at
`mothership/test/acceptance/`. This is a plan document — it assigns identifiers and files;
implementing the scenarios is follow-up work tracked against the parent umbrella bead.

---

## 1. Source claims being mapped

From `README.md` lines 13–20 (verbatim figures — every scenario assertion below cites one):

| # | Claim | README line | Advertised figure |
|---|-------|-------------|-------------------|
| C0 | Presence detection | L13 | "reliably, with 2+ nodes on opposite sides of a space" |
| C1 | Approximate 2D position | L14 | "±0.5–1.0 m with 4+ nodes" |
| C2 | Motion / trajectory tracking | L15 | "follows moving people" (no separate numeric figure) |
| C3 | Rough person count | L16 | "distinguishes 1 vs. 2+ (degrades at 3+)" |
| C4 | Rough Z-axis | L17 | "±1–2 m with mixed-height node placement (enables fall detection)" |
| C5 | Stationary-person detection | L18 | "via breathing micro-motion (0.1–0.5 Hz)" |
| N1 | *Not achievable:* sub-10 cm accuracy | L20 | negative claim |
| N2 | *Not achievable:* skeletal pose | L20 | negative claim |
| N3 | *Not achievable:* reliable 5+ person tracking | L20 | negative claim |

---

## 2. Existing suite inventory and numbering

Current `mothership/test/acceptance/` files (AS numbers taken):

| File | Scenario |
|------|----------|
| `as1_first_time_setup_test.go` | AS-1 first-time setup |
| `as2_walking_detection_test.go` | AS-2 walking detection (presence + motion appear/disappear) |
| `as3_fall_detection_test.go` | AS-3 fall detection (trigger, confirmation, webhook, bag-on-couch false positive) |
| `as4_ble_identity_test.go` | AS-4 BLE identity |
| `as5_ota_test.go` | AS-5 OTA update (sole claimant of the number since the 2026-09-25 rehome — see the numbering rules below) |
| `wifi_restart_race_test.go` | *not an AS scenario* — bf-9gfph firmware-fix verification, rehomed out of the AS-5 number on 2026-09-25 (spaxel-1cd1155f) |
| `as6_replay_test.go` | AS-6 replay |
| `as7_auth_reject_test.go` | AS-7 auth rejection |

Plus shared `test_helpers.go` (`getBlobsResponse`, `getNodesResponse`, `getEventsByType`,
`checkMothershipHealth`) and `integration_test.go`, which owns the scenario registry
(`integration_test.go:64-70`) and the `TestAcceptanceScenarios` runner.

**Suite convention:** each `asN_<name>_test.go` defines `AS<N>_<Name>` functions that are
registered by name in `integration_test.go`'s scenario table — a new file is not runnable
until its entries are added there.

**Numbering rules (this map is the scenario-numbering authority):**

1. **One number, one scenario.** A number assigned here (or in `docs/plan/plan.md`'s
   Acceptance Scenarios section) is never given to a second scenario, and an
   `asN_<name>_test.go` name may only be taken by a scenario this map assigns. The AS-5
   number was once silently double-used (`as5_ota_test.go` + the WiFi restart race
   verification, rehomed 2026-09-25, spaxel-1cd1155f) — that is the failure mode these
   rules exist to prevent.
2. **Non-scenario tests stay out of the namespace.** Firmware-defect verifications and
   other regression tests are not acceptance scenarios: they take no `asN_` name, are not
   registered in `integration_test.go`'s scenario table, and live under a plain
   descriptive file name (`wifi_restart_race_test.go` is the precedent).
3. **Duplicates are mechanically rejected.** The doc-enumeration tripwire
   (`doc_enumeration_tripwire_test.go`) fails on two files claiming the same N, so a
   double-assignment cannot land silently again.
4. **Numbers stay contiguous.** The tripwire's "AS-1 … AS-N" enumeration contract in
   `README.md` / `docs/codebase-structure-and-test-patterns.md` requires every number up
   to the highest implemented scenario to have a test file, so new scenarios take the
   next number in sequence — a high number cannot be reserved while lower ones are
   unimplemented.

**Next free AS numbers: 8, 9, 10.** This map assigns (as of 2026-09-25, AS-8 and
AS-9 are implemented and AS-10's fixture has landed — see §8 for recorded status):

- `as8_2d_position_accuracy_test.go` (new — C1, N1)
- `as9_person_count_test.go` (new — C3, N3)
- `as10_stationary_breathing_test.go` (new — C5)
- extend-in-place `as2_walking_detection_test.go` (C0, C2)
- extend-in-place `as3_fall_detection_test.go` (C4, N2)

No identifier collides with as1–as7, and since the 2026-09-25 rehome the number 5 has
exactly one scenario.

---

## 3. Simulator primitives available

### In-process engine — `mothership/internal/simulator`

| Primitive | Where | Role in scenarios |
|-----------|-------|-------------------|
| `Engine` / `SimulationResult` | `engine.go` | runs space+nodes+walkers → blobs; `SimulationResult.Accuracy` carries an `AccuracyReport` |
| `SimWalker` (with `TrueHistory`) | `engine.go:37` | ground-truth trajectory per tick |
| `BlobResult` (`TrueError` field) | `engine.go:80` | per-blob distance from true position |
| `NewPathWalker` / `WalkerSet` / `CreatePathWalkers` | `walker.go:68`, `walker.go:284` | **scripted (RNG-free) trajectories** |
| `Space` / `WallSegment` / `FresnelZoneNumber` | `space.go` | room geometry |
| `NodeSet` / `DefaultNodePositions` | `node.go:281` | node placement; **already alternates Z (0.25/0.75 of room height) by cell parity for count ≥ 3** — i.e. mixed-height placement exists at engine level |
| `PhysicsModel` (`DeltaRMS`, `ComputeRSSI`, `PathLossdB`) | `physics.go` | link-level signal synthesis |
| `AccuracyEstimator.Compute` → `AccuracyReport` | `accuracy.go:48`, `accuracy.go:19` | `MedianError`, `MeanError`, `MaxError`, `P95Error`, `RecallAt1m`, `RecallAt2m`, `DetectionRate` |

### `spaxel-sim` CLI — `mothership/cmd/sim` (drives a live mothership per the e2e recipe)

- `--seed N` — fixed-seed RNG; seeded walker paths are covered by
  `cmd/sim/main_test.go:180-213` (`TestSeedReproducibility`, `TestSeedReproducibilityWalkerPaths`).
- `--walker-type path` + `--path-file <json>` — fully scripted trajectories (no RNG in the
  motion itself). `--walker-type random|node-to-node` for the seeded variants.
- `--output-csv <file>` — per-tick ground truth (walker positions, nodes, walls) written by
  `CSVWriter` (`cmd/sim/main.go:345-351`, `1093-1094`); the assertion-side counterpart to
  `SimWalker.TrueHistory`.
- `--nodes N --space WxDxH --rate Hz --duration s --noise-sigma --wall x1,y1,x2,y2`.
- `--scenario normal|fall|ota|bag-on-couch` with `--fall-delay/--fall-duration/--stillness`
  driving `FallScenarioState` (`cmd/sim/scenario.go:36-65`): walking → falling (velocity
  `MinVelocity` margin) → `on_floor` at `EndZ` → recovering; `bag-on-couch` is the as3
  false-positive control.
- **Known placement gap:** the CLI places all virtual nodes on the perimeter at **Z = 2.0 m**
  (`cmd/sim/main.go:457-466`) — uniform height. Mixed-height placement exists only in the
  engine's `DefaultNodePositions` (§ above). See scenario AS-3-ext.
- The CLI does **not** model breathing micro-motion (no oscillator anywhere in
  `cmd/sim/` or `internal/simulator/`). See scenario AS-10.

### Live-mothership e2e recipe (how scenarios drive the real pipeline)

Per `tests/e2e/e2e_test.go`: build `./cmd/mothership` and `./cmd/sim`
(`e2e_test.go:207`, `e2e_test.go:330`), spawn the mothership on an ephemeral address
(`SPAXEL_E2E_BIND_ADDR` override, `e2e_test.go:151-162`), gate on `/healthz`, then run the
sim against `ws://<addr>/ws/node`. Acceptance tests assert over REST:
`/api/blobs` (`getBlobsResponse`), `/api/events?type=…` (`getEventsByType`).

### Assertion surfaces relevant to C5

- `DwellState.String()` returns `"STATIONARY_DETECTED"` (`internal/signal/breathing.go:670`);
  dwell/breathing state is broadcast per link as `MotionStateItem.BreathingState`,
  `BreathingBPM`, `BreathingFreqHz` (`internal/ingestion/server.go:1187-1220`).
- Breathing is computed only while the room is still: `SmoothDeltaRMS < BreathingMotionThreshold`
  (`internal/signal/processor.go:105`).
- `stationary_detected` is a valid event type in the events vocabulary
  (`internal/api/events.go:186`).

---

## 4. Capability → scenario map

### C1 — Approximate 2D position, ±0.5–1.0 m with 4+ nodes (README L14)

| Field | Value |
|---|---|
| Scenario ID | **AS-8** |
| Target file | `as8_2d_position_accuracy_test.go` (**new**, register in `integration_test.go`) |
| Verdict | **covered-by-sim** |
| Sim primitives | live e2e recipe; `spaxel-sim --nodes 4 --walker-type path --path-file <loop> --seed 42 --space 6x5x2.5 --duration 60 --output-csv gt.csv`; engine `AccuracyReport` (`RecallAt1m`, `MedianError`, `P95Error`) and `BlobResult.TrueError` as the error definitions |
| Assertion | median horizontal (XY) error of tracked blobs vs. ground-truth CSV ≤ **1.0 m** (README L14 upper bound). Report RecallAt1m/RecallAt2m as diagnostics; the 0.5 m end of the L14 band is a non-gating target, the 1.0 m end is the gate |
| Determinism | `--seed 42` + scripted `--path-file` polyline; fixed default `--noise-sigma` |
| hardware-required | **false** — path loss, wall attenuation and Fresnel fusion are fully modeled; real RF adds nothing the error bound depends on |

Also carries guard **N1** (see §5).

### C0 + C2 — Presence (L13) and motion/trajectory tracking (L15)

| Field | Value |
|---|---|
| Scenario ID | **AS-2-ext** (no new number — extends the existing scenario) |
| Target file | `as2_walking_detection_test.go` (**extend-in-place**) |
| Verdict | **covered-by-sim** |
| Existing coverage | presence: blob count > 0 for > 80 % of the run, appear < 3 s, disappear < 5 s after stop (file header + `AS2_PersonDetectedWhileWalking`) — this already realizes C0 ("reliably, with 2+ nodes": the test runs `nodes = 2`), so **no duplicate presence scenario is planned** |
| Extension for C2 | with the walker on a scripted path (`--walker-type path --path-file`), sample `/api/blobs` for the run and assert the per-sample horizontal distance from blob to the ground-truth polyline ≤ **1.0 m for ≥ 80 % of tracked samples**. C2 (L15) carries no numeric figure of its own; the bound is borrowed from L14's ±1.0 m and cited as such |
| Sim primitives | `NewPathWalker`/`--path-file`, `--output-csv` ground truth, `as8GetBlobs` (the bare-array `/api/blobs` decoder — the shared envelope helper sees zero live blobs, see C0 row in §8) |
| Determinism | fixed seed + scripted polyline (the added assertion must use the scripted walker, not the existing random-walk fixture, or the bound is unfalsifiable) |
| hardware-required | **false** — "follows moving people" is a pipeline property fully exercised by synthetic frames |

### C3 — Rough person count, 1 vs 2+, degrades at 3+ (README L16)

| Field | Value |
|---|---|
| Scenario ID | **AS-9** |
| Target file | `as9_person_count_test.go` (**new**, register in `integration_test.go`) |
| Verdict | **covered-by-sim** |
| Sim primitives | `WalkerSet`/`CreatePathWalkers` on **disjoint** scripted paths (opposite halves of the room, so Fresnel zones don't merge by construction); three `spaxel-sim` sub-runs at fixed seed with `--walkers 1|2|3` |
| Assertions | (a) 1 walker → steady-state median blob count == **1** (L16 "distinguishes 1"); (b) 2 walkers → ≥ **2** distinct blobs simultaneously present for ≥ 50 % of the run (L16 "vs 2+"); (c) 3 walkers → **stability-only**: run completes, blob count stays in [1, `max_tracked_blobs`], no crash — explicitly *not* asserted == 3, per L16 "degrades at 3+" |
| Determinism | `--seed 42`; disjoint `--path-file` polylines per walker |
| hardware-required | **false** — count degradation comes from Fresnel-zone merging, which the engine models |

Also carries **N3** (5+ walkers) — see §5.

### C4 — Rough Z-axis ±1–2 m with mixed-height nodes (README L17)

| Field | Value |
|---|---|
| Scenario ID | **AS-3-ext** (extends existing fall scenario — no new number) |
| Target file | `as3_fall_detection_test.go` (**extend-in-place**) |
| Verdict | **covered-by-sim** (conditional on one CLI extension, below) |
| Existing coverage | the *fall-detection* half of L17 ("enables fall detection") is already realized: trigger on rapid descent + low Z, confirmation window, webhook, `fall_alert` event fields `start_z`/`end_z`/`peak_velocity`, and the `bag-on-couch` false-positive control. **No duplicate fall scenario is planned** |
| Extension for the Z-accuracy claim | assert \|blob.z − walker ground-truth z\| ≤ **2.0 m** (L17 "±1–2 m" upper bound) at two ground-truth heights: standing (walker `Height` = 1.7 m) and post-fall floor (`EndZ` ≈ 0 m). The 1.0 m end of the L17 band is a non-gating target |
| Required sim change | the CLI places every node at Z = 2.0 m (`cmd/sim/main.go:457-466`) — L17 is conditioned on *mixed-height* placement. Surface the engine's existing parity-alternating `DefaultNodePositions` (`internal/simulator/node.go:281`, Z ∈ {0.25, 0.75}·height) to the CLI (e.g. `--node-heights mixed`). Until that lands, the deterministic fallback is the in-process `Engine`, which already uses mixed heights |
| Determinism | `--seed 42`; fall choreography is already parameterized and scripted (`--fall-delay/--fall-duration/--stillness`, `FallScenarioParams`) |
| hardware-required | **false** — Z geometry is 3-D end to end (node heights, walker height, grid Z resolution 0.10 m); no RF property is missing |

### C5 — Stationary-person detection via breathing 0.1–0.5 Hz (README L18)

| Field | Value |
|---|---|
| Scenario ID | **AS-10** |
| Target file | `as10_stationary_breathing_test.go` (**new**, register in `integration_test.go`) |
| Verdict | **covered-by-sim** — but *blocked on a simulator extension*: neither `cmd/sim` nor `internal/simulator` models micro-motion today, so a stationary walker currently produces a phase-constant link and the pipeline's breathing path sees nothing |
| Required sim change | add a scripted micro-oscillation to the walker model: chest-wall displacement of a few millimeters at a fixed frequency/phase (e.g. 0.2 Hz ≈ 12 BPM), exposed as `--scenario stationary` (or a `breathing_hz` walker field). The displacement flows through the existing `generateCSIFrame` phase model, so `internal/signal`'s BreathingDetector (passband 0.1–0.5 Hz) is exercised unmodified |
| Sim primitives | pipeline `BreathingDetector` / `DwellTracker`; assertion surfaces: `MotionStateItem.BreathingState` reaching `"STATIONARY_DETECTED"` (`internal/signal/breathing.go:670`) and `BreathingBPM`/`BreathingFreqHz` on the motion-state broadcast (`internal/ingestion/server.go:1187-1220`); `getEventsByType(…, "stationary_detected")` as the REST-side check (vocabulary: `internal/api/events.go:186`) |
| Assertions | (a) with a stationary oscillating walker held still (low deltaRMS — breathing is only computed when `SmoothDeltaRMS < BreathingMotionThreshold`, `internal/signal/processor.go:105`), `BreathingState` transitions to `STATIONARY_DETECTED` within the dwell window; (b) reported `BreathingBPM` ∈ **[6, 30]** — the L18 band 0.1–0.5 Hz converted to BPM (0.1 Hz = 6, 0.5 Hz = 30); (c) negative control: empty room (`--walkers 0`) never reaches `STATIONARY_DETECTED` |
| Determinism | the oscillation is scripted (fixed frequency, fixed phase, no RNG); fixed seed for the ambient walkers |
| hardware-required | **false** — the claim is about detecting periodic micro-motion, not about human physiology; a deterministic sinusoidal displacement exercises the identical detection path. (This is the one claim where "sim can exercise it" requires building the extension first — still not hardware.) |

---

## 5. Not-achievable claims (README L20) → verdicts

Each negative claim gets an explicit verdict and a concrete mechanism so the suite *pins the
limit* instead of silently ignoring it.

| Claim | Verdict | Mechanism |
|---|---|---|
| **N1 — sub-10 cm accuracy** | **covered-by-sim** (guard in AS-8) | `as8_2d_position_accuracy_test.go` asserts the resolution floor: the grid cell read from `/api/settings` (`SPAXEL_GRID_CELL_M`, default 0.2 m) is ≥ **0.10 m**, and the suite-wide policy recorded here: **no acceptance assertion may pin position error below 0.10 m**. AS-8's positive gate stays at L14's ≤ 1.0 m. A future PR that tightens an accuracy bound below 0.10 m should fail review against this map |
| **N2 — skeletal pose** | **covered-by-sim** (negative-surface assertion in AS-3-ext) | `as3_fall_detection_test.go` already works with the coarse, Z-derived posture vocabulary (`posture` field on tracks — `internal/api/tracks.go:36`; fixtures use `lying`, `standing`). The extension adds a surface assertion: `/api/blobs` and the track payload expose only position/velocity/confidence/posture — **no joint or skeletal structure** — pinning that the advertised surface is posture-class, not pose |
| **N3 — reliable 5+ person tracking** | **covered-by-sim** (degradation run in AS-9) | `as9_person_count_test.go` adds a fourth sub-run with `--walkers 5` at fixed seed: assert run completes, blob count stays bounded in [1, `max_tracked_blobs`], no crash — and **never assert 5 distinct blobs**. The measured undercount/merge rate is logged, documenting L20's "not reliable" rather than fighting it |

All three are exercised in simulation; none is hardware-required (the absence of a
capability is a property of the pipeline and its resolution, which the sim reproduces).

---

## 6. Determinism policy (applies to every scenario above)

1. **Fixed seed everywhere:** `--seed 42` in all `spaxel-sim` invocations; the seeded-RNG
   contract is already test-pinned (`cmd/sim/main_test.go:180-213`).
2. **Scripted trajectories for any assertion with a spatial bound:** `--walker-type path` +
   `--path-file` (or `NewPathWalker` in-process). Random-walk fixtures are only used where the
   assertion is distribution-level (AS-2's detection-ratio) — never against a numeric error
   bound.
3. **Scripted scenario choreography:** fall via `FallScenarioParams` (deterministic descent
   velocity, `EndZ`, stillness window), breathing via the scripted oscillator (AS-10).
4. **Ground truth by construction:** `--output-csv` / `SimWalker.TrueHistory` is the reference
   for every error assertion; the test never infers ground truth from the system under test.
5. **Explicitly non-asserted regions:** where README says a capability *degrades* (3+ people)
   or is *not achievable* (5+ tracking, sub-10 cm), the scenario asserts stability bounds only
   and records measured values — it never asserts a passing grade the README doesn't claim.

---

## 7. Implementation notes

- New files (`as8`, `as9`, `as10`) must register their `AS<N>_<Name>` entries in
  `integration_test.go`'s scenario table (`integration_test.go:64-70`) — a file alone is
  inert under `TestAcceptanceScenarios`.
- Two simulator extensions are prerequisites, both small and both reusable:
  1. **Mixed-height CLI node placement** (AS-3-ext) — reuse
     `internal/simulator.DefaultNodePositions`; no new algorithm.
  2. **Breathing micro-oscillation** (AS-10) — scripted sinusoidal displacement in the
     walker/generator; no pipeline change.
- Ordering: AS-8 and AS-9 are implementable against HEAD's sim today (scripted paths,
  ground-truth CSV, disjoint walker sets all exist). AS-3-ext's Z assertion and AS-10 are
  gated on the extensions above.
- The pre-existing AS-5 double-use (`as5_ota_test.go` / `as5_wifi_restart_race_test.go`)
  was resolved on 2026-09-25 (spaxel-1cd1155f) by rehoming, not renumbering: the WiFi
  restart race verification is a bf-9gfph firmware-fix check, not an acceptance scenario,
  so it moved to `wifi_restart_race_test.go` (functions `WiFiRestartRace_*`) outside the
  `asN_` namespace. Renumbering was not available — AS-10 is already assigned, and the
  contiguity rule in §2 rules out a second scenario at 11+ while AS-10 has no test file.
  The number 5 now has exactly one scenario (`as5_ota_test.go`); see the numbering rules
  in §2.

---

## 8. Recorded acceptance status (as of 2026-09-25)

Measured outcomes of the scenarios above at the current tip of `main`. This
section is what `README.md`'s capability bullets cite for their present-tense
status; update it whenever a scenario re-measures. Gates are never loosened to
pass — a measured FAIL is the recorded outcome until the owning defect closes
(the AS-8 header precedent). Measurements live on their owning beads; the bead
IDs are given so the numbers stay traceable.

| Capability | Scenario | Status at HEAD | Recorded measurement |
|---|---|---|---|
| C0 presence | AS-2 | **working** — detection demonstrated; AS-2's own live leg is red on a shared test-helper defect (decodes a lowercase `/api/blobs` envelope; the live endpoint returns a bare array with Go-default capitalized keys), not on detection | deterministic runs record a scripted walker detected for 100 % of the post-warmup window (AS-8/AS-9 logs, seed 42) |
| C1 2D position | AS-8 | **measured above gate** | median XY error 1.140 m / 1.273 m vs the 1.0 m gate (two runs, seed 42); p90 1.474 m; RecallAt1m ≈ 25 %, RecallAt2m 100 %; N1 grid-cell guard PASS at 0.200 m (spaxel-28131727) |
| C2 trajectory | AS-2-ext | **measured above gate** — extension landed, honest FAIL per the AS-8 precedent | scripted-rectangle walker (seed 42, 4 nodes): 45/45 post-warmup polls tracked (detection 100 %); nearest-blob distance to the ground-truth polyline median 1.140 m, p90 1.500 m, within the 1.0 m bound 11.1 % (5/45) vs the ≥ 80 % gate — **FAIL**; CSV cross-check median 1.140 m matches C1's per-point figure exactly; fixture audit: recorded walker positions hug the polyline (median 0.028 m, max 0.096 m) (spaxel-aea34d4d) |
| C3 person count | AS-9 | **partially met** — halves inverted by the spaxel-33776a7f fragmentation fix: "1" half now genuinely passing, "2+" half measured failing | 1 walker → median 1.0, == 1 gate **PASS** (45 post-warmup polls, min 1 max 2); 2 walkers → ≥ 2 blobs for 20.0 % of polls vs the ≥ 50 % gate, **FAIL** (min 1 max 3 — 36/45 polls served exactly 1 blob with both walkers active); 3- and 5-walker stability PASS (spaxel-92ce3d2a at HEAD 0eb47693, seed 42; supersedes the spaxel-501751c2 pre-fix run) |
| C4 Z-axis / fall | AS-3 + AS-3-ext | **working** — measured PASS | fall chain fires with the bag-on-couch false-positive control; Z gate \|Δz\| ≤ 2.0 m: median 0.40 m standing, 1.00 m post-fall floor, 2/2 runs; N2 posture-class surface pin PASS (spaxel-501751c2) |
| C5 stationary/breathing | AS-10 | **not validated** — fixture landed (`cmd/sim/breathing.go`, `--scenario stationary`), acceptance test not yet written | STATIONARY_DETECTED is unreachable end to end while two open P1 detector defects stand: spaxel-a27b6dba (breathing RMS no-op — the mean OLS residual over the data subcarriers is identically zero) and spaxel-d1790d51 (DwellTracker feeds its 2 Hz-designed FFT at the 20 Hz frame rate) |
| N1 sub-10 cm | AS-8 guard | **pinned** | grid_cell_m = 0.200 m ≥ the 0.10 m floor; no assertion pins error below the floor |
| N2 skeletal pose | AS-3-ext guard | **pinned** | posture-class key allowlist on `/api/blobs` + `/api/tracks` — any new key fails the test |
| N3 5+ tracking | AS-9 guard | **pinned** | 5-walker run is stability-only: bounded blob count, no crash, 5 distinct blobs never asserted (measured median 2.0, max 3 = logged undercount; spaxel-92ce3d2a) |
