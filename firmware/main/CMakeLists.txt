idf_component_register(
    SRCS "main.c"
         "wifi.c"
         "websocket.c"
         "csi.c"
         "ble.c"
    INCLUDE_DIRS "."
    REQUIRES esp_wifi esp_netif nvs_flash mdns esp_http_client esp_timer bt driver log
)
