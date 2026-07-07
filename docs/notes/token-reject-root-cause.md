# Root Cause: tokenless-sim REJECT in the WS hello path

**Tracking:** bead `bf-34lwt` (first link of the `bf-4iewr` split).
**Date:** 2026-07-07.
**Scope:** explain exactly *why* a tokenless `spaxel-sim` node is currently
NOT rejected on a default boot, and exactly *when* it breaks. This is the
precise target the remaining `bf-4iewr` children build on.

> **Path convention.** Citations below are full repo-relative paths. The
> `bf-4iewr` task body drops the `mothership/` module prefix, so e.g. its
> `cmd/sim/main.go:633` is `mothership/cmd/sim/main.go:633` here, and its
> `cmd/mothership/main.go:4494` is `mothership/cmd/mothership/main.go:4494`.
>
> **Two sim copies — both behave identically for this finding.** There are two
> `spaxel-sim` source trees and both send the token only as a header with no
> `token` in the hello body:
> - `cmd/sim/main.go` — the module `go.work` lists (`use ./cmd/sim`); the dev/test
>   path. Header set at `cmd/sim/main.go:313`; hello body `cmd/sim/main.go:348-359`
>   (no `token` key). This copy has no `scenario.go`.
> - `mothership/cmd/sim/` — the copy the **Dockerfile actually builds** (it does
>   `COPY mothership/ ./` then `go build ./cmd/sim`, so `./cmd/sim` resolves here).
>   Header set at `mothership/cmd/sim/main.go:633` and
>   `mothership/cmd/sim/scenario.go:318`; hello bodies
>   `mothership/cmd/sim/main.go:652-665` and `mothership/cmd/sim/scenario.go:327-340`
>   (neither has a `token` key).
>
> A `grep '"token"'` over all four hello bodies returns only the `--token` flag
> definition, never a JSON body key — so `hello.Token` deserializes to `""` from
> either binary.

## TL;DR

The current "no reject" is a **24-hour migration-window MASK, not a real
fix.** The token validator is wired unconditionally, but the sim presents its
token only as an HTTP header the mothership never reads, so `hello.Token` is
always empty and `tokenOK` is always `false`. The only thing keeping sim nodes
connected is the default 24h migration window, which accepts any tokenless
node as `Unpaired`. The moment that window closes (`SPAXEL_MIGRATION_WINDOW_HOURS=0`
or uptime > 24h), every sim node is `sendReject`-ed with `invalid_token` and
disconnected.

## Confirmed facts (with citations)

### 1. The validator is wired UNCONDITIONALLY

`mothership/cmd/mothership/main.go:4494`:

```go
ingestSrv.SetTokenValidator(provSrv.ValidateToken)
```

There is **no config flag and no build gate** around this call. "Validator
configured?" is therefore **always YES** at runtime. The only thing the next
block gates on is the *deadline*, not the validator:

```go
// mothership/cmd/mothership/main.go:4495-4499
if cfg.MigrationWindowHours > 0 {
    deadline := time.Now().Add(time.Duration(cfg.MigrationWindowHours) * time.Hour)
    ingestSrv.SetMigrationDeadline(deadline)
    ...
}
```

So `s.tokenValidator` is always non-nil; only `s.migrationDeadline` is
conditional.

### 2. The validator reads `hello.Token` only

`mothership/internal/ingestion/server.go:513`:

```go
tokenOK := hello.Token != "" && validator(hello.MAC, hello.Token)
```

`validator` is `provSrv.ValidateToken`, signature `func(mac, token string) bool`
(see `SetTokenValidator` at `mothership/internal/ingestion/server.go:321-327`).
Its only inputs are `hello.MAC` and `hello.Token`. `hello.Token` maps to the
JSON field `token,omitempty` (`mothership/internal/ingestion/message.go:22`).

There is no other path that can set `tokenOK = true`: it requires a non-empty
`hello.Token` that the validator accepts.

### 3. The sim sends its token ONLY as the `X-Spaxel-Token` HTTP header

- `mothership/cmd/sim/main.go:633`: `headers.Set("X-Spaxel-Token", token)`
- `mothership/cmd/sim/scenario.go:318`: `headers.Set("X-Spaxel-Token", token)`

The header is attached to the WebSocket *dial* request
(`websocket.DefaultDialer.DialContext(ctx, url, headers)`).

The hello **JSON body** does **NOT** include a `token` field. The hello map
built at `mothership/cmd/sim/main.go:652-665` contains only:

```
type, mac, firmware_version, capabilities, chip, flash_mb,
uptime_ms, wifi_rssi, ip, pos_x, pos_y, pos_z
```

(identical field set in `mothership/cmd/sim/scenario.go:327-340`). No `token`
key. So `hello.Token` always deserializes to `""`.

### 4. The mothership NEVER reads that header → the token-supply path is DEAD

A search of the entire mothership tree for the literal header name returns
**zero** hits in any non-sim, non-test source:

```
$ grep -rn 'X-Spaxel-Token' mothership/   # only sim files
mothership/cmd/sim/main.go:633:        headers.Set("X-Spaxel-Token", token)
mothership/cmd/sim/scenario.go:318:    headers.Set("X-Spaxel-Token", token)
```

The WS upgrade handler never inspects the request either.
`mothership/internal/ingestion/server.go:469`:

```go
conn, err := s.upgrader.Upgrade(w, r, nil)
```

