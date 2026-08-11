# Migration Guide: From Per-Device to Fleet-Wide WiFi Configuration

**Last updated:** 2026-08-11  
**Purpose:** Guide for migrating from per-device WiFi credential entry to mothership-level fleet-wide configuration

## Overview

This guide helps you migrate from the old per-device WiFi configuration model (where each ESP32 node required manual WiFi credential entry) to the new fleet-wide model (where credentials are configured once on the mothership).

### What Changes

| Aspect | Old Model (Per-Device) | New Model (Fleet-Wide) |
|--------|------------------------|---------------------|
| **Credential entry** | Enter WiFi SSID/password for EACH node during onboarding | Enter credentials ONCE on mothership (Settings > Network) |
| **Onboarding steps** | WiFi step required for every node | WiFi step auto-skipped (credentials pre-filled) |
| **WiFi password changes** | Re-provision every node individually | Update once in Settings > Network |
| **Credential storage** | NVS on each ESP32 | SQLite database on mothership + NVS on each ESP32 |
| **Consistency** | Manual, error-prone | Automatic, consistent |

### Benefits of Migration

- **Faster onboarding:** Fewer steps per node
- **Easier maintenance:** WiFi changes in one place
- **Reduced errors:** No repeated data entry
- **Better automation:** Script fleet configuration once

---

## Migration Scenarios

### Scenario 1: Fresh Deployment (No Nodes Deployed Yet)

**Current state:** New Spaxel installation, no ESP32 nodes provisioned yet

**Migration steps:**

1. **Deploy mothership** (if not already done)
   ```bash
   docker compose up -d
   ```

2. **Configure fleet-wide WiFi credentials**
   - **Option A (Dashboard):**
     - Open `http://<server-ip>:8080`
     - Complete first-run PIN setup
     - Navigate to **Settings > Network**
     - Enter WiFi SSID and password
     - Click **Save**
   
   - **Option B (API):**
     ```bash
     # Set PIN first (first-run)
     curl -X POST http://<server-ip>:8080/api/auth/setup \
       -H "Content-Type: application/json" \
       -d '{"pin":"1234"}'
     
     # Configure WiFi
     curl -X PUT http://<server-ip>:8080/api/settings/network \
       -H "Content-Type: application/json" \
       -d '{
         "wifi_ssid": "MyNetwork",
         "wifi_password": "MyPassword"
       }'
     ```

3. **Verify configuration**
   ```bash
   curl http://<server-ip>:8080/api/settings/network
   ```
   Expected response:
   ```json
   {
     "wifi_ssid": "MyNetwork",
     "configured": true
   }
   ```

4. **Provision nodes**
   - Use Web Serial onboarding wizard for each ESP32-S3
   - WiFi step is automatically skipped
   - Nodes receive fleet-wide credentials automatically

**Time estimate:** 5-10 minutes

### Scenario 2: Existing Deployment (Nodes Already Deployed)

**Current state:** Spaxel running with nodes that have individually-configured WiFi credentials

**Migration approach:** Gradual, non-disruptive migration

