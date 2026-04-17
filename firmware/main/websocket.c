#include "websocket.h"
#include "spaxel.h"
#include "csi.h"
#include "wifi.h"
#include "ntp.h"
#include "led.h"
#include "esp_log.h"
#include "esp_timer.h"
#include "esp_system.h"
#include "esp_netif.h"
#include "esp_flash.h"
#include "driver/temperature_sensor.h"
#include "esp_ota_ops.h"
#include "esp_http_client.h"
#include "mbedtls/sha256.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "freertos/semphr.h"
#include "cJSON.h"
#include <string.h>
#include <strings.h>

// ESP-IDF WebSocket client
#include "esp_websocket_client.h"

static const char *TAG = "ws";

static esp_websocket_client_handle_t s_ws = NULL;
static SemaphoreHandle_t s_tx_mutex = NULL;
static volatile bool s_connected = false;

// OTA state
static char s_ota_url[256] = {0};
static char s_ota_sha256[65] = {0};
static char s_ota_version[32] = {0};

// OTA rollback confirmation — set once we receive a role message after connecting.
// Cancels the automatic rollback timer in the ESP-IDF OTA framework.
static bool s_ota_confirmed = false;

// One-shot timer: if role is not received within 60 s of connection,
// the new OTA partition stays unconfirmed and the bootloader will
// roll back to the previous partition on the next reset.
static esp_timer_handle_t s_ota_valid_timer = NULL;

#define SPAXEL_OTA_VALID_TIMEOUT_S 60

static void ota_validation_timeout_cb(void *arg) {
    if (!s_ota_confirmed) {
        ESP_LOGW(TAG, "OTA validation: timed out, rollback on next reset");
    }
}

static bool is_running_ota_partition(void) {
    const esp_partition_t *running = esp_ota_get_running_partition();
    return running != NULL &&
           running->type == ESP_PARTITION_TYPE_APP &&
           running->subtype != ESP_PARTITION_SUBTYPE_APP_FACTORY;
}

static void start_ota_validation_timer(void) {
    if (s_ota_valid_timer == NULL) {
        esp_timer_create_args_t timer_args = {
            .callback = ota_validation_timeout_cb,
            .name = "ota_valid",
        };
        esp_timer_create(&timer_args, &s_ota_valid_timer);
    }
    esp_timer_start_once(s_ota_valid_timer, SPAXEL_OTA_VALID_TIMEOUT_S * 1000000ULL);
    ESP_LOGI(TAG, "OTA validation: waiting for role message (timeout %ds)",
             SPAXEL_OTA_VALID_TIMEOUT_S);
}

static void stop_ota_validation_timer(void) {
    if (s_ota_valid_timer != NULL) {
        esp_timer_stop(s_ota_valid_timer);
    }
}

// Forward declarations
static void ws_event_handler(void *args, esp_event_base_t base,
                              int32_t id, void *data);
static void handle_role_msg(cJSON *root);
static void handle_config_msg(cJSON *root);
static void handle_ota_msg(cJSON *root);
static void handle_reboot_msg(cJSON *root);
static void handle_identify_msg(cJSON *root);
static void ota_task(void *arg);

esp_err_t websocket_init(void) {
    s_tx_mutex = xSemaphoreCreateMutex();
    if (!s_tx_mutex) {
        ESP_LOGE(TAG, "Failed to create TX mutex");
        return ESP_ERR_NO_MEM;
    }
    return ESP_OK;
}

