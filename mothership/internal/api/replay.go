// Package api provides REST API handlers for Spaxel CSI replay/time-travel.
package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi"
)

// ReplayHandler manages CSI replay sessions.
type ReplayHandler struct {
	mu         sync.RWMutex
	store      *RecordingStore
	sessions   map[string]*ReplaySession
	nextID     int
	replayPath string
}

// RecordingStore is the interface to the CSI recording store.
type RecordingStore interface {
	Stats() Stats
	Scan(fn func(recvTimeNS int64, frame []byte) bool) bool
	Close() error
}

// Stats represents recording store statistics.
type Stats struct {
	HasData   bool
	WritePos  int64
	OldestPos int64
	FileSize  int64
}

// _replaySession represents an active replay session.
type _replaySession struct {
	ID        string
	FromMS    int64
	ToMS      int64
	CurrentMS int64
	Speed     int
	State     string // playing, paused, stopped
	Params    map[string]interface{}
	CreatedAt time.Time
}

// NewReplayHandler creates a new replay handler.
func NewReplayHandler(replayPath string, store RecordingStore) (*ReplayHandler, error) {
	return &ReplayHandler{
		store:      store,
		sessions:   make(map[string]*_replaySession),
		nextID:     1,
		replayPath: replayPath,
	}, nil
}

// Close closes the replay handler.
func (h *ReplayHandler) Close() error {
	return h.store.Close()
}

// RegisterRoutes registers replay endpoints.
//
// GET  /api/replay/sessions   — list available recording sessions
// POST /api/replay/start      — start replay at given timestamp
// POST /api/replay/stop       — stop replay, return to live
// POST /api/replay/seek       — seek to timestamp within session
// POST /api/replay/tune       — update pipeline parameters mid-replay
func (h *ReplayHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/replay/sessions", h.listSessions)
	r.Post("/api/replay/start", h.startSession)
	r.Post("/api/replay/stop", h.stopSession)
	r.Post("/api/replay/seek", h.seek)
	r.Post("/api/replay/tune", h.tune)
}

type replayInfo struct {
	HasData   bool   `json:"has_data"`
	FileSize  int64  `json:"file_size_mb"`
	WritePos  int64  `json:"write_pos"`
	OldestPos int64  `json:"oldest_pos"`
	OldestTS  int64  `json:"oldest_timestamp_ms"`
	NewestTS  int64  `json:"newest_timestamp_ms"`
	Sessions  []*_replaySession `json:"sessions"`
}

func (h *ReplayHandler) listSessions(w http.ResponseWriter, r *http.Request) {
	stats := h.store.Stats()

	h.mu.RLock()
	sessions := make([]*_replaySession, 0, len(h.sessions))
	for _, s := range h.sessions {
		sessions = append(sessions, s)
	}
	h.mu.RUnlock()

	// Get oldest and newest timestamps
	var oldestTS, newestTS int64
	if stats.HasData {
		h.scanOldest(&oldestTS)
		h.scanNewest(&newestTS)
	}

	info := replayInfo{
		HasData:   stats.HasData,
		FileSize:  stats.FileSize / (1024 * 1024),
		WritePos:  stats.WritePos,
		OldestPos: stats.OldestPos,
		OldestTS:  oldestTS,
		NewestTS:  newestTS,
		Sessions:  sessions,
	}

	writeJSON(w, info)
}

type startSessionRequest struct {
	FromISO8601 string `json:"from_iso8601"`
	ToISO8601   string `json:"to_iso8601"`
	Speed       int    `json:"speed,omitempty"` // 1, 2, 5
}

