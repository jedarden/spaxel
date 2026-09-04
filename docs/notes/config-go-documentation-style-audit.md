# Documentation-Style Audit: `mothership/internal/config/config.go`

**Bead:** spaxel-f1e7dfaa — "Analyze documentation patterns"
**Date:** 2026-09-04
**Source:** `mothership/internal/config/config.go` (552 lines; `Config` declared at
`:20`, body closes at `:83`)
**Verified at:** HEAD `409275fb` — the analysis base is `git show
HEAD:mothership/internal/config/config.go`, byte-identical to the working tree
except for one uncommitted 28th field (`GitHubAPIURL`) belonging to another
agent's in-flight work. Nothing below is derived from uncommitted code. The
working-tree edit would add one more trailing-comment field in the `GitHub API
access` group and change no conclusion in this audit.
**Companion deliverable:** `docs/inventory/config-struct-structure.md`
(spaxel-70f09f10, closed) — the *structural* half of this family. That document
explicitly scopes prose quality out ("that is f1e7dfaa's subject"); this audit is
that prose half. Structural facts (field counts, groups, env bindings) are
cited from it rather than re-derived; where the two documents touch the same
fact they agree.
**Scope:** documentation style only. No code was changed by this bead.

---

## 1. What style is this? (AC 1: comments vs godoc)

**Both, deliberately split by payload size — and the AC's either/or framing
doesn't survive contact with the file.** There are four distinct comment
mechanisms in play:

| Mechanism | Where | Count | Renders in `go doc`? |
|---|---|---|---|
| Trailing `//` line comment on the field | 26 of 27 fields | 26 | **Yes** — verified: `go doc -all` prints `BindAddr string // HTTP bind address (default "0.0.0.0:8080")` |
| Prose block `//` comment above the field | 2 fields (`AdvertisedBaseURL` `:24-32`, `NTPLocalEnabled` `:66-69`) | 2 | Yes (standard godoc) |
| Group-header `//` comment inside the struct body | 11 lines (10 top-level + nested `// Replay buffer`) | 11 | Yes, as undifferentiated comment lines |
| Full godoc paragraph above functions/methods | every helper and method | all | Yes |

No `/* */` block comments exist anywhere in the file. The load-bearing finding
is the first row: **the trailing-inline style is not a godoc visibility
failure.** The usual reason to forbid trailing field comments (they vanish from
generated docs) does not apply — `go doc -all` reproduces them verbatim. So the
file's dominant style is defensible, not merely tolerated; a "convert everything
to godoc paragraphs" refactor would gain nothing.

The style axis that *does* matter is **annotation vs prose**: a trailing comment
carries a one-line fact (default, range, optionality); a prose block carries
*rationale* (why the field exists, what breaks without it). The file assigns
prose only to the two fields whose semantics exceed one line, which is the
right instinct — see §3.

Two caveats on the group headers: they are bare `// Name` lines, not godoc
section markers, so `go doc` shows them as unlabeled comment lines with no
grouping semantics; and they are the **only** machine-parseable grouping in the
file (no tags, no embedded structs — companion doc §5). Documentation is doing
structural work here, which raises the cost of letting it drift (§4).

## 2. Patterns in the field documentation (AC 2)

### 2.1 The struct trailing-comment grammar

Trailing comments follow a recognizable additive template:

```
<prose summary> [(default X)] [range [a,b]] [(optional …)] [<env var> - …]
```

Census over the 26 trailing comments (all counts verified by grep against the
HEAD extract):

