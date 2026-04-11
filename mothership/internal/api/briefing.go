// Package api provides REST API handlers for morning briefings.
package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"
	"github.com/spaxel/mothership/internal/briefing"
)

// BriefingHandler manages morning briefing REST endpoints.
type BriefingHandler struct {
	generator     *briefing.Generator
	db            *sql.DB
	notifyService briefing.NotifyService
	zoneProvider  briefing.ZoneProvider
	personProvider briefing.PersonProvider
	predictionProvider briefing.PredictionProvider
	healthProvider briefing.HealthProvider
}

// NewBriefingHandler creates a new briefing handler.
func NewBriefingHandler(dataDir string) (*BriefingHandler, error) {
	gen, err := briefing.NewGenerator(dataDir + "/spaxel.db")
	if err != nil {
		return nil, err
	}

	// Open database connection for settings persistence
	db, err := sql.Open("sqlite", dataDir+"/spaxel.db")
	if err != nil {
		gen.Close()
		return nil, err
	}
	db.SetMaxOpenConns(1)

	return &BriefingHandler{
		generator: gen,
		db:        db,
	}, nil
}

// SetProviders sets the provider interfaces for briefing generation.
func (h *BriefingHandler) SetProviders(z briefing.ZoneProvider, p briefing.PersonProvider, pr briefing.PredictionProvider, hp briefing.HealthProvider) {
	h.zoneProvider = z
	h.personProvider = p
	h.predictionProvider = pr
	h.healthProvider = hp
	h.generator.SetProviders(z, p, pr, hp)
}

