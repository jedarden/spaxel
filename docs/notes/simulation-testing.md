# Simulated ESP32 Installation Testing

The goal is to be able to develop and test the full installation flow — web installer, provisioning, OTA — without requiring a physical ESP32-S3 connected via USB.

---

## 1. Simulating the Web Installer (ESP Web Tools)

ESP Web Tools uses the browser's **Web Serial API**, which only activates on actual serial port connections. There is no official simulation mode. Three approaches:

### Option A: Virtual Serial Port (Linux)

Create a virtual serial port pair using `socat`:

```bash
socat -d -d pty,raw,echo=0 pty,raw,echo=0
# Creates: /dev/pts/3 <---> /dev/pts/4
```

Then write a Python script that mimics the ESP32 bootloader's ROM command protocol on one end of the pty. The installer's `esptool-js` library communicates using the same SLIP-framed protocol as `esptool.py`.

Key bootloader commands to emulate:
- `SYNC` (0x08) — ROM sync, 36-byte sequence
- `READ_REG` (0x0A) — returns chip registers including chip magic number
- `SPI_FLASH_MD5` — used for verification

The chip magic number determines the detected chip family:
```python
CHIP_MAGIC = {
    0x6F51306F: "ESP32-S3",   # Returns this to identify as S3
    0x00F01D83: "ESP32",
    0x000007C6: "ESP32-S2",
    0x6921506F: "ESP32-C3",
}
```

Reference: `esptool-js` source at `github.com/espressif/esptool-js`.

### Option B: Chrome DevTools Protocol Mock

Use Playwright or Puppeteer to intercept Web Serial API calls and mock responses. More complex but allows end-to-end testing of the web UI without any serial hardware:

```javascript
// In Playwright test:
await page.exposeFunction('__mockSerial', async (command) => {
    if (command === 'sync') return { success: true, chipFamily: 'ESP32-S3' };
    if (command === 'flash') return { success: true };
});
```

This approach tests the UI logic (chip detection display, progress bars, error states) independently of hardware.

### Option C: Physical Target on Development Machine

The simplest approach: keep one physical ESP32-S3 permanently connected to the development machine via USB. Use it as the integration test target. Flash, verify detection works, then proceed to automated testing for everything above the hardware layer.

**Recommended for Spaxel:** Option C for hardware-level validation, Option B for CI pipeline testing of the UI.

---

## 2. Simulating the Provisioning Portal

The captive portal (SoftAP + HTTP server on the ESP32) can be tested by running the HTML form served by `provisioning.c` directly in a browser, pointing form submissions at a local Go server that mocks the NVS save:

```bash
# Run a mock provisioning endpoint:
go run ./cmd/mock-prov -port 80

# Open in browser: http://localhost/
# Submit form → check NVS keys were received correctly
```

The form HTML in `provisioning.c` is self-contained and renders in any browser. Test:
- Valid SSID + IP → 200 + "Rebooting" message
- Empty SSID → 400 Bad Request
- Very long strings → truncation handling

---

## 3. Simulating CSI Data (Node-Level)

For testing the mothership pipeline without physical nodes, write a CSI packet generator that produces valid UDP packets in Spaxel's wire format:

```go
// tools/csi-sim/main.go
package main

import (
    "math"
    "math/rand"
    "net"
    "time"
    "encoding/binary"
)

func main() {
    conn, _ := net.Dial("udp", "localhost:4210")
    txMAC := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0x01}
    rxMAC := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0x02}

    for seq := uint32(0); ; seq++ {
        pkt := buildCSIPacket(txMAC, rxMAC, seq, simulateHuman(seq))
        conn.Write(pkt)
        time.Sleep(10 * time.Millisecond) // 100 Hz
    }
}

// simulateHuman generates amplitude perturbation pattern for a person
// walking across the Fresnel zone between tx and rx.
func simulateHuman(tick uint32) []float64 {
    amp := make([]float64, 52)
    t := float64(tick) * 0.01 // seconds
    personX := 2.5 + 1.5*math.Sin(0.3*t) // oscillates across room

    for k := range amp {
        // Static background
        amp[k] = 30.0 + float64(k)*0.1 + rand.NormFloat64()*2.0
        // Person perturbation: Fresnel zone crossing produces cosine variation
        amp[k] += 8.0 * math.Cos(2*math.Pi*personX/0.125 + float64(k)*0.2)
    }
    return amp
}
```

Scenarios to simulate:
- **Empty room**: baseline noise only
- **Walking person**: sinusoidal Fresnel zone crossing
- **Stationary person**: static offset above baseline (3–8 dB per affected link)
- **Two people**: superposition of two independent perturbation patterns
- **Breathing**: 0.25 Hz amplitude modulation on top of static offset

---

## 4. End-to-End CI Pipeline

For automated testing in CI (no hardware available):

```yaml
# .github/workflows/test.yml
jobs:
  test-server:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Start mothership stack
        run: docker-compose up -d broker spaxel

      - name: Run CSI simulator
        run: go run ./tools/csi-sim -duration 10s -nodes 4 -scenario walking

      - name: Assert blobs detected
        run: |
          sleep 5  # let positioning engine accumulate
          curl http://localhost/api/blobs | jq '.[] | select(.confidence > 0.5)' | wc -l | grep -v '^0$'

  test-firmware-build:
    runs-on: ubuntu-latest
    container: espressif/idf:v5.3
    steps:
      - uses: actions/checkout@v4
      - name: Build firmware
        working-directory: node/
        run: idf.py build
      - name: Verify binary exists
        run: test -f node/build/spaxel-node.bin
```

---

## 5. Simulating OTA

Test the full OTA cycle without flashing real hardware:

1. Publish a firmware version via the API: `POST /api/ota/publish`
2. Run a mock node client that calls `GET /api/ota/check?version=0.0.1`
3. Verify it receives the update URL
4. Verify the binary is downloadable: `GET /api/ota/firmware/0.0.2/firmware.bin`

```bash
# Publish test firmware
curl -X POST http://localhost/api/ota/publish \
  -F "version=0.0.2" \
  -F "firmware=@node/build/spaxel-node.bin"

# Check OTA as a node running 0.0.1
curl "http://localhost/api/ota/check?version=0.0.1"
# Expected: {"version":"0.0.2","url":"/api/ota/firmware/0.0.2/firmware.bin"}

# Check OTA as a node already on latest
curl "http://localhost/api/ota/check?version=0.0.2"
# Expected: 204 No Content
```
