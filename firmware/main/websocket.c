#include "websocket.h"
#include "spaxel.h"
#include "csi.h"
#include "version.h"
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
#include "esp_crt_bundle.h"
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
// Guards every read AND write of s_ws/s_connected, not just concurrent
// sends. websocket_disconnect() (called from the state machine's reconnect
// path) used to null and destroy s_ws with no coordination against the CSI
// RX task / health task mid-send -- a send that read a non-NULL s_ws just
// before disconnect tore it down would call esp_websocket_client_send_*()
// on a freed handle. Observed on hardware as a LoadProhibited panic during
// a mothership-restart reconnect (bf-3c282). Recursive because
// websocket_connect() calls websocket_disconnect() while already holding
// the lock.
static SemaphoreHandle_t s_ws_mutex = NULL;
static volatile bool s_connected = false;

// Connection debounce timestamp to suppress spurious disconnect events
// during reconnection. See ADR-004 / bead spaxel-61de1fce.
static int64_t s_last_connect_time_us = 0;
#define SPAXEL_CONNECT_DEBOUNCE_MS 200  // 200ms debounce after connect

// OTA state
static char s_ota_url[256] = {0};
static char s_ota_sha256[65] = {0};
static char s_ota_version[32] = {0};

// OTA rollback confirmation. Cancels the automatic rollback timer in the
// ESP-IDF OTA framework once the running image is judged to actually work.
//
// "Actually work" requires BOTH a role message (proves the mothership
// accepted us) AND at least one measured CSI frame sent (proves the sensing
// path itself is alive). Role-message-only was the original bar, and it is
// not enough: the very bug this firmware shipped with (bf-5x46, CSI never
// enabled because esp_wifi_set_csi() ran before esp_wifi_start()) would
// still have connected and received a role message under that bar, self
// validated, and permanently locked itself in with no way to roll back to a
// working build short of physical re-flash. See ADR-004 / bf-5vwo8.
static bool s_ota_role_received = false;
static bool s_ota_confirmed = false;

// One-shot timer: if both conditions above aren't met within 60 s of
// connecting, the new OTA partition stays unconfirmed and the bootloader
// will roll back to the previous partition on the next reset.
static esp_timer_handle_t s_ota_valid_timer = NULL;

// Periodic timer: polls csi_measured_rate_hz() once role has been received,
// since "CSI is flowing" isn't an event we get told about, only something we
// can observe. Stopped as soon as validation completes or the 60s window
// expires.
static esp_timer_handle_t s_ota_check_timer = NULL;

#define SPAXEL_OTA_VALID_TIMEOUT_S 60
#define SPAXEL_OTA_CHECK_INTERVAL_S 2

static bool is_running_ota_partition(void) {
    const esp_partition_t *running = esp_ota_get_running_partition();
    return running != NULL &&
           running->type == ESP_PARTITION_TYPE_APP &&
           running->subtype != ESP_PARTITION_SUBTYPE_APP_FACTORY;
}

static void confirm_ota_valid(void) {
    if (s_ota_confirmed) {
        return;
    }
    s_ota_confirmed = true;
    if (s_ota_check_timer) {
        esp_timer_stop(s_ota_check_timer);
    }
    if (s_ota_valid_timer) {
        esp_timer_stop(s_ota_valid_timer);
    }
    if (is_running_ota_partition()) {
        esp_err_t err = esp_ota_mark_app_valid_cancel_rollback();
        if (err == ESP_OK) {
            ESP_LOGI(TAG, "OTA validation: marked valid (role received, CSI flowing)");
        } else {
            ESP_LOGW(TAG, "OTA validation: failed to mark valid: %s", esp_err_to_name(err));
        }
    }
}

static void ota_check_cb(void *arg) {
    if (s_ota_role_received && csi_measured_rate_hz() > 0) {
        confirm_ota_valid();
    }
}

