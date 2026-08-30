#pragma once

#include <stdbool.h>
#include "esp_err.h"

// Watchdog timeout in seconds
// CRITICAL: Must be longer than SAFE_MODE_BOOT_GOOD_AFTER_S (60s) to avoid
// the ESPHome regression where the watchdog fired before the boot validation
// window completed, spuriously triggering OTA rollback.
// See: esphome/esphome#15767, ADR-004 / bf-2tgcx
#define SPAXEL_WATCHDOG_TIMEOUT_S 90

/**
 * Initialize task watchdog.
 *
 * Adds the main task to the task watchdog with a timeout of SPAXEL_WATCHDOG_TIMEOUT_S.
 * The watchdog will reset the device if the task does not reset (feed) the watchdog
 * within the timeout period.
 *
 * IMPORTANT: The watchdog timeout is intentionally longer than the boot validation
 * window (60s) to avoid the ESPHome regression. See SPAXEL_WATCHDOG_TIMEOUT_S.
 *
 * @return ESP_OK on success, error code otherwise
 */
esp_err_t watchdog_init(void);

/**
 * Feed (reset) the watchdog.
 *
 * Call this periodically from the main task to prevent watchdog reset.
 * The watchdog is automatically reset by the task watchdog subsystem.
 *
 * @return ESP_OK on success, error code otherwise
 */
esp_err_t watchdog_feed(void);
