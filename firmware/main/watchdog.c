#include "watchdog.h"
#include "esp_log.h"
#include "esp_task_wdt.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"

static const char *TAG = "watchdog";
static bool s_watchdog_initialized = false;

// Validation note: the task watchdog cannot be exercised under QEMU. A control
// build arming this 90s window with a task that deliberately never fed ran 30s
// past its due reset in the emulator without resetting, so a "no reset within
// Ns" QEMU uptime run is inert evidence in both directions. Trip behaviour is
// validated on-target only, on the bench rig; QEMU remains useful for every
// other behaviour (spaxel-69cdd68c).
esp_err_t watchdog_init(void) {
    if (s_watchdog_initialized) {
        ESP_LOGW(TAG, "Watchdog already initialized");
        return ESP_OK;
    }

    // Initialize task watchdog (IDF 5.x API: takes a config struct, not a timeout).
    // The TWDT is already initialized at startup (CONFIG_ESP_TASK_WDT_INIT=y) at
    // the 5s Kconfig default — CONFIG_ESP_TASK_WDT_TIMEOUT_S is declared
    // "range 1 60" in components/esp_system/Kconfig, so the value in
    // sdkconfig.defaults is dropped on every reconfigure. This call is the only
    // way to arm a window beyond 60s, and it is what makes the effective
    // timeout 90s rather than that 5s default.
    const esp_task_wdt_config_t wdt_config = {
        .timeout_ms = SPAXEL_WATCHDOG_TIMEOUT_S * 1000,
        .idle_core_mask = (1 << portNUM_PROCESSORS) - 1,
        .trigger_panic = true,
    };
    esp_err_t err = esp_task_wdt_init(&wdt_config);
    if (err == ESP_ERR_INVALID_STATE) {
        // Already initialized by startup - reconfigure instead
        err = esp_task_wdt_reconfigure(&wdt_config);
    }
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to initialize task watchdog: %s", esp_err_to_name(err));
        return err;
    }

    // Subscribe only the idle tasks (done above via idle_core_mask, fed by
    // IDF's idle hook). The calling task is deliberately NOT subscribed: this
    // runs in app_main, which returns as soon as the worker tasks are spawned.
    // A subscribed task must reset its own entry via esp_task_wdt_reset() —
    // the TWDT fires on any entry still showing has_reset == false when the
    // window expires — so a subscription here was a 90-second reboot timer on
    // an otherwise healthy node. Long-lived tasks opt in via
    // watchdog_subscribe() instead; see main.c state_machine_task().

    s_watchdog_initialized = true;
    ESP_LOGI(TAG, "Task watchdog initialized (timeout: %d seconds)", SPAXEL_WATCHDOG_TIMEOUT_S);
    ESP_LOGI(TAG, "Watchdog timeout > 60s boot validation window avoids ESPHome regression");

    return ESP_OK;
}

esp_err_t watchdog_subscribe(void) {
    if (!s_watchdog_initialized) {
        ESP_LOGW(TAG, "Watchdog not initialized - %s not subscribed", pcTaskGetName(NULL));
        return ESP_ERR_INVALID_STATE;
    }

    // NULL means "the calling task", which is the only task that can feed it.
    esp_err_t err = esp_task_wdt_add(NULL);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to subscribe %s to watchdog: %s",
                 pcTaskGetName(NULL), esp_err_to_name(err));
        return err;
    }

    ESP_LOGI(TAG, "Task %s subscribed to watchdog", pcTaskGetName(NULL));
    return ESP_OK;
}

esp_err_t watchdog_feed(void) {
    if (!s_watchdog_initialized) {
        ESP_LOGW(TAG, "Watchdog not initialized - feed ignored");
        return ESP_ERR_INVALID_STATE;
    }

    // Reset this task's own entry. Without it the entry keeps has_reset == false
    // and the TWDT fires on expiry — yielding or blocking does not feed it.
    esp_err_t err = esp_task_wdt_reset();
    if (err == ESP_ERR_NOT_FOUND) {
        ESP_LOGW(TAG, "%s is not subscribed to the watchdog - feed ignored",
                 pcTaskGetName(NULL));
    }
    return err;
}
