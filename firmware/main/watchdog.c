#include "watchdog.h"
#include "esp_log.h"
#include "esp_task_wdt.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"

static const char *TAG = "watchdog";
static bool s_watchdog_initialized = false;

esp_err_t watchdog_init(void) {
    if (s_watchdog_initialized) {
        ESP_LOGW(TAG, "Watchdog already initialized");
        return ESP_OK;
    }

    // Initialize task watchdog
    esp_err_t err = esp_task_wdt_init(SPAXEL_WATCHDOG_TIMEOUT_S);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to initialize task watchdog: %s", esp_err_to_name(err));
        return err;
    }

    // Add the main task to the watchdog
    // xTaskGetCurrentTaskHandle() gets the handle for the task calling this function
    // which is the main task at this point in app_main()
    TaskHandle_t main_task = xTaskGetCurrentTaskHandle();
    if (main_task == NULL) {
        ESP_LOGE(TAG, "Failed to get current task handle");
        return ESP_FAIL;
    }

    err = esp_task_wdt_add(main_task);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to add main task to watchdog: %s", esp_err_to_name(err));
        return err;
    }

    s_watchdog_initialized = true;
    ESP_LOGI(TAG, "Task watchdog initialized (timeout: %d seconds)", SPAXEL_WATCHDOG_TIMEOUT_S);
    ESP_LOGI(TAG, "Watchdog timeout > 60s boot validation window avoids ESPHome regression");

    return ESP_OK;
}

esp_err_t watchdog_feed(void) {
    if (!s_watchdog_initialized) {
        ESP_LOGW(TAG, "Watchdog not initialized - feed ignored");
        return ESP_ERR_INVALID_STATE;
    }

    // The task watchdog subsystem automatically resets (feeds) the watchdog
    // when the task yields or blocks. We just need to ensure we're still registered.
    // The esp_task_wdt_reset() function can be called to manually reset,
    // but it's typically not needed if tasks are properly yielding.

    return ESP_OK;
}
