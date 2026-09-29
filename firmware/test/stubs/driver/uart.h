/*
 * Host stand-in for ESP-IDF's driver/uart.h — the recording fake slice that
 * firmware/main/transport.c reaches (spaxel-db7aab09, esp_task_wdt.h
 * precedent).
 *
 * Every call is recorded (config struct copies, install arguments, a byte
 * log for writes/reads) and every outcome is test-driven through the
 * uart_mock_*_ret knobs, so test_transport.c can pin the backend decision
 * table — 115200 8N1 no-flow on UART_NUM_0 with 512-byte driver buffers —
 * and the write contract (bytes then wait_tx_done; a failed wait is -1)
 * without a UART peripheral. Like esp_task_wdt.h, state and mocks are static:
 * transport.c is compiled into the single test TU that includes this header,
 * so nothing escapes it and nothing collides with another stub dialect.
 */
#ifndef SPAXEL_TEST_STUB_DRIVER_UART_H
#define SPAXEL_TEST_STUB_DRIVER_UART_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include "esp_err.h"
#include "freertos/FreeRTOS.h"

/* ---- Types (mirrors of the IDF enums transport.c names) ------------------- */

typedef int uart_port_t;

typedef enum { UART_DATA_5_BITS = 0, UART_DATA_6_BITS, UART_DATA_7_BITS,
               UART_DATA_8_BITS } uart_word_length_t;
typedef enum { UART_PARITY_DISABLE = 0, UART_PARITY_EVEN, UART_PARITY_ODD
} uart_parity_t;
typedef enum { UART_STOP_BITS_1 = 1, UART_STOP_BITS_1_5, UART_STOP_BITS_2
} uart_stop_bits_t;
typedef enum { UART_HW_FLOWCTRL_DISABLE = 0, UART_HW_FLOWCTRL_RTS,
               UART_HW_FLOWCTRL_CTS, UART_HW_FLOWCTRL_CTS_RTS
} uart_hw_flowcontrol_t;
typedef enum { UART_SCLK_DEFAULT = 0 } uart_sclk_t;

#define UART_NUM_0 ((uart_port_t)0)

typedef struct {
    int baud_rate;
    uart_word_length_t data_bits;
    uart_parity_t      parity;
    uart_stop_bits_t   stop_bits;
    uart_hw_flowcontrol_t flow_ctrl;
    uart_sclk_t        source_clk;
} uart_config_t;

/* ---- Recorded calls -------------------------------------------------------- */

#define UART_MOCK_WRITE_LOG 256

typedef struct {
    uart_port_t  port;
    uint8_t      bytes[UART_MOCK_WRITE_LOG];
    size_t       len;
} uart_mock_write_t;

__attribute__((unused))
static int               uart_mock_param_config_calls;
__attribute__((unused))
static uart_port_t       uart_mock_param_config_port;
__attribute__((unused))
static uart_config_t     uart_mock_param_config_cfg;

__attribute__((unused))
static int          uart_mock_driver_install_calls;
__attribute__((unused))
static uart_port_t  uart_mock_driver_install_port;
__attribute__((unused))
static int          uart_mock_driver_install_rx_buf;
__attribute__((unused))
static int          uart_mock_driver_install_tx_buf;

__attribute__((unused))
static int         uart_mock_driver_delete_calls;
__attribute__((unused))
static uart_port_t uart_mock_driver_delete_port;

__attribute__((unused))
static uart_mock_write_t uart_mock_write_log[UART_MOCK_WRITE_LOG / 16];
__attribute__((unused))
static int               uart_mock_write_calls;

__attribute__((unused))
static int          uart_mock_wait_tx_done_calls;
__attribute__((unused))
static uart_port_t  uart_mock_wait_tx_done_port;
__attribute__((unused))
static TickType_t   uart_mock_wait_tx_done_timeout;

__attribute__((unused))
static int  uart_mock_read_calls;
__attribute__((unused))
static int  uart_mock_flush_input_calls;
__attribute__((unused))
static uart_port_t uart_mock_flush_input_port;

/* ---- Return values the test drives ------------------------------------------ */

__attribute__((unused))
static esp_err_t uart_mock_param_config_ret = ESP_OK;
__attribute__((unused))
static esp_err_t uart_mock_driver_install_ret = ESP_OK;
__attribute__((unused))
static int       uart_mock_write_bytes_ret = -2; /* <0 default; tests set the
                                                  * byte count they want */
__attribute__((unused))
static esp_err_t uart_mock_wait_tx_done_ret = ESP_OK;
__attribute__((unused))
static int       uart_mock_read_bytes_ret = 0;
__attribute__((unused))
static esp_err_t uart_mock_flush_input_ret = ESP_OK;

