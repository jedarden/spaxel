#pragma once

#include <stdbool.h>
#include <stdint.h>
#include "esp_err.h"

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
