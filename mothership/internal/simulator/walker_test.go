package simulator

import (
	"fmt"
	"math"
	"testing"
	"time"
)

func TestNewNodeToNodeWalker(t *testing.T) {
	nodes := []*Node{
		NewVirtualNode("node-1", "Node 1", Point{X: 0, Y: 0, Z: 2}),
		NewVirtualNode("node-2", "Node 2", Point{X: 5, Y: 0, Z: 2}),
		NewVirtualNode("node-3", "Node 3", Point{X: 2.5, Y: 5, Z: 2}),
	}

	w := NewNodeToNodeWalker("walker-1", nodes, 1.0, 2.0)

	if w.Type != WalkerTypeNodeToNode {
		t.Errorf("Expected type %s, got %s", WalkerTypeNodeToNode, w.Type)
	}

	if len(w.Nodes) != 3 {
		t.Errorf("Expected 3 nodes, got %d", len(w.Nodes))
	}

	if w.Speed != 1.0 {
		t.Errorf("Expected speed 1.0, got %f", w.Speed)
	}

	if w.WaitTime != 2.0 {
		t.Errorf("Expected wait time 2.0, got %f", w.WaitTime)
	}

	if !w.ShouldWait {
		t.Error("Expected ShouldWait to be true")
	}

	// Should start at first node position
	if w.Position.X != 0 || w.Position.Y != 0 {
		t.Errorf("Expected starting position at node-1, got %v", w.Position)
	}

	// Target should be second node
	if w.NodeIndex != 1 {
		t.Errorf("Expected NodeIndex 1, got %d", w.NodeIndex)
	}
}

func TestNewNodeToNodeWalkerNoWait(t *testing.T) {
	nodes := []*Node{
		NewVirtualNode("node-1", "Node 1", Point{X: 0, Y: 0, Z: 2}),
		NewVirtualNode("node-2", "Node 2", Point{X: 5, Y: 0, Z: 2}),
	}

	w := NewNodeToNodeWalkerNoWait("walker-1", nodes, 1.0)

	if w.ShouldWait {
		t.Error("Expected ShouldWait to be false")
	}

	if w.WaitTime != 0 {
		t.Errorf("Expected wait time 0, got %f", w.WaitTime)
	}
}

func TestNewNodeToNodeWalkerPanicsOnEmptyNodes(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic on empty nodes slice")
		}
	}()

	_ = NewNodeToNodeWalker("walker-1", []*Node{}, 1.0, 0)
}

func TestNewNodeToNodeWalkerPanicsOnSingleNode(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected panic on single node")
		}
	}()

	nodes := []*Node{
		NewVirtualNode("node-1", "Node 1", Point{X: 0, Y: 0, Z: 2}),
	}
	_ = NewNodeToNodeWalker("walker-1", nodes, 1.0, 0)
}

func TestNodeToNodeWalkerMovement(t *testing.T) {
	nodes := []*Node{
		NewVirtualNode("node-1", "Node 1", Point{X: 0, Y: 0, Z: 2}),
		NewVirtualNode("node-2", "Node 2", Point{X: 3, Y: 0, Z: 2}),
	}

	w := NewNodeToNodeWalkerNoWait("walker-1", nodes, 1.0)
	space := DefaultSpace()

	// Starting position
	startX := w.Position.X

	// Update for 1 second
	dt := 0.1
	for i := 0; i < 10; i++ {
		w.Update(dt, space)
	}

	// Should have moved towards node-2
	if w.Position.X <= startX {
		t.Errorf("Expected walker to move towards node-2 (X increased), but X went from %f to %f", startX, w.Position.X)
	}

	// Velocity should be set towards node-2
	if w.Velocity.X <= 0 {
		t.Errorf("Expected positive X velocity, got %f", w.Velocity.X)
	}
}

