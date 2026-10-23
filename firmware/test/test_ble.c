/*
 * ============================================================================
 *  BLE scan cache — host contract test (spaxel-db7aab09, item 4 of 5)
 * ============================================================================
 *
 *  WHY THIS TEST EXISTS
 *  --------------------
 *  firmware/main/ble.c is the node's ambient-Bluetooth ear: it keeps a 60
 *  entry cache of devices heard during passive scan and ships it to the
 *  mothership as JSON. Two halves are host-testable and both have bitten
 *  before:
 *
 *    - the ADV TLV walk. Advertising payloads are length-prefixed fields;
 *      a malformed field (zero length, or one claiming more bytes than the
 *      buffer holds) must stop the walk, not read past it, and the 31/32
 *      byte caps on name/manufacturer data must hold so the fixed-size
 *      ble_device_t can never overflow;
 *    - the cache semantics. Re-seeing a device updates rssi/addr_type and
 *      preserves its name, the cache refuses entry 61, and the JSON render
 *      (addr formatting, "public"/"random", mfr_data_hex, optional keys)
 *      is the mothership's ingestion contract.
 *
 *  The radio itself cannot exist on a host, so ble_start_scan/stop_scan are
 *  pinned at the seam the code already has: the parameters handed to
 *  ble_gap_disc (interval 0x50 / window 0x30 / duplicate filtering), the
 *  callback registered, the idempotence guards, and ble_stop_scan's
 *  tolerance of BLE_HS_EALREADY. Injection goes back through the
 *  registered callback, proving the production handler — not a copy — was
 *  wired in.
 *
 *  HOW IT COMPILES
 *  ---------------
 *  The production TU is included directly (#include "../main/ble.c",
 *  watchdog precedent). ble.c's statics (device cache, s_scanning) become
 *  members of this translation unit and are reset between tests.
 *
 *  The one wrinkle: ble_scan_task calls websocket_is_connected() and
 *  websocket_send_ble(). The real websocket.c lands in test_websocket.o
 *  (item 5), but this unit must link green on its own — and it may not
 *  define those symbols itself or the link would clash once item 5 lands.
 *  So a rename macro active only while ble.c is included re-points the two
 *  call sites onto ble_test_* fakes with unique symbol names; the scan
 *  loop itself never runs on the host (it never returns), so the fakes
 *  exist purely to satisfy the link. Nothing here is reachable from the
 *  ESP-IDF build: no CMake file references firmware/test.
 *  ============================================================================
 */
#include "test_runner.h"

#include <stdbool.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>

#include "stubs/cJSON.h"
#include "stubs/esp_err.h"
#include "stubs/freertos/FreeRTOS.h"
#include "stubs/freertos/semphr.h"
#include "stubs/freertos/task.h"
#include "stubs/host/ble_gap.h"
#include "stubs/host/ble_hs.h"

/* ---- The code under test ---------------------------------------------------- */

/*
 * Rename ble.c's two websocket references onto uniquely-named host fakes for
 * the duration of the include (see the header comment for why).
 */
bool ble_test_ws_is_connected(void);
esp_err_t ble_test_ws_send_ble(const char *json);

static bool ble_test_ws_connected;
static int  ble_test_ws_send_ble_calls;
static char *ble_test_ws_send_ble_last; /* malloc'd copy of the payload */

/* strdup() hides under -std=c11; same memcpy copy the cJSON shim uses. */
static char *ble_test_strdup(const char *s)
{
    size_t n  = strlen(s) + 1;
    char *out = (char *)malloc(n);
    if (out) {
        memcpy(out, s, n);
    }
    return out;
}

bool ble_test_ws_is_connected(void)
{
    return ble_test_ws_connected;
}

esp_err_t ble_test_ws_send_ble(const char *json)
{
    ble_test_ws_send_ble_calls++;
    free(ble_test_ws_send_ble_last);
    ble_test_ws_send_ble_last = json ? ble_test_strdup(json) : NULL;
    return ESP_OK;
}

#define websocket_is_connected ble_test_ws_is_connected
#define websocket_send_ble     ble_test_ws_send_ble

#include "../main/ble.c"

#undef websocket_send_ble
#undef websocket_is_connected