bool websocket_connect(const char *host, uint16_t port) {
    if (s_ws) {
        websocket_disconnect();
        vTaskDelay(pdMS_TO_TICKS(100));
    }

    // Build WebSocket URI
    char uri[128];
    snprintf(uri, sizeof(uri), "ws://%s:%d%s", host, port, SPAXEL_WS_PATH);

    ESP_LOGI(TAG, "Connecting to %s", uri);

    // Configure WebSocket client
    esp_websocket_client_config_t cfg = {
        .uri = uri,
        .reconnect_timeout_ms = 5000,
        .network_timeout_ms = 30000,
        .ping_interval_sec = 30,
        .task_stack = 8192,
        .buffer_size = 2048,
    };

    // Add auth header if we have a token
    // Note: esp_websocket_client doesn't directly support custom headers,
    // so we'd use the URI query param or implement a custom handshake
    // For now, we'll use a simplified approach

    s_ws = esp_websocket_client_init(&cfg);
    if (!s_ws) {
        ESP_LOGE(TAG, "Failed to init WebSocket client");
        return false;
    }

    // Register event handlers
    esp_websocket_register_events(s_ws, WEBSOCKET_EVENT_ANY, ws_event_handler, NULL);

    // Connect
    esp_err_t err = esp_websocket_client_start(s_ws);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to start WebSocket: %s", esp_err_to_name(err));
        esp_websocket_client_destroy(s_ws);
        s_ws = NULL;
        return false;
    }

    // Wait for connection
    int timeout = 50; // 5 seconds
    while (!s_connected && timeout-- > 0) {
        vTaskDelay(pdMS_TO_TICKS(100));
    }

    if (!s_connected) {
        ESP_LOGE(TAG, "WebSocket connection timeout");
        websocket_disconnect();
        return false;
    }

    // Send hello message
    websocket_send_hello();

    // If booting from an unconfirmed OTA partition, start a 60 s timer.
    // If the mothership doesn't send a role message within that window,
    // the partition stays unconfirmed and the bootloader will roll back
    // to the previous firmware on the next reset.
    if (!s_ota_confirmed && is_running_ota_partition()) {
        start_ota_validation_timer();
    }

    return true;
}

void websocket_disconnect(void) {
    if (s_ws) {
        esp_websocket_client_stop(s_ws);
        esp_websocket_client_destroy(s_ws);
        s_ws = NULL;
    }
    s_connected = false;
    stop_ota_validation_timer();

    // Stop any running LED blink on disconnect
    led_stop_blink();
}

bool websocket_is_connected(void) {
    return s_connected && s_ws != NULL;
}

static void ws_event_handler(void *args, esp_event_base_t base,
                              int32_t id, void *data) {
    esp_websocket_event_data_t *event = (esp_websocket_event_data_t *)data;

    switch (id) {
        case WEBSOCKET_EVENT_CONNECTED:
            ESP_LOGI(TAG, "WebSocket connected");
            s_connected = true;
            break;

        case WEBSOCKET_EVENT_DISCONNECTED:
            ESP_LOGW(TAG, "WebSocket disconnected");
            s_connected = false;
            xEventGroupSetBits(g_state.events, SPAXEL_EVENT_WS_DISCONNECTED);
            break;

        case WEBSOCKET_EVENT_DATA:
            // Handle incoming message
            if (event->data_len > 0 && event->op_code == 0x01) {
                // Text frame (JSON)
                char *json = malloc(event->data_len + 1);
                if (json) {
                    memcpy(json, event->data_ptr, event->data_len);
                    json[event->data_len] = '\0';
                    websocket_handle_message(json, event->data_len);
                    free(json);
                }
            }
            break;

        case WEBSOCKET_EVENT_ERROR:
            ESP_LOGE(TAG, "WebSocket error");
            break;

        default:
            break;
    }
}

esp_err_t websocket_send_csi(const uint8_t *peer_mac, uint64_t timestamp_us,
                              int8_t rssi, int8_t noise_floor, uint8_t channel,
                              const int8_t *iq_data, uint8_t n_sub) {
    if (!s_connected || !s_ws) {
        return ESP_ERR_INVALID_STATE;
    }

    // Build binary frame
    // Header: 24 bytes
    // Payload: n_sub * 2 bytes
    size_t frame_len = SPAXEL_FRAME_HEADER_SIZE + (n_sub * 2);
    uint8_t *frame = malloc(frame_len);
    if (!frame) {
        return ESP_ERR_NO_MEM;
    }

    // Pack header (little-endian)
    memcpy(frame + 0, g_state.mac, 6);              // node_mac
    memcpy(frame + 6, peer_mac, 6);                  // peer_mac
    memcpy(frame + 12, &timestamp_us, 8);            // timestamp_us
    frame[20] = (uint8_t)rssi;                       // rssi (int8 as uint8)
    frame[21] = (uint8_t)noise_floor;                // noise_floor
    frame[22] = channel;                             // channel
    frame[23] = n_sub;                               // n_sub

    // Pack I/Q payload
    memcpy(frame + 24, iq_data, n_sub * 2);

    // Send binary frame
    xSemaphoreTake(s_tx_mutex, portMAX_DELAY);
    int sent = esp_websocket_client_send_bin(s_ws, (char *)frame, frame_len,
                                              portMAX_DELAY);
    xSemaphoreGive(s_tx_mutex);

    free(frame);

    return (sent == frame_len) ? ESP_OK : ESP_FAIL;
}

