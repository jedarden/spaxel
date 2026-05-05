// Package ota provides tests for server functionality.
package ota

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
