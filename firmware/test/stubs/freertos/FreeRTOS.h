/*
 * Host stand-in for the slice of FreeRTOS.h that the production TUs use.
 *
 * portNUM_PROCESSORS is the value portmacro.h provides on target (2 — the
 * ESP32-S3 is dual core). watchdog_init() derives its idle_core_mask from it
 * and the contract test asserts that mask against the same expression, so the
 * two can only disagree if the production code stops deriving it.
 *
 * The tick/bool surface below was added for the transport/ntp/led/ble/
 * websocket host units (spaxel-db7aab09). pdMS_TO_TICKS maps milliseconds to
 * ticks 1:1 — the recorded vTaskDelay values read directly as milliseconds,
 * which is what the LED blink-timing assertions are written against.
 */
#ifndef SPAXEL_TEST_STUB_FREERTOS_H
#define SPAXEL_TEST_STUB_FREERTOS_H

#include <stdbool.h>
#include <stdint.h>

#define portNUM_PROCESSORS 2

typedef uint32_t TickType_t;
typedef int32_t  BaseType_t;
typedef uint32_t UBaseType_t;

#define pdTRUE  ((BaseType_t)1)
#define pdFALSE ((BaseType_t)0)
#define pdPASS  pdTRUE
#define pdFAIL  pdFALSE

#define pdMS_TO_TICKS(ms) ((TickType_t)(ms))
#define portMAX_DELAY     ((TickType_t)0xffffffffUL)

/* Event-bit helpers (esp_bit_helpers.h on target). BIT0 is all watchdog.c-era
 * TUs ever needed; BIT0..BIT7 cover the spaxel.h SPAXEL_EVENT_* set and the
 * NTP sync bit. */
#define BIT0 (1 << 0)
#define BIT1 (1 << 1)
#define BIT2 (1 << 2)
#define BIT3 (1 << 3)
#define BIT4 (1 << 4)
#define BIT5 (1 << 5)
#define BIT6 (1 << 6)
#define BIT7 (1 << 7)

#endif /* SPAXEL_TEST_STUB_FREERTOS_H */
