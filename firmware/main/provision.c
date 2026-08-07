#include "provision.h"
#include "spaxel.h"
#include "esp_log.h"
#include "nvs_flash.h"
#include "nvs.h"
#include "cJSON.h"
#include "driver/usb_serial_jtag.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include <string.h>

static const char *TAG = "provision";

#define PROVISION_WINDOW_MS_FRESH   120000  // 2 min for unprovisioned boards
#define PROVISION_WINDOW_MS_REPROV   15000  // 15 s for already-provisioned boards
#define MAX_LINE_LEN                768

// Bound on every provisioning-window write. usb_serial_jtag_write_bytes() with
// portMAX_DELAY blocks forever once the peripheral's TX ring fills, and it fills
// whenever a USB host is enumerated but no process is draining the port — the
// normal state of a board plugged into a computer for power, and of this bench
// rig any time nobody is capturing the console. The window then never exits, so
// wifi_init() is never reached and the node never joins: it presents as "boots
// fine, never connects", indistinguishable from a WiFi fault. Bounding the write
// costs at most a dropped beacon line and lets boot proceed.
#define PROVISION_TX_TIMEOUT        pdMS_TO_TICKS(200)

void provision_listen_window(void) {
    // Same install pattern ESP-IDF's own esp_console_repl.c uses to combine
    // a USB-Serial-JTAG console (CONFIG_ESP_CONSOLE_USB_SERIAL_JTAG, already
    // active for esp_log by this point) with driver-level read/write on the
    // same peripheral.
    usb_serial_jtag_driver_config_t usbsj_cfg = USB_SERIAL_JTAG_DRIVER_CONFIG_DEFAULT();
    if (usb_serial_jtag_driver_install(&usbsj_cfg) != ESP_OK) {
        ESP_LOGW(TAG, "USB-Serial-JTAG driver install failed, skipping provision window");
        return;
    }

    char mac_str[18];
    mac_to_str(g_state.mac, mac_str, sizeof(mac_str));

    uint32_t window_ms = g_state.provisioned
        ? PROVISION_WINDOW_MS_REPROV
        : PROVISION_WINDOW_MS_FRESH;

    // Signal that firmware is ready for provisioning (includes MAC for display).
    // Broadcast every 1 s so the host can open the port at any time during
    // the window — not just at the exact moment of first boot.
    char ready_msg[64];
    snprintf(ready_msg, sizeof(ready_msg), "SPAXEL READY %s\n", mac_str);
    usb_serial_jtag_write_bytes(ready_msg, strlen(ready_msg), PROVISION_TX_TIMEOUT);

    ESP_LOGI(TAG, "Provisioning window open for %u ms (MAC: %s)", (unsigned)window_ms, mac_str);

    TickType_t deadline   = xTaskGetTickCount() + pdMS_TO_TICKS(window_ms);
    TickType_t last_ready = xTaskGetTickCount();
    char line[MAX_LINE_LEN];
    int line_pos = 0;

    while (xTaskGetTickCount() < deadline) {
        // Re-broadcast READY every 1 s so the host can connect at any time
        if ((xTaskGetTickCount() - last_ready) >= pdMS_TO_TICKS(1000)) {
            usb_serial_jtag_write_bytes(ready_msg, strlen(ready_msg), PROVISION_TX_TIMEOUT);
            last_ready = xTaskGetTickCount();
        }

        uint8_t ch;
        int n = usb_serial_jtag_read_bytes(&ch, 1, pdMS_TO_TICKS(50));
        if (n <= 0) {
            continue;
        }

        if (ch == '\r') {
            continue; // ignore CR
        }

        if (ch == '\n') {
            line[line_pos] = '\0';
            line_pos = 0;

            if (strlen(line) == 0) {
                continue;
            }

            cJSON *root = cJSON_Parse(line);
            if (!root) {
                const char *err_resp = "{\"ok\":false,\"error\":\"invalid_json\"}\n";
                usb_serial_jtag_write_bytes(err_resp, strlen(err_resp), PROVISION_TX_TIMEOUT);
                continue;
            }

            cJSON *prov = cJSON_GetObjectItem(root, "provision");
            if (!prov) {
                cJSON_Delete(root);
                const char *err_resp = "{\"ok\":false,\"error\":\"missing_provision_key\"}\n";
                usb_serial_jtag_write_bytes(err_resp, strlen(err_resp), PROVISION_TX_TIMEOUT);
                continue;
            }

            esp_err_t err = provision_write_nvs(prov);
            cJSON_Delete(root);

            if (err == ESP_OK) {
                char resp[80];
                snprintf(resp, sizeof(resp), "{\"ok\":true,\"mac\":\"%s\"}\n", mac_str);
                usb_serial_jtag_write_bytes(resp, strlen(resp), PROVISION_TX_TIMEOUT);
                ESP_LOGI(TAG, "Provisioning complete via serial");
                usb_serial_jtag_driver_uninstall();
                return;
            } else {
                const char *err_resp = "{\"ok\":false,\"error\":\"nvs_write_failed\"}\n";
                usb_serial_jtag_write_bytes(err_resp, strlen(err_resp), PROVISION_TX_TIMEOUT);
            }
        } else if (line_pos < MAX_LINE_LEN - 1) {
            line[line_pos++] = (char)ch;
        } else {
            // Line too long — flush buffer
            line_pos = 0;
        }
    }

    ESP_LOGI(TAG, "Provisioning window closed (no provisioning received)");
    usb_serial_jtag_driver_uninstall();
}

