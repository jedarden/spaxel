// Package localization provides ground truth integration for self-improving localization
package localization

import (
	"log"
	"math"
	"sync"
	"time"
)

// GroundTruthSource represents a source of ground truth position data
type GroundTruthSource interface {
	// GetGroundTruth returns the known position of a tracked entity
	// Returns nil if no ground truth is available
	GetGroundTruth(entityID string) *GroundTruthPosition

	// GetAllGroundTruth returns all available ground truth positions
	GetAllGroundTruth() map[string]*GroundTruthPosition

	// Confidence returns the confidence level of this source (0-1)
	Confidence() float64
}

// Ensure BLEGroundTruthProvider implements GroundTruthSource
var _ GroundTruthSource = (*BLEGroundTruthProvider)(nil)

// GroundTruthPosition represents a known position from a ground truth source
type GroundTruthPosition struct {
	EntityID   string    `json:"entity_id"`
	X          float64   `json:"x"`            // Position X in metres
	Y          float64   `json:"y"`            // Position Y in metres
	Z          float64   `json:"z"`            // Position Z in metres
	Accuracy   float64   `json:"accuracy"`   // Position accuracy in metres (1σ)
	Timestamp  time.Time `json:"timestamp"`
	Source     string    `json:"source"`     // e.g., "ble", "manual"
	Confidence float64   `json:"confidence"` // Source confidence (0-1)
}

// BLETrilaterationConfig holds configuration for BLE-based trilateration
type BLETrilaterationConfig struct {
	// PathLossExponent is the n in RSSI = TX - 10*n*log10(d)
	// Typical: 2.0 (free space) to 4.0 (indoor with obstacles)
	PathLossExponent float64

	// ReferenceRSSI is the RSSI at 1 metre distance
	ReferenceRSSI float64

	// MinObservations is the minimum number of nodes needed for trilateration
	MinObservations int

	// MaxAge is the maximum age of RSSI observations to use
	MaxAge time.Duration

	// SmoothingAlpha is the EMA smoothing factor (0-1)
	SmoothingAlpha float64
}

// DefaultBLETrilaterationConfig returns sensible defaults
func DefaultBLETrilaterationConfig() BLETrilaterationConfig {
	return BLETrilaterationConfig{
		PathLossExponent: 2.5,
		ReferenceRSSI:    -59.0, // Typical for phones at 1m
		MinObservations:  3,
		MaxAge:           5 * time.Second,
		SmoothingAlpha:   0.3,
	}
}

// RSSIObservation is a single RSSI reading from a node
type RSSIObservation struct {
	NodeMAC   string    `json:"node_mac"`
	RSSIdBm   float64   `json:"rssi_dbm"`
	Timestamp time.Time `json:"timestamp"`
}

// BLEGroundTruthProvider uses BLE RSSI trilateration to provide ground truth positions
type BLEGroundTruthProvider struct {
	mu       sync.RWMutex
	config   BLETrilaterationConfig
	nodePos  map[string]NodePosition // Node MAC -> position

	// RSSI buffer: entityID -> nodeMAC -> observation
	observations map[string]map[string]*RSSIObservation

	// Smoothed positions: entityID -> last computed position
	smoothedPos map[string]*GroundTruthPosition

	// Distance estimate cache: nodeMAC -> entityID -> distance
	distanceCache map[string]map[string]float64
}

// NewBLEGroundTruthProvider creates a new BLE-based ground truth provider
func NewBLEGroundTruthProvider(config BLETrilaterationConfig) *BLEGroundTruthProvider {
	return &BLEGroundTruthProvider{
		config:        config,
		nodePos:       make(map[string]NodePosition),
		observations:  make(map[string]map[string]*RSSIObservation),
		smoothedPos:   make(map[string]*GroundTruthPosition),
		distanceCache: make(map[string]map[string]float64),
	}
}

// SetNodePosition updates a node's position
func (p *BLEGroundTruthProvider) SetNodePosition(mac string, x, y, z float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.nodePos[mac] = NodePosition{MAC: mac, X: x, Y: y, Z: z}
}

