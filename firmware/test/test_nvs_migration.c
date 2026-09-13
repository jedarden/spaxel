/*
 * ============================================================================
 *  Host test: NVS schema migration
 * ============================================================================
 *
 *  Covers the plan's Testing-Strategy requirement:
 *      `nvs` — NVS schema migration: simulate schema_ver=0→1 upgrade.
 *
 *  Unlike the rest of the harness, this test links the REAL production TU,
 *  firmware/main/nvs_migration.c — not a mirror of its logic. That became
 *  possible via the host_compat/ stub headers: the stubs supply the esp_* and
 *  nvs API surface that TU uses, while the REAL spaxel.h and nvs_migration.h are
 *  still included (so the NVS key strings and COMPILED_NVS_VERSION track
 *  production instead of drifting). See the Makefile's "production TU" block
 *  for the compile, and test_runner.h for why this is the harness's only
 *  linked firmware source.
 *
 *  The production entry point is compiled four times, differing only by
 *  -DCOMPILED_NVS_VERSION and a renamed entry-point symbol:
 *
 *      nvs_migration_run                 COMPILED_NVS_VERSION 1 (shipped build)
 *      nvs_migration_run_compiled_v2     COMPILED_NVS_VERSION 2
 *      nvs_migration_run_compiled_v3     COMPILED_NVS_VERSION 3
 *      nvs_migration_run_compiled_v4     COMPILED_NVS_VERSION 4
 *
 *  That is what makes the forward-migration loop reachable at all: today's
 *  shipped build has COMPILED_NVS_VERSION == 1, so `schema_ver < compiled`
 *  never holds and the loop body is dead code in production. Raising the
 *  constant at compile time (exactly what a future firmware release does)
 *  drives it, while the migration table itself — migrate_v1_to_v2 and the
 *  migrate_v2_to_v3 placeholder — stays the production one.
 *
 *  Subtlety pinned by the fresh-install case: COMPILED_NVS_VERSION is 1 and a
 *  missing schema_ver key initializes to 1, so the plan's "schema_ver=0→1
 *  upgrade" is really the no-key-present → initialize-to-v1 path. A stored
 *  schema_ver of 0 does NOT take that path — it enters the loop, computes
 *  migration_idx = (size_t)(0 - 1), and lands in the out-of-range guard,
 *  returning ESP_ERR_NOT_FOUND. Both are asserted.
 *
 *  In-memory NVS: rows are typed (u8/str) and a key's type is fixed by its
 *  first write, so a type-confused access on an existing key surfaces as
 *  ESP_ERR_NVS_TYPE_MISMATCH rather than silently reading as a zero — matching
 *  a real NVS partition. Writes become visible immediately and nvs_commit()
 *  is counted rather than journaled — modeling real NVS transactional
 *  rollback would mean rebuilding the mirror this test exists to retire.
 *  The commit *count* is still meaningful: nvs_migration.c commits after
 *  every individual write, which is what keeps a mid-migration power loss
 *  from leaving the store half-migrated.
 * ============================================================================
 */
#include "test_runner.h"

#include "esp_err.h"
#include "nvs.h"
#include "nvs_migration.h"
#include "spaxel.h"

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <string.h>

/* The raised-COMPILED_NVS_VERSION entry points built by the Makefile's
 * production-TU block. nvs_migration.h cannot declare them: each is the same
 * production function compiled at a different constant, renamed only so the
 * four objects link together. */
esp_err_t nvs_migration_run_compiled_v2(void);
esp_err_t nvs_migration_run_compiled_v3(void);
esp_err_t nvs_migration_run_compiled_v4(void);

/* ---- In-memory NVS backing the stub declarations in host_compat/nvs.h ----- */

#define NVS_T_MAX_ROWS 32
#define NVS_T_KEY_LEN  16   /* ESP-IDF NVS key limit is 15 chars + NUL */
#define NVS_T_VAL_LEN  128

