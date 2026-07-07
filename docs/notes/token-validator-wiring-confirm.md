# Confirmation: validator wired unconditionally + reads `hello.Token` only

**Tracking:** bead `bf-5ig3e` (first link of the `bf-34lwt` split).
**Date:** 2026-07-07.
**Scope:** isolate and re-verify the *validator-side* half of the
tokenless-sim REJECT root cause against current source. This confirms the two
statements "validator configured? always YES" and "acceptance/rejection hinges
entirely on `hello.Token`". The header-supply half of the root cause is covered
by the parent note [`token-reject-root-cause.md`](./token-reject-root-cause.md)
(bead `bf-34lwt`); this note re-checks the two facts that note depends on, so a
drift in either is detectable on its own.

> **Path convention.** Citations are full repo-relative paths. The `bf-5ig3e`
> task body drops the `mothership/` module prefix, so its
> `cmd/mothership/main.go:4494` is `mothership/cmd/mothership/main.go:4494`
> here, and its `internal/ingestion/server.go:513` is
> `mothership/internal/ingestion/server.go:513`.

## Fact 1 — validator is wired UNCONDITIONALLY

`mothership/cmd/mothership/main.go:4492-4499`:

```go
provSrv := provisioning.NewServer(cfg.DataDir, cfg.MDNSName, msPort, cfg.NTPServer, cfg.InstallSecret)
r.Post("/api/provision", provSrv.HandleProvision)
ingestSrv.SetTokenValidator(provSrv.ValidateToken)   // :4494 — bare statement, no guard
if cfg.MigrationWindowHours > 0 {                     // :4495 — gates the DEADLINE, not the validator
    deadline := time.Now().Add(time.Duration(cfg.MigrationWindowHours) * time.Hour)
    ingestSrv.SetMigrationDeadline(deadline)
    log.Printf("[INFO] Migration window open until %s (%d h)", deadline.Format(time.RFC3339), cfg.MigrationWindowHours)
}
```

The `SetTokenValidator` call at `:4494` is a top-level statement in the wiring
sequence. There is **no `if` around it** — not behind `cfg.MigrationWindowHours`,
not behind any env var, not behind a build tag. `provSrv` is itself created
unconditionally at `:4492`. The immediately following `if
cfg.MigrationWindowHours > 0` (`:4495`) gates `SetMigrationDeadline`, not
`SetTokenValidator`. Therefore `s.tokenValidator` is always non-nil at runtime:
**"validator configured?" is always YES.**

## Fact 2 — the validator reads `hello.Token` only

`mothership/internal/ingestion/server.go:506-513`:

```go
// Token validation: if a validator is configured, reject unauthenticated nodes
// unless the migration window is still open, in which case allow but mark as Unpaired.
s.mu.RLock()
validator := s.tokenValidator
deadline := s.migrationDeadline
s.mu.RUnlock()
if validator != nil {
    tokenOK := hello.Token != "" && validator(hello.MAC, hello.Token)   // :513
```

`tokenOK` is a function of exactly two values, both drawn from the deserialized
`hello` WebSocket JSON frame:

- `hello.Token` — the `token,omitempty` JSON field of `HelloMessage`
  (`mothership/internal/ingestion/message.go:22`), populated only by
  `ParseJSONMessage(msg)` from the node's first WS frame (`server.go:489`,
  `hello, ok := parsed.(*HelloMessage)` at `:496`). It is **not** an HTTP
  header value.
- `hello.MAC` — passed through to the validator; irrelevant to *where* the
  token comes from.

The validator itself is `provSrv.ValidateToken`,
`mothership/internal/provisioning/server.go:135`:

```go
func (s *Server) ValidateToken(mac, token string) bool {
    ...
    expected := s.deriveToken(mac)
    return subtle.ConstantTimeCompare([]byte(expected), []byte(token)) == 1
}
```

Its signature takes only `(mac, token)` — there is no header, no `*http.Request`,
no `r *http.Request` in scope anywhere on this path. A grep for the header name
over the ingestion tree returns **no** reads:

```
$ grep -rn 'X-Spaxel-Token' mothership/internal/ingestion/    # (no hits)
```

The only `Header()` reference in `server.go` (`:463`) is `w.Header().Set(...)`
on an *outbound* response. The WS upgrade at `:469`
(`conn, err := s.upgrader.Upgrade(w, r, nil)`) inspects nothing on `r`.

So there is **no** path through which an HTTP header (e.g. the `X-Spaxel-Token`
header the sim does send) can flip `tokenOK` to `true`. Acceptance/rejection
hinges entirely on `hello.Token`.

## Conclusion

> The validator is always configured (`main.go:4494`, unconditional); acceptance
> vs. rejection hinges entirely on `hello.Token` (`server.go:513`), the JSON body
> field — no HTTP header is read on this path.

This isolates the validator-side half of `bf-34lwt`. Combined with the
header-side facts in the parent note (the sim supplies its token only as the
unread `X-Spaxel-Token` header, so `hello.Token` is always `""`), it follows
that `tokenOK` is always `false` for a tokenless sim node — and the node is
accepted only while the migration window is open (the default 24h mask), then
`sendReject`-ed with `invalid_token` once it closes.
