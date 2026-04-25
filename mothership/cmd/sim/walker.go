// Package main provides walker simulation utilities for the CSI simulator.
package main

import (
	"math"
)

// computeWalkerDeltaRMS computes the expected deltaRMS for a walker at a given position
// relative to a TX-RX link pair, using the Fresnel zone model.
// Zone 1: 0.15, zone 2: 0.15/4, zone 3: 0.15/9, etc. (inverse square decay).
func computeWalkerDeltaRMS(tx, rx, walker Point) float64 {
	d1 := distance(tx, walker)
	d2 := distance(walker, rx)
	dDirect := distance(tx, rx)

	excess := d1 + d2 - dDirect
	if excess < 0 {
		excess = 0
	}

	zoneNumber := int(math.Ceil(excess / halfWavelength))
	if zoneNumber < 1 {
		zoneNumber = 1
	}

	decay := 1.0 / math.Pow(float64(zoneNumber), 2.0)
	return 0.15 * decay
}
