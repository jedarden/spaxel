// Package replay tests for the replay pipeline.
package replay

import (
	"sync"
	"testing"
)

// mockBroadcasterForPipeline implements BlobBroadcaster for testing.
type mockBroadcasterForPipeline struct {
	blobs     []BlobUpdate
	timestamp int64
	mu        sync.Mutex
}

func (m *mockBroadcasterForPipeline) BroadcastReplayBlobs(blobs []BlobUpdate, timestampMS int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blobs = blobs
	m.timestamp = timestampMS
}

// TestNewPipeline verifies pipeline creation.
func TestNewPipeline(t *testing.T) {
	params := &TunableParams{
		DeltaRMSThreshold: float64Ptr(0.02),
		TauS:               float64Ptr(30.0),
	}
	broadcaster := &mockBroadcasterForPipeline{}

	pipeline := NewPipeline(params, broadcaster)

	if pipeline == nil {
		t.Fatal("NewPipeline returned nil")
	}
	if pipeline.speed != 1.0 {
		t.Errorf("speed = %f, want 1.0", pipeline.speed)
	}
	if pipeline.blobIDCounter != 1 {
		t.Errorf("blobIDCounter = %d, want 1", pipeline.blobIDCounter)
	}
	if pipeline.blobStates == nil {
		t.Error("blobStates not initialized")
	}
}

// TestProcessFrame verifies frame processing produces blob updates.
func TestProcessFrame(t *testing.T) {
	params := &TunableParams{}
	broadcaster := &mockBroadcasterForPipeline{}

	pipeline := NewPipeline(params, broadcaster)

	// Create a test CSI frame (24-byte header + 128 subcarriers * 2 bytes)
	frame := make([]byte, 24+128*2)
	frame[0] = 0xAA // node MAC byte 0
	frame[6] = 0xBB // peer MAC byte 0
	frame[20] = 206 // RSSI: -50 as unsigned byte (two's complement)
	frame[22] = 6   // channel
	frame[23] = 64  // nSub

	timestampNS := int64(1234567890 * 1_000_000)

	pipeline.ProcessFrame(frame, timestampNS)

	// Verify broadcast was called
	broadcaster.mu.Lock()
	defer broadcaster.mu.Unlock()

	if broadcaster.timestamp != timestampNS/1_000_000 {
		t.Errorf("timestamp = %d, want %d", broadcaster.timestamp, timestampNS/1_000_000)
	}

	// Should have at least one blob (demo blob)
	if len(broadcaster.blobs) == 0 {
		t.Error("No blobs produced")
	}
}

// TestProcessFrameWithShortFrame verifies handling of header-only frame.
func TestProcessFrameWithShortFrame(t *testing.T) {
	params := &TunableParams{}
	broadcaster := &mockBroadcasterForPipeline{}

	pipeline := NewPipeline(params, broadcaster)

	// Header-only frame (n_sub = 0)
	frame := make([]byte, 24)

	pipeline.ProcessFrame(frame, 1234567890)

	// Should not crash, may or may not produce blobs
}

// TestPipelineSetSpeed verifies speed changes on a Pipeline.
func TestPipelineSetSpeed(t *testing.T) {
	params := &TunableParams{}
	broadcaster := &mockBroadcasterForPipeline{}

	pipeline := NewPipeline(params, broadcaster)

	pipeline.SetSpeed(2.5)

	if pipeline.speed != 2.5 {
		t.Errorf("speed = %f, want 2.5", pipeline.speed)
	}
}

// TestStop verifies pipeline stopping.
func TestStop(t *testing.T) {
	params := &TunableParams{}
	broadcaster := &mockBroadcasterForPipeline{}

	pipeline := NewPipeline(params, broadcaster)

	// Stop the pipeline
	pipeline.Stop()

	// Try to process a frame after stop - should not crash
	frame := make([]byte, 152)
	pipeline.ProcessFrame(frame, 1234567890)
}

// TestTrailUpdate verifies trail accumulation for blobs.
func TestTrailUpdate(t *testing.T) {
	params := &TunableParams{}
	broadcaster := &mockBroadcasterForPipeline{}

	pipeline := NewPipeline(params, broadcaster)

	// Process multiple frames to build trail
	frame := make([]byte, 152)
	for i := 0; i < 70; i++ { // More than max trail length
		pipeline.ProcessFrame(frame, 1234567890+int64(i)*50_000_000)
	}

	broadcaster.mu.Lock()
	defer broadcaster.mu.Unlock()

	if len(broadcaster.blobs) == 0 {
		t.Fatal("No blobs produced")
	}

	// Check that trail is bounded
	blob := broadcaster.blobs[0]
	if len(blob.Trail) > 60 {
		t.Errorf("Trail length = %d, want <= 60", len(blob.Trail))
	}
}

// TestGetTrail verifies trail retrieval and update.
func TestGetTrail(t *testing.T) {
	params := &TunableParams{}
	broadcaster := &mockBroadcasterForPipeline{}

	pipeline := NewPipeline(params, broadcaster)

	// Get trail for non-existent blob
	trail := pipeline.getTrail(1, 1.0, 2.0)
	if len(trail) != 2 {
		t.Errorf("Initial trail length = %d, want 2 (x,z)", len(trail))
	}

	// Update same blob
	trail = pipeline.getTrail(1, 1.5, 2.5)
	if len(trail) != 4 {
		t.Errorf("Updated trail length = %d, want 4 (x,z,x,z)", len(trail))
	}

	// Verify values
	if trail[2] != 1.5 || trail[3] != 2.5 {
		t.Errorf("Trail values incorrect: got %v", trail)
	}
}

// TestFloat64Helpers verifies math helper functions.
func TestFloat64Helpers(t *testing.T) {
	// Test float64Sin at key points
	tests := []struct {
		x    float64
		want float64
	}{
		{0, 0},
		{3.14159265359, 0}, // sin(π) ≈ 0
		{1.57079632679, 1}, // sin(π/2) ≈ 1
	}

	for _, tt := range tests {
		got := float64Sin(tt.x)
		if got != tt.want {
			// Allow some tolerance for approximation
			if abs(got-tt.want) > 0.1 {
				t.Errorf("float64Sin(%f) = %f, want %f", tt.x, got, tt.want)
			}
		}
	}

	// Test float64Cos
	cosTests := []struct {
		x    float64
		want float64
	}{
		{0, 1},           // cos(0) = 1
		{3.14159265359, -1}, // cos(π) ≈ -1
	}

	for _, tt := range cosTests {
		got := float64Cos(tt.x)
		if got != tt.want {
			if abs(got-tt.want) > 0.1 {
				t.Errorf("float64Cos(%f) = %f, want %f", tt.x, got, tt.want)
			}
		}
	}
}

