# Auto-Update Manager Version Selection Investigation

**Date:** 2026-08-08  
**Context:** Understanding why auto-update manager might pick older firmware (0.1.357) instead of newer (0.1.358)  

## How Version Selection Works

### 1. Server's Latest Determination (`internal/ota/server.go`)

The `Server` struct maintains the authoritative version list:

- **`GetLatest()` method** (lines 176-185):
  - Returns the firmware marked with `IsLatest = true`
  - Returns `nil` if no firmware is available

- **`Scan()` method** (lines 81-117) determines which firmware is "latest":
  - Scans the firmware directory for all `.bin` files
  - For each file, calls `computeMeta()` to extract version via regex: `\d+\.\d+\.\d+` (line 148)
  - **Only files containing semantic versions are considered** for latest (lines 103-108)
  - Uses `compareVersions()` function (lines 160-174) for proper numeric comparison
  - Compares major, minor, and patch components numerically (preventing "1.9.0" > "1.10.0" errors)
  - Sets `IsLatest = true` on the winner

- **`GetByVersion()` method** (lines 198-215):
  - Scans the entire `s.firmware` map to find a matching version string
  - Needed because the map is keyed by filename, not version

### 2. Auto-Update Manager's Usage (`internal/ota/autoupdate.go`)

The `AutoUpdateManager` checks for updates:

- **`checkForNewFirmware()`** (lines 249-292):
  - Runs every 60 seconds via ticker (line 236)
  - Calls `m.server.GetLatest()` **directly** (line 259) - NO intermediate caching
  - Compares result with `m.pendingFirmware` (line 274)
  - If filenames differ, updates `pendingFirmware` and proceeds

- **`pendingFirmware` field** (line 34):
  - **This is the ONLY caching in the auto-update manager**
  - Stores the `*FirmwareMeta` of the firmware being considered
  - Compared by `Filename` field (line 274)

### 3. No Timer-Based Snapshots

**Critical Finding:** The auto-update manager does **NOT** use timer-based snapshots that could become stale.

- Every check calls `GetLatest()` directly on the server
- The server's `GetLatest()` returns the current in-memory state
- The server's in-memory state is updated by `Scan()` operations

## Potential Stale State Issues

### Scenario 1: Filesystem Changes After Startup

**Issue:** If a new firmware file is added to the firmware directory after the server starts:

- **Detection:** The server will detect it on the next `Scan()` call
- **When Scan() is called:**
  - On server startup (`NewServer`, line 59)
  - After every firmware upload (`HandleUpload`, line 345)
  - If a requested file is not found during serving (`HandleServe`, line 266)

**Result:** Filesystem changes are detected within 1 minute (the auto-update check interval) OR immediately if a file is requested that isn't in the cache.

### Scenario 2: Pending Firmware Cache

**Issue:** The `pendingFirmware` field could potentially cause stale behavior:

**How it could stale:**
1. Auto-update checks and sets `pendingFirmware = version-A` (line 277)
2. A newer firmware `version-B` is uploaded
3. On next check (within 1 minute), `GetLatest()` returns `version-B`
4. Comparison at line 274 checks if `pendingFirmware.Filename == latest.Filename`
5. If filenames differ, it updates to the new firmware

**Protection:** The comparison uses `Filename`, which includes the version in semantic versioning (e.g., `spaxel-0.1.358.bin`). Since versions should have unique filenames, this protection works.

**Risk:** If someone overwrites a file with the same name but newer content, the version string in the filename wouldn't change, and the stale content might persist until rescan. However, this is unlikely with proper semantic versioning.

## Why 0.1.357 Might Be Picked Over 0.1.358

Based on the code analysis, here are the most likely explanations:

### 1. 0.1.358 File Missing or Incorrectly Named
- Check if `spaxel-0.1.358.bin` exists in the firmware directory
- Verify the filename exactly matches the pattern scanned by the version regex
- The regex looks for `\d+\.\d+\.\d+` in the filename (line 148)

### 2. 0.1.358 Not Detected During Scan
- If the file was added after server startup and no rescan has occurred
- Check if `Scan()` has been called since the file was added
- Look for log messages indicating scans have occurred

### 3. Version Parsing Issue
- If the filename contains "0.1.358" but in an unexpected format
- Check the exact filename - it must contain the version as a continuous substring matching `\d+\.\d+\.\d+`

### 4. Server Not Rescanned After Upload
- If firmware was uploaded via a mechanism that doesn't trigger `Scan()` (line 345 in `HandleUpload`)
- Check if the upload callback is properly wired: `s.SetUploadCallback()` (line 223)

## Recommended Investigation Steps

To diagnose why 0.1.357 is selected over 0.1.358:

1. **Verify file existence:**
   ```bash
   ls -la /firmware/spaxel-0.1.358.bin
   ```

2. **Check server's view:**
   - Call `GET /api/firmware` to see what the server lists
   - Verify which firmware has `"is_latest": true`

3. **Check server logs:**
   - Look for scan messages: `[INFO] ota: uploaded ...`
   - Verify scans are happening after uploads

4. **Trigger manual rescan:**
   - Restart the mothership (triggers scan on startup)
   - Upload any firmware to trigger scan (line 345)

5. **Verify version comparison:**
   - Ensure both filenames follow semantic versioning
   - Check that 0.1.358 > 0.1.357 in the `compareVersions()` logic

## Conclusion

The auto-update manager does **NOT** use stale timer-based snapshots. It calls `GetLatest()` directly every minute, which returns the server's current in-memory state. The server's state is updated by directory scans.

The most likely causes of picking 0.1.357 over 0.1.358 are:
1. The 0.1.358 file doesn't exist or isn't readable
2. The file wasn't detected during the last scan
3. The filename doesn't match the expected version pattern
4. A rescan hasn't occurred since the file was added
