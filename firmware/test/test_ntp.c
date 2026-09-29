/*
 * ============================================================================
 *  NTP sync lifecycle — host contract test (spaxel-db7aab09, item 2 of 5)
 * ============================================================================
 *
 *  WHY THIS TEST EXISTS
 *  --------------------
 *  firmware/main/ntp.c owns time-of-day for every node; health reports and
 *  mothership-side correlation all lean on it being synced. The dangerous
 *  part of the module is not the SNTP client (that needs a network stack and
 *  cannot exist on a host) but the CALL ORDER around it: on target, calling
 *  esp_sntp_setoperatingmode() while the client is already running is a hard
 *  ESP-IDF assertion — the whole node aborts, not a soft error. And
 *  ntp_start_sync() is an ordinary, repeatable event: initial WiFi connect,
 *  WIFI_LOST recovery, and every runtime server change pushed from the
 *  mothership. So the load-bearing contract pinned here:
 *
 *    - every reconfigure path — ntp_start_sync() AND the periodic resync
 *      callback — issues esp_sntp_stop() BEFORE setoperatingmode
 *      (asserted through the stub's sequence log, not call counts);
 *    - the configured server is stored, handed to setservername(0, ...), and
 *      a NULL argument falls back to pool.ntp.org;
 *    - a server name longer than the 64-byte store is truncated and still
 *      NUL-terminated (what reaches SNTP is exactly what was stored);
 *    - the time-sync callback flips ntp_is_synced()/ntp_status_str() and
 *      raises the event bit a later ntp_wait_sync() really observes; a NULL
 *      timeval is ignored, not treated as a sync;
 *    - the periodic resync timer is created once under the name "ntp_resync",
 *      armed at NTP_RESYNC_INTERVAL_US, refuses a double start, cleans up
 *      after a failed arm (deleted + NULL, so a later start can retry), and
 *      firing it re-syncs against the STORED server;
 *    - ntp_stop() stops SNTP, tears the timer down, and clears synced;
 *    - ntp_wait_sync() before ntp_init() is a clean false, not a crash;
 *    - ntp_init() creates the event group exactly once and reports
 *      ESP_ERR_NO_MEM when creation fails.
 *
 *  HOW IT COMPILES
 *  ---------------
 *  The production TU is included directly:
 *
 *      #include "../main/ntp.c"
 *
 *  (watchdog/safe_mode precedent), with ntp.c's IDF includes — esp_sntp.h,
 *  esp_timer.h, freertos/event_groups.h — resolving to the recording fakes in
 *  stubs/ via the Makefile's target-specific include override for this
 *  object. esp_err_to_name resolves across the link from test_watchdog.o, the
 *  suite's single definition. Nothing here is reachable from the ESP-IDF
 *  build: no CMake file references firmware/test.
 *  ============================================================================
 */
#include "test_runner.h"

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <string.h>
#include <sys/time.h>

#include "stubs/esp_err.h"
#include "stubs/esp_sntp.h"
#include "stubs/esp_timer.h"
#include "stubs/freertos/FreeRTOS.h"
#include "stubs/freertos/event_groups.h"
#include "stubs/freertos/task.h"

/* ---- The code under test ------------------------------------------------------ */

#include "../main/ntp.c"

/*
 * ntp.c keeps its state in file-scope statics, which became part of this
 * translation unit with the include above. Restored directly between tests so
 * every test is order-independent.
 */
static void reset_ntp_under_test(void)
{
    s_ntp_events   = NULL;
    s_resync_timer = NULL;
    s_is_synced    = false;
    strncpy(s_ntp_server, "pool.ntp.org", sizeof(s_ntp_server) - 1);
    s_ntp_server[sizeof(s_ntp_server) - 1] = '\0';

    sntp_mock_reset_all();
    evgrp_mock_reset();
    etimer_mock_reset();
}

/* Sequence helper: assert the exact op order ntp.c emitted since reset. */
static void assert_seq(const int *want, int want_len)
{
    ASSERT_EQ(sntp_mock_seq_len, want_len);
    for (int i = 0; i < want_len && i < sntp_mock_seq_len; i++) {
        ASSERT_EQ(sntp_mock_seq[i], want[i]);
    }
}

/* ---- Init ------------------------------------------------------------------------ */