/*
 * ble.c's file-scope statics joined this TU with the include above. Each
 * test starts from the same slate — cache empty, a fresh valid mutex, scan
 * stopped, fakes cleared — so tests are order-independent without running
 * ble_init()'s task creation in every one of them.
 */
static void reset_ble_under_test(void)
{
    memset(s_devices, 0, sizeof(s_devices));
    s_device_count  = 0;
    s_scanning      = false;
    s_ble_task      = NULL;
    s_devices_mutex = sem_mock_create(); /* fresh handle, never dereferenced */

    ble_test_ws_connected    = false;
    ble_test_ws_send_ble_calls = 0;
    free(ble_test_ws_send_ble_last);
    ble_test_ws_send_ble_last = NULL;

    gap_mock_reset();
    sem_mock_reset();
    task_mock_reset();
}

/*
 * Feed one discovery event straight to the production handler. ble_gap_event
 * is static inside this TU (it came with the include), so the test can drive
 * it directly for cache-semantics tests; the scan-lifecycle test injects
 * through gap_mock_inject_disc() instead to prove the registration wiring.
 */
static void ble_test_inject(const uint8_t *addr, uint8_t addr_type,
                            int8_t rssi, const uint8_t *adv, uint8_t adv_len)
{
    struct ble_gap_event ev;
    memset(&ev, 0, sizeof(ev));
    ev.type             = BLE_GAP_EVENT_DISC;
    ev.disc.addr.type   = addr_type;
    memcpy(ev.disc.addr.val, addr, 6);
    ev.disc.rssi        = rssi;
    ev.disc.length_data = adv_len;
    ev.disc.data        = adv;

    ASSERT_EQ(ble_gap_event(&ev, NULL), 0);
}

/* ---- Init ---------------------------------------------------------------------- */

TEST(ble_init_creates_mutex_zeroes_host_cfg_and_spawns_scan_task)
{
    reset_ble_under_test();

    ASSERT_EQ(ble_init(), ESP_OK);

    ASSERT_EQ(sem_mock_create_calls, 1);
    ASSERT_TRUE(s_devices_mutex != NULL);

    /* Scan-only: every host callback stays unset. */
    ASSERT_TRUE(ble_hs_cfg.sync_cb == NULL);
    ASSERT_TRUE(ble_hs_cfg.gatts_register_cb == NULL);
    ASSERT_TRUE(ble_hs_cfg.store_status_cb == NULL);
    ASSERT_TRUE(ble_hs_cfg.reset_cb == NULL);

    /* Exactly one scan task, at the documented shape. */
    ASSERT_EQ(task_mock_create_calls, 1);
    ASSERT_TRUE(strcmp(task_mock_created[0].name, "ble_scan") == 0);
    ASSERT_EQ(task_mock_created[0].priority, 5);
    ASSERT_TRUE(task_mock_created[0].arg == NULL);
    ASSERT_TRUE(task_mock_created[0].handle != NULL);
    ASSERT_TRUE(s_ble_task == task_mock_created[0].handle);
}

TEST(ble_init_mutex_failure_reports_no_mem_and_spawns_nothing)
{
    reset_ble_under_test();

    sem_mock_create_fail = true;
    ASSERT_EQ(ble_init(), ESP_ERR_NO_MEM);
    sem_mock_create_fail = false;

    ASSERT_EQ(task_mock_create_calls, 0);
    ASSERT_TRUE(s_ble_task == NULL);
}

/* ---- ADV TLV parsing ---------------------------------------------------------------- */

