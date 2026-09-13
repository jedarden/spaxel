/*
 * Host stand-in for FreeRTOS.h.
 *
 * Pulled in only because firmware/main/spaxel.h includes it on ESP-IDF. The
 * only things spaxel.h's own body needs from it are the BIT* macros used to
 * build the SPAXEL_EVENT_* bitmasks and size_t, which the real header provides
 * transitively; nothing here runs a scheduler.
 */
#pragma once

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#define BIT0 (1 << 0)
#define BIT1 (1 << 1)
#define BIT2 (1 << 2)
#define BIT3 (1 << 3)
#define BIT4 (1 << 4)
#define BIT5 (1 << 5)
#define BIT6 (1 << 6)
#define BIT7 (1 << 7)
