/*
 * ============================================================================
 *  Safe-mode boot-failure counter + fallback entry — host contract test
 *  (spaxel-5d2498fb, child of spaxel-19ac094f)
 * ============================================================================
 *
 *  WHY THIS TEST EXISTS
 *  --------------------
 *  safe_mode.c is the node's recovery path from a boot loop. A failed boot
 *  increments a counter persisted in NVS (NVS_KEY_BOOT_COUNTER); when the
 *  count reaches SAFE_MODE_BOOT_COUNT_THRESHOLD the latch in NVS
 *  (NVS_KEY_SAFE_MODE) flips and the next boot comes up in safe mode —
 *  network + OTA only, CSI/BLE disabled — instead of crash-looping on broken
 *  mainline firmware. Two timers bracket recovery: a boot-good window
 *  (SAFE_MODE_BOOT_GOOD_AFTER_S) whose expiry marks the boot healthy and
 *  zeroes the counter, and — only while safe mode is active — an exit timer
 *  (SAFE_MODE_REBOOT_TIMEOUT_S) that sets g_state.restarting and reboots so
 *  normal firmware gets another chance.
 *
 *  None of that state machine can run on a host through the real IDF, but its
 *  decisions are exactly what a boot-looping node depends on, so the whole
 *  contract is pinned here:
 *
 *    - init restores the persisted counter and latch, and creates both
 *      timers exactly once even if init runs twice;
 *    - mark_boot_failed increments and persists immediately (a node that
 *      crashes a microsecond later must not lose the count);
 *    - crossing SAFE_MODE_BOOT_COUNT_THRESHOLD enters safe mode and latches
 *      the flag; staying below it does not;
 *    - mark_boot_good zeroes the counter in memory and NVS, writes nothing
 *      when the count is already zero, and stops a running exit timer;
 *    - enter/exit move the flag (exit also clears the counter) and a re-init
 *      after exit boots normal;
 *    - the timers refuse to start before init, arm at their configured
 *      delays, and each is skipped where it does not apply (boot-good timer
 *      in safe mode; exit timer outside it);
 *    - firing the recorded timer callbacks reproduces the wiring: boot-good
 *      expiry resets the counter; exit expiry sets g_state.restarting BEFORE
 *      calling esp_restart.
 *
 *  HOW IT COMPILES
 *  ---------------
 *  The production TU is included directly:
 *
 *      #include "../main/safe_mode.c"
 *
 *  following the watchdog precedent, so the test exercises the real state
 *  machine rather than a mirror of it, and the Makefile's test_*.c wildcard
 *  needs no per-test wiring. safe_mode.c's IDF includes resolve through
 *  host_compat/ (the Makefile's shared -Ihost_compat -I../main): esp_err.h,
 *  esp_log.h and nvs.h already existed there; esp_timer.h was added for this
 *  test. Only safe_mode.c's includes go through that path — this file names
 *  the headers it needs itself and deliberately does NOT include esp_log.h,
 *  so exactly one esp_log.h is ever expanded per TU.
 *
 *  The NVS fake is the one in test_nvs_migration.c (the harness links one
 *  binary, so the API definitions must exist exactly once); its reset/seed
 *  helpers are reached through nvs_fake.h. The esp_timer calls are recorded
 *  by the fakes below, which hand out stable handles and remember each
 *  timer's name, callback, running state and armed period — tests find the
 *  two production timers by name and fire their callbacks by hand. esp_restart
 *  is likewise recorded, never performed: the host process must survive.
 *
 *  safe_mode.c's file-scope statics became TU-local with the include, so the
 *  reset helper restores them directly between tests and every test below is
 *  order-independent.
 *
 *  Nothing here is reachable from the ESP-IDF build: no CMake file references
 *  firmware/test.
 *  ============================================================================
 */
#include "test_runner.h"

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <string.h>

#include "esp_err.h"
#include "esp_timer.h"
#include "nvs.h"
#include "nvs_fake.h"
#include "safe_mode.h"
#include "spaxel.h"

/* ---- Host fakes for the IDF surface safe_mode.c uses ---------------------- */

/*
 * esp_restart() is resolved transitively by the real IDF headers on target;
 * no host_compat header declares it, so the declaration lives here, ahead of
 * the production TU, and the definition records rather than restarts.
 */
