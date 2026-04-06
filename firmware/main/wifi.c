#include "wifi.h"
#include "spaxel.h"
#include "esp_log.h"
#include "esp_wifi.h"
#include "esp_event.h"
#include "esp_netif.h"
#include "nvs_flash.h"
#include "nvs.h"
#include "mdns.h"
#include "lwip/sockets.h"
#include "freertos/FreeRTOS.h"
#include "freertos/task.h"
#include <string.h>
#include <ctype.h>

static const char *TAG = "wifi";

static bool s_connected = false;
static int8_t s_rssi = 0;
static uint8_t s_channel = 0;
static esp_netif_t *s_sta_netif = NULL;
static esp_netif_t *s_ap_netif = NULL;

// Exponential backoff state
static int s_backoff_ms = 1000;
static const int s_backoff_max_ms = 30000;

static void wifi_event_handler(void *arg, esp_event_base_t base,
                                int32_t id, void *data) {
    if (base == WIFI_EVENT) {
        switch (id) {
            case WIFI_EVENT_STA_START:
                ESP_LOGI(TAG, "WiFi STA started");
                break;

            case WIFI_EVENT_STA_CONNECTED:
                {
                    wifi_event_sta_connected_t *event = (wifi_event_sta_connected_t *)data;
                    s_channel = event->channel;
                    ESP_LOGI(TAG, "WiFi connected to channel %d", s_channel);
                }
                break;

            case WIFI_EVENT_STA_DISCONNECTED:
                ESP_LOGW(TAG, "WiFi disconnected");
                s_connected = false;
                s_rssi = 0;
                xEventGroupSetBits(g_state.events, SPAXEL_EVENT_WIFI_FAILED);
                break;

            case WIFI_EVENT_AP_STACONNECTED:
                {
                    wifi_event_ap_staconnected_t *event = (wifi_event_ap_staconnected_t *)data;
                    ESP_LOGI(TAG, "Station connected to AP: " MACSTR,
                             MAC2STR(event->mac));
                }
                break;

            case WIFI_EVENT_AP_STADISCONNECTED:
                {
                    wifi_event_ap_stadisconnected_t *event = (wifi_event_ap_stadisconnected_t *)data;
                    ESP_LOGI(TAG, "Station disconnected from AP: " MACSTR,
                             MAC2STR(event->mac));
                }
                break;

            default:
                break;
        }
    } else if (base == IP_EVENT) {
        switch (id) {
            case IP_EVENT_STA_GOT_IP:
                {
                    ip_event_got_ip_t *event = (ip_event_got_ip_t *)data;
                    ESP_LOGI(TAG, "Got IP: " IPSTR, IP2STR(&event->ip_info.ip));
                    s_connected = true;
                    s_backoff_ms = 1000; // Reset backoff
                    xEventGroupSetBits(g_state.events, SPAXEL_EVENT_WIFI_CONNECTED);
                }
                break;

            default:
                break;
        }
    }
}

esp_err_t wifi_init(void) {
    // Initialize TCP/IP stack
    ESP_ERROR_CHECK(esp_netif_init());
    ESP_ERROR_CHECK(esp_event_loop_create_default());

    // Create STA and AP netif
    s_sta_netif = esp_netif_create_default_wifi_sta();
    s_ap_netif = esp_netif_create_default_wifi_ap();

    // Initialize WiFi with default config
    wifi_init_config_t cfg = WIFI_INIT_CONFIG_DEFAULT();
    ESP_ERROR_CHECK(esp_wifi_init(&cfg));

    // Register event handlers
    ESP_ERROR_CHECK(esp_event_handler_register(WIFI_EVENT, ESP_EVENT_ANY_ID,
                                                &wifi_event_handler, NULL));
    ESP_ERROR_CHECK(esp_event_handler_register(IP_EVENT, IP_EVENT_STA_GOT_IP,
                                                &wifi_event_handler, NULL));

    // Initialize mDNS
    ESP_ERROR_CHECK(mdns_init());
    ESP_ERROR_CHECK(mdns_hostname_set("spaxel-node"));
    ESP_LOGI(TAG, "mDNS initialized: spaxel-node.local");

    return ESP_OK;
}