**Step 1: Configure fleet-wide credentials** (doesn't affect existing nodes)

```bash
# Configure fleet-wide credentials
curl -X PUT http://mothership:8080/api/settings/network \
  -H "Content-Type: application/json" \
  -d '{"wifi_ssid":"MyNetwork","wifi_password":"NewPassword"}'
```

**Step 2: Choose migration strategy**

**Option A: Natural migration (recommended)**
- Existing nodes continue using their current NVS-stored credentials
- Next time each node is re-provisioned (firmware update, reset, maintenance), it receives fleet-wide credentials
- **Pros:** Non-disruptive, no immediate action needed
- **Cons:** Mixed state until all nodes updated
- **Timeline:** Days to weeks (natural re-provisioning cadence)

**Option B: Immediate migration**
- Re-provision all nodes via Web Serial onboarding
- All nodes immediately use fleet-wide credentials
- **Pros:** Consistent state immediately
- **Cons:** Requires physical access to each node
- **Timeline:** Hours (depending on node count)

**Option C: Maintenance-triggered migration**
- Plan firmware update for all nodes
- Configure fleet-wide credentials
- Deploy OTA update — nodes reboot with new firmware and new credentials
- **Pros:** Firmware + credentials updated together, minimizes physical access
- **Cons:** Requires downtime per node for OTA
- **Timeline:** Days (coordinated maintenance)

**Step 3: Verify migration**

```bash
# Check fleet-wide config
curl http://mothership:8080/api/settings/network

# Check individual nodes
curl http://mothership:8080/api/nodes
```

Look for all nodes showing `status: "online"` — they're successfully connected regardless of which credential source they're using.

### Scenario 3: WiFi Password Change

**Current state:** Need to change WiFi network password

**Old model (per-device):**
1. Change password on router
2. Re-provision every node individually with new password
3. Time: 30 minutes × N nodes

**New model (fleet-wide):**
1. Change password on router
2. Update credentials once in mothership
3. **Done**

**Migration steps:**

```bash
# Update fleet-wide credentials
curl -X PUT http://mothership:8080/api/settings/network \
  -H "Content-Type: application/json" \
  -d '{
    "wifi_ssid": "MyNetwork",
    "wifi_password": "NewPassword123"
  }'
```

**Impact on nodes:**
- **Immediate:** None — nodes continue using old credentials in NVS
- **Next re-provisioning:** Nodes receive new password automatically
- **For immediate effect:** Trigger OTA update or re-provisioning

**Verification:**

```bash
# Verify new credentials stored
curl http://mothership:8080/api/settings/network
```

---

## Rollback Plan

If you need to revert to per-device configuration after migration:

**Step 1: Delete fleet-wide credentials**

```bash
# Clear fleet-wide SSID
curl -X PUT http://mothership:8080/api/settings/network \
  -H "Content-Type: application/json" \
  -d '{"wifi_ssid":""}'

# Clear fleet-wide password (optional)
curl -X PUT http://mothership:8080/api/settings/network \
  -H "Content-Type: application/json" \
  -d '{"wifi_password":""}'
```

**Step 2: Re-provision nodes with per-device credentials**

For each node, use the onboarding wizard's "Advanced" mode:
1. Click "Advanced: use a different network for this node"
2. Enter WiFi SSID and password
3. Complete provisioning

**Result:** System behaves like the old per-device model

---

## Pre-Migration Checklist

Use this checklist before migrating:

- [ ] **Backup current database**
  ```bash
  curl http://mothership:8080/api/backup > spaxel-backup-$(date +%Y%m%d).zip
  ```

- [ ] **Document current WiFi credentials** (for each node if different)
  - List SSID and password for each node
  - Note any nodes on different networks

- [ ] **Verify dashboard access**
  - Can you login to the dashboard?
  - Do you have admin/PIN access?

- [ ] **Test API access** (if using scripted migration)
  ```bash
  curl http://mothership:8080/api/healthz
  curl http://mothership:8080/api/settings/network
  ```

- [ ] **Plan maintenance window** (if doing immediate migration)
  - Schedule downtime for re-provisioning
  - Notify users of temporary detection outage

---

## Post-Migration Verification

### Verification Steps

**1. Verify fleet-wide configuration**

```bash
curl http://mothership:8080/api/settings/network
```

Expected output:
```json
{
  "wifi_ssid": "YourNetwork",
  "configured": true
}
```

**2. Verify node connectivity**

```bash
curl http://mothership:8080/api/nodes
```

Check:
- All expected nodes listed?
- Status showing `online` or `stale` (not `offline`)?
- Last seen timestamps recent?

**3. Verify CSI streaming**

For each node, check:
- Dashboard shows node as ONLINE (green)
- CSI frames being received (check `/api/links` endpoint)
- Motion detection working (walk through coverage area)

**4. Verify onboarding**

Test provisioning a new node:
- Use Web Serial onboarding wizard
- WiFi step should auto-skip (showing "Fleet network already configured")
- Node should come online with correct credentials

---

## Common Migration Issues and Solutions

### Issue 1: "No WiFi credentials configured" error

**Symptom:** Onboarding wizard shows error; provisioning API returns 400

**Cause:** Fleet-wide credentials not configured

**Solution:**
```bash
# Set credentials
curl -X PUT http://mothership:8080/api/settings/network \
  -H "Content-Type: application/json" \
  -d '{"wifi_ssid":"MyNetwork","wifi_password":"MyPassword"}'
```

### Issue 2: Nodes won't join WiFi after migration

**Symptom:** Node shows `status: "offline"` after provisioning

**Possible causes:**

1. **Wrong credentials:**
   ```bash
   # Verify stored credentials
   curl http://mothership:8080/api/settings/network
   # Re-enter if incorrect
   ```

2. **Network not reachable:**
   - Verify SSID matches actual network name
   - Check password (case-sensitive)
   - Test network with another device

3. **Node hardware issue:**
   - Check node logs via serial
   - Verify antenna connection
   - Try re-provisioning

### Issue 3: Some nodes on different networks

**Scenario:** Some nodes need different SSID (isolated sensors, multiple buildings)

**Solution:** Use per-node override

**Via dashboard:**
1. Start onboarding wizard
2. Click "Advanced: use a different network for this node"
3. Enter credentials for that node's network
4. Complete provisioning

**Via API:**
```bash
curl -X POST http://mothership:8080/api/provision \
  -H "Content-Type: application/json" \
  -d '{
    "mac": "AA:BB:CC:DD:EE:FF",
    "wifi_ssid": "DifferentNetwork",
    "wifi_pass": "differentpass"
  }'
```

### Issue 4: Environment variables don't work

**Symptom:** Set `SPAXEL_WIFI_SSID`/`SPAXEL_WIFI_PASSWORD` but nodes don't use them

**Explanation:** These env vars are **documented but NOT implemented** (see [`environment-variables.md`](environment-variables.md))

**Solution:** Use dashboard or API instead:
- Dashboard: Settings > Network
- API: `PUT /api/settings/network`

---

## Migration Scripts

### Script 1: Automated Migration (API-based)

```bash
#!/bin/bash
# migrate-to-fleet-wide.sh

set -e

MOTHERSHIP="${1:-localhost:8080}"
WIFI_SSID="${2:-MyNetwork}"
WIFI_PASS="${3:-MyPassword}"

echo "Spaxel Fleet-Wide WiFi Migration"
echo "================================"
echo "Mothership: $MOTHERSHIP"
echo "WiFi SSID: $WIFI_SSID"
echo ""

# Test mothership connectivity
echo "Testing mothership connectivity..."
if ! curl -s http://$MOTHERSHIP/api/healthz > /dev/null; then
    echo "ERROR: Cannot reach mothership at $MOTHERSHIP"
    echo "Check if mothership is running and port 8080 is accessible"
    exit 1
fi
echo "✓ Mothership reachable"
echo ""

# Configure fleet-wide credentials
echo "Configuring fleet-wide WiFi credentials..."
curl -X PUT http://$MOTHERSHIP/api/settings/network \
  -H "Content-Type: application/json" \
  -d "{
    \"wifi_ssid\": \"$WIFI_SSID\",
    \"wifi_password\": \"$WIFI_PASS\"
  }"

echo ""
echo "Verifying configuration..."
RESPONSE=$(curl -s http://$MOTHERSHIP/api/settings/network)
echo "Current configuration:"
echo "$RESPONSE" | jq '.'

if echo "$RESPONSE" | jq -e '.configured != true'; then
    echo "WARNING: Configuration shows as not configured"
    echo "Check the API response above for details"
    exit 1
fi

echo ""
echo "✓ Migration complete!"
echo ""
echo "Next steps:"
echo "1. Provision new nodes via Web Serial — they will auto-join '$WIFI_SSID'"
echo "2. Existing nodes will use fleet credentials on next re-provisioning"
echo "3. To update existing nodes immediately, re-provision each via Web Serial"
echo ""
echo "Dashboard: http://$MOTHERSHIP"
```

**Usage:**
```bash
chmod +x migrate-to-fleet-wide.sh
./migrate-to-fleet-wide.sh localhost:8080 MyNetwork MyPassword
```

### Script 2: Verify Migration

```bash
#!/bin/bash
# verify-migration.sh

set -e

MOTHERSHIP="${1:-localhost:8080}"

echo "Verifying Spaxel fleet-wide WiFi configuration"
echo "============================================"
echo ""

# Check 1: Fleet config
echo "1. Checking fleet-wide configuration..."
FLEET_CONFIG=$(curl -s http://$MOTHERSHIP/api/settings/network)
echo "$FLEET_CONFIG" | jq '.'
echo ""

# Check 2: Node connectivity
echo "2. Checking node connectivity..."
NODES=$(curl -s http://$MOTHERSHIP/api/nodes)
ONLINE_COUNT=$(echo "$NODES" | jq '[.[] | select(.status == "online") | length]')

echo "Total nodes: $(echo "$NODES" | jq 'length')"
echo "Online nodes: $ONLINE_COUNT"
echo ""

# Check 3: CSI links
echo "3. Checking CSI links..."
LINKS=$(curl -s http://$MOTHERSHIP/api/links)
ACTIVE_LINKS=$(echo "$LINKS" | jq 'length')

echo "Active CSI links: $ACTIVE_LINKS"
echo ""

if [ "$ONLINE_COUNT" -gt 0 ] && [ "$ACTIVE_LINKS" -gt 0 ]; then
    echo "✓ Migration verification PASSED"
    echo ""
    echo "System is operational with fleet-wide WiFi configuration"
else
    echo "✗ Migration verification FAILED"
    echo ""
    echo "Issues detected:"
    [ "$ONLINE_COUNT" -eq 0 ] && echo "- No nodes online"
    [ "$ACTIVE_LINKS" -eq 0 ] && echo "- No CSI links active"
    echo ""
    echo "Check dashboard for diagnostics: http://$MOTHERSHIP"
fi
```

**Usage:**
```bash
chmod +x verify-migration.sh
./verify-migration.sh localhost:8080
```

---

## Timeline Examples

### Small Fleet (6 nodes, immediate migration)

| Time | Action | Notes |
|------|--------|-------|
| 0:00 | Configure fleet credentials via dashboard | 2 minutes |
| 0:02 | Re-provision 6 nodes via Web Serial | 15 minutes (2.5 min/node) |
| 0:17 | Verification | All nodes online, CSI streaming |

**Total migration time:** ~17 minutes

### Large Fleet (30 nodes, gradual migration)

| Time | Action | Notes |
|------|--------|-------|
| Week 1 | Configure fleet credentials | 2 minutes |
| Week 1-4 | Natural migration during maintenance | ~5 nodes/week re-provisioned naturally |
| Week 4 | Trigger OTA update for remaining nodes | 2 weeks for all nodes |
| Week 6 | Verification | All nodes on fleet credentials |

**Total migration time:** ~6 weeks (background, non-disruptive)

---

## FAQ

### Q: Can I mix fleet-wide and per-node credentials?

**A:** Yes. Configure fleet-wide credentials as the default, then use "Advanced" mode in onboarding or explicit `POST /api/provision` calls for nodes on different networks.

### Q: What happens if I change the fleet-wide password?

**A:** The new password is stored in the mothership database. Existing nodes continue using their old NVS-stored password until re-provisioned. New nodes and re-provisioned nodes get the new password.

### Q: Do I need to re-provision all nodes immediately?

**A:** No. Existing nodes continue working. Re-provision them naturally (firmware updates, maintenance) or intentionally when convenient. The fleet-wide credentials only affect new and re-provisioned nodes.

### Q: Can I revert to per-device configuration?

**A:** Yes. Delete the fleet-wide credentials (see Rollback Plan above) and re-provision nodes individually. The system supports both models simultaneously.

### Q: Will environment variables ever work for WiFi?

**A:** Not in the current implementation. The ADR-005 design intent was never coded. Use the dashboard Settings > Network panel or `PUT /api/settings/network` API instead.

### Q: Is my WiFi password secure?

**A:** Yes. WiFi passwords are stored:
- In the mothership SQLite database (protected by dashboard PIN)
- In ESP32 NVS (protected by flash encryption, if enabled)
- Never returned by the API (write-only)
- Never logged or exposed in diagnostics

Use HTTPS/reverse proxy for WAN access.

---

## Related Documentation

- [`wifi-configuration.md`](wifi-configuration.md) — Complete WiFi configuration guide
- [`environment-variables.md`](environment-variables.md) — Environment variable reference
- [`../wifi-credential-provisioning-flow.md`](../wifi-credential-provisioning-flow.md) — Technical implementation audit
- [`../../README.md`](../../README.md) — Project quickstart
