idf_component_register(
    SRCS "main.c"
         "wifi.c"
         "websocket.c"
         "csi.c"
         "ble.c"
         "provision.c"
         "nvs_migration.c"
         "ntp.c"
         "led.c"
    INCLUDE_DIRS "." "${CMAKE_BINARY_DIR}/spaxel-firmware/main"
    REQUIRES esp_wifi esp_netif nvs_flash mdns esp_http_client esp_timer bt driver log esp_http_server mbedtls esp_ota
)
