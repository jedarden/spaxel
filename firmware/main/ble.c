#include "ble.h"
#include "spaxel.h"
#include "websocket.h"
#include "esp_log.h"
#include "esp_bt.h"
#include "esp_bt_main.h"
#include "esp_gap_ble_api.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "freertos/semphr.h"
#include "cJSON.h"
#include <string.h>

static const char *TAG = "ble";

// Device cache
#define MAX_BLE_DEVICES 60
static ble_device_t s_devices[MAX_BLE_DEVICES];
static int s_device_count = 0;
static SemaphoreHandle_t s_devices_mutex = NULL;
static TaskHandle_t s_ble_task = NULL;
static volatile bool s_scanning = false;

// Forward declarations
static void gap_event_handler(esp_gap_ble_cb_event_t event,
                               esp_ble_gap_cb_param_t *param);
static void ble_scan_task(void *arg);
static int find_device_by_addr(uint8_t *addr);
static void update_or_add_device(esp_ble_gap_cb_param_t *param);

esp_err_t ble_init(void) {
    ESP_LOGI(TAG, "Initializing BLE");

    // Create mutex for device cache
    s_devices_mutex = xSemaphoreCreateMutex();
    if (!s_devices_mutex) {
        ESP_LOGE(TAG, "Failed to create mutex");
        return ESP_ERR_NO_MEM;
    }

    // Initialize BT controller
    esp_bt_controller_config_t bt_cfg = BT_CONTROLLER_INIT_CONFIG_DEFAULT();

    // Allocate BT controller memory from PSRAM if available
    esp_err_t ret = esp_bt_controller_init(&bt_cfg);
    if (ret != ESP_OK) {
        ESP_LOGE(TAG, "Failed to init BT controller: %s", esp_err_to_name(ret));
        return ret;
    }

    // Enable BT controller in BLE only mode
    ret = esp_bt_controller_enable(ESP_BT_MODE_BLE);
    if (ret != ESP_OK) {
        ESP_LOGE(TAG, "Failed to enable BT controller: %s", esp_err_to_name(ret));
        return ret;
    }

    // Initialize Bluedroid stack
    ret = esp_bluedroid_init();
    if (ret != ESP_OK) {
        ESP_LOGE(TAG, "Failed to init Bluedroid: %s", esp_err_to_name(ret));
        return ret;
    }

    ret = esp_bluedroid_enable();
    if (ret != ESP_OK) {
        ESP_LOGE(TAG, "Failed to enable Bluedroid: %s", esp_err_to_name(ret));
        return ret;
    }

    // Register GAP callback
    ret = esp_ble_gap_register_callback(gap_event_handler);
    if (ret != ESP_OK) {
        ESP_LOGE(TAG, "Failed to register GAP callback: %s", esp_err_to_name(ret));
        return ret;
    }

    // Configure scan parameters
    static esp_ble_scan_params_t scan_params = {
        .scan_type = BLE_SCAN_TYPE_PASSIVE,
        .own_addr_type = BLE_ADDR_TYPE_PUBLIC,
        .scan_filter_policy = BLE_SCAN_FILTER_ALLOW_ALL,
        .scan_interval = 0x50,  // 50 ms
        .scan_window = 0x30,    // 30 ms
        .scan_duplicate = BLE_SCAN_DUPLICATE_ENABLE,
    };

    ret = esp_ble_gap_set_scan_params(&scan_params);
    if (ret != ESP_OK) {
        ESP_LOGE(TAG, "Failed to set scan params: %s", esp_err_to_name(ret));
        return ret;
    }

    // Start BLE reporting task on Core 0
    xTaskCreatePinnedToCore(ble_scan_task, "ble_scan", 4096, NULL, 5, &s_ble_task, 0);

    ESP_LOGI(TAG, "BLE initialized");
    return ESP_OK;
}

static void gap_event_handler(esp_gap_ble_cb_event_t event,
                               esp_ble_gap_cb_param_t *param) {
    switch (event) {
        case ESP_GAP_BLE_SCAN_PARAM_SET_COMPLETE_EVT:
            ESP_LOGD(TAG, "Scan params set complete");
            break;

        case ESP_GAP_BLE_SCAN_START_COMPLETE_EVT:
            if (param->scan_start_cmpl.status != ESP_BT_STATUS_SUCCESS) {
                ESP_LOGE(TAG, "Scan start failed");
            } else {
                ESP_LOGI(TAG, "BLE scan started");
                s_scanning = true;
            }
            break;

        case ESP_GAP_BLE_SCAN_STOP_COMPLETE_EVT:
            if (param->scan_stop_cmpl.status != ESP_BT_STATUS_SUCCESS) {
                ESP_LOGE(TAG, "Scan stop failed");
            } else {
                ESP_LOGI(TAG, "BLE scan stopped");
                s_scanning = false;
            }
            break;

        case ESP_GAP_BLE_SCAN_RESULT_EVT:
            if (param->scan_rst.search_evt == ESP_GAP_SEARCH_INQ_RES_EVT) {
                update_or_add_device(param);
            }
            break;

        default:
            break;
    }
}

static int find_device_by_addr(uint8_t *addr) {
    for (int i = 0; i < s_device_count; i++) {
        if (memcmp(s_devices[i].addr, addr, 6) == 0) {
            return i;
        }
    }
    return -1;
}

