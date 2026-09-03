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

## Addendum (2026-09-03, later): AC3 closed — post-HTTPS image measured in CI

The "Not measured" disclosure above is now discharged. A real esp32s3 build of
the HTTPS-enabled config was executed in `iad-ci` on Argo Workflows
(standalone workflow cloning Forgejo `main`, no local patches), and the binary
was sized with `idf.py size` plus the build's own
`check_sizes.py` partition gate. AC3 is **pass**: the image fits the OTA slot
with 41% headroom.

### Result

| | Bytes | vs 2,031,616 B slot |
|---|---|---|
| **After — HTTPS enabled** (`CONFIG_ESP_HTTP_CLIENT_ENABLE_HTTPS=y`) | **1,190,512 B** (`0x122a70`) | **841,104 B (41%) free** |
| **Before — same tree, flag off** (control, single-line sdkconfig flip) | **1,190,128 B** (`0x1228f0`) | 841,488 B (41%) free |
| HTTPS-attributable delta (after − control) | **384 B** | — |

Verdict: **fits, comfortably.** The OTA A/B budget risk flagged for AC3 does
not exist at current tree state. Nothing needs tuning — no mbedTLS
record-buffer or `CONFIG_MBEDTLS_SSL_VARIABLE_BUFFER_LENGTH` override is
required, and no follow-up bead is filed. (Bin sizes are alignment-padded, so
384 B is an upper bound on the flag's flash cost; the true delta is smaller.)
The flag's cost is this small because mbedTLS, esp-tls and the CA bundle are
already linked for the `wss://` control channel — the config flip only wires
esp_http_client's TLS transport, exactly as the "Flash budget" section above
predicted.

### After-build detail (workflow `spaxel-fwsize-41b1f030-v4-stxm8`)

- Source: `main` @ `003fc0cf` (`CLONED_HEAD=003fc0cfa980ed71e187a7b185f45713b07366ca`),
  i.e. post-6dca95b7 (HTTPS flag) plus the two build-fix commits
  51174c80 / 378db82c (see provenance below).
- Image `espressif/idf:v5.2`, image-native idf-component-manager **1.5.1**,
  committed `dependencies.lock` honored (`format 1.0.0`,
  `LOCKUSED=committed`) — **espressif/mdns 1.12.0** compiled in.
- Build: `idf.py set-target esp32s3 && idf.py build`, exit 0, "Project build
  complete", 2026-09-03 16:35:14→16:38:14Z.
- `idf.py size`: total image 1,190,405 B (bin padded to 1,190,512 B); flash
  `.text` 788,179 B + `.rodata` 294,652 B; static IRAM 87,546 B (24.2% used);
  `.data` 19,772 B + `.bss` 25,632 B.
- `check_sizes.py` (the same gate CI enforces): "spaxel-firmware.bin binary
  size 0x122a70 bytes. Smallest app partition is 0x1f0000 bytes. 0xcd590 bytes
  (41%) free."

### Control build (workflow `spaxel-fwsize-41b1f030-ctrl2-4jvzf`)

Identical spec, same source `003fc0cf`, same image / manager / lock — the only
difference is one line in `firmware/sdkconfig.defaults` flipped off after
clone (`sed` line 50: `CONFIG_ESP_HTTP_CLIENT_ENABLE_HTTPS=y` → `=n`), with a
guard that aborts the build if the flip does not apply. Result: bin
1,190,128 B (`0x1228f0`), 841,488 B (41%) free, total image 1,190,021 B.
The delta column in the table is `after − control`.

### Why the plan's "before" figure is not the comparator

`docs/plan/plan.md:4624` records 1,684,597 B (~17% free) as the image size.
That number predates the current tree; today's HTTPS build is ~494 KB *smaller*
than it, which is obviously not a property of a config-only flag flip — the
codebase moved between that measurement and now. Attributing that difference
to `CONFIG_ESP_HTTP_CLIENT_ENABLE_HTTPS` would be wrong, so this addendum
measures its own control instead: identical tree, identical CI image, the flag
flipped off as the only difference. The HTTPS-attributable flash delta is the
after−control difference above; the fit verdict is independent of the stale
plan figure either way.

### Build provenance (what the measurement needed to compile)

Two commits between 6dca95b7 and this measurement exist solely to make the
tree build in CI; neither changes firmware behavior:

- `51174c80` — `firmware/dependencies.lock` regenerated in
  component-manager **1.x** format. The committed 2.0.0-format lock is only
  parseable by manager 2.x (fleet-side fix 717ca012), while the
  `espressif/idf:v5.2` build image ships manager **1.5.1** — so any build that
  uses the image as-is (including the fleet Docker build) needs the 1.x lock.
  The re-solve bumped espressif/mdns 1.11.3 → **1.12.0** (manifest constraint
  `>=1.3.0`). Both builds measured here use mdns 1.12.0, so the before/after
  pair is internally consistent; absolute sizes shift by whatever that bump
  contributes.
- `378db82c` — `firmware/main/safe_mode.c`: renamed the local NVS helper
  wrappers (`nvs_set_u32_commit` / `nvs_set_u8_commit`) that shadowed the IDF
  API functions of the same name, which broke compilation under
  `-Werror`. Renames only; call sites updated in the same file.

Also fixed on main earlier the same day (already cited in the bead chain):
`b0f5e1b6` (`-Werror=format` sites + IDF 5.x task-watchdog API) and
`0ab406bc` (CMake `REQUIRES` for `esp_task_wdt`).

### Reproduction

The exact spec is in the cluster — re-running it re-measures:

```bash
kubectl --server=http://traefik-iad-ci:8001 get workflow \
  spaxel-fwsize-41b1f030-v4-stxm8 -n argo-workflows -o yaml
```

Shape: a `measure` pod (image `espressif/idf:v5.2`, clone of Forgejo `main`,
`idf.py set-target esp32s3 && idf.py build`, then `idf.py size` +
`check_sizes.py` + `stat` between `=== FWSIZE-41B1F030 BEGIN/END ===` log
markers) followed by a 3600 s `hold` pod; `podGC: OnWorkflowCompletion` keeps
both pods (and therefore the log) retrievable for ~1 h after completion.
Submit the copied spec with `kubectl --kubeconfig=/home/coding/.kube/iad-ci.kubeconfig
create -f -` (workflow creation is the sanctioned write in `iad-ci`).
