package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/spaxel/mothership/internal/replay"
)

// mockRecordingStore is a mock implementation of FrameReader for testing.
type mockRecordingStore struct {
	stats        replay.Stats
	scanFunc     func(fn func(recvTimeNS int64, frame []byte) bool) error
	scanRangeFunc func(fromNS, toNS int64, fn func(recvTimeNS int64, frame []byte) bool) error
	closed       bool
	closeErr     error
}

func (m *mockRecordingStore) Stats() replay.Stats {
	return m.stats
}

func (m *mockRecordingStore) Scan(fn func(recvTimeNS int64, frame []byte) bool) error {
	if m.scanFunc != nil {
		return m.scanFunc(fn)
	}
	// Default: just call the function once and return nil
	fn(0, []byte("test"))
	return nil
}

func (m *mockRecordingStore) ScanRange(fromNS, toNS int64, fn func(recvTimeNS int64, frame []byte) bool) error {
	if m.scanRangeFunc != nil {
		return m.scanRangeFunc(fromNS, toNS, fn)
	}
	// Default: call Scan with the function
	return m.Scan(func(recvTimeNS int64, frame []byte) bool {
		return fn(recvTimeNS, frame)
	})
}

func (m *mockRecordingStore) Close() error {
	m.closed = true
	if m.closeErr != nil {
		return m.closeErr
	}
	return nil
}

// newTestReplayHandler creates a ReplayHandler with a mock store.
func newTestReplayHandler(t *testing.T, hasData bool) *ReplayHandler {
	t.Helper()

	store := &mockRecordingStore{
		stats: replay.Stats{
			HasData:   hasData,
			WritePos:  5000,
			OldestPos: 32,
			FileSize:  360 * 1024 * 1024,
		},
		scanFunc: func(fn func(recvTimeNS int64, frame []byte) bool) error {
			// Simulate some frames at different timestamps
			timestamps := []int64{
				1710450000000000000, // 2024-03-14 12:00:00
				1710450030000000000, // 2024-03-14 12:00:30
				1710450060000000000, // 2024-03-14 12:01:00
			}
			frames := [][]byte{
				[]byte("frame1"),
				[]byte("frame2"),
				[]byte("frame3"),
			}
			for i, ts := range timestamps {
				if !fn(ts, frames[i]) {
					break
				}
			}
			return nil
		},
	}

	handler, err := NewReplayHandler(store)
	if err != nil {
		t.Fatalf("NewReplayHandler: %v", err)
	}
	return handler
}

// setupReplayRouter creates a chi.Router with replay routes registered.
func setupReplayRouter(h *ReplayHandler) *chi.Mux {
	r := chi.NewRouter()
	h.RegisterRoutes(r)
	return r
}

// TestListSessions tests GET /api/replay/sessions.
func TestListSessions(t *testing.T) {
	tests := []struct {
		name       string
		hasData    bool
		wantStatus int
		check      func(*testing.T, map[string]interface{})
	}{
		{
			name:       "list sessions with data",
			hasData:    true,
			wantStatus: http.StatusOK,
			check: func(t *testing.T, resp map[string]interface{}) {
				if resp["has_data"] != true {
					t.Errorf("Expected has_data=true, got %v", resp["has_data"])
				}
				if fileSize, ok := resp["file_size_mb"].(float64); !ok || fileSize == 0 {
					t.Errorf("Expected non-zero file_size_mb, got %v", resp["file_size_mb"])
				}
				if oldestTS, ok := resp["oldest_timestamp_ms"].(float64); !ok || oldestTS == 0 {
					t.Errorf("Expected non-zero oldest_timestamp_ms, got %v", resp["oldest_timestamp_ms"])
				}
				if newestTS, ok := resp["newest_timestamp_ms"].(float64); !ok || newestTS == 0 {
					t.Errorf("Expected non-zero newest_timestamp_ms, got %v", resp["newest_timestamp_ms"])
				}
				sessions, ok := resp["sessions"].([]interface{})
				if !ok {
					t.Errorf("Expected sessions array, got %T", resp["sessions"])
				}
				// Empty sessions list initially
				if len(sessions) != 0 {
					t.Errorf("Expected 0 sessions, got %d", len(sessions))
				}
			},
		},
		{
			name:       "list sessions with no data",
			hasData:    false,
			wantStatus: http.StatusOK,
			check: func(t *testing.T, resp map[string]interface{}) {
				if resp["has_data"] != false {
					t.Errorf("Expected has_data=false, got %v", resp["has_data"])
				}
				if oldestTS, ok := resp["oldest_timestamp_ms"].(float64); ok && oldestTS != 0 {
					t.Errorf("Expected zero oldest_timestamp_ms when no data, got %v", oldestTS)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := newTestReplayHandler(t, tt.hasData)

			r := setupReplayRouter(handler)
			req := httptest.NewRequest("GET", "/api/replay/sessions", nil)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("Expected status %d, got %d: %s", tt.wantStatus, rr.Code, rr.Body.String())
			}

			var resp map[string]interface{}
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if tt.check != nil {
				tt.check(t, resp)
			}
		})
	}
}

