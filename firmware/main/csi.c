#include "csi.h"
#include "spaxel.h"
#include "websocket.h"
#include "esp_log.h"
#include "esp_wifi.h"
#include "esp_timer.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "freertos/queue.h"
#include <string.h>

static const char *TAG = "csi";

// Global variance threshold for motion hints
float g_variance_threshold = 0.02f;

// CSI statistics
static csi_stats_t s_stats = {0};

// TX task handle
static TaskHandle_t s_tx_task = NULL;
static volatile bool s_tx_running = false;

// CSI data queue
static QueueHandle_t s_csi_queue = NULL;

// Amplitude history for on-device variance check
#define VARIANCE_WINDOW 100
static float s_amplitude_history[VARIANCE_WINDOW] = {0};
static int s_amplitude_idx = 0;
static int s_amplitude_count = 0;

// Passive mode BSSID filter
static uint8_t s_passive_bssid[6] = {0};
static bool s_passive_filter_enabled = false;

// Forward declarations
static void csi_rx_task(void *arg);
static void csi_tx_task(void *arg);
static float compute_amplitude_variance(float new_amp);
static void wifi_csi_cb(void *ctx, esp_wifi_csi_info_t *info);

esp_err_t csi_init(void) {
    // Create CSI queue
    s_csi_queue = xQueueCreate(SPAXEL_CSI_QUEUE_SIZE, sizeof(esp_wifi_csi_info_t *));
    if (!s_csi_queue) {
        ESP_LOGE(TAG, "Failed to create CSI queue");
        return ESP_ERR_NO_MEM;
    }

    // Configure CSI
    wifi_csi_config_t csi_config = {
        .lltf_en = true,
        .htltf_en = true,
        .stbc_htltf2_en = true,
        .ltf_merge_en = true,
        .channel_filter_en = false,
        .manu_scale = false,
        .shift = 0,
    };

    esp_err_t err = esp_wifi_set_csi_config(&csi_config);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to set CSI config: %s", esp_err_to_name(err));
        return err;
    }

    // Register CSI callback
    err = esp_wifi_set_csi_rx_cb(wifi_csi_cb, NULL);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to register CSI callback: %s", esp_err_to_name(err));
        return err;
    }

    // Enable CSI
    err = esp_wifi_set_csi(true);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to enable CSI: %s", esp_err_to_name(err));
        return err;
    }

    // Start RX task
    xTaskCreatePinnedToCore(csi_rx_task, "csi_rx", 8192, NULL, 5, NULL, 1);

    ESP_LOGI(TAG, "CSI initialized");
    return ESP_OK;
}

static void wifi_csi_cb(void *ctx, esp_wifi_csi_info_t *info) {
    s_stats.frames_received++;

    // Apply passive BSSID filter if enabled
    if (s_passive_filter_enabled) {
        if (memcmp(info->mac, s_passive_bssid, 6) != 0) {
            // Not from the AP we're tracking, skip
            s_stats.frames_dropped++;
            return;
        }
    }

    // Copy CSI info to queue (pointer to heap-allocated copy)
    esp_wifi_csi_info_t *copy = malloc(sizeof(esp_wifi_csi_info_t));
    if (copy) {
        memcpy(copy, info, sizeof(esp_wifi_csi_info_t));
        if (xQueueSend(s_csi_queue, &copy, 0) != pdPASS) {
            free(copy);
            s_stats.frames_dropped++;
        }
    } else {
        s_stats.frames_dropped++;
    }
}

static void csi_rx_task(void *arg) {
    esp_wifi_csi_info_t *info;

    while (1) {
        if (xQueueReceive(s_csi_queue, &info, portMAX_DELAY) == pdPASS) {
            // Process CSI frame
            int8_t *csi_data = info->buf;
            uint8_t n_sub = info->len / 2; // I/Q pairs

            if (n_sub > SPAXEL_CSI_MAX_SUBCARRIERS) {
                n_sub = SPAXEL_CSI_MAX_SUBCARRIERS;
            }

            // Compute average amplitude for variance tracking
            float amp_sum = 0;
            for (int i = 0; i < n_sub; i++) {
                int8_t i_val = csi_data[i * 2];
                int8_t q_val = csi_data[i * 2 + 1];
                amp_sum += sqrtf((float)i_val * i_val + (float)q_val * q_val);
            }
            float avg_amp = amp_sum / n_sub;
            float variance = compute_amplitude_variance(avg_amp);

            // Check for motion hint (if variance exceeds threshold at low rate)
            if (variance > g_variance_threshold && g_state.packet_rate < 20) {
                ESP_LOGD(TAG, "On-device motion hint: variance=%.4f", variance);
                websocket_send_motion_hint(variance);
            }

            // Send to mothership if connected
            if (websocket_is_connected()) {
                uint64_t timestamp = (uint64_t)esp_timer_get_time();

                // Determine peer MAC
                uint8_t peer_mac[6];
                if (g_state.role == NODE_ROLE_TX) {
                    // In TX mode, peer is ourselves
                    memcpy(peer_mac, g_state.mac, 6);
                } else {
                    // In RX/TX_RX mode, peer is the transmitter
                    memcpy(peer_mac, info->mac, 6);
                }

                esp_err_t err = websocket_send_csi(
                    peer_mac,
                    timestamp,
                    info->rx_ctrl.rssi,
                    info->rx_ctrl.noise_floor,
                    info->rx_ctrl.channel,
                    csi_data,
                    n_sub
                );

                if (err == ESP_OK) {
                    s_stats.frames_sent++;
                } else {
                    s_stats.frames_dropped++;
                }
            }

            free(info);
        }
    }
}

