package ble

import (
	"log"
	"math"
	"reflect"
	"sync"
	"time"
)

// Triangulation parameters per specification
const (
	RefDistance    = 1.0  // d0 = 1.0m reference distance
	RefRSSI        = -65  // RSSI at 1m in typical indoor environment (dBm)
	PathLossExp    = 2.5  // Indoor path loss exponent (typical range 2.0-3.5)
	RSSINoiseSigma = 5.0  // RSSI noise sigma in dBm
)

// Matching thresholds per specification
const (
	MaxBLEBlobDistance    = 2.0  // Maximum distance for BLE-to-blob matching (metres)
	MinMatchConfidence    = 0.6  // Minimum confidence for identity assignment
	IdentityPersistence   = 5 * time.Minute
	ObservationWindow     = 5 * time.Second
	ObservationWindowLong = 15 * time.Second
)

// NodePositionAccessor provides node positions for triangulation.
type NodePositionAccessor interface {
	GetNodePosition(mac string) (x, y, z float64, ok bool)
}

// IdentityMatch represents a match between a blob and a device/person.
type IdentityMatch struct {
	BlobID            int       `json:"blob_id"`
	DeviceAddr        string    `json:"device_addr"`
	DeviceName        string    `json:"device_name,omitempty"`
	PersonID          string    `json:"person_id,omitempty"`
	PersonName        string    `json:"person_name,omitempty"`
	PersonColor       string    `json:"person_color,omitempty"`
	Confidence        float64   `json:"confidence"`
	TriangulationPos  Position  `json:"triangulation_pos"`
	TriangulationConf float64   `json:"triangulation_confidence"`
	Timestamp         time.Time `json:"timestamp"`
	IsBLEOnly         bool      `json:"is_ble_only"` // True if no CSI blob within range
}

// Position represents a 3D position.
type Position struct {
	X, Y, Z float64
}

// TriangulatedDevice represents a BLE device with its triangulated position.
type TriangulatedDevice struct {
	Device      *DeviceRecord
	Position    Position
	Confidence  float64
	Residual    float64 // Triangulation residual (quality indicator)
	NodeCount   int
	LastSeenAge time.Duration // Time since last RSSI observation
}

// IdentityMatcher matches BLE devices to CSI blobs using RSSI triangulation.
type IdentityMatcher struct {
	registry  *Registry
	rssiCache *RSSICache
	nodePos   NodePositionAccessor
	rotationDetector *RotationDetector // For address rotation detection

	mu              sync.RWMutex
	matches         map[int]*IdentityMatch // blobID -> match
	bleOnlyTracks   map[string]*IdentityMatch // personID -> BLE-only placeholder
	persistentIdent map[int]*IdentityMatch // blobID -> persisted identity (for 5-min persistence)
	lastBLEUpdate   time.Time
	cachedDevices   []*TriangulatedDevice

	matchTimeout    time.Duration
	persistenceTime time.Duration
}

// SetRotationDetector sets the rotation detector for alias resolution.
func (m *IdentityMatcher) SetRotationDetector(detector *RotationDetector) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rotationDetector = detector
}

// NewIdentityMatcher creates a new identity matcher.
func NewIdentityMatcher(registry *Registry, rssiCache *RSSICache, nodePos NodePositionAccessor) *IdentityMatcher {
	return &IdentityMatcher{
		registry:        registry,
		rssiCache:       rssiCache,
		nodePos:         nodePos,
		matches:         make(map[int]*IdentityMatch),
		bleOnlyTracks:   make(map[string]*IdentityMatch),
		persistentIdent: make(map[int]*IdentityMatch),
		matchTimeout:    30 * time.Second,
		persistenceTime: IdentityPersistence,
	}
}

// UpdateBlobs processes new blob positions and matches them to BLE devices.
// This implements the full BLE-to-blob matching algorithm per specification.
func (m *IdentityMatcher) UpdateBlobs(blobs []struct {
	ID     int
	X, Y, Z float64
	Weight float64
}) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()

	// Step 1: Decay old matches and handle persistence
	m.decayOldMatches(now)

	// Step 2: Triangulate BLE device positions (cached, updated every 5s)
	triangulated := m.getTriangulatedDevices(now)

	// Step 3: Clear current matches (but keep persistent identities)
	for blobID := range m.matches {
		if persist, ok := m.persistentIdent[blobID]; ok {
			// Keep persistent identity if still within persistence window
			if now.Sub(persist.Timestamp) < m.persistenceTime {
				continue
			}
		}
		delete(m.matches, blobID)
	}

	// Step 4: Assign BLE devices to nearest blobs
	m.assignBLEToBlobs(triangulated, blobs, now)

	// Step 5: Create BLE-only placeholder tracks for unmatched devices
	m.createBLEOnlyTracks(triangulated, blobs, now)
}

