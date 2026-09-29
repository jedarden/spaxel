/*
 * ============================================================================
 *  LED blink lifecycle — host contract test (spaxel-db7aab09, item 3 of 5)
 * ============================================================================
 *
 *  WHY THIS TEST EXISTS
 *  --------------------
 *  firmware/main/led.c drives the node's only physical UI: the mothership's
 *  identify command blinks it, operators find hardware by it, and every exit
 *  path — blink completion, mid-blink stop, disconnect — must leave the pin
 *  LOW. A blink that wedges on, or a stop that leaks a task spinning the
 *  GPIO, is a field-visible defect. None of this needs the radio, so a host
 *  pins it deterministically:
 *
 *    - init configures the module GPIO (default GPIO8) as an output starting
 *      LOW, exactly once — re-init is a no-op;
 *    - blink-before-init refuses (ESP_ERR_INVALID_STATE, no task);
 *    - identify() creates exactly one "led_blink" task at priority 5 carrying
 *      the duration as its argument, and durations above 60 s are capped;
 *    - running the task synchronously emits the real pattern: alternating
 *      100 ms on/off writes, no off-step past the duration, and a final LOW
 *      no matter how the loop ended;
 *    - led_stop_blink() flags the task down (the 50 ms yield), never delays
 *      when nothing is blinking, and still forces the pin LOW either way;
 *    - a failed xTaskCreate reports ESP_ERR_NO_MEM and leaves the module
 *      reporting not-blinking;
 *    - a second identify() cancels the first (stop → fresh task) rather than
 *      stacking tasks.
 *
 *  (The dispatch mentioned "OTA-progress patterns"; led.c has none — blink
 *  identify is its only pattern. The suite tests the real logic. Recorded in
 *  the bead.)
 *
 *  HOW IT COMPILES
 *  ---------------
 *  The production TU is included directly:
 *
 *      #include "../main/led.c"
 *
 *  (watchdog precedent). led.c's IDF includes resolve to recording fakes in
 *  stubs/ — driver/gpio.h logs every (pin, level) write so the emitted
 *  pattern is assertable, stubs/freertos/task.h records xTaskCreate and runs
 *  the task entry point synchronously on the test's thread (the task's
 *  vTaskDelete(NULL) is its last statement, so a recording stub that returns
 *  is enough). Nothing here is reachable from the ESP-IDF build: no CMake
 *  file references firmware/test.
 *  ============================================================================
 */
#include "test_runner.h"

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <string.h>

#include "stubs/driver/gpio.h"
#include "stubs/esp_err.h"
#include "stubs/esp_timer.h"
#include "stubs/freertos/FreeRTOS.h"
#include "stubs/freertos/task.h"
#include "stubs/soc/gpio_num.h"

/* ---- The code under test ---------------------------------------------------- */

#include "../main/led.c"

/*
 * led.c keeps its state in a file-scope static struct, which became part of
 * this translation unit with the include above. Restored — with every fake —
 * directly between tests so each test is order-independent.
 */
static void reset_led_under_test(void)
{
    s_led_state.initialized = false;
    s_led_state.blinking    = false;
    s_led_state.duration_ms = 0;
    s_led_state.task_handle = NULL;

    gpio_mock_reset_all();
    task_mock_reset();
    etimer_mock_reset();
    esp_rom_mock_reset();
}

/* ---- Init ---------------------------------------------------------------------- */

TEST(led_init_configures_gpio8_output_starting_low)
{
    reset_led_under_test();

    ASSERT_EQ(led_init(), ESP_OK);

    ASSERT_EQ(gpio_mock_reset_pin_calls, 1);
    ASSERT_EQ(gpio_mock_reset_pin_last, GPIO_NUM_8);
    ASSERT_EQ(gpio_mock_set_direction_calls, 1);
    ASSERT_EQ(gpio_mock_set_direction_pin, GPIO_NUM_8);
    ASSERT_EQ(gpio_mock_set_direction_mode, GPIO_MODE_OUTPUT);
    /* And it starts OFF. */
    ASSERT_EQ(gpio_mock_level_calls, 1);
    ASSERT_EQ(gpio_mock_level_at(0), 0);
}

TEST(led_init_is_idempotent)
{
    reset_led_under_test();

    ASSERT_EQ(led_init(), ESP_OK);
    int resets = gpio_mock_reset_pin_calls;
    int dirs   = gpio_mock_set_direction_calls;
    int levels = gpio_mock_level_calls;

    ASSERT_EQ(led_init(), ESP_OK);

    ASSERT_EQ(gpio_mock_reset_pin_calls, resets);
    ASSERT_EQ(gpio_mock_set_direction_calls, dirs);
    ASSERT_EQ(gpio_mock_level_calls, levels);
}

