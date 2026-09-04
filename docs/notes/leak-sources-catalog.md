# Leak Sources Catalog

**Rev 2 — 2026-09-04** (supersedes rev 1 of 2026-08-27, `87d81346`)
**Bead:** spaxel-9e7d77ca ("Create prioritized leak sources catalog document")
**Line numbers verified at HEAD `407ddc44`** (the only source change in flight on
these files is an unrelated esp-web-tools import guard in `onboard.js`, which
touches no timer code).
**Companion documents:**
`docs/research/profiling-signal-code-mapping.md` (signal → code proof),
`docs/research/timer-websocket-pattern-inventory.md` (full pattern sweep).

---

## 0. What changed in rev 2, and why

Rev 1 ranked the `onboard.js` auto-advance timers as **HIGH-priority leak
sources** on the strength of the raw profiling signals ("16–27 setTimeout calls
not cleared", "heap grew 3–13 MB"). The Sep 4 code mapping
(`profiling-signal-code-mapping.md`) traced every signal to the code that
produced it and refuted that reading:

1. The timer delta is **exactly +27 in 8/8 recorded runs** across 8 days and 8
   distinct PIDs. A growth leak varies with workload and accumulates run over
   run; this number never moves by even one unit, including across a suite fix
   (`ca6483ce`) that rev 1's own "Next Steps" predicted would move it.
2. The heap delta **flips sign** across runs (+13.8, −2.3, −3.0, +13.7, −2.4,
   +15.6, +15.0, **−55.2 MB**) with no trend in suite-end heap. A real leak
   cannot run backwards.
3. Intervals and WebSockets are **0 in every run** — the two patterns that can
   actually leak are the two the instrument shows clean.

Rev 1's HIGH/MEDIUM ranking therefore ranked instrument artifacts as product
defects, and its recommended fixes would have added `clearTimeout` plumbing to
one-shot timers that are dead 400 ms after firing — no behaviour change, more
code. Rev 2 keeps every entry and re-ranks by what the signals actually
support. The full per-signal proof lives in the mapping document; this file is
the prioritized catalog.

---

## 1. Verdict

**No leak source exists in the measured window** (dashboard `js/onboard.test.js`
suite, 2026-08-23 → 2026-09-04, all recorded artifacts). Every entry below is
either (a) an instrument defect that manufactures leak evidence, (b) stale
narrative that misdirects the next investigator, or (c) genuine but harmless
hygiene debt. The strongest signals in the data are the *negative* results:
`pollTimer`, `calibrateTimer` and the WebSocket handle show 0 delta in 8/8 runs.

---

## 2. Priority model

"Signal strength" in rev 1 meant *how big the number looked*. Rev 2 ranks by
how strongly the recorded data implicates each item as something worth acting
on:

- **HIGH** — fires on every recorded run and produces the misleading artifact.
  Acting on these is the only way to stop the false positive from re-deriving
  itself on each future run. Evidence: 8/8 runs identical.
- **MEDIUM** — no per-run artifact, but a standing measurement or documentation
  defect that distorts any future investigation that trusts it.
- **LOW** — real, tiny, and safe to leave: hygiene debt with no functional
  effect.
- **NO ACTION** — verified properly managed. Listed because rev 1 (and the
  profiling artifacts) flag them, and because a future sweep will hit them
  again; each entry records why it is clean so the re-check is one grep.

There is **no HIGH or MEDIUM product-code item**. All HIGH and MEDIUM items are
in the test instrument and the narrative documents. That is the finding.

---

## 3. Quick reference table

| # | Priority | File | Line(s) | What it is | Verdict |
|---|---|---|---|---|---|
| H1 | 🔴 HIGH | `dashboard/js/testProfiler.js` | 86–89, 99, 103 | Timer map is only shrunk by explicit clear — fired handles count forever | Instrument defect |
| H2 | 🔴 HIGH | `dashboard/js/testProfiler.js` | 210 | `after.timers.total > before.timers.total` ⇒ "timer-leak" issue | Instrument defect |
| H3 | 🔴 HIGH | `dashboard/js/onboard.leak-isolation.test.js` | 126, 152, 189, 222, 246, 282, 317, 348, 375 | Same predicate, 9 copies ⇒ `LEAKS` verdicts on flat heap | Instrument defect |
| H4 | 🔴 HIGH | `dashboard/js/testProfiler.js` | 186–188 | Append-only JSON writer emits a concatenation, not JSON | Instrument defect |
| M1 | 🟡 MEDIUM | `dashboard/js/testProfiler.js` | 199–207 | Heap "high" = one run's delta > 5 MB, `forceGC()` the only control | Measurement defect |
| M2 | 🟡 MEDIUM | `dashboard/js/testProfiler.js` | 86–89 (+ WS equivalent) | Tracker retains `{created, stack}` per handle for the whole suite | Self-retention |
| M3 | 🟡 MEDIUM | `dashboard/CONFIRMED_LEAK_REPORT.md`, `dashboard/LEAK_PROFILING_ANALYSIS.md` | — | Root cause refuted; stale numbers presented as current | Stale narrative |
| L1 | 🟢 LOW | `dashboard/js/onboard.js` | 177, 426, 901, 1119, 1170, 1210 | `Promise.race` loser timers never cancelled | Hygiene, not a leak |
| L2 | 🟢 LOW | `dashboard/js/onboard.test.js` | 80, 90 | Harness `setTimeout(resolve, 0)` handles never cleared | Hygiene, not a leak |
| — | ⚪ NO ACTION | `dashboard/js/onboard.js` | 304, 566, 818, 1078, 1179, 1354 | Fire-and-forget one-shots (auto-advance / sleep) | Not a leak |
| — | ⚪ NO ACTION | `dashboard/js/onboard.js` | 1300 → 1308, 1347, 1362, 1769 | `state.pollTimer` polling interval | Properly managed |
| — | ⚪ NO ACTION | `dashboard/js/onboard.js` | 1588 → 1390 | `state.calibrateTimer` self-re-arming chain | Properly managed |
| — | ⚪ NO ACTION | `dashboard/js/onboard.js` | 1402 | WebSocket under test | Properly managed |
| — | ⚪ NO ACTION | `dashboard/js/onboard.test.js` | 532–534 (+ 5 blocks) | `jest.useRealTimers()` afterEach hooks | Properly managed |
| — | ⚪ NO ACTION | `dashboard/js/websocket.js` | 130 → 109, 127; 143 → 155; 199/202 → 206 | Reconnect / disconnect / RAF | Properly managed |
| — | ⚪ NO ACTION | `mothership/internal/*` | see §6 | 9 `time.AfterFunc` sites | Properly managed |
| — | ⚪ NO ACTION | `dashboard/js/*.js` (13 files) | see §7 | App-lifetime `setInterval` pollers | Legitimate pattern |

---

## 4. HIGH — the instrument manufactures the leak

All four fire on every run. Until they are fixed, every future profiling run
reports a timer leak regardless of what the product code does — that is exactly
what happened across the 8 recorded runs, including the runs taken after
rev 1's prescribed product fix landed.

### H1 — Fired timers are never evicted from the tracker

`dashboard/js/testProfiler.js:83-103` (`instrumentTimers`) monkey-patches the
global timer functions and inserts every created handle into a `Map`
(`:86-89`). The only deletion path is an explicit `clearTimeout`/`clearInterval`
(`:99`, `:103`). A one-shot that *fires and completes* is dead in the event
loop but stays in the map for the rest of the suite. The metric is therefore
**"created − explicitly cleared"**, not "alive at suite end".

Why the number is 27: the dominant idiom in `js/onboard.js` is the
fire-and-forget one-shot (§7, row 1) — 12 `setTimeout` call sites that are
never handed to `clearTimeout` by design, plus the harness's own 2
(`onboard.test.js:80`, `:90`). Several of those sites execute more than once
across the suite's 66 tests, so the run creates 27 un-cleared handles in total
(the count goes 2 → 29 with zero clears). It is a hygiene count, not a census
of live handles.

**Fix:** wrap the callback when inserting, so firing deletes the entry:

```js
global.setTimeout = function (...args) {
    const [cb, delay, ...rest] = args;
    const timeoutId = originalSetTimeout(function (...cbArgs) {
        timerTracker.get('timeouts').delete(timeoutId);
        return cb(...cbArgs);
    }, delay, ...rest);
    timerTracker.get('timeouts').set(timeoutId, { created: Date.now() });
    return timeoutId;
};
```

(or read live handle counts via `process._getActiveHandles()` instead of
tracking creation). Apply the same change to `setInterval` and to
`instrumentWebSockets`.

### H2 — The verdict predicate converts hygiene into "timer-leak"

`dashboard/js/testProfiler.js:210`:

```js
if (after.timers.total > before.timers.total) {
    issues.push({ severity: 'medium', type: 'timer-leak', ... })
```

Any suite containing a single un-cleared one-shot is reported as a timer leak.
Combined with H1, the constant +27 became the "16–27 leaked timers" headline in
rev 1 and in `CONFIRMED_LEAK_REPORT.md`.

**Fix:** `LEAKS` should require a monotonic across-run trend *and* heap
corroboration — e.g. N ≥ 3 runs with strictly increasing live-handle counts and
a same-direction heap trend. A single `+1` must be `CLEAN`.

### H3 — Nine copies of the same predicate in the isolation suite

`dashboard/js/onboard.leak-isolation.test.js` repeats, verbatim, at
`:126`, `:152`, `:189`, `:222`, `:246`, `:282`, `:317`, `:348`, `:375`:

```js
verdict: after.timers.total > before.timers.total ? 'LEAKS' : 'CLEAN'
```

This is why all three recorded isolation cases read `LEAKS` with heap deltas of
−183 KB, +82 KB and −134 KB — flat to four significant figures. The `LEAKS`
verdict currently carries no information about leaks.

Additional caveat when this suite is re-run: `leak-isolation-results.json`
records three case names (`fake-timers-with-cleanup`,
`wizard-lifecycle-with-aftereach`, `settimeout-beforeall-hook`) that match no
HEAD test title, and only 3 of the suite's 9 defined cases
(`ISOLATION-1`…`ISOLATION-9`, `:104`-`:356`). The file is rewritten by
`writeFileSync` in `afterAll` (`:30-33`), so the recorded run came from an
earlier variant. **Do not cite the recorded file as a 9-case result.**

**Fix:** share one `verdictFor(before, after)` helper implementing the H2 rule,
and record all nine cases.

### H4 — The result file is not valid JSON

`dashboard/js/testProfiler.js:186-188` appends `,\n{...}` to
`test-profiling-results.json` and never writes a closing bracket, so the file
is a concatenation of objects: `json.loads` fails with `Extra data` at char
2501 at HEAD (3 runs committed). `leak-detection-report.json` and
`leak-test-full-lifecycle.json` have the same defect. Every consumer has to
brace-extract, and any tooling that assumes valid JSON silently reads nothing.

**Fix:** rewrite the whole file per run (`writeFileSync` of the full run list),
or emit JSON lines (one object per line) and say so in the filename.

---

## 5. MEDIUM — measurement and documentation defects

### M1 — Heap "high" severity is one uncontrolled sample

`dashboard/js/testProfiler.js:199-207`: a single run's `heapUsed` delta
crossing 5 MB raises a `high` issue; `profiler.forceGC()` in the two hooks
(`js/onboard.test.js:76`, `:88`) is the only baseline control. The 8 recorded
runs show the resulting signal is bimodal GC-timing noise: peaks (+13.7 to
+15.6 MB) land when the run starts from a low ~50–65 MB baseline, negative
deltas (−2.3 to −3.0, and −55.2 MB in the most recent run) when it starts
high. Suite-end heap: 79.6, 106.0, 59.7, 79.1, 59.8, 65.7, 64.8, 68.3 MB — no
trend.

**Fix:** N ≥ 3 runs under `node --expose-gc`, compare *medians of suite-end
heap*, and require a same-direction trend before raising an issue. Drop the
per-run > 5 MB rule.

### M2 — The instrument inflates the metric it reports

`dashboard/js/testProfiler.js:86-89` stores `{ created, stack: new Error().stack }`
per timer handle — a multi-KB string plus the closure — and (per H1) never
evicts it. `instrumentWebSockets` does the same for WS instances. Across a
suite that creates 29 tracked timers, the instrument adds a fixed, non-trivial
cost to the very heap number it reports. This is part of why the +13 MB peaks
look scary next to a 60 MB working set.

**Fix:** the H1 eviction removes the unbounded part; additionally do not retain
the stack string past creation (or keep only the last N).

### M3 — Two narrative documents name a refuted root cause

- `dashboard/CONFIRMED_LEAK_REPORT.md` names `'Wizard lifecycle'`
  (`js/onboard.test.js:529-600`) + missing `afterEach` as the confirmed leak
  source and prescribes `jest.useRealTimers()`. The exact prescribed fix is in
  the tree (`:532-534`, added by `ca6483ce`, 2026-08-27 09:54) and every
  fake-timer block now carries one (`:713`, `:953`, `:1150-1152`, `:1243-1245`,
  `:1331-1333`) — **the signal did not move**: pre-fix runs read +16
  (2026-08-23), post-fix runs read +27. The report's "Additional Blocks
  Requiring Same Fix" table (`:129-137`) prescribes cleanup for three blocks
  that contain zero `jest.useFakeTimers()` calls.
- `dashboard/LEAK_PROFILING_ANALYSIS.md` presents the single 2026-08-27 peak as
  "2 significant leaks". Subsequent runs flipped the sign.

**Fix:** prepend a stale-evidence banner to both files pointing at
`docs/research/profiling-signal-code-mapping.md` §4.2/§4.3 and at this
catalog's §1 verdict. Do not treat either file's numbers as describing HEAD.

---

## 6. LOW — the only actionable product/harness code

Neither item is a leak; both are hygiene. They are the *entire* set of code
locations the profiling signals legitimately point at once instrument artifacts
are discounted.

### L1 — Race-loser timeouts are never cancelled

`dashboard/js/onboard.js:177, 426, 901, 1119, 1170, 1210`. When `Promise.race`
resolves via the read branch, the loser timeout still fires later and calls
`reject` on an already-settled promise (a no-op). Functionally harmless; a long
serial session allocates one dangling timer per read.

**Fix (when touched):** `clearTimeout` in a `finally`, or abort the race with
`AbortSignal.timeout()`. Do not do this as a "leak fix" — it changes no leak
measurement except through H1's eviction, where it will finally be visible.

### L2 — Harness self-timers

`dashboard/js/onboard.test.js:80` and `:90` create `setTimeout(resolve, 0)`
handles inside `beforeAll`/`afterAll` that nothing clears. Two of the +27.

**Fix:** hold the id and `clearTimeout` it; only worth doing after H1, since
until eviction exists the counter cannot see the difference.

---

## 7. NO ACTION — verified clean, with the reason recorded

These are the entries rev 1 and the profiling artifacts flag. Each is clean;
the verification is one grep away.

**One-shot auto-advance / sleep timers** — `dashboard/js/onboard.js:304` (400 ms
browser-check advance), `:566` and `:818` (1200 ms step advance), `:1078` and
`:1179` (`await` sleeps), `:1354` (1000 ms post-poll advance). Fire once and
die; bounded per run; rev 1's HIGH entries. No cleanup is appropriate — there
is nothing to cancel once it has fired, and cancelling it early would break the
behaviour.

**Node-detection poller** — `dashboard/js/onboard.js:1300`
`state.pollTimer = setInterval`, cleared on success `:1308`, on error `:1347`,
on step exit `:1362`, in `close()` `:1769`; handle held in `state`, so no
orphan copy exists. Interval delta 0 in 8/8 runs.

**Calibration ticker** — `dashboard/js/onboard.js:1588`
`state.calibrateTimer = setTimeout(tick, 200)`, self-re-arming chain that
terminates itself at `durationMs`; mid-flight cancellation goes through
`close()` → `:1390` `clearTimeout`. No accumulation across runs.

**WebSocket under test** — `dashboard/js/onboard.js:1402`
`state.ws = new WebSocket(url)`, closed by the `afterEach` hooks
(`js/onboard.test.js:200, 245, 278, 323, 426`). WS count 0 at suite end in 8/8
runs.

**Fake-timer hygiene** — every `jest.useFakeTimers()` block in
`js/onboard.test.js` carries `afterEach(() => { jest.useRealTimers(); })`
(`:532-534`, `:713`, `:953`, `:1150-1152`, `:1243-1245`, `:1331-1333`) since
`ca6483ce`. Note: creations under fake timers bypass the patched globals
entirely, so the +27 is a *real-timers-only* count — an instrument blind spot
undocumented before the mapping analysis.

**`dashboard/js/websocket.js`** (dashboard-owned WS client, outside the
measured suite): reconnect timer `:130` cleared `:109` and `:127`; disconnect
state interval `:143` cleared `:155-157`; extrapolation RAF `:199`/`:202`
cancelled `:206-208`. All three have owner-held handles and symmetric cleanup.

**Go backend timers** — all 9 `time.AfterFunc`/batch-timer sites are
owner-held and stopped or scoped; none is in the measured window and none shows
any growth mechanism:

| Site | Purpose | Management |
|---|---|---|
| `mothership/internal/notify/service.go:394` | batch flush | `s.batchTimer` field, stopped before reuse |
| `mothership/internal/notify/service_enhanced.go:235` | enhanced batch flush | `ext.batchTimer` field, stopped before reuse |
| `mothership/internal/notifications/manager.go:408` | batch flush | `m.batchTimer` field, stopped before reuse |
| `mothership/internal/falldetect/detector.go:612` | escalation tier 1 | fires once per alert lifecycle |
| `mothership/internal/falldetect/detector.go:627` | escalation tier 2 | fires once per alert lifecycle |
| `mothership/internal/analytics/anomaly.go:1224` | alert delay | `state.alertTimer`, stopped before reuse |
| `mothership/internal/analytics/anomaly.go:1238` | webhook delay | `state.webhookTimer`, stopped before reuse |
| `mothership/internal/analytics/anomaly.go:1252` | escalation delay | `state.escalationTimer`, stopped before reuse |
| `mothership/internal/automation/engine.go:997` | 30 s delayed action | fires once per trigger |

(Rev 1 listed three of these files with estimated line numbers — "~140",
"~80", "~200-220" — all wrong; the numbers above are verified.)

**App-lifetime pollers** — 13 `dashboard/js` files call `setInterval` with no
same-file `clearInterval`: `accuracy.js`, `anomaly.js`, `apdetection.js`,
`ble-panel.js`, `briefing.js`, `fleet-page.js`, `home-cards.js`,
`linkhealth.js`, `ota.js`, `replay.js`, `security-panel.js`,
`simple-mode.js`, `troubleshoot.js` (pattern inventory, observation 2). These
are deliberate poll-the-whole-page-lifetime loops — legitimate as long as the
panel is never re-initialized on navigation. The only risk is a future SPA-style
re-mount leaking one interval per mount; if that refactor happens, add
teardown then. Not measurable by the current suite and not a defect today.

**`dashboard/static/js/fleet.js:388`** — `otaStaggerMs` delay wrapped in a
one-shot `Promise`; scoped, fires once per OTA batch. (Rev 1 cited
`dashboard/static/js/fleet.js` "~260, not shown in grep" — the real line is
388.)

---

## 8. Recommended fixes, in order (HIGH and MEDIUM only)

1. **H1 + M2** — evict tracker entries on fire; stop retaining stack strings.
   `dashboard/js/testProfiler.js:83-103` and the WS tracker.
2. **H2 + H3** — replace the `after > before` predicate with the
   trend-plus-heap rule; share one helper across the 9 isolation copies.
3. **H4** — rewrite the result JSONs whole per run (or switch to JSON lines).
4. **M1** — heap protocol: N ≥ 3 runs, `--expose-gc`, compare medians of
   suite-end heap; delete the per-run > 5 MB rule.
5. **M3** — stale banners on `CONFIRMED_LEAK_REPORT.md` and
   `LEAK_PROFILING_ANALYSIS.md`.

After 1–4 land, re-run the profiled suite: the expected result is
`timer-leak: 0 issues` and `heap-growth: 0 issues` — because that is what the
product code actually does. Only then do L1/L2 become measurable (and worth
about one line each).

Explicitly **not** recommended, retiring rev 1's prescriptions: adding
`clearTimeout`/`state.autoAdvanceTimer` plumbing to `onboard.js:304, 566, 818,
1354`, and reordering `jest.useRealTimers()` inside `afterEach`. The first is
dead code around already-dead one-shots; the second landed on 2026-08-27
(`ca6483ce`) and demonstrably did not change the signal, because there was no
leak to change.

---

## 9. Settled answers for the open sibling beads

These questions are answered by the recorded data plus the mapping analysis;
a claimer can verify in one grep and decline rather than re-run the
investigation.

| Bead | Question | Settled answer |
|---|---|---|
| `spaxel-46656f8a` | Which test shows uncontrolled heap growth? | None. 8/8 runs completed and wrote an `afterTest` snapshot; suite-end heap has no trend and three deltas are negative, including −55.2 MB in the most recent run (2026-09-04 16:02). |
| `spaxel-dd11eef9` | Which component is leaking? | No component. The `LEAKS` verdicts are the H3 predicate artifact, on flat heap. |
| `spaxel-a0557a23` | Fix the identified timer/interval leak | No leak location exists to clean up. If touched at all, fix the instrument (§8 first). The suite already hangs/OOM-free in every recorded run. |
| `spaxel-78c38fb7` | Verify the cleanup fix eliminates the leak | The cleanup fix (`ca6483ce`, the exact prescribed change) landed 2026-08-27 09:54; the same-day 11:57 run and every run since still read +27. There was nothing to eliminate. |
| `spaxel-6598cb91` | Name the exact leaking component and cleanup line | The "exact component" is the instrument: `testProfiler.js:210` flagging `onboard.test.js:80/:90` + 12 one-shot `onboard.js` sites. The report is this catalog plus the mapping document. |
| `spaxel-1285f33d` | Catalog and prioritize leak sources | This file is that catalog (the beads are twins; same two input JSONs named in the AC). |

---

## Appendix A — the recorded runs

`dashboard/test-profiling-results.json`, suite `js/onboard.test.js`.
3 runs are committed at HEAD; 8 exist on disk at dispatch time (the 5 newer are
uncommitted output from a concurrent worker; their content is consistent with
the committed 3). The file is a non-JSON concatenation (H4) — parsed here by
brace extraction.

| Run (UTC) | heap before (B) | heap after (B) | heap Δ | timeouts | intervals | WS | pid |
|---|---|---|---|---|---|---|---|
| 2026-08-27 11:57:52 | 65,792,264 | 79,613,584 | **+13.82 MB** | 2 → 29 (+27) | 0 → 0 | 0 → 0 | 1316713 |
| 2026-08-29 09:55:59 | 108,293,160 | 105,993,840 | **−2.30 MB** | 2 → 29 (+27) | 0 → 0 | 0 → 0 | 4010571 |
| 2026-08-29 09:56:37 | 62,644,368 | 59,672,520 | **−2.97 MB** | 2 → 29 (+27) | 0 → 0 | 0 → 0 | 4011935 |
| 2026-09-04 13:38:33 | 65,417,912 | 79,119,600 | **+13.70 MB** | 2 → 29 (+27) | 0 → 0 | 0 → 0 | 2178676 |
| 2026-09-04 13:39:41 | 62,243,344 | 59,819,248 | **−2.42 MB** | 2 → 29 (+27) | 0 → 0 | 0 → 0 | 2184938 |
| 2026-09-04 13:56:16 | 50,018,816 | 65,650,208 | **+15.63 MB** | 2 → 29 (+27) | 0 → 0 | 0 → 0 | 2222481 |
| 2026-09-04 15:23:27 | 49,760,808 | 64,779,256 | **+15.02 MB** | 2 → 29 (+27) | 0 → 0 | 0 → 0 | 2506883 |
| 2026-09-04 16:02:12 | 123,462,328 | 68,281,400 | **−55.18 MB** | 2 → 29 (+27) | 0 → 0 | 0 → 0 | 2615575 |

Supporting artifacts: `leak-detection-report.json` (2026-08-23, pre-`ca6483ce`)
timeouts +16 in both runs with *negative* heap delta; `leak-test-full-lifecycle.json`
timeouts +2 in all 3 runs, heap −14.39 MB in its 2026-09-04 run (a "leak" that
runs backwards); `leak-isolation-results.json` 3 cases, all `LEAKS`, heap flat
(−183 KB / +82 KB / −134 KB).

## Appendix B — corrections to rev 1

| Rev 1 claim | Status at HEAD |
|---|---|
| "16–27 setTimeout calls not cleared" = primary leak | Instrument semantics (H1) + hygiene count; not live handles |
| onboard.js :304/:566/:818/:1354 HIGH leak sources | One-shots that fire and die — NO ACTION |
| "Root cause: fake-timers/afterEach interference" | Refuted — fix landed, signal unchanged (M3) |
| websocket.js MEDIUM "reconnect/disconnect timers" | Never leaks; symmetric cleanup; and outside the measured suite |
| Go timers at `notify/service.go` "~140", `service_enhanced.go` "~80", `detector.go` "~200-220" | Real lines 394 / 235 / 612+627 (§6) |
| fleet.js "line ~260" | `dashboard/static/js/fleet.js:388` |
| Heap growth "3–13 MB" | Sign-flipping GC noise, −55 to +16 MB across 8 runs (M1) |
