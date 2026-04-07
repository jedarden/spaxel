// Package api provides REST API handlers for Spaxel automation trigger volumes.
// This handler replaces the previous triggers.go implementation with
// proper shape_json-based volume geometry and state machine support.
package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/spaxel/mothership/internal/volume"

	"github.com/go-chi/chi"
)

// MQTTClient interface for MQTT publishing.
type MQTTClient interface {
	Publish(topic string, payload []byte) error
	IsConnected() bool
}

// NotificationClient interface for sending notifications.
type NotificationClient interface {
	SendViaChannel(channelType string, title, body string, data map[string]interface{}) error
}

// WSBroadcaster sends messages to dashboard WebSocket clients.
type WSBroadcaster interface {
	BroadcastAlert(alertID string, timestamp time.Time, severity, description string, acknowledged bool)
	BroadcastTriggerState(triggerID, name string, lastFired time.Time, enabled bool)
}

// VolumeTriggersHandler manages automation trigger volumes with 3D geometry.
type VolumeTriggersHandler struct {
	mu          sync.RWMutex
	store       *volume.Store
	httpClient  *http.Client
	mqttClient   MQTTClient
	notifyClient NotificationClient
	wsBroadcaster WSBroadcaster
}

// TriggerResponse represents a trigger as returned by the API.
type TriggerResponse struct {
	ID             string                  `json:"id"`
	Name           string                  `json:"name"`
	Shape          volume.ShapeJSON        `json:"shape"`
	Condition      string                  `json:"condition"`
	ConditionParams volume.ConditionParams `json:"condition_params"`
	TimeConstraint *volume.TimeConstraint `json:"time_constraint,omitempty"`
	Actions        []volume.Action         `json:"actions"`
	Enabled        bool                    `json:"enabled"`
	ErrorMessage  string                  `json:"error_message,omitempty"`
	ErrorCount     int                     `json:"error_count"`
	LastFired      *time.Time              `json:"last_fired,omitempty"`
	Elapsed        int                     `json:"elapsed,omitempty"` // seconds since last fire
	CreatedAt      time.Time               `json:"created_at"`
	UpdatedAt      time.Time               `json:"updated_at"`
}

// WebhookTestResult is returned by the test endpoint.
type WebhookTestResult struct {
	Status    string        `json:"status"`
	ResponseMs int64         `json:"response_ms"`
	Error     string        `json:"error,omitempty"`
	Actions   []ActionResult `json:"actions"`
}

// ActionResult represents the outcome of executing a single action.
type ActionResult struct {
	Type      string `json:"type"`
	URL       string `json:"url,omitempty"`
	Status    int    `json:"status,omitempty"`
	ResponseMs int64  `json:"response_ms,omitempty"`
	Error     string `json:"error,omitempty"`
}

// NewVolumeTriggersHandler creates a new triggers handler with volume support.
func NewVolumeTriggersHandler(dbPath string) (*VolumeTriggersHandler, error) {
	store, err := volume.NewStore(dbPath)
	if err != nil {
		return nil, err
	}

	h := &VolumeTriggersHandler{
		store: store,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}

	// Set up firing callback
	store.SetOnFired(h.onTriggerFired)

	return h, nil
}

// SetMQTTClient sets the MQTT client for action execution.
func (h *VolumeTriggersHandler) SetMQTTClient(client MQTTClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.mqttClient = client
}

// SetNotificationClient sets the notification client for action execution.
func (h *VolumeTriggersHandler) SetNotificationClient(client NotificationClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.notifyClient = client
}

// SetWSBroadcaster sets the WebSocket broadcaster for dashboard alerts.
func (h *VolumeTriggersHandler) SetWSBroadcaster(broadcaster WSBroadcaster) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.wsBroadcaster = broadcaster
}

// Close closes the underlying store.
func (h *VolumeTriggersHandler) Close() error {
	return h.store.Close()
}

