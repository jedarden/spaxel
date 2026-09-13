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

#include "freertos/FreeRTOS.h"

typedef void *TaskHandle_t;

static TaskHandle_t wdt_mock_current_task;
static const char  *wdt_mock_current_task_name = "wdt-host-test-task";

const char *pcTaskGetName(TaskHandle_t task_to_query);
TaskHandle_t xTaskGetCurrentTaskHandle(void);

#endif /* SPAXEL_TEST_STUB_FREERTOS_TASK_H */
