package tracker

import (
	"math"
	"sync"
	"time"
)

// Posture is the estimated body posture of a tracked person.
type Posture int

const (
	PostureUnknown  Posture = iota
	PostureStanding         // upright, slow or stationary
	PostureWalking          // upright, moving
	PostureSeated           // centroid ~0.4–0.8 m above floor
	PostureLying            // centroid below ~0.4 m (on floor)
)

func (p Posture) String() string {
	switch p {
	case PostureStanding:
		return "standing"
	case PostureWalking:
		return "walking"
	case PostureSeated:
		return "seated"
	case PostureLying:
		return "lying"
	default:
		return "unknown"
	}
}

// Blob is a tracked entity with a persistent numeric identity.
type Blob struct {
	ID      int
	X, Y, Z float64      // world-space position, metres
	VX, VY, VZ float64   // velocity, m/s
	Weight  float64      // detection confidence [0..1]
	Posture Posture
	LastSeen time.Time
	// Trail holds the last TrailMaxLen positions (newest last).
	Trail [][3]float64

	// Identity fields (populated by BLE-to-blob matching)
	PersonID           string    `json:"person_id,omitempty"`            // UUID from BLE registry
	PersonLabel        string    `json:"person_label,omitempty"`         // Display name
	PersonColor        string    `json:"person_color,omitempty"`         // Hex color for dashboard
	IdentityConfidence float64   `json:"identity_confidence,omitempty"`  // Match confidence [0..1]
	IdentitySource     string    `json:"identity_source,omitempty"`      // "ble_triangulation", "ble_only", or ""
	IdentityLastSeen   time.Time `json:"-"`                              // Last time identity was confirmed

	ukf *UKF // internal — nil in copies returned to callers
}

// TrailMaxLen is the maximum number of trail points kept per blob.
const TrailMaxLen = 60

const (
	maxAssocDist  = 2.0           // m  — measurement-to-track gate radius
	gapTolerance  = 3 * time.Second // persistence through occlusion
	minSeparation = 0.4           // m  — collision avoidance floor
	walkThreshold = 0.3           // m/s horizontal speed → walking posture
)

// Posture height thresholds (Y = blob centroid height above floor, metres).
const (
	lyingMaxY  = 0.4 // below → lying
	seatedMaxY = 0.8 // below → seated
)

// Tracker manages a set of active 3-D blob tracks.
type Tracker struct {
	mu      sync.Mutex
	blobs   []*Blob
	nextID  int
	lastRun time.Time

	// onBlobAppeared is called when a new blob track is created.
	onBlobAppeared func(b *Blob)
	// onBlobDisappeared is called when a blob track is pruned after gap tolerance.
	onBlobDisappeared func(b *Blob)
}

// OnBlobAppeared sets the callback invoked when a new blob track is created.
func (t *Tracker) OnBlobAppeared(cb func(b *Blob)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onBlobAppeared = cb
}

// OnBlobDisappeared sets the callback invoked when a blob track is pruned.
func (t *Tracker) OnBlobDisappeared(cb func(b *Blob)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onBlobDisappeared = cb
}

// NewTracker creates an empty Tracker.
func NewTracker() *Tracker {
	return &Tracker{lastRun: time.Now()}
}

