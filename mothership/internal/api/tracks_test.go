package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// mockTracksProvider implements TracksProvider for testing.
type mockTracksProvider struct {
	blobs []TrackedBlob
}

func (m *mockTracksProvider) GetTrackedBlobs() []TrackedBlob {
	return m.blobs
}

// TestListTracks_NoBlobs tests GET /api/tracks with no tracked blobs.
func TestListTracks_NoBlobs(t *testing.T) {
	provider := &mockTracksProvider{blobs: []TrackedBlob{}}
	handler := NewTracksHandler(provider)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest("GET", "/api/tracks", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var tracks []Track
	if err := json.NewDecoder(w.Body).Decode(&tracks); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(tracks) != 0 {
		t.Errorf("expected 0 tracks, got %d", len(tracks))
	}
}

// TestListTracks_WithBlobs tests GET /api/tracks with tracked blobs.
func TestListTracks_WithBlobs(t *testing.T) {
	blobs := []TrackedBlob{
		{
			ID:                 1,
			X:                  1.5,
			Y:                  2.3,
			Z:                  0.8,
			VX:                 0.1,
			VY:                 0.2,
			VZ:                 0.0,
			Weight:             0.95,
			PersonID:           "person-123",
			PersonLabel:        "Alice",
			PersonColor:        "#ff0000",
			IdentityConfidence: 0.85,
			IdentitySource:     "ble",
			Posture:            "standing",
		},
		{
			ID:                 2,
			X:                  3.2,
			Y:                  4.1,
			Z:                  0.0,
			VX:                 0.0,
			VY:                 0.0,
			VZ:                 0.0,
			Weight:             0.75,
			PersonID:           "", // No identity match
			PersonLabel:        "",
			PersonColor:        "",
			IdentityConfidence: 0.0,
			IdentitySource:     "",
			Posture:            "",
		},
	}

	provider := &mockTracksProvider{blobs: blobs}
	handler := NewTracksHandler(provider)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest("GET", "/api/tracks", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var tracks []Track
	if err := json.NewDecoder(w.Body).Decode(&tracks); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(tracks) != 2 {
		t.Fatalf("expected 2 tracks, got %d", len(tracks))
	}

	// Verify first track (with identity)
	if tracks[0].ID != 1 {
		t.Errorf("expected ID 1, got %d", tracks[0].ID)
	}
	if tracks[0].X != 1.5 {
		t.Errorf("expected X 1.5, got %f", tracks[0].X)
	}
	if tracks[0].Y != 2.3 {
		t.Errorf("expected Y 2.3, got %f", tracks[0].Y)
	}
	if tracks[0].Z != 0.8 {
		t.Errorf("expected Z 0.8, got %f", tracks[0].Z)
	}
	if tracks[0].PersonID != "person-123" {
		t.Errorf("expected PersonID person-123, got %s", tracks[0].PersonID)
	}
	if tracks[0].PersonLabel != "Alice" {
		t.Errorf("expected PersonLabel Alice, got %s", tracks[0].PersonLabel)
	}
	if tracks[0].PersonColor != "#ff0000" {
		t.Errorf("expected PersonColor #ff0000, got %s", tracks[0].PersonColor)
	}
	if tracks[0].IdentityConfidence != 0.85 {
		t.Errorf("expected IdentityConfidence 0.85, got %f", tracks[0].IdentityConfidence)
	}
	if tracks[0].IdentitySource != "ble" {
		t.Errorf("expected IdentitySource ble, got %s", tracks[0].IdentitySource)
	}
	if tracks[0].Posture != "standing" {
		t.Errorf("expected Posture standing, got %s", tracks[0].Posture)
	}

	// Verify second track (without identity)
	if tracks[1].ID != 2 {
		t.Errorf("expected ID 2, got %d", tracks[1].ID)
	}
	if tracks[1].PersonID != "" {
		t.Errorf("expected empty PersonID, got %s", tracks[1].PersonID)
	}
	if tracks[1].IdentityConfidence != 0.0 {
		t.Errorf("expected IdentityConfidence 0.0, got %f", tracks[1].IdentityConfidence)
	}
}

// TestListTracks_ContentType verifies the response Content-Type header.
func TestListTracks_ContentType(t *testing.T) {
	provider := &mockTracksProvider{blobs: []TrackedBlob{}}
	handler := NewTracksHandler(provider)

	r := chi.NewRouter()
	handler.RegisterRoutes(r)

	req := httptest.NewRequest("GET", "/api/tracks", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", ct)
	}
}
