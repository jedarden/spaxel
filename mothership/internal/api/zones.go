// Package api provides REST API handlers for Spaxel zones and portals.
package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi"
	_ "modernc.org/sqlite"
)

// ZonesHandler manages zones and portals.
type ZonesHandler struct {
	mu      sync.RWMutex
	db      *sql.DB
	zones   map[string]*Zone
	portals map[string]*Portal
}

// Zone represents a spatial region.
type Zone struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	X               float64   `json:"x"`
	Y               float64   `json:"y"`
	Z               float64   `json:"z"`
	W               float64   `json:"w"`
	D               float64   `json:"d"`
	H               float64   `json:"h"`
	ZoneType       string    `json:"zone_type"`
	Occupancy      int       `json:"occupancy"`
	People         []string  `json:"people"`
	CreatedAt      time.Time `json:"created_at"`
}

// Portal represents a doorway between zones.
type Portal struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	ZoneA           string    `json:"zone_a"`
	ZoneB           string    `json:"zone_b"`
	Points          [2][2]float64 `json:"points"` // [[x1,y1], [x2,y2]]
	Crossings       int       `json:"crossings"`
	CreatedAt       time.Time `json:"created_at"`
}

// NewZonesHandler creates a new zones handler.
func NewZonesHandler(dbPath string) (*ZonesHandler, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	z := &ZonesHandler{
		db:      db,
		zones:   make(map[string]*Zone),
		portals: make(map[string]*Portal),
	}

	if err := z.migrate(); err != nil {
		db.Close()
		return nil, err
	}

	if err := z.loadZones(); err != nil {
		log.Printf("[WARN] Failed to load zones: %v", err)
	}
	if err := z.loadPortals(); err != nil {
		log.Printf("[WARN] Failed to load portals: %v", err)
	}

	return z, nil
}

func (z *ZonesHandler) migrate() error {
	_, err := z.db.Exec(`
		CREATE TABLE IF NOT EXISTS zones (
			id         TEXT PRIMARY KEY,
			name       TEXT    NOT NULL,
			x          REAL    NOT NULL DEFAULT 0,
			y          REAL    NOT NULL DEFAULT 0,
			z          REAL    NOT NULL DEFAULT 0,
			w          REAL    NOT NULL DEFAULT 1,
			d          REAL    NOT NULL DEFAULT 1,
			h          REAL    NOT NULL DEFAULT 1,
			zone_type  TEXT    NOT NULL DEFAULT 'general',
			created_at INTEGER NOT NULL DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS portals (
			id          TEXT PRIMARY KEY,
			name        TEXT    NOT NULL DEFAULT '',
			zone_a_id   TEXT    NOT NULL DEFAULT '',
			zone_b_id   TEXT    NOT NULL DEFAULT '',
			points_json TEXT    NOT NULL DEFAULT '[]',
			created_at INTEGER NOT NULL DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS portal_crossings (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			portal_id   TEXT    NOT NULL,
			timestamp_ms INTEGER NOT NULL,
			direction   TEXT    NOT NULL,
			blob_id     INTEGER,
			person      TEXT    DEFAULT '',
			FOREIGN KEY (portal_id) REFERENCES portals(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_portal_crossings_portal ON portal_crossings(portal_id);
		CREATE INDEX IF NOT EXISTS idx_portal_crossings_time ON portal_crossings(timestamp_ms);
	`)
	return err
}

func (z *ZonesHandler) loadZones() error {
	rows, err := z.db.Query(`SELECT id, name, x, y, z, w, d, h, zone_type, created_at FROM zones`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var zone Zone
		var createdNS int64
		if err := rows.Scan(&zone.ID, &zone.Name, &zone.X, &zone.Y, &zone.Z,
			&zone.W, &zone.D, &zone.H, &zone.ZoneType, &createdNS); err != nil {
			continue
		}
		zone.CreatedAt = time.Unix(0, createdNS)
		zone.People = []string{}
		z.zones[zone.ID] = &zone
	}
	return nil
}

func (z *ZonesHandler) loadPortals() error {
	rows, err := z.db.Query(`SELECT id, name, zone_a_id, zone_b_id, points_json, created_at FROM portals`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var portal Portal
		var pointsJSON string
		var createdNS int64
		if err := rows.Scan(&portal.ID, &portal.Name, &portal.ZoneA, &portal.ZoneB,
			&pointsJSON, &createdNS); err != nil {
			continue
		}

		if err := json.Unmarshal([]byte(pointsJSON), &portal.Points); err != nil {
			log.Printf("[WARN] Failed to parse portal points: %v", err)
			continue
		}

		portal.CreatedAt = time.Unix(0, createdNS)
		z.portals[portal.ID] = &portal
	}
	return nil
}

