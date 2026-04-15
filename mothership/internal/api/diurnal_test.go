// Package api provides tests for the diurnal baseline API.
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spaxel/mothership/internal/signal"
)

// mockProcessorManager mocks the ProcessorManager interface for testing.
type mockDiurnalProcessorManager struct {
	processors map[string]*mockDiurnalLinkProcessor
}

type mockDiurnalLinkProcessor struct {
	diurnal *signal.DiurnalBaseline
}

func (m *mockDiurnalLinkProcessor) GetDiurnal() *signal.DiurnalBaseline {
	return m.diurnal
}

func (m *mockDiurnalProcessorManager) GetDiurnalLearningStatus() []signal.DiurnalLearningStatus {
	// For testing, we'll create mock status directly
	return []signal.DiurnalLearningStatus{
		{
			LinkID:            "AA:BB:CC:DD:EE:FF",
			IsLearning:        true,
			DaysRemaining:     5.0,
			Progress:          28.5,
			IsReady:           false,
			SlotsReady:        8,
			DiurnalConfidence: 0.33,
			CreatedAt:         time.Now().Add(-2 * 24 * time.Hour),
		},
	}
}

func (m *mockDiurnalProcessorManager) GetProcessor(linkID string) DiurnalLinkProcessor {
	if p, ok := m.processors[linkID]; ok {
		return p
	}
	return nil
}

// newMockDiurnalBaseline creates a mock diurnal baseline with test data.
func newMockDiurnalBaseline() *signal.DiurnalBaseline {
	db := signal.NewDiurnalBaseline("AA:BB:CC:DD:EE:FF", 64)

	// Simulate having some data by directly manipulating the slots
	for i := 0; i < 24; i++ {
		slot := db.GetSlot(i)
		if slot != nil {
			// Fill with test amplitude values
			for k := 0; k < 64; k++ {
				slot.Values[k] = 0.5 + float64(i)*0.01
			}
			slot.SampleCount = 300 + i*10 // Start with minimum required samples
			slot.LastUpdate = time.Now().Add(-time.Duration(i) * time.Hour)
		}
	}

	return db
}

// Test getDiurnalStatus
func TestGetDiurnalStatus(t *testing.T) {
	handler := &DiurnalHandler{
		pm: &mockDiurnalProcessorManager{},
	}

	req := httptest.NewRequest("GET", "/api/diurnal/status", nil)
	w := httptest.NewRecorder()

	handler.getDiurnalStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var statuses []signal.DiurnalLearningStatus
	if err := json.NewDecoder(w.Body).Decode(&statuses); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(statuses) != 1 {
		t.Fatalf("got %d statuses, want 1", len(statuses))
	}

	status := statuses[0]
	if status.LinkID != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("link_id = %q, want AA:BB:CC:DD:EE:FF", status.LinkID)
	}
	if !status.IsLearning {
		t.Error("is_learning = false, want true")
	}
	if status.DaysRemaining != 5.0 {
		t.Errorf("days_remaining = %f, want 5.0", status.DaysRemaining)
	}
	if status.Progress != 28.5 {
		t.Errorf("progress = %f, want 28.5", status.Progress)
	}
}

// Test getDiurnalSlots
func TestGetDiurnalSlots(t *testing.T) {
	mockDiurnal := newMockDiurnalBaseline()
	mockProc := &mockDiurnalLinkProcessor{diurnal: mockDiurnal}

	handler := &DiurnalHandler{
		pm: &mockDiurnalProcessorManager{
			processors: map[string]*mockDiurnalLinkProcessor{
				"AA:BB:CC:DD:EE:FF": mockProc,
			},
		},
	}

	req := httptest.NewRequest("GET", "/api/diurnal/slots/AA:BB:CC:DD:EE:FF", nil)
	w := httptest.NewRecorder()

	handler.getDiurnalSlots(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if response["link_id"] != "AA:BB:CC:DD:EE:FF" {
		t.Errorf("link_id = %v, want AA:BB:CC:DD:EE:FF", response["link_id"])
	}

	// Check slot_amplitudes exists and has 24 slots (JSON unmarshals as []interface{})
	slotAmplitudesRaw, ok := response["slot_amplitudes"].([]interface{})
	if !ok {
		t.Fatalf("slot_amplitudes missing or wrong type, got %T", response["slot_amplitudes"])
	}

	if len(slotAmplitudesRaw) != 24 {
		t.Errorf("got %d slots, want 24", len(slotAmplitudesRaw))
	}

	// Check first slot has data (each slot is []interface{} of float64 values)
	if slot0, ok := slotAmplitudesRaw[0].([]interface{}); ok {
		if len(slot0) != 64 {
			t.Errorf("slot 0 has %d values, want 64", len(slot0))
		}
	} else {
		t.Errorf("slot 0 wrong type: %T", slotAmplitudesRaw[0])
	}

	// Check confidence values exist (JSON unmarshals as []interface{})
	slotConfidencesRaw, ok := response["slot_confidences"].([]interface{})
	if !ok {
		t.Fatalf("slot_confidences missing or wrong type, got %T", response["slot_confidences"])
	}

	if len(slotConfidencesRaw) != 24 {
		t.Errorf("got %d confidences, want 24", len(slotConfidencesRaw))
	}

	// Check learning status
	if response["is_learning"] != true {
		t.Error("is_learning = false, want true")
	}

	if response["is_ready"] != false {
		t.Error("is_ready = true, want false (only 2 days old)")
	}
}

// Test getDiurnalSlots - missing linkID
func TestGetDiurnalSlots_MissingLinkID(t *testing.T) {
	handler := &DiurnalHandler{
		pm: &mockDiurnalProcessorManager{},
	}

	req := httptest.NewRequest("GET", "/api/diurnal/slots/", nil)
	w := httptest.NewRecorder()

	handler.getDiurnalSlots(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}

	var errResp map[string]string
	json.NewDecoder(w.Body).Decode(&errResp)

	if errResp["error"] == "" {
		t.Error("expected error message")
	}
}

// Test getDiurnalSlots - link not found
func TestGetDiurnalSlots_LinkNotFound(t *testing.T) {
	handler := &DiurnalHandler{
		pm: &mockDiurnalProcessorManager{
			processors: map[string]*mockDiurnalLinkProcessor{},
		},
	}

	req := httptest.NewRequest("GET", "/api/diurnal/slots/AA:BB:CC:DD:EE:FF", nil)
	w := httptest.NewRecorder()

	handler.getDiurnalSlots(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

// Test getDiurnalSlots - nil diurnal
func TestGetDiurnalSlots_NilDiurnal(t *testing.T) {
	mockProc := &mockDiurnalLinkProcessor{diurnal: nil}

	handler := &DiurnalHandler{
		pm: &mockDiurnalProcessorManager{
			processors: map[string]*mockDiurnalLinkProcessor{
				"AA:BB:CC:DD:EE:FF": mockProc,
			},
		},
	}

	req := httptest.NewRequest("GET", "/api/diurnal/slots/AA:BB:CC:DD:EE:FF", nil)
	w := httptest.NewRecorder()

	handler.getDiurnalSlots(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}