esp_err_t provision_write_nvs(cJSON *prov) {
    nvs_handle_t nvs;
    esp_err_t err = nvs_open(SPAXEL_NAMESPACE, NVS_READWRITE, &nvs);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "NVS open failed: %s", esp_err_to_name(err));
        return err;
    }

    cJSON *ssid = cJSON_GetObjectItem(prov, "wifi_ssid");
    if (ssid && cJSON_IsString(ssid) && strlen(ssid->valuestring) > 0) {
        nvs_set_str(nvs, NVS_KEY_WIFI_SSID, ssid->valuestring);
    } else {
        nvs_close(nvs);
        return ESP_ERR_INVALID_ARG;
    }

    cJSON *pass = cJSON_GetObjectItem(prov, "wifi_pass");
    if (pass && cJSON_IsString(pass)) {
        nvs_set_str(nvs, NVS_KEY_WIFI_PASS, pass->valuestring);
    }

    cJSON *node_id = cJSON_GetObjectItem(prov, "node_id");
    if (node_id && cJSON_IsString(node_id)) {
        nvs_set_str(nvs, NVS_KEY_NODE_ID, node_id->valuestring);
    }

    cJSON *token = cJSON_GetObjectItem(prov, "node_token");
    if (token && cJSON_IsString(token)) {
        nvs_set_str(nvs, NVS_KEY_NODE_TOKEN, token->valuestring);
    }

    cJSON *mdns_name = cJSON_GetObjectItem(prov, "ms_mdns");
    if (mdns_name && cJSON_IsString(mdns_name)) {
        nvs_set_str(nvs, NVS_KEY_MS_MDNS, mdns_name->valuestring);
    }

    cJSON *ms_ip = cJSON_GetObjectItem(prov, "ms_ip");
    if (ms_ip && cJSON_IsString(ms_ip) && strlen(ms_ip->valuestring) > 0) {
        nvs_set_str(nvs, NVS_KEY_MS_IP, ms_ip->valuestring);
        nvs_set_str(nvs, NVS_KEY_MS_IP_PROV, ms_ip->valuestring);
    }

    cJSON *port = cJSON_GetObjectItem(prov, "ms_port");
    if (port && cJSON_IsNumber(port) && port->valueint > 0) {
        nvs_set_u16(nvs, NVS_KEY_MS_PORT, (uint16_t)port->valueint);
    }

    cJSON *debug_flag = cJSON_GetObjectItem(prov, "debug");
    if (debug_flag) {
        nvs_set_u8(nvs, NVS_KEY_DEBUG, cJSON_IsTrue(debug_flag) ? 1 : 0);
    }

    cJSON *ntp_server = cJSON_GetObjectItem(prov, "ntp_server");
    if (ntp_server && cJSON_IsString(ntp_server)) {
        nvs_set_str(nvs, NVS_KEY_NTP_SERVER, ntp_server->valuestring);
    }

    nvs_set_u8(nvs, NVS_KEY_PROVISIONED, 1);
    nvs_set_u8(nvs, NVS_KEY_SCHEMA_VER, NVS_SCHEMA_VERSION);

    err = nvs_commit(nvs);
    nvs_close(nvs);

    if (err == ESP_OK) {
        g_state.provisioned = true;
    }
    return err;
}