static void ota_validation_timeout_cb(void *arg) {
    if (!s_ota_confirmed) {
        ESP_LOGE(TAG, "OTA validation: timed out (role_received=%d, csi_rate_hz=%lu), "
                 "rebooting to trigger rollback",
                 s_ota_role_received, (unsigned long)csi_measured_rate_hz());
        if (s_ota_check_timer) {
            esp_timer_stop(s_ota_check_timer);
        }
        // esp_ota_mark_app_valid_cancel_rollback() was never called, so the
        // partition is still ESP_OTA_IMG_PENDING_VERIFY -- but the bootloader
        // only acts on that at boot time. A build that fails validation
        // without crashing (e.g. connects fine but a sensing path is
        // silently dead) would otherwise just keep running in that
        // unconfirmed state forever with no actual rollback, since nothing
        // else forces a reset. Force one here so the safety net this
        // validation exists for actually engages. See bf-23nar.
        vTaskDelay(pdMS_TO_TICKS(500));  // let the log line above flush
        g_state.restarting = true;
        ESP_LOGW(TAG, "Setting restarting flag before OTA timeout restart");
        esp_restart();
    }
}

static void start_ota_validation_timer(void) {
    if (s_ota_valid_timer == NULL) {
        esp_timer_create_args_t timer_args = {
            .callback = ota_validation_timeout_cb,
            .name = "ota_valid",
        };
        esp_timer_create(&timer_args, &s_ota_valid_timer);
    }
    if (s_ota_check_timer == NULL) {
        esp_timer_create_args_t check_args = {
            .callback = ota_check_cb,
            .name = "ota_check",
        };
        esp_timer_create(&check_args, &s_ota_check_timer);
    }
    esp_timer_start_once(s_ota_valid_timer, SPAXEL_OTA_VALID_TIMEOUT_S * 1000000ULL);
    esp_timer_start_periodic(s_ota_check_timer, SPAXEL_OTA_CHECK_INTERVAL_S * 1000000ULL);
    ESP_LOGI(TAG, "OTA validation: waiting for role message + live CSI (timeout %ds)",
             SPAXEL_OTA_VALID_TIMEOUT_S);
}