esp_err_t websocket_send_hello(void) {
    if (!s_connected || !s_ws) {
        return ESP_ERR_INVALID_STATE;
    }

    cJSON *root = cJSON_CreateObject();
    cJSON_AddStringToObject(root, "type", "hello");

    char mac_str[18];
    mac_to_str(g_state.mac, mac_str, sizeof(mac_str));
    cJSON_AddStringToObject(root, "mac", mac_str);

    if (strlen(g_state.node_id) > 0) {
        cJSON_AddStringToObject(root, "node_id", g_state.node_id);
    }

    // Firmware version (from build)
    cJSON_AddStringToObject(root, "firmware_version", "1.0.0");

    // Capabilities
    cJSON *caps = cJSON_CreateArray();
    cJSON_AddItemToArray(caps, cJSON_CreateString("csi"));
    cJSON_AddItemToArray(caps, cJSON_CreateString("ble"));
    cJSON_AddItemToArray(caps, cJSON_CreateString("tx"));
    cJSON_AddItemToArray(caps, cJSON_CreateString("rx"));
    cJSON_AddItemToObject(root, "capabilities", caps);

    cJSON_AddStringToObject(root, "chip", "ESP32-S3");
    uint32_t flash_size = 0;
    esp_flash_get_size(NULL, &flash_size);
    cJSON_AddNumberToObject(root, "flash_mb", (int)(flash_size / (1024 * 1024)));
    cJSON_AddNumberToObject(root, "uptime_ms", esp_timer_get_time() / 1000);

    // AP BSSID and channel (for passive radar auto-detection)
    uint8_t ap_bssid[6];
    if (wifi_get_ap_bssid(ap_bssid)) {
        char bssid_str[18];
        mac_to_str(ap_bssid, bssid_str, sizeof(bssid_str));
        cJSON_AddStringToObject(root, "ap_bssid", bssid_str);
        cJSON_AddNumberToObject(root, "ap_channel", wifi_get_channel());
    }

    char *json = cJSON_PrintUnformatted(root);
    cJSON_Delete(root);

    if (!json) {
        return ESP_ERR_NO_MEM;
    }

    xSemaphoreTake(s_tx_mutex, portMAX_DELAY);
    int sent = esp_websocket_client_send_text(s_ws, json, strlen(json),
                                               portMAX_DELAY);
    xSemaphoreGive(s_tx_mutex);

    free(json);
    return (sent > 0) ? ESP_OK : ESP_FAIL;
}

