/*
 * ============================================================================
 *  Host test: NVS schema migration
 * ============================================================================
 *
 *  Covers the plan's Testing-Strategy requirement:
 *      `nvs` — NVS schema migration: simulate schema_ver=0→1 upgrade.
 *
 *  This is a gcc host test (see test_runner.h's header comment + the decision
 *  record docs/notes/firmware-host-test-approach.md, bead bf-21t, for why this
 *  is plain gcc and NOT ESP-IDF --target linux: firmware/main builds as one
 *  idf_component whose REQUIRES names esp_wifi/bt/driver, none of which have a
 *  linux build, so nvs_migration.c — whose own includes WOULD be hostable in
 *  isolation — is still unhostable as part of the component). The harness
 *  therefore mirrors the migration *logic* as a pure, dependency-free extraction
 *  against an in-memory key-value store rather than linking the firmware source.
 *
 *  What is mirrored (byte-for-byte decision logic) from
 *  firmware/main/nvs_migration.c:
 *
 *    - NVS schema version key is "schema_ver" (spaxel.h NVS_KEY_SCHEMA_VER).
 *    - Fresh install (no schema_ver) initializes it to 1, NOT 0 — the "0→1"
 *      in the plan is loose wording for "first-run initialization".
 *    - The forward-migration loop runs migrations[v-1] for v in
 *      [found_ver, compiled_ver). i.e. migration v→(v+1) lives at index (v-1).
 *    - found_ver > compiled_ver → return OK WITHOUT downgrading (caution path).
 *    - found_ver == compiled_ver → no-op, OK.
 *    - A requested migration index past the end of the migrations[] array →
 *      ESP_ERR_NOT_FOUND (a defined-but-unimplemented version gap).
 *    - migrate_v1_to_v2 concrete effects: rename key "ms_ip" → "mothership_ip"
 *      (only if present), and set "ntp_server"="pool.ntp.org" if absent.
 *
 *  Subtlety pinned here: today COMPILED_NVS_VERSION == 1, so the loop body
 *  never executes in production (fresh init sets schema_ver straight to 1).
 *  The migrations[] array is defined ahead-of-time for FUTURE bumps. The
 *  machinery must still be correct so the day COMPILED goes to 2 the rename
 *  fires automatically — so these tests drive it against a simulated higher
 *  compiled version to prove the dispatch + side effects work.
 *
 *  The real esp_ NVS call sites remain validated on-target and via the Go
 *  spaxel-sim acceptance suite; this is the logic safety net.
 * ============================================================================
 */
#include "test_runner.h"

#include <stdbool.h>
#include <stdint.h>
#include <stdio.h>
#include <string.h>

/* ---- Status codes (mirror the subset of esp_err_t the migration uses) ----- */
enum {
    MIG_OK = 0,            /* ESP_OK   */
    MIG_ERR_NOT_FOUND,     /* ESP_ERR_NOT_FOUND  — undefined migration index   */
};

/* ---- In-memory NVS stand-in ---------------------------------------------- */
/*
 * Tiny string key→string value store. The production migration only touches
 * string keys (schema_ver is u8, carried here as its decimal string), so a
 * string store models everything nvs_migration.c actually reads and writes.
 */
#define KV_MAX   32
#define KEY_LEN  16   /* ESP-IDF NVS key limit is 15 chars + NUL */
#define VAL_LEN  128

typedef struct {
    char key[KEY_LEN];
    char val[VAL_LEN];
} kv_t;

typedef struct {
    kv_t rows[KV_MAX];
    int  count;
} nvs_store_t;

static void store_reset(nvs_store_t *s) { s->count = 0; }

static kv_t *store_find(nvs_store_t *s, const char *key) {
    for (int i = 0; i < s->count; i++) {
        if (strncmp(s->rows[i].key, key, KEY_LEN) == 0) {
            return &s->rows[i];
        }
    }
    return NULL;
}

/* Returns true if a key is present (mirrors nvs_str_exists). */
static bool store_exists(nvs_store_t *s, const char *key) {
    return store_find(s, key) != NULL;
}

