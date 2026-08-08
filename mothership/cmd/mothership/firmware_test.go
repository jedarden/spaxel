package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestServeSerialFirmware(t *testing.T) {
	dir := t.TempDir()
	const filename = "spaxel-firmware-1.2.3-merged.bin"
	path := filepath.Join(dir, filename)
	want := []byte("merged full-flash image")
	if err := os.WriteFile(path, want, 0644); err != nil {
		t.Fatal(err)
	}

	r := chi.NewRouter()
	r.Get("/firmware/serial/{filename}", serveSerialFirmware(filename, path))

	t.Run("serves configured merged image", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/firmware/serial/"+filename, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}
		if got := rec.Body.Bytes(); string(got) != string(want) {
			t.Fatalf("body = %q, want %q", got, want)
		}
	})

	t.Run("rejects other filenames", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/firmware/serial/other.bin", nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", rec.Code)
		}
	})
}