typedef enum {
    NVS_T_NONE = 0,   /* fresh row: type is fixed by its first write */
    NVS_T_U8 = 1,
    NVS_T_STR = 2,
} nvs_t_type_t;

typedef struct {
    char key[NVS_T_KEY_LEN];
    nvs_t_type_t type;
    uint8_t u8;
    char str[NVS_T_VAL_LEN];
} nvs_t_row_t;

static struct {
    nvs_t_row_t rows[NVS_T_MAX_ROWS];
    int count;
    bool open;
    int commits;
    /* One-shot fault injection; cleared on delivery. */
    esp_err_t fail_open;
    esp_err_t fail_set;
    esp_err_t fail_commit;
} g_nvs;

static void nvs_t_reset(void)
{
    memset(&g_nvs, 0, sizeof(g_nvs));
}

/* Seed helpers: write straight into the store, bypassing the API's commit
 * accounting, so a test's starting state is not mistaken for migration work. */
static void nvs_t_seed_u8(const char *key, uint8_t value)
{
    nvs_t_row_t *r = &g_nvs.rows[g_nvs.count++];
    memset(r, 0, sizeof(*r));
    strncpy(r->key, key, sizeof(r->key) - 1);
    r->type = NVS_T_U8;
    r->u8 = value;
}

static void nvs_t_seed_str(const char *key, const char *value)
{
    nvs_t_row_t *r = &g_nvs.rows[g_nvs.count++];
    memset(r, 0, sizeof(*r));
    strncpy(r->key, key, sizeof(r->key) - 1);
    r->type = NVS_T_STR;
    strncpy(r->str, value, sizeof(r->str) - 1);
}

static esp_err_t nvs_t_fail(esp_err_t *hook)
{
    esp_err_t err = *hook;
    *hook = ESP_OK;
    return err;
}

static nvs_t_row_t *nvs_t_find(const char *key)
{
    if (key == NULL || strlen(key) >= NVS_T_KEY_LEN) {
        return NULL;
    }
    for (int i = 0; i < g_nvs.count; i++) {
        if (strcmp(g_nvs.rows[i].key, key) == 0) {
            return &g_nvs.rows[i];
        }
    }
    return NULL;
}

/* Slot for a write, or NULL if the store is full (a test bug at these sizes). */
static nvs_t_row_t *nvs_t_slot(const char *key)
{
    nvs_t_row_t *r = nvs_t_find(key);
    if (r != NULL) {
        return r;
    }
    if (g_nvs.count >= NVS_T_MAX_ROWS) {
        return NULL;
    }
    r = &g_nvs.rows[g_nvs.count++];
    memset(r, 0, sizeof(*r));
    strncpy(r->key, key, sizeof(r->key) - 1);
    return r;
}

/* ---- nvs.h API, backed by the store above --------------------------------- */

esp_err_t nvs_open(const char *ns, nvs_open_mode_t open_mode,
                   nvs_handle_t *out_handle)
{
    if (g_nvs.fail_open != ESP_OK) {
        return nvs_t_fail(&g_nvs.fail_open);
    }
    if (ns == NULL || out_handle == NULL) {
        return ESP_ERR_INVALID_ARG;
    }
    (void)open_mode;  /* production always opens READWRITE; both accepted */
    g_nvs.open = true;
    *out_handle = 1;
    return ESP_OK;
}

void nvs_close(nvs_handle_t handle)
{
    (void)handle;
    g_nvs.open = false;
}

esp_err_t nvs_get_u8(nvs_handle_t handle, const char *key, uint8_t *out_value)
{
    (void)handle;
    nvs_t_row_t *r = nvs_t_find(key);
    if (r == NULL) {
        return ESP_ERR_NVS_NOT_FOUND;
    }
    if (r->type != NVS_T_U8) {
        return ESP_ERR_NVS_TYPE_MISMATCH;
    }
    if (out_value != NULL) {
        *out_value = r->u8;
    }
    return ESP_OK;
}

