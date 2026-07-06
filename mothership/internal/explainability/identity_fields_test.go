package explainability

import (
	"encoding/json"
	"strings"
	"testing"
)

// canonicalIdentityKeys is the set of bf-5151 canonical identity fields every
// identity-bearing blob type must emit (camelCase, matching
// dashboard/types/spaxel.d.ts). BlobSnapshot is the explainability ("Why is this
// here?") view of a blob — the Tier-1 #2 type from bf-1q3m, which previously
// lacked identity fields entirely. Its canonical fields were added in bf-5151
// but left at zero values until a follow-up bead folds in the parallel
// identityMap from the BLE identity sidecar.
var canonicalIdentityKeys = []string{"personName", "assignedColor", "identityResolved"}

// TestBlobSnapshot_CanonicalIdentitySerialization guards the three canonical
// identity fields on BlobSnapshot. Per the task scope they are left at their Go
// zero values for existing blobs: with omitempty they must serialize as OMITTED
// (undefined in JS). IdentityResolved is *bool so that a non-nil false
// (resolution attempted, failed) is distinct from nil (unattempted) — omitempty
// only drops the nil case. This locks in default-handling consistency with the
// other identity-bearing blob types (signal.TrackedBlob, automation.TrackedBlob,
// tracker.Blob, tracking.Blob, volume.BlobPos).
func TestBlobSnapshot_CanonicalIdentitySerialization(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		blob    BlobSnapshot
		wantKey map[string]bool // key → expected present in JSON
	}{
		{
			name: "zero-value blob omits all three (undefined)",
			blob: BlobSnapshot{ID: 1, X: 1, Y: 2, Z: 3, Confidence: 1},
			wantKey: map[string]bool{
				"personName": false, "assignedColor": false, "identityResolved": false,
			},
		},
		{
			name: "resolved identity emits camelCase keys",
			blob: BlobSnapshot{
				ID: 2, PersonName: "Alice", AssignedColor: "#4488ff",
				IdentityResolved: ptrBoolExplain(true),
			},
			wantKey: map[string]bool{
				"personName": true, "assignedColor": true, "identityResolved": true,
			},
		},
		{
			name: "failed resolution still emits identityResolved=false",
			blob: BlobSnapshot{ID: 3, IdentityResolved: ptrBoolExplain(false)},
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
				var decoded BlobSnapshot
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

// ptrBoolExplain is a small helper for the *bool tri-state IdentityResolved field.
func ptrBoolExplain(b bool) *bool { return &b }