/* ---- Reset ------------------------------------------------------------------- */

__attribute__((unused))
static void uart_mock_reset_all(void)
{
    uart_mock_param_config_calls = 0;
    uart_mock_param_config_port  = 0;
    memset(&uart_mock_param_config_cfg, 0, sizeof(uart_mock_param_config_cfg));

    uart_mock_driver_install_calls = 0;
    uart_mock_driver_install_port  = 0;
    uart_mock_driver_install_rx_buf = 0;
    uart_mock_driver_install_tx_buf = 0;

    uart_mock_driver_delete_calls = 0;
    uart_mock_driver_delete_port  = 0;

    memset(uart_mock_write_log, 0, sizeof(uart_mock_write_log));
    uart_mock_write_calls = 0;

    uart_mock_wait_tx_done_calls   = 0;
    uart_mock_wait_tx_done_port    = 0;
    uart_mock_wait_tx_done_timeout = 0;

    uart_mock_read_calls  = 0;
    uart_mock_flush_input_calls = 0;
    uart_mock_flush_input_port  = 0;

    uart_mock_param_config_ret   = ESP_OK;
    uart_mock_driver_install_ret = ESP_OK;
    uart_mock_write_bytes_ret    = -2;
    uart_mock_wait_tx_done_ret   = ESP_OK;
    uart_mock_read_bytes_ret     = 0;
    uart_mock_flush_input_ret    = ESP_OK;
}

/* ---- The mocked API ------------------------------------------------------------ */

__attribute__((unused))
static esp_err_t uart_param_config(uart_port_t uart_num,
                                   const uart_config_t *uart_config)
{
    uart_mock_param_config_port = uart_num;
    if (uart_config) {
        uart_mock_param_config_cfg = *uart_config;
    }
    uart_mock_param_config_calls++;
    return uart_mock_param_config_ret;
}

__attribute__((unused))
static esp_err_t uart_driver_install(uart_port_t uart_num, int rx_buffer_size,
                                     int tx_buffer_size, int event_queue_size,
                                     void *event_queue_handle, int intr_flags)
{
    (void)event_queue_size;
    (void)event_queue_handle;
    (void)intr_flags;
    uart_mock_driver_install_port    = uart_num;
    uart_mock_driver_install_rx_buf  = rx_buffer_size;
    uart_mock_driver_install_tx_buf  = tx_buffer_size;
    uart_mock_driver_install_calls++;
    return uart_mock_driver_install_ret;
}

__attribute__((unused))
static esp_err_t uart_driver_delete(uart_port_t uart_num)
{
    uart_mock_driver_delete_port = uart_num;
    uart_mock_driver_delete_calls++;
    return ESP_OK;
}

__attribute__((unused))
static int uart_write_bytes(uart_port_t uart_num, const char *data,
                            uint32_t len)
{
    uart_mock_write_t *entry = &uart_mock_write_log[uart_mock_write_calls %
        (int)(sizeof(uart_mock_write_log) / sizeof(uart_mock_write_log[0]))];
    entry->port = uart_num;
    entry->len  = (len > UART_MOCK_WRITE_LOG) ? UART_MOCK_WRITE_LOG : len;
    memcpy(entry->bytes, data, entry->len);
    uart_mock_write_calls++;
    /* The byte-count knob stands in for "what the driver accepted"; a length
     * was never recorded through it. */
    return (uart_mock_write_bytes_ret == -2) ? (int)len
                                             : uart_mock_write_bytes_ret;
}

__attribute__((unused))
static esp_err_t uart_wait_tx_done(uart_port_t uart_num, TickType_t timeout)
{
    uart_mock_wait_tx_done_port    = uart_num;
    uart_mock_wait_tx_done_timeout = timeout;
    uart_mock_wait_tx_done_calls++;
    return uart_mock_wait_tx_done_ret;
}

__attribute__((unused))
static int uart_read_bytes(uart_port_t uart_num, uint8_t *buf, uint32_t length,
                           TickType_t timeout)
{
    (void)uart_num;
    (void)buf;
    (void)timeout;
    (void)length;
    uart_mock_read_calls++;
    return uart_mock_read_bytes_ret;
}

__attribute__((unused))
static esp_err_t uart_flush_input(uart_port_t uart_num)
{
    uart_mock_flush_input_port = uart_num;
    uart_mock_flush_input_calls++;
    return uart_mock_flush_input_ret;
}

#endif /* SPAXEL_TEST_STUB_DRIVER_UART_H */
