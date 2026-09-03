# Type Definitions in `mothership/internal/logging/logger.go`

**Source:** `mothership/internal/logging/logger.go` (package `logging`, 358 lines)
**Extracted:** 2026-09-03 (bead spaxel-de0df241)
**Scope:** read-only extraction — no source was modified.

## Summary

The file declares **3 type definitions** and **0 type aliases**:

| # | Name | Kind | Declared | Underlying / definition |
|---|------|------|----------|------------------------|
| 1 | `Level` | defined type (named integer type) | `logger.go:16` | `int` |
| 2 | `Logger` | struct | `logger.go:71` | struct (9 unexported fields) |
| 3 | `Config` | struct | `logger.go:84` | struct (5 exported fields) |

No interfaces, no function types, no generics, no embedded type declarations.

## Type aliases

**None.** There is no `type X = Y` declaration anywhere in the file (verified with
`grep -n "^type.*=" logger.go`). The nearest thing to an alias is `Level`, which is a
*defined type* (`type Level int`), not an alias: it has its own method set and is
distinct from `int` in the type system.

## 1. `Level` — `type Level int` (`logger.go:16`)

Severity of a log message. Defined type with underlying type `int`, carrying an
`iota` constant block (`logger.go:18-31`):

| Constant | Value | Doc comment |
|----------|-------|-------------|
| `TraceLevel` | `0` | Extremely verbose, detailed execution flow; typically disabled in production |
| `DebugLevel` | `1` | Voluminous; usually disabled in production |
| `InfoLevel` | `2` | Default logging priority |
| `WarnLevel` | `3` | More important than Info; no individual human review needed |
| `ErrorLevel` | `4` | High-priority; a smooth-running app shouldn't emit these |

Ordering is significant: `Logger.shouldLog` uses `level >= l.level`, so larger
constants are more severe.

### Methods on `Level`

| Signature | Receiver | Line | Notes |
|-----------|----------|------|-------|
| `String() string` | value `(l Level)` | `logger.go:34` | Maps constants to `"TRACE"`/`"DEBUG"`/`"INFO"`/`"WARN"`/`"ERROR"`; out-of-range values return `"UNKNOWN"` |

## 2. `Logger` — `type Logger struct` (`logger.go:71`)

The main logging structure; thread-safe via its own mutex, can write to stdout and
a size-rotated file simultaneously. **All fields are unexported** — construction and
mutation go through `New`, `SetLevel`, `SetFilePath`.

| Field | Type | Exported | Purpose |
|-------|------|----------|---------|
| `mu` | `sync.Mutex` | no | Serializes writes and all state mutation |
| `level` | `Level` | no | Current minimum severity |
| `file` | `*os.File` | no | Open log file handle; `nil` when file logging is disabled/closed |
| `filePath` | `string` | no | Path of the active log file |
| `enableStdout` | `bool` | no | Whether `write` mirrors messages to stdout |
| `enableFile` | `bool` | no | Whether file output is active |
| `prefix` | `string` | no | Rendered as `[prefix] ` in each message |
| `lastRotate` | `time.Time` | no | Set at file init (`initFileLogging`); not read elsewhere in this file |
| `maxSize` | `int64` | no | Rotation threshold in bytes; defaulted to `100 * 1024 * 1024` (100 MB) by `New` when `<= 0` |

### Methods on `*Logger` — exported (12)

All use a pointer receiver `(l *Logger)`.

| Signature | Line | Notes |
|-----------|------|-------|
| `Close() error` | `logger.go:146` | Closes the log file if open; safe to call multiple times; takes `mu` |
| `Trace(format string, args ...interface{})` | `logger.go:274` | Convenience wrapper → `log(TraceLevel, …)` |
| `Debug(format string, args ...interface{})` | `logger.go:279` | → `log(DebugLevel, …)` |
| `Info(format string, args ...interface{})` | `logger.go:284` | → `log(InfoLevel, …)` |
| `Warn(format string, args ...interface{})` | `logger.go:289` | → `log(WarnLevel, …)` |
| `Error(format string, args ...interface{})` | `logger.go:294` | → `log(ErrorLevel, …)`; Error-level logs also trigger `checkRotation` |
| `Fatal(format string, args ...interface{})` | `logger.go:300` | Logs at ErrorLevel then `os.Exit(1)` |
| `SetLevel(level Level)` | `logger.go:306` | Changes minimum level under lock |
| `SetFilePath(filePath string) error` | `logger.go:314` | Reinitializes file logging; empty path disables file output; on failure disables file logging and returns the error |
| `GetFilePath() string` | `logger.go:340` | Returns current path under lock |
| `IsFileEnabled() bool` | `logger.go:347` | Returns `enableFile` under lock |
| `IsStdoutEnabled() bool` | `logger.go:354` | Returns `enableStdout` under lock (read without lock — field is never mutated after `New`) |

