# Empty-Room Baseline Capture - BF-3V39

## Task Context
- **Bead ID**: spaxel-65c76469 (split-child 2 of bf-3v39)
- **Parent Task**: BF-3V39 (baseline capture blockers verification)
- **Dependency**: Node connectivity verification (child 1) must be complete

## Physical Precondition ⚠️
**ROOM MUST BE CONFIRMED EMPTY BEFORE CAPTURE**
- No people in the room
- No movement during the 60-second capture window
- Clear line-of-sight between all spaxel nodes

## Technical Requirements
- Mothership URL: https://spaxel.ardenone.com
- API Endpoint: POST /api/baseline/capture
- Authentication: Google OAuth required
- Capture Duration: 60 seconds
- EMA Stabilization: tau=30s
- Expected deltaRMS: ~0.02 (empty-room noise floor)

## API Authentication Issue
**Status**: BLOCKED - Google OAuth Required

The mothership API is protected by Google OAuth authentication. Direct API calls require:
- Google OAuth token
- Session cookie authentication

```
$ curl -X POST https://spaxel.ardenone.com/api/baseline/capture
# Returns: Redirect to Google OAuth
```

## Alternative Approaches

### Option 1: Manual Dashboard Execution
1. Access https://spaxel.ardenonce.com/dashboard
2. Authenticate via Google OAuth
3. Use dashboard baseline capture feature
4. Monitor stabilization via dashboard metrics

### Option 2: Direct Database Access (if available)
- Access baselines.db directly
- Trigger baseline reset via ProcessorManager
- Monitor EMA convergence

### Option 3: Local Development Override
- Run mothership locally with demo mode
- Test baseline capture locally
- Verify deltaRMS behavior

## Execution Steps (when unblocked)

### Step 1: Room Preparation
- [ ] Operator confirms room is EMPTY
- [ ] All personnel leave the room
- [ ] Doors closed, no movement

### Step 2: Baseline Capture
```bash
# POST to start capture (requires auth)
curl -X POST https://spaxel.ardenone.com/api/baseline/capture \
  -H "Content-Type: application/json" \
  -d '{"links": []}' # Empty = all links
```

Expected Response:
```json
{
  "ok": true,
  "links_captured": N,
  "links": ["link1", "link2", ...],
  "message": "Baseline capture started. Keep the room clear for 60 seconds."
}
```

### Step 3: Monitor Baseline Recording
```bash
# Poll baseline status
curl -X GET https://spaxel.ardenone.com/api/baseline
```

Expected Response:
```json
[
  {
    "link_id": "node_mac:peer_mac",
    "snapshot_time_ms": 1234567890,
    "confidence": 0.95,
    "n_sub": 64
  }
]
```

### Step 4: EMA Stabilization Verification
- Wait for tau=30s EMA stabilization period
- Monitor deltaRMS convergence to ~0.02
- Verify each link/subcarrier settles

### Step 5: Document Results
- Record baseline ID
- Capture timestamp
- Document link/subcarrier coverage
- Record final deltaRMS values
- Save evidence to this file

## Expected Outcomes

### Success Criteria
- Baseline ID generated and recorded
- All active links show baseline entries
- EMA stabilization completes within expected time
- deltaRMS settles to ~0.02 ± 0.01
- Coverage verified per link/subcarrier

### Failure Modes
- **Authentication failure**: Cannot access API without OAuth token
- **No nodes online**: No links to capture baseline
- **Room not empty**: deltaRMS remains elevated (>0.05)
- **Insufficient stabilization**: EMA doesn't converge within expected time

## Investigation Results

### Environment Assessment (2026-08-29)

**Remote Mothership Status**: ✅ Online
- URL: https://spaxel.ardenone.com
- Health: OK (uptime: 14+ days, version: 0.2.24)
- Nodes Online: 0
- Authentication: Google OAuth Required

**Local Environment**: 
- Local databases found: `./data/baselines.db`, `./data/spaxel.db`
- No local mothership instance running
- Baseline database: Empty (no existing baselines)

**API Authentication Test**:
```bash
$ curl -X GET https://spaxel.ardenone.com/api/baseline
# Response: Redirect to Google OAuth
```

## Current Status
- **Date**: 2026-08-29
- **Status**: BLOCKED - Two Prerequisites Required
- **Blockers**:
  1. **Physical**: Room must be confirmed EMPTY by operator
  2. **Technical**: Google OAuth authentication required for API access

## Investigation Summary

The baseline capture task is blocked by two independent requirements:

### Blocker 1: Physical Precondition (Operator Action Required)
The task explicitly states: *"room must be confirmed EMPTY before capture — get explicit operator confirmation first."*

This requires:
- Visual confirmation that no people are in the room
- Agreement to keep the room clear for 60 seconds during capture
- Verification that all spaxel nodes are online and communicating

### Blocker 2: API Authentication (Technical Constraint)
The mothership API is protected by Google OAuth authentication:
- All API endpoints redirect to Google OAuth flow
- No programmatic authentication method available from agent context
- Would require operator to authenticate in browser and provide session token

**Attempted Workarounds**:
- ❌ Direct API calls (blocked by OAuth)
- ❌ Local mothership access (no instance running)
- ❌ Database direct access (insufficient without live CSI data)
- ✅ Alternative: Dashboard manual execution by operator

## Recommended Resolution Path

**Option A: Operator Executes via Dashboard (Recommended)**
1. Operator navigates to https://spaxel.ardenone.com/dashboard
2. Authenticates via Google OAuth
3. Confirms room is EMPTY
4. Uses dashboard baseline capture feature
5. Monitors stabilization in real-time
6. Documents results back to this bead

**Option B: Provide OAuth Credentials (If Available)**
- If operator can provide OAuth token/session cookie
- Agent can execute API calls programmatically
- Still requires physical precondition confirmation

**Option C: Schedule for Later Execution**
- Document current investigation results
- Close bead as "blocked on prerequisites"
- Reopen when operator is available for execution

## Next Steps
1. **Immediate**: Get operator confirmation that room is empty
2. **Resolve Authentication**: Obtain OAuth token OR use alternative approach
3. **Execute Capture**: Follow execution steps above
4. **Document Results**: Record all evidence and measurements

---

**Last Updated**: 2026-08-29 by agent (claude-code-glm-4.7-glm-mta:spaxel-65c76469)
