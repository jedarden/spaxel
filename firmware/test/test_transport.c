/*
 * ============================================================================
 *  Provisioning transport backend table — host contract test (spaxel-db7aab09,
 *  retires the parked parent spaxel-5c56d980)
 * ============================================================================
 *
 *  WHY THIS TEST EXISTS
 *  --------------------
 *  firmware/main/transport.c is the provisioning link's backend table: the
 *  same framing/NVS parser code must work over UART0 (bridge-equipped boards)
 *  and native USB-Serial/JTAG (the current caller's choice). A wrong baud
 *  config, a missing TX wait, or a write routed to the wrong peripheral does
 *  not fail loudly — provisioning just never answers, and the node looks
 *  dead. What is pinned here:
 *
 *    - the two singletons are stable and carry the backend decision in their
 *      function tables (distinct write implementations, recorded names);
 *    - uart0 init configures exactly 115200 8N1, no flow control, default
 *      clock, and installs the driver with 512-byte RX/TX buffers on
 *      UART_NUM_0 — and is idempotent while installed;
 *    - deinit uninstalls and a later init reinstalls (the provisioning
 *      window reopens fresh — see transport.c's usb flush comment);
 *    - init propagates a param-config or driver-install failure and leaves
 *      the backend uninstalled;
 *    - uart0 write is bytes-then-wait: a failed TX wait is reported as -1,
 *      and a driver-level negative write propagates WITHOUT a wait call;
 *    - usb-serial-jtag writes route to the USB fake and never touch the UART
 *      one (and vice versa for reads), carry the default 1 KiB driver
 *      config, and flush is unconditionally ESP_OK with no UART input flush
 *      — the driver has no input-flush API, which the NULL-router assertion
 *      below pins against accidental cross-wiring;
 *    - the wrapper layer rejects NULL transports, missing callbacks and NULL
 *      buffers (INVALID_ARG for init/flush, -1 for read/write) instead of
 *      dereferencing them.
 *
 *  HOW IT COMPILES
 *  ---------------
 *  The production TU is included directly:
 *
 *      #include "../main/transport.c"
 *
 *  following the watchdog precedent, so the test exercises the real table
 *  rather than a copy of it, and the Makefile's test_*.c wildcard needs no
 *  per-test wiring. transport.c's IDF includes (driver/uart.h,
 *  driver/usb_serial_jtag.h) resolve to the recording fakes in stubs/driver/
 *  via a target-specific include override for this object in the Makefile.
 *  Nothing here is reachable from the ESP-IDF build: no CMake file references
 *  firmware/test.
 *  ============================================================================
 */
#include "test_runner.h"

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>
#include <string.h>

#include "stubs/esp_err.h"
#include "stubs/freertos/FreeRTOS.h"
#include "stubs/freertos/task.h"

/* ---- The code under test --------------------------------------------------- */

#include "../main/transport.c"

/*
 * transport.c keeps its installed flags in file-scope statics, which became
 * part of this translation unit with the include above. The suite restores
 * them — and every fake — directly between tests instead of relying on TEST()
 * registration (link) order, so every test below is order-independent.
 */
static void reset_transport_under_test(void)
{
    s_uart0_installed = false;
    s_usb_serial_jtag_installed = false;
    uart_mock_reset_all();
    usb_jtag_mock_reset_all();
}

/* ---- The contract ----------------------------------------------------------- */

/*
 * The two singletons are the backend decision: stable addresses (callers hold
 * the pointer across the provisioning window), honest names for the logs, and
 * — the actual routing decision — distinct write implementations.
 */
TEST(transport_backends_are_distinct_stable_singletons)
{
    reset_transport_under_test();

    transport_t *uart0 = transport_uart0();
    transport_t *usb   = transport_usb_serial_jtag();

    ASSERT_TRUE(uart0 != NULL);
    ASSERT_TRUE(usb != NULL);
    ASSERT_TRUE(transport_uart0() == uart0);
    ASSERT_TRUE(transport_usb_serial_jtag() == usb);
    ASSERT_TRUE(uart0 != usb);

    ASSERT_TRUE(strcmp(uart0->name, "uart0") == 0);
    ASSERT_TRUE(strcmp(usb->name, "usb-serial-jtag") == 0);

    /* Different backends must write through different code paths. */
    ASSERT_TRUE(uart0->write != usb->write);
    ASSERT_TRUE(uart0->read != usb->read);
    ASSERT_TRUE(uart0->init != usb->init);
}

/*
 * The UART0 provisioning contract: exactly 115200 8N1, no flow control,
 * default clock source, driver installed on UART_NUM_0 with the module's
 * 512-byte RX and TX buffers.
 */
