/*
 * ============================================================================
 *  Console selection contract — committed sdkconfig defaults (spaxel-5049d982)
 * ============================================================================
 *
 *  WHY THIS TEST EXISTS
 *  --------------------
 *  The console defect this guards against was not a one-off bug but a
 *  recurring shape: a hand-configured sdkconfig had USB-Serial/JTAG console
 *  set, the setting was never captured in sdkconfig.defaults, and it silently
 *  vanished the first time anyone built from scratch. The stock build then
 *  inherited UART0 (GPIO43/44) — unconnected on a native-USB-only ESP32-S3 —
 *  and the device booted in complete silence over its only host link. See
 *  ADR-002 and the console build instructions in firmware/README.md.
 *
 *  verify-console-config.sh checks the GENERATED sdkconfig, which only exists
 *  after an ESP-IDF configure that this gcc harness (and most hosts) cannot
 *  run. This test checks the COMMITTED defaults files directly, so a
 *  regression that deletes or flips the console lines fails `make test` on
 *  any machine with a C compiler — no toolchain, no board, no CI image.
 *
 *  WHAT IT ASSERTS
 *  ---------------
 *  1. The shared sdkconfig.defaults does not hardcode a board console.
 *  2. sdkconfig.usbjtag selects USB-Serial/JTAG and keeps panic output
 *     printing (PRINT_REBOOT, never silent reboot).
 *  3. sdkconfig.uart-console selects UART0 with the same panic behaviour and
 *     retains native USB only as the secondary provisioning transport.
 *  4. CMake selects sdkconfig.usbjtag only when SDKCONFIG_DEFAULTS was not
 *     supplied by the caller, leaving the documented UART0 override reachable.
 * ============================================================================
 */

#include "test_runner.h"

#include <stdio.h>
#include <string.h>

/* ---- File loading --------------------------------------------------------- */

/*
 * Load the first existing candidate of a config file into buf. The harness
 * runs from firmware/test (make's directory), so "../x" is the normal path;
 * "firmware/x" covers someone running the binary from the repo root.
 * Returns the length read, or -1 if no candidate exists / none fits.
 */
static long load_config(const char *const *candidates, char *buf, size_t bufsize)
{
    for (size_t i = 0; candidates[i] != NULL; i++) {
        FILE *f = fopen(candidates[i], "r");
        if (f == NULL) {
            continue;
        }
        size_t n = fread(buf, 1, bufsize - 1, f);
        fclose(f);
        if (n == 0 || n == bufsize - 1) {
            return -1; /* empty or truncated past the buffer */
        }
        buf[n] = '\0';
        return (long)n;
    }
    return -1;
}

static char shared_text[32 * 1024];
static char usb_text[32 * 1024];
static char uart_text[32 * 1024];
static char cmake_text[32 * 1024];

/* Load both files once; every test below fails fast if this did not work. */
static bool configs_loaded(void)
{
    static int done = 0;
    static bool ok = false;
    if (!done) {
        done = 1;
        static const char *const shared_paths[] = {
            "../sdkconfig.defaults", "firmware/sdkconfig.defaults", NULL};
        static const char *const usb_paths[] = {
            "../sdkconfig.usbjtag", "firmware/sdkconfig.usbjtag", NULL};
        static const char *const uart_paths[] = {
            "../sdkconfig.uart-console", "firmware/sdkconfig.uart-console", NULL};
        static const char *const cmake_paths[] = {
            "../CMakeLists.txt", "firmware/CMakeLists.txt", NULL};
        ok = load_config(shared_paths, shared_text, sizeof(shared_text)) >= 0 &&
             load_config(usb_paths, usb_text, sizeof(usb_text)) >= 0 &&
             load_config(uart_paths, uart_text, sizeof(uart_text)) >= 0 &&
             load_config(cmake_paths, cmake_text, sizeof(cmake_text)) >= 0;
    }
    return ok;
}

/* ---- Whole-line matching --------------------------------------------------- */

/* True if line (e.g. "CONFIG_ESP_CONSOLE_USB_SERIAL_JTAG=y") appears as an
 * exact line in text. Exact-line, not substring: "CONFIG_X=y" must not match
 * a "CONFIG_X=yn" typo or a commented-out "# CONFIG_X=y" line. */
static bool has_exact_line(const char *text, const char *line)
{
    size_t len = strlen(line);
    const char *p = text;
    while ((p = strstr(p, line)) != NULL) {
        bool bol = (p == text || p[-1] == '\n');
        bool eol = (p[len] == '\0' || p[len] == '\n' || p[len] == '\r');
        if (bol && eol) {
            return true;
        }
        p += 1;
    }
    return false;
}

