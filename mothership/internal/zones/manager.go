// Package zones provides room zones, portal, and occupancy management.
package zones

import (
	"database/sql"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// OccupancyStatus represents the confidence state of a zone's occupancy count.
type OccupancyStatus string

const (
	OccupancyUncertain  OccupancyStatus = "uncertain"  // Restored from persisted data, not yet verified
	OccupancyReconciled OccupancyStatus = "reconciled" // Verified against live blob counts
)

// ZoneType represents the type of zone for behavior customization.
type ZoneType string

const (
	ZoneTypeNormal    ZoneType = "normal"     // Default zone
	ZoneTypeBedroom   ZoneType = "bedroom"    // Enables sleep monitoring
	ZoneTypeKitchen   ZoneType = "kitchen"    // No special behavior
	ZoneTypeChildren  ZoneType = "children"   // Suppresses fall detection
)

// Zone represents a spatial region in the room.
type Zone struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Color          string    `json:"color"` // Hex color for visualization
	MinX float64 `json:"min_x"`
	MinY float64 `json:"min_y"`
	MinZ float64 `json:"min_z"`
	MaxX float64 `json:"max_x"`
	MaxY float64 `json:"max_y"`
	MaxZ float64 `json:"max_z"`
	Enabled        bool      `json:"enabled"`
	ZoneType       ZoneType  `json:"zone_type"`        // Zone type for behavior customization
	IsChildrenZone bool      `json:"is_children_zone"` // Suppresses fall detection in this zone (deprecated, use ZoneType)
	CreatedAt      time.Time `json:"created_at"`
}

// Portal represents a doorway/transition plane between zones.
type Portal struct {
	ID           string    `json:"id"`
	Name         string    `json:"name"`
	ZoneAID      string    `json:"zone_a_id"`
	ZoneBID      string    `json:"zone_b_id"`
	// Portal plane definition (3 points defining the doorway plane)
	P1X float64 `json:"p1_x"`
	P1Y float64 `json:"p1_y"`
	P1Z float64 `json:"p1_z"`
	P2X float64 `json:"p2_x"`
	P2Y float64 `json:"p2_y"`
	P2Z float64 `json:"p2_z"`
	P3X float64 `json:"p3_x"`
	P3Y float64 `json:"p3_y"`
	P3Z float64 `json:"p3_z"`
	// Portal normal vector (computed from points)
	NX float64 `json:"n_x"`
	NY float64 `json:"n_y"`
	NZ float64 `json:"n_z"`
	Width       float64   `json:"width"` // Portal width in meters
	Height      float64   `json:"height"` // Portal height in meters
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at"`
}

// CrossingEvent represents a detected portal crossing.
type CrossingEvent struct {
	PortalID    string    `json:"portal_id"`
	BlobID      int       `json:"blob_id"`
	Direction   int       `json:"direction"` // 1 = A->B, -1 = B->A
	FromZone    string    `json:"from_zone"`
	ToZone      string    `json:"to_zone"`
	Timestamp   time.Time `json:"timestamp"`
	Identity    string    `json:"identity,omitempty"` // Device name if matched
}

// ZoneTransitionEvent represents a blob entering or leaving a zone.
type ZoneTransitionEvent struct {
	BlobID    int       `json:"blob_id"`
	ZoneID    string    `json:"zone_id"`
	ZoneName  string    `json:"zone_name"`
	Kind      string    `json:"kind"` // "zone_entry" or "zone_exit"
	Timestamp time.Time `json:"timestamp"`
}

// ZoneOccupancy tracks current occupancy per zone.
type ZoneOccupancy struct {
	ZoneID      string          `json:"zone_id"`
	Count       int             `json:"count"`
	BlobIDs     []int           `json:"blob_ids"`
	LastUpdated time.Time       `json:"last_updated"`
	Status      OccupancyStatus `json:"status"` // uncertain or reconciled
}

// Manager handles zones, portals, and occupancy.
type Manager struct {
	mu        sync.RWMutex
	db        *sql.DB
	zones    map[string]*Zone
	portals  map[string]*Portal

	// Occupancy tracking
	occupancy     map[string]*ZoneOccupancy
	blobPositions map[int]struct {
		X, Y, Z     float64
		ZoneID      string
		LastUpdated time.Time
	}

	// Crossing detection state
	blobSide      map[int]float64 // blobID -> which side of portal (>0 = A side, <0 = B side)

	// Reconciliation state
	startedAt    time.Time // time this session started
	reconciled   bool      // whether initial reconciliation is complete
	reconChecks  int       // consecutive checks where portal vs blob counts agree
	reconDiscrep int       // consecutive checks where they disagree
	tz           *time.Location

	// Callbacks
	onCrossing  func(CrossingEvent)
	onZoneEntry  func(ZoneTransitionEvent)
	onZoneExit   func(ZoneTransitionEvent)
}