// RemoveNode removes a node
func (p *BLEGroundTruthProvider) RemoveNode(mac string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.nodePos, mac)
}

// AddObservation adds a new RSSI observation
func (p *BLEGroundTruthProvider) AddObservation(entityID, nodeMAC string, rssi float64, timestamp time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.observations[entityID] == nil {
		p.observations[entityID] = make(map[string]*RSSIObservation)
	}
	p.observations[entityID][nodeMAC] = &RSSIObservation{
		NodeMAC:   nodeMAC,
		RSSIdBm:   rssi,
		Timestamp: timestamp,
	}

	// Update distance cache
	distance := p.rssiToDistance(rssi)
	if p.distanceCache[nodeMAC] == nil {
		p.distanceCache[nodeMAC] = make(map[string]float64)
	}
	p.distanceCache[nodeMAC][entityID] = distance
}

// rssiToDistance converts RSSI to estimated distance using path loss model
func (p *BLEGroundTruthProvider) rssiToDistance(rssi float64) float64 {
	// d = 10^((TX - RSSI) / (10 * n))
	ratio := (p.config.ReferenceRSSI - rssi) / (10.0 * p.config.PathLossExponent)
	distance := math.Pow(10, ratio)

	// Clamp to reasonable range
	if distance < 0.1 {
		distance = 0.1
	}
	if distance > 30.0 {
		distance = 30.0
	}

	return distance
}

// GetGroundTruth returns the ground truth position for an entity
func (p *BLEGroundTruthProvider) GetGroundTruth(entityID string) *GroundTruthPosition {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Get recent observations
	obs := p.getRecentObservationsLocked(entityID)
	if len(obs) < p.config.MinObservations {
		return nil
	}

	// Trilaterate
	pos := p.trilaterateLocked(entityID, obs)
	if pos == nil {
		return nil
	}

	// Apply smoothing
	return p.smoothPositionLocked(entityID, pos)
}

// GetAllGroundTruth returns all available ground truth positions
func (p *BLEGroundTruthProvider) GetAllGroundTruth() map[string]*GroundTruthPosition {
	p.mu.RLock()
	defer p.mu.RUnlock()

	result := make(map[string]*GroundTruthPosition)
	for entityID := range p.observations {
		obs := p.getRecentObservationsLocked(entityID)
		if len(obs) < p.config.MinObservations {
			continue
		}

		pos := p.trilaterateLocked(entityID, obs)
		if pos == nil {
			continue
		}

		smoothed := p.smoothPositionLocked(entityID, pos)
		if smoothed != nil {
			result[entityID] = smoothed
		}
	}

	return result
}

// getRecentObservationsLocked returns recent observations for an entity
// Must be called with lock held
func (p *BLEGroundTruthProvider) getRecentObservationsLocked(entityID string) []*RSSIObservation {
	entityObs, ok := p.observations[entityID]
	if !ok {
		return nil
	}

	now := time.Now()
	var recent []*RSSIObservation

	for _, obs := range entityObs {
		if now.Sub(obs.Timestamp) <= p.config.MaxAge {
			recent = append(recent, obs)
		}
	}

	return recent
}

