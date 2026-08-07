// Package ota handles firmware binary serving and OTA update orchestration.
package ota

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// FirmwareMeta holds metadata about a firmware binary.
type FirmwareMeta struct {
	Filename   string    `json:"filename"`
	Version    string    `json:"version"`
	SHA256     string    `json:"sha256"`
	SizeBytes  int64     `json:"size_bytes"`
	IsLatest   bool      `json:"is_latest"`
	UploadedAt time.Time `json:"uploaded_at"`
}

// FirmwareUploadCallback is called when new firmware is uploaded.
type FirmwareUploadCallback func(filename string)

// TokenValidator is a function that validates a node token.
// Takes (mac, token) and returns true if valid.
type TokenValidator func(mac, token string) bool

// Server serves firmware binaries and tracks available versions.
type Server struct {
	mu                sync.RWMutex
	firmwareDir       string
	firmware          map[string]*FirmwareMeta
	latestFile        string
	uploadCallback    FirmwareUploadCallback
	tokenValidator    TokenValidator
	migrationDeadline time.Time // Zero value means strict mode (no migration window)
}

// NewServer creates a firmware server backed by firmwareDir.
// It scans the directory on creation to discover existing binaries.
func NewServer(firmwareDir string) *Server {
	if err := os.MkdirAll(firmwareDir, 0755); err != nil {
		log.Printf("[WARN] ota: mkdir %s: %v", firmwareDir, err)
	}
	s := &Server{
		firmwareDir: firmwareDir,
		firmware:    make(map[string]*FirmwareMeta),
	}
	s.Scan()
	return s
}

// SetTokenValidator sets the function used to validate node tokens in firmware downloads.
// If set, requests must include valid X-Spaxel-MAC and X-Spaxel-Token headers,
// except during the migration window when tokenless requests are allowed.
func (s *Server) SetTokenValidator(fn TokenValidator) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tokenValidator = fn
}

// SetMigrationDeadline sets the time after which tokenless requests are rejected.
// Zero value means strict mode (no migration window).
func (s *Server) SetMigrationDeadline(t time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.migrationDeadline = t
}

// Scan refreshes the firmware list from disk.
func (s *Server) Scan() {
	s.mu.Lock()
	defer s.mu.Unlock()

	entries, err := os.ReadDir(s.firmwareDir)
	if err != nil {
		return
	}

	fresh := make(map[string]*FirmwareMeta)
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".bin") {
			continue
		}
		m := s.computeMeta(e.Name())
		if m != nil {
			fresh[e.Name()] = m
			names = append(names, e.Name())
		}
	}
	s.firmware = fresh

	s.latestFile = ""
	if len(names) > 0 {
		sort.Strings(names)
		s.latestFile = names[len(names)-1]
		s.firmware[s.latestFile].IsLatest = true
	}
}

// computeMeta computes SHA-256 and reads metadata for a firmware file.
// Must be called without holding s.mu.
func (s *Server) computeMeta(filename string) *FirmwareMeta {
	path := filepath.Join(s.firmwareDir, filename)
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close() //nolint:errcheck

	stat, err := f.Stat()
	if err != nil {
		return nil
	}

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil
	}

	return &FirmwareMeta{
		Filename:   filename,
		Version:    parseVersion(filename),
		SHA256:     hex.EncodeToString(h.Sum(nil)),
		SizeBytes:  stat.Size(),
		UploadedAt: stat.ModTime(),
	}
}

var versionRe = regexp.MustCompile(`\d+\.\d+\.\d+`)

func parseVersion(filename string) string {
	if v := versionRe.FindString(filename); v != "" {
		return v
	}
	return strings.TrimSuffix(filename, ".bin")
}

// GetLatest returns metadata for the newest firmware binary, or nil if none.
func (s *Server) GetLatest() *FirmwareMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.latestFile == "" {
		return nil
	}
	m := *s.firmware[s.latestFile]
	return &m
}

// GetByFilename returns metadata for a specific firmware file, or nil.
func (s *Server) GetByFilename(filename string) *FirmwareMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if m, ok := s.firmware[filename]; ok {
		cp := *m
		return &cp
	}
	return nil
}

// GetByVersion returns metadata for a firmware by its version string, or nil.
//
// Callers reaching this from the API pass a version (e.g. "0.1.358"), which is
// not the map key — the map is keyed by filename. Looking a version up via
// GetByFilename silently missed, so `POST /api/nodes/{mac}/ota` with a version
// that `GET /api/firmware` reported as present returned "not found".
// See ADR-004 / bf-2cb85.
func (s *Server) GetByVersion(version string) *FirmwareMeta {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, m := range s.firmware {
		if m.Version == version {
			cp := *m
			return &cp
		}
	}
	return nil
}