// Close closes the generator and database connection.
func (h *BriefingHandler) Close() error {
	var firstErr error
	if h.generator != nil {
		if err := h.generator.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if h.db != nil {
		if err := h.db.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// SetNotifyService sets the notification service for sending test notifications.
func (h *BriefingHandler) SetNotifyService(notifySvc briefing.NotifyService) {
	h.notifyService = notifySvc
}

// RegisterRoutes registers the briefing API routes.
func (h *BriefingHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/briefing", h.handleGetBriefing)
	r.Get("/api/briefing/today", h.handleGetTodayBriefing)
	r.Get("/api/briefing/{date}", h.handleGetBriefingByDate)
	r.Post("/api/briefing/generate", h.handleGenerateBriefing)
	r.Get("/api/briefing/latest", h.handleGetLatestBriefing)
	r.Post("/api/briefing/{id}/acknowledge", h.handleAcknowledgeBriefing)
	r.Get("/api/briefing/settings", h.handleGetSettings)
	r.Patch("/api/briefing/settings", h.handleUpdateSettings)
	r.Post("/api/briefing/test", h.handleTestNotification)
}

// handleGetBriefing returns the briefing for today or a specific date.
func (h *BriefingHandler) handleGetBriefing(w http.ResponseWriter, r *http.Request) {
	date := r.URL.Query().Get("date")
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}

	person := r.URL.Query().Get("person")

	b, err := h.generator.Get(date, person)
	if err != nil {
		http.Error(w, "Briefing not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, b)
}

// handleGetBriefingByDate returns the briefing for a specific date (RESTful path parameter).
func (h *BriefingHandler) handleGetBriefingByDate(w http.ResponseWriter, r *http.Request) {
	date := chi.URLParam(r, "date")
	if date == "" {
		http.Error(w, "Date parameter required", http.StatusBadRequest)
		return
	}

	person := r.URL.Query().Get("person")

	b, err := h.generator.Get(date, person)
	if err != nil {
		http.Error(w, "Briefing not found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, b)
}

// handleGenerateBriefing generates a new briefing for the given date.
func (h *BriefingHandler) handleGenerateBriefing(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Date   string `json:"date"`
		Person string `json:"person"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if req.Date == "" {
		req.Date = time.Now().Format("2006-01-02")
	}

	b, err := h.generator.Generate(req.Date, req.Person)
	if err != nil {
		log.Printf("[ERROR] Failed to generate briefing: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := h.generator.Save(b); err != nil {
		log.Printf("[ERROR] Failed to save briefing: %v", err)
		// Still return the briefing even if save failed
	}

	writeJSON(w, http.StatusOK, b)
}

// handleGetLatestBriefing returns the most recent briefing.
func (h *BriefingHandler) handleGetLatestBriefing(w http.ResponseWriter, r *http.Request) {
	b, err := h.generator.GetLatest()
	if err != nil {
		http.Error(w, "No briefing found", http.StatusNotFound)
		return
	}

	writeJSON(w, http.StatusOK, b)
}

// handleGetSettings returns briefing settings.
func (h *BriefingHandler) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	// Try to load settings from database
	var settingsJSON sql.NullString
	err := h.db.QueryRow("SELECT value_json FROM settings WHERE key = 'briefing_config'").Scan(&settingsJSON)

	settings := map[string]interface{}{
		"enabled":           true,
		"time":              "07:00",
		"push_notification": true,
		"auto_generate":     true,
	}

	if err == nil && settingsJSON.Valid {
		var savedConfig map[string]interface{}
		if err := json.Unmarshal([]byte(settingsJSON.String), &savedConfig); err == nil {
			if enabled, ok := savedConfig["enabled"].(bool); ok {
				settings["enabled"] = enabled
			}
			if timeStr, ok := savedConfig["time"].(string); ok {
				settings["time"] = timeStr
			}
			if push, ok := savedConfig["push_notification"].(bool); ok {
				settings["push_notification"] = push
			}
			if auto, ok := savedConfig["auto_generate"].(bool); ok {
				settings["auto_generate"] = auto
			}
		}
	}

	writeJSON(w, http.StatusOK, settings)
}

// handleUpdateSettings updates briefing settings.
func (h *BriefingHandler) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var settings map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// Validate settings
	if timeStr, ok := settings["time"].(string); ok {
		// Validate time format (HH:MM)
		var h, m int
		_, err := fmt.Sscanf(timeStr, "%d:%d", &h, &m)
		if err != nil || h < 0 || h > 23 || m < 0 || m > 59 {
			http.Error(w, "Invalid time format, use HH:MM", http.StatusBadRequest)
			return
		}
	}

	// Save to database settings table
	settingsJSON, err := json.Marshal(settings)
	if err != nil {
		log.Printf("[ERROR] Failed to marshal briefing settings: %v", err)
		http.Error(w, "Failed to save settings", http.StatusInternalServerError)
		return
	}

	_, err = h.db.Exec(`
		INSERT OR REPLACE INTO settings (key, value_json, updated_at)
		VALUES ('briefing_config', ?, strftime('%s', 'now') * 1000)
	`, string(settingsJSON))
	if err != nil {
		log.Printf("[ERROR] Failed to save briefing settings: %v", err)
		http.Error(w, "Failed to save settings", http.StatusInternalServerError)
		return
	}

	log.Printf("[INFO] Briefing settings updated: %+v", settings)

	// Update scheduler config if available
	// Note: The scheduler will pick up the new config on next check

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleTestNotification sends a test briefing notification.
func (h *BriefingHandler) handleTestNotification(w http.ResponseWriter, r *http.Request) {
	// Generate a test briefing for today
	date := time.Now().Format("2006-01-02")
	b, err := h.generator.Generate(date, "")
	if err != nil {
		log.Printf("[ERROR] Failed to generate test briefing: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Send via notification service if available
	if h.notifyService != nil {
		notif := briefing.Notification{
			Title:     "Morning Briefing (Test)",
			Body:       b.Content,
			Priority:  1,
			Tags:       []string{"briefing", "test"},
			Timestamp: time.Now(),
		}
		if err := h.notifyService.Send(notif); err != nil {
			log.Printf("[ERROR] Failed to send test notification: %v", err)
			writeJSON(w, http.StatusInternalServerError, map[string]interface{}{
				"status":   "error",
				"error":    err.Error(),
				"briefing": b,
			})
			return
		}
		log.Printf("[INFO] Test briefing notification sent")
	} else {
		log.Printf("[INFO] Test briefing notification (no notify service): %s", b.Content)
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":   "sent",
		"briefing": b,
	})
}

// handleGetTodayBriefing returns today's briefing, generating if needed.
func (h *BriefingHandler) handleGetTodayBriefing(w http.ResponseWriter, r *http.Request) {
	today := time.Now().Format("2006-01-02")
	person := r.URL.Query().Get("person")

	// First try to get existing briefing
	b, err := h.generator.Get(today, person)
	if err == nil {
		// Check if it's marked as delivered
		if !b.Delivered {
			// Mark as delivered on first fetch
			if err := h.generator.MarkDelivered(b.ID); err != nil {
				log.Printf("[WARN] Failed to mark briefing as delivered: %v", err)
			}
			b.Delivered = true
		}
		writeJSON(w, http.StatusOK, b)
		return
	}

	// No briefing exists, generate one
	b, err = h.generator.Generate(today, person)
	if err != nil {
		log.Printf("[ERROR] Failed to generate today's briefing: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Save the new briefing
	if err := h.generator.Save(b); err != nil {
		log.Printf("[ERROR] Failed to save briefing: %v", err)
	}

	writeJSON(w, http.StatusOK, b)
}

// handleAcknowledgeBriefing marks a briefing as acknowledged by the user.
func (h *BriefingHandler) handleAcknowledgeBriefing(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		http.Error(w, "Briefing ID required", http.StatusBadRequest)
		return
	}

	// Mark as acknowledged
	if err := h.generator.MarkAcknowledged(id); err != nil {
		log.Printf("[ERROR] Failed to acknowledge briefing: %v", err)
		http.Error(w, "Failed to acknowledge briefing", http.StatusInternalServerError)
		return
	}

	log.Printf("[INFO] Briefing %s acknowledged", id)

	writeJSON(w, http.StatusOK, map[string]string{"status": "acknowledged"})
}

// GetGenerator returns the underlying briefing generator.
func (h *BriefingHandler) GetGenerator() *briefing.Generator {
	return h.generator
}
