/*
 * Host stand-in for ESP-IDF's nvs.h — declarations only.
 *
 * Signatures mirror the ESP-IDF API subset firmware/main/nvs_migration.c uses,
 * so that file compiles unmodified. The in-memory implementation backing them
 * lives in the test that needs it (firmware/test/test_nvs_migration.c), which
 * is what links against the production object; the stub headers stay free of
 * behavior so this file remains a faithful API mirror.
 */
#pragma once

#include <stddef.h>
#include <stdint.h>

#include "esp_err.h"

typedef uint32_t nvs_handle_t;

typedef enum {
    NVS_READONLY = 0,
    NVS_READWRITE = 2,
} nvs_open_mode_t;

esp_err_t nvs_open(const char *ns, nvs_open_mode_t open_mode,
                   nvs_handle_t *out_handle);
void nvs_close(nvs_handle_t handle);

esp_err_t nvs_get_u8(nvs_handle_t handle, const char *key, uint8_t *out_value);
esp_err_t nvs_set_u8(nvs_handle_t handle, const char *key, uint8_t value);

esp_err_t nvs_get_str(nvs_handle_t handle, const char *key, char *out_value,
                      size_t *length);
esp_err_t nvs_set_str(nvs_handle_t handle, const char *key, const char *value);

esp_err_t nvs_erase_key(nvs_handle_t handle, const char *key);
esp_err_t nvs_commit(nvs_handle_t handle);