// Update runs a single tracking cycle.
//
// measurements is a slice of [x, y, z, weight] tuples sourced from fusion.Blob.
// The method predicts existing tracks, associates measurements, spawns new tracks
// for unmatched detections, applies collision avoidance, and prunes stale tracks.
//
// It returns a snapshot of currently active blobs (ukf field is nil).
func (t *Tracker) Update(measurements [][4]float64) []Blob {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	dt := clampDT(now.Sub(t.lastRun).Seconds())
	t.lastRun = now

	// Predict all existing tracks.
	for _, b := range t.blobs {
		b.ukf.Predict(dt)
		b.X, b.Y, b.Z = b.ukf.Position()
		b.VX, b.VY, b.VZ = b.ukf.Velocity()
	}

	// Greedy nearest-neighbour association.
	assigned := make([]bool, len(measurements))
	updated := make([]bool, len(t.blobs))

	for mi, m := range measurements {
		mx, my, mz, mw := m[0], m[1], m[2], m[3]
		bestIdx, bestDist := -1, maxAssocDist
		for bi, b := range t.blobs {
			if updated[bi] {
				continue
			}
			if d := dist3(mx, my, mz, b.X, b.Y, b.Z); d < bestDist {
				bestDist = d
				bestIdx = bi
			}
		}
		if bestIdx >= 0 {
			b := t.blobs[bestIdx]
			b.ukf.Update([measN]float64{mx, my, mz})
			b.X, b.Y, b.Z = b.ukf.Position()
			b.VX, b.VY, b.VZ = b.ukf.Velocity()
			b.Weight = mw
			b.LastSeen = now
			b.Trail = appendTrail3(b.Trail, b.X, b.Y, b.Z)
			b.Posture = estimatePosture(b.Y, b.VX, b.VZ)
			updated[bestIdx] = true
			assigned[mi] = true
		}
	}

	// Spawn new tracks for unmatched measurements.
	for mi, m := range measurements {
		if assigned[mi] {
			continue
		}
		b := &Blob{
			ID:       t.nextID,
			X: m[0], Y: m[1], Z: m[2],
			Weight:   m[3],
			LastSeen: now,
			Trail:    [][3]float64{{m[0], m[1], m[2]}},
			Posture:  PostureUnknown,
			ukf:      NewUKF(m[0], m[1], m[2]),
		}
		t.nextID++
		t.blobs = append(t.blobs, b)
		if t.onBlobAppeared != nil {
			t.onBlobAppeared(b)
		}
	}

	// Prune tracks unseen beyond gap tolerance.
	live := t.blobs[:0]
	for _, b := range t.blobs {
		if now.Sub(b.LastSeen) < gapTolerance {
			live = append(live, b)
		} else if t.onBlobDisappeared != nil {
			t.onBlobDisappeared(b)
		}
	}
	t.blobs = live

	// Collision avoidance: push overlapping blobs apart.
	applyCollisionAvoidance(t.blobs)

	// Return deep-copy snapshot (ukf field omitted).
	out := make([]Blob, len(t.blobs))
	for i, b := range t.blobs {
		out[i] = *b
		trail := make([][3]float64, len(b.Trail))
		copy(trail, b.Trail)
		out[i].Trail = trail
		out[i].ukf = nil
	}
	return out
}

// Reset clears all active tracks.
func (t *Tracker) Reset() {
	t.mu.Lock()
	t.blobs = nil
	t.mu.Unlock()
}

// ─── internal helpers ─────────────────────────────────────────────────────────

func dist3(x1, y1, z1, x2, y2, z2 float64) float64 {
	dx, dy, dz := x1-x2, y1-y2, z1-z2
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

func appendTrail3(trail [][3]float64, x, y, z float64) [][3]float64 {
	trail = append(trail, [3]float64{x, y, z})
	if len(trail) > TrailMaxLen {
		trail = trail[len(trail)-TrailMaxLen:]
	}
	return trail
}

func clampDT(dt float64) float64 {
	if dt < 0.01 {
		return 0.01
	}
	if dt > 2.0 {
		return 2.0
	}
	return dt
}

// estimatePosture classifies body posture from centroid height and horizontal speed.
func estimatePosture(y, vx, vz float64) Posture {
	switch {
	case y < lyingMaxY:
		return PostureLying
	case y < seatedMaxY:
		return PostureSeated
	default:
		if math.Sqrt(vx*vx+vz*vz) > walkThreshold {
			return PostureWalking
		}
		return PostureStanding
	}
}

// applyCollisionAvoidance pushes co-located blobs apart in the floor plane.
// The repulsion nudge is half the overlap on each side, capped to a single pass.
func applyCollisionAvoidance(blobs []*Blob) {
	for i := 0; i < len(blobs); i++ {
		for j := i + 1; j < len(blobs); j++ {
			a, b := blobs[i], blobs[j]
			dx := a.X - b.X
			dz := a.Z - b.Z
			d := math.Sqrt(dx*dx + dz*dz)
			if d < minSeparation && d > 1e-6 {
				push := (minSeparation - d) * 0.5 / d
				a.X += dx * push
				a.Z += dz * push
				b.X -= dx * push
				b.Z -= dz * push
				// Reflect the corrected position back into the UKF state.
				a.ukf.X.SetVec(0, a.X)
				a.ukf.X.SetVec(2, a.Z)
				b.ukf.X.SetVec(0, b.X)
				b.ukf.X.SetVec(2, b.Z)
			}
		}
	}
}
