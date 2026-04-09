// Package api provides REST API handlers for Spaxel CSI replay/time-travel.
package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/spaxel/mothership/internal/replay"
	sigproc "github.com/spaxel/mothership/internal/signal"
)

// ReplayHandler manages CSI replay sessions.
type ReplayHandler struct {
	mu       sync.RWMutex
	worker   *replay.Worker
	sessions map[string]*_replaySession
	nextID   int
}

// _replaySession represents an active replay session (API layer).
type _replaySession struct {
	ID        string
	FromMS    int64
	ToMS      int64
	CurrentMS int64
	Speed     int
	State     string // playing, paused, stopped
	Params    map[string]interface{}
	CreatedAt string
}

// NewReplayHandler creates a new replay handler.
func NewReplayHandler(store replay.RecordingStore) (*ReplayHandler, error) {
	// Create replay worker
	worker := replay.NewWorker(store, nil, nil) // processor and broadcaster set later

	return &ReplayHandler{
		worker:   worker,
		sessions: make(map[string]*_replaySession),
		nextID:    1,
	}
}

// SetProcessorManager sets the signal processing pipeline for the replay worker.
func (h *ReplayHandler) SetProcessorManager(pm interface{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Type assertion to signal.ProcessorManager
	if procMgr, ok := pm.(*sigproc.ProcessorManager); ok {
		h.worker.SetProcessorManager(procMgr)
	}
}

// SetBlobBroadcaster sets the blob broadcaster for replay results.
func (h *ReplayHandler) SetBlobBroadcaster(broadcaster replay.BlobBroadcaster) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.worker.SetBroadcaster(broadcaster)
}

// Start the replay worker.
func (h *ReplayHandler) Start() {
	h.worker.Start()
}

// Stop the replay worker.
func (h *ReplayHandler) Stop() {
	h.worker.Stop()
}

// Close closes the replay handler.
func (h *ReplayHandler) Close() error {
	h.Stop()
	return nil
}

// RegisterRoutes registers replay endpoints.
func (h *ReplayHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/replay/sessions", h.listSessions)
	r.Post("/api/replay/start", h.startSession)
	r.Post("/api/replay/stop", h.stopSession)
	r.Post("/api/replay/seek", h.seek)
	r.Post("/api/replay/tune", h.tune)
	r.Post("/api/replay/set-speed", h.setSpeed)
	r.Post("/api/replay/set-state", h.setState)

	// Session state endpoint for polling
	r.Get("/api/replay/session/{id}", h.getSessionState)
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
	stats := h.worker.GetStoreStats()

	h.mu.RLock()
	sessions := make([]*_replaySession, 0, len(h.sessions))
	for _, s := range h.sessions {
		sessions = append(sessions, s)
	}
	h.mu.RUnlock()

	// Get oldest and newest timestamps
	oldestTS := int64(0)
	newestTS := int64(0)
	if stats.HasData {
		// Scan to find timestamps
		h.scanTimestamps(&oldestTS, &newestTS)
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

// scanTimestamps scans the replay store to find oldest and newest timestamps.
func (h *ReplayHandler) scanTimestamps(oldest, newest *int64) {
	stats := h.worker.GetStoreStats()
	if !stats.HasData {
		return
	}

	// Scan for oldest and newest
	store := h.worker.GetStore()
	store.Scan(func(recvTimeNS int64, frame []byte) bool {
		recvMS := recvTimeNS / 1e6
		if *oldest == 0 || recvMS < *oldest {
			*oldest = recvMS
		}
		*newest = recvMS
		return true // continue to find newest
	})
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

	toMS := timeNowMillis()
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

	// Start session via worker
	sessionID, err := h.worker.StartSession(fromMS, toMS, speed)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Track session in API layer
	h.mu.Lock()
	session := &_replaySession{
		ID:        sessionID,
		FromMS:    fromMS,
		ToMS:      toMS,
		CurrentMS: fromMS,
		Speed:     speed,
		State:     "paused",
		Params:    make(map[string]interface{}),
		CreatedAt: formatTimestamp(fromMS),
	}
	h.sessions[sessionID] = session
	h.nextID++
	h.mu.Unlock()

	log.Printf("[INFO] Replay session started: %s (from %d to %d, speed %dx)",
		sessionID, fromMS, toMS, speed)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"session_id": sessionID,
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

	if err := h.worker.StopSession(req.SessionID); err != nil {
		if err.Error() == "session not found" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	h.mu.Lock()
	delete(h.sessions, req.SessionID)
	h.mu.Unlock()

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

	targetMS, err := parseISO8601(req.TimestampISO8601)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid timestamp: " + err.Error()})
		return
	}

	session, err := h.worker.GetSession(req.SessionID)
	if err != nil {
		if err.Error() == "session not found" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	fromMS := session.FromMS
	toMS := session.ToMS

	if targetMS < fromMS || targetMS > toMS {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "timestamp outside session range"})
		return
	}

	if err := h.worker.Seek(req.SessionID, targetMS); err != nil {
		if err.Error() == "timestamp outside session range" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "timestamp outside session range"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Update API layer session state
	h.mu.Lock()
	if s, exists := h.sessions[req.SessionID]; exists {
		s.CurrentMS = targetMS
		s.State = "paused"
	}
	h.mu.Unlock()

	log.Printf("[INFO] Replay session seeked: %s to %d", req.SessionID, targetMS)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":      "seeked",
		"current_ms":  targetMS,
		"frame_found": true,
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

	session, err := h.worker.GetSession(req.SessionID)
	if err != nil {
		if err.Error() == "session not found" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Build params map
	params := make(map[string]interface{})
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

	if err := h.worker.UpdateParams(req.SessionID, params); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	log.Printf("[INFO] Replay session tuned: %s params=%+v", req.SessionID, params)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "tuned",
		"params":  params,
		"session": req.SessionID,
	})
}

// setSpeedRequest represents the request body for POST /api/replay/set-speed.
type setSpeedRequest struct {
	// SessionID is the ID of the session to modify.
	SessionID string `json:"session_id"`
	// Speed is the playback speed multiplier: 1, 2, or 5.
	Speed int `json:"speed"`
}

// setSpeed handles POST /api/replay/set-speed.
// Changes the playback speed for a replay session.
func (h *ReplayHandler) setSpeed(w http.ResponseWriter, r *http.Request) {
	var req setSpeedRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	if req.Speed != 1 && req.Speed != 2 && req.Speed != 5 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "speed must be 1, 2, or 5"})
		return
	}

	if err := h.worker.SetPlaybackSpeed(req.SessionID, req.Speed); err != nil {
		if err.Error() == "session not found" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Update API layer session state
	h.mu.Lock()
	if s, exists := h.sessions[req.SessionID]; exists {
		s.Speed = req.Speed
	}
	h.mu.Unlock()

	log.Printf("[INFO] Replay session speed changed: %s to %dx", req.SessionID, req.Speed)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "speed_set",
		"session": req.SessionID,
		"speed":   req.Speed,
	})
}

