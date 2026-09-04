# Profiling Signal → Code Location Mapping — onboard timer/leak family

**Date:** 2026-09-04
**Bead:** spaxel-e4ad65a6 (cross-reference profiling signals with code patterns)
**Cross-references:**
- Pattern inventory (blocker bead spaxel-f86f4b2c): `docs/research/timer-websocket-pattern-inventory.md`
- Prior catalog: `docs/notes/leak-sources-catalog.md`
- Prior analyses: `dashboard/CONFIRMED_LEAK_REPORT.md`, `dashboard/LEAK_PROFILING_ANALYSIS.md`

**Scope:** every profiling artifact in the repo, mapped to the code that produced
each signal, with a leak-likelihood verdict per pattern. All line numbers verified
at HEAD (`18a828e2`).

---

## 1. Verdict up front

**No pattern in the measured window is an actual leak.** Every signal that the
profiling artifacts flag as a leak decomposes, once mapped to code, into either
(a) an instrument semantics artifact, (b) GC-timing noise, or (c) a
fire-and-forget one-shot timer idiom that is bounded per run and does not grow.
The two genuinely repeating timers (`pollTimer`, `calibrateTimer`) and the
WebSocket mock all show **zero** delta in every recorded run — they are properly
managed. One prior "confirmed root cause" is refuted by runs taken after its fix
landed (§4.2).

---

## 2. Inputs — what data exists, and who wrote it

| Artifact | Producer (code) | Runs recorded | Notes |
|---|---|---|---|
| `dashboard/test-profiling-results.json` | `js/onboard.test.js:69-118` hooks → `js/testProfiler.js:148` `writeProfilingData` | 3 at HEAD + 4 more on disk (2026-09-04, uncommitted twin output — **not** committed by this bead) | Invalid JSON as a whole (§6.5); parsed here by brace extraction |
| `dashboard/leak-detection-report.json` | `js/onboard.leak-detection.test.js:23` | 2 (2026-08-23, pre-fix) | Valid JSON |
| `dashboard/leak-test-full-lifecycle.json` | `js/onboard.leak-detection.test.js:154` | 3 (2 × 2026-08-23, 1 × 2026-09-04) | Invalid JSON as a whole; brace-extracted |
| `dashboard/leak-isolation-results.json` | `js/onboard.leak-isolation.test.js:30` | 3 cases | Valid JSON; `writeFileSync` rewrite in `afterAll` |
| `dashboard/CONFIRMED_LEAK_REPORT.md` | hand-written | — | Root cause **refuted** below (§4.2) |
| `dashboard/LEAK_PROFILING_ANALYSIS.md` | hand-written | — | Documents the 2026-08-27 run |

Instrumentation itself landed in `df2a6207` (2026-08-23, "add test profiling to
capture leak evidence"); the `afterEach` fix landed in `ca6483ce` (2026-08-27
09:54, "add afterEach cleanup to Wizard lifecycle block").

---

## 3. The signals, as recorded

All seven `test-profiling-results.json` runs, suite = `js/onboard.test.js`
(66 `test()` call sites at HEAD):

| Run (UTC) | heap before | heap after | heap Δ | timeouts | intervals | websockets | pid |
|---|---|---|---|---|---|---|---|
| 2026-08-27 11:57:52 | 65,792,264 | 79,613,584 | **+13,821,320** | 2 → 29 (+27) | 0 → 0 | +0 | 1316713 |
| 2026-08-29 09:55:59 | 108,293,160 | 105,993,840 | **−2,299,320** | 2 → 29 (+27) | 0 → 0 | +0 | 4010571 |
| 2026-08-29 09:56:37 | 62,644,368 | 59,672,520 | **−2,971,848** | 2 → 29 (+27) | 0 → 0 | +0 | 4011935 |
| 2026-09-04 13:38:33 | 65,417,912 | 79,119,600 | **+13,701,688** | 2 → 29 (+27) | 0 → 0 | +0 | 2178676 |
| 2026-09-04 13:39:41 | 62,243,344 | 59,819,248 | **−2,424,096** | 2 → 29 (+27) | 0 → 0 | +0 | 2184938 |
| 2026-09-04 13:56:16 | 50,018,816 | 65,650,208 | **+15,631,392** | 2 → 29 (+27) | 0 → 0 | +0 | 2222481 |
| 2026-09-04 15:23:27 | 49,760,808 | 64,779,256 | **+15,018,448** | 2 → 29 (+27) | 0 → 0 | +0 | 2506883 |