// RegisterRoutes registers volume trigger endpoints.
//
// GET    /api/triggers              — list all triggers
// POST   /api/triggers              — create trigger
// GET    /api/triggers/{id}         — get single trigger
// PUT    /api/triggers/{id}         — update trigger
// DELETE /api/triggers/{id}         — delete trigger
// POST   /api/triggers/{id}/test    — fire webhook actions once with synthetic payload
// POST   /api/triggers/{id}/enable  — clear error state and re-enable
// POST   /api/triggers/{id}/disable — disable trigger
// GET    /api/triggers/{id}/webhook-log — last N webhook firings for a trigger
// GET    /api/triggers/log          — get recent firing log
func (h *VolumeTriggersHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/triggers", h.listTriggers)
	r.Post("/api/triggers", h.createTrigger)
	r.Get("/api/triggers/{id}", h.getTrigger)
	r.Put("/api/triggers/{id}", h.updateTrigger)
	r.Delete("/api/triggers/{id}", h.deleteTrigger)
	r.Post("/api/triggers/{id}/test", h.testTrigger)
	r.Post("/api/triggers/{id}/enable", h.enableTrigger)
	r.Post("/api/triggers/{id}/disable", h.disableTrigger)
	r.Get("/api/triggers/{id}/webhook-log", h.getWebhookLog)
	r.Get("/api/triggers/log", h.getTriggerLog)
}

func (h *VolumeTriggersHandler) listTriggers(w http.ResponseWriter, r *http.Request) {
	triggers := h.store.GetAll()

	response := make([]*TriggerResponse, 0, len(triggers))
	now := time.Now()

	for _, t := range triggers {
		resp := h.toResponse(t, now)
		response = append(response, resp)
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *VolumeTriggersHandler) getTrigger(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	trigger, err := h.store.Get(id)
	if err != nil {
		http.Error(w, "trigger not found", http.StatusNotFound)
		return
	}

	resp := h.toResponse(trigger, time.Now())
	writeJSON(w, http.StatusOK, resp)
}

type volumeCreateTriggerRequest struct {
	Name            string                   `json:"name"`
	Shape           volume.ShapeJSON         `json:"shape"`
	Condition       string                   `json:"condition"`
	ConditionParams volume.ConditionParams `json:"condition_params,omitempty"`
	TimeConstraint  *volume.TimeConstraint   `json:"time_constraint,omitempty"`
	Actions         []volume.Action          `json:"actions"`
	Enabled         *bool                    `json:"enabled,omitempty"`
}

func (h *VolumeTriggersHandler) createTrigger(w http.ResponseWriter, r *http.Request) {
	var req volumeCreateTriggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}

	// Validate shape
	if !h.isValidShape(&req.Shape) {
		http.Error(w, "invalid shape geometry", http.StatusBadRequest)
		return
	}

	// Validate condition
	validConditions := map[string]bool{
		"enter":  true,
		"leave":  true,
		"dwell":  true,
		"vacant": true,
		"count":  true,
	}
	if !validConditions[req.Condition] {
		http.Error(w, "condition must be one of: enter, leave, dwell, vacant, count", http.StatusBadRequest)
		return
	}

	// Set default enabled if not specified
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	trigger := &volume.Trigger{
		Name:            req.Name,
		Shape:           req.Shape,
		Condition:       req.Condition,
		ConditionParams: req.ConditionParams,
		TimeConstraint:  req.TimeConstraint,
		Actions:         req.Actions,
		Enabled:         enabled,
	}

	id, err := h.store.Create(trigger)
	if err != nil {
		http.Error(w, "failed to create trigger", http.StatusInternalServerError)
		log.Printf("[ERROR] Failed to create trigger: %v", err)
		return
	}

	// Fetch the created trigger to get the full object
	created, err := h.store.Get(id)
	if err != nil {
		http.Error(w, "failed to retrieve created trigger", http.StatusInternalServerError)
		return
	}

	resp := h.toResponse(created, time.Now())
	writeJSON(w, http.StatusCreated, resp)
}

