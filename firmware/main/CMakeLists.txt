# Ensure the generated headers directory exists before ESP-IDF validates include paths
set(GENERATED_HEADERS_DIR "${CMAKE_BINARY_DIR}/spaxel-firmware/main")
file(MAKE_DIRECTORY "${GENERATED_HEADERS_DIR}")

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
    INCLUDE_DIRS "." "${GENERATED_HEADERS_DIR}"
    REQUIRES esp_wifi esp_netif nvs_flash esp_http_client esp_timer bt driver log esp_http_server mbedtls app_update json freertos
)
