#include "nvs_migration.h"
#include "spaxel.h"
#include "esp_log.h"
#include "nvs_flash.h"
#include "nvs.h"
#include <string.h>

static const char *TAG = "nvs_migration";

// Forward declarations for migration functions
static esp_err_t migrate_v1_to_v2(nvs_handle_t nvs);
static esp_err_t migrate_v2_to_v3(nvs_handle_t nvs);

// Helper: Read a string key, return ESP_OK if exists
static esp_err_t nvs_str_exists(nvs_handle_t nvs, const char *key) {
    size_t len = 0;
    esp_err_t err = nvs_get_str(nvs, key, NULL, &len);
    return (err == ESP_OK) ? ESP_OK : ESP_ERR_NVS_NOT_FOUND;
}

// Helper: Rename a string key
static esp_err_t nvs_rename_str(nvs_handle_t nvs, const char *old_key, const char *new_key) {
    char buf[128];
    size_t len = sizeof(buf);

    esp_err_t err = nvs_get_str(nvs, old_key, buf, &len);
    if (err != ESP_OK) {
        return err;  // Old key doesn't exist or other error
    }

    err = nvs_set_str(nvs, new_key, buf);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to set new key '%s': %s", new_key, esp_err_to_name(err));
        return err;
    }

    err = nvs_commit(nvs);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to commit after rename to '%s': %s", new_key, esp_err_to_name(err));
        return err;
    }

    err = nvs_erase_key(nvs, old_key);
    if (err != ESP_OK && err != ESP_ERR_NVS_NOT_FOUND) {
        ESP_LOGW(TAG, "Failed to erase old key '%s': %s", old_key, esp_err_to_name(err));
        // Non-fatal: new key exists, continue
    }

    err = nvs_commit(nvs);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to commit after erasing '%s': %s", old_key, esp_err_to_name(err));
        return err;
    }

    return ESP_OK;
}

// Migration v1 → v2
// Changes:
// - Rename 'ms_ip' to 'mothership_ip'
// - Add 'ntp_server' with default 'pool.ntp.org'
static esp_err_t migrate_v1_to_v2(nvs_handle_t nvs) {
    ESP_LOGI(TAG, "Running migration v1→v2...");

    // Rename ms_ip → mothership_ip
    esp_err_t err = nvs_rename_str(nvs, NVS_KEY_MS_IP, "mothership_ip");
    if (err == ESP_OK) {
        ESP_LOGI(TAG, "  [v1→v2] Renamed 'ms_ip' → 'mothership_ip'");
    } else if (err == ESP_ERR_NVS_NOT_FOUND) {
        ESP_LOGD(TAG, "  [v1→v2] 'ms_ip' not found, skipping rename");
    } else {
        ESP_LOGE(TAG, "  [v1→v2] Failed to rename 'ms_ip': %s", esp_err_to_name(err));
        return err;
    }

    // Add ntp_server with default if not exists
    if (nvs_str_exists(nvs, "ntp_server") != ESP_OK) {
        err = nvs_set_str(nvs, "ntp_server", "pool.ntp.org");
        if (err != ESP_OK) {
            ESP_LOGE(TAG, "  [v1→v2] Failed to set 'ntp_server': %s", esp_err_to_name(err));
            return err;
        }
        err = nvs_commit(nvs);
        if (err != ESP_OK) {
            ESP_LOGE(TAG, "  [v1→v2] Failed to commit 'ntp_server': %s", esp_err_to_name(err));
            return err;
        }
        ESP_LOGI(TAG, "  [v1→v2] Added 'ntp_server' = 'pool.ntp.org'");
    } else {
        ESP_LOGD(TAG, "  [v1→v2] 'ntp_server' already exists, skipping");
    }

    ESP_LOGI(TAG, "Migration v1→v2 complete");
    return ESP_OK;
}

// Migration v2 → v3 (placeholder for future migrations)
// Add future migrations here
static esp_err_t migrate_v2_to_v3(nvs_handle_t nvs) {
    ESP_LOGI(TAG, "Running migration v2→v3...");
    // No changes yet
    ESP_LOGI(TAG, "Migration v2→v3 complete");
    return ESP_OK;
}

// Array of migration functions
// Index i contains the migration from version i to i+1
typedef esp_err_t (*migration_fn_t)(nvs_handle_t);

