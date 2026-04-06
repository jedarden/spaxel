package ble

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi"
)

// Handler serves the BLE REST API.
type Handler struct {
	registry *Registry
}

// NewHandler creates a new BLE REST handler.
func NewHandler(registry *Registry) *Handler {
	return &Handler{registry: registry}
}

// RegisterRoutes mounts BLE endpoints on r.
//
//	GET    /api/ble/devices              — list all BLE devices
//	GET    /api/ble/devices/{mac}        — get single device
//	GET    /api/ble/devices/{mac}/aliases — get alias history for device
//	PUT    /api/ble/devices/{mac}        — update device (label, device_type, person_id)
//	DELETE /api/ble/devices/{mac}        — archive device (soft delete)
//	GET    /api/ble/duplicates           — list possible duplicate devices
//	POST   /api/ble/merge                — merge two devices (MAC rotation)
//	POST   /api/ble/split                — split alias from canonical device
//	GET    /api/people                   — list all people with device counts
//	POST   /api/people                   — create new person
//	GET    /api/people/{id}              — get single person with devices
//	PUT    /api/people/{id}              — update person name/color
//	DELETE /api/people/{id}              — delete person
func (h *Handler) RegisterRoutes(r chi.Router) {
	// Device endpoints
	r.Get("/api/ble/devices", h.listDevices)
	r.Get("/api/ble/devices/{mac}", h.getDevice)
	r.Get("/api/ble/devices/{mac}/aliases", h.getDeviceAliases)
	r.Put("/api/ble/devices/{mac}", h.updateDevice)
	r.Delete("/api/ble/devices/{mac}", h.archiveDevice)

	// Duplicate detection
	r.Get("/api/ble/duplicates", h.listDuplicates)
	r.Post("/api/ble/merge", h.mergeDevices)
	r.Post("/api/ble/split", h.splitDevice)

	// People endpoints
	r.Get("/api/people", h.listPeople)
	r.Post("/api/people", h.createPerson)
	r.Get("/api/people/{id}", h.getPerson)
	r.Put("/api/people/{id}", h.updatePerson)
	r.Delete("/api/people/{id}", h.deletePerson)
}

// ── Device endpoints ──────────────────────────────────────────────────────────

func (h *Handler) listDevices(w http.ResponseWriter, r *http.Request) {
	includeArchived := r.URL.Query().Get("archived") == "true"

	devices, err := h.registry.GetDevices(includeArchived)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if devices == nil {
		devices = []DeviceRecord{}
	}

	// Add privacy notice in response header
	w.Header().Set("X-Privacy-Notice", "Phones may appear multiple times due to address rotation. Wearables and AirTags have stable addresses.")

	writeJSON(w, map[string]interface{}{
		"devices":       devices,
		"privacy_notice": "Phones may appear multiple times due to address rotation. Wearables and AirTags have stable addresses.",
	})
}

func (h *Handler) getDevice(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")
	device, err := h.registry.GetDevice(mac)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, device)
}

type updateDeviceRequest struct {
	Label      string `json:"label"`
	DeviceType string `json:"device_type"`
	PersonID   string `json:"person_id"`
}

func (h *Handler) updateDevice(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")

	// Verify device exists
	if _, err := h.registry.GetDevice(mac); errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var req updateDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	updates := make(map[string]interface{})

	if req.Label != "" {
		updates["label"] = req.Label
	}
	if req.DeviceType != "" {
		updates["device_type"] = req.DeviceType
	}
	if req.PersonID != "" {
		// Verify person exists
		if _, err := h.registry.GetPerson(req.PersonID); err != nil {
			http.Error(w, "person not found", http.StatusBadRequest)
			return
		}
		updates["person_id"] = req.PersonID
	}

	if len(updates) > 0 {
		if err := h.registry.UpdateDevice(mac, updates); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}

	device, err := h.registry.GetDevice(mac)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, device)
}

func (h *Handler) archiveDevice(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")

	// Verify device exists
	if _, err := h.registry.GetDevice(mac); errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "device not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if err := h.registry.ArchiveDevice(mac); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Duplicate detection endpoints ─────────────────────────────────────────────

func (h *Handler) listDuplicates(w http.ResponseWriter, r *http.Request) {
	duplicates, err := h.registry.DetectPossibleDuplicates()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if duplicates == nil {
		duplicates = []PossibleDuplicate{}
	}
	writeJSON(w, map[string]interface{}{
		"duplicates": duplicates,
		"message":    "These devices may be the same device with a rotated MAC address. Review and merge if appropriate.",
	})
}

type mergeDevicesRequest struct {
	MAC1 string `json:"mac1"`
	MAC2 string `json:"mac2"`
}

func (h *Handler) mergeDevices(w http.ResponseWriter, r *http.Request) {
	var req mergeDevicesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MAC1 == "" || req.MAC2 == "" {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.MAC1 == req.MAC2 {
		http.Error(w, "cannot merge device with itself", http.StatusBadRequest)
		return
	}

	// Verify both devices exist
	if _, err := h.registry.GetDevice(req.MAC1); errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "device 1 not found", http.StatusNotFound)
		return
	}
	if _, err := h.registry.GetDevice(req.MAC2); errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "device 2 not found", http.StatusNotFound)
		return
	}

	if err := h.registry.MergeDevices(req.MAC1, req.MAC2); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	device, err := h.registry.GetDevice(req.MAC1)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]interface{}{
		"merged": device,
		"message": "Devices merged successfully. " + req.MAC2 + " has been removed.",
	})
}

// ── People endpoints ───────────────────────────────────────────────────────────