esp_err_t nvs_set_u8(nvs_handle_t handle, const char *key, uint8_t value)
{
    (void)handle;
    if (g_nvs.fail_set != ESP_OK) {
        return nvs_t_fail(&g_nvs.fail_set);
    }
    nvs_t_row_t *r = nvs_t_slot(key);
    if (r == NULL) {
        return ESP_ERR_NO_MEM;
    }
    if (r->type == NVS_T_NONE) {
        r->type = NVS_T_U8;   /* first write fixes the type, like real NVS */
    } else if (r->type != NVS_T_U8) {
        return ESP_ERR_NVS_TYPE_MISMATCH;
    }
    r->u8 = value;
    return ESP_OK;
}

esp_err_t nvs_get_str(nvs_handle_t handle, const char *key, char *out_value,
                      size_t *length)
{
    (void)handle;
    nvs_t_row_t *r = nvs_t_find(key);
    if (r == NULL) {
        return ESP_ERR_NVS_NOT_FOUND;
    }
    if (r->type != NVS_T_STR) {
        return ESP_ERR_NVS_TYPE_MISMATCH;
    }
    size_t needed = strlen(r->str) + 1;
    if (length != NULL) {
        if (out_value != NULL && *length < needed) {
            *length = needed;
            return ESP_ERR_NVS_INVALID_LENGTH;
        }
        *length = needed;
    }
    if (out_value != NULL) {
        memcpy(out_value, r->str, needed);
    }
    return ESP_OK;
}

esp_err_t nvs_set_str(nvs_handle_t handle, const char *key, const char *value)
{
    (void)handle;
    if (g_nvs.fail_set != ESP_OK) {
        return nvs_t_fail(&g_nvs.fail_set);
    }
    if (key == NULL || value == NULL || strlen(key) >= NVS_T_KEY_LEN ||
        strlen(value) >= NVS_T_VAL_LEN) {
        return ESP_ERR_NVS_INVALID_LENGTH;
    }
    nvs_t_row_t *r = nvs_t_slot(key);
    if (r == NULL) {
        return ESP_ERR_NO_MEM;
    }
    if (r->type == NVS_T_NONE) {
        r->type = NVS_T_STR;   /* first write fixes the type, like real NVS */
    } else if (r->type != NVS_T_STR) {
        return ESP_ERR_NVS_TYPE_MISMATCH;
    }
    strcpy(r->str, value);
    return ESP_OK;
}

esp_err_t nvs_erase_key(nvs_handle_t handle, const char *key)
{
    (void)handle;
    nvs_t_row_t *r = nvs_t_find(key);
    if (r == NULL) {
        return ESP_ERR_NVS_NOT_FOUND;
    }
    *r = g_nvs.rows[g_nvs.count - 1];
    g_nvs.count--;
    return ESP_OK;
}

esp_err_t nvs_commit(nvs_handle_t handle)
{
    (void)handle;
    if (g_nvs.fail_commit != ESP_OK) {
        return nvs_t_fail(&g_nvs.fail_commit);
    }
    g_nvs.commits++;
    return ESP_OK;
}

/* ---- Stubs declared by the other host_compat headers ---------------------- */

void host_compat_log(const char *tag, const char *fmt, ...)
{
    /* Format strings are still checked at the call site via the printf
     * attribute; nothing is emitted so the test output stays clean. */
    (void)tag;
    (void)fmt;
}

const char *esp_err_to_name(esp_err_t code)
{
    (void)code;
    return "ESP_ERR";
}

/* ---- Assertions over the store -------------------------------------------- */

static void assert_key_u8(const char *key, uint8_t expected)
{
    nvs_t_row_t *r = nvs_t_find(key);
    ASSERT_TRUE(r != NULL);
    ASSERT_EQ(r->type, NVS_T_U8);
    ASSERT_EQ(r->u8, expected);
}

