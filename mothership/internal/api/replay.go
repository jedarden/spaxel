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
	"github.com/spaxel/mothership/internal/localization"
	"github.com/spaxel/mothership/internal/replay"
	sigproc "github.com/spaxel/mothership/internal/signal"
)

// ReplayHandler manages CSI replay sessions.
type ReplayHandler struct {
	mu              sync.RWMutex
	worker          *replay.Worker
	sessions        map[string]*_replaySession
	nextID          int
	activeSessionID string // Currently active session for dashboard control
	settingsHandler SettingsPersister // For ApplyToLive functionality
}

// SettingsPersister is the interface for persisting replay parameters to live settings.
type SettingsPersister interface {
	Update(updates map[string]interface{}) error
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

// SessionInfo represents a public view of a replay session.
type SessionInfo struct {
	ID        string `json:"id"`
	FromMS    int64  `json:"from_ms"`
	ToMS      int64  `json:"to_ms"`
	CurrentMS int64  `json:"current_ms"`
	State     string `json:"state"`
}

// NewReplayHandler creates a new replay handler.
func NewReplayHandler(store replay.FrameReader) (*ReplayHandler, error) {
	// Create replay worker
	worker := replay.NewWorker(store, nil, nil) // processor and broadcaster set later

	return &ReplayHandler{
		worker:   worker,
		sessions: make(map[string]*_replaySession),
		nextID:    1,
	}, nil
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

// SetFusionEngine sets the fusion engine for replay blob generation.
func (h *ReplayHandler) SetFusionEngine(fusionEngine interface{}) {
	h.mu.Lock()
	defer h.mu.Unlock()
	// Type assertion to fusion engine interface
	if engine, ok := fusionEngine.(interface {
		Fuse(links []localization.LinkMotion) *localization.FusionResult
		SetNodePosition(mac string, x, z float64)
	}); ok {
		h.worker.SetFusionEngine(engine)
	}
}

// SetSettingsHandler sets the settings handler for ApplyToLive functionality.
func (h *ReplayHandler) SetSettingsHandler(settingsHandler SettingsPersister) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.settingsHandler = settingsHandler
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

	if _, err := h.worker.GetSession(req.SessionID); err != nil {
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

	// Convert replay blobs to API format
	blobs := make([]map[string]interface{}, 0, len(session.LastBlobs))
	for _, b := range session.LastBlobs {
		blob := map[string]interface{}{
			"id":                  b.ID,
			"x":                   b.X,
			"y":                   b.Y,
			"z":                   b.Z,
			"vx":                  b.VX,
			"vy":                  b.VY,
			"vz":                  b.VZ,
			"weight":              b.Weight,
			"posture":             b.Posture,
			"person_id":           b.PersonID,
			"person_label":        b.PersonLabel,
			"person_color":        b.PersonColor,
			"identity_confidence": b.IdentityConfidence,
			"identity_source":     b.IdentitySource,
			"trail":               b.Trail,
		}
		blobs = append(blobs, blob)
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
		"blobs":       blobs,
		"timestamp_ms": session.LastBlobTime,
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

// GetReplayPath returns the path to the CSI replay binary file.
func (h *ReplayHandler) GetReplayPath() string {
	return "" // The recording buffer manages the file
}

// GetStoreStats returns statistics about the replay store.
func (h *ReplayHandler) GetStoreStats() replay.StoreStats {
	return h.worker.GetStoreStats()
}

// GetSessions returns a list of all active replay sessions.
func (h *ReplayHandler) GetSessions() []SessionInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()

	sessions := make([]SessionInfo, 0, len(h.sessions))
	for _, s := range h.sessions {
		sessions = append(sessions, SessionInfo{
			ID:        s.ID,
			FromMS:    s.FromMS,
			ToMS:      s.ToMS,
			CurrentMS: s.CurrentMS,
			State:     s.State,
		})
	}
	return sessions
}

// SeekTo moves the active replay session to the target timestamp.
// Implements dashboard.ReplayHandler interface.
func (h *ReplayHandler) SeekTo(targetMS int64) error {
	h.mu.Lock()
	sessionID := h.activeSessionID
	h.mu.Unlock()

	if sessionID == "" {
		return fmt.Errorf("no active replay session")
	}

	return h.worker.Seek(sessionID, targetMS)
}

// Play starts playback of the active replay session at the specified speed.
// Implements dashboard.ReplayHandler interface.
func (h *ReplayHandler) Play(speed float64) error {
	h.mu.Lock()
	sessionID := h.activeSessionID
	h.mu.Unlock()

	if sessionID == "" {
		return fmt.Errorf("no active replay session")
	}

	// Convert float speed to int (1x=1, 2x=2, 0.5x=1 for now)
	speedInt := 1
	if speed >= 2.0 {
		speedInt = 2
	} else if speed >= 5.0 {
		speedInt = 5
	}

	// Set speed first
	if err := h.worker.SetPlaybackSpeed(sessionID, speedInt); err != nil {
		return err
	}

	// Then set state to playing
	return h.worker.SetState(sessionID, "playing")
}

// Pause pauses playback of the active replay session.
// Implements dashboard.ReplayHandler interface.
func (h *ReplayHandler) Pause() error {
	h.mu.Lock()
	sessionID := h.activeSessionID
	h.mu.Unlock()

	if sessionID == "" {
		return fmt.Errorf("no active replay session")
	}

	return h.worker.SetState(sessionID, "paused")
}

// SetParams updates the replay pipeline parameters for the active session.
// Implements dashboard.ReplayHandler interface.
func (h *ReplayHandler) SetParams(params *replay.TunableParams) error {
	h.mu.Lock()
	sessionID := h.activeSessionID
	h.mu.Unlock()

	if sessionID == "" {
		return fmt.Errorf("no active replay session")
	}

	// Convert TunableParams to map for worker
	paramMap := make(map[string]interface{})
	if params.DeltaRMSThreshold != nil {
		paramMap["delta_rms_threshold"] = *params.DeltaRMSThreshold
	}
	if params.TauS != nil {
		paramMap["tau_s"] = *params.TauS
	}
	if params.FresnelDecay != nil {
		paramMap["fresnel_decay"] = *params.FresnelDecay
	}
	if params.FresnelWeightSigma != nil {
		paramMap["fresnel_weight_sigma"] = *params.FresnelWeightSigma
	}
	if params.MinConfidence != nil {
		paramMap["min_confidence"] = *params.MinConfidence
	}
	if params.BreathingSensitivity != nil {
		paramMap["breathing_sensitivity"] = *params.BreathingSensitivity
	}
	if params.NSubcarriers != nil {
		paramMap["n_subcarriers"] = *params.NSubcarriers
	}

	return h.worker.UpdateParams(sessionID, paramMap)
}

// ApplyToLive copies the current replay parameters to the live configuration.
// Implements dashboard.ReplayHandler interface.
func (h *ReplayHandler) ApplyToLive() error {
	h.mu.Lock()
	sessionID := h.activeSessionID
	settingsHandler := h.settingsHandler
	h.mu.Unlock()

	if sessionID == "" {
		return fmt.Errorf("no active replay session")
	}

	if settingsHandler == nil {
		log.Printf("[WARN] ApplyToLive: No settings handler configured, parameters not persisted")
		return fmt.Errorf("settings handler not configured")
	}

	// Get the current session's parameters
	session, err := h.worker.GetSession(sessionID)
	if err != nil {
		return err
	}

	// Convert replay params to settings format
	updates := make(map[string]interface{})

	// Map replay parameters to live settings
	if val, ok := session.Params["delta_rms_threshold"]; ok {
		if f, ok := val.(float64); ok {
			updates["delta_rms_threshold"] = f
		}
	}
	if val, ok := session.Params["tau_s"]; ok {
		if f, ok := val.(float64); ok {
			updates["tau_s"] = f
		}
	}
	if val, ok := session.Params["fresnel_decay"]; ok {
		if f, ok := val.(float64); ok {
			updates["fresnel_decay"] = f
		}
	}
	if val, ok := session.Params["fresnel_weight_sigma"]; ok {
		if f, ok := val.(float64); ok {
			updates["fresnel_weight_sigma"] = f
		}
	}
	if val, ok := session.Params["min_confidence"]; ok {
		if f, ok := val.(float64); ok {
			updates["min_confidence"] = f
		}
	}
	if val, ok := session.Params["breathing_sensitivity"]; ok {
		if f, ok := val.(float64); ok {
			updates["breathing_sensitivity"] = f
		}
	}
	if val, ok := session.Params["n_subcarriers"]; ok {
		if i, ok := val.(int); ok {
			updates["n_subcarriers"] = i
		} else if f, ok := val.(float64); ok {
			updates["n_subcarriers"] = int(f)
		}
	}

	if len(updates) == 0 {
		log.Printf("[INFO] ApplyToLive: No replay parameters to apply")
		return nil
	}

	// Persist to settings database
	if err := settingsHandler.Update(updates); err != nil {
		log.Printf("[ERROR] ApplyToLive: Failed to persist parameters: %v", err)
		return fmt.Errorf("failed to persist parameters: %w", err)
	}

	log.Printf("[INFO] ApplyToLive: Applied replay parameters to live: %+v", updates)
	return nil
}

// SetSpeed changes the playback speed of the active replay session.
// Implements dashboard.ReplayHandler interface.
func (h *ReplayHandler) SetSpeed(speed float64) error {
	h.mu.Lock()
	sessionID := h.activeSessionID
	h.mu.Unlock()

	if sessionID == "" {
		return fmt.Errorf("no active replay session")
	}

	// Convert float speed to int
	speedInt := 1
	if speed >= 2.0 {
		speedInt = 2
	} else if speed >= 5.0 {
		speedInt = 5
	}

	return h.worker.SetPlaybackSpeed(sessionID, speedInt)
}

// SetActiveSession sets the active replay session for dashboard control.
func (h *ReplayHandler) SetActiveSession(sessionID string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.activeSessionID = sessionID
	log.Printf("[INFO] Active replay session set to: %s", sessionID)
}

// GetActiveSession returns the currently active replay session ID.
func (h *ReplayHandler) GetActiveSession() string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.activeSessionID
}
