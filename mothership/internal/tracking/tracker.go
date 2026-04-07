package tracking

import (
	"math"
	"sync"
	"time"
)

// Posture is the estimated body posture of a tracked person.
type Posture string

const (
	PostureUnknown  Posture = "unknown"
	PostureStanding Posture = "standing"
	PostureWalking  Posture = "walking"
	PostureSeated   Posture = "seated"
	PostureLying    Posture = "lying"
)

// Blob represents a tracked person/object in the room.
type Blob struct {
	ID        int
	X         float64   // metres, room X
	Z         float64   // metres, room Z
	VX        float64   // m/s
	VZ        float64   // m/s
	Weight    float64   // localisation confidence [0..1]
	LastSeen  time.Time
	Trail     [][2]float64 // recent positions (newest last)
	ukf       *UKF

	// Identity fields (populated by BLE-to-blob matching)
	PersonID           string    `json:"person_id,omitempty"`            // UUID from BLE registry
	PersonLabel        string    `json:"person_label,omitempty"`         // Display name
	PersonColor        string    `json:"person_color,omitempty"`         // Hex color for dashboard
	IdentityConfidence float64   `json:"identity_confidence,omitempty"`  // Match confidence [0..1]
	IdentitySource     string    `json:"identity_source,omitempty"`      // "ble_triangulation", "ble_only", or ""
	IdentityLastSeen   time.Time `json:"-"`                              // Last time identity was confirmed
	Posture            Posture   `json:"posture,omitempty"`             // Estimated body posture
}

// TrailMaxLen is the maximum number of trail points kept per blob.
const TrailMaxLen = 60

// maxAssocDist is the maximum distance for associating a measurement to a track.
const maxAssocDist = 2.0 // metres

// staleTimeout is how long without measurement before removing a track.
const staleTimeout = 5 * time.Second

// BlobEvent represents a blob lifecycle event (appear or disappear).
type BlobEvent struct {
	BlobID    int
	X, Z      float64
	Timestamp time.Time
}

// Tracker manages a set of active blob tracks.
type Tracker struct {
	mu              sync.Mutex
	blobs           []*Blob
	nextID          int
	lastRun         time.Time
	onBlobAppear    func(BlobEvent)
	onBlobDisappear func(BlobEvent)
}

// NewTracker creates an empty tracker.
func NewTracker() *Tracker {
	return &Tracker{
		lastRun: time.Now(),
	}
}

// SetOnBlobAppear sets a callback fired when a new blob is first detected.
func (t *Tracker) SetOnBlobAppear(cb func(BlobEvent)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onBlobAppear = cb
}

// SetOnBlobDisappear sets a callback fired when a blob is removed after staleness.
func (t *Tracker) SetOnBlobDisappear(cb func(BlobEvent)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.onBlobDisappear = cb
}

// Update runs a single tracking step given a set of (x, z, weight) measurements.
// It returns the current set of active blobs after association and pruning.
func (t *Tracker) Update(measurements [][3]float64) []Blob {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	dt := now.Sub(t.lastRun).Seconds()
	if dt < 0.01 {
		dt = 0.01
	}
	if dt > 2.0 {
		dt = 2.0
	}
	t.lastRun = now

	// Predict all existing tracks.
	for _, b := range t.blobs {
		b.ukf.Predict(dt)
		bx, bz := b.ukf.Position()
		b.X = bx
		b.Z = bz
		vx, vz := b.ukf.Velocity()
		b.VX = vx
		b.VZ = vz
	}

	// Hungarian-style nearest-neighbour association (greedy, sufficient for ≤8 blobs).
	assigned := make([]bool, len(measurements))
	updated := make([]bool, len(t.blobs))

	for mi, meas := range measurements {
		mx, mz, mw := meas[0], meas[1], meas[2]

		bestIdx := -1
		bestDist := maxAssocDist
		for bi, b := range t.blobs {
			if updated[bi] {
				continue
			}
			bx, bz := b.ukf.Position()
			dist := euclidean(mx, mz, bx, bz)
			if dist < bestDist {
				bestDist = dist
				bestIdx = bi
			}
		}

		if bestIdx >= 0 {
			// Update existing track.
			b := t.blobs[bestIdx]
			b.ukf.Update([2]float64{mx, mz})
			bx, bz := b.ukf.Position()
			b.X = bx
			b.Z = bz
			vx, vz := b.ukf.Velocity()
			b.VX = vx
			b.VZ = vz
			b.Weight = mw
			b.LastSeen = now
			b.Trail = appendTrail(b.Trail, bx, bz)
			updated[bestIdx] = true
			assigned[mi] = true
		}
	}

	// Spawn new tracks for unassigned measurements.
	for mi, meas := range measurements {
		if assigned[mi] {
			continue
		}
		b := &Blob{
			ID:       t.nextID,
			X:        meas[0],
			Z:        meas[1],
			Weight:   meas[2],
			LastSeen: now,
			Trail:    [][2]float64{{meas[0], meas[1]}},
			ukf:      NewUKF(meas[0], meas[1]),
		}
		t.nextID++
		t.blobs = append(t.blobs, b)

		if t.onBlobAppear != nil {
			t.onBlobAppear(BlobEvent{
				BlobID:    b.ID,
				X:         b.X,
				Z:         b.Z,
				Timestamp: now,
			})
		}
	}

	// Remove stale tracks.
	live := t.blobs[:0]
	for _, b := range t.blobs {
		if now.Sub(b.LastSeen) < staleTimeout {
			live = append(live, b)
		} else if t.onBlobDisappear != nil {
			t.onBlobDisappear(BlobEvent{
				BlobID:    b.ID,
				X:         b.X,
				Z:         b.Z,
				Timestamp: now,
			})
		}
	}
	t.blobs = live

	// Return snapshot.
	out := make([]Blob, len(t.blobs))
	for i, b := range t.blobs {
		out[i] = *b
		// Deep-copy trail slice.
		trail := make([][2]float64, len(b.Trail))
		copy(trail, b.Trail)
		out[i].Trail = trail
	}
	return out
}

// Reset clears all active tracks.
func (t *Tracker) Reset() {
	t.mu.Lock()
	t.blobs = nil
	t.mu.Unlock()
}

func euclidean(x1, z1, x2, z2 float64) float64 {
	dx := x1 - x2
	dz := z1 - z2
	return math.Sqrt(dx*dx + dz*dz)
}

func appendTrail(trail [][2]float64, x, z float64) [][2]float64 {
	trail = append(trail, [2]float64{x, z})
	if len(trail) > TrailMaxLen {
		trail = trail[len(trail)-TrailMaxLen:]
	}
	return trail
}
