#pragma once

#include "esp_err.h"
#include <stdint.h>

// Current compiled NVS schema version
// Increment this when adding new migrations
#define COMPILED_NVS_VERSION 1

// Run NVS schema migration on boot
// Opens 'spaxel' NVS namespace and reads schema_ver.
// If missing, initializes schema_ver to 1.
// If schema_ver < COMPILED_NVS_VERSION, runs migrations in order.
// Returns ESP_OK on success, or error code on failure.
esp_err_t nvs_migration_run(void);
