// Package api provides tests for security API endpoints.
package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi"
	"github.com/spaxel/mothership/internal/analytics"
	"github.com/spaxel/mothership/internal/events"
)

// mockDetectorProvider is a mock implementation of DetectorProvider for testing.
type mockDetectorProvider struct {
	mode             analytics.SecurityMode
	isActive         bool
	progress         float64
	modelReady       bool
	activeAnomalies  []*events.AnomalyEvent
	history          []*events.AnomalyEvent
	modeChanges      []analytics.SecurityMode
}

func (m *mockDetectorProvider) GetSecurityMode() analytics.SecurityMode {
	return m.mode
}

func (m *mockDetectorProvider) SetSecurityMode(mode analytics.SecurityMode, reason string) {
	m.mode = mode
	m.modeChanges = append(m.modeChanges, mode)
}

func (m *mockDetectorProvider) IsSecurityModeActive() bool {
	return m.isActive
}

func (m *mockDetectorProvider) GetLearningProgress() float64 {
	return m.progress
}

func (m *mockDetectorProvider) IsModelReady() bool {
	return m.modelReady
}

func (m *mockDetectorProvider) GetActiveAnomalies() []*events.AnomalyEvent {
	return m.activeAnomalies
}

func (m *mockDetectorProvider) GetAnomalyHistory(limit int) []*events.AnomalyEvent {
	if len(m.history) <= limit {
		return m.history
	}
	return m.history[len(m.history)-limit:]
}

func (m *mockDetectorProvider) CountAnomaliesSince(since time.Time) (int, error) {
	count := 0
	for _, e := range m.history {
		if e.Timestamp.After(since) {
			count++
		}
	}
	return count, nil
}

func TestSecurityHandler_Status(t *testing.T) {
	tests := []struct {
		name           string
		mode           analytics.SecurityMode
		isActive       bool
		modelReady     bool
		progress       float64
		anomalies24h   int
		wantStatusCode int
		wantArmed      bool
		wantMode       string
	}{
		{
			name:           "disarmed mode",
			mode:           analytics.SecurityModeDisarmed,
			isActive:       false,
			modelReady:     false,
			progress:       0.5,
			anomalies24h:   3,
			wantStatusCode: http.StatusOK,
			wantArmed:      false,
			wantMode:       "disarmed",
		},
		{
			name:           "armed mode",
			mode:           analytics.SecurityModeArmed,
			isActive:       true,
			modelReady:     true,
			progress:       1.0,
			anomalies24h:   0,
			wantStatusCode: http.StatusOK,
			wantArmed:      true,
			wantMode:       "armed",
		},
		{
			name:           "armed_stay mode",
			mode:           analytics.SecurityModeArmedStay,
			isActive:       true,
			modelReady:     true,
			progress:       1.0,
			anomalies24h:   1,
			wantStatusCode: http.StatusOK,
			wantArmed:      true,
			wantMode:       "armed_stay",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create anomalies for the last 24h
			history := make([]*events.AnomalyEvent, tt.anomalies24h)
			for i := 0; i < tt.anomalies24h; i++ {
				history[i] = &events.AnomalyEvent{
					ID:        time.Now().Add(time.Duration(i) * time.Hour).Format("20060102150405"),
					Timestamp: time.Now().Add(time.Duration(i) * time.Hour),
				}
			}

			mock := &mockDetectorProvider{
				mode:         tt.mode,
				isActive:     tt.isActive,
				modelReady:   tt.modelReady,
				progress:     tt.progress,
				history:      history,
			}

			handler := NewSecurityHandler(mock)
			r := chi.NewRouter()
			handler.RegisterRoutes(r)

			req := httptest.NewRequest("GET", "/api/security/status", nil)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatusCode {
				t.Errorf("status code = %d, want %d", w.Code, tt.wantStatusCode)
			}

			var status SecurityStatus
			if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			if status.Armed != tt.wantArmed {
				t.Errorf("Armed = %v, want %v", status.Armed, tt.wantArmed)
			}

			if status.Mode != tt.wantMode {
				t.Errorf("Mode = %s, want %s", status.Mode, tt.wantMode)
			}

			if status.ModelReady != tt.modelReady {
				t.Errorf("ModelReady = %v, want %v", status.ModelReady, tt.modelReady)
			}

			if status.AnomalyCount24h != tt.anomalies24h {
				t.Errorf("AnomalyCount24h = %d, want %d", status.AnomalyCount24h, tt.anomalies24h)
			}

			// Check learning_until is set when model is not ready
			if !tt.modelReady && status.LearningUntil == "" {
				t.Error("LearningUntil should be set when model is not ready")
			}
			if tt.modelReady && status.LearningUntil != "" {
				t.Error("LearningUntil should be empty when model is ready")
			}
		})
	}
}