| Element | Fields | Share |
|---|---|---|
| `(default X)` | 17 | 65% |
| `optional` | 6 (`LogFilePath`, `InstallSecret`, `MQTTBroker`, `MQTTUsername`, `MQTTPassword`, `GitHubToken`) | 23% |
| `range [a,b]` | 3 (`ReplayMaxMB`, `ReplayChunkSizeMB`, `MigrationWindowHours`) | 11% |
| names its env var | **3** (`WifiSSID`, `WifiPassword`, `GitHubToken`) | 11% |
| pipe-separated enum (`debug|info|warn|error`) | 1 (`LogLevel`) | 4% |
| `MUST`/normative language | 0 in trailing comments (1, in the `AdvertisedBaseURL` prose block) | — |
| ADR reference | 0 in trailing comments (2 in the body: `ADR-004` in the `AdvertisedBaseURL` block `:31`, `ADR-005` in the `WiFi credentials` group header `:77`) | — |

The pattern is: **defaults are near-universal, constraints are stated only where
`Load()` enforces a range, and everything machine-checkable lives in `Load()`
rather than here** (next point).

### 2.2 The `Load()` comment grammar — the strongest pattern in the file

Every environment read in `Load()` carries a uniform label comment:

```
// SPAXEL_FUSION_RATE_HZ - int, default 10, range [1,20]
```

This grammar is *complete*: exactly 27 comment lines for exactly 27 environment
variables, set-identical to the variables the file actually reads — zero
orphans in either direction (verified by extracting both sets and diffing).
It is also the more precise record: it names the type, the default, and the
enforced range, whereas the struct's trailing comment often states only the
default.

This makes the documentation **two-tier and redundant by design**: the same
binding is recorded twice (struct prose summary + `Load()` label). Redundancy
here buys one thing — the `Load()` line is greppable by env var, the struct
line is greppable by field. That is also why the drift in §4.2 (two quoting
conventions) and §4.3 (env var named in only 3 of 27 struct comments) matters:
the two records already disagree about formatting, and only one of them is
complete.

### 2.3 Other recurring patterns worth naming

- **Rationale-with-failure-story.** The best prose in the file explains a
  decision by describing the incident that motivated it: `AdvertisedBaseURL`
  (`:24-32`) documents that deriving the OTA URL from `BindAddr` "handed nodes
  `http://0.0.0.0:8080/firmware/...` and made OTA impossible in every default
  deployment". This is documentation of *why*, not *what* — the rarest and most
  valuable kind.
- **Normative language in exactly one place.** `MUST` appears once (`:25`,
  "It MUST be routable from the nodes"), on the one field where a wrong value
  is fatal to a downstream consumer. Elsewhere constraints are stated
  descriptively.
- **Consequence-not-mechanics.** `isWildcardHost` (`:363-365`) explains what the
  predicate *means operationally* ("valid to bind to but is never reachable as
  a destination, so it must never appear in a URL handed to a node") rather
  than restating its boolean logic. Same pattern in `EnvExists` (`:354-357`),
  which contrasts itself with the naive `os.Getenv() != ""` idiom and says what
  breaks without the distinction.
- **Cross-reference by ADR.** Deep rationale is delegated out (`See ADR-004`,
  `per ADR-005`) rather than duplicated inline — but only in prose blocks and
  group headers, never in trailing comments.
- **Load()-order ≠ struct-order.** `Load()` reads env vars in its own order,
  not the struct's: `MDNSName`/`MDNSEnabled` are swapped (struct `:40-41` vs
  `Load()` reading `SPAXEL_MDNS_ENABLED` first), and the `Security` group is
  split across the function — `InstallSecret`/`MigrationWindowHours` are read
  mid-function while `DemoMode`/`MaxDashboardClients` are read last, after the
  `GitHub` group. The group headers therefore describe the struct only; a reader
  tracing `Load()` top-to-bottom sees a different grouping than the struct
  advertises.

## 3. Well-documented exemplars (AC 3)

Ranked, with the property each one demonstrates:

1. **`AdvertisedBaseURL` (`:24-32`) — the file's model field comment.** Seven
   lines carrying, in order: an example value, the invariant (`MUST be
   routable`), the distinction from its sibling field (`BindAddr`), the
   concrete failure the distinction prevents (the `0.0.0.0` OTA story), and a
   pointer to the deciding ADR. It is also the *only* field with no trailing
   comment — the prose block fully subsumes the annotation role.