TEST(transport_uart0_init_installs_115200_8n1_with_512_byte_buffers)
{
    reset_transport_under_test();

    ASSERT_EQ(transport_init(transport_uart0()), ESP_OK);

    ASSERT_EQ(uart_mock_param_config_calls, 1);
    ASSERT_EQ(uart_mock_param_config_port, UART_NUM_0);
    ASSERT_EQ(uart_mock_param_config_cfg.baud_rate, 115200);
    ASSERT_EQ(uart_mock_param_config_cfg.data_bits, UART_DATA_8_BITS);
    ASSERT_EQ(uart_mock_param_config_cfg.parity, UART_PARITY_DISABLE);
    ASSERT_EQ(uart_mock_param_config_cfg.stop_bits, UART_STOP_BITS_1);
    ASSERT_EQ(uart_mock_param_config_cfg.flow_ctrl, UART_HW_FLOWCTRL_DISABLE);
    ASSERT_EQ(uart_mock_param_config_cfg.source_clk, UART_SCLK_DEFAULT);

    ASSERT_EQ(uart_mock_driver_install_calls, 1);
    ASSERT_EQ(uart_mock_driver_install_port, UART_NUM_0);
    ASSERT_EQ(uart_mock_driver_install_rx_buf, 512);
    ASSERT_EQ(uart_mock_driver_install_tx_buf, 512);
}

/* Re-init while installed must not reinstall the driver underneath callers. */
TEST(transport_uart0_init_is_idempotent_while_installed)
{
    reset_transport_under_test();

    ASSERT_EQ(transport_init(transport_uart0()), ESP_OK);
    int installs = uart_mock_driver_install_calls;

    ASSERT_EQ(transport_init(transport_uart0()), ESP_OK);
    ASSERT_EQ(uart_mock_driver_install_calls, installs);
    ASSERT_EQ(uart_mock_param_config_calls, 1);
}

/*
 * Closing the provisioning window uninstalls; reopening it reinstalls. A
 * stale install across windows would answer nothing — the reopen-fresh
 * guarantee the USB flush comment leans on.
 */
TEST(transport_uart0_deinit_uninstalls_and_reinit_reinstalls)
{
    reset_transport_under_test();

    ASSERT_EQ(transport_init(transport_uart0()), ESP_OK);
    transport_deinit(transport_uart0());

    ASSERT_EQ(uart_mock_driver_delete_calls, 1);
    ASSERT_EQ(uart_mock_driver_delete_port, UART_NUM_0);

    ASSERT_EQ(transport_init(transport_uart0()), ESP_OK);
    ASSERT_EQ(uart_mock_driver_install_calls, 2);
}

/* A config failure must not leave a driver installed against nothing. */
TEST(transport_uart0_init_propagates_param_config_failure)
{
    reset_transport_under_test();
    uart_mock_param_config_ret = ESP_ERR_INVALID_ARG;

    ASSERT_EQ(transport_init(transport_uart0()), ESP_ERR_INVALID_ARG);
    ASSERT_EQ(uart_mock_driver_install_calls, 0);

    /* Still uninstalled: a later init really tries again. */
    ASSERT_EQ(uart_mock_param_config_calls, 1);
    uart_mock_param_config_ret = ESP_OK;
    ASSERT_EQ(transport_init(transport_uart0()), ESP_OK);
    ASSERT_EQ(uart_mock_driver_install_calls, 1);
}

TEST(transport_uart0_init_propagates_driver_install_failure)
{
    reset_transport_under_test();
    uart_mock_driver_install_ret = ESP_ERR_NO_MEM;

    ASSERT_EQ(transport_init(transport_uart0()), ESP_ERR_NO_MEM);
    ASSERT_EQ(uart_mock_param_config_calls, 1);
    ASSERT_EQ(uart_mock_driver_install_calls, 1);

    /* Uninstalled: the next init re-runs the whole sequence. */
    uart_mock_driver_install_ret = ESP_OK;
    ASSERT_EQ(transport_init(transport_uart0()), ESP_OK);
    ASSERT_EQ(uart_mock_driver_install_calls, 2);
}

/*
 * The uart0 write contract: the bytes go out first, then the driver is
 * waited on with the caller's timeout, and only then is the count reported.
 * The wait is what keeps a provisioning reply from being truncated by a
 * deinit racing the TX ring buffer.
 */