TEST(ntp_init_creates_event_group_once)
{
    reset_ntp_under_test();

    ASSERT_EQ(ntp_init(), ESP_OK);
    ASSERT_EQ(evgrp_mock_create_calls, 1);
    ASSERT_TRUE(s_ntp_events != NULL);

    /* Second init is a no-op on the event group. */
    ASSERT_EQ(ntp_init(), ESP_OK);
    ASSERT_EQ(evgrp_mock_create_calls, 1);
}

TEST(ntp_init_reports_no_mem_when_event_group_creation_fails)
{
    reset_ntp_under_test();
    evgrp_mock_create_fail = true;

    ASSERT_EQ(ntp_init(), ESP_ERR_NO_MEM);
    ASSERT_TRUE(s_ntp_events == NULL);
}

/* ---- start_sync: the stop-before-reconfigure contract ------------------------------ */

TEST(ntp_start_sync_configures_poll_mode_server_and_callback)
{
    reset_ntp_under_test();
    ASSERT_EQ(ntp_init(), ESP_OK);

    ASSERT_EQ(ntp_start_sync("time.example.com"), ESP_OK);

    /* The reconfigure sequence, in order: stop (defensive), configure,
     * register, start. */
    const int want[] = {SNTP_MOCK_STOP, SNTP_MOCK_SETMODE,
                        SNTP_MOCK_SETSERVER, SNTP_MOCK_SETCB, SNTP_MOCK_INIT};
    assert_seq(want, 5);

    ASSERT_EQ(sntp_mock_opmode, SNTP_OPMODE_POLL);
    ASSERT_EQ(sntp_mock_server_idx, 0);
    ASSERT_TRUE(strcmp(sntp_mock_server_name, "time.example.com") == 0);
    ASSERT_TRUE(sntp_mock_time_cb != NULL);
    ASSERT_TRUE(sntp_mock_running);
    /* A fresh sync starts unsynced, with the event bit cleared. */
    ASSERT_FALSE(ntp_is_synced());
    ASSERT_EQ(evgrp_mock_bits & NTP_SYNC_BIT, 0);
}

/*
 * The hard-abort contract: ntp_start_sync() is called repeatedly per boot
 * (WiFi connect, WIFI_LOST recovery, mothership-pushed server changes), and
 * on target setoperatingmode() while the client runs ASSERTS the node. The
 * second call must stop the client before touching the mode again — exactly
 * the ordering the sequence log records.
 */
TEST(ntp_start_sync_stops_running_client_before_reconfiguring)
{
    reset_ntp_under_test();
    ASSERT_EQ(ntp_init(), ESP_OK);

    ASSERT_EQ(ntp_start_sync("pool.ntp.org"), ESP_OK);
    ASSERT_TRUE(sntp_mock_running);

    sntp_mock_seq_len = 0; /* keep state, watch only the second reconfigure */
    ASSERT_EQ(ntp_start_sync("time.example.com"), ESP_OK);

    const int want[] = {SNTP_MOCK_STOP, SNTP_MOCK_SETMODE,
                        SNTP_MOCK_SETSERVER, SNTP_MOCK_SETCB, SNTP_MOCK_INIT};
    assert_seq(want, 5);
    /* And the new server is the one that got configured. */
    ASSERT_TRUE(strcmp(sntp_mock_server_name, "time.example.com") == 0);
}

TEST(ntp_start_sync_null_server_falls_back_to_pool)
{
    reset_ntp_under_test();
    ASSERT_EQ(ntp_init(), ESP_OK);

    ASSERT_EQ(ntp_start_sync(NULL), ESP_OK);

    ASSERT_TRUE(strcmp(sntp_mock_server_name, "pool.ntp.org") == 0);
    ASSERT_TRUE(strcmp(s_ntp_server, "pool.ntp.org") == 0);
}

/*
 * The store is 64 bytes and gets a strncpy with explicit last-byte NUL; an
 * over-long name must arrive at SNTP truncated but terminated — never as an
 * unterminated buffer read past the store.
 */