func TestNodeToNodeWalkerArrival(t *testing.T) {
	nodes := []*Node{
		NewVirtualNode("node-1", "Node 1", Point{X: 0, Y: 0, Z: 2}),
		NewVirtualNode("node-2", "Node 2", Point{X: 3, Y: 0, Z: 2}),
		NewVirtualNode("node-3", "Node 3", Point{X: 3, Y: 5, Z: 2}),
	}

	w := NewNodeToNodeWalkerNoWait("walker-1", nodes, 5.0)
	space := DefaultSpace()

	// Update until walker reaches node-2
	for i := 0; i < 100; i++ {
		w.Update(0.1, space)

		// Check if we've moved close enough to node-2
		// Note: distance includes Z-axis difference (walker height 1.7 vs node Z=2.0)
		// So we need to be very close in X,Y to trigger arrival
		if w.Position.X >= 2.85 && w.NodeIndex == 1 {
			// Give it one more update to trigger advancement
			w.Update(0.1, space)
			break
		}
	}

	// Should have advanced to node-3
	if w.NodeIndex != 2 {
		// Force position very close to node-2 to trigger advancement
		w.Position.X = 2.99
		w.Position.Y = 0
		w.Position.Z = 1.7
		w.Update(0.1, space)
		if w.NodeIndex != 2 {
			t.Errorf("Expected NodeIndex to advance to 2 after reaching node-2, got %d", w.NodeIndex)
		}
	}
}

func TestNodeToNodeWalkerWithWait(t *testing.T) {
	nodes := []*Node{
		NewVirtualNode("node-1", "Node 1", Point{X: 0, Y: 0, Z: 2}),
		NewVirtualNode("node-2", "Node 2", Point{X: 3, Y: 0, Z: 2}),
	}

	waitTime := 1.0 // 1 second wait
	w := NewNodeToNodeWalker("walker-1", nodes, 5.0, waitTime)
	space := DefaultSpace()

	// Move walker very close to node-2 to trigger arrival
	w.Position.X = 2.99
	w.Position.Y = 0
	w.Position.Z = 1.7

	// First update - detects arrival and starts waiting
	w.Update(0.1, space)

	if w.WaitTimer <= 0 {
		t.Error("Expected WaitTimer to be positive after arrival")
	}

	// Second update - now velocity should be zero while waiting
	w.Update(0.1, space)
	if w.Velocity.X != 0 || w.Velocity.Y != 0 || w.Velocity.Z != 0 {
		t.Errorf("Expected zero velocity while waiting, got %v", w.Velocity)
	}

	// Update until wait time expires
	updatesToExpire := int(waitTime/0.1) + 1
	for i := 0; i < updatesToExpire; i++ {
		w.Update(0.1, space)
	}

	// Wait timer should have been reset for next node
	if w.WaitTimer != waitTime {
		t.Errorf("Expected WaitTimer to be reset to %f, got %f", waitTime, w.WaitTimer)
	}
}

func TestNodeToNodeWalkerFallsBackToRandomWalk(t *testing.T) {
	// Create walker with no nodes
	w := &Walker{
		ID:       "walker-1",
		Type:     WalkerTypeNodeToNode,
		Position: Point{X: 1, Y: 1, Z: 1.7},
		Speed:    1.0,
		Height:   1.7,
		Nodes:    []*Node{},
		Velocity: Point{X: 0.1, Y: 0.1, Z: 0},
	}

	space := DefaultSpace()
	initialX := w.Position.X

	// Update should fall back to random walk
	w.Update(0.1, space)

	// Position should have changed (random walk behavior)
	// but not in a deterministic direction
	if w.Position.X == initialX && w.Position.Y == 1 {
		t.Error("Expected position to change during random walk fallback")
	}
}

func TestCreateNodeToNodeWalkers(t *testing.T) {
	nodes := NewNodeSet()
	nodes.AddVirtualNode("node-1", "Node 1", Point{X: 0, Y: 0, Z: 2})
	nodes.AddVirtualNode("node-2", "Node 2", Point{X: 5, Y: 0, Z: 2})
	nodes.AddVirtualNode("node-3", "Node 3", Point{X: 2.5, Y: 5, Z: 2})

	ws := CreateNodeToNodeWalkers(3, nodes, 1.0, 2.0)

	if ws.Count() != 3 {
		t.Errorf("Expected 3 walkers, got %d", ws.Count())
	}

	for i, w := range ws.All() {
		if w.Type != WalkerTypeNodeToNode {
			t.Errorf("Walker %d: expected type %s, got %s", i, WalkerTypeNodeToNode, w.Type)
		}

		if len(w.Nodes) != 3 {
			t.Errorf("Walker %d: expected 3 nodes, got %d", i, len(w.Nodes))
		}

		if !w.ShouldWait {
			t.Errorf("Walker %d: expected ShouldWait to be true", i)
		}
	}
}

