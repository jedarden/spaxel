package simulator

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"time"
)

// WalkerType defines how a walker moves
type WalkerType string

const (
	WalkerTypeRandomWalk WalkerType = "random_walk" // Random Gaussian walk
	WalkerTypePathFollow  WalkerType = "path_follow"  // Follow a predefined path
)

// Walker represents a simulated person moving through the space
type Walker struct {
	ID         string     `json:"id"`
	Name       string     `json:"name,omitempty"`
	Position   Point      `json:"position"`
	Velocity   Point      `json:"velocity"`
	Type       WalkerType `json:"type"`
	Path       []Point    `json:"path,omitempty"`      // For path-following mode
	PathIndex  int        `json:"path_index,omitempty"` // Current position along path
	Speed      float64    `json:"speed"`                // Movement speed in m/s
	Height     float64    `json:"height"`               // Person height in meters
	BLEAddress string     `json:"ble_address,omitempty"` // Simulated BLE device
}

// NewWalker creates a new walker at the given position
func NewWalker(id string, position Point) *Walker {
	return &Walker{
		ID:       id,
		Position: position,
		Type:     WalkerTypeRandomWalk,
		Speed:    1.0,  // 1 m/s default
		Height:   1.7,  // Average person height
		Velocity: Point{X: 0, Y: 0, Z: 0},
	}
}

// NewRandomWalker creates a walker with random motion
func NewRandomWalker(id string, position Point, speed float64) *Walker {
	w := NewWalker(id, position)
	w.Type = WalkerTypeRandomWalk
	w.Speed = speed
	// Initial random velocity
	angle := rand.Float64() * 2 * math.Pi
	w.Velocity = Point{
		X: math.Cos(angle) * speed * 0.5,
		Y: math.Sin(angle) * speed * 0.5,
		Z: 0,
	}
	return w
}

// NewPathWalker creates a walker that follows a predefined path
func NewPathWalker(id string, path []Point, speed float64) *Walker {
	if len(path) == 0 {
		panic("path cannot be empty")
	}
	w := NewWalker(id, path[0])
	w.Type = WalkerTypePathFollow
	w.Path = path
	w.PathIndex = 0
	w.Speed = speed
	return w
}

// Update updates the walker's position based on their movement type
// dt is the time step in seconds
func (w *Walker) Update(dt float64, space *Space) {
	switch w.Type {
	case WalkerTypeRandomWalk:
		w.updateRandomWalk(dt, space)
	case WalkerTypePathFollow:
		w.updatePathFollow(dt)
	}
}

// updateRandomWalk implements random walk motion
func (w *Walker) updateRandomWalk(dt float64, space *Space) {
	// Update position
	w.Position.X += w.Velocity.X * dt
	w.Position.Y += w.Velocity.Y * dt

	// Get space bounds for collision
	minX, minY, _, maxX, maxY, maxZ := space.Bounds()

	// Bounce off walls (with some margin)
	margin := 0.2 // 20cm margin
	if w.Position.X < minX+margin {
		w.Position.X = minX + margin
		w.Velocity.X *= -1
	}
	if w.Position.X > maxX-margin {
		w.Position.X = maxX - margin
		w.Velocity.X *= -1
	}
	if w.Position.Y < minY+margin {
		w.Position.Y = minY + margin
		w.Velocity.Y *= -1
	}
	if w.Position.Y > maxY-margin {
		w.Position.Y = maxY - margin
		w.Velocity.Y *= -1
	}

	// Random velocity perturbation (simulates human motion)
	// Change direction gradually, not abruptly
	perturbation := 0.1 // rad/s
	angle := math.Atan2(w.Velocity.Y, w.Velocity.X)
	angle += (rand.Float64() - 0.5) * perturbation * dt

	// Clamp velocity magnitude
	currentSpeed := math.Sqrt(w.Velocity.X*w.Velocity.X + w.Velocity.Y*w.Velocity.Y)
	targetSpeed := w.Speed * (0.5 + rand.Float64()*0.5) // 50%-100% of set speed
	newSpeed := currentSpeed + (targetSpeed-currentSpeed)*0.1 // Smooth speed change

	maxSpeed := w.Speed * 1.5
	if newSpeed > maxSpeed {
		newSpeed = maxSpeed
	}

	w.Velocity.X = math.Cos(angle) * newSpeed
	w.Velocity.Y = math.Sin(angle) * newSpeed

	// Keep Z at person height (standing)
	w.Position.Z = w.Height
}

// updatePathFollow implements path-following motion
func (w *Walker) updatePathFollow(dt float64) {
	if len(w.Path) == 0 {
		return
	}

	// Get current target point
	target := w.Path[w.PathIndex]

	// Vector to target
	dx := target.X - w.Position.X
	dy := target.Y - w.Position.Y
	dz := target.Z - w.Position.Z
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

	// If very close to target, move to next point
	if dist < 0.1 {
		w.PathIndex = (w.PathIndex + 1) % len(w.Path)
		return
	}

	// Move towards target at constant speed
	moveDist := w.Speed * dt
	if moveDist > dist {
		moveDist = dist
	}

	t := moveDist / dist
	w.Position.X += dx * t
	w.Position.Y += dy * t
	w.Position.Z += dz * t

	// Update velocity vector for consistency
	w.Velocity.X = (dx / dist) * w.Speed
	w.Velocity.Y = (dy / dist) * w.Speed
	w.Velocity.Z = (dz / dist) * w.Speed
}