TEST(ntp_start_sync_truncates_overlong_server_and_stays_terminated)
{
    reset_ntp_under_test();
    ASSERT_EQ(ntp_init(), ESP_OK);

    char long_name[128];
    memset(long_name, 'a', sizeof(long_name));
    long_name[sizeof(long_name) - 1] = '\0';

    ASSERT_EQ(ntp_start_sync(long_name), ESP_OK);

    /* Stored copy: 63 chars + NUL, exactly. */
    ASSERT_EQ(strlen(s_ntp_server), 63);
    ASSERT_EQ(s_ntp_server[63], '\0');
    /* What reached SNTP is the same string. */
    ASSERT_EQ(strlen(sntp_mock_server_name), 63);
    ASSERT_TRUE(strncmp(sntp_mock_server_name, long_name, 63) == 0);
}

/* ---- Sync callback + wait ------------------------------------------------------------- */

TEST(ntp_time_sync_callback_marks_synced_and_sets_event_bit)
{
    reset_ntp_under_test();
    ASSERT_EQ(ntp_init(), ESP_OK);
    ASSERT_EQ(ntp_start_sync("pool.ntp.org"), ESP_OK);
    ASSERT_FALSE(ntp_is_synced());
    ASSERT_TRUE(strcmp(ntp_status_str(), "unsynced") == 0);

    struct timeval tv = {.tv_sec = 1700000000, .tv_usec = 123456};
    sntp_mock_fire_time_cb(&tv);

    ASSERT_TRUE(ntp_is_synced());
    ASSERT_TRUE(strcmp(ntp_status_str(), "synced") == 0);
    ASSERT_TRUE(evgrp_mock_bits & NTP_SYNC_BIT);

    /* A waiter that arrives after the callback really sees the bit. */
    ASSERT_TRUE(ntp_wait_sync(0));
    ASSERT_EQ(evgrp_mock_last_wait_bits, NTP_SYNC_BIT);
    ASSERT_EQ(evgrp_mock_last_wait_ticks, pdMS_TO_TICKS(0));
}

TEST(ntp_time_sync_callback_ignores_null_timeval)
{
    reset_ntp_under_test();
    ASSERT_EQ(ntp_init(), ESP_OK);
    ASSERT_EQ(ntp_start_sync("pool.ntp.org"), ESP_OK);

    sntp_mock_fire_time_cb(NULL);

    ASSERT_FALSE(ntp_is_synced());
    ASSERT_EQ(evgrp_mock_bits & NTP_SYNC_BIT, 0);
}

TEST(ntp_wait_sync_times_out_when_no_sync_arrived)
{
    reset_ntp_under_test();
    ASSERT_EQ(ntp_init(), ESP_OK);

    ASSERT_FALSE(ntp_wait_sync(250));
    ASSERT_EQ(evgrp_mock_wait_calls, 1);
    ASSERT_EQ(evgrp_mock_last_wait_ticks, pdMS_TO_TICKS(250));
    ASSERT_FALSE(ntp_is_synced());
}

TEST(ntp_wait_sync_before_init_is_a_clean_false)
{
    reset_ntp_under_test();

    ASSERT_FALSE(ntp_wait_sync(100));
    ASSERT_EQ(evgrp_mock_wait_calls, 0); /* never touched the absent group */
}

/* ---- Periodic resync timer ---------------------------------------------------------------- */

TEST(ntp_periodic_resync_creates_and_arms_one_timer)
{
    reset_ntp_under_test();
    ASSERT_EQ(ntp_init(), ESP_OK);

    ntp_start_periodic_resync();

    etimer_mock_timer_t *t = etimer_mock_by_name("ntp_resync");
    ASSERT_TRUE(t != NULL);
    ASSERT_TRUE(t->running);
    ASSERT_TRUE(t->periodic);
    ASSERT_EQ((long)t->period_us, (long)NTP_RESYNC_INTERVAL_US);

    /* A second start must not create or re-arm a second timer. */
    int starts = etimer_mock_start_periodic_calls;
    ntp_start_periodic_resync();
    ASSERT_EQ(etimer_mock_count, 1);
    ASSERT_EQ(etimer_mock_start_periodic_calls, starts);
}

