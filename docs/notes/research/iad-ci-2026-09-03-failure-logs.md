# iad-ci failure logs — 2026-09-03

Raw evidence for the 2026-09-03 iad-ci failures. The controller runs
`podGC: OnPodCompletion`, so every failing pod was deleted the moment it
finished and no logs survived. This document is the log capture that evidence
chain was missing: each failing step was re-run with `podGC: OnWorkflowCompletion`
so the pod lived until workflow completion, and its `main` container log was
streamed to disk while it ran.

**Scope:** capture only. Root-cause conclusions and fixes are explicitly out of
scope for the bead this was produced under (spaxel-c466198b).

## Method

| Capture | Mechanism |
|---|---|
| `mta-my-way-build-logcap-gdh6h` | `workflowTemplateRef: mta-my-way-build` resubmit + `podGC.strategy: OnWorkflowCompletion` override |
| `acb-build-logcap-cfpwz` | `workflowTemplateRef: acb-build` resubmit + same override |
| `spaxel-build-step-mirror-nhksn` | Faithful one-off mirror of the two failing `spaxel-build` steps (see §3 for why a template resubmit could not work) |

A polling watcher (`kubectl logs -f` per pod, started the moment each pod
appeared) captured the full `main` logs below before podGC deleted them.
Failing-node names, phases and exit codes were additionally confirmed from
workflow-level status, which survives pod deletion.

## 2026-09-03 failing-workflow inventory (all templates)

Context for the captures — every Failed/Error workflow on iad-ci that day:

| Time (UTC) | Workflow | Failing node[step] | Exit |
|---|---|---|---|
| 00:50:40 | `needle-ci-7tgdg` | verify[verify] | 1 |
| 07:47:29 | `acb-site-pages-build-k9zhk` | build-and-deploy ×4 attempts | 128 |
| 07:47:30 | `acb-build-fgnv7` | test[run-tests] | 1 |
| 08:06:03 | `spaxel-e2e-2tmtz` | go-test, acceptance-tests (1); docker-e2e (22) | 1/22 |
| 08:06:04 | `spaxel-build-qssc4` | lint(0)[golangci-lint], a11y-test(0)[a11y-test] | 1 |
| 08:06:42 | `spaxel-e2e-gqv8n` | go-test, acceptance-tests (1); docker-e2e (22) | 1/22 |
| 08:06:45 | `spaxel-build-wtc5b` | lint(0), a11y-test(0) | 1 |
| 08:29:46 | `mta-my-way-build-ppdzr` | lint[lint] | 2 |
| 08:30:08 | `mta-my-way-build-9d9zm` | lint[lint] | 2 |
| 08:35:07 | `spaxel-build-9nvs6` | lint(0), a11y-test(0) | 1 |
| 08:35:10 | `spaxel-e2e-m75bv` | go-test, acceptance-tests (1); docker-e2e (22) | 1/22 |
| 08:35:42 | `spaxel-e2e-htdcj` | go-test, acceptance-tests (1); docker-e2e (22) | 1/22 |
| 08:35:43 | `spaxel-build-6cgzf` | lint(0), a11y-test(0) | 1 |
| 09:00:00 | `armor-drift-check-daily-1788426000` | run-check(0)[version-drift-check] | 2 |
| 09:13:33 | `mta-my-way-build-zk4gw` | lint[lint] | 2 |

Captured below: **spaxel-build** (§3, two steps), **mta-my-way-build** (§1) and
**acb-build** (§2) — three different templates, two of them non-spaxel.
Independent corroboration: a separate debug capture by another actor
(`mta-my-way-build-dbglog-ctl82`, `acb-build-dbglog-k5wbm`, 09:30) failed with
identical node/exit-code signatures.

---

## 1. mta-my-way-build — step `lint`

| Field | Value |
|---|---|
| Template | `mta-my-way-build` |
| Capture workflow | `mta-my-way-build-logcap-gdh6h` (Failed) |
| Failing node / step | `lint[lint]` (attempt 0; no retry fired) |
| Image | `node:22-slim` |
| Exit code | 2 |
| Commit captured | `076668d` (origin/main at capture time; original 08:29/08:30 runs captured `0f640294`) |
| Step commands | `apt-get install git` → clone → `npm ci --prefer-offline` → `npm run lint` → `npm run typecheck` |

**Where it fails:** `npm run lint` (`biome check . && eslint .`) **passes**
("Checked 628 files … No fixes applied"). The exit 2 comes from
`npm run typecheck` (`tsc --build`), whose first errors are a duplicate
identifier, followed by hundreds of TS errors across `packages/web`:

```
+ npm run typecheck

> mta-my-way@0.0.1 typecheck
> tsc --build

packages/shared/src/index.ts(237,3): error TS2300: Duplicate identifier 'formatDuration'.
packages/shared/src/index.ts(351,3): error TS2300: Duplicate identifier 'formatDuration'.
```

**Last 100 lines of the step log** (of 716 total; captured in full during the run):

<details>
<summary>click to expand</summary>

