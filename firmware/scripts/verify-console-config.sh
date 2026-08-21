#!/usr/bin/env bash
# Verify the generated ESP-IDF console selection before flashing a board.
#
# Usage:
#   ./scripts/verify-console-config.sh [sdkconfig] [usb|uart]

set -euo pipefail

CONFIG_FILE="${1:-sdkconfig}"
PROFILE="${2:-usb}"

if [[ ! -f "$CONFIG_FILE" ]]; then
    echo "ERROR: generated config not found: $CONFIG_FILE" >&2
    echo "Run an ESP-IDF build first." >&2
    exit 1
fi

require_line() {
    local expected="$1"
    if ! grep -Fxq "$expected" "$CONFIG_FILE"; then
        echo "ERROR: $CONFIG_FILE is missing: $expected" >&2
        exit 1
    fi
}

case "$PROFILE" in
    usb)
        require_line 'CONFIG_ESP_CONSOLE_USB_SERIAL_JTAG=y'
        require_line '# CONFIG_ESP_CONSOLE_UART_DEFAULT is not set'
        require_line 'CONFIG_ESP_CONSOLE_SECONDARY_NONE=y'
        require_line 'CONFIG_ESP_CONSOLE_UART_NUM=-1'
        ;;
    uart)
        require_line 'CONFIG_ESP_CONSOLE_UART_DEFAULT=y'
        require_line '# CONFIG_ESP_CONSOLE_USB_SERIAL_JTAG is not set'
        require_line 'CONFIG_ESP_CONSOLE_SECONDARY_USB_SERIAL_JTAG=y'
        require_line 'CONFIG_ESP_CONSOLE_UART_NUM=0'
        ;;
    *)
        echo "Usage: $0 [sdkconfig] [usb|uart]" >&2
        exit 2
        ;;
esac

require_line 'CONFIG_ESP_SYSTEM_PANIC_PRINT_REBOOT=y'
printf 'Console profile %s is valid in %s.\n' "$PROFILE" "$CONFIG_FILE"
