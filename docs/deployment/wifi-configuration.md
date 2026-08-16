# WiFi Configuration for Spaxel Deployments

**Last updated:** 2026-08-11  
**Purpose:** Comprehensive guide for configuring fleet-wide WiFi credentials in Spaxel deployments

## Overview

Spaxel uses a **mothership-level WiFi configuration model** — you configure the fleet's WiFi network **once** on the mothership, and all nodes automatically join that network. This replaces the older per-device configuration approach where each node required manual WiFi credential entry.

### Key Concepts

- **Fleet-wide credentials:** Stored once in the mothership's database (Settings > Network panel)
- **Database-backed:** Credentials persist in SQLite; environment variables seed an empty database on first boot
- **Zero-touch onboarding:** Nodes automatically receive credentials during provisioning

---

## Configuration Methods

### Method 1: Dashboard UI (Recommended)

**Best for:** Home deployments, interactive setups, first-time users

**Steps:**

1. Start the mothership container
2. Open dashboard at `http://<server-ip>:8080`
3. Complete first-run PIN setup
4. Navigate to **Settings > Network**
5. Enter your WiFi network credentials:
   - **WiFi SSID:** Your network name (max 32 characters)
   - **WiFi Password:** Your WPA2 password (min 8 characters; leave empty for open networks)
6. Click **Save**

**Result:** All subsequent nodes provisioned via the onboarding wizard will automatically join this network. The wizard displays the fleet status and never asks for per-device credentials.

### Method 2: REST API

**Best for:** Headless deployments, automation, scripted setups

**Step 1: First-run PIN setup**

The dashboard requires a PIN on first run. You can set this via the API:

```bash
# Set initial PIN
curl -X POST http://mothership:8080/api/auth/setup \
  -H "Content-Type: application/json" \
  -d '{"pin":"1234"}'
```

**Step 2: Configure WiFi credentials**

```bash
# Set fleet WiFi credentials
curl -X PUT http://mothership:8080/api/settings/network \
  -H "Content-Type: application/json" \
  -d '{
    "wifi_ssid": "MyNetwork",
    "wifi_password": "MyPassword"
  }'
```

**Step 3: Verify configuration**

```bash
curl http://mothership:8080/api/settings/network
```

Response:
```json
{
  "wifi_ssid": "MyNetwork",
  "configured": true
}
```

### Method 3: Docker Compose (Environment Variable)

**Best for:** Headless deployments and reproducible first-boot setup

Set both variables on the mothership:

```yaml
environment:
  SPAXEL_WIFI_SSID: "MyNetwork"
  SPAXEL_WIFI_PASSWORD: "MyPassword"
```

On first boot, when no fleet network setting exists, the mothership seeds the
database from these values. Settings > Network remains authoritative afterward;
changing the environment on a later restart does not overwrite stored values.

---

## Deployment Examples

### Example 1: Single Home Deployment (Docker Compose)

**Scenario:** Typical home deployment with 2-6 ESP32-S3 nodes

```yaml
# docker-compose.yml
services:
  spaxel:
    image: ronaldraygun/spaxel:latest
    network_mode: host  # Required for mDNS
    volumes:
      - spaxel-data:/data
    environment:
      TZ: America/New_York
      SPAXEL_WIFI_SSID: "MyNetwork"
      SPAXEL_WIFI_PASSWORD: "MyPassword"
    restart: unless-stopped
```

**Post-deployment steps:**

1. Start container: `docker compose up -d`
2. Open dashboard: `http://<server-ip>:8080`
3. Setup PIN: Follow first-run wizard
4. Configure WiFi: first boot seeds the fleet setting from the environment; Settings > Network can update it later
5. Add nodes: All nodes auto-join the configured network

### Example 2: Multi-Network Deployment

**Scenario:** Some nodes on isolated network (e.g., outdoor sensor on separate AP)

**Step 1: Configure fleet-wide network**

```bash
curl -X PUT http://mothership:8080/api/settings/network \
  -H "Content-Type: application/json" \
  -d '{"wifi_ssid":"MainHome","wifi_password":"homepass"}'
```

