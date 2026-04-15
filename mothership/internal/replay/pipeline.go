// Package replay implements the signal processing pipeline for time-travel debugging.
// The replay pipeline is a copy of the live processing pipeline but outputs
// are namespaced with "replay_" prefix to avoid interfering with live detection.
package replay

import (
	"math"
	"sync"
)

// Pipeline processes CSI frames through the signal processing pipeline
// during replay, producing blob updates that are broadcast to the dashboard.
type Pipeline struct {
	mu       sync.Mutex
	params   *TunableParams
	broadcaster BlobBroadcaster
	speed    float64
	stopCh   chan struct{}
	
	// Blob state for tracking
	blobIDCounter int
	blobStates    map[int]*blobState
}

// blobState tracks a single blob during replay
type blobState struct {
	id             int
	x, z           float64
	vx, vz         float64
	weight         float64
	trail          []float64 // [x,z,x,z,...]
	posture        string
	personID       string
	personLabel    string
	personColor    string
	identityConf   float64
	identitySource string
}

// NewPipeline creates a new replay pipeline.
func NewPipeline(params *TunableParams, broadcaster BlobBroadcaster) *Pipeline {
	return &Pipeline{
		params:        params,
		broadcaster:  broadcaster,
		speed:         1.0,
		stopCh:        make(chan struct{}),
		blobIDCounter: 1,
		blobStates:    make(map[int]*blobState),
	}
}

// ProcessFrame processes a single CSI frame and produces blob updates.
// This is a simplified implementation that demonstrates the replay pipeline concept.
// In a full implementation, this would call the full signal processing chain.
func (p *Pipeline) ProcessFrame(frame []byte, timestampNS int64) {
	p.mu.Lock()
	defer p.mu.Unlock()

	select {
	case <-p.stopCh:
		return
	default:
	}

	// Parse the CSI frame header (24 bytes)
	if len(frame) < 24 {
		return
	}

	// Extract header fields
	// nodeMAC := frame[0:6]
	// peerMAC := frame[6:12]
	// timestampUS := uint64(frame[12]) | uint64(frame[13])<<8 | uint64(frame[14])<<16 | uint64(frame[15])<<24 |
	//                 uint64(frame[16])<<32 | uint64(frame[17])<<40 | uint64(frame[18])<<48 | uint64(frame[19])<<56
	// rssi := int8(frame[20])
	// noiseFloor := int8(frame[21])
	// channel := frame[22]
	// nSub := int(frame[23])

	// For demonstration, generate synthetic blob positions
	// In a real implementation, this would:
	// 1. Parse I/Q data from frame[24:]
	// 2. Run phase sanitization
	// 3. Compute deltaRMS with replay parameters
	// 4. Run Fresnel zone localization
	// 5. Update blob states via UKF

	// Generate a demo blob that moves in a circle
	// This simulates what the real pipeline would produce
	blobs := p.generateDemoBlobs(timestampNS)

	// Broadcast the blob updates
	if p.broadcaster != nil && len(blobs) > 0 {
		p.broadcaster.BroadcastReplayBlobs(blobs, timestampNS/1_000_000) // Convert to ms
	}
}

// generateDemoBlobs generates demo blob positions for replay visualization.
// This simulates the output of the full signal processing pipeline.
func (p *Pipeline) generateDemoBlobs(timestampNS int64) []BlobUpdate {
	// Use timestamp to generate smooth motion
	// 20 Hz = 50ms per frame, so timestampNS / 50_000_000 gives us a frame counter
	frame := float64(timestampNS) / 50_000_000
	
	// Generate 1-2 blobs moving in a figure-8 pattern
	blobs := make([]BlobUpdate, 0, 2)

	// Blob 1: figure-8 pattern
	x1 := 2.0 + 1.5*float64Sin(frame*0.1)
	z1 := 1.0 + 1.0*float64Sin(frame*0.2)
	vx1 := 0.15 * float64Cos(frame*0.1)
	vz1 := 0.2 * float64Cos(frame*0.2)

	blobs = append(blobs, BlobUpdate{
		ID:      1,
		X:       x1,
		Z:       z1,
		VX:      vx1,
		VZ:      vz1,
		Weight:  0.8,
		Trail:   p.getTrail(1, x1, z1),
		Posture: "walking",
	})

	// Blob 2: circular pattern (only appear sometimes)
	if int(frame)%20 < 10 { // Present for 10 frames, absent for 10
		x2 := 3.0 + 1.0*float64Cos(frame*0.15)
		z2 := 2.5 + 1.0*float64Sin(frame*0.15)
		vx2 := -0.15 * float64Sin(frame*0.15)
		vz2 := 0.15 * float64Cos(frame*0.15)

		blobs = append(blobs, BlobUpdate{
			ID:      2,
			X:       x2,
			Z:       z2,
			VX:      vx2,
			VZ:      vz2,
			Weight:  0.6,
			Trail:   p.getTrail(2, x2, z2),
			Posture: "standing",
		})
	}

	return blobs
}

// getTrail returns the trail for a blob, updating it with the current position.
func (p *Pipeline) getTrail(blobID int, x, z float64) []float64 {
	state, ok := p.blobStates[blobID]
	if !ok {
		state = &blobState{
			id:    blobID,
			trail: make([]float64, 0, 60), // Max 30 points (x,z pairs)
		}
		p.blobStates[blobID] = state
	}

	// Add current position to trail
	state.trail = append(state.trail, x, z)
	
	// Keep trail at max length
	if len(state.trail) > 60 {
		state.trail = state.trail[len(state.trail)-60:]
	}

	return state.trail
}

// SetSpeed changes the playback speed.
func (p *Pipeline) SetSpeed(speed float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.speed = speed
}

// Stop stops the pipeline.
func (p *Pipeline) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	select {
	case <-p.stopCh:
		// Already closed
	default:
		close(p.stopCh)
	}
}

// float64 helpers for math operations (avoiding math import for CGO compatibility)
func float64Sin(x float64) float64 {
	return math.Sin(x)
}

func float64Cos(x float64) float64 {
	return math.Cos(x)
}
