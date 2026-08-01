#pragma once

#include <stdint.h>
#include <stdbool.h>
#include "esp_err.h"
#include "spaxel.h"

// Global variance threshold for on-device motion hints
extern float g_variance_threshold;

/**
 * Initialize CSI capture subsystem.
 * Sets up WiFi promiscuous mode and CSI callback.
 */
esp_err_t csi_init(void);

/**
 * Set node role for CSI capture.
 *
 * @param role Node role (TX, RX, TX_RX, PASSIVE, IDLE)
 * @param passive_bssid BSSID to filter for passive mode (NULL = disabled)
 */
esp_err_t csi_set_role(node_role_t role, const uint8_t *passive_bssid);

/**
 * Set CSI packet rate.
 *
 * @param rate_hz Rate in Hz (1-100)
 */
esp_err_t csi_set_rate(uint8_t rate_hz);

/**
 * Get CSI statistics.
 */
typedef struct {
    uint32_t frames_received;
    uint32_t frames_sent;
    uint32_t frames_dropped;
    uint32_t tx_packets;
} csi_stats_t;

void csi_get_stats(csi_stats_t *stats);

/**
 * Measured CSI frame rate in Hz, from frames actually sent since the previous
 * call.
 *
 * This is deliberately NOT the configured packet rate. Health reported
 * g_state.packet_rate — the target loaded from NVS — so a node capturing and
 * sending zero CSI frames reported 20 Hz forever, and a completely dead sensing
 * path looked perfectly healthy. See ADR-003 / bf-54cx2.
 */
uint32_t csi_measured_rate_hz(void);

/**
 * Start TX packet transmission.
 */
esp_err_t csi_start_tx(void);

/**
 * Stop TX packet transmission.
 */
esp_err_t csi_stop_tx(void);