static void stop_ota_validation_timer(void) {
    if (s_ota_valid_timer != NULL) {
        esp_timer_stop(s_ota_valid_timer);
    }
    if (s_ota_check_timer != NULL) {
        esp_timer_stop(s_ota_check_timer);
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
    s_ws_mutex = xSemaphoreCreateRecursiveMutex();
    if (!s_ws_mutex) {
        ESP_LOGE(TAG, "Failed to create WS mutex");
        return ESP_ERR_NO_MEM;
    }
    return ESP_OK;
}

bool websocket_connect(const char *host, uint16_t port) {
    // Tear down any existing client BEFORE taking the lock below.
    // esp_websocket_client_stop()/_destroy() (called inside
    // websocket_disconnect()) can block for a while tearing down a stuck
    // socket -- ESP-IDF's own docs for both calls say "cannot be called from
    // the websocket event handler" for exactly this reason. Holding
    // s_ws_mutex across that call, as an earlier version of this function
    // did via a nested websocket_disconnect() call, meant every other task's
    // fast-fail connected-check (health, BLE, CSI TX) queued up behind it for
    // the same duration -- observed on hardware as the whole node (all
    // periodic logging included) going silent for minutes after a mothership
    // restart, self-recovering only because the underlying teardown
    // eventually finished on its own. websocket_disconnect() manages its own
    // locking internally and always releases before its own teardown call,
    // so this is safe to call unconditionally and unlocked.
    websocket_disconnect();
    vTaskDelay(pdMS_TO_TICKS(100));

    xSemaphoreTakeRecursive(s_ws_mutex, portMAX_DELAY);

    // Build WebSocket URI.
    //
    // Port 443 selects TLS, matching the provisioning convention in bf-2po1
    // ("spaxel.ardenone.com, port 443, WSS"). A plaintext ws:// cannot reach a
    // mothership published through Cloudflare, which serves HTTPS only — and a
    // node pushes CSI plus its bearer token, so plaintext over the public
    // internet is not an acceptable alternative. Local/bench deployments keep
    // using ws:// on 8080 unchanged.
    const bool use_tls = (port == 443);
    char uri[128];
    snprintf(uri, sizeof(uri), "%s://%s:%d%s",
             use_tls ? "wss" : "ws", host, port, SPAXEL_WS_PATH);

    ESP_LOGI(TAG, "Connecting to %s", uri);

    // Configure WebSocket client.
    //
    // disable_auto_reconnect=true is load-bearing: the client defaults to
    // reconnecting itself in a background task after any transport error,
    // using this same handle. That races the state machine's own reconnect
    // loop (main.c NODE_STATE_MOTHERSHIP_DISCOVERY), which already destroys
    // and recreates the client on every attempt with mDNS/cached-IP fallback
    // and backoff. With both active, a mothership restart produced two
    // concurrent connection attempts on overlapping handles -- observed on
    // hardware as connect, then an immediate spurious disconnect a few tens
    // of ms later, before settling. In the field this raced into a stuck
    // state requiring a physical power cycle. See ADR-004 / bf-3c282.
    esp_websocket_client_config_t cfg = {
        .uri = uri,
        .disable_auto_reconnect = true,
        .network_timeout_ms = 30000,
        .ping_interval_sec = 30,
        .task_stack = 8192,
        .buffer_size = 2048,
        // Only referenced when use_tls; the CA bundle is already compiled in
        // (CONFIG_MBEDTLS_CERTIFICATE_BUNDLE, full 200-cert set) but the linker
        // drops it until esp_crt_bundle_attach() is actually referenced, so
        // this is what makes TLS cost anything at all (~1.4 KB — mbedTLS is
        // already linked for the OTA SHA-256 path).
        .crt_bundle_attach = use_tls ? esp_crt_bundle_attach : NULL,
    };

    // Add auth header if we have a token
    // Note: esp_websocket_client doesn't directly support custom headers,
    // so we'd use the URI query param or implement a custom handshake
    // For now, we'll use a simplified approach

    s_ws = esp_websocket_client_init(&cfg);
    if (!s_ws) {
        ESP_LOGE(TAG, "Failed to init WebSocket client");
        xSemaphoreGiveRecursive(s_ws_mutex);
        return false;
    }

    // Register event handlers
    esp_websocket_register_events(s_ws, WEBSOCKET_EVENT_ANY, ws_event_handler, NULL);

    // Connect
    esp_err_t err = esp_websocket_client_start(s_ws);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to start WebSocket: %s", esp_err_to_name(err));
        esp_websocket_client_handle_t bad = s_ws;
        s_ws = NULL;
        xSemaphoreGiveRecursive(s_ws_mutex);
        esp_websocket_client_destroy(bad);  // outside the lock -- see comment above
        return false;
    }
    xSemaphoreGiveRecursive(s_ws_mutex);

    // Wait for connection. Lock released above -- this just polls a bool for
    // up to 5s and must not block senders/disconnect for that long.
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
    xSemaphoreTakeRecursive(s_ws_mutex, portMAX_DELAY);
    esp_websocket_client_handle_t old = s_ws;
    // Clear the shared handle and connected flag BEFORE tearing down, so any
    // sender that acquires the lock after this point sees s_ws == NULL and
    // bails out instead of touching a handle that's mid-teardown or freed.
    s_ws = NULL;
    s_connected = false;
    xSemaphoreGiveRecursive(s_ws_mutex);

    if (old) {
        esp_websocket_client_stop(old);
        esp_websocket_client_destroy(old);
    }

    stop_ota_validation_timer();

    // Stop any running LED blink on disconnect
    led_stop_blink();
}

bool websocket_is_connected(void) {
    xSemaphoreTakeRecursive(s_ws_mutex, portMAX_DELAY);
    bool result = s_connected && s_ws != NULL;
    xSemaphoreGiveRecursive(s_ws_mutex);
    return result;
}

