# Coverage Degradation Overlay Implementation

## Summary
Implemented before/after coverage overlay in dashboard for self-healing fleet events, as required by plan.md Component 12.

## Changes Made

### viz3d.js
Added comprehensive coverage degradation overlay functionality:

1. **showCoverageDegradation(data)** - Main function that:
   - Receives fleet change event data from WebSocket
   - Compares GDOP values before/after node loss
   - Creates ground plane cells colored by degradation level
   - Displays legend with before/after statistics
   - Highlights affected zones in red/amber/yellow

2. **dismissCoverageDegradation()** - Cleans up overlay when node recovers

3. **Supporting functions**:
   - `calculateGDOPDegradation(before, after)` - Categorizes degradation as severe/moderate/mild/none
   - `getDegradationColor(level, gdopAfter)` - Maps degradation levels to colors
   - `createDegradationLegend(data)` - Shows before/after stats with color-coded legend
   - `clearCoverageOverlay()` - Removes all overlay meshes and sprites

4. **State variables**:
   - `_coverageOverlayMeshes` - Stores degraded cell meshes
   - `_coverageOverlayVisible` - Toggle state
   - `_coverageOverlayData` - Current event data
   - `_coverageOverlayGroup` - THREE.Group for organization
   - `_degradationLegendSprites` - Legend sprite storage

5. **Public API exports**:
   - Added `showCoverageDegradation` and `dismissCoverageDegradation` to module exports

## Integration

The implementation integrates with existing infrastructure:
- WebSocket handler in app.js already calls `Viz3D.showCoverageDegradation()` on fleet_change events
- FleetChangeEvent in selfheal.go already includes all required GDOP before/after data
- Dashboard toast notifications and timeline events already working

## Degradation Levels

- **Severe** (red): >2x GDOP increase OR GDOP > 8 (poor coverage)
- **Moderate** (amber): >1.5x increase OR GDOP > 4 (fair coverage)
- **Mild** (yellow): >20% GDOP increase
- **None**: No significant change (not displayed)

## User Experience

When a node goes offline:
1. Toast notification shows degradation warning
2. Timeline event logged with before/after stats
3. 3D view shows red/amber/yellow cells highlighting degraded zones
4. Legend displays coverage percentage delta
5. Links lost/remaining estimated

When node recovers:
1. Toast notification shows recovery success
2. Overlay automatically dismissed
3. Full coverage restored visualization

## Files Modified

- `/home/coding/spaxel/dashboard/js/viz3d.js` - Added coverage degradation overlay (~250 lines)

## Testing

- Dashboard tests run successfully (283 passed)
- No syntax errors in JavaScript
- Functions properly exported to public API
