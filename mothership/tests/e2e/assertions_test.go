package e2e

import (
	"strings"
	"testing"
)

// TestAssertBlobObserved is a table-driven unit test for the reusable blob
// assertion helper. It validates that the helper passes when the dashboard WS
// feed showed at least one blob and fails (with a descriptive message) when it
// did not — including the empty and all-zero edge cases. No mothership required.
func TestAssertBlobObserved(t *testing.T) {
	cases := []struct {
		name       string
		blobCounts []int
		wantErr    bool
		wantSubstr string // substring expected in the error message on failure
	}{
		{
			name:       "peak above one passes",
			blobCounts: []int{0, 0, 1, 0, 2, 1},
			wantErr:    false,
		},
		{
			name:       "single tick with one blob passes",
			blobCounts: []int{1},
			wantErr:    false,
		},
		{
			name:       "all zero fails",
			blobCounts: []int{0, 0, 0, 0},
			wantErr:    true,
			wantSubstr: "max blob count observed was 0",
		},
		{
			name:       "empty slice fails",
			blobCounts: []int{},
			wantErr:    true,
			wantSubstr: "max blob count observed was 0",
		},
		{
			name:       "nil slice fails",
			blobCounts: nil,
			wantErr:    true,
			wantSubstr: "max blob count observed was 0",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := AssertBlobObserved(tc.blobCounts)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.wantSubstr != "" && !strings.Contains(err.Error(), tc.wantSubstr) {
					t.Errorf("error %q does not contain expected substring %q", err.Error(), tc.wantSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
		})
	}
}

// TestAssertDetectionEventsObserved is a table-driven unit test for the reusable
// detection-event assertion helper. It validates the pass case (>=1 event) and
// the two failure cases (empty events list, nil response). No mothership required.
func TestAssertDetectionEventsObserved(t *testing.T) {
	cases := []struct {
		name       string
		events     *EventsResponse
		wantErr    bool
		wantSubstr string
	}{
		{
			name: "non-empty events passes",
			events: &EventsResponse{
				Events: []Event{{ID: 1, Type: "detection"}},
			},
			wantErr: false,
		},
		{
			name:       "empty events fails",
			events:     &EventsResponse{Events: []Event{}},
			wantErr:    true,
			wantSubstr: "got 0",
		},
		{
			name:       "nil response fails",
			events:     nil,
			wantErr:    true,
			wantSubstr: "events response was nil",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := AssertDetectionEventsObserved(tc.events)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				if tc.wantSubstr != "" && !strings.Contains(err.Error(), tc.wantSubstr) {
					t.Errorf("error %q does not contain expected substring %q", err.Error(), tc.wantSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
		})
	}
}

// TestAssertHelpersReturnNonNil is a guard ensuring the failure paths return a
// real error (not nil). A silent nil would reintroduce the dodge these hard-fail
// primitives are meant to fix, so the non-nil contract is the core guarantee.
func TestAssertHelpersReturnNonNil(t *testing.T) {
	if err := AssertBlobObserved(nil); err == nil {
		t.Error("AssertBlobObserved(nil) must return a non-nil error")
	}
	if err := AssertDetectionEventsObserved(nil); err == nil {
		t.Error("AssertDetectionEventsObserved(nil) must return a non-nil error")
	}
}
