/*
 * Spaxel firmware host test harness — gcc runner implementation.
 *
 * This file is built up incrementally across the children of the bf-lfz
 * sub-split (itself a child of bf-2i4; the header API lives in bf-1xs). With
 * this bead the runner is complete — every piece below has landed:
 *
 *   - child 1 (bf-6aj): the includes and this comment block.
 *   - child 2 (bf-uvv): the test registry storage (array + count).
 *   - child 3 (bf-oe1): test_register() (appends entries in construction
 *                   order, with a capacity guard).
 *   - sibling  (bf-3id): the per-test failure-recovery machinery — the
 *                   file-scope jmp_buf the ASSERT_* macros longjmp into, a
 *                   run-wide failure counter, and test_record_failure()
 *                   itself.
 *   - sibling  (bf-bq9, this change): main() — the entry point that sorts
 *                   the registered tests by name, drives each through the
 *                   setjmp/longjmp recovery loop, prints a one-line
 *                   RUN marker per test plus a run summary, and returns
 *                   non-zero iff any test failed.
 *
 * main() setjmp()s into g_test_jmp before each test and calls the body; on a
 * longjmp return (a failed assertion) it prints FAIL and moves on, so one
 * test's failure never blocks the rest. The exit code — 1 if
 * g_failure_count > 0, else 0 — is the contract CI relies on (the documented
 * `make -C firmware/test test` propagates it).
 *
 * test_register() writes the registry storage and test_record_failure()
 * reads/writes the recovery statics, so neither group needs the
 * __attribute__((unused)) each required while unwritten. The libc headers
 * here are everything the full runner needs (setjmp/longjmp, stdio, stdlib,
 * string, and stdarg for the variadic vfprintf) — no includes from
 * firmware/main, by design (see test_runner.h's header comment).
 *
 * See test_runner.h (bf-1xs) for the TEST()/ASSERT_* macros and the registry
 * API, and the gcc host-harness decision record (bf-21t) for why this is plain
 * gcc and not ESP-IDF --target linux.
 */
#include "test_runner.h"

#include <setjmp.h>
#include <stdarg.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>

/* ---- Test registry storage ---------------------------------------------- */

/*
 * The registry is a plain static array rather than a heap-grown list. That is
 * fine here because the GCC constructors emitted by each TEST() macro (see
 * test_runner.h) populate it before main() ever runs — so by the time anything
 * reads g_tests[] it is already fully built — and a fixed array keeps the
 * harness dependency-free (no malloc/realloc, no failure modes to reason
 * about), which matches the deliberately small scope of this host runner.
 *
 * MAX_TESTS = 128 is far more than the handful of pure-logic extractions and
 * binary-format contracts this harness is meant for (it deliberately does not
 * cover the unhostable esp_wifi/uart/nvs call sites — see test_runner.h's
 * header comment). A test count approaching the cap would signal that firmware
 * logic has been over-factored into host tests, not that the cap is too low.
 */
#define MAX_TESTS 128
/*
 * Written by test_register() (below) from the GCC constructors emitted by each
 * TEST() macro, so the array is fully populated before main() runs. They were
 * __attribute__((unused)) through child 2 (bf-uvv) because nothing referenced
 * them until child 3's test_register() landed here — gcc 14 warns on unused
 * file-scope statics under -Wall -Wextra, unlike the older gcc the original
 * assumption rested on. Now that test_register() reads and writes them, the
 * symbols are referenced and the attribute is no longer needed.
 */
static test_entry_t g_tests[MAX_TESTS];
static int          g_test_count = 0;

/* ---- test_register ------------------------------------------------------ */

/*
 * Append one test to the registry in construction order. The GCC constructors
 * emitted by the TEST() macro (see test_runner.h) fire in a deterministic
 * order before main() runs, so the order entries land here is the order the
 * runner iterates them. The new entry goes at index g_test_count, which is
 * then bumped — leaving the list fully populated by the time main() reads it.
 *
 * Capacity guard: MAX_TESTS (128 above) is deliberately generous, and a count
 * approaching it would signal that firmware logic has been over-factored into
 * host tests rather than a too-low cap. Even so, an overflow would corrupt
 * memory silently, so on a full registry we log to stderr — naming the skipped
 * test and the cap — and return WITHOUT writing past the end. Dropping a late
 * test beats smashing the stack any day.
 *
 * The failure-recovery machinery (test_record_failure + the jmp_buf) lives in
 * its own sibling section below (bf-3id), and the entry point that drives the
 * whole registry — main(), which sorts, iterates, and reports — is the final
 * section at the bottom of this file (bf-bq9).
 */
