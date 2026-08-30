#include "safe_mode.h"
#include "spaxel.h"
#include "esp_log.h"
#include "esp_timer.h"
#include "nvs.h"
#include <string.h>

static const char *TAG = "safe_mode";

// State
static bool s_safe_mode_active = false;
static uint32_t s_boot_count = 0;
static esp_timer_handle_t s_boot_good_timer = NULL;
static esp_timer_handle_t s_exit_timer = NULL;

// NVS helpers
static esp_err_t nvs_get_u32_default(const char *key, uint32_t default_val, uint32_t *out) {
    nvs_handle_t nvs;
    esp_err_t err = nvs_open(SPAXEL_NAMESPACE, NVS_READONLY, &nvs);
    if (err == ESP_OK) {
        err = nvs_get_u32(nvs, key, out);
        nvs_close(nvs);
        if (err != ESP_OK) {
            *out = default_val;
            return ESP_OK;
        }
        return ESP_OK;
    }
    *out = default_val;
    return err;
}

static esp_err_t nvs_set_u32(const char *key, uint32_t val) {
    nvs_handle_t nvs;
    esp_err_t err = nvs_open(SPAXEL_NAMESPACE, NVS_READWRITE, &nvs);
    if (err != ESP_OK) {
        return err;
    }
    err = nvs_set_u32(nvs, key, val);
    if (err == ESP_OK) {
        nvs_commit(nvs);
    }
    nvs_close(nvs);
    return err;
}

static esp_err_t nvs_set_u8(const char *key, uint8_t val) {
    nvs_handle_t nvs;
    esp_err_t err = nvs_open(SPAXEL_NAMESPACE, NVS_READWRITE, &nvs);
    if (err != ESP_OK) {
        return err;
    }
    err = nvs_set_u8(nvs, key, val);
    if (err == ESP_OK) {
        nvs_commit(nvs);
    }
    nvs_close(nvs);
    return err;
}

// Timer callbacks
static void boot_good_timer_cb(void *arg) {
    ESP_LOGI(TAG, "Boot validation timer expired - marking boot as good");
    safe_mode_mark_boot_good();
}

static void exit_timer_cb(void *arg) {
    ESP_LOGW(TAG, "Safe mode exit timeout - rebooting to attempt normal boot");
    ESP_LOGW(TAG, "Set restarting flag before safe mode exit restart");
    g_state.restarting = true;
    esp_restart();
}

// API implementation
esp_err_t safe_mode_init(void) {
    // Load boot counter from NVS
    nvs_get_u32_default(NVS_KEY_BOOT_COUNTER, 0, &s_boot_count);

    // Load safe mode flag from NVS
    nvs_handle_t nvs;
    esp_err_t err = nvs_open(SPAXEL_NAMESPACE, NVS_READONLY, &nvs);
    if (err == ESP_OK) {
        uint8_t safe_mode_flag = 0;
        if (nvs_get_u8(nvs, NVS_KEY_SAFE_MODE, &safe_mode_flag) == ESP_OK) {
            s_safe_mode_active = (safe_mode_flag == SAFE_MODE_ENABLED);
        }
        nvs_close(nvs);
    }

    if (s_safe_mode_active) {
        ESP_LOGW(TAG, "SAFE MODE ACTIVE - Network + OTA only, CSI/BLE disabled");
        ESP_LOGW(TAG, "Boot failure count: %u", s_boot_count);
    } else if (s_boot_count > 0) {
        ESP_LOGI(TAG, "Boot failure count: %u (threshold: %d)",
                 s_boot_count, SAFE_MODE_BOOT_COUNT_THRESHOLD);
    }

    // Create timers (don't start them yet)
    if (s_boot_good_timer == NULL) {
        esp_timer_create_args_t timer_args = {
            .callback = boot_good_timer_cb,
            .name = "boot_good",
        };
        err = esp_timer_create(&timer_args, &s_boot_good_timer);
        if (err != ESP_OK) {
            ESP_LOGE(TAG, "Failed to create boot good timer: %s", esp_err_to_name(err));
            return err;
        }
    }

    if (s_exit_timer == NULL) {
        esp_timer_create_args_t timer_args = {
            .callback = exit_timer_cb,
            .name = "safe_exit",
        };
        err = esp_timer_create(&timer_args, &s_exit_timer);
        if (err != ESP_OK) {
            ESP_LOGE(TAG, "Failed to create exit timer: %s", esp_err_to_name(err));
            return err;
        }
    }

    return ESP_OK;
}

bool safe_mode_is_active(void) {
    return s_safe_mode_active;
}