**Step 2: Provision normal nodes**

Use the standard onboarding wizard — nodes automatically join "MainHome".

**Step 3: Provision isolated node**

```bash
# Provision with explicit network override
curl -X POST http://mothership:8080/api/provision \
  -H "Content-Type: application/json" \
  -d '{
    "mac": "AA:BB:CC:DD:EE:FF",
    "wifi_ssid": "OutdoorAP",
    "wifi_pass": "outdoorpass"
  }'
```

The isolated node joins "OutdoorAP" while all other nodes use "MainHome".

### Example 3: Headless/Scripted Deployment

**Scenario:** Automated deployment without dashboard interaction

```bash
#!/bin/bash
# deploy-spaxel.sh

MOTHERSHIP_IP="192.168.1.100"
WIFI_SSID="MyNetwork"
WIFI_PASS="MyPassword"
PIN="1234"

# 1. Start container
docker compose up -d

# 2. Wait for mothership to be ready
sleep 10

# 3. Set PIN (first-run setup)
curl -X POST http://${MOTHERSHIP_IP}:8080/api/auth/setup \
  -H "Content-Type: application/json" \
  -d "{\"pin\":\"${PIN}\"}"

# 4. Configure WiFi credentials
curl -X PUT http://${MOTHERSHIP_IP}:8080/api/settings/network \
  -H "Content-Type: application/json" \
  -d "{
    \"wifi_ssid\": \"${WIFI_SSID}\",
    \"wifi_password\": \"${WIFI_PASS}\"
  }"

# 5. Verify configuration
curl http://${MOTHERSHIP_IP}:8080/api/settings/network

echo "Spaxel configured. WiFi SSID: ${WIFI_SSID}"
echo "Dashboard: http://${MOTHERSHIP_IP}:8080"
echo "Add nodes via Web Serial onboarding wizard."
```

### Example 4: Kubernetes Deployment

**Scenario:** Spaxel running in Kubernetes cluster

**Step 1: Create ConfigMap for initial PIN (optional)**

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: spaxel-bootstrap
  namespace: home
data:
  setup.sh: |
    #!/bin/sh
    curl -X POST http://spaxel:8080/api/auth/setup \
      -H "Content-Type: application/json" \
      -d '{"pin":"1234"}'
```

**Step 2: Deploy Spaxel**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: spaxel
  namespace: home
spec:
  replicas: 1
  selector:
    matchLabels:
      app: spaxel
  template:
    metadata:
      labels:
        app: spaxel
    spec:
      containers:
      - name: spaxel
        image: ronaldraygun/spaxel:latest
        ports:
        - containerPort: 8080
        env:
        - name: TZ
          value: "America/New_York"
        - name: SPAXEL_MDNS_ENABLED
          value: "true"
        volumeMounts:
        - name: data
          mountPath: /data
      volumes:
      - name: data
        persistentVolumeClaim:
          claimName: spaxel-data
      # Use hostNetwork for mDNS (requires Pod security context)
      hostNetwork: true
```

**Step 3: Configure WiFi via API (post-deployment)**

```bash
# Port-forward to access dashboard
kubectl port-forward -n home deployment/spaxel 8080:8080

# Configure WiFi
curl -X PUT http://localhost:8080/api/settings/network \
  -H "Content-Type: application/json" \
  -d '{"wifi_ssid":"MyNetwork","wifi_password":"MyPassword"}'
```

### Example 5: Large-Scale Deployment (10+ Nodes)

**Scenario:** Commercial deployment with many nodes across multiple buildings

**Best practices:**

1. **Configure once, provision many:**
   ```bash
   # Configure fleet-wide credentials
   curl -X PUT http://mothership:8080/api/settings/network \
     -H "Content-Type: application/json" \
     -d '{"wifi_ssid":"CorpNetwork","wifi_password":"corppass"}'
   
   # Batch-provision nodes (all auto-join CorpNetwork)
   for node in node01 node02 node03 ...; do
     # Use Web Serial onboarding wizard for each node
     # Wizard displays the fleet network; no per-node WiFi fields are shown
   done
   ```

