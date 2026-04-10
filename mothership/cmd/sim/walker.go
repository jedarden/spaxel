// Package main provides walker simulation for the CSI simulator.
package main

import (
	"encoding/csv"
	"fmt"
	"math"
	"math/rand"
	"os"
	"time"

	"github.com/spaxel/mothership/internal/simulator"
)

// WalkerSimulator manages simulated walkers
type WalkerSimulator struct {
	walkers     []*SimWalker
	spaceWidth  float64
	spaceDepth  float64
	spaceHeight float64
	rng         *rand.Rand
	csvFile     *os.File
	csvWriter   *csv.Writer
}

// SimWalker represents a simulated person moving through space
type SimWalker struct {
	ID           string
	Position     [3]float64 // x, y, z in meters
	Velocity     [3]float64 // vx, vy, vz in m/s
	BLEAddress   string     // Simulated BLE device
	lastUpdate   time.Time
	path         []*WalkerPathPoint
	pathIndex    int
	seed         int64
}

// WalkerPathPoint represents a point in a predefined path
type WalkerPathPoint struct {
	Position [3]float64
	WaitTime time.Duration
}

// NewWalkerSimulator creates a new walker simulator
func NewWalkerSimulator(count int, width, depth, height float64, seed int64) *WalkerSimulator {
	rng := rand.New(rand.NewSource(seed))

	walkers := make([]*SimWalker, count)
	for i := 0; i < count; i++ {
		walkers[i] = &SimWalker{
			ID: fmt.Sprintf("walker-%d", i),
			Position: [3]float64{
				width/2 + (rng.Float64()-0.5)*width*0.5,
				depth/2 + (rng.Float64()-0.5)*depth*0.5,
				1.7, // Average person height
			},
			Velocity: [3]float64{
				(rng.Float64() - 0.5) * 0.5,
				(rng.Float64() - 0.5) * 0.5,
				0,
			},
			BLEAddress: fmt.Sprintf("11:22:33:44:55:%02X", i),
			lastUpdate: time.Now(),
			seed:      seed + int64(i),
		}
	}

	return &WalkerSimulator{
		walkers:     walkers,
		spaceWidth:  width,
		spaceDepth:  depth,
		spaceHeight: height,
		rng:         rng,
	}
}

// SetPath sets a predefined path for a walker
func (ws *WalkerSimulator) SetPath(walkerIndex int, path [][3]float64) {
	if walkerIndex >= 0 && walkerIndex < len(ws.walkers) {
		ws.walkers[walkerIndex].path = make([]*WalkerPathPoint, len(path))
		for i, p := range path {
			ws.walkers[walkerIndex].path[i] = &WalkerPathPoint{
				Position: p,
				WaitTime: 0,
			}
		}
		ws.walkers[walkerIndex].pathIndex = 0
	}
}

// OpenCSV opens a CSV file for writing ground truth data
func (ws *WalkerSimulator) OpenCSV(filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create CSV file: %w", err)
	}

	ws.csvFile = file
	ws.csvWriter = csv.NewWriter(file)

	// Write header
	header := []string{"timestamp_ms", "walker_id", "x", "y", "z", "vx", "vy", "vz"}
	if err := ws.csvWriter.Write(header); err != nil {
		return fmt.Errorf("failed to write CSV header: %w", err)
	}

	return nil
}

// CloseCSV closes the CSV file
func (ws *WalkerSimulator) CloseCSV() error {
	if ws.csvWriter != nil {
		ws.csvWriter.Flush()
	}
	if ws.csvFile != nil {
		return ws.csvFile.Close()
	}
	return nil
}

// WriteCSVRow writes a row to the CSV file
func (ws *WalkerSimulator) WriteCSVRow(timestamp time.Time, walker *SimWalker) error {
	if ws.csvWriter == nil {
		return nil
	}

	row := []string{
		fmt.Sprintf("%d", timestamp.UnixMilli()),
		walker.ID,
		fmt.Sprintf("%.3f", walker.Position[0]),
		fmt.Sprintf("%.3f", walker.Position[1]),
		fmt.Sprintf("%.3f", walker.Position[2]),
		fmt.Sprintf("%.3f", walker.Velocity[0]),
		fmt.Sprintf("%.3f", walker.Velocity[1]),
		fmt.Sprintf("%.3f", walker.Velocity[2]),
	}

	return ws.csvWriter.Write(row)
}