/* ---- Refusals --------------------------------------------------------------------- */

TEST(led_blink_before_init_is_rejected)
{
    reset_led_under_test();

    ASSERT_EQ(led_blink_identify(5000), ESP_ERR_INVALID_STATE);
    ASSERT_EQ(task_mock_create_calls, 0);
    ASSERT_FALSE(led_is_blinking());
}

/*
 * stop when nothing is blinking must not delay, but MUST still drive the pin
 * low — the "ensure off" half of the contract is unconditional.
 */
TEST(led_stop_without_blink_still_forces_pin_low)
{
    reset_led_under_test();
    ASSERT_EQ(led_init(), ESP_OK);
    ASSERT_EQ(gpio_mock_level_calls, 1); /* the init LOW */

    led_stop_blink();

    ASSERT_EQ(task_mock_delay_calls, 0);
    ASSERT_EQ(gpio_mock_level_calls, 2);
    ASSERT_EQ(gpio_mock_level_at(-1), 0);
}

/* ---- The blink pattern -------------------------------------------------------------- */

/*
 * Running the recorded task synchronously must produce the physical pattern:
 * on/off pairs of 100 ms each, no off-step beyond the requested duration, and
 * a final LOW. For 600 ms: three on-steps and two off-steps inside the loop,
 * then the guaranteed exit LOW.
 */
TEST(led_blink_task_emits_100ms_alternating_pattern_with_final_low)
{
    reset_led_under_test();
    ASSERT_EQ(led_init(), ESP_OK);

    ASSERT_EQ(led_blink_identify(600), ESP_OK);
    ASSERT_TRUE(led_is_blinking());
    ASSERT_EQ(task_mock_create_calls, 1);

    const task_mock_slot_t *slot = &task_mock_created[0];
    ASSERT_TRUE(strcmp(slot->name, "led_blink") == 0);
    ASSERT_EQ(slot->priority, 5);
    ASSERT_EQ((uintptr_t)slot->arg, (uintptr_t)600);

    task_mock_run_created(0);

    /* Level log: two pre-pattern LOWs (init, then identify's cancel-stop),
     * the blink itself — on, off, on, off, on, off for 600 ms — and the
     * exit LOW. */
    ASSERT_EQ(gpio_mock_level_calls, 9);
    ASSERT_EQ(gpio_mock_level_at(0), 0); /* init LOW */
    ASSERT_EQ(gpio_mock_level_at(1), 0); /* identify's cancel-stop LOW */
    ASSERT_EQ(gpio_mock_level_at(2), 1);
    ASSERT_EQ(gpio_mock_level_at(3), 0);
    ASSERT_EQ(gpio_mock_level_at(4), 1);
    ASSERT_EQ(gpio_mock_level_at(5), 0);
    ASSERT_EQ(gpio_mock_level_at(6), 1);
    ASSERT_EQ(gpio_mock_level_at(7), 0);
    ASSERT_EQ(gpio_mock_level_at(8), 0); /* exit LOW */

    /* All delays are the 100 ms half-period: 3 on + 3 off — the break check
     * runs after an on-delay, so the last off-step's delay still happens
     * before the loop condition ends the run. */
    ASSERT_EQ(task_mock_delay_calls, 6);
    ASSERT_EQ(task_mock_delay_total_ticks, pdMS_TO_TICKS(600));
    for (int i = 0; i < task_mock_delay_ring_len; i++) {
        ASSERT_EQ(task_mock_delay_ring[i], pdMS_TO_TICKS(100));
    }

    /* The task retired cleanly: module reports idle, pin left LOW. */
    ASSERT_FALSE(led_is_blinking());
    ASSERT_TRUE(s_led_state.task_handle == NULL);
    ASSERT_EQ(task_mock_delete_calls, 1);
}

/*
 * The 60 s cap: a mothership identify asking for an hour must get a blink
 * that ends after 60 s — the recorded task argument carries the capped
 * value, and the run shows the full 60 s of half-periods then stops.
 */
TEST(led_blink_duration_is_capped_at_60s)
{
    reset_led_under_test();
    ASSERT_EQ(led_init(), ESP_OK);

    ASSERT_EQ(led_blink_identify(999999), ESP_OK);

    ASSERT_EQ((uintptr_t)task_mock_created[0].arg, (uintptr_t)60000);

    task_mock_run_created(0);

    /* 60000 ms / 100 ms = 600 half-period delays; the loop emits exactly
     * 600 level writes (300 on/off pairs) plus the exit LOW, after the two
     * pre-pattern LOWs from init and identify's cancel-stop. */
    ASSERT_EQ(task_mock_delay_calls, 600);
    ASSERT_EQ(task_mock_delay_total_ticks, pdMS_TO_TICKS(60000));
    ASSERT_FALSE(gpio_mock_level_dropped);
    ASSERT_EQ(gpio_mock_level_calls, 603);
    ASSERT_EQ(gpio_mock_level_at(0), 0);
    ASSERT_EQ(gpio_mock_level_at(-1), 0);
    ASSERT_FALSE(led_is_blinking());
}

