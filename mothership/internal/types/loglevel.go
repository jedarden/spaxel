// Package types provides common type definitions used across the Spaxel system.
package types

// LogLevel represents the verbosity level for logging.
// Levels are ordered from most verbose (Trace) to least verbose (Error).
type LogLevel int8

const (
	// TraceLevel represents the most verbose logging level, including detailed
	// execution traces and fine-grained debugging information.
	TraceLevel LogLevel = iota

	// DebugLevel represents detailed debugging information for development
	// and troubleshooting.
	DebugLevel

	// InfoLevel represents general informational messages about normal operation.
	InfoLevel

	// WarnLevel represents warning messages for potentially harmful situations.
	WarnLevel

	// ErrorLevel represents error messages for critical issues that need attention.
	ErrorLevel
)

// String returns the string representation of the log level.
func (l LogLevel) String() string {
	switch l {
	case TraceLevel:
		return "trace"
	case DebugLevel:
		return "debug"
	case InfoLevel:
		return "info"
	case WarnLevel:
		return "warn"
	case ErrorLevel:
		return "error"
	default:
		return "unknown"
	}
}

// ParseLogLevel parses a string into a LogLevel.
// Returns the level and true if successful, zero value and false otherwise.
func ParseLogLevel(s string) (LogLevel, bool) {
	switch s {
	case "trace":
		return TraceLevel, true
	case "debug":
		return DebugLevel, true
	case "info":
		return InfoLevel, true
	case "warn", "warning":
		return WarnLevel, true
	case "error":
		return ErrorLevel, true
	default:
		return 0, false
	}
}
