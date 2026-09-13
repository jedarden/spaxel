/*
 * Host stand-in for ESP-IDF's esp_task_wdt.h, plus a recording mock of the
 * five entry points firmware/main/watchdog.c can reach (spaxel-61d41649).
 *
 * The Task Watchdog itself cannot exist on a host: it is a timer interrupt
 * plus an idle-hook construct, and QEMU provably cannot reproduce a trip
 * (spaxel-69cdd68c — a control build arming the window with a task that never
 * fed ran 30s past its due reset without resetting). What a host CAN pin is
 * the call pattern that makes a trip impossible on a healthy node, so the
 * mock records every call and its configuration instead of modelling expiry.
 *
 * State and mocks are static: watchdog.c is compiled into the single test TU
 * that includes this header, so nothing escapes it and nothing collides with
 * another test directory's stubs.
 */
#ifndef SPAXEL_TEST_STUB_ESP_TASK_WDT_H
#define SPAXEL_TEST_STUB_ESP_TASK_WDT_H

#include <stdbool.h>
#include <stdint.h>

#include "esp_err.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"

typedef struct {
    uint32_t timeout_ms;
    uint32_t idle_core_mask;
    bool trigger_panic;
} esp_task_wdt_config_t;

/* ---- Recorded calls ------------------------------------------------------ */

static int wdt_mock_init_calls;
static int wdt_mock_reconfigure_calls;
static int wdt_mock_add_calls;
static int wdt_mock_reset_calls;
static int wdt_mock_delete_calls;

static esp_task_wdt_config_t wdt_mock_init_cfg;
static esp_task_wdt_config_t wdt_mock_reconfigure_cfg;

/* Handle passed to esp_task_wdt_add(): the contract requires NULL ("the
 * calling task"), so a regression that subscribes a captured handle shows up
 * here as a non-NULL value. */
static TaskHandle_t wdt_mock_added_handle;

/* ---- Return values the test drives --------------------------------------- */

/* init_ret defaults to ESP_ERR_INVALID_STATE because that is the on-target
 * state when watchdog_init() runs: startup already called esp_task_wdt_init()
 * (CONFIG_ESP_TASK_WDT_INIT) at the 5s Kconfig default, so the production
 * path is init -> INVALID_STATE -> reconfigure. */
static esp_err_t wdt_mock_init_ret        = ESP_ERR_INVALID_STATE;
static esp_err_t wdt_mock_reconfigure_ret = ESP_OK;
static esp_err_t wdt_mock_add_ret         = ESP_OK;
static esp_err_t wdt_mock_reset_ret       = ESP_OK;

/*
 * Clear every recorded call and restore the production-default return values
 * above. Call this at the top of each test — TEST() registration order is
 * link order, so no test may depend on running first.
 */
static void wdt_mock_reset_all(void)
{
    wdt_mock_init_calls        = 0;
    wdt_mock_reconfigure_calls = 0;
    wdt_mock_add_calls         = 0;
    wdt_mock_reset_calls       = 0;
    wdt_mock_delete_calls      = 0;

    wdt_mock_init_cfg.timeout_ms     = 0;
    wdt_mock_init_cfg.idle_core_mask = 0;
    wdt_mock_init_cfg.trigger_panic  = false;

    wdt_mock_reconfigure_cfg.timeout_ms     = 0;
    wdt_mock_reconfigure_cfg.idle_core_mask = 0;
    wdt_mock_reconfigure_cfg.trigger_panic  = false;

    wdt_mock_added_handle = NULL;

    wdt_mock_init_ret        = ESP_ERR_INVALID_STATE;
    wdt_mock_reconfigure_ret = ESP_OK;
    wdt_mock_add_ret         = ESP_OK;
    wdt_mock_reset_ret       = ESP_OK;
}

/* ---- The mocked API ------------------------------------------------------ */

static esp_err_t esp_task_wdt_init(const esp_task_wdt_config_t *config)
{
    wdt_mock_init_cfg = *config;
    wdt_mock_init_calls++;
    return wdt_mock_init_ret;
}

static esp_err_t esp_task_wdt_reconfigure(const esp_task_wdt_config_t *config)
{
    wdt_mock_reconfigure_cfg = *config;
    wdt_mock_reconfigure_calls++;
    return wdt_mock_reconfigure_ret;
}

static esp_err_t esp_task_wdt_add(TaskHandle_t task_handle)
{
    wdt_mock_added_handle = task_handle;
    wdt_mock_add_calls++;
    return wdt_mock_add_ret;
}

static esp_err_t esp_task_wdt_reset(void)
{
    wdt_mock_reset_calls++;
    return wdt_mock_reset_ret;
}

/* The contract never unsubscribes (watchdog.c has no delete call site), so
 * nothing in the suite invokes this; it is recorded so that a future call site
 * is counted rather than invisible. `unused` keeps the unexercised mock from
 * warning while it waits. */
__attribute__((unused))
static esp_err_t esp_task_wdt_delete(TaskHandle_t task_handle)
{
    (void)task_handle;
    wdt_mock_delete_calls++;
    return ESP_OK;
}

#endif /* SPAXEL_TEST_STUB_ESP_TASK_WDT_H */