// getTriangulatedDevices returns cached triangulated device positions,
// refreshing the cache if stale (older than 5 seconds).
func (m *IdentityMatcher) getTriangulatedDevices(now time.Time) []*TriangulatedDevice {
	// Cache BLE positions between updates (BLE only updates at ~5s intervals)
	if now.Sub(m.lastBLEUpdate) < ObservationWindow && m.cachedDevices != nil {
		return m.cachedDevices
	}

	m.cachedDevices = m.triangulateAllDevices(now)
	m.lastBLEUpdate = now
	return m.cachedDevices
}

// triangulateAllDevices triangulates all person-assigned BLE devices,
// including any rotated address aliases.
func (m *IdentityMatcher) triangulateAllDevices(now time.Time) []*TriangulatedDevice {
	// Get devices including aliases - map of all addresses (canonical + aliases) to canonical device
	devicesMap, err := m.registry.GetAllPersonDevicesWithAliases()
	if err != nil {
		log.Printf("[WARN] ble: failed to get person devices with aliases: %v", err)
		return nil
	}

	// Track which canonical devices we've already processed
	processed := make(map[string]bool)

	var result []*TriangulatedDevice
	for addr, dev := range devicesMap {
		if !dev.Enabled {
			continue
		}

		// Skip if we already processed this canonical device
		if processed[dev.Addr] {
			// Update alias last_seen timestamp if this is an alias
			if addr != dev.Addr {
				m.registry.UpdateAliasLastSeen(addr)
			}
			continue
		}

		// Get recent RSSI readings for this address (could be canonical or alias)
		readings := m.rssiCache.GetRecent(addr, ObservationWindow)
		if len(readings) == 0 {
			// Check older window for persistence
			readings = m.rssiCache.GetRecent(addr, ObservationWindowLong)
			if len(readings) == 0 {
				continue
			}
		}

		// Update alias last_seen timestamp
		if addr != dev.Addr {
			m.registry.UpdateAliasLastSeen(addr)
		}

		// Find most recent observation age
		var lastSeenAge time.Duration
		for _, r := range readings {
			age := now.Sub(r.Timestamp)
			if age < lastSeenAge || lastSeenAge == 0 {
				lastSeenAge = age
			}
		}

		// Triangulate position
		pos, conf, residual := m.triangulate(readings)
		if conf < 0.1 {
			continue // Too low confidence
		}

		result = append(result, &TriangulatedDevice{
			Device:      dev,
			Position:    pos,
			Confidence:  conf,
			Residual:    residual,
			NodeCount:   len(readings),
			LastSeenAge: lastSeenAge,
		})

		processed[dev.Addr] = true
	}

	return result
}

