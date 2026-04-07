#include "spaxel.h"
#include "wifi.h"
#include "websocket.h"
#include "csi.h"
#include "ble.h"
#include "provision.h"
#include "nvs_migration.h"
#include "ntp.h"
#include "esp_log.h"
#include "esp_system.h"
#include "esp_timer.h"
#include "nvs_flash.h"
#include "nvs.h"
#include "esp_mac.h"
#include "freertos/task.h"

static const char *TAG = "spaxel";

// Global state
spaxel_state_t g_state = {0};

// Forward declarations
static void state_machine_task(void *arg);
static esp_err_t load_nvs_config(void);
static esp_err_t save_nvs_role(node_role_t role);
static esp_err_t save_nvs_rate(uint8_t rate);

const char* node_state_str(node_state_t state) {
    switch (state) {
        case NODE_STATE_BOOT: return "BOOT";
        case NODE_STATE_WIFI_CONNECTING: return "WIFI_CONNECTING";
        case NODE_STATE_MOTHERSHIP_DISCOVERY: return "MOTHERSHIP_DISCOVERY";
        case NODE_STATE_CONNECTED: return "CONNECTED";
        case NODE_STATE_WIFI_LOST: return "WIFI_LOST";
        case NODE_STATE_MOTHERSHIP_UNAVAILABLE: return "MOTHERSHIP_UNAVAILABLE";
        case NODE_STATE_CAPTIVE_PORTAL: return "CAPTIVE_PORTAL";
        default: return "UNKNOWN";
    }
}

const char* node_role_str(node_role_t role) {
    switch (role) {
        case NODE_ROLE_TX: return "tx";
        case NODE_ROLE_RX: return "rx";
        case NODE_ROLE_TX_RX: return "tx_rx";
        case NODE_ROLE_PASSIVE: return "passive";
        case NODE_ROLE_IDLE: return "idle";
        default: return "unknown";
    }
}

void mac_to_str(uint8_t *mac, char *buf, size_t len) {
    snprintf(buf, len, "%02X:%02X:%02X:%02X:%02X:%02X",
             mac[0], mac[1], mac[2], mac[3], mac[4], mac[5]);
}

void str_to_mac(const char *str, uint8_t *mac) {
    int values[6];
    sscanf(str, "%02X:%02X:%02X:%02X:%02X:%02X",
           &values[0], &values[1], &values[2],
           &values[3], &values[4], &values[5]);
    for (int i = 0; i < 6; i++) {
        mac[i] = (uint8_t)values[i];
    }
}

static esp_err_t load_nvs_config(void) {
    nvs_handle_t nvs;
    esp_err_t err = nvs_open(SPAXEL_NAMESPACE, NVS_READONLY, &nvs);
    if (err != ESP_OK) {
        ESP_LOGW(TAG, "Failed to open NVS namespace: %s", esp_err_to_name(err));
        return err;
    }

    // Check provisioned flag
    g_state.provisioned = false;
    uint8_t provisioned = 0;
    if (nvs_get_u8(nvs, NVS_KEY_PROVISIONED, &provisioned) == ESP_OK) {
        g_state.provisioned = (provisioned == 1);
    }

    // Load role
    uint8_t role = NODE_ROLE_TX_RX;
    if (nvs_get_u8(nvs, NVS_KEY_ROLE, &role) == ESP_OK) {
        g_state.role = (node_role_t)role;
    } else {
        g_state.role = NODE_ROLE_TX_RX;
    }

    // Load packet rate
    uint8_t rate = 20;
    if (nvs_get_u8(nvs, NVS_KEY_PKT_RATE, &rate) == ESP_OK) {
        g_state.packet_rate = rate;
    } else {
        g_state.packet_rate = 20;
    }

    // Load mDNS name
    size_t len = sizeof(g_state.ms_mdns);
    if (nvs_get_str(nvs, NVS_KEY_MS_MDNS, g_state.ms_mdns, &len) != ESP_OK) {
        strncpy(g_state.ms_mdns, "spaxel", sizeof(g_state.ms_mdns));
    }

    // Load port
    uint16_t port = SPAXEL_MDNS_PORT;
    if (nvs_get_u16(nvs, NVS_KEY_MS_PORT, &port) == ESP_OK) {
        g_state.ms_port = port;
    } else {
        g_state.ms_port = SPAXEL_MDNS_PORT;
    }

    // Load fallback IP
    len = sizeof(g_state.ms_ip);
    nvs_get_str(nvs, NVS_KEY_MS_IP, g_state.ms_ip, &len);

    // Load node ID
    len = sizeof(g_state.node_id);
    nvs_get_str(nvs, NVS_KEY_NODE_ID, g_state.node_id, &len);

    // Load node token
    len = sizeof(g_state.node_token);
    nvs_get_str(nvs, NVS_KEY_NODE_TOKEN, g_state.node_token, &len);

    // Load passive BSSID
    size_t blob_len = 6;
    nvs_get_blob(nvs, NVS_KEY_PASSIVE_BSS, g_state.passive_bssid, &blob_len);

    // Load debug flag
    uint8_t debug = 0;
    if (nvs_get_u8(nvs, NVS_KEY_DEBUG, &debug) == ESP_OK) {
        g_state.debug = (debug == 1);
    }

    nvs_close(nvs);

    ESP_LOGI(TAG, "NVS config loaded: provisioned=%d, role=%s, rate=%d Hz",
             g_state.provisioned, node_role_str(g_state.role), g_state.packet_rate);

    return ESP_OK;
}