static void store_set(nvs_store_t *s, const char *key, const char *val) {
    kv_t *row = store_find(s, key);
    if (row == NULL) {
        /* Capacity is generous for these tests; a full store is a test bug. */
        if (s->count >= KV_MAX) {
            return;
        }
        row = &s->rows[s->count++];
        strncpy(row->key, key, KEY_LEN - 1);
        row->key[KEY_LEN - 1] = '\0';
    }
    strncpy(row->val, val, VAL_LEN - 1);
    row->val[VAL_LEN - 1] = '\0';
}

static const char *store_get(nvs_store_t *s, const char *key) {
    kv_t *row = store_find(s, key);
    return row ? row->val : NULL;
}

static void store_del(nvs_store_t *s, const char *key) {
    for (int i = 0; i < s->count; i++) {
        if (strncmp(s->rows[i].key, key, KEY_LEN) == 0) {
            /* Compact by moving the last row into the freed slot. */
            s->rows[i] = s->rows[s->count - 1];
            s->count--;
            return;
        }
    }
}

/* ---- Mirror of the migration steps (firmware/main/nvs_migration.c) -------- */
/*
 * NVS schema version key + the rename target/default from the real migration.
 */
#define KEY_SCHEMA_VER   "schema_ver"
#define KEY_MS_IP        "ms_ip"
#define KEY_MS_IP_RENAMED "mothership_ip"
#define KEY_NTP_SERVER   "ntp_server"
#define DEFAULT_NTP      "pool.ntp.org"

/*
 * Test-only dispatch probe: the index of the most recent migration the runner
 * dispatched. Lets the index-arithmetic test assert WHICH step fired without
 * depending on a migration's side effects (production v2→v3 is a no-op).
 */
static int g_last_migration_idx = -1;

static int migrate_v1_to_v2(nvs_store_t *s) {
    g_last_migration_idx = 0;
    /* Rename ms_ip → mothership_ip (only if ms_ip is present). */
    const char *ms_ip = store_get(s, KEY_MS_IP);
    if (ms_ip) {
        store_set(s, KEY_MS_IP_RENAMED, ms_ip);
        store_del(s, KEY_MS_IP);
    }
    /* Add ntp_server default only if absent. */
    if (!store_exists(s, KEY_NTP_SERVER)) {
        store_set(s, KEY_NTP_SERVER, DEFAULT_NTP);
    }
    return MIG_OK;
}

static int migrate_v2_to_v3(nvs_store_t *s) {
    g_last_migration_idx = 1;
    (void)s; /* No changes yet — matches production placeholder. */
    return MIG_OK;
}

typedef int (*migration_fn_t)(nvs_store_t *);
static const migration_fn_t g_migrations[] = {
    migrate_v1_to_v2,  /* index 0: v1 → v2 */
    migrate_v2_to_v3,  /* index 1: v2 → v3 */
};
#define MIGRATION_COUNT ((int)(sizeof(g_migrations) / sizeof(g_migrations[0])))

/*
 * Mirror of nvs_migration_run(), but parameterized on the compiled target
 * version so the machinery can be exercised against a future bump. Returns
 * MIG_OK on success / no-op, MIG_ERR_NOT_FOUND if a needed migration index is
 * beyond the defined array.
 *
 * found_ver is passed explicitly instead of read from the store so the fresh-
 * install branch (missing schema_ver → init to 1) can be tested distinctly;
 * on a fresh install the caller passes found_ver=0 with *was_missing=true.
 */
static int migration_run(nvs_store_t *s, uint8_t found_ver, bool was_missing,
                         uint8_t compiled_ver)
{
    /* Fresh install: missing schema_ver initializes to 1 (NOT 0). */
    if (was_missing) {
        found_ver = 1;
        char buf[8];
        snprintf(buf, sizeof(buf), "%u", (unsigned)found_ver);
        store_set(s, KEY_SCHEMA_VER, buf);
    }

    /* Downgrade caution path: found newer than compiled → leave it, OK. */
    if (found_ver > compiled_ver) {
        return MIG_OK;
    }
    /* Already current → no-op. */
    if (found_ver == compiled_ver) {
        return MIG_OK;
    }

    /* Forward migration loop: for v in [found_ver, compiled_ver). */
    for (uint8_t v = found_ver; v < compiled_ver; v++) {
        size_t idx = (size_t)(v - 1);   /* migration v→v+1 lives at index v-1 */
        if (idx >= (size_t)MIGRATION_COUNT) {
            return MIG_ERR_NOT_FOUND;
        }
        int rc = g_migrations[idx](s);
        if (rc != MIG_OK) {
            return rc;
        }
        char buf[8];
        snprintf(buf, sizeof(buf), "%u", (unsigned)(v + 1));
        store_set(s, KEY_SCHEMA_VER, buf);
    }
    return MIG_OK;
}

