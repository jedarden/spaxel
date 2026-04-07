// Package shutdown provides tests for the graceful shutdown sequence.
package shutdown

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// mockBaselineFlusher implements BaselineFlusher.
type mockBaselineFlusher struct {
	called bool
	err    error
}

func (m *mockBaselineFlusher) FlushBaselines(ctx context.Context) error {
	m.called = true
	return m.err
}

// mockRecordingSyncer implements RecordingSyncer.
type mockRecordingSyncer struct {
	called bool
	err    error
}

func (m *mockRecordingSyncer) Sync(ctx context.Context) error {
	m.called = true
	return m.err
}

// mockDashboardBroadcaster implements DashboardBroadcaster.
type mockDashboardBroadcaster struct {
	called bool
	msg    ShutdownMessage
}

func (m *mockDashboardBroadcaster) BroadcastShutdown(msg ShutdownMessage) {
	m.called = true
	m.msg = msg
}

// mockNodeConnectionCloser implements NodeConnectionCloser.
type mockNodeConnectionCloser struct {
	called bool
	err    error
}

func (m *mockNodeConnectionCloser) CloseAllConnections() error {
	m.called = true
	return m.err
}

// mockEventWriter implements EventWriter.
type mockEventWriter struct {
	called bool
	err    error
}

func (m *mockEventWriter) WriteSystemStoppedEvent() error {
	m.called = true
	return m.err
}

// mockIngestionShutdowner implements IngestionShutdowner.
type mockIngestionShutdowner struct {
	called bool
}

func (m *mockIngestionShutdowner) SetShuttingDown() {
	m.called = true
}

// TestShutdown_AllSteps tests that all shutdown steps are executed.
func TestShutdown_AllSteps(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer db.Close()

	// Create events table for the test
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp_ms INTEGER NOT NULL,
			type TEXT NOT NULL,
			zone TEXT,
			person TEXT,
			blob_id INTEGER,
			detail_json TEXT,
			severity TEXT NOT NULL DEFAULT 'info'
		)
	`)
	if err != nil {
		t.Fatalf("Failed to create events table: %v", err)
	}

	mockBaseline := &mockBaselineFlusher{}
	mockRecording := &mockRecordingSyncer{}
	mockDashboard := &mockDashboardBroadcaster{}
	mockNodeCloser := &mockNodeConnectionCloser{}
	mockEventWriter := &mockEventWriter{err: nil} // Will write to DB
	mockIngestion := &mockIngestionShutdowner{}

	// Create event writer that actually writes to the test database
	eventWriter := &testEventWriter{db: db}

	manager := NewManager(db)
	manager.SetBaselineFlusher(mockBaseline)
	manager.SetRecordingSyncer(mockRecording)
	manager.SetDashboardBroadcaster(mockDashboard)
	manager.SetNodeCloser(mockNodeCloser)
	manager.SetEventWriter(eventWriter)
	manager.SetIngestionShutdowner(mockIngestion)

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	// Run shutdown
	completed := manager.Shutdown(ctx, cancel)

	// Verify all steps were called
	if !mockIngestion.called {
		t.Error("Ingestion shutdowner not called")
	}
	if !mockDashboard.called {
		t.Error("Dashboard broadcaster not called")
	}
	if mockDashboard.msg.Type != "shutdown" {
		t.Errorf("Expected shutdown message type 'shutdown', got '%s'", mockDashboard.msg.Type)
	}
	if mockDashboard.msg.ReconnectInMS != 30000 {
		t.Errorf("Expected reconnect_in_ms 30000, got %d", mockDashboard.msg.ReconnectInMS)
	}
	if !mockBaseline.called {
		t.Error("Baseline flusher not called")
	}
	if !mockRecording.called {
		t.Error("Recording syncer not called")
	}
	if !mockNodeCloser.called {
		t.Error("Node connection closer not called")
	}

	// Verify event was written
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM events WHERE type = 'system'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query events: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 system event, got %d", count)
	}

	if !completed {
		t.Error("Expected shutdown to complete within deadline")
	}
}

// TestShutdown_WithErrors tests that shutdown continues even when steps fail.
func TestShutdown_WithErrors(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	defer db.Close()

	mockBaseline := &mockBaselineFlusher{err: context.DeadlineExceeded}
	mockRecording := &mockRecordingSyncer{err: context.DeadlineExceeded}
	mockDashboard := &mockDashboardBroadcaster{}
	mockNodeCloser := &mockNodeConnectionCloser{err: context.DeadlineExceeded}
	mockEventWriter := &mockEventWriter{err: context.DeadlineExceeded}
	mockIngestion := &mockIngestionShutdowner{}

	manager := NewManager(db)
	manager.SetBaselineFlusher(mockBaseline)
	manager.SetRecordingSyncer(mockRecording)
	manager.SetDashboardBroadcaster(mockDashboard)
	manager.SetNodeCloser(mockNodeCloser)
	manager.SetEventWriter(mockEventWriter)
	manager.SetIngestionShutdowner(mockIngestion)

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	// Shutdown should complete despite errors
	completed := manager.Shutdown(ctx, cancel)

	// All steps should still have been attempted
	if !mockBaseline.called {
		t.Error("Baseline flusher not called despite error")
	}
	if !mockRecording.called {
		t.Error("Recording syncer not called despite error")
	}
	if !mockNodeCloser.called {
		t.Error("Node connection closer not called despite error")
	}
	if !mockEventWriter.called {
		t.Error("Event writer not called despite error")
	}

	// Should complete within deadline
	if !completed {
		t.Error("Expected shutdown to complete within deadline despite errors")
	}
}

// testEventWriter is an EventWriter that writes to the test database.
type testEventWriter struct {
	db *sql.DB
}

func (w *testEventWriter) WriteSystemStoppedEvent() error {
	detailJSON := `{"description":"Mothership stopped"}`
	_, err := w.db.Exec(`
		INSERT INTO events (timestamp_ms, type, zone, person, blob_id, detail_json, severity)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, time.Now().UnixNano()/1e6, "system", "", "", 0, detailJSON, "info")
	return err
}
