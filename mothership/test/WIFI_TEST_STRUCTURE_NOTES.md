# WiFi Credentials Test Structure - Exploration Notes

## Test File Location

All WiFi credential tests are located in:
```
/home/coding/spaxel/mothership/test/
```

### Existing Test Files

| File | Purpose |
|------|---------|
| `wifi_credential_missing_env_vars_test.go` | Tests behavior when WIFI_SSID/WIFI_PASSWORD env vars are not set |
| `wifi_credential_env_test.go` | Tests environment variable provisioning flows |
| `wifi_credential_e2e_test.go` | End-to-end acceptance criteria tests |
| `wifi_credential_flow_test.go` | Tests various provisioning flows and fallbacks |

## Test Naming Convention

Pattern: `wifi_credential_<purpose>_<scenario>_test.go`

Examples:
- `wifi_credential_missing_env_vars_test.go`
- `wifi_credential_env_test.go` 
- `wifi_credential_e2e_test.go`
- `wifi_credential_flow_test.go`

## How Environment Variables Are Set in Tests

### Method 1: Using `t.Setenv()`
```go
// Set env var for test (automatically restored after test)
t.Setenv("SPAXEL_WIFI_SSID", "TestSSID")
t.Setenv("SPAXEL_WIFI_PASSWORD", "TestPass123")

// Clear env var
t.Setenv("SPAXEL_WIFI_SSID", "")
```

### Method 2: Using `os.Unsetenv()`
```go
// Permanently unset for test duration
os.Unsetenv("SPAXEL_WIFI_SSID")
os.Unsetenv("SPAXEL_WIFI_PASSWORD")
```

### Method 3: Using Helper Function (from `config_test.go`)
```go
func clearEnvVars() {
    envVars := []string{
        "SPAXEL_WIFI_SSID",
        "SPAXEL_WIFI_PASSWORD",
        // ... other env vars
    }
    for _, v := range envVars {
        os.Unsetenv(v)
    }
}
```

## WiFi Credential Validation Implementation

### Configuration Loading
Location: `/home/coding/spaxel/mothership/internal/config/config.go`

```go
type Config struct {
    // WiFi credentials (optional, first-boot seeding only per ADR-005)
    WifiSSID     string // SPAXEL_WIFI_SSID - seeds DB on first boot only, ignored after
    WifiPassword string // SPAXEL_WIFI_PASSWORD - seeds DB on first boot only, ignored after
}
```

### Validation Logic
```go
// Environment variables are read with defaults
cfg.WifiSSID = envOr("SPAXEL_WIFI_SSID", "")
cfg.WifiPassword = envOr("SPAXEL_WIFI_PASSWORD", "")

// Logged when set (but no validation errors are raised)
if cfg.WifiSSID != "" {
    log.Printf("[CONFIG] SPAXEL_WIFI_SSID=%s (will seed DB on first boot if no existing setting)", cfg.WifiSSID)
}
```

**Key Point:** WiFi credentials are **optional** and receive **no validation** in config loading. Missing or empty credentials are allowed per ADR-005 design (captive portal onboarding).

## Existing Error Messages and Patterns

### Test Assertion Messages

```go
// Missing credentials
t.Error("Expected empty SSID when no credentials configured, got %q", payload.WifiSSID)
t.Error("Expected empty password when no credentials configured, got %q", payload.WifiPass)

// Credential presence checks
t.Error("Expected non-empty WiFi SSID in provisioning payload")
t.Error("Expected non-empty WiFi password in provisioning payload")

// Fallback behavior
t.Errorf("Expected database SSID %q when env vars missing, got %q", dbSSID, payload.WifiSSID)
```

### Config Validation Pattern (for other settings)
```go
// Example from config_test.go - shows validation pattern for OTHER settings
if !strings.Contains(err.Error(), "must be in range [1,20]") {
    t.Errorf("error = %v, want containing 'must be in range [1,20]'", err)
}
```

**Note:** WiFi credentials do NOT follow this validation pattern - they are explicitly allowed to be empty/missing.

## Current Test Patterns

