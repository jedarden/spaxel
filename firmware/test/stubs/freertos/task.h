/*
 * Host stand-in for the slice of FreeRTOS task.h that watchdog.c uses.
 *
 * The stub models exactly one task. wdt_mock_current_task is that task's
 * identity and the test points it at a non-NULL sentinel before subscribing,
 * so the "which task got subscribed" question has a discriminating answer:
 * the contract requires esp_task_wdt_add(NULL) — "the calling task" — and a
 * regression that passes a captured handle instead is visible as a non-NULL
 * recorded handle rather than as a silently identical host value.
 *
 * Both functions are declared here and defined in the test TU, keeping this
 * directory header-only.
 */
#ifndef SPAXEL_TEST_STUB_FREERTOS_TASK_H
#define SPAXEL_TEST_STUB_FREERTOS_TASK_H

#include <stddef.h>
#include <string.h>

#include "freertos/FreeRTOS.h"

typedef void *TaskHandle_t;

__attribute__((unused))
static TaskHandle_t wdt_mock_current_task;
__attribute__((unused))
static const char  *wdt_mock_current_task_name = "wdt-host-test-task";

const char *pcTaskGetName(TaskHandle_t task_to_query);
TaskHandle_t xTaskGetCurrentTaskHandle(void);

/*
 * ---------------------------------------------------------------------------
 * Task + delay surface for the transport/ntp/led/ble/websocket host units
 * (spaxel-db7aab09).
 *
 * Tasks are never spawned: xTaskCreate()/xTaskCreatePinnedToCore() record the
 * entry point, argument and priority and return pdPASS, and
 * task_mock_run_created() calls a recorded entry point synchronously on the
 * test's own thread — led_blink_task's loop is deterministic under that model.
 * vTaskDelete(NULL) records and returns; the LED task's delete is its last
 * statement, so nothing after it is skipped. vTaskDelay records into a small
 * ring plus running totals (the LED 60 s cap test walks 600 delays, so the
 * ring is only for short sequence assertions).
 *
 * Everything is static: each production TU sees the copy of this header that
 * its own test TU expanded, so recordings are per-TU and nothing collides at
 * link time.
 * ---------------------------------------------------------------------------
 */
typedef void (*TaskFunction_t)(void *);

#define TASK_MOCK_SLOTS     8
#define TASK_MOCK_DELAY_RING 64

typedef struct {
    TaskFunction_t fn;
    void          *arg;
    UBaseType_t    priority;
    TaskHandle_t   handle;
    const char    *name;
} task_mock_slot_t;

__attribute__((unused))
static task_mock_slot_t task_mock_created[TASK_MOCK_SLOTS];
__attribute__((unused))
static int        task_mock_create_calls;
__attribute__((unused))
static BaseType_t task_mock_create_fail = pdFALSE; /* force xTaskCreate to fail */
__attribute__((unused))
static int        task_mock_delete_calls;

__attribute__((unused))
static int        task_mock_delay_calls;
__attribute__((unused))
static TickType_t task_mock_delay_total_ticks;
__attribute__((unused))
static TickType_t task_mock_delay_ring[TASK_MOCK_DELAY_RING];
__attribute__((unused))
static int        task_mock_delay_ring_len;

__attribute__((unused))
static void task_mock_reset(void)
{
    memset(task_mock_created, 0, sizeof(task_mock_created));
    task_mock_create_calls = 0;
    task_mock_create_fail  = pdFALSE;
    task_mock_delete_calls = 0;
    task_mock_delay_calls      = 0;
    task_mock_delay_total_ticks = 0;
    task_mock_delay_ring_len   = 0;
}

__attribute__((unused))
static void vTaskDelay(TickType_t ticks)
{
    task_mock_delay_calls++;
    task_mock_delay_total_ticks += ticks;
    if (task_mock_delay_ring_len < TASK_MOCK_DELAY_RING) {
        task_mock_delay_ring[task_mock_delay_ring_len++] = ticks;
    }
}

__attribute__((unused))
static void vTaskDelete(TaskHandle_t task_to_delete)
{
    (void)task_to_delete; /* NULL means "this task" — there is no task to die. */
    task_mock_delete_calls++;
}

__attribute__((unused))
static BaseType_t xTaskCreate(TaskFunction_t task_fn, const char *name,
                              uint32_t stack_depth, void *parameters,
                              UBaseType_t priority, TaskHandle_t *created_handle)
{
    (void)stack_depth;
    if (task_mock_create_fail) {
        return pdFAIL;
    }
    if (task_mock_create_calls >= TASK_MOCK_SLOTS) {
        return pdFAIL;
    }
    task_mock_slot_t *slot = &task_mock_created[task_mock_create_calls++];
    slot->fn       = task_fn;
    slot->name     = name;
    slot->arg      = parameters;
    slot->priority = priority;
    slot->handle   = (TaskHandle_t)(uintptr_t)(0x7a5b000 + task_mock_create_calls);
    if (created_handle) {
        *created_handle = slot->handle;
    }
    return pdPASS;
}

__attribute__((unused))
static BaseType_t xTaskCreatePinnedToCore(TaskFunction_t task_fn,
                                          const char *name,
                                          uint32_t stack_depth, void *parameters,
                                          UBaseType_t priority,
                                          TaskHandle_t *created_handle,
                                          BaseType_t core_id)
{
    (void)stack_depth;
    (void)core_id;
    /* Same model as xTaskCreate; the core pin is unobservable on a host. */
    return xTaskCreate(task_fn, name, stack_depth, parameters, priority,
                       created_handle);
}

/* Run the n-th recorded task creation synchronously (0-based). */
__attribute__((unused))
static void task_mock_run_created(int index)
{
    if (index < 0 || index >= task_mock_create_calls) {
        return;
    }
    task_mock_created[index].fn(task_mock_created[index].arg);
}

#endif /* SPAXEL_TEST_STUB_FREERTOS_TASK_H */