```
packages/web/src/hooks/useTripTracker.test.ts(261,8): error TS2352: Conversion of type 'Error' to type '{ status: number; }' may be a mistake because neither type sufficiently overlaps with the other. If this was intentional, convert the expression to 'unknown' first.
  Property 'status' is missing in type 'Error' but required in type '{ status: number; }'.
packages/web/src/hooks/useTripTracker.test.ts(276,53): error TS2554: Expected 1 arguments, but got 3.
packages/web/src/hooks/useTripTracker.test.ts(314,52): error TS2345: Argument of type '{ tripId: string; routeId: string; direction: string; destination: string; progressPercent: number; currentStopIndex: number; stops: ({ stopId: string; stationId: string; stationName: string; arrivalTime: number; departureTime: number; } | { ...; })[]; }' is not assignable to parameter of type 'TripData'.
  Type '{ tripId: string; routeId: string; direction: string; destination: string; progressPercent: number; currentStopIndex: number; stops: ({ stopId: string; stationId: string; stationName: string; arrivalTime: number; departureTime: number; } | { ...; })[]; }' is missing the following properties from type 'TripData': isAssigned, trainId, updatedAt, feedAge, and 2 more.
packages/web/src/hooks/useTripTracker.test.ts(337,52): error TS2345: Argument of type '{ tripId: string; routeId: string; direction: string; destination: string; progressPercent: number; currentStopIndex: number; stops: ({ stopId: string; stationId: string; stationName: string; arrivalTime: number; departureTime: number; } | { ...; })[]; }' is not assignable to parameter of type 'TripData'.
  Type '{ tripId: string; routeId: string; direction: string; destination: string; progressPercent: number; currentStopIndex: number; stops: ({ stopId: string; stationId: string; stationName: string; arrivalTime: number; departureTime: number; } | { ...; })[]; }' is missing the following properties from type 'TripData': isAssigned, trainId, updatedAt, feedAge, and 2 more.
packages/web/src/hooks/useTripTracker.test.ts(387,52): error TS2345: Argument of type '{ tripId: string; routeId: string; direction: string; destination: string; progressPercent: number; currentStopIndex: number; stops: ({ stopId: string; stationId: string; stationName: string; arrivalTime: number; departureTime: number; } | { ...; })[]; }' is not assignable to parameter of type 'TripData'.
  Type '{ tripId: string; routeId: string; direction: string; destination: string; progressPercent: number; currentStopIndex: number; stops: ({ stopId: string; stationId: string; stationName: string; arrivalTime: number; departureTime: number; } | { ...; })[]; }' is missing the following properties from type 'TripData': isAssigned, trainId, updatedAt, feedAge, and 2 more.
packages/web/src/hooks/useTripTracker.test.ts(408,52): error TS2345: Argument of type '{ stops: ({ arrivalTime: null; stopId: string; stationId: string; stationName: string; departureTime: number; } | { arrivalTime: null; stopId: string; stationId: string; stationName: string; departureTime: null; })[]; ... 5 more ...; currentStopIndex: number; }' is not assignable to parameter of type 'TripData'.
  Type '{ stops: ({ arrivalTime: null; stopId: string; stationId: string; stationName: string; departureTime: number; } | { arrivalTime: null; stopId: string; stationId: string; stationName: string; departureTime: null; })[]; ... 5 more ...; currentStopIndex: number; }' is missing the following properties from type 'TripData': isAssigned, trainId, updatedAt, feedAge, and 2 more.
packages/web/src/hooks/useTripTracker.test.ts(426,52): error TS2345: Argument of type '{ progressPercent: number; tripId: string; routeId: string; direction: string; destination: string; currentStopIndex: number; stops: ({ stopId: string; stationId: string; stationName: string; arrivalTime: number; departureTime: number; } | { ...; })[]; }' is not assignable to parameter of type 'TripData'.
  Type '{ stops: ({ arrivalTime: null; stopId: string; stationId: string; stationName: string; departureTime: number; } | { arrivalTime: null; stopId: string; stationId: string; stationName: string; departureTime: null; })[]; ... 5 more ...; currentStopIndex: number; }' is missing the following properties from type 'TripData': isAssigned, trainId, updatedAt, feedAge, and 2 more.
packages/web/src/hooks/useTripTracker.test.ts(443,52): error TS2345: Argument of type '{ currentStopIndex: number; tripId: string; routeId: string; direction: string; destination: string; progressPercent: number; stops: ({ stopId: string; stationId: string; stationName: string; arrivalTime: number; departureTime: number; } | { ...; })[]; }' is not assignable to parameter of type 'TripData'.
  Type '{ currentStopIndex: number; tripId: string; routeId: string; direction: string; destination: string; progressPercent: number; stops: ({ stopId: string; stationId: string; stationName: string; arrivalTime: number; departureTime: number; } | { ...; })[]; }' is missing the following properties from type 'TripData': isAssigned, trainId, updatedAt, feedAge, and 2 more.
packages/web/src/hooks/useTripTracker.ts(141,16): error TS2345: Argument of type '{ isActive: true; trip: TripData; stops: TripStopProgress[]; eta: number | null; minutesToDestination: number | null; progressPercent: number; isLoading: false; error: null; isExpired: false; updatedAt: number; }' is not assignable to parameter of type 'SetStateAction<TripTrackerState>'.
  Property 'prediction' is missing in type '{ isActive: true; trip: TripData; stops: TripStopProgress[]; eta: number | null; minutesToDestination: number | null; progressPercent: number; isLoading: false; error: null; isExpired: false; updatedAt: number; }' but required in type 'TripTrackerState'.
packages/web/src/hooks/useTripTracker.ts(185,16): error TS2345: Argument of type '{ isActive: false; trip: null; stops: never[]; eta: null; minutesToDestination: null; progressPercent: number; isLoading: false; error: null; isExpired: false; updatedAt: null; }' is not assignable to parameter of type 'SetStateAction<TripTrackerState>'.
  Property 'prediction' is missing in type '{ isActive: false; trip: null; stops: never[]; eta: null; minutesToDestination: null; progressPercent: number; isLoading: false; error: null; isExpired: false; updatedAt: null; }' but required in type 'TripTrackerState'.
packages/web/src/hooks/useTripTracker.ts(208,14): error TS2345: Argument of type '{ isActive: false; trip: null; stops: never[]; eta: null; minutesToDestination: null; progressPercent: number; isLoading: false; error: null; isExpired: false; updatedAt: null; }' is not assignable to parameter of type 'SetStateAction<TripTrackerState>'.
  Property 'prediction' is missing in type '{ isActive: false; trip: null; stops: never[]; eta: null; minutesToDestination: null; progressPercent: number; isLoading: false; error: null; isExpired: false; updatedAt: null; }' but required in type 'TripTrackerState'.
packages/web/src/lib/api.test.ts(63,12): error TS18048: 'callArgs' is possibly 'undefined'.
packages/web/src/lib/api.test.ts(93,12): error TS2532: Object is possibly 'undefined'.
packages/web/src/lib/api.test.ts(95,12): error TS2532: Object is possibly 'undefined'.
packages/web/src/lib/api.test.ts(107,12): error TS18048: 'callArgs' is possibly 'undefined'.
packages/web/src/lib/api.test.ts(194,12): error TS18048: 'callArgs' is possibly 'undefined'.
packages/web/src/lib/api.test.ts(380,14): error TS18048: 'callArgs' is possibly 'undefined'.
packages/web/src/lib/api.test.ts(381,14): error TS18048: 'callArgs' is possibly 'undefined'.
packages/web/src/lib/api.test.ts(383,31): error TS18048: 'callArgs' is possibly 'undefined'.
packages/web/src/lib/api.test.ts(438,31): error TS2345: Argument of type '{ subscription: { endpoint: string; keys: {}; }; favorites: never[]; }' is not assignable to parameter of type 'PushSubscribeRequest'.
  The types of 'subscription.keys' are incompatible between these types.
    Type '{}' is missing the following properties from type '{ p256dh: string; auth: string; }': p256dh, auth
packages/web/src/lib/api.test.ts(441,14): error TS18048: 'callArgs' is possibly 'undefined'.
packages/web/src/lib/api.test.ts(442,14): error TS18048: 'callArgs' is possibly 'undefined'.
packages/web/src/lib/api.test.ts(454,14): error TS18048: 'callArgs' is possibly 'undefined'.
packages/web/src/lib/api.test.ts(455,14): error TS18048: 'callArgs' is possibly 'undefined'.
packages/web/src/lib/api.test.ts(470,14): error TS18048: 'callArgs' is possibly 'undefined'.
packages/web/src/lib/api.test.ts(471,14): error TS18048: 'callArgs' is possibly 'undefined'.
packages/web/src/lib/api.test.ts(540,12): error TS18048: 'callArgs' is possibly 'undefined'.
packages/web/src/lib/api.test.ts(553,12): error TS18048: 'callArgs' is possibly 'undefined'.
packages/web/src/lib/apiCached.test.ts(371,52): error TS2345: Argument of type '{ subscription: { endpoint: string; keys: {}; }; favorites: never[]; }' is not assignable to parameter of type 'PushSubscribeRequest'.
  The types of 'subscription.keys' are incompatible between these types.
    Type '{}' is missing the following properties from type '{ p256dh: string; auth: string; }': p256dh, auth
packages/web/src/lib/apiEnhanced.test.ts(11,10): error TS2305: Module '"vitest"' has no exported member 'act'.
packages/web/src/lib/apiEnhanced.test.ts(11,10): error TS6133: 'act' is declared but its value is never read.
packages/web/src/lib/apiEnhanced.test.ts(559,12): error TS18048: 'callArgs' is possibly 'undefined'.
packages/web/src/lib/apiEnhanced.test.ts(560,12): error TS18048: 'callArgs' is possibly 'undefined'.
packages/web/src/lib/apiEnhanced.test.ts(562,29): error TS18048: 'callArgs' is possibly 'undefined'.
packages/web/src/lib/apiEnhanced.test.ts(612,37): error TS2345: Argument of type '{ subscription: { endpoint: string; keys: {}; }; favorites: never[]; }' is not assignable to parameter of type 'PushSubscribeRequest'.
  The types of 'subscription.keys' are incompatible between these types.
    Type '{}' is missing the following properties from type '{ p256dh: string; auth: string; }': p256dh, auth
packages/web/src/lib/apiEnhanced.test.ts(615,12): error TS18048: 'callArgs' is possibly 'undefined'.
packages/web/src/lib/apiEnhanced.test.ts(616,12): error TS18048: 'callArgs' is possibly 'undefined'.
packages/web/src/lib/apiEnhanced.test.ts(628,12): error TS18048: 'callArgs' is possibly 'undefined'.
packages/web/src/lib/apiEnhanced.test.ts(629,12): error TS18048: 'callArgs' is possibly 'undefined'.
packages/web/src/lib/apiEnhanced.test.ts(669,12): error TS18048: 'callArgs' is possibly 'undefined'.
packages/web/src/lib/apiEnhanced.test.ts(708,11): error TS6133: 'elapsed' is declared but its value is never read.
packages/web/src/lib/backgroundSync.test.ts(11,7): error TS2559: Type 'MockServiceWorkerRegistration' has no properties in common with type 'Partial<ServiceWorkerRegistration>'.
packages/web/src/lib/backgroundSync.test.ts(13,40): error TS2304: Cannot find name 'SyncRegistration'.
packages/web/src/lib/backgroundSync.test.ts(144,64): error TS2339: Property 'sync' does not exist on type 'ServiceWorkerRegistration'.
packages/web/src/lib/backgroundSync.test.ts(155,43): error TS2339: Property 'sync' does not exist on type 'ServiceWorkerRegistration'.
packages/web/src/lib/backgroundSync.test.ts(170,27): error TS2339: Property 'sync' does not exist on type 'ServiceWorkerRegistration'.
packages/web/src/lib/backgroundSync.test.ts(202,27): error TS18048: 'addCall' is possibly 'undefined'.
packages/web/src/lib/backgroundSync.test.ts(220,27): error TS18048: 'addCall' is possibly 'undefined'.
packages/web/src/lib/backgroundSync.test.ts(465,30): error TS2532: Object is possibly 'undefined'.
packages/web/src/lib/backgroundSync.ts(36,21): error TS2304: Cannot find name 'SyncRegistration'.
packages/web/src/lib/backgroundSync.ts(87,50): error TS2339: Property 'sync' does not exist on type 'ServiceWorkerRegistration'.
packages/web/src/lib/backgroundSync.ts(122,28): error TS2339: Property 'sync' does not exist on type 'ServiceWorkerRegistration'.
packages/web/src/lib/prefetch.ts(140,70): error TS2339: Property 'href' does not exist on type 'never'.
packages/web/src/lib/prefetch.ts(140,86): error TS2339: Property 'pathname' does not exist on type 'never'.
packages/web/src/lib/serviceWorkerRegistration.test.ts(20,31): error TS2339: Property 'mockReset' does not exist on type '(options?: { immediate?: boolean | undefined; onRegistered?: ((registration: ServiceWorkerRegistration | undefined) => void) | undefined; onRegisterError?: ((error: unknown) => void) | undefined; onNeedRefresh?: (() => void) | undefined; onOfflineReady?: (() => void) | undefined; } | undefined) => () => void'.
packages/web/src/lib/serviceWorkerRegistration.test.ts(42,40): error TS2339: Property '_mockOptions' does not exist on type '(options?: { immediate?: boolean | undefined; onRegistered?: ((registration: ServiceWorkerRegistration | undefined) => void) | undefined; onRegisterError?: ((error: unknown) => void) | undefined; onNeedRefresh?: (() => void) | undefined; onOfflineReady?: (() => void) | undefined; } | undefined) => () => void'.
packages/web/src/lib/serviceWorkerRegistration.test.ts(43,40): error TS2339: Property '_mockOptions' does not exist on type '(options?: { immediate?: boolean | undefined; onRegistered?: ((registration: ServiceWorkerRegistration | undefined) => void) | undefined; onRegisterError?: ((error: unknown) => void) | undefined; onNeedRefresh?: (() => void) | undefined; onOfflineReady?: (() => void) | undefined; } | undefined) => () => void'.
packages/web/src/lib/serviceWorkerRegistration.test.ts(65,33): error TS2339: Property 'triggerOnRegistered' does not exist on type '(options?: { immediate?: boolean | undefined; onRegistered?: ((registration: ServiceWorkerRegistration | undefined) => void) | undefined; onRegisterError?: ((error: unknown) => void) | undefined; onNeedRefresh?: (() => void) | undefined; onOfflineReady?: (() => void) | undefined; } | undefined) => () => void'.
packages/web/src/lib/serviceWorkerRegistration.test.ts(79,33): error TS2339: Property 'triggerOnRegisterError' does not exist on type '(options?: { immediate?: boolean | undefined; onRegistered?: ((registration: ServiceWorkerRegistration | undefined) => void) | undefined; onRegisterError?: ((error: unknown) => void) | undefined; onNeedRefresh?: (() => void) | undefined; onOfflineReady?: (() => void) | undefined; } | undefined) => () => void'.
packages/web/src/lib/serviceWorkerRegistration.test.ts(95,33): error TS2339: Property 'triggerOnOfflineReady' does not exist on type '(options?: { immediate?: boolean | undefined; onRegistered?: ((registration: ServiceWorkerRegistration | undefined) => void) | undefined; onRegisterError?: ((error: unknown) => void) | undefined; onNeedRefresh?: (() => void) | undefined; onOfflineReady?: (() => void) | undefined; } | undefined) => () => void'.
packages/web/src/lib/serviceWorkerRegistration.test.ts(114,35): error TS2339: Property 'triggerOnNeedRefresh' does not exist on type '(options?: { immediate?: boolean | undefined; onRegistered?: ((registration: ServiceWorkerRegistration | undefined) => void) | undefined; onRegisterError?: ((error: unknown) => void) | undefined; onNeedRefresh?: (() => void) | undefined; onOfflineReady?: (() => void) | undefined; } | undefined) => () => void'.
packages/web/src/screens/HomeScreen.test.tsx(17,42): error TS6133: 'waitFor' is declared but its value is never read.
packages/web/src/screens/HomeScreen.test.tsx(36,7): error TS6133: 'favorite' is declared but its value is never read.
packages/web/src/screens/HomeScreen.test.tsx(110,34): error TS2345: Argument of type '{ onboardingComplete: boolean; commutes: { id: string; name: string; origin: { stationId: string; stationName: string; }; destination: { stationId: string; stationName: string; }; preferredLines: string[]; }[]; tapHistory: never[]; }' is not assignable to parameter of type 'FavoritesState'.
  Type '{ onboardingComplete: boolean; commutes: { id: string; name: string; origin: { stationId: string; stationName: string; }; destination: { stationId: string; stationName: string; }; preferredLines: string[]; }[]; tapHistory: never[]; }' is missing the following properties from type 'FavoritesState': favorites, addFavorite, updateFavorite, removeFavorite, and 10 more.
packages/web/src/screens/HomeScreen.test.tsx(117,34): error TS2345: Argument of type '{ hapticFeedback: boolean; }' is not assignable to parameter of type 'SettingsState'.
  Type '{ hapticFeedback: boolean; }' is missing the following properties from type 'SettingsState': theme, showUnassignedTrips, refreshInterval, alertSeverityFilter, and 11 more.
packages/web/src/screens/HomeScreen.test.tsx(155,7): error TS2322: Type '{ id: string; stationId: string; stationName: string; lines: string[]; direction: string; }[]' is not assignable to type 'Favorite[]'.
  Property 'sortOrder' is missing in type '{ id: string; stationId: string; stationName: string; lines: string[]; direction: string; }' but required in type 'Favorite'.
packages/web/src/screens/HomeScreen.test.tsx(162,57): error TS2345: Argument of type 'undefined' is not assignable to parameter of type 'UsePrefetchReturn'.
packages/web/src/screens/HomeScreen.test.tsx(187,36): error TS2345: Argument of type '{ onboardingComplete: boolean; commutes: never[]; tapHistory: never[]; }' is not assignable to parameter of type 'FavoritesState'.
  Type '{ onboardingComplete: boolean; commutes: never[]; tapHistory: never[]; }' is missing the following properties from type 'FavoritesState': favorites, addFavorite, updateFavorite, removeFavorite, and 10 more.
packages/web/src/screens/HomeScreen.test.tsx(217,61): error TS2345: Argument of type '{ favorites: never[]; hasFavorites: false; updateFavorite: Mock<Procedure>; removeFavorite: Mock<Procedure>; reorderFavorites: Mock<Procedure>; }' is not assignable to parameter of type '{ favorites: Favorite[]; hasFavorites: boolean; onboardingComplete: boolean; addFavorite: (favorite: Omit<Favorite, "id" | "sortOrder">) => string; updateFavorite: (id: string, updates: Partial<Favorite>) => void; ... 4 more ...; completeOnboarding: () => void; }'.
  Type '{ favorites: never[]; hasFavorites: false; updateFavorite: Mock<Procedure>; removeFavorite: Mock<Procedure>; reorderFavorites: Mock<Procedure>; }' is missing the following properties from type '{ favorites: Favorite[]; hasFavorites: boolean; onboardingComplete: boolean; addFavorite: (favorite: Omit<Favorite, "id" | "sortOrder">) => string; updateFavorite: (id: string, updates: Partial<Favorite>) => void; ... 4 more ...; completeOnboarding: () => void; }': onboardingComplete, addFavorite, togglePin, recordTap, completeOnboarding
packages/web/src/screens/HomeScreen.test.tsx(252,36): error TS2345: Argument of type '{ onboardingComplete: boolean; commutes: never[]; tapHistory: never[]; }' is not assignable to parameter of type 'FavoritesState'.
  Type '{ onboardingComplete: boolean; commutes: never[]; tapHistory: never[]; }' is missing the following properties from type 'FavoritesState': favorites, addFavorite, updateFavorite, removeFavorite, and 10 more.
packages/web/src/screens/HomeScreen.test.tsx(300,9): error TS2322: Type '{ id: string; stationId: string; stationName: string; lines: string[]; direction: string; }[]' is not assignable to type 'Favorite[]'.
  Property 'sortOrder' is missing in type '{ id: string; stationId: string; stationName: string; lines: string[]; direction: string; }' but required in type 'Favorite'.
packages/web/src/screens/HomeScreen.test.tsx(465,61): error TS2345: Argument of type '{ favorites: never[]; hasFavorites: false; updateFavorite: Mock<Procedure>; removeFavorite: Mock<Procedure>; reorderFavorites: Mock<Procedure>; }' is not assignable to parameter of type '{ favorites: Favorite[]; hasFavorites: boolean; onboardingComplete: boolean; addFavorite: (favorite: Omit<Favorite, "id" | "sortOrder">) => string; updateFavorite: (id: string, updates: Partial<Favorite>) => void; ... 4 more ...; completeOnboarding: () => void; }'.
  Type '{ favorites: never[]; hasFavorites: false; updateFavorite: Mock<Procedure>; removeFavorite: Mock<Procedure>; reorderFavorites: Mock<Procedure>; }' is missing the following properties from type '{ favorites: Favorite[]; hasFavorites: boolean; onboardingComplete: boolean; addFavorite: (favorite: Omit<Favorite, "id" | "sortOrder">) => string; updateFavorite: (id: string, updates: Partial<Favorite>) => void; ... 4 more ...; completeOnboarding: () => void; }': onboardingComplete, addFavorite, togglePin, recordTap, completeOnboarding
packages/web/src/stores/fareStore.ts(74,9): error TS6133: 'breakEvenRides' is declared but its value is never read.
time=2026-09-03T09:24:52.194Z level=INFO msg="sub-process exited" argo=true error="exit status 2"
time=2026-09-03T09:24:52.194Z level=INFO msg="file signal handler exiting due to context cancellation" argo=true
Error: exit status 2
```

