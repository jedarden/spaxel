package ble

import "testing"

// Label returns the canonical human-facing identity for an IdentityMatch:
// the PersonName when set, the BLE DeviceName otherwise, and the empty
// string when neither is known. A nil receiver must also yield the empty
// string without panicking.
func TestIdentityMatch_Label(t *testing.T) {
	tests := []struct {
		name string
		m    *IdentityMatch
		want string
	}{
		{
			name: "person name preferred over device name",
			m:    &IdentityMatch{PersonName: "Alice", DeviceName: "iPhone"},
			want: "Alice",
		},
		{
			name: "device name fallback when person name empty",
			m:    &IdentityMatch{DeviceName: "Dog Tracker"},
			want: "Dog Tracker",
		},
		{
			name: "empty when both names empty",
			m:    &IdentityMatch{},
			want: "",
		},
		{
			name: "nil receiver returns empty without panic",
			m:    nil,
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.m.Label(); got != tc.want {
				t.Errorf("Label() = %q, want %q", got, tc.want)
			}
		})
	}
}
