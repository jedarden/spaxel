# Blob identity: why live blobs have empty identity fields (bf-m1ynp)

Read-only diagnosis. Parent: bf-5h1t (populate canonical identity fields on a
runtime blob). Goal: pinpoint the single broken link in
`matcher-active → match-found → written-onto-served-blob → observable-at-/api/blobs`
so the fix child is minimal and certain.

## TL;DR

The broken link is **match-found**: `IdentityMatcher.GetMatch(blobID)` always
returns `nil` in the canonical reproduce path, so the canonical identity fields
`PersonName` / `AssignedColor` are never written. Three independent defects feed
this one broken link, and **all three** must be fixed for a live blob to carry
non-empty identity:

1. **No BLE advertisements are emitted.** `scripts/run-sim-local.sh` does not
   pass `--ble` to the sim → zero `"ble"` WebSocket frames → `rssiCache` stays
   empty.
2. **No person is registered.** Even with `--ble`, the walker's BLE address
   arrives only as a *discovered* device; `GetAllPersonDevices` requires
   `person_id IS NOT NULL`, and nothing registers/labels the walker.
3. **RSSI field-name mismatch (latent).** The sim sends `"rssi"` but the parser
   reads `"rssi_dbm"`, so RSSI is read as 0 → triangulation collapses.

The other three links in the chain are **OK** and were verified against the
current code (the stale `bf-2fz8` note in `processor.go:610` claiming
`SetTrackedBlobs` has no callers is superseded — bf-243os wired the producers).

## Link-by-link evidence

### Link 1 — matcher-active: **OK**

- Guard: `cmd/mothership/main.go:806-808`
  `if bleRegistry != nil { identityMatcher = ble.NewIdentityMatcher(...) }`.
- `bleRegistry` is built at `main.go:790-800` via
  `ble.NewRegistry(DataDir/ble.db)`. `NewRegistry`
  (`internal/ble/registry.go:227`) does `MkdirAll` + `sql.Open` + `migrate`,
  all of which succeed on `run-sim-local.sh`'s fresh writable `mktemp` data dir.
- Therefore `bleRegistry != nil` and `identityMatcher != nil` in the canonical
  path. The matcher is constructed and the write-back block at `main.go:2112`
  is entered every tick.

### Link 2 — match-found: **BROKEN** (the defect)

`IdentityMatcher.GetMatch(blobID)` (`internal/ble/identity.go:609`) returns
`m.matches[blobID]`. `m.matches` is populated by `UpdateBlobs` →
`triangulateAllDevices` (`identity.go:153`), which requires **both**:

- (a) ≥1 person-assigned, enabled device from
  `registry.GetAllPersonDevicesWithAliases()` — backed by
  `GetAllPersonDevices` (`internal/ble/registry.go`):
  `WHERE d.person_id IS NOT NULL AND d.is_archived = 0 AND d.enabled = 1`.
- (b) ≥1 recent RSSI reading in `m.rssiCache.GetRecent(addr, …)` with
  triangulation confidence ≥ 0.1 (`identity.go:180-207`).

In the canonical path neither holds:

- **(a) fails** — nothing registers a person. The sim's walker BLE address
  `AA:BB:CC:DD:EE:%02X` (`cmd/sim/main.go:1141`, walker IDs start at 0 —
  `cmd/sim/main.go:459`) is ingested as a *discovered* device only
  (`person_id` NULL). `run-sim-local.sh` does not `POST /api/ble/devices`.
- **(b) fails** — `run-sim-local.sh` does not pass `--ble`, so the sim never
  calls `sendBLEMessages` (`cmd/sim/main.go:980-981`), no `"ble"` frames reach
  the mothership, `rssiCache.AddWithTime` (`main.go:1767`) never fires, and
  `GetRecent` returns empty.

Result: `triangulateAllDevices` → nil → `assignBLEToBlobs` matches nothing →
`m.matches` empty → `GetMatch` → nil.

### Link 3 — written-onto-served-blob: **OK**

The canonical-field write-back is present and targets the served slice:
`cmd/mothership/main.go:2136-2152`

```go
identityYes, identityNo := true, false
for i := range blobs {
    match := identityMatcher.GetMatch(blobs[i].ID)
    if match == nil {
        blobs[i].IdentityResolved = &identityNo   // attempted-but-unmatched
        continue
    }
    blobs[i].PersonName    = match.PersonName      // canonical
    blobs[i].AssignedColor = match.PersonColor     // canonical
    blobs[i].IdentityResolved = &identityYes
}
pm.SetTrackedBlobs(blobs)                          // main.go:2152
```

`blobs` is the same slice returned by `blobTracker.track(result)` at
`main.go:2064`; the re-publish at `2152` overwrites the identity-less publish at
`2065`. So the write-back writes onto the **same** `[]TrackedBlob` that is
served. (Today, because `GetMatch` is nil, this writes `IdentityResolved=false`
and leaves `PersonName`/`AssignedColor` empty — the observed symptom.)

### Link 4 — observable-at-/api/blobs: **OK**

`/api/blobs` handler (`main.go:4304-4306`) calls `pm.GetTrackedBlobs()` and
`writeJSON`s it directly — same `ProcessorManager` store the write-back
published to. Same slice, canonical JSON keys (`processor.go:605-607`).

### Q3 — does `main.go:5568` (identity-less tracker output) bypass the copy sites?

No. `5568` is the `sigproc.TrackedBlob{…}` literal built inside
`blobTracker.track()`. It is the **input** to the copy sites, not a bypass: the
slice it belongs to is published identity-less at `2065` and then re-published
**with** identity at `2152` inside the same goroutine/tick. Not a broken link.

## Observed `/api/blobs` output in the canonical path

Because the matcher is non-nil but `GetMatch` is nil, the write-back runs and
sets `IdentityResolved = &false` while leaving the name/color empty:

```jsonc
{ "id": 1, "x": …, "personName": "", "assignedColor": "", "identityResolved": false }
```

i.e. identity appears *attempted-but-unmatched* with empty canonical name/color —
the reported "empty identity fields."

## Minimal change to make a live blob carry non-empty identity

All three are required (any one alone still yields an empty match):

1. **Emit BLE.** `scripts/run-sim-local.sh`: add `--ble` to the sim invocation.
2. **Register a person.** Before/early in the run, register the walker's
   address as a person so `GetAllPersonDevices` returns it:
   ```bash
   curl -s -X POST "http://localhost:$PORT/api/ble/devices" \
     -H 'Content-Type: application/json' \
     -d '{"addr":"AA:BB:CC:DD:EE:00","label":"Alice","type":"person","color":"#4488ff"}'
   ```
   (Address matches `cmd/sim/main.go:1141`; walker IDs start at 0.)
3. **Fix the RSSI field name.** Either change the sim to emit `"rssi_dbm"`
   (`cmd/sim/main.go:1143`) or align the parser (`internal/ingestion/message.go:55`
   reads `json:"rssi_dbm"`). Without this, RSSI is read as 0 and triangulation
   collapses regardless of (1) and (2).

## Pointers for the fix child

- Write-back site (already correct): `cmd/mothership/main.go:2136-2152`.
- Matcher data deps: `internal/ble/identity.go:153-222`.
- RSSI cache feed: `cmd/mothership/main.go:1767` (fires only on a `"ble"` frame).
- Person filter: `internal/ble/registry.go` `GetAllPersonDevices`
  (`WHERE person_id IS NOT NULL`).
- Stale note to correct: `internal/signal/processor.go:610-639` still claims
  `SetTrackedBlobs` has no callers — superseded by bf-243os (`main.go:2065,2152`).
