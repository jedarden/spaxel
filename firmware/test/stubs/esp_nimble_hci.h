/*
 * Host stand-in for ESP-IDF's esp_nimble_hci.h (spaxel-db7aab09).
 *
 * firmware/main/ble.c includes this for the NimBLE HCI/controller init
 * hooks; its scan-only path calls none of them, so the host surface is
 * intentionally empty — the include just has to resolve on -I. The BLE
 * controller itself has no host analogue here; ble_gap.h carries the
 * recording fakes that matter.
 */
#ifndef SPAXEL_TEST_STUB_ESP_NIMBLE_HCI_H
#define SPAXEL_TEST_STUB_ESP_NIMBLE_HCI_H

#endif /* SPAXEL_TEST_STUB_ESP_NIMBLE_HCI_H */
