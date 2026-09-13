/*
 * Host stand-in for FreeRTOS event_groups.h.
 *
 * Only firmware/main/spaxel.h's `EventGroupHandle_t events;` struct member
 * needs this file; the type is declared but never used at runtime on the host.
 */
#pragma once

#include <stdint.h>

typedef void *EventGroupHandle_t;