static esp_err_t save_nvs_role(node_role_t role) {
    nvs_handle_t nvs;
    esp_err_t err = nvs_open(SPAXEL_NAMESPACE, NVS_READWRITE, &nvs);
    if (err != ESP_OK) return err;

    err = nvs_set_u8(nvs, NVS_KEY_ROLE, (uint8_t)role);
    if (err == ESP_OK) {
        nvs_commit(nvs);
        g_state.role = role;
    }
    nvs_close(nvs);
    return err;
}

static esp_err_t save_nvs_rate(uint8_t rate) {
    nvs_handle_t nvs;
    esp_err_t err = nvs_open(SPAXEL_NAMESPACE, NVS_READWRITE, &nvs);
    if (err != ESP_OK) return err;

    err = nvs_set_u8(nvs, NVS_KEY_PKT_RATE, rate);
    if (err == ESP_OK) {
        nvs_commit(nvs);
        g_state.packet_rate = rate;
    }
    nvs_close(nvs);
    return err;
}

static void state_machine_task(void *arg) {
    int wifi_fail_count = 0;
    int discovery_fail_count = 0;
    TickType_t last_state_change = xTaskGetTickCount();

    while (1) {
        ESP_LOGD(TAG, "State machine: %s", node_state_str(g_state.state));

        switch (g_state.state) {
            case NODE_STATE_BOOT:
                // Check if provisioned
                if (!g_state.provisioned) {
                    ESP_LOGI(TAG, "Not provisioned, starting captive portal");
                    g_state.state = NODE_STATE_CAPTIVE_PORTAL;
                } else {
                    ESP_LOGI(TAG, "Provisioned, connecting to WiFi");
                    g_state.state = NODE_STATE_WIFI_CONNECTING;
                    wifi_start_connect();
                }
                break;

            case NODE_STATE_WIFI_CONNECTING:
                // Wait for WiFi event
                EventBits_t bits = xEventGroupWaitBits(
                    g_state.events,
                    SPAXEL_EVENT_WIFI_CONNECTED | SPAXEL_EVENT_WIFI_FAILED,
                    pdTRUE, pdFALSE,
                    pdMS_TO_TICKS(30000)
                );

                if (bits & SPAXEL_EVENT_WIFI_CONNECTED) {
                    ESP_LOGI(TAG, "WiFi connected");
                    wifi_fail_count = 0;
                    g_state.state = NODE_STATE_MOTHERSHIP_DISCOVERY;
                    discovery_fail_count = 0;
                } else if (bits & SPAXEL_EVENT_WIFI_FAILED) {
                    wifi_fail_count++;
                    ESP_LOGW(TAG, "WiFi failed (attempt %d)", wifi_fail_count);
                    if (wifi_fail_count >= 10) {
                        ESP_LOGE(TAG, "WiFi failed 10 times, starting captive portal");
                        g_state.state = NODE_STATE_CAPTIVE_PORTAL;
                    }
                    // Exponential backoff handled in wifi.c
                }
                break;

            case NODE_STATE_MOTHERSHIP_DISCOVERY:
                {
                    char ms_ip[64] = {0};
                    uint16_t ms_port = g_state.ms_port;

                    // Try mDNS first
                    if (wifi_discover_mothership(ms_ip, sizeof(ms_ip), &ms_port)) {
                        ESP_LOGI(TAG, "Mothership discovered via mDNS: %s:%d", ms_ip, ms_port);
                        strncpy(g_state.ms_ip, ms_ip, sizeof(g_state.ms_ip) - 1);
                        g_state.ms_port = ms_port;
                        discovery_fail_count = 0;
                    } else if (strlen(g_state.ms_ip) > 0) {
                        // Fallback to cached IP
                        ESP_LOGI(TAG, "Using cached mothership IP: %s", g_state.ms_ip);
                    } else {
                        discovery_fail_count++;
                        ESP_LOGW(TAG, "Mothership discovery failed (attempt %d)", discovery_fail_count);

                        if (discovery_fail_count >= 10) {
                            ESP_LOGW(TAG, "Mothership unavailable, continuing in degraded mode");
                            g_state.state = NODE_STATE_MOTHERSHIP_UNAVAILABLE;
                            break;
                        }
                        vTaskDelay(pdMS_TO_TICKS(5000));
                        break;
                    }

                    // Attempt WebSocket connection
                    if (websocket_connect(g_state.ms_ip, g_state.ms_port)) {
                        ESP_LOGI(TAG, "WebSocket connected to mothership");
                        g_state.state = NODE_STATE_CONNECTED;

                        // Save discovered IP
                        nvs_handle_t nvs;
                        if (nvs_open(SPAXEL_NAMESPACE, NVS_READWRITE, &nvs) == ESP_OK) {
                            nvs_set_str(nvs, NVS_KEY_MS_IP, g_state.ms_ip);
                            nvs_commit(nvs);
                            nvs_close(nvs);
                        }
                    } else {
                        discovery_fail_count++;
                        ESP_LOGW(TAG, "WebSocket connection failed (attempt %d)", discovery_fail_count);
                        vTaskDelay(pdMS_TO_TICKS(5000));
                    }
                }
                break;

            case NODE_STATE_CONNECTED:
                // Normal operation - wait for disconnect or commands
                {
                    EventBits_t bits = xEventGroupWaitBits(
                        g_state.events,
                        SPAXEL_EVENT_WS_DISCONNECTED |
                        SPAXEL_EVENT_WIFI_FAILED |
                        SPAXEL_EVENT_ROLE_CHANGED |
                        SPAXEL_EVENT_OTA_TRIGGER |
                        SPAXEL_EVENT_REBOOT,
                        pdTRUE, pdFALSE,
                        pdMS_TO_TICKS(1000)
                    );

                    if (bits & SPAXEL_EVENT_REBOOT) {
                        ESP_LOGI(TAG, "Reboot requested");
                        esp_restart();
                    }

                    if (bits & SPAXEL_EVENT_OTA_TRIGGER) {
                        ESP_LOGI(TAG, "OTA triggered");
                        // OTA handling in websocket.c
                    }

                    if (bits & SPAXEL_EVENT_ROLE_CHANGED) {
                        ESP_LOGI(TAG, "Role changed, reconfiguring CSI");
                        csi_set_role(g_state.role, g_state.passive_bssid);
                    }

                    if (bits & SPAXEL_EVENT_WS_DISCONNECTED) {
                        ESP_LOGW(TAG, "WebSocket disconnected");
                        g_state.state = NODE_STATE_MOTHERSHIP_DISCOVERY;
                    }

                    if (bits & SPAXEL_EVENT_WIFI_FAILED) {
                        ESP_LOGW(TAG, "WiFi lost");
                        g_state.state = NODE_STATE_WIFI_LOST;
                    }
                }
                break;

            case NODE_STATE_WIFI_LOST:
                // Try to reconnect to WiFi
                ESP_LOGI(TAG, "Attempting WiFi reconnect");
                wifi_start_connect();

                bits = xEventGroupWaitBits(
                    g_state.events,
                    SPAXEL_EVENT_WIFI_CONNECTED | SPAXEL_EVENT_WIFI_FAILED,
                    pdTRUE, pdFALSE,
                    pdMS_TO_TICKS(30000)
                );

                if (bits & SPAXEL_EVENT_WIFI_CONNECTED) {
                    ESP_LOGI(TAG, "WiFi reconnected");
                    g_state.state = NODE_STATE_MOTHERSHIP_DISCOVERY;
                } else {
                    wifi_fail_count++;
                    if (wifi_fail_count >= 10) {
                        ESP_LOGE(TAG, "WiFi lost 10 times, starting captive portal");
                        g_state.state = NODE_STATE_CAPTIVE_PORTAL;
                    }
                }
                break;

            case NODE_STATE_MOTHERSHIP_UNAVAILABLE:
                // Continue operating at last known role, retry discovery periodically
                ESP_LOGD(TAG, "Operating in degraded mode, retrying discovery in 30s");

                // Continue CSI capture and BLE scanning
                csi_set_role(g_state.role, g_state.passive_bssid);

                vTaskDelay(pdMS_TO_TICKS(30000));

                // Try discovery again
                g_state.state = NODE_STATE_MOTHERSHIP_DISCOVERY;
                discovery_fail_count = 0;
                break;

            case NODE_STATE_CAPTIVE_PORTAL:
                // Start captive portal AP mode
                ESP_LOGI(TAG, "Starting captive portal");
                wifi_start_captive_portal();

                // Captive portal runs indefinitely until provisioned
                // Provisioning handler will reboot the device
                vTaskDelay(pdMS_TO_TICKS(60000));
                break;
        }

        vTaskDelay(pdMS_TO_TICKS(100));
    }
}