static float compute_amplitude_variance(float new_amp) {
    // Welford's online algorithm for variance
    s_amplitude_history[s_amplitude_idx] = new_amp;
    s_amplitude_idx = (s_amplitude_idx + 1) % VARIANCE_WINDOW;
    if (s_amplitude_count < VARIANCE_WINDOW) {
        s_amplitude_count++;
    }

    if (s_amplitude_count < 10) {
        return 0; // Not enough samples
    }

    // Compute variance over window
    float mean = 0;
    for (int i = 0; i < s_amplitude_count; i++) {
        mean += s_amplitude_history[i];
    }
    mean /= s_amplitude_count;

    float var_sum = 0;
    for (int i = 0; i < s_amplitude_count; i++) {
        float diff = s_amplitude_history[i] - mean;
        var_sum += diff * diff;
    }

    return var_sum / s_amplitude_count;
}

esp_err_t csi_set_role(node_role_t role, const uint8_t *passive_bssid) {
    ESP_LOGI(TAG, "Setting role: %s", node_role_str(role));

    // Handle passive mode filter
    if (role == NODE_ROLE_PASSIVE && passive_bssid) {
        memcpy(s_passive_bssid, passive_bssid, 6);
        s_passive_filter_enabled = true;
        ESP_LOGI(TAG, "Passive mode BSSID: %02X:%02X:%02X:%02X:%02X:%02X",
                 passive_bssid[0], passive_bssid[1], passive_bssid[2],
                 passive_bssid[3], passive_bssid[4], passive_bssid[5]);
    } else {
        s_passive_filter_enabled = false;
    }

    // Handle TX mode
    if (role == NODE_ROLE_TX || role == NODE_ROLE_TX_RX) {
        if (!s_tx_running) {
            csi_start_tx();
        }
    } else {
        if (s_tx_running) {
            csi_stop_tx();
        }
    }

    // Handle promiscuous mode for RX
    if (role == NODE_ROLE_RX || role == NODE_ROLE_TX_RX || role == NODE_ROLE_PASSIVE) {
        esp_wifi_set_promiscuous(true);
    } else {
        esp_wifi_set_promiscuous(false);
    }

    return ESP_OK;
}

esp_err_t csi_set_rate(uint8_t rate_hz) {
    if (rate_hz < 1 || rate_hz > 100) {
        return ESP_ERR_INVALID_ARG;
    }

    g_state.packet_rate = rate_hz;
    ESP_LOGI(TAG, "Setting rate: %d Hz", rate_hz);
    return ESP_OK;
}

esp_err_t csi_start_tx(void) {
    if (s_tx_running) {
        return ESP_OK;
    }

    s_tx_running = true;
    xTaskCreate(csi_tx_task, "csi_tx", 4096, NULL, 5, &s_tx_task);

    ESP_LOGI(TAG, "TX started");
    return ESP_OK;
}

esp_err_t csi_stop_tx(void) {
    s_tx_running = false;

    if (s_tx_task) {
        vTaskDelete(s_tx_task);
        s_tx_task = NULL;
    }

    ESP_LOGI(TAG, "TX stopped");
    return ESP_OK;
}

static void csi_tx_task(void *arg) {
    // TX task sends null data packets that other nodes can receive CSI from
    // Using ESP-NOW or custom packets

    uint8_t broadcast_mac[6] = {0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF};

    while (s_tx_running) {
        uint32_t interval_ms = 1000 / g_state.packet_rate;

        // Send a packet that other nodes can capture CSI from
        // ESP32-S3 doesn't have direct TX CSI, but we can send packets
        // that trigger CSI capture on receivers

        // For now, use a simple approach: send a null data frame
        // This requires being in station mode and associated
        // The actual implementation would use esp_wifi_80211_tx

        s_stats.tx_packets++;

        vTaskDelay(pdMS_TO_TICKS(interval_ms));
    }

    vTaskDelete(NULL);
}

void csi_get_stats(csi_stats_t *stats) {
    if (stats) {
        memcpy(stats, &s_stats, sizeof(csi_stats_t));
    }
}