void esp_restart(void);

static int  sm_mock_restart_calls;
static bool sm_mock_restarting_at_restart;   /* g_state.restarting snapshot
                                              * taken when esp_restart fired */

void esp_restart(void)
{
    sm_mock_restart_calls++;
    sm_mock_restarting_at_restart = g_state.restarting;
    /* Deliberately no longjmp/exit: the test asserts on the record after the
     * callback returns. */
}

/*
 * Recording esp_timer fake. Each create() takes a slot with a stable handle;
 * start_once()/stop() flip running and remember the armed period; tests fire
 * a timer's callback by name to model expiry.
 */
typedef struct {
    esp_timer_handle_t handle;
    const char *name;
    esp_timer_cb_t callback;
    void *arg;
    bool running;
    uint64_t period_us;
} sm_fake_timer_t;

#define SM_FAKE_TIMER_MAX 8

static sm_fake_timer_t sm_timers[SM_FAKE_TIMER_MAX];
static int sm_timer_count;

static sm_fake_timer_t *sm_timer_by_handle(esp_timer_handle_t handle)
{
    for (int i = 0; i < sm_timer_count; i++) {
        if (sm_timers[i].handle == handle) {
            return &sm_timers[i];
        }
    }
    return NULL;
}

static sm_fake_timer_t *sm_timer_by_name(const char *name)
{
    for (int i = 0; i < sm_timer_count; i++) {
        if (strcmp(sm_timers[i].name, name) == 0) {
            return &sm_timers[i];
        }
    }
    return NULL;
}

esp_err_t esp_timer_create(const esp_timer_create_args_t *create_args,
                           esp_timer_handle_t *out_handle)
{
    if (create_args == NULL || out_handle == NULL ||
        create_args->callback == NULL) {
        return ESP_ERR_INVALID_ARG;
    }
    if (sm_timer_count >= SM_FAKE_TIMER_MAX) {
        return ESP_ERR_NO_MEM;
    }
    sm_fake_timer_t *t = &sm_timers[sm_timer_count++];
    memset(t, 0, sizeof(*t));
    t->handle   = (esp_timer_handle_t)(uintptr_t)(0x5afe000 + sm_timer_count);
    t->name     = create_args->name;
    t->callback = create_args->callback;
    t->arg      = create_args->arg;
    *out_handle = t->handle;
    return ESP_OK;
}

esp_err_t esp_timer_start_once(esp_timer_handle_t timer, uint64_t timeout_us)
{
    sm_fake_timer_t *t = sm_timer_by_handle(timer);
    if (t == NULL) {
        return ESP_ERR_INVALID_ARG;
    }
    t->running   = true;
    t->period_us = timeout_us;
    return ESP_OK;
}

esp_err_t esp_timer_stop(esp_timer_handle_t timer)
{
    sm_fake_timer_t *t = sm_timer_by_handle(timer);
    if (t == NULL) {
        return ESP_ERR_INVALID_ARG;
    }
    t->running = false;
    return ESP_OK;
}

/* Model a one-shot expiry: the timer stops itself, then the callback runs. */
static void sm_timer_fire(const char *name)
{
    sm_fake_timer_t *t = sm_timer_by_name(name);
    if (t == NULL || t->callback == NULL) {
        return;
    }
    t->running = false;
    t->callback(t->arg);
}

/* ---- The code under test -------------------------------------------------- */

/*
 * safe_mode.c's exit callback sets the shared restart flag through spaxel.h's
 * `extern spaxel_state_t g_state` — exactly one definition binary-wide lives
 * here. (The restart-scenario tests carry their own TU-local static stand-ins
 * and never link this symbol.)
 */
spaxel_state_t g_state;

#include "../main/safe_mode.c"

/*
 * safe_mode.c keeps its state in file-scope statics, which became part of
 * this translation unit with the include above. The suite restores them —
 * and every fake — directly between tests instead of relying on TEST()
 * registration (link) order, so each test is order-independent.
 */
static void reset_safe_mode_under_test(void)
{
    memset(&g_state, 0, sizeof(g_state));
    nvs_t_reset();
    memset(sm_timers, 0, sizeof(sm_timers));
    sm_timer_count = 0;
    sm_mock_restart_calls = 0;
    sm_mock_restarting_at_restart = false;
    s_safe_mode_active = false;
    s_boot_count = 0;
    s_boot_good_timer = NULL;
    s_exit_timer = NULL;
}

