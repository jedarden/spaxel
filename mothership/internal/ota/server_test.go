// Package ota provides tests for server functionality.
package ota

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestServerSetUploadCallback verifies the upload callback mechanism.
func TestServerSetUploadCallback(t *testing.T) {
	tmpDir := t.TempDir()
	srv := NewServer(tmpDir)

	srv.SetUploadCallback(func(filename string) {
		// Callback received
	})

	// Create a test firmware file
	firmwareContent := []byte("test firmware")
	testFile := filepath.Join(tmpDir, "test-1.0.0.bin")
	if err := os.WriteFile(testFile, firmwareContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Simulate upload by directly calling HandleUpload
	ts := httptest.NewServer(http.HandlerFunc(srv.HandleUpload))
	defer ts.Close()

	// Create multipart form upload request
	req, _ := http.NewRequest("POST", ts.URL+"/api/firmware/upload", strings.NewReader(""))
	req.Header.Set("Content-Type", "multipart/form-data")
	// Note: We're not actually doing a proper multipart upload here,
	// just testing that the callback mechanism exists

	if srv.uploadCallback == nil {
		t.Error("upload callback not set")
	}
}

// TestServerScan verifies firmware scanning works correctly.
func TestServerScan(t *testing.T) {
	tmpDir := t.TempDir()
	srv := NewServer(tmpDir)

	// Initially no firmware
	if srv.GetLatest() != nil {
		t.Error("expected no latest firmware initially")
	}

	// Create a test firmware file
	firmwareContent := []byte("test firmware")
	testFile := filepath.Join(tmpDir, "test-1.0.0.bin")
	if err := os.WriteFile(testFile, firmwareContent, 0644); err != nil {
		t.Fatal(err)
	}

	// Scan should pick up the new file
	srv.Scan()

	latest := srv.GetLatest()
	if latest == nil {
		t.Fatal("expected latest firmware after scan")
	}

	if latest.Filename != "test-1.0.0.bin" {
		t.Errorf("expected filename test-1.0.0.bin, got %s", latest.Filename)
	}

	if latest.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", latest.Version)
	}

	if !latest.IsLatest {
		t.Error("expected IsLatest to be true")
	}
}

// TestServerScanLatestRequiresSemver pins the safety boundary for image-baked
// firmware: an old unversioned seed may remain on a persistent volume after an
// upgrade, but it must never outrank a real release or be selected for OTA.
func TestServerScanLatestRequiresSemver(t *testing.T) {
	tmpDir := t.TempDir()
	files := []string{
		"spaxel-firmware.bin",
		"spaxel-firmware-1.9.0.bin",
		"spaxel-firmware-1.10.0.bin",
	}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, name), []byte(name), 0644); err != nil {
			t.Fatal(err)
		}
	}

	srv := NewServer(tmpDir)
	latest := srv.GetLatest()
	if latest == nil {
		t.Fatal("expected a latest semantic-versioned firmware")
	}
	if latest.Filename != "spaxel-firmware-1.10.0.bin" {
		t.Fatalf("latest filename = %q, want spaxel-firmware-1.10.0.bin", latest.Filename)
	}
	if latest.Version != "1.10.0" {
		t.Fatalf("latest version = %q, want 1.10.0", latest.Version)
	}

	legacy := srv.GetByFilename("spaxel-firmware.bin")
	if legacy == nil {
		t.Fatal("legacy firmware should remain addressable by filename")
	}
	if legacy.IsLatest {
		t.Fatal("unversioned legacy firmware must not be marked latest")
	}
}

func TestServerScanUnversionedOnlyHasNoLatest(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "spaxel-firmware.bin"), []byte("legacy"), 0644); err != nil {
		t.Fatal(err)
	}

	srv := NewServer(tmpDir)
	if latest := srv.GetLatest(); latest != nil {
		t.Fatalf("GetLatest() = %+v, want nil for an unversioned-only store", latest)
	}
}

