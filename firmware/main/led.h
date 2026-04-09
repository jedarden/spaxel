#pragma once

#include <stdint.h>
#include <stdbool.h>
#include "esp_err.h"

/**
 * Initialize LED control.
 * Sets up GPIO for LED blink (default GPIO8 for ESP32-S3-DevKitC).
 */
esp_err_t led_init(void);

/**
 * Start LED blink for identification.
 * Creates a FreeRTOS task that blinks the LED at ~5 Hz (100ms on/100ms off)
 * for the specified duration. Any running blink is cancelled first.
 *
 * @param duration_ms Blink duration in milliseconds (max 60000 = 60 seconds)
 * @return ESP_OK on success
 */
esp_err_t led_blink_identify(uint32_t duration_ms);

/**
 * Stop any running LED blink.
 */
void led_stop_blink(void);

/**
 * Check if LED is currently blinking.
 * @return true if blinking
 */
bool led_is_blinking(void);