Three properties carry all the information:

1. **The timer delta is exactly +27 in 7/7 runs across 8 days and 5 distinct
   PIDs.** It never varies by one unit, and it never accumulates run over run.
2. **The heap delta flips sign** (+13.8 / −2.3 / −3.0 / +13.7 / −2.4 / +15.6 /
   +15.0 MB). The suite-end heap values (79.6, 106.0, 59.7, 79.1, 59.8, 65.7,
   64.8 MB) have **no upward trend**.
3. **Intervals and WebSockets are exactly 0 in every run.**

Isolation runs (`leak-isolation-results.json`, 2026-08-27 12:11), suite =
`js/onboard.leak-isolation.test.js`:

| Recorded name | timeouts | heap Δ | verdict |
|---|---|---|---|
| `fake-timers-with-cleanup` | 2 → 3 (+1) | −183,400 B | **LEAKS** |
| `wizard-lifecycle-with-aftereach` | 3 → 4 (+1) | +82,176 B | **LEAKS** |
| `settimeout-beforeall-hook` | 4 → 6 (+2) | −134,096 B | **LEAKS** |

Other artifacts: `leak-detection-report.json` (2026-08-23, pre-fix) records
timeout Δ **+16** with heap Δ **−3,350,976 B** (negative) in both of its runs;
`leak-test-full-lifecycle.json` records timeout Δ **+2** in all three of its runs.

---

## 4. Signal → code mapping

### 4.1 Signal A — `timeoutDelta: +27`, constant ("medium timer-leak")

**Instrument:** `js/testProfiler.js:83-103` (`instrumentTimers`) monkey-patches
`global.setTimeout`/`setInterval` and inserts every created handle into a `Map`;
**the map is only ever shrunk by an explicit `clearTimeout`/`clearInterval`**
(`:99`, `:103`). A timer that *fires and completes* is dead in the event loop
but stays in the map forever. So the metric is
**"created − explicitly cleared", not "alive at suite end"** — and the
`analyzeLeaks` rule `after.timers.total > before.timers.total`
(`js/testProfiler.js:210`) turns that hygiene counter into a "timer-leak" issue.

**Code that produces the 27:** the dominant timer idiom in `js/onboard.js` is
the fire-and-forget one-shot, which by design is never handed to `clearTimeout`.
Mapped call sites (line numbers from the pattern inventory, re-verified at HEAD):

| `js/onboard.js` | Role | Cleared? | Leak? |
|---|---|---|---|
| `:177` | per-read race-loser timeout in a `Promise.race` serial-read loop | no | no — fires, then is garbage |
| `:304` | browser-check auto-advance, 400 ms one-shot | no | no |
| `:426` | 600 ms `timedOut` race-loser | no | no |
| `:566`, `:818` | 1200 ms step auto-advance one-shots | no | no |
| `:901` | 5000 ms reject race-loser | no | no |
| `:1078`, `:1179` | `await`-style one-shot sleeps | no | no |
| `:1119`, `:1210` | remaining+50 ms reject race-losers | no | no |
| `:1170` | 2000 ms write-timeout race-loser | no | no |
| `:1354` | 1000 ms post-poll advance one-shot | no | no |
| `:1300` | **`state.pollTimer = setInterval`** | **yes** `:1308`, `:1347`, `:1362`, `:1769` | no |
| `:1588` | **`state.calibrateTimer`** self-re-arming 200 ms chain | **yes** `:1390` | no |
| `:1402` | `state.ws = new WebSocket(url)` | closed in afterEach | no |

Plus the harness's own plumbing: `js/onboard.test.js:80` and `:90` create
`setTimeout(resolve, 0)` handles inside `beforeAll`/`afterAll` that are never
cleared — they resolve by execution and stay counted.

**Second instrument property that caps what this signal can see:**
`jest.useFakeTimers()` replaces the global timer functions while active, so any
timer created under fake timers bypasses the tracker wrapper entirely. The +27
is a **real-timers-only** count. This is undocumented in
`LEAK_PROFILING_ANALYSIS.md`.

