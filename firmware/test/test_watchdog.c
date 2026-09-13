/*
 * ============================================================================
 *  Watchdog subscription contract — host contract test (spaxel-61d41649)
 * ============================================================================
 *
 *  WHY THIS TEST EXISTS
 *  --------------------
 *  firmware/main/watchdog.c used to subscribe the calling task from inside
 *  watchdog_init(), and watchdog_feed() returned ESP_OK without calling
 *  esp_task_wdt_reset(). A subscribed task that never feeds is not a slowdown:
 *  the TWDT panics and reboots the node SPAXEL_WATCHDOG_TIMEOUT_S after boot,
 *  on a perfectly healthy node — every node, every boot. The fix arms the
 *  window only (idle tasks via idle_core_mask, which IDF's idle hook feeds)
 *  and moves subscription into an explicit watchdog_subscribe() that a
 *  long-lived task calls once, with watchdog_feed() as its mandatory per-loop
 *  reset.
 *
 *  The real TWDT cannot exist on a host, and QEMU cannot reproduce the trip
 *  (spaxel-69cdd68c: a control build arming the window with a task that never
 *  fed ran 30s past its due reset without resetting). What is pinned here is
 *  the contract that makes the trip impossible on a healthy node:
 *
 *    - watchdog_init() arms timeout_ms == SPAXEL_WATCHDOG_TIMEOUT_S * 1000
 *      with trigger_panic, through esp_task_wdt_init() or (the production
 *      path, where startup already initialized the TWDT at the 5s Kconfig
 *      default) esp_task_wdt_reconfigure(), and subscribes NO task;
 *    - re-initializing is a no-op;
 *    - watchdog_subscribe() refuses before init, subscribes exactly the
 *      calling task after it;
 *    - watchdog_feed() really calls esp_task_wdt_reset() and returns its
 *      result, and refuses before init.
 *
 *  HOW IT COMPILES
 *  ---------------
 *  The production TU is included directly:
 *
 *      #include "../main/watchdog.c"
 *
 *  so the test always exercises the real code rather than a copy, and the
 *  Makefile's test_*.c wildcard needs no per-test wiring. watchdog.c's IDF
 *  includes (esp_err.h, esp_log.h, esp_task_wdt.h, freertos headers) resolve
 *  to the host stand-ins in stubs/ via a target-specific include override for
 *  this object in the Makefile. Nothing here is reachable from the ESP-IDF
 *  build: no CMake file references firmware/test.
 *
 *  Only watchdog.c's includes go through the stub path. This file names the
 *  stubs it needs explicitly (stubs/esp_err.h, stubs/esp_task_wdt.h, and the
 *  stubs/freertos headers) but deliberately does NOT include stubs/esp_log.h —
 *  watchdog.c's own include must be the one to resolve it, so exactly one
 *  esp_log.h is ever expanded per TU.
 *
 *  AGAINST THE PRE-FIX CODE (79f3f286~1) THIS SUITE IS RED
 *  -------------------------------------------------------
 *  The harness fails to link: watchdog_subscribe() did not exist. With a
 *  link shim supplying that symbol the remaining assertions still fail —
 *  watchdog_init() calls esp_task_wdt_add() (subscribing the one task that
 *  can never feed it) and watchdog_feed() never calls esp_task_wdt_reset().
 *  Either way the suite cannot pass while the defect is present.
 *  ============================================================================
 */
#include "test_runner.h"

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include "stubs/esp_err.h"
#include "stubs/esp_task_wdt.h"
#include "stubs/freertos/FreeRTOS.h"
#include "stubs/freertos/task.h"

/* ---- Host implementations of the stub surface ---------------------------- */

/*
 * The harness links every test_*.c into one binary, so this is the single
 * esp_err_to_name() for the whole suite — nvs_migration.c's ESP_LOG formatting
 * resolves here too (test_nvs_migration.c carries a pointer, not a second
 * definition, which would be a link error).
 */
