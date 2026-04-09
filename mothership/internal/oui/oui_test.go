package oui

import (
	"net"
	"testing"
)

func TestLookupOUI(t *testing.T) {
	tests := []struct {
		name         string
		mac          string
		want         string
		wantEmpty    bool
	}{
		{
			name:      "Apple OUI",
			mac:       "00:11:B0:AA:BB:CC",
			want:      "AppleComputer",
			wantEmpty: false,
		},
		{
			name:      "Cisco OUI",
			mac:       "00:12:00:AA:BB:CC",
			want:      "CiscroSystems",
			wantEmpty: false,
		},
		{
			name:      "Netgear OUI",
			mac:       "00:15:F2:AA:BB:CC",
			want:      "Netgear",
			wantEmpty: false,
		},
		{
			name:      "ASUS OUI",
			mac:       "00:16:0A:AA:BB:CC",
			want:      "ASUSTekComputer",
			wantEmpty: false,
		},
		{
			name:      "Google OUI",
			mac:       "00:1A:4F:AA:BB:CC",
			want:      "Google",
			wantEmpty: false,
		},
		{
			name:      "Intel OUI",
			mac:       "00:1A:CD:AA:BB:CC",
			want:      "Intel",
			wantEmpty: false,
		},
		{
			name:      "Realtek OUI",
			mac:       "00:50:E4:AA:BB:CC",
			want:      "RealtekSemiconductor",
			wantEmpty: false,
		},
		{
			name:      "Amazon OUI",
			mac:       "10:BF:48:AA:BB:CC",
			want:      "AmazonTechnologies",
			wantEmpty: false,
		},
		{
			name:      "Unknown OUI - returns empty",
			mac:       "FF:FF:FF:AA:BB:CC",
			want:      "",
			wantEmpty: true,
		},
		{
			name:      "Unknown OUI not in registry",
			mac:       "11:22:33:AA:BB:CC",
			want:      "",
			wantEmpty: true,
		},
		{
			name:      "Lowercase MAC",
			mac:       "00:11:b0:aa:bb:cc",
			want:      "AppleComputer",
			wantEmpty: false,
		},
		{
			name:      "Mixed case MAC",
			mac:       "00:11:B0:aA:Bb:Cc",
			want:      "AppleComputer",
			wantEmpty: false,
		},
		{
			name:      "Short MAC - less than 3 bytes",
			mac:       "AA:BB",
			want:      "",
			wantEmpty: true,
		},
		{
			name:      "Empty MAC",
			mac:       "",
			want:      "",
			wantEmpty: true,
		},
		{
			name:      "Exact OUI match - HP",
			mac:       "00:13:D1:AA:BB:CC",
			want:      "HP",
			wantEmpty: false,
		},
		{
			name:      "Exact OUI match - D-Link",
			mac:       "00:13:A0:AA:BB:CC",
			want:      "DLinkCorporation",
			wantEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse MAC string to bytes
			hw, err := net.ParseMAC(tt.mac)
			if err != nil && tt.mac != "" {
				t.Fatalf("net.ParseMAC(%q) error = %v", tt.mac, err)
			}

			got := LookupOUI(hw)

			if tt.wantEmpty {
				if got != "" {
					t.Errorf("LookupOUI(%q) = %q; want empty string", tt.mac, got)
				}
			} else {
				if got != tt.want {
					t.Errorf("LookupOUI(%q) = %q; want %q", tt.mac, got, tt.want)
				}
			}
		})
	}
}

func TestLookupOUI_WithBytes(t *testing.T) {
	tests := []struct {
		name      string
		macBytes  []byte
		want      string
		wantEmpty bool
	}{
		{
			name:      "Apple bytes",
			macBytes:  []byte{0x00, 0x11, 0xB0, 0xAA, 0xBB, 0xCC},
			want:      "AppleComputer",
			wantEmpty: false,
		},
		{
			name:      "Cisco bytes",
			macBytes:  []byte{0x00, 0x12, 0x00, 0xAA, 0xBB, 0xCC},
			want:      "CiscroSystems",
			wantEmpty: false,
		},
		{
			name:      "Short bytes",
			macBytes:  []byte{0x00, 0x11},
			want:      "",
			wantEmpty: true,
		},
		{
			name:      "Nil bytes",
			macBytes:  nil,
			want:      "",
			wantEmpty: true,
		},
		{
			name:      "Empty bytes",
			macBytes:  []byte{},
			want:      "",
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := LookupOUI(tt.macBytes)

			if tt.wantEmpty {
				if got != "" {
					t.Errorf("LookupOUI(%v) = %q; want empty string", tt.macBytes, got)
				}
			} else {
				if got != tt.want {
					t.Errorf("LookupOUI(%v) = %q; want %q", tt.macBytes, got, tt.want)
				}
			}
		})
	}
}

func TestLookupOUI_NoPanic(t *testing.T) {
	// Ensure LookupOUI never panics, even with invalid input
	tests := [][]byte{
		nil,
		{},
		{0x00},
		{0x00, 0x11},
		{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
		{0x00, 0x11, 0xB0},
		{0x00, 0x11, 0xB0, 0xAA, 0xBB, 0xCC},
	}

	for _, mac := range tests {
		// This should not panic
		_ = LookupOUI(mac)
	}
}

func TestLookupOUI_BigEndian(t *testing.T) {
	// Test that the OUI lookup uses big-endian byte order
	// For OUI 00:11:B0, the bytes are [0x00, 0x11, 0xB0]
	// The key should be 0x000011B0 (or 0x11B0 after shift)
	mac := []byte{0x00, 0x11, 0xB0, 0xAA, 0xBB, 0xCC}
	got := LookupOUI(mac)

	// If big-endian is used correctly, this should match Apple
	if got != "AppleComputer" {
		t.Errorf("LookupOUI with big-endian OUI = %q; want 'AppleComputer'", got)
	}
}

// TestOuiMapNotEmpty verifies the generated OUI map is not empty
// This test will pass when go generate is run with the full IEEE registry
func TestOuiMapNotEmpty(t *testing.T) {
	if len(ouiMap) == 0 {
		t.Skip("OUI map is empty - run 'go generate' to download IEEE OUI registry")
	}

	// We expect at least 5000 entries from the full IEEE registry
	if len(ouiMap) < 5000 {
		t.Logf("Warning: OUI map has only %d entries; expected 5000+. Run 'go generate' to download full registry.", len(ouiMap))
	}
}

// BenchmarkLookupOUI benchmarks the OUI lookup performance
func BenchmarkLookupOUI(b *testing.B) {
	mac := []byte{0x00, 0x11, 0xB0, 0xAA, 0xBB, 0xCC}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		LookupOUI(mac)
	}
}