// TestStartSession tests POST /api/replay/start.
func TestStartSession(t *testing.T) {
	pastTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano)

	tests := []struct {
		name       string
		body       startSessionRequest
		wantStatus int
		check      func(*testing.T, map[string]interface{})
	}{
		{
			name: "start session with valid range",
			body: startSessionRequest{
				FromISO8601: pastTime,
				ToISO8601:   "",
				Speed:       1,
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, resp map[string]interface{}) {
				sessionID, ok := resp["session_id"].(string)
				if !ok || sessionID == "" {
					t.Errorf("Expected non-empty session_id, got %v", resp["session_id"])
				}
				if fromMS, ok := resp["from_ms"].(float64); !ok || fromMS == 0 {
					t.Errorf("Expected non-zero from_ms, got %v", resp["from_ms"])
				}
				if toMS, ok := resp["to_ms"].(float64); !ok || toMS == 0 {
					t.Errorf("Expected non-zero to_ms, got %v", resp["to_ms"])
				}
				if state, ok := resp["state"].(string); !ok || state != "paused" {
					t.Errorf("Expected state=paused, got %v", resp["state"])
				}
			},
		},
		{
			name: "start session with explicit to time",
			body: startSessionRequest{
				FromISO8601: time.Now().Add(-2 * time.Hour).Format(time.RFC3339Nano),
				ToISO8601:   time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano),
				Speed:       2,
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, resp map[string]interface{}) {
				if speed, ok := resp["speed"].(float64); !ok || speed != 2 {
					t.Errorf("Expected speed=2, got %v", resp["speed"])
				}
			},
		},
		{
			name: "start session with speed 5",
			body: startSessionRequest{
				FromISO8601: pastTime,
				Speed:       5,
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, resp map[string]interface{}) {
				if speed, ok := resp["speed"].(float64); !ok || speed != 5 {
					t.Errorf("Expected speed=5, got %v", resp["speed"])
				}
			},
		},
		{
			name: "default speed when not specified",
			body: startSessionRequest{
				FromISO8601: pastTime,
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, resp map[string]interface{}) {
				if speed, ok := resp["speed"].(float64); !ok || speed != 1 {
					t.Errorf("Expected default speed=1, got %v", resp["speed"])
				}
			},
		},
		{
			name: "invalid from timestamp",
			body: startSessionRequest{
				FromISO8601: "invalid-timestamp",
			},
			wantStatus: http.StatusBadRequest,
			check: func(t *testing.T, resp map[string]interface{}) {
				if _, ok := resp["error"]; !ok {
					t.Error("Expected error in response")
				}
			},
		},
		{
			name: "invalid to timestamp",
			body: startSessionRequest{
				FromISO8601: pastTime,
				ToISO8601:   "not-a-timestamp",
			},
			wantStatus: http.StatusBadRequest,
			check: func(t *testing.T, resp map[string]interface{}) {
				if _, ok := resp["error"]; !ok {
					t.Error("Expected error in response")
				}
			},
		},
		{
			name: "to before from",
			body: startSessionRequest{
				FromISO8601: time.Now().Format(time.RFC3339Nano),
				ToISO8601:   time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano),
			},
			wantStatus: http.StatusBadRequest,
			check: func(t *testing.T, resp map[string]interface{}) {
				if _, ok := resp["error"]; !ok {
					t.Error("Expected error in response")
				}
			},
		},
		{
			name: "invalid speed",
			body: startSessionRequest{
				FromISO8601: pastTime,
				Speed:       3,
			},
			wantStatus: http.StatusBadRequest,
			check: func(t *testing.T, resp map[string]interface{}) {
				if _, ok := resp["error"]; !ok {
					t.Error("Expected error in response")
				}
			},
		},
		{
			name:       "empty body",
			body:       startSessionRequest{},
			wantStatus: http.StatusBadRequest,
			check: func(t *testing.T, resp map[string]interface{}) {
				if _, ok := resp["error"]; !ok {
					t.Error("Expected error in response")
				}
			},
		},
		{
			name:       "malformed JSON",
			body:       startSessionRequest{},
			wantStatus: http.StatusBadRequest,
			check: func(t *testing.T, resp map[string]interface{}) {
				if _, ok := resp["error"]; !ok {
					t.Error("Expected error in response")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := newTestReplayHandler(t, true)
			r := setupReplayRouter(handler)

			var body []byte
			var err error
			if tt.name == "malformed JSON" {
				body = []byte(`{invalid json}`)
			} else if tt.name == "empty body" {
				body = []byte(``)
			} else {
				body, err = json.Marshal(tt.body)
				if err != nil {
					t.Fatalf("Failed to marshal request: %v", err)
				}
			}

			req := httptest.NewRequest("POST", "/api/replay/start", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("Expected status %d, got %d: %s", tt.wantStatus, rr.Code, rr.Body.String())
			}

			var resp map[string]interface{}
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if tt.check != nil {
				tt.check(t, resp)
			}
		})
	}
}

