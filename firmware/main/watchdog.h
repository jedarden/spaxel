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
 * Reconfigures the task watchdog to a timeout of SPAXEL_WATCHDOG_TIMEOUT_S and
 * subscribes the idle tasks of every core. It does NOT subscribe the calling
 * task: app_main returns once the worker tasks are spawned, so its task is
 * short-lived and can never feed. A subscribed task that never feeds is reset
 * by the TWDT after SPAXEL_WATCHDOG_TIMEOUT_S, which on a healthy node is a
 * guaranteed reboot loop — call watchdog_subscribe() from a long-lived task
 * and feed it instead.
 *
 * IMPORTANT: The watchdog timeout is intentionally longer than the boot validation
 * window (60s) to avoid the ESPHome regression. See SPAXEL_WATCHDOG_TIMEOUT_S.
 *
 * @return ESP_OK on success, error code otherwise
 */
esp_err_t watchdog_init(void);

/**
 * Subscribe the calling task to the task watchdog.
 *
 * Call this once, at the top of a task that runs for the lifetime of the node
 * and that will feed the watchdog on every pass of its loop. The TWDT resets
 * the device if the task goes a full SPAXEL_WATCHDOG_TIMEOUT_S without feeding.
 *
 * @return ESP_OK on success, error code otherwise
 */
esp_err_t watchdog_subscribe(void);

/**
 * Feed (reset) the watchdog.
 *
 * Call this periodically from a subscribed task — every iteration of its main
 * loop. Feeds are per-task: this resets only the entry for the calling task,
 * so a task that forgets to feed still trips the watchdog.
 *
 * @return ESP_OK on success, error code otherwise
 */
esp_err_t watchdog_feed(void);
