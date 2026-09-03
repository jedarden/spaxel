// Package logging provides a structured logging framework for the Spaxel mothership.
// It supports simultaneous output to file and stdout, with configurable log levels
// and proper error handling for file creation failures.
package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Logger is the main logging structure. It's thread-safe and can write to multiple
// outputs simultaneously.
type Logger struct {
	mu           sync.Mutex
	level        Level
	file         *os.File
	filePath     string
	enableStdout bool
	enableFile   bool
	prefix       string
	lastRotate   time.Time
	maxSize      int64 // Maximum file size in bytes before rotation
}

// Config holds the configuration for the logger.
type Config struct {
	// Level is the minimum log level to output. Messages below this level are ignored.
	Level Level
	// FilePath is the path to the log file. If empty, file logging is disabled.
	FilePath string
	// EnableStdout enables logging to stdout. If false, only file logging is used.
	EnableStdout bool
	// Prefix is a string prefix added to every log message.
	Prefix string
	// MaxSize is the maximum size in bytes before log rotation. Default is 100MB.
	MaxSize int64
}

// New creates a new Logger with the given configuration. It returns an error if
// file logging is enabled but the file cannot be created or opened.
func New(cfg Config) (*Logger, error) {
	logger := &Logger{
		level:        cfg.Level,
		enableStdout: cfg.EnableStdout,
		prefix:       cfg.Prefix,
		maxSize:      cfg.MaxSize,
	}

	// Default to 100MB if not specified
	if logger.maxSize <= 0 {
		logger.maxSize = 100 * 1024 * 1024
	}

	// Initialize file logging if path is provided
	if cfg.FilePath != "" {
		if err := logger.initFileLogging(cfg.FilePath); err != nil {
			return nil, fmt.Errorf("failed to initialize file logging: %w", err)
		}
		logger.enableFile = true
	}

	return logger, nil
}

// initFileLogging sets up file logging with proper error handling.
// It creates the log file and any necessary directories, with fallback strategies.
func (l *Logger) initFileLogging(filePath string) error {
	// Ensure the directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create log directory %s: %w", dir, err)
	}

	// Open the file in append mode, creating it if it doesn't exist
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file %s: %w", filePath, err)
	}

	l.file = file
	l.filePath = filePath
	l.lastRotate = time.Now()

	return nil
}

// Close closes the log file if it's open. It's safe to call multiple times.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file != nil {
		if err := l.file.Close(); err != nil {
			return err
		}
		l.file = nil
	}
	return nil
}

// shouldLog returns true if a message at the given level should be logged.
func (l *Logger) shouldLog(level Level) bool {
	return level >= l.level
}

// formatMessage formats a log message with timestamp, level, and prefix.
func (l *Logger) formatMessage(level Level, format string, args ...interface{}) string {
	timestamp := time.Now().Format("2006-01-02T15:04:05.000Z07:00")

	prefix := ""
	if l.prefix != "" {
		prefix = fmt.Sprintf("[%s] ", l.prefix)
	}

	message := fmt.Sprintf(format, args...)
	return fmt.Sprintf("%s [%s] %s%s\n", timestamp, level.String(), prefix, message)
}

// write outputs a message to the configured outputs.
func (l *Logger) write(message string) {
	// Write to stdout if enabled
	if l.enableStdout {
		fmt.Print(message)
	}

	// Write to file if enabled. A failed write is deliberately dropped: the
	// logger has no sink left to report it through, and escalating a log write
	// into a caller-visible error would change every call site's contract.
	if l.enableFile && l.file != nil {
		_, _ = l.file.WriteString(message)
	}
}

// log is the internal logging method used by all exported log functions.
func (l *Logger) log(level Level, format string, args ...interface{}) {
	if !l.shouldLog(level) {
		return
	}

	message := l.formatMessage(level, format, args...)

	l.mu.Lock()
	defer l.mu.Unlock()

	l.write(message)

	// Check for rotation need on error-level messages (less frequent check)
	if level == ErrorLevel {
		l.checkRotation()
	}
}

