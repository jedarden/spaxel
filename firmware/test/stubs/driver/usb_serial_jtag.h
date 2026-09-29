/*
 * Host stand-in for ESP-IDF's driver/usb_serial_jtag.h — the recording fake
 * slice firmware/main/transport.c reaches (spaxel-db7aab09, esp_task_wdt.h
 * precedent).
 *
 * The native USB-Serial/JTAG peripheral does not exist on a host; what the
 * transport test pins is that the USB backend installs with the default
 * driver config and that reads/writes route here — and NOT to the UART fake —
 * once this backend is selected. Static state, single-TU inclusion, same as
 * the uart.h stand-in beside it.
 */
#ifndef SPAXEL_TEST_STUB_DRIVER_USB_SERIAL_JTAG_H
#define SPAXEL_TEST_STUB_DRIVER_USB_SERIAL_JTAG_H

#include <stdbool.h>
#include <stddef.h>
#include <stdint.h>

#include "esp_err.h"
#include "freertos/FreeRTOS.h"

typedef struct {
    int tx_buffer_size;
    int rx_buffer_size;
} usb_serial_jtag_driver_config_t;

/* Mirrors the IDF default (1 KiB each way). */
#define USB_SERIAL_JTAG_DRIVER_CONFIG_DEFAULT() \
    { .tx_buffer_size = 1024, .rx_buffer_size = 1024 }

/* ---- Recorded calls -------------------------------------------------------- */

__attribute__((unused))
static int usb_jtag_mock_install_calls;
__attribute__((unused))
static usb_serial_jtag_driver_config_t usb_jtag_mock_install_cfg;

__attribute__((unused))
static int usb_jtag_mock_uninstall_calls;

__attribute__((unused))
static int usb_jtag_mock_read_calls;

__attribute__((unused))
static uint8_t usb_jtag_mock_last_write[256];
__attribute__((unused))
static size_t  usb_jtag_mock_last_write_len;
__attribute__((unused))
static int     usb_jtag_mock_write_calls;

/* ---- Return values the test drives ------------------------------------------ */

__attribute__((unused))
static esp_err_t usb_jtag_mock_install_ret = ESP_OK;
__attribute__((unused))
static int       usb_jtag_mock_write_ret = -2; /* -2 default: echo the length */
__attribute__((unused))
static int       usb_jtag_mock_read_ret = 0;

/* ---- Reset ------------------------------------------------------------------- */

__attribute__((unused))
static void usb_jtag_mock_reset_all(void)
{
    usb_jtag_mock_install_calls = 0;
    memset(&usb_jtag_mock_install_cfg, 0, sizeof(usb_jtag_mock_install_cfg));
    usb_jtag_mock_uninstall_calls = 0;
    usb_jtag_mock_read_calls = 0;
    memset(usb_jtag_mock_last_write, 0, sizeof(usb_jtag_mock_last_write));
    usb_jtag_mock_last_write_len = 0;
    usb_jtag_mock_write_calls = 0;
    usb_jtag_mock_install_ret = ESP_OK;
    usb_jtag_mock_write_ret   = -2;
    usb_jtag_mock_read_ret    = 0;
}

/* ---- The mocked API ------------------------------------------------------------ */

__attribute__((unused))
static esp_err_t usb_serial_jtag_driver_install(
    const usb_serial_jtag_driver_config_t *usb_serial_jtag_config)
{
    if (usb_serial_jtag_config) {
        usb_jtag_mock_install_cfg = *usb_serial_jtag_config;
    }
    usb_jtag_mock_install_calls++;
    return usb_jtag_mock_install_ret;
}

__attribute__((unused))
static esp_err_t usb_serial_jtag_driver_uninstall(void)
{
    usb_jtag_mock_uninstall_calls++;
    return ESP_OK;
}

__attribute__((unused))
static int usb_serial_jtag_read_bytes(uint8_t *buf, uint32_t length,
                                      TickType_t timeout)
{
    (void)buf;
    (void)length;
    (void)timeout;
    usb_jtag_mock_read_calls++;
    return usb_jtag_mock_read_ret;
}

__attribute__((unused))
static int usb_serial_jtag_write_bytes(const uint8_t *buf, uint32_t length,
                                       TickType_t timeout)
{
    (void)timeout;
    usb_jtag_mock_last_write_len = (length > sizeof(usb_jtag_mock_last_write))
        ? sizeof(usb_jtag_mock_last_write) : length;
    memcpy(usb_jtag_mock_last_write, buf, usb_jtag_mock_last_write_len);
    usb_jtag_mock_write_calls++;
    return (usb_jtag_mock_write_ret == -2) ? (int)length
                                           : usb_jtag_mock_write_ret;
}

#endif /* SPAXEL_TEST_STUB_DRIVER_USB_SERIAL_JTAG_H */
