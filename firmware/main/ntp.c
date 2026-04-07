#include "ntp.h"
#include "esp_log.h"
#include "esp_sntp.h"
#include "esp_timer.h"
#include "freertos/FreeRTOS.h"
#include "freertos/event_groups.h"
#include <string.h>

static const char *TAG = "ntp";

// NTP sync event bits
#define NTP_SYNC_BIT BIT0

static EventGroupHandle_t s_ntp_events = NULL;
static esp_timer_handle_t s_resync_timer = NULL;
static bool s_is_synced = false;
static char s_ntp_server[64] = "pool.ntp.org";

// Resync interval: 10 minutes (600 seconds)
#define NTP_RESYNC_INTERVAL_US (600LL * 1000000LL)

// SNTP callback - called when time sync completes
static void sntp_sync_time_callback(struct timeval *tv) {
    if (tv) {
        ESP_LOGI(TAG, "NTP synchronized: %lld.%06ld", (long long)tv->tv_sec, tv->tv_usec);
        s_is_synced = true;
        if (s_ntp_events) {
            xEventGroupSetBits(s_ntp_events, NTP_SYNC_BIT);
        }
    } else {
        ESP_LOGW(TAG, "NTP sync callback received NULL timeval");
    }
}

// Periodic resync timer callback
static void periodic_resync_callback(void *arg) {
    ESP_LOGI(TAG, "Periodic NTP resync triggered");
    esp_sntp_setservername(0, s_ntp_server);
    esp_sntp_init();
    // No need to wait here - the callback will handle completion
}

esp_err_t ntp_init(void) {
    if (s_ntp_events == NULL) {
        s_ntp_events = xEventGroupCreate();
        if (!s_ntp_events) {
            ESP_LOGE(TAG, "Failed to create event group");
            return ESP_ERR_NO_MEM;
        }
    }

    ESP_LOGI(TAG, "NTP client initialized (server: %s)", s_ntp_server);
    return ESP_OK;
}

esp_err_t ntp_start_sync(const char *ntp_server) {
    if (!ntp_server) {
        ntp_server = "pool.ntp.org";
    }

    // Store server for resync
    strncpy(s_ntp_server, ntp_server, sizeof(s_ntp_server) - 1);
    s_ntp_server[sizeof(s_ntp_server) - 1] = '\0';

    ESP_LOGI(TAG, "Starting NTP sync with server: %s", s_ntp_server);

    // Clear previous sync state
    s_is_synced = false;
    if (s_ntp_events) {
        xEventGroupClearBits(s_ntp_events, NTP_SYNC_BIT);
    }

    // Configure SNTP
    esp_sntp_setoperatingmode(SNTP_OPMODE_POLL);
    esp_sntp_setservername(0, s_ntp_server);

    // Set sync callback
    sntp_set_time_sync_notification_cb(sntp_sync_time_callback);

    // Start SNTP
    esp_sntp_init();

    return ESP_OK;
}

bool ntp_wait_sync(int timeout_ms) {
    if (!s_ntp_events) {
        ESP_LOGW(TAG, "NTP event group not initialized");
        return false;
    }

    TickType_t ticks = pdMS_TO_TICKS(timeout_ms);
    EventBits_t bits = xEventGroupWaitBits(s_ntp_events, NTP_SYNC_BIT,
                                          pdFALSE, pdFALSE, ticks);

    if (bits & NTP_SYNC_BIT) {
        ESP_LOGI(TAG, "NTP sync successful");
        return true;
    }

    ESP_LOGW(TAG, "NTP sync timeout after %d ms", timeout_ms);
    return false;
}

bool ntp_is_synced(void) {
    return s_is_synced;
}

const char* ntp_status_str(void) {
    return s_is_synced ? "synced" : "unsynced";
}

void ntp_start_periodic_resync(void) {
    if (s_resync_timer != NULL) {
        ESP_LOGW(TAG, "Periodic resync timer already started");
        return;
    }

    const esp_timer_create_args_t timer_args = {
        .callback = &periodic_resync_callback,
        .name = "ntp_resync",
    };

    esp_err_t err = esp_timer_create(&timer_args, &s_resync_timer);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to create resync timer: %s", esp_err_to_name(err));
        return;
    }

    err = esp_timer_start_periodic(s_resync_timer, NTP_RESYNC_INTERVAL_US);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to start resync timer: %s", esp_err_to_name(err));
        esp_timer_delete(s_resync_timer);
        s_resync_timer = NULL;
        return;
    }

    ESP_LOGI(TAG, "Periodic NTP resync started (interval: %lld seconds)",
             NTP_RESYNC_INTERVAL_US / 1000000LL);
}

void ntp_stop(void) {
    esp_sntp_stop();

    if (s_resync_timer) {
        esp_timer_stop(s_resync_timer);
        esp_timer_delete(s_resync_timer);
        s_resync_timer = NULL;
    }

    s_is_synced = false;
    ESP_LOGI(TAG, "NTP client stopped");
}