// NewManager creates a new zones manager. If tz is nil, UTC is used.
func NewManager(dbPath string, tz *time.Location) (*Manager, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	if tz == nil {
		tz = time.UTC
	}

	m := &Manager{
		db:            db,
		zones:         make(map[string]*Zone),
		portals:       make(map[string]*Portal),
		occupancy:     make(map[string]*ZoneOccupancy),
		blobPositions: make(map[int]struct {
			X, Y, Z     float64
			ZoneID      string
			LastUpdated time.Time
		}),
		blobSide:    make(map[int]float64),
		startedAt:   time.Now(),
		reconciled:  false,
		tz:          tz,
	}

	if err := m.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	// Load zones and portals into memory
	if err := m.loadZones(); err != nil {
		log.Printf("[WARN] Failed to load zones: %v", err)
	}
	if err := m.loadPortals(); err != nil {
		log.Printf("[WARN] Failed to load portals: %v", err)
	}

	// Reconcile occupancy from persisted data + portal crossings since midnight
	m.reconcileOccupancy()

	return m, nil
}

func (m *Manager) migrate() error {
	_, err := m.db.Exec(`
		CREATE TABLE IF NOT EXISTS zones (
			id        TEXT PRIMARY KEY,
			name     TEXT    NOT NULL DEFAULT '',
			color    TEXT    NOT NULL DEFAULT '#4fc3f7',
			min_x     REAL    NOT NULL DEFAULT 0,
			min_y     REAL    NOT NULL DEFAULT 0,
			min_z     REAL    NOT NULL DEFAULT 0,
			max_x     REAL    NOT NULL DEFAULT 1,
			max_y     REAL    NOT NULL DEFAULT 1,
			max_z     REAL    NOT NULL DEFAULT 1,
			enabled  INTEGER NOT NULL DEFAULT 1,
			zone_type TEXT   NOT NULL DEFAULT 'normal',
			is_children_zone INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS portals (
			id        TEXT PRIMARY KEY,
			name     TEXT    NOT NULL DEFAULT '',
			zone_a_id TEXT    NOT NULL DEFAULT '',
			zone_b_id TEXT    NOT NULL DEFAULT '',
			p1_x      REAL    NOT NULL DEFAULT 0,
			p1_y      REAL    NOT NULL DEFAULT 0,
			p1_z      REAL    NOT NULL DEFAULT 0,
			p2_x      REAL    NOT NULL DEFAULT 0,
			p2_y      REAL    NOT NULL DEFAULT 0,
			p2_z      REAL    NOT NULL DEFAULT 0,
			p3_x      REAL    NOT NULL DEFAULT 0,
			p3_y      REAL    NOT NULL DEFAULT 0,
			p3_z      REAL    NOT NULL DEFAULT 0,
			n_x      REAL    NOT NULL DEFAULT 0,
			n_y      REAL    NOT NULL DEFAULT 0,
			n_z      REAL    NOT NULL DEFAULT 0,
			width    REAL    NOT NULL DEFAULT 1,
			height   REAL    NOT NULL DEFAULT 2,
			enabled  INTEGER NOT NULL DEFAULT 1,
			created_at INTEGER NOT NULL DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS crossing_events (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			portal_id   TEXT    NOT NULL,
			blob_id     INTEGER NOT NULL,
			direction  INTEGER NOT NULL,
			from_zone  TEXT    NOT NULL,
			to_zone    TEXT    NOT NULL,
			timestamp  INTEGER NOT NULL,
			identity   TEXT    DEFAULT ''
		);

		CREATE INDEX IF NOT EXISTS idx_crossing_time ON crossing_events(timestamp);
	`)
	if err != nil {
		return err
	}

	// Add zone_type column if it doesn't exist (migration for existing databases)
	m.db.Exec(`ALTER TABLE zones ADD COLUMN zone_type TEXT NOT NULL DEFAULT 'normal'`)
	m.db.Exec(`ALTER TABLE zones ADD COLUMN is_children_zone INTEGER NOT NULL DEFAULT 0`)

	// Add last_known_occupancy column for restart reconciliation
	m.db.Exec(`ALTER TABLE zones ADD COLUMN last_known_occupancy INTEGER NOT NULL DEFAULT 0`)
	m.db.Exec(`ALTER TABLE zones ADD COLUMN occupancy_updated_at INTEGER`)

	return nil
}

func (m *Manager) loadZones() error {
	rows, err := m.db.Query(`SELECT id, name, color, min_x, min_y, min_z, max_x, max_y, max_z, enabled, zone_type, is_children_zone, created_at FROM zones`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var enabled, isChildrenZone int
		var createdAt int64
		var zoneType string
		z := &Zone{}
		if err := rows.Scan(&z.ID, &z.Name, &z.Color, &z.MinX, &z.MinY, &z.MinZ, &z.MaxX, &z.MaxY, &z.MaxZ, &enabled, &zoneType, &isChildrenZone, &createdAt); err != nil {
			log.Printf("[WARN] Failed to scan zone: %v", err)
			continue
		}
		z.Enabled = enabled != 0
		z.ZoneType = ZoneType(zoneType)
		if z.ZoneType == "" {
			z.ZoneType = ZoneTypeNormal
		}
		// Backward compatibility: if zone_type is children, set IsChildrenZone
		if z.ZoneType == ZoneTypeChildren {
			z.IsChildrenZone = true
		} else {
			z.IsChildrenZone = isChildrenZone != 0
		}
		if createdAt > 0 {
			z.CreatedAt = time.Unix(0, createdAt)
		}
		m.zones[z.ID] = z
	}
	return rows.Err()
}