static void assert_key_str(const char *key, const char *expected)
{
    nvs_t_row_t *r = nvs_t_find(key);
    ASSERT_TRUE(r != NULL);
    ASSERT_EQ(r->type, NVS_T_STR);
    ASSERT_EQ(strcmp(r->str, expected), 0);
}

static void assert_key_absent(const char *key)
{
    ASSERT_TRUE(nvs_t_find(key) == NULL);
}

/* ---- Shipped build (COMPILED_NVS_VERSION 1) ------------------------------- */

/* The plan's "schema_ver=0→1" case: no schema_ver key at all initializes to
 * v1, writes nothing else, and runs no migration. */
TEST(nvs_migration_fresh_install_initializes_schema_ver_to_1)
{
    nvs_t_reset();

    esp_err_t rc = nvs_migration_run();

    ASSERT_EQ(rc, ESP_OK);
    assert_key_u8(NVS_KEY_SCHEMA_VER, 1);
    ASSERT_EQ(g_nvs.count, 1);        /* schema_ver is the only key written */
    ASSERT_EQ(g_nvs.commits, 1);      /* and it is committed, not left dirty */
}

/* Already current: the common boot path must not write a single thing. */
TEST(nvs_migration_already_current_is_noop)
{
    nvs_t_reset();
    nvs_t_seed_u8(NVS_KEY_SCHEMA_VER, 1);

    esp_err_t rc = nvs_migration_run();

    ASSERT_EQ(rc, ESP_OK);
    assert_key_u8(NVS_KEY_SCHEMA_VER, 1);
    ASSERT_EQ(g_nvs.commits, 0);
    ASSERT_EQ(g_nvs.count, 1);
}

/* Downgrade caution: a newer store is left exactly as found. */
TEST(nvs_migration_newer_schema_ver_is_not_downgraded)
{
    nvs_t_reset();
    nvs_t_seed_u8(NVS_KEY_SCHEMA_VER, 3);
    nvs_t_seed_str(NVS_KEY_MS_IP, "10.0.0.1");

    esp_err_t rc = nvs_migration_run();

    ASSERT_EQ(rc, ESP_OK);            /* proceeds, does not fail the boot */
    assert_key_u8(NVS_KEY_SCHEMA_VER, 3);
    assert_key_str(NVS_KEY_MS_IP, "10.0.0.1");
    assert_key_absent("mothership_ip");   /* no migration side effects */
    assert_key_absent(NVS_KEY_NTP_SERVER);
    ASSERT_EQ(g_nvs.commits, 0);
}

/* A stored schema_ver of 0 is not the fresh-install path: it enters the loop,
 * underflows migration_idx into the out-of-range guard, and errors out rather
 * than silently initializing. Pinned so a refactor cannot turn this into a
 * wildcard-fitting read. */
TEST(nvs_migration_stored_schema_ver_zero_returns_not_found)
{
    nvs_t_reset();
    nvs_t_seed_u8(NVS_KEY_SCHEMA_VER, 0);

    esp_err_t rc = nvs_migration_run();

    ASSERT_EQ(rc, ESP_ERR_NOT_FOUND);
    assert_key_u8(NVS_KEY_SCHEMA_VER, 0);   /* untouched: no step ran */
    ASSERT_EQ(g_nvs.commits, 0);
}

/* A schema_ver stored under the wrong type is an error, not a silent zero. */
TEST(nvs_migration_type_mismatched_schema_ver_is_propagated)
{
    nvs_t_reset();
    nvs_t_seed_str(NVS_KEY_SCHEMA_VER, "3");

    esp_err_t rc = nvs_migration_run();

    ASSERT_EQ(rc, ESP_ERR_NVS_TYPE_MISMATCH);
    ASSERT_EQ(g_nvs.commits, 0);
}

/* Failure legs: the migration returns the NVS error instead of booting with a
 * half-written store. */