TEST(ble_disc_adds_device_with_name_and_le_manufacturer_data)
{
    reset_ble_under_test();

    /* Flags + complete local name + manufacturer data (little-endian
     * company id 0x0059, two payload bytes). A TLV length byte counts the
     * type byte too, so the 8-char name is a length-9 field and the
     * 2-id+2-data mfr payload is a length-5 field. */
    static const uint8_t adv[] = {
        0x02, 0x01, 0x06,
        0x09, 0x09, 'S', 'p', 'a', 'x', 'e', 'l', '-', '1',
        0x05, 0xFF, 0x59, 0x00, 0xDE, 0xAD,
    };
    static const uint8_t addr[6] = { 0x01, 0x02, 0x03, 0x04, 0x05, 0x06 };

    ble_test_inject(addr, 0, -52, adv, sizeof(adv));

    ASSERT_EQ(ble_get_device_count(), 1);
    ASSERT_EQ(s_devices[0].rssi, -52);
    ASSERT_EQ(s_devices[0].addr_type, 0);
    ASSERT_TRUE(memcmp(s_devices[0].addr, addr, 6) == 0);
    ASSERT_TRUE(strcmp(s_devices[0].name, "Spaxel-1") == 0);
    ASSERT_EQ(s_devices[0].mfr_id, 0x0059);
    ASSERT_EQ(s_devices[0].mfr_data_len, 2);
    ASSERT_EQ(s_devices[0].mfr_data[0], 0xDE);
    ASSERT_EQ(s_devices[0].mfr_data[1], 0xAD);
}

TEST(ble_disc_resees_device_updates_rssi_and_preserves_name)
{
    reset_ble_under_test();

    static const uint8_t addr[6] = { 0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF };
    static const uint8_t named[] = { 0x05, 0x09, 'P', 'u', 'c', 'k' };
    static const uint8_t bare[]  = { 0x02, 0x01, 0x06 };

    ble_test_inject(addr, 1, -70, named, sizeof(named));
    ASSERT_EQ(ble_get_device_count(), 1);

    /* Second sighting carries no name field — it must not clobber the
     * stored one — and refreshes rssi + addr_type. */
    ble_test_inject(addr, 0, -58, bare, sizeof(bare));

    ASSERT_EQ(ble_get_device_count(), 1);
    ASSERT_TRUE(strcmp(s_devices[0].name, "Puck") == 0);
    ASSERT_EQ(s_devices[0].rssi, -58);
    ASSERT_EQ(s_devices[0].addr_type, 0);
}

TEST(ble_disc_name_longer_than_31_is_truncated)
{
    reset_ble_under_test();

    /* 40-character name in one name field. */
    uint8_t adv[42];
    adv[0] = 41;
    adv[1] = 0x09;
    memset(&adv[2], 'x', 40);

    static const uint8_t addr[6] = { 1, 1, 1, 1, 1, 1 };
    ble_test_inject(addr, 0, -40, adv, sizeof(adv));

    ASSERT_EQ(ble_get_device_count(), 1);
    ASSERT_EQ(strlen(s_devices[0].name), 31); /* fixed 32-byte field */
    ASSERT_TRUE(s_devices[0].name[31] == '\0');
    for (int i = 0; i < 31; i++) {
        ASSERT_EQ(s_devices[0].name[i], 'x');
    }
}

TEST(ble_disc_mfr_data_longer_than_32_is_truncated)
{
    reset_ble_under_test();

    /* Manufacturer field claiming 40 payload bytes (2 id + 38 data). */
    uint8_t adv[45];
    adv[0] = 44;
    adv[1] = 0xFF;
    adv[2] = 0xE1; /* company id 0x00E1 little-endian */
    adv[3] = 0x00;
    memset(&adv[4], 0x5A, 41);

    static const uint8_t addr[6] = { 2, 2, 2, 2, 2, 2 };
    ble_test_inject(addr, 0, -30, adv, sizeof(adv));

    ASSERT_EQ(ble_get_device_count(), 1);
    ASSERT_EQ(s_devices[0].mfr_id, 0x00E1);
    ASSERT_EQ(s_devices[0].mfr_data_len, 32); /* fixed 32-byte field */
    for (int i = 0; i < 32; i++) {
        ASSERT_EQ(s_devices[0].mfr_data[i], 0x5A);
    }
}

/*
 * A zero-length TLV is garbage (real fields are at least the type byte);
 * the walk must stop there, having kept whatever came before.
 */
TEST(ble_disc_zero_length_field_stops_the_tlv_walk)
{
    reset_ble_under_test();

    static const uint8_t adv[] = {
        0x02, 0x01, 0x06,
        0x00,               /* zero-length field: stop */
        0x05, 0x09, 'H', 'i', '!', /* must NOT be reached */
    };
    static const uint8_t addr[6] = { 3, 3, 3, 3, 3, 3 };

    ble_test_inject(addr, 0, -60, adv, sizeof(adv));

    ASSERT_EQ(ble_get_device_count(), 1); /* device itself was added */
    ASSERT_EQ(s_devices[0].name[0], '\0'); /* walk stopped before the name */
    ASSERT_EQ(s_devices[0].mfr_id, 0);
}

