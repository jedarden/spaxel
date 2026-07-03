/*
 * Spaxel firmware host test harness — gcc runner implementation.
 *
 * This file is built up incrementally across the three children of the bf-lfz
 * sub-split (itself a child of bf-2i4; the header API lives in bf-1xs). All
 * three children are now present:
 *
 *   - child 1 (bf-6aj): the includes and this comment block.
 *   - child 2 (bf-uvv): the test registry storage (array + count).
 *   - child 3 (this change, bf-oe1): test_register() (appends entries in
 *                   construction order, with a capacity guard).
 *
 * The failure handler (test_record_failure, per-test longjmp) and main()
 * (sorted iteration, PASS/FAIL reporting, non-zero-on-failure exit) are NOT
 * part of this chain — they arrive in the sibling beads bf-3id and bf-bq9,
 * and are intentionally absent here.
 *
 * With child 3 landed this translation unit compiles cleanly to an object
 * (gcc -std=c11 -Wall -Wextra -c) but is still not linkable into a runnable
 * harness: test_record_failure() remains undefined and there is no main().
 * test_register() now writes the registry, so the static storage below is
 * referenced (read/written) and no longer carries the __attribute__((unused))
 * it needed while unwritten. The libc headers here are everything the full
 * runner will need (longjmp, stdio, exit/abort/qsort, string compares) — no
 * includes from firmware/main, by design (see test_runner.h's header comment).
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
 * NOTE: main() (sorted iteration, PASS/FAIL reporting, non-zero-on-failure
 * exit) and test_record_failure() (the per-test longjmp failure handler)
 * arrive in the sibling beads bf-bq9 and bf-3id respectively. They are
 * intentionally absent here: this chain carries only the registry itself.
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