TEST(nvs_migration_open_failure_is_propagated)
{
    nvs_t_reset();
    g_nvs.fail_open = ESP_ERR_NVS_READ_ONLY;   /* arbitrary non-OK code */

    esp_err_t rc = nvs_migration_run();

    ASSERT_EQ(rc, ESP_ERR_NVS_READ_ONLY);
    ASSERT_EQ(g_nvs.commits, 0);
}

TEST(nvs_migration_initial_write_failure_is_propagated)
{
    nvs_t_reset();
    g_nvs.fail_set = ESP_ERR_NVS_NO_FREE_PAGES;   /* what a full partition gives */

    esp_err_t rc = nvs_migration_run();

    ASSERT_EQ(rc, ESP_ERR_NVS_NO_FREE_PAGES);
    assert_key_absent(NVS_KEY_SCHEMA_VER);
}

TEST(nvs_migration_initial_commit_failure_is_propagated)
{
    nvs_t_reset();
    g_nvs.fail_commit = ESP_ERR_INVALID_STATE;

    esp_err_t rc = nvs_migration_run();

    ASSERT_EQ(rc, ESP_ERR_INVALID_STATE);
}

/* ---- COMPILED_NVS_VERSION 2: the first forward migration ------------------ */

TEST(nvs_migration_v1_to_v2_renames_ms_ip_and_defaults_ntp)
{
    nvs_t_reset();
    nvs_t_seed_u8(NVS_KEY_SCHEMA_VER, 1);
    nvs_t_seed_str(NVS_KEY_MS_IP, "192.168.1.10");

    esp_err_t rc = nvs_migration_run_compiled_v2();

    ASSERT_EQ(rc, ESP_OK);
    assert_key_u8(NVS_KEY_SCHEMA_VER, 2);
    assert_key_absent(NVS_KEY_MS_IP);                    /* old key erased */
    assert_key_str("mothership_ip", "192.168.1.10");     /* value carried over */
    assert_key_str(NVS_KEY_NTP_SERVER, "pool.ntp.org");
    /* rename: 2 (set + erase), ntp: 1, schema_ver bump: 1. Each write is
     * committed individually, which is what keeps a power loss mid-migration
     * from stranding the store between versions. */
    ASSERT_EQ(g_nvs.commits, 4);
}

TEST(nvs_migration_v1_to_v2_preserves_existing_ntp)
{
    nvs_t_reset();
    nvs_t_seed_u8(NVS_KEY_SCHEMA_VER, 1);
    nvs_t_seed_str(NVS_KEY_MS_IP, "10.0.0.5");
    nvs_t_seed_str(NVS_KEY_NTP_SERVER, "time.google.com");

    esp_err_t rc = nvs_migration_run_compiled_v2();

    ASSERT_EQ(rc, ESP_OK);
    assert_key_str(NVS_KEY_NTP_SERVER, "time.google.com");
    assert_key_absent(NVS_KEY_MS_IP);
    assert_key_str("mothership_ip", "10.0.0.5");
    /* rename 2 + schema_ver bump 1 — no ntp write, no extra commit. */
    ASSERT_EQ(g_nvs.commits, 3);
}

TEST(nvs_migration_v1_to_v2_without_ms_ip_skips_rename)
{
    nvs_t_reset();
    nvs_t_seed_u8(NVS_KEY_SCHEMA_VER, 1);

    esp_err_t rc = nvs_migration_run_compiled_v2();

    ASSERT_EQ(rc, ESP_OK);
    assert_key_u8(NVS_KEY_SCHEMA_VER, 2);
    assert_key_absent(NVS_KEY_MS_IP);
    assert_key_absent("mothership_ip");   /* no key invented */
    assert_key_str(NVS_KEY_NTP_SERVER, "pool.ntp.org");
    ASSERT_EQ(g_nvs.commits, 2);          /* ntp default + schema_ver bump */
}

/* A failed rename must not advance schema_ver: the node retries the whole
 * migration on the next boot from a store that still reads as v1. */
