# ESP32 Provisioning Instructions

## Task: Provision ESP32 Node (bf-2po1)

### Context
The ESP32-S3 has been successfully flashed with firmware (bf-4gbf complete). Now it needs WiFi credentials and mothership URL to connect.

### Problem
The ESP32 device is physically connected to your local machine (via USB-C), but this development environment is a remote Hetzner server with no physical access to USB devices. Therefore, the provisioning must be performed locally on your machine.

### Two Provisioning Methods Available

The firmware supports **both** captive portal AND serial provisioning. Use serial provisioning - it's more reliable and doesn't require WiFi juggling.

## Method 1: Serial Provisioning (RECOMMENDED)

### Prerequisites
- ESP32-S3 connected via USB-C
- Python 3 installed (for the serial script)
- Your home WiFi SSID and password
- Mothership URL: `spaxel.ardenone.com` (port 443, WSS)

### Step 1: Find the serial port

```bash
# Linux
ls /dev/ttyUSB* /dev/ttyACM*

# macOS
ls /dev/tty.usbserial*

# Windows
# Check Device Manager for COM port under "Ports (COM & LPT)"
```

### Step 2: Install pyserial (if needed)

```bash
pip install pyserial
```

### Step 3: Run the provisioning script

Create a file `provision_esp32.py`:

```python
#!/usr/bin/env python3
import serial
import json
import time

# Configuration - MODIFY THESE
SERIAL_PORT = "/dev/ttyUSB0"  # Change to your port
WIFI_SSID = "your-wifi-ssid"      # Your home WiFi SSID
WIFI_PASS = "your-wifi-password"  # Your WiFi password
MOTHERSHIP_URL = "spaxel.ardenone.com"  # Mothership hostname

# Provisioning payload
provision = {
    "provision": {
        "wifi_ssid": WIFI_SSID,
        "wifi_pass": WIFI_PASS,
        "ms_ip": MOTHERSHIP_URL,
        "ms_port": 443,
        "debug": False
    }
}

# Open serial port (115200 baud, 8N1)
ser = serial.Serial(SERIAL_PORT, 115200, timeout=5)
ser.dtr = False  # Reset ESP32
time.sleep(0.1)
ser.dtr = True
time.sleep(2)    # Wait for boot

print("Listening for 'SPAXEL READY' message...")

# Wait for READY message
while True:
    if ser.in_waiting > 0:
        line = ser.readline().decode('utf-8', errors='ignore').strip()
        print(f"Received: {line}")
        if "SPAXEL READY" in line:
            print("ESP32 is ready for provisioning!")
            break

# Send provisioning JSON
payload = json.dumps(provision)
print(f"Sending: {payload}")
ser.write((payload + "\n").encode())

# Wait for response
time.sleep(2)
while ser.in_waiting > 0:
    line = ser.readline().decode('utf-8', errors='ignore').strip()
    print(f"Response: {line}")

ser.close()
print("Done! ESP32 should now reboot and connect to WiFi.")
```

Run it:

```bash
python3 provision_esp32.py
```

### Step 4: Verify connection

After provisioning, the ESP32 will:
1. Write credentials to NVS
2. Reboot
3. Connect to your home WiFi
4. Connect to mothership at `wss://spaxel.ardenone.com:443`
5. Send a hello message with MAC, firmware version, and capabilities

Check the dashboard: `https://spaxel.ardenone.com`

You should see the node appear as **CONNECTED** in the fleet dashboard.

## Method 2: Captive Portal (Alternative)

If serial provisioning doesn't work, use the captive portal:

### Step 1: Connect to the ESP32 AP

The ESP32 broadcasts an AP named `spaxel-XXXX` (last 4 hex digits of MAC).

1. Check your phone/laptop WiFi networks
2. Find and connect to `spaxel-XXXX` (no password)

### Step 2: Open captive portal

1. Your browser should redirect to `http://192.168.4.1`
2. If not, manually navigate to `http://192.168.4.1`

### Step 3: Enter credentials

- **WiFi SSID**: Your home network SSID
- **WiFi Password**: Your home network password
- **Mothership URL**: `spaxel.ardenone.com` (port 443, WSS)

The firmware will derive `ws://` or `wss://` internally based on the port.

### Step 4: Submit and verify

- Click Submit
- ESP32 writes creds to NVS and reboots
- After ~30 seconds, check the dashboard for the CONNECTED node

## Verification Checklist

- [ ] ESP32 is powered on (LED solid or breathing)
- [ ] ESP32 AP `spaxel-XXXX` is visible in WiFi list
- [ ] Credentials sent successfully (serial: `{"ok":true,"mac":"..."}`)
- [ ] ESP32 reboots after provisioning
- [ ] Node appears in fleet dashboard as CONNECTED
- [ ] Node shows correct MAC address
- [ ] Node shows firmware version

## Troubleshooting

### Serial port not found
- Check USB cable (use a data cable, not charging-only)
- Try different USB port
- Check `dmesg | grep tty` for kernel messages

### Provisioning fails
- Verify JSON format is correct
- Check ESP32 is broadcasting "SPAXEL READY"
- Try pressing RESET button on ESP32

### Node doesn't connect to WiFi
- Verify SSID and password are correct
- Check ESP32 is within range of router
- Look at serial output for error messages

### Node doesn't appear in dashboard
- Verify mothership URL is correct: `spaxel.ardenone.com`
- Check mothership is running and accessible
- Verify port 443 (WSS) is not blocked

## Migration Window

The `SPAXEL_MIGRATION_WINDOW_HOURS=168` setting means the node can connect without a provisioned token for **7 days**. Token management is not needed for the first node.

## Notes

- The MAC address is printed in serial output: `SPAXEL READY aa:bb:cc:dd:ee:ff`
- The last 4 hex digits (e.g., `eeff`) appear in the AP name: `spaxel-eeff`
- Mothership uses mDNS (`_spaxel._tcp.local`) for discovery, but falls back to `ms_ip` NVS key when remote
- WSS (secure WebSocket) on port 443 is the default connection method

## Next Steps

Once the node is CONNECTED:
1. Verify it appears in the dashboard fleet view
2. Check that it sends position updates (blob tracking)
3. Confirm firmware version matches what was flashed