func TestSecurityHandler_Arm(t *testing.T) {
	tests := []struct {
		name           string
		requestBody    string
		initialMode    analytics.SecurityMode
		wantMode       analytics.SecurityMode
		wantStatusCode int
	}{
		{
			name:           "arm without mode defaults to armed",
			requestBody:    `{}`,
			initialMode:    analytics.SecurityModeDisarmed,
			wantMode:       analytics.SecurityModeArmed,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "arm with armed mode",
			requestBody:    `{"mode": "armed"}`,
			initialMode:    analytics.SecurityModeDisarmed,
			wantMode:       analytics.SecurityModeArmed,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "arm with armed_stay mode",
			requestBody:    `{"mode": "armed_stay"}`,
			initialMode:    analytics.SecurityModeDisarmed,
			wantMode:       analytics.SecurityModeArmedStay,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "invalid mode returns bad request",
			requestBody:    `{"mode": "invalid"}`,
			initialMode:    analytics.SecurityModeDisarmed,
			wantMode:       analytics.SecurityModeDisarmed, // unchanged
			wantStatusCode: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockDetectorProvider{
				mode: tt.initialMode,
			}

			handler := NewSecurityHandler(mock)
			r := chi.NewRouter()
			handler.RegisterRoutes(r)

			req := httptest.NewRequest("POST", "/api/security/arm", bytes.NewBufferString(tt.requestBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatusCode {
				t.Errorf("status code = %d, want %d", w.Code, tt.wantStatusCode)
			}

			if tt.wantStatusCode == http.StatusOK {
				if mock.mode != tt.wantMode {
					t.Errorf("mode = %s, want %s", mock.mode, tt.wantMode)
				}

				var resp map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatalf("failed to unmarshal response: %v", err)
				}

				if resp["armed"] != true {
					t.Errorf("armed = %v, want true", resp["armed"])
				}
			} else {
				// Mode should not have changed on error
				if mock.mode != tt.initialMode {
					t.Errorf("mode = %s, want %s (unchanged)", mock.mode, tt.initialMode)
				}
			}
		})
	}
}

func TestSecurityHandler_Disarm(t *testing.T) {
	tests := []struct {
		name           string
		initialMode    analytics.SecurityMode
		wantMode       analytics.SecurityMode
		wantStatusCode int
	}{
		{
			name:           "disarm from armed",
			initialMode:    analytics.SecurityModeArmed,
			wantMode:       analytics.SecurityModeDisarmed,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "disarm from armed_stay",
			initialMode:    analytics.SecurityModeArmedStay,
			wantMode:       analytics.SecurityModeDisarmed,
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "disarm when already disarmed",
			initialMode:    analytics.SecurityModeDisarmed,
			wantMode:       analytics.SecurityModeDisarmed,
			wantStatusCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockDetectorProvider{
				mode: tt.initialMode,
			}

			handler := NewSecurityHandler(mock)
			r := chi.NewRouter()
			handler.RegisterRoutes(r)

			req := httptest.NewRequest("POST", "/api/security/disarm", nil)
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if w.Code != tt.wantStatusCode {
				t.Errorf("status code = %d, want %d", w.Code, tt.wantStatusCode)
			}

			if mock.mode != tt.wantMode {
				t.Errorf("mode = %s, want %s", mock.mode, tt.wantMode)
			}

			var resp map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatalf("failed to unmarshal response: %v", err)
			}

			if resp["armed"] != false {
				t.Errorf("armed = %v, want false", resp["armed"])
			}
		})
	}
}

func TestSecurityHandler_NilDetector(t *testing.T) {
	handler := NewSecurityHandler(nil)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "status", method: "GET", path: "/api/security/status"},
		{name: "arm", method: "POST", path: "/api/security/arm", body: `{}`},
		{name: "disarm", method: "POST", path: "/api/security/disarm"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body *bytes.Buffer
			if tt.body != "" {
				body = bytes.NewBufferString(tt.body)
			} else {
				body = &bytes.Buffer{}
			}

			req := httptest.NewRequest(tt.method, tt.path, body)
			if tt.method == "POST" {
				req.Header.Set("Content-Type", "application/json")
			}
			w := httptest.NewRecorder()

			r.ServeHTTP(w, req)

			if w.Code != http.StatusServiceUnavailable {
				t.Errorf("status code = %d, want %d", w.Code, http.StatusServiceUnavailable)
			}
		})
	}
}

func TestSecurityHandler_CountAnomalies24h(t *testing.T) {
	now := time.Now()
	history := []*events.AnomalyEvent{
		{Timestamp: now.Add(-1 * time.Hour)},     // Within 24h
		{Timestamp: now.Add(-12 * time.Hour)},    // Within 24h
		{Timestamp: now.Add(-25 * time.Hour)},    // Outside 24h
		{Timestamp: now.Add(-48 * time.Hour)},    // Outside 24h
	}

	mock := &mockDetectorProvider{
		mode:    analytics.SecurityModeDisarmed,
		history: history,
	}

	handler := NewSecurityHandler(mock)
	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest("GET", "/api/security/status", nil)
	w := httptest.NewRecorder()

	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", w.Code, http.StatusOK)
	}

	var status SecurityStatus
	if err := json.Unmarshal(w.Body.Bytes(), &status); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	// Should count only the 2 anomalies within 24h
	if status.AnomalyCount24h != 2 {
		t.Errorf("AnomalyCount24h = %d, want 2", status.AnomalyCount24h)
	}
}