// trilaterateLocked computes position using nonlinear least squares
// Must be called with lock held
func (p *BLEGroundTruthProvider) trilaterateLocked(entityID string, observations []*RSSIObservation) *GroundTruthPosition {
	if len(observations) < p.config.MinObservations {
		return nil
	}

	// Build distance constraints
	type constraint struct {
		nodePos   NodePosition
		distance  float64
		weight    float64
	}

	var constraints []constraint
	for _, obs := range observations {
		nodePos, ok := p.nodePos[obs.NodeMAC]
		if !ok {
			continue
		}

		distance := p.rssiToDistance(obs.RSSIdBm)

		// Weight by recency and signal strength
		age := time.Since(obs.Timestamp).Seconds()
		recencyWeight := math.Exp(-age / p.config.MaxAge.Seconds())
		signalWeight := math.Min(1.0, math.Max(0.1, (obs.RSSIdBm+100)/70.0))
		weight := recencyWeight * signalWeight

		constraints = append(constraints, constraint{
			nodePos:  nodePos,
			distance: distance,
			weight:   weight,
		})
	}

	if len(constraints) < p.config.MinObservations {
		return nil
	}

	// Initial guess: centroid of all nodes weighted by inverse distance
	var gx, gy, gz, gweight float64
	for _, c := range constraints {
		w := c.weight / math.Max(c.distance, 0.1)
		gx += c.nodePos.X * w
		gy += c.nodePos.Y * w
		gz += c.nodePos.Z * w
		gweight += w
	}
	if gweight < 0.001 {
		return nil
	}
	gx /= gweight
	gy /= gweight
	gz /= gweight

	// Gauss-Newton iteration
	x, y, z := gx, gy, gz
	const maxIter = 20
	const tolerance = 0.001

	for iter := 0; iter < maxIter; iter++ {
		// Build Jacobian and residual
		var jacobian [][3]float64   // n x 3
		var residuals []float64     // n x 1
		var weights []float64

		for _, c := range constraints {
			dx := x - c.nodePos.X
			dy := y - c.nodePos.Y
			dz := z - c.nodePos.Z
			predDist := math.Sqrt(dx*dx + dy*dy + dz*dz)

			if predDist < 0.01 {
				predDist = 0.01
			}

			// Residual: predicted - measured
			residual := predDist - c.distance

			// Jacobian row: d(distance)/dx, d(distance)/dy, d(distance)/dz
			jacobian = append(jacobian, [3]float64{
				dx / predDist,
				dy / predDist,
				dz / predDist,
			})
			residuals = append(residuals, residual)
			weights = append(weights, c.weight)
		}

		// Weighted least squares: (J^T W J) delta = J^T W r
		// Solve for delta using normal equations
		var JTJ [3][3]float64
		var JTr [3]float64

		for i, row := range jacobian {
			w := weights[i]
			r := residuals[i] * w
			for j := 0; j < 3; j++ {
				for k := 0; k < 3; k++ {
					JTJ[j][k] += row[j] * row[k] * w * w
				}
				JTr[j] += row[j] * r * w
			}
		}

		// Solve 3x3 system using Cramer's rule (simple and stable for 3x3)
		det := JTJ[0][0]*(JTJ[1][1]*JTJ[2][2]-JTJ[1][2]*JTJ[2][1]) -
			JTJ[0][1]*(JTJ[1][0]*JTJ[2][2]-JTJ[1][2]*JTJ[2][0]) +
			JTJ[0][2]*(JTJ[1][0]*JTJ[2][1]-JTJ[1][1]*JTJ[2][0])

		if math.Abs(det) < 1e-10 {
			break // Singular matrix
		}

		dx := (JTr[0]*(JTJ[1][1]*JTJ[2][2]-JTJ[1][2]*JTJ[2][1]) -
			JTr[1]*(JTJ[0][1]*JTJ[2][2]-JTJ[0][2]*JTJ[2][1]) +
			JTr[2]*(JTJ[0][1]*JTJ[1][2]-JTJ[0][2]*JTJ[1][1])) / det

		dy := (JTJ[0][0]*(JTr[1]*JTJ[2][2]-JTJ[1][2]*JTr[2]) -
			JTJ[0][1]*(JTr[0]*JTJ[2][2]-JTJ[0][2]*JTr[2]) +
			JTJ[0][2]*(JTr[0]*JTJ[1][2]-JTr[1]*JTJ[0][2])) / det

		dz := (JTJ[0][0]*(JTJ[1][1]*JTr[2]-JTr[1]*JTJ[2][1]) -
			JTJ[0][1]*(JTJ[1][0]*JTr[2]-JTr[0]*JTJ[2][1]) +
			JTJ[0][2]*(JTJ[1][0]*JTr[1]-JTr[0]*JTJ[1][1])) / det

		// Damping factor for stability
		damping := 0.5
		x += damping * dx
		y += damping * dy
		z += damping * dz

		// Check convergence
		if dx*dx+dy*dy+dz*dz < tolerance*tolerance {
			break
		}
	}

	// Compute residual error as accuracy estimate
	var totalError float64
	var totalWeight float64
	for _, c := range constraints {
		dx := x - c.nodePos.X
		dy := y - c.nodePos.Y
		dz := z - c.nodePos.Z
		predDist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		error := math.Abs(predDist - c.distance)
		totalError += error * c.weight
		totalWeight += c.weight
	}

	accuracy := 1.0 // Default 1m accuracy
	if totalWeight > 0 {
		accuracy = totalError / totalWeight
		// Scale accuracy by number of observations (more = better)
		accuracy *= math.Sqrt(float64(p.config.MinObservations) / float64(len(constraints)))
	}

	// Compute confidence based on observation count and spread
	confidence := math.Min(1.0, float64(len(constraints))/5.0)

	return &GroundTruthPosition{
		EntityID:   entityID,
		X:          x,
		Y:          y,
		Z:          z,
		Accuracy:   accuracy,
		Timestamp:  time.Now(),
		Source:     "ble",
		Confidence: confidence,
	}
}

