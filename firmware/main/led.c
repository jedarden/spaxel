#include "led.h"
#include "esp_log.h"
#include "esp_timer.h"
#include "esp_rom_sys.h"
#include "driver/gpio.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include "soc/gpio_num.h"

static const char *TAG = "led";

// LED GPIO configuration (can be overridden via sdkconfig)
#ifndef CONFIG_SPAXEL_LED_GPIO
#define CONFIG_SPAXEL_LED_GPIO 8  // Default GPIO8 for ESP32-S3-DevKitC
#endif

#define LED_GPIO ((gpio_num_t)CONFIG_SPAXEL_LED_GPIO)

// Blink state
static struct {
    bool initialized;
    bool blinking;
    uint32_t duration_ms;
    TaskHandle_t task_handle;
} s_led_state = {
    .initialized = false,
    .blinking = false,
    .task_handle = NULL,
};

// LED blink task
static void led_blink_task(void *arg) {
    uint32_t duration_ms = (uint32_t)(uintptr_t)arg;
    ESP_LOGI(TAG, "LED blink task started for %lu ms", duration_ms);

    const TickType_t blink_on = pdMS_TO_TICKS(100);   // 100ms on
    const TickType_t blink_off = pdMS_TO_TICKS(100);  // 100ms off
    const TickType_t total_ticks = pdMS_TO_TICKS(duration_ms);
    TickType_t elapsed = 0;

    while (s_led_state.blinking && elapsed < total_ticks) {
        // Turn LED on
        gpio_set_level(LED_GPIO, 1);
        vTaskDelay(blink_on);
        elapsed += blink_on;

        if (!s_led_state.blinking || elapsed >= total_ticks) break;

        // Turn LED off
        gpio_set_level(LED_GPIO, 0);
        vTaskDelay(blink_off);
        elapsed += blink_off;
    }

    // Ensure LED is off before exiting
    gpio_set_level(LED_GPIO, 0);

    ESP_LOGI(TAG, "LED blink task finished");
    s_led_state.blinking = false;
    s_led_state.task_handle = NULL;
    vTaskDelete(NULL);
}

esp_err_t led_init(void) {
    if (s_led_state.initialized) {
        return ESP_OK;
    }

    // Configure LED GPIO as output
    gpio_reset_pin(LED_GPIO);
    gpio_set_direction(LED_GPIO, GPIO_MODE_OUTPUT);
    gpio_set_level(LED_GPIO, 0);  // Start with LED off

    ESP_LOGI(TAG, "LED initialized on GPIO %d", LED_GPIO);
    s_led_state.initialized = true;
    return ESP_OK;
}

esp_err_t led_blink_identify(uint32_t duration_ms) {
    if (!s_led_state.initialized) {
        ESP_LOGW(TAG, "LED not initialized");
        return ESP_ERR_INVALID_STATE;
    }

    // Limit duration to 60 seconds
    if (duration_ms > 60000) {
        duration_ms = 60000;
    }

    // Cancel any running blink
    led_stop_blink();

    // Start new blink task
    s_led_state.blinking = true;
    s_led_state.duration_ms = duration_ms;

    BaseType_t ret = xTaskCreate(
        led_blink_task,
        "led_blink",
        2048,
        (void *)(uintptr_t)duration_ms,
        5,  // Priority
        &s_led_state.task_handle
    );

    if (ret != pdPASS) {
        ESP_LOGE(TAG, "Failed to create LED blink task");
        s_led_state.blinking = false;
        return ESP_ERR_NO_MEM;
    }

    ESP_LOGI(TAG, "LED identify blink started for %lu ms", duration_ms);
    return ESP_OK;
}

void led_stop_blink(void) {
    if (s_led_state.blinking && s_led_state.task_handle != NULL) {
        ESP_LOGD(TAG, "Stopping LED blink");
        s_led_state.blinking = false;
        // Task will delete itself
        vTaskDelay(pdMS_TO_TICKS(50));  // Give task time to exit
    }
    // Ensure LED is off
    gpio_set_level(LED_GPIO, 0);
}

bool led_is_blinking(void) {
    return s_led_state.blinking;
}