// triangulate performs RSSI-based triangulation using weighted least squares.
// Returns position, confidence, and residual.
func (m *IdentityMatcher) triangulate(readings []*RSSIObservation) (Position, float64, float64) {
	if len(readings) < 1 {
		return Position{}, 0, 0
	}

	// Convert RSSI readings to distance estimates with node positions
	type nodeReading struct {
		x, y, z float64
		rssi    int
		distance float64
		weight   float64
	}

	var nodes []nodeReading
	for _, r := range readings {
		nx, ny, nz, ok := m.nodePos.GetNodePosition(r.NodeMAC)
		if !ok {
			continue
		}

		// RSSI to distance: d = d0 * 10^((RSSI_ref - RSSI) / (10 * n))
		distance := rssiToDistance(r.RSSIdBm)

		// Weight based on distance uncertainty (sigma = d * ln(10) / (10 * n) * RSSI_noise)
		sigma := distance * math.Ln10 / (10 * PathLossExp) * RSSINoiseSigma
		weight := 1.0 / (sigma * sigma)

		nodes = append(nodes, nodeReading{
			x: nx, y: ny, z: nz,
			rssi:     r.RSSIdBm,
			distance: distance,
			weight:   weight,
		})
	}

	if len(nodes) == 0 {
		return Position{}, 0, 0
	}

	// Handle single node case
	if len(nodes) == 1 {
		// Only range estimate - position is somewhere on a sphere around the node
		// Use node position as rough estimate with low confidence
		return Position{X: nodes[0].x, Y: nodes[0].y, Z: nodes[0].z}, 0.2, 0
	}

	// For 2+ nodes, perform weighted least squares triangulation
	// Initial guess: weighted centroid
	var sumWx, sumWy, sumWz, sumW float64
	for _, n := range nodes {
		sumWx += n.x * n.weight
		sumWy += n.y * n.weight
		sumWz += n.z * n.weight
		sumW += n.weight
	}
	if sumW == 0 {
		return Position{}, 0, 0
	}

	// Initial position
	px, py, pz := sumWx/sumW, sumWy/sumW, sumWz/sumW

	// Gauss-Newton iterations (typically 3-5 for convergence)
	const maxIter = 5
	const epsilon = 1e-6

	for iter := 0; iter < maxIter; iter++ {
		// Compute residual and Jacobian
		var gradX, gradY, gradZ float64
		var residual float64

		for _, n := range nodes {
			dx := px - n.x
			dy := py - n.y
			dz := pz - n.z
			predictedDist := math.Sqrt(dx*dx + dy*dy + dz*dz)

			if predictedDist < epsilon {
				predictedDist = epsilon
			}

			// Residual: predicted_distance - measured_distance
			r := predictedDist - n.distance
			residual += n.weight * r * r

			// Gradient of residual w.r.t. position
			gradX += n.weight * r * dx / predictedDist
			gradY += n.weight * r * dy / predictedDist
			gradZ += n.weight * r * dz / predictedDist
		}

		// Simple gradient descent step (step size ~0.5)
		step := 0.5
		px -= step * gradX
		py -= step * gradY
		pz -= step * gradZ

		// Check convergence
		if residual < 0.01 {
			break
		}
	}

	// Compute final residual for quality indicator
	var finalResidual float64
	for _, n := range nodes {
		dx := px - n.x
		dy := py - n.y
		dz := pz - n.z
		predictedDist := math.Sqrt(dx*dx + dy*dy + dz*dz)
		r := predictedDist - n.distance
		finalResidual += r * r
	}
	finalResidual = math.Sqrt(finalResidual / float64(len(nodes)))

	// Compute confidence based on node count
	confidence := 0.0
	switch len(nodes) {
	case 1:
		confidence = 0.2
	case 2:
		confidence = 0.5
	default:
		confidence = math.Min(1.0, 0.7+0.1*float64(len(nodes)-3))
	}

	// Reduce confidence if residual is high (poor fit)
	if finalResidual > 0.5 {
		confidence *= math.Max(0.3, 1.0-finalResidual/2.0)
	}

	return Position{X: px, Y: py, Z: pz}, confidence, finalResidual
}

// rssiToDistance converts RSSI to distance using the log-distance path loss model.
// d = d0 * 10^((RSSI_ref - RSSI) / (10 * n))
func rssiToDistance(rssi int) float64 {
	return RefDistance * math.Pow(10, float64(RefRSSI-rssi)/(10*PathLossExp))
}