</details>

---

## 2. acb-build — step `run-tests`

| Field | Value |
|---|---|
| Template | `acb-build` |
| Capture workflow | `acb-build-logcap-cfpwz` (Failed) |
| Failing node / step | `test[run-tests]` (attempt 0; no retry fired) |
| Image | `golang:1.25-alpine` |
| Exit code | 1 |
| Commit captured | resolve-sha `248273d` — **identical to the original** `acb-build-fgnv7` (07:47:30), so this is an exact reproduction |
| Step commands | `apk add git nodejs npm curl ca-certificates gcc musl-dev` (`CGO_ENABLED=1`) → clone → `go test ./engine/... -count=1 -timeout 120s` → `go test ./cmd/...` → web checks |

**Where it fails:** `go test ./engine/...` panics on its own 2-minute timeout —
`TestCombatDensityMetrics` (subtest `6-player`) is still running `ComputeWinProbability`
random rollouts when the alarm fires. The step never reaches `go test ./cmd/...`.

**Last 100 lines of the step log** (104 lines total, captured in full):

<details>
<summary>click to expand</summary>

```
( 5/34) Installing nghttp2-libs (1.69.0-r0)
( 6/34) Installing libpsl (0.21.5-r3)
( 7/34) Installing zstd-libs (1.5.7-r2)
( 8/34) Installing libcurl (8.22.0-r0)
( 9/34) Installing curl (8.22.0-r0)
(10/34) Installing libgcc (15.2.0-r5)
(11/34) Installing jansson (2.15.0-r0)
(12/34) Installing libstdc++ (15.2.0-r5)
(13/34) Installing binutils (2.45.1-r1)
(14/34) Installing libgcc-static (15.2.0-r5)
(15/34) Installing libgomp (15.2.0-r5)
(16/34) Installing libatomic (15.2.0-r5)
(17/34) Installing gmp (6.3.0-r4)
(18/34) Installing isl26 (0.26-r2)
(19/34) Installing mpfr4 (4.2.2-r0)
(20/34) Installing mpc1 (1.3.1-r1)
(21/34) Installing gcc (15.2.0-r5)
(22/34) Installing libexpat (2.8.4-r0)
(23/34) Installing pcre2 (10.48-r0)
(24/34) Installing git (2.54.0-r0)
(25/34) Installing git-init-template (2.54.0-r0)
(26/34) Installing musl-dev (1.2.6-r2)
(27/34) Installing ada-libs (3.3.0-r0)
(28/34) Installing icu-data-en (78.1-r0)
  Executing icu-data-en-78.1-r0.post-install
  * If you need ICU with non-English locales and legacy charset support, install
  * package icu-data-full.
(29/34) Installing icu-libs (78.1-r0)
(30/34) Installing simdjson (4.2.4-r0)
(31/34) Installing simdutf (9.0.0-r0)
(32/34) Installing sqlite-libs (3.53.4-r0)
(33/34) Installing nodejs (24.18.1-r0)
(34/34) Installing npm (11.12.1-r0)
Executing busybox-1.37.0-r31.trigger
OK: 280.7 MiB in 51 packages
Cloning into '/src'...
panic: test timed out after 2m0s
	running tests:
		TestCombatDensityMetrics (1m56s)
		TestCombatDensityMetrics/6-player (1m3s)

goroutine 17214 [running]:
testing.(*M).startAlarm.func1()
	/usr/local/go/src/testing/testing.go:2682 +0x359
created by time.goFunc
	/usr/local/go/src/time/sleep.go:215 +0x2d

goroutine 1 [chan receive, 1 minutes]:
testing.(*T).Run(0xc00009a380, {0x83ccbc?, 0x0?}, 0x858f20)
	/usr/local/go/src/testing/testing.go:2005 +0x485
testing.runTests.func1(0xc00009a380)
	/usr/local/go/src/testing/testing.go:2477 +0x3e
testing.tRunner(0xc00009a380, 0xc000063c70)
	/usr/local/go/src/testing/testing.go:1934 +0xea
testing.runTests(0xc000012120, {0xb5a4c0, 0x49, 0x49}, {0xb615a0?, 0xcc2aee457407f4b8?, 0xb5fe60?})
	/usr/local/go/src/testing/testing.go:2475 +0x4b4
testing.(*M).Run(0xc0000903c0)
	/usr/local/go/src/testing/testing.go:2337 +0x63a
main.main()
	_testmain.go:189 +0x9b

goroutine 1032 [chan receive, 1 minutes]:
testing.(*T).Run(0xc0002b8380, {0x834c1a?, 0xc0001c9f50?}, 0x859250)
	/usr/local/go/src/testing/testing.go:2005 +0x485
github.com/aicodebattle/acb/engine.TestCombatDensityMetrics(0xc0002b8380)
	/src/engine/integration_test.go:339 +0xa8
testing.tRunner(0xc0002b8380, 0x858f20)
	/usr/local/go/src/testing/testing.go:1934 +0xea
created by testing.(*T).Run in goroutine 1
	/usr/local/go/src/testing/testing.go:1997 +0x465

goroutine 12368 [runnable]:
internal/runtime/maps.rand()
	/usr/local/go/src/runtime/rand.go:183 +0x16
internal/runtime/maps.(*Iter).Init(0xc0002b3608, 0xc000055808?, 0xc0002b35d8)
	/usr/local/go/src/internal/runtime/maps/table.go:668 +0xa5
github.com/aicodebattle/acb/engine.(*GameState).GetLivingPlayers(...)
	/src/engine/game.go:210
github.com/aicodebattle/acb/engine.(*GameState).checkWinConditions(0xc00010cd20)
	/src/engine/turn.go:485 +0x152
github.com/aicodebattle/acb/engine.(*GameState).ExecuteTurn(0xc00010cd20)
	/src/engine/turn.go:46 +0xa7
github.com/aicodebattle/acb/engine.runRandomRollout(0xc00010cd20)
	/src/engine/winprob.go:66 +0xff
github.com/aicodebattle/acb/engine.ComputeWinProbability({0xc0000cea08, 0x29, 0x269}, 0x64, 0xc0004292f0)
	/src/engine/winprob.go:38 +0x35b
github.com/aicodebattle/acb/engine.(*MatchRunner).Run(0xc000280200)
	/src/engine/match.go:197 +0xcf3
github.com/aicodebattle/acb/engine.TestCombatDensityMetrics.func2(0xc00009ac40)
	/src/engine/integration_test.go:364 +0x32f
testing.tRunner(0xc00009ac40, 0x859250)
	/usr/local/go/src/testing/testing.go:1934 +0xea
created by testing.(*T).Run in goroutine 1032
	/usr/local/go/src/testing/testing.go:1997 +0x465
FAIL	github.com/aicodebattle/acb/engine	120.014s
FAIL
time=2026-09-03T09:25:14.045Z level=INFO msg="file signal handler exiting due to context cancellation" argo=true
time=2026-09-03T09:25:14.045Z level=INFO msg="sub-process exited" argo=true error="exit status 1"
Error: exit status 1
```