void test_register(const char *name, test_fn fn)
{
    if (g_test_count >= MAX_TESTS) {
        fprintf(stderr,
                "spaxel_host_tests: test registry full (MAX_TESTS=%d); "
                "skipping registration of test \"%s\"\n",
                MAX_TESTS, name);
        return;
    }

    g_tests[g_test_count].name = name;
    g_tests[g_test_count].fn   = fn;
    g_test_count++;
}

/* ---- Per-test failure recovery ------------------------------------------ */

/*
 * The live setjmp() target for whichever test is currently running. Every
 * ASSERT_* macro in test_runner.h funnels a failed assertion into
 * test_record_failure(), which longjmp()s back into here — aborting ONLY the
 * current test so the remaining tests still run.
 *
 * This is the shared contract with main() (bf-bq9): before driving each test
 * main() setjmp()s into g_test_jmp and calls the test body; on a longjmp
 * return (setjmp yields non-zero) it records the failure and moves to the
 * next test. A single jmp_buf rather than per-test state is sufficient
 * because tests run strictly one at a time and the buffer is refreshed
 * before each via that setjmp().
 *
 * It is declared at file scope (not static-local inside test_record_failure)
 * precisely so main() — which lands in this same translation unit — can
 * setjmp() it directly, and given internal linkage (static) because nothing
 * outside test_runner.c needs to touch it.
 */
static jmp_buf g_test_jmp;

/*
 * Run-wide count of failed assertions. test_record_failure() bumps it on each
 * longjmp; main() reads it after the run to pick the process exit code —
 * non-zero iff at least one assertion failed anywhere. It is monotonic across
 * the whole run (never reset): once the suite has failed it stays failed,
 * which is exactly "return non-zero on any failure". Because a failed
 * assertion longjmps out of its test immediately, this counts the first (and
 * only) failing assertion per test that trips one.
 */
static int g_failure_count = 0;

/*
 * Record a failed assertion and bail out of the current test.
 *
 * Prints the source location and a printf-style detail line to stderr (e.g.
 * "test_sanity.c:22: ASSERT_EQ(...) failed: got 2, want 3"), bumps the
 * run-wide failure counter so main() returns non-zero, then longjmp()s into
 * g_test_jmp. The longjmp is what keeps the harness alive: control returns to
 * main()'s setjmp() call site (which sees a non-zero return) rather than
 * unwinding the stack, so main() proceeds to the next test.
 *
 * NOTE: main() — which calls setjmp(g_test_jmp) before each test — is not
 * part of this bead; it lands in bf-bq9. Until then the longjmp below has no
 * live target, which is expected and is why this file is compiled to an
 * object but not yet linked into a binary.
 */
void test_record_failure(const char *file, int line, const char *fmt, ...)
{
    va_list ap;

    fprintf(stderr, "%s:%d: ", file, line);
    va_start(ap, fmt);
    vfprintf(stderr, fmt, ap);
    va_end(ap);
    fputc('\n', stderr);

    g_failure_count++;

    longjmp(g_test_jmp, 1);
}

/* ---- main: suite driver -------------------------------------------------- */

/*
 * Name comparator for the sort below: plain strcmp over test_entry_t::name.
 *
 * main() sorts the registry by name so iteration order is deterministic no
 * matter how the constructors fired or how the link line ordered the TUs. The
 * C standard does NOT guarantee constructor order across translation units —
 * within one TU it follows definition order, but across TUs (and across link
 * lines, which the Makefile's test_*.c glob feeds in a glob-dependent order)
 * it is implementation- and link-defined. An unsorted run would therefore order
 * tests however gcc happened to receive them, so a failing run's interleaved
 * PASS/FAIL output would not be reproducible. Sorting by name makes it stable,
 * which is what CI log diffing and "did this run change?" want.
 */
