#pragma once

#include "esp_err.h"
#include <stdint.h>

// Current compiled NVS schema version
// Increment this when adding new migrations
//
// Guarded so the host test harness can raise it with -DCOMPILED_NVS_VERSION=N
// and link a second copy of this module (renamed entry point) to reach the
// forward-migration loop, which a v1 build never executes. See
// firmware/test/test_nvs_migration.c and firmware/test/Makefile.
#ifndef COMPILED_NVS_VERSION
#define COMPILED_NVS_VERSION 1
#endif

// Run NVS schema migration on boot
// Opens 'spaxel' NVS namespace and reads schema_ver.
// If missing, initializes schema_ver to 1.
// If schema_ver < COMPILED_NVS_VERSION, runs migrations in order.
// Returns ESP_OK on success, or error code on failure.
esp_err_t nvs_migration_run(void);
