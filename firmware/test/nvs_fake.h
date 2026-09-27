/*
 * Shared handle onto the in-memory NVS fake implemented in
 * firmware/test/test_nvs_migration.c.
 *
 * The harness links every test_*.c into one binary, so the fake's esp_err/nvs
 * API definitions must exist exactly once — they live in test_nvs_migration.c,
 * which owns the store and its rows. Tests that include a real production TU
 * needing NVS state (test_safe_mode.c includes ../main/safe_mode.c) cannot
 * reach the store's statics directly; these are the few helpers those tests
 * need, deliberately kept non-static there and declared here so both sides
 * agree on the signatures.
 */
#pragma once

#include <stdint.h>

/* Clear the whole fake store, open/commit accounting, and fault hooks. */
void nvs_t_reset(void);

/* Seed a typed row directly, bypassing commit accounting, so a test's starting
 * state is not mistaken for work done through the API. */
void nvs_t_seed_u8(const char *key, uint8_t value);
void nvs_t_seed_u32(const char *key, uint32_t value);
