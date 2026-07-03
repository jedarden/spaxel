/*
 * Spaxel firmware host test harness — gcc runner implementation.
 *
 * This file is built up incrementally across the three children of the bf-lfz
 * sub-split (itself a child of bf-2i4; the header API lives in bf-1xs). Only
 * the skeleton is present here:
 *
 *   - child 1 (this change, bf-6aj): the includes and this comment block.
 *   - child 2: the test registry storage (the array + count).
 *   - child 3: test_register() (appends entries in construction order).
 *
 * The failure handler (test_record_failure, per-test longjmp) and main()
 * (sorted iteration, PASS/FAIL reporting, non-zero-on-failure exit) are NOT
 * part of this chain — they arrive in the sibling beads bf-3id and bf-bq9.
 *
 * With only the skeleton present this translation unit compiles cleanly to an
 * object (gcc -std=c11 -Wall -Wextra -c) but is not yet linkable into a
 * runnable harness: test_register() and test_record_failure() are still
 * undefined, and there is no main(). The libc headers below are everything the
 * full runner will need (longjmp, stdio, exit/abort/qsort, string compares) —
 * no includes from firmware/main, by design (see test_runner.h's header comment).
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
