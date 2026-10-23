/*
 * Host stand-in for the slice of FreeRTOS semphr.h the production TUs use
 * (spaxel-db7aab09).
 *
 * The host suite is single-threaded: every take/give succeeds (mutexes are
 * contention-free by construction) and NULL handles are tolerated as no-ops —
 * that is what lets a test exercise, say, websocket_is_connected() without
 * having run websocket_init() first. Creation records a handle (or NULL when
 * the test injects failure).
 */
#ifndef SPAXEL_TEST_STUB_FREERTOS_SEMPHR_H
#define SPAXEL_TEST_STUB_FREERTOS_SEMPHR_H

#include <stdbool.h>
#include <stdint.h>

#include "freertos/FreeRTOS.h"

typedef void *SemaphoreHandle_t;

__attribute__((unused))
static int  sem_mock_create_calls;
__attribute__((unused))
static bool sem_mock_create_fail = false;

__attribute__((unused))
static void sem_mock_reset(void)
{
    sem_mock_create_calls = 0;
    sem_mock_create_fail  = false;
}

static SemaphoreHandle_t sem_mock_create(void)
{
    if (sem_mock_create_fail) {
        return NULL;
    }
    return (SemaphoreHandle_t)(uintptr_t)(0x5e3b000 + ++sem_mock_create_calls);
}

__attribute__((unused))
static SemaphoreHandle_t xSemaphoreCreateMutex(void)
{
    return sem_mock_create();
}

__attribute__((unused))
static SemaphoreHandle_t xSemaphoreCreateRecursiveMutex(void)
{
    return sem_mock_create();
}

__attribute__((unused))
static BaseType_t xSemaphoreTake(SemaphoreHandle_t handle, TickType_t ticks)
{
    (void)handle;
    (void)ticks;
    return pdTRUE;
}

__attribute__((unused))
static BaseType_t xSemaphoreGive(SemaphoreHandle_t handle)
{
    (void)handle;
    return pdTRUE;
}

__attribute__((unused))
static BaseType_t xSemaphoreTakeRecursive(SemaphoreHandle_t handle,
                                          TickType_t ticks)
{
    (void)handle;
    (void)ticks;
    return pdTRUE;
}

__attribute__((unused))
static BaseType_t xSemaphoreGiveRecursive(SemaphoreHandle_t handle)
{
    (void)handle;
    return pdTRUE;
}

#endif /* SPAXEL_TEST_STUB_FREERTOS_SEMPHR_H */