/*
 * A stop issued before the task has run (the host model of "stopped within
 * the 50 ms yield") leaves the task nothing to do: it must exit without a
 * single on-step — never a blink that outlives its cancellation.
 */
TEST(led_stop_before_task_run_means_task_exits_without_blinking)
{
    reset_led_under_test();
    ASSERT_EQ(led_init(), ESP_OK);
    ASSERT_EQ(led_blink_identify(60000), ESP_OK);

    led_stop_blink();
    ASSERT_FALSE(led_is_blinking());
    /* The stop's own 50 ms yield is the only delay so far. */
    ASSERT_EQ(task_mock_delay_calls, 1);
    ASSERT_EQ(task_mock_delay_ring[0], pdMS_TO_TICKS(50));

    task_mock_run_created(0);

    /* Loop never ran (blinking was false): only the exit LOW joins the two
     * earlier LOWs (init, identify's cancel-stop). */
    ASSERT_EQ(gpio_mock_level_calls, 4);
    ASSERT_EQ(gpio_mock_level_at(0), 0);
    ASSERT_EQ(gpio_mock_level_at(1), 0);
    ASSERT_EQ(gpio_mock_level_at(2), 0); /* stop LOW */
    ASSERT_EQ(gpio_mock_level_at(3), 0); /* exit LOW */
    ASSERT_EQ(task_mock_delay_calls, 1); /* no further delays */
    ASSERT_EQ(task_mock_delete_calls, 1);
}

/* ---- Restart / failure paths ------------------------------------------------------------ */

/*
 * A second identify cancels the first: the stop runs (yield + pin LOW), a
 * NEW task is created with the new duration, and the module reports
 * blinking again only for the new task.
 */
TEST(led_second_identify_cancels_first_and_starts_fresh)
{
    reset_led_under_test();
    ASSERT_EQ(led_init(), ESP_OK);

    ASSERT_EQ(led_blink_identify(60000), ESP_OK);
    ASSERT_EQ(led_blink_identify(100), ESP_OK);

    ASSERT_EQ(task_mock_create_calls, 2);
    ASSERT_EQ((uintptr_t)task_mock_created[0].arg, (uintptr_t)60000);
    ASSERT_EQ((uintptr_t)task_mock_created[1].arg, (uintptr_t)100);
    ASSERT_TRUE(led_is_blinking());

    /* Run the NEW task (the one the handle points at): a single 100 ms
     * cycle — on-step, break at the duration, exit LOW. */
    task_mock_run_created(1);

    /* Level log: init LOW, identify-1's cancel-stop LOW, identify-2's
     * cancel-stop LOW, then the new blink's on and exit LOW. */
    ASSERT_EQ(gpio_mock_level_calls, 5);
    ASSERT_EQ(gpio_mock_level_at(0), 0);
    ASSERT_EQ(gpio_mock_level_at(1), 0);
    ASSERT_EQ(gpio_mock_level_at(2), 0);
    ASSERT_EQ(gpio_mock_level_at(3), 1);
    ASSERT_EQ(gpio_mock_level_at(4), 0);

    ASSERT_EQ(task_mock_delay_calls, 2); /* cancel yield 50 + blink 100 */
    ASSERT_EQ(task_mock_delay_ring[0], pdMS_TO_TICKS(50));
    ASSERT_EQ(task_mock_delay_ring[1], pdMS_TO_TICKS(100));
    ASSERT_FALSE(led_is_blinking());
    ASSERT_EQ(task_mock_delete_calls, 1);
}

TEST(led_task_creation_failure_reports_no_mem_and_stays_idle)
{
    reset_led_under_test();
    ASSERT_EQ(led_init(), ESP_OK);

    task_mock_create_fail = pdTRUE;
    ASSERT_EQ(led_blink_identify(5000), ESP_ERR_NO_MEM);

    /* The module must not report a blink it could not start. */
    ASSERT_FALSE(led_is_blinking());
    ASSERT_EQ(task_mock_create_calls, 0);
    ASSERT_TRUE(s_led_state.task_handle == NULL);

    /* And a retry once creation works really starts. */
    task_mock_create_fail = pdFALSE;
    ASSERT_EQ(led_blink_identify(5000), ESP_OK);
    ASSERT_TRUE(led_is_blinking());
}