</details>

---

## 3. spaxel-build — steps `golangci-lint` and `a11y-test`

| Field | Value |
|---|---|
| Template | `spaxel-build` |
| Capture workflow | `spaxel-build-step-mirror-nhksn` (Failed) — faithful one-off mirror of the two failing steps |
| Failing nodes / steps | `golangci-lint(0)` and `a11y-test(0)` — same DAG group, both exit 1, no retry fired on either |
| Images | `golang:1.25-bookworm` (lint), `node:20-bookworm` (a11y) |
| Exit codes | 1 and 1 — **identical to the original runs** (`spaxel-build-qssc4`/`-wtc5b`/`-9nvs6`/`-6cgzf`, 08:06–08:35, all `lint(0)` + `a11y-test(0)` exit 1, no retries fired) |
| Commit captured | `88b4af9c` (origin/main at capture time) |

**Why a mirror instead of a template resubmit:** a `workflowTemplateRef`
resubmit (`spaxel-build-logcap-h978m`, 09:0x) ran `resolve-version` against a
docs-only tip (`88b4af9c`), so its doc-only gate set `should-build=false` and
both failing steps were **Skipped** — no logs to capture. The template's clone
step has no commit-SHA parameter (`git clone --branch <ref>` only), so a rerun
cannot be pinned back to the failing tree. The mirror workflow reproduces both
steps verbatim — same images, same `sh -c` scripts, same env/secretKeyRefs
(`FORGEJO_TOKEN` ← `forgejo-webhook-token`, `GH_TOKEN` ← `github-webhook-secret`),
same resources, same 600s deadlines and `retryStrategy` — against
`--branch main`, which at capture time was `88b4af9c`.

