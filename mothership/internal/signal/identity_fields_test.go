package signal

import (
	"encoding/json"
	"strings"
	"testing"
)

// canonicalIdentityKeys is the set of bf-5151 canonical identity fields every
// tracked-blob type must emit (camelCase, matching dashboard/types/spaxel.d.ts).
var canonicalIdentityKeys = []string{"personName", "assignedColor", "identityResolved"}

// TestTrackedBlob_CanonicalIdentitySerialization guards the three bf-5151
// canonical identity fields on signal.TrackedBlob — the canonical tracked-blob
// type that /api/blobs serializes directly and that api.TrackedBlob aliases.
//
// Per the task scope the fields are left at their Go zero values for existing
// blobs: with omitempty they must serialize as OMITTED (undefined in JS) until a
// follow-up bead populates them from the BLE identity sidecar. IdentityResolved
// is *bool so that a non-nil false (resolution attempted, failed) is distinct
// from nil (unattempted) — omitempty only drops the nil case.
func TestTrackedBlob_CanonicalIdentitySerialization(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		blob    TrackedBlob
		wantKey map[string]bool // key → expected present in JSON
	}{
		{
			name: "zero-value blob omits all three (undefined)",
			blob: TrackedBlob{ID: 1, X: 1, Y: 2, Z: 3, Weight: 1},
			wantKey: map[string]bool{
				"personName": false, "assignedColor": false, "identityResolved": false,
			},
		},
		{
			name: "resolved identity emits camelCase keys",
			blob: TrackedBlob{
				ID: 2, PersonName: "Alice", AssignedColor: "#4488ff",
				IdentityResolved: ptrBool(true),
			},
			wantKey: map[string]bool{
				"personName": true, "assignedColor": true, "identityResolved": true,
			},
		},
		{
			name: "failed resolution still emits identityResolved=false",
			blob: TrackedBlob{ID: 3, IdentityResolved: ptrBool(false)},
			wantKey: map[string]bool{
				"personName": false, "assignedColor": false, "identityResolved": true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw, err := json.Marshal(tc.blob)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			body := string(raw)
			for _, k := range canonicalIdentityKeys {
				got := strings.Contains(body, `"`+k+`"`)
				if got != tc.wantKey[k] {
					t.Errorf("key %q: present=%v, want %v (body=%s)", k, got, tc.wantKey[k], body)
				}
			}
			// Verify the tri-state value round-trips when present.
			if tc.wantKey["identityResolved"] {
				var decoded TrackedBlob
				if err := json.Unmarshal(raw, &decoded); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if decoded.IdentityResolved == nil {
					t.Fatalf("identityResolved unexpectedly nil after round-trip (body=%s)", body)
				}
			}
		})
	}
}

// ptrBool is a small helper for the *bool tri-state IdentityResolved field.
func ptrBool(b bool) *bool { return &b }