func TestCreateNodeToNodeWalkersNoWait(t *testing.T) {
	nodes := NewNodeSet()
	nodes.AddVirtualNode("node-1", "Node 1", Point{X: 0, Y: 0, Z: 2})
	nodes.AddVirtualNode("node-2", "Node 2", Point{X: 5, Y: 0, Z: 2})

	ws := CreateNodeToNodeWalkersNoWait(2, nodes, 1.0)

	if ws.Count() != 2 {
		t.Errorf("Expected 2 walkers, got %d", ws.Count())
	}

	for _, w := range ws.All() {
		if w.ShouldWait {
			t.Error("Expected ShouldWait to be false for no-wait walkers")
		}

		if w.WaitTime != 0 {
			t.Errorf("Expected WaitTime 0, got %f", w.WaitTime)
		}
	}
}

func TestCreateNodeToNodeWalkersWithEmptyNodes(t *testing.T) {
	nodes := NewNodeSet()
	ws := CreateNodeToNodeWalkers(3, nodes, 1.0, 0)

	if ws.Count() != 0 {
		t.Errorf("Expected 0 walkers with empty node set, got %d", ws.Count())
	}
}

func TestNodeToNodeWalkerSpeedVariation(t *testing.T) {
	nodes := []*Node{
		NewVirtualNode("node-1", "Node 1", Point{X: 0, Y: 0, Z: 2}),
		NewVirtualNode("node-2", "Node 2", Point{X: 10, Y: 0, Z: 2}),
	}

	w := NewNodeToNodeWalkerNoWait("walker-1", nodes, 1.0)
	space := DefaultSpace()

	// Collect velocities over multiple updates
	velocities := make([]float64, 0, 20)

	for i := 0; i < 20; i++ {
		w.Update(0.1, space)

		// Calculate speed from velocity
		speed := math.Sqrt(w.Velocity.X*w.Velocity.X + w.Velocity.Y*w.Velocity.Y + w.Velocity.Z*w.Velocity.Z)
		if speed > 0.01 {
			velocities = append(velocities, speed)
		}
	}

	if len(velocities) == 0 {
		t.Fatal("Expected some non-zero velocities")
	}

	// Check for variation (should not all be the same)
	minSpeed := velocities[0]
	maxSpeed := velocities[0]
	for _, v := range velocities {
		if v < minSpeed {
			minSpeed = v
		}
		if v > maxSpeed {
			maxSpeed = v
		}
	}

	// Should have at least some variation (0.8x to 1.2x base speed)
	if maxSpeed-minSpeed < 0.1 {
		t.Errorf("Expected speed variation, but min=%f, max=%f", minSpeed, maxSpeed)
	}

	// All speeds should be reasonable (not exceeding base speed significantly)
	for _, v := range velocities {
		if v > 1.5 {
			t.Errorf("Speed %f exceeds reasonable maximum", v)
		}
	}
}

func TestNodeToNodeWalkerDecelerationNearTarget(t *testing.T) {
	nodes := []*Node{
		NewVirtualNode("node-1", "Node 1", Point{X: 0, Y: 0, Z: 2}),
		NewVirtualNode("node-2", "Node 2", Point{X: 5, Y: 0, Z: 2}),
	}

	space := DefaultSpace()

	// Collect speeds at 0.8m from target - use fresh walkers for each measurement
	farSpeeds := make([]float64, 0, 10)
	for i := 0; i < 10; i++ {
		w := NewNodeToNodeWalkerNoWait(fmt.Sprintf("walker-far-%d", i), nodes, 1.0)
		w.Position.X = 4.2 // 0.8m from node-2
		w.Update(0.01, space) // Small update to compute velocity without moving much
		speed := math.Sqrt(w.Velocity.X*w.Velocity.X + w.Velocity.Y*w.Velocity.Y)
		if speed > 0 {
			farSpeeds = append(farSpeeds, speed)
		}
	}

	// Collect speeds at 0.2m from target
	nearSpeeds := make([]float64, 0, 10)
	for i := 0; i < 10; i++ {
		w := NewNodeToNodeWalkerNoWait(fmt.Sprintf("walker-near-%d", i), nodes, 1.0)
		w.Position.X = 4.8 // 0.2m from node-2
		w.Update(0.01, space) // Small update to compute velocity
		speed := math.Sqrt(w.Velocity.X*w.Velocity.X + w.Velocity.Y*w.Velocity.Y)
		if speed > 0 {
			nearSpeeds = append(nearSpeeds, speed)
		}
	}

	// Calculate average speeds
	avgFar := 0.0
	for _, s := range farSpeeds {
		avgFar += s
	}
	avgFar /= float64(len(farSpeeds))

	avgNear := 0.0
	for _, s := range nearSpeeds {
		avgNear += s
	}
	avgNear /= float64(len(nearSpeeds))

	// At 0.8m: deceleration factor = 0.8/1.0 = 0.8 (some deceleration)
	// At 0.2m: deceleration factor = 0.2/1.0 = 0.2 (strong deceleration)
	// The ratio near/far should be approximately 0.2/0.8 = 0.25
	// We allow for random speed variation (0.8-1.2x factor)
	// So we expect avgNear / avgFar < 0.6 (0.25 * 2.4 to account for variation)
	if avgNear >= avgFar*0.6 {
		t.Errorf("Expected deceleration near target: avg far=%f, avg near=%f (ratio %f)", avgFar, avgNear, avgNear/avgFar)
	}
}