// smoothPositionLocked applies EMA smoothing to position estimates
// Must be called with lock held
func (p *BLEGroundTruthProvider) smoothPositionLocked(entityID string, pos *GroundTruthPosition) *GroundTruthPosition {
	prev, ok := p.smoothedPos[entityID]
	if !ok {
		p.smoothedPos[entityID] = pos
		return pos
	}

	alpha := p.config.SmoothingAlpha
	smoothed := &GroundTruthPosition{
		EntityID:   entityID,
		X:          alpha*pos.X + (1-alpha)*prev.X,
		Y:          alpha*pos.Y + (1-alpha)*prev.Y,
		Z:          alpha*pos.Z + (1-alpha)*prev.Z,
		Accuracy:   alpha*pos.Accuracy + (1-alpha)*prev.Accuracy,
		Timestamp:  pos.Timestamp,
		Source:     "ble",
		Confidence: alpha*pos.Confidence + (1-alpha)*prev.Confidence,
	}

	p.smoothedPos[entityID] = smoothed
	return smoothed
}

// Confidence returns the overall confidence of BLE ground truth
func (p *BLEGroundTruthProvider) Confidence() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Confidence depends on having enough nodes with known positions
	nodeCount := len(p.nodePos)
	if nodeCount < 3 {
		return 0.2 // Low confidence without enough nodes
	}
	if nodeCount < 5 {
		return 0.5
	}
	return 0.8
}

// Prune removes stale observations
func (p *BLEGroundTruthProvider) Prune() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	for entityID, entityObs := range p.observations {
		for nodeMAC, obs := range entityObs {
			if now.Sub(obs.Timestamp) > p.config.MaxAge*2 {
				delete(entityObs, nodeMAC)
			}
		}
		if len(entityObs) == 0 {
			delete(p.observations, entityID)
			delete(p.smoothedPos, entityID)
		}
	}
}

// GetObservationCount returns the number of active observations
func (p *BLEGroundTruthProvider) GetObservationCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()

	count := 0
	for _, entityObs := range p.observations {
		count += len(entityObs)
	}
	return count
}

// GetDistanceEstimate returns the cached distance estimate for a node-entity pair
func (p *BLEGroundTruthProvider) GetDistanceEstimate(nodeMAC, entityID string) (float64, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if nodeCache, ok := p.distanceCache[nodeMAC]; ok {
		if dist, ok := nodeCache[entityID]; ok {
			return dist, true
		}
	}
	return 0, false
}

// RegisterMetrics registers Prometheus metrics for monitoring
func (p *BLEGroundTruthProvider) RegisterMetrics() {
	// Metrics registration would go here if using Prometheus
	// For now, just log status periodically
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		for range ticker.C {
			p.mu.RLock()
			entityCount := len(p.observations)
			nodeCount := len(p.nodePos)
			p.mu.RUnlock()

			if entityCount > 0 {
				log.Printf("[DEBUG] BLE ground truth: %d entities tracked, %d nodes",
					entityCount, nodeCount)
			}
		}
	}()
}