func (h *Handler) listPeople(w http.ResponseWriter, r *http.Request) {
	people, err := h.registry.GetPeopleWithDevices()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if people == nil {
		people = []map[string]interface{}{}
	}
	writeJSON(w, people)
}

type createPersonRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (h *Handler) createPerson(w http.ResponseWriter, r *http.Request) {
	var req createPersonRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Default color if not provided
	if req.Color == "" {
		req.Color = "#3b82f6"
	}

	person, err := h.registry.CreatePerson(req.Name, req.Color)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, person)
}

func (h *Handler) getPerson(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	person, err := h.registry.GetPerson(id)
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "person not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Get devices for this person
	devices, err := h.registry.GetPersonDevices(id)
	if err != nil {
		devices = nil
	}

	// Find most recent last_seen among devices
	var lastSeen time.Time
	for _, d := range devices {
		if d.LastSeenAt.After(lastSeen) {
			lastSeen = d.LastSeenAt
		}
	}

	writeJSON(w, map[string]interface{}{
		"id":           person.ID,
		"name":         person.Name,
		"color":        person.Color,
		"created_at":   person.CreatedAt,
		"device_count": len(devices),
		"devices":      devices,
		"last_seen":    lastSeen,
	})
}

type updatePersonRequest struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

func (h *Handler) updatePerson(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Verify person exists
	if _, err := h.registry.GetPerson(id); errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "person not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	var req updatePersonRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" && req.Color == "" {
		http.Error(w, "no updates provided", http.StatusBadRequest)
		return
	}

	if err := h.registry.UpdatePerson(id, req.Name, req.Color); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	person, err := h.registry.GetPerson(id)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, person)
}

func (h *Handler) deletePerson(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	// Verify person exists
	if _, err := h.registry.GetPerson(id); errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "person not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	if err := h.registry.DeletePerson(id); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Alias endpoints ─────────────────────────────────────────────────────────────

// getDeviceAliases returns the alias history for a device.
// This includes all rotated addresses that have been merged to this canonical device.
func (h *Handler) getDeviceAliases(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")

	// First check if this is an alias - resolve to canonical if so
	canonicalAddr := h.registry.ResolveAlias(mac)

	// Get aliases for the canonical address
	aliases, err := h.registry.GetAliases(canonicalAddr)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Build response with device info
	device, _ := h.registry.GetDevice(canonicalAddr)

	writeJSON(w, map[string]interface{}{
		"canonical_addr": canonicalAddr,
		"device":         device,
		"aliases":        aliases,
		"alias_count":    len(aliases),
		"note":           "Devices with auto-rotating addresses (phones) may have multiple historical addresses. Trackers typically have stable addresses.",
	})
}

type splitDeviceRequest struct {
	CanonicalAddr string `json:"canonical_addr"` // The canonical device address
	AliasAddr     string `json:"alias_addr"`      // The alias to split off
	NewPersonID   string `json:"new_person_id"`   // Optional: assign to different person
}

// splitDevice splits an alias from its canonical device, creating a separate device entry.
// Use this when a rotation merge was incorrect.
func (h *Handler) splitDevice(w http.ResponseWriter, r *http.Request) {
	var req splitDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.CanonicalAddr == "" || req.AliasAddr == "" {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.CanonicalAddr == req.AliasAddr {
		http.Error(w, "cannot split device from itself", http.StatusBadRequest)
		return
	}

	// Verify canonical device exists
	if _, err := h.registry.GetDevice(req.CanonicalAddr); errors.Is(err, sql.ErrNoRows) {
		http.Error(w, "canonical device not found", http.StatusNotFound)
		return
	}

	// Remove the alias relationship
	if err := h.registry.RemoveAlias(req.AliasAddr); err != nil {
		http.Error(w, "internal error removing alias", http.StatusInternalServerError)
		return
	}

	// If the alias has observations in ble_devices, update it
	// Create a proper device entry from the alias
	now := time.Now().UnixNano()
	_, err := h.registry.db.Exec(`
		UPDATE ble_devices SET
			person_id = ?,
			last_seen_at = ?
		WHERE mac = ?
	`, req.NewPersonID, now, req.AliasAddr)
	if err != nil {
		// Alias might not exist in ble_devices yet, which is fine
		// Create a new device entry
		_, err2 := h.registry.db.Exec(`
			INSERT INTO ble_devices (mac, person_id, last_seen_at, first_seen_at, enabled)
			VALUES (?, ?, ?, ?, 1)
		`, req.AliasAddr, req.NewPersonID, now, now)
		if err2 != nil {
			http.Error(w, "internal error creating device", http.StatusInternalServerError)
			return
		}
	}

	// Get the updated canonical device
	device, err := h.registry.GetDevice(req.CanonicalAddr)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Get the split device
	splitDevice, _ := h.registry.GetDevice(req.AliasAddr)

	writeJSON(w, map[string]interface{}{
		"canonical_device": device,
		"split_device":     splitDevice,
		"message":          "Successfully split " + req.AliasAddr + " from " + req.CanonicalAddr,
	})
}

// ── Utility endpoints ─────────────────────────────────────────────────────────

// ArchiveStaleHandler triggers archival of devices not seen for > 7 days.
func (h *Handler) ArchiveStaleHandler(w http.ResponseWriter, r *http.Request) {
	daysStr := r.URL.Query().Get("days")
	days := 7
	if daysStr != "" {
		if n, err := strconv.Atoi(daysStr); err == nil && n > 0 {
			days = n
		}
	}

	count, err := h.registry.ArchiveStale(time.Duration(days) * 24 * time.Hour)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"archived_count": count,
		"message":        "Archived " + strconv.FormatInt(count, 10) + " devices not seen in " + strconv.Itoa(days) + " days.",
	})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}
