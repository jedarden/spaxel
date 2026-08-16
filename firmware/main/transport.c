#include "transport.h"

#include "driver/uart.h"
#include "driver/usb_serial_jtag.h"

#define TRANSPORT_BUFFER_SIZE 512

static bool s_uart0_installed;
static bool s_usb_serial_jtag_installed;

static esp_err_t uart0_init(void) {
    if (s_uart0_installed) {
        return ESP_OK;
    }

    const uart_config_t config = {
        .baud_rate = 115200,
        .data_bits = UART_DATA_8_BITS,
        .parity = UART_PARITY_DISABLE,
        .stop_bits = UART_STOP_BITS_1,
        .flow_ctrl = UART_HW_FLOWCTRL_DISABLE,
        .source_clk = UART_SCLK_DEFAULT,
    };

    esp_err_t err = uart_param_config(UART_NUM_0, &config);
    if (err != ESP_OK) {
        return err;
    }

    err = uart_driver_install(UART_NUM_0, TRANSPORT_BUFFER_SIZE,
                              TRANSPORT_BUFFER_SIZE, 0, NULL, 0);
    if (err == ESP_OK) {
        s_uart0_installed = true;
    }
    return err;
}

static void uart0_deinit(void) {
    if (!s_uart0_installed) {
        return;
    }
    uart_driver_delete(UART_NUM_0);
    s_uart0_installed = false;
}

static int uart0_read(uint8_t *buffer, size_t length, TickType_t timeout) {
    return uart_read_bytes(UART_NUM_0, buffer, length, timeout);
}

static int uart0_write(const uint8_t *buffer, size_t length,
                       TickType_t timeout) {
    int written = uart_write_bytes(UART_NUM_0, (const char *)buffer, length);
    if (written < 0) {
        return written;
    }
    if (uart_wait_tx_done(UART_NUM_0, timeout) != ESP_OK) {
        return -1;
    }
    return written;
}

static esp_err_t uart0_flush(void) {
    return uart_flush_input(UART_NUM_0);
}

static esp_err_t usb_serial_jtag_init(void) {
    if (s_usb_serial_jtag_installed) {
        return ESP_OK;
    }

    usb_serial_jtag_driver_config_t config =
        USB_SERIAL_JTAG_DRIVER_CONFIG_DEFAULT();
    esp_err_t err = usb_serial_jtag_driver_install(&config);
    if (err == ESP_OK) {
        s_usb_serial_jtag_installed = true;
    }
    return err;
}

static void usb_serial_jtag_deinit(void) {
    if (!s_usb_serial_jtag_installed) {
        return;
    }
    usb_serial_jtag_driver_uninstall();
    s_usb_serial_jtag_installed = false;
}

static int usb_serial_jtag_read(uint8_t *buffer, size_t length,
                                TickType_t timeout) {
    return usb_serial_jtag_read_bytes(buffer, length, timeout);
}

static int usb_serial_jtag_write(const uint8_t *buffer, size_t length,
                                 TickType_t timeout) {
    return usb_serial_jtag_write_bytes(buffer, length, timeout);
}

static esp_err_t usb_serial_jtag_flush(void) {
    // The USB-Serial/JTAG driver has no input-flush API. Reads are bounded by
    // the caller's timeout, and the next provisioning window starts fresh
    // when this driver is installed again.
    return ESP_OK;
}

static transport_t s_uart0 = {
    .name = "uart0",
    .init = uart0_init,
    .deinit = uart0_deinit,
    .read = uart0_read,
    .write = uart0_write,
    .flush = uart0_flush,
};

static transport_t s_usb_serial_jtag = {
    .name = "usb-serial-jtag",
    .init = usb_serial_jtag_init,
    .deinit = usb_serial_jtag_deinit,
    .read = usb_serial_jtag_read,
    .write = usb_serial_jtag_write,
    .flush = usb_serial_jtag_flush,
};

transport_t *transport_uart0(void) {
    return &s_uart0;
}

transport_t *transport_usb_serial_jtag(void) {
    return &s_usb_serial_jtag;
}

esp_err_t transport_init(transport_t *transport) {
    if (!transport || !transport->init) {
        return ESP_ERR_INVALID_ARG;
    }
    return transport->init();
}

void transport_deinit(transport_t *transport) {
    if (transport && transport->deinit) {
        transport->deinit();
    }
}

int transport_read(transport_t *transport, uint8_t *buffer, size_t length,
                   TickType_t timeout) {
    if (!transport || !transport->read || !buffer) {
        return -1;
    }
    return transport->read(buffer, length, timeout);
}

int transport_write(transport_t *transport, const uint8_t *buffer,
                    size_t length, TickType_t timeout) {
    if (!transport || !transport->write || !buffer) {
        return -1;
    }
    return transport->write(buffer, length, timeout);
}

esp_err_t transport_flush(transport_t *transport) {
    if (!transport || !transport->flush) {
        return ESP_ERR_INVALID_ARG;
    }
    return transport->flush();
}
