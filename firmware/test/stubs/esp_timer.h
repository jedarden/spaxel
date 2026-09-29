/*
 * Host stand-in for ESP-IDF's esp_timer.h — the recording fake slice the
 * production TUs reach (spaxel-db7aab09; mirrors the declaration surface of
 * host_compat/esp_timer.h but in the stubs/ dialect, with behavior).
 *
 * Timers never fire on their own on a host: create() takes a slot with a
 * stable handle and remembers name/callback/period, start_*() arm it, and a
 * test fires a timer by name with etimer_mock_fire() to model expiry —
 * one-shot timers stop themselves, periodic ones keep running. Now is a
 * controllable counter (etimer_mock_now_us) so debounce windows and uptime
 * reads are deterministic.
 *
 * Everything is static: each production TU sees the copy its own test TU
 * expanded. Note for the link: test_safe_mode.o exports global
 * esp_timer_create/start_once/stop definitions; the statics here shadow those
 * symbols inside every TU that includes this header, so no definition
 * collides and the production code compiled into that TU binds to its own
 * private fake.
 */
#ifndef SPAXEL_TEST_STUB_ESP_TIMER_H
#define SPAXEL_TEST_STUB_ESP_TIMER_H

#include <stdbool.h>
#include <stdint.h>
#include <string.h>

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

/* ---- Recorded state --------------------------------------------------------- */

#define ETIMER_MOCK_MAX 8

typedef struct {
    esp_timer_handle_t handle;
    const char        *name;
    esp_timer_cb_t     callback;
    void              *arg;
    bool               running;
    bool               periodic;
    uint64_t           period_us;
    bool               deleted;
} etimer_mock_timer_t;

__attribute__((unused))
static etimer_mock_timer_t etimer_mock_timers[ETIMER_MOCK_MAX];
__attribute__((unused))
static int      etimer_mock_count;
__attribute__((unused))
static int      etimer_mock_create_calls;
__attribute__((unused))
static int      etimer_mock_start_once_calls;
__attribute__((unused))
static int      etimer_mock_start_periodic_calls;
__attribute__((unused))
static int      etimer_mock_stop_calls;
__attribute__((unused))
static int      etimer_mock_delete_calls;
__attribute__((unused))
static uint64_t etimer_mock_last_period_us;

/* ---- Return values the test drives ------------------------------------------- */

__attribute__((unused))
static esp_err_t etimer_mock_create_ret         = ESP_OK;
__attribute__((unused))
static esp_err_t etimer_mock_start_once_ret     = ESP_OK;
__attribute__((unused))
static esp_err_t etimer_mock_start_periodic_ret = ESP_OK;
__attribute__((unused))
static esp_err_t etimer_mock_stop_ret           = ESP_OK;

/* Controllable clock for esp_timer_get_time(). */
__attribute__((unused))
static int64_t etimer_mock_now_us = 0;

/* ---- Reset + helpers ------------------------------------------------------------ */

__attribute__((unused))
static void etimer_mock_reset(void)
{
    memset(etimer_mock_timers, 0, sizeof(etimer_mock_timers));
    etimer_mock_count                = 0;
    etimer_mock_create_calls         = 0;
    etimer_mock_start_once_calls     = 0;
    etimer_mock_start_periodic_calls = 0;
    etimer_mock_stop_calls           = 0;
    etimer_mock_delete_calls         = 0;
    etimer_mock_last_period_us       = 0;
    etimer_mock_create_ret           = ESP_OK;
    etimer_mock_start_once_ret       = ESP_OK;
    etimer_mock_start_periodic_ret   = ESP_OK;
    etimer_mock_stop_ret             = ESP_OK;
    etimer_mock_now_us               = 0;
}

__attribute__((unused))
static etimer_mock_timer_t *etimer_mock_by_name(const char *name)
{
    for (int i = 0; i < etimer_mock_count; i++) {
        if (!etimer_mock_timers[i].deleted &&
            strcmp(etimer_mock_timers[i].name, name) == 0) {
            return &etimer_mock_timers[i];
        }
    }
    return NULL;
}

