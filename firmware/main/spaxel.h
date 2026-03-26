#pragma once

#include <stdint.h>
#include <stdbool.h>
#include "esp_err.h"
#include "freertos/FreeRTOS.h"
#include "freertos/event_groups.h"

// System configuration
#define SPAXEL_NAMESPACE "spaxel"
#define SPAXEL_MDNS_SERVICE "_spaxel"
#define SPAXEL_MDNS_PROTO "_tcp"
#define SPAXEL_MDNS_PORT 8080
#define SPAXEL_WS_PATH "/ws/node"

// WebSocket configuration
#define SPAXEL_WS_RECONNECT_MS 5000
#define SPAXEL_WS_TIMEOUT_MS 30000
#define SPAXEL_PING_INTERVAL_SEC 30

// Health reporting interval
#define SPAXEL_HEALTH_INTERVAL_MS 10000
#define SPAXEL_BLE_INTERVAL_MS 5000

// CSI configuration
#define SPAXEL_CSI_MAX_SUBCARRIERS 64
#define SPAXEL_CSI_QUEUE_SIZE 32

// Binary frame header size
#define SPAXEL_FRAME_HEADER_SIZE 24

// Node states
typedef enum {
    NODE_STATE_BOOT,
    NODE_STATE_WIFI_CONNECTING,
    NODE_STATE_MOTHERSHIP_DISCOVERY,
    NODE_STATE_CONNECTED,
    NODE_STATE_WIFI_LOST,
    NODE_STATE_MOTHERSHIP_UNAVAILABLE,
    NODE_STATE_CAPTIVE_PORTAL,
} node_state_t;

// Node roles
typedef enum {
    NODE_ROLE_TX = 0,
    NODE_ROLE_RX = 1,
    NODE_ROLE_TX_RX = 2,
    NODE_ROLE_PASSIVE = 3,
    NODE_ROLE_IDLE = 4,
} node_role_t;

// NVS keys (max 15 chars)
#define NVS_KEY_SCHEMA_VER "schema_ver"
#define NVS_KEY_PROVISIONED "provisioned"
#define NVS_KEY_WIFI_SSID "wifi_ssid"
#define NVS_KEY_WIFI_PASS "wifi_pass"
#define NVS_KEY_NODE_ID "node_id"
#define NVS_KEY_NODE_TOKEN "node_token"
#define NVS_KEY_MS_MDNS "ms_mdns"
#define NVS_KEY_MS_IP "ms_ip"
#define NVS_KEY_MS_PORT "ms_port"
#define NVS_KEY_PASSIVE_BSS "passive_bss"
#define NVS_KEY_ROLE "role"
#define NVS_KEY_PKT_RATE "pkt_rate"
#define NVS_KEY_AP_MODE "ap_mode"
#define NVS_KEY_DEBUG "debug"

// Current NVS schema version
#define NVS_SCHEMA_VERSION 1

// Global state
typedef struct {
    node_state_t state;
    node_role_t role;
    uint8_t packet_rate;
    uint8_t passive_bssid[6];
    char node_id[38];
    char node_token[65];
    char ms_mdns[65];
    char ms_ip[47];
    uint16_t ms_port;
    bool provisioned;
    bool debug;
    uint8_t mac[6];
    EventGroupHandle_t events;
} spaxel_state_t;

// Event bits
#define SPAXEL_EVENT_WIFI_CONNECTED BIT0
#define SPAXEL_EVENT_WIFI_FAILED BIT1
#define SPAXEL_EVENT_WS_CONNECTED BIT2
#define SPAXEL_EVENT_WS_DISCONNECTED BIT3
#define SPAXEL_EVENT_ROLE_CHANGED BIT4
#define SPAXEL_EVENT_OTA_TRIGGER BIT5
#define SPAXEL_EVENT_REBOOT BIT6

// Global state instance
extern spaxel_state_t g_state;

// Utility functions
const char* node_state_str(node_state_t state);
const char* node_role_str(node_role_t role);
void mac_to_str(uint8_t *mac, char *buf, size_t len);
void str_to_mac(const char *str, uint8_t *mac);
