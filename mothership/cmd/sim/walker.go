// Package main provides walker simulation utilities for the CSI simulator.
package main

import (
	"math"
)

// updateWalkerPosition updates a single walker's position using a random walk model.
// Walkers bounce off room walls with a margin and have Gaussian velocity perturbations.
func updateWalkerPosition(w *Walker, space *Space, rng interface {
	Float64() float64
}, dt float64) {
	w.Position.X += w.Velocity.X * dt
	w.Position.Y += w.Velocity.Y * dt

	margin := 0.2
	if w.Position.X < margin {
		w.Position.X = margin
		w.Velocity.X *= -1
	}
	if w.Position.X > space.Width-margin {
		w.Position.X = space.Width - margin
		w.Velocity.X *= -1
	}
	if w.Position.Y < margin {
		w.Position.Y = margin
		w.Velocity.Y *= -1
	}
	if w.Position.Y > space.Depth-margin {
		w.Position.Y = space.Depth - margin
		w.Velocity.Y *= -1
	}

	w.Position.Z = w.Height
}

// computeWalkerDeltaRMS computes the expected deltaRMS for a walker at a given position
// relative to a TX-RX link pair, using the Fresnel zone model.
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

	// Zone decay (inverse square) matched to the fusion engine
	decay := 1.0 / math.Pow(float64(zoneNumber), 2.0)

	// Base deltaRMS scaled by zone decay
	return 0.15 * decay
}

// walkerDistanceToNode computes Euclidean distance from a walker to a node
func walkerDistanceToNode(walker Point, node Point) float64 {
	return distance(walker, node)
}
