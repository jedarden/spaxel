#include "provision.h"
#include "transport.h"
#include "spaxel.h"
#include "esp_log.h"
#include "nvs_flash.h"
#include "nvs.h"
#include "cJSON.h"
#include "esp_vfs_dev.h"
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

// Release the transport and restore console to non-blocking mode.
// For USB-Serial-JTAG, this points stdio back at ROM/polling BEFORE destroying
// the driver — uninstalling while the VFS console still refers to the driver
// leaves esp_log writing into a torn-down driver, so the board goes permanently
// silent the moment the provisioning window closes. That is the "booted into
// the correct slot, then no console output" signature in
// docs/notes/esp32-ota-and-reconnection-handoff.md.
static void provision_release_transport(transport_t *tp) {
    if (!tp) {
        return;
    }
    if (strcmp(tp->name, "usb-serial-jtag") == 0) {
        esp_vfs_usb_serial_jtag_use_nonblocking();
    }
    transport_deinit(tp);
}

void provision_listen_window(void) {
    // Get both transports for concurrent dual-transport listening.
    transport_t *tp_usb = transport_usb_serial_jtag();
    transport_t *tp_uart = transport_uart0();

    // Initialize both transports (installs drivers, configures pins, etc.).
    esp_err_t err_usb = transport_init(tp_usb);
    esp_err_t err_uart = transport_init(tp_uart);

    bool usb_available = (err_usb == ESP_OK);
    bool uart_available = (err_uart == ESP_OK);

    if (!usb_available && !uart_available) {
        ESP_LOGW(TAG, "Both transports failed (USB: 0x%x, UART: 0x%x), skipping provision window",
                 err_usb, err_uart);
        return;
    }

    char mac_str[18];
    mac_to_str(g_state.mac, mac_str, sizeof(mac_str));

    uint32_t window_ms = g_state.provisioned
        ? PROVISION_WINDOW_MS_REPROV
        : PROVISION_WINDOW_MS_FRESH;

    // Signal that firmware is ready for provisioning (includes MAC for display).
    // Broadcast every 1 s on ALL available transports so the host can open
    // the port at any time during the window.
    char ready_msg[64];
    snprintf(ready_msg, sizeof(ready_msg), "SPAXEL READY %s\n", mac_str);

    if (usb_available) {
        transport_write(tp_usb, (const uint8_t *)ready_msg, strlen(ready_msg), PROVISION_TX_TIMEOUT);
    }
    if (uart_available) {
        transport_write(tp_uart, (const uint8_t *)ready_msg, strlen(ready_msg), PROVISION_TX_TIMEOUT);
    }

    ESP_LOGI(TAG, "Provisioning window open for %u ms on %s%s%s (MAC: %s)",
             (unsigned)window_ms,
             usb_available ? tp_usb->name : "",
             (usb_available && uart_available) ? " + " : "",
             uart_available ? tp_uart->name : "",
             mac_str);

    TickType_t deadline   = xTaskGetTickCount() + pdMS_TO_TICKS(window_ms);
    TickType_t last_ready = xTaskGetTickCount();

    // Maintain separate read buffers for each transport to avoid interleaving.
    char line_usb[MAX_LINE_LEN];
    char line_uart[MAX_LINE_LEN];
    int line_pos_usb = 0;
    int line_pos_uart = 0;

    while (xTaskGetTickCount() < deadline) {
        // Re-broadcast READY every 1 s on all available transports
        if ((xTaskGetTickCount() - last_ready) >= pdMS_TO_TICKS(1000)) {
            if (usb_available) {
                transport_write(tp_usb, (const uint8_t *)ready_msg, strlen(ready_msg), PROVISION_TX_TIMEOUT);
            }
            if (uart_available) {
                transport_write(tp_uart, (const uint8_t *)ready_msg, strlen(ready_msg), PROVISION_TX_TIMEOUT);
            }
            last_ready = xTaskGetTickCount();
        }

        // Poll both transports with a short timeout to allow responsive switching.
        // Use 25 ms per transport (50 ms total) for 1 Hz responsiveness while
        // keeping CPU usage low.
        uint8_t ch;
        int n;

        // Try USB-Serial/JTAG first
        if (usb_available) {
            n = transport_read(tp_usb, &ch, 1, pdMS_TO_TICKS(25));
            if (n > 0) {
                // Character received on USB transport
                if (ch == '\r') {
                    // ignore CR
                } else if (ch == '\n') {
                    line_usb[line_pos_usb] = '\0';
                    line_pos_usb = 0;

                    if (strlen(line_usb) > 0) {
                        // Process the JSON payload
                        cJSON *root = cJSON_Parse(line_usb);
                        if (!root) {
                            const char *err_resp = "{\"ok\":false,\"error\":\"invalid_json\"}\n";
                            transport_write(tp_usb, (const uint8_t *)err_resp, strlen(err_resp), PROVISION_TX_TIMEOUT);
                            continue;
                        }

                        cJSON *prov = cJSON_GetObjectItem(root, "provision");
                        if (!prov) {
                            cJSON_Delete(root);
                            const char *err_resp = "{\"ok\":false,\"error\":\"missing_provision_key\"}\n";
                            transport_write(tp_usb, (const uint8_t *)err_resp, strlen(err_resp), PROVISION_TX_TIMEOUT);
                            continue;
                        }

                        esp_err_t err = provision_write_nvs(prov);
                        cJSON_Delete(root);

                        if (err == ESP_OK) {
                            char resp[80];
                            snprintf(resp, sizeof(resp), "{\"ok\":true,\"mac\":\"%s\"}\n", mac_str);
                            transport_write(tp_usb, (const uint8_t *)resp, strlen(resp), PROVISION_TX_TIMEOUT);
                            ESP_LOGI(TAG, "Provisioning complete via %s", tp_usb->name);
                            provision_release_transport(tp_usb);
                            provision_release_transport(tp_uart);
                            return;
                        } else {
                            const char *err_resp = "{\"ok\":false,\"error\":\"nvs_write_failed\"}\n";
                            transport_write(tp_usb, (const uint8_t *)err_resp, strlen(err_resp), PROVISION_TX_TIMEOUT);
                        }
                    }
                } else if (line_pos_usb < MAX_LINE_LEN - 1) {
                    line_usb[line_pos_usb++] = (char)ch;
                } else {
                    // Line too long — flush buffer
                    line_pos_usb = 0;
                }
                // Continue to next iteration to maintain fair polling
                continue;
            }
        }

        // Try UART0
        if (uart_available) {
            n = transport_read(tp_uart, &ch, 1, pdMS_TO_TICKS(25));
            if (n > 0) {
                // Character received on UART transport
                if (ch == '\r') {
                    // ignore CR
                } else if (ch == '\n') {
                    line_uart[line_pos_uart] = '\0';
                    line_pos_uart = 0;

                    if (strlen(line_uart) > 0) {
                        // Process the JSON payload
                        cJSON *root = cJSON_Parse(line_uart);
                        if (!root) {
                            const char *err_resp = "{\"ok\":false,\"error\":\"invalid_json\"}\n";
                            transport_write(tp_uart, (const uint8_t *)err_resp, strlen(err_resp), PROVISION_TX_TIMEOUT);
                            continue;
                        }

                        cJSON *prov = cJSON_GetObjectItem(root, "provision");
                        if (!prov) {
                            cJSON_Delete(root);
                            const char *err_resp = "{\"ok\":false,\"error\":\"missing_provision_key\"}\n";
                            transport_write(tp_uart, (const uint8_t *)err_resp, strlen(err_resp), PROVISION_TX_TIMEOUT);
                            continue;
                        }

                        esp_err_t err = provision_write_nvs(prov);
                        cJSON_Delete(root);

                        if (err == ESP_OK) {
                            char resp[80];
                            snprintf(resp, sizeof(resp), "{\"ok\":true,\"mac\":\"%s\"}\n", mac_str);
                            transport_write(tp_uart, (const uint8_t *)resp, strlen(resp), PROVISION_TX_TIMEOUT);
                            ESP_LOGI(TAG, "Provisioning complete via %s", tp_uart->name);
                            provision_release_transport(tp_usb);
                            provision_release_transport(tp_uart);
                            return;
                        } else {
                            const char *err_resp = "{\"ok\":false,\"error\":\"nvs_write_failed\"}\n";
                            transport_write(tp_uart, (const uint8_t *)err_resp, strlen(err_resp), PROVISION_TX_TIMEOUT);
                        }
                    }
                } else if (line_pos_uart < MAX_LINE_LEN - 1) {
                    line_uart[line_pos_uart++] = (char)ch;
                } else {
                    // Line too long — flush buffer
                    line_pos_uart = 0;
                }
            }
        }
    }

    ESP_LOGI(TAG, "Provisioning window closed (no provisioning received)");
    provision_release_transport(tp_usb);
    provision_release_transport(tp_uart);
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
