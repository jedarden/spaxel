#include "spaxel.h"
#include "wifi.h"
#include "websocket.h"
#include "csi.h"
#include "ble.h"
#include "provision.h"
#include "nvs_migration.h"
#include "ntp.h"
#include "led.h"
#include "esp_log.h"
#include "esp_system.h"
#include "esp_timer.h"
#include "nvs_flash.h"
#include "nvs.h"
#include "esp_mac.h"
#include "freertos/task.h"
#include <string.h>

static const char *TAG = "spaxel";

// Global state
spaxel_state_t g_state = {0};

// Forward declarations
static void state_machine_task(void *arg);
static esp_err_t load_nvs_config(void);

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

    // Load provisioning-time IP override
    len = sizeof(g_state.ms_ip_prov);
    nvs_get_str(nvs, NVS_KEY_MS_IP_PROV, g_state.ms_ip_prov, &len);

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

    // Load NTP server
    len = sizeof(g_state.ntp_server);
    if (nvs_get_str(nvs, NVS_KEY_NTP_SERVER, g_state.ntp_server, &len) != ESP_OK) {
        strncpy(g_state.ntp_server, "pool.ntp.org", sizeof(g_state.ntp_server));
    }

    nvs_close(nvs);

    ESP_LOGI(TAG, "NVS config loaded: provisioned=%d, role=%s, rate=%d Hz",
             g_state.provisioned, node_role_str(g_state.role), g_state.packet_rate);

    return ESP_OK;
}