// WalkerSet is a collection of walkers
type WalkerSet struct {
	walkers []*Walker
}

// NewWalkerSet creates an empty walker set
func NewWalkerSet() *WalkerSet {
	return &WalkerSet{walkers: make([]*Walker, 0)}
}

// Add adds a walker
func (ws *WalkerSet) Add(w *Walker) {
	ws.walkers = append(ws.walkers, w)
}

// AddRandomWalker adds a random walker at the given position
func (ws *WalkerSet) AddRandomWalker(id string, position Point, speed float64) {
	ws.Add(NewRandomWalker(id, position, speed))
}

// AddPathWalker adds a path-following walker
func (ws *WalkerSet) AddPathWalker(id string, path []Point, speed float64) {
	ws.Add(NewPathWalker(id, path, speed))
}

// Count returns the number of walkers
func (ws *WalkerSet) Count() int {
	return len(ws.walkers)
}

// All returns all walkers
func (ws *WalkerSet) All() []*Walker {
	return ws.walkers
}

// GetByID returns a walker by ID
func (ws *WalkerSet) GetByID(id string) *Walker {
	for _, w := range ws.walkers {
		if w.ID == id {
			return w
		}
	}
	return nil
}

// Remove removes a walker by ID
func (ws *WalkerSet) Remove(id string) bool {
	for i, w := range ws.walkers {
		if w.ID == id {
			ws.walkers = append(ws.walkers[:i], ws.walkers[i+1:]...)
			return true
		}
	}
	return false
}

// Clear removes all walkers
func (ws *WalkerSet) Clear() {
	ws.walkers = make([]*Walker, 0)
}

// Update updates all walkers
func (ws *WalkerSet) Update(dt float64, space *Space) {
	for _, w := range ws.walkers {
		w.Update(dt, space)
	}
}

// Positions returns all walker positions
func (ws *WalkerSet) Positions() []Point {
	positions := make([]Point, len(ws.walkers))
	for i, w := range ws.walkers {
		positions[i] = w.Position
	}
	return positions
}

// MarshalJSON implements custom JSON marshaling
func (ws *WalkerSet) MarshalJSON() ([]byte, error) {
	return json.Marshal(ws.walkers)
}

// UnmarshalJSON implements custom JSON unmarshaling
func (ws *WalkerSet) UnmarshalJSON(data []byte) error {
	return json.Unmarshal(data, &ws.walkers)
}

// CreateRandomWalkers creates random walkers distributed in the space
func CreateRandomWalkers(count int, space *Space) *WalkerSet {
	ws := NewWalkerSet()
	minX, minY, _, maxX, maxY, _ := space.Bounds()

	for i := 0; i < count; i++ {
		position := Point{
			X: minX + rand.Float64()*(maxX-minX),
			Y: minY + rand.Float64()*(maxY-minY),
			Z: 1.7, // Average person height
		}
		ws.AddRandomWalker(
			fmt.Sprintf("walker-%d", i),
			position,
			0.8+rand.Float64()*0.4, // 0.8-1.2 m/s
		)
	}

	return ws
}

// CreatePathWalkers creates walkers that follow rectangular paths around the space perimeter
func CreatePathWalkers(count int, space *Space) *WalkerSet {
	ws := NewWalkerSet()
	minX, minY, _, maxX, maxY, _ := space.Bounds()

	// Create a rectangular path around the perimeter
	path := []Point{
		{X: minX + 0.5, Y: minY + 0.5, Z: 1.7},
		{X: maxX - 0.5, Y: minY + 0.5, Z: 1.7},
		{X: maxX - 0.5, Y: maxY - 0.5, Z: 1.7},
		{X: minX + 0.5, Y: maxY - 0.5, Z: 1.7},
	}

	for i := 0; i < count; i++ {
		// Offset each walker to start at different positions on the path
		offset := (float64(i) / float64(count)) * float64(len(path))
		startIdx := int(offset) % len(path)

		walker := NewPathWalker(
			fmt.Sprintf("walker-%d", i),
			path,
			0.8+rand.Float64()*0.4,
		)
		walker.PathIndex = startIdx
		walker.Position = path[startIdx]
		ws.Add(walker)
	}

	return ws
}

// SimulationTick represents one tick of simulation state
type SimulationTick struct {
	Timestamp time.Time `json:"timestamp"`
	Walkers   []*Walker `json:"walkers"`
}

// GenerateTicks generates simulation ticks at the given rate for a duration
func (ws *WalkerSet) GenerateTicks(rateHz int, duration time.Duration, space *Space) <-chan SimulationTick {
	out := make(chan SimulationTick)

	go func() {
		defer close(out)

		dt := 1.0 / float64(rateHz)
		start := time.Now()
		elapsed := time.Duration(0)

		for elapsed < duration {
			tick := SimulationTick{
				Timestamp: start.Add(elapsed),
				Walkers:   make([]*Walker, len(ws.walkers)),
			}

			// Update all walkers
			ws.Update(dt, space)

			// Copy current walker states
			for i, w := range ws.walkers {
				// Create a copy of the walker
				wCopy := *w
				tick.Walkers[i] = &wCopy
			}

			select {
			case out <- tick:
			default:
				// Channel full, skip this tick
			}

			elapsed += time.Duration(float64(time.Second) / float64(rateHz))
		}
	}()

	return out
}