func (m *Manager) loadPortals() error {
	rows, err := m.db.Query(`SELECT id, name, zone_a_id, zone_b_id, p1_x, p1_y, p1_z, p2_x, p2_y, p2_z, p3_x, p3_y, p3_z, n_x, n_y, n_z, width, height, enabled, created_at FROM portals`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var enabled int
		var createdAt int64
		p := &Portal{}
		if err := rows.Scan(&p.ID, &p.Name, &p.ZoneAID, &p.ZoneBID, &p.P1X, &p.P1Y, &p.P1Z, &p.P2X, &p.P2Y, &p.P2Z, &p.P3X, &p.P3Y, &p.P3Z, &p.NX, &p.NY, &p.NZ, &p.Width, &p.Height, &enabled, &createdAt); err != nil {
			log.Printf("[WARN] Failed to scan portal: %v", err)
			continue
		}
		p.Enabled = enabled != 0
		if createdAt > 0 {
			p.CreatedAt = time.Unix(0, createdAt)
		}
		m.portals[p.ID] = p
	}
	return rows.Err()
}

// Close closes the database.
func (m *Manager) Close() error {
	return m.db.Close()
}

// SetOnCrossing sets the callback for crossing events.
func (m *Manager) SetOnCrossing(cb func(CrossingEvent)) {
	m.mu.Lock()
	m.onCrossing = cb
	m.mu.Unlock()
}

// SetOnZoneEntry sets the callback for zone entry events.
func (m *Manager) SetOnZoneEntry(cb func(ZoneTransitionEvent)) {
	m.mu.Lock()
	m.onZoneEntry = cb
	m.mu.Unlock()
}

// SetOnZoneExit sets the callback for zone exit events.
func (m *Manager) SetOnZoneExit(cb func(ZoneTransitionEvent)) {
	m.mu.Lock()
	m.onZoneExit = cb
	m.mu.Unlock()
}

// CreateZone creates a new zone.
func (m *Manager) CreateZone(zone *Zone) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Set default zone type if not specified
	if zone.ZoneType == "" {
		zone.ZoneType = ZoneTypeNormal
	}

	now := time.Now().UnixNano()
	_, err := m.db.Exec(`
		INSERT INTO zones (id, name, color, min_x, min_y, min_z, max_x, max_y, max_z, enabled, zone_type, is_children_zone, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, zone.ID, zone.Name, zone.Color, zone.MinX, zone.MinY, zone.MinZ, zone.MaxX, zone.MaxY, zone.MaxZ, zone.Enabled, string(zone.ZoneType), zone.IsChildrenZone, now)
	if err != nil {
		return err
	}

	zone.CreatedAt = time.Unix(0, now)
	m.zones[zone.ID] = zone
	return nil
}

// UpdateZone updates an existing zone.
func (m *Manager) UpdateZone(zone *Zone) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Set default zone type if not specified
	if zone.ZoneType == "" {
		zone.ZoneType = ZoneTypeNormal
	}

	_, err := m.db.Exec(`
		UPDATE zones SET name=?, color=?, min_x=?, min_y=?, min_z=?, max_x=?, max_y=?, max_z=?, enabled=?, zone_type=?, is_children_zone=?
		WHERE id=?
	`, zone.Name, zone.Color, zone.MinX, zone.MinY, zone.MinZ, zone.MaxX, zone.MaxY, zone.MaxZ, zone.Enabled, string(zone.ZoneType), zone.IsChildrenZone, zone.ID)
	if err != nil {
		return err
	}

	m.zones[zone.ID] = zone
	return nil
}

// DeleteZone deletes a zone.
func (m *Manager) DeleteZone(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`DELETE FROM zones WHERE id=?`, id)
	if err != nil {
		return err
	}

	delete(m.zones, id)
	delete(m.occupancy, id)
	return nil
}

// GetZone returns a zone by ID.
func (m *Manager) GetZone(id string) *Zone {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.zones[id]
}

// GetAllZones returns all zones.
func (m *Manager) GetAllZones() []*Zone {
	m.mu.RLock()
	defer m.mu.RUnlock()

	zones := make([]*Zone, 0, len(m.zones))
	for _, z := range m.zones {
		zones = append(zones, z)
	}
	return zones
}

// CreatePortal creates a new portal.
func (m *Manager) CreatePortal(portal *Portal) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Compute normal vector from three points
	portal.NX, portal.NY, portal.NZ = computeNormal(portal)

	now := time.Now().UnixNano()
	_, err := m.db.Exec(`
		INSERT INTO portals (id, name, zone_a_id, zone_b_id, p1_x, p1_y, p1_z, p2_x, p2_y, p2_z, p3_x, p3_y, p3_z, n_x, n_y, n_z, width, height, enabled, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, portal.ID, portal.Name, portal.ZoneAID, portal.ZoneBID,
		portal.P1X, portal.P1Y, portal.P1Z, portal.P2X, portal.P2Y, portal.P2Z,
		portal.P3X, portal.P3Y, portal.P3Z, portal.NX, portal.NY, portal.NZ,
		portal.Width, portal.Height, portal.Enabled, now)
	if err != nil {
		return err
	}

	portal.CreatedAt = time.Unix(0, now)
	m.portals[portal.ID] = portal
	return nil
}

