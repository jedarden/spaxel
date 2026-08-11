# Canary Rollback Version Tracking Research

**Date:** 2026-08-11  
**Bead:** bf-3rao3  
**Scope:** Research only — documents where previous firmware version is stored for canary rollback

---

## Executive Summary

The AutoUpdateManager **already tracks** the canary node's previous firmware version for rollback purposes. This research confirms where the data lives, how it's populated, and what additional information would be useful for a more robust rollback mechanism.

---

## 1. Current Implementation

### 1.1 Storage Location

**File:** `mothership/internal/ota/autoupdate.go`  
**Struct:** `AutoUpdateManager`  
**Field:** `canaryPreviousVersion string` (line 52)

```go
type AutoUpdateManager struct {
    // ... other fields ...
    currentCanaryNode     string
    canaryPreviousVersion string // Firmware version before canary update, for rollback
    baselineQuality       float64
    // ... other fields ...
}
```

**Key characteristics:**
- **Storage:** In-memory only (not persisted to SQLite)
- **Lifetime:** Exists only during a single auto-update cycle
- **Scope:** Per-canary-node (only one canary at a time)
- **Type:** `string` (semantic version only, e.g., "0.2.19")

### 1.2 How It's Populated

**Location:** `autoupdate.go:466-473` in `startUpdateCycle()`

```go
// Store the canary node's current firmware version for potential rollback
m.mu.Lock()
if m.nodeProvider != nil {
    m.canaryPreviousVersion = m.nodeProvider.GetNodeFirmwareVersion(canaryMAC)
} else {
    m.canaryPreviousVersion = ""
}
m.mu.Unlock()
```

**Data source:** The `NodeProvider` interface's `GetNodeFirmwareVersion(mac)` method reads from the `nodes.firmware_version` column in SQLite (see `migrations.go:250`).

**Timing:** Captured **after** canary node selection but **before** the OTA command is sent.

### 1.3 How It's Used for Rollback

**Location:** `autoupdate.go:646-657` in `evaluateCanary()`

```go
// Trigger rollback to previous firmware version
if previousVersion != "" {
    if err := m.otaManager.SendOTAVersion(canaryMAC, previousVersion); err != nil {
        log.Printf("[ERROR] ota: failed to trigger rollback for canary %s to version %s: %v",
            canaryMAC, previousVersion, err)
        m.failUpdateCycle(fmt.Sprintf("canary quality degraded and rollback failed: %v", err))
        return
    }
    log.Printf("[INFO] ota: triggered rollback for canary %s to version %s", canaryMAC, previousVersion)
} else {
    log.Printf("[WARN] ota: cannot rollback canary %s: previous firmware version unknown", canaryMAC)
}
```

**Rollback trigger:** Quality degradation exceeds threshold (`qualityChanged > config.QualityThreshold`)

**Method:** `SendOTAVersion(mac, versionOrFilename)` (manager.go:135-144) resolves the version to a `FirmwareMeta` via `server.GetByVersion()`, constructs the full OTA URL, and sends the command to the node.

---

## 2. What Information Is Available

### 2.1 Currently Tracked

| Field | Value | Source |
|-------|-------|--------|
| `canaryPreviousVersion` | `"0.2.19"` (version string) | `NodeProvider.GetNodeFirmwareVersion()` → `nodes.firmware_version` |

### 2.2 Available but NOT Tracked

The `FirmwareMeta` structure (server.go:22-29) contains additional metadata that is **NOT** stored for rollback:

```go
type FirmwareMeta struct {
    Filename   string    `json:"filename"`     // e.g., "spaxel-firmware-0.2.19.bin"
    Version    string    `json:"version"`      // e.g., "0.2.19"
    SHA256     string    `json:"sha256"`       // e.g., "e3b0c44298fc1c149af..."
    SizeBytes  int64     `json:"size_bytes"`
    IsLatest   bool      `json:"is_latest"`
    UploadedAt time.Time `json:"uploaded_at"`
}
```

**What's missing from rollback state:**
- SHA256 hash of the previous firmware image
- Exact filename/URL for the previous firmware
- Size bytes (useful for download progress tracking)
- Upload timestamp (useful for debugging)

---