// TestGetByFilename verifies looking up specific firmware files.
func TestGetByFilename(t *testing.T) {
	tmpDir := t.TempDir()
	srv := NewServer(tmpDir)

	// Create test firmware files
	files := []string{"test-1.0.0.bin", "test-1.1.0.bin", "test-1.2.0.bin"}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(tmpDir, f), []byte(f), 0644); err != nil {
			t.Fatal(err)
		}
	}

	srv.Scan()

	// Test getting each file
	for _, f := range files {
		meta := srv.GetByFilename(f)
		if meta == nil {
			t.Errorf("expected metadata for %s", f)
			continue
		}

		if meta.Filename != f {
			t.Errorf("expected filename %s, got %s", f, meta.Filename)
		}
	}

	// Test non-existent file
	meta := srv.GetByFilename("nonexistent.bin")
	if meta != nil {
		t.Error("expected nil for non-existent file")
	}
}

// TestFirmwareDir verifies the firmware directory is returned correctly.
func TestFirmwareDir(t *testing.T) {
	tmpDir := t.TempDir()
	srv := NewServer(tmpDir)

	if srv.FirmwareDir() != tmpDir {
		t.Errorf("expected firmware dir %s, got %s", tmpDir, srv.FirmwareDir())
	}
}

// TestHandleServe_Authenticated verifies that authenticated requests succeed.
func TestHandleServe_Authenticated(t *testing.T) {
	tmpDir := t.TempDir()
	srv := NewServer(tmpDir)

	// Create a test firmware file
	testFile := filepath.Join(tmpDir, "test-1.0.0.bin")
	testContent := []byte("test firmware content")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatal(err)
	}
	srv.Scan()

	// Set up token validator that accepts one specific MAC/token pair
	srv.SetTokenValidator(func(mac, token string) bool {
		return mac == "AA:BB:CC:DD:EE:FF" && token == "valid-token-123"
	})

	// Create authenticated request
	req := httptest.NewRequest("GET", "/firmware/test-1.0.0.bin", nil)
	req.Header.Set("X-Spaxel-MAC", "AA:BB:CC:DD:EE:FF")
	req.Header.Set("X-Spaxel-Token", "valid-token-123")
	w := httptest.NewRecorder()

	srv.HandleServe(w, req)

	// Should succeed with 200 OK
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Verify content type header
	if ct := w.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Errorf("expected Content-Type application/octet-stream, got %s", ct)
	}

	// Verify SHA256 header is present
	if w.Header().Get("X-SHA256") == "" {
		t.Error("expected X-SHA256 header")
	}

	// Verify body contains firmware content
	if w.Body.Len() != len(testContent) {
		t.Errorf("expected body length %d, got %d", len(testContent), w.Body.Len())
	}
}

// TestHandleServe_InvalidToken verifies that invalid tokens get 404 (not 401).
func TestHandleServe_InvalidToken(t *testing.T) {
	tmpDir := t.TempDir()
	srv := NewServer(tmpDir)

	// Create a test firmware file
	testFile := filepath.Join(tmpDir, "test-1.0.0.bin")
	if err := os.WriteFile(testFile, []byte("test firmware"), 0644); err != nil {
		t.Fatal(err)
	}
	srv.Scan()

	srv.SetTokenValidator(func(mac, token string) bool {
		// Only accept the exact valid token
		return mac == "AA:BB:CC:DD:EE:FF" && token == "valid-token-123"
	})

	testCases := []struct {
		name     string
		mac      string
		token    string
	}{
		{"wrong token", "AA:BB:CC:DD:EE:FF", "wrong-token"},
		{"wrong MAC", "AA:BB:CC:DD:EE:00", "valid-token-123"},
		{"empty token", "AA:BB:CC:DD:EE:FF", ""},
		{"empty MAC", "", "valid-token-123"},
		{"both empty", "", ""},
		{"garbage token", "AA:BB:CC:DD:EE:FF", "garbage!!!"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/firmware/test-1.0.0.bin", nil)
			req.Header.Set("X-Spaxel-MAC", tc.mac)
			req.Header.Set("X-Spaxel-Token", tc.token)
			w := httptest.NewRecorder()

			srv.HandleServe(w, req)

			// Should return 404, not 401 (to avoid leaking which firmware exists)
			if w.Code != http.StatusNotFound {
				t.Errorf("expected status 404, got %d", w.Code)
			}
		})
	}
}

