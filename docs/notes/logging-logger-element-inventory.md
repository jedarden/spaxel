# Element Inventory — `mothership/internal/logging/logger.go`

**Source:** `mothership/internal/logging/logger.go` — package `logging`, 358 lines
**Compiled:** 2026-09-03 (bead spaxel-b2cb6101)
**Scope:** read-only compilation of the earlier extractions of this file — no source was modified.
**Companion:** [logging-logger-type-definitions.md](logging-logger-type-definitions.md)
(bead spaxel-de0df241) holds the full field-by-field and method-by-method tables for
the types summarized here.

## At a glance

| Category | Count | Elements (line of declaration) |
|---|---|---|
| Constants | 5 | `TraceLevel` (21), `DebugLevel` (23), `InfoLevel` (25), `WarnLevel` (27), `ErrorLevel` (30) — one `iota` block, lines 18–31 |
| Types | 3 | `Level` (16), `Logger` (71), `Config` (84) |
| Interfaces | 0 | none declared in this file |
| Package-level functions | 2 | `ParseLevel` (52), `New` (99) |
| Methods | 21 | 1 on `Level` (34); 20 on `*Logger` (125–354) |

The package clause is at line 4; imports at lines 6–13 (`fmt`, `io`, `os`,
`path/filepath`, `sync`, `time`). There are no package-level `var` declarations.

## Constants — 5, one `iota` block (lines 18–31)

Values are severity-ordered and compared with `>=` by `shouldLog` (line 160), so a
larger constant is more severe. Error-level messages additionally trigger the
rotation check inside `log` (lines 204–206).

| Constant | Value | Line | Doc-comment gist |
|---|---|---|---|
| `TraceLevel` | `0` | 21 | extremely verbose, detailed execution flow; deep debugging only |
| `DebugLevel` | `1` | 23 | voluminous; usually disabled in production |
| `InfoLevel` | `2` | 25 | default logging priority |
| `WarnLevel` | `3` | 27 | more important than Info; no individual human review needed |
| `ErrorLevel` | `4` | 30 | high-priority; a healthy application emits none |

These are the only constants in the file.

## Types — 3

### `Level` — `type Level int` (line 16)

Defined type (not an alias) over `int`, carrying the constant block above.

- Method: `String() string` (line 34, value receiver) → `"TRACE"`/`"DEBUG"`/`"INFO"`/`"WARN"`/`"ERROR"`; out-of-range values return `"UNKNOWN"`.
- Free function `ParseLevel` (line 52) parses the other direction, case-insensitively, defaulting to `InfoLevel` on unknown input.

### `Logger` — `type Logger struct` (lines 71–81)

The main logging structure; thread-safe via its own mutex, writes to stdout and a
size-rotated file simultaneously. All 9 fields are unexported: `mu`, `level`,
`file`, `filePath`, `enableStdout`, `enableFile`, `prefix`, `lastRotate`,
`maxSize` — field table in the companion doc. `lastRotate` is set at file init
(line 140) and never read elsewhere in this file. 20 methods (12 exported,
8 unexported), all pointer receivers — index below.

### `Config` — `type Config struct` (lines 84–95)

Constructor input for `New`. 5 exported fields: `Level`, `FilePath`,
`EnableStdout`, `Prefix`, `MaxSize`. `MaxSize <= 0` is defaulted to 100 MB by
`New` (lines 108–110). No methods.

## Interfaces — 0

**No interface type is declared in this file.** The only `interface` matches in
the source are the `args ...interface{}` variadic parameters of 9 functions
(lines 165, 191, 265, 274, 279, 284, 289, 294, 300). `Logger` asserts no
interface and is documented against none; structurally its `Close() error`
(line 146) gives it an `io.Closer`-shaped method set.

## Function / method line index

Package-level functions (2):

| Signature | Line | Notes |
|---|---|---|
| `ParseLevel(level string) Level` | 52 | case-insensitive; unknown input → `InfoLevel` |
| `New(cfg Config) (*Logger, error)` | 99 | sole constructor; errors only when file logging is requested and the file cannot be created/opened |

Methods on `Level` (1):

| Signature | Line | Notes |
|---|---|---|
| `String() string` | 34 | value receiver |

Methods on `*Logger` (20, pointer receivers):

| Signature | Line | Kind / notes |
|---|---|---|
| `initFileLogging(filePath string) error` | 125 | unexported — `MkdirAll` + `O_CREATE\|O_WRONLY\|O_APPEND` open; sets `file`/`filePath`/`lastRotate` |
| `Close() error` | 146 | exported — idempotent; takes `mu` |
| `shouldLog(level Level) bool` | 160 | unexported — `level >= l.level` |
| `formatMessage(level Level, format string, args ...interface{}) string` | 165 | unexported — timestamp + level + prefix rendering |
| `write(message string)` | 178 | unexported — stdout + file fan-out |
| `log(level Level, format string, args ...interface{})` | 191 | unexported — gate → format → lock → write |
| `checkRotation()` | 211 | unexported — `Stat` + `rotate` when `size >= maxSize` |
| `rotate() error` | 233 | unexported — close → `os.Rename` to timestamped path → re-init |
| `formatAndLogWithoutLock(level Level, format string, args ...interface{})` | 265 | unexported — no-lock path used from `checkRotation`/`rotate` |
| `Trace(format string, args ...interface{})` | 274 | exported — wrapper over `log` |
| `Debug(format string, args ...interface{})` | 279 | exported — wrapper over `log` |
| `Info(format string, args ...interface{})` | 284 | exported — wrapper over `log` |
| `Warn(format string, args ...interface{})` | 289 | exported — wrapper over `log` |
| `Error(format string, args ...interface{})` | 294 | exported — wrapper; Error-level also triggers `checkRotation` |
| `Fatal(format string, args ...interface{})` | 300 | exported — Error + `os.Exit(1)` |
| `SetLevel(level Level)` | 306 | exported — under `mu` |
| `SetFilePath(filePath string) error` | 314 | exported — re-init; empty path disables file output; on failure disables and returns the error |
| `GetFilePath() string` | 340 | exported — under `mu` |
| `IsFileEnabled() bool` | 347 | exported — under `mu` |
| `IsStdoutEnabled() bool` | 354 | exported — under `mu` |

## Verification

Cross-checked mechanically against the live file:

- `grep -n "^const"` → one block (18–31); the five names at lines 21, 23, 25, 27, 30.
- `grep -n "^type"` → exactly `Level` (16), `Logger` (71), `Config` (84).
- `grep -n "interface"` → 9 hits, all `...interface{}` parameters; no declaration.
- `grep -n "^func"` → 23 declarations (2 package-level + 21 methods), each indexed above.
- `grep -n "^var"` → no matches (no package-level variables).

## Context notes

- `mothership/internal/logging/` is untracked in git as of compilation (the
  package's own publication is a separate deliverable, noted by spaxel-de0df241
  as well); the line numbers above refer to the live worktree file.
- The sibling test file `logger_test.go` (544 lines) declares no package-level
  elements and is out of scope.
- Attribution: this file was previously extracted by bead spaxel-de0df241 (type
  definitions). This bead's scope is the category-organized inventory across
  constants, types, and interfaces; the overlap with the earlier doc is
  deliberate and cross-referenced rather than re-derived.
