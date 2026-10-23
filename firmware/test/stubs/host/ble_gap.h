/*
 * Host stand-in for NimBLE's host/ble_gap.h (spaxel-db7aab09).
 *
 * The event constants, descriptor structs, and the two discovery calls
 * firmware/main/ble.c uses. The radio is fake but the wiring is real:
 *
 *   - ble_gap_disc() records the own-address type, duration, scan params,
 *     and — crucially — the event callback that was registered, then
 *     succeeds or fails per the test's knob;
 *   - gap_mock_inject_disc() feeds a discovery event back through that
 *     registered callback, so a test proves the production handler was the
 *     one wired in, not just that the call returned.
 *
 * All state is static-in-header (esp_task_wdt.h precedent): per-TU private
 * copies, nothing exported at link time.
 */
#ifndef SPAXEL_TEST_STUB_HOST_BLE_GAP_H
#define SPAXEL_TEST_STUB_HOST_BLE_GAP_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <string.h>

#include "host/ble_hs.h"

/* GAP event types (NimBLE's real numbering). ble.c handles DISC,
 * DISC_COMPLETE, and ADV_COMPLETE; the rest exist so the default case of the
 * production switch is exercised honestly. */
#define BLE_GAP_EVENT_CONNECT         0
#define BLE_GAP_EVENT_DISCONNECT      1
#define BLE_GAP_EVENT_DISC            2
#define BLE_GAP_EVENT_DISC_COMPLETE   3
#define BLE_GAP_EVENT_ADV_COMPLETE    4
#define BLE_GAP_EVENT_TERM_FAILURE    5

/* Advertising report shapes: connectable undirected and non-connectable
 * (scan response style) cover every DISC the tests inject. */
#define BLE_HCI_ADV_TYPE_ADV_IND        0x00
#define BLE_HCI_ADV_TYPE_ADV_NONCONN_IND 0x03

/* Forward-declared before the callback typedef: a tag first named inside a
 * prototype's parameter list gets prototype scope, which would make the
 * callback type incompatible with the real struct below. */
struct ble_gap_event;
struct ble_gap_disc_desc;

typedef int ble_gap_event_fn(struct ble_gap_event *event, void *arg);

struct ble_gap_disc_desc {
    ble_addr_t     addr;
    int8_t         rssi;
    uint8_t        event_type;
    uint8_t        length_data;
    const uint8_t *data;
};

struct ble_gap_event {
    int type;
    struct ble_gap_disc_desc disc;
};

struct ble_gap_disc_params {
    uint16_t itvl;
    uint16_t window;
    uint16_t filter_policy;
    uint8_t  limited;
    uint8_t  passive;
    uint8_t  filter_duplicates;
};

/* ---- Recording fakes -------------------------------------------------------- */

#define GAP_MOCK_MAX 8

typedef struct {
    ble_addr_t addr;
    int8_t     rssi;
    uint8_t    length_data;
    const uint8_t *data;
} gap_mock_disc_evt_t;

__attribute__((unused))
static struct {
    int calls;
    uint8_t own_addr_type[GAP_MOCK_MAX];
    int32_t duration_ms[GAP_MOCK_MAX];
    struct ble_gap_disc_params params[GAP_MOCK_MAX];
    ble_gap_event_fn *cb[GAP_MOCK_MAX];
    void *cb_arg[GAP_MOCK_MAX];
} gap_mock_disc;

__attribute__((unused))
static int gap_mock_disc_ret;   /* what ble_gap_disc returns; 0 = started */
__attribute__((unused))
static int gap_mock_cancel_calls;
__attribute__((unused))
static int gap_mock_cancel_ret; /* what ble_gap_disc_cancel returns */

/* Events routed through the registered callback by gap_mock_inject_disc(). */
__attribute__((unused))
static gap_mock_disc_evt_t gap_mock_injected[GAP_MOCK_MAX];
__attribute__((unused))
static int gap_mock_injected_len;

__attribute__((unused))
static void gap_mock_reset(void)
{
    memset(&gap_mock_disc, 0, sizeof(gap_mock_disc));
    memset(gap_mock_injected, 0, sizeof(gap_mock_injected));
    gap_mock_injected_len = 0;
    gap_mock_disc_ret     = 0;
    gap_mock_cancel_calls = 0;
    gap_mock_cancel_ret   = 0;
}

__attribute__((unused))
static int ble_gap_disc(uint8_t own_addr_type, int32_t duration_ms,
                        const struct ble_gap_disc_params *params,
                        ble_gap_event_fn *cb, void *cb_arg)
{
    if (gap_mock_disc_ret != 0) {
        return gap_mock_disc_ret;
    }
    if (gap_mock_disc.calls < GAP_MOCK_MAX) {
        int i = gap_mock_disc.calls;
        gap_mock_disc.own_addr_type[i] = own_addr_type;
        gap_mock_disc.duration_ms[i]   = duration_ms;
        if (params) {
            gap_mock_disc.params[i] = *params;
        }
        gap_mock_disc.cb[i]     = cb;
        gap_mock_disc.cb_arg[i] = cb_arg;
    }
    gap_mock_disc.calls++;
    return 0;
}

__attribute__((unused))
static int ble_gap_disc_cancel(void)
{
    gap_mock_cancel_calls++;
    return gap_mock_cancel_ret;
}

/*
 * Push a DISC event through the most recently registered discovery
 * callback — the production ble_gap_event, when a scan is (or was) active.
 * Returns the callback's return value, or -1 when nothing was ever
 * registered (i.e. the scan never started).
 */
__attribute__((unused))
static int gap_mock_inject_disc(const ble_addr_t *addr, int8_t rssi,
                                uint8_t event_type, const uint8_t *adv,
                                uint8_t adv_len)
{
    if (gap_mock_disc.calls == 0 || !gap_mock_disc.cb[gap_mock_disc.calls - 1]) {
        return -1;
    }

    struct ble_gap_event ev;
    memset(&ev, 0, sizeof(ev));
    ev.type             = BLE_GAP_EVENT_DISC;
    ev.disc.addr        = *addr;
    ev.disc.rssi        = rssi;
    ev.disc.event_type  = event_type;
    ev.disc.length_data = adv_len;
    ev.disc.data        = adv;

    if (gap_mock_injected_len < GAP_MOCK_MAX) {
        gap_mock_injected[gap_mock_injected_len].addr        = *addr;
        gap_mock_injected[gap_mock_injected_len].rssi        = rssi;
        gap_mock_injected[gap_mock_injected_len].length_data = adv_len;
        gap_mock_injected[gap_mock_injected_len].data        = adv;
        gap_mock_injected_len++;
    }

    return gap_mock_disc.cb[gap_mock_disc.calls - 1](&ev,
        gap_mock_disc.cb_arg[gap_mock_disc.calls - 1]);
}

#endif /* SPAXEL_TEST_STUB_HOST_BLE_GAP_H */
