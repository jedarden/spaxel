/*
 * ============================================================================
 *  Spaxel firmware host test harness — gcc runner
 * ============================================================================
 *
 *  SINGLE COMMAND TO COMPILE AND RUN (from the repo root):
 *
 *      make -C firmware/test test
 *
 *  That compiles every test_*.c in firmware/test/ with plain gcc and runs the
 *  whole suite. The runner exits non-zero if ANY assertion fails, so it is safe
 *  to wire into CI (make's exit code reflects the test result).
 *
 *  ----------------------------------------------------------------------------
 *  WHY A PLAIN gcc HARNESS (and not ESP-IDF --target linux / Unity-host)
 *  ----------------------------------------------------------------------------
 *  the firmware/main C sources cannot be compiled or linked for the ESP-IDF `linux`
 *  target, so the idf.py host-test / Unity path was rejected:
 *
 *    - csi.c        pulls in esp_wifi.h    — CSI callback + promiscuous-mode
 *                                            API backed by the radio MAC/baseband
 *                                            blob. No host build exists.
 *    - provision.c  pulls in driver/uart.h — UART peripheral driver. No host
 *                                            build exists.
 *
 *  Worse, firmware/main builds as ONE idf_component_register(...) whose
 *  REQUIRES line names esp_wifi, bt, and driver — three components with no
 *  linux build — so the whole component (and thus every translation unit in
 *  it) is unhostable, even nvs_migration.c, whose own includes would otherwise
 *  be hostable in isolation. The IDF host-test model builds whole components,
 *  not cherry-picked TUs, so there is no way to host-compile csi.c/wifi.c/
 *  websocket.c/ble.c/provision.c/led.c/ntp.c without refactoring production
 *  firmware purely to satisfy tests.
 *
 *  This harness therefore does NOT #include or link the firmware/main C sources. It tests
 *  pure-logic extractions and binary-format contracts in self-contained units
 *  that compile with nothing more than a C compiler. The esp_wifi/uart/nvs call
 *  sites themselves remain validated on-target and via the Go-side spaxel-sim
 *  acceptance suite.
 *
 *  Full decision record: docs/notes/firmware-host-test-approach.md (bead
 *  bf-21t). This harness is the bf-4ne build-out.
 *
 *  ----------------------------------------------------------------------------
 *  ADDING A TEST
 *  ----------------------------------------------------------------------------
 *  Drop a new file firmware/test/test_<thing>.c and write:
 *
 *      #include "test_runner.h"
 *
 *      TEST(my_thing) {
 *          ASSERT_EQ(1 + 1, 2);
 *      }
 *
 *  The Makefile globs test_*.c, so the new file is compiled and its TEST() is
 *  self-registered via a GCC constructor — no change to the runner required.
 *  ============================================================================
 */
#pragma once

#include <setjmp.h>
#include <stdbool.h>
#include <stdint.h>

/* ---- Per-test self-registration ----------------------------------------- */

typedef void (*test_fn)(void);

typedef struct {
    const char *name;
    test_fn     fn;
} test_entry_t;

/*
 * Append a test to the global registry. Called from constructor functions
 * emitted by the TEST() macro, so the list is fully populated before main().
 */
void test_register(const char *name, test_fn fn);

/*
 * Define a self-registering test. Expands to a static body function plus a
 * constructor that registers it.
 *
 *     TEST(addition) { ASSERT_EQ(1 + 1, 2); }
 */
#define TEST(name)                                                            \
    static void spaxel_test_##name##_body(void);                              \
    static void spaxel_test_##name##_reg(void) __attribute__((constructor)); \
    static void spaxel_test_##name##_reg(void) {                              \
        test_register(#name, spaxel_test_##name##_body);                      \
    }                                                                         \
    static void spaxel_test_##name##_body(void)

/* ---- Assertions --------------------------------------------------------- */
/*
 * On failure these record the location, mark the suite failed, and longjmp out
 * of the CURRENT test only — the remaining tests still run. main() returns
 * non-zero if any assertion has failed across the whole run.
 */

void test_record_failure(const char *file, int line, const char *fmt, ...);

#define ASSERT_TRUE(cond)                                                      \
    do {                                                                       \
        if (!(cond)) {                                                         \
            test_record_failure(__FILE__, __LINE__,                            \
                                "ASSERT_TRUE(%s) failed", #cond);              \
        }                                                                      \
    } while (0)

#define ASSERT_FALSE(cond)                                                     \
    do {                                                                       \
        if ((cond)) {                                                          \
            test_record_failure(__FILE__, __LINE__,                            \
                                "ASSERT_FALSE(%s) failed", #cond);             \
        }                                                                      \
    } while (0)

/*
 * Integer-equality assertion (actual/expected are evaluated once each and
 * compared as long). Use ASSERT_TRUE for floats, strings, or pointers.
 */
#define ASSERT_EQ(actual, expected)                                            \
    do {                                                                       \
        long _spaxel_a = (long)(actual);                                       \
        long _spaxel_e = (long)(expected);                                     \
        if (_spaxel_a != _spaxel_e) {                                          \
            test_record_failure(__FILE__, __LINE__,                            \
                                "ASSERT_EQ(%s, %s) failed: got %ld, want %ld", \
                                #actual, #expected, _spaxel_a, _spaxel_e);     \
        }                                                                      \
    } while (0)
