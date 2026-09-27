/*
 * Host stand-in for ESP-IDF's esp_timer.h — declarations only.
 *
 * Signatures mirror the ESP-IDF API subset firmware/main/safe_mode.c uses, so
 * that file compiles unmodified. The recording implementation backing them
 * lives in the test that includes the production TU
 * (firmware/test/test_safe_mode.c); like the other host_compat headers, this
 * file stays free of behavior so it remains a faithful API mirror.
 */
#pragma once

#include <stdbool.h>
#include <stdint.h>

#include "esp_err.h"

typedef void *esp_timer_handle_t;

typedef void (*esp_timer_cb_t)(void *arg);

typedef enum {
    ESP_TIMER_TASK = 0,
    ESP_TIMER_ISR,
} esp_timer_dispatch_t;

typedef struct {
    esp_timer_cb_t callback;
    void *arg;
    esp_timer_dispatch_t dispatch_method;
    const char *name;
    bool skip_unhandled_events;
} esp_timer_create_args_t;

esp_err_t esp_timer_create(const esp_timer_create_args_t *create_args,
                           esp_timer_handle_t *out_handle);
esp_err_t esp_timer_start_once(esp_timer_handle_t timer, uint64_t timeout_us);
esp_err_t esp_timer_stop(esp_timer_handle_t timer);
