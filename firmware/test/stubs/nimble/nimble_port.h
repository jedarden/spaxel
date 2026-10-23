/*
 * Host stand-in for NimBLE's nimble/nimble_port.h (spaxel-db7aab09).
 *
 * ble.c includes it for the host-stack bootstrap (nimble_port_init/init
 * callbacks); the scan-only code path calls none of it, so the host surface
 * is intentionally empty. Radio bring-up simply does not exist on a host.
 */
#ifndef SPAXEL_TEST_STUB_NIMBLE_NIMBLE_PORT_H
#define SPAXEL_TEST_STUB_NIMBLE_NIMBLE_PORT_H

#endif /* SPAXEL_TEST_STUB_NIMBLE_NIMBLE_PORT_H */
