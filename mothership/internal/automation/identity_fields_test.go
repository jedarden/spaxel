package automation

import (
	"encoding/json"
	"strings"
	"testing"
)

// canonicalIdentityKeys is the set of bf-5151 canonical identity fields the
// automation.TrackedBlob type must emit (camelCase, matching
// dashboard/types/spaxel.d.ts). automation.TrackedBlob is the Tier-1 #1 type
// from bf-1q3m: it previously carried NO identity fields at all, which
// structurally blocked person-aware automations ("when Alice enters…").
var canonicalIdentityKeys = []string{"personName", "assignedColor", "identityResolved"}

// TestTrackedBlob_CanonicalIdentitySerialization guards the three bf-5151
// canonical identity fields on automation.TrackedBlob. Per the task scope they
// are left at their Go zero values for existing blobs: with omitempty they must
// serialize as OMITTED (undefined in JS) until a follow-up bead populates them
// from the BLE identity sidecar. The *bool tri-state is exercised: nil is
// omitted, while a non-nil false must still emit "identityResolved":false.
func TestTrackedBlob_CanonicalIdentitySerialization(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		blob    TrackedBlob
		wantKey map[string]bool
	}{
		{
			name: "zero-value blob omits all three (undefined)",
			blob: TrackedBlob{ID: 1, X: 1, Y: 2, Z: 3, Confidence: 1},
			wantKey: map[string]bool{
				"personName": false, "assignedColor": false, "identityResolved": false,
			},
		},
		{
			name: "resolved identity emits camelCase keys",
			blob: TrackedBlob{
				ID: 2, PersonName: "Alice", AssignedColor: "#4488ff",
				IdentityResolved: ptrBoolAuto(true),
			},
			wantKey: map[string]bool{
				"personName": true, "assignedColor": true, "identityResolved": true,
			},
		},
		{
			name: "failed resolution still emits identityResolved=false",
			blob: TrackedBlob{ID: 3, IdentityResolved: ptrBoolAuto(false)},
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
		})
	}
}

// ptrBoolAuto is a small helper for the *bool tri-state IdentityResolved field.
func ptrBoolAuto(b bool) *bool { return &b }
