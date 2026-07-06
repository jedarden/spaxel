package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// canonicalIdentityKeys is the set of bf-5151 canonical identity fields the
// /api/tracks Track projection must emit (camelCase, matching
// dashboard/types/spaxel.d.ts).
var canonicalIdentityKeys = []string{"personName", "assignedColor", "identityResolved"}

// trackResponseJSON drives GET /api/tracks through the real handler and returns
// the raw JSON body so key presence/absence can be asserted precisely.
func trackResponseJSON(t *testing.T, blobs []TrackedBlob) string {
	t.Helper()
	h := NewTracksHandler(&mockTracksProvider{blobs: blobs})
	r := chi.NewRouter()
	h.RegisterRoutes(r)

	req := httptest.NewRequest("GET", "/api/tracks", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	return w.Body.String()
}

// TestListTracks_CanonicalIdentitySerialization guards that listTracks (a)
// PROPAGATES the three bf-5151 canonical identity fields from the source
// signal.TrackedBlob onto the serialized Track, and (b) leaves them omitted
// (undefined in JS) for zero-value blobs. The *bool tri-state is exercised:
// nil is omitted, while a non-nil false (resolution attempted, failed) must
// still emit "identityResolved":false.
func TestListTracks_CanonicalIdentitySerialization(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		blob    TrackedBlob
		wantKey map[string]bool
	}{
		{
			name: "zero-value blob omits all three (undefined)",
			blob: TrackedBlob{ID: 1, X: 1, Y: 2, Z: 3, Weight: 1},
			wantKey: map[string]bool{
				"personName": false, "assignedColor": false, "identityResolved": false,
			},
		},
		{
			name: "resolved identity propagated and emitted in camelCase",
			blob: TrackedBlob{
				ID: 2, PersonName: "Alice", AssignedColor: "#4488ff",
				IdentityResolved: ptrBoolTrack(true),
			},
			wantKey: map[string]bool{
				"personName": true, "assignedColor": true, "identityResolved": true,
			},
		},
		{
			name: "failed resolution propagated as identityResolved=false",
			blob: TrackedBlob{ID: 3, IdentityResolved: ptrBoolTrack(false)},
			wantKey: map[string]bool{
				"personName": false, "assignedColor": false, "identityResolved": true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			body := trackResponseJSON(t, []TrackedBlob{tc.blob})

			for _, k := range canonicalIdentityKeys {
				got := strings.Contains(body, `"`+k+`"`)
				if got != tc.wantKey[k] {
					t.Errorf("key %q: present=%v, want %v (body=%s)", k, got, tc.wantKey[k], body)
				}
			}

			// Decode and assert the propagated Go values for the resolved case.
			var tracks []Track
			if err := json.Unmarshal([]byte(body), &tracks); err != nil { //nolint:errcheck
				t.Fatalf("unmarshal: %v", err)
			}
			if len(tracks) != 1 || tracks[0].ID != tc.blob.ID {
				t.Fatalf("expected 1 track with ID %d, got %+v", tc.blob.ID, tracks)
			}
			tr := tracks[0]
			if tr.PersonName != tc.blob.PersonName {
				t.Errorf("PersonName propagation: got %q, want %q", tr.PersonName, tc.blob.PersonName)
			}
			if tr.AssignedColor != tc.blob.AssignedColor {
				t.Errorf("AssignedColor propagation: got %q, want %q", tr.AssignedColor, tc.blob.AssignedColor)
			}
			if (tr.IdentityResolved == nil) != (tc.blob.IdentityResolved == nil) {
				t.Errorf("IdentityResolved propagation: got %v, want %v", tr.IdentityResolved, tc.blob.IdentityResolved)
			} else if tr.IdentityResolved != nil && *tr.IdentityResolved != *tc.blob.IdentityResolved {
				t.Errorf("IdentityResolved value: got %v, want %v", *tr.IdentityResolved, *tc.blob.IdentityResolved)
			}
		})
	}
}

// ptrBoolTrack is a small helper for the *bool tri-state IdentityResolved field.
func ptrBoolTrack(b bool) *bool { return &b }