/*
 * A field claiming more bytes than the buffer holds must stop the walk
 * instead of reading past it — the classic malformed-advertisement crash.
 */
TEST(ble_disc_overrunning_field_stops_the_tlv_walk)
{
    reset_ble_under_test();

    /* Length byte says 5 bytes follow the type but the buffer ends after
     * two of them: i + field_len (0 + 5 = 5) is >= adv_len (4). */
    static const uint8_t adv[] = { 0x05, 0x09, 'A', 'B' };
    static const uint8_t addr[6] = { 4, 4, 4, 4, 4, 4 };

    ble_test_inject(addr, 0, -45, adv, sizeof(adv));

    ASSERT_EQ(ble_get_device_count(), 1);
    ASSERT_EQ(s_devices[0].name[0], '\0');
    ASSERT_EQ(s_devices[0].mfr_id, 0);
}

TEST(ble_disc_unknown_field_types_are_skipped)
{
    reset_ble_under_test();

    /* Flags and an incomplete service-UUID list: neither is name nor
     * manufacturer data, and neither may disturb the device record. */
    static const uint8_t adv[] = { 0x02, 0x01, 0x06, 0x05, 0x03, 0x0F, 0x18, 0x00, 0x00 };
    static const uint8_t addr[6] = { 5, 5, 5, 5, 5, 5 };

    ble_test_inject(addr, 1, -25, adv, sizeof(adv));

    ASSERT_EQ(ble_get_device_count(), 1);
    ASSERT_EQ(s_devices[0].addr_type, 1);
    ASSERT_EQ(s_devices[0].name[0], '\0');
    ASSERT_EQ(s_devices[0].mfr_id, 0);
}

/* Manufacturer field shorter than the 3 bytes an id requires is skipped
 * whole — no half-read id, no negative length. */
TEST(ble_disc_short_mfr_field_is_skipped)
{
    reset_ble_under_test();

    static const uint8_t adv[] = { 0x02, 0xFF, 0x59 };
    static const uint8_t addr[6] = { 6, 6, 6, 6, 6, 6 };

    ble_test_inject(addr, 0, -35, adv, sizeof(adv));

    ASSERT_EQ(ble_get_device_count(), 1);
    ASSERT_EQ(s_devices[0].mfr_id, 0);
    ASSERT_EQ(s_devices[0].mfr_data_len, 0);
}

/* An id with a zero-byte payload is legal: id recorded, no hex data —
 * which the JSON render must mirror (mfr_id present, mfr_data_hex absent). */
TEST(ble_disc_mfr_id_with_empty_payload_sets_id_only)
{
    reset_ble_under_test();

    static const uint8_t adv[] = { 0x03, 0xFF, 0x59, 0x00 };
    static const uint8_t addr[6] = { 7, 7, 7, 7, 7, 7 };

    ble_test_inject(addr, 0, -35, adv, sizeof(adv));

    ASSERT_EQ(ble_get_device_count(), 1);
    ASSERT_EQ(s_devices[0].mfr_id, 0x0059);
    ASSERT_EQ(s_devices[0].mfr_data_len, 0);

    char *json = ble_get_devices_json();
    ASSERT_TRUE(json != NULL);
    cJSON *root = cJSON_Parse(json);
    ASSERT_TRUE(root != NULL);
    cJSON *dev = root->child;
    ASSERT_TRUE(dev != NULL);
    ASSERT_TRUE(cJSON_GetObjectItem(dev, "mfr_id") != NULL);
    ASSERT_TRUE(cJSON_GetObjectItem(dev, "mfr_data_hex") == NULL);
    cJSON_Delete(root);
    free(json);
}

/* ---- Cache capacity -------------------------------------------------------------------- */

