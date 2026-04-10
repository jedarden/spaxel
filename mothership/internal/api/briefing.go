// Package api provides REST API handlers for morning briefings.
package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"
	"github.com/spaxel/mothership/internal/briefing"
)

// BriefingHandler manages morning briefing REST endpoints.
type BriefingHandler struct {
	generator *briefing.Generator
	db        *sql.DB
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

// RegisterRoutes registers the briefing API routes.
func (h *BriefingHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/briefing", h.handleGetBriefing)
	r.Get("/api/briefing/{date}", h.handleGetBriefingByDate)
	r.Post("/api/briefing/generate", h.handleGenerateBriefing)
	r.Get("/api/briefing/latest", h.handleGetLatestBriefing)
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

	writeJSON(w, b)
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

	writeJSON(w, b)
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

	writeJSON(w, b)
}

// handleGetLatestBriefing returns the most recent briefing.
func (h *BriefingHandler) handleGetLatestBriefing(w http.ResponseWriter, r *http.Request) {
	b, err := h.generator.GetLatest()
	if err != nil {
		http.Error(w, "No briefing found", http.StatusNotFound)
		return
	}

	writeJSON(w, b)
}

// handleGetSettings returns briefing settings.
func (h *BriefingHandler) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	// For now, return default settings
	// TODO: Load from database settings table
	settings := map[string]interface{}{
		"enabled":        true,
		"time":           "07:00",
		"push_notification": true,
		"auto_generate":  true,
	}

	writeJSON(w, settings)
}

// handleUpdateSettings updates briefing settings.
func (h *BriefingHandler) handleUpdateSettings(w http.ResponseWriter, r *http.Request) {
	var settings map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	// TODO: Save to database settings table
	log.Printf("[INFO] Briefing settings updated: %+v", settings)

	writeJSON(w, map[string]string{"status": "ok"})
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

	// TODO: Send via notification service
	log.Printf("[INFO] Test briefing notification: %s", b.Content)

	writeJSON(w, map[string]interface{}{
		"status":   "sent",
		"briefing": b,
	})
}

// GetGenerator returns the underlying briefing generator.
func (h *BriefingHandler) GetGenerator() *briefing.Generator {
	return h.generator
}
