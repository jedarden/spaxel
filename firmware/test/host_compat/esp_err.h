/*
 * Host stand-in for ESP-IDF's esp_err.h.
 *
 * Only what firmware/main/nvs_migration.c needs to compile on a host:
 * esp_err_t, the codes it compares against, and esp_err_to_name(). The NVS
 * codes follow ESP-IDF's ESP_ERR_NVS_BASE (0x1100) layout so the comparisons
 * behave identically; no real esp_err.h is included, so nothing else from IDF
 * leaks into the host build.
 */
#pragma once

typedef int esp_err_t;

#define ESP_OK                  0
#define ESP_ERR_NO_MEM        0x101
#define ESP_ERR_INVALID_ARG   0x102
#define ESP_ERR_INVALID_STATE 0x103
#define ESP_ERR_INVALID_SIZE  0x104
#define ESP_ERR_NOT_FOUND     0x105

#define ESP_ERR_NVS_BASE              0x1100
#define ESP_ERR_NVS_NOT_FOUND         (ESP_ERR_NVS_BASE + 0x02)
#define ESP_ERR_NVS_NEW_VERSION_FOUND (ESP_ERR_NVS_BASE + 0x03)
#define ESP_ERR_NVS_NO_FREE_PAGES     (ESP_ERR_NVS_BASE + 0x04)
#define ESP_ERR_NVS_INVALID_LENGTH    (ESP_ERR_NVS_BASE + 0x06)
#define ESP_ERR_NVS_TYPE_MISMATCH     (ESP_ERR_NVS_BASE + 0x07)
#define ESP_ERR_NVS_READ_ONLY         (ESP_ERR_NVS_BASE + 0x08)

const char *esp_err_to_name(esp_err_t code);
