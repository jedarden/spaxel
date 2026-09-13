/*
 * Host stand-in for ESP-IDF's esp_err.h: exactly the slice
 * firmware/main/watchdog.c needs (spaxel-61d41649).
 *
 * esp_err_t is int and the codes carry their real ESP-IDF bit patterns, so
 * comparisons in the code under test behave identically on the host. Only the
 * harness searches stubs/ (see the Makefile's STUBS_INC); the ESP-IDF build
 * never references firmware/test, so this can never shadow the real header on
 * target.
 */
#ifndef SPAXEL_TEST_STUB_ESP_ERR_H
#define SPAXEL_TEST_STUB_ESP_ERR_H

typedef int esp_err_t;

#define ESP_OK                  0
#define ESP_FAIL               -1
#define ESP_ERR_NO_MEM        0x101
#define ESP_ERR_INVALID_ARG   0x102
#define ESP_ERR_INVALID_STATE 0x103
#define ESP_ERR_INVALID_SIZE  0x104
#define ESP_ERR_NOT_FOUND     0x105

/* Declared here, defined in the test TU that includes the production code —
 * keeps this directory header-only so the Makefile needs no new source. */
const char *esp_err_to_name(esp_err_t code);

#endif /* SPAXEL_TEST_STUB_ESP_ERR_H */
