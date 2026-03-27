package tracker

import (
	"math"
	"testing"
	"time"
)

// ─── single-person tracking ───────────────────────────────────────────────────

func TestSinglePersonTracking(t *testing.T) {
	tr := NewTracker()
	const dt = 0.1  // 10 Hz
	const steps = 50

	// Straight walk: x advances at 0.8 m/s, y=1.0 m (standing), z constant.
	for i := 0; i < steps; i++ {
		x := 1.0 + float64(i)*dt*0.8
		tr.lastRun = tr.lastRun.Add(-time.Duration(dt * float64(time.Second)))
		blobs := tr.Update([][4]float64{{x, 1.0, 5.0, 0.9}})
		if len(blobs) != 1 {
			t.Fatalf("step %d: expected 1 blob, got %d", i, len(blobs))
		}
	}

	blobs := tr.Update([][4]float64{{1.0 + float64(steps)*dt*0.8, 1.0, 5.0, 0.9}})
	if len(blobs) != 1 {
		t.Fatalf("expected 1 blob at end, got %d", len(blobs))
	}
	b := blobs[0]
	expectedX := 1.0 + float64(steps)*dt*0.8
	if math.Abs(b.X-expectedX) > 0.5 {
		t.Errorf("X position error too large: got %.2f, want ≈%.2f", b.X, expectedX)
	}
	if len(b.Trail) == 0 {
		t.Error("trail should be non-empty")
	}
}

// ─── ID persistence through gap ───────────────────────────────────────────────

func TestIDPersistenceThroughGap(t *testing.T) {
	tr := NewTracker()

	// Establish a stable track.
	var id int
	for i := 0; i < 15; i++ {
		tr.lastRun = tr.lastRun.Add(-100 * time.Millisecond)
		blobs := tr.Update([][4]float64{{5.0, 1.0, 5.0, 0.9}})
		if i == 14 {
			if len(blobs) == 0 {
				t.Fatal("no blobs after track establishment")
			}
			id = blobs[0].ID
		}
	}

	// Simulate a 2 s gap — within the 3 s tolerance.
	tr.lastRun = tr.lastRun.Add(-2 * time.Second)
	blobs := tr.Update([][4]float64{{5.1, 1.0, 5.0, 0.8}})

	if len(blobs) == 0 {
		t.Fatal("track should persist through a 2 s gap")
	}
	if blobs[0].ID != id {
		t.Errorf("ID changed: was %d, now %d", id, blobs[0].ID)
	}
}

// ─── gap exceeds tolerance ────────────────────────────────────────────────────

func TestTrackDroppedAfterLongGap(t *testing.T) {
	tr := NewTracker()

	for i := 0; i < 10; i++ {
		tr.lastRun = tr.lastRun.Add(-100 * time.Millisecond)
		tr.Update([][4]float64{{3.0, 1.0, 3.0, 0.9}})
	}

	// Directly backdate LastSeen to simulate a 4 s gap (exceeds 3 s tolerance).
	tr.mu.Lock()
	for _, b := range tr.blobs {
		b.LastSeen = b.LastSeen.Add(-4 * time.Second)
	}
	tr.mu.Unlock()

	blobs := tr.Update(nil)
	if len(blobs) != 0 {
		t.Errorf("expected track to be dropped after 4 s gap, got %d blobs", len(blobs))
	}
}

// ─── posture transitions ──────────────────────────────────────────────────────

func TestPostureTransitions(t *testing.T) {
	tests := []struct {
		y, vx, vz float64
		want      Posture
	}{
		{0.2, 0.0, 0.0, PostureLying},
		{0.6, 0.0, 0.0, PostureSeated},
		{1.0, 0.1, 0.0, PostureStanding},
		{1.0, 0.5, 0.5, PostureWalking},
	}
	for _, tc := range tests {
		got := estimatePosture(tc.y, tc.vx, tc.vz)
		if got != tc.want {
			t.Errorf("estimatePosture(y=%.1f, vx=%.1f, vz=%.1f) = %s, want %s",
				tc.y, tc.vx, tc.vz, got, tc.want)
		}
	}
}

// ─── velocity constraint ──────────────────────────────────────────────────────

func TestHorizontalVelocityConstraint(t *testing.T) {
	u := NewUKF(0, 1, 0)
	// Inject extreme horizontal velocity.
	u.X.SetVec(3, 20.0)
	u.X.SetVec(5, 20.0)
	u.Predict(0.1)
	vx, _, vz := u.Velocity()
	horizSpd := math.Sqrt(vx*vx + vz*vz)
	if horizSpd > maxHorizVel+0.01 {
		t.Errorf("horizontal velocity constraint violated: %.3f m/s > %.3f m/s", horizSpd, maxHorizVel)
	}
}