// setStateRequest represents the request body for POST /api/replay/set-state.
type setStateRequest struct {
	// SessionID is the ID of the session to modify.
	SessionID string `json:"session_id"`
	// State is the playback state: "playing" or "paused".
	State string `json:"state"`
}

// setState handles POST /api/replay/set-state.
// Changes the playback state for a replay session.
func (h *ReplayHandler) setState(w http.ResponseWriter, r *http.Request) {
	var req setStateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body: " + err.Error()})
		return
	}

	if req.State != "playing" && req.State != "paused" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "state must be 'playing' or 'paused'"})
		return
	}

	if err := h.worker.SetState(req.SessionID, req.State); err != nil {
		if err.Error() == "session not found" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Update API layer session state
	h.mu.Lock()
	if s, exists := h.sessions[req.SessionID]; exists {
		s.State = req.State
	}
	h.mu.Unlock()

	log.Printf("[INFO] Replay session state changed: %s to %s", req.SessionID, req.State)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":  "state_set",
		"session": req.SessionID,
		"state":   req.State,
	})
}

// getSessionState handles GET /api/replay/session/{id}.
// Returns the current state of a replay session including blobs.
func (h *ReplayHandler) getSessionState(w http.ResponseWriter, r *http.Request) {
	sessionID := chi.URLParam(r, "id")

	session, err := h.worker.GetSession(sessionID)
	if err != nil {
		if err.Error() == "session not found" {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "session not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	// Update API layer session state
	h.mu.Lock()
	if s, exists := h.sessions[sessionID]; exists {
		s.CurrentMS = session.CurrentMS
		s.State = session.State
	}
	h.mu.Unlock()

	// Calculate progress
	duration := session.ToMS - session.FromMS
	progress := 0.0
	if duration > 0 {
		progress = float64(session.CurrentMS-session.FromMS) / float64(duration)
	}

	// Build response with session state and blobs
	response := map[string]interface{}{
		"session_id":  sessionID,
		"current_ms":  session.CurrentMS,
		"from_ms":     session.FromMS,
		"to_ms":       session.ToMS,
		"state":       session.State,
		"speed":       session.Speed,
		"progress":    progress,
		"params":      session.Params,
		"blobs":       []interface{}{}, // TODO: populate with actual blob data
	}

	writeJSON(w, http.StatusOK, response)
}

// Helper functions
func timeNowMillis() int64 {
	return time.Now().UnixNano() / 1e6
}

func parseISO8601(s string) (int64, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return 0, err
	}
	return t.UnixNano() / 1e6, nil
}

func formatTimestamp(ms int64) string {
	return time.Unix(ms/1000, (ms%1000)*1e6).Format(time.RFC3339Nano)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

// GetReplayPath returns the path to the CSI replay binary file.
func (h *ReplayHandler) GetReplayPath() string {
	return "" // The recording buffer manages the file
}

// GetStoreStats returns statistics about the replay store.
func (h *ReplayHandler) GetStoreStats() replay.Stats {
	return h.worker.GetStoreStats()
}
