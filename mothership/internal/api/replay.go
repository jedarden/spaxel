// Package api provides REST API handlers for Spaxel CSI replay/time-travel.
package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi"
)

// ReplayHandler manages CSI replay sessions.
type ReplayHandler struct {
	mu         sync.RWMutex
	store      RecordingStore
	sessions   map[string]*_replaySession
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
// Replay/Time-Travel Endpoints:
//
//	GET  /api/replay/sessions   — list recording sessions and replay store info
//
//	@Summary		List replay sessions
//	@Description	Returns information about available recorded data and active replay sessions.
//	@Description	Includes file size, timestamp range, and all active sessions.
//	@Tags			replay
//	@Produce		json
//	@Success		200	{object}	replayInfo	"Replay store info and active sessions"
//	@Router			/api/replay/sessions [get]
//
//	POST /api/replay/start      — start replay at given timestamp
//
//	@Summary		Start replay session
//	@Description	Creates a new replay session for the specified time range. The session
//	@Description	starts in paused state. Use speed to control playback rate (1, 2, or 5).
//	@Tags			replay
//	@Accept			json
//	@Produce		json
//	@Param			request	body	startSessionRequest	true	"Replay start parameters"
//	@Success		200	{object}	map[string]interface{}	"Session created with ID and state"
//	@Failure		400	{object}	map[string]string	"Invalid request parameters"
//	@Router			/api/replay/start [post]
//
//	POST /api/replay/stop       — stop replay, return to live
//
//	@Summary		Stop replay session
//	@Description	Stops the specified replay session and returns to live mode.
//	@Tags			replay
//	@Accept			json
//	@Produce		json
//	@Param			request	body	stopSessionRequest	true	"Session to stop"
//	@Success		200	{object}	map[string]string	"Session stopped"
//	@Failure		404	{object}	map[string]string	"Session not found"
//	@Router			/api/replay/stop [post]
//
//	POST /api/replay/seek       — seek to timestamp within session
//
//	@Summary		Seek within replay session
//	@Description	Moves the replay cursor to the specified timestamp within the session range.
//	@Description	Pauses playback and reads one frame at the target position.
//	@Tags			replay
//	@Accept			json
//	@Produce		json
//	@Param			request	body	seekRequest	true	"Seek parameters"
//	@Success		200	{object}	map[string]interface{}	"Seek complete with current position"
//	@Failure		400	{object}	map[string]string	"Invalid timestamp or out of range"
//	@Failure		404	{object}	map[string]string	"Session not found"
//	@Router			/api/replay/seek [post]
//
//	POST /api/replay/tune       — update pipeline parameters mid-replay
//
//	@Summary		Tune replay pipeline parameters
//	@Description	Updates detection pipeline parameters for the replay session without
//	@Description	affecting live processing. Useful for exploring how parameter changes
//	@Description	affect detection on historical data.
//	@Tags			replay
//	@Accept			json
//	@Produce		json
//	@Param			request	body	tuneRequest	true	"Parameter updates"
//	@Success		200	{object}	map[string]interface{}	"Parameters updated"
//	@Failure		400	{object}	map[string]string	"Invalid request"
//	@Failure		404	{object}	map[string]string	"Session not found"
//	@Router			/api/replay/tune [post]
func (h *ReplayHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/replay/sessions", h.listSessions)
	r.Post("/api/replay/start", h.startSession)
	r.Post("/api/replay/stop", h.stopSession)
	r.Post("/api/replay/seek", h.seek)
	r.Post("/api/replay/tune", h.tune)
}

// replayInfo represents the response from GET /api/replay/sessions.
type replayInfo struct {
	HasData   bool             `json:"has_data"`
	FileSize  int64            `json:"file_size_mb"`
	WritePos  int64            `json:"write_pos"`
	OldestPos int64            `json:"oldest_pos"`
	OldestTS  int64            `json:"oldest_timestamp_ms"`
	NewestTS  int64            `json:"newest_timestamp_ms"`
	Sessions  []*_replaySession `json:"sessions"`
}

// listSessions handles GET /api/replay/sessions.
// Returns replay store statistics and all active sessions.
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

	writeJSON(w, http.StatusOK, info)
}