const char *esp_err_to_name(esp_err_t code)
{
    switch (code) {
    case ESP_OK:                return "ESP_OK";
    case ESP_FAIL:              return "ESP_FAIL";
    case ESP_ERR_NO_MEM:        return "ESP_ERR_NO_MEM";
    case ESP_ERR_INVALID_ARG:   return "ESP_ERR_INVALID_ARG";
    case ESP_ERR_INVALID_STATE: return "ESP_ERR_INVALID_STATE";
    case ESP_ERR_INVALID_SIZE:  return "ESP_ERR_INVALID_SIZE";
    case ESP_ERR_NOT_FOUND:     return "ESP_ERR_NOT_FOUND";
    default:                    return "ESP_ERR_UNKNOWN";
    }
}

const char *pcTaskGetName(TaskHandle_t task_to_query)
{
    /* Single-task stub: every query names the same task. */
    (void)task_to_query;
    return wdt_mock_current_task_name;
}

TaskHandle_t xTaskGetCurrentTaskHandle(void)
{
    return wdt_mock_current_task;
}

/* ---- The code under test -------------------------------------------------- */

#include "../main/watchdog.c"

/*
 * watchdog.c keeps its armed-state in a file-scope static, which became part
 * of this translation unit with the include above. The suite restores it
 * directly between tests instead of relying on TEST() registration (link)
 * order, so every test below is order-independent.
 */
static void reset_watchdog_under_test(void)
{
    s_watchdog_initialized = false;
    wdt_mock_reset_all();
}

/* ---- The contract --------------------------------------------------------- */

/*
 * The production path. Startup already initialized the TWDT at the 5s Kconfig
 * default, so esp_task_wdt_init() answers ESP_ERR_INVALID_STATE and
 * watchdog_init() must fall through to reconfigure to raise the window to
 * SPAXEL_WATCHDOG_TIMEOUT_S — and must not subscribe anything, because the
 * only task present here is app_main's, which returns once the workers are
 * spawned and can never feed. That missing feed is what rebooted every
 * healthy node ~90s after boot.
 */
TEST(watchdog_init_reconfigures_window_and_subscribes_nothing)
{
    reset_watchdog_under_test();

    ASSERT_EQ(watchdog_init(), ESP_OK);

    ASSERT_EQ(wdt_mock_init_calls, 1);
    ASSERT_EQ(wdt_mock_reconfigure_calls, 1);
    ASSERT_EQ(wdt_mock_reconfigure_cfg.timeout_ms, SPAXEL_WATCHDOG_TIMEOUT_S * 1000);
    ASSERT_EQ(wdt_mock_reconfigure_cfg.idle_core_mask, (1 << portNUM_PROCESSORS) - 1);
    ASSERT_TRUE(wdt_mock_reconfigure_cfg.trigger_panic);

    /* The defect this suite exists for. */
    ASSERT_EQ(wdt_mock_add_calls, 0);
    ASSERT_EQ(wdt_mock_delete_calls, 0);
}

/*
 * Same contract when this firmware is the first to touch the TWDT (startup
 * watchdog disabled): the arming config must reach esp_task_wdt_init() intact,
 * and still subscribe nothing.
 */
TEST(watchdog_init_arms_window_when_twdt_not_yet_initialized)
{
    reset_watchdog_under_test();
    wdt_mock_init_ret = ESP_OK;

    ASSERT_EQ(watchdog_init(), ESP_OK);

    ASSERT_EQ(wdt_mock_init_calls, 1);
    ASSERT_EQ(wdt_mock_reconfigure_calls, 0);
    ASSERT_EQ(wdt_mock_init_cfg.timeout_ms, SPAXEL_WATCHDOG_TIMEOUT_S * 1000);
    ASSERT_EQ(wdt_mock_init_cfg.idle_core_mask, (1 << portNUM_PROCESSORS) - 1);
    ASSERT_TRUE(wdt_mock_init_cfg.trigger_panic);
    ASSERT_EQ(wdt_mock_add_calls, 0);
}

/*
 * A second init must be a no-op: re-arming mid-run would move a window that
 * subscribed tasks are already living under.
 */