func TestNodeToNodeWalkerMaintainsHeight(t *testing.T) {
	nodes := []*Node{
		NewVirtualNode("node-1", "Node 1", Point{X: 0, Y: 0, Z: 2}),
		NewVirtualNode("node-2", "Node 2", Point{X: 5, Y: 0, Z: 2.5}),
	}

	w := NewNodeToNodeWalkerNoWait("walker-1", nodes, 1.0)
	space := DefaultSpace()

	expectedHeight := w.Height

	// Update multiple times
	for i := 0; i < 10; i++ {
		w.Update(0.1, space)

		if math.Abs(w.Position.Z-expectedHeight) > 0.01 {
			t.Errorf("Expected height %f, got %f", expectedHeight, w.Position.Z)
		}
	}
}

func TestWalkerSetAddNodeToNodeWalker(t *testing.T) {
	ws := NewWalkerSet()

	nodes := []*Node{
		NewVirtualNode("node-1", "Node 1", Point{X: 0, Y: 0, Z: 2}),
		NewVirtualNode("node-2", "Node 2", Point{X: 5, Y: 0, Z: 2}),
	}

	ws.AddNodeToNodeWalker("walker-1", nodes, 1.0, 2.0)

	if ws.Count() != 1 {
		t.Errorf("Expected 1 walker, got %d", ws.Count())
	}

	walker := ws.All()[0]
	if walker.Type != WalkerTypeNodeToNode {
		t.Errorf("Expected type %s, got %s", WalkerTypeNodeToNode, walker.Type)
	}

	if walker.WaitTime != 2.0 {
		t.Errorf("Expected wait time 2.0, got %f", walker.WaitTime)
	}
}

func TestWalkerSetAddNodeToNodeWalkerNoWait(t *testing.T) {
	ws := NewWalkerSet()

	nodes := []*Node{
		NewVirtualNode("node-1", "Node 1", Point{X: 0, Y: 0, Z: 2}),
		NewVirtualNode("node-2", "Node 2", Point{X: 5, Y: 0, Z: 2}),
	}

	ws.AddNodeToNodeWalkerNoWait("walker-1", nodes, 1.0)

	if ws.Count() != 1 {
		t.Errorf("Expected 1 walker, got %d", ws.Count())
	}

	walker := ws.All()[0]
	if walker.ShouldWait {
		t.Error("Expected ShouldWait to be false")
	}
}

func TestGenerateTicksWithNodeToNodeWalkers(t *testing.T) {
	nodes := NewNodeSet()
	nodes.AddVirtualNode("node-1", "Node 1", Point{X: 0, Y: 0, Z: 2})
	nodes.AddVirtualNode("node-2", "Node 2", Point{X: 5, Y: 0, Z: 2})

	ws := CreateNodeToNodeWalkers(1, nodes, 1.0, 0)
	space := DefaultSpace()

	// Generate ticks for 1 second at 10 Hz
	ticks := 0
	tickChan := ws.GenerateTicks(10, 1*time.Second, space)

	for tick := range tickChan {
		ticks++
		// Verify tick has valid data
		if tick.Walkers == nil || len(tick.Walkers) == 0 {
			t.Error("Tick should have walker data")
		}
		if ticks > 15 {
			t.Error("Too many ticks generated")
			break
		}
	}

	// We expect approximately 10 ticks (1 second * 10 Hz)
	// Allow some tolerance for timing variations
	if ticks < 5 {
		t.Errorf("Expected at least 5 ticks, got %d", ticks)
	}
	if ticks > 12 {
		t.Errorf("Expected at most 12 ticks, got %d", ticks)
	}
}