TEST(ntp_periodic_resync_failure_cleans_up_and_allows_retry)
{
    reset_ntp_under_test();
    ASSERT_EQ(ntp_init(), ESP_OK);

    etimer_mock_start_periodic_ret = ESP_ERR_INVALID_ARG;
    ntp_start_periodic_resync();

    /* The half-created timer was deleted and the module let go of it. */
    ASSERT_EQ(etimer_mock_delete_calls, 1);
    ASSERT_TRUE(s_resync_timer == NULL);
    /* No live timer under that name anymore (deleted slots are invisible). */
    ASSERT_TRUE(etimer_mock_by_name("ntp_resync") == NULL);

    /* A later start retries from scratch — with the knob restored. */
    etimer_mock_start_periodic_ret = ESP_OK;
    ntp_start_periodic_resync();
    ASSERT_EQ(etimer_mock_count, 2);
    ASSERT_TRUE(etimer_mock_by_name("ntp_resync")->running);
}

/*
 * Firing the resync timer is the running client's own reconfigure path — it
 * must also stop-before-reconfigure, and it must re-sync against the STORED
 * server (what ntp_start_sync saved), not a stale literal.
 */
TEST(ntp_resync_callback_stops_then_reconfigures_with_stored_server)
{
    reset_ntp_under_test();
    ASSERT_EQ(ntp_init(), ESP_OK);
    ASSERT_EQ(ntp_start_sync("time.example.com"), ESP_OK);
    ntp_start_periodic_resync();
    ASSERT_TRUE(etimer_mock_by_name("ntp_resync") != NULL);
    ASSERT_TRUE(sntp_mock_running);

    /* Simulate a completed sync so the callback has state to clear. */
    struct timeval tv = {.tv_sec = 1700000000, .tv_usec = 0};
    sntp_mock_fire_time_cb(&tv);
    ASSERT_TRUE(ntp_is_synced());
    ASSERT_TRUE(evgrp_mock_bits & NTP_SYNC_BIT);

    sntp_mock_seq_len = 0;
    etimer_mock_fire("ntp_resync");

    const int want[] = {SNTP_MOCK_STOP, SNTP_MOCK_SETMODE,
                        SNTP_MOCK_SETSERVER, SNTP_MOCK_SETCB, SNTP_MOCK_INIT};
    assert_seq(want, 5);
    ASSERT_TRUE(strcmp(sntp_mock_server_name, "time.example.com") == 0);

    /* Sync state was cleared with the client: the next wait really waits. */
    ASSERT_FALSE(ntp_is_synced());
    ASSERT_EQ(evgrp_mock_bits & NTP_SYNC_BIT, 0);
}

/* ---- Stop ---------------------------------------------------------------------------------- */

TEST(ntp_stop_tears_down_client_and_timer_and_clears_sync)
{
    reset_ntp_under_test();
    ASSERT_EQ(ntp_init(), ESP_OK);
    ASSERT_EQ(ntp_start_sync("pool.ntp.org"), ESP_OK);
    ntp_start_periodic_resync();
    struct timeval tv = {.tv_sec = 1700000000, .tv_usec = 0};
    sntp_mock_fire_time_cb(&tv);
    ASSERT_TRUE(ntp_is_synced());
    ASSERT_TRUE(sntp_mock_running);

    int stops_before = sntp_mock_stop_calls;
    ntp_stop();

    ASSERT_EQ(sntp_mock_stop_calls, stops_before + 1);
    ASSERT_FALSE(sntp_mock_running);
    ASSERT_EQ(etimer_mock_stop_calls, 1);
    ASSERT_EQ(etimer_mock_delete_calls, 1);
    ASSERT_TRUE(s_resync_timer == NULL);
    ASSERT_FALSE(ntp_is_synced());

    /* The freed timer means a later start creates a fresh one, not a
     * "already started" refusal against a dangling handle. */
    ntp_start_periodic_resync();
    ASSERT_EQ(etimer_mock_count, 2);
    ASSERT_TRUE(etimer_mock_by_name("ntp_resync")->running);
}

TEST(ntp_stop_without_resync_timer_is_a_clean_noop)
{
    reset_ntp_under_test();
    ASSERT_EQ(ntp_init(), ESP_OK);
    ASSERT_EQ(ntp_start_sync("pool.ntp.org"), ESP_OK);

    ntp_stop();

    ASSERT_EQ(etimer_mock_stop_calls, 0);
    ASSERT_EQ(etimer_mock_delete_calls, 0);
    ASSERT_FALSE(ntp_is_synced());
}