/* ---- Tests ------------------------------------------------------------------ */

/* Board-neutral settings stay shared without selecting either physical port. */
TEST(shared_defaults_do_not_hardcode_console)
{
    ASSERT_TRUE(configs_loaded());

    ASSERT_FALSE(has_exact_line(shared_text, "CONFIG_ESP_CONSOLE_USB_SERIAL_JTAG=y"));
    ASSERT_FALSE(has_exact_line(shared_text, "CONFIG_ESP_CONSOLE_UART_DEFAULT=y"));
    ASSERT_FALSE(has_exact_line(shared_text, "CONFIG_ESP_CONSOLE_UART_DEFAULT=n"));
}

/* Shipped profile: native USB primary, UART0 explicitly off, panic printing. */
TEST(default_usbjtag_profile_selects_native_usb)
{
    ASSERT_TRUE(configs_loaded());

    ASSERT_TRUE(has_exact_line(usb_text, "CONFIG_ESP_CONSOLE_USB_SERIAL_JTAG=y"));
    ASSERT_TRUE(has_exact_line(usb_text, "CONFIG_ESP_CONSOLE_UART_DEFAULT=n"));
    ASSERT_TRUE(has_exact_line(usb_text, "CONFIG_ESP_CONSOLE_SECONDARY_NONE=y"));
    ASSERT_TRUE(has_exact_line(usb_text, "CONFIG_ESP_SYSTEM_PANIC_PRINT_REBOOT=y"));
}

/* Bridge profile: UART0 primary, native USB secondary for the provisioning
 * VFS, and panic printing. USB is explicitly off as the primary console. */
TEST(uart_console_profile_selects_uart0)
{
    ASSERT_TRUE(configs_loaded());

    ASSERT_TRUE(has_exact_line(uart_text, "CONFIG_ESP_CONSOLE_UART_DEFAULT=y"));
    ASSERT_TRUE(has_exact_line(uart_text, "CONFIG_ESP_CONSOLE_USB_SERIAL_JTAG=n"));
    ASSERT_TRUE(has_exact_line(
        uart_text, "CONFIG_ESP_CONSOLE_SECONDARY_USB_SERIAL_JTAG=y"));
    ASSERT_TRUE(has_exact_line(uart_text, "CONFIG_ESP_SYSTEM_PANIC_PRINT_REBOOT=y"));
}

/* CMake chooses USB only as its default. A caller-provided SDKCONFIG_DEFAULTS
 * skips this assignment and can therefore select the UART profile cleanly. */
TEST(cmake_defaults_to_usbjtag_profile_conditionally)
{
    ASSERT_TRUE(configs_loaded());

    ASSERT_TRUE(has_exact_line(cmake_text, "if(NOT DEFINED SDKCONFIG_DEFAULTS)"));
    ASSERT_TRUE(has_exact_line(
        cmake_text,
        "    set(SDKCONFIG_DEFAULTS \"sdkconfig.defaults;sdkconfig.usbjtag\")"));
    ASSERT_TRUE(has_exact_line(cmake_text, "endif()"));
}

/* A backtrace is only useful if it reaches the port the operator is watching.
 * Neither profile may ever select a silent or halting panic behaviour. */
TEST(both_console_profiles_keep_panic_output_visible)
{
    ASSERT_TRUE(configs_loaded());

    /* The one required setting, present in both files... */
    ASSERT_TRUE(has_exact_line(usb_text, "CONFIG_ESP_SYSTEM_PANIC_PRINT_REBOOT=y"));
    ASSERT_TRUE(has_exact_line(uart_text, "CONFIG_ESP_SYSTEM_PANIC_PRINT_REBOOT=y"));

    /* ...and none of the settings that would swallow the backtrace. */
    const char *silent[] = {
        "CONFIG_ESP_SYSTEM_PANIC_SILENT_REBOOT=y",
        "CONFIG_ESP_SYSTEM_PANIC_PRINT_HALT=y",
        "CONFIG_ESP_SYSTEM_PANIC_GDBSTUB=y",
    };
    for (size_t i = 0; i < sizeof(silent) / sizeof(silent[0]); i++) {
        ASSERT_FALSE(has_exact_line(usb_text, silent[i]));
        ASSERT_FALSE(has_exact_line(uart_text, silent[i]));
    }
}