// checkRotation checks if the log file needs rotation and performs it if necessary.
// This is a no-op if file logging is disabled.
func (l *Logger) checkRotation() {
	if !l.enableFile || l.file == nil {
		return
	}

	// Get file info
	info, err := l.file.Stat()
	if err != nil {
		// If we can't stat the file, log but don't fail
		l.formatAndLogWithoutLock(WarnLevel, "Failed to stat log file for rotation check: %v", err)
		return
	}

	// Check if rotation is needed
	if info.Size() >= l.maxSize {
		if err := l.rotate(); err != nil {
			l.formatAndLogWithoutLock(ErrorLevel, "Failed to rotate log file: %v", err)
		}
	}
}

// rotate performs log rotation by renaming the current file and creating a new one.
func (l *Logger) rotate() error {
	if l.file == nil {
		return nil
	}

	// Close current file
	if err := l.file.Close(); err != nil {
		return fmt.Errorf("failed to close current log file: %w", err)
	}

	// Rename current file with timestamp
	timestamp := time.Now().Format("20060102-150405")
	rotatedPath := l.filePath + "." + timestamp
	if err := os.Rename(l.filePath, rotatedPath); err != nil {
		// Try to re-open the original file if rename fails
		if reopenErr := l.initFileLogging(l.filePath); reopenErr != nil {
			return fmt.Errorf("failed to rename log file (%v) and failed to re-open original (%w): %v",
				err, reopenErr, err)
		}
		return fmt.Errorf("failed to rename log file: %w", err)
	}

	// Create new file
	if err := l.initFileLogging(l.filePath); err != nil {
		return fmt.Errorf("failed to create new log file after rotation: %w", err)
	}

	return nil
}

// formatAndLogWithoutLock formats and logs a message without acquiring the lock.
// This is used internally to avoid deadlocks when already holding the lock.
func (l *Logger) formatAndLogWithoutLock(level Level, format string, args ...interface{}) {
	if !l.shouldLog(level) {
		return
	}
	message := l.formatMessage(level, format, args...)
	l.write(message)
}

// Trace logs a message at TraceLevel.
func (l *Logger) Trace(format string, args ...interface{}) {
	l.log(TraceLevel, format, args...)
}

// Debug logs a message at DebugLevel.
func (l *Logger) Debug(format string, args ...interface{}) {
	l.log(DebugLevel, format, args...)
}

// Info logs a message at InfoLevel.
func (l *Logger) Info(format string, args ...interface{}) {
	l.log(InfoLevel, format, args...)
}

// Warn logs a message at WarnLevel.
func (l *Logger) Warn(format string, args ...interface{}) {
	l.log(WarnLevel, format, args...)
}

// Error logs a message at ErrorLevel.
func (l *Logger) Error(format string, args ...interface{}) {
	l.log(ErrorLevel, format, args...)
}

// Fatal logs a message at ErrorLevel and then calls os.Exit(1).
// This should only be used for unrecoverable errors.
func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(ErrorLevel, format, args...)
	os.Exit(1)
}

// SetLevel changes the minimum log level.
func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

// SetFilePath changes the log file path and reinitializes file logging.
// Returns an error if the new file cannot be created.
func (l *Logger) SetFilePath(filePath string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// Close existing file if open
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}

	if filePath == "" {
		l.enableFile = false
		return nil
	}

	if err := l.initFileLogging(filePath); err != nil {
		// If we failed to set up the new file, disable file logging
		l.enableFile = false
		return err
	}

	l.enableFile = true
	return nil
}

// GetFilePath returns the current log file path.
func (l *Logger) GetFilePath() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.filePath
}

// IsFileEnabled returns true if file logging is currently enabled.
func (l *Logger) IsFileEnabled() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.enableFile
}

// IsStdoutEnabled returns true if stdout logging is currently enabled.
func (l *Logger) IsStdoutEnabled() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.enableStdout
}