__attribute__((unused))
static etimer_mock_timer_t *etimer_mock_by_handle(esp_timer_handle_t handle)
{
    for (int i = 0; i < etimer_mock_count; i++) {
        if (etimer_mock_timers[i].handle == handle) {
            return &etimer_mock_timers[i];
        }
    }
    return NULL;
}

__attribute__((unused))
static void etimer_mock_advance_us(int64_t delta_us)
{
    etimer_mock_now_us += delta_us;
}

/* Fire a timer's callback by name: one-shots stop themselves first, periodic
 * timers keep running — matching the IDF semantics the production code
 * assumes. */
__attribute__((unused))
static void etimer_mock_fire(const char *name)
{
    etimer_mock_timer_t *t = etimer_mock_by_name(name);
    if (t == NULL || t->callback == NULL) {
        return;
    }
    if (!t->periodic) {
        t->running = false;
    }
    t->callback(t->arg);
}

/* ---- The mocked API ---------------------------------------------------------------- */

__attribute__((unused))
static esp_err_t esp_timer_create(const esp_timer_create_args_t *create_args,
                                  esp_timer_handle_t *out_handle)
{
    if (create_args == NULL || out_handle == NULL ||
        create_args->callback == NULL) {
        return ESP_ERR_INVALID_ARG;
    }
    if (etimer_mock_count >= ETIMER_MOCK_MAX) {
        return ESP_ERR_NO_MEM;
    }
    if (etimer_mock_create_ret != ESP_OK) {
        return etimer_mock_create_ret;
    }
    etimer_mock_timer_t *t = &etimer_mock_timers[etimer_mock_count++];
    memset(t, 0, sizeof(*t));
    t->handle   = (esp_timer_handle_t)(uintptr_t)(0x71de000 + etimer_mock_count);
    t->name     = create_args->name;
    t->callback = create_args->callback;
    t->arg      = create_args->arg;
    *out_handle = t->handle;
    etimer_mock_create_calls++;
    return ESP_OK;
}

__attribute__((unused))
static esp_err_t esp_timer_start_once(esp_timer_handle_t timer,
                                      uint64_t timeout_us)
{
    etimer_mock_timer_t *t = etimer_mock_by_handle(timer);
    if (t == NULL) {
        return ESP_ERR_INVALID_ARG;
    }
    etimer_mock_start_once_calls++;
    etimer_mock_last_period_us = timeout_us;
    if (etimer_mock_start_once_ret != ESP_OK) {
        return etimer_mock_start_once_ret;
    }
    t->running   = true;
    t->periodic  = false;
    t->period_us = timeout_us;
    return ESP_OK;
}

__attribute__((unused))
static esp_err_t esp_timer_start_periodic(esp_timer_handle_t timer,
                                          uint64_t period_us)
{
    etimer_mock_timer_t *t = etimer_mock_by_handle(timer);
    if (t == NULL) {
        return ESP_ERR_INVALID_ARG;
    }
    etimer_mock_start_periodic_calls++;
    etimer_mock_last_period_us = period_us;
    if (etimer_mock_start_periodic_ret != ESP_OK) {
        return etimer_mock_start_periodic_ret;
    }
    t->running   = true;
    t->periodic  = true;
    t->period_us = period_us;
    return ESP_OK;
}

__attribute__((unused))
static esp_err_t esp_timer_stop(esp_timer_handle_t timer)
{
    etimer_mock_timer_t *t = etimer_mock_by_handle(timer);
    if (t == NULL) {
        return ESP_ERR_INVALID_ARG;
    }
    etimer_mock_stop_calls++;
    if (etimer_mock_stop_ret != ESP_OK) {
        return etimer_mock_stop_ret;
    }
    t->running = false;
    return ESP_OK;
}

__attribute__((unused))
static esp_err_t esp_timer_delete(esp_timer_handle_t timer)
{
    etimer_mock_timer_t *t = etimer_mock_by_handle(timer);
    if (t == NULL) {
        return ESP_ERR_INVALID_ARG;
    }
    etimer_mock_delete_calls++;
    t->deleted = true;
    t->running = false;
    return ESP_OK;
}

__attribute__((unused))
static int64_t esp_timer_get_time(void)
{
    return etimer_mock_now_us;
}

#endif /* SPAXEL_TEST_STUB_ESP_TIMER_H */