2. **Segment by building/floor:**
   - Each floor on a different VLAN? Use a separate mothership deployment
   - Each building has a different AP? Configure that mothership's fleet network

3. **Automate with Ansible:**
   ```yaml
   # ansible-playbook spaxel-nodes.yml
   
   - name: Configure Spaxel mothership WiFi
     uri:
       url: "http://{{ mothership_ip }}:8080/api/settings/network"
       method: PUT
       body_format: json
       body:
         wifi_ssid: "{{ fleet_wifi_ssid }}"
         wifi_password: "{{ fleet_wifi_password }}"
   
   - name: Provision ESP32 nodes
     # ... Web Serial automation or esptool provisioning
   ```

---

## Migration Guide: From Per-Device to Fleet-Wide

### Old Model (Per-Device)

**What you used to do:**

1. Provision node via Web Serial onboarding
2. Manually enter WiFi SSID and password for **each node**
3. Repeat for every single device
4. If WiFi password changes: Re-provision every node individually

**Problems:**
- Repetitive data entry
- Error-prone (typos, forgotten passwords)
- Time-consuming for large fleets
- WiFi password changes require full re-provisioning

### New Model (Fleet-Wide)

**What you do now:**

1. **One-time setup:** Configure WiFi credentials once in mothership Settings > Network
2. **Provision nodes:** Use onboarding wizard — it reads the fleet network from the mothership
3. **WiFi change:** Update credentials once in Settings > Network, done

**Benefits:**
- Configure once, provision many
- WiFi changes in one place
- Faster onboarding (fewer steps)
- Consistent credential management

### Migration Steps

**If you already have deployed nodes:**

**Step 1: Configure fleet-wide credentials**

```bash
# Via dashboard or API
curl -X PUT http://mothership:8080/api/settings/network \
  -H "Content-Type: application/json" \
  -d '{"wifi_ssid":"MyNetwork","wifi_password":"NewPassword"}'
```

**Step 2: No node re-provisioning needed**

Existing nodes continue working with their NVS-stored credentials. The next time you re-provision a node (firmware update, reset, etc.), it will receive the fleet-wide credentials.

**Step 3: Gradual migration (optional)**

If you want all nodes to use the fleet-wide credentials immediately:

1. Re-provision each node via Web Serial onboarding
2. The wizard shows the configured fleet network; no WiFi fields are entered per node
3. Node receives updated credentials

Or simply re-provision nodes naturally as needed (firmware updates, maintenance).

---

## Credential Precedence Rules

When provisioning a node, credentials are selected in this order:

1. **Database settings** (fleet-wide default)
   - `network_wifi_ssid` / `network_wifi_password` from Settings > Network
   - Use case: Normal node provisioning

2. **First-boot environment seed**
   - `SPAXEL_WIFI_SSID` / `SPAXEL_WIFI_PASSWORD`
   - Applies only when no database settings exist

3. **Explicit API override** (direct API callers only)
   - `wifi_ssid` / `wifi_pass` in `POST /api/provision`
   - The onboarding wizard does not expose or send this override

4. **Empty credentials** (if neither available)
   - The mothership returns a payload without WiFi credentials and logs a warning
   - Configure the fleet network before provisioning production nodes

---

## Validation Rules

### API Layer (`/api/settings/network`)

| Field | Required | Validation | Error Message |
|-------|----------|------------|---------------|
| `wifi_ssid` | Yes | Max 32 chars, not empty after trim | "wifi_ssid: must not be empty" or "wifi_ssid: must be 32 characters or fewer" |
| `wifi_password` | No | Min 8 chars if non-empty; empty allowed for open networks | "wifi_password: must be at least 8 characters (WPA2 minimum) or empty for an open network" |

### Firmware Layer (NVS storage)

| Field | Max Storage | Validation |
|-------|------------|------------|
| `wifi_ssid` | 32 bytes | Non-empty required |
| `wifi_password` | 64 bytes | Any length including empty (open networks) |

---

## Troubleshooting

### Problem: "No WiFi credentials configured" error

