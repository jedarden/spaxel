#!/usr/bin/env python3
"""
ESP32 Serial Provisioning Script for Spaxel

This script provisions a freshly-flashed ESP32-S3 node with WiFi credentials
and mothership URL via USB serial.

The ESP32 firmware broadcasts "SPAXEL READY <MAC>" and accepts a JSON
provisioning payload over USB serial at 115200 baud.

Prerequisites:
    pip install pyserial

Usage:
    1. Edit the configuration below (WIFI_SSID, WIFI_PASS, SERIAL_PORT)
    2. Connect ESP32 via USB-C
    3. Run: python3 scripts/provision_esp32.py
    4. Check spaxel.ardenone.com for the CONNECTED node
"""

import serial
import json
import time
import sys
import argparse

# ============================================================
# CONFIGURATION - EDIT THESE BEFORE RUNNING
# ============================================================

# WiFi credentials
WIFI_SSID = "your-wifi-ssid-here"
WIFI_PASS = "your-wifi-password-here"

# Mothership configuration
MOTHERSHIP_URL = "spaxel.ardenone.com"  # Hostname only, firmware derives wss://
MOTHERSHIP_PORT = 443                    # WSS port

# Serial port
# Linux: /dev/ttyUSB0 or /dev/ttyACM0
# macOS: /dev/tty.usbserial-xxx
# Windows: COM3, COM4, etc.
SERIAL_PORT = None  # Set via --port argument or edit here

BAUD_RATE = 115200
PROVISIONING_TIMEOUT = 120  # seconds to wait for READY message


def detect_serial_port():
    """Try to detect the ESP32 serial port automatically."""
    import platform

    system = platform.system()

    # Common port names to try
    candidates = []

    if system == "Linux":
        import os
        for dev in os.listdir("/dev"):
            if dev.startswith("ttyUSB") or dev.startswith("ttyACM"):
                candidates.append(f"/dev/{dev}")

    elif system == "Darwin":  # macOS
        import os
        for dev in os.listdir("/dev"):
            if dev.startswith("tty.usbserial"):
                candidates.append(f"/dev/{dev}")

    elif system == "Windows":
        import serial.tools.list_ports
        ports = serial.tools.list_ports.comports()
        candidates = [port.device for port in ports]

    if not candidates:
        return None

    if len(candidates) == 1:
        print(f"Auto-detected serial port: {candidates[0]}")
        return candidates[0]

    print(f"Multiple serial ports found: {', '.join(candidates)}")
    print("Please specify with --port")
    return None


