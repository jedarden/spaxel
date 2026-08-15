#pragma once

#include <stdbool.h>
#include <stdint.h>
#include "esp_err.h"

/**
 * RESTART-SAFE PATTERN FOR WIFI OPERATIONS
 * ===========================================
 *
 * Any function that calls ESP-IDF WiFi APIs (esp_wifi_set_mode, esp_wifi_set_config,
 * esp_wifi_start, esp_wifi_connect, esp_wifi_stop) MUST check the restart guard before
 * invoking those APIs.
 *
 * WHY THIS IS NEEDED:
 * ESP-IDF WiFi APIs use ESP_ERROR_CHECK internally, which aborts the system if called
 * while WiFi is in an invalid state (e.g., during OTA download or system restart).
 *
 * THE PATTERN:
 * 1. Check g_state.restarting at the start of your WiFi operation function
 * 2. If true, log a warning and return ESP_OK (graceful skip, not an error)
 * 3. Otherwise, proceed with WiFi API calls normally
 *
 * EXAMPLE:
 * ```
 * esp_err_t my_wifi_operation(void) {
 *     // RESTART-SAFE GUARD
 *     if (g_state.restarting) {
 *         ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] Skipping operation - restart flag is set");
 *         return ESP_OK;  // Graceful skip, NOT an error
 *     }
 *
 *     // Safe to proceed with WiFi API calls
 *     esp_err_t err = esp_wifi_set_mode(WIFI_MODE_STA);
 *     // ... rest of implementation
 * }
 * ```
 *
 * WHERE THE RESTART FLAG IS SET:
 * - main.c:346 - REBOOT event handler (user/command-initiated)
 * - main.c:513 - Health task reboot (3-minute timeout)
 * - wifi.c:543 - Captive portal save reboot
 * - websocket.c:127 - OTA timeout restart
 * - websocket.c:833 - Reboot command from mothership
 * - websocket.c:1042 - OTA completion restart
 *
 * SEE ALSO:
 * - wifi.c:218 - Implementation example in wifi_start_connect()
 * - docs/notes/ota-wifi-reconnection-race-summary.md - Full design rationale
 * - firmware/test/test_ota_during_wifi_reconnect.c - Test coverage
 */

/**
 * Initialize WiFi subsystem.
 * Sets up WiFi stack, event handlers, and mDNS.
 */
esp_err_t wifi_init(void);

/**
 * Start WiFi connection using stored credentials.
 * Uses exponential backoff on failure.
 */
esp_err_t wifi_start_connect(void);

/**
 * Discover mothership via mDNS.
 *
 * @param ip_buf Buffer to store discovered IP address
 * @param buf_len Length of IP buffer
 * @param port Pointer to store discovered port (may be updated)
 * @return true if discovered, false otherwise
 */
bool wifi_discover_mothership(char *ip_buf, size_t buf_len, uint16_t *port);

/**
 * Start captive portal AP mode.
 * Creates AP "spaxel-XXXX" and serves config page.
 *
 * Idempotent: safe to call again while already running (tears down and
 * recreates the DNS/HTTP servers rather than failing to rebind them).
 *
 * @return ESP_OK if the AP, DNS server, and HTTP server all started; ESP_FAIL
 *         if DNS or HTTP could not bind (the AP/SSID may still be visible).
 */
esp_err_t wifi_start_captive_portal(void);

/**
 * Get current WiFi RSSI.
 * @return RSSI in dBm, or 0 if not connected
 */
int8_t wifi_get_rssi(void);

/**
 * Get current WiFi channel.
 * @return Channel number, or 0 if not connected
 */
uint8_t wifi_get_channel(void);

/**
 * Check if WiFi is connected.
 */
bool wifi_is_connected(void);

/**
 * Get AP BSSID (router MAC address).
 * @param bssid Buffer to store 6-byte BSSID
 * @return true if connected and BSSID retrieved, false otherwise
 */
bool wifi_get_ap_bssid(uint8_t *bssid);
