/*
 * Host stand-in for the slice of FreeRTOS event_groups.h the production TUs
 * use (spaxel-db7aab09).
 *
 * One shared bit field per TU, so SetBits → WaitBits behaves like the real
 * thing within the TU: the NTP sync callback sets NTP_SYNC_BIT and a later
 * ntp_wait_sync() really observes it. Waits on unset bits return 0 (timeout
 * model) and record the requested mask and tick count.
 */
#ifndef SPAXEL_TEST_STUB_FREERTOS_EVENT_GROUPS_H
#define SPAXEL_TEST_STUB_FREERTOS_EVENT_GROUPS_H

#include <stdbool.h>
#include <stdint.h>

#include "freertos/FreeRTOS.h"

typedef void *EventGroupHandle_t;
typedef uint32_t EventBits_t;

__attribute__((unused))
static EventGroupHandle_t evgrp_mock_handle;
__attribute__((unused))
static int        evgrp_mock_create_calls;
__attribute__((unused))
static bool       evgrp_mock_create_fail = false;
__attribute__((unused))
static EventBits_t evgrp_mock_bits;
__attribute__((unused))
static int        evgrp_mock_set_calls;
__attribute__((unused))
static int        evgrp_mock_clear_calls;
__attribute__((unused))
static int        evgrp_mock_wait_calls;
__attribute__((unused))
static EventBits_t evgrp_mock_last_wait_bits;
__attribute__((unused))
static TickType_t  evgrp_mock_last_wait_ticks;

__attribute__((unused))
static void evgrp_mock_reset(void)
{
    evgrp_mock_handle          = NULL;
    evgrp_mock_create_calls    = 0;
    evgrp_mock_create_fail     = false;
    evgrp_mock_bits            = 0;
    evgrp_mock_set_calls       = 0;
    evgrp_mock_clear_calls     = 0;
    evgrp_mock_wait_calls      = 0;
    evgrp_mock_last_wait_bits  = 0;
    evgrp_mock_last_wait_ticks = 0;
}

__attribute__((unused))
static EventGroupHandle_t xEventGroupCreate(void)
{
    if (evgrp_mock_create_fail) {
        return NULL;
    }
    evgrp_mock_handle = (EventGroupHandle_t)(uintptr_t)(0x3a7e000 +
                                                        ++evgrp_mock_create_calls);
    return evgrp_mock_handle;
}

__attribute__((unused))
static EventBits_t xEventGroupSetBits(EventGroupHandle_t group,
                                      EventBits_t bits_to_set)
{
    (void)group;
    evgrp_mock_set_calls++;
    evgrp_mock_bits |= bits_to_set;
    return evgrp_mock_bits;
}

__attribute__((unused))
static EventBits_t xEventGroupClearBits(EventGroupHandle_t group,
                                        EventBits_t bits_to_clear)
{
    (void)group;
    evgrp_mock_clear_calls++;
    evgrp_mock_bits &= ~bits_to_clear;
    return evgrp_mock_bits;
}

__attribute__((unused))
static EventBits_t xEventGroupWaitBits(EventGroupHandle_t group,
                                       EventBits_t bits_to_wait_for,
                                       bool clear_on_exit, bool wait_for_all_bits,
                                       TickType_t ticks_to_wait)
{
    (void)group;
    (void)wait_for_all_bits; /* single-bit waits only in this suite */
    evgrp_mock_wait_calls++;
    evgrp_mock_last_wait_bits  = bits_to_wait_for;
    evgrp_mock_last_wait_ticks = ticks_to_wait;

    if (evgrp_mock_bits & bits_to_wait_for) {
        EventBits_t snapshot = evgrp_mock_bits;
        if (clear_on_exit) {
            evgrp_mock_bits &= ~bits_to_wait_for;
        }
        return snapshot;
    }
    return 0; /* timeout model: nothing blocked, nothing arrived */
}

#endif /* SPAXEL_TEST_STUB_FREERTOS_EVENT_GROUPS_H */