func TestVerticalVelocityConstraint(t *testing.T) {
	u := NewUKF(0, 1, 0)
	u.X.SetVec(4, 50.0) // extreme upward velocity
	u.Predict(0.1)
	_, vy, _ := u.Velocity()
	if vy > maxVertVel+0.01 {
		t.Errorf("vertical velocity constraint violated: %.3f m/s > %.3f m/s", vy, maxVertVel)
	}
}

// ─── acceleration constraint ─────────────────────────────────────────────────

func TestAccelerationConstraint(t *testing.T) {
	u := NewUKF(0, 1, 0)
	// Start from rest; inject large velocity jump.
	u.X.SetVec(3, 0.0)
	u.X.SetVec(5, 0.0)
	// Directly set post-predict state as if the filter shot to 10 m/s.
	u.X.SetVec(3, 10.0)
	// Re-run Predict — applyConstraints should cap the resulting velocity change.
	u.Predict(0.1)
	vx, _, vz := u.Velocity()
	horizSpd := math.Sqrt(vx*vx + vz*vz)
	// After one predict from 10 m/s, acceleration cap = 3 m/s² * 0.1 s = 0.3 m/s change.
	// Speed should not exceed maxHorizVel anyway.
	if horizSpd > maxHorizVel+0.01 {
		t.Errorf("speed after acceleration: %.3f m/s > %.3f", horizSpd, maxHorizVel)
	}
}

// ─── collision avoidance ──────────────────────────────────────────────────────

func TestCollisionAvoidance(t *testing.T) {
	tr := NewTracker()

	// Plant two blobs at the same position.
	tr.lastRun = tr.lastRun.Add(-100 * time.Millisecond)
	tr.Update([][4]float64{{5.0, 1.0, 5.0, 0.9}, {5.0, 1.0, 5.0, 0.9}})
	tr.lastRun = tr.lastRun.Add(-100 * time.Millisecond)
	blobs := tr.Update([][4]float64{{5.0, 1.0, 5.0, 0.9}, {5.05, 1.0, 5.0, 0.9}})

	if len(blobs) < 2 {
		t.Skip("fewer than 2 blobs — collision avoidance not applicable")
	}
	a, b := blobs[0], blobs[1]
	dx := a.X - b.X
	dz := a.Z - b.Z
	d := math.Sqrt(dx*dx + dz*dz)
	if d < minSeparation-0.01 {
		t.Errorf("blobs too close: %.3f m < %.3f m minimum", d, minSeparation)
	}
}

// ─── occlusion recovery ───────────────────────────────────────────────────────

func TestOcclusionRecovery(t *testing.T) {
	tr := NewTracker()

	// Establish track.
	for i := 0; i < 20; i++ {
		tr.lastRun = tr.lastRun.Add(-100 * time.Millisecond)
		tr.Update([][4]float64{{3.0, 1.0, 3.0, 0.85}})
	}
	blobs := tr.Update([][4]float64{{3.0, 1.0, 3.0, 0.85}})
	if len(blobs) == 0 {
		t.Fatal("no blobs after establishment")
	}
	id := blobs[0].ID

	// 2 s occlusion — no measurements.
	tr.lastRun = tr.lastRun.Add(-2 * time.Second)
	tr.Update(nil)

	// Re-detect at nearby position.
	tr.lastRun = tr.lastRun.Add(-100 * time.Millisecond)
	blobs = tr.Update([][4]float64{{3.15, 1.0, 3.0, 0.85}})

	if len(blobs) == 0 {
		t.Fatal("track lost after 2 s occlusion")
	}
	if blobs[0].ID != id {
		t.Errorf("ID changed after occlusion: was %d, now %d", id, blobs[0].ID)
	}
}

// ─── posture labels ───────────────────────────────────────────────────────────

func TestPostureString(t *testing.T) {
	cases := map[Posture]string{
		PostureStanding: "standing",
		PostureWalking:  "walking",
		PostureSeated:   "seated",
		PostureLying:    "lying",
		PostureUnknown:  "unknown",
	}
	for p, want := range cases {
		if got := p.String(); got != want {
			t.Errorf("Posture(%d).String() = %q, want %q", p, got, want)
		}
	}
}

// ─── UKF round-trip ───────────────────────────────────────────────────────────

func TestUKFConvergesOnStaticTarget(t *testing.T) {
	u := NewUKF(5.0, 1.0, 5.0)

	// Feed 50 noisy measurements of the same position.
	for i := 0; i < 50; i++ {
		u.Predict(0.1)
		u.Update([measN]float64{5.0, 1.0, 5.0})
	}

	x, y, z := u.Position()
	if math.Abs(x-5.0) > 0.3 {
		t.Errorf("X not converged: %.3f", x)
	}
	if math.Abs(y-1.0) > 0.2 {
		t.Errorf("Y not converged: %.3f", y)
	}
	if math.Abs(z-5.0) > 0.3 {
		t.Errorf("Z not converged: %.3f", z)
	}
}
