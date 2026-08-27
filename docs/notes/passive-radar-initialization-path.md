# Passive-Radar Service Initialization Path

**Date:** 2026-08-27  
**Purpose:** Document where and how the passive-radar service is instantiated and initialized

## Service Constructor

**Location:** `mothership/internal/apdetector/detector.go`  
**Function:** `NewDetector(db *sql.DB) *Detector` (line 52)

```go
// NewDetector creates a new AP detector
func NewDetector(db *sql.DB) *Detector {
    return &Detector{
        db:          db,
        reports:     make(map[string][]BSSIDReport),
        subscribers: make([]chan APInfo, 0),
    }
}
```

The `Detector` struct manages:
- AP BSSID detection and consensus across nodes
- Virtual router node creation in the database
- AP change detection and alerting
- Subscriber notification for AP updates

## Constructor Call Sites

**Only ONE call site exists in the codebase:**

**File:** `mothership/cmd/mothership/main.go`  
**Lines:** 4814-4815

```go
apDet := apdetector.NewDetector(mainDB)
ingestSrv.SetAPDetector(apDet)
log.Printf("[INFO] Passive-radar AP auto-detection enabled")
```

## Current Initialization Path

### Phase 1: Mothership Startup (main.go)

1. **Database initialization** (earlier in main.go):
   - `mainDB` is opened via `db.OpenSQLite()`

2. **Ingestion server creation** (line 771):
   ```go
   ingestSrv := ingestion.NewServer()
   ```

3. **AP detector creation and injection** (lines 4814-4816):
   ```go
   apDet := apdetector.NewDetector(mainDB)
   ingestSrv.SetAPDetector(apDet)
   ```

### Phase 2: Node Connection (ingestion/server.go)

When a node connects and sends a `hello` message:

1. **Hello processing** (lines 620-639):
   - Node connection is registered
   - AP detector is retrieved from server state (line 628):
     ```go
     apDet := s.apDetector
     ```
   - If detector is non-nil, process the hello's AP BSSID (lines 635-639):
     ```go
     if apDet != nil {
         if err := apDet.ProcessHello(hello.MAC, hello.APBSSID, hello.APChannel); err != nil {
             log.Printf("[WARN] AP detector process hello failed: %v", err)
         }
     }
     ```

2. **Inside ProcessHello** (apdetector/detector.go lines 61-110):
   - Extracts AP BSSID and channel from hello message
   - Aggregates reports from all nodes
   - Detects dominant AP via consensus (80% agreement threshold)
   - Creates/updates virtual router node in database
   - Notifies subscribers of AP changes

## Service Structure

### Ingestion Server Field

**File:** `mothership/internal/ingestion/server.go`  
**Line:** 138

```go
type Server struct {
    // ... other fields ...
    apDetector *apdetector.Detector
    // ... other fields ...
}
```

### Setter Method

**File:** `mothership/internal/ingestion/server.go`  
**Lines:** 336-339

```go
// SetAPDetector sets the AP detector for passive radar auto-detection.
func (s *Server) SetAPDetector(detector *apdetector.Detector) {
    s.apDetector = detector
}
```

## How It Works: End-to-End Flow

```
Mothership starts
    │
    ├─► Open SQLite database (mainDB)
    │
    ├─► Create ingestion server (ingestion.NewServer)
    │
    └─► Create AP detector (apdetector.NewDetector(mainDB)
         with:
         - DB handle for virtual node persistence
         - Report map for BSSID aggregation
         - Subscriber channels for AP updates
         │
         └─► Inject into ingestion server
              (ingestSrv.SetAPDetector(apDet))
              
Node connects via WebSocket
    │
    ├─► Send hello message with:
    │   - MAC address
    │   - ap_bssid (router BSSID)
    │   - ap_channel (WiFi channel)
    │
    └─► Ingestion server processes hello
         │
         └─► apDetector.ProcessHello(mac, apBSSID, apChannel)
              │
              ├─► Aggregate BSSID reports from all nodes
              │
              ├─► Detect dominant AP (≥80% agreement)
              │
              ├─► Create/update virtual router node in DB
              │   - MAC = AP BSSID
              │   - Name = "{Manufacturer} Router"
              │   - Role = 'ap'
              │   - Virtual = 1
              │   - node_type = 'ap'
              │
              └─► Emit AP change event if BSSID changed
```

## Key Implementation Details

### 1. BSSID Consensus Algorithm

**Location:** `apdetector/detector.go` lines 169-230

- Collects BSSID reports from all connected nodes
- Requires ≥80% of nodes to agree on the same BSSID
- Handles mesh networks with multiple BSSIDs
- Falls back to no AP if consensus not reached

### 2. Virtual Node Creation

**Location:** `apdetector/detector.go` lines 233-257

- Uses `INSERT ... ON CONFLICT(mac) DO UPDATE SET`
- Creates node with:
  - `virtual = 1` (marks as virtual, not physical hardware)
  - `node_type = 'ap'` (distinguishes from ESP32 nodes)
  - `ap_bssid` and `ap_channel` columns populated
  - Position defaults to (0, 0, 2.5) meters

### 3. OUI Lookup for Manufacturer Name

**Location:** `apdetector/detector.go` lines 215-220

- Extracts first 3 bytes of BSSID (OUI)
- Looks up manufacturer name via `oui.LookupOUI()`
- Defaults to "Unknown Router" if lookup fails

### 4. AP Change Detection

**Location:** `apdetector/detector.go` lines 86-106

- Detects when dominant AP changes
- Logs change event to events table
- Creates new virtual node for new AP
- Notifies all subscribers

## Historical Note (ADR-003)

According to the comment in `main.go` lines 4807-4813:

> "This was fully implemented but never constructed: SetAPDetector existed and ProcessHello was called behind a nil guard, so s.apDetector was permanently nil and ambient sensing could never start."

This defect was tracked as:
- `bf-41h7g`: AP detector never constructed
- `bf-4p0ne`: Parent bead for ambient sensing activation

The fix was simple: call the constructor and inject the detector (lines 4814-4815), which is now present in the codebase.

## Dependencies

The passive-radar service requires:
1. **SQLite database** (`mainDB`) - for persisting virtual nodes and AP change events
2. **OUI lookup table** (`internal/oui`) - for manufacturer name resolution
3. **Node hello messages** - containing `ap_bssid` and `ap_channel` fields

## Testing Notes

To verify passive-radar initialization:
1. Start mothership and check logs for: `[INFO] Passive-radar AP auto-detection enabled`
2. Connect a node that reports an AP BSSID in its hello
3. Check database for virtual node entry:
   ```sql
   SELECT * FROM nodes WHERE virtual = 1 AND node_type = 'ap';
   ```
4. Verify events table for AP detection events:
   ```sql
   SELECT * FROM events WHERE type = 'ap_changed';
   ```