**Cause:** Neither database settings nor request override provided credentials

**Solution:**

```bash
# Check if configured
curl http://mothership:8080/api/settings/network

# If not configured, set credentials
curl -X PUT http://mothership:8080/api/settings/network \
  -H "Content-Type: application/json" \
  -d '{"wifi_ssid":"MyNetwork","wifi_password":"MyPassword"}'
```

### Problem: Node won't join WiFi after provisioning

**Diagnostic steps:**

1. **Check credentials were sent:**
   ```bash
   # View node registry
   curl http://mothership:8080/api/nodes
   ```
   Look for `last_seen_ms` timestamp — if updating, node is online

2. **Check node logs via serial:**
   - Connect via USB-Serial/JTAG
   - Look for WiFi association messages
   - Check for "WiFi connection failed" errors

3. **Verify credentials are correct:**
   - Re-enter credentials in Settings > Network
   - Re-provision the node

### Problem: Need different networks for different nodes

**Solution:** Use per-node override in provisioning API

```bash
# Node on different network
curl -X POST http://mothership:8080/api/provision \
  -H "Content-Type: application/json" \
  -d '{
    "mac": "AA:BB:CC:DD:EE:FF",
    "wifi_ssid": "DifferentNetwork",
    "wifi_pass": "differentpass"
  }'
```

---

## Security Considerations

### Credential Storage

- **Location:** SQLite database at `/data/spaxel.db`
- **Table:** `settings` table
- **Keys:** `network_wifi_ssid`, `network_wifi_password`
- **Access:** Protected by dashboard PIN authentication

### Credential Exposure

- **Password is write-only:** GET `/api/settings/network` never returns password
- **TLS recommended:** Use reverse proxy with HTTPS for WAN access
- **LAN-only deployment:** No TLS required for local network

### Backup and Restore

WiFi credentials are included in database backups:

```bash
# Manual backup
curl http://mothership:8080/api/backup > spaxel-backup.zip

# Restore (stops container, extracts backup)
docker compose down
# Extract backup to /data/
docker compose up -d
```

---

## API Reference

### GET /api/settings/network

Retrieve fleet WiFi configuration status.

**Response:**
```json
{
  "wifi_ssid": "MyNetwork",
  "configured": true
}
```

**Note:** `wifi_password` is never returned (write-only).

### PUT /api/settings/network

Update fleet WiFi credentials.

**Request:**
```json
{
  "wifi_ssid": "MyNetwork",
  "wifi_password": "MyPassword"
}
```

**Partial updates:** Omit fields to leave unchanged. Use empty string for password to clear.

**Validation:**
- `wifi_ssid`: Max 32 chars, required (non-empty after trim)
- `wifi_password`: Min 8 chars if non-empty, optional (empty = open network)

**Response:** Updated configuration (same as GET)

### POST /api/provision

Provision a node with WiFi credentials.

**Request:**
```json
{
  "mac": "AA:BB:CC:DD:EE:FF",
  "wifi_ssid": "MyNetwork",
  "wifi_pass": "MyPassword"
}
```

**Credential precedence:**
1. Request body `wifi_ssid`/`wifi_pass` (if provided)
2. Fleet network settings from database (if request body omitted)

**Response:**
```json
{
  "version": 1,
  "wifi_ssid": "MyNetwork",
  "wifi_pass": "MyPassword",
  "node_id": "f47ac10b-...",
  "node_token": "a1b2c3...",
  "ms_mdns": "spaxel",
  "ms_ip": "",
  "ms_port": 8080,
  "ntp_server": "pool.ntp.org",
  "debug": false
}
```

---

## Related Documentation

- [`../plan/plan.md`](../plan/plan.md) - ADR-005: Architecture decision for fleet-wide WiFi configuration
- [`../wifi-credential-provisioning-flow.md`](../wifi-credential-provisioning-flow.md) - Complete audit of credential flow implementation
- [`../notes/mdns-override.md`](../notes/mdns-override.md) - mDNS troubleshooting
- [`README.md`](../../README.md) - Project quickstart and overview