// UpdatePortal updates an existing portal.
func (m *Manager) UpdatePortal(portal *Portal) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Compute normal vector from three points
	portal.NX, portal.NY, portal.NZ = computeNormal(portal)

	_, err := m.db.Exec(`
		UPDATE portals SET name=?, zone_a_id=?, zone_b_id=?, p1_x=?, p1_y=?, p1_z=?, p2_x=?, p2_y=?, p2_z=?, p3_x=?, p3_y=?, p3_z=?, n_x=?, n_y=?, n_z=?, width=?, height=?, enabled=?
		WHERE id=?
	`, portal.Name, portal.ZoneAID, portal.ZoneBID,
		portal.P1X, portal.P1Y, portal.P1Z, portal.P2X, portal.P2Y, portal.P2Z,
		portal.P3X, portal.P3Y, portal.P3Z, portal.NX, portal.NY, portal.NZ,
		portal.Width, portal.Height, portal.Enabled, portal.ID)
	if err != nil {
		return err
	}

	m.portals[portal.ID] = portal
	return nil
}

// DeletePortal deletes a portal.
func (m *Manager) DeletePortal(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, err := m.db.Exec(`DELETE FROM portals WHERE id=?`, id)
	if err != nil {
		return err
	}

	delete(m.portals, id)
	return nil
}

// GetPortal returns a portal by ID.
func (m *Manager) GetPortal(id string) *Portal {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.portals[id]
}

// GetAllPortals returns all portals.
func (m *Manager) GetAllPortals() []*Portal {
	m.mu.RLock()
	defer m.mu.RUnlock()

	portals := make([]*Portal, 0, len(m.portals))
	for _, p := range m.portals {
		portals = append(portals, p)
	}
	return portals
}

// pendingCrossing holds a crossing event and associated zone transition events to fire after lock release.
type pendingCrossing struct {
	crossing CrossingEvent
	exit     *ZoneTransitionEvent
	entry    *ZoneTransitionEvent
}

// UpdateBlobPositions updates blob positions and detects portal crossings.
// Callbacks are fired synchronously after the lock is released to avoid deadlock.
func (m *Manager) UpdateBlobPositions(blobs []struct {
	ID     int
	X, Y, Z float64
}) {
	now := time.Now()

	// Collect pending events while holding the lock.
	var pending []pendingCrossing

	m.mu.Lock()

	for _, blob := range blobs {
		// Get previous position
		prev, existed := m.blobPositions[blob.ID]

		// Determine which zone the blob is in
		zoneID := m.findZoneForPosition(blob.X, blob.Y, blob.Z)

		// Update position
		m.blobPositions[blob.ID] = struct {
			X, Y, Z     float64
			ZoneID      string
			LastUpdated time.Time
		}{blob.X, blob.Y, blob.Z, zoneID, now}

		if existed && prev.ZoneID != zoneID {
			// Remove blob from old zone occupancy
			if prev.ZoneID != "" {
				m.removeFromOccupancy(prev.ZoneID, blob.ID)
			}
		}

		// Add to new zone occupancy
		if zoneID != "" {
			m.updateOccupancy(zoneID, blob.ID)
		}

		// Detect portal crossings and collect zone transition events
		if existed && prev.ZoneID != zoneID {
			crossings := m.collectCrossings(blob.ID, prev.X, prev.Y, prev.Z, blob.X, blob.Y, blob.Z, zoneID)

			// Build zone exit event
			var exitEvt *ZoneTransitionEvent
			if prev.ZoneID != "" {
				prevName := ""
				if z, ok := m.zones[prev.ZoneID]; ok {
					prevName = z.Name
				}
				exitEvt = &ZoneTransitionEvent{
					BlobID:    blob.ID,
					ZoneID:    prev.ZoneID,
					ZoneName:  prevName,
					Kind:      "zone_exit",
					Timestamp: now,
				}
			}

			// Build zone entry event
			var entryEvt *ZoneTransitionEvent
			if zoneID != "" {
				newName := ""
				if z, ok := m.zones[zoneID]; ok {
					newName = z.Name
				}
				entryEvt = &ZoneTransitionEvent{
					BlobID:    blob.ID,
					ZoneID:    zoneID,
					ZoneName:  newName,
					Kind:      "zone_entry",
					Timestamp: now,
				}
			}

			for _, c := range crossings {
				pending = append(pending, pendingCrossing{crossing: c, exit: exitEvt, entry: entryEvt})
				// Only attach zone events to the first crossing
				exitEvt = nil
				entryEvt = nil
			}

			// If no portal crossings but zone changed, still fire zone events
			if len(crossings) == 0 {
				pending = append(pending, pendingCrossing{exit: exitEvt, entry: entryEvt})
			}
		}
	}

	// Clean up old blob positions (not seen in 10 seconds)
	for id, pos := range m.blobPositions {
		if now.Sub(pos.LastUpdated) > 10*time.Second {
			delete(m.blobPositions, id)
			// Also remove from occupancy and persist
			for zoneID, occ := range m.occupancy {
				newBlobIDs := make([]int, 0)
				for _, bid := range occ.BlobIDs {
					if bid != id {
						newBlobIDs = append(newBlobIDs, bid)
					}
				}
				if len(newBlobIDs) != len(occ.BlobIDs) {
					occ.BlobIDs = newBlobIDs
					occ.Count = len(occ.BlobIDs)
					m.persistOccupancyCount(zoneID, occ.Count)
				}
			}
		}
	}

	onCrossing := m.onCrossing
	onZoneEntry := m.onZoneEntry
	onZoneExit := m.onZoneExit

	m.mu.Unlock()

	// Fire callbacks synchronously after releasing the lock.
	for _, p := range pending {
		if p.crossing.PortalID != "" && onCrossing != nil {
			onCrossing(p.crossing)
		}
		if p.exit != nil && onZoneExit != nil {
			onZoneExit(*p.exit)
		}
		if p.entry != nil && onZoneEntry != nil {
			onZoneEntry(*p.entry)
		}
	}
}