static void ws_event_handler(void *args, esp_event_base_t base,
                              int32_t id, void *data) {
    esp_websocket_event_data_t *event = (esp_websocket_event_data_t *)data;

    switch (id) {
        case WEBSOCKET_EVENT_CONNECTED:
            ESP_LOGI(TAG, "WebSocket connected");
            s_connected = true;
            s_last_connect_time_us = esp_timer_get_time();
            break;

        case WEBSOCKET_EVENT_DISCONNECTED:
            // Debounce: ignore disconnects within 200ms of a successful connection.
            // This prevents spurious disconnect events during reconnection when the
            // WebSocket client is stabilizing the connection. Without this, a mothership
            // restart can cause 1-2 transient disconnects before the connection settles.
            // See ADR-004 / bead spaxel-61de1fce.
            int64_t now_us = esp_timer_get_time();
            if (s_last_connect_time_us > 0 &&
                (now_us - s_last_connect_time_us) < (SPAXEL_CONNECT_DEBOUNCE_MS * 1000)) {
                ESP_LOGD(TAG, "WebSocket disconnect ignored (debounce window: %lld ms since connect)",
                         (now_us - s_last_connect_time_us) / 1000);
                // Don't set s_connected or trigger state machine; this is a transient event
                break;
            }
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

    // Send binary frame. Re-check under the lock: s_ws may have been torn
    // down by websocket_disconnect() between the fast-path check above and
    // here -- see s_ws_mutex comment.
    xSemaphoreTakeRecursive(s_ws_mutex, portMAX_DELAY);
    int sent = (s_connected && s_ws)
        ? esp_websocket_client_send_bin(s_ws, (char *)frame, frame_len, portMAX_DELAY)
        : -1;
    xSemaphoreGiveRecursive(s_ws_mutex);

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
    // Report the ACTUAL build version, not a literal. The mothership compares
    // this against the version it derived from the uploaded firmware filename;
    // with a hardcoded "1.0.0" they could never agree, so every successful OTA
    // was reported as a rollback — and autoupdate treats a canary rollback as
    // grounds to abort the whole update cycle. See ADR-004 / bf-556tl.
    cJSON_AddStringToObject(root, "firmware_version", SPAXEL_FIRMWARE_VERSION);

    // Running partition, so the mothership can corroborate an update by which
    // slot actually booted rather than by version string alone.
    {
        const esp_partition_t *running = esp_ota_get_running_partition();
        if (running) {
            cJSON_AddStringToObject(root, "running_partition", running->label);
        }
    }

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

    // Re-check under the lock: s_ws may have been torn down by
    // websocket_disconnect() between the fast-path check above and here.
    xSemaphoreTakeRecursive(s_ws_mutex, portMAX_DELAY);
    int sent = (s_connected && s_ws)
        ? esp_websocket_client_send_text(s_ws, json, strlen(json), portMAX_DELAY)
        : -1;
    xSemaphoreGiveRecursive(s_ws_mutex);

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

    // csi_rate_hz is the MEASURED rate. Reporting the configured target here
    // meant a node emitting nothing still claimed 20 Hz. The configured value is
    // still reported, under a name that says what it is. See bf-54cx2.
    cJSON_AddNumberToObject(root, "csi_rate_hz", csi_measured_rate_hz());
    cJSON_AddNumberToObject(root, "csi_rate_configured_hz", g_state.packet_rate);
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

    // Re-check under the lock: s_ws may have been torn down by
    // websocket_disconnect() between the fast-path check above and here.
    xSemaphoreTakeRecursive(s_ws_mutex, portMAX_DELAY);
    int sent = (s_connected && s_ws)
        ? esp_websocket_client_send_text(s_ws, json, strlen(json), portMAX_DELAY)
        : -1;
    xSemaphoreGiveRecursive(s_ws_mutex);

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

    // Re-check under the lock: s_ws may have been torn down by
    // websocket_disconnect() between the fast-path check above and here.
    xSemaphoreTakeRecursive(s_ws_mutex, portMAX_DELAY);
    int sent = (s_connected && s_ws)
        ? esp_websocket_client_send_text(s_ws, json, strlen(json), portMAX_DELAY)
        : -1;
    xSemaphoreGiveRecursive(s_ws_mutex);

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

    // Re-check under the lock: s_ws may have been torn down by
    // websocket_disconnect() between the fast-path check above and here.
    xSemaphoreTakeRecursive(s_ws_mutex, portMAX_DELAY);
    int sent = (s_connected && s_ws)
        ? esp_websocket_client_send_text(s_ws, json, strlen(json), portMAX_DELAY)
        : -1;
    xSemaphoreGiveRecursive(s_ws_mutex);

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

    // Re-check under the lock: s_ws may have been torn down by
    // websocket_disconnect() between the fast-path check above and here.
    xSemaphoreTakeRecursive(s_ws_mutex, portMAX_DELAY);
    int sent = (s_connected && s_ws)
        ? esp_websocket_client_send_text(s_ws, json, strlen(json), portMAX_DELAY)
        : -1;
    xSemaphoreGiveRecursive(s_ws_mutex);

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
        // Don't call websocket_disconnect() here -- this runs on the
        // websocket client's own event-handler task, and both
        // esp_websocket_client_stop()/_destroy() are documented as unsafe to
        // call from that context (can hang tearing down their own task).
        // Defer to the state machine, same as a normal WEBSOCKET_EVENT_DISCONNECTED.
        s_connected = false;
        xEventGroupSetBits(g_state.events, SPAXEL_EVENT_WS_DISCONNECTED);
    }
    // Unknown types are silently ignored (forward-compatible)

    cJSON_Delete(root);
}

