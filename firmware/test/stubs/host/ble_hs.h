/*
 * Host stand-in for NimBLE's host/ble_hs.h (spaxel-db7aab09).
 *
 * Only the slice ble.c actually touches: the address type it renders into
 * the device JSON ("public"/"random"), the host-controller error codes it
 * compares against, and the global ble_hs_cfg struct it zeroes in ble_init().
 *
 * ble_hs_cfg follows the static-in-header pattern (stubs/esp_task_wdt.h
 * precedent): each TU that expands this header gets its own private
 * zero-initialized copy, so nothing is exported at link time and there is no
 * clash with any other definition.
 */
#ifndef SPAXEL_TEST_STUB_HOST_BLE_HS_H
#define SPAXEL_TEST_STUB_HOST_BLE_HS_H

#include <stdint.h>

/* BLE address. type 0 = public, 1 = random — the only distinction ble.c
 * surfaces (ble_get_devices_json renders it as a string). */
typedef struct {
    uint8_t type;
    uint8_t val[6];
} ble_addr_t;

/* Host-controller status codes (NimBLE's real numeric values). Production
 * code names only BLE_HS_EALREADY — ble_stop_scan() treats it as "the scan
 * had already stopped", which is success for that caller. */
#define BLE_HS_EAGAIN    1
#define BLE_HS_EALREADY  2
#define BLE_HS_EINVAL    3
#define BLE_HS_ENOENT    5
#define BLE_HS_ENOMEM    6
#define BLE_HS_ENOTCONN  7
#define BLE_HS_ENOTSUP   8

struct ble_hs_cfg {
    void (*sync_cb)(void);
    void (*gatts_register_cb)(void *ctxt);
    int  (*store_status_cb)(void *event);
    void (*reset_cb)(int reason);
};

/* NimBLE exposes one global config struct. On the host it is zero-init and
 * ble_init() stores NULL into all four callbacks; test_ble.c asserts that. */
__attribute__((unused))
static struct ble_hs_cfg ble_hs_cfg;

#endif /* SPAXEL_TEST_STUB_HOST_BLE_HS_H */
