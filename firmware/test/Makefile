# ============================================================================
#  Spaxel firmware host test harness — Makefile
# ============================================================================
#
#  Build & run the whole suite, in one command, from the repo root:
#
#      make -C firmware/test test
#
#  That compiles every test_*.c in this directory together with test_runner.c
#  using plain gcc (no ESP-IDF toolchain) and runs the resulting binary. The
#  runner returns non-zero if ANY assertion fails, and make propagates that exit
#  code, so the command is safe to wire straight into CI.
#
#  See test_runner.h for the gcc-harness rationale (decision record: bead bf-21t;
#  this build-out is the split of parent bead bf-4ne into child bf-56v).
#
#  Adding a test: drop firmware/test/test_<thing>.c that uses the TEST() macro
#  from test_runner.h. The wildcard below picks it up automatically; its TEST()
#  self-registers via a GCC constructor — no SOURCES edit, no runner change.
#
#  The one manual addition is the "production TU" block below, which exists for
#  the nvs migration test only.
# ============================================================================

CC      ?= gcc
CFLAGS  ?= -std=c11 -Wall -Wextra -O2 -g
LDFLAGS ?=

# Every self-registering test unit, plus the runner that holds main(). The
# wildcard means a new test_*.c is compiled and linked with zero edits here.
#
# test_runner.c is itself matched by the test_*.c glob, so it is filtered out of
# the test set and added back exactly once as RUNNER_SRC — otherwise main() and
# the registry functions would be defined twice and the link would fail.
RUNNER_SRC := test_runner.c
TEST_SRCS := $(filter-out $(RUNNER_SRC), $(wildcard test_*.c))

BUILD_DIR := build
BIN       := $(BUILD_DIR)/spaxel_host_tests

# ============================================================================
#  Production TU: firmware/main/nvs_migration.c
# ============================================================================
#
#  The rest of the harness never links firmware sources (test_runner.h explains
#  why the firmware/main component cannot build on a host). nvs_migration.c is
#  the single authorized exception: its only externals are esp_*/nvs calls,
#  which host_compat/ stubs, so the REAL migration decision table is tested
#  instead of a mirror copy of it that could drift. The REAL spaxel.h and
#  nvs_migration.h are still included, so NVS key strings and the version
#  constant track production.
#
#  That TU is compiled four times. The shipped build keeps
#  COMPILED_NVS_VERSION=1 — where the forward-migration loop is unreachable, so
#  only the fresh-install / up-to-date / downgrade paths can run. The other
#  three raise the constant (exactly what a future firmware release does) and
#  rename the entry point, which is its only extern symbol, so the objects link
#  together without clashing:
#
#      nvs_migration.o             nvs_migration_run
#      nvs_migration_v2.o          nvs_migration_run_compiled_v2
#      nvs_migration_v3.o          nvs_migration_run_compiled_v3
#      nvs_migration_v4.o          nvs_migration_run_compiled_v4
#
#  host_compat/ is searched BEFORE ../main so its stub esp_err.h/esp_log.h/nvs.h
#  shadow the ESP-IDF ones, while the real spaxel.h/nvs_migration.h still
#  resolve. The same -I is applied to the test objects, which is harmless: no
#  other test includes an esp_*/nvs header.
# ============================================================================

NVS_MIG_SRC   := ../main/nvs_migration.c
NVS_MIG_HDRS  := ../main/nvs_migration.h ../main/spaxel.h
HOST_COMPAT   := $(wildcard host_compat/*.h host_compat/freertos/*.h)
NVS_MIG_INC   := -Ihost_compat -I../main

# The production TU keeps -Wextra but drops -Wunused-parameter: the placeholder
# migrate_v2_to_v3() takes the handle and does nothing with it yet, and the
# ESP-IDF build (which compiles this same file) does not pass -Wextra, so this
# is an artifact of the host's stricter flags rather than a code defect.
NVS_MIG_CFLAGS := -Wno-unused-parameter

NVS_MIG_OBJS := $(BUILD_DIR)/nvs_migration.o

$(BUILD_DIR)/nvs_migration.o: $(NVS_MIG_SRC) $(NVS_MIG_HDRS) $(HOST_COMPAT)
	@mkdir -p $(BUILD_DIR)
	$(CC) $(CFLAGS) $(NVS_MIG_INC) $(NVS_MIG_CFLAGS) -c $< -o $@

# Unflagged: this is the shipped build, so it takes COMPILED_NVS_VERSION's
# default from nvs_migration.h — which also proves that #ifndef guard holds.
# The _vN_ objects are the same TU at a raised version with the entry point
# renamed, making the migration loop (dead code at v1) testable.
NVS_MIG_OBJS += $(BUILD_DIR)/nvs_migration_v2.o
$(BUILD_DIR)/nvs_migration_v2.o: $(NVS_MIG_SRC) $(NVS_MIG_HDRS) $(HOST_COMPAT)
	@mkdir -p $(BUILD_DIR)
	$(CC) $(CFLAGS) $(NVS_MIG_INC) $(NVS_MIG_CFLAGS) -DCOMPILED_NVS_VERSION=2 \
		-Dnvs_migration_run=nvs_migration_run_compiled_v2 -c $< -o $@

NVS_MIG_OBJS += $(BUILD_DIR)/nvs_migration_v3.o
$(BUILD_DIR)/nvs_migration_v3.o: $(NVS_MIG_SRC) $(NVS_MIG_HDRS) $(HOST_COMPAT)
	@mkdir -p $(BUILD_DIR)
	$(CC) $(CFLAGS) $(NVS_MIG_INC) $(NVS_MIG_CFLAGS) -DCOMPILED_NVS_VERSION=3 \
		-Dnvs_migration_run=nvs_migration_run_compiled_v3 -c $< -o $@

NVS_MIG_OBJS += $(BUILD_DIR)/nvs_migration_v4.o
$(BUILD_DIR)/nvs_migration_v4.o: $(NVS_MIG_SRC) $(NVS_MIG_HDRS) $(HOST_COMPAT)
	@mkdir -p $(BUILD_DIR)
	$(CC) $(CFLAGS) $(NVS_MIG_INC) $(NVS_MIG_CFLAGS) -DCOMPILED_NVS_VERSION=4 \
		-Dnvs_migration_run=nvs_migration_run_compiled_v4 -c $< -o $@

# ============================================================================
#  Build rules
# ============================================================================

TEST_OBJS := $(patsubst %.c,$(BUILD_DIR)/%.o,$(TEST_SRCS)) $(BUILD_DIR)/$(RUNNER_SRC:.c=.o)
ALL_OBJS  := $(TEST_OBJS) $(NVS_MIG_OBJS)

# Object files (not archives) are linked directly, so every test_*.c's TEST()
# constructor is retained and runs at load — no --whole-archive needed. Only
# unused *archive* members get dropped by the linker, never direct .o files.
.PHONY: test build run clean

# Default goal: the documented single command. `build` produces the binary,
# `run` executes it; make returns the binary's exit code, so failures surface.
test: build run

build: $(BIN)

$(BUILD_DIR)/%.o: %.c test_runner.h $(HOST_COMPAT)
	@mkdir -p $(BUILD_DIR)
	$(CC) $(CFLAGS) $(NVS_MIG_INC) -c $< -o $@

$(BIN): $(ALL_OBJS)
	$(CC) $(CFLAGS) $^ -o $@ $(LDFLAGS)

run: $(BIN)
	./$(BIN)

clean:
	rm -rf $(BUILD_DIR)