TEST(ble_cache_caps_at_60_devices)
{
    reset_ble_under_test();

    static const uint8_t adv[] = { 0x02, 0x01, 0x06 };
    uint8_t addr[6] = { 0, 0, 0, 0, 0, 0 };

    for (int i = 0; i < 60; i++) {
        addr[0] = (uint8_t)i;
        ble_test_inject(addr, 0, (int8_t)-50, adv, sizeof(adv));
    }
    ASSERT_EQ(ble_get_device_count(), 60);

    /* The 61st distinct device must not displace anything. */
    addr[0] = 60;
    ble_test_inject(addr, 0, -50, adv, sizeof(adv));
    ASSERT_EQ(ble_get_device_count(), 60);

    /* And a re-sighting of an existing entry still updates in place. */
    addr[0] = 59;
    ble_test_inject(addr, 1, -33, adv, sizeof(adv));
    ASSERT_EQ(ble_get_device_count(), 60);
    ASSERT_EQ(s_devices[59].rssi, -33);
    ASSERT_EQ(s_devices[59].addr_type, 1);
}

/* ---- Scan lifecycle ---------------------------------------------------------------------- */

TEST(ble_start_scan_records_params_callback_and_marks_scanning)
{
    reset_ble_under_test();

    ASSERT_EQ(ble_start_scan(), ESP_OK);
    ASSERT_TRUE(s_scanning);

    ASSERT_EQ(gap_mock_disc.calls, 1);
    ASSERT_EQ(gap_mock_disc.own_addr_type[0], 0);
    ASSERT_EQ(gap_mock_disc.duration_ms[0], 0); /* infinite */
    ASSERT_EQ(gap_mock_disc.params[0].itvl, 0x50);
    ASSERT_EQ(gap_mock_disc.params[0].window, 0x30);
    ASSERT_EQ(gap_mock_disc.params[0].filter_duplicates, 1);
    /* The production handler itself was registered — injection in the next
     * test rides the very same wiring. */
    ASSERT_TRUE(gap_mock_disc.cb[0] == ble_gap_event);
}

TEST(ble_start_scan_is_idempotent_while_scanning)
{
    reset_ble_under_test();

    ASSERT_EQ(ble_start_scan(), ESP_OK);
    ASSERT_EQ(ble_start_scan(), ESP_OK);
    ASSERT_EQ(gap_mock_disc.calls, 1); /* second call was a no-op */
}

TEST(ble_start_scan_failure_propagates_and_stays_stopped)
{
    reset_ble_under_test();

    gap_mock_disc_ret = BLE_HS_ENOTSUP;
    ASSERT_EQ(ble_start_scan(), ESP_FAIL);
    ASSERT_FALSE(s_scanning);
    gap_mock_disc_ret = 0;

    /* Once the radio cooperates, the next attempt starts for real. */
    ASSERT_EQ(ble_start_scan(), ESP_OK);
    ASSERT_TRUE(s_scanning);
}

/*
 * End-to-end wiring: an injected advertisement flows through the callback
 * that ble_start_scan registered into the production cache.
 */
TEST(ble_scan_injection_routes_through_the_registered_callback)
{
    reset_ble_under_test();

    ASSERT_EQ(ble_start_scan(), ESP_OK);

    static const uint8_t adv[] = { 0x0B, 0x09, 'H', 'o', 's', 't', '-', 'B', 'l', 'e', '!', 0 };
    const ble_addr_t addr = { .type = 0, .val = { 0x31, 0x41, 0x59, 0x26, 0x53, 0x58 } };

    /* No scan registered -> nothing to route through. */
    ASSERT_EQ(gap_mock_inject_disc(&addr, -44, BLE_HCI_ADV_TYPE_ADV_NONCONN_IND,
                                   adv, sizeof(adv)), 0);

    ASSERT_EQ(ble_get_device_count(), 1);
    ASSERT_TRUE(strcmp(s_devices[0].name, "Host-Ble!") == 0);
    ASSERT_EQ(s_devices[0].rssi, -44);
    /* The event that reached the handler is what the fake recorded. */
    ASSERT_EQ(gap_mock_injected_len, 1);
    ASSERT_TRUE(memcmp(gap_mock_injected[0].addr.val, addr.val, 6) == 0);
}