// assignBLEToBlobs assigns triangulated BLE devices to the nearest CSI blobs.
func (m *IdentityMatcher) assignBLEToBlobs(devices []*TriangulatedDevice, blobs []struct {
	ID     int
	X, Y, Z float64
	Weight float64
}, now time.Time) {
	// Track which blobs have been assigned
	assignedBlobs := make(map[int]bool)

	// Track which persons have been assigned (to handle multiple devices per person)
	personAssigned := make(map[string]int) // personID -> blobID

	// Sort devices by confidence (highest first) for priority assignment
	sortedDevices := make([]*TriangulatedDevice, len(devices))
	copy(sortedDevices, devices)
	sortDevicesByConfidence(sortedDevices)

	for _, td := range sortedDevices {
		if td.Device.PersonID == "" {
			continue
		}

		// Find nearest blob within 2m (using horizontal plane only)
		var bestBlob *struct {
			ID     int
			X, Y, Z float64
			Weight float64
		}
		bestDist := MaxBLEBlobDistance

		for i := range blobs {
			b := &blobs[i]
			if assignedBlobs[b.ID] {
				continue
			}

			// Horizontal distance (ignore Y/height for BLE since antenna height is variable)
			hDist := math.Sqrt(math.Pow(td.Position.X-b.X, 2) + math.Pow(td.Position.Z-b.Z, 2))

			if hDist < bestDist {
				bestDist = hDist
				bestBlob = b
			}
		}

		if bestBlob == nil {
			continue
		}

		// Check if this person already has a blob assigned
		if existingBlobID, exists := personAssigned[td.Device.PersonID]; exists {
			// Keep the higher confidence assignment
			existingMatch := m.matches[existingBlobID]
			if existingMatch != nil && existingMatch.Confidence >= computeMatchConfidence(td, bestDist) {
				continue
			}
			// New assignment is better, remove old one
			delete(m.matches, existingBlobID)
			delete(assignedBlobs, existingBlobID)
		}

		// Compute match confidence
		matchConf := computeMatchConfidence(td, bestDist)
		if matchConf < MinMatchConfidence {
			continue
		}

		// Create the match
		deviceName := td.Device.Name
		if deviceName == "" {
			deviceName = td.Device.DeviceName
		}

		match := &IdentityMatch{
			BlobID:            bestBlob.ID,
			DeviceAddr:        td.Device.Addr,
			DeviceName:        deviceName,
			PersonID:          td.Device.PersonID,
			PersonName:        td.Device.PersonName,
			PersonColor:       getPersonColor(td.Device),
			Confidence:        matchConf,
			TriangulationPos:  td.Position,
			TriangulationConf: td.Confidence,
			Timestamp:         now,
			IsBLEOnly:         false,
		}

		m.matches[bestBlob.ID] = match
		m.persistentIdent[bestBlob.ID] = match
		assignedBlobs[bestBlob.ID] = true
		personAssigned[td.Device.PersonID] = bestBlob.ID

		// Update device's last known location
		if err := m.registry.UpdateLocation(td.Device.Addr, Location{
			X:          td.Position.X,
			Y:          td.Position.Y,
			Z:          td.Position.Z,
			Confidence: matchConf,
			Timestamp:  now,
		}); err != nil {
			log.Printf("[WARN] ble: failed to update location for %s: %v", td.Device.Addr, err)
		}
	}
}

// computeMatchConfidence computes the overall match confidence score.
// confidence = f_observations * f_node_count * f_residual * f_distance
func computeMatchConfidence(td *TriangulatedDevice, blobDist float64) float64 {
	// f_observations: 1.0 if seen in last 5s, 0.5 if last 15s, 0.0 if older
	fObs := 0.0
	if td.LastSeenAge <= ObservationWindow {
		fObs = 1.0
	} else if td.LastSeenAge <= ObservationWindowLong {
		fObs = 0.5
	}

	// f_node_count: 0.2 (1 node), 0.5 (2 nodes), 0.8+ (3+ nodes)
	fNodes := 0.0
	switch td.NodeCount {
	case 1:
		fNodes = 0.2
	case 2:
		fNodes = 0.5
	default:
		fNodes = math.Min(1.0, 0.8+0.05*float64(td.NodeCount-3))
	}

	// f_residual: 1.0 - min(1.0, residual/2.0)
	fResidual := math.Max(0, 1.0-math.Min(1.0, td.Residual/2.0))

	// f_distance: 1.0 if < 0.5m, linear decay to 0 at 2.0m
	fDist := 0.0
	if blobDist < 0.5 {
		fDist = 1.0
	} else if blobDist < MaxBLEBlobDistance {
		fDist = 1.0 - (blobDist-0.5)/(MaxBLEBlobDistance-0.5)
	}

	return fObs * fNodes * fResidual * fDist
}

// createBLEOnlyTracks creates placeholder tracks for BLE devices without nearby CSI blobs.
func (m *IdentityMatcher) createBLEOnlyTracks(devices []*TriangulatedDevice, blobs []struct {
	ID     int
	X, Y, Z float64
	Weight float64
}, now time.Time) {
	// Track which persons already have blob assignments
	hasBlobAssignment := make(map[string]bool)
	for _, match := range m.matches {
		if !match.IsBLEOnly {
			hasBlobAssignment[match.PersonID] = true
		}
	}

	for _, td := range devices {
		if td.Device.PersonID == "" {
			continue
		}

		// Skip if this person already has a blob assignment
		if hasBlobAssignment[td.Device.PersonID] {
			continue
		}

		// Check if there's any blob within 2m
		hasNearbyBlob := false
		for _, b := range blobs {
			hDist := math.Sqrt(math.Pow(td.Position.X-b.X, 2) + math.Pow(td.Position.Y-b.Y, 2))
			if hDist < MaxBLEBlobDistance {
				hasNearbyBlob = true
				break
			}
		}

		if hasNearbyBlob {
			continue // Will be handled by regular assignment
		}

		// Create BLE-only placeholder track
		matchConf := computeMatchConfidence(td, MaxBLEBlobDistance) // Use max distance for confidence

		deviceName := td.Device.Name
		if deviceName == "" {
			deviceName = td.Device.DeviceName
		}

		match := &IdentityMatch{
			BlobID:            -1, // No blob ID for BLE-only tracks
			DeviceAddr:        td.Device.Addr,
			DeviceName:        deviceName,
			PersonID:          td.Device.PersonID,
			PersonName:        td.Device.PersonName,
			PersonColor:       getPersonColor(td.Device),
			Confidence:        matchConf * 0.5, // Lower confidence for BLE-only
			TriangulationPos:  td.Position,
			TriangulationConf: td.Confidence,
			Timestamp:         now,
			IsBLEOnly:         true,
		}

		// Store by person ID (one BLE-only track per person)
		m.bleOnlyTracks[td.Device.PersonID] = match
	}
}