// Health reporting task
static void health_task(void *arg) {
    while (1) {
        vTaskDelay(pdMS_TO_TICKS(SPAXEL_HEALTH_INTERVAL_MS));

        if (g_state.state == NODE_STATE_CONNECTED) {
            websocket_send_health();
        }
    }
}

void app_main(void) {
    ESP_LOGI(TAG, "SPAXEL Firmware starting...");

    // Initialize NVS
    esp_err_t ret = nvs_flash_init();
    if (ret == ESP_ERR_NVS_NO_FREE_PAGES || ret == ESP_ERR_NVS_NEW_VERSION_FOUND) {
        ESP_LOGW(TAG, "NVS partition corrupted, erasing...");
        nvs_flash_erase();
        ret = nvs_flash_init();
    }
    ESP_ERROR_CHECK(ret);

    // Run NVS schema migration if needed
    esp_err_t migration_err = nvs_migration_run();
    if (migration_err != ESP_OK) {
        ESP_LOGE(TAG, "NVS migration failed: %s", esp_err_to_name(migration_err));
        // Continue anyway - NVS should be in a consistent state
    }

    // Get MAC address
    esp_read_mac(g_state.mac, ESP_MAC_WIFI_STA);
    char mac_str[18];
    mac_to_str(g_state.mac, mac_str, sizeof(mac_str));
    ESP_LOGI(TAG, "Node MAC: %s", mac_str);

    // Create event group
    g_state.events = xEventGroupCreate();

    // Load configuration from NVS
    load_nvs_config();

    // Open serial provisioning window (10 s) — active before normal boot.
    // Host sends {"provision": {...}}\n via Web Serial; firmware replies and proceeds.
    provision_listen_window();

    // Reload NVS config in case provisioning changed it
    load_nvs_config();

    // Initialize WiFi
    wifi_init();

    // Initialize CSI
    csi_init();

    // Initialize BLE
    ble_init();

    // Initialize WebSocket
    websocket_init();

    // Start state machine
    xTaskCreate(state_machine_task, "state_machine", 8192, NULL, 5, NULL);

    // Start health reporting
    xTaskCreate(health_task, "health", 4096, NULL, 3, NULL);

    ESP_LOGI(TAG, "SPAXEL Firmware initialized");
}