TEST(ble_disc_complete_event_clears_scanning_flag)
{
    reset_ble_under_test();

    ASSERT_EQ(ble_start_scan(), ESP_OK);
    ASSERT_TRUE(s_scanning);

    struct ble_gap_event ev;
    memset(&ev, 0, sizeof(ev));
    ev.type = BLE_GAP_EVENT_DISC_COMPLETE;
    ASSERT_EQ(ble_gap_event(&ev, NULL), 0);

    ASSERT_FALSE(s_scanning);
}

TEST(ble_adv_complete_event_is_a_noop)
{
    reset_ble_under_test();

    ASSERT_EQ(ble_start_scan(), ESP_OK);

    struct ble_gap_event ev;
    memset(&ev, 0, sizeof(ev));
    ev.type = BLE_GAP_EVENT_ADV_COMPLETE;
    ASSERT_EQ(ble_gap_event(&ev, NULL), 0);

    /* ADV completion says nothing about the scan itself. */
    ASSERT_TRUE(s_scanning);
    ASSERT_EQ(ble_get_device_count(), 0);
}

TEST(ble_unknown_gap_event_leaves_state_untouched)
{
    reset_ble_under_test();

    struct ble_gap_event ev;
    memset(&ev, 0, sizeof(ev));
    ev.type = BLE_GAP_EVENT_TERM_FAILURE;
    ASSERT_EQ(ble_gap_event(&ev, NULL), 0);

    ASSERT_FALSE(s_scanning);
    ASSERT_EQ(ble_get_device_count(), 0);
}

/* ---- Stop scan ------------------------------------------------------------------------------ */

TEST(ble_stop_scan_when_idle_is_ok_without_cancel)
{
    reset_ble_under_test();

    ASSERT_EQ(ble_stop_scan(), ESP_OK);
    ASSERT_EQ(gap_mock_cancel_calls, 0);
}

TEST(ble_stop_scan_cancels_and_clears_scanning)
{
    reset_ble_under_test();

    ASSERT_EQ(ble_start_scan(), ESP_OK);
    ASSERT_EQ(ble_stop_scan(), ESP_OK);

    ASSERT_EQ(gap_mock_cancel_calls, 1);
    ASSERT_FALSE(s_scanning);

    /* A fresh start issues a new discovery, not a reuse of the old one. */
    ASSERT_EQ(ble_start_scan(), ESP_OK);
    ASSERT_EQ(gap_mock_disc.calls, 2);
    ASSERT_TRUE(s_scanning);
}

/* NimBLE reports EALREADY when the scan had already stopped underneath us —
 * ble_stop_scan must treat that as success. */
TEST(ble_stop_scan_tolerates_already_stopped)
{
    reset_ble_under_test();

    ASSERT_EQ(ble_start_scan(), ESP_OK);
    gap_mock_cancel_ret = BLE_HS_EALREADY;

    ASSERT_EQ(ble_stop_scan(), ESP_OK);
    ASSERT_FALSE(s_scanning);
    gap_mock_cancel_ret = 0;
}

TEST(ble_stop_scan_real_failure_fails_and_keeps_scanning)
{
    reset_ble_under_test();

    ASSERT_EQ(ble_start_scan(), ESP_OK);
    gap_mock_cancel_ret = BLE_HS_ENOTSUP;

    ASSERT_EQ(ble_stop_scan(), ESP_FAIL);
    /* A scan the controller could not cancel is still running from the
     * module's point of view — the flag must not lie. */
    ASSERT_TRUE(s_scanning);
    gap_mock_cancel_ret = 0;
}

/* ---- JSON render -------------------------------------------------------------------------------- */

TEST(ble_json_without_devices_is_an_empty_array)
{
    reset_ble_under_test();

    char *json = ble_get_devices_json();
    ASSERT_TRUE(json != NULL);
    ASSERT_TRUE(strcmp(json, "[]") == 0);
    free(json);
}

/*
 * The mothership's ingestion contract, parsed back through the same shim
 * the production code builds with: address formatting, public/random
 * rendering, rssi, and the optional keys (name only when non-empty,
 * mfr_id/mfr_data_hex only when a company id was heard).
 */
