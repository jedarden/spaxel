#pragma once

#include <stddef.h>
#include <stdint.h>
#include <stdbool.h>

#include "esp_err.h"
#include "freertos/FreeRTOS.h"

// Provisioning transport interface. The provisioning parser uses this small
// interface so UART0 and native USB-Serial/JTAG can share the same framing and
// NVS code. The current caller selects USB-Serial/JTAG; UART0 remains available
// for bridge-equipped boards and for the dual-listener follow-up.
typedef struct transport transport_t;

struct transport {
    const char *name;
    esp_err_t (*init)(void);
    void (*deinit)(void);
    int (*read)(uint8_t *buffer, size_t length, TickType_t timeout);
    int (*write)(const uint8_t *buffer, size_t length, TickType_t timeout);
    esp_err_t (*flush)(void);
};

transport_t *transport_uart0(void);
transport_t *transport_usb_serial_jtag(void);

esp_err_t transport_init(transport_t *transport);
void transport_deinit(transport_t *transport);
int transport_read(transport_t *transport, uint8_t *buffer, size_t length,
                   TickType_t timeout);
int transport_write(transport_t *transport, const uint8_t *buffer,
                    size_t length, TickType_t timeout);
esp_err_t transport_flush(transport_t *transport);