esp_err_t websocket_send_health(void) {
    if (!s_connected || !s_ws) {
        return ESP_ERR_INVALID_STATE;
    }

    cJSON *root = cJSON_CreateObject();
    cJSON_AddStringToObject(root, "type", "health");

    char mac_str[18];
    mac_to_str(g_state.mac, mac_str, sizeof(mac_str));
    cJSON_AddStringToObject(root, "mac", mac_str);

    cJSON_AddNumberToObject(root, "timestamp_ms",
                            esp_timer_get_time() / 1000);
    cJSON_AddNumberToObject(root, "free_heap_bytes",
                            esp_get_free_heap_size());
    cJSON_AddNumberToObject(root, "wifi_rssi_dbm", wifi_get_rssi());
    cJSON_AddNumberToObject(root, "uptime_ms",
                            esp_timer_get_time() / 1000);

    // Temperature (if available)
    {
        float tsens_value = 0.0f;
        temperature_sensor_handle_t tsens = NULL;
        temperature_sensor_config_t tsens_cfg = TEMPERATURE_SENSOR_CONFIG_DEFAULT(10, 50);
        if (temperature_sensor_install(&tsens_cfg, &tsens) == ESP_OK) {
            temperature_sensor_enable(tsens);
            temperature_sensor_get_celsius(tsens, &tsens_value);
            temperature_sensor_disable(tsens);
            temperature_sensor_uninstall(tsens);
        }
        cJSON_AddNumberToObject(root, "temperature_c", tsens_value);
    }

    cJSON_AddNumberToObject(root, "csi_rate_hz", g_state.packet_rate);
    cJSON_AddNumberToObject(root, "wifi_channel", wifi_get_channel());

    // Get IP address
    esp_netif_t *netif = esp_netif_get_handle_from_ifkey("WIFI_STA_DEF");
    if (netif) {
        esp_netif_ip_info_t ip_info;
        if (esp_netif_get_ip_info(netif, &ip_info) == ESP_OK) {
            char ip_str[16];
            snprintf(ip_str, sizeof(ip_str), IPSTR, IP2STR(&ip_info.ip));
            cJSON_AddStringToObject(root, "ip", ip_str);
        }
    }

    // NTP sync status
    cJSON_AddBoolToObject(root, "ntp_synced", ntp_is_synced());

    char *json = cJSON_PrintUnformatted(root);
    cJSON_Delete(root);

    if (!json) {
        return ESP_ERR_NO_MEM;
    }

    xSemaphoreTake(s_tx_mutex, portMAX_DELAY);
    int sent = esp_websocket_client_send_text(s_ws, json, strlen(json),
                                               portMAX_DELAY);
    xSemaphoreGive(s_tx_mutex);

    free(json);
    return (sent > 0) ? ESP_OK : ESP_FAIL;
}

esp_err_t websocket_send_ble(const char *devices_json) {
    if (!s_connected || !s_ws || !devices_json) {
        return ESP_ERR_INVALID_STATE;
    }

    // Build BLE message
    cJSON *root = cJSON_CreateObject();
    cJSON_AddStringToObject(root, "type", "ble");

    char mac_str[18];
    mac_to_str(g_state.mac, mac_str, sizeof(mac_str));
    cJSON_AddStringToObject(root, "mac", mac_str);

    cJSON_AddNumberToObject(root, "timestamp_ms",
                            esp_timer_get_time() / 1000);

    // Parse and add devices array
    cJSON *devices = cJSON_Parse(devices_json);
    if (devices) {
        cJSON_AddItemToObject(root, "devices", devices);
    }

    char *json = cJSON_PrintUnformatted(root);
    cJSON_Delete(root);

    if (!json) {
        return ESP_ERR_NO_MEM;
    }

    xSemaphoreTake(s_tx_mutex, portMAX_DELAY);
    int sent = esp_websocket_client_send_text(s_ws, json, strlen(json),
                                               portMAX_DELAY);
    xSemaphoreGive(s_tx_mutex);

    free(json);
    return (sent > 0) ? ESP_OK : ESP_FAIL;
}

esp_err_t websocket_send_motion_hint(float variance) {
    if (!s_connected || !s_ws) {
        return ESP_ERR_INVALID_STATE;
    }

    // Rate-limit to at most 1 hint per second.
    static int64_t s_last_hint_us = 0;
    int64_t now_us = esp_timer_get_time();
    if (now_us - s_last_hint_us < 1000000) {
        return ESP_OK;
    }
    s_last_hint_us = now_us;

    cJSON *root = cJSON_CreateObject();
    cJSON_AddStringToObject(root, "type", "motion_hint");

    char mac_str[18];
    mac_to_str(g_state.mac, mac_str, sizeof(mac_str));
    cJSON_AddStringToObject(root, "mac", mac_str);

    cJSON_AddNumberToObject(root, "timestamp_ms", now_us / 1000);
    cJSON_AddNumberToObject(root, "variance", variance);

    char *json = cJSON_PrintUnformatted(root);
    cJSON_Delete(root);

    if (!json) {
        return ESP_ERR_NO_MEM;
    }

    xSemaphoreTake(s_tx_mutex, portMAX_DELAY);
    int sent = esp_websocket_client_send_text(s_ws, json, strlen(json),
                                               portMAX_DELAY);
    xSemaphoreGive(s_tx_mutex);

    free(json);
    return (sent > 0) ? ESP_OK : ESP_FAIL;
}