// Update updates all walker positions
// dt is the time step in seconds
func (ws *WalkerSimulator) Update(dt float64) {
	for _, w := range ws.walkers {
		ws.updateWalker(w, dt)
	}
}

// updateWalker updates a single walker's position
func (ws *WalkerSimulator) updateWalker(w *SimWalker, dt float64) {
	// If path is defined, follow path
	if len(w.path) > 0 {
		ws.followPath(w, dt)
		return
	}

	// Random walk motion
	const dtStep = 0.05 // 50ms step

	// Update position
	w.Position[0] += w.Velocity[0] * dtStep
	w.Position[1] += w.Velocity[1] * dtStep

	// Bounce off walls
	margin := 0.2 // 20cm margin
	if w.Position[0] < margin {
		w.Position[0] = margin
		w.Velocity[0] *= -1
	}
	if w.Position[0] > ws.spaceWidth-margin {
		w.Position[0] = ws.spaceWidth - margin
		w.Velocity[0] *= -1
	}
	if w.Position[1] < margin {
		w.Position[1] = margin
		w.Velocity[1] *= -1
	}
	if w.Position[1] > ws.spaceDepth-margin {
		w.Position[1] = ws.spaceDepth - margin
		w.Velocity[1] *= -1
	}

	// Random velocity perturbation
	w.Velocity[0] += (ws.rng.Float64() - 0.5) * 0.1
	w.Velocity[1] += (ws.rng.Float64() - 0.5) * 0.1

	// Clamp velocity
	maxSpeed := 0.5
	speed := math.Sqrt(w.Velocity[0]*w.Velocity[0] + w.Velocity[1]*w.Velocity[1])
	if speed > maxSpeed {
		scale := maxSpeed / speed
		w.Velocity[0] *= scale
		w.Velocity[1] *= scale
	}

	w.lastUpdate = time.Now()
}

// followPath makes a walker follow a predefined path
func (ws *WalkerSimulator) followPath(w *SimWalker, dt float64) {
	if w.pathIndex >= len(w.path) {
		w.pathIndex = 0 // Loop back to start
	}

	target := w.path[w.pathIndex].Position

	// Vector to target
	dx := target[0] - w.Position[0]
	dy := target[1] - w.Position[1]
	dz := target[2] - w.Position[2]
	dist := math.Sqrt(dx*dx + dy*dy + dz*dz)

	// If very close to target, move to next point
	if dist < 0.1 {
		w.pathIndex++
		return
	}

	// Move towards target at constant speed
	moveDist := 0.5 * dt // 0.5 m/s
	if moveDist > dist {
		moveDist = dist
	}

	t := moveDist / dist
	w.Position[0] += dx * t
	w.Position[1] += dy * t
	w.Position[2] += dz * t

	// Update velocity vector for consistency
	if dist > 0 {
		w.Velocity[0] = (dx / dist) * 0.5
		w.Velocity[1] = (dy / dist) * 0.5
		w.Velocity[2] = (dz / dist) * 0.5
	}

	w.lastUpdate = time.Now()
}

// GetWalkers returns all walkers
func (ws *WalkerSimulator) GetWalkers() []*SimWalker {
	return ws.walkers
}

// GetWalkerPositions returns walker positions as simulator.Point slice
func (ws *WalkerSimulator) GetWalkerPositions() []simulator.Point {
	positions := make([]simulator.Point, len(ws.walkers))
	for i, w := range ws.walkers {
		positions[i] = simulator.Point{
			X: w.Position[0],
			Y: w.Position[1],
			Z: w.Position[2],
		}
	}
	return positions
}

// GetWalkerByID returns a walker by ID
func (ws *WalkerSimulator) GetWalkerByID(id string) *SimWalker {
	for _, w := range ws.walkers {
		if w.ID == id {
			return w
		}
	}
	return nil
}

