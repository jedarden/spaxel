# Extraction Plan — `Level` group from `mothership/internal/logging/logger.go` to `types.go`

**Compiled:** 2026-09-03 (bead spaxel-0a457c3d)
**Basis:** the Level-related categorization recorded on bead spaxel-6f8fb6b6,
applied to the element inventory in
[logging-logger-element-inventory.md](logging-logger-element-inventory.md)
(bead spaxel-b2cb6101) and the type tables in
[logging-logger-type-definitions.md](logging-logger-type-definitions.md)
(bead spaxel-de0df241).
**Scope:** documentation only — no source file was modified by this bead.
**Serves:** the acceptance criteria of bead **spaxel-6a004fc5** ("Identify types
to extract from logger.go"), quoted and answered in
[Acceptance-criteria cross-reference](#acceptance-criteria-cross-reference) below.

## Plan in one sentence

Move the 8 `Level`-domain elements (1 type, 5 constants, 2 functions) verbatim,
in source order, into a new `types.go` in the same package with **no imports**,
and leave the other 23 elements (2 types, 1 constructor, 20 `*Logger` methods)
in `logger.go` untouched — deleting the moved declarations from `logger.go` is
the only change that file receives.

## Elements that move to `types.go` — 8

Criterion (from spaxel-6f8fb6b6): an element is Level-related when `Level` is
part of its **definition** — it defines `Level`, is typed `Level`, or is
inseparable from `Level`'s semantics — not when it merely consumes `Level`.

| # | Element | Current lines | Why it moves |
|---|---|---|---|
| 1 | `type Level int` | 16 | the anchor; defines the severity domain the whole group hangs off |
| 2 | `TraceLevel Level = iota` | 21 (const block 18–31) | typed `Level`; the `iota` ordering **is** the severity semantics (`shouldLog` compares `>=`) |
| 3 | `DebugLevel` | 23 | same block |
| 4 | `InfoLevel` | 25 | same block; also `ParseLevel`'s fallback default and the documented default priority |
| 5 | `WarnLevel` | 27 | same block |
| 6 | `ErrorLevel` | 30 | same block; special-cased by `log` (line 204) |
| 7 | `func (l Level) String() string` | 34 | method **on** `Level`, value receiver; defines the `TRACE`…`UNKNOWN` display vocabulary |
| 8 | `func ParseLevel(level string) Level` | 52 | signature entirely Level-domain (`string → Level`); inverse of `String` |

Constraints on the move:

- The const block moves **verbatim and in order**, doc comments included. The
  numeric values (0–4) are load-bearing for `shouldLog`'s `level >= l.level`
  (line 161) and for `Level`'s zero value.
- **`String` and `ParseLevel` move even though downstream bead
  spaxel-810c21bf's title names only "type and constants".** They are `Level`'s
  public API, depend on nothing beyond the constants, and move at zero cost;
  leaving them behind splits the type's API across two files for no dependency
  reason. This is the plan's explicit resolution of that title gap.
- Target layout for `types.go` (all in source order): package clause
  `package logging` (no package doc comment — see mechanics below),
  `type Level int`, the const block 18–31 verbatim, `String`, `ParseLevel`.
- The 8 elements use only `int`, `string`, and `switch` — **`types.go` needs no
  import statement at all.**
- No test changes: `logger_test.go` is in-package and uses bare identifiers, so
  it compiles unchanged against either file.

## Elements that remain in `logger.go` — 23

| # | Element | Current lines | Why it stays |
|---|---|---|---|
| 1 | `type Logger struct` | 71–81 | **consumes** `Level` (field `level Level` at 73; params/passes at 160, 165, 191, 265, 274–300, 306) but is defined by `sync`/`os`/`time`/`filepath`/`io`; moving it would drag 5 imports into the dependency-free file. Consumer, not definition |
| 2 | `type Config struct` | 84–95 | the borderline case, resolved to stay: one `Level` field (86) plus four string/bool/int64 fields; pairs with `New`. Test: if `Level` moved to another file of the same package, `Config` would not change at all |
| 3 | `func New(cfg Config) (*Logger, error)` | 99 | constructor; copies `cfg.Level`, defines nothing about `Level` |
| 4–23 | all 20 `*Logger` methods | 125, 146, 160, 165, 178, 191, 211, 233, 265, 274, 279, 284, 289, 294, 300, 306, 314, 340, 347, 354 | `Logger` behavior; the 5 Level-typed signatures and constant passes are consumption, not definition (`initFileLogging`, `Close`, `shouldLog`, `formatMessage`, `write`, `log`, `checkRotation`, `rotate`, `formatAndLogWithoutLock`, `Trace`, `Debug`, `Info`, `Warn`, `Error`, `Fatal`, `SetLevel`, `SetFilePath`, `GetFilePath`, `IsFileEnabled`, `IsStdoutEnabled`) |

The package doc comment (lines 1–3) and the import block (6–13) also stay in
`logger.go`.

## Interfaces — 0 (explicit empty category)

No interface type is declared in `logger.go`; there is nothing to move. The
only `interface` matches are the 9 `args ...interface{}` variadic parameters at
lines 165, 191, 265, 274, 279, 284, 289, 294, 300 — all on `*Logger` methods,
so they stay with the methods.

## Mechanics for the downstream beads

- **spaxel-9ec2c98f — "Create types.go file with package declaration":**
  `mothership/internal/logging/types.go`, package clause `package logging`,
  empty (absent) import block. That is the entire bead; content arrives in the
  next step.
- **spaxel-810c21bf — "Extract Level type and constants to types.go":** cut
  lines 16, 18–31, 34–49, 51–67 from `logger.go` and paste into `types.go` in
  that order — per the table above, **including `String` and `ParseLevel`**.
- **spaxel-baa6c116 — "Update logger.go to import types.go":** the title is a
  misnomer. Both files declare `package logging` and Go has no intra-package
  imports — nothing is added to any import block. The actual work is
  **deleting the moved declarations from `logger.go`** (and nothing else). If
  that bead is dispatched literally as "add an import", it is a no-op and
  should be closed as one.
- After all three, `logger.go` = package doc + imports + `Logger` + `Config` +
  `New` + the 20 methods; `types.go` = the 8 Level elements; `gofmt`-clean,
  `go vet`-clean, `logger_test.go` passing unchanged.

### Behavior invariants that must survive the split

1. Const values stay 0–4 in `iota` order (`shouldLog`'s `>=` at 161 and the
   zero value depend on it).
2. `Level`'s zero value is `TraceLevel`; `New` defaults `MaxSize` (108–110) but
   **not** `Level` (101 copies `cfg.Level` straight through) — so a zero-value
   `Config` logs at Trace. Preserve exactly; do not "fix" while moving.
3. `String`'s out-of-range default is `"UNKNOWN"`; `ParseLevel`'s default is
   `InfoLevel` and parsing is case-insensitive on the lowercase/uppercase pair
   only.
4. `log` still special-cases `ErrorLevel` for the rotation check (204–206).

## Completeness and unambiguity confirmation

The inventory accounts for **31 package-level elements** (3 types + 5
constants + 2 package-level functions + 21 methods). This plan assigns each
exactly once: 8 move (§"Elements that move") + 23 stay (§"Elements that
remain") = 31, a disjoint and exhaustive partition with no element left
unassigned and no element assigned twice. Interfaces are explicitly empty
rather than silently omitted. Line numbers were re-verified against the live
worktree file (358 lines) before publishing:

- `grep -nE "^(type|func|const)"` reproduces every declaration line cited above.
- `grep -n "interface"` → the 9 variadic-parameter hits only, no declaration.
- `grep -rn "internal/logging"` across all `*.go` outside the package → zero
  hits, so the split today touches exactly `logger.go` and `types.go` (plus the
  untouched in-package test).

Nothing in the plan depends on a judgment call left open: the one borderline
element (`Config`) is resolved with a stated test, the one title gap
(`String`/`ParseLevel`) is resolved explicitly, and the one misnomer
(`baa6c116`) is resolved to a deletion. **The plan is complete and
unambiguous.**

## Acceptance-criteria cross-reference

Bead spaxel-6a004fc5 requires:

1. **"List all Level-related types and constants identified"** → §"Elements
   that move to types.go" (rows 1–6: the `Level` type and the five constants).
2. **"Document any other types found in logger.go that should be in types.go"**
   → none besides `Level`. `Logger` and `Config` are documented as staying,
   with reasons, in §"Elements that remain in logger.go"; the two Level-domain
   functions (`String`, `ParseLevel`) ride along per the explicit resolution.
3. **"Confirm the extraction plan"** → §"Completeness and unambiguity
   confirmation".

Bead spaxel-0a457c3d requires: what moves (§move), what stays (§stay),
completeness/unambiguity confirmation (§confirmation), and this reference to
spaxel-6a004fc5's criteria (this section).

## Context notes

- `mothership/internal/logging/` is untracked in git as of this writing; its
  publication is a separate deliverable and is **not** part of this plan. All
  line numbers refer to the live worktree file.
- Nothing outside the package references it, so no import-path or API
  migration is triggered anywhere else in the repo.
