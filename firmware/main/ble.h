#pragma once

#include <stdint.h>
#include <stdbool.h>
#include "esp_err.h"

/**
 * Initialize BLE scanning subsystem.
 * Runs on Core 0 concurrent with WiFi.
 */
esp_err_t ble_init(void);

/**
 * Start BLE scanning.
 */
esp_err_t ble_start_scan(void);

/**
 * Stop BLE scanning.
 */
esp_err_t ble_stop_scan(void);

/**
 * Get discovered devices as JSON string.
 * Caller must free returned string.
 *
 * @return JSON array of discovered devices, or NULL on error
 */
char *ble_get_devices_json(void);

/**
 * BLE device info structure.
 */
typedef struct {
    uint8_t addr[6];
    uint8_t addr_type;  // 0=public, 1=random
    int8_t rssi;
    char name[32];
    uint16_t mfr_id;
    uint8_t mfr_data[32];
    uint8_t mfr_data_len;
} ble_device_t;

/**
 * Get number of discovered devices.
 */
int ble_get_device_count(void);

/**
 * Clear discovered devices cache.
 */
void ble_clear_devices(void);