esp_err_t wifi_start_connect(void) {
    if (!g_state.provisioned) {
        ESP_LOGW(TAG, "Not provisioned, cannot connect");
        return ESP_ERR_INVALID_STATE;
    }

    // Get WiFi credentials from NVS
    nvs_handle_t nvs;
    esp_err_t err = nvs_open(SPAXEL_NAMESPACE, NVS_READONLY, &nvs);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to open NVS: %s", esp_err_to_name(err));
        return err;
    }

    char ssid[33] = {0};
    char password[65] = {0};
    size_t len;

    len = sizeof(ssid);
    err = nvs_get_str(nvs, NVS_KEY_WIFI_SSID, ssid, &len);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to get WiFi SSID: %s", esp_err_to_name(err));
        nvs_close(nvs);
        return err;
    }

    len = sizeof(password);
    err = nvs_get_str(nvs, NVS_KEY_WIFI_PASS, password, &len);
    if (err != ESP_OK) {
        ESP_LOGE(TAG, "Failed to get WiFi password: %s", esp_err_to_name(err));
        nvs_close(nvs);
        return err;
    }
    nvs_close(nvs);

    // Configure WiFi
    wifi_config_t wifi_config = {0};
    strncpy((char *)wifi_config.sta.ssid, ssid, sizeof(wifi_config.sta.ssid) - 1);
    strncpy((char *)wifi_config.sta.password, password, sizeof(wifi_config.sta.password) - 1);
    wifi_config.sta.threshold.authmode = WIFI_AUTH_WPA2_PSK;
    wifi_config.sta.pmf_cfg.capable = true;
    wifi_config.sta.pmf_cfg.required = false;

    ESP_LOGI(TAG, "Connecting to WiFi: %s", ssid);

    ESP_ERROR_CHECK(esp_wifi_set_mode(WIFI_MODE_STA));
    ESP_ERROR_CHECK(esp_wifi_set_config(WIFI_IF_STA, &wifi_config));
    ESP_ERROR_CHECK(esp_wifi_start());

    // Apply exponential backoff delay
    vTaskDelay(pdMS_TO_TICKS(s_backoff_ms));
    s_backoff_ms = MIN(s_backoff_ms * 2, s_backoff_max_ms);

    ESP_ERROR_CHECK(esp_wifi_connect());

    return ESP_OK;
}

bool wifi_discover_mothership(char *ip_buf, size_t buf_len, uint16_t *port) {
    if (!ip_buf || !port || buf_len == 0) {
        return false;
    }

    ESP_LOGI(TAG, "Querying mDNS for %s.%s.local:%d",
             g_state.ms_mdns, SPAXEL_MDNS_SERVICE, *port);

    // Query mDNS for the mothership service
    mdns_result_t *results = NULL;
    esp_err_t err = mdns_query_ptr(SPAXEL_MDNS_SERVICE, SPAXEL_MDNS_PROTO,
                                    5000, &results);

    if (err != ESP_OK || results == NULL) {
        ESP_LOGW(TAG, "mDNS query failed or no results");
        if (results) {
            mdns_query_results_free(results);
        }
        return false;
    }

    // Find matching service
    mdns_result_t *r = results;
    while (r) {
        if (r->hostname && strstr(r->hostname, g_state.ms_mdns) != NULL) {
            // Found matching service
            if (r->addr) {
                // Convert IP to string
                if (r->addr->addr.type == IPADDR_TYPE_V4) {
                    snprintf(ip_buf, buf_len, IPSTR,
                             IP2STR(&r->addr->addr.u_addr.ip4));
                    *port = r->port;
                    ESP_LOGI(TAG, "Found mothership: %s:%d", ip_buf, *port);
                    mdns_query_results_free(results);
                    return true;
                }
            }
        }
        r = r->next;
    }

    // Use first result if no match by hostname
    if (results->addr && results->addr->addr.type == IPADDR_TYPE_V4) {
        snprintf(ip_buf, buf_len, IPSTR,
                 IP2STR(&results->addr->addr.u_addr.ip4));
        *port = results->port;
        ESP_LOGI(TAG, "Using first mDNS result: %s:%d", ip_buf, *port);
        mdns_query_results_free(results);
        return true;
    }

    mdns_query_results_free(results);
    return false;
}

// ─── Captive portal DNS + HTTP ────────────────────────────────────────────────

#include "esp_http_server.h"
#include "lwip/udp.h"
#include "lwip/ip4_addr.h"

static httpd_handle_t s_captive_server = NULL;
static struct udp_pcb *s_dns_pcb = NULL;