// findZoneForPosition returns the zone ID containing the position.
func (m *Manager) findZoneForPosition(x, y, z float64) string {
	for id, zone := range m.zones {
		if !zone.Enabled {
			continue
		}
		if x >= zone.MinX && x <= zone.MaxX &&
			y >= zone.MinY && y <= zone.MaxY &&
			z >= zone.MinZ && z <= zone.MaxZ {
			return id
		}
	}
	return ""
}

// updateOccupancy updates the occupancy count for a zone.
// Persists the new count to SQLite for restart recovery.
func (m *Manager) updateOccupancy(zoneID string, blobID int) {
	occ, exists := m.occupancy[zoneID]
	if !exists {
		occ = &ZoneOccupancy{
			ZoneID:  zoneID,
			BlobIDs: []int{blobID},
			Count:   1,
		}
		m.occupancy[zoneID] = occ
		m.persistOccupancyCount(zoneID, 1)
		return
	}

	// Check if blob already in zone
	for _, id := range occ.BlobIDs {
		if id == blobID {
			return
		}
	}

	occ.BlobIDs = append(occ.BlobIDs, blobID)
	occ.Count = len(occ.BlobIDs)
	m.persistOccupancyCount(zoneID, occ.Count)
}

// removeFromOccupancy removes a blob from a zone's occupancy tracking.
// Caller must hold m.mu write lock.
func (m *Manager) removeFromOccupancy(zoneID string, blobID int) {
	occ, exists := m.occupancy[zoneID]
	if !exists {
		return
	}
	newBlobIDs := make([]int, 0, len(occ.BlobIDs))
	for _, id := range occ.BlobIDs {
		if id != blobID {
			newBlobIDs = append(newBlobIDs, id)
		}
	}
	if len(newBlobIDs) == len(occ.BlobIDs) {
		return // blob was not in this zone
	}
	occ.BlobIDs = newBlobIDs
	occ.Count = len(occ.BlobIDs)
	m.persistOccupancyCount(zoneID, occ.Count)
}

// persistOccupancyCount writes a single zone's occupancy to SQLite.
// Caller must hold m.mu write lock.
func (m *Manager) persistOccupancyCount(zoneID string, count int) {
	nowMs := time.Now().UnixMilli()
	_, err := m.db.Exec(`
		UPDATE zones SET last_known_occupancy = ?, occupancy_updated_at = ? WHERE id = ?
	`, count, nowMs, zoneID)
	if err != nil {
		log.Printf("[WARN] Failed to persist occupancy for zone %s: %v", zoneID, err)
	}
}

// collectCrossings detects portal crossings and persists them, returning the events.
// Caller must hold m.mu. Callbacks are NOT fired here — caller fires them after releasing the lock.
func (m *Manager) collectCrossings(blobID int, prevX, prevY, prevZ, currX, currY, currZ float64, newZoneID string) []CrossingEvent {
	var events []CrossingEvent

	for _, portal := range m.portals {
		if !portal.Enabled {
			continue
		}

		// Check if portal connects to the new zone
		if portal.ZoneAID != newZoneID && portal.ZoneBID != newZoneID {
			continue
		}

		// Compute signed distance from portal plane
		prevSide := pointPlaneSide(prevX, prevY, prevZ, portal.P1X, portal.P1Y, portal.P1Z, portal.NX, portal.NY, portal.NZ)
		currSide := pointPlaneSide(currX, currY, currZ, portal.P1X, portal.P1Y, portal.P1Z, portal.NX, portal.NY, portal.NZ)

		// Check if crossed (signs are different)
		if prevSide*currSide < 0 {
			// Determine direction based on where the blob came FROM (prevSide),
			// not where it is now. prevSide > 0 means it was on the A side
			// (positive half-space), so it crossed from A to B.
			var direction int
			var fromZone, toZone string
			if prevSide > 0 {
				direction = 1 // A->B (was on A side, now on B side)
				fromZone = portal.ZoneAID
				toZone = portal.ZoneBID
			} else {
				direction = -1 // B->A (was on B side, now on A side)
				fromZone = portal.ZoneBID
				toZone = portal.ZoneAID
			}

			event := CrossingEvent{
				PortalID:  portal.ID,
				BlobID:    blobID,
				Direction: direction,
				FromZone:  fromZone,
				ToZone:    toZone,
				Timestamp: time.Now(),
			}

			// Persist event
			m.recordCrossing(event)

			log.Printf("[INFO] Portal crossing: blob %d crossed %s (direction: %d)", blobID, portal.Name, direction)

			events = append(events, event)
		}
	}

	return events
}