### Methods on `*Logger` — unexported (8)

| Signature | Line | Notes |
|-----------|------|-------|
| `initFileLogging(filePath string) error` | `logger.go:125` | `MkdirAll` on the parent dir, open with `O_CREATE\|O_WRONLY\|O_APPEND` mode `0644`, sets `file`/`filePath`/`lastRotate` |
| `shouldLog(level Level) bool` | `logger.go:160` | `level >= l.level` |
| `formatMessage(level Level, format string, args ...interface{}) string` | `logger.go:165` | `<timestamp> [<LEVEL>] [<prefix> ]<message>\n` with RFC3339-ish `2006-01-02T15:04:05.000Z07:00` timestamp |
| `write(message string)` | `logger.go:178` | Fan-out to stdout (`fmt.Print`) and file (`l.file.WriteString`) |
| `log(level Level, format string, args ...interface{})` | `logger.go:191` | Gate → format → lock → write; rotation check on ErrorLevel only |
| `checkRotation()` | `logger.go:211` | `Stat`s the file; calls `rotate` when `size >= maxSize`; logs via the no-lock path |
| `rotate() error` | `logger.go:233` | Close → `os.Rename` to `<path>.<YYYYMMDD-HHMMSS>` → re-init; on rename failure tries to reopen the original |
| `formatAndLogWithoutLock(level Level, format string, args ...interface{})` | `logger.go:265` | Format + write without acquiring `mu`; used from `checkRotation`/`rotate` to avoid deadlock (those run with the lock held) |

## 3. `Config` — `type Config struct` (`logger.go:84`)

Configuration passed to `New`. **All fields exported; no methods attached.**

| Field | Type | Exported | Purpose |
|-------|------|----------|---------|
| `Level` | `Level` | yes | Minimum level to output; messages below are ignored |
| `FilePath` | `string` | yes | Log file path; empty disables file logging |
| `EnableStdout` | `bool` | yes | `false` = file-only output |
| `Prefix` | `string` | yes | Prefix added to every message |
| `MaxSize` | `int64` | yes | Rotation threshold in bytes; `<= 0` → 100 MB default |

## Package-level functions (not methods — listed for completeness)

| Signature | Line | Notes |
|-----------|------|-------|
| `ParseLevel(level string) Level` | `logger.go:52` | Case-insensitive name → `Level`; unknown input falls back to `InfoLevel` |
| `New(cfg Config) (*Logger, error)` | `logger.go:99` | Sole constructor; returns error only when `FilePath != ""` and the file can't be created/opened (wrapped with `%w`) |

## Method-set summary

| Type | Exported methods | Unexported methods | Total |
|------|------------------|--------------------|-------|
| `Level` (value receivers) | 1 (`String`) | 0 | 1 |
| `*Logger` (pointer receivers) | 12 | 8 | 20 |
| `Config` | 0 | 0 | 0 |

`Logger` implements no interfaces declared in this file and satisfies none
explicitly (no `io.Writer`/`io.Closer` assertions) — although its `Close` method
does give it an `io.Closer`-shaped method set structurally.

## Verification

Inventory cross-checked mechanically against the source rather than by eye:

- `grep -n "^func" logger.go` → 24 declarations (22 methods + `ParseLevel` + `New`); each listed above with its line.
- `grep -n "^type" logger.go` → exactly `Level` (16), `Logger` (71), `Config` (84).
- `grep -n "^type.*=" logger.go` → no matches (no aliases).
- Struct bodies read directly from `logger.go:71-95`.

## Context notes

- The sibling file `mothership/internal/logging/logger_test.go` (544 lines, 15 test
  functions) is out of scope for this extraction — it declares no new exported types.
- At extraction time this package was **untracked in git** while
  `mothership/cmd/mothership/main.go:39` imports it — i.e. the package's own
  publication is a separate deliverable from this documentation and was not part of
  this bead (read-only extraction only).
