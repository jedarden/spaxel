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
	WalkerTypeRandomWalk  WalkerType = "random_walk"  // Random Gaussian walk
	WalkerTypePathFollow   WalkerType = "path_follow"   // Follow a predefined path
	WalkerTypeNodeToNode   WalkerType = "node_to_node"  // Traverse between virtual nodes
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
	// Node-to-node traversal fields
	Nodes      []*Node    `json:"nodes,omitempty"`      // List of nodes to visit
	NodeIndex  int        `json:"node_index,omitempty"` // Current target node index
	WaitTimer  float64    `json:"wait_timer,omitempty"` // Time remaining at current node
	WaitTime   float64    `json:"wait_time,omitempty"`  // How long to wait at each node (seconds)
	ShouldWait bool       `json:"should_wait,omitempty"` // Whether to wait at nodes
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

// NewNodeToNodeWalker creates a walker that traverses between virtual nodes
func NewNodeToNodeWalker(id string, nodes []*Node, speed float64, waitTime float64) *Walker {
	if len(nodes) == 0 {
		panic("nodes cannot be empty")
	}
	if len(nodes) == 1 {
		panic("need at least 2 nodes for node-to-node traversal")
	}

	// Start at the first node
	w := NewWalker(id, nodes[0].Position)
	w.Type = WalkerTypeNodeToNode
	w.Nodes = nodes
	w.NodeIndex = 1 // Target is the second node
	w.Speed = speed
	w.WaitTime = waitTime
	w.WaitTimer = waitTime
	w.ShouldWait = waitTime > 0

	return w
}

// NewNodeToNodeWalkerNoWait creates a walker that traverses between nodes without waiting
func NewNodeToNodeWalkerNoWait(id string, nodes []*Node, speed float64) *Walker {
	return NewNodeToNodeWalker(id, nodes, speed, 0)
}

// Update updates the walker's position based on their movement type
// dt is the time step in seconds
func (w *Walker) Update(dt float64, space *Space) {
	switch w.Type {
	case WalkerTypeRandomWalk:
		w.updateRandomWalk(dt, space)
	case WalkerTypePathFollow:
		w.updatePathFollow(dt)
	case WalkerTypeNodeToNode:
		w.updateNodeToNode(dt, space)
	}
}

// updateRandomWalk implements random walk motion
func (w *Walker) updateRandomWalk(dt float64, space *Space) {
	// Update position
	w.Position.X += w.Velocity.X * dt
	w.Position.Y += w.Velocity.Y * dt

	// Get space bounds for collision
	minX, minY, _, maxX, maxY, _ := space.Bounds()

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

	// Update velocity vector for consistency
	w.Velocity.X = (dx / dist) * w.Speed
	w.Velocity.Y = (dy / dist) * w.Speed
	w.Velocity.Z = (dz / dist) * w.Speed
}