## 3. Mothership Information Availability

### 3.1 At Rollback Time

**Question:** Does the mothership have sufficient information to perform a rollback?

**Answer:** **YES**, with limitations.

**How rollback works (step-by-step):**

1. **AutoUpdateManager** holds `canaryPreviousVersion = "0.2.19"`
2. **Rollback call:** `m.otaManager.SendOTAVersion(canaryMAC, "0.2.19")`
3. **Manager resolves version:**
   ```go
   meta := m.server.GetByVersion("0.2.19")  // or GetByFilename() as fallback
   ```
4. **Server returns `FirmwareMeta`:**
   - Filename: `"spaxel-firmware-0.2.19.bin"`
   - SHA256: `"e3b0c44298fc1c149af..."`
   - Version: `"0.2.19"`
5. **Manager constructs OTA URL:**
   ```go
   url := fmt.Sprintf("%s/firmware/%s", m.baseURL, meta.Filename)
   // e.g., "http://mothership:8080/firmware/spaxel-firmware-0.2.19.bin"
   ```
6. **Manager sends to node:**
   ```go
   sender.SendOTAToMAC(mac, url, meta.SHA256, meta.Version)
   ```

**Dependencies:**
- The previous firmware binary **must still exist** in `/firmware/` directory
- The binary must have been scanned by `Server.Scan()` (happens on startup and after uploads)
- The version string **must be discoverable** from the filename (via `versionRe` regex)

### 3.2 Failure Modes

| Scenario | Can Rollback? | Why? |
|----------|----------------|-------|
| Previous firmware deleted | **NO** | `GetByVersion()` returns `nil` → `ErrFirmwareNotFound` |
| Previous firmware never had semver in filename | **MAYBE** | `parseVersion()` falls back to filename without `.bin` → may resolve correctly if filename matches |
| Mothership restarted after canary deployed | **NO** | `canaryPreviousVersion` is in-memory only → lost on restart |
| Node provider unavailable at canary selection | **NO** | `canaryPreviousVersion` remains `""` → rollback skipped with warning |

---

## 4. Data Structure for Previous Version

### 4.1 Current Structure (Single String)

```go
canaryPreviousVersion string // e.g., "0.2.19"
```

**Pros:**
- Simple
- Works with existing `SendOTAVersion()` API

**Cons:**
- Lost on mothership restart
- Only captures version, not full metadata
- No recovery if lookup fails

### 4.2 Recommended Structure (Full Metadata)

For a more robust rollback mechanism, track the complete `FirmwareMeta`:

```go
// CanaryRollbackState holds all information needed to rollback a canary update
type CanaryRollbackState struct {
    MAC            string    // Canary node MAC address
    PreviousFirmware *FirmwareMeta // Complete metadata of version before update
    CapturedAt     time.Time // When this snapshot was taken
}
```

**Integration point:** Replace `canaryPreviousVersion string` with this struct in `AutoUpdateManager`.

**Benefits:**
- Rollback works even if `/firmware/` is rescanned (metadata cached in memory)
- Debugging includes filename, SHA256, and timestamp
- Can be persisted to SQLite if needed (for restart recovery)

---

## 5. Code Comment Recommendations

### 5.1 Existing Comment (Adequate)

**Location:** `autoupdate.go:52` (field declaration)

```go
canaryPreviousVersion string // Firmware version before canary update, for rollback
```

**Assessment:** Already clear and accurate. No change needed.

### 5.2 Additional Documentation (Recommended)

**Location:** `autoupdate.go:466-473` (capture point)

**Add before the capture code:**

```go
// CAPTURE: Store the canary node's current firmware version for potential rollback.
// This version string will be used if the canary update degrades detection quality
// beyond the configured threshold. The rollback uses SendOTAVersion(), which
// resolves the version to a FirmwareMeta via Server.GetByVersion(), then constructs
// the full OTA URL (baseURL + filename) and sends it to the node with SHA256 verification.
//
// Dependencies:
//   - The previous firmware binary must exist in /firmware/ (not deleted since upload)
//   - Server.Scan() must have indexed it (automatic on startup and after uploads)
//   - NodeProvider must be able to query the node's current version from nodes.firmware_version
//
// Failure mode: If canaryPreviousVersion is empty, rollback is skipped with a warning.
// This can happen if NodeProvider is nil or the node has no recorded firmware_version.
```