def provision_esp32(port, wifi_ssid, wifi_pass, ms_url, ms_port, debug=False):
    """
    Provision ESP32 via serial connection.

    Args:
        port: Serial port device path
        wifi_ssid: WiFi SSID
        wifi_pass: WiFi password
        ms_url: Mothership URL/hostname
        ms_port: Mothership port (443 for WSS)
        debug: Enable debug mode on ESP32

    Returns:
        bool: True if successful, False otherwise
    """
    print(f"Connecting to serial port: {port}")

    try:
        ser = serial.Serial(port, BAUD_RATE, timeout=5)
    except serial.SerialException as e:
        print(f"ERROR: Cannot open serial port: {e}")
        print("Common causes:")
        print("  - Port does not exist (check --port)")
        print("  - Another program is using the port")
        print("  - Insufficient permissions (try: sudo chmod 666 {})".format(port))
        return False

    # Reset ESP32 by toggling DTR
    print("Resetting ESP32...")
    ser.dtr = False
    time.sleep(0.1)
    ser.dtr = True
    ser.flushInput()
    ser.flushOutput()

    print("Waiting for ESP32 boot and READY message...")
    deadline = time.time() + PROVISIONING_TIMEOUT
    mac = None

    while time.time() < deadline:
        if ser.in_waiting > 0:
            try:
                line = ser.readline().decode('utf-8', errors='ignore').strip()
            except UnicodeDecodeError:
                continue

            if line:
                print(f"ESP32: {line}")

                if "SPAXEL READY" in line:
                    parts = line.split()
                    if len(parts) >= 3:
                        mac = parts[2]
                        print(f"\n✓ ESP32 ready for provisioning (MAC: {mac})")
                        break

        time.sleep(0.1)

    if not mac:
        print("\nERROR: Timeout waiting for SPAXEL READY message")
        print("Make sure ESP32 is powered on and running the latest firmware.")
        ser.close()
        return False

    # Build provisioning JSON
    provision_payload = {
        "provision": {
            "wifi_ssid": wifi_ssid,
            "wifi_pass": wifi_pass,
            "ms_ip": ms_url,
            "ms_port": ms_port,
            "debug": debug
        }
    }

    payload_json = json.dumps(provision_payload)
    print(f"\nSending provisioning JSON:")
    print(f"  {payload_json}")

    try:
        ser.write((payload_json + "\n").encode())
    except serial.SerialException as e:
        print(f"ERROR: Failed to write to serial port: {e}")
        ser.close()
        return False

    # Wait for response
    print("\nWaiting for response...")
    time.sleep(2)

    success = False
    while ser.in_waiting > 0:
        try:
            line = ser.readline().decode('utf-8', errors='ignore').strip()
        except UnicodeDecodeError:
            continue

        if line:
            print(f"ESP32: {line}")

            try:
                response = json.loads(line)
                if response.get("ok"):
                    print(f"\n✓ Provisioning successful!")
                    print(f"  MAC: {response.get('mac')}")
                    success = True
                    break
            except json.JSONDecodeError:
                pass

    ser.close()

    if success:
        print("\nThe ESP32 will now reboot and connect to WiFi.")
        print(f"SSID: {wifi_ssid}")
        print(f"Mothership: wss://{ms_url}:{ms_port}")
        print("\nCheck the dashboard in ~30 seconds:")
        print(f"  https://{ms_url}")
    else:
        print("\n✗ Provisioning may have failed.")
        print("Check the ESP32 serial output above for errors.")
        print("You can try re-running this script.")

    return success


def main():
    parser = argparse.ArgumentParser(
        description="Provision Spaxel ESP32 node via serial",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python3 scripts/provision_esp32.py --port /dev/ttyUSB0
  python3 scripts/provision_esp32.py --ssid MyNetwork --pass secret123
  python3 scripts/provision_esp32.py --auto --debug
        """
    )

    parser.add_argument("--port", help="Serial port device (auto-detect if not specified)")
    parser.add_argument("--ssid", default=WIFI_SSID, help="WiFi SSID")
    parser.add_argument("--pass", default=WIFI_PASS, dest="password", help="WiFi password")
    parser.add_argument("--ms-url", default=MOTHERSHIP_URL, help="Mothership URL")
    parser.add_argument("--ms-port", type=int, default=MOTHERSHIP_PORT, help="Mothership port")
    parser.add_argument("--auto", action="store_true", help="Auto-detect serial port")
    parser.add_argument("--debug", action="store_true", help="Enable debug mode on ESP32")

    args = parser.parse_args()

    # Validate inputs
    if not args.port and not args.auto:
        print("ERROR: No serial port specified.")
        print("Use --port /dev/ttyUSB0 or --auto for auto-detection")
        return 1

    if args.auto:
        args.port = detect_serial_port()
        if not args.port:
            return 1

    if args.ssid == "your-wifi-ssid-here":
        print("ERROR: Please configure WiFi credentials in the script")
        print("or use: --ssid YOUR_SSID --pass YOUR_PASSWORD")
        return 1

    # Provision
    success = provision_esp32(
        port=args.port,
        wifi_ssid=args.ssid,
        wifi_pass=args.password,
        ms_url=args.ms_url,
        ms_port=args.ms_port,
        debug=args.debug
    )

    return 0 if success else 1


if __name__ == "__main__":
    sys.exit(main())
