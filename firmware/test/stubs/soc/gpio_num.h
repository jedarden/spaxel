/*
 * Host stand-in for ESP-IDF's soc/gpio_num.h — the GPIO number enum slice
 * firmware/main/led.c names (spaxel-db7aab09). GPIO_NUM_8 is the DevKitC
 * onboard LED the module defaults to via CONFIG_SPAXEL_LED_GPIO; the full
 * chip enum is irrelevant to a host, so only the named constants used by the
 * tests exist.
 */
#ifndef SPAXEL_TEST_STUB_SOC_GPIO_NUM_H
#define SPAXEL_TEST_STUB_SOC_GPIO_NUM_H

typedef int gpio_num_t;

#define GPIO_NUM_0  ((gpio_num_t)0)
#define GPIO_NUM_8  ((gpio_num_t)8)
#define GPIO_NUM_48 ((gpio_num_t)48)

#define GPIO_NUM_NC ((gpio_num_t)-1)

#endif /* SPAXEL_TEST_STUB_SOC_GPIO_NUM_H */