TEST(transport_uart0_write_sends_bytes_then_waits_for_tx)
{
    reset_transport_under_test();
    ASSERT_EQ(transport_init(transport_uart0()), ESP_OK);

    const uint8_t frame[] = {'S', 'P', 'X', 0x01, 0x02};
    uart_mock_write_bytes_ret = (int)sizeof(frame);

    ASSERT_EQ(transport_write(transport_uart0(), frame, sizeof(frame), 42),
              (int)sizeof(frame));

    ASSERT_EQ(uart_mock_write_calls, 1);
    ASSERT_EQ(uart_mock_write_log[0].port, UART_NUM_0);
    ASSERT_EQ(uart_mock_write_log[0].len, sizeof(frame));
    ASSERT_TRUE(memcmp(uart_mock_write_log[0].bytes, frame, sizeof(frame)) == 0);

    ASSERT_EQ(uart_mock_wait_tx_done_calls, 1);
    ASSERT_EQ(uart_mock_wait_tx_done_port, UART_NUM_0);
    ASSERT_EQ(uart_mock_wait_tx_done_timeout, 42);
}

/* A TX wait that never completes means the write did not happen — -1, the
 * caller's retry signal, not a partial-success count. */
TEST(transport_uart0_write_reports_failure_when_tx_wait_times_out)
{
    reset_transport_under_test();
    ASSERT_EQ(transport_init(transport_uart0()), ESP_OK);

    const uint8_t frame[] = {0xAA, 0xBB};
    uart_mock_write_bytes_ret = (int)sizeof(frame);
    uart_mock_wait_tx_done_ret = ESP_ERR_INVALID_STATE;

    ASSERT_EQ(transport_write(transport_uart0(), frame, sizeof(frame), 10), -1);
    ASSERT_EQ(uart_mock_write_calls, 1);
    ASSERT_EQ(uart_mock_wait_tx_done_calls, 1);
}

/* A driver-level write error propagates verbatim, with no wait behind it. */
TEST(transport_uart0_write_propagates_driver_error_without_waiting)
{
    reset_transport_under_test();
    ASSERT_EQ(transport_init(transport_uart0()), ESP_OK);

    const uint8_t frame[] = {0xCC};
    uart_mock_write_bytes_ret = -1;

    ASSERT_EQ(transport_write(transport_uart0(), frame, sizeof(frame), 10), -1);
    ASSERT_EQ(uart_mock_write_calls, 1);
    ASSERT_EQ(uart_mock_wait_tx_done_calls, 0);
}

TEST(transport_uart0_read_and_flush_route_to_the_uart)
{
    reset_transport_under_test();
    ASSERT_EQ(transport_init(transport_uart0()), ESP_OK);

    uint8_t buf[8];
    uart_mock_read_bytes_ret = 5;
    ASSERT_EQ(transport_read(transport_uart0(), buf, sizeof(buf), 7), 5);
    ASSERT_EQ(uart_mock_read_calls, 1);

    uart_mock_flush_input_ret = ESP_ERR_INVALID_STATE;
    ASSERT_EQ(transport_flush(transport_uart0()), ESP_ERR_INVALID_STATE);
    ASSERT_EQ(uart_mock_flush_input_calls, 1);
    ASSERT_EQ(uart_mock_flush_input_port, UART_NUM_0);
}

/*
 * The USB backend must install with the DEFAULT driver config — the module
 * does not tune the buffer sizes, so a change to the default macro or a
 * hand-rolled config shows up here.
 */
TEST(transport_usb_serial_jtag_installs_with_default_driver_config)
{
    reset_transport_under_test();

    ASSERT_EQ(transport_init(transport_usb_serial_jtag()), ESP_OK);

    ASSERT_EQ(usb_jtag_mock_install_calls, 1);
    ASSERT_EQ(usb_jtag_mock_install_cfg.tx_buffer_size, 1024);
    ASSERT_EQ(usb_jtag_mock_install_cfg.rx_buffer_size, 1024);

    /* ...and never touches the UART backend. */
    ASSERT_EQ(uart_mock_param_config_calls, 0);
    ASSERT_EQ(uart_mock_driver_install_calls, 0);
}

/*
 * The routing decision this whole table exists for: after selecting
 * usb-serial-jtag, bytes reach the USB fake and never the UART one.
 */
TEST(transport_usb_serial_jtag_write_routes_to_usb_not_uart)
{
    reset_transport_under_test();
    ASSERT_EQ(transport_init(transport_usb_serial_jtag()), ESP_OK);

    const uint8_t frame[] = {'O', 'K', ':'};
    ASSERT_EQ(transport_write(transport_usb_serial_jtag(), frame,
                              sizeof(frame), 33), (int)sizeof(frame));

    ASSERT_EQ(usb_jtag_mock_write_calls, 1);
    ASSERT_EQ(usb_jtag_mock_last_write_len, sizeof(frame));
    ASSERT_TRUE(memcmp(usb_jtag_mock_last_write, frame, sizeof(frame)) == 0);

    /* The other backend saw nothing, and there is no TX-wait on this
     * peripheral — the write returns what the driver accepted. */
    ASSERT_EQ(uart_mock_write_calls, 0);
    ASSERT_EQ(uart_mock_wait_tx_done_calls, 0);
}