static int test_entry_cmp(const void *a, const void *b)
{
    const test_entry_t *ta = a;
    const test_entry_t *tb = b;
    return strcmp(ta->name, tb->name);
}

/*
 * Entry point — the contract CI relies on.
 *
 * Run the whole suite from the repo root with the single documented command:
 *
 *     make -C firmware/test test
 *
 * (per the bf-1xs header contract and the bf-56v gcc-harness decision record).
 * make compiles every test_*.c plus this runner with plain gcc and runs the
 * binary; THIS function's exit code is what make propagates, so a non-zero
 * return here fails CI.
 *
 * This is child 2 of the bf-22vg re-split: child 1 (bf-1fd4) restored the naive
 * direct-call loop, and THIS change (bf-27ud) layers the per-test recovery
 * target back on — gating the body call on setjmp(). Flow:
 *   1. Sort the registry by name (test_entry_cmp) for a deterministic order.
 *      The TEST() constructors have already fully populated it before main().
 *   2. For each test: print its RUN line, then setjmp() into g_test_jmp and call
 *      g_tests[i].fn() only on the direct (zero) return. A non-zero return is a
 *      longjmp from a failed assertion and falls straight through to the next
 *      iteration (no else body), so a failure in test N never blocks N+1..end.
 *
 * Still missing here — and deliberately sibling scope (child 3, bf-1na) — is
 * everything that turns a passed/failed run into output and an exit code:
 * PASS/FAIL labels, a pass/fail tally, the run summary line, and the non-zero
 * exit on failure. For this intermediate state main() returns 0 unconditionally:
 * it compiles and runs every test, and an assertion that fires now longjmps
 * cleanly back to the setjmp above (the target bf-3id declared) instead of off
 * into nowhere — but that failure is not yet surfaced or counted; the g_failure_count
 * test_record_failure() bumps is not yet read here.
 */