**Likelihood: NONE (not a leak).** A growth leak would vary with the number of
tests and accumulate across runs; this value is *bit-identical* across 8 days,
two of which bracket a source fix to the suite. +27 is the suite's steady-state
count of "one-shot timers whose ids were never passed to `clearTimeout`".

### 4.2 Signal B — the "confirmed" root cause (missing `afterEach`)

`dashboard/CONFIRMED_LEAK_REPORT.md` names `'Wizard lifecycle'`
(`js/onboard.test.js:529-600`) with `jest.useFakeTimers()` at "560, 586" and no
`afterEach` as the leak source, and prescribes adding
`afterEach(() => { jest.useRealTimers(); })`.

**That fix is in the tree, and the signal did not move.**

- `js/onboard.test.js:532-534` — `afterEach(() => { jest.useRealTimers(); })`,
  added by `ca6483ce` (2026-08-27 09:54), i.e. the exact prescribed fix.
- The **same-day 11:57** profiling run still reports +27 — and the pre-fix runs
  of 2026-08-23 reported **+16**. The delta went *up* when the fix landed, not
  down.
- Every fake-timer-using describe block now carries the hook: `:713`,
  `:953`, `:1150-1152`, `:1243-1245`, `:1331-1333`, in addition to `:532-534`.
- The remaining blocks without an `afterEach` — `'Step definitions'` `:140`,
  `'Error message mapping'` `:636`, `'Provisioning payload assembly…'` `:1050`
  (its `afterEach` at `:1052` only clears encoded data) — contain **zero**
  `jest.useFakeTimers()` calls, so the report's "Additional Blocks Requiring
  Same Fix" table (`CONFIRMED_LEAK_REPORT.md:129-137`) prescribes cleanup for
  blocks that have nothing to clean up.

**Likelihood: REFUTED.** The missing `afterEach` was neither necessary nor
sufficient for the +27. `CONFIRMED_LEAK_REPORT.md` should be read as historical;
its "16+ leaked timers per test run" and "+3 MB heap growth" impacts
(`:113-115`) describe the pre-fix 2026-08-23 data and do not describe HEAD.

### 4.3 Signal C — heap growth flagged "high" (> 5 MB)

**Instrument:** `js/testProfiler.js:199-207` — a single run's
`heapUsed` delta crossing 5 MB, after `profiler.forceGC()` in both hooks
(`js/onboard.test.js:76`, `:88`).

**Code that produces the peaks (+13.7 to +15.6 MB):**

1. **Normal jsdom work.** 66 tests each build and tear down wizard DOM;
   the suite's working set is tens of MB, so the sign of the delta depends on
   how much of the garbage `forceGC()` manages to collect before the snapshot.
   The 7-run table is the evidence: suite-end heap has no trend, and three runs
   are *negative*. A real leak cannot have a negative run.
2. **The profiler retains what it measures.** `js/testProfiler.js:86-89` stores
   `{ created, stack: new Error().stack }` per timer handle — a multi-KB stack
   string plus the timer closure — and never evicts fired handles, for the whole
   suite. `instrumentWebSockets` does the same for WS instances. The instrument
   adds a fixed per-timer cost to the very heap metric it reports.
3. The bimodal shape (peaks land when the run starts from a low ~50-65 MB
   baseline; negative deltas when it starts from ~62-108 MB) is GC-timing
   noise around a stable working set, not retention.

**Likelihood: NONE (measurement artifact).** Note the *same* suite produced
−14.4 MB in `leak-test-full-lifecycle.json`'s 2026-09-04 run — a "high-severity
heap leak" that runs backwards.

### 4.4 Signal D — `intervalDelta: 0` and `websocketDelta: 0` in 7/7 runs

These are the **negative results**, and they are the strongest signal in the
data, because these are the two patterns that *can* actually leak:

| Pattern | Code | Why it cannot leak |
|---|---|---|
| Node-detection poller | `js/onboard.js:1300` `state.pollTimer = setInterval` | cleared on success `:1308`, on error `:1347`, on step exit `:1362`, and in `close()` `:1769`; handle held in `state`, so no orphan copy exists |
| Calibration ticker | `js/onboard.js:1588` `state.calibrateTimer = setTimeout(tick, 200)` (self-re-arming chain, the setInterval-equivalent) | chain terminates itself at `durationMs`; mid-flight cancellation goes through `close()` → `:1390` `clearTimeout(state.calibrateTimer)`; handle held in `state` |
| WebSocket | `js/onboard.js:1402` `state.ws = new WebSocket(url)` | closed by the `afterEach` hooks at `js/onboard.test.js:200`, `:245`, `:278`, `:323`, `:426`; tracker count 0 at suite end in 7/7 runs |

