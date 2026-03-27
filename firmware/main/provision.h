#pragma once

#include "esp_err.h"
#include "cJSON.h"

// provision_listen_window opens a 10-second serial provisioning window.
// Prints "SPAXEL READY\n" and reads {"provision": {...}}\n from UART0.
// Responds with {"ok": true, "mac": "..."}\n on success.
// Safe to call even if no host is connected — times out cleanly.
void provision_listen_window(void);

// provision_write_nvs writes the provisioning JSON blob to NVS.
// Expected keys: wifi_ssid, wifi_pass, node_id, node_token, ms_mdns, ms_port, debug.
esp_err_t provision_write_nvs(cJSON *prov);