static void update_or_add_device(esp_ble_gap_cb_param_t *param) {
    xSemaphoreTake(s_devices_mutex, portMAX_DELAY);

    int idx = find_device_by_addr(param->scan_rst.bda);

    if (idx >= 0) {
        // Update existing device
        s_devices[idx].rssi = param->scan_rst.rssi;
        s_devices[idx].addr_type = param->scan_rst.ble_addr_type;
    } else if (s_device_count < MAX_BLE_DEVICES) {
        // Add new device
        idx = s_device_count++;
        memcpy(s_devices[idx].addr, param->scan_rst.bda, 6);
        s_devices[idx].addr_type = param->scan_rst.ble_addr_type;
        s_devices[idx].rssi = param->scan_rst.rssi;
        s_devices[idx].name[0] = '\0';
        s_devices[idx].mfr_id = 0;
        s_devices[idx].mfr_data_len = 0;

        // Parse advertising data for name and manufacturer data
        uint8_t *adv_data = param->scan_rst.ble_adv;
        uint8_t adv_len = param->scan_rst.adv_data_len;

        int i = 0;
        while (i < adv_len) {
            uint8_t field_len = adv_data[i];
            if (field_len == 0 || i + field_len >= adv_len) break;

            uint8_t field_type = adv_data[i + 1];

            if (field_type == 0x09) {  // Complete Local Name
                int name_len = field_len - 1;
                if (name_len > 31) name_len = 31;
                memcpy(s_devices[idx].name, &adv_data[i + 2], name_len);
                s_devices[idx].name[name_len] = '\0';
            } else if (field_type == 0xFF) {  // Manufacturer Specific Data
                if (field_len >= 3) {
                    s_devices[idx].mfr_id = adv_data[i + 2] | (adv_data[i + 3] << 8);
                    int mfr_len = field_len - 3;
                    if (mfr_len > 32) mfr_len = 32;
                    memcpy(s_devices[idx].mfr_data, &adv_data[i + 4], mfr_len);
                    s_devices[idx].mfr_data_len = mfr_len;
                }
            }

            i += field_len + 1;
        }
    }

    xSemaphoreGive(s_devices_mutex);
}

static void ble_scan_task(void *arg) {
    // Start scanning
    ble_start_scan();

    while (1) {
        vTaskDelay(pdMS_TO_TICKS(SPAXEL_BLE_INTERVAL_MS));

        if (g_state.state == NODE_STATE_CONNECTED && websocket_is_connected()) {
            // Send BLE scan results to mothership
            char *json = ble_get_devices_json();
            if (json) {
                websocket_send_ble(json);
                free(json);
            }
        }
    }
}

esp_err_t ble_start_scan(void) {
    if (s_scanning) {
        return ESP_OK;
    }

    esp_err_t ret = esp_ble_gap_start_scanning(0);  // 0 = continuous
    if (ret != ESP_OK) {
        ESP_LOGE(TAG, "Failed to start scan: %s", esp_err_to_name(ret));
        return ret;
    }

    return ESP_OK;
}

esp_err_t ble_stop_scan(void) {
    if (!s_scanning) {
        return ESP_OK;
    }

    esp_err_t ret = esp_ble_gap_stop_scanning();
    if (ret != ESP_OK) {
        ESP_LOGE(TAG, "Failed to stop scan: %s", esp_err_to_name(ret));
        return ret;
    }

    return ESP_OK;
}

char *ble_get_devices_json(void) {
    xSemaphoreTake(s_devices_mutex, portMAX_DELAY);

    cJSON *devices = cJSON_CreateArray();

    for (int i = 0; i < s_device_count; i++) {
        cJSON *dev = cJSON_CreateObject();

        // Address as string
        char addr_str[18];
        snprintf(addr_str, sizeof(addr_str), "%02X:%02X:%02X:%02X:%02X:%02X",
                 s_devices[i].addr[0], s_devices[i].addr[1],
                 s_devices[i].addr[2], s_devices[i].addr[3],
                 s_devices[i].addr[4], s_devices[i].addr[5]);
        cJSON_AddStringToObject(dev, "addr", addr_str);

        cJSON_AddStringToObject(dev, "addr_type",
                                 s_devices[i].addr_type == 0 ? "public" : "random");
        cJSON_AddNumberToObject(dev, "rssi_dbm", s_devices[i].rssi);

        if (s_devices[i].name[0]) {
            cJSON_AddStringToObject(dev, "name", s_devices[i].name);
        }

        if (s_devices[i].mfr_id != 0) {
            cJSON_AddNumberToObject(dev, "mfr_id", s_devices[i].mfr_id);

            // Convert manufacturer data to hex string
            if (s_devices[i].mfr_data_len > 0) {
                char *hex = malloc(s_devices[i].mfr_data_len * 2 + 1);
                if (hex) {
                    for (int j = 0; j < s_devices[i].mfr_data_len; j++) {
                        snprintf(hex + j * 2, 3, "%02X", s_devices[i].mfr_data[j]);
                    }
                    cJSON_AddStringToObject(dev, "mfr_data_hex", hex);
                    free(hex);
                }
            }
        }

        cJSON_AddItemToArray(devices, dev);
    }

    xSemaphoreGive(s_devices_mutex);

    char *json = cJSON_PrintUnformatted(devices);
    cJSON_Delete(devices);

    return json;
}

int ble_get_device_count(void) {
    xSemaphoreTake(s_devices_mutex, portMAX_DELAY);
    int count = s_device_count;
    xSemaphoreGive(s_devices_mutex);
    return count;
}

void ble_clear_devices(void) {
    xSemaphoreTake(s_devices_mutex, portMAX_DELAY);
    s_device_count = 0;
    memset(s_devices, 0, sizeof(s_devices));
    xSemaphoreGive(s_devices_mutex);
}