TEST(nvs_migration_v1_to_v2_rename_failure_keeps_schema_ver_at_v1)
{
    nvs_t_reset();
    nvs_t_seed_u8(NVS_KEY_SCHEMA_VER, 1);
    nvs_t_seed_str(NVS_KEY_MS_IP, "192.168.1.10");
    /* The rename's nvs_set_str is the first write of the whole run. */
    g_nvs.fail_set = ESP_ERR_NVS_NO_FREE_PAGES;

    esp_err_t rc = nvs_migration_run_compiled_v2();

    ASSERT_EQ(rc, ESP_ERR_NVS_NO_FREE_PAGES);
    assert_key_u8(NVS_KEY_SCHEMA_VER, 1);
    assert_key_str(NVS_KEY_MS_IP, "192.168.1.10");
    assert_key_absent("mothership_ip");
    assert_key_absent(NVS_KEY_NTP_SERVER);
}

/* ---- COMPILED_NVS_VERSION 3 and 4: index arithmetic, ordering, gaps ------- */

/* A v2 store must dispatch migrations[1] (v2→v3), not migrations[0]: ms_ip is
 * left alone because renaming it is v1→v2's job, and a re-run of an old
 * migration would clobber live settings. */
TEST(nvs_migration_index_arithmetic_dispatches_v2_to_v3)
{
    nvs_t_reset();
    nvs_t_seed_u8(NVS_KEY_SCHEMA_VER, 2);
    nvs_t_seed_str(NVS_KEY_MS_IP, "10.0.0.5");

    esp_err_t rc = nvs_migration_run_compiled_v3();

    ASSERT_EQ(rc, ESP_OK);
    assert_key_u8(NVS_KEY_SCHEMA_VER, 3);
    assert_key_str(NVS_KEY_MS_IP, "10.0.0.5");
    assert_key_absent("mothership_ip");
    assert_key_absent(NVS_KEY_NTP_SERVER);
    ASSERT_EQ(g_nvs.commits, 1);   /* only the schema_ver bump: v2→v3 is a no-op */
}

/* Two steps in order: v1→v2's side effects land, then v2→v3 runs, and the run
 * returns OK. Guards against an off-by-one in the loop bound. */
TEST(nvs_migration_multi_step_advance_v1_to_v3)
{
    nvs_t_reset();
    nvs_t_seed_u8(NVS_KEY_SCHEMA_VER, 1);
    nvs_t_seed_str(NVS_KEY_MS_IP, "172.16.0.2");

    esp_err_t rc = nvs_migration_run_compiled_v3();

    ASSERT_EQ(rc, ESP_OK);
    assert_key_u8(NVS_KEY_SCHEMA_VER, 3);
    assert_key_str("mothership_ip", "172.16.0.2");
    assert_key_str(NVS_KEY_NTP_SERVER, "pool.ntp.org");
    ASSERT_EQ(g_nvs.commits, 5);   /* rename 2 + ntp 1 + schema_ver bumps 2 */
}

/* Defined-but-unimplemented gap: v3→v4 has no function, so the run stops at
 * the last consistent version and reports NOT_FOUND rather than dispatching
 * past the end of migrations[]. */
TEST(nvs_migration_undefined_future_version_returns_not_found)
{
    nvs_t_reset();
    nvs_t_seed_u8(NVS_KEY_SCHEMA_VER, 1);
    nvs_t_seed_str(NVS_KEY_MS_IP, "10.9.8.7");

    esp_err_t rc = nvs_migration_run_compiled_v4();

    ASSERT_EQ(rc, ESP_ERR_NOT_FOUND);
    /* Both defined steps ran, in order, each committing its version bump. */
    assert_key_u8(NVS_KEY_SCHEMA_VER, 3);
    assert_key_str("mothership_ip", "10.9.8.7");
    assert_key_str(NVS_KEY_NTP_SERVER, "pool.ntp.org");
    ASSERT_EQ(g_nvs.commits, 5);
}