static void handle_role_msg(cJSON *root) {
    cJSON *role = cJSON_GetObjectItem(root, "role");
    if (!role || !cJSON_IsString(role)) {
        return;
    }

    // Role received is necessary but not sufficient to confirm the OTA
    // partition -- see s_ota_role_received comment. If CSI is already
    // flowing (the periodic check may have already been running for a
    // couple of seconds), confirm immediately rather than waiting for the
    // next 2s tick.
    s_ota_role_received = true;
    if (!s_ota_confirmed && csi_measured_rate_hz() > 0) {
        confirm_ota_valid();
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
    g_state.restarting = true;
    ESP_LOGW(TAG, "Setting restarting flag before reboot command restart");
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
    ESP_LOGI(TAG, "[OTA] ===========================================");
    ESP_LOGI(TAG, "[OTA] Starting OTA download: %s", s_ota_url);
    ESP_LOGI(TAG, "[OTA] Current state: %s", node_state_str(g_state.state));

    // Mark OTA as in progress to prevent WiFi reconnection interference
    g_state.ota_in_progress = true;
    ESP_LOGI(TAG, "[OTA] Set ota_in_progress=true - WiFi reconnection blocked");
    ESP_LOGI(TAG, "[OTA] ===========================================");

    websocket_send_ota_status("downloading", 0, NULL);

    esp_http_client_config_t http_cfg = {
        .url = s_ota_url,
        .timeout_ms = 30000,
        .buffer_size = 4096,
        // The OTA URL comes from the mothership's SPAXEL_ADVERTISED_BASE_URL,
        // which is https:// for any internet-facing deployment. Without the
        // bundle attached esp_http_client cannot validate the chain and the
        // download fails before a single byte is written to the OTA slot.
        // Harmless for a plain http:// bench URL — it is simply unused.
        .crt_bundle_attach = esp_crt_bundle_attach,
    };

    esp_http_client_handle_t http = esp_http_client_init(&http_cfg);
    if (!http) {
        ESP_LOGE(TAG, "[OTA] FAILED to init HTTP client");
        g_state.ota_in_progress = false;
        ESP_LOGI(TAG, "[OTA] Cleared ota_in_progress=false due to HTTP init failure");
        websocket_send_ota_status("failed", 0, "download_failed");
        vTaskDelete(NULL);
        return;
    }

    // Add authentication headers for ADR-006
    // Format MAC as uppercase colon-separated (e.g., 50:78:7D:1A:3D:C8)
    char mac_str[18];
    mac_to_str(g_state.mac, mac_str, sizeof(mac_str));

    // Set headers: MAC and node token (may be empty for unprovisioned nodes in migration window)
    esp_http_client_set_header(http, "X-Spaxel-MAC", mac_str);
    esp_http_client_set_header(http, "X-Spaxel-Token", g_state.node_token);

    esp_err_t err = esp_http_client_open(http, 0);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "[OTA] FAILED to open HTTP connection: %s", esp_err_to_name(err));
        esp_http_client_cleanup(http);
        g_state.ota_in_progress = false;
        ESP_LOGI(TAG, "[OTA] Cleared ota_in_progress=false due to HTTP connection failure");
        websocket_send_ota_status("failed", 0, "download_failed");
        vTaskDelete(NULL);
        return;
    }

    int content_len = esp_http_client_fetch_headers(http);
    if (content_len <= 0) {
        ESP_LOGE(TAG, "[OTA] FAILED to fetch headers: content_len=%d", content_len);
        esp_http_client_cleanup(http);
        g_state.ota_in_progress = false;
        ESP_LOGI(TAG, "[OTA] Cleared ota_in_progress=false due to header fetch failure");
        websocket_send_ota_status("failed", 0, "download_failed");
        vTaskDelete(NULL);
        return;
    }
    ESP_LOGI(TAG, "[OTA] HTTP connection open, content length: %d bytes", content_len);

    // Find next OTA partition
    const esp_partition_t *update_part = esp_ota_get_next_update_partition(NULL);
    if (!update_part) {
        ESP_LOGE(TAG, "[OTA] FAILED to get next update partition");
        esp_http_client_cleanup(http);
        g_state.ota_in_progress = false;
        ESP_LOGI(TAG, "[OTA] Cleared ota_in_progress=false due to partition failure");
        websocket_send_ota_status("failed", 0, "no_partition");
        vTaskDelete(NULL);
        return;
    }
    ESP_LOGI(TAG, "[OTA] Target partition: %s", update_part->label);

    // Begin OTA
    esp_ota_handle_t ota_handle;
    ESP_LOGI(TAG, "[OTA] Calling esp_ota_begin()...");
    err = esp_ota_begin(update_part, OTA_SIZE_UNKNOWN, &ota_handle);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "[OTA] FAILED to begin OTA: %s", esp_err_to_name(err));
        esp_http_client_cleanup(http);
        g_state.ota_in_progress = false;
        ESP_LOGI(TAG, "[OTA] Cleared ota_in_progress=false due to OTA begin failure");
        websocket_send_ota_status("failed", 0, "write_failed");
        vTaskDelete(NULL);
        return;
    }
    ESP_LOGI(TAG, "[OTA] OTA begin successful, handle=%lu",
             (unsigned long)ota_handle);

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
            ESP_LOGE(TAG, "[OTA] FAILED to write OTA data at offset %d: %s", total_read, esp_err_to_name(err));
            free(buf);
            if (do_sha_verify) mbedtls_sha256_free(&sha_ctx);
            esp_ota_abort(ota_handle);
            esp_http_client_cleanup(http);
            g_state.ota_in_progress = false;
            ESP_LOGI(TAG, "[OTA] Cleared ota_in_progress=false due to write failure");
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
            ESP_LOGE(TAG, "[OTA] SHA-256 mismatch: expected %s, got %s", s_ota_sha256, hash_hex);
            ESP_LOGE(TAG, "[OTA] Aborting OTA due to hash mismatch");
            esp_ota_abort(ota_handle);
            g_state.ota_in_progress = false;
            ESP_LOGI(TAG, "[OTA] Cleared ota_in_progress=false due to hash mismatch");
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

    ESP_LOGI(TAG, "[OTA] ===========================================");
    ESP_LOGI(TAG, "[OTA] OTA complete, preparing to reboot");
    ESP_LOGI(TAG, "[OTA] Clearing ota_in_progress=false before restart");
    g_state.ota_in_progress = false;
    websocket_send_ota_status("rebooting", 100, NULL);

    vTaskDelay(pdMS_TO_TICKS(1000));
    g_state.restarting = true;
    ESP_LOGW(TAG, "[OTA] Setting restarting flag before esp_restart()");
    ESP_LOGI(TAG, "[OTA] Calling esp_restart() NOW");
    esp_restart();

    vTaskDelete(NULL);
}