// Close closes the database.
func (z *ZonesHandler) Close() error {
	return z.db.Close()
}

// RegisterRoutes registers zones and portals endpoints.
//
// Zones:
//   GET  /api/zones              — list all zones
//   POST /api/zones              — create zone
//   PUT  /api/zones/{id}         — update zone
//   DELETE /api/zones/{id}       — delete zone
//   GET  /api/zones/{id}/history — zone occupancy history
//
// Portals:
//   GET  /api/portals            — list all portals
//   POST /api/portals            — create portal
//   PUT  /api/portals/{id}       — update
//   DELETE /api/portals/{id}     — delete
//   GET  /api/portals/{id}/crossings — portal crossing log
func (z *ZonesHandler) RegisterRoutes(r chi.Router) {
	// Zones
	r.Get("/api/zones", z.listZones)
	r.Post("/api/zones", z.createZone)
	r.Put("/api/zones/{id}", z.updateZone)
	r.Delete("/api/zones/{id}", z.deleteZone)
	r.Get("/api/zones/{id}/history", z.getZoneHistory)

	// Portals
	r.Get("/api/portals", z.listPortals)
	r.Post("/api/portals", z.createPortal)
	r.Put("/api/portals/{id}", z.updatePortal)
	r.Delete("/api/portals/{id}", z.deletePortal)
	r.Get("/api/portals/{id}/crossings", z.getPortalCrossings)
}

// ── Zones ───────────────────────────────────────────────────────────────────────

func (z *ZonesHandler) listZones(w http.ResponseWriter, r *http.Request) {
	z.mu.RLock()
	zones := make([]*Zone, 0, len(z.zones))
	for _, zone := range z.zones {
		zones = append(zones, zone)
	}
	z.mu.RUnlock()

	writeJSON(w, zones)
}

type createZoneRequest struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	X        float64 `json:"x"`
	Y        float64 `json:"y"`
	Z        float64 `json:"z"`
	W        float64 `json:"w"`
	D        float64 `json:"d"`
	H        float64 `json:"h"`
	ZoneType string  `json:"zone_type,omitempty"`
}

