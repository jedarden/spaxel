#pragma once

#include "esp_err.h"
#include "cJSON.h"

// provision_listen_window opens a serial provisioning window.
// Unprovisioned boards wait 120 s; already-provisioned boards wait 15 s.
// Prints "SPAXEL READY <MAC>\n" and reads {"provision": {...}}\n over the
// native USB-Serial-JTAG peripheral (the same one the console/esp_log uses —
// this board has no USB-UART bridge chip, so UART0's GPIO43/44 pins go
// nowhere; see the sdkconfig.usbjtag console-routing comment for the
// identical failure mode this previously hit for esp_log output).
// Responds with {"ok": true, "mac": "..."}\n on success.
// Safe to call even if no host is connected — times out cleanly.
void provision_listen_window(void);

// provision_write_nvs writes the provisioning JSON blob to NVS.
// Expected keys: wifi_ssid, wifi_pass, node_id, node_token, ms_mdns, ms_port, debug.
esp_err_t provision_write_nvs(cJSON *prov);
