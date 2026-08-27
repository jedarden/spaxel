#include "ble.h"
#include "spaxel.h"
#include "websocket.h"
#include "esp_log.h"
#include "esp_nimble_hci.h"
#include "nimble/nimble_port.h"
#include "host/ble_hs.h"
#include "host/ble_gap.h"
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
static void ble_scan_task(void *arg);
static int find_device_by_addr(const uint8_t *addr);
static void update_or_add_device(const struct ble_gap_disc_desc *desc);

// NimBLE GAP event handling
static int ble_gap_event(struct ble_gap_event *event, void *arg) {
    switch (event->type) {
        case BLE_GAP_EVENT_DISC:
            // New device discovered or scan response received
            update_or_add_device(&event->disc);
            break;

        case BLE_GAP_EVENT_DISC_COMPLETE:
            ESP_LOGI(TAG, "BLE scan complete");
            s_scanning = false;
            break;

        case BLE_GAP_EVENT_ADV_COMPLETE:
            // No-op for scanning
            break;

        default:
            break;
    }
    return 0;
}

esp_err_t ble_init(void) {
    ESP_LOGI(TAG, "Initializing BLE with NimBLE");

    // Create mutex for device cache
    s_devices_mutex = xSemaphoreCreateMutex();
    if (!s_devices_mutex) {
        ESP_LOGE(TAG, "Failed to create mutex");
        return ESP_ERR_NO_MEM;
    }

    // Initialize the NimBLE host
    ble_hs_cfg.sync_cb = NULL;  // No sync callback needed for scan-only
    ble_hs_cfg.gatts_register_cb = NULL;
    ble_hs_cfg.store_status_cb = NULL;
    ble_hs_cfg.reset_cb = NULL;

    // Start BLE scanning task
    xTaskCreatePinnedToCore(ble_scan_task, "ble_scan", 4096, NULL, 5, &s_ble_task, 0);

    ESP_LOGI(TAG, "BLE initialized with NimBLE");
    return ESP_OK;
}

static void update_or_add_device(const struct ble_gap_disc_desc *desc) {
    xSemaphoreTake(s_devices_mutex, portMAX_DELAY);

    int idx = find_device_by_addr(desc->addr.val);

    if (idx >= 0) {
        // Update existing device
        s_devices[idx].rssi = desc->rssi;
        s_devices[idx].addr_type = desc->addr.type;
    } else if (s_device_count < MAX_BLE_DEVICES) {
        // Add new device
        idx = s_device_count++;
        memcpy(s_devices[idx].addr, desc->addr.val, 6);
        s_devices[idx].addr_type = desc->addr.type;
        s_devices[idx].rssi = desc->rssi;
        s_devices[idx].name[0] = '\0';
        s_devices[idx].mfr_id = 0;
        s_devices[idx].mfr_data_len = 0;

        // Parse advertising data
        const uint8_t *adv_data = desc->data;
        uint8_t adv_len = desc->length_data;

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

static int find_device_by_addr(const uint8_t *addr) {
    for (int i = 0; i < s_device_count; i++) {
        if (memcmp(s_devices[i].addr, addr, 6) == 0) {
            return i;
        }
    }
    return -1;
}

esp_err_t ble_start_scan(void) {
    if (s_scanning) {
        return ESP_OK;
    }

    // Set scan parameters
    struct ble_gap_disc_params scan_params = {
        .itvl = 0x50,        // 50 ms
        .window = 0x30,     // 30 ms
        .filter_duplicates = 1,
    };

    // Start scanning (0 = own_addr_type public, 0 = duration_ms = infinite)
    int rc = ble_gap_disc(0, 0, &scan_params, ble_gap_event, NULL);
    if (rc != 0) {
        ESP_LOGE(TAG, "Failed to start scan: %d", rc);
        return ESP_FAIL;
    }

    s_scanning = true;
    ESP_LOGI(TAG, "BLE scan started");
    return ESP_OK;
}

esp_err_t ble_stop_scan(void) {
    if (!s_scanning) {
        return ESP_OK;
    }

    int rc = ble_gap_disc_cancel();
    if (rc != 0 && rc != BLE_HS_EALREADY) {
        ESP_LOGE(TAG, "Failed to stop scan: %d", rc);
        return ESP_FAIL;
    }

    s_scanning = false;
    ESP_LOGI(TAG, "BLE scan stopped");
    return ESP_OK;
}

static void ble_scan_task(void *arg) {
    // Wait for NimBLE host to start
    // Small delay to ensure BLE host is ready
    vTaskDelay(pdMS_TO_TICKS(1000));

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