// Minimal DNS server: respond to all A queries with 192.168.4.1.
// This makes iOS/Android/Windows captive portal detection work.
static void captive_dns_recv(void *arg, struct udp_pcb *pcb, struct pbuf *p,
                              const ip_addr_t *addr, u16_t port) {
    if (!p) return;

    // DNS response: copy query, set QR=1, RCODE=0, add answer pointing to 192.168.4.1
    // Minimal valid DNS response: header (12) + question (from request) + answer RR
    if (p->len < 12) {
        pbuf_free(p);
        return;
    }

    // Build response in a fixed buffer (max 256 bytes is more than enough)
    uint8_t resp[256];
    uint16_t resp_len = 0;

    // Copy DNS header from query
    uint8_t *q = (uint8_t *)p->payload;
    uint16_t q_len = p->len;

    if (q_len > 200) {
        pbuf_free(p);
        return;
    }

    // Header (12 bytes): copy ID, set flags QR=1 AA=1 RCODE=0
    memcpy(resp, q, 12);
    resp[2] = 0x81; // QR=1, OPCODE=0, AA=1, TC=0, RD=1
    resp[3] = 0x80; // RA=1, RCODE=0
    // QDCOUNT stays same, ANCOUNT=1, NSCOUNT=0, ARCOUNT=0
    resp[6] = 0x00; resp[7] = 0x01; // ANCOUNT=1
    resp[8] = 0x00; resp[9] = 0x00;
    resp[10] = 0x00; resp[11] = 0x00;
    resp_len = 12;

    // Copy question section from query
    uint16_t q_section_len = q_len - 12;
    if (resp_len + q_section_len > sizeof(resp) - 16) {
        pbuf_free(p);
        return;
    }
    memcpy(resp + resp_len, q + 12, q_section_len);
    resp_len += q_section_len;

    // Answer RR: pointer to question name (0xC00C), type A, class IN, TTL 60s, RDATA 4 bytes
    resp[resp_len++] = 0xC0; resp[resp_len++] = 0x0C; // name ptr to offset 12
    resp[resp_len++] = 0x00; resp[resp_len++] = 0x01; // type A
    resp[resp_len++] = 0x00; resp[resp_len++] = 0x01; // class IN
    resp[resp_len++] = 0x00; resp[resp_len++] = 0x00; // TTL high
    resp[resp_len++] = 0x00; resp[resp_len++] = 0x3C; // TTL low (60s)
    resp[resp_len++] = 0x00; resp[resp_len++] = 0x04; // RDLENGTH=4
    // 192.168.4.1
    resp[resp_len++] = 192; resp[resp_len++] = 168;
    resp[resp_len++] = 4;   resp[resp_len++] = 1;

    struct pbuf *r = pbuf_alloc(PBUF_TRANSPORT, resp_len, PBUF_RAM);
    if (r) {
        memcpy(r->payload, resp, resp_len);
        udp_sendto(pcb, r, addr, port);
        pbuf_free(r);
    }
    pbuf_free(p);
}

// URL decode helper for captive portal form parsing
static void url_decode(char *dst, const char *src, size_t dst_size) {
    size_t i = 0;
    size_t j = 0;

    while (src[i] && j < dst_size - 1) {
        if (src[i] == '+') {
            dst[j++] = ' ';
            i++;
        } else if (src[i] == '%' && isxdigit(src[i+1]) && isxdigit(src[i+2])) {
            char hex[3] = {src[i+1], src[i+2], 0};
            dst[j++] = (char)strtol(hex, NULL, 16);
            i += 3;
        } else {
            dst[j++] = src[i++];
        }
    }
    dst[j] = '\0';
}

static esp_err_t captive_root_handler(httpd_req_t *req) {
    const char *html =
        "<!DOCTYPE html>"
        "<html><head><title>SPAXEL Setup</title>"
        "<meta name='viewport' content='width=device-width,initial-scale=1'>"
        "<style>"
        "body{font-family:Arial,sans-serif;max-width:400px;margin:50px auto;padding:20px}"
        "input{width:100%;padding:10px;margin:5px 0;box-sizing:border-box}"
        "button{width:100%;padding:12px;background:#4CAF50;color:white;border:none}"
        "h1{color:#333}"
        "</style></head>"
        "<body>"
        "<h1>SPAXEL Setup</h1>"
        "<form action='/save' method='post'>"
        "<label>WiFi Network</label>"
        "<input type='text' name='ssid' placeholder='SSID' required>"
        "<label>WiFi Password</label>"
        "<input type='password' name='password' placeholder='Password'>"
        "<label>Mothership IP (optional)</label>"
        "<input type='text' name='ms_ip' placeholder='auto-detect'>"
        "<button type='submit'>Save &amp; Connect</button>"
        "</form></body></html>";

    httpd_resp_set_type(req, "text/html");
    httpd_resp_send(req, html, strlen(html));
    return ESP_OK;
}

