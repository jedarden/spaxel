// Package simulator provides pre-deployment simulation capabilities.
//
// It allows users to model their space, place virtual nodes, and run
// synthetic walkers to estimate expected accuracy before purchasing hardware.
package simulator

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

// Session represents a simulation session.
type Session struct {
	mu           sync.RWMutex
	id           string
	space        *Space
	nodes        []*VirtualNode
	walkers      []*Walker
	params       *SimulationParams
	state        SessionState
	created_at   int64
	updated_at   int64
	ctx          chan struct{}
}

// SessionState is the state of a simulation session.
type SessionState string

const (
	StateSetup    SessionState = "setup"
	StateRunning  SessionState = "running"
	StatePaused   SessionState = "paused"
	StateComplete SessionState = "complete"
)

// Space represents the simulated physical space.
type Space struct {
	Width  float64 `json:"width"`  // meters
	Depth  float64 `json:"depth"`  // meters
	Height float64 `json:"height"` // meters
	Walls  []Wall  `json:"walls"`
}

// Wall represents a wall segment that affects signal propagation.
type Wall struct {
	X1     float64 `json:"x1"` // start point (meters)
	Y1     float64 `json:"y1"`
	X2     float64 `json:"x2"` // end point (meters)
	Y2     float64 `json:"y2"`
	Material string `json:"material"` // "drywall", "brick", "concrete", "glass", "metal"
}

// Wall attenuation values in dB
var wallAttenuationDB = map[string]float64{
	"drywall":  3.0,
	"brick":    10.0,
	"concrete": 10.0,
	"glass":    2.0,
	"metal":    20.0,
}

// VirtualNode represents a simulated node.
type VirtualNode struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Position Vector3 `json:"position"` // x, y, z in meters
	Role     string  `json:"role"`     // "tx", "rx", "tx_rx"
}

// Vector3 represents a 3D position.
type Vector3 struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	Z float64 `json:"z"`
}

// Walker represents a simulated person moving through the space.
type Walker struct {
	ID          string    `json:"id"`
	Type        WalkerType `json:"type"`
	Position    Vector3    `json:"position"`
	Velocity    Vector3    `json:"velocity"`
	Path        []Vector3  `json:"path,omitempty"`        // for path walks
	PathIndex   int        `json:"path_index,omitempty"`   // current position in path
	TargetZones []string   `json:"target_zones,omitempty"` // for zone walks
}

// WalkerType defines the type of walker movement.
type WalkerType string

const (
	WalkerTypeRandom WalkerType = "random"
	WalkerTypePath   WalkerType = "path"
	WalkerTypeZone   WalkerType = "zone"
)

// SimulationParams holds simulation parameters.
type SimulationParams struct {
	TickRateHz        int     `json:"tick_rate_hz"`         // 10 Hz default
	WalkerSpeed       float64 `json:"walker_speed"`          // m/s
	SignalAmplitude   float64 `json:"signal_amplitude"`      // 0.05
	FresnelSigma      float64 `json:"fresnel_sigma"`         // 0.3m
	NoiseSigma        float64 `json:"noise_sigma"`           // Gaussian noise std dev
	DefaultRSSI       float64 `json:"default_rssi"`          // -30 dBm at 1m
	WallAttenuationDB float64 `json:"wall_attenuation_db"`   // default 4 dB
}

// DefaultSimulationParams returns the default simulation parameters.
func DefaultSimulationParams() *SimulationParams {
	return &SimulationParams{
		TickRateHz:        10,
		WalkerSpeed:       1.0,
		SignalAmplitude:   0.05,
		FresnelSigma:      0.3,
		NoiseSigma:        0.01,
		DefaultRSSI:       -30.0,
		WallAttenuationDB: 4.0,
	}
}

// NewSession creates a new simulation session.
func NewSession(id string, space *Space) *Session {
	return &Session{
		id:         id,
		space:      space,
		nodes:      []*VirtualNode{},
		walkers:    []*Walker{},
		params:     DefaultSimulationParams(),
		state:      StateSetup,
		created_at: time.Now().UnixMilli(),
		updated_at: time.Now().UnixMilli(),
		ctx:        make(chan struct{}),
	}
}

// ID returns the session ID.
func (s *Session) ID() string {
	return s.id
}

// State returns the current session state.
func (s *Session) State() SessionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// AddNode adds a virtual node to the simulation.
func (s *Session) AddNode(node *VirtualNode) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateSetup {
		return fmt.Errorf("cannot add nodes in state %s", s.state)
	}

	s.nodes = append(s.nodes, node)
	s.updated_at = time.Now().UnixMilli()
	return nil
}

// RemoveNode removes a virtual node from the simulation.
func (s *Session) RemoveNode(nodeID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateSetup {
		return fmt.Errorf("cannot remove nodes in state %s", s.state)
	}

	for i, node := range s.nodes {
		if node.ID == nodeID {
			s.nodes = append(s.nodes[:i], s.nodes[i+1:]...)
			s.updated_at = time.Now().UnixMilli()
			return nil
		}
	}
	return fmt.Errorf("node not found: %s", nodeID)
}

// AddWalker adds a walker to the simulation.
func (s *Session) AddWalker(walker *Walker) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateSetup {
		return fmt.Errorf("cannot add walkers in state %s", s.state)
	}

	s.walkers = append(s.walkers, walker)
	s.updated_at = time.Now().UnixMilli()
	return nil
}

// Start starts the simulation.
func (s *Session) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateSetup && s.state != StatePaused {
		return fmt.Errorf("cannot start in state %s", s.state)
	}

	if len(s.nodes) < 2 {
		return fmt.Errorf("need at least 2 nodes for simulation")
	}

	if len(s.walkers) == 0 {
		return fmt.Errorf("need at least 1 walker for simulation")
	}

	s.state = StateRunning
	s.updated_at = time.Now().UnixMilli()

	// Start simulation loop in background
	go s.simulationLoop()

	return nil
}

