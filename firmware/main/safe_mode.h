#pragma once

#include <stdbool.h>
#include "esp_err.h"

// Safe mode configuration (matching ESPHome safe_mode component defaults)
#define SAFE_MODE_BOOT_COUNT_THRESHOLD 10     // num_attempts: consecutive failures before safe mode
#define SAFE_MODE_BOOT_GOOD_AFTER_S 60       // boot_is_good_after: seconds before boot counts as good
#define SAFE_MODE_REBOOT_TIMEOUT_S 300       // reboot_timeout: seconds before exiting safe mode

// Safe mode state
typedef enum {
    SAFE_MODE_DISABLED = 0,  // Normal operation
    SAFE_MODE_ENABLED = 1,   // In safe mode (network + OTA only)
} safe_mode_state_t;

/**
 * Initialize safe mode subsystem.
 *
 * Loads boot counter from NVS and determines if we should boot into safe mode.
 * Must be called early in boot sequence, before initializing CSI/BLE.
 *
 * @return ESP_OK on success, error code otherwise
 */
esp_err_t safe_mode_init(void);

/**
 * Check if system is currently in safe mode.
 *
 * @return true if in safe mode, false otherwise
 */
bool safe_mode_is_active(void);

/**
 * Mark current boot as successful.
 *
 * Called after the device has stayed up for SAFE_MODE_BOOT_GOOD_AFTER_S seconds
 * without crashing or failing validation. Resets the boot counter to zero.
 *
 * @return ESP_OK on success, error code otherwise
 */
esp_err_t safe_mode_mark_boot_good(void);

/**
 * Mark current boot as failed.
 *
 * Increments the boot failure counter. If threshold is reached, enables safe mode
 * for the next boot.
 *
 * @return ESP_OK on success, error code otherwise
 */
esp_err_t safe_mode_mark_boot_failed(void);

/**
 * Enter safe mode immediately.
 *
 * Sets safe mode flag in NVS so next boot will be in safe mode.
 * Use this to proactively enter safe mode (e.g., from mothership command).
 *
 * @return ESP_OK on success, error code otherwise
 */
esp_err_t safe_mode_enter(void);

/**
 * Exit safe mode.
 *
 * Clears safe mode flag from NVS. System will boot normally next time.
 *
 * @return ESP_OK on success, error code otherwise
 */
esp_err_t safe_mode_exit(void);

/**
 * Get current boot failure count.
 *
 * @return Boot failure counter value (0 if not in NVS yet)
 */
uint32_t safe_mode_get_boot_count(void);

/**
 * Start the boot-good timer.
 *
 * Starts a one-shot timer that will call safe_mode_mark_boot_good() after
 * SAFE_MODE_BOOT_GOOD_AFTER_S seconds. If the device crashes or reboots before
 * the timer fires, the boot is not counted as good.
 *
 * @return ESP_OK on success, error code otherwise
 */
esp_err_t safe_mode_start_boot_good_timer(void);

/**
 * Stop the boot-good timer.
 *
 * Stops the timer if it's running.
 *
 * @return ESP_OK on success, error code otherwise
 */
esp_err_t safe_mode_stop_boot_good_timer(void);

/**
 * Start the safe mode exit timer.
 *
 * In safe mode, starts a timer that will reboot after SAFE_MODE_REBOOT_TIMEOUT_S
 * to exit safe mode and attempt normal boot.
 *
 * @return ESP_OK on success, error code otherwise
 */
esp_err_t safe_mode_start_exit_timer(void);