// startSessionRequest represents the request body for POST /api/replay/start.
type startSessionRequest struct {
	// FromISO8601 is the start timestamp in ISO8601 format (e.g., "2024-03-15T14:30:00Z")
	FromISO8601 string `json:"from_iso8601"`
	// ToISO8601 is the end timestamp in ISO8601 format. If empty, defaults to now.
	ToISO8601 string `json:"to_iso8601"`
	// Speed is the playback speed multiplier: 1, 2, or 5. Defaults to 1.
	Speed int `json:"speed,omitempty"`
}

// startSession handles POST /api/replay/start.
// Creates a new replay session for the specified time range.
func (h *ReplayHandler) startSession(w http.ResponseWriter, r *http.Request) {
	var req startSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	fromMS, err := parseISO8601(req.FromISO8601)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid from_iso8601: " + err.Error()})
		return
	}

	toMS := time.Now().UnixNano() / 1e6
	if req.ToISO8601 != "" {
		toMS, err = parseISO8601(req.ToISO8601)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid to_iso8601: " + err.Error()})
			return
		}
	}

	if toMS < fromMS {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "to_iso8601 must be after from_iso8601"})
		return
	}

	speed := req.Speed
	if speed == 0 {
		speed = 1
	}
	if speed != 1 && speed != 2 && speed != 5 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "speed must be 1, 2, or 5"})
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

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"session_id": session.ID,
		"from_ms":    fromMS,
		"to_ms":      toMS,
		"speed":      speed,
		"state":      "paused",
	})
}

// stopSessionRequest represents the request body for POST /api/replay/stop.
type stopSessionRequest struct {
	// SessionID is the ID of the session to stop.
	SessionID string `json:"session_id"`
}

// stopSession handles POST /api/replay/stop.
// Stops the specified replay session and deletes it.
func (h *ReplayHandler) stopSession(w http.ResponseWriter, r *http.Request) {
	var req stopSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	session, exists := h.sessions[req.SessionID]
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}

	session.State = "stopped"
	delete(h.sessions, req.SessionID)

	log.Printf("[INFO] Replay session stopped: %s", req.SessionID)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "stopped",
		"session": req.SessionID,
	})
}

// seekRequest represents the request body for POST /api/replay/seek.
type seekRequest struct {
	// SessionID is the ID of the session to seek within.
	SessionID string `json:"session_id"`
	// TimestampISO8601 is the target timestamp in ISO8601 format.
	TimestampISO8601 string `json:"timestamp_iso8601"`
}

// seek handles POST /api/replay/seek.
// Seeks to the specified timestamp within the session.
func (h *ReplayHandler) seek(w http.ResponseWriter, r *http.Request) {
	var req seekRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	session, exists := h.sessions[req.SessionID]
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
		return
	}

	targetMS, err := parseISO8601(req.TimestampISO8601)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid timestamp: " + err.Error()})
		return
	}

	if targetMS < session.FromMS || targetMS > session.ToMS {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "timestamp outside session range"})
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
	})

	log.Printf("[INFO] Replay session seeked: %s to %d", req.SessionID, targetMS)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "seeked",
		"current_ms":  targetMS,
		"frame_found": len(frameData) > 0,
	})
}

// tuneRequest represents the request body for POST /api/replay/tune.
type tuneRequest struct {
	// SessionID is the ID of the session to tune.
	SessionID string `json:"session_id"`
	// DeltaRMSThreshold is the motion detection threshold (0.001-1.0).
	DeltaRMSThreshold *float64 `json:"delta_rms_threshold,omitempty"`
	// TauS is the EMA baseline time constant in seconds (1-600).
	TauS *float64 `json:"tau_s,omitempty"`
	// FresnelDecay is the Fresnel zone weight decay rate (1.0-4.0).
	FresnelDecay *float64 `json:"fresnel_decay,omitempty"`
	// Subcarriers is the number of subcarriers for NBVI selection (8-47).
	Subcarriers *int `json:"n_subcarriers,omitempty"`
	// BreathingSensitivity is the breathing detection threshold in radians RMS (0.001-0.1).
	BreathingSensitivity *float64 `json:"breathing_sensitivity,omitempty"`
}

// tune handles POST /api/replay/tune.
// Updates pipeline parameters for the replay session.
func (h *ReplayHandler) tune(w http.ResponseWriter, r *http.Request) {
	var req tuneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	session, exists := h.sessions[req.SessionID]
	if !exists {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
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

	writeJSON(w, http.StatusOK, map[string]interface{}{
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