// updateNodeToNode implements traversal between virtual nodes
func (w *Walker) updateNodeToNode(dt float64, space *Space) {
	// If no nodes configured, fall back to random walk
	if len(w.Nodes) == 0 {
		w.updateRandomWalk(dt, space)
		return
	}

	// Get current target node
	targetNode := w.Nodes[w.NodeIndex]
	targetPos := targetNode.Position

	// Vector to target
	dx := targetPos.X - w.Position.X
	dy := targetPos.Y - w.Position.Y
	dz := targetPos.Z - w.Position.Z
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

	// Horizontal distance (X/Y only) for arrival detection
	// Walkers maintain constant height, so we check horizontal proximity
	horizontalDist := math.Sqrt(dx*dx + dy*dy)

	// Check if we've arrived at the target node
	if horizontalDist < 0.3 { // Within 30cm horizontally of node
		// Wait at node if configured
		if w.ShouldWait {
			if w.WaitTimer > 0 {
				// Still waiting
				w.WaitTimer -= dt
				// Set velocity to zero while waiting
				w.Velocity = Point{X: 0, Y: 0, Z: 0}
				return
			}
			// Done waiting, reset timer for next node
			w.WaitTimer = w.WaitTime
		}

		// Move to next node
		w.NodeIndex = (w.NodeIndex + 1) % len(w.Nodes)
		return
	}

	// Move towards target node
	// Calculate speed variation for realism (0.8x to 1.2x base speed)
	speedVariation := 0.8 + 0.4*rand.Float64()
	currentSpeed := w.Speed * speedVariation

	// Accelerate/decelerate naturally when starting/stopping
	maxSpeed := currentSpeed
	if horizontalDist < 1.0 {
		// Slow down when approaching target
		maxSpeed = currentSpeed * (horizontalDist / 1.0)
		if maxSpeed < 0.1 {
			maxSpeed = 0.1
		}
	}

	moveDist := maxSpeed * dt
	if moveDist > horizontalDist {
		moveDist = horizontalDist
	}

	t := moveDist / horizontalDist
	w.Position.X += dx * t
	w.Position.Y += dy * t

	// Update velocity vector for consistency
	w.Velocity.X = (dx / horizontalDist) * maxSpeed
	w.Velocity.Y = (dy / horizontalDist) * maxSpeed
	w.Velocity.Z = (dz / dist) * maxSpeed

	// Keep walker at standing height
	w.Position.Z = w.Height
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

// AddNodeToNodeWalker adds a node-to-node traversal walker
func (ws *WalkerSet) AddNodeToNodeWalker(id string, nodes []*Node, speed float64, waitTime float64) {
	ws.Add(NewNodeToNodeWalker(id, nodes, speed, waitTime))
}

// AddNodeToNodeWalkerNoWait adds a node-to-node walker that doesn't wait at nodes
func (ws *WalkerSet) AddNodeToNodeWalkerNoWait(id string, nodes []*Node, speed float64) {
	ws.Add(NewNodeToNodeWalkerNoWait(id, nodes, speed))
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

// CreateNodeToNodeWalkers creates walkers that traverse between virtual nodes
// The walkers move from node to node, optionally waiting at each node
func CreateNodeToNodeWalkers(count int, nodes *NodeSet, speed float64, waitTime float64) *WalkerSet {
	ws := NewWalkerSet()

	allNodes := nodes.All()
	if len(allNodes) < 2 {
		// Not enough nodes, return empty set
		return ws
	}

	for i := 0; i < count; i++ {
		// Each walker gets the same set of nodes but starts at a different target
		// Create a copy of nodes for this walker
		nodeList := make([]*Node, len(allNodes))
		copy(nodeList, allNodes)

		// Shuffle the node order for variety (except first, keep it consistent)
		if i > 0 && len(nodeList) > 2 {
			// Simple rotation for variety
			offset := i % (len(nodeList) - 1)
			for j := 0; j < offset; j++ {
				// Rotate nodes[1:] by one position
				first := nodeList[1]
				copy(nodeList[1:], nodeList[2:])
				nodeList[len(nodeList)-1] = first
			}
		}

		walker := NewNodeToNodeWalker(
			fmt.Sprintf("walker-%d", i),
			nodeList,
			speed,
			waitTime,
		)

		// Start at first node position
		walker.Position = nodeList[0].Position
		walker.NodeIndex = 1 // Target is second node

		ws.Add(walker)
	}

	return ws
}

// CreateNodeToNodeWalkersNoWait creates node-to-node walkers that don't wait at nodes
func CreateNodeToNodeWalkersNoWait(count int, nodes *NodeSet, speed float64) *WalkerSet {
	return CreateNodeToNodeWalkers(count, nodes, speed, 0)
}

// SimulationTick represents one tick of simulation state
type SimulationTick struct {
	Timestamp time.Time `json:"timestamp"`
	Walkers   []*Walker `json:"walkers"`
}

// GenerateTicks generates simulation ticks at the given rate for a duration
func (ws *WalkerSet) GenerateTicks(rateHz int, duration time.Duration, space *Space) <-chan SimulationTick {
	// Use buffered channel to avoid race condition where producer
	// finishes before consumer starts
	out := make(chan SimulationTick, 100)

	go func() {
		defer close(out)

		dt := 1.0 / float64(rateHz)
		start := time.Now()
		var elapsed time.Duration

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

			elapsed += time.Duration(float64(dt) * float64(time.Second))
		}
	}()

	return out
}