2. **`deriveAdvertisedBaseURL` (`:389-396`) + its `Load()` call site
   (`:104-109`) — the model comment *pair*.** The function doc states the
   derivation rule and justifies why ambiguity is fatal ("a wrong guess on a
   multi-homed host would reintroduce exactly the silent OTA failure this
   function exists to prevent, and a startup error is far cheaper to diagnose
   than a node that reports a successful trigger and then cannot download").
   The call site then documents the *opposite* case with equal care — why
   auto-derivation failing is NOT fatal ("refusing to start — taking down CSI
   ingestion, the dashboard and everything else — because the OTA URL is
   ambiguous would be wildly disproportionate"), and closes by reconciling the
   two ("an explicitly-set-but-invalid value above IS fatal, because that is an
   operator typo, not ambiguity"). Together they are the best documentation in
   the file: a fatality policy stated, justified, and contrasted, in the two
   places a reader actually looks. Note the asymmetry in §4.8: both are
   untested.
3. **`NTPLocalEnabled` (`:66-70`) — best example of coupling documentation.**
   States what the flag *starts* (an embedded SNTP responder), its external
   requirement (`CAP_NET_BIND_SERVICE` or root), and the failure mode (bind
   failure ⇒ non-fatal warning). It is the only field that documents a
   capability requirement.
4. **`EnvExists` (`:354-357`) — model comparative doc.** Every line earns its
   place: the contract (true even when set to an empty string), the alternative
   idiom, and the specific ambiguity ("not set" vs "set to empty string") that
   motivates the function.
5. **The `Load()` range comments** (e.g. `// SPAXEL_REPLAY_MAX_MB - int,
   default 360, range [10,10000]`) — the model *annotation*, precisely because
   they are uniform enough to grep mechanically.

**Anti-exemplar, for contrast:** `isValidLogLevel` (`:457`) — "checks if the
log level is valid." This restates the function name with no added information;
it is the only doc comment in the file whose content is fully derivable from
the identifier it names.

## 4. Consistency issues (AC 4)

Ranked by how likely each is to mislead a reader, with evidence. All line
numbers are HEAD `409275fb`.

### 4.1 The file is not gofmt-clean, and no gate would notice

`gofmt -l` flags the file; the diff is three sites:

- `:52-54` — the `Replay` block's column alignment is over-wide (fields padded
  to a longer name than exists in that alignment block).
- `:60` — `MaxDashboardClients` carries an extra alignment space.
- `:499-503` — the entire `SPAXEL_GITHUB_TOKEN` log block is indented one tab
  too deep (leftover from an enclosing `if` that was removed around it — the
  over-indentation *is* the fossil of the refactor).

Nothing polices this: `.golangci.yml` enables `errcheck, staticcheck, govet,
ineffassign, unused` — **no formatter linter** — and its `presets: [comments]`
exclusion actively suppresses comment-related diagnostics. So every finding
below is invisible to CI by configuration. Documentation quality in this file
is enforced by nothing and maintained by hand.

### 4.2 Quote drift: the same default stated twice in two conventions

Eight defaults appear in *both* the struct trailing comment and the `Load()`
label comment, and the two tiers quote them differently, 1:1 across all eight
(`0.0.0.0:8080`, `/data`, `/dashboard`, `/firmware`, `spaxel`, `info`,
`pool.ntp.org`, `UTC`):

```
:22  // HTTP bind address (default "0.0.0.0:8080")
:91  // SPAXEL_BIND_ADDR - string, default '0.0.0.0:8080'
```

Single set of facts, two formats, in the same file, with nothing reconciling
them. Cosmetic individually; collectively it signals that the two tiers are
edited independently and will eventually disagree about *content* rather than
punctuation.

### 4.3 The env-var binding is named in only 3 of 27 struct comments

The struct's trailing comments document defaults and ranges but almost never
name the environment variable that sets them (3 of 27: `WifiSSID`,
`WifiPassword`, `GitHubToken` — the three most recently added fields). So the
struct is not self-sufficient: to learn that `MigrationWindowHours` comes from
`SPAXEL_MIGRATION_WINDOW_HOURS` you must leave the struct and find the `Load()`
line. The three exceptions also break the file's own convention *by* naming
it, and prefix-style (`SPAXEL_GITHUB_TOKEN - …`) differs from the prose style
of the other 24. Either all 27 should name their variable or none should; the
file currently does neither, and the companion doc §2 records the three
bindings where the naive field-name derivation would be wrong
(`ReplayChunkSizeMB` ← `SPAXEL_REPLAY_CHUNK_MB`, `Timezone` ← `TZ`,
`MDNSName` ← `SPAXEL_MDNS_NAME`) — exactly the cases a comment convention
exists to prevent.

### 4.4 `NTPLocalEnabled` is the file's only style hybrid — and its halves are incomplete

The file's convention is clean either/or: 25 fields carry trailing comments
only, `AdvertisedBaseURL` carries a prose block only. `NTPLocalEnabled`
(`:66-70`) carries **both** a 4-line prose block and a trailing
`// default false`. The hybrid is also incomplete in an odd direction: the
prose block says nothing about the default, and the trailing comment says
nothing that isn't the default — each half exists only because the other half
omits something. A reader cannot infer from this one field whether the
file's rule is "blocks subsume annotations" (as `AdvertisedBaseURL` implies)
or "blocks supplement annotations" (as `NTPLocalEnabled` shows).

### 4.5 Four secrets, three redaction policies, one of them documented

`logConfig()` (defined `:468`; secret block `:483-503`) logs four secret-bearing
values under three different policies:

| Secret | Policy | Line | Documented? |
|---|---|---|---|
| `MQTTPassword` | fully redacted `***` | `:493` | **Yes** — `:75` "never logged", `:286` "sensitive - never logged" |
| `WifiPassword` | fully redacted `***` | `:497` | No — redacted but nowhere stated |
| `InstallSecret` | first 16 hex chars shown | `:484` | No — only "(truncated)" in the log string itself |
| `GitHubToken` | first 8 chars shown | `:500` | No — no comment records that partial logging is deliberate |

The file's own convention (§2.1) is to annotate sensitivity on the field
(`optional, never logged`), and exactly one field got it. A future editor
adding a fifth secret has four precedents and no stated rule to follow — which
is how the inconsistency arose. This is the known unowned side finding from the
GitHub-client family (`config.go:500` logs a partial token); recording it here
because it is at root a *documentation* defect: the policy exists but is
written down for one secret in four.

### 4.6 Two comments that are wrong or empty

- **`:541` — "Import here to avoid circular dependency"** (inside
  `LoggingConfig`). Nothing is imported. The circular dependency is avoided by
  returning an anonymous struct instead of the logging package's type — the
  comment describes a mechanism the code does not use, and would send a reader
  looking for an import that isn't there. Its neighbour line, "This returns a
  struct that matches logging.Config", is the *only* documentation of an
  `interface{}`-returning method and does not name the return's fields.
- **`:457` — `isValidLogLevel` "checks if the log level is valid."** Restates
  the identifier; the only zero-information doc comment in the file. The
  non-obvious part it should document — that the set is exactly
  `debug|info|warn|error` and where that set is enforced — is absent.

### 4.7 `Load()` order diverges from struct order

§2.3's finding, stated as a consistency defect: the 11 group headers describe
the struct's layout, but `Load()` reads the same variables in a different
order (`MDNSName`/`MDNSEnabled` swapped; the `Security` group split with
`DemoMode`/`MaxDashboardClients` read after the `GitHub` group). Neither order
is wrong, but the file presents two different groupings of the same 27 knobs
without remarking on the divergence.

### 4.8 Documented invariants with zero test coverage

The best-documented code in the file is its least-tested. Repo-wide grep shows
`deriveAdvertisedBaseURL`, `validateAdvertisedBaseURL`, and `isWildcardHost`
are referenced only by `config.go` itself, and `config_test.go` (20 tests)
exercises none of them, nor `logConfig()`'s redaction, nor
`SPAXEL_REPLAY_COMPRESSION` / `SPAXEL_REPLAY_CHUNK_MB` parsing. Every normative
statement in §3 — "MUST be routable", "non-fatal warning, not a startup
failure", "never logged" — is currently a comment with no test holding it true.
The comments are the only guard on those invariants, which makes the drift
risks in §4.1-4.5 load-bearing rather than cosmetic.

### 4.9 Documentation-vs-code drift, family-wide (cross-references)

Two drift instances already recorded elsewhere in this family, listed so the
next reader of this audit finds them:

- `SPAXEL_SKIP_MIGRATIONS` is documented in `docs/plan/plan.md` and read by no
  Go code (companion doc §7).
- The firmware seed directory is documented in `BUILD.md` as `SPAXEL_FIRMWARE_DIR`
  and read by no code; the real variable is `SPAXEL_SEED_FIRMWARE_DIR`
  (`:36`), non-recursive, and the compose mount that shadows it is a separate
  defect. (Recorded in the firmware-directories notes; `:36`'s own comment says
  only `/firmware` — it does not say non-recursive, which is the operational
  surprise.)

## 5. Recommendations

Small, ordered, and none require a behavioural change:

1. **Run `gofmt` on the file** (fixes §4.1's three sites mechanically). If doc
   alignment drift is to stay out of CI, the least-wrong policy is that
   committed files are formatter-clean.
2. **Pick one quote convention for stated defaults** (double quotes match the
   struct's 17 `default "X"` instances and the Go string literal in the actual
   default; the eight `'X'` occurrences are all in `Load()`) and apply it to
   the other tier.
3. **Decide the hybrid question** for `NTPLocalEnabled`: either add
   `(default false)` to the prose block and drop the trailing comment (the
   `AdvertisedBaseURL` model), or leave the trailing comment and trim the
   block to what the annotation can't say.
4. **State the redaction policy once** — one comment on `logConfig()`
   enumerating which secrets are fully redacted and which are logged partially,
   and why the partial prefix is safe — instead of one field comment carrying
   the whole rule.
5. **Fix the two comments in §4.6** (delete the false "Import here" line;
   give `isValidLogLevel` a comment that names the accepted set).
6. **If the two-tier design stays**, consider naming the env var in all 27
   struct trailing comments (or stating the derivation rule plus exceptions
   once in the package doc) so the struct is self-sufficient for operators.

This audit changes no code; items 1-6 are candidate work for an implementation
bead, not part of this one.

## 6. Method / reproducibility

Every quantitative claim above was re-derived from `git show
HEAD:mothership/internal/config/config.go` at `409275fb` (not the working
tree), using: field/trailing-comment counts by anchored grep over `:20-83`;
env-var coverage by extracting the set of variables the file reads and the set
carrying `// VAR - type, default` labels and diffing them (identical, 27 = 27);
quote drift by extracting `default "X"` and `default 'X'` and pairing (8 = 8);
`Load()` order by mapping each label comment to the nearest `cfg.X` assignment
below it; gofmt status by `gofmt -l` plus an explicit diff; godoc rendering by
`go doc -all` in a scratch module. Gates run for the package (scoped, since the
working tree carries another agent's in-flight edits elsewhere):
`go build ./internal/config/`, `go vet ./internal/config/`,
`go test -count=1 ./internal/config/` — all clean (`ok … 0.018s`).