static const migration_fn_t migrations[] = {
    migrate_v1_to_v2,  // Index 0: v1 → v2
    migrate_v2_to_v3,  // Index 1: v2 → v3
    // Add new migrations here
};

esp_err_t nvs_migration_run(void) {
    nvs_handle_t nvs;
    esp_err_t err;

    ESP_LOGI(TAG, "Starting NVS schema migration check...");

    // Open NVS namespace
    err = nvs_open(SPAXEL_NAMESPACE, NVS_READWRITE, &nvs);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to open NVS namespace '%s': %s", SPAXEL_NAMESPACE, esp_err_to_name(err));
        return err;
    }

    // Read current schema version
    uint8_t schema_ver = 0;
    err = nvs_get_u8(nvs, NVS_KEY_SCHEMA_VER, &schema_ver);
    if (err == ESP_ERR_NVS_NOT_FOUND) {
        // No schema version found - initialize to 1
        ESP_LOGI(TAG, "No schema_ver found, initializing to v1");
        schema_ver = 1;
        err = nvs_set_u8(nvs, NVS_KEY_SCHEMA_VER, schema_ver);
        if (err != ESP_OK) {
            ESP_LOGE(TAG, "Failed to set initial schema_ver: %s", esp_err_to_name(err));
            nvs_close(nvs);
            return err;
        }
        err = nvs_commit(nvs);
        if (err != ESP_OK) {
            ESP_LOGE(TAG, "Failed to commit initial schema_ver: %s", esp_err_to_name(err));
            nvs_close(nvs);
            return err;
        }
    } else if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to read schema_ver: %s", esp_err_to_name(err));
        nvs_close(nvs);
        return err;
    }

    ESP_LOGI(TAG, "Current NVS schema version: %d, compiled version: %d", schema_ver, COMPILED_NVS_VERSION);

    // Check if migration is needed
    if (schema_ver > COMPILED_NVS_VERSION) {
        ESP_LOGW(TAG, "NVS schema version %d is newer than compiled version %d. "
                      "Firmware may be downgraded. Proceeding with caution.",
                 schema_ver, COMPILED_NVS_VERSION);
        nvs_close(nvs);
        return ESP_OK;  // Don't downgrade, just continue
    }

    if (schema_ver == COMPILED_NVS_VERSION) {
        ESP_LOGI(TAG, "NVS schema is up to date");
        nvs_close(nvs);
        return ESP_OK;
    }

    // Run migrations in order
    ESP_LOGI(TAG, "Migrating NVS schema from v%d to v%d...", schema_ver, COMPILED_NVS_VERSION);

    for (uint8_t v = schema_ver; v < COMPILED_NVS_VERSION; v++) {
        // Index in migrations array is (v - 1) since array is 0-indexed
        // Example: v1→v2 is at index 0, v2→v3 is at index 1
        size_t migration_idx = v - 1;

        if (migration_idx >= (sizeof(migrations) / sizeof(migrations[0]))) {
            ESP_LOGE(TAG, "Migration function for v%d→v%d not found!", v, v + 1);
            nvs_close(nvs);
            return ESP_ERR_NOT_FOUND;
        }

        ESP_LOGI(TAG, "Running migration v%d→v%d...", v, v + 1);
        err = migrations[migration_idx](nvs);
        if (err != ESP_OK) {
            ESP_LOGE(TAG, "Migration v%d→v%d failed: %s", v, v + 1, esp_err_to_name(err));
            ESP_LOGE(TAG, "NVS left in consistent state at v%d. Please investigate.", v);
            nvs_close(nvs);
            return err;
        }

        // Update schema_ver after successful migration
        uint8_t new_ver = v + 1;
        err = nvs_set_u8(nvs, NVS_KEY_SCHEMA_VER, new_ver);
        if (err != ESP_OK) {
            ESP_LOGE(TAG, "Failed to update schema_ver to %d: %s", new_ver, esp_err_to_name(err));
            nvs_close(nvs);
            return err;
        }

        err = nvs_commit(nvs);
        if (err != ESP_OK) {
            ESP_LOGE(TAG, "Failed to commit schema_ver update: %s", esp_err_to_name(err));
            nvs_close(nvs);
            return err;
        }

        ESP_LOGI(TAG, "Schema version updated to v%d", new_ver);
    }

    ESP_LOGI(TAG, "NVS migration complete: v%d → v%d", schema_ver, COMPILED_NVS_VERSION);
    nvs_close(nvs);
    return ESP_OK;
}
