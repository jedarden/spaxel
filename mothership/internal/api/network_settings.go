package api

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// Settings keys used to persist the fleet-wide WiFi network. Stored through
// the shared SettingsHandler (not a separate table/cache) so that other
// in-process consumers — namely the provisioning server — see writes
// immediately via SettingsHandler.GetSingle without needing their own DB
// access or a second cache to keep in sync.
const (
	networkSettingWifiSSID     = "network_wifi_ssid"
	networkSettingWifiPassword = "network_wifi_password"
)

// NetworkSettingsHandler manages the fleet-wide WiFi network setting used to
// provision ESP32 nodes (ADR-005: configured once in the mothership
// dashboard rather than per-device at flash time).
type NetworkSettingsHandler struct {
	settings *SettingsHandler
}

// NewNetworkSettingsHandler creates a handler backed by the given settings store.
func NewNetworkSettingsHandler(settings *SettingsHandler) *NetworkSettingsHandler {
	return &NetworkSettingsHandler{settings: settings}
}

// networkSettingsResponse is returned by GET /api/settings/network.
// WifiPassword is intentionally omitted — write-only, never echoed back,
// matching the MQTT password convention in integrations.go.
type networkSettingsResponse struct {
	WifiSSID   string `json:"wifi_ssid"`
	Configured bool   `json:"configured"` // true once both SSID and password have been set
}

// networkSettingsRequest is the body for PUT /api/settings/network.
// Pointer fields distinguish "omitted" from "set to empty string" for partial updates.
type networkSettingsRequest struct {
	WifiSSID     *string `json:"wifi_ssid,omitempty"`
	WifiPassword *string `json:"wifi_password,omitempty"`
}

// RegisterRoutes registers network settings endpoints on the given router.
//
// Network Settings Endpoints:
//
//	GET /api/settings/network — Return the fleet WiFi SSID and whether credentials are configured
//
//	@Summary		Get network settings
//	@Description	Returns the fleet-wide WiFi SSID used to provision new nodes. The password
//	@Description	is never returned; "configured" indicates whether both SSID and password are set.
//	@Tags			settings
//	@Produce		json
//	@Success		200	{object}	networkSettingsResponse
//	@Router			/api/settings/network [get]
//
//	PUT /api/settings/network — Update the fleet WiFi SSID and/or password
//
//	@Summary		Update network settings
//	@Description	Updates the fleet-wide WiFi SSID and/or password used to provision new nodes.
//	@Description	Only the fields provided are modified.
//	@Tags			settings
//	@Accept			json
//	@Produce		json
//	@Param			request	body	networkSettingsRequest	true	"Network settings to update (partial)"
//	@Success		200	{object}	networkSettingsResponse
//	@Failure		400	{object}	map[string]string	"Invalid request body or validation error"
//	@Failure		500	{object}	map[string]string	"Failed to update network settings"
//	@Router			/api/settings/network [put]
func (h *NetworkSettingsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/api/settings/network", h.handleGet)
	r.Put("/api/settings/network", h.handleUpdate)
}

func (h *NetworkSettingsHandler) handleGet(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.response())
}

func (h *NetworkSettingsHandler) handleUpdate(w http.ResponseWriter, r *http.Request) {
	var req networkSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.WifiSSID != nil {
		ssid := strings.TrimSpace(*req.WifiSSID)
		if ssid == "" {
			writeJSONError(w, http.StatusBadRequest, "wifi_ssid: must not be empty")
			return
		}
		if len(ssid) > 32 {
			writeJSONError(w, http.StatusBadRequest, "wifi_ssid: must be 32 characters or fewer")
			return
		}
		if err := h.settings.Set(networkSettingWifiSSID, ssid); err != nil {
			log.Printf("[ERROR] Failed to save network settings: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to save network settings")
			return
		}
	}

	if req.WifiPassword != nil {
		pass := *req.WifiPassword
		if pass != "" && len(pass) < 8 {
			writeJSONError(w, http.StatusBadRequest, "wifi_password: must be at least 8 characters (WPA2 minimum) or empty for an open network")
			return
		}
		if err := h.settings.Set(networkSettingWifiPassword, pass); err != nil {
			log.Printf("[ERROR] Failed to save network settings: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "failed to save network settings")
			return
		}
	}

	writeJSON(w, http.StatusOK, h.response())
}

func (h *NetworkSettingsHandler) response() networkSettingsResponse {
	ssid, _ := h.settings.GetSingle(networkSettingWifiSSID)
	ssidStr, _ := ssid.(string)

	_, hasPass := h.settings.GetSingle(networkSettingWifiPassword)

	// Configured when SSID is set and password exists (even if empty for open networks)
	// Empty password is valid for open networks; missing password field shows unconfigured
	return networkSettingsResponse{
		WifiSSID:   ssidStr,
		Configured: ssidStr != "" && hasPass,
	}
}