type volumeUpdateTriggerRequest struct {
	Name            *string                  `json:"name,omitempty"`
	Shape           *volume.ShapeJSON        `json:"shape,omitempty"`
	Condition       *string                  `json:"condition,omitempty"`
	ConditionParams *volume.ConditionParams `json:"condition_params,omitempty"`
	TimeConstraint  *volume.TimeConstraint   `json:"time_constraint,omitempty"`
	Actions         *[]volume.Action         `json:"actions,omitempty"`
	Enabled         *bool                    `json:"enabled,omitempty"`
}

func (h *VolumeTriggersHandler) updateTrigger(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	trigger, err := h.store.Get(id)
	if err != nil {
		http.Error(w, "trigger not found", http.StatusNotFound)
		return
	}

	var req volumeUpdateTriggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Apply updates
	if req.Name != nil {
		trigger.Name = *req.Name
	}
	if req.Shape != nil {
		if !h.isValidShape(req.Shape) {
			http.Error(w, "invalid shape geometry", http.StatusBadRequest)
			return
		}
		trigger.Shape = *req.Shape
	}
	if req.Condition != nil {
		trigger.Condition = *req.Condition
	}
	if req.ConditionParams != nil {
		trigger.ConditionParams = *req.ConditionParams
	}
	if req.TimeConstraint != nil {
		trigger.TimeConstraint = req.TimeConstraint
	}
	if req.Actions != nil {
		trigger.Actions = *req.Actions
	}
	if req.Enabled != nil {
		trigger.Enabled = *req.Enabled
	}

	if err := h.store.Update(trigger); err != nil {
		http.Error(w, "failed to update trigger", http.StatusInternalServerError)
		log.Printf("[ERROR] Failed to update trigger: %v", err)
		return
	}

	resp := h.toResponse(trigger, time.Now())
	writeJSON(w, http.StatusOK, resp)
}

