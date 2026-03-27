#pragma once

#include <stdint.h>
#include <stdbool.h>
#include "esp_err.h"

/**
 * Initialize WebSocket client.
 */
esp_err_t websocket_init(void);

/**
 * Connect to mothership WebSocket server.
 *
 * @param host Mothership IP or hostname
 * @param port Mothership port
 * @return true if connected successfully
 */
bool websocket_connect(const char *host, uint16_t port);

/**
 * Disconnect from mothership.
 */
void websocket_disconnect(void);

/**
 * Check if WebSocket is connected.
 */
bool websocket_is_connected(void);

/**
 * Send CSI binary frame to mothership.
 *
 * @param peer_mac MAC of transmitting peer
 * @param timestamp_us Timestamp in microseconds
 * @param rssi RSSI in dBm
 * @param noise_floor Noise floor in dBm
 * @param channel WiFi channel
 * @param iq_data Pointer to I/Q pairs (int8 pairs)
 * @param n_sub Number of subcarriers
 * @return ESP_OK on success
 */
esp_err_t websocket_send_csi(const uint8_t *peer_mac, uint64_t timestamp_us,
                             int8_t rssi, int8_t noise_floor, uint8_t channel,
                             const int8_t *iq_data, uint8_t n_sub);

/**
 * Send hello message to mothership.
 */
esp_err_t websocket_send_hello(void);

/**
 * Send health message to mothership.
 */
esp_err_t websocket_send_health(void);

/**
 * Send BLE scan results to mothership.
 */
esp_err_t websocket_send_ble(const char *devices_json);

/**
 * Send motion hint to mothership.
 *
 * Called when on-device amplitude variance exceeds the configured threshold.
 * The mothership uses this to ramp the node (and adjacent nodes) to RateActive
 * before the next server-side detection frame arrives.
 *
 * Rate-limited to at most one message per second.
 *
 * @param variance The measured amplitude variance that triggered the hint
 */
esp_err_t websocket_send_motion_hint(float variance);

/**
 * Send OTA status to mothership.
 *
 * @param state "downloading", "verifying", "writing", "rebooting", "failed"
 * @param progress_pct Progress percentage (0-100)
 * @param error Error string if state is "failed", NULL otherwise
 */
esp_err_t websocket_send_ota_status(const char *state, uint8_t progress_pct,
                                     const char *error);

/**
 * Handle incoming JSON message from mothership.
 *
 * @param json JSON string
 * @param len Length of JSON string
 */
void websocket_handle_message(const char *json, size_t len);
