/*
 * Sanity test for the Spaxel firmware host test harness.
 *
 * This is the minimal passing test that proves the harness wires up end to end:
 * the TEST() macro self-registers via its GCC constructor, the runner drives the
 * registered test, and ASSERT_EQ reports correctly.
 *
 * Real module tests (nvs/csi/prov logic extractions + binary-format contracts)
 * are deliberately NOT here — they are added by the sibling beads that follow.
 * Keeping this bead (bf-56v) to scaffolding + a single sanity test is what lets
 * the "compiles + runs + exits non-zero on failure" contract be verified in
 * isolation before any module behavior is exercised.
 */
#include "test_runner.h"

/*
 * 1 + 1 == 2: the smallest possible assertion. If this fails, the harness itself
 * is broken, not the firmware logic under test.
 */
TEST(arithmetic_sanity)
{
    ASSERT_EQ(1 + 1, 2);
}