func (h *VolumeTriggersHandler) deleteTrigger(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.store.Delete(id); err != nil {
		http.Error(w, "failed to delete trigger", http.StatusInternalServerError)
		log.Printf("[ERROR] Failed to delete trigger: %v", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// testTrigger fires webhook actions once with a synthetic payload.
// Returns {status, response_ms, actions: [{type, url, status, response_ms, error}]}
func (h *VolumeTriggersHandler) testTrigger(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	trigger, err := h.store.Get(id)
	if err != nil {
		http.Error(w, "trigger not found", http.StatusNotFound)
		return
	}

	now := time.Now()

	// Build synthetic payload per spec:
	// {trigger_id, trigger_name, condition, blob_id, person, position:{x,y,z}, zone, dwell_s, timestamp_ms}
	payload := map[string]interface{}{
		"trigger_id":   trigger.ID,
		"trigger_name": trigger.Name,
		"condition":    trigger.Condition,
		"blob_id":      0,
		"person":       nil,
		"position": map[string]float64{"x": 0, "y": 0, "z": 0},
		"zone":         nil,
		"dwell_s":      0,
		"timestamp_ms":  now.UnixMilli(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, "failed to marshal test payload", http.StatusInternalServerError)
		return
	}

	// Execute each action and collect results
	var results []ActionResult
	totalStart := time.Now()

	for _, action := range trigger.Actions {
		result := ActionResult{Type: action.Type}

		switch action.Type {
		case "webhook":
			url, ok := action.Params["url"].(string)
			if ok && url != "" {
				result.URL = url
				statusCode, latencyMs, err := h.doWebhookPost(url, data, action.Params)
				result.Status = statusCode
				result.ResponseMs = latencyMs
				if err != nil {
					result.Error = err.Error()
				}
			} else {
				result.Error = "missing url"
			}

		case "mqtt":
			result.URL, _ = action.Params["topic"].(string)
			if result.URL == "" {
				result.URL = "(no topic)"
			}
			result.Error = "test mode — mqtt not executed"

		default:
			result.URL = "(n/a)"
			result.Error = "test mode — action type not executable"
		}

		results = append(results, result)
	}

	totalMs := time.Since(totalStart).Milliseconds()

	resp := WebhookTestResult{
		Status:    "ok",
		ResponseMs: totalMs,
		Actions:   results,
	}

	writeJSON(w, http.StatusOK, resp)
}

// enableTrigger clears error state and re-enables a trigger.
func (h *VolumeTriggersHandler) enableTrigger(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	if err := h.store.EnableTrigger(id); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Broadcast updated trigger state to dashboard
	h.broadcastTriggerState(id)

	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}

// disableTrigger disables a trigger.
func (h *VolumeTriggersHandler) disableTrigger(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	trigger, err := h.store.Get(id)
	if err != nil {
		http.Error(w, "trigger not found", http.StatusNotFound)
		return
	}

	trigger.Enabled = false
	if err := h.store.Update(trigger); err != nil {
		http.Error(w, "failed to update trigger", http.StatusInternalServerError)
		return
	}

	// Broadcast updated trigger state to dashboard
	h.broadcastTriggerState(id)

	writeJSON(w, http.StatusOK, map[string]interface{}{"status": "ok"})
}

// getWebhookLog returns the last N webhook firings for a specific trigger.
func (h *VolumeTriggersHandler) getWebhookLog(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	limitStr := r.URL.Query().Get("limit")
	limit := 20
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	entries := h.store.GetWebhookLog(id, limit)
	writeJSON(w, http.StatusOK, entries)
}

func (h *VolumeTriggersHandler) getTriggerLog(w http.ResponseWriter, r *http.Request) {
	// Get limit from query param (default 10, max 100)
	limitStr := r.URL.Query().Get("limit")
	limit := 10
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	records := h.store.GetRecentFirings(limit)

	// Convert to response format
	response := make([]map[string]interface{}, 0, len(records))
	for _, r := range records {
		response = append(response, map[string]interface{}{
			"trigger_id":   r.TriggerID,
			"trigger_name": r.TriggerName,
			"condition":    r.Condition,
			"fired_at":     r.FiredAt.Format(time.RFC3339),
		})
	}

	writeJSON(w, http.StatusOK, response)
}

// isValidShape validates that a shape has all required fields.
func (h *VolumeTriggersHandler) isValidShape(shape *volume.ShapeJSON) bool {
	switch shape.Type {
	case volume.ShapeBox:
		return shape.X != nil && shape.Y != nil && shape.Z != nil &&
			shape.W != nil && shape.D != nil && shape.H != nil &&
			*shape.W > 0 && *shape.D > 0 && *shape.H > 0
	case volume.ShapeCylinder:
		return shape.CX != nil && shape.CY != nil && shape.Z != nil &&
			shape.R != nil && shape.H != nil &&
			*shape.R > 0 && *shape.H > 0
	default:
		return false
	}
}

// toResponse converts a trigger to the API response format.
func (h *VolumeTriggersHandler) toResponse(t *volume.Trigger, now time.Time) *TriggerResponse {
	resp := &TriggerResponse{
		ID:             t.ID,
		Name:           t.Name,
		Shape:          t.Shape,
		Condition:      t.Condition,
		ConditionParams: t.ConditionParams,
		TimeConstraint: t.TimeConstraint,
		Actions:        t.Actions,
		Enabled:        t.Enabled,
		ErrorMessage:  t.ErrorMessage,
		ErrorCount:     t.ErrorCount,
		LastFired:      t.LastFired,
		CreatedAt:      t.CreatedAt,
		UpdatedAt:      t.UpdatedAt,
	}

	if t.LastFired != nil {
		resp.Elapsed = int(now.Sub(*t.LastFired).Seconds())
	}

	return resp
}

// EvaluateTriggers is called by the fusion engine to evaluate triggers.
// Returns a list of trigger IDs that fired.
func (h *VolumeTriggersHandler) EvaluateTriggers(blobs []volume.BlobPos) []string {
	return h.store.Evaluate(blobs, time.Now())
}

// IsInVolume checks if a point is inside a trigger volume.
func (h *VolumeTriggersHandler) IsInVolume(triggerID string, x, y, z float64) bool {
	return h.store.IsInVolume(triggerID, x, y, z)
}

// onTriggerFired is called by the volume store when a trigger fires.
func (h *VolumeTriggersHandler) onTriggerFired(event volume.FiredEvent) {
	t := h.store.GetTrigger(event.TriggerID)
	if t == nil {
		return
	}

	// Execute all actions
	for _, action := range t.Actions {
		h.executeAction(action, event)
	}

	// Broadcast trigger state to dashboard
	h.mu.RLock()
	broadcaster := h.wsBroadcaster
	h.mu.RUnlock()
	if broadcaster != nil {
		broadcaster.BroadcastTriggerState(t.ID, t.Name, event.Timestamp, t.Enabled)
	}

	log.Printf("[INFO] Trigger fired: %s (%s, %d blob(s))", t.Name, t.Condition, len(event.BlobIDs))
}

// executeAction executes a single trigger action.
func (h *VolumeTriggersHandler) executeAction(action volume.Action, event volume.FiredEvent) {
	switch action.Type {
	case "webhook":
		h.executeWebhook(action, event)
	case "mqtt":
		h.executeMQTT(action, event)
	case "ntfy", "pushover":
		h.executeNotification(action, event)
	}
}

// doWebhookPost sends an HTTP POST and returns status code, latency, error.
// This is the low-level webhook call shared by normal firing and test.
func (h *VolumeTriggersHandler) doWebhookPost(url string, data []byte, params map[string]interface{}) (statusCode int, latencyMs int64, err error) {
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return 0, 0, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Add custom headers from action params
	for k, v := range params {
		if k != "url" {
			if headerStr, ok := v.(string); ok {
				req.Header.Set(k, headerStr)
			}
		}
	}

	start := time.Now()
	resp, err := h.httpClient.Do(req)
	latencyMs = time.Since(start).Milliseconds()

	if err != nil {
		return 0, latencyMs, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()
	// Drain body to allow connection reuse
	io.Copy(io.Discard, resp.Body)

	return resp.StatusCode, latencyMs, nil
}

// executeWebhook sends an HTTP POST to a webhook URL with fault tolerance.
// - 4xx: disable trigger, set error_message, push WS alert, log webhook_log
// - 5xx / timeout: log warning, increment error_count, do NOT disable
// - 2xx: reset error_count, log webhook_log
func (h *VolumeTriggersHandler) executeWebhook(action volume.Action, event volume.FiredEvent) {
	url, ok := action.Params["url"].(string)
	if !ok || url == "" {
		log.Printf("[WARN] Webhook action missing URL")
		return
	}

	// Build payload per spec:
	// {trigger_id, trigger_name, condition, blob_id, person, position:{x,y,z}, zone, dwell_s, timestamp_ms}
	t := h.store.GetTrigger(event.TriggerID)
	payload := map[string]interface{}{
		"trigger_id":   event.TriggerID,
		"trigger_name":  t.Name,
		"condition":    t.Condition,
		"blob_id":      0,
		"person":       nil,
		"position":     map[string]float64{"x": 0, "y": 0, "z": 0},
		"zone":         nil,
		"dwell_s":      0,
		"timestamp_ms":  event.Timestamp.UnixMilli(),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[WARN] Failed to marshal webhook payload: %v", err)
		return
	}

	statusCode, latencyMs, reqErr := h.doWebhookPost(url, data, action.Params)
	firedAtMs := event.Timestamp.UnixMilli()

	// Log the attempt
	logErr := ""
	if reqErr != nil {
		logErr = reqErr.Error()
	}
	h.store.WriteWebhookLog(event.TriggerID, url, firedAtMs, statusCode, latencyMs, logErr)

	// Error handling based on response class
	if reqErr != nil {
		// Timeout or network error — treat as 5xx-equivalent
		log.Printf("[WARN] Webhook request failed for trigger %q: %v", t.Name, reqErr)
		h.store.IncrementErrorCount(event.TriggerID)
		return
	}

	if statusCode >= 400 && statusCode < 500 {
		// 4xx — client error, likely misconfigured URL
		errMsg := fmt.Sprintf("Webhook returned HTTP %d — trigger disabled. Fix the URL and re-enable.", statusCode)
		h.store.DisableTriggerWithError(event.TriggerID, errMsg)

		// Push WS alert to dashboard
		h.mu.RLock()
		broadcaster := h.wsBroadcaster
		h.mu.RUnlock()
		if broadcaster != nil {
			broadcaster.BroadcastAlert("trigger_disabled", time.Now(), "warning", fmt.Sprintf("Trigger %q disabled: webhook returned HTTP %d", t.Name, statusCode), false)
		}
		return
	}

	if statusCode >= 500 {
		// 5xx — server error, transient
		log.Printf("[WARN] Webhook returned HTTP %d for trigger %q (server error, not disabling)", statusCode, t.Name)
		h.store.IncrementErrorCount(event.TriggerID)
		return
	}

	// 2xx — success, reset error count
	h.store.ResetErrorCount(event.TriggerID)
	log.Printf("[INFO] Webhook delivered for trigger %q (HTTP %d, %dms)", t.Name, statusCode, latencyMs)
}

// executeMQTT publishes to an MQTT topic.
func (h *VolumeTriggersHandler) executeMQTT(action volume.Action, event volume.FiredEvent) {
	h.mu.RLock()
	client := h.mqttClient
	h.mu.RUnlock()

	if client == nil {
		log.Printf("[WARN] MQTT client not configured")
		return
	}

	if !client.IsConnected() {
		log.Printf("[WARN] MQTT client not connected")
		return
	}

	topic, ok := action.Params["topic"].(string)
	if !ok || topic == "" {
		log.Printf("[WARN] MQTT action missing topic")
		return
	}

	t := h.store.GetTrigger(event.TriggerID)
	payload := map[string]interface{}{
		"trigger_id":   event.TriggerID,
		"trigger_name":  t.Name,
		"condition":    t.Condition,
		"fired_at":     event.Timestamp.Format(time.RFC3339),
		"blob_ids":     event.BlobIDs,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[WARN] Failed to marshal MQTT payload: %v", err)
		return
	}

	if err := client.Publish(topic, data); err != nil {
		log.Printf("[WARN] MQTT publish failed: %v", err)
	}
}

// executeNotification sends a push notification.
func (h *VolumeTriggersHandler) executeNotification(action volume.Action, event volume.FiredEvent) {
	h.mu.RLock()
	client := h.notifyClient
	h.mu.RUnlock()

	if client == nil {
		log.Printf("[WARN] Notification client not configured")
		return
	}

	title := fmt.Sprintf("Spaxel Trigger: %s", event.TriggerName)
	body := fmt.Sprintf("%s triggered (%s)", event.TriggerName, event.Condition)

	data := map[string]interface{}{
		"trigger_id":  event.TriggerID,
		"trigger_name": event.TriggerName,
		"condition":   event.Condition,
		"timestamp":   event.Timestamp.Unix(),
	}

	if err := client.SendViaChannel(action.Type, title, body, data); err != nil {
		log.Printf("[WARN] Notification failed: %v", err)
	}
}

// broadcastTriggerState sends a trigger_state WebSocket message for a trigger by ID.
func (h *VolumeTriggersHandler) broadcastTriggerState(triggerID string) {
	t := h.store.GetTrigger(triggerID)
	if t == nil {
		return
	}

	h.mu.RLock()
	broadcaster := h.wsBroadcaster
	h.mu.RUnlock()

	if broadcaster != nil {
		var lastFired time.Time
		if t.LastFired != nil {
			lastFired = *t.LastFired
		}
		broadcaster.BroadcastTriggerState(t.ID, t.Name, lastFired, t.Enabled)
	}
}

// GetTriggerStates returns all trigger states for the dashboard snapshot/delta protocol.
// Implements dashboard.TriggerState interface.
func (h *VolumeTriggersHandler) GetTriggerStates() []map[string]interface{} {
	triggers := h.store.GetAll()
	states := make([]map[string]interface{}, 0, len(triggers))
	for _, t := range triggers {
		state := map[string]interface{}{
			"id":      t.ID,
			"name":    t.Name,
			"enabled": t.Enabled,
		}
		if t.LastFired != nil {
			state["last_fired"] = t.LastFired.UnixMilli()
		}
		states = append(states, state)
	}
	return states
}