static void state_machine_task(void *arg) {
    // Tracks whether CSI has been armed for the current CONNECTED session, so
    // entering CONNECTED arms it exactly once rather than on every loop pass.
    bool csi_armed_this_session = false;

    int wifi_fail_count = 0;
    int discovery_fail_count = 0;

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
                    esp_err_t err = wifi_start_connect();
                    if (err != ESP_OK) {
                        // Log with context to distinguish expected failures during restart
                        if (g_state.restarting) {
                            ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] WiFi connect skipped during restart (expected): %s", esp_err_to_name(err));
                            ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] This is a guard-triggered skip, NOT an error");
                        } else {
                            ESP_LOGE(TAG, "WiFi connect failed on boot (unexpected): %s", esp_err_to_name(err));
                        }
                        // State machine will handle failure via event bits
                    }
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
                    discovery_fail_count = 0;

                    // Initialize NTP after WiFi is up
                    ESP_LOGI(TAG, "Starting NTP sync with server: %s", g_state.ntp_server);
                    ntp_init();
                    ntp_start_sync(g_state.ntp_server);
                    if (!ntp_wait_sync(10000)) {
                        ESP_LOGW(TAG, "NTP sync failed, proceeding without stagger");
                    }
                    ntp_start_periodic_resync();

                    g_state.state = NODE_STATE_MOTHERSHIP_DISCOVERY;
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

                    // 1. Try provisioned IP override first (for mDNS-less networks)
                    if (strlen(g_state.ms_ip_prov) > 0) {
                        ESP_LOGI(TAG, "Trying provisioned mothership IP: %s:%d", g_state.ms_ip_prov, ms_port);
                        strncpy(g_state.ms_ip, g_state.ms_ip_prov, sizeof(g_state.ms_ip) - 1);
                        // Skip mDNS on first attempt if provisioned IP is set
                        if (discovery_fail_count == 0) {
                            goto attempt_connect;
                        }
                    }

                    // 2. Try mDNS discovery
                    if (wifi_discover_mothership(ms_ip, sizeof(ms_ip), &ms_port)) {
                        ESP_LOGI(TAG, "Mothership discovered via mDNS: %s:%d", ms_ip, ms_port);
                        strncpy(g_state.ms_ip, ms_ip, sizeof(g_state.ms_ip) - 1);
                        g_state.ms_port = ms_port;
                        discovery_fail_count = 0;
                    } else if (strlen(g_state.ms_ip) > 0) {
                        // 3. Fallback to cached IP
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

                attempt_connect:

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
                // Arm CSI whenever we enter CONNECTED, not only on a role CHANGE.
                //
                // Recovery used to depend solely on SPAXEL_EVENT_ROLE_CHANGED,
                // which only fires when the assigned role DIFFERS from the
                // persisted one — so a node whose stored role already matched
                // what the fleet assigned never armed CSI and silently
                // captured nothing for the whole boot. See ADR-003 / bf-3mb8j.
                //
                // Enabling CSI on the radio itself (esp_wifi_set_csi_config /
                // _rx_cb / _csi(true)) is handled separately, once, from the
                // WIFI_EVENT_STA_START handler in wifi.c — those calls fail
                // with ESP_ERR_WIFI_NOT_STARTED if issued before
                // esp_wifi_start() completes. See ADR-003 / bf-5x46.
                if (!csi_armed_this_session) {
                    ESP_LOGI(TAG, "Arming CSI for role %s", node_role_str(g_state.role));
                    csi_set_role(g_state.role, g_state.passive_bssid);
                    csi_armed_this_session = true;
                }

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
                        g_state.restarting = true;
                        ESP_LOGW(TAG, "Setting restarting flag before event-based reboot");
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
                        csi_armed_this_session = false;
                        g_state.state = NODE_STATE_MOTHERSHIP_DISCOVERY;
                    }

                    if (bits & SPAXEL_EVENT_WIFI_FAILED) {
                        ESP_LOGW(TAG, "WiFi lost");
                        g_state.state = NODE_STATE_WIFI_LOST;
                    }
                }
                break;

            case NODE_STATE_WIFI_LOST:
                // Check if OTA is in progress - if so, delay reconnection
                // to avoid racing with the OTA download process.
                // See ADR-004 / bf-xss9y.
                if (g_state.ota_in_progress) {
                    ESP_LOGW(TAG, "WiFi lost but OTA in progress - delaying reconnection");
                    vTaskDelay(pdMS_TO_TICKS(5000));
                    break;
                }

                // Try to reconnect to WiFi
                ESP_LOGI(TAG, "Attempting WiFi reconnect");
                esp_err_t err = wifi_start_connect();
                if (err != ESP_OK) {
                    // Log with context to distinguish expected failures during restart
                    if (g_state.restarting) {
                        ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] WiFi reconnect skipped during restart (expected): %s", esp_err_to_name(err));
                        ESP_LOGW(TAG, "[RESTART-SAFE-GUARD] This is a guard-triggered skip, NOT an error");
                    } else {
                        ESP_LOGE(TAG, "WiFi reconnect failed unexpectedly: %s", esp_err_to_name(err));
                    }
                    // Continue to wait for event bits; failure will be counted below
                }

                bits = xEventGroupWaitBits(
                    g_state.events,
                    SPAXEL_EVENT_WIFI_CONNECTED | SPAXEL_EVENT_WIFI_FAILED,
                    pdTRUE, pdFALSE,
                    pdMS_TO_TICKS(30000)
                );

                if (bits & SPAXEL_EVENT_WIFI_CONNECTED) {
                    ESP_LOGI(TAG, "WiFi reconnected");

                    // Re-sync NTP after reconnect
                    ntp_init();
                    ntp_start_sync(g_state.ntp_server);
                    if (!ntp_wait_sync(10000)) {
                        ESP_LOGW(TAG, "NTP resync failed after WiFi reconnect");
                    }
                    ntp_start_periodic_resync();

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

            case NODE_STATE_CAPTIVE_PORTAL: {
                // This state is re-dispatched by the enclosing loop every ~60s
                // since nothing here ever changes g_state.state -- captive
                // portal mode is exited only via esp_restart() in the
                // provisioning save handler. Without this guard that meant
                // calling wifi_start_captive_portal() again on every
                // re-dispatch, which failed to rebind the DNS/HTTP servers
                // and left a bare, non-functional SSID after the first
                // minute (bf-1b7c). The static resets on reboot along with
                // everything else, so a fresh boot always retries startup.
                static bool portal_started = false;
                if (!portal_started) {
                    ESP_LOGI(TAG, "Starting captive portal");
                    portal_started = (wifi_start_captive_portal() == ESP_OK);
                    if (!portal_started) {
                        ESP_LOGE(TAG, "Captive portal failed to start, will retry in 60s");
                    }
                }

                // Captive portal runs indefinitely until provisioned
                // Provisioning handler will reboot the device
                vTaskDelay(pdMS_TO_TICKS(60000));
                break;
            }
        }

        vTaskDelay(pdMS_TO_TICKS(100));
    }
}