func (z *ZonesHandler) createZone(w http.ResponseWriter, r *http.Request) {
	var req createZoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if req.W <= 0 || req.D <= 0 || req.H <= 0 {
		http.Error(w, "dimensions must be positive", http.StatusBadRequest)
		return
	}

	zoneType := req.ZoneType
	if zoneType == "" {
		zoneType = "general"
	}

	now := time.Now().UnixNano()
	_, err := z.db.Exec(`
		INSERT INTO zones (id, name, x, y, z, w, d, h, zone_type, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, req.ID, req.Name, req.X, req.Y, req.Z, req.W, req.D, req.H, zoneType, now)
	if err != nil {
		http.Error(w, "failed to create zone", http.StatusInternalServerError)
		return
	}

	z.mu.Lock()
	z.zones[req.ID] = &Zone{
		ID:        req.ID,
		Name:      req.Name,
		X:         req.X,
		Y:         req.Y,
		Z:         req.Z,
		W:         req.W,
		D:         req.D,
		H:         req.H,
		ZoneType:  zoneType,
		CreatedAt: time.Unix(0, now),
		People:    []string{},
	}
	z.mu.Unlock()

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, z.zones[req.ID])
}

type updateZoneRequest struct {
	Name     *string  `json:"name,omitempty"`
	X        *float64 `json:"x,omitempty"`
	Y        *float64 `json:"y,omitempty"`
	Z        *float64 `json:"z,omitempty"`
	W        *float64 `json:"w,omitempty"`
	D        *float64 `json:"d,omitempty"`
	H        *float64 `json:"h,omitempty"`
	ZoneType *string  `json:"zone_type,omitempty"`
}

func (z *ZonesHandler) updateZone(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	z.mu.RLock()
	zone, exists := z.zones[id]
	z.mu.RUnlock()

	if !exists {
		http.Error(w, "zone not found", http.StatusNotFound)
		return
	}

	var req updateZoneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	updates := []string{}
	args := []interface{}{}

	if req.Name != nil {
		updates = append(updates, "name = ?")
		args = append(args, *req.Name)
	}
	if req.X != nil {
		updates = append(updates, "x = ?")
		args = append(args, *req.X)
	}
	if req.Y != nil {
		updates = append(updates, "y = ?")
		args = append(args, *req.Y)
	}
	if req.Z != nil {
		updates = append(updates, "z = ?")
		args = append(args, *req.Z)
	}
	if req.W != nil {
		if *req.W <= 0 {
			http.Error(w, "width must be positive", http.StatusBadRequest)
			return
		}
		updates = append(updates, "w = ?")
		args = append(args, *req.W)
	}
	if req.D != nil {
		if *req.D <= 0 {
			http.Error(w, "depth must be positive", http.StatusBadRequest)
			return
		}
		updates = append(updates, "d = ?")
		args = append(args, *req.D)
	}
	if req.H != nil {
		if *req.H <= 0 {
			http.Error(w, "height must be positive", http.StatusBadRequest)
			return
		}
		updates = append(updates, "h = ?")
		args = append(args, *req.H)
	}
	if req.ZoneType != nil {
		updates = append(updates, "zone_type = ?")
		args = append(args, *req.ZoneType)
	}

	if len(updates) == 0 {
		writeJSON(w, zone)
		return
	}

	args = append(args, id)
	query := "UPDATE zones SET " + joinComma(updates) + " WHERE id = ?"

	_, err := z.db.Exec(query, args...)
	if err != nil {
		http.Error(w, "failed to update zone", http.StatusInternalServerError)
		return
	}

	// Update in-memory copy
	z.mu.Lock()
	if req.Name != nil {
		zone.Name = *req.Name
	}
	if req.X != nil {
		zone.X = *req.X
	}
	if req.Y != nil {
		zone.Y = *req.Y
	}
	if req.Z != nil {
		zone.Z = *req.Z
	}
	if req.W != nil {
		zone.W = *req.W
	}
	if req.D != nil {
		zone.D = *req.D
	}
	if req.H != nil {
		zone.H = *req.H
	}
	if req.ZoneType != nil {
		zone.ZoneType = *req.ZoneType
	}
	z.mu.Unlock()

	writeJSON(w, zone)
}

func (z *ZonesHandler) deleteZone(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	z.mu.RLock()
	_, exists := z.zones[id]
	z.mu.RUnlock()

	if !exists {
		http.Error(w, "zone not found", http.StatusNotFound)
		return
	}

	_, err := z.db.Exec(`DELETE FROM zones WHERE id = ?`, id)
	if err != nil {
		http.Error(w, "failed to delete zone", http.StatusInternalServerError)
		return
	}

	z.mu.Lock()
	delete(z.zones, id)
	z.mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

type historyEntry struct {
	Timestamp int64    `json:"timestamp"`
	Count     int       `json:"count"`
	People   []string  `json:"people"`
}

func (z *ZonesHandler) getZoneHistory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	z.mu.RLock()
	_, exists := z.zones[id]
	z.mu.RUnlock()

	if !exists {
		http.Error(w, "zone not found", http.StatusNotFound)
		return
	}

	period := r.URL.Query().Get("period")
	limit := 24
	if period == "7d" {
		limit = 24 * 7
	} else if period == "30d" {
		limit = 24 * 30
	}

	// Generate synthetic history data (in real implementation, query from events)
	history := make([]historyEntry, limit)
	now := time.Now()
	for i := range history {
		h := historyEntry{
			Timestamp: now.Add(-time.Duration(i) * time.Hour).UnixNano() / 1e6,
			Count:     0,
			People:    []string{},
		}
		history[i] = h
	}

	writeJSON(w, history)
}

// ── Portals ─────────────────────────────────────────────────────────────────────

func (z *ZonesHandler) listPortals(w http.ResponseWriter, r *http.Request) {
	z.mu.RLock()
	portals := make([]*Portal, 0, len(z.portals))
	for _, portal := range z.portals {
		portals = append(portals, portal)
	}
	z.mu.RUnlock()

	writeJSON(w, portals)
}

type createPortalRequest struct {
	ID     string      `json:"id"`
	Name   string      `json:"name"`
	ZoneA  string      `json:"zone_a"`
	ZoneB  string      `json:"zone_b"`
	Points [2][2]float64 `json:"points"`
}

func (z *ZonesHandler) createPortal(w http.ResponseWriter, r *http.Request) {
	var req createPortalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if req.ZoneA == "" || req.ZoneB == "" {
		http.Error(w, "zone_a and zone_b are required", http.StatusBadRequest)
		return
	}

	z.mu.RLock()
	_, zoneAExists := z.zones[req.ZoneA]
	_, zoneBExists := z.zones[req.ZoneB]
	z.mu.RUnlock()

	if !zoneAExists || !zoneBExists {
		http.Error(w, "one or both zones not found", http.StatusBadRequest)
		return
	}

	pointsJSON, _ := json.Marshal(req.Points)
	now := time.Now().UnixNano()
	_, err := z.db.Exec(`
		INSERT INTO portals (id, name, zone_a_id, zone_b_id, points_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
	`, req.ID, req.Name, req.ZoneA, req.ZoneB, string(pointsJSON), now)
	if err != nil {
		http.Error(w, "failed to create portal", http.StatusInternalServerError)
		return
	}

	z.mu.Lock()
	z.portals[req.ID] = &Portal{
		ID:        req.ID,
		Name:      req.Name,
		ZoneA:     req.ZoneA,
		ZoneB:     req.ZoneB,
		Points:    req.Points,
		CreatedAt: time.Unix(0, now),
	}
	z.mu.Unlock()

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, z.portals[req.ID])
}

type updatePortalRequest struct {
	Name    *string         `json:"name,omitempty"`
	ZoneA   *string         `json:"zone_a,omitempty"`
	ZoneB   *string         `json:"zone_b,omitempty"`
	Points   *[2][2]float64 `json:"points,omitempty"`
}

func (z *ZonesHandler) updatePortal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	z.mu.RLock()
	portal, exists := z.portals[id]
	z.mu.RUnlock()

	if !exists {
		http.Error(w, "portal not found", http.StatusNotFound)
		return
	}

	var req updatePortalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	updates := []string{}
	args := []interface{}{}

	if req.Name != nil {
		updates = append(updates, "name = ?")
		args = append(args, *req.Name)
	}
	if req.ZoneA != nil {
		updates = append(updates, "zone_a_id = ?")
		args = append(args, *req.ZoneA)
	}
	if req.ZoneB != nil {
		updates = append(updates, "zone_b_id = ?")
		args = append(args, *req.ZoneB)
	}
	if req.Points != nil {
		pointsJSON, _ := json.Marshal(req.Points)
		updates = append(updates, "points_json = ?")
		args = append(args, string(pointsJSON))
	}

	if len(updates) == 0 {
		writeJSON(w, portal)
		return
	}

	args = append(args, id)
	query := "UPDATE portals SET " + joinComma(updates) + " WHERE id = ?"

	_, err := z.db.Exec(query, args...)
	if err != nil {
		http.Error(w, "failed to update portal", http.StatusInternalServerError)
		return
	}

	// Update in-memory copy
	z.mu.Lock()
	if req.Name != nil {
		portal.Name = *req.Name
	}
	if req.ZoneA != nil {
		portal.ZoneA = *req.ZoneA
	}
	if req.ZoneB != nil {
		portal.ZoneB = *req.ZoneB
	}
	if req.Points != nil {
		portal.Points = *req.Points
	}
	z.mu.Unlock()

	writeJSON(w, portal)
}

func (z *ZonesHandler) deletePortal(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	z.mu.RLock()
	_, exists := z.portals[id]
	z.mu.RUnlock()

	if !exists {
		http.Error(w, "portal not found", http.StatusNotFound)
		return
	}

	_, err := z.db.Exec(`DELETE FROM portals WHERE id = ?`, id)
	if err != nil {
		http.Error(w, "failed to delete portal", http.StatusInternalServerError)
		return
	}

	z.mu.Lock()
	delete(z.portals, id)
	z.mu.Unlock()

	w.WriteHeader(http.StatusNoContent)
}

type crossingEntry struct {
	ID        string    `json:"id"`
	Timestamp int64     `json:"timestamp_ms"`
	Direction string    `json:"direction"`
	Person    string    `json:"person,omitempty"`
}

func (z *ZonesHandler) getPortalCrossings(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	z.mu.RLock()
	_, exists := z.portals[id]
	z.mu.RUnlock()

	if !exists {
		http.Error(w, "portal not found", http.StatusNotFound)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
	}

	rows, err := z.db.Query(`
		SELECT id, timestamp_ms, direction, person
		FROM portal_crossings
		WHERE portal_id = ?
		ORDER BY timestamp_ms DESC
		LIMIT ?
	`, id, limit)
	if err != nil {
		http.Error(w, "failed to query crossings", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var crossings []crossingEntry
	for rows.Next() {
		var c crossingEntry
		if err := rows.Scan(&c.ID, &c.Timestamp, &c.Direction, &c.Person); err != nil {
			continue
		}
		crossings = append(crossings, c)
	}

	writeJSON(w, crossings)
}

// ── Occupancy updates (called by fusion engine) ───────────────────────────────────

// UpdateOccupancy updates the current occupancy for all zones.
func (z *ZonesHandler) UpdateOccupancy(occupancy map[string]int) {
	z.mu.Lock()
	defer z.mu.Unlock()

	for id, zone := range z.zones {
		count := occupancy[id]
		zone.Occupancy = count
	}
}

func joinComma(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	return result
}
