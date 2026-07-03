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
ALL_SRCS  := $(TEST_SRCS) $(RUNNER_SRC)

BUILD_DIR := build
BIN       := $(BUILD_DIR)/spaxel_host_tests

# Object files (not archives) are linked directly, so every test_*.c's TEST()
# constructor is retained and runs at load — no --whole-archive needed. Only
# unused *archive* members get dropped by the linker, never direct .o files.
.PHONY: test build run clean

# Default goal: the documented single command. `build` produces the binary,
# `run` executes it; make returns the binary's exit code, so failures surface.
test: build run

build: $(BIN)

$(BIN): $(ALL_SRCS) test_runner.h
	@mkdir -p $(BUILD_DIR)
	$(CC) $(CFLAGS) $(ALL_SRCS) -o $@ $(LDFLAGS)

run: $(BIN)
	./$(BIN)

clean:
	rm -rf $(BUILD_DIR)