// Health reporting task
// Supervisory self-recovery.
//
// A node that cannot reach the mothership must eventually reboot itself rather
// than retry forever. Three distinct unrecoverable hangs were observed on real
// hardware, all of which this catches:
//   1. a websocket retry loop spamming "Websocket client is not connected"
//   2. a connect loop returning EHOSTUNREACH while the AP simultaneously showed
//      the station associated at -35 dBm with a valid lease and zero tx failures
//   3. silence after an OTA reboot — booted, but never rejoined
//
// In every case the only recovery was physical: a USB replug. That is the one
// thing a node mounted around the house cannot receive, and with several nodes
// deployed a rare per-node hang becomes a routine trip up a ladder.
//
// A reboot is cheap and safe here: WiFi credentials and the provisioned flag
// live in NVS, and the bootloader's rollback protection still applies, so the
// worst case is a node that reboots periodically and stays reachable — which is
// strictly better than one that is silently gone.
// 3 minutes: a mothership restart completes in seconds, so a node still adrift
// after three minutes is not "waiting", it is stuck. Rebooting costs ~30s of
// downtime and reliably rejoins; staying stuck costs a physical visit. With
// several nodes deployed, every mothership restart would otherwise strand the
// whole fleet at once — which is exactly what was observed.
#define SPAXEL_MOTHERSHIP_LOST_REBOOT_MS (3 * 60 * 1000)

static void health_task(void *arg) {
    int64_t last_connected_us = esp_timer_get_time();

    while (1) {
        vTaskDelay(pdMS_TO_TICKS(SPAXEL_HEALTH_INTERVAL_MS));

        if (g_state.state == NODE_STATE_CONNECTED) {
            last_connected_us = esp_timer_get_time();
            websocket_send_health();
            continue;
        }

        int64_t lost_ms = (esp_timer_get_time() - last_connected_us) / 1000;
        if (lost_ms >= SPAXEL_MOTHERSHIP_LOST_REBOOT_MS) {
            ESP_LOGE(TAG,
                     "No mothership connection for %lld ms (state=%s) — rebooting to recover",
                     (long long)lost_ms, node_state_str(g_state.state));
            g_state.restarting = true;
            ESP_LOGW(TAG, "Setting restarting flag before mothership-lost reboot");
            vTaskDelay(pdMS_TO_TICKS(200)); // let the log drain
            esp_restart();
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
    if (ret != ESP_OK) {
        ESP_LOGE(TAG, "Failed to initialize NVS after recovery attempt: %s", esp_err_to_name(ret));
        ESP_LOGE(TAG, "System cannot continue without NVS storage");
        // Graceful shutdown instead of abort
        return;
    }

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
    ret = wifi_init();
    if (ret != ESP_OK) {
        ESP_LOGE(TAG, "Failed to initialize WiFi stack: %s", esp_err_to_name(ret));
        ESP_LOGE(TAG, "System cannot continue without WiFi connectivity");
        // Graceful shutdown instead of abort
        return;
    }

    // Initialize LED
    led_init();

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
