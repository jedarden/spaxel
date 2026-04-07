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
//	POST   /api/ble/devices/preregister  — manually register a device by MAC address
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
	r.Get("/api/ble/devices/{mac}/history", h.getDeviceHistory)
	r.Get("/api/ble/devices/{mac}/aliases", h.getDeviceAliases)
	r.Put("/api/ble/devices/{mac}", h.updateDevice)
	r.Delete("/api/ble/devices/{mac}", h.archiveDevice)
	r.Post("/api/ble/devices/preregister", h.preregisterDevice)

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

// listDevices handles GET /api/ble/devices.
//
// Returns a list of all BLE devices seen by the system. Devices can be filtered
// by registration status (registered/discovered), time window (hours parameter),
// and archival status.
//
// Query parameters:
//   - registered: "true" to return only devices assigned to a person
//   - discovered: "true" to return only unassigned devices
//   - archived: "true" to include archived (soft-deleted) devices
//   - hours: time window in hours (default: 24)
//
// Response: JSON object with "devices" array and "privacy_notice" string.
// Each device includes: mac, name, label, manufacturer, device_type, device_name,
// person_id, person_name, rssi_min, rssi_max, rssi_avg, first_seen_at, last_seen_at,
// last_seen_node, is_archived, is_wearable, enabled, last_location.
//
// Status codes:
//   - 200: Success
//   - 500: Internal error
func (h *Handler) listDevices(w http.ResponseWriter, r *http.Request) {
	includeArchived := r.URL.Query().Get("archived") == "true"
	registered := r.URL.Query().Get("registered")
	discovered := r.URL.Query().Get("discovered")

	// Parse hours parameter (default: 24 hours)
	hoursStr := r.URL.Query().Get("hours")
	hours := 24 // Default to last 24 hours
	if hoursStr != "" {
		if n, err := strconv.Atoi(hoursStr); err == nil && n > 0 {
			hours = n
		}
	}

	var devices []DeviceRecord
	var err error

	// Filter by registration status and time window
	if registered == "true" {
		devices, err = h.registry.GetDevicesSeenInHours(hours, includeArchived)
		// Filter to only registered devices (has person_id)
		var registeredDevices []DeviceRecord
		for _, d := range devices {
			if d.PersonID != "" {
				registeredDevices = append(registeredDevices, d)
			}
		}
		devices = registeredDevices
	} else if discovered == "true" {
		devices, err = h.registry.GetDevicesSeenInHours(hours, includeArchived)
		// Filter to only unregistered devices (no person_id)
		var unregisteredDevices []DeviceRecord
		for _, d := range devices {
			if d.PersonID == "" {
				unregisteredDevices = append(unregisteredDevices, d)
			}
		}
		devices = unregisteredDevices
	} else {
		// Get all devices seen in the time window
		devices, err = h.registry.GetDevicesSeenInHours(hours, includeArchived)
	}

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

// getDevice handles GET /api/ble/devices/{mac}.
//
// Returns detailed information about a single BLE device by its MAC address.
// The MAC address should be in uppercase colon-separated hex format (e.g., "AA:BB:CC:DD:EE:FF").
//
// URL parameters:
//   - mac: BLE device MAC address
//
// Response: JSON device object with all fields including location history.
//
// Status codes:
//   - 200: Success, device found
//   - 404: Device not found
//   - 500: Internal error
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

// getDeviceHistory handles GET /api/ble/devices/{mac}/history.
//
// Returns the sighting history for a specific BLE device. This includes
// RSSI observations from nodes that have detected this device over time.
//
// URL parameters:
//   - mac: BLE device MAC address
//
// Query parameters:
//   - limit: maximum number of history entries to return (default: 100, max: 1000)
//
// Response: JSON object with "mac", "history" (array of sighting entries),
// and "limit" fields. Each history entry includes timestamp, rssi_dbm, and node_mac.
//
// Status codes:
//   - 200: Success
//   - 404: Device not found
//   - 500: Internal error
func (h *Handler) getDeviceHistory(w http.ResponseWriter, r *http.Request) {
	mac := chi.URLParam(r, "mac")

	// Parse limit parameter
	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
			limit = n
		}
	}

	history, err := h.registry.GetDeviceSightingHistory(mac, limit)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "device not found", http.StatusNotFound)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]interface{}{
		"mac":     mac,
		"history": history,
		"limit":   limit,
	})
}

type updateDeviceRequest struct {
	Label      string `json:"label"`       // User-assigned display label
	DeviceType string `json:"device_type"` // Device type (apple_phone, apple_watch, tile, etc.)
	PersonID   string `json:"person_id"`   // Person ID to assign device to
}

// updateDevice handles PUT /api/ble/devices/{mac}.
//
// Updates a BLE device's properties. This endpoint is used to set a human-readable
// label for a device and/or assign it to a person for identity tracking.
//
// URL parameters:
//   - mac: BLE device MAC address (uppercase colon-separated hex)
//
// Request body: JSON object with optional fields:
//   - label: User-assigned display label (e.g., "Alice's iPhone")
//   - device_type: Device type identifier (e.g., "apple_phone", "tile", "fitbit")
//   - person_id: UUID of person to assign this device to (must exist)
//
// Response: Updated device object as JSON.
//
// Status codes:
//   - 200: Success, device updated
//   - 400: Invalid request body or person_id not found
//   - 404: Device not found
//   - 500: Internal error
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

// ── Pre-registration endpoint ───────────────────────────────────────────────────────

type preregisterDeviceRequest struct {
	MAC   string `json:"mac"`
	Label string `json:"label"`
}

// preregisterDevice manually creates a device entry for a known MAC address.
// This is useful for pre-registering tracker tags that haven't been seen yet.
func (h *Handler) preregisterDevice(w http.ResponseWriter, r *http.Request) {
	var req preregisterDeviceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.MAC == "" {
		http.Error(w, "invalid request body: mac is required", http.StatusBadRequest)
		return
	}

	// Default label to MAC if not provided
	if req.Label == "" {
		req.Label = req.MAC
	}

	device, err := h.registry.PreregisterDevice(req.MAC, req.Label)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	writeJSON(w, device)
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
