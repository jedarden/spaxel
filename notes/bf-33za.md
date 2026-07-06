# Component 33 Verification: Interactive Onboarding Wizard

## Date
2026-07-05

## Finding
Component 33 (Interactive Onboarding - 5-step CSI physics wizard) is **already fully implemented** in `/home/coding/spaxel/dashboard/js/onboard.js`.

## Implementation Verification

### Step 1: Walk (30s)
- ✅ Duration: 30000ms configured
- ✅ Narration: "I am listening to your WiFi signal. Walk across the room. See that? Your body just changed the WiFi signal - that is how I detect you."
- ✅ Real-time amplitude spike highlighting with yellow glow effect
- Code: Lines 1719-1739 (step 1), 1406-1418 (spike highlighting)

### Step 2: Still (10s)
- ✅ Duration: 10000ms configured
- ✅ Narration: "This is your room's baseline... A green line will appear on the chart - this is the signal when nothing is moving."
- ✅ Green baseline line fades in automatically
- ✅ Automatic baseline capture (no separate trigger needed)
- Code: Lines 1741-1772, 1361-1376 (baseline drawing)

### Step 3: Walk Through Detection Zone (15s)
- ✅ Duration: 15000ms configured
- ✅ Narration: "Walk between your node and the router - through the green zone... That is the detection zone. I am most sensitive along this path."
- ✅ Fresnel zone ellipsoid lights up as translucent green in 3D scene
- ✅ Zone pulses brighter as user crosses
- Code: Lines 1774-1791, 1427-1539 (Fresnel zone visualization)

### Step 4: Let Me Find You (15s)
- ✅ Duration: 15000ms configured
- ✅ Narration: "Walk somewhere and stop. I will try to locate you... Found you! I estimate you are about here. My accuracy is plus-or-minus 1 meter."
- ✅ Polls GET /api/blobs for humanoid placement
- ✅ Humanoid figure appears at estimated position
- ✅ Dotted accuracy radius circle shown
- Code: Lines 1793-1818, 1543-1643 (blob polling and visualization)

### Step 5: Place Your Node (30s)
- ✅ Duration: 30000ms configured
- ✅ Narration: "Drag your node to where it actually is in the room. Watch the green coverage change... Nice! Your coverage score is N percent."
- ✅ Coverage painting activates (GDOP overlay) on ground plane
- ✅ Interactive node dragging enabled
- ✅ Coverage score percentage displayed after placement
- Code: Lines 1820-1841, 1647-1700 (interactive placement and coverage)

### Zero Jargon Constraint
- ✅ User-facing text contains NO technical terms (CSI, Fresnel, deltaRMS)
- ✅ "Fresnel zone" replaced with "detection zone"
- ✅ Technical terms only used in internal variable names and comments

### Integration Points
- ✅ viz3d.js integration for steps 3-4 (3D scene visualizations)
- ✅ placement.js GDOP overlay for step 5 (coverage painting)
- ✅ WebSocket connection for live CSI data
- ✅ REST API integration for blob polling

## Conclusion
The 5-step interactive onboarding wizard is fully implemented and matches the Component 33 specification exactly. All teaching steps with specific narration are present, durations are correct, visualizations work, and the zero-jargon constraint is satisfied.