// decayOldMatches removes stale matches and handles identity persistence.
func (m *IdentityMatcher) decayOldMatches(now time.Time) {
	// Remove old matches
	for blobID, match := range m.matches {
		if now.Sub(match.Timestamp) > m.matchTimeout {
			delete(m.matches, blobID)
		}
	}

	// Remove expired persistent identities
	for blobID, match := range m.persistentIdent {
		if now.Sub(match.Timestamp) > m.persistenceTime {
			delete(m.persistentIdent, blobID)
		}
	}

	// Remove old BLE-only tracks
	for personID, match := range m.bleOnlyTracks {
		if now.Sub(match.Timestamp) > m.matchTimeout {
			delete(m.bleOnlyTracks, personID)
		}
	}
}

// GetMatches returns all current identity matches.
func (m *IdentityMatcher) GetMatches() map[int]*IdentityMatch {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[int]*IdentityMatch, len(m.matches))
	for k, v := range m.matches {
		result[k] = v
	}
	return result
}

// GetMatch returns the identity match for a specific blob.
func (m *IdentityMatcher) GetMatch(blobID int) *IdentityMatch {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.matches[blobID]
}

// GetBLEOnlyTracks returns BLE-only placeholder tracks.
func (m *IdentityMatcher) GetBLEOnlyTracks() map[string]*IdentityMatch {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*IdentityMatch, len(m.bleOnlyTracks))
	for k, v := range m.bleOnlyTracks {
		result[k] = v
	}
	return result
}

// GetAllMatches returns both blob matches and BLE-only tracks.
func (m *IdentityMatcher) GetAllMatches() []*IdentityMatch {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []*IdentityMatch
	for _, m := range m.matches {
		result = append(result, m)
	}
	for _, m := range m.bleOnlyTracks {
		result = append(result, m)
	}
	return result
}

// GetPersistentIdentity returns the persistent identity for a blob if it exists.
func (m *IdentityMatcher) GetPersistentIdentity(blobID int) *IdentityMatch {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.persistentIdent[blobID]
}

// sortDevicesByConfidence sorts devices by confidence in descending order.
func sortDevicesByConfidence(devices []*TriangulatedDevice) {
	for i := 0; i < len(devices); i++ {
		for j := i + 1; j < len(devices); j++ {
			if devices[j].Confidence > devices[i].Confidence {
				devices[i], devices[j] = devices[j], devices[i]
			}
		}
	}
}

// getPersonColor returns the person's color or a default.
func getPersonColor(device *DeviceRecord) string {
	// The color is stored in the people table, but we need to get it from somewhere
	// For now, return a default if not available
	if device.PersonID != "" {
		// Use a hash-based color if no specific color is set
		return defaultColorForPerson(device.PersonID)
	}
	return "#6b7280" // Gray default
}

// defaultColorForPerson generates a consistent color for a person ID.
func defaultColorForPerson(personID string) string {
	colors := []string{
		"#3b82f6", // Blue
		"#ef4444", // Red
		"#22c55e", // Green
		"#f59e0b", // Amber
		"#8b5cf6", // Purple
		"#ec4899", // Pink
		"#06b6d4", // Cyan
		"#f97316", // Orange
	}

	// Simple hash to pick a consistent color
	var hash int
	for _, c := range personID {
		hash += int(c)
	}
	return colors[hash%len(colors)]
}

// ForceMatch forces a specific blob to match a specific person (for manual override).
func (m *IdentityMatcher) ForceMatch(blobID int, personID, personName, personColor string, confidence float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	match := &IdentityMatch{
		BlobID:      blobID,
		PersonID:    personID,
		PersonName:  personName,
		PersonColor: personColor,
		Confidence:  confidence,
		Timestamp:   now,
		IsBLEOnly:   false,
	}
	m.matches[blobID] = match
	m.persistentIdent[blobID] = match
}