func (h *ReplayHandler) startSession(w http.ResponseWriter, r *http.Request) {
	var req startSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	fromMS, err := parseISO8601(req.FromISO8601)
	if err != nil {
		http.Error(w, "invalid from_iso8601: "+err.Error(), http.StatusBadRequest)
		return
	}

	toMS := time.Now().UnixNano() / 1e6
	if req.ToISO8601 != "" {
		toMS, err = parseISO8601(req.ToISO8601)
		if err != nil {
			http.Error(w, "invalid to_iso8601: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	if toMS < fromMS {
		http.Error(w, "to_iso8601 must be after from_iso8601", http.StatusBadRequest)
		return
	}

	speed := req.Speed
	if speed == 0 {
		speed = 1
	}
	if speed != 1 && speed != 2 && speed != 5 {
		http.Error(w, "speed must be 1, 2, or 5", http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	session := &_replaySession{
		ID:        fmt.Sprintf("replay-%d", h.nextID),
		FromMS:    fromMS,
		ToMS:      toMS,
		CurrentMS: fromMS,
		Speed:     speed,
		State:     "paused",
		Params:    make(map[string]interface{}),
		CreatedAt: time.Now(),
	}
	h.nextID++
	h.sessions[session.ID] = session

	log.Printf("[INFO] Replay session started: %s (from %d to %d, speed %dx)",
		session.ID, fromMS, toMS, speed)

	writeJSON(w, map[string]interface{}{
		"session_id": session.ID,
		"from_ms":    fromMS,
		"to_ms":      toMS,
		"speed":      speed,
		"state":      "paused",
	})
}

type stopSessionRequest struct {
	SessionID string `json:"session_id"`
}

func (h *ReplayHandler) stopSession(w http.ResponseWriter, r *http.Request) {
	var req stopSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	session, exists := h.sessions[req.SessionID]
	if !exists {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	session.State = "stopped"
	delete(h.sessions, req.SessionID)

	writeJSON(w, map[string]interface{}{
		"status": "stopped",
		"session": req.SessionID,
	})
}

type seekRequest struct {
	SessionID       string `json:"session_id"`
	TimestampISO8601 string `json:"timestamp_iso8601"`
}

func (h *ReplayHandler) seek(w http.ResponseWriter, r *http.Request) {
	var req seekRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	session, exists := h.sessions[req.SessionID]
	if !exists {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	targetMS, err := parseISO8601(req.TimestampISO8601)
	if err != nil {
		http.Error(w, "invalid timestamp: "+err.Error(), http.StatusBadRequest)
		return
	}

	if targetMS < session.FromMS || targetMS > session.ToMS {
		http.Error(w, "timestamp outside session range", http.StatusBadRequest)
		return
	}

	session.CurrentMS = targetMS
	session.State = "paused"

	// Read one frame at the target position
	var frameData []byte
	h.store.Scan(func(recvTimeNS int64, frame []byte) bool {
		recvMS := recvTimeNS / 1e6
		if recvMS >= targetMS {
			frameData = frame
			return false // stop after first match
		}
		return true
	}

	writeJSON(w, map[string]interface{}{
		"status":      "seeked",
		"current_ms":  targetMS,
		"frame_found": len(frameData) > 0,
	})
}

type tuneRequest struct {
	SessionID          string                  `json:"session_id"`
	DeltaRMSThreshold  *float64                `json:"delta_rms_threshold,omitempty"`
	TauS               *float64                `json:"tau_s,omitempty"`
	FresnelDecay       *float64                `json:"fresnel_decay,omitempty"`
	Subcarriers       *int                    `json:"n_subcarriers,omitempty"`
	BreathingSensitivity *float64                `json:"breathing_sensitivity,omitempty"`
}

func (h *ReplayHandler) tune(w http.ResponseWriter, r *http.Request) {
	var req tuneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	session, exists := h.sessions[req.SessionID]
	if !exists {
		http.Error(w, "session not found", http.StatusNotFound)
		return
	}

	// Update params
	params := session.Params
	if req.DeltaRMSThreshold != nil {
		params["delta_rms_threshold"] = *req.DeltaRMSThreshold
	}
	if req.TauS != nil {
		params["tau_s"] = *req.TauS
	}
	if req.FresnelDecay != nil {
		params["fresnel_decay"] = *req.FresnelDecay
	}
	if req.Subcarriers != nil {
		params["n_subcarriers"] = *req.Subcarriers
	}
	if req.BreathingSensitivity != nil {
		params["breathing_sensitivity"] = *req.BreathingSensitivity
	}

	log.Printf("[INFO] Replay session tuned: %s params=%+v", req.SessionID, params)

	writeJSON(w, map[string]interface{}{
		"status":  "tuned",
		"params":  params,
		"session": req.SessionID,
	})
}

// scanOldest scans for the oldest timestamp in the store.
func (h *ReplayHandler) scanOldest(result *int64) {
	h.store.Scan(func(recvTimeNS int64, frame []byte) bool {
		*result = recvTimeNS / 1e6
		return false // stop at first (oldest)
	})
}

// scanNewest scans for the newest timestamp in the store.
func (h *ReplayHandler) scanNewest(result *int64) {
	h.store.Scan(func(recvTimeNS int64, frame []byte) bool {
		*result = recvTimeNS / 1e6
		return true // continue to find newest
	})
}

// parseISO8601 parses an ISO8601 timestamp to milliseconds since epoch.
func parseISO8601(s string) (int64, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return 0, err
	}
	return t.UnixNano() / 1e6, nil
}

// formatTimestamp formats milliseconds since epoch as ISO8601.
func formatTimestamp(ms int64) string {
	return time.Unix(ms/1000, (ms%1000)*1e6).Format(time.RFC3339Nano)
}

// GetSessions returns all active replay sessions.
func (h *ReplayHandler) GetSessions() []*_replaySession {
	h.mu.RLock()
	defer h.mu.RUnlock()

	sessions := make([]*_replaySession, 0, len(h.sessions))
	for _, s := range h.sessions {
		sessions = append(sessions, s)
	}
	return sessions
}

// GetReplayPath returns the path to the CSI replay binary file.
func (h *ReplayHandler) GetReplayPath() string {
	return h.replayPath
}
