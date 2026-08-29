# Empty-Room Baseline Capture - Blocker Verification (2026-08-29)

**Bead:** spaxel-65c76469 (split-child 2 of bf-3v39)
**Attempted:** 2026-08-29 ~13:24 UTC
**Status:** ❌ BLOCKED - Cannot proceed

## Blocker 1: API Authentication Required

The mothership API endpoints are protected by Google OAuth at the Traefik layer:

```bash
# All API calls return OAuth redirect
$ curl -s https://spaxel.ardenone.com/api/healthz
# Returns: Google OAuth authentication redirect

$ curl -s https://spaxel.ardenone.com/healthz
# Returns: 404 Not Found (route not exposed)
```

**Impact:** Cannot programmatically trigger baseline capture or query system status without authentication.

**Code Evidence:** From `mothership/cmd/mothership/main.go`:
- Line 775: `// Auth is handled at the Traefik layer (Google OAuth) — no in-app PIN auth.`
- Line 753: `r.Use(auth.DemoModeMiddleware(cfg.DemoMode))` - demo mode blocks mutating endpoints

## Blocker 2: Zero Nodes Connected (PRIMARY)

From existing notes `bf-3v39-baseline.md`:

```json
{
  "status": "ok",
  "uptime_s": 1223685,
  "version": "0.2.24",
  "nodes_online": 0,
  "db": "ok",
  "shedding_level": 0,
  "reason": "no nodes connected"
}
```

**Impact:**
- No nodes = no links = no CSI data = no baseline capture possible
- POST /api/baseline/capture would return `links_captured: 0`
- GET /api/baseline would return empty array

**Last node seen:** 2026-08-07 11:50:18 EDT (22 days ago)

## Dependency Chain Status

This task (child 2) depends on child 1 completion:

1. **Child 1:** spaxel-082135bc - "Node connectivity verification"
   - **Status:** Not complete
   - **Blocker:** Node offline since Aug 7

2. **Child 2:** spaxel-65c76469 - **THIS TASK** (empty-room baseline capture)
   - **Status:** BLOCKED
   - **Reasons:**
     1. API authentication prevents programmatic access
     2. No nodes connected (dependency on child 1)

3. **Child 3:** spaxel-b075c0f3 - "Run walk-through presence test"
   - **Status:** Blocked by child 1

4. **Child 4:** spaxel-2ce98275 - "Verify presence blob in dashboard"
   - **Status:** Blocked by child 3

## Required Actions to Unblock

### Option 1: Restore Node Connectivity (Recommended)

**Physical verification required:**

1. **Check ESP32-S3 node status:**
   - Is it powered on?
   - Connected to correct WiFi?
   - Can reach https://spaxel.ardenone.com?

2. **Verify node appears in system:**
   - Dashboard at https://spaxel.ardenone.com should show node
   - `/healthz` should show `nodes_online > 0`

### Option 2: Use Dashboard UI (Workaround)

Since the API requires OAuth, the baseline capture could potentially be triggered through:

1. **Dashboard UI** at https://spaxel.ardenone.com
   - Navigate to settings/calibration
   - Look for baseline capture button
   - This would bypass the programmatic API requirement

2. **Direct database access** (if available):
   - Access SQLite database directly
   - Query `baselines` table
   - This bypasses the API layer entirely

### Option 3: Skip API Requirements (Not Recommended)

Modify the task to accept:
- Dashboard UI screenshots instead of API responses
- Manual verification rather than programmatic polling
- Document the limitation that programmatic access requires OAuth tokens

## Code Reference: Baseline API

From `mothership/internal/api/baseline.go`:

**POST /api/baseline/capture** (lines 102-183):
```go
// captureBaseline handles POST /api/baseline/capture
// Starts a 60-second quiet-room baseline capture.
// The actual capture is handled by the baseline system in the signal processor;
// this endpoint initiates the capture process by resetting baselines and
// allowing them to re-accumulate during the quiet period.
```

**Response when no links exist:**
```go
if len(linksToCapture) == 0 {
    writeJSON(w, http.StatusOK, captureResponse{
        OK:            true,
        LinksCaptured: 0,
        Message:       "No links found to capture. Capture will start automatically once links are active.",
    })
    return
}
```

## Conclusion

**This task cannot be completed until:**

1. ✅ **Child 1 (node connectivity verification) is complete** - at least one node must be online
2. ✅ **API access is resolved** - either:
   - Through OAuth authentication
   - Through dashboard UI workaround
   - Through direct database access

**Recommendation:** Close this bead as "Blocked - Awaiting node connectivity" and retry when child 1 completes. The baseline capture requirements (empty room, 60s capture, EMA stabilization) cannot be tested without active nodes/links.

---

**Last Updated:** 2026-08-29 13:24 UTC
**Status:** BLOCKED - Cannot proceed without nodes + API access
**Next Action:** Awaiting child 1 completion (node connectivity)