// ClearMatch removes a match for a specific blob.
func (m *IdentityMatcher) ClearMatch(blobID int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.matches, blobID)
	delete(m.persistentIdent, blobID)
}

// ProcessBLEObservations processes new BLE scan results through the rotation detector.
// Should be called whenever new BLE scan results arrive from nodes.
func (m *IdentityMatcher) ProcessBLEObservations(observations map[string][]*RSSIObservation) {
	if m.rotationDetector == nil {
		return // Rotation detector not set
	}

	m.rotationDetector.ProcessObservations(observations)
}

// GetRotationCandidates returns active rotation candidates.
func (m *IdentityMatcher) GetRotationCandidates() []*RotationCandidate {
	if m.rotationDetector == nil {
		return nil
	}

	return m.rotationDetector.GetCandidates()
}

// GetRotationHistory returns the rotation history for a canonical address.
func (m *IdentityMatcher) GetRotationHistory(canonicalAddr string) []string {
	if m.rotationDetector == nil {
		return nil
	}

	return m.rotationDetector.GetRotationHistory(canonicalAddr)
}

// ExtendGracePeriod extends the grace period for a device's identity.
// Use this when a device that was thought to have rotated is seen again.
func (m *IdentityMatcher) ExtendGracePeriod(canonicalAddr string) {
	if m.rotationDetector == nil {
		return
	}

	m.rotationDetector.ExtendGracePeriod(canonicalAddr)
}

// IsWithinGracePeriod returns true if the device's identity is within the grace period.
func (m *IdentityMatcher) IsWithinGracePeriod(canonicalAddr string) bool {
	if m.rotationDetector == nil {
		return false
	}

	return m.rotationDetector.IsWithinGracePeriod(canonicalAddr)
}

// EnrichBlobsWithIdentity adds identity information to a slice of blob pointers.
// This is used to enrich TrackedBlob from the fusion engine with BLE identity.
// The blobs slice should contain pointers to TrackedBlob structs that have
// PersonID, PersonLabel, PersonColor, IdentityConfidence, and IdentitySource fields.
func (m *IdentityMatcher) EnrichBlobsWithIdentity(blobs interface{}) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Use reflection to handle both *TrackedBlob and *tracking.Blob
	val := reflect.ValueOf(blobs)
	if val.Kind() != reflect.Ptr || val.IsNil() {
		return
	}

	slice := val.Elem()
	if slice.Kind() != reflect.Slice {
		return
	}

	now := time.Now()
	for i := 0; i < slice.Len(); i++ {
		blobElem := slice.Index(i)
		if blobElem.Kind() != reflect.Ptr || blobElem.IsNil() {
			continue
		}

		// Get the ID field
		idField := blobElem.Elem().FieldByName("ID")
		if !idField.IsValid() || idField.Kind() != reflect.Int {
			continue
		}
		blobID := int(idField.Int())

		// Try current match first
		var match *IdentityMatch
		if m.matches[blobID] != nil {
			match = m.matches[blobID]
			if now.Sub(match.Timestamp) >= m.persistenceTime {
				match = nil
			}
		}
		if match == nil && m.persistentIdent[blobID] != nil {
			match = m.persistentIdent[blobID]
			if now.Sub(match.Timestamp) >= m.persistenceTime {
				match = nil
			}
		}

		if match != nil && match.PersonID != "" {
			// Set identity fields on the blob
			if personIDField := blobElem.Elem().FieldByName("PersonID"); personIDField.IsValid() {
				personIDField.SetString(match.PersonID)
			}
			if personLabelField := blobElem.Elem().FieldByName("PersonLabel"); personLabelField.IsValid() {
				personLabelField.SetString(match.PersonName)
			}
			if personColorField := blobElem.Elem().FieldByName("PersonColor"); personColorField.IsValid() {
				personColorField.SetString(match.PersonColor)
			}
			if confField := blobElem.Elem().FieldByName("IdentityConfidence"); confField.IsValid() {
				confField.SetFloat(match.Confidence)
			}
			if sourceField := blobElem.Elem().FieldByName("IdentitySource"); sourceField.IsValid() {
				source := "ble_triangulation"
				if match.IsBLEOnly {
					source = "ble_only"
				}
				sourceField.SetString(source)
			}
		}
	}
}