/*
 * The USB-Serial/JTAG driver has no input-flush API: flush must be an
 * unconditional ESP_OK that does NOT reach for a UART call — a cross-wired
 * flush here would drain the wrong peripheral's RX buffer.
 */
TEST(transport_usb_serial_jtag_flush_is_always_ok_and_never_touches_uart)
{
    reset_transport_under_test();
    ASSERT_EQ(transport_init(transport_usb_serial_jtag()), ESP_OK);

    uint8_t buf[4];
    usb_jtag_mock_read_ret = 3;
    ASSERT_EQ(transport_read(transport_usb_serial_jtag(), buf, sizeof(buf), 9), 3);
    ASSERT_EQ(usb_jtag_mock_read_calls, 1);
    ASSERT_EQ(uart_mock_read_calls, 0);

    ASSERT_EQ(transport_flush(transport_usb_serial_jtag()), ESP_OK);
    ASSERT_EQ(uart_mock_flush_input_calls, 0);
}

TEST(transport_usb_serial_jtag_deinit_uninstalls_and_reinit_reinstalls)
{
    reset_transport_under_test();

    ASSERT_EQ(transport_init(transport_usb_serial_jtag()), ESP_OK);
    transport_deinit(transport_usb_serial_jtag());
    ASSERT_EQ(usb_jtag_mock_uninstall_calls, 1);

    ASSERT_EQ(transport_init(transport_usb_serial_jtag()), ESP_OK);
    ASSERT_EQ(usb_jtag_mock_install_calls, 2);
}

/* A short USB write (driver accepted fewer bytes) propagates as the count. */
TEST(transport_usb_serial_jtag_write_propagates_short_write)
{
    reset_transport_under_test();
    ASSERT_EQ(transport_init(transport_usb_serial_jtag()), ESP_OK);

    const uint8_t frame[] = {1, 2, 3, 4};
    usb_jtag_mock_write_ret = 2;

    ASSERT_EQ(transport_write(transport_usb_serial_jtag(), frame,
                              sizeof(frame), 5), 2);
}

/* ---- Wrapper guards ---------------------------------------------------------- */

/*
 * The wrapper layer is what the provisioning parser actually calls; a NULL
 * transport or a backend with a missing op must be refused (-1 for the
 * read/write pair, INVALID_ARG for init/flush) rather than dereferenced.
 */
TEST(transport_wrappers_reject_null_transport_and_missing_callbacks)
{
    reset_transport_under_test();

    uint8_t buf[4];

    /* NULL transport. */
    ASSERT_EQ(transport_init(NULL), ESP_ERR_INVALID_ARG);
    ASSERT_EQ(transport_write(NULL, buf, sizeof(buf), 1), -1);
    ASSERT_EQ(transport_read(NULL, buf, sizeof(buf), 1), -1);
    ASSERT_EQ(transport_flush(NULL), ESP_ERR_INVALID_ARG);
    transport_deinit(NULL); /* must not crash */

    /* Present transport, missing callback. */
    transport_t half = {0};
    ASSERT_EQ(transport_init(&half), ESP_ERR_INVALID_ARG);
    ASSERT_EQ(transport_write(&half, buf, sizeof(buf), 1), -1);
    ASSERT_EQ(transport_read(&half, buf, sizeof(buf), 1), -1);
    ASSERT_EQ(transport_flush(&half), ESP_ERR_INVALID_ARG);

    /* Nothing above may have touched either backend fake. */
    ASSERT_EQ(uart_mock_write_calls, 0);
    ASSERT_EQ(uart_mock_read_calls, 0);
    ASSERT_EQ(uart_mock_driver_install_calls, 0);
    ASSERT_EQ(usb_jtag_mock_write_calls, 0);
    ASSERT_EQ(usb_jtag_mock_install_calls, 0);
}

/* NULL buffers are refused before the callback runs. */
TEST(transport_wrappers_reject_null_buffers)
{
    reset_transport_under_test();
    ASSERT_EQ(transport_init(transport_uart0()), ESP_OK);

    const uint8_t frame[] = {0x01};
    ASSERT_EQ(transport_write(transport_uart0(), NULL, sizeof(frame), 1), -1);
    ASSERT_EQ(transport_read(transport_uart0(), NULL, sizeof(frame), 1), -1);

    ASSERT_EQ(uart_mock_write_calls, 0);
    ASSERT_EQ(uart_mock_read_calls, 0);
}