static esp_err_t captive_save_handler(httpd_req_t *req) {
    char buf[512];
    int ret = httpd_req_recv(req, buf, sizeof(buf) - 1);
    if (ret <= 0) {
        httpd_resp_send_err(req, HTTPD_400_BAD_REQUEST, "No data");
        return ESP_FAIL;
    }
    buf[ret] = '\0';

    // Parse form data
    char ssid[33] = {0};
    char password[65] = {0};
    char ms_ip[47] = {0};
    char decoded[128];

    // Parse URL-encoded form data
    char *p = strtok(buf, "&");
    while (p) {
        if (strncmp(p, "ssid=", 5) == 0) {
            url_decode(decoded, p + 5, sizeof(decoded));
            strncpy(ssid, decoded, sizeof(ssid) - 1);
        } else if (strncmp(p, "password=", 9) == 0) {
            url_decode(decoded, p + 9, sizeof(decoded));
            strncpy(password, decoded, sizeof(password) - 1);
        } else if (strncmp(p, "ms_ip=", 6) == 0) {
            url_decode(decoded, p + 6, sizeof(decoded));
            strncpy(ms_ip, decoded, sizeof(ms_ip) - 1);
        }
        p = strtok(NULL, "&");
    }

    // Save to NVS
    nvs_handle_t nvs;
    if (nvs_open(SPAXEL_NAMESPACE, NVS_READWRITE, &nvs) == ESP_OK) {
        nvs_set_str(nvs, NVS_KEY_WIFI_SSID, ssid);
        nvs_set_str(nvs, NVS_KEY_WIFI_PASS, password);
        if (strlen(ms_ip) > 0) {
            nvs_set_str(nvs, NVS_KEY_MS_IP, ms_ip);
        }
        nvs_set_u8(nvs, NVS_KEY_PROVISIONED, 1);
        nvs_set_u8(nvs, NVS_KEY_SCHEMA_VER, NVS_SCHEMA_VERSION);
        nvs_commit(nvs);
        nvs_close(nvs);

        const char *resp = "<html><body><h1>Saved!</h1><p>Rebooting...</p></body></html>";
        httpd_resp_send(req, resp, strlen(resp));

        vTaskDelay(pdMS_TO_TICKS(1000));
        esp_restart();
        return ESP_OK;
    }

    httpd_resp_send_err(req, HTTPD_500_INTERNAL_SERVER_ERROR, "Save failed");
    return ESP_FAIL;
}

static httpd_uri_t captive_uris[] = {
    {"/", HTTP_GET, captive_root_handler, NULL},
    {"/save", HTTP_POST, captive_save_handler, NULL},
};

esp_err_t wifi_start_captive_portal(void) {
    // Create AP
    char ap_ssid[20];
    snprintf(ap_ssid, sizeof(ap_ssid), "spaxel-%02X%02X",
             g_state.mac[4], g_state.mac[5]);

    wifi_config_t ap_config = {0};
    strncpy((char *)ap_config.ap.ssid, ap_ssid, sizeof(ap_config.ap.ssid));
    ap_config.ap.ssid_len = strlen(ap_ssid);
    ap_config.ap.channel = 1;
    ap_config.ap.max_connection = 4;
    ap_config.ap.authmode = WIFI_AUTH_OPEN;

    ESP_ERROR_CHECK(esp_wifi_set_mode(WIFI_MODE_AP));
    ESP_ERROR_CHECK(esp_wifi_set_config(WIFI_IF_AP, &ap_config));
    ESP_ERROR_CHECK(esp_wifi_start());

    ESP_LOGI(TAG, "Captive portal AP started: %s", ap_ssid);

    // Start DNS server on UDP port 53 — redirects all queries to 192.168.4.1
    s_dns_pcb = udp_new();
    if (s_dns_pcb) {
        if (udp_bind(s_dns_pcb, IP_ADDR_ANY, 53) == ERR_OK) {
            udp_recv(s_dns_pcb, captive_dns_recv, NULL);
            ESP_LOGI(TAG, "Captive portal DNS server started on port 53");
        } else {
            udp_remove(s_dns_pcb);
            s_dns_pcb = NULL;
            ESP_LOGW(TAG, "Failed to bind DNS server to port 53");
        }
    }

    // Start HTTP server
    httpd_config_t config = HTTPD_DEFAULT_CONFIG();
    config.server_port = 80;

    if (httpd_start(&s_captive_server, &config) == ESP_OK) {
        for (int i = 0; i < sizeof(captive_uris) / sizeof(captive_uris[0]); i++) {
            httpd_register_uri_handler(s_captive_server, &captive_uris[i]);
        }
        ESP_LOGI(TAG, "Captive portal HTTP server started on 192.168.4.1:80");
    }

    return ESP_OK;
}

int8_t wifi_get_rssi(void) {
    if (!s_connected) return 0;

    wifi_ap_record_t ap_info;
    if (esp_wifi_sta_get_ap_info(&ap_info) == ESP_OK) {
        s_rssi = ap_info.rssi;
    }
    return s_rssi;
}

uint8_t wifi_get_channel(void) {
    return s_channel;
}

bool wifi_is_connected(void) {
    return s_connected;
}

bool wifi_get_ap_bssid(uint8_t *bssid) {
    if (!bssid || !s_connected) {
        return false;
    }

    wifi_ap_record_t ap_info;
    if (esp_wifi_sta_get_ap_info(&ap_info) == ESP_OK) {
        memcpy(bssid, ap_info.bssid, 6);
        return true;
    }
    return false;
}
