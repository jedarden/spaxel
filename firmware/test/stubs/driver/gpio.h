/*
 * Host stand-in for ESP-IDF's driver/gpio.h — the recording fake slice
 * firmware/main/led.c reaches (spaxel-db7aab09, esp_task_wdt.h precedent).
 *
 * The LED is the node's only physical UI: identify blinks, disconnects stop
 * blinking, and every exit path must end with the pin low. To make the
 * emitted LEVEL SEQUENCE assertable the fake logs every (pin, level) pair in
 * order — the blink tests read that log back as the pattern the LED would
 * have shown. Static state, single-TU inclusion (led.c is compiled only into
 * the test TU that includes this header).
 */
#ifndef SPAXEL_TEST_STUB_DRIVER_GPIO_H
#define SPAXEL_TEST_STUB_DRIVER_GPIO_H

#include <stdbool.h>
#include <stddef.h>
#include <string.h>

#include "esp_err.h"
#include "soc/gpio_num.h"

typedef enum {
    GPIO_MODE_DISABLE = 0,
    GPIO_MODE_INPUT,
    GPIO_MODE_OUTPUT,
} gpio_mode_t;

/* ---- Recorded calls -------------------------------------------------------- */

#define GPIO_MOCK_LEVEL_LOG 2048

typedef struct {
    gpio_num_t pin;
    int        level;
} gpio_mock_level_t;

__attribute__((unused))
static int              gpio_mock_reset_pin_calls;
__attribute__((unused))
static gpio_num_t       gpio_mock_reset_pin_last;
__attribute__((unused))
static int              gpio_mock_set_direction_calls;
__attribute__((unused))
static gpio_num_t       gpio_mock_set_direction_pin;
__attribute__((unused))
static gpio_mode_t      gpio_mock_set_direction_mode;
__attribute__((unused))
static gpio_mock_level_t gpio_mock_levels[GPIO_MOCK_LEVEL_LOG];
__attribute__((unused))
static int              gpio_mock_level_calls;
__attribute__((unused))
static bool             gpio_mock_level_dropped; /* log overflowed */

/* ---- Reset + read helpers ------------------------------------------------------ */

__attribute__((unused))
static void gpio_mock_reset_all(void)
{
    gpio_mock_reset_pin_calls    = 0;
    gpio_mock_reset_pin_last     = GPIO_NUM_NC;
    gpio_mock_set_direction_calls = 0;
    gpio_mock_set_direction_pin   = GPIO_NUM_NC;
    gpio_mock_set_direction_mode  = GPIO_MODE_DISABLE;
    memset(gpio_mock_levels, 0, sizeof(gpio_mock_levels));
    gpio_mock_level_calls  = 0;
    gpio_mock_level_dropped = false;
}

/* The level written count times — negative index counts back from the end. */
__attribute__((unused))
static int gpio_mock_level_at(int index)
{
    if (index < 0) {
        index += gpio_mock_level_calls;
    }
    if (index < 0 || index >= gpio_mock_level_calls) {
        return -1;
    }
    return gpio_mock_levels[index].level;
}

/* ---- The mocked API --------------------------------------------------------------- */

__attribute__((unused))
static esp_err_t gpio_reset_pin(gpio_num_t gpio_num)
{
    gpio_mock_reset_pin_calls++;
    gpio_mock_reset_pin_last = gpio_num;
    return ESP_OK;
}

__attribute__((unused))
static esp_err_t gpio_set_direction(gpio_num_t gpio_num, gpio_mode_t mode)
{
    gpio_mock_set_direction_pin  = gpio_num;
    gpio_mock_set_direction_mode = mode;
    gpio_mock_set_direction_calls++;
    return ESP_OK;
}

__attribute__((unused))
static esp_err_t gpio_set_level(gpio_num_t gpio_num, uint32_t level)
{
    if (gpio_mock_level_calls < GPIO_MOCK_LEVEL_LOG) {
        gpio_mock_levels[gpio_mock_level_calls].pin   = gpio_num;
        gpio_mock_levels[gpio_mock_level_calls].level = (int)level;
    } else {
        gpio_mock_level_dropped = true;
    }
    gpio_mock_level_calls++;
    return ESP_OK;
}

#endif /* SPAXEL_TEST_STUB_DRIVER_GPIO_H */