Every timer-owning describe block also clears both handles unconditionally in
`afterEach` (`js/onboard.test.js:163-164`, `:198-199`, `:243-244`, `:276-277`,
`:321-322`, `:424-425`) — the "proven pattern" that `CONFIRMED_LEAK_REPORT.md:144`
refers to.

**Likelihood: NONE — properly managed.** This corroborates
`LEAK_PROFILING_ANALYSIS.md` §"WebSocket State".

### 4.5 Signal E — isolation verdicts `LEAKS` on flat heap

**Instrument:** `js/onboard.leak-isolation.test.js:126`, `:152`, `:189`, `:222`,
`:246`, `:282`, `:317`, `:348`, `:375` — nine copies of

```js
verdict: after.timers.total > before.timers.total ? 'LEAKS' : 'CLEAN'
```

Any single un-cleared one-shot — including a timer that already fired — trips
it. That is why all three recorded cases are `LEAKS` with heap deltas of
−183 KB, +82 KB and −134 KB (flat to 4 significant figures). The `LEAKS`
verdict carries no information about leaks.

**Coverage gap:** the suite defines nine isolation cases (`ISOLATION-1` …
`ISOLATION-9`, `js/onboard.leak-isolation.test.js:104`-`:356`), including
`ISOLATION-4: Wizard lifecycle WITH afterEach cleanup` (`:197`) — the control
that would have falsified the verdict — but `leak-isolation-results.json`
records only three, under names (`fake-timers-with-cleanup`,
`wizard-lifecycle-with-aftereach`, `settimeout-beforeall-hook`) that match no
HEAD test title. The file is written by `writeFileSync` in `afterAll`
(`:30-33`), so the recorded run came from an earlier uncommitted variant and
pushed only three results. The isolation matrix is **incompletely recorded**;
do not cite it as a 9-case result.

---

## 5. Properly managed vs actual leak — the AC-4 table

| # | Pattern | Code | Evidence | Verdict |
|---|---|---|---|---|
| 1 | Node-detection polling interval | `js/onboard.js:1300` | interval Δ 0 in 7/7 runs; 4 clear sites | **Properly managed** |
| 2 | Calibration ticker (self-re-arming) | `js/onboard.js:1588` | no accumulation; cleared in `close()` `:1390` | **Properly managed** |
| 3 | WebSocket link | `js/onboard.js:1402` | WS Δ 0 in 7/7 runs; closed in 5 `afterEach` hooks | **Properly managed** |
| 4 | Fake timers in tests | `js/onboard.test.js:564`, `:590`, `:718`…, `:970`…, `:1156`, `:1199`, `:1250` | `afterEach(useRealTimers)` present in every fake-timer block since `ca6483ce` | **Properly managed (post-fix)** |
| 5 | One-shot auto-advance / sleep timers | `js/onboard.js:304`, `:566`, `:818`, `:1078`, `:1179`, `:1354` | fire and die; bounded count per run | **Not a leak** — no action needed |
| 6 | Race-loser reject timeouts | `js/onboard.js:177`, `:426`, `:901`, `:1119`, `:1170`, `:1210` | counted by the instrument, but fire and die | **Not a leak** — genuine hygiene debt, LOW priority (§7) |
| 7 | Suite heap footprint | `js/onboard.test.js` (66 tests, jsdom) | sign flips across runs, no trend | **Not a leak** — GC noise |
| 8 | Timer/heap leak, unbounded growth | — | **no such signal exists in any artifact** | **No actual leak found** |

Out of the measured window but relevant to the family: the pattern inventory
(blocker deliverable, observation 2) lists 13 `dashboard/js` files whose
`setInterval` has no same-file `clearInterval` — app-lifetime pollers, safe
unless re-initialized on navigation. That app-side question is tracked in
`docs/notes/leak-sources-catalog.md` and is **not** what the profiling
artifacts measure.

---

## 6. Instrument defects (why the artifacts say "leak")

