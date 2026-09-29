/*
 * Host stand-in for ESP-IDF's esp_rom_sys.h (spaxel-db7aab09).
 *
 * firmware/main/led.c includes this header for esp_rom_delay_us(); the
 * include is part of the TU's surface, so it must resolve on the host include
 * path. The busy-wait itself is meaningless without hardware, but recording
 * the call keeps the fake honest if a future call site appears.
 */
#ifndef SPAXEL_TEST_STUB_ESP_ROM_SYS_H
#define SPAXEL_TEST_STUB_ESP_ROM_SYS_H

#include <stdint.h>

__attribute__((unused))
static int esp_rom_delay_us_calls;
__attribute__((unused))
static uint32_t esp_rom_delay_us_last;

__attribute__((unused))
static void esp_rom_delay_us(uint32_t us)
{
    esp_rom_delay_us_calls++;
    esp_rom_delay_us_last = us;
}

__attribute__((unused))
static void esp_rom_mock_reset(void)
{
    esp_rom_delay_us_calls = 0;
    esp_rom_delay_us_last  = 0;
}

#endif /* SPAXEL_TEST_STUB_ESP_ROM_SYS_H */