/* ---- Store readbacks ------------------------------------------------------- */
/*
 * Assertions read the store through the fake's public nvs_* API (the same
 * calls production makes), not through nvs_fake.h seeding helpers, so a test
 * passes only if the module's writes really landed with the right type.
 */
static esp_err_t sm_nvs_read_u32(const char *key, uint32_t *out)
{
    nvs_handle_t h;
    esp_err_t err = nvs_open(SPAXEL_NAMESPACE, NVS_READONLY, &h);
    if (err != ESP_OK) {
        return err;
    }
    err = nvs_get_u32(h, key, out);
    nvs_close(h);
    return err;
}

static esp_err_t sm_nvs_read_u8(const char *key, uint8_t *out)
{
    nvs_handle_t h;
    esp_err_t err = nvs_open(SPAXEL_NAMESPACE, NVS_READONLY, &h);
    if (err != ESP_OK) {
        return err;
    }
    err = nvs_get_u8(h, key, out);
    nvs_close(h);
    return err;
}

/* ---- Init: restore persisted state ----------------------------------------- */

TEST(safe_mode_init_fresh_store_starts_inactive_at_zero)
{
    reset_safe_mode_under_test();

    ASSERT_EQ(safe_mode_init(), ESP_OK);

    ASSERT_EQ(safe_mode_get_boot_count(), 0);
    ASSERT_FALSE(safe_mode_is_active());
    /* Both timers exist but nothing is armed. */
    ASSERT_EQ(sm_timer_count, 2);
    ASSERT_TRUE(sm_timer_by_name("boot_good") != NULL);
    ASSERT_TRUE(sm_timer_by_name("safe_exit") != NULL);
    ASSERT_FALSE(sm_timer_by_name("boot_good")->running);
    ASSERT_FALSE(sm_timer_by_name("safe_exit")->running);
}

TEST(safe_mode_init_restores_persisted_boot_count)
{
    reset_safe_mode_under_test();
    nvs_t_seed_u32(NVS_KEY_BOOT_COUNTER, 7);

    ASSERT_EQ(safe_mode_init(), ESP_OK);

    ASSERT_EQ(safe_mode_get_boot_count(), 7);
    ASSERT_FALSE(safe_mode_is_active());
}

TEST(safe_mode_init_restores_persisted_safe_mode_flag)
{
    reset_safe_mode_under_test();
    nvs_t_seed_u8(NVS_KEY_SAFE_MODE, SAFE_MODE_ENABLED);
    nvs_t_seed_u32(NVS_KEY_BOOT_COUNTER, 3);

    ASSERT_EQ(safe_mode_init(), ESP_OK);

    ASSERT_TRUE(safe_mode_is_active());
    ASSERT_EQ(safe_mode_get_boot_count(), 3);
}

TEST(safe_mode_init_cleared_flag_boots_normal)
{
    reset_safe_mode_under_test();
    /* The shape safe_mode_exit() leaves behind for the next boot. */
    nvs_t_seed_u8(NVS_KEY_SAFE_MODE, SAFE_MODE_DISABLED);
    nvs_t_seed_u32(NVS_KEY_BOOT_COUNTER, 0);

    ASSERT_EQ(safe_mode_init(), ESP_OK);

    ASSERT_FALSE(safe_mode_is_active());
    ASSERT_EQ(safe_mode_get_boot_count(), 0);
}

TEST(safe_mode_init_creates_each_timer_only_once)
{
    reset_safe_mode_under_test();

    ASSERT_EQ(safe_mode_init(), ESP_OK);
    ASSERT_EQ(safe_mode_init(), ESP_OK);

    ASSERT_EQ(sm_timer_count, 2);
}

/* ---- Boot-failure counter: increment + persistence -------------------------- */