esp_err_t websocket_send_ota_status(const char *state, uint8_t progress_pct,
                                     const char *error) {
    if (!s_connected || !s_ws) {
        return ESP_ERR_INVALID_STATE;
    }

    cJSON *root = cJSON_CreateObject();
    cJSON_AddStringToObject(root, "type", "ota_status");

    char mac_str[18];
    mac_to_str(g_state.mac, mac_str, sizeof(mac_str));
    cJSON_AddStringToObject(root, "mac", mac_str);

    cJSON_AddStringToObject(root, "state", state);
    cJSON_AddNumberToObject(root, "progress_pct", progress_pct);

    if (error) {
        cJSON_AddStringToObject(root, "error", error);
    }

    char *json = cJSON_PrintUnformatted(root);
    cJSON_Delete(root);

    if (!json) {
        return ESP_ERR_NO_MEM;
    }

    xSemaphoreTake(s_tx_mutex, portMAX_DELAY);
    int sent = esp_websocket_client_send_text(s_ws, json, strlen(json),
                                               portMAX_DELAY);
    xSemaphoreGive(s_tx_mutex);

    free(json);
    return (sent > 0) ? ESP_OK : ESP_FAIL;
}

void websocket_handle_message(const char *json, size_t len) {
    cJSON *root = cJSON_ParseWithLength(json, len);
    if (!root) {
        ESP_LOGW(TAG, "Failed to parse JSON message");
        return;
    }

    cJSON *type = cJSON_GetObjectItem(root, "type");
    if (!type || !cJSON_IsString(type)) {
        cJSON_Delete(root);
        return;
    }

    ESP_LOGD(TAG, "Received message type: %s", type->valuestring);

    if (strcmp(type->valuestring, "role") == 0) {
        handle_role_msg(root);
    } else if (strcmp(type->valuestring, "config") == 0) {
        handle_config_msg(root);
    } else if (strcmp(type->valuestring, "ota") == 0) {
        handle_ota_msg(root);
    } else if (strcmp(type->valuestring, "reboot") == 0) {
        handle_reboot_msg(root);
    } else if (strcmp(type->valuestring, "identify") == 0) {
        handle_identify_msg(root);
    } else if (strcmp(type->valuestring, "reject") == 0) {
        cJSON *reason = cJSON_GetObjectItem(root, "reason");
        ESP_LOGE(TAG, "Rejected by mothership: %s",
                 reason ? reason->valuestring : "unknown");
        websocket_disconnect();
    }
    // Unknown types are silently ignored (forward-compatible)

    cJSON_Delete(root);
}

static void handle_role_msg(cJSON *root) {
    cJSON *role = cJSON_GetObjectItem(root, "role");
    if (!role || !cJSON_IsString(role)) {
        return;
    }

    // Confirm OTA partition valid on first role message received after boot.
    // This means we successfully connected and the mothership accepted us.
    if (!s_ota_confirmed) {
        s_ota_confirmed = true;
        stop_ota_validation_timer();
        if (is_running_ota_partition()) {
            esp_err_t err = esp_ota_mark_app_valid_cancel_rollback();
            if (err == ESP_OK) {
                ESP_LOGI(TAG, "OTA validation: marked valid after role received");
            } else {
                ESP_LOGW(TAG, "OTA validation: failed to mark valid: %s",
                         esp_err_to_name(err));
            }
        }
    }

    const char *role_str = role->valuestring;
    node_role_t new_role = g_state.role;

    if (strcmp(role_str, "tx") == 0) {
        new_role = NODE_ROLE_TX;
    } else if (strcmp(role_str, "rx") == 0) {
        new_role = NODE_ROLE_RX;
    } else if (strcmp(role_str, "tx_rx") == 0) {
        new_role = NODE_ROLE_TX_RX;
    } else if (strcmp(role_str, "passive") == 0) {
        new_role = NODE_ROLE_PASSIVE;
        // Get passive BSSID
        cJSON *bssid = cJSON_GetObjectItem(root, "passive_bssid");
        if (bssid && cJSON_IsString(bssid)) {
            str_to_mac(bssid->valuestring, g_state.passive_bssid);
        }
    } else if (strcmp(role_str, "idle") == 0) {
        new_role = NODE_ROLE_IDLE;
    }

    if (new_role != g_state.role) {
        g_state.role = new_role;
        xEventGroupSetBits(g_state.events, SPAXEL_EVENT_ROLE_CHANGED);
        ESP_LOGI(TAG, "Role changed to: %s", node_role_str(new_role));
    }
}

