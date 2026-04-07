#pragma once

#include <stdint.h>
#include <stdbool.h>
#include "esp_err.h"

/**
 * Initialize NTP client.
 * Sets up SNTP service with default configuration.
 */
esp_err_t ntp_init(void);

/**
 * Set NTP server and start synchronization.
 * Call this after WiFi connects.
 *
 * @param ntp_server NTP server hostname (e.g., "pool.ntp.org")
 * @return ESP_OK on success
 */
esp_err_t ntp_start_sync(const char *ntp_server);

/**
 * Wait for NTP synchronization to complete.
 *
 * @param timeout_ms Maximum time to wait in milliseconds
 * @return true if sync succeeded, false if timeout
 */
bool ntp_wait_sync(int timeout_ms);

/**
 * Check if NTP is synchronized.
 *
 * @return true if synchronized, false otherwise
 */
bool ntp_is_synced(void);

/**
 * Get current NTP sync status as a string.
 *
 * @return "synced" or "unsynced"
 */
const char* ntp_status_str(void);

/**
 * Start periodic NTP resync timer.
 * Resyncs every 10 minutes by default.
 */
void ntp_start_periodic_resync(void);

/**
 * Stop NTP and cleanup resources.
 */
void ntp_stop(void);