**Location:** `autoupdate.go:646-657` (rollback path)

**Add before the rollback block:**

```go
// ROLLBACK: Trigger OTA to revert canary node to its previous firmware version.
// SendOTAVersion() resolves the version string to a FirmwareMeta via GetByVersion(),
// then constructs the OTA URL and sends it with SHA256 verification. This only works
// if the previous firmware binary still exists in /firmware/ and has been indexed by
// Server.Scan(). If the version lookup fails, the update cycle fails and the canary
// node remains on the degraded firmware.
```

---

## 6. Persistence Considerations

### 6.1 Current State (In-Memory Only)

**Problem:** If the mothership restarts during or after a canary deployment, `canaryPreviousVersion` is lost.

**Impact:** 
- Cannot rollback a canary that was deployed before the restart
- Must either (a) proceed with fleet update blindly or (b) fail the entire update cycle

**Mitigation (not implemented):** Persist rollback state to SQLite:

```sql
CREATE TABLE IF NOT EXISTS ota_rollback_state (
    mac             TEXT PRIMARY KEY,  -- canary node MAC
    previous_version TEXT NOT NULL,    -- firmware version to rollback to
    previous_sha256  TEXT NOT NULL,    -- SHA256 for verification
    previous_filename TEXT NOT NULL,  -- for URL construction
    captured_at      INTEGER NOT NULL  -- when snapshot was taken
);
```

### 6.2 Restart Recovery Flow (Recommended)

If persistence is added:

1. **On startup:** Check for incomplete canary cycles in `ota_rollback_state`
2. **On reconnect:** If canary node comes back with degraded firmware, offer rollback option in dashboard
3. **On timeout:** If canary monitoring doesn't complete within X minutes, auto-rollback

---

## 7. Acceptance Criteria Checklist

- ✅ **Document where currentCanaryNode's pre-update version is (or should be) stored**
  - **Answer:** Stored in `AutoUpdateManager.canaryPreviousVersion` (in-memory field)
  - **Location:** `mothership/internal/ota/autoupdate.go:52`

- ✅ **Document the data structure for tracking previous version/sha256/url**
  - **Answer:** Currently tracks only `string` (version). Full metadata available in `FirmwareMeta` struct (includes filename, SHA256, URL) but not captured.
  - **Recommended:** Capture full `FirmwareMeta` in a `CanaryRollbackState` struct

- ✅ **Confirm the mothership already has this information available at rollback time**
  - **Answer:** **YES** — Mothership has all needed information:
    - `canaryPreviousVersion` (version string)
    - `Server.GetByVersion()` (resolves to `FirmwareMeta` with SHA256, filename)
    - `baseURL` (constructs full OTA URL)
  - **Caveat:** Requires previous firmware binary to still exist in `/firmware/` directory

- ✅ **Add code comment or doc note explaining the rollback version source**
  - **Answer:** Existing comment at line 52 is adequate. Additional detailed comments recommended in Section 5.2 above.

---

## 8. Conclusions

1. **Previous firmware version IS tracked** for canary rollback in `AutoUpdateManager.canaryPreviousVersion`
2. **Rollback IS functional** using the version string → `FirmwareMeta` lookup → OTA URL construction path
3. **Limitations exist:**
   - Only version string stored, not full metadata (SHA256, filename)
   - In-memory only — lost on mothership restart
   - Relies on previous firmware still existing in `/firmware/`
4. **Improvement path:** Capture full `FirmwareMeta` at canary selection time and optionally persist to SQLite for restart recovery

---

## References

- **AutoUpdateManager:** `mothership/internal/ota/autoupdate.go:36-63`
- **Canary capture:** `autoupdate.go:466-473`
- **Rollback execution:** `autoupdate.go:646-657`
- **Firmware metadata:** `mothership/internal/ota/server.go:22-29`
- **Version lookup:** `mothership/internal/ota/server.go:207-217`
- **OTA manager send:** `mothership/internal/ota/manager.go:135-192`
- **Database schema:** `mothership/internal/db/migrations.go:242-263`
