#include "provision.h"
#include "spaxel.h"
#include "esp_log.h"
#include "nvs_flash.h"
#include "nvs.h"
#include "cJSON.h"
#include "driver/uart.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include <string.h>

static const char *TAG = "provision";

#define PROVISION_UART              UART_NUM_0
#define PROVISION_BAUD_RATE         115200
#define PROVISION_WINDOW_MS_FRESH   120000  // 2 min for unprovisioned boards
#define PROVISION_WINDOW_MS_REPROV   15000  // 15 s for already-provisioned boards
#define UART_RX_BUF_SIZE            1024
#define MAX_LINE_LEN                768

void provision_listen_window(void) {
    uart_config_t uart_cfg = {
        .baud_rate  = PROVISION_BAUD_RATE,
        .data_bits  = UART_DATA_8_BITS,
        .parity     = UART_PARITY_DISABLE,
        .stop_bits  = UART_STOP_BITS_1,
        .flow_ctrl  = UART_HW_FLOWCTRL_DISABLE,
        .source_clk = UART_SCLK_DEFAULT,
    };

    if (uart_param_config(PROVISION_UART, &uart_cfg) != ESP_OK) {
        ESP_LOGW(TAG, "UART param config failed, skipping provision window");
        return;
    }
    if (uart_driver_install(PROVISION_UART, UART_RX_BUF_SIZE, 0, 0, NULL, 0) != ESP_OK) {
        ESP_LOGW(TAG, "UART driver install failed, skipping provision window");
        return;
    }

    char mac_str[18];
    mac_to_str(g_state.mac, mac_str, sizeof(mac_str));

    uint32_t window_ms = g_state.provisioned
        ? PROVISION_WINDOW_MS_REPROV
        : PROVISION_WINDOW_MS_FRESH;

    // Signal that firmware is ready for provisioning (includes MAC for display)
    char ready_msg[64];
    snprintf(ready_msg, sizeof(ready_msg), "SPAXEL READY %s\n", mac_str);
    uart_write_bytes(PROVISION_UART, ready_msg, strlen(ready_msg));

    ESP_LOGI(TAG, "Provisioning window open for %u ms (MAC: %s)", (unsigned)window_ms, mac_str);

    TickType_t deadline = xTaskGetTickCount() + pdMS_TO_TICKS(window_ms);
    char line[MAX_LINE_LEN];
    int line_pos = 0;

    while (xTaskGetTickCount() < deadline) {
        uint8_t ch;
        int n = uart_read_bytes(PROVISION_UART, &ch, 1, pdMS_TO_TICKS(50));
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
                uart_write_bytes(PROVISION_UART, err_resp, strlen(err_resp));
                continue;
            }

            cJSON *prov = cJSON_GetObjectItem(root, "provision");
            if (!prov) {
                cJSON_Delete(root);
                const char *err_resp = "{\"ok\":false,\"error\":\"missing_provision_key\"}\n";
                uart_write_bytes(PROVISION_UART, err_resp, strlen(err_resp));
                continue;
            }

            esp_err_t err = provision_write_nvs(prov);
            cJSON_Delete(root);

            if (err == ESP_OK) {
                char resp[80];
                snprintf(resp, sizeof(resp), "{\"ok\":true,\"mac\":\"%s\"}\n", mac_str);
                uart_write_bytes(PROVISION_UART, resp, strlen(resp));
                ESP_LOGI(TAG, "Provisioning complete via serial");
                uart_driver_delete(PROVISION_UART);
                return;
            } else {
                const char *err_resp = "{\"ok\":false,\"error\":\"nvs_write_failed\"}\n";
                uart_write_bytes(PROVISION_UART, err_resp, strlen(err_resp));
            }
        } else if (line_pos < MAX_LINE_LEN - 1) {
            line[line_pos++] = (char)ch;
        } else {
            // Line too long — flush buffer
            line_pos = 0;
        }
    }

    ESP_LOGI(TAG, "Provisioning window closed (no provisioning received)");
    uart_driver_delete(PROVISION_UART);
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