**Why the same failure should have been present in the original runs:** the
code each step trips over predates them —

- `mothership/internal/beads/monitored_pluck.go` and `diagnostic_test.go`
  were last modified by `1294b7f8` (2026-08-30); at `88b4af9c` neither `os` /
  `path/filepath` (former) nor `database/sql` (latter) has any usage in its file.
- `dashboard/package.json` pins `@axe-core/playwright` to `4.10.1` via
  `423f2a61` (2026-08-29) while `package-lock.json` holds `4.11.2`.

Both changes are older than every failing run of that morning.

### 3a. Step `golangci-lint` — exit 1

Fails in the typecheck of the `internal/beads` **test** binary: unused imports.
`golangci-lint` v2.13.2 (installed by the step itself). Full step log (69 lines):

```
time=2026-09-03T09:32:38.409Z level=INFO msg="waiting for signals" argo=true signalPath=/var/run/argo/ctr/main/signal
Cloning into 'repo'...
golangci/golangci-lint info checking GitHub for latest tag
golangci/golangci-lint info found version: 2.13.2 for v2.13.2/linux/amd64
golangci/golangci-lint info installed /usr/local/bin/golangci-lint
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/fleet/fleethandler\\\\.go\", Linters: \"unused\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/provisioning/server\\\\.go\", Linters: \"unused\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"cmd/sim/main\\\\.go\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/simulator/\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/localization/groundtruth_store\\\\.go\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/briefing/\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/diagnostics/\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/api/\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/automation/\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/timeline/\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"cmd/sim/scenario\\\\.go\", Linters: \"errcheck, unused\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/notify/\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/prediction/history\\\\.go\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/zones/\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/dashboard/\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/api/zones\\\\.go\", Linters: \"unused\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/dashboard/\", Linters: \"unused\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/learning/accuracy\\\\.go\", Linters: \"unused\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/learning/feedback_processor\\\\.go\", Linters: \"errcheck\"]"
internal/beads/diagnostic.go:1: : # github.com/spaxel/mothership/internal/beads [github.com/spaxel/mothership/internal/beads.test]
internal/beads/monitored_pluck.go:7:2: "os" imported and not used
internal/beads/monitored_pluck.go:9:2: "path/filepath" imported and not used
internal/beads/diagnostic_test.go:4:2: "database/sql" imported and not used (typecheck)
package beads
1 issues:
* typecheck: 1
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/recorder/\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/sleep/handler\\\\.go\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/volume/\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/localization/fusion\\\\.go\", Linters: \"unused\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/analytics/anomaly\\\\.go\", Linters: \"unused\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/learning/accuracy\\\\.go\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/mqtt/\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/analytics/\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/config/\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/localization/weightlearner\\\\.go\", Linters: \"unused\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/replay/\", Linters: \"unused\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/simulator/virtual_state\\\\.go\", Linters: \"unused\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/ingestion/\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/replay/\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/ble/\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/auth/handler\\\\.go\", Linters: \"unused\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/sleep/\", Linters: \"unused\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"_test\\\\.go\", Linters: \"errcheck, govet, ineffassign, staticcheck, unused\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"test/acceptance/\", Linters: \"errcheck, unused\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/notifications/\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/recording/\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/startup/\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/api/utils\\\\.go\", Linters: \"unused\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/automation/engine\\\\.go\", Linters: \"unused\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/briefing/briefing\\\\.go\", Linters: \"unused\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/help/notifier\\\\.go\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/ota/\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"cmd/mothership/main\\\\.go\", Linters: \"unused\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/falldetect/detector\\\\.go\", Linters: \"unused\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/ingestion/server\\\\.go\", Linters: \"unused\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"cmd/mothership/main\\\\.go\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/auth/handler\\\\.go\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/provisioning/\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/explainability/\", Linters: \"errcheck\"]"
level=warning msg="[runner/exclusion_rules] Skipped 0 issues by rules: [Path: \"internal/notifications/manager\\\\.go\", Linters: \"ineffassign\"]"
time=2026-09-03T09:35:05.568Z level=INFO msg="sub-process exited" argo=true error="exit status 1"
time=2026-09-03T09:35:05.568Z level=INFO msg="file signal handler exiting due to context cancellation" argo=true
Error: exit status 1
```