### Test Structure Template
```go
func TestWiFiCredential_<Scenario>(t *testing.T) {
    t.Run("<SubScenario>", func(t *testing.T) {
        // 1. Setup: Create temp dir and database
        tmpDir := t.TempDir()
        dbPath := filepath.Join(tmpDir, "test.db")
        db, _ := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)...")
        
        // 2. Set/clear environment variables
        t.Setenv("SPAXEL_WIFI_SSID", "TestSSID")
        t.Setenv("SPAXEL_WIFI_PASSWORD", "TestPass123")
        
        // 3. Create handlers and server
        settingsHandler := api.NewSettingsHandler(db)
        provSrv := provisioning.NewServer(tmpDir, "spaxel", 8080, "pool.ntp.org", "")
        provSrv.SetSettingsProvider(settingsHandler)
        
        // 4. Execute test
        provReq := httptest.NewRequest(http.MethodPost, "/api/provision", nil)
        provRR := httptest.NewRecorder()
        provSrv.HandleProvision(provRR, provReq)
        
        // 5. Assert results
        if provRR.Code != http.StatusOK {
            t.Errorf("Expected HTTP 200, got %d", provRR.Code)
        }
        
        // 6. Decode and validate payload
        var payload provisioning.Payload
        json.NewDecoder(provRR.Body).Decode(&payload)
        // ... assertions
    })
}
```

### Panic Recovery Pattern
```go
// Tests verify no panic occurs
defer func() {
    if r := recover(); r != nil {
        t.Errorf("Provisioning with missing credentials panicked: %v", r)
    }
}()
// ... code that should not panic
```

## ADR-005 Design Context

The tests implement ADR-005 which states:

1. **WiFi credentials are optional** - no validation errors should be raised
2. **Empty credentials are allowed** - enables captive portal onboarding
3. **Environment variables seed database on first boot only** - ignored after
4. **Database settings are authoritative** after first boot
5. **Request body overrides both** env vars and database settings

## Where to Add New Tests

For a new test related to WiFi credential validation:

1. **Create new file**: `/home/coding/spaxel/mothership/test/wifi_credential_<purpose>_test.go`
2. **Use existing patterns**: Follow the template above
3. **Test package**: `package test`
4. **Import dependencies**:
   ```go
   import (
       "database/sql"
       "encoding/json"
       "net/http"
       "net/http/httptest"
       "os"
       "path/filepath"
       "strings"
       "testing"
       _ "modernc.org/sqlite"
       "github.com/go-chi/chi/v5"
       "github.com/spaxel/mothership/internal/api"
       "github.com/spaxel/mothership/internal/provisioning"
   )
   ```

## Key Validation Points

### 1. No Panic on Missing Credentials
Location: `wifi_credential_missing_env_vars_test.go:93-96`
```go
defer func() {
    if r := recover(); r != nil {
        t.Errorf("Provisioning with missing credentials panicked: %v", r)
    }
}()
```

### 2. Empty Credentials Are Returned
Location: `wifi_credential_missing_env_vars_test.go:119-126`
```go
// Verify the response clearly indicates missing credentials
// (empty wifi_ssid and wifi_pass fields)
if payload.WifiSSID != "" {
    t.Errorf("Expected empty SSID when no credentials configured, got %q", payload.WifiSSID)
}
if payload.WifiPass != "" {
    t.Errorf("Expected empty password when no credentials configured, got %q", payload.WifiPass)
}
```

### 3. Essential Fields Still Generated
Location: `wifi_credential_missing_env_vars_test.go:128-144`
```go
// Verify essential provisioning fields are still generated
// (device can still be provisioned for captive portal onboarding)
if payload.NodeID == "" {
    t.Error("Expected node_id to be generated even without WiFi credentials")
}
if payload.NodeToken == "" {
    t.Error("Expected node_token to be generated even without WiFi credentials")
}
```

## Summary

**Test Structure:** Well-organized with separate files by purpose (env vars, missing vars, e2e, flows)

**Environment Setup:** Uses `t.Setenv()` for test-scoped changes, `os.Unsetenv()` for permanent clears, or helper functions

**Validation Approach:** WiFi credentials are intentionally NOT validated (per ADR-005) - tests verify graceful handling of missing/empty credentials

**Error Pattern:** No validation errors are raised for WiFi credentials - tests check for empty values and ensure no panics occur

**Where to Add:** New tests go in `/home/coding/spaxel/mothership/test/wifi_credential_<purpose>_test.go` following the established template pattern