// Pause pauses the simulation.
func (s *Session) Pause() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateRunning {
		return fmt.Errorf("cannot pause in state %s", s.state)
	}

	s.state = StatePaused
	s.updated_at = time.Now().UnixMilli()
	return nil
}

// Stop stops the simulation.
func (s *Session) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.state != StateRunning && s.state != StatePaused {
		return fmt.Errorf("cannot stop in state %s", s.state)
	}

	s.state = StateComplete
	close(s.ctx)
	s.updated_at = time.Now().UnixMilli()
	return nil
}

// simulationLoop runs the main simulation loop.
func (s *Session) simulationLoop() {
	ticker := time.NewTicker(time.Second / time.Duration(s.params.TickRateHz))
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.mu.Lock()
			if s.state != StateRunning {
				s.mu.Unlock()
				continue
			}

			// Update walker positions
			for _, walker := range s.walkers {
				s.updateWalkerPosition(walker)
			}

			s.updated_at = time.Now().UnixMilli()
			s.mu.Unlock()

		case <-s.ctx:
			return
		}
	}
}

// updateWalkerPosition updates a single walker's position based on its type.
func (s *Session) updateWalkerPosition(walker *Walker) {
	switch walker.Type {
	case WalkerTypeRandom:
		s.updateRandomWalker(walker)
	case WalkerTypePath:
		s.updatePathWalker(walker)
	case WalkerTypeZone:
		s.updateZoneWalker(walker)
	}
}

// updateRandomWalker updates a random walker using Gaussian velocity updates.
func (s *Session) updateRandomWalker(walker *Walker) {
	// Apply velocity with small random changes
	walker.Position.X += walker.Velocity.X / float64(s.params.TickRateHz)
	walker.Position.Y += walker.Velocity.Y / float64(s.params.TickRateHz)

	// Random velocity changes
	velocityChange := 0.5 / float64(s.params.TickRateHz)
	walker.Velocity.X += (randFloat64()*2 - 1) * velocityChange
	walker.Velocity.Y += (randFloat64()*2 - 1) * velocityChange

	// Clamp velocity to max speed
	maxSpeed := s.params.WalkerSpeed
	speed := math.Sqrt(walker.Velocity.X*walker.Velocity.X + walker.Velocity.Y*walker.Velocity.Y)
	if speed > maxSpeed {
		scale := maxSpeed / speed
		walker.Velocity.X *= scale
		walker.Velocity.Y *= scale
	}

	// Bounce off walls
	if walker.Position.X < 0 {
		walker.Position.X = 0
		walker.Velocity.X *= -1
	}
	if walker.Position.X > s.space.Width {
		walker.Position.X = s.space.Width
		walker.Velocity.X *= -1
	}
	if walker.Position.Y < 0 {
		walker.Position.Y = 0
		walker.Velocity.Y *= -1
	}
	if walker.Position.Y > s.space.Depth {
		walker.Position.Y = s.space.Depth
		walker.Velocity.Y *= -1
	}
}

// updatePathWalker updates a path-following walker.
func (s *Session) updatePathWalker(walker *Walker) {
	if len(walker.Path) == 0 {
		return
	}

	target := walker.Path[walker.PathIndex]
	dx := target.X - walker.Position.X
	dy := target.Y - walker.Position.Y
	distance := math.Sqrt(dx*dx + dy*dy)

	stepSize := s.params.WalkerSpeed / float64(s.params.TickRateHz)

	if distance <= stepSize {
		// Reached target, move to next waypoint
		walker.Position = target
		walker.PathIndex = (walker.PathIndex + 1) % len(walker.Path)
	} else {
		// Move toward target
		walker.Position.X += (dx / distance) * stepSize
		walker.Position.Y += (dy / distance) * stepSize
	}
}

// updateZoneWalker updates a zone-walking walker.
func (s *Session) updateZoneWalker(walker *Walker) {
	// For now, treat zone walkers as random walkers
	// TODO: Implement zone-based movement
	s.updateRandomWalker(walker)
}

// GetSnapshot returns the current simulation state.
func (s *Session) GetSnapshot() *SessionSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	walkerPositions := make([]WalkerPosition, len(s.walkers))
	for i, w := range s.walkers {
		walkerPositions[i] = WalkerPosition{
			ID:       w.ID,
			Position: w.Position,
		}
	}

	return &SessionSnapshot{
		ID:              s.id,
		State:           s.state,
		Space:           s.space,
		NodeCount:       len(s.nodes),
		WalkerPositions: walkerPositions,
		UpdatedAt:       s.updated_at,
	}
}

// SessionSnapshot represents a point-in-time snapshot of the simulation.
type SessionSnapshot struct {
	ID              string           `json:"id"`
	State           SessionState     `json:"state"`
	Space           *Space           `json:"space"`
	NodeCount       int              `json:"node_count"`
	WalkerPositions []WalkerPosition `json:"walker_positions"`
	UpdatedAt       int64            `json:"updated_at"`
}

// WalkerPosition represents a walker's position at a point in time.
type WalkerPosition struct {
	ID       string  `json:"id"`
	Position Vector3 `json:"position"`
}

// randFloat64 returns a random float64 in [0, 1).
func randFloat64() float64 {
	return float64(time.Now().UnixNano()%1000) / 1000.0
}

// ToJSON converts the session to JSON.
func (s *Session) ToJSON() ([]byte, error) {
	snapshot := s.GetSnapshot()
	return json.Marshal(snapshot)
}