TEST(safe_mode_mark_boot_failed_increments_and_persists)
{
    reset_safe_mode_under_test();
    ASSERT_EQ(safe_mode_init(), ESP_OK);

    ASSERT_EQ(safe_mode_mark_boot_failed(), ESP_OK);
    ASSERT_EQ(safe_mode_get_boot_count(), 1);

    ASSERT_EQ(safe_mode_mark_boot_failed(), ESP_OK);
    ASSERT_EQ(safe_mode_get_boot_count(), 2);

    /* Each increment must already be durable: a node that dies right after
     * the call reboots with the count it died holding. */
    uint32_t stored = 0;
    ASSERT_EQ(sm_nvs_read_u32(NVS_KEY_BOOT_COUNTER, &stored), ESP_OK);
    ASSERT_EQ(stored, 2);
}

/* ---- Threshold: where fallback mode is entered ------------------------------ */

TEST(safe_mode_mark_boot_failed_below_threshold_stays_normal)
{
    reset_safe_mode_under_test();
    nvs_t_seed_u32(NVS_KEY_BOOT_COUNTER, SAFE_MODE_BOOT_COUNT_THRESHOLD - 2);
    ASSERT_EQ(safe_mode_init(), ESP_OK);

    ASSERT_EQ(safe_mode_mark_boot_failed(), ESP_OK);

    ASSERT_EQ(safe_mode_get_boot_count(), SAFE_MODE_BOOT_COUNT_THRESHOLD - 1);
    ASSERT_FALSE(safe_mode_is_active());
    /* No latch written below the threshold. */
    uint8_t flag = 0;
    ASSERT_EQ(sm_nvs_read_u8(NVS_KEY_SAFE_MODE, &flag), ESP_ERR_NVS_NOT_FOUND);
}

TEST(safe_mode_threshold_crossing_enters_safe_mode)
{
    reset_safe_mode_under_test();
    nvs_t_seed_u32(NVS_KEY_BOOT_COUNTER, SAFE_MODE_BOOT_COUNT_THRESHOLD - 1);
    ASSERT_EQ(safe_mode_init(), ESP_OK);
    ASSERT_FALSE(safe_mode_is_active());

    ASSERT_EQ(safe_mode_mark_boot_failed(), ESP_OK);

    ASSERT_EQ(safe_mode_get_boot_count(), SAFE_MODE_BOOT_COUNT_THRESHOLD);
    ASSERT_TRUE(safe_mode_is_active());
    /* Both the count and the latch are persisted, so the NEXT boot — not this
     * one — is the safe-mode boot. */
    uint32_t stored_count = 0;
    uint8_t stored_flag = 0;
    ASSERT_EQ(sm_nvs_read_u32(NVS_KEY_BOOT_COUNTER, &stored_count), ESP_OK);
    ASSERT_EQ(stored_count, SAFE_MODE_BOOT_COUNT_THRESHOLD);
    ASSERT_EQ(sm_nvs_read_u8(NVS_KEY_SAFE_MODE, &stored_flag), ESP_OK);
    ASSERT_EQ(stored_flag, SAFE_MODE_ENABLED);
}

/* ---- Reset on clean boot ----------------------------------------------------- */

TEST(safe_mode_mark_boot_good_resets_counter_and_persists)
{
    reset_safe_mode_under_test();
    nvs_t_seed_u32(NVS_KEY_BOOT_COUNTER, 4);
    ASSERT_EQ(safe_mode_init(), ESP_OK);

    ASSERT_EQ(safe_mode_mark_boot_good(), ESP_OK);

    ASSERT_EQ(safe_mode_get_boot_count(), 0);
    uint32_t stored = 99;
    ASSERT_EQ(sm_nvs_read_u32(NVS_KEY_BOOT_COUNTER, &stored), ESP_OK);
    ASSERT_EQ(stored, 0);
}

TEST(safe_mode_mark_boot_good_at_zero_writes_nothing)
{
    reset_safe_mode_under_test();
    ASSERT_EQ(safe_mode_init(), ESP_OK);

    ASSERT_EQ(safe_mode_mark_boot_good(), ESP_OK);

    /* The healthy-node fast path must not touch NVS at all. */
    uint32_t stored = 0;
    ASSERT_EQ(sm_nvs_read_u32(NVS_KEY_BOOT_COUNTER, &stored),
              ESP_ERR_NVS_NOT_FOUND);
}