// Count returns the number of walkers
func (ws *WalkerSimulator) Count() int {
	return len(ws.walkers)
}

// ValidateWalkerPosition checks if a walker position is within bounds
func (ws *WalkerSimulator) ValidateWalkerPosition(pos [3]float64) bool {
	return pos[0] >= 0 && pos[0] <= ws.spaceWidth &&
		pos[1] >= 0 && pos[1] <= ws.spaceDepth &&
		pos[2] >= 0 && pos[2] <= ws.spaceHeight
}

// GenerateNodeToNodePath generates a path through node positions
func (ws *WalkerSimulator) GenerateNodeToNodePath(nodePositions [][3]float64) [][3]float64 {
	if len(nodePositions) < 2 {
		return nodePositions
	}

	// Create path that visits each node in order
	path := make([][3]float64, len(nodePositions))
	copy(path, nodePositions)

	return path
}

// GenerateRandomPath generates a random rectangular path around the space
func (ws *WalkerSimulator) GenerateRandomPath(numPoints int) [][3]float64 {
	path := make([][3]float64, numPoints)
	margin := 0.5 // 50cm margin from walls

	for i := 0; i < numPoints; i++ {
		// Generate random positions within bounds
		path[i] = [3]float64{
			margin + ws.rng.Float64()*(ws.spaceWidth-2*margin),
			margin + ws.rng.Float64()*(ws.spaceDepth-2*margin),
			1.7, // Average person height
		}
	}

	return path
}

// GeneratePerimeterPath generates a rectangular path around the space perimeter
func (ws *WalkerSimulator) GeneratePerimeterPath() [][3]float64 {
	margin := 0.5 // 50cm margin from walls

	return [][3]float64{
		{margin, margin, 1.7},
		{ws.spaceWidth - margin, margin, 1.7},
		{ws.spaceWidth - margin, ws.spaceDepth - margin, 1.7},
		{margin, ws.spaceDepth - margin, 1.7},
	}
}

// GetWalkerSpeed returns the current speed of a walker
func (w *SimWalker) GetSpeed() float64 {
	return math.Sqrt(w.Velocity[0]*w.Velocity[0] + w.Velocity[1]*w.Velocity[1] + w.Velocity[2]*w.Velocity[2])
}

// IsMoving returns true if the walker is moving (speed > threshold)
func (w *SimWalker) IsMoving() bool {
	return w.GetSpeed() > 0.01
}

// GetPositionAsPoint returns walker position as simulator.Point
func (w *SimWalker) GetPositionAsPoint() simulator.Point {
	return simulator.Point{
		X: w.Position[0],
		Y: w.Position[1],
		Z: w.Position[2],
	}
}

// SetPosition sets the walker's position
func (w *SimWalker) SetPosition(x, y, z float64) {
	w.Position[0] = x
	w.Position[1] = y
	w.Position[2] = z
}

// SetVelocity sets the walker's velocity
func (w *SimWalker) SetVelocity(vx, vy, vz float64) {
	w.Velocity[0] = vx
	w.Velocity[1] = vy
	w.Velocity[2] = vz
}

// GetDistanceToNode returns distance from walker to a node position
func (w *SimWalker) GetDistanceToNode(nodePos [3]float64) float64 {
	dx := nodePos[0] - w.Position[0]
	dy := nodePos[1] - w.Position[1]
	dz := nodePos[2] - w.Position[2]
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// Clone creates a deep copy of the walker
func (w *SimWalker) Clone() *SimWalker {
	clone := &SimWalker{
		ID:         w.ID,
		Position:   w.Position,
		Velocity:   w.Velocity,
		BLEAddress: w.BLEAddress,
		lastUpdate: w.lastUpdate,
		pathIndex:  w.pathIndex,
		seed:       w.seed,
	}

	if len(w.path) > 0 {
		clone.path = make([]*WalkerPathPoint, len(w.path))
		for i, p := range w.path {
			clone.path[i] = &WalkerPathPoint{
				Position: p.Position,
				WaitTime: p.WaitTime,
			}
		}
	}

	return clone
}
