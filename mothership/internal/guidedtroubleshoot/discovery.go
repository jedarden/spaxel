// Package guidedtroubleshoot provides first-time feature discovery tooltips.
package guidedtroubleshoot

import (
	"sync"
	"time"
)

// DiscoveryTracker tracks which features have been discovered by the user.
// It provides first-run contextual help tooltips that are shown once per feature.
type DiscoveryTracker struct {
	mu       sync.RWMutex
	discovered map[string]discoveryState
}

type discoveryState struct {
	firstShownAt time.Time
	shownCount   int
}

// NewDiscoveryTracker creates a new discovery tracker.
func NewDiscoveryTracker() *DiscoveryTracker {
	return &DiscoveryTracker{
		discovered: make(map[string]discoveryState),
	}
}

// Feature definitions with their tooltips.
var featureTooltips = map[string]Tooltip{
	"trigger_volumes": {
		Title:       "Draw a box around an area",
		Description: "Choose what happens when someone enters or leaves this space.",
		Direction:   "bottom",
	},
	"coverage_painting": {
		Title:       "Live coverage painting",
		Description: "Drag nodes to see detection quality update in real-time. Green = excellent coverage.",
		Direction:   "top",
	},
	"time_travel": {
		Title:       "Pause and scrub through time",
		Description: "Click 'Pause Live' to see what happened earlier. Adjust parameters and see how detection would change.",
		Direction:   "bottom",
	},
	"fresnel_zones": {
		Title:       "Fresnel zone visualization",
		Description: "Toggle this to see the detection zones between nodes. Brighter zones = better sensitivity.",
		Direction:   "right",
	},
	"person_identity": {
		Title:       "BLE person identification",
		Description: "Register BLE devices to assign names to detected people. Go to Settings > People & Devices.",
		Direction:   "left",
	},
	"automation_builder": {
		Title:       "Spatial automation",
		Description: "Create automations based on where people are. Draw a zone and choose an action.",
		Direction:   "bottom",
	},
}

// Tooltip represents a first-time discovery tooltip.
type Tooltip struct {
	Title       string
	Description string
	Direction   string // "top", "bottom", "left", "right"
}

// ShouldShowTooltip returns true if the tooltip for this feature should be shown.
// A tooltip is shown if the feature hasn't been discovered yet.
func (t *DiscoveryTracker) ShouldShowTooltip(featureID string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	_, exists := t.discovered[featureID]
	return !exists
}

// MarkTooltipShown marks that a tooltip has been shown for a feature.
func (t *DiscoveryTracker) MarkTooltipShown(featureID string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if _, exists := t.discovered[featureID]; !exists {
		t.discovered[featureID] = discoveryState{
			firstShownAt: time.Now(),
			shownCount:   1,
		}
	} else {
		state := t.discovered[featureID]
		state.shownCount++
		t.discovered[featureID] = state
	}
}

// GetTooltip returns the tooltip content for a feature, if available.
func (t *DiscoveryTracker) GetTooltip(featureID string) (Tooltip, bool) {
	tooltip, exists := featureTooltips[featureID]
	return tooltip, exists
}

// GetAllFeatures returns all available feature IDs that have tooltips.
func GetAllFeatures() []string {
	features := make([]string, 0, len(featureTooltips))
	for featureID := range featureTooltips {
		features = append(features, featureID)
	}
	return features
}

// Reset clears all discovery state (used for testing).
func (t *DiscoveryTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.discovered = make(map[string]discoveryState)
}

// GetDiscoveredFeatures returns a list of features that have been discovered.
func (t *DiscoveryTracker) GetDiscoveredFeatures() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()

	features := make([]string, 0, len(t.discovered))
	for featureID := range t.discovered {
		features = append(features, featureID)
	}
	return features
}

// IsFeatureDiscovered returns true if the feature has been discovered (tooltip shown).
func (t *DiscoveryTracker) IsFeatureDiscovered(featureID string) bool {
	t.mu.RLock()
	defer t.mu.RUnlock()

	_, exists := t.discovered[featureID]
	return exists
}