TEST(watchdog_init_is_idempotent)
{
    reset_watchdog_under_test();

    ASSERT_EQ(watchdog_init(), ESP_OK);
    int inits    = wdt_mock_init_calls;
    int reconfigs = wdt_mock_reconfigure_calls;

    ASSERT_EQ(watchdog_init(), ESP_OK);

    ASSERT_EQ(wdt_mock_init_calls, inits);
    ASSERT_EQ(wdt_mock_reconfigure_calls, reconfigs);
    ASSERT_EQ(wdt_mock_add_calls, 0);
}

/*
 * Arming can genuinely fail, and the caller has to see it: an ESP_OK from a
 * failed arm would let a long-lived task subscribe to a watchdog that is not
 * running. The module must also stay disarmed afterwards, so a late subscribe
 * is refused rather than registered against nothing.
 */
TEST(watchdog_init_propagates_arm_failure_and_stays_disarmed)
{
    reset_watchdog_under_test();
    wdt_mock_init_ret        = ESP_ERR_NO_MEM;
    wdt_mock_reconfigure_ret = ESP_ERR_NO_MEM;

    ASSERT_EQ(watchdog_init(), ESP_ERR_NO_MEM);
    ASSERT_EQ(wdt_mock_reconfigure_calls, 0);

    ASSERT_EQ(watchdog_subscribe(), ESP_ERR_INVALID_STATE);
    ASSERT_EQ(wdt_mock_add_calls, 0);
}

/*
 * Subscribing before init must refuse rather than quietly register a task
 * against a watchdog that is not running — which is the silent variant of the
 * same bug: a subscription that can never be fed and never trips either.
 */
TEST(watchdog_subscribe_before_init_is_rejected)
{
    reset_watchdog_under_test();

    ASSERT_EQ(watchdog_subscribe(), ESP_ERR_INVALID_STATE);
    ASSERT_EQ(wdt_mock_add_calls, 0);
}

/*
 * The long-lived task opts in explicitly, and what gets subscribed is the
 * CALLING task: esp_task_wdt_add(NULL). The test gives the stub a concrete
 * current-task identity first, so a regression that subscribes a captured
 * handle instead of NULL — the shape that rebooted every healthy node — is
 * caught rather than hidden by an identical host value.
 */
TEST(watchdog_subscribe_after_init_subscribes_calling_task)
{
    reset_watchdog_under_test();
    wdt_mock_current_task = (TaskHandle_t)0x1000;

    ASSERT_EQ(watchdog_init(), ESP_OK);
    ASSERT_EQ(watchdog_subscribe(), ESP_OK);

    ASSERT_EQ(wdt_mock_add_calls, 1);
    ASSERT_TRUE(wdt_mock_added_handle == NULL);
}

/*
 * Feeding must actually reset this task's entry — yielding or blocking does
 * not, and a feed that forgets to reset is the original defect wearing a
 * different hat — and must return the TWDT's answer so a caller can tell a
 * healthy feed from "not subscribed".
 */
TEST(watchdog_feed_resets_and_returns_twdt_result)
{
    reset_watchdog_under_test();

    ASSERT_EQ(watchdog_init(), ESP_OK);

    wdt_mock_reset_ret = ESP_ERR_NOT_FOUND;
    ASSERT_EQ(watchdog_feed(), ESP_ERR_NOT_FOUND);
    ASSERT_EQ(wdt_mock_reset_calls, 1);

    wdt_mock_reset_ret = ESP_OK;
    ASSERT_EQ(watchdog_feed(), ESP_OK);
    ASSERT_EQ(wdt_mock_reset_calls, 2);
}

/* Feeding before init must refuse and must not touch the absent watchdog. */
TEST(watchdog_feed_before_init_is_rejected)
{
    reset_watchdog_under_test();

    ASSERT_EQ(watchdog_feed(), ESP_ERR_INVALID_STATE);
    ASSERT_EQ(wdt_mock_reset_calls, 0);
}