static void handle_config_msg(cJSON *root) {
    bool changed = false;

    // Rate
    cJSON *rate = cJSON_GetObjectItem(root, "rate_hz");
    if (rate && cJSON_IsNumber(rate)) {
        uint8_t new_rate = (uint8_t)rate->valueint;
        if (new_rate >= 1 && new_rate <= 100 && new_rate != g_state.packet_rate) {
            g_state.packet_rate = new_rate;
            changed = true;
            ESP_LOGI(TAG, "Rate changed to: %d Hz", new_rate);
        }
    }

    // Variance threshold (for on-device motion hints)
    cJSON *var_thresh = cJSON_GetObjectItem(root, "variance_threshold");
    if (var_thresh && cJSON_IsNumber(var_thresh)) {
        // Store in global for CSI module to use
        extern float g_variance_threshold;
        g_variance_threshold = (float)var_thresh->valuedouble;
    }

    // NTP server (runtime reconfiguration)
    cJSON *ntp = cJSON_GetObjectItem(root, "ntp_server");
    if (ntp && cJSON_IsString(ntp) && strlen(ntp->valuestring) > 0) {
        strncpy(g_state.ntp_server, ntp->valuestring, sizeof(g_state.ntp_server) - 1);
        g_state.ntp_server[sizeof(g_state.ntp_server) - 1] = '\0';
        ESP_LOGI(TAG, "NTP server changed to: %s", g_state.ntp_server);
        ntp_start_sync(g_state.ntp_server);
    }

    if (changed) {
        csi_set_rate(g_state.packet_rate);
    }
}

static void handle_ota_msg(cJSON *root) {
    cJSON *url = cJSON_GetObjectItem(root, "url");
    cJSON *sha256 = cJSON_GetObjectItem(root, "sha256");
    cJSON *version = cJSON_GetObjectItem(root, "version");

    if (!url || !cJSON_IsString(url)) {
        ESP_LOGW(TAG, "OTA message missing URL");
        return;
    }

    strncpy(s_ota_url, url->valuestring, sizeof(s_ota_url) - 1);
    if (sha256 && cJSON_IsString(sha256)) {
        strncpy(s_ota_sha256, sha256->valuestring, sizeof(s_ota_sha256) - 1);
    }
    if (version && cJSON_IsString(version)) {
        strncpy(s_ota_version, version->valuestring, sizeof(s_ota_version) - 1);
    }

    ESP_LOGI(TAG, "OTA triggered: %s", s_ota_url);

    // Start OTA task
    xTaskCreate(ota_task, "ota", 16384, NULL, 5, NULL);
}

static void handle_reboot_msg(cJSON *root) {
    cJSON *delay = cJSON_GetObjectItem(root, "delay_ms");
    uint32_t delay_ms = 1000;

    if (delay && cJSON_IsNumber(delay)) {
        delay_ms = (uint32_t)delay->valueint;
    }

    ESP_LOGI(TAG, "Reboot requested in %lu ms", delay_ms);
    vTaskDelay(pdMS_TO_TICKS(delay_ms));
    esp_restart();
}

static void handle_identify_msg(cJSON *root) {
    cJSON *duration = cJSON_GetObjectItem(root, "duration_ms");
    uint32_t duration_ms = 5000;

    if (duration && cJSON_IsNumber(duration)) {
        duration_ms = (uint32_t)duration->valueint;
    }

    ESP_LOGI(TAG, "Identify: blinking LED for %lu ms", duration_ms);

    // Start LED blink
    esp_err_t err = led_blink_identify(duration_ms);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to start LED blink: %s", esp_err_to_name(err));
    }
}

