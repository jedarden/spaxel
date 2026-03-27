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

// Server serves firmware binaries and tracks available versions.
type Server struct {
	mu          sync.RWMutex
	firmwareDir string
	firmware    map[string]*FirmwareMeta
	latestFile  string
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
	defer f.Close()

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

// FirmwareDir returns the directory where firmware binaries are stored.
func (s *Server) FirmwareDir() string {
	return s.firmwareDir
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
// No authentication required (local network only, IP-restricted by Docker).
func (s *Server) HandleServe(w http.ResponseWriter, r *http.Request) {
	filename := filepath.Base(r.URL.Path)
	if filename == "" || filename == "." || !strings.HasSuffix(filename, ".bin") {
		http.NotFound(w, r)
		return
	}

	// Check known list; refresh if missing (file may have been added after start).
	s.mu.RLock()
	meta, ok := s.firmware[filename]
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
	defer file.Close()

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
	defer out.Close()

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
}
