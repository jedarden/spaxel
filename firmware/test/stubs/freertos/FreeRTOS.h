/*
 * Host stand-in for the slice of FreeRTOS.h that watchdog.c uses.
 *
 * portNUM_PROCESSORS is the value portmacro.h provides on target (2 — the
 * ESP32-S3 is dual core). watchdog_init() derives its idle_core_mask from it
 * and the contract test asserts that mask against the same expression, so the
 * two can only disagree if the production code stops deriving it.
 */
#ifndef SPAXEL_TEST_STUB_FREERTOS_H
#define SPAXEL_TEST_STUB_FREERTOS_H

#include <stdbool.h>
#include <stdint.h>

#define portNUM_PROCESSORS 2

#endif /* SPAXEL_TEST_STUB_FREERTOS_H */