// pointPlaneSide returns which side of a plane a point is on (>0 or <0).
func pointPlaneSide(px, py, pz, p1x, p1y, p1z, nx, ny, nz float64) float64 {
	// Vector from plane point to point
	dx := px - p1x
	dy := py - p1y
	dz := pz - p1z

	// Dot product with normal
	return dx*nx + dy*ny + dz*nz
}

// computeNormal computes the normal vector from three points.
func computeNormal(p *Portal) (nx, ny, nz float64) {
	// Vectors from P1 to P2 and P1 to P3
	v1x := p.P2X - p.P1X
	v1y := p.P2Y - p.P1Y
	v1z := p.P2Z - p.P1Z

	v2x := p.P3X - p.P1X
	v2y := p.P3Y - p.P1Y
	v2z := p.P3Z - p.P1Z

	// Cross product
	nx = v1y*v2z - v1z*v2y
	ny = v1z*v2x - v1x*v2z
	nz = v1x*v2y - v1y*v2x

	// Normalize
	length := math.Sqrt(nx*nx + ny*ny + nz*nz)
	if length > 0 {
		nx /= length
		ny /= length
		nz /= length
	}

	return nx, ny, nz
}

// recordCrossing persists a crossing event.
func (m *Manager) recordCrossing(event CrossingEvent) {
	_, err := m.db.Exec(`
		INSERT INTO crossing_events (portal_id, blob_id, direction, from_zone, to_zone, timestamp, identity)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, event.PortalID, event.BlobID, event.Direction, event.FromZone, event.ToZone, event.Timestamp.UnixMilli(), event.Identity)
	if err != nil {
		log.Printf("[WARN] Failed to record crossing event: %v", err)
	}
}

// GetOccupancy returns current occupancy for all zones.
func (m *Manager) GetOccupancy() map[string]*ZoneOccupancy {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*ZoneOccupancy)
	for k, v := range m.occupancy {
		result[k] = v
	}
	return result
}

// GetZoneOccupancy returns occupancy for a specific zone.
func (m *Manager) GetZoneOccupancy(zoneID string) *ZoneOccupancy {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.occupancy[zoneID]
}

// GetBlobZone returns the zone ID that a blob is currently in.
func (m *Manager) GetBlobZone(blobID int) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	pos, exists := m.blobPositions[blobID]
	if !exists {
		return ""
	}
	return pos.ZoneID
}

// UpdateBlobPosition updates a single blob's position (convenience method).
func (m *Manager) UpdateBlobPosition(blobID int, x, y, z float64) {
	m.UpdateBlobPositions([]struct {
		ID     int
		X, Y, Z float64
	}{{ID: blobID, X: x, Y: y, Z: z}})
}

// GetRecentCrossings returns recent crossing events.
func (m *Manager) GetRecentCrossings(limit int) []CrossingEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	rows, err := m.db.Query(`
		SELECT portal_id, blob_id, direction, from_zone, to_zone, timestamp, identity
		FROM crossing_events
		ORDER BY timestamp DESC
		LIMIT ?
	`, limit)
	if err != nil {
		log.Printf("[WARN] Failed to query crossings: %v", err)
		return nil
	}
	defer rows.Close()

	var events []CrossingEvent
	for rows.Next() {
		var event CrossingEvent
		var ts int64
		if err := rows.Scan(&event.PortalID, &event.BlobID, &event.Direction, &event.FromZone, &event.ToZone, &ts, &event.Identity); err != nil {
			continue
		}
		event.Timestamp = time.UnixMilli(ts)
		events = append(events, event)
	}
	return events
}

// GetBlobDwellTime returns how long a blob has been in a specific zone.
func (m *Manager) GetBlobDwellTime(blobID int, zoneID string) (time.Duration, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Find the most recent crossing event where this blob entered this zone
	var enterTime int64
	err := m.db.QueryRow(`
		SELECT timestamp FROM crossing_events
		WHERE blob_id = ? AND to_zone = ?
		ORDER BY timestamp DESC
		LIMIT 1
	`, blobID, zoneID).Scan(&enterTime)

	if err != nil {
		// No crossing event found - use last position update time
		pos, exists := m.blobPositions[blobID]
		if !exists || pos.ZoneID != zoneID {
			return 0, false
		}
		return time.Since(pos.LastUpdated), true
	}

	// Calculate dwell time since entering the zone
	dwellTime := time.Since(time.UnixMilli(enterTime))
	return dwellTime, true
}

// IsBedroomZone returns true if the zone is a bedroom zone.
func (z *Zone) IsBedroomZone() bool {
	return z.ZoneType == ZoneTypeBedroom
}

// IsChildrenZoneType returns true if the zone is a children zone.
func (z *Zone) IsChildrenZoneType() bool {
	return z.ZoneType == ZoneTypeChildren || z.IsChildrenZone
}

// GetBedroomZones returns all zones configured as bedrooms.
func (m *Manager) GetBedroomZones() []*Zone {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var bedrooms []*Zone
	for _, zone := range m.zones {
		if zone.Enabled && zone.IsBedroomZone() {
			bedrooms = append(bedrooms, zone)
		}
	}
	return bedrooms
}

// GetZoneByPosition returns the zone containing the given position.
func (m *Manager) GetZoneByPosition(x, y, z float64) *Zone {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, zone := range m.zones {
		if !zone.Enabled {
			continue
		}
		if x >= zone.MinX && x <= zone.MaxX &&
			y >= zone.MinY && y <= zone.MaxY &&
			z >= zone.MinZ && z <= zone.MaxZ {
			return zone
		}
	}
	return nil
}

// ─── Occupancy Reconciliation ─────────────────────────────────────────────

// reconcileOccupancy restores zone occupancy counts from persisted values
// plus net portal crossings since midnight. Called once on startup.
func (m *Manager) reconcileOccupancy() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().In(m.tz)
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, m.tz)
	midnightMs := midnight.UnixMilli()

	// Step 1: Load last_known_occupancy per zone
	rows, err := m.db.Query(`SELECT id, last_known_occupancy FROM zones`)
	if err != nil {
		log.Printf("[WARN] Failed to load persisted occupancy: %v", err)
		return
	}
	type persisted struct {
		zoneID string
		count  int
	}
	var persistedOcc []persisted
	for rows.Next() {
		var p persisted
		if err := rows.Scan(&p.zoneID, &p.count); err != nil {
			continue
		}
		persistedOcc = append(persistedOcc, p)
	}
	rows.Close()

	// Step 2: Compute net portal crossings since midnight
	crossRows, err := m.db.Query(`
		SELECT from_zone, to_zone, timestamp
		FROM crossing_events
		WHERE timestamp >= ?
	`, midnightMs)
	if err != nil {
		log.Printf("[WARN] Failed to query portal crossings since midnight: %v", err)
		return
	}
	defer crossRows.Close()

	netPerZone := make(map[string]int)
	for crossRows.Next() {
		var fromZone, toZone string
		var tsMs int64
		if err := crossRows.Scan(&fromZone, &toZone, &tsMs); err != nil {
			continue
		}
		// Each crossing: from_zone loses one, to_zone gains one
		netPerZone[fromZone]--
		netPerZone[toZone]++
	}

	// Step 3: Apply net crossings to loaded occupancy
	anyRestored := false
	for _, p := range persistedOcc {
		if _, exists := m.zones[p.zoneID]; !exists {
			continue
		}
		reconciled := p.count + netPerZone[p.zoneID]
		if reconciled < 0 {
			reconciled = 0
		}
		m.occupancy[p.zoneID] = &ZoneOccupancy{
			ZoneID:      p.zoneID,
			Count:       reconciled,
			BlobIDs:     nil,
			LastUpdated: now,
			Status:      OccupancyUncertain,
		}
		if reconciled > 0 {
			anyRestored = true
			log.Printf("[INFO] Zone %s: restored occupancy %d (persisted %d + net crossings %+d)",
				p.zoneID, reconciled, p.count, netPerZone[p.zoneID])
		}
	}

	if anyRestored {
		log.Printf("[INFO] Occupancy restored from persisted values (uncertain until verified)")
	} else {
		m.reconciled = true
	}
}

// ReconcileTick should be called every ~30s for the first 60s of operation.
// It compares portal-based occupancy against live blob counts per zone.
// If they differ by >1 for 2 consecutive checks, blob count wins.
// After 60s of live operation, marks all occupancies as reconciled.
func (m *Manager) ReconcileTick() {
	m.mu.Lock()
	defer m.mu.Unlock()

	elapsed := time.Since(m.startedAt)

	// Count blobs per zone from live positions
	blobCounts := make(map[string]int)
	for _, pos := range m.blobPositions {
		if pos.ZoneID != "" {
			blobCounts[pos.ZoneID]++
		}
	}

	for zoneID, occ := range m.occupancy {
		if occ.Status == OccupancyReconciled {
			continue
		}
		blobCount := blobCounts[zoneID]
		diff := occ.Count - blobCount
		if diff < 0 {
			diff = -diff
		}

		if diff > 1 {
			m.reconDiscrep++
			m.reconChecks = 0
			if m.reconDiscrep >= 2 {
				oldCount := occ.Count
				occ.Count = blobCount
				occ.BlobIDs = nil
				occ.LastUpdated = time.Now()
				log.Printf("[INFO] Zone %s: reconciling occupancy %d -> %d (blob count ground truth)",
					zoneID, oldCount, blobCount)
				m.reconDiscrep = 0
			}
		} else if diff == 0 {
			// Exact match — mark reconciled after 2 consecutive checks
			m.reconChecks++
			m.reconDiscrep = 0
			if m.reconChecks >= 2 {
				occ.Status = OccupancyReconciled
			}
		} else {
			// diff == 1: close but not exact, stay uncertain
			m.reconChecks = 0
			m.reconDiscrep = 0
		}
	}

	// Also mark zones with no occupancy entry as reconciled
	for zoneID := range m.zones {
		if _, exists := m.occupancy[zoneID]; !exists {
			m.occupancy[zoneID] = &ZoneOccupancy{
				ZoneID:      zoneID,
				Count:       0,
				BlobIDs:     nil,
				LastUpdated: time.Now(),
				Status:      OccupancyReconciled,
			}
		}
	}

	// After 60s, force-reconcile everything
	if elapsed >= 60*time.Second {
		for _, occ := range m.occupancy {
			if occ.Status == OccupancyUncertain {
				occ.Status = OccupancyReconciled
				occ.Count = blobCounts[occ.ZoneID]
				occ.BlobIDs = nil
				occ.LastUpdated = time.Now()
			}
		}
		if !m.reconciled {
			m.reconciled = true
			log.Printf("[INFO] Occupancy reconciliation complete (60s elapsed)")
		}
		return
	}

	if !m.reconciled {
		allReconciled := true
		for _, occ := range m.occupancy {
			if occ.Status != OccupancyReconciled {
				allReconciled = false
				break
			}
		}
		if allReconciled && len(m.occupancy) > 0 {
			m.reconciled = true
			log.Printf("[INFO] Occupancy reconciliation complete (all zones verified)")
		}
	}
}

// PersistOccupancy writes current occupancy counts to SQLite for restart recovery.
// Should be called on graceful shutdown and periodically.
func (m *Manager) PersistOccupancy() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	nowMs := time.Now().UnixMilli()
	for zoneID, occ := range m.occupancy {
		_, err := m.db.Exec(`
			UPDATE zones SET last_known_occupancy = ?, occupancy_updated_at = ? WHERE id = ?
		`, occ.Count, nowMs, zoneID)
		if err != nil {
			return fmt.Errorf("persist occupancy for zone %s: %w", zoneID, err)
		}
	}
	return nil
}

// PersistZoneOccupancy updates the persisted occupancy for a single zone.
func (m *Manager) PersistZoneOccupancy(zoneID string) error {
	m.mu.RLock()
	occ, exists := m.occupancy[zoneID]
	m.mu.RUnlock()

	if !exists {
		return nil
	}

	nowMs := time.Now().UnixMilli()
	_, err := m.db.Exec(`
		UPDATE zones SET last_known_occupancy = ?, occupancy_updated_at = ? WHERE id = ?
	`, occ.Count, nowMs, zoneID)
	return err
}

// IsReconciled returns whether the initial occupancy reconciliation is complete.
func (m *Manager) IsReconciled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.reconciled
}

// HistoryEntry represents an hourly occupancy bucket for the zone history API.
type HistoryEntry struct {
	Timestamp int64    `json:"timestamp"`
	Count     int      `json:"count"`
	People    []string `json:"people"`
}

// GetZoneHistory returns hourly occupancy buckets for a zone by querying
// crossing_events from SQLite. It computes net entry count per hour window.
func (m *Manager) GetZoneHistory(zoneID string, hours int) []HistoryEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	entries := make([]HistoryEntry, hours)

	// Build hourly buckets from now backwards
	for i := 0; i < hours; i++ {
		bucketEnd := now.Add(-time.Duration(i) * time.Hour)
		bucketStart := bucketEnd.Add(-time.Hour)
		entries[i] = HistoryEntry{
			Timestamp: bucketEnd.UnixNano() / 1e6,
			Count:     0,
			People:    []string{},
		}

		// Query net crossings into this zone during this bucket
		var netIn int
		row := m.db.QueryRow(`
			SELECT
				COALESCE(SUM(CASE WHEN to_zone = ? THEN 1 ELSE 0 END), 0)
				- COALESCE(SUM(CASE WHEN from_zone = ? THEN 1 ELSE 0 END), 0)
			FROM crossing_events
			WHERE timestamp >= ? AND timestamp < ?
		`, zoneID, zoneID, bucketStart.UnixMilli(), bucketEnd.UnixMilli())
		if err := row.Scan(&netIn); err == nil && netIn > 0 {
			entries[i].Count = netIn
		}
	}

	return entries
}

// GetOccupancyStatus returns the status map for all zones.
func (m *Manager) GetOccupancyStatus() map[string]OccupancyStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]OccupancyStatus, len(m.occupancy))
	for id, occ := range m.occupancy {
		result[id] = occ.Status
	}
	return result
}