static void ota_task(void *arg) {
    ESP_LOGI(TAG, "Starting OTA download: %s", s_ota_url);

    websocket_send_ota_status("downloading", 0, NULL);

    esp_http_client_config_t http_cfg = {
        .url = s_ota_url,
        .timeout_ms = 30000,
        .buffer_size = 4096,
    };

    esp_http_client_handle_t http = esp_http_client_init(&http_cfg);
    if (!http) {
        websocket_send_ota_status("failed", 0, "download_failed");
        vTaskDelete(NULL);
        return;
    }

    esp_err_t err = esp_http_client_open(http, 0);
    if (err != ESP_OK) {
        esp_http_client_cleanup(http);
        websocket_send_ota_status("failed", 0, "download_failed");
        vTaskDelete(NULL);
        return;
    }

    int content_len = esp_http_client_fetch_headers(http);
    if (content_len <= 0) {
        esp_http_client_cleanup(http);
        websocket_send_ota_status("failed", 0, "download_failed");
        vTaskDelete(NULL);
        return;
    }

    // Find next OTA partition
    const esp_partition_t *update_part = esp_ota_get_next_update_partition(NULL);
    if (!update_part) {
        esp_http_client_cleanup(http);
        websocket_send_ota_status("failed", 0, "no_partition");
        vTaskDelete(NULL);
        return;
    }

    // Begin OTA
    esp_ota_handle_t ota_handle;
    err = esp_ota_begin(update_part, OTA_SIZE_UNKNOWN, &ota_handle);
    if (err != ESP_OK) {
        esp_http_client_cleanup(http);
        websocket_send_ota_status("failed", 0, "write_failed");
        vTaskDelete(NULL);
        return;
    }

    // Initialize SHA-256 for verification
    mbedtls_sha256_context sha_ctx;
    bool do_sha_verify = (strlen(s_ota_sha256) == 64);
    if (do_sha_verify) {
        mbedtls_sha256_init(&sha_ctx);
        mbedtls_sha256_starts(&sha_ctx, 0);  // 0 = SHA-256
    }

    // Download and write
    char *buf = malloc(4096);
    int total_read = 0;
    int read;

    while ((read = esp_http_client_read(http, buf, 4096)) > 0) {
        // Update SHA-256 hash if verifying
        if (do_sha_verify) {
            mbedtls_sha256_update(&sha_ctx, (unsigned char *)buf, read);
        }

        err = esp_ota_write(ota_handle, buf, read);
        if (err != ESP_OK) {
            free(buf);
            if (do_sha_verify) mbedtls_sha256_free(&sha_ctx);
            esp_ota_abort(ota_handle);
            esp_http_client_cleanup(http);
            websocket_send_ota_status("failed", 0, "write_failed");
            vTaskDelete(NULL);
            return;
        }
        total_read += read;

        uint8_t progress = (uint8_t)((total_read * 100) / content_len);
        websocket_send_ota_status("downloading", progress, NULL);
    }

    free(buf);
    esp_http_client_cleanup(http);

    // Verify and complete
    websocket_send_ota_status("verifying", 100, NULL);

    // SHA-256 verification
    if (do_sha_verify) {
        unsigned char hash[32];
        mbedtls_sha256_finish(&sha_ctx, hash);
        mbedtls_sha256_free(&sha_ctx);

        // Convert binary hash to hex string
        char hash_hex[65];
        for (int i = 0; i < 32; i++) {
            sprintf(hash_hex + (i * 2), "%02x", hash[i]);
        }
        hash_hex[64] = '\0';

        // Compare with expected hash (case-insensitive)
        if (strcasecmp(hash_hex, s_ota_sha256) != 0) {
            ESP_LOGE(TAG, "SHA-256 mismatch: expected %s, got %s", s_ota_sha256, hash_hex);
            esp_ota_abort(ota_handle);
            websocket_send_ota_status("failed", 0, "sha256_mismatch");
            vTaskDelete(NULL);
            return;
        }
        ESP_LOGI(TAG, "SHA-256 verified: %s", hash_hex);
    }

    err = esp_ota_end(ota_handle);
    if (err != ESP_OK) {
        websocket_send_ota_status("failed", 0, "verify_failed");
        vTaskDelete(NULL);
        return;
    }

    // Set boot partition
    err = esp_ota_set_boot_partition(update_part);
    if (err != ESP_OK) {
        websocket_send_ota_status("failed", 0, "boot_partition_failed");
        vTaskDelete(NULL);
        return;
    }

    ESP_LOGI(TAG, "OTA complete, rebooting");
    websocket_send_ota_status("rebooting", 100, NULL);

    vTaskDelay(pdMS_TO_TICKS(1000));
    esp_restart();

    vTaskDelete(NULL);
}