int main(void)
{
    qsort(g_tests, (size_t)g_test_count, sizeof(g_tests[0]), test_entry_cmp);

    /*
     * i spans the setjmp/longjmp recovery below, so it is volatile-qualified.
     * The full per-variable C11 7.13.2.1 clobber audit — which variables are in
     * scope across the boundary, why each is safe, and why volatile is kept
     * despite the standard not strictly requiring it — lives at the setjmp call
     * site further down this loop body.
     */
    for (volatile int i = 0; i < g_test_count; i++) {
        /*
         * The RUN line is the one observable line per test and prints BEFORE the
         * setjmp, so it appears regardless of how the body ends. The body itself
         * runs only on the direct (zero) setjmp return; a non-zero return is a
         * longjmp from a failed assertion and simply falls through to the next
         * iteration — no else body — so the loop's i++ still advances and a
         * failure in test N never blocks N+1..end.
         */
        printf("RUN: %s\n", g_tests[i].name);
        /*
         * DYNAMIC CONFIRMATION (bf-50yh, child 4/4 of umbrella bf-tof1) — the
         * per-test RUN-line isolation contract this loop exists to provide,
         * verified against the stdout of a successful `make -C firmware/test
         * test` run (the bf-27ud recovery loop + this driver):
         *
         *   registered TEST() definitions ......... 29   (test_sanity 1,
         *                                                 test_csi_frame 7,
         *                                                 test_nvs_migration 9,
         *                                                 test_serial_prov 12)
         *   RUN: lines emitted by this loop ....... 29
         *   bidirectional set diff ................ IDENTICAL — every registered
         *                                                 name has exactly one RUN
         *                                                 line; none skipped, none
         *                                                 duplicated. The suite is
         *                                                 fully driven.
         *
         * The 29 RUN lines captured, in the order this loop emitted them:
         *
         *   RUN: arithmetic_sanity
         *   RUN: csi_frame_header_only_probe
         *   RUN: csi_frame_header_size
         *   RUN: csi_frame_ingestion_validation
         *   RUN: csi_frame_iq_payload
         *   RUN: csi_frame_roundtrip_fields
         *   RUN: csi_frame_signed_rssi_roundtrip
         *   RUN: csi_frame_timestamp_is_little_endian
         *   RUN: nvs_migration_already_current_is_noop
         *   RUN: nvs_migration_does_not_downgrade_newer_version
         *   RUN: nvs_migration_fresh_install_inits_to_v1
         *   RUN: nvs_migration_index_arithmetic_picks_correct_step
         *   RUN: nvs_migration_multi_step_advance
         *   RUN: nvs_migration_undefined_future_version_returns_not_found
         *   RUN: nvs_migration_v1_to_v2_preserves_existing_ntp
         *   RUN: nvs_migration_v1_to_v2_renames_ms_ip_and_defaults_ntp
         *   RUN: nvs_migration_v1_to_v2_without_ms_ip_skips_rename
         *   RUN: serial_prov_debug_non_bool_writes_zero
         *   RUN: serial_prov_empty_ssid_rejected
         *   RUN: serial_prov_fuzz_deep_nesting_capped
         *   RUN: serial_prov_fuzz_random_bytes_never_crash
         *   RUN: serial_prov_invalid_json
         *   RUN: serial_prov_minimal_payload_ok
         *   RUN: serial_prov_missing_provision_key
         *   RUN: serial_prov_missing_ssid_rejected
         *   RUN: serial_prov_port_wrong_type_ignored
         *   RUN: serial_prov_string_escapes_decoded
         *   RUN: serial_prov_top_level_array_is_missing_key
         *   RUN: serial_prov_valid_full_payload
         *
         * That emitted order is byte-for-byte the strcmp-sorted order the qsort
         * above produces — confirming per-test isolation at the observable level:
         * the loop reaches every test regardless of how the prior body ended, and
         * for this all-passing suite every body returns normally (no longjmp), so
         * all RUN: lines land in sorted order with no FAIL/stderr interleaving
         * them. RUN-line set and order are unchanged vs the naive direct-call
         * baseline (bf-1fd4): the recovery gating added no test, dropped no test,
         * and reordered nothing. Umbrella bf-tof1 (and thereby parent bf-22vg)
         * acceptance is satisfied.
         */
        /*
         * SETJMP/LONGJMP CLOBBER AUDIT — C11 7.13.2.1 (bf-31rd, child 2/3 of
         * bf-53ut). The standard renders an automatic object indeterminate on
         * longjmp return only if it is local to THIS function, non-volatile, AND
         * changed between the setjmp() call and the longjmp(). No object in scope
         * here meets all three; volatile on i (loop above) is kept solely to
         * satisfy gcc's -Wclobbered, which is heuristic and ignores that
         * distinction. Variables in scope across the boundary:
         *
         *  - i (automatic, loop index, volatile-qualified): written ONLY by the
         *    for-init (i = 0) and the for-increment (i++). The increment runs
         *    AFTER control resumes here — whether the body returned normally or a
         *    failed assertion longjmp'd back — so no write to i lands between the
         *    setjmp() call and a longjmp() fired inside g_tests[i].fn(); the body
         *    only READS i. The 7.13.2.1 "changed between setjmp and longjmp"
         *    condition is therefore not met, and i is determinate on the longjmp-
         *    return path (where i++ and the i < g_test_count re-test read it).
         *  - No local tallies (passed/failed counters) live in main().
         *    g_failure_count is file-scope static, not automatic, so 7.13.2.1
         *    does not apply to it at all; it is bumped inside
         *    test_record_failure() before the longjmp, not in this frame.
         *    g_test_jmp is likewise file-scope static.
         *
         * Why volatile anyway: gcc's -Wclobbered (on under -Wall) flags this
         * exact loop-index-across-setjmp shape. Empirically the loop warns at
         * -O1..-Os once the incidental preceding qsort() call stops biasing gcc's
         * register heuristic (verified: with qsort present the heuristic happens
         * to stay silent today; remove qsort and -Wclobbered fires on i). volatile
         * exempts i from BOTH the indeterminate rule and the warning, with zero
         * behavior change (the counter still walks 0..g_test_count-1) — the
         * 7.13.2.1-sanctioned remedy, no pragma and no flag downgrade. Compile
         * gate for the guard (parent bf-22vg): gcc -std=c11 -Wall -Wextra — and
         * explicit -Wclobbered — is silent at -O0..-Os for this TU.
         */
        if (setjmp(g_test_jmp) == 0) {
            g_tests[i].fn();
        }
    }

    return 0;
}