These are the root causes of the *signals*, as opposed to the code. They matter
because the sibling beads (`spaxel-9e7d77ca` catalog, `spaxel-46656f8a` heap
profiling, `spaxel-dd11eef9` isolation) will otherwise rank instrument noise as
product defects.

1. **Counter semantics** — `js/testProfiler.js:86-89`, `:99`, `:103`: fired
   handles are never evicted; the metric is "created − explicitly cleared".
   Fix: wrap the callback so firing deletes the entry, or read live handle
   counts instead.
2. **Verdict predicate** — `js/onboard.leak-isolation.test.js:126` (+ 8 copies)
   and `js/testProfiler.js:210`: `after > before` ⇒ leak. A single +1 must be
   `CLEAN`; `LEAKS` should require a monotonic trend across repeated runs *and*
   heap corroboration.
3. **Heap threshold without a protocol** — `js/testProfiler.js:199`: one run,
   `> 5 MB`, with `forceGC()` as the only baseline control (and `--expose-gc`
   only mentioned as optional in `LEAK_PROFILING_ANALYSIS.md:146`). Fix: N runs,
   `--expose-gc`, compare medians of suite-end heap.
4. **Self-retention** — the tracker maps (`js/testProfiler.js:86-89`, and the WS
   equivalent) retain a closure plus a stack string per timer for the whole
   suite, inflating the heap metric they report.
5. **Malformed artifact files** — `js/testProfiler.js:186-188` appends
   `,\n{...}` to `test-profiling-results.json` and never writes the closing
   bracket, so the file is a concatenation, not JSON: `json.loads` fails with
   `Extra data` at char 2501 at HEAD. 3 runs are committed at HEAD; 7 exist on
   disk. Consumers must brace-extract (as this analysis did). Fix: rewrite the
   file per run, or write JSON lines.
6. **Fake-timer blindness** — creations under `jest.useFakeTimers()` bypass the
   patched globals, so the +27 is real-timers-only. Undocumented.
7. **Stale narrative docs** — `CONFIRMED_LEAK_REPORT.md` names a root cause
   whose fix (`ca6483ce`) demonstrably did not change the signal, and
   `LEAK_PROFILING_ANALYSIS.md` presents the single 2026-08-27 peak as "2
   significant leaks". Both should carry a stale pointer to this document
   rather than be treated as current.

---

## 7. Residual code findings (the only actionable code)

Nothing here is a leak. Both are hygiene, and both are LOW priority.

1. **Race-loser timeouts are never cancelled** — `js/onboard.js:177`, `:426`,
   `:901`, `:1119`, `:1170`, `:1210`. When `Promise.race` resolves via the read
   branch, the loser timer still fires later and calls `reject` on an
   already-settled promise (a no-op). Functionally harmless; a long serial
   session allocates one dangling timer per read. `clearTimeout` in a
   `finally` is the idiomatic fix. This is the *entire* set of code locations
   the profiling signals legitimately point at.
2. **Harness self-timers** — `js/onboard.test.js:80`, `:90` create
   `setTimeout(resolve, 0)` handles that nothing clears. Trivially fixable and
   worth doing only if the instrument is fixed first, otherwise the counter
   cannot see the difference.

---

## 8. Answers the sibling beads can take as settled

- **"Which specific test/pattern shows uncontrolled growth?"**
  (`spaxel-46656f8a`) — none. All 7 recorded runs completed and wrote an
  `afterTest` snapshot, including 4 on 2026-09-04; the OOM/hang premise in the
  older reports does not reproduce.
- **"Which component is leaking?"** (`spaxel-dd11eef9`) — no component. The
  isolation suite's `LEAKS` verdicts are an artifact of
  `js/onboard.leak-isolation.test.js:126` and carry no leak information.
- **"Root cause of the observed timer leaks"** — an instrument that counts
  created-minus-explicitly-cleared handles, applied to a codebase whose dominant
  idiom is the fire-and-forget one-shot; the resulting constant +27 was present
  before and after the `ca6483ce` fix that the earlier report credits.
- **"Which patterns to prioritize?"** (`spaxel-9e7d77ca`) — no HIGH/MEDIUM
  product items exist in this window. Rank §7.1 (race-loser `clearTimeout`
  hygiene) LOW, and rank the §6 instrument fixes ahead of any further product
  leak hunting, because until they are fixed every future run will re-derive
  this same false positive.