TEST(ble_json_round_trips_name_mfr_and_addr_fields)
{
    reset_ble_under_test();

    static const uint8_t adv_a[] = {
        0x07, 0x09, 'P', 'u', 'c', 'k', '-', '1',
        0x05, 0xFF, 0x59, 0x00, 0xDE, 0xAD,
    };
    static const uint8_t addr_a[6] = { 0xA4, 0x5E, 0x60, 0x11, 0x22, 0x33 };
    ble_test_inject(addr_a, 0, -52, adv_a, sizeof(adv_a));

    static const uint8_t adv_b[] = { 0x02, 0x01, 0x06 };
    static const uint8_t addr_b[6] = { 0xF8, 0xFF, 0xC0, 0x00, 0x7A, 0x01 };
    ble_test_inject(addr_b, 1, -80, adv_b, sizeof(adv_b));

    char *json = ble_get_devices_json();
    ASSERT_TRUE(json != NULL);

    cJSON *root = cJSON_Parse(json);
    ASSERT_TRUE(root != NULL);
    ASSERT_TRUE(cJSON_IsArray(root));

    /* Device A — everything present. */
    cJSON *a = root->child;
    ASSERT_TRUE(a != NULL);
    cJSON *item;

    item = cJSON_GetObjectItem(a, "addr");
    ASSERT_TRUE(cJSON_IsString(item));
    ASSERT_TRUE(strcmp(item->valuestring, "A4:5E:60:11:22:33") == 0);

    item = cJSON_GetObjectItem(a, "addr_type");
    ASSERT_TRUE(cJSON_IsString(item));
    ASSERT_TRUE(strcmp(item->valuestring, "public") == 0);

    item = cJSON_GetObjectItem(a, "rssi_dbm");
    ASSERT_TRUE(cJSON_IsNumber(item));
    ASSERT_EQ(item->valueint, -52);

    item = cJSON_GetObjectItem(a, "name");
    ASSERT_TRUE(cJSON_IsString(item));
    ASSERT_TRUE(strcmp(item->valuestring, "Puck-1") == 0);

    item = cJSON_GetObjectItem(a, "mfr_id");
    ASSERT_TRUE(cJSON_IsNumber(item));
    ASSERT_EQ(item->valueint, 0x0059);

    item = cJSON_GetObjectItem(a, "mfr_data_hex");
    ASSERT_TRUE(cJSON_IsString(item));
    ASSERT_TRUE(strcmp(item->valuestring, "DEAD") == 0);

    /* Device B — bare sighting: name and manufacturer keys absent. */
    cJSON *b = a->next;
    ASSERT_TRUE(b != NULL);
    ASSERT_TRUE(b->next == NULL); /* exactly two devices */

    item = cJSON_GetObjectItem(b, "addr");
    ASSERT_TRUE(cJSON_IsString(item));
    ASSERT_TRUE(strcmp(item->valuestring, "F8:FF:C0:00:7A:01") == 0);

    item = cJSON_GetObjectItem(b, "addr_type");
    ASSERT_TRUE(cJSON_IsString(item));
    ASSERT_TRUE(strcmp(item->valuestring, "random") == 0);

    item = cJSON_GetObjectItem(b, "rssi_dbm");
    ASSERT_TRUE(cJSON_IsNumber(item));
    ASSERT_EQ(item->valueint, -80);

    ASSERT_TRUE(cJSON_GetObjectItem(b, "name") == NULL);
    ASSERT_TRUE(cJSON_GetObjectItem(b, "mfr_id") == NULL);
    ASSERT_TRUE(cJSON_GetObjectItem(b, "mfr_data_hex") == NULL);

    cJSON_Delete(root);
    free(json);
}

/* ---- Cache clearing ------------------------------------------------------------------------------ */

TEST(ble_clear_devices_empties_the_cache)
{
    reset_ble_under_test();

    static const uint8_t adv[] = { 0x05, 0x09, 'G', 'o', 'n', 'e' };
    static const uint8_t addr[6] = { 9, 9, 9, 9, 9, 9 };
    ble_test_inject(addr, 0, -50, adv, sizeof(adv));
    ASSERT_EQ(ble_get_device_count(), 1);

    ble_clear_devices();

    ASSERT_EQ(ble_get_device_count(), 0);
    char *json = ble_get_devices_json();
    ASSERT_TRUE(json != NULL);
    ASSERT_TRUE(strcmp(json, "[]") == 0);
    free(json);
}
