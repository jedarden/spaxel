# Confirmation: sim supplies its token only as the unread `X-Spaxel-Token` header → token-supply path is DEAD

**Tracking:** bead `bf-29wyl` (second link of the `bf-34lwt` split).
**Date:** 2026-07-07.
**Scope:** isolate and re-verify the *sim/header-side* half of the
tokenless-sim REJECT root cause against current source. This confirms the
three statements "the sim sets the token only as the `X-Spaxel-Token`
header", "the hello JSON body has no `token` field", and "the mothership
never reads that header on the WS path" — i.e. the **token-supply option is
currently dead**. The validator-side half is covered by the sibling note
[`token-validator-wiring-confirm.md`](./token-validator-wiring-confirm.md)
(bead `bf-5ig3e`); both roll up into the parent
[`token-reject-root-cause.md`](./token-reject-root-cause.md) (bead
`bf-34lwt`). This note re-checks the three header-side facts that chain into
"dead path", so a drift in any of them is detectable on its own.

> **Path convention.** Citations are full repo-relative paths. The `bf-29wyl`
> task body drops the `mothership/` module prefix, so its `cmd/sim/main.go`
> is `mothership/cmd/sim/main.go` here.
>
> **Two sim copies — both behave identically for this finding.**
> - `cmd/sim/main.go` — the module `go.work` lists (`use ./cmd/sim`); the
>   dev/test path. This copy has no `scenario.go`.
> - `mothership/cmd/sim/` — the copy the **Dockerfile actually builds** (it
>   does `COPY mothership/ ./` then `go build ./cmd/sim`, so `./cmd/sim`
>   resolves here).

## Fact 1 — the sim sets its token ONLY as the `X-Spaxel-Token` HTTP header

The token is attached to the WebSocket *dial* request as a request header in
all three connect paths, and nowhere else:

- `cmd/sim/main.go:313`: `reqHeader.Set("X-Spaxel-Token", token)` — fed to
  `websocket.DefaultDialer.DialContext(ctx, u.String(), reqHeader)` (the dial
  a few lines below).
- `mothership/cmd/sim/main.go:633`: `headers.Set("X-Spaxel-Token", token)`.
- `mothership/cmd/sim/scenario.go:318`: `headers.Set("X-Spaxel-Token", token)`.

`token` here is the provisioned node token (auto-generated to a 64-hex dummy
if `--token` is empty — `cmd/sim/main.go:120-125`,
`mothership/cmd/sim/main.go:596-601`). It is supplied to the mothership
exclusively via this HTTP header on the upgrade request. There is no second
delivery channel.

## Fact 2 — the hello JSON body has NO `token` field

The hello map built and sent as the node's first WS frame omits `token`
entirely:

- `cmd/sim/main.go:348-359`:
  ```
  hello := map[string]interface{}{
      "type":            "hello",
      "mac":             macToString(n.MAC),
      "firmware_version": "sim-1.0.0",
      "capabilities":    []string{"csi", "ble", "tx", "rx"},
      "chip":            "ESP32-S3",
      "flash_mb":        16,
      "uptime_ms":       1000,
      "pos_x":           n.Position.X,
      "pos_y":           n.Position.Y,
      "pos_z":           n.Position.Z,
  }
  ```
- `mothership/cmd/sim/main.go:652-665`: keys are
  `type, mac, firmware_version, capabilities, chip, flash_mb, uptime_ms,
   wifi_rssi, ip, pos_x, pos_y, pos_z`.
- `mothership/cmd/sim/scenario.go:331-342`: keys are
  `type, mac, firmware_version, capabilities, chip, flash_mb, uptime_ms,
   wifi_rssi, ip`.

None of the three contains a `token` key. A targeted grep confirms it — the
only `"token"` hits in each file are the `--token` *flag definition* (the CLI
argument), never a JSON body key:

```
$ grep -n '"token"' cmd/sim/main.go mothership/cmd/sim/main.go mothership/cmd/sim/scenario.go
cmd/sim/main.go:54:    flagToken = flag.String("token", "", "Provisioning token (auto-generated if empty)")
mothership/cmd/sim/main.go:49:    flagToken = flag.String("token", "", "Provisioning token (auto-generated if empty)")
mothership/cmd/sim/scenario.go:    (no "token" hit at all)
```

`HelloMessage.Token` carries the JSON tag `json:"token,omitempty"`
(`mothership/internal/ingestion/message.go:22`). Since the body never sets
`token`, that field deserializes to `""` from either sim binary.

## Fact 3 — the mothership NEVER reads `X-Spaxel-Token` on the WS path

A grep for the literal header name across the entire mothership tree,
excluding the sim sources that *set* it and the test files that document the
contract, returns **zero** hits:

```
$ grep -rn 'X-Spaxel-Token' mothership/ | grep -v '_test.go' | grep -v 'cmd/sim/'
(no output; exit status 1)
```

The only remaining references in the tree are the sim's own `Set(...)` calls
and an e2e test *comment* that asserts this exact fact
(`mothership/tests/e2e/e2e_test.go:86-88`: *"spaxel-sim nodes present no
token in their hello message (only the X-Spaxel-Token header, which the
ingestion server does not read)"*). There is no production code that reads
it.

The WS upgrade handler inspects nothing off the incoming request either.
`mothership/internal/ingestion/server.go:455-469`:

```go
func (s *Server) HandleNodeWS(w http.ResponseWriter, r *http.Request) {
    ...
    w.Header().Set("Content-Type", "application/json")   // :463 — OUTBOUND response header
    ...
    conn, err := s.upgrader.Upgrade(w, r, nil)           // :469 — r passed through, never read for headers
```

The sole `Header` reference on this path is `w.Header().Set(...)` on the
*outbound* response (`:463`); `r` (the request bearing the `X-Spaxel-Token`
header) is handed straight to `Upgrade` and never inspected. (Every other
`Header.Get` / `Header.Set` hit under `mothership/internal/` is on an
*outbound* webhook/notify path or in its tests — `webhook`, `notify`,
`notifications/ntfy.go`, etc. — none touches the node WS path.)

## Conclusion — the token-supply path is currently DEAD

The validator reads only `hello.Token`
(`mothership/internal/ingestion/server.go:513`:
`tokenOK := hello.Token != "" && validator(hello.MAC, hello.Token)`), and
that field comes only from the JSON body (`message.go:22`). Combining the
three facts above:

- The sim supplies its token **only** as the `X-Spaxel-Token` HTTP header
  (Fact 1);
- which the mothership **never reads** on the WS path (Fact 3);
- and the hello body the mothership *does* read has **no `token`** field
  (Fact 2).

Therefore for every sim node the header is silently dropped, `hello.Token`
is `""`, and `hello.Token != ""` short-circuits `tokenOK` to `false` at
`server.go:513` **regardless of what token the sim provisioned**. The
parent bead's "have the sim supply valid tokens" option is **currently
NON-FUNCTIONAL** — the token is supplied to a header nobody reads.

> The token-supply path is dead: header ignored → `hello.Token` empty →
> `tokenOK` always `false`.

This isolates the sim/header-side half of `bf-34lwt`. Combined with the
validator-side facts in the sibling note (`bf-5ig3e`: the validator is
always wired, and acceptance/rejection hinges entirely on `hello.Token`),
it follows that a sim node is accepted only while the migration window is
open (the default 24h mask), then `sendReject`-ed with `invalid_token`
(`server.go:519-528`) once it closes — because the supplied token never
reaches the validator at all.