// TestHandleServe_MigrationWindowInside verifies tokenless requests succeed inside the migration window.
func TestHandleServe_MigrationWindowInside(t *testing.T) {
	tmpDir := t.TempDir()
	srv := NewServer(tmpDir)

	// Create a test firmware file
	testFile := filepath.Join(tmpDir, "test-1.0.0.bin")
	testContent := []byte("test firmware")
	if err := os.WriteFile(testFile, testContent, 0644); err != nil {
		t.Fatal(err)
	}
	srv.Scan()

	// Set validator and migration window (1 hour from now)
	srv.SetTokenValidator(func(mac, token string) bool {
		return false // Validator always rejects
	})
	srv.SetMigrationDeadline(time.Now().Add(1 * time.Hour))

	// Request without authentication headers
	req := httptest.NewRequest("GET", "/firmware/test-1.0.0.bin", nil)
	w := httptest.NewRecorder()

	srv.HandleServe(w, req)

	// Should succeed - inside migration window
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 inside migration window, got %d", w.Code)
	}
}

// TestHandleServe_MigrationWindowOutside verifies tokenless requests fail after the migration window closes.
func TestHandleServe_MigrationWindowOutside(t *testing.T) {
	tmpDir := t.TempDir()
	srv := NewServer(tmpDir)

	// Create a test firmware file
	testFile := filepath.Join(tmpDir, "test-1.0.0.bin")
	if err := os.WriteFile(testFile, []byte("test firmware"), 0644); err != nil {
		t.Fatal(err)
	}
	srv.Scan()

	// Set validator and expired migration window (1 hour ago)
	srv.SetTokenValidator(func(mac, token string) bool {
		return false
	})
	srv.SetMigrationDeadline(time.Now().Add(-1 * time.Hour))

	// Request without authentication headers
	req := httptest.NewRequest("GET", "/firmware/test-1.0.0.bin", nil)
	w := httptest.NewRecorder()

	srv.HandleServe(w, req)

	// Should return 404 - migration window closed
	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404 after migration window, got %d", w.Code)
	}
}

// TestHandleServe_StrictMode verifies tokenless requests fail when no migration window is set.
func TestHandleServe_StrictMode(t *testing.T) {
	tmpDir := t.TempDir()
	srv := NewServer(tmpDir)

	// Create a test firmware file
	testFile := filepath.Join(tmpDir, "test-1.0.0.bin")
	if err := os.WriteFile(testFile, []byte("test firmware"), 0644); err != nil {
		t.Fatal(err)
	}
	srv.Scan()

	// Set validator but NO migration window (strict mode)
	srv.SetTokenValidator(func(mac, token string) bool {
		return false
	})
	// Not calling SetMigrationDeadline means zero deadline (strict mode)

	// Request without authentication headers
	req := httptest.NewRequest("GET", "/firmware/test-1.0.0.bin", nil)
	w := httptest.NewRecorder()

	srv.HandleServe(w, req)

	// Should return 404 - strict mode requires token
	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404 in strict mode, got %d", w.Code)
	}
}

// TestHandleServe_NoValidator verifies that without a validator, all requests succeed.
func TestHandleServe_NoValidator(t *testing.T) {
	tmpDir := t.TempDir()
	srv := NewServer(tmpDir)

	// Create a test firmware file
	testFile := filepath.Join(tmpDir, "test-1.0.0.bin")
	if err := os.WriteFile(testFile, []byte("test firmware"), 0644); err != nil {
		t.Fatal(err)
	}
	srv.Scan()

	// No validator set - should allow all requests
	req := httptest.NewRequest("GET", "/firmware/test-1.0.0.bin", nil)
	w := httptest.NewRecorder()

	srv.HandleServe(w, req)

	// Should succeed
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 without validator, got %d", w.Code)
	}
}