TEST(safe_mode_mark_boot_good_stops_running_exit_timer)
{
    reset_safe_mode_under_test();
    /* The escape hatch: the operator recovers the node over OTA while it sits
     * in safe mode; marking boot good must disarm the pending reboot. */
    nvs_t_seed_u8(NVS_KEY_SAFE_MODE, SAFE_MODE_ENABLED);
    nvs_t_seed_u32(NVS_KEY_BOOT_COUNTER, 5);
    ASSERT_EQ(safe_mode_init(), ESP_OK);
    ASSERT_EQ(safe_mode_start_exit_timer(), ESP_OK);
    ASSERT_TRUE(sm_timer_by_name("safe_exit")->running);

    ASSERT_EQ(safe_mode_mark_boot_good(), ESP_OK);

    ASSERT_FALSE(sm_timer_by_name("safe_exit")->running);
    ASSERT_EQ(safe_mode_get_boot_count(), 0);
}

/* ---- Enter / exit ------------------------------------------------------------ */

TEST(safe_mode_enter_sets_flag_without_touching_counter)
{
    reset_safe_mode_under_test();
    ASSERT_EQ(safe_mode_init(), ESP_OK);

    ASSERT_EQ(safe_mode_enter(), ESP_OK);

    ASSERT_TRUE(safe_mode_is_active());
    uint8_t stored_flag = 0;
    ASSERT_EQ(sm_nvs_read_u8(NVS_KEY_SAFE_MODE, &stored_flag), ESP_OK);
    ASSERT_EQ(stored_flag, SAFE_MODE_ENABLED);
    /* Entering does not fabricate or clear a failure count. */
    uint32_t stored_count = 0;
    ASSERT_EQ(sm_nvs_read_u32(NVS_KEY_BOOT_COUNTER, &stored_count),
              ESP_ERR_NVS_NOT_FOUND);
}

TEST(safe_mode_exit_clears_flag_and_resets_counter)
{
    reset_safe_mode_under_test();
    nvs_t_seed_u8(NVS_KEY_SAFE_MODE, SAFE_MODE_ENABLED);
    nvs_t_seed_u32(NVS_KEY_BOOT_COUNTER, SAFE_MODE_BOOT_COUNT_THRESHOLD);
    ASSERT_EQ(safe_mode_init(), ESP_OK);
    ASSERT_TRUE(safe_mode_is_active());

    ASSERT_EQ(safe_mode_exit(), ESP_OK);

    ASSERT_FALSE(safe_mode_is_active());
    uint8_t stored_flag = SAFE_MODE_ENABLED;
    uint32_t stored_count = 99;
    ASSERT_EQ(sm_nvs_read_u8(NVS_KEY_SAFE_MODE, &stored_flag), ESP_OK);
    ASSERT_EQ(stored_flag, SAFE_MODE_DISABLED);
    ASSERT_EQ(sm_nvs_read_u32(NVS_KEY_BOOT_COUNTER, &stored_count), ESP_OK);
    ASSERT_EQ(stored_count, 0);

    /* The next boot reads a clean store and comes up normal. */
    ASSERT_EQ(safe_mode_init(), ESP_OK);
    ASSERT_FALSE(safe_mode_is_active());
    ASSERT_EQ(safe_mode_get_boot_count(), 0);
}

/* ---- Timer contracts ---------------------------------------------------------- */

TEST(safe_mode_timer_starts_before_init_are_rejected)
{
    reset_safe_mode_under_test();

    ASSERT_EQ(safe_mode_start_boot_good_timer(), ESP_ERR_INVALID_STATE);
    ASSERT_EQ(safe_mode_start_exit_timer(), ESP_ERR_INVALID_STATE);
}

TEST(safe_mode_boot_good_timer_starts_at_configured_delay)
{
    reset_safe_mode_under_test();
    ASSERT_EQ(safe_mode_init(), ESP_OK);

    ASSERT_EQ(safe_mode_start_boot_good_timer(), ESP_OK);

    sm_fake_timer_t *t = sm_timer_by_name("boot_good");
    ASSERT_TRUE(t != NULL);
    ASSERT_TRUE(t->running);
    ASSERT_EQ((long)t->period_us, (long)(SAFE_MODE_BOOT_GOOD_AFTER_S * 1000000ULL));
    /* Only the boot-good window is armed by a normal boot. */
    ASSERT_FALSE(sm_timer_by_name("safe_exit")->running);
}

