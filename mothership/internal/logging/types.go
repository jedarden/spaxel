// types.go holds the Level severity domain for the logging package: the
// Level type, its five ordered constants, and the two Level-domain
// functions (String and ParseLevel), moved verbatim from logger.go per
// docs/notes/logging-types-extraction-plan.md. The file deliberately has
// no imports.

package logging

// Level represents the severity of a log message.
type Level int

const (
	// TraceLevel logs are extremely verbose and include detailed execution flow.
	// They are typically disabled in production and only used for deep debugging.
	TraceLevel Level = iota
	// DebugLevel logs are typically voluminous and are usually disabled in production.
	DebugLevel
	// InfoLevel is the default logging priority.
	InfoLevel
	// WarnLevel logs are more important than Info, but don't need individual human review.
	WarnLevel
	// ErrorLevel logs are high-priority. If an application is running smoothly, it shouldn't
	// generate any error-level logs.
	ErrorLevel
)

// String returns the string representation of the log level.
func (l Level) String() string {
	switch l {
	case TraceLevel:
		return "TRACE"
	case DebugLevel:
		return "DEBUG"
	case InfoLevel:
		return "INFO"
	case WarnLevel:
		return "WARN"
	case ErrorLevel:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// ParseLevel parses a string into a Level. Returns InfoLevel as default if parsing fails.
func ParseLevel(level string) Level {
	switch level {
	case "trace", "TRACE":
		return TraceLevel
	case "debug", "DEBUG":
		return DebugLevel
	case "info", "INFO":
		return InfoLevel
	case "warn", "WARN":
		return WarnLevel
	case "error", "ERROR":
		return ErrorLevel
	default:
		return InfoLevel
	}
}
