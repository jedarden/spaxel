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
 *  ADR-002 and docs/notes/console-default-verification.md.
 *
 *  verify-console-config.sh checks the GENERATED sdkconfig, which only exists
 *  after an ESP-IDF configure that this gcc harness (and most hosts) cannot
 *  run. This test checks the COMMITTED defaults files directly, so a
 *  regression that deletes or flips the console lines fails `make test` on
 *  any machine with a C compiler — no toolchain, no board, no CI image.
 *
 *  WHAT IT ASSERTS
 *  ---------------
 *  1. sdkconfig.defaults selects USB-Serial/JTAG as the primary console and
 *     keeps panic output printing (PRINT_REBOOT, never silent reboot).
 *  2. sdkconfig.uart-console (the documented bridge-board override, layered
 *     via SDKCONFIG_DEFAULTS) flips the primary console to UART0 while
 *     keeping the panic behaviour.
 *  3. Layering the override over the defaults — the exact mechanism the
 *     README documents — yields the UART0 profile for every console key,
 *     because a later defaults file redefines the same keys. This is the
 *     "override is reachable" contract, not just file contents.
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

static char defaults_text[32 * 1024];
static char override_text[32 * 1024];

/* Load both files once; every test below fails fast if this did not work. */
static bool configs_loaded(void)
{
    static int done = 0;
    static bool ok = false;
    if (!done) {
        done = 1;
        static const char *const defaults_paths[] = {
            "../sdkconfig.defaults", "firmware/sdkconfig.defaults", NULL};
        static const char *const override_paths[] = {
            "../sdkconfig.uart-console", "firmware/sdkconfig.uart-console", NULL};
        ok = load_config(defaults_paths, defaults_text, sizeof(defaults_text)) >= 0 &&
             load_config(override_paths, override_text, sizeof(override_text)) >= 0;
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

/* ---- Minimal CONFIG_KEY=value layering ------------------------------------- */

#define MAX_KV 256
typedef struct {
    char key[96];
    char val[32];
} kv_t;

/* Parse "CONFIG_X=V" lines. Defaults files carry only explicit =y/=n lines;
 * comments and blanks are skipped. Later duplicate keys overwrite earlier
 * ones, which is also the semantics ESP-IDF applies across layered defaults
 * files — last definition wins. */
static size_t parse_kv(const char *text, kv_t *out, size_t max)
{
    size_t n = 0;
    const char *p = text;
    while (*p != '\0' && n < max) {
        const char *eol = strchr(p, '\n');
        size_t linelen = eol ? (size_t)(eol - p) : strlen(p);

        if (strncmp(p, "CONFIG_", 7) == 0) {
            const char *eq = memchr(p, '=', linelen);
            if (eq != NULL) {
                size_t klen = (size_t)(eq - p);
                size_t vlen = linelen - klen - 1;
                if (klen > 0 && klen < sizeof(out[0].key) && vlen < sizeof(out[0].val)) {
                    kv_t *slot = &out[n];
                    /* overwrite an earlier definition of the same key */
                    for (size_t i = 0; i < n; i++) {
                        if (strncmp(out[i].key, p, klen) == 0 && out[i].key[klen] == '\0') {
                            slot = &out[i];
                            n--; /* re-fill the same slot */
                            break;
                        }
                    }
                    memcpy(slot->key, p, klen);
                    slot->key[klen] = '\0';
                    memcpy(slot->val, eq + 1, vlen);
                    slot->val[vlen] = '\0';
                    n++;
                }
            }
        }
        p = eol ? eol + 1 : p + linelen;
    }
    return n;
}

/* Value of key after applying defaults then override (later file wins).
 * Returns NULL when neither file defines the key — the caller asserts on the
 * value, so a missing key must fail loudly, not read as "n". */
static const char *effective_value(const kv_t *defs, size_t ndefs,
                                   const kv_t *ovr, size_t novr,
                                   const char *key)
{
    for (size_t i = 0; i < novr; i++) {
        if (strcmp(ovr[i].key, key) == 0) {
            return ovr[i].val;
        }
    }
    for (size_t i = 0; i < ndefs; i++) {
        if (strcmp(defs[i].key, key) == 0) {
            return defs[i].val;
        }
    }
    return NULL;
}

/* ---- Tests ------------------------------------------------------------------ */

/* Default build: USB-Serial/JTAG primary, UART0 explicitly off, no secondary
 * console, panic output prints. These are the lines whose absence made the
 * stock build boot silently — each one is asserted individually so a failure
 * names the exact line that regressed. */
TEST(console_defaults_select_usb_serial_jtag)
{
    ASSERT_TRUE(configs_loaded());

    ASSERT_TRUE(has_exact_line(defaults_text, "CONFIG_ESP_CONSOLE_USB_SERIAL_JTAG=y"));
    ASSERT_TRUE(has_exact_line(defaults_text, "CONFIG_ESP_CONSOLE_UART_DEFAULT=n"));
    ASSERT_TRUE(has_exact_line(defaults_text, "CONFIG_ESP_CONSOLE_SECONDARY_NONE=y"));
    ASSERT_TRUE(has_exact_line(defaults_text, "CONFIG_ESP_SYSTEM_PANIC_PRINT_REBOOT=y"));
}

/* Bridge-board override file: UART0 primary, USB-Serial/JTAG explicitly off.
 * The explicit =n matters — without it the layered build would leave the
 * defaults' =y standing and the console choice would be ambiguous. */
TEST(uart_console_override_selects_uart0)
{
    ASSERT_TRUE(configs_loaded());

    ASSERT_TRUE(has_exact_line(override_text, "CONFIG_ESP_CONSOLE_UART_DEFAULT=y"));
    ASSERT_TRUE(has_exact_line(override_text, "CONFIG_ESP_CONSOLE_USB_SERIAL_JTAG=n"));
    ASSERT_TRUE(has_exact_line(override_text, "CONFIG_ESP_CONSOLE_SECONDARY_NONE=y"));
    ASSERT_TRUE(has_exact_line(override_text, "CONFIG_ESP_SYSTEM_PANIC_PRINT_REBOOT=y"));
}

/* The README's documented invocation layers the override over the defaults:
 *   idf.py -D SDKCONFIG_DEFAULTS="sdkconfig.defaults;sdkconfig.uart-console" ...
 * Applying the files in that order must yield the UART0 profile for every
 * console key. This is the mechanism, not just the file contents — it fails
 * if someone renames a key in one file but not the other. */
TEST(uart_console_override_layers_over_defaults)
{
    ASSERT_TRUE(configs_loaded());

    static kv_t defs[MAX_KV], ovr[MAX_KV];
    size_t ndefs = parse_kv(defaults_text, defs, MAX_KV);
    size_t novr = parse_kv(override_text, ovr, MAX_KV);
    ASSERT_TRUE(ndefs > 0);
    ASSERT_TRUE(novr > 0);

    const struct {
        const char *key;
        const char *want;
    } expected[] = {
        {"CONFIG_ESP_CONSOLE_UART_DEFAULT", "y"},
        {"CONFIG_ESP_CONSOLE_USB_SERIAL_JTAG", "n"},
        {"CONFIG_ESP_CONSOLE_SECONDARY_NONE", "y"},
        {"CONFIG_ESP_SYSTEM_PANIC_PRINT_REBOOT", "y"},
    };
    for (size_t i = 0; i < sizeof(expected) / sizeof(expected[0]); i++) {
        const char *got = effective_value(defs, ndefs, ovr, novr, expected[i].key);
        if (got == NULL) {
            test_record_failure(__FILE__, __LINE__,
                                "layered config is missing key %s",
                                expected[i].key);
            continue;
        }
        if (strcmp(got, expected[i].want) != 0) {
            test_record_failure(__FILE__, __LINE__,
                                "layered %s = %s, want %s",
                                expected[i].key, got, expected[i].want);
        }
    }
}

/* A backtrace is only useful if it reaches the port the operator is watching.
 * Neither profile may ever select a silent or halting panic behaviour. */
TEST(both_console_profiles_keep_panic_output_visible)
{
    ASSERT_TRUE(configs_loaded());

    /* The one required setting, present in both files... */
    ASSERT_TRUE(has_exact_line(defaults_text, "CONFIG_ESP_SYSTEM_PANIC_PRINT_REBOOT=y"));
    ASSERT_TRUE(has_exact_line(override_text, "CONFIG_ESP_SYSTEM_PANIC_PRINT_REBOOT=y"));

    /* ...and none of the settings that would swallow the backtrace. */
    const char *silent[] = {
        "CONFIG_ESP_SYSTEM_PANIC_SILENT_REBOOT=y",
        "CONFIG_ESP_SYSTEM_PANIC_PRINT_HALT=y",
        "CONFIG_ESP_SYSTEM_PANIC_GDBSTUB=y",
    };
    for (size_t i = 0; i < sizeof(silent) / sizeof(silent[0]); i++) {
        ASSERT_FALSE(has_exact_line(defaults_text, silent[i]));
        ASSERT_FALSE(has_exact_line(override_text, silent[i]));
    }
}