esp_err_t safe_mode_mark_boot_good(void) {
    if (s_boot_count == 0) {
        ESP_LOGD(TAG, "Boot count already zero - nothing to clear");
        return ESP_OK;
    }

    ESP_LOGI(TAG, "Marking boot as good - resetting boot count from %u to 0", s_boot_count);
    s_boot_count = 0;
    esp_err_t err = nvs_set_u32(NVS_KEY_BOOT_COUNTER, 0);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to reset boot counter: %s", esp_err_to_name(err));
        return err;
    }

    // Stop the exit timer if running
    if (s_exit_timer != NULL) {
        esp_timer_stop(s_exit_timer);
    }

    ESP_LOGI(TAG, "Boot validation complete - firmware is stable");
    return ESP_OK;
}

esp_err_t safe_mode_mark_boot_failed(void) {
    s_boot_count++;
    ESP_LOGW(TAG, "Boot failure - incrementing count to %u (threshold: %d)",
             s_boot_count, SAFE_MODE_BOOT_COUNT_THRESHOLD);

    esp_err_t err = nvs_set_u32(NVS_KEY_BOOT_COUNTER, s_boot_count);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to save boot counter: %s", esp_err_to_name(err));
        return err;
    }

    // Check if we should enable safe mode
    if (s_boot_count >= SAFE_MODE_BOOT_COUNT_THRESHOLD) {
        ESP_LOGE(TAG, "Boot failure threshold reached - entering safe mode on next boot");
        return safe_mode_enter();
    }

    return ESP_OK;
}

esp_err_t safe_mode_enter(void) {
    ESP_LOGW(TAG, "Entering safe mode - only network + OTA will be available");
    s_safe_mode_active = true;

    esp_err_t err = nvs_set_u8(NVS_KEY_SAFE_MODE, SAFE_MODE_ENABLED);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to set safe mode flag: %s", esp_err_to_name(err));
        return err;
    }

    return ESP_OK;
}

esp_err_t safe_mode_exit(void) {
    ESP_LOGI(TAG, "Exiting safe mode - normal operation will resume on next boot");
    s_safe_mode_active = false;

    esp_err_t err = nvs_set_u8(NVS_KEY_SAFE_MODE, SAFE_MODE_DISABLED);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to clear safe mode flag: %s", esp_err_to_name(err));
        return err;
    }

    // Also reset boot count since we're exiting safe mode
    s_boot_count = 0;
    err = nvs_set_u32(NVS_KEY_BOOT_COUNTER, 0);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to reset boot counter: %s", esp_err_to_name(err));
        return err;
    }

    return ESP_OK;
}

uint32_t safe_mode_get_boot_count(void) {
    return s_boot_count;
}

esp_err_t safe_mode_start_boot_good_timer(void) {
    if (s_boot_good_timer == NULL) {
        ESP_LOGE(TAG, "Boot good timer not initialized");
        return ESP_ERR_INVALID_STATE;
    }

    // Don't start the timer in safe mode - we want to exit quickly
    if (s_safe_mode_active) {
        ESP_LOGD(TAG, "Skipping boot good timer - in safe mode");
        return ESP_OK;
    }

    ESP_LOGI(TAG, "Starting boot validation timer (%d seconds)",
             SAFE_MODE_BOOT_GOOD_AFTER_S);
    esp_timer_stop(s_boot_good_timer);  // Ensure not already running
    esp_timer_start_once(s_boot_good_timer,
                         SAFE_MODE_BOOT_GOOD_AFTER_S * 1000000ULL);

    return ESP_OK;
}

esp_err_t safe_mode_stop_boot_good_timer(void) {
    if (s_boot_good_timer != NULL) {
        esp_timer_stop(s_boot_good_timer);
    }
    return ESP_OK;
}

esp_err_t safe_mode_start_exit_timer(void) {
    if (s_exit_timer == NULL) {
        ESP_LOGE(TAG, "Exit timer not initialized");
        return ESP_ERR_INVALID_STATE;
    }

    if (!s_safe_mode_active) {
        ESP_LOGD(TAG, "Not starting exit timer - not in safe mode");
        return ESP_OK;
    }

    ESP_LOGW(TAG, "Starting safe mode exit timer (%d seconds) - will reboot to normal mode",
             SAFE_MODE_REBOOT_TIMEOUT_S);
    esp_timer_stop(s_exit_timer);  // Ensure not already running
    esp_timer_start_once(s_exit_timer,
                         SAFE_MODE_REBOOT_TIMEOUT_S * 1000000ULL);

    return ESP_OK;
}