/* ---- Tests ---------------------------------------------------------------- */

/* Fresh install (no schema_ver) initializes to v1 and writes nothing else. */
TEST(nvs_migration_fresh_install_inits_to_v1)
{
    nvs_store_t s; store_reset(&s);
    g_last_migration_idx = -1;

    int rc = migration_run(&s, 0, true /*was_missing*/, 1);

    ASSERT_EQ(rc, MIG_OK);
    ASSERT_TRUE(store_exists(&s, KEY_SCHEMA_VER));
    ASSERT_EQ(strcmp(store_get(&s, KEY_SCHEMA_VER), "1"), 0);
    ASSERT_EQ(g_last_migration_idx, -1);   /* loop never ran (1 < 1 is false) */
    ASSERT_EQ(s.count, 1);                 /* only schema_ver was written */
}

/* Already at the current version: no-op, no migrations dispatched. */
TEST(nvs_migration_already_current_is_noop)
{
    nvs_store_t s; store_reset(&s);
    store_set(&s, KEY_SCHEMA_VER, "1");
    g_last_migration_idx = -1;

    int rc = migration_run(&s, 1, false, 1);

    ASSERT_EQ(rc, MIG_OK);
    ASSERT_EQ(strcmp(store_get(&s, KEY_SCHEMA_VER), "1"), 0);
    ASSERT_EQ(g_last_migration_idx, -1);
}

/*
 * No-downgrade guard: a schema_ver NEWER than compiled is left untouched and
 * the run still returns OK (production logs a warning but does not downgrade).
 */
TEST(nvs_migration_does_not_downgrade_newer_version)
{
    nvs_store_t s; store_reset(&s);
    store_set(&s, KEY_SCHEMA_VER, "3");
    store_set(&s, KEY_MS_IP, "10.0.0.1");  /* must survive untouched */
    g_last_migration_idx = -1;

    int rc = migration_run(&s, 3, false, 1);

    ASSERT_EQ(rc, MIG_OK);
    ASSERT_EQ(strcmp(store_get(&s, KEY_SCHEMA_VER), "3"), 0);
    ASSERT_TRUE(store_exists(&s, KEY_MS_IP));
    ASSERT_FALSE(store_exists(&s, KEY_MS_IP_RENAMED));
    ASSERT_EQ(g_last_migration_idx, -1);
}

/*
 * The plan's headline case, driven against a simulated compiled=v2 so the
 * forward machinery actually fires: v1 + ms_ip → v2, ms_ip renamed to
 * mothership_ip, ntp_server defaulted in.
 */
TEST(nvs_migration_v1_to_v2_renames_ms_ip_and_defaults_ntp)
{
    nvs_store_t s; store_reset(&s);
    store_set(&s, KEY_SCHEMA_VER, "1");
    store_set(&s, KEY_MS_IP, "192.168.1.10");
    g_last_migration_idx = -1;

    int rc = migration_run(&s, 1, false, 2);

    ASSERT_EQ(rc, MIG_OK);
    ASSERT_EQ(g_last_migration_idx, 0);                 /* v1→v2 fired        */
    ASSERT_EQ(strcmp(store_get(&s, KEY_SCHEMA_VER), "2"), 0);
    ASSERT_FALSE(store_exists(&s, KEY_MS_IP));          /* old key erased     */
    ASSERT_EQ(strcmp(store_get(&s, KEY_MS_IP_RENAMED), "192.168.1.10"), 0);
    ASSERT_EQ(strcmp(store_get(&s, KEY_NTP_SERVER), DEFAULT_NTP), 0);
}