The substantive failure, extracted:

```
internal/beads/diagnostic.go:1: : # github.com/spaxel/mothership/internal/beads [github.com/spaxel/mothership/internal/beads.test]
internal/beads/monitored_pluck.go:7:2: "os" imported and not used
internal/beads/monitored_pluck.go:9:2: "path/filepath" imported and not used
internal/beads/diagnostic_test.go:4:2: "database/sql" imported and not used (typecheck)
package beads
1 issues:
* typecheck: 1
```

### 3b. Step `a11y-test` — exit 1

Fails before any test runs: `npm ci` refuses the tree because
`dashboard/package.json` and `package-lock.json` disagree on `@axe-core/playwright`
(`4.10.1` pinned in package.json by `423f2a61`, `4.11.2` in the lock file).
Full step log (35 lines):

```
time=2026-09-03T09:32:38.372Z level=INFO msg="waiting for signals" argo=true signalPath=/var/run/argo/ctr/main/signal
Cloning into 'repo'...
npm error code EUSAGE
npm error
npm error `npm ci` can only install packages when your package.json and package-lock.json or npm-shrinkwrap.json are in sync. Please update your lock file with `npm install` before continuing.
npm error
npm error Invalid: lock file's @axe-core/playwright@4.11.2 does not satisfy @axe-core/playwright@4.10.1
npm error Invalid: lock file's axe-core@4.11.3 does not satisfy axe-core@4.10.3
npm error
npm error Clean install a project
npm error
npm error Usage:
npm error npm ci
npm error
npm error Options:
npm error [--install-strategy <hoisted|nested|shallow|linked>] [--legacy-bundling]
npm error [--global-style] [--omit <dev|optional|peer> [--omit <dev|optional|peer> ...]]
npm error [--include <prod|dev|optional|peer> [--include <prod|dev|optional|peer> ...]]
npm error [--strict-peer-deps] [--foreground-scripts] [--ignore-scripts] [--no-audit]
npm error [--no-bin-links] [--no-fund] [--dry-run]
npm error [-w|--workspace <workspace-name> [-w|--workspace <workspace-name> ...]]
npm error [-ws|--workspaces] [--include-workspace-root] [--install-links]
npm error
npm error aliases: clean-install, ic, install-clean, isntall-clean
npm error
npm error Run "npm help ci" for more info
npm notice
npm notice New major version of npm available! 10.8.2 -> 12.0.2
npm notice Changelog: https://github.com/npm/cli/releases/tag/v12.0.2
npm notice To update run: npm install -g npm@12.0.2
npm notice
npm error A complete log of this run can be found in: /root/.npm/_logs/2026-09-03T09_32_49_119Z-debug-0.log
time=2026-09-03T09:32:52.385Z level=INFO msg="sub-process exited" argo=true error="exit status 1"
time=2026-09-03T09:32:52.385Z level=INFO msg="file signal handler exiting due to context cancellation" argo=true
Error: exit status 1
```

---

## Caveats

- The three captures ran between 09:21 and 09:35 UTC against tips that are at
  or ahead of the original 07:47–08:35 failures (mta: `076668d` vs `0f640294`;
  acb: identical SHA; spaxel: `88b4af9c`). The exit codes, failing nodes and
  retry shapes all match the originals exactly; the specific error text is
  therefore evidence for what the originals hit, not a byte-for-byte record of
  those runs — their logs were destroyed by `podGC: OnPodCompletion` before
  anyone could read them.
- Full raw logs, per-pod snapshots and workflow JSON for each capture were
  preserved outside the repo under `/tmp/spaxel-logcap/` (ephemeral); this
  document is the durable copy of the failing-step excerpts.