TEST(safe_mode_boot_good_timer_skipped_in_safe_mode)
{
    reset_safe_mode_under_test();
    nvs_t_seed_u8(NVS_KEY_SAFE_MODE, SAFE_MODE_ENABLED);
    ASSERT_EQ(safe_mode_init(), ESP_OK);

    /* Safe mode skips deliberately and reports OK — arming the healthy-boot
     * countdown is pointless when the node is already in fallback. */
    ASSERT_EQ(safe_mode_start_boot_good_timer(), ESP_OK);

    sm_fake_timer_t *t = sm_timer_by_name("boot_good");
    ASSERT_TRUE(t != NULL);
    ASSERT_FALSE(t->running);
}

TEST(safe_mode_exit_timer_skipped_outside_safe_mode)
{
    reset_safe_mode_under_test();
    ASSERT_EQ(safe_mode_init(), ESP_OK);

    ASSERT_EQ(safe_mode_start_exit_timer(), ESP_OK);

    sm_fake_timer_t *t = sm_timer_by_name("safe_exit");
    ASSERT_TRUE(t != NULL);
    ASSERT_FALSE(t->running);
}

TEST(safe_mode_exit_timer_starts_at_configured_delay_in_safe_mode)
{
    reset_safe_mode_under_test();
    nvs_t_seed_u8(NVS_KEY_SAFE_MODE, SAFE_MODE_ENABLED);
    ASSERT_EQ(safe_mode_init(), ESP_OK);

    ASSERT_EQ(safe_mode_start_exit_timer(), ESP_OK);

    sm_fake_timer_t *t = sm_timer_by_name("safe_exit");
    ASSERT_TRUE(t != NULL);
    ASSERT_TRUE(t->running);
    ASSERT_EQ((long)t->period_us, (long)(SAFE_MODE_REBOOT_TIMEOUT_S * 1000000ULL));
}

TEST(safe_mode_stop_boot_good_timer_stops_running_timer)
{
    reset_safe_mode_under_test();
    ASSERT_EQ(safe_mode_init(), ESP_OK);
    ASSERT_EQ(safe_mode_start_boot_good_timer(), ESP_OK);
    ASSERT_TRUE(sm_timer_by_name("boot_good")->running);

    ASSERT_EQ(safe_mode_stop_boot_good_timer(), ESP_OK);

    ASSERT_FALSE(sm_timer_by_name("boot_good")->running);
}

/* ---- Callback wiring ------------------------------------------------------------ */

TEST(safe_mode_boot_good_expiry_resets_counter)
{
    reset_safe_mode_under_test();
    nvs_t_seed_u32(NVS_KEY_BOOT_COUNTER, 5);
    ASSERT_EQ(safe_mode_init(), ESP_OK);
    ASSERT_EQ(safe_mode_start_boot_good_timer(), ESP_OK);

    /* The window expires without anyone calling stop: the boot is healthy. */
    sm_timer_fire("boot_good");

    ASSERT_EQ(safe_mode_get_boot_count(), 0);
    uint32_t stored = 99;
    ASSERT_EQ(sm_nvs_read_u32(NVS_KEY_BOOT_COUNTER, &stored), ESP_OK);
    ASSERT_EQ(stored, 0);
    ASSERT_EQ(sm_mock_restart_calls, 0);
}

TEST(safe_mode_exit_timer_expiry_sets_restarting_then_restarts)
{
    reset_safe_mode_under_test();
    nvs_t_seed_u8(NVS_KEY_SAFE_MODE, SAFE_MODE_ENABLED);
    ASSERT_EQ(safe_mode_init(), ESP_OK);
    ASSERT_EQ(safe_mode_start_exit_timer(), ESP_OK);
    ASSERT_FALSE(g_state.restarting);
    ASSERT_EQ(sm_mock_restart_calls, 0);

    sm_timer_fire("safe_exit");

    ASSERT_TRUE(g_state.restarting);
    ASSERT_EQ(sm_mock_restart_calls, 1);
    /* The restarting flag is what the main loop observes to shut workers down
     * cleanly — it must be set BEFORE esp_restart() is reached. */
    ASSERT_TRUE(sm_mock_restarting_at_restart);
}