// TestStopSession tests POST /api/replay/stop.
func TestStopSession(t *testing.T) {
	pastTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano)

	tests := []struct {
		name       string
		setup      func(*ReplayHandler) string
		body       stopSessionRequest
		wantStatus int
		check      func(*testing.T, *ReplayHandler, map[string]interface{})
	}{
		{
			name: "stop existing session",
			setup: func(h *ReplayHandler) string {
				// Create a session first
				body, _ := json.Marshal(startSessionRequest{
					FromISO8601: pastTime,
					Speed:       1,
				})
				r := setupReplayRouter(h)
				req := httptest.NewRequest("POST", "/api/replay/start", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				rr := httptest.NewRecorder()
				r.ServeHTTP(rr, req)

				var resp map[string]interface{}
				json.NewDecoder(rr.Body).Decode(&resp)
				return resp["session_id"].(string)
			},
			body:       stopSessionRequest{SessionID: "replay-1"},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, h *ReplayHandler, resp map[string]interface{}) {
				if resp["status"] != "stopped" {
					t.Errorf("Expected status=stopped, got %v", resp["status"])
				}
				// Verify session is removed
				h.mu.RLock()
				_, exists := h.sessions["replay-1"]
				h.mu.RUnlock()
				if exists {
					t.Error("Session should be removed after stop")
				}
			},
		},
		{
			name:  "stop nonexistent session",
			setup: func(h *ReplayHandler) string { return "" },
			body: stopSessionRequest{
				SessionID: "does-not-exist",
			},
			wantStatus: http.StatusNotFound,
			check: func(t *testing.T, h *ReplayHandler, resp map[string]interface{}) {
				if _, ok := resp["error"]; !ok {
					t.Error("Expected error in response")
				}
			},
		},
		{
			name:       "empty session_id",
			setup:      func(h *ReplayHandler) string { return "" },
			body:       stopSessionRequest{},
			wantStatus: http.StatusNotFound,
			check: func(t *testing.T, h *ReplayHandler, resp map[string]interface{}) {
				if _, ok := resp["error"]; !ok {
					t.Error("Expected error in response")
				}
			},
		},
		{
			name:       "malformed JSON",
			setup:      func(h *ReplayHandler) string { return "" },
			body:       stopSessionRequest{},
			wantStatus: http.StatusBadRequest,
			check: func(t *testing.T, h *ReplayHandler, resp map[string]interface{}) {
				if _, ok := resp["error"]; !ok {
					t.Error("Expected error in response")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := newTestReplayHandler(t, true)

			// For the "malformed JSON" test, we need special handling
			if tt.name == "malformed JSON" {
				r := setupReplayRouter(handler)
				req := httptest.NewRequest("POST", "/api/replay/stop", bytes.NewReader([]byte(`{invalid}`)))
				req.Header.Set("Content-Type", "application/json")
				rr := httptest.NewRecorder()
				r.ServeHTTP(rr, req)

				if rr.Code != tt.wantStatus {
					t.Errorf("Expected status %d, got %d", tt.wantStatus, rr.Code)
				}
				return
			}

			sessionID := tt.setup(handler)
			if sessionID != "" {
				tt.body.SessionID = sessionID
			}

			r := setupReplayRouter(handler)
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest("POST", "/api/replay/stop", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("Expected status %d, got %d: %s", tt.wantStatus, rr.Code, rr.Body.String())
			}

			var resp map[string]interface{}
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if tt.check != nil {
				tt.check(t, handler, resp)
			}
		})
	}
}

