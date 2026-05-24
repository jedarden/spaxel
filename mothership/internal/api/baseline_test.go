// Package api provides tests for baseline API handlers.
package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestBaselineHandler_ListBaselines tests the GET /api/baseline endpoint.
func TestBaselineHandler_ListBaselines(t *testing.T) {
	t.Run("empty database returns empty list", func(t *testing.T) {
		db := setupBaselineTestDB(t)
		defer db.Close()
		handler := NewBaselineHandler(db)

		req := httptest.NewRequest(http.MethodGet, "/api/baseline", nil)
		w := httptest.NewRecorder()

		handler.listBaselines(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var resp []BaselineEntry
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(resp) != 0 {
			t.Errorf("expected empty list, got %d entries", len(resp))
		}
	})

	t.Run("returns most recent baseline per link", func(t *testing.T) {
		db := setupBaselineTestDB(t)
		defer db.Close()
		handler := NewBaselineHandler(db)

		// Insert test baselines - multiple snapshots for same link
		now := int64(1712345678000) // 2024-04-04 12:34:38 UTC
		baselines := []struct {
			linkID     string
			capturedAt int64
			confidence float64
			nSub       int
			amplitude  []float32
			phase      []float32
		}{
			{"AA:BB:CC:DD:EE:FF", now - 10000, 0.8, 64, []float32{1.0, 2.0}, []float32{0.1, 0.2}},
			{"AA:BB:CC:DD:EE:FF", now, 0.9, 64, []float32{1.1, 2.1}, []float32{0.11, 0.21}}, // Most recent
			{"11:22:33:44:55:66", now - 5000, 0.7, 64, []float32{0.5, 1.5}, []float32{0.05, 0.15}},
		}

		for _, b := range baselines {
			_, err := db.Exec(`
				INSERT INTO baselines (link_id, captured_at, n_sub, amplitude, phase, confidence)
				VALUES (?, ?, ?, ?, ?, ?)
			`, b.linkID, b.capturedAt, b.nSub, float32SliceToBytes(b.amplitude), float32SliceToBytes(b.phase), b.confidence)
			if err != nil {
				t.Fatalf("failed to insert baseline: %v", err)
			}
		}

		req := httptest.NewRequest(http.MethodGet, "/api/baseline", nil)
		w := httptest.NewRecorder()

		handler.listBaselines(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var resp []BaselineEntry
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(resp) != 2 {
			t.Errorf("expected 2 baseline entries (one per link), got %d", len(resp))
		}

		// Verify the most recent snapshot is returned for the first link
		var firstLink *BaselineEntry
		for _, b := range resp {
			if b.LinkID == "AA:BB:CC:DD:EE:FF" {
				firstLink = &b
				break
			}
		}
		if firstLink == nil {
			t.Fatal("first link not found in response")
		}

		if firstLink.SnapshotTime != now {
			t.Errorf("expected most recent snapshot time %d, got %d", now, firstLink.SnapshotTime)
		}

		if firstLink.Confidence != 0.9 {
			t.Errorf("expected confidence 0.9, got %f", firstLink.Confidence)
		}
	})
}

// TestBaselineHandler_CaptureBaseline tests the POST /api/baseline/capture endpoint.
func TestBaselineHandler_CaptureBaseline(t *testing.T) {
	t.Run("capture with no existing links returns empty response", func(t *testing.T) {
		db := setupBaselineTestDB(t)
		defer db.Close()
		handler := NewBaselineHandler(db)

		req := httptest.NewRequest(http.MethodPost, "/api/baseline/capture", nil)
		w := httptest.NewRecorder()

		handler.captureBaseline(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		var resp captureResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if !resp.OK {
			t.Errorf("expected ok=true, got %v", resp.OK)
		}

		if resp.LinksCaptured != 0 {
			t.Errorf("expected 0 links captured, got %d", resp.LinksCaptured)
		}

		if resp.Message == "" {
			t.Errorf("expected message about no links found")
		}
	})

	t.Run("capture all links when no specific links requested", func(t *testing.T) {
		db := setupBaselineTestDB(t)
		defer db.Close()
		handler := NewBaselineHandler(db)

		// Insert test baselines
		now := int64(1712345678000)
		_, err := db.Exec(`
			INSERT INTO baselines (link_id, captured_at, n_sub, amplitude, phase, confidence)
			VALUES (?, ?, ?, ?, ?, ?)
		`, "AA:BB:CC:DD:EE:FF", now, 64, float32SliceToBytes([]float32{1.0}), float32SliceToBytes([]float32{0.1}), 0.8)
		if err != nil {
			t.Fatalf("failed to insert baseline: %v", err)
		}

		req := httptest.NewRequest(http.MethodPost, "/api/baseline/capture", nil)
		w := httptest.NewRecorder()

		handler.captureBaseline(w, req)

		if w.Code != http.StatusAccepted {
			t.Errorf("expected status 202, got %d", w.Code)
		}

		var resp captureResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if !resp.OK {
			t.Errorf("expected ok=true, got %v", resp.OK)
		}

		if resp.LinksCaptured != 1 {
			t.Errorf("expected 1 link captured, got %d", resp.LinksCaptured)
		}

		if len(resp.Links) != 1 || resp.Links[0] != "AA:BB:CC:DD:EE:FF" {
			t.Errorf("expected links=[AA:BB:CC:DD:EE:FF], got %v", resp.Links)
		}

		// Verify capture marker was inserted
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM baselines WHERE link_id = ? AND amplitude = X'' AND phase = X''", "AA:BB:CC:DD:EE:FF").Scan(&count)
		if err != nil {
			t.Fatalf("failed to query capture marker: %v", err)
		}
		if count != 1 {
			t.Errorf("expected 1 capture marker, got %d", count)
		}
	})

	t.Run("capture specific links when requested", func(t *testing.T) {
		db := setupBaselineTestDB(t)
		defer db.Close()
		handler := NewBaselineHandler(db)

		// Insert test baselines for two links
		now := int64(1712345678000)
		for _, linkID := range []string{"AA:BB:CC:DD:EE:FF", "11:22:33:44:55:66"} {
			_, err := db.Exec(`
				INSERT INTO baselines (link_id, captured_at, n_sub, amplitude, phase, confidence)
				VALUES (?, ?, ?, ?, ?, ?)
			`, linkID, now, 64, float32SliceToBytes([]float32{1.0}), float32SliceToBytes([]float32{0.1}), 0.8)
			if err != nil {
				t.Fatalf("failed to insert baseline: %v", err)
			}
		}

		// Request capture for only the first link
		body := `{"links": ["AA:BB:CC:DD:EE:FF"]}`
		req := httptest.NewRequest(http.MethodPost, "/api/baseline/capture", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.captureBaseline(w, req)

		if w.Code != http.StatusAccepted {
			t.Errorf("expected status 202, got %d", w.Code)
		}

		var resp captureResponse
		if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if resp.LinksCaptured != 1 {
			t.Errorf("expected 1 link captured, got %d", resp.LinksCaptured)
		}

		if len(resp.Links) != 1 || resp.Links[0] != "AA:BB:CC:DD:EE:FF" {
			t.Errorf("expected links=[AA:BB:CC:DD:EE:FF], got %v", resp.Links)
		}
	})

	t.Run("invalid request body is rejected", func(t *testing.T) {
		db := setupBaselineTestDB(t)
		defer db.Close()
		handler := NewBaselineHandler(db)

		req := httptest.NewRequest(http.MethodPost, "/api/baseline/capture", bytes.NewBufferString("invalid json"))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		handler.captureBaseline(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("expected status 400, got %d", w.Code)
		}
	})
}

// setupBaselineTestDB creates an in-memory SQLite database with the baselines table.
func setupBaselineTestDB(t *testing.T) *sql.DB {
	t.Helper()

	// Create a temporary directory for the database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Open database with WAL mode
	dsn := dbPath + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("failed to open test database: %v", err)
	}

	// Create the baselines table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS baselines (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			link_id     TEXT NOT NULL,
			captured_at INTEGER NOT NULL,
			n_sub       INTEGER NOT NULL,
			amplitude   BLOB NOT NULL,
			phase       BLOB NOT NULL,
			confidence  REAL NOT NULL DEFAULT 0
		);
		CREATE INDEX IF NOT EXISTS idx_baselines_link ON baselines(link_id, captured_at DESC);
	`)
	if err != nil {
		db.Close()
		t.Fatalf("failed to create baselines table: %v", err)
	}

	// Run checkpoint to finalize WAL
	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		db.Close()
		t.Fatalf("failed to checkpoint: %v", err)
	}

	return db
}

// float32SliceToBytes converts a []float32 to a byte slice for BLOB storage.
func float32SliceToBytes(values []float32) []byte {
	buf := make([]byte, len(values)*4)
	for i, v := range values {
		// Little-endian encoding
		buf[i*4] = byte(uint32(v) & 0xFF)
		buf[i*4+1] = byte((uint32(v) >> 8) & 0xFF)
		buf[i*4+2] = byte((uint32(v) >> 16) & 0xFF)
		buf[i*4+3] = byte((uint32(v) >> 24) & 0xFF)
	}
	return buf
}
