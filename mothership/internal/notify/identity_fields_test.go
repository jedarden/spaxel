package notify

import (
	"bytes"
	"path/filepath"
	"testing"
)

// thumbBlob is the anonymous struct Service.GenerateFloorPlanThumbnail accepts.
type thumbBlob = struct {
	X, Y, Z  float64
	Identity string
	IsFall   bool
}

// newTestService builds a notify.Service backed by a throwaway SQLite DB for
// thumbnail-rendering tests.
func newTestService(t *testing.T) *Service {
	t.Helper()
	s, err := NewService(filepath.Join(t.TempDir(), "notify-test.db"))
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// TestGenerateFloorPlanThumbnail_ObservesIdentity verifies the notify projection
// observes a NON-EMPTY identity at runtime: GenerateFloorPlanThumbnail branches on
// blob.Identity (green when identified, blue when unknown). Rendering the same
// position with a populated Identity vs an empty one must therefore produce
// DIFFERENT output — proving the identity field was read and consumed. Both calls
// must succeed without error (no dereference of the identity field). (bf-2v9g)
func TestGenerateFloorPlanThumbnail_ObservesIdentity(t *testing.T) {
	t.Parallel()
	s := newTestService(t)

	identified := []thumbBlob{{X: 2, Y: 0, Z: 2, Identity: "Alice", IsFall: false}}
	unknown := []thumbBlob{{X: 2, Y: 0, Z: 2, Identity: "", IsFall: false}}

	pngIdentified, err := s.GenerateFloorPlanThumbnail(100, 100, identified)
	if err != nil {
		t.Fatalf("identified render failed: %v", err)
	}
	if len(pngIdentified) == 0 {
		t.Fatal("identified render returned empty PNG")
	}

	pngUnknown, err := s.GenerateFloorPlanThumbnail(100, 100, unknown)
	if err != nil {
		t.Fatalf("unknown render failed: %v", err)
	}
	if len(pngUnknown) == 0 {
		t.Fatal("unknown render returned empty PNG")
	}

	// The identity value changes the blob color (green vs blue), so the two
	// renderings differ — i.e. the projection observed and used the identity.
	if bytes.Equal(pngIdentified, pngUnknown) {
		t.Fatal("notify projection did not observe identity: identified and unknown blobs rendered identically")
	}
}

// TestGenerateFloorPlanThumbnail_FallAndIdentityNilSafe verifies the notify
// projection never dereferences an UNSET identity field at runtime: fall blobs and
// fully-unidentified blobs both render without panic. (bf-2v9g)
func TestGenerateFloorPlanThumbnail_FallAndIdentityNilSafe(t *testing.T) {
	t.Parallel()
	s := newTestService(t)

	cases := []struct {
		name string
		blob thumbBlob
	}{
		{"unidentified blob", thumbBlob{X: 2, Y: 0, Z: 2, Identity: "", IsFall: false}},
		{"identified blob", thumbBlob{X: 2, Y: 0, Z: 2, Identity: "Bob", IsFall: false}},
		{"unidentified fall", thumbBlob{X: 2, Y: 0, Z: 2, Identity: "", IsFall: true}},
		{"identified fall", thumbBlob{X: 2, Y: 0, Z: 2, Identity: "Bob", IsFall: true}},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			png, err := s.GenerateFloorPlanThumbnail(100, 100, []thumbBlob{tc.blob})
			if err != nil {
				t.Fatalf("render failed: %v", err)
			}
			if len(png) == 0 {
				t.Fatal("render returned empty PNG")
			}
		})
	}
}