// TestSeek tests POST /api/replay/seek.
func TestSeek(t *testing.T) {
	pastTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano)
	midTime := time.Now().Add(-30 * time.Minute).Format(time.RFC3339Nano)

	tests := []struct {
		name       string
		setup      func(*ReplayHandler) string
		body       seekRequest
		wantStatus int
		check      func(*testing.T, map[string]interface{})
	}{
		{
			name: "seek to valid timestamp within range",
			setup: func(h *ReplayHandler) string {
				body, _ := json.Marshal(startSessionRequest{
					FromISO8601: time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano),
					Speed:       1,
				})
				r := setupReplayRouter(h)
				req := httptest.NewRequest("POST", "/api/replay/start", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				rr := httptest.NewRecorder()
				r.ServeHTTP(rr, req)

				var resp map[string]interface{}
				json.NewDecoder(rr.Body).Decode(&resp)
				return resp["session_id"].(string)
			},
			body: seekRequest{
				TimestampISO8601: midTime,
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, resp map[string]interface{}) {
				if resp["status"] != "seeked" {
					t.Errorf("Expected status=seeked, got %v", resp["status"])
				}
				if currentMS, ok := resp["current_ms"].(float64); !ok || currentMS == 0 {
					t.Errorf("Expected non-zero current_ms, got %v", resp["current_ms"])
				}
				// Mock store should find a frame
				if frameFound, ok := resp["frame_found"].(bool); !ok || !frameFound {
					t.Error("Expected frame_found=true with mock store")
				}
			},
		},
		{
			name: "seek before session range",
			setup: func(h *ReplayHandler) string {
				fromTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano)
				body, _ := json.Marshal(startSessionRequest{
					FromISO8601: fromTime,
					Speed:       1,
				})
				r := setupReplayRouter(h)
				req := httptest.NewRequest("POST", "/api/replay/start", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				rr := httptest.NewRecorder()
				r.ServeHTTP(rr, req)

				var resp map[string]interface{}
				json.NewDecoder(rr.Body).Decode(&resp)
				return resp["session_id"].(string)
			},
			body: seekRequest{
				TimestampISO8601: time.Now().Add(-2 * time.Hour).Format(time.RFC3339Nano),
			},
			wantStatus: http.StatusBadRequest,
			check: func(t *testing.T, resp map[string]interface{}) {
				if _, ok := resp["error"]; !ok {
					t.Error("Expected error in response")
				}
			},
		},
		{
			name: "invalid timestamp format",
			setup: func(h *ReplayHandler) string {
				body, _ := json.Marshal(startSessionRequest{
					FromISO8601: pastTime,
					Speed:       1,
				})
				r := setupReplayRouter(h)
				req := httptest.NewRequest("POST", "/api/replay/start", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				rr := httptest.NewRecorder()
				r.ServeHTTP(rr, req)

				var resp map[string]interface{}
				json.NewDecoder(rr.Body).Decode(&resp)
				return resp["session_id"].(string)
			},
			body: seekRequest{
				TimestampISO8601: "not-a-timestamp",
			},
			wantStatus: http.StatusBadRequest,
			check: func(t *testing.T, resp map[string]interface{}) {
				if _, ok := resp["error"]; !ok {
					t.Error("Expected error in response")
				}
			},
		},
		{
			name:  "session not found",
			setup: func(h *ReplayHandler) string { return "" },
			body: seekRequest{
				SessionID:        "does-not-exist",
				TimestampISO8601: midTime,
			},
			wantStatus: http.StatusNotFound,
			check: func(t *testing.T, resp map[string]interface{}) {
				if _, ok := resp["error"]; !ok {
					t.Error("Expected error in response")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := newTestReplayHandler(t, true)
			sessionID := tt.setup(handler)
			if sessionID != "" {
				tt.body.SessionID = sessionID
			}

			r := setupReplayRouter(handler)
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest("POST", "/api/replay/seek", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("Expected status %d, got %d: %s", tt.wantStatus, rr.Code, rr.Body.String())
			}

			var resp map[string]interface{}
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if tt.check != nil {
				tt.check(t, resp)
			}
		})
	}
}

// TestTune tests POST /api/replay/tune.
func TestTune(t *testing.T) {
	pastTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano)

	deltaThreshold := 0.05
	tauS := 45.0
	fresnelDecay := 2.5
	subcarriers := 24
	breathingSens := 0.008

	tests := []struct {
		name       string
		setup      func(*ReplayHandler) string
		body       tuneRequest
		wantStatus int
		check      func(*testing.T, map[string]interface{})
	}{
		{
			name: "tune all parameters",
			setup: func(h *ReplayHandler) string {
				body, _ := json.Marshal(startSessionRequest{
					FromISO8601: pastTime,
					Speed:       1,
				})
				r := setupReplayRouter(h)
				req := httptest.NewRequest("POST", "/api/replay/start", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				rr := httptest.NewRecorder()
				r.ServeHTTP(rr, req)

				var resp map[string]interface{}
				json.NewDecoder(rr.Body).Decode(&resp)
				return resp["session_id"].(string)
			},
			body: tuneRequest{
				DeltaRMSThreshold:   &deltaThreshold,
				TauS:               &tauS,
				FresnelDecay:       &fresnelDecay,
				Subcarriers:       &subcarriers,
				BreathingSensitivity: &breathingSens,
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, resp map[string]interface{}) {
				if resp["status"] != "tuned" {
					t.Errorf("Expected status=tuned, got %v", resp["status"])
				}
				params, ok := resp["params"].(map[string]interface{})
				if !ok {
					t.Fatalf("Expected params map, got %T", resp["params"])
				}
				if params["delta_rms_threshold"] != 0.05 {
					t.Errorf("Expected delta_rms_threshold=0.05, got %v", params["delta_rms_threshold"])
				}
				if params["tau_s"] != 45.0 {
					t.Errorf("Expected tau_s=45.0, got %v", params["tau_s"])
				}
				if params["fresnel_decay"] != 2.5 {
					t.Errorf("Expected fresnel_decay=2.5, got %v", params["fresnel_decay"])
				}
				if params["n_subcarriers"] != float64(24) {
					t.Errorf("Expected n_subcarriers=24, got %v", params["n_subcarriers"])
				}
				if params["breathing_sensitivity"] != 0.008 {
					t.Errorf("Expected breathing_sensitivity=0.008, got %v", params["breathing_sensitivity"])
				}
			},
		},
		{
			name: "tune single parameter",
			setup: func(h *ReplayHandler) string {
				body, _ := json.Marshal(startSessionRequest{
					FromISO8601: pastTime,
					Speed:       1,
				})
				r := setupReplayRouter(h)
				req := httptest.NewRequest("POST", "/api/replay/start", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				rr := httptest.NewRecorder()
				r.ServeHTTP(rr, req)

				var resp map[string]interface{}
				json.NewDecoder(rr.Body).Decode(&resp)
				return resp["session_id"].(string)
			},
			body: tuneRequest{
				DeltaRMSThreshold: &deltaThreshold,
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, resp map[string]interface{}) {
				params, ok := resp["params"].(map[string]interface{})
				if !ok {
					t.Fatalf("Expected params map, got %T", resp["params"])
				}
				if params["delta_rms_threshold"] != 0.05 {
					t.Errorf("Expected delta_rms_threshold=0.05, got %v", params["delta_rms_threshold"])
				}
			},
		},
		{
			name:  "session not found",
			setup: func(h *ReplayHandler) string { return "" },
			body: tuneRequest{
				DeltaRMSThreshold: &deltaThreshold,
			},
			wantStatus: http.StatusNotFound,
			check: func(t *testing.T, resp map[string]interface{}) {
				if _, ok := resp["error"]; !ok {
					t.Error("Expected error in response")
				}
			},
		},
		{
			name:  "malformed JSON",
			setup: func(h *ReplayHandler) string { return "" },
			body:  tuneRequest{},
			wantStatus: http.StatusBadRequest,
			check: func(t *testing.T, resp map[string]interface{}) {
				if _, ok := resp["error"]; !ok {
					t.Error("Expected error in response")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := newTestReplayHandler(t, true)

			// Special handling for malformed JSON test
			if tt.name == "malformed JSON" {
				r := setupReplayRouter(handler)
				req := httptest.NewRequest("POST", "/api/replay/tune", bytes.NewReader([]byte(`{invalid}`)))
				req.Header.Set("Content-Type", "application/json")
				rr := httptest.NewRecorder()
				r.ServeHTTP(rr, req)

				if rr.Code != tt.wantStatus {
					t.Errorf("Expected status %d, got %d", tt.wantStatus, rr.Code)
				}
				return
			}

			sessionID := tt.setup(handler)
			if sessionID != "" {
				tt.body.SessionID = sessionID
			}

			r := setupReplayRouter(handler)
			body, _ := json.Marshal(tt.body)
			req := httptest.NewRequest("POST", "/api/replay/tune", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("Expected status %d, got %d: %s", tt.wantStatus, rr.Code, rr.Body.String())
			}

			var resp map[string]interface{}
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if tt.check != nil {
				tt.check(t, resp)
			}
		})
	}
}

// TestReplaySessionLifecycle tests the full lifecycle: start -> tune -> seek -> stop.
func TestReplaySessionLifecycle(t *testing.T) {
	handler := newTestReplayHandler(t, true)
	r := setupReplayRouter(handler)

	pastTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano)
	midTime := time.Now().Add(-30 * time.Minute).Format(time.RFC3339Nano)

	// 1. Start a session
	startBody, _ := json.Marshal(startSessionRequest{
		FromISO8601: pastTime,
		Speed:       2,
	})
	req := httptest.NewRequest("POST", "/api/replay/start", bytes.NewReader(startBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Start: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var startResp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&startResp); err != nil {
		t.Fatalf("Failed to decode start response: %v", err)
	}

	sessionID, ok := startResp["session_id"].(string)
	if !ok || sessionID == "" {
		t.Fatal("Expected non-empty session_id")
	}

	// 2. Tune the session
	threshold := 0.03
	tuneBody, _ := json.Marshal(tuneRequest{
		SessionID:        sessionID,
		DeltaRMSThreshold: &threshold,
	})
	req = httptest.NewRequest("POST", "/api/replay/tune", bytes.NewReader(tuneBody))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Tune: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// 3. Seek within the session
	seekBody, _ := json.Marshal(seekRequest{
		SessionID:        sessionID,
		TimestampISO8601: midTime,
	})
	req = httptest.NewRequest("POST", "/api/replay/seek", bytes.NewReader(seekBody))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Seek: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// 4. Verify session appears in list
	req = httptest.NewRequest("GET", "/api/replay/sessions", nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("List: expected 200, got %d", rr.Code)
	}

	var listResp map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&listResp); err != nil {
		t.Fatalf("Failed to decode list response: %v", err)
	}

	sessions, ok := listResp["sessions"].([]interface{})
	if !ok {
		t.Fatalf("Expected sessions array, got %T", listResp["sessions"])
	}
	if len(sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(sessions))
	}

	// 5. Stop the session
	stopBody, _ := json.Marshal(stopSessionRequest{
		SessionID: sessionID,
	})
	req = httptest.NewRequest("POST", "/api/replay/stop", bytes.NewReader(stopBody))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Stop: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// 6. Verify session is removed from list
	req = httptest.NewRequest("GET", "/api/replay/sessions", nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("List after stop: expected 200, got %d", rr.Code)
	}

	json.NewDecoder(rr.Body).Decode(&listResp)
	sessions, _ = listResp["sessions"].([]interface{})
	if len(sessions) != 0 {
		t.Errorf("Expected 0 sessions after stop, got %d", len(sessions))
	}
}

// TestMultipleSessions tests managing multiple concurrent replay sessions.
func TestMultipleSessions(t *testing.T) {
	handler := newTestReplayHandler(t, true)
	r := setupReplayRouter(handler)

	pastTime1 := time.Now().Add(-2 * time.Hour).Format(time.RFC3339Nano)
	pastTime2 := time.Now().Add(-1 * time.Hour).Format(time.RFC3339Nano)

	// Start two sessions
	startBody, _ := json.Marshal(startSessionRequest{
		FromISO8601: pastTime1,
		Speed:       1,
	})
	req := httptest.NewRequest("POST", "/api/replay/start", bytes.NewReader(startBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var resp1 map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp1)
	sessionID1 := resp1["session_id"].(string)

	startBody, _ = json.Marshal(startSessionRequest{
		FromISO8601: pastTime2,
		Speed:       5,
	})
	req = httptest.NewRequest("POST", "/api/replay/start", bytes.NewReader(startBody))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var resp2 map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&resp2)
	sessionID2 := resp2["session_id"].(string)

	// Verify both sessions exist
	req = httptest.NewRequest("GET", "/api/replay/sessions", nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	var listResp map[string]interface{}
	json.NewDecoder(rr.Body).Decode(&listResp)
	sessions, _ := listResp["sessions"].([]interface{})

	if len(sessions) != 2 {
		t.Fatalf("Expected 2 sessions, got %d", len(sessions))
	}

	// Stop first session
	stopBody, _ := json.Marshal(stopSessionRequest{
		SessionID: sessionID1,
	})
	req = httptest.NewRequest("POST", "/api/replay/stop", bytes.NewReader(stopBody))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Verify one session remains
	req = httptest.NewRequest("GET", "/api/replay/sessions", nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	json.NewDecoder(rr.Body).Decode(&listResp)
	sessions, _ = listResp["sessions"].([]interface{})

	if len(sessions) != 1 {
		t.Errorf("Expected 1 session after stopping one, got %d", len(sessions))
	}

	// Stop second session
	stopBody, _ = json.Marshal(stopSessionRequest{
		SessionID: sessionID2,
	})
	req = httptest.NewRequest("POST", "/api/replay/stop", bytes.NewReader(stopBody))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Verify no sessions remain
	req = httptest.NewRequest("GET", "/api/replay/sessions", nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	json.NewDecoder(rr.Body).Decode(&listResp)
	sessions, _ = listResp["sessions"].([]interface{})

	if len(sessions) != 0 {
		t.Errorf("Expected 0 sessions after stopping both, got %d", len(sessions))
	}
}

// TestGetSessions tests the GetSessions method.
func TestGetSessions(t *testing.T) {
	handler := newTestReplayHandler(t, true)

	// Initially empty
	sessions := handler.GetSessions()
	if len(sessions) != 0 {
		t.Errorf("Expected 0 sessions initially, got %d", len(sessions))
	}

	// Create a session
	handler.mu.Lock()
	handler.sessions["test-session"] = &_replaySession{
		ID:     "test-session",
		FromMS: 1000,
		ToMS:   2000,
		State:  "paused",
	}
	handler.mu.Unlock()

	sessions = handler.GetSessions()
	if len(sessions) != 1 {
		t.Errorf("Expected 1 session, got %d", len(sessions))
	}
	if sessions[0].ID != "test-session" {
		t.Errorf("Expected session ID 'test-session', got %s", sessions[0].ID)
	}
}

// TestGetReplayPath tests the GetReplayPath method.
func TestGetReplayPath(t *testing.T) {
	handler := newTestReplayHandler(t, true)

	path := handler.GetReplayPath()
	if path != "/data/csi_replay.bin" {
		t.Errorf("Expected path '/data/csi_replay.bin', got %s", path)
	}
}

// TestClose tests the Close method.
func TestClose(t *testing.T) {
	store := &mockRecordingStore{}
	handler, err := NewReplayHandler(store)
	if err != nil {
		t.Fatalf("NewReplayHandler: %v", err)
	}

	if err := handler.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}

	if !store.closed {
		t.Error("Expected store to be closed")
	}
}

// TestCloseWithError tests Close when store returns an error.
func TestCloseWithError(t *testing.T) {
	expectedErr := fmt.Errorf("close failed")
	store := &mockRecordingStore{closeErr: expectedErr}
	handler, err := NewReplayHandler(store)
	if err != nil {
		t.Fatalf("NewReplayHandler: %v", err)
	}

	if err := handler.Close(); err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}
}

// TestParseISO8601 tests the parseISO8601 helper function.
func TestParseISO8601(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(int64) bool
	}{
		{
			name:    "valid RFC3339 timestamp",
			input:   "2024-03-15T14:30:00Z",
			wantErr: false,
			check: func(ms int64) bool {
				expected := int64(1710513000000) // 2024-03-15 14:30:00 UTC in ms
				return ms == expected
			},
		},
		{
			name:    "valid RFC3339Nano timestamp",
			input:   "2024-03-15T14:30:00.123456789Z",
			wantErr: false,
			check: func(ms int64) bool {
				return ms > 1710513000000 && ms < 1710513000200
			},
		},
		{
			name:    "invalid timestamp",
			input:   "not-a-timestamp",
			wantErr: true,
			check:   nil,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
			check:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms, err := parseISO8601(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseISO8601(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				if !tt.check(ms) {
					t.Errorf("parseISO8601(%q) = %d, check failed", tt.input, ms)
				}
			}
		})
	}
}

// TestFormatTimestamp tests the formatTimestamp helper function.
func TestFormatTimestamp(t *testing.T) {
	ms := int64(1710519800000) // 2024-03-15 14:30:00 UTC
	formatted := formatTimestamp(ms)

	if formatted == "" {
		t.Error("formatTimestamp returned empty string")
	}

	// Verify it can be parsed back
	_, err := time.Parse(time.RFC3339Nano, formatted)
	if err != nil {
		t.Errorf("formatTimestamp(%d) returned invalid format: %v", ms, err)
	}
}

// TestJumpToTime tests POST /api/replay/jump-to-time.
func TestJumpToTime(t *testing.T) {
	tests := []struct {
		name       string
		body       interface{}
		wantStatus int
		setup      func(*ReplayHandler)
		check      func(*testing.T, *ReplayHandler, map[string]interface{})
	}{
		{
			name: "jump with valid timestamp",
			body: jumpToTimeRequest{
				TimestampMS: 1710519800000,
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, h *ReplayHandler, resp map[string]interface{}) {
				if _, ok := resp["session_id"].(string); !ok {
					t.Errorf("Expected session_id string, got %v", resp["session_id"])
				}
				if ts, ok := resp["timestamp_ms"].(float64); !ok || int64(ts) != 1710519800000 {
					t.Errorf("Expected timestamp_ms=1710519800000, got %v", resp["timestamp_ms"])
				}
				if from, ok := resp["from_ms"].(float64); !ok || int64(from) != 1710519795000 {
					t.Errorf("Expected from_ms=1710519795000, got %v", resp["from_ms"])
				}
				if to, ok := resp["to_ms"].(float64); !ok || int64(to) != 1710519805000 {
					t.Errorf("Expected to_ms=1710519805000, got %v", resp["to_ms"])
				}
				if state, ok := resp["state"].(string); !ok || state != "paused" {
					t.Errorf("Expected state=paused, got %v", resp["state"])
				}
			},
		},
		{
			name: "jump with custom window",
			body: jumpToTimeRequest{
				TimestampMS: 1710519800000,
				WindowMS:    10000,
			},
			wantStatus: http.StatusOK,
			check: func(t *testing.T, h *ReplayHandler, resp map[string]interface{}) {
				if from, ok := resp["from_ms"].(float64); !ok || int64(from) != 1710519790000 {
					t.Errorf("Expected from_ms=1710519790000 (±10s), got %v", resp["from_ms"])
				}
				if to, ok := resp["to_ms"].(float64); !ok || int64(to) != 1710519810000 {
					t.Errorf("Expected to_ms=1710519810000 (±10s), got %v", resp["to_ms"])
				}
			},
		},
		{
			name: "jump replaces previous active session",
			body: jumpToTimeRequest{
				TimestampMS: 1710519900000,
			},
			wantStatus: http.StatusOK,
			setup: func(h *ReplayHandler) {
				// Create a prior session
				body, _ := json.Marshal(jumpToTimeRequest{TimestampMS: 1710519800000})
				r := setupReplayRouter(h)
				req := httptest.NewRequest("POST", "/api/replay/jump-to-time", bytes.NewReader(body))
				req.Header.Set("Content-Type", "application/json")
				rr := httptest.NewRecorder()
				r.ServeHTTP(rr, req)
			},
			check: func(t *testing.T, h *ReplayHandler, resp map[string]interface{}) {
				newID, _ := resp["session_id"].(string)
				h.mu.RLock()
				active := h.activeSessionID
				count := len(h.sessions)
				h.mu.RUnlock()
				if active != newID {
					t.Errorf("Active session = %q, want %q", active, newID)
				}
				if count != 1 {
					t.Errorf("Expected 1 session after replacement, got %d", count)
				}
			},
		},
		{
			name: "missing timestamp_ms",
			body: map[string]interface{}{
				"window_ms": 5000,
			},
			wantStatus: http.StatusBadRequest,
			check: func(t *testing.T, h *ReplayHandler, resp map[string]interface{}) {
				if _, ok := resp["error"]; !ok {
					t.Error("Expected error in response")
				}
			},
		},
		{
			name: "negative timestamp_ms",
			body: jumpToTimeRequest{
				TimestampMS: -100,
			},
			wantStatus: http.StatusBadRequest,
			check: func(t *testing.T, h *ReplayHandler, resp map[string]interface{}) {
				if _, ok := resp["error"]; !ok {
					t.Error("Expected error in response")
				}
			},
		},
		{
			name: "zero timestamp_ms",
			body: jumpToTimeRequest{
				TimestampMS: 0,
			},
			wantStatus: http.StatusBadRequest,
			check: func(t *testing.T, h *ReplayHandler, resp map[string]interface{}) {
				if _, ok := resp["error"]; !ok {
					t.Error("Expected error in response")
				}
			},
		},
		{
			name:       "malformed JSON",
			body:       nil,
			wantStatus: http.StatusBadRequest,
			check: func(t *testing.T, h *ReplayHandler, resp map[string]interface{}) {
				if _, ok := resp["error"]; !ok {
					t.Error("Expected error in response")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := newTestReplayHandler(t, true)

			if tt.setup != nil {
				tt.setup(handler)
			}

			r := setupReplayRouter(handler)

			var body []byte
			if tt.name == "malformed JSON" {
				body = []byte(`{invalid json}`)
			} else if tt.body != nil {
				var err error
				body, err = json.Marshal(tt.body)
				if err != nil {
					t.Fatalf("Failed to marshal request: %v", err)
				}
			}

			req := httptest.NewRequest("POST", "/api/replay/jump-to-time", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			if rr.Code != tt.wantStatus {
				t.Fatalf("Expected status %d, got %d: %s", tt.wantStatus, rr.Code, rr.Body.String())
			}

			var resp map[string]interface{}
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("Failed to decode response: %v", err)
			}

			if tt.check != nil {
				tt.check(t, handler, resp)
			}
		})
	}
}
