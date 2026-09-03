// Package logging tests the logging infrastructure.
package logging

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestLogger returns a logger writing only to a fresh file inside a
// per-test temp directory, plus that file's path. Capturing through a real
// file exercises the actual write path instead of stubbing the sink out.
func newTestLogger(t *testing.T, cfg Config) (*Logger, string) {
	t.Helper()

	logFile := filepath.Join(t.TempDir(), "spaxel-test.log")
	cfg.FilePath = logFile
	cfg.EnableStdout = false

	logger, err := New(cfg)
	if err != nil {
		t.Fatalf("New(%+v) returned error: %v", cfg, err)
	}
	t.Cleanup(func() { logger.Close() })

	return logger, logFile
}

// readLog returns the full contents of a log file.
func readLog(t *testing.T, path string) string {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("Failed to read log file %s: %v", path, err)
	}
	return string(content)
}

// blockingPath returns a log path whose parent component is a regular file,
// so creating the log directory there fails on every platform and under any
// user. /proc and /root based paths were rejected: they depend on the OS and
// on the test not running as root.
func blockingPath(t *testing.T) string {
	t.Helper()

	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("Failed to create blocker file: %v", err)
	}
	return filepath.Join(blocker, "test.log")
}

// TestNewLogger tests basic logger initialization.
func TestNewLogger(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectError bool
	}{
		{
			name: "stdout only",
			config: Config{
				Level:        InfoLevel,
				EnableStdout: true,
				FilePath:     "",
			},
			expectError: false,
		},
		{
			name: "file logging only",
			config: Config{
				Level:        WarnLevel,
				EnableStdout: false,
				FilePath:     filepath.Join(t.TempDir(), "file-only.log"),
			},
			expectError: false,
		},
		{
			name: "both file and stdout",
			config: Config{
				Level:        DebugLevel,
				EnableStdout: true,
				FilePath:     filepath.Join(t.TempDir(), "file-and-stdout.log"),
			},
			expectError: false,
		},
		{
			name: "unwritable file path",
			config: Config{
				Level:        InfoLevel,
				EnableStdout: true,
				FilePath:     blockingPath(t),
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := New(tt.config)

			if tt.expectError {
				if err == nil {
					logger.Close()
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if logger == nil {
				t.Errorf("Expected logger but got nil")
				return
			}

			logger.Close()
		})
	}
}

// TestLevelParsing tests the ParseLevel function.
func TestLevelParsing(t *testing.T) {
	tests := []struct {
		input    string
		expected Level
	}{
		{"debug", DebugLevel},
		{"DEBUG", DebugLevel},
		{"info", InfoLevel},
		{"INFO", InfoLevel},
		{"warn", WarnLevel},
		{"WARN", WarnLevel},
		{"error", ErrorLevel},
		{"ERROR", ErrorLevel},
		{"invalid", InfoLevel}, // Default to InfoLevel
		{"", InfoLevel},        // Default to InfoLevel
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := ParseLevel(tt.input)
			if result != tt.expected {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

// TestLevelString tests the Level.String() method.
func TestLevelString(t *testing.T) {
	tests := []struct {
		level    Level
		expected string
	}{
		{DebugLevel, "DEBUG"},
		{InfoLevel, "INFO"},
		{WarnLevel, "WARN"},
		{ErrorLevel, "ERROR"},
	}

	for _, tt := range tests {
		t.Run(tt.level.String(), func(t *testing.T) {
			result := tt.level.String()
			if result != tt.expected {
				t.Errorf("Level.String() = %s, want %s", result, tt.expected)
			}
		})
	}
}

// TestLogLevels tests that different log levels are respected.
func TestLogLevels(t *testing.T) {
	logger, logFile := newTestLogger(t, Config{Level: WarnLevel})

	// Messages below the configured level must not reach the sink at all.
	logger.Debug("debug message")
	logger.Info("info message")

	if content := readLog(t, logFile); content != "" {
		t.Errorf("Messages below the level should be suppressed when level is Warn, got: %q", content)
	}

	// Messages at or above the level must be written with their level tag.
	logger.Warn("warn message")
	logger.Error("error message")

	content := readLog(t, logFile)
	if !strings.Contains(content, "WARN") || !strings.Contains(content, "warn message") {
		t.Errorf("Warn message should be logged when level is Warn, got: %q", content)
	}
	if !strings.Contains(content, "ERROR") || !strings.Contains(content, "error message") {
		t.Errorf("Error message should be logged when level is Warn, got: %q", content)
	}
}

// TestFileLogging tests actual file output.
func TestFileLogging(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "spaxel-file.log")

	logger, err := New(Config{Level: InfoLevel, EnableStdout: false, FilePath: logFile})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	testMessage := "Test file logging message"
	logger.Info("%s", testMessage)

	content := readLog(t, logFile)

	if !strings.Contains(content, testMessage) {
		t.Errorf("Log file does not contain expected message. Got: %s", content)
	}

	if !strings.Contains(content, "INFO") {
		t.Errorf("Log file does not contain level INFO")
	}
}

// TestStdoutLogging tests stdout output.
func TestStdoutLogging(t *testing.T) {
	// Capture stdout, since the logger writes to it directly.
	originalStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Failed to create pipe: %v", err)
	}
	os.Stdout = w

	logger, err := New(Config{Level: InfoLevel, EnableStdout: true, FilePath: ""})
	if err != nil {
		os.Stdout = originalStdout
		t.Fatalf("Failed to create logger: %v", err)
	}

	testMessage := "Test stdout message"
	logger.Info("%s", testMessage)
	logger.Close()

	// Restore stdout before blocking on the read side.
	if err := w.Close(); err != nil {
		t.Errorf("Failed to close pipe writer: %v", err)
	}
	os.Stdout = originalStdout

	captured, err := io.ReadAll(r)
	if err != nil {
		t.Errorf("Failed to read captured stdout: %v", err)
	}
	output := string(captured)
	if !strings.Contains(output, testMessage) {
		t.Errorf("Stdout does not contain expected message. Got: %s", output)
	}
}

// TestLogRotation tests log file rotation.
//
// The logger evaluates the size limit when an error-level message is written
// (the deliberate "less frequent check"), so the load here is written at
// ErrorLevel to exercise the rotation path.
func TestLogRotation(t *testing.T) {
	logFile := filepath.Join(t.TempDir(), "spaxel-rotation.log")

	const maxSize = 1024
	logger, err := New(Config{
		Level:        InfoLevel,
		EnableStdout: false,
		FilePath:     logFile,
		MaxSize:      maxSize,
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Write enough data to push well past the rotation limit.
	message := strings.Repeat("x", 100)
	for i := 0; i < 100; i++ {
		logger.Error("%s", message)
	}

	// Rotated segments must exist and still hold what was written.
	matches, err := filepath.Glob(logFile + ".*")
	if err != nil {
		t.Fatalf("Failed to glob rotated log files: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("No rotated log files found")
	}

	rotated := readLog(t, matches[0])
	if !strings.Contains(rotated, message) {
		t.Errorf("Rotated log file %s does not contain the logged messages", matches[0])
	}

	// The active file must have restarted below the limit.
	info, err := os.Stat(logFile)
	if err != nil {
		t.Fatalf("Failed to stat log file: %v", err)
	}
	if info.Size() >= maxSize {
		t.Errorf("Log file size %d >= max size %d, rotation may not have worked", info.Size(), maxSize)
	}
}

// TestConcurrentLogging tests thread-safe concurrent logging.
func TestConcurrentLogging(t *testing.T) {
	logger, logFile := newTestLogger(t, Config{Level: InfoLevel})

	// Launch multiple goroutines logging simultaneously
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				logger.Info("Goroutine %d, message %d", id, j)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Count how many INFO lines we got
	lines := strings.Count(readLog(t, logFile), "INFO")
	expectedLines := 10 * 100 // 10 goroutines * 100 messages
	if lines < expectedLines {
		t.Errorf("Expected %d INFO lines, got %d", expectedLines, lines)
	}
}

// TestPrefix tests message prefix functionality.
func TestPrefix(t *testing.T) {
	logger, logFile := newTestLogger(t, Config{Level: InfoLevel, Prefix: "TEST"})

	testMessage := "Test message"
	logger.Info("%s", testMessage)

	output := readLog(t, logFile)
	if !strings.Contains(output, "[TEST]") {
		t.Errorf("Output does not contain prefix [TEST]. Got: %s", output)
	}
	if !strings.Contains(output, testMessage) {
		t.Errorf("Output does not contain expected message. Got: %s", output)
	}
}

// TestSetLevel tests changing log level dynamically.
func TestSetLevel(t *testing.T) {
	logger, logFile := newTestLogger(t, Config{Level: InfoLevel})

	// Initially, debug should be suppressed
	logger.Debug("debug message")
	if content := readLog(t, logFile); content != "" {
		t.Errorf("Debug should be suppressed at InfoLevel, got: %q", content)
	}

	// Change to debug level
	logger.SetLevel(DebugLevel)

	// Now debug should be logged
	logger.Debug("debug message")
	if content := readLog(t, logFile); !strings.Contains(content, "DEBUG") {
		t.Errorf("Debug should be logged after level change, got: %q", content)
	}
}

// TestSetFilePath tests changing log file path.
func TestSetFilePath(t *testing.T) {
	dir := t.TempDir()
	initialFile := filepath.Join(dir, "initial.log")
	newFile := filepath.Join(dir, "new.log")

	logger, err := New(Config{Level: InfoLevel, EnableStdout: false, FilePath: initialFile})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Verify initial file path
	if logger.GetFilePath() != initialFile {
		t.Errorf("Initial file path not set correctly: got %q, want %q", logger.GetFilePath(), initialFile)
	}

	// Change to new file path
	if err := logger.SetFilePath(newFile); err != nil {
		t.Fatalf("Failed to set file path: %v", err)
	}

	if logger.GetFilePath() != newFile {
		t.Errorf("File path not updated correctly: got %q, want %q", logger.GetFilePath(), newFile)
	}

	// Log to new file
	testMessage := "Message after path change"
	logger.Info("%s", testMessage)

	if content := readLog(t, newFile); !strings.Contains(content, testMessage) {
		t.Errorf("New log file does not contain expected message, got: %q", content)
	}
}

// TestSetFilePathEmpty tests that clearing the path disables file logging.
func TestSetFilePathEmpty(t *testing.T) {
	logger, logFile := newTestLogger(t, Config{Level: InfoLevel})

	if !logger.IsFileEnabled() {
		t.Fatal("File logging should be enabled before clearing the path")
	}

	if err := logger.SetFilePath(""); err != nil {
		t.Fatalf("Failed to clear file path: %v", err)
	}

	if logger.IsFileEnabled() {
		t.Error("File logging should be disabled after clearing the path")
	}
	// SetFilePath("") disables file logging but deliberately leaves the recorded
	// path in place; the path is historical once file logging is off.
	if got := logger.GetFilePath(); got != logFile {
		t.Errorf("GetFilePath() = %q after clearing, want the path that was set (%q)", got, logFile)
	}
}

// TestIsFileEnabled tests file logging state queries.
func TestIsFileEnabled(t *testing.T) {
	loggerWithFile, _ := newTestLogger(t, Config{Level: InfoLevel})
	if !loggerWithFile.IsFileEnabled() {
		t.Error("File logging should be enabled")
	}

	loggerWithoutFile, err := New(Config{Level: InfoLevel, EnableStdout: true})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer loggerWithoutFile.Close()

	if loggerWithoutFile.IsFileEnabled() {
		t.Error("File logging should be disabled")
	}
}

// TestIsStdoutEnabled tests stdout logging state queries.
func TestIsStdoutEnabled(t *testing.T) {
	loggerWithStdout, err := New(Config{Level: InfoLevel, EnableStdout: true})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer loggerWithStdout.Close()

	if !loggerWithStdout.IsStdoutEnabled() {
		t.Error("Stdout logging should be enabled")
	}

	loggerWithoutStdout, _ := newTestLogger(t, Config{Level: InfoLevel})
	if loggerWithoutStdout.IsStdoutEnabled() {
		t.Error("Stdout logging should be disabled")
	}
}

// TestCloseIdempotent tests that Close can be called multiple times safely.
func TestCloseIdempotent(t *testing.T) {
	logger, _ := newTestLogger(t, Config{Level: InfoLevel})

	// Close should not error
	if err := logger.Close(); err != nil {
		t.Errorf("First Close failed: %v", err)
	}

	// Second close should also not error
	if err := logger.Close(); err != nil {
		t.Errorf("Second Close failed: %v", err)
	}
}

// TestErrorHandling tests error handling for file operations.
func TestErrorHandling(t *testing.T) {
	// A regular file used as a parent directory makes creation fail
	// deterministically, regardless of platform or user.
	config := Config{
		Level:        InfoLevel,
		EnableStdout: false,
		FilePath:     blockingPath(t),
	}

	logger, err := New(config)
	if err == nil {
		logger.Close()
		t.Error("Expected error for invalid file path")
	}
}
