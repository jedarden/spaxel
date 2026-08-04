# BF-64SF9: Wire POST /api/provision to default wifi_ssid/wifi_pass from stored network setting

## Status: VERIFIED AND COMPLETE

This feature has already been fully implemented in commit `ec72d40` on 2026-08-03.

## Implementation Details

### Code Location
- **Main Handler**: `mothership/internal/provisioning/server.go` (lines 193-269)
- **Settings Provider Wiring**: `mothership/cmd/mothership/main.go` (line 4723)
- **Network Settings API**: `mothership/internal/api/network_settings.go`

### How It Works

1. **Settings Provider Interface** (`provisioning/server.go` lines 46-52):
   - A narrow `NetworkSettingsProvider` interface allows the provisioning server to access stored network settings
   - Mirrors the pattern used by `internal/ota`

2. **Defaulting Logic** (`provisioning/server.go` lines 218-238):
   ```go
   wifiSSID := req.WifiSSID
   wifiPass := req.WifiPass
   if wifiSSID == "" {
       if v, ok := s.networkSetting(networkSettingWifiSSID); ok {
           wifiSSID = v
       }
   }
   if wifiPass == "" {
       if v, ok := s.networkSetting(networkSettingWifiPassword); ok {
           wifiPass = v
       }
   }
   ```

3. **Fallback Behavior**:
   - If request body omits `wifi_ssid`/`wifi_pass`, use fleet-wide Settings > Network values
   - Explicit request values override stored settings
   - Returns 400 error if no credentials available (fails closed rather than provisioning dead nodes)

### Settings Keys
- `network_wifi_ssid` - Fleet-wide WiFi SSID
- `network_wifi_password` - Fleet-wide WiFi password (write-only, never returned in GET)

## Verification

All provisioning tests pass:
```
✓ TestHandleProvision_DefaultsFromSettingsProvider
✓ TestHandleProvision_RequestBodyOverridesStoredSetting  
✓ TestHandleProvision_ErrorsWithNoCredentialsAvailable
✓ TestHandleProvision_ErrorsWhenProviderHasNoValue
✓ TestHandleProvision_SucceedsWithSSIDOnlyOpenNetwork
```

The implementation correctly:
- Defaults to stored network settings when request omits credentials
- Allows explicit request values to override stored settings
- Returns clear 400 error when no credentials are available from any source
- Supports open networks (SSID-only, no password)

## ADR-005 Compliance

This is part of ADR-005: "WiFi config belongs in the dashboard, not env vars"

- Users configure fleet-wide WiFi once in Settings > Network dashboard
- All new nodes automatically join the fleet network
- No per-node WiFi configuration at flash time (unless explicit override)
