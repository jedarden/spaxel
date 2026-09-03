# OTA HTTPS Enablement — `CONFIG_ESP_HTTP_CLIENT_ENABLE_HTTPS=y`

**Date**: 2026-09-03
**Bead**: spaxel-bbe7f6fc (child of spaxel-8aa9703c, ADR-004)
**Change**: `firmware/sdkconfig.defaults` — flip `CONFIG_ESP_HTTP_CLIENT_ENABLE_HTTPS` from `=n` to `=y`

## What changed

One config flag, nothing else. `CONFIG_ESP_HTTP_CLIENT_ENABLE_HTTPS` compiles the
TLS transport into `esp_http_client`. With `=n` the component physically cannot
open an `https://` URL — `esp_http_client_perform()` fails with
`ESP_ERR_HTTP_CONNECT` before any byte is requested. The stale `# WebSocket
buffer sizes` header above it was replaced with a comment explaining what the
component actually serves, because the old comment pointed at the wrong
subsystem entirely.

This affects only the esp_http_client path. The `wss://` control channel uses
`esp_websocket_client` over `esp-tls` directly and was never gated by this flag.

## URL construction — verified, no code change needed

The OTA URL a node fetches is fully determined by the mothership's advertised
base URL; the firmware does no scheme rewriting:

- `mothership/internal/ota/manager.go:164`:
  `url := fmt.Sprintf("%s/firmware/%s", m.baseURL, meta.Filename)` — passes the
  configured scheme through verbatim.
- `mothership/internal/config/config.go:375-386`
  (`validateAdvertisedBaseURL`) accepts http and https; derivation from
  `SPAXEL_BIND_ADDR` is a fallback only, never an override.
- **Production is already https** (verified live, read-only, 2026-09-03 via the
  credential-free kubectl endpoint on `ardenone-cluster`, ns `spaxel`):
  env `SPAXEL_ADVERTISED_BASE_URL=https://spaxel.ardenone.com` on the pinned
  pod. So every OTA URL production hands a node is *already* `https://` — and
  with the flag `=n`, `ota_task` could not fetch it. **This flag flip is a
  repair of an already-broken path, not a stranding risk**: nodes on current
  firmware were unable to complete any OTA from the public origin regardless.
  Zero nodes are online (2026-08-29 verification,
  `docs/notes/ota-prerequisite-verification-2026-08-29.md`), so there is no
  fleet mid-transition to protect.
- Server-side `/firmware` auth is live (404-not-401, exempted from OAuth
  middleware in the IngressRoute) — same 2026-08-29 note.

Everything downstream of the URL was already TLS-ready:
`ota_task` sets `.crt_bundle_attach = esp_crt_bundle_attach`
(`firmware/main/websocket.c:1029`) and sends `X-Spaxel-MAC` /
`X-Spaxel-Token` headers (`websocket.c:1048-1049`, ADR-006). The CA bundle is
compiled in (`CONFIG_MBEDTLS_CERTIFICATE_BUNDLE`) and was already linked for
`wss://` (`websocket.c:278`), so no new certificate material is needed.

## Flash budget

| Slot | Size |
|---|---|
| `ota_0` / `ota_1` (each, `firmware/partitions.csv`) | 0x1F0000 = **2,031,616 B** |
| App at A/B redesign (partitions.csv comment, bf-436bh) | 1,623,392 B (~399 KiB free) |
| App incl. CA bundle, ADR-008 measurement | 1,684,597 B (~339 KiB free) |

The CA bundle (+65,968 B, ADR-008) is already in the image for `wss://`, and
mbedTLS is already linked. The incremental flash cost of this flag is the
esp_http_client TLS transport wiring itself — expected small relative to the
~339 KiB of remaining headroom.

**Not measured — no ESP-IDF toolchain on ex44.** The before/after binary sizes
could not be produced locally. Disclosure per the dispatch rules: the flag flip
is config-only and was verified by read + diff, not by an `idf.py build`. The
post-change image size will be observable from the CI `spaxel-build` firmware
job this push triggers, but that is a follow-up observation, not part of this
commit.

## RAM cost of a TLS OTA download (OTA path itself)

Code-verified costs on the `ota_task` path (`websocket.c`):

| Item | Cost | Where |
|---|---|---|
| `ota_task` stack | 16,384 B | `websocket.c:902` (`xTaskCreate(…, 16384, …)`) |
| esp_http_client internal buffer | 4,096 B | `websocket.c:1023` (`.buffer_size = 4096`) |
| Download buffer | 4,096 B | `websocket.c:1112` (`malloc(4096)`) |
| TLS session (handshake + record buffers) | **estimated ~40–50 KB peak** | config-derived, see below |
| CA bundle | flash-resident (`.rodata`), not heap | ADR-008 |

The ~40–50 KB TLS figure is an **estimate, not a measurement** — derived from
the mbedTLS default incoming/outgoing record buffer sizes (16 KB each, since
`CONFIG_MBEDTLS_SSL_VARIABLE_BUFFER_LENGTH` is not set in
`sdkconfig.defaults`) plus handshake-time certificate parsing and
bookkeeping. Do not quote it as measured.

**Measurement ownership**: the hardware heap measurement for a TLS handshake is
bead **spaxel-8cbba203** (still open — no result to cite). That bead covers the
`wss://` control-channel handshake; the OTA download adds a **second
concurrent TLS session** on top of it, because the control channel stays up
during the download (`websocket_send_ota_status` is called from `ota_task`
mid-download). SPIRAM is enabled (`CONFIG_SPIRAM=y`,
`SPIRAM_USE_MALLOC=y`), which is the headroom backstop. Bench verification is
additionally blocked by spaxel-6c9344e4 (hardware access).

**Plan-gate implication**: `docs/plan/plan.md:733` specifies "check free heap
≥ 20 KB before starting OTA, reject with `low_heap`". That gate is **not
implemented** in `websocket.c` today — `ota_task` starts unconditionally — and
a 20 KB threshold written against a plain-HTTP download would be insufficient
for a TLS download anyway (single-session estimate alone exceeds it). If the
gate is implemented later it should be sized against measured TLS numbers, not
the plan's 20 KB. Flagging as a plan follow-up; out of scope here.

Node health reporting (`free_heap_bytes`, `websocket.c:571`) already exists,
so post-rollout fleet heap behavior is observable without new firmware.

## Disclosures

- **No ESP-IDF toolchain on ex44** → no local firmware build, no binary size
  delta, no runtime heap measurement. Verified by read + config diff only.
- **No Go toolchain on ex44** (memory: `spaxel-go-toolchain-absent`) →
  `go test ./...` / `go vet` not run. No Go files were modified; the mothership
  side of the URL path was read and verified only.
- **spaxel-8cbba203 (wss:// handshake heap) still open** → its number could not
  be cited; estimates above are labeled as such.