/* An existing ntp_server must NOT be overwritten by the default on migration. */
TEST(nvs_migration_v1_to_v2_preserves_existing_ntp)
{
    nvs_store_t s; store_reset(&s);
    store_set(&s, KEY_SCHEMA_VER, "1");
    store_set(&s, KEY_NTP_SERVER, "time.google.com");

    int rc = migration_run(&s, 1, false, 2);

    ASSERT_EQ(rc, MIG_OK);
    ASSERT_EQ(strcmp(store_get(&s, KEY_NTP_SERVER), "time.google.com"), 0);
}

/* ms_ip absent → the rename step is skipped cleanly, no key invented. */
TEST(nvs_migration_v1_to_v2_without_ms_ip_skips_rename)
{
    nvs_store_t s; store_reset(&s);
    store_set(&s, KEY_SCHEMA_VER, "1");

    int rc = migration_run(&s, 1, false, 2);

    ASSERT_EQ(rc, MIG_OK);
    ASSERT_FALSE(store_exists(&s, KEY_MS_IP));
    ASSERT_FALSE(store_exists(&s, KEY_MS_IP_RENAMED));
    ASSERT_TRUE(store_exists(&s, KEY_NTP_SERVER));   /* default still applied */
}

/*
 * Migration-index arithmetic: a store already at v2 advancing to v3 must
 * dispatch the v2→v3 step (index 1), NOT v1→v2. Proves the loop selects
 * migrations[v-1], not a fixed index, and would never re-run an old migration.
 */
TEST(nvs_migration_index_arithmetic_picks_correct_step)
{
    nvs_store_t s; store_reset(&s);
    store_set(&s, KEY_SCHEMA_VER, "2");
    store_set(&s, KEY_MS_IP, "10.0.0.5");   /* would be renamed ONLY by v1→v2 */
    g_last_migration_idx = -1;

    int rc = migration_run(&s, 2, false, 3);

    ASSERT_EQ(rc, MIG_OK);
    ASSERT_EQ(g_last_migration_idx, 1);                 /* v2→v3 fired        */
    ASSERT_EQ(strcmp(store_get(&s, KEY_SCHEMA_VER), "3"), 0);
    /* ms_ip untouched: proves v1→v2 (index 0) did NOT run. */
    ASSERT_TRUE(store_exists(&s, KEY_MS_IP));
    ASSERT_FALSE(store_exists(&s, KEY_MS_IP_RENAMED));
}

/*
 * Undefined-version gap: asking for a compiled version whose migration index
 * is beyond the defined array returns MIG_ERR_NOT_FOUND rather than reading
 * garbage. Production returns ESP_ERR_NOT_FOUND and leaves NVS at the prior
 * consistent version.
 */
TEST(nvs_migration_undefined_future_version_returns_not_found)
{
    nvs_store_t s; store_reset(&s);
    store_set(&s, KEY_SCHEMA_VER, "1");

    /* Only v1→v2 and v2→v3 are defined; compiled=4 needs v3→v4 (index 2). */
    int rc = migration_run(&s, 1, false, 4);

    ASSERT_EQ(rc, MIG_ERR_NOT_FOUND);
    /* schema_ver stays where the last SUCCESSFUL step left it (v3). */
    ASSERT_EQ(strcmp(store_get(&s, KEY_SCHEMA_VER), "3"), 0);
}

/*
 * Multi-step advance (v1 → v3) runs both migrations in order and lands at v3.
 * Guards against off-by-one in the loop bound (compiled_ver is exclusive).
 */
TEST(nvs_migration_multi_step_advance)
{
    nvs_store_t s; store_reset(&s);
    store_set(&s, KEY_SCHEMA_VER, "1");
    store_set(&s, KEY_MS_IP, "172.16.0.2");

    int rc = migration_run(&s, 1, false, 3);

    ASSERT_EQ(rc, MIG_OK);
    ASSERT_EQ(strcmp(store_get(&s, KEY_SCHEMA_VER), "3"), 0);
    ASSERT_EQ(strcmp(store_get(&s, KEY_MS_IP_RENAMED), "172.16.0.2"), 0);
    ASSERT_EQ(strcmp(store_get(&s, KEY_NTP_SERVER), DEFAULT_NTP), 0);
}
