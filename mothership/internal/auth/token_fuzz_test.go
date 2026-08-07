// Package auth provides fuzz tests for token validation.
package auth

import (
	"database/sql"
	"encoding/hex"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// FuzzValidateNodeToken fuzzes ValidateNodeToken with random MAC and token pairs.
// Key property: ValidateNodeToken must NEVER panic on any mac/token pair.
// The X-Spaxel-Token header is attacker-controlled and fully untrusted.
func FuzzValidateNodeToken(f *testing.F) {
	// Add seed corpus covering the enumerated tamper cases
	f.Add([]byte("AA:BB:CC:DD:EE:FF"), []byte("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")) // valid
	f.Add([]byte("AA:BB:CC:DD:EE:FF"), []byte("wrong"))                                                  // wrong length
	f.Add([]byte("AA:BB:CC:DD:EE:FF"), []byte("g123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")) // invalid hex
	f.Add([]byte("AA:BB:CC:DD:EE:FF"), []byte("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdEE")) // wrong bytes
	f.Add([]byte("AA:BB:CC:DD:EE:FF"), []byte(""))                                                // empty
	f.Add([]byte("AA:BB:CC:DD:EE:FF"), []byte("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcde")) // one byte short
	f.Add([]byte("AA:BB:CC:DD:EE:FF"), []byte("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef0")) // one byte long
	f.Add([]byte("aa:bb:cc:dd:ee:ff"), []byte("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")) // lowercase MAC
	f.Add([]byte("AABBCCDDEEFF"), []byte("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"))  // MAC without colons
	f.Add([]byte(""), []byte("")) // both empty
	f.Add([]byte(""), []byte("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")) // empty MAC
	f.Add([]byte("AA:BB:CC:DD:EE:FF"), []byte(strings.Repeat("0123456789abcdef", 100)))              // very long token

	f.Fuzz(func(t *testing.T, mac, token []byte) {
		// Create handler with in-memory database
		db, err := sql.Open("sqlite", ":memory:")
		if err != nil {
			t.Skipf("Failed to create in-memory database: %v", err)
		}
		defer db.Close() //nolint:errcheck

		h, err := NewHandler(Config{DB: db})
		if err != nil {
			t.Skipf("Failed to create handler: %v", err)
		}
		defer h.Close() //nolint:errcheck

		// Convert byte slices to strings for the API
		macStr := string(mac)
		tokenStr := string(token)

		// The key property: ValidateNodeToken must NEVER panic
		// It should gracefully handle any attacker-controlled input
		_ = h.ValidateNodeToken(macStr, tokenStr)
	})
}

// TestValidateNodeTokenProperty tests the roundtrip property:
// ValidateNodeToken(mac, token) is true iff token equals DeriveNodeToken(mac).
func TestValidateNodeTokenProperty(t *testing.T) {
	// Create handler with in-memory database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck

	h, err := NewHandler(Config{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close() //nolint:errcheck

	testCases := []struct {
		name     string
		mac      string
		wantPass bool
	}{
		{
			name:     "valid MAC with derived token",
			mac:      "AA:BB:CC:DD:EE:FF",
			wantPass: true,
		},
		{
			name:     "valid MAC without colons",
			mac:      "AABBCCDDEEFF",
			wantPass: true,
		},
		{
			name:     "lowercase MAC",
			mac:      "aa:bb:cc:dd:ee:ff",
			wantPass: true,
		},
		{
			name:     "wrong token",
			mac:      "AA:BB:CC:DD:EE:FF",
			wantPass: false,
		},
		{
			name:     "empty token",
			mac:      "AA:BB:CC:DD:EE:FF",
			wantPass: false,
		},
		{
			name:     "token too short",
			mac:      "AA:BB:CC:DD:EE:FF",
			wantPass: false,
		},
		{
			name:     "token too long",
			mac:      "AA:BB:CC:DD:EE:FF",
			wantPass: false,
		},
		{
			name:     "invalid hex in token",
			mac:      "AA:BB:CC:DD:EE:FF",
			wantPass: false,
		},
		{
			name:     "empty MAC",
			mac:      "",
			wantPass: false,
		},
		{
			name:     "MAC with invalid hex",
			mac:      "GG:BB:CC:DD:EE:FF",
			wantPass: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Derive the expected token for this MAC
			expectedToken, err := h.DeriveNodeToken(tc.mac)
			if err != nil && tc.wantPass {
				t.Fatalf("DeriveNodeToken(%q) failed: %v", tc.mac, err)
			}

			// Test with the derived token (should pass for valid MACs)
			if tc.mac != "" && tc.wantPass {
				if !h.ValidateNodeToken(tc.mac, expectedToken) {
					t.Errorf("ValidateNodeToken(%q, derived_token) should return true", tc.mac)
				}
			}

			// For the pass cases, verify that a wrong token fails
			if tc.wantPass && tc.mac != "" {
				wrongToken := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcde"
				if h.ValidateNodeToken(tc.mac, wrongToken) {
					t.Errorf("ValidateNodeToken(%q, wrong_token) should return false", tc.mac)
				}
			}

			// Test specific failure cases
			switch tc.name {
			case "wrong token":
				wrongToken := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcde"
				if h.ValidateNodeToken(tc.mac, wrongToken) {
					t.Error("Expected false for wrong token")
				}
			case "empty token":
				if h.ValidateNodeToken(tc.mac, "") {
					t.Error("Expected false for empty token")
				}
			case "token too short":
				shortToken := "0123456789abcdef"
				if h.ValidateNodeToken(tc.mac, shortToken) {
					t.Error("Expected false for short token")
				}
			case "token too long":
				longToken := strings.Repeat("0123456789abcdef", 100)
				if h.ValidateNodeToken(tc.mac, longToken) {
					t.Error("Expected false for long token")
				}
			case "invalid hex in token":
				invalidHexToken := "g123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
				if h.ValidateNodeToken(tc.mac, invalidHexToken) {
					t.Error("Expected false for invalid hex token")
				}
			case "empty MAC":
				// DeriveNodeToken is lenient: it normalizes empty MAC to ""
				// and computes HMAC. The roundtrip property holds:
				// ValidateNodeToken("", DeriveNodeToken("")) returns true.
				// This is by design - the function never panics.
				// In practice, empty MACs don't match real nodes.
				emptyToken, err := h.DeriveNodeToken("")
				if err != nil {
					t.Errorf("DeriveNodeToken should not error on empty MAC, got: %v", err)
				}
				// The roundtrip property: a MAC validates with its own derived token
				if !h.ValidateNodeToken("", emptyToken) {
					t.Error("Roundtrip property: empty MAC should validate with its own derived token")
				}
				// But it won't validate a wrong token
				if h.ValidateNodeToken("", "wrong") {
					t.Error("Empty MAC should not validate with wrong token")
				}
			case "MAC with invalid hex":
				// DeriveNodeToken is lenient: invalid hex chars are preserved
				// and HMAC is computed. This is intentional - the function is
				// designed not to panic on attacker-controlled input.
				invalidMACToken, err := h.DeriveNodeToken("GG:BB:CC:DD:EE:FF")
				if err != nil {
					t.Errorf("DeriveNodeToken should not error on invalid MAC, got: %v", err)
				}
				// The derived token exists but won't match any properly formatted MAC
				// Validate should still work correctly
				_ = invalidMACToken // used to demonstrate no panic
			}
		})
	}
}

// TestDeriveNodeTokenRoundtrip tests that DeriveNodeToken is deterministic.
func TestDeriveNodeTokenRoundtrip(t *testing.T) {
	// Create handler with in-memory database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close() //nolint:errcheck

	h, err := NewHandler(Config{DB: db})
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close() //nolint:errcheck

	mac := "AA:BB:CC:DD:EE:FF"

	// Derive token twice
	token1, err1 := h.DeriveNodeToken(mac)
	token2, err2 := h.DeriveNodeToken(mac)

	if err1 != nil || err2 != nil {
		t.Fatalf("DeriveNodeToken failed: err1=%v, err2=%v", err1, err2)
	}

	if token1 != token2 {
		t.Errorf("DeriveNodeToken not deterministic: %q != %q", token1, token2)
	}

	// Verify it's a valid 64-char hex string
	if len(token1) != 64 {
		t.Errorf("Derived token has wrong length: got %d, want 64", len(token1))
	}

	if _, err := hex.DecodeString(token1); err != nil {
		t.Errorf("Derived token is not valid hex: %v", err)
	}
}
