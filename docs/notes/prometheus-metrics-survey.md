# Prometheus Metrics Survey

**Date:** 2026-09-05
**Context:** Bead `spaxel-c5daa144` — how metrics are defined, registered, exposed and
tested in the spaxel mothership, and where auto-update trigger events can be hooked.
Everything below was verified against `origin/main` (HEAD `d73a938c`), not the working
tree (which carries unrelated in-flight edits — see the last section).

## Where metrics live

All metrics in the codebase live in **`mothership/internal/ota/`**, declared as
package-level vars registered into the **default global registry** via
`promauto` (`github.com/prometheus/client_golang/prometheus/promauto`). There is
exactly one registry in the process — no `prometheus.NewRegistry()` call exists
anywhere, and nothing calls `MustRegister` directly. This is deliberate: a
package-scoped `promauto` var is registered exactly once at process start, and the
`/metrics` route in `cmd/mothership` picks it up with zero wiring.

| Metric | Type | Labels | Declared |
|---|---|---|---|
| `auto_update_triggers_total` | CounterVec | `trigger_type` (`"auto"`), `result` (`"success"`/`"failure"`) | `internal/ota/autoupdate.go:17` |
| `node_firmware_drift_fault` | GaugeVec | `mac`, `node_version`, `expected_version` | `internal/ota/drift.go:24` |
| `node_firmware_drift_seconds` | GaugeVec | `mac` | `internal/ota/drift.go:31` |

Dependency: `github.com/prometheus/client_golang v1.19.1`, a **direct** requirement of
`mothership/go.mod` (promoted from `// indirect` at `e40e3bca`).

## The endpoint pattern

`mothership/cmd/mothership/main.go:808`:

```go
r.Handle("/metrics", promhttp.Handler())
```

- Mounted on the same chi router as the REST API, immediately after `r.Get("/healthz", …)`
  (`main.go:801`), before the auth routes.
- **Unauthenticated by design**, like `/healthz` — scrape traffic is not dashboard
  traffic; auth in production happens at the Traefik layer, not in the app.
- Serves the default registry, so every `promauto` metric in every package is exported
  with no per-metric wiring. A new metric added to any `internal/` package is
  automatically scrapeable after a rebuild.

## Existing metric examples to follow

### Counter: `auto_update_triggers_total` (autoupdate.go)

Semantics fixed by `e18e3c57` ("count each auto-update trigger exactly once at
outcome"): **exactly one increment per trigger attempt, written only once the cycle's
outcome is known.**

- Success increment — `fleetRollout` completion, `autoupdate.go:794`:
  `autoUpdateTriggerCounter.WithLabelValues("auto", "success").Inc()`
- Failure increment — `failUpdateCycle`, `autoupdate.go:923`, reached from every
  failure/cancel path (no canary, OTA send failure, canary rollback, quality
  degradation, cancelled mid-gap, …).
- A cycle *starting* (`startUpdateCycle`) or the canary *deploying* does **not** count.

### Gauges: drift family (drift.go)

Written by `DriftMonitor.Evaluate()` (`drift.go:212`) on its periodic pass:
`drift.go:399` sets `node_firmware_drift_seconds`, `drift.go:402` sets the fault
gauge to 1 for a drifted node, and `drift.go:411` uses
`DeletePartialMatch(prometheus.Labels{"mac": …})` to clear that node's series when
drift resolves — the established pattern for deleting per-node series that become
stale.

## Pattern to follow for a new metric

1. **Declare in the package that owns the behaviour**, as a package-level `promauto`
   var (see the `var (...)` blocks above). Never create a registry, never register
   manually.
2. **Labels encode outcome/dimension only** — no timestamps, no free-form reasons.
   High-cardinality label values are bounded (`mac`, versions), matching
   `node_firmware_drift_fault`.
3. **Counters record outcomes, not attempts.** `auto_update_triggers_total` is the
   precedent: pick the single point where the outcome is known and increment there.
   Do not increment at cycle entry or intermediate stages.
4. **Test by scraping, not by reading the counter.** `internal/ota/metrics_test.go`
   is the template: `httptest.NewServer(promhttp.Handler())` — the *same* handler
   `main.go` mounts — then regex-extract `auto_update_triggers_total{…}` samples from
   the exposition body, assert `Content-Type` is `text/plain; version=0.0.4`, and
   assert per-label **deltas** around a real cycle. `autoupdate_test.go:1461` shows
   the in-process variant, `testutil.ToFloat64(...WithLabelValues(...))`.
5. **The counter is package-global** — assertions must be deltas between two scrapes,
   never absolute values, because other tests in the package have already driven it.

## Where auto-update trigger events occur

Three entry points converge on `startUpdateCycle` (`autoupdate.go:459`), which is the
start of every cycle:

| Entry point | Location | Fires when |
|---|---|---|
| Background poll loop | `run`, `autoupdate.go:294` → `checkForNewFirmware` (`:315`) | immediately on `Start(ctx)` (`main.go:4846`), then every 1 min |
| Firmware upload callback | `OnFirmwareUploaded`, `autoupdate.go:1007` → `checkForNewFirmware` | `otaSrv.SetUploadCallback(…)` wiring at `main.go:4871` |
| Manual REST trigger | `TriggerUpdate`, `autoupdate.go:973` | `POST /api/ota/auto/trigger` → `AutoAPIHandler.handleTrigger` (`internal/ota/autoapi.go:135`, route registered `:93`) |

`checkForNewFirmware` gates on `config.Enabled` and the quiet window, then calls
`startUpdateCycle` (`autoupdate.go:411` is the loop/callback path; the manual path
calls it directly at `:989`). A cycle then proceeds: `startUpdateCycle` → canary
select/deploy/monitor → `fleetRollout` (`:736`) or `failUpdateCycle` (`:917`).

**To hook a new piece of instrumentation onto trigger events:** the outcome points
above (`:794` success, `:923` failure) are the only sanctioned observation points —
they are where the existing counter increments, and where the state machine has
settled. Anything added at cycle entry will double-count relative to this metric and
contradicts the contract established by `e18e3c57`.

## Gotchas for the next step

- **A childless `*Vec` emits no series.** `auto_update_triggers_total` currently has
  no pre-created children at HEAD, so the series is **absent** from `/metrics` on a
  fresh boot until the first cycle resolves — scrape dashboards and `absent()` alerts
  see the metric vanish on every restart. The fix is a package-level
  `_ = counter.WithLabelValues(...)` per label combination, as the working tree
  currently carries (uncommitted — see below). The drift gauges have the same shape
  and are still untouched.
- **`promauto` panics on duplicate registration.** A second `promauto.New…` with the
  same name, or re-registering in a test that imports the package twice, is a
  process-start panic. This is also why tests assert deltas on the shared counter.
- **Deleting per-node/per-label series** uses `DeletePartialMatch`, not manual
  bookkeeping (`drift.go:411`).
- **Auth posture:** keep `/metrics` unauthenticated in-app; scraping is expected to be
  restricted at the Traefik layer like `/healthz`.
- **Working tree vs HEAD:** at the time of writing the shared checkout carries
  *uncommitted* edits to `internal/ota/autoupdate.go` (the pre-created-children block)
  and `cmd/mothership/main.go` (+7 lines). Line numbers in this document are HEAD
  (`d73a938c`) numbers; the working-tree files are offset by +8/+7 lines respectively
  past the touched regions. Those edits belong to another worker and are not part of
  this survey.
