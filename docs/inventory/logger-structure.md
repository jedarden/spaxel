# Logger.go Structure Inventory

**Generated:** 2026-09-01  
**Source:** `mothership/internal/logging/logger.go`  
**Purpose:** Consolidated inventory of all types, constants, and interfaces

---

## Summary

| Category | Count |
|----------|-------|
| Types | 3 |
| Constants | 5 |
| Interfaces | 0 |
| **Total** | **8** |

---

## Types (3)

### 1. Level
- **Line:** 16
- **Definition:** `type Level int`
- **Description:** Represents the severity of a log message
- **Methods:**
  - `String() string` (line 34) - Returns string representation
  - `ParseLevel(level string) Level` (line 52) - Parses string into Level

### 2. Logger
- **Line:** 71
- **Definition:** `type Logger struct`
- **Description:** Main logging structure, thread-safe, writes to multiple outputs
- **Fields:**
  - `mu sync.Mutex` - Thread safety
  - `level Level` - Minimum log level
  - `file *os.File` - Log file handle
  - `filePath string` - Log file path
  - `enableStdout bool` - Stdout output flag
  - `enableFile bool` - File output flag
  - `prefix string` - Message prefix
  - `lastRotate time.Time` - Last rotation time
  - `maxSize int64` - Max file size before rotation
- **Key Methods:**
  - `New(cfg Config) (*Logger, error)` - Constructor
  - `Close() error` - Closes log file
  - `Trace/Debug/Info/Warn/Error/Fatal(format string, args ...interface{})` - Logging methods
  - `SetLevel(level Level)` - Changes log level
  - `SetFilePath(filePath string) error` - Changes log file path
  - `GetFilePath() string` - Returns current file path
  - `IsFileEnabled() bool` - File logging status
  - `IsStdoutEnabled() bool` - Stdout logging status

### 3. Config
- **Line:** 84
- **Definition:** `type Config struct`
- **Description:** Holds configuration for the logger
- **Fields:**
  - `Level Level` - Minimum log level to output
  - `FilePath string` - Path to log file
  - `EnableStdout bool` - Enable stdout logging
  - `Prefix string` - Message prefix
  - `MaxSize int64` - Max size before rotation (default 100MB)

---

## Constants (5)

All constants are of type `Level` and use iota for auto-incrementing values:

| Constant | Value | Line | Description |
|----------|-------|------|-------------|
| `TraceLevel` | 0 | 21 | Extremely verbose, detailed execution flow |
| `DebugLevel` | 1 | 22 | Voluminous, usually disabled in production |
| `InfoLevel` | 2 | 24 | Default logging priority |
| `WarnLevel` | 3 | 26 | More important than Info, no individual review needed |
| `ErrorLevel` | 4 | 28 | High-priority, indicates problems |

---

## Interfaces (0)

No interfaces are defined in this file.

---

## Additional Functions (Non-methods)

### ParseLevel
- **Line:** 52
- **Signature:** `func ParseLevel(level string) Level`
- **Description:** Parses a string into a Level, returns InfoLevel as default if parsing fails
- **Supported inputs:** "trace", "debug", "info", "warn", "error" (case-insensitive)

---

## Private Methods (Internal)

These methods are not exported but are part of the Logger implementation:

- `initFileLogging(filePath string) error` - Sets up file logging
- `shouldLog(level Level) bool` - Checks if message should be logged
- `formatMessage(level Level, format string, args ...interface{}) string` - Formats log message
- `write(message string)` - Outputs to configured destinations
- `log(level Level, format string, args ...interface{})` - Internal logging method
- `checkRotation()` - Checks if rotation is needed
- `rotate() error` - Performs log rotation
- `formatAndLogWithoutLock(level Level, format string, args ...interface{})` - Logs without lock (deadlock prevention)

---

## Exported API Summary

**Constructor:**
- `New(cfg Config) (*Logger, error)`

**Logging Methods:**
- `Trace(format string, args ...interface{})`
- `Debug(format string, args ...interface{})`
- `Info(format string, args ...interface{})`
- `Warn(format string, args ...interface{})`
- `Error(format string, args ...interface{})`
- `Fatal(format string, args ...interface{})` - Exits with code 1

**Configuration Methods:**
- `SetLevel(level Level)`
- `SetFilePath(filePath string) error`

**Query Methods:**
- `GetFilePath() string`
- `IsFileEnabled() bool`
- `IsStdoutEnabled() bool`

**Lifecycle:**
- `Close() error`

---

## Design Notes

1. **Thread Safety:** Logger uses `sync.Mutex` for concurrent access
2. **Dual Output:** Supports simultaneous stdout and file logging
3. **Log Rotation:** Automatic rotation based on file size (default 100MB)
4. **Error Handling:** Graceful fallback strategies for file operations
5. **Level Filtering:** Messages below configured level are ignored
6. **Timestamp Format:** ISO 8601 (`2006-01-02T15:04:05.000Z07:00`)
7. **Default Level:** InfoLevel used as safe default for parsing failures

---

*This inventory consolidates all structural elements of logger.go for reference and documentation purposes.*
