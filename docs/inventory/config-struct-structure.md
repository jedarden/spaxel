# Config Struct Structure Inventory

**Bead:** spaxel-70f09f10 — "Extract Config struct structure"
**Date:** 2026-09-04
**Source:** `mothership/internal/config/config.go`
**Verified at:** HEAD `380a65c9` (committed state). The working tree carries an
unrelated in-flight edit by another agent (see §6 Delta) — none of the findings
below come from uncommitted files.
**Feeds:** spaxel-f1e7dfaa ("Study how fields are documented in
`mothership/internal/config/config.go`") — this bead blocks it.
**Sibling:** `docs/inventory/logger-structure.md` (same inventory format, same
bead family).
**Scope:** structure only — fields, types, tags/annotations, grouping. Prose
quality of the in-source comments is deliberately out of scope (that is
f1e7dfaa's subject).

---

## 1. Summary

| Property | Value |
|---|---|
| Declared at | `config.go:20` (`type Config struct {`), body closes at `config.go:83` |
| Total fields | **27** |
| `string` fields | 17 |
| `int` fields | 5 |
| `bool` fields | 5 |
| Struct tags | **None** — zero backticked tags on any field |
| Annotation style | Line comments (`//`) + block comments (`//` paragraphs above the field); env-var name recorded in the comment, never in a tag |
| Named groups | **11** group-header comment lines inside the body: 10 top-level groups + 1 sub-comment (`// Replay buffer`, nested inside `Processing`) |
| Ungrouped fields | 2 (`AdvertisedBaseURL`, `NTPLocalEnabled`) — both carry multi-line block doc comments instead of a group header |
| Exported types in package | 1 (`Config` is the only `type` declaration in the file) |
| Env vars read by `Load()` | 27 (`SPAXEL_BIND_ADDR` … `TZ`) |
| Other structs | None — `Config` is the sole struct; no nested/embedded types, all fields are scalars |

Every field is exported (uppercase), scalar-typed, and has no pointer, slice,
map, struct or interface type anywhere in the struct.

---

## 2. Field list

Line numbers are from HEAD. `Default` / `Validation` are what `Load()`
enforces — the ranges live in `Load()`, not in the struct body.

| # | Line | Field | Type | Group | Env var | Default | Validation |
|---|---|---|---|---|---|---|---|
| 1 | 22 | `BindAddr` | `string` | Network | `SPAXEL_BIND_ADDR` | `0.0.0.0:8080` | — |
| 2 | 32 | `AdvertisedBaseURL` | `string` | *(ungrouped)* | `SPAXEL_ADVERTISED_BASE_URL` | *derived* (see §4) | explicit value must be http/https + non-wildcard host, else fatal |
| 3 | 35 | `DataDir` | `string` | Paths | `SPAXEL_DATA_DIR` | `/data` | — |
| 4 | 36 | `StaticDir` | `string` | Paths | `SPAXEL_STATIC_DIR` | `/dashboard` | — |
| 5 | 37 | `SeedFirmwareDir` | `string` | Paths | `SPAXEL_SEED_FIRMWARE_DIR` | `/firmware` | — |
| 6 | 40 | `MDNSName` | `string` | mDNS | `SPAXEL_MDNS_NAME` | `spaxel` | — |
| 7 | 41 | `MDNSEnabled` | `bool` | mDNS | `SPAXEL_MDNS_ENABLED` | `true` | `true/false/1/0` |
| 8 | 44 | `LogLevel` | `string` | Logging | `SPAXEL_LOG_LEVEL` | `info` | one of `debug,info,warn,error` |
| 9 | 45 | `LogFilePath` | `string` | Logging | `SPAXEL_LOG_FILE_PATH` | *(empty)* | optional; set ⇒ file logging |
| 10 | 46 | `LogStdout` | `bool` | Logging | `SPAXEL_LOG_STDOUT` | `true` | `true/false/1/0` |
| 11 | 49 | `FusionRateHz` | `int` | Processing | `SPAXEL_FUSION_RATE_HZ` | `10` | integer, `[1,20]` |
| 12 | 52 | `ReplayMaxMB` | `int` | Processing / Replay buffer | `SPAXEL_REPLAY_MAX_MB` | `360` | integer, `[10,10000]` |
| 13 | 53 | `ReplayCompression` | `bool` | Processing / Replay buffer | `SPAXEL_REPLAY_COMPRESSION` | `true` | `true`/`false` |
| 14 | 54 | `ReplayChunkSizeMB` | `int` | Processing / Replay buffer | `SPAXEL_REPLAY_CHUNK_MB` | `64` | integer, `[1,100]` |
| 15 | 57 | `InstallSecret` | `string` | Security | `SPAXEL_INSTALL_SECRET` | *(auto-generated, stored in DB)* | hex; if set ≥ 32 bytes (64 hex chars) |
| 16 | 58 | `MigrationWindowHours` | `int` | Security | `SPAXEL_MIGRATION_WINDOW_HOURS` | `24` | integer, `[0,168]` (`0` = disabled) |
| 17 | 59 | `DemoMode` | `bool` | Security | `SPAXEL_DEMO_MODE` | `false` | `true/false/1/0` |
| 18 | 60 | `MaxDashboardClients` | `int` | Security | `SPAXEL_MAX_DASHBOARD_CLIENTS` | `10` | integer, `[1,100]` |
| 19 | 63 | `NTPServer` | `string` | Time | `SPAXEL_NTP_SERVER` | `pool.ntp.org` (or this host's address when `NTPLocalEnabled`) | — |
| 20 | 64 | `Timezone` | `string` | Time | `TZ` | `UTC` | must load as an IANA zone |
| 21 | 70 | `NTPLocalEnabled` | `bool` | *(ungrouped)* | `SPAXEL_NTP_LOCAL_ENABLED` | `false` | `true/false/1/0` |
| 22 | 73 | `MQTTBroker` | `string` | MQTT (optional) | `SPAXEL_MQTT_BROKER` | *(disabled)* | if set: valid URL, scheme `mqtt://` or `mqtts://` |
| 23 | 74 | `MQTTUsername` | `string` | MQTT (optional) | `SPAXEL_MQTT_USERNAME` | *(empty)* | optional |
| 24 | 75 | `MQTTPassword` | `string` | MQTT (optional) | `SPAXEL_MQTT_PASSWORD` | *(empty)* | optional; never logged |
| 25 | 78 | `WifiSSID` | `string` | WiFi credentials | `SPAXEL_WIFI_SSID` | *(unset)* | optional; first-boot seed only (ADR-005) |
| 26 | 79 | `WifiPassword` | `string` | WiFi credentials | `SPAXEL_WIFI_PASSWORD` | *(unset)* | optional; first-boot seed only (ADR-005) |
| 27 | 82 | `GitHubToken` | `string` | GitHub API access | `SPAXEL_GITHUB_TOKEN` | *(empty)* | optional |

Two env vars have non-`SPAXEL_` names: `TZ` (→ `Timezone`) and — historically —
none other. All 25 remaining bindings are `SPAXEL_<UPPER_SNAKE>` of the field
name, but the mapping is **manual**: nothing derives it, and three bindings
break the naive field-name expectation:

- `ReplayChunkSizeMB` ← `SPAXEL_REPLAY_CHUNK_MB` (field says `ChunkSize`, env says `CHUNK`)
- `Timezone` ← `TZ` (not `SPAXEL_TIMEZONE`)
- `MDNSName` / `MDNSEnabled` ← `SPAXEL_MDNS_*` (lowercase acronym in env, `MDNS` in field)

---

## 3. Grouping

Grouping is by **`// <Group name>` comment lines inside the struct body** —
plain comments, not Go constructs. There are 11 such header lines: 10 top-level
groups, plus one sub-comment (`// Replay buffer`) nested inside the
`Processing` group. Blank lines separate groups.

| Group header (verbatim) | Fields | Lines |
|---|---|---|
| `// Network` | `BindAddr` | 21–22 |
| *(none — block doc comment only)* | `AdvertisedBaseURL` | 24–32 |
| `// Paths` | `DataDir`, `StaticDir`, `SeedFirmwareDir` | 34–37 |
| `// mDNS` | `MDNSName`, `MDNSEnabled` | 39–41 |
| `// Logging` | `LogLevel`, `LogFilePath`, `LogStdout` | 43–46 |
| `// Processing` (+ sub-comment `// Replay buffer`) | `FusionRateHz`, `ReplayMaxMB`, `ReplayCompression`, `ReplayChunkSizeMB` | 48–54 |
| `// Security` | `InstallSecret`, `MigrationWindowHours`, `DemoMode`, `MaxDashboardClients` | 56–60 |
| `// Time` | `NTPServer`, `Timezone` | 62–64 |
| *(none — block doc comment only)* | `NTPLocalEnabled` | 66–70 |
| `// MQTT (optional)` | `MQTTBroker`, `MQTTUsername`, `MQTTPassword` | 72–75 |
| `// WiFi credentials (optional, first-boot seeding only per ADR-005)` | `WifiSSID`, `WifiPassword` | 77–79 |
| `// GitHub API access (for Kaniko releases and other GitHub operations)` | `GitHubToken` | 81–82 |

Notes on the two ungrouped fields:

- `AdvertisedBaseURL` (L32) follows `Network` but carries a 7-line block
  comment explaining the BindAddr/advertised-URL split (ADR-004) rather than a
  group header — it reads as a member of `Network` by position only.
- `NTPLocalEnabled` (L70) follows `Time` and is semantically time-related (it
  starts an embedded SNTP responder), but again carries a 4-line block comment
  instead of a group header. Nothing in the file states either membership.

Structural consequence: any consumer wanting machine-readable grouping must
parse comments — there is no tag, embedded struct, or nested type to key off.

---

## 4. Fields whose value is not a plain env→field copy

These are the only fields where `Load()` does something other than
`envOr(...)` into a scalar, which matters for anyone mapping the struct onto
the documented environment surface:

| Field | Behaviour in `Load()` |
|---|---|
| `AdvertisedBaseURL` | If `SPAXEL_ADVERTISED_BASE_URL` set: validated (fatal on bad value) and right-trimmed of `/`. If unset: **derived** from `BindAddr` via `deriveAdvertisedBaseURL`; derivation failure is *non-fatal* — field left empty with a `[WARN] OTA disabled` log (ADR-004). It is the only field with a fallback-derivation path. |
| `NTPServer` | Defaults to `pool.ntp.org`, but if `NTPLocalEnabled` is true **and** the server was not explicitly set, it takes the **hostname of `AdvertisedBaseURL`** (the already-validated, node-routable address) so nodes sync to the mothership itself; if that URL is unparseable it falls back to `pool.ntp.org`. Three `Time`-adjacent fields are therefore coupled: `NTPLocalEnabled` → (`NTPServer`, `AdvertisedBaseURL`). |
| `Timezone` | Read from `TZ`, then validated by actually loading the zone. `cmd/mothership/main.go:979` reads `TZ` a second time independently, so the env var has two readers. |
| `InstallSecret` | Optional; if unset the mothership generates one and persists it in SQLite. The struct field carries either the env override or the generated value. |

All other 23 fields are straight `envOr(name, default)` plus type/range checks,
with errors **collected** into `errs` and returned together (not fail-fast).

---

## 5. Struct tags / annotations

**There are no struct tags.** `grep '`'` over the struct body returns nothing,
and no field carries `` `json:…` ``, `` `yaml:…` ``, `` `env:…` ``, or any other
tag. The struct is never (de)serialised as a unit — `Load()` populates it
field-by-field from `os.Getenv`, and nothing marshals `Config` itself. The
JSON-facing config surface is the separate `settings` table / `/api/settings`
handler, not this struct.

Annotations are therefore comments only, of three shapes:

1. **Trailing line comment** — 26 of 27 fields (all but `AdvertisedBaseURL`,
   whose annotation is a block comment). Carries the env var name,
   default, and/or validation range. This is the *only* place the env-var
   binding is recorded; see the three mismatched bindings in §2.
2. **Multi-line block comment above the field** — `AdvertisedBaseURL` (7 lines)
   and `NTPLocalEnabled` (4 lines). These are the only fields whose semantics
   need more than a line, and both reference an ADR.
3. **Group header comment** — 9 lines, listed in §3.

Because the env-var name lives in free text rather than a tag, a field rename
or an env-var rename cannot be caught by the compiler or by any lint; the
`Load()` comment (`// SPAXEL_X - type, default N`) is the second, redundant
record of the same binding.

---

## 6. Delta vs the working tree (in-flight, not committed)

The working tree contains one uncommitted edit to `config.go` by another
agent's in-flight work (the GitHub API base-URL override): it appends a 28th
field `GitHubAPIURL string` (`SPAXEL_GITHUB_API_URL`, optional) at the end of
the struct, plus the matching `Load()` and `logConfig()` lines. It is **not**
part of this inventory, which documents HEAD. If that edit lands, it becomes
field #28 in the `GitHub API access` group and raises the `string` count to 18
— no other field, group, or tag would change.

---

## 7. Side finding

`SPAXEL_SKIP_MIGRATIONS` is documented in `docs/plan/plan.md` (Deployment >
Environment Variables, "Set to `true` to skip automatic schema migrations") but
is read by **no Go code** — `grep -rn SPAXEL_SKIP_MIGRATIONS mothership/
--include=*.go` returns nothing. It is a documented no-op. Out of scope for
this structure inventory; recorded here so the next config-surface bead does
not re-derive it.
