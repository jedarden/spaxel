/*
 * Host stand-in for ESP-IDF's nvs_flash.h.
 *
 * firmware/main/nvs_migration.c includes this header but calls nothing from
 * it. nvs_flash_init() itself runs in firmware/main/main.c, which the host
 * harness deliberately does not build (see test_runner.h for why the whole
 * firmware/main component cannot link on a host); the in-memory store that
 * stands in for the partition is owned by the test.
 */
#pragma once

#include "esp_err.h"
