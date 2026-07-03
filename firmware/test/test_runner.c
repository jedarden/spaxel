/*
 * Spaxel firmware host test harness — gcc runner implementation.
 *
 * This file is built up incrementally across the three children of the bf-lfz
 * sub-split (itself a child of bf-2i4; the header API lives in bf-1xs). The
 * skeleton plus the registry storage are present here:
 *
 *   - child 1 (bf-6aj): the includes and this comment block.
 *   - child 2 (this change, bf-uvv): the test registry storage (array + count).
 *   - child 3: test_register() (appends entries in construction order).
 *
 * The failure handler (test_record_failure, per-test longjmp) and main()
 * (sorted iteration, PASS/FAIL reporting, non-zero-on-failure exit) are NOT
 * part of this chain — they arrive in the sibling beads bf-3id and bf-bq9.
 *
 * With child 2 landed this translation unit still compiles cleanly to an
 * object (gcc -std=c11 -Wall -Wextra -c) but is not yet linkable into a
 * runnable harness: test_register() and test_record_failure() are still
 * undefined, and there is no main(). The registry storage is static
 * file-scope, so the symbols are defined but not yet read or written — gcc
 * does not warn on unused file-scope statics, so the object stays clean. The
 * libc headers below are everything the full runner will need (longjmp,
 * stdio, exit/abort/qsort, string compares) — no includes from firmware/main,
 * by design (see test_runner.h's header comment).
 *
 * See test_runner.h (bf-1xs) for the TEST()/ASSERT_* macros and the registry
 * API, and the gcc host-harness decision record (bf-21t) for why this is plain
 * gcc and not ESP-IDF --target linux.
 */
#include "test_runner.h"

#include <setjmp.h>
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
/* Marked __attribute__((unused)): nothing reads or writes the registry until
 * child 3's test_register() lands. gcc 14 (unlike the older gcc the original
 * "statics don't warn when unused" assumption rested on) DOES warn on unused
 * file-scope statics under -Wall -Wextra; without this the object would emit
 * two -Wunused-variable warnings and trip the zero-warning build gate. The
 * attribute is harmless once the symbols are referenced. */
static test_entry_t g_tests[MAX_TESTS] __attribute__((unused));
static int          g_test_count __attribute__((unused)) = 0;