// FirmwareDir returns the directory where firmware binaries are stored.
func (s *Server) FirmwareDir() string {
	return s.firmwareDir
}

// SetUploadCallback sets the callback to be invoked when new firmware is uploaded.
func (s *Server) SetUploadCallback(cb FirmwareUploadCallback) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.uploadCallback = cb
}

// HandleList serves GET /api/firmware — JSON array of available firmware versions.
func (s *Server) HandleList(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	list := make([]*FirmwareMeta, 0, len(s.firmware))
	for _, m := range s.firmware {
		cp := *m
		list = append(list, &cp)
	}
	s.mu.RUnlock()

	sort.Slice(list, func(i, j int) bool {
		return list[i].Filename < list[j].Filename
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(list)
}

// HandleServe serves GET /firmware/<filename> — the raw binary for OTA.
// Authenticates requests using X-Spaxel-MAC and X-Spaxel-Token headers when a token
// validator is configured. Rejects with 404 (not 401) to avoid leaking which firmware
// versions exist. Nodes inside the migration window may download without a token.
func (s *Server) HandleServe(w http.ResponseWriter, r *http.Request) {
	filename := filepath.Base(r.URL.Path)
	if filename == "" || filename == "." || !strings.HasSuffix(filename, ".bin") {
		http.NotFound(w, r)
		return
	}

	// Check known list; refresh if missing (file may have been added after start).
	s.mu.RLock()
	meta, ok := s.firmware[filename]
	validator := s.tokenValidator
	deadline := s.migrationDeadline
	s.mu.RUnlock()

	if !ok {
		s.Scan()
		s.mu.RLock()
		meta, ok = s.firmware[filename]
		s.mu.RUnlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
	}

	// Authentication check (ADR-006)
	if validator != nil {
		mac := r.Header.Get("X-Spaxel-MAC")
		token := r.Header.Get("X-Spaxel-Token")

		// If both headers are present, validate the token
		if mac != "" && token != "" {
			if !validator(mac, token) {
				// Return 404, not 401 — we don't want to leak which firmware filenames exist
				http.NotFound(w, r)
				return
			}
		} else {
			// Tokenless request — check if inside migration window
			if !deadline.IsZero() && time.Now().Before(deadline) {
				// Inside migration window, allow tokenless request
				log.Printf("[INFO] ota: tokenless request allowed (migration window open until %s)", deadline.Format(time.RFC3339))
			} else if deadline.IsZero() {
				// No migration window configured — strict mode, reject tokenless
				http.NotFound(w, r)
				return
			} else {
				// Migration window closed — reject tokenless request
				log.Printf("[WARN] ota: tokenless request rejected (migration window closed at %s)", deadline.Format(time.RFC3339))
				http.NotFound(w, r)
				return
			}
		}
	}

	path := filepath.Join(s.firmwareDir, filename)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("X-SHA256", meta.SHA256)
	w.Header().Set("X-Firmware-Version", meta.Version)
	http.ServeFile(w, r, path)
}

// HandleUpload serves POST /api/firmware/upload — stores a new firmware binary.
func (s *Server) HandleUpload(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		http.Error(w, "invalid multipart form", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("firmware")
	if err != nil {
		http.Error(w, "missing 'firmware' field", http.StatusBadRequest)
		return
	}
	defer file.Close() //nolint:errcheck

	filename := filepath.Base(header.Filename)
	if !strings.HasSuffix(filename, ".bin") || strings.ContainsAny(filename, "/\\") {
		http.Error(w, "filename must end in .bin", http.StatusBadRequest)
		return
	}

	dest := filepath.Join(s.firmwareDir, filename)
	out, err := os.Create(dest)
	if err != nil {
		http.Error(w, "failed to save firmware", http.StatusInternalServerError)
		return
	}
	defer out.Close() //nolint:errcheck

	if _, err := io.Copy(out, file); err != nil {
		http.Error(w, "write error", http.StatusInternalServerError)
		return
	}

	s.Scan()

	s.mu.RLock()
	meta := s.firmware[filename]
	s.mu.RUnlock()

	log.Printf("[INFO] ota: uploaded %s (sha256=%s)", filename, meta.SHA256)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(meta)

	// Notify callback if set (triggers auto-update check)
	s.mu.RLock()
	cb := s.uploadCallback
	s.mu.RUnlock()
	if cb != nil {
		go cb(filename)
	}
}