`HandleNodeWS` (`mothership/internal/ingestion/server.go:455`) reads nothing
off `r` before upgrading — no `r.Header.Get(...)`. (All `Header.Get` / `Header.Set`
hits under `mothership/internal/` are on the *outbound* webhook/notify path or
in their tests, e.g. `webhook`, `notify`, `notifications/ntfy.go`; none touch
the node WS path.)

**Consequence:** for every sim node, `hello.Token == ""`, so
`hello.Token != ""` short-circuits to `false` at `server.go:513`, making
`tokenOK` always `false` regardless of what token the sim provisioned. The
parent bead's "have the sim supply valid tokens" option is therefore
**currently NON-FUNCTIONAL** — the token is supplied to a header nobody reads.

### 5. The only thing accepting sim nodes is the migration window

`mothership/internal/ingestion/server.go:514-519`:

```go
if !tokenOK {
    if !deadline.IsZero() && time.Now().Before(deadline) {
        // accepted as Unpaired
        nc.Unpaired = true
    } else {
        ... reject ...
    }
}
```

`MigrationWindowHours` **defaults to 24** (`mothership/internal/config/config.go:139`,
`cfg.MigrationWindowHours = 24`). On a fresh boot with the default,
`mothership/cmd/mothership/main.go:4496` sets
`deadline = time.Now().Add(24h)`, so `!deadline.IsZero() && Now().Before(deadline)`
is true → every tokenless sim node is accepted with `nc.Unpaired = true`
rather than rejected. This matches the "no reject" observed in `PROGRESS.md`
under bead `bf-3hji`.

**Independent corroboration in the test harness.** The e2e harness pins this
exact behavior open and documents it inline — `mothership/tests/e2e/e2e_test.go:86-94`
sets `SPAXEL_MIGRATION_WINDOW_HOURS=24` with the comment: *"spaxel-sim nodes
present no token in their hello message (only the X-Spaxel-Token header, which
the ingestion server does not read), so they are effectively tokenless … a
tokenless node is only rejected when the migration window is closed … Pin the
window open here so the harness never rejects tokenless sim nodes regardless of
the production default (bf-4iewr)."* The harness itself therefore asserts that
the production default (24h) is the only thing preventing reject, and that the
header is unread — exactly points 4–6.

### 6. REJECT fires ONLY when the window is closed

The reject branch (`server.go:519-528`) is reached in exactly two cases:

**(a) `SPAXEL_MIGRATION_WINDOW_HOURS=0`.** Then
`mothership/cmd/mothership/main.go:4495` (`if cfg.MigrationWindowHours > 0`)
is false, `SetMigrationDeadline` is **never called**, and
`s.migrationDeadline` stays at its zero value (`time.Time{}`).
`server.go:515`'s `!deadline.IsZero()` is then false → reject branch.

**(b) Uptime > 24h** (default window). `time.Now().Before(deadline)` becomes
false → reject branch.

In both cases, because `hello.Token == ""` (point 4), `server.go:520-521`
logs `Node <MAC> rejected: missing token`, then `server.go:525`
`s.sendReject(conn, "invalid_token")` and `server.go:526` `conn.Close()`
disconnect the node. (`sendReject` is defined at
`mothership/internal/ingestion/server.go:841-846`; it writes a
`{"type":"reject","reason":"invalid_token"}` frame and the sim exits non-zero
on receipt, per the plan's simulator contract.)

## Summary statements (for the `bf-4iewr` acceptance criteria)

1. **Validator configured? — always YES.**
   `mothership/cmd/mothership/main.go:4494` wires it unconditionally.
2. **Migration window closed? — not on a default boot.**
   Default 24h (`config.go:139`) opens a window that masks the missing token.
3. **Current "no reject" is a 24h-window MASK, not a real fix.**
   It vanishes under `SPAXEL_MIGRATION_WINDOW_HOURS=0` or after 24h uptime.
4. **The token-supply path is currently DEAD.**
   The sim sends `X-Spaxel-Token` as a header (`cmd/sim/main.go:633`,
   `cmd/sim/scenario.go:318`) that the mothership never reads; `hello.Token`
   is empty in the body (`cmd/sim/main.go:652-665`), so `tokenOK` is always
   `false` (`server.go:513`).

## Implications for the remaining `bf-4iewr` children

There are three independent, non-overlapping fixes; the children should pick
exactly one and the others become moot:

- **(A) Accept tokenless sim nodes on purpose** — leave the validator on but
  guarantee the migration window is open for sim/test boots (e.g. a test-only
  `SetMigrationDeadline` far in the future, or `SPAXEL_MIGRATION_WINDOW_HOURS`
  sized to the test). Cheapest, but leaves the token path dead and relies on
  the very mask documented here.
- **(B) Make the token path real** — have the mothership read `X-Spaxel-Token`
  off the WS upgrade request in `HandleNodeWS` (`server.go:455-469`) and feed
  it into validation, OR have the sim include `token` in the hello body so
  `server.go:513` sees it. This makes "have the sim supply valid tokens" a
  genuine option and is the correct long-term fix; it also requires the sim's
  provisioned token to actually pass `ValidateToken` (HMAC over the install
  secret).
- **(C) Disable the validator for the hardware-free build** — add a config/build
  gate around the `SetTokenValidator` call at `main.go:4494` (e.g. an env flag
  defaulting to off only in the sim/test path). Simplest for CI but disables a
  real security control and must not leak into production.

The two statements that must hold in the final state regardless of choice:
the "no reject" must no longer depend on the 24h mask, and the chosen token
contract (header vs. body vs. none) must be honored end-to-end by both the
sim and the mothership.
