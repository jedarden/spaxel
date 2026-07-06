> ⚠️ **Secondary — folded into the consolidated inventory.** This was an input to
> `notes/bf-3ldj-findings.md`, which is itself **superseded** (see its banner). These four
> `new Blob()` sites are the **browser binary-`Blob` download API** (file payloads, not Spaxel
> domain blobs) and are catalogued as out-of-scope in `notes/bf-1q3m-consolidated.md` §4/§3.5
> — read that file for the authoritative inventory. Retained for provenance only.

---

# Blob Constructor Search Results (bf-5kns) (secondary — see banner above)

## Summary
Found **4 occurrences** of direct `Blob()` constructor calls across **3 files**.

## Locations

### 1. `/home/coding/spaxel/dashboard/static/js/fleet.js` - Line 457
**Context:** CSV export functionality
```javascript
const blob = new Blob([csvContent], { type: 'text/csv' });
```
- **Purpose:** Creates a blob for CSV file download containing fleet data
- **Type:** `text/csv`
- **Function:** `downloadCSV()`

### 2. `/home/coding/spaxel/dashboard/js/fleet-page.js` - Line 1034
**Context:** Configuration export functionality
```javascript
const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
```
- **Purpose:** Creates a blob for JSON configuration file download
- **Type:** `application/json`
- **Function:** `exportConfig()`

### 3. `/home/coding/spaxel/dashboard/js/fleet-page.js` - Line 1369
**Context:** CSV export functionality
```javascript
const blob = new Blob([csvContent], { type: 'text/csv' });
```
- **Purpose:** Creates a blob for CSV file download containing filtered fleet data
- **Type:** `text/csv`
- **Function:** `downloadCSV()`

### 4. `/home/coding/spaxel/dashboard/js/fleet.js` - Line 1997
**Context:** Configuration export functionality
```javascript
var blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
```
- **Purpose:** Creates a blob for JSON configuration file download
- **Type:** `application/json`
- **Function:** `exportConfig()`

## Patterns Observed

All Blob constructor calls follow the same pattern:
1. **Array-wrapped content:** `[data]` - always wrapped in an array
2. **MIME type specified:** All calls include explicit `{ type: '...' }` option
3. **Two content types:**
   - `text/csv` for fleet data exports
   - `application/json` for configuration exports

## Acceptance Criteria Met

- ✅ All 'new Blob()' calls identified (4 total)
- ✅ Each location documented with file path and line number
- ✅ Code context captured for each site

## Files Analyzed

**JavaScript files:**
- `/home/coding/spaxel/dashboard/static/js/fleet.js`
- `/home/coding/spaxel/dashboard/js/fleet-page.js`
- `/home/coding/spaxel/dashboard/js/fleet.js`

**No TypeScript files** with Blob constructors were found in the codebase.
