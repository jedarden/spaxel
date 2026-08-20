# Console default verification state (spaxel-5049d982)

Record: 2026-08-20. This note states plainly what is verified, how, and what
remains a bench step — so the next person does not re-derive it from the bead
history (three workers released this bead before one was completed; each had
to rediscover the same split between "verified" and "needs a board").

## What landed

- `af303e6` (2026-07-31): default console switched to USB-Serial/JTAG in
  `firmware/sdkconfig.defaults`, plus the `firmware/sdkconfig.uart-console`
  override layer and ADR-002 decisions 3/4.
- `2ab43ab` (2026-08-16): README build instructions for both profiles,
  `firmware/scripts/verify-console-config.sh`, explicit `=n` lines and
  `CONFIG_ESP_SYSTEM_PANIC_PRINT_REBOOT=y` in both files.

## Verified at config level (2026-08-20, this host)

No board, no ESP-IDF on this host — everything below is config-level:

- **Host tests** (`firmware/test/test_console_config.c`, part of
  `make -C firmware/test test`): the committed defaults select
  USB-Serial/JTAG; the override file selects UART0; layering the override over
  the defaults via last-definition-wins yields the UART0 profile for every
  console key; neither profile ever selects a silent/halting panic behaviour.
  Suite: 57 tests, exit 0.
- **Script, both branches**: `verify-console-config.sh` run against synthetic
  generated-style fixtures — `uart` profile passes on a UART0 config and fails
  on a USB config naming the exact missing line (`exit 1`); `usb` profile
  passes on a USB config (also previously verified against the real generated
  `firmware/sdkconfig`); missing file fails with "Run an ESP-IDF build first."

## Not verifiable from this host — remains a bench step

AC3 of the bead (panic backtrace visible after a deliberate fault) requires a
flashed board. The procedure is documented and deliberate:
`firmware/README.md` § "Console panic check" (temporary
`esp_system_abort("console panic probe")`, confirm `Guru Meditation Error` /
`Backtrace:`, remove the probe). The on-device boot-log evidence for AC1/AC2
(2026-07-30/31 bring-up, ~49 bytes vs. full log) is recorded in the bead text
and ADR-002.

**Do not close another console bead for this reason without a board attached**
— the config-level half is now guarded by tests; only the on-device panic
probe is outstanding, and it cannot be done from a host without hardware.
