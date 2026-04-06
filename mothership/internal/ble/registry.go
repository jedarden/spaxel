// Package ble provides BLE device registry and identity matching.
package ble

import (
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// DeviceType represents the auto-detected or manually assigned type of a BLE device.
type DeviceType string

const (
	DeviceTypeUnknown      DeviceType = "unknown"
	DeviceTypeApplePhone   DeviceType = "apple_phone"
	DeviceTypeAppleWatch   DeviceType = "apple_watch"
	DeviceTypeAppleEarbuds DeviceType = "apple_earbuds"
	DeviceTypeMicrosoft    DeviceType = "microsoft"
	DeviceTypeSamsung      DeviceType = "samsung"
	DeviceTypeFitbit       DeviceType = "fitbit"
	DeviceTypeGarmin       DeviceType = "garmin"
	DeviceTypeTile         DeviceType = "tile"
	DeviceTypeRuuvi        DeviceType = "ruuvi"
	DeviceTypeGoogle       DeviceType = "google"
)

// ManufacturerInfo maps company IDs to manufacturer name and likely device type.
var ManufacturerInfo = map[int]struct {
	Name string
	Type DeviceType
}{
	0x004C: {"Apple", DeviceTypeApplePhone},   // Apple - iPhone, iPad, AirPods, Apple Watch
	0x0006: {"Microsoft", DeviceTypeMicrosoft}, // Microsoft - Windows devices
	0x0075: {"Samsung", DeviceTypeSamsung},     // Samsung - phones/tablets
	0x009E: {"Fitbit", DeviceTypeFitbit},       // Fitbit - fitness trackers
	0x0157: {"Garmin", DeviceTypeGarmin},       // Garmin - GPS watches/fitness
	0x0059: {"Nordic", DeviceTypeTile},         // Nordic Semiconductor - Tile trackers
	0x0499: {"Ruuvi", DeviceTypeRuuvi},         // Ruuvi - temperature/humidity sensors
	0x00E0: {"Google", DeviceTypeGoogle},       // Google - Android Nearby Share
}

// DeviceRecord represents a registered BLE device.
type DeviceRecord struct {
	Addr         string     `json:"mac"`            // MAC address
	Name         string     `json:"name"`           // User-assigned name (e.g., "Alice's Phone")
	Label        string     `json:"label"`          // Short label for display
	Manufacturer string     `json:"manufacturer"`   // Auto-detected manufacturer name
	DeviceType   DeviceType `json:"device_type"`    // Auto-detected or manual type
	DeviceName   string     `json:"device_name"`    // Name from advertising (e.g., "iPhone")
	MfrID        int        `json:"mfr_id"`         // Manufacturer ID from advertising
	MfrDataHex   string     `json:"mfr_data_hex"`   // Raw manufacturer data (hex)
	PersonID     string     `json:"person_id"`      // FK to people.id
	PersonName   string     `json:"person_name"`    // Person name (joined from people)
	RSSIMin      int        `json:"rssi_min"`       // Min RSSI observed
	RSSIMax      int        `json:"rssi_max"`       // Max RSSI observed
	RSSIAvg      int        `json:"rssi_avg"`       // Average RSSI
	FirstSeenAt  time.Time  `json:"first_seen_at"`
	LastSeenAt   time.Time  `json:"last_seen_at"`
	LastSeenNode string     `json:"last_seen_node"` // MAC of node that last saw this device
	IsArchived   bool       `json:"is_archived"`    // Soft-delete flag
	IsWearable   bool       `json:"is_wearable"`    // Heuristic: possibly wearable
	Enabled      bool       `json:"enabled"`        // Whether tracking is enabled
	LastLocation Location   `json:"last_location"`  // Last known location from triangulation
}

// Person represents a named person in the system.
type Person struct {
	ID        string    `json:"id"`         // UUID
	Name      string    `json:"name"`       // Display name
	Color     string    `json:"color"`      // Hex color for dashboard
	CreatedAt time.Time `json:"created_at"`
}

// PossibleDuplicate represents a pair of devices that may be the same device with rotated MAC.
type PossibleDuplicate struct {
	MAC1       string  `json:"mac1"`
	MAC2       string  `json:"mac2"`
	Name1      string  `json:"name1"`
	Name2      string  `json:"name2"`
	Reason     string  `json:"reason"`
	Confidence float64 `json:"confidence"`
}

// Location represents a 3D position with confidence.
type Location struct {
	X          float64   `json:"x"`
	Y          float64   `json:"y"`
	Z          float64   `json:"z"`
	Confidence float64   `json:"confidence"`
	Timestamp  time.Time `json:"timestamp"`
}

// RSSIObservation is a single RSSI reading from a node.
type RSSIObservation struct {
	NodeMAC   string    `json:"node_mac"`
	RSSIdBm   int       `json:"rssi_dbm"`
	Timestamp time.Time `json:"timestamp"`
}

// RSSICache holds recent RSSI observations per device per node.
type RSSICache struct {
	mu           sync.RWMutex
	observations map[string]map[string]*RSSIObservation // addr -> nodeMAC -> observation
	maxAge       time.Duration
}

// NewRSSICache creates a new RSSI cache.
func NewRSSICache(maxAge time.Duration) *RSSICache {
	return &RSSICache{
		observations: make(map[string]map[string]*RSSIObservation),
		maxAge:       maxAge,
	}
}

// Add records an RSSI observation.
func (c *RSSICache) Add(addr, nodeMAC string, rssi int) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.observations[addr] == nil {
		c.observations[addr] = make(map[string]*RSSIObservation)
	}
	c.observations[addr][nodeMAC] = &RSSIObservation{
		NodeMAC:   nodeMAC,
		RSSIdBm:   rssi,
		Timestamp: time.Now(),
	}
}

// AddWithTime records an RSSI observation with a specific timestamp.
func (c *RSSICache) AddWithTime(addr, nodeMAC string, rssi int, ts time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.observations[addr] == nil {
		c.observations[addr] = make(map[string]*RSSIObservation)
	}
	c.observations[addr][nodeMAC] = &RSSIObservation{
		NodeMAC:   nodeMAC,
		RSSIdBm:   rssi,
		Timestamp: ts,
	}
}

// CleanOlder removes observations older than the specified duration.
func (c *RSSICache) CleanOlder(maxAge time.Duration) {
	c.Prune()
}

// GetRecent returns recent observations for a device within the max age.
func (c *RSSICache) GetRecent(addr string, maxAge time.Duration) []*RSSIObservation {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()
	var result []*RSSIObservation
	for _, obs := range c.observations[addr] {
		if now.Sub(obs.Timestamp) < maxAge {
			result = append(result, obs)
		}
	}
	return result
}

// Get returns all non-stale observations for a device.
func (c *RSSICache) Get(addr string) []*RSSIObservation {
	return c.GetRecent(addr, c.maxAge)
}

// GetAll returns all current observations (for identity matching).
func (c *RSSICache) GetAll() map[string][]*RSSIObservation {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()
	result := make(map[string][]*RSSIObservation)
	for addr, nodeObs := range c.observations {
		for _, obs := range nodeObs {
			if now.Sub(obs.Timestamp) < c.maxAge {
				result[addr] = append(result[addr], obs)
			}
		}
	}
	return result
}

// Prune removes stale observations.
func (c *RSSICache) Prune() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for addr, nodeObs := range c.observations {
		for nodeMAC, obs := range nodeObs {
			if now.Sub(obs.Timestamp) > c.maxAge {
				delete(nodeObs, nodeMAC)
			}
		}
		if len(nodeObs) == 0 {
			delete(c.observations, addr)
		}
	}
}

// Registry is a SQLite-backed BLE device registry with people management.
type Registry struct {
	db        *sql.DB
	rssiCache *RSSICache
	mu        sync.RWMutex
}

// NewRegistry opens or creates the BLE device database.
func NewRegistry(dbPath string) (*Registry, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	conn.SetMaxOpenConns(1)

	r := &Registry{
		db:        conn,
		rssiCache: NewRSSICache(30 * time.Second),
	}
	if err := r.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return r, nil
}

func (r *Registry) migrate() error {
	// Create people table
	if _, err := r.db.Exec(`
		CREATE TABLE IF NOT EXISTS people (
			id         TEXT PRIMARY KEY,
			name       TEXT NOT NULL,
			color      TEXT NOT NULL DEFAULT '#3b82f6',
			created_at INTEGER NOT NULL DEFAULT (strftime('%s', 'now') * 1000000000)
		)
	`); err != nil {
		return fmt.Errorf("create people table: %w", err)
	}

	// Create ble_devices table with enhanced schema
	if _, err := r.db.Exec(`
		CREATE TABLE IF NOT EXISTS ble_devices (
			mac            TEXT PRIMARY KEY,
			name           TEXT    NOT NULL DEFAULT '',
			label          TEXT    NOT NULL DEFAULT '',
			manufacturer   TEXT    NOT NULL DEFAULT '',
			device_type    TEXT    NOT NULL DEFAULT 'unknown',
			device_name    TEXT    NOT NULL DEFAULT '',
			mfr_id         INTEGER NOT NULL DEFAULT 0,
			mfr_data_hex   TEXT    NOT NULL DEFAULT '',
			person_id      TEXT    REFERENCES people(id),
			rssi_min       INTEGER NOT NULL DEFAULT 0,
			rssi_max       INTEGER NOT NULL DEFAULT 0,
			rssi_avg       INTEGER NOT NULL DEFAULT 0,
			rssi_count     INTEGER NOT NULL DEFAULT 0,
			rssi_sum       INTEGER NOT NULL DEFAULT 0,
			first_seen_at  INTEGER NOT NULL DEFAULT 0,
			last_seen_at   INTEGER NOT NULL DEFAULT 0,
			last_seen_node TEXT    NOT NULL DEFAULT '',
			is_archived    INTEGER NOT NULL DEFAULT 0,
			is_wearable    INTEGER NOT NULL DEFAULT 0,
			enabled        INTEGER NOT NULL DEFAULT 1,
			last_x         REAL    NOT NULL DEFAULT 0,
			last_y         REAL    NOT NULL DEFAULT 0,
			last_z         REAL    NOT NULL DEFAULT 0,
			last_confidence REAL  NOT NULL DEFAULT 0,
			last_loc_time  INTEGER NOT NULL DEFAULT 0
		)
	`); err != nil {
		return fmt.Errorf("create ble_devices table: %w", err)
	}

	// Create indexes
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_ble_devices_person_id ON ble_devices(person_id)`,
		`CREATE INDEX IF NOT EXISTS idx_ble_devices_device_type ON ble_devices(device_type)`,
		`CREATE INDEX IF NOT EXISTS idx_ble_devices_archived ON ble_devices(is_archived)`,
		`CREATE INDEX IF NOT EXISTS idx_ble_devices_last_seen ON ble_devices(last_seen_at)`,
	}
	for _, idx := range indexes {
		if _, err := r.db.Exec(idx); err != nil {
			return fmt.Errorf("create index: %w", err)
		}
	}

	return nil
}

// Close closes the database.
func (r *Registry) Close() error {
	return r.db.Close()
}

// BLEObservation represents a single BLE device observation from a node.
type BLEObservation struct {
	Addr       string // MAC address
	Name       string // Device name from advertising
	MfrID      int    // Manufacturer ID
	MfrDataHex string // Raw manufacturer data (hex)
	RSSIdBm    int    // Signal strength
}

// ProcessRelayMessage processes BLE relay messages from a node, upserting all devices.
func (r *Registry) ProcessRelayMessage(nodeMAC string, devices []BLEObservation) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UnixNano()

	for _, dev := range devices {
		// Add to RSSI cache
		r.rssiCache.Add(dev.Addr, nodeMAC, dev.RSSIdBm)

		// Auto-detect device type and manufacturer
		deviceType, manufacturer := detectDeviceTypeAndManufacturer(dev.MfrID, dev.MfrDataHex, dev.Name)

		// Check if this might be a wearable based on device type
		isWearable := isLikelyWearable(deviceType)

		// Upsert device
		_, err := r.db.Exec(`
			INSERT INTO ble_devices (
				mac, device_type, device_name, manufacturer, mfr_id, mfr_data_hex,
				rssi_min, rssi_max, rssi_avg, rssi_count, rssi_sum,
				first_seen_at, last_seen_at, last_seen_node, is_wearable, enabled
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, 1)
			ON CONFLICT(mac) DO UPDATE SET
				device_type = CASE WHEN device_type = 'unknown' THEN excluded.device_type ELSE device_type END,
				device_name = CASE WHEN excluded.device_name != '' THEN excluded.device_name ELSE device_name END,
				manufacturer = CASE WHEN excluded.manufacturer != '' THEN excluded.manufacturer ELSE manufacturer END,
				mfr_id = CASE WHEN excluded.mfr_id != 0 THEN excluded.mfr_id ELSE mfr_id END,
				mfr_data_hex = excluded.mfr_data_hex,
				rssi_min = CASE WHEN excluded.rssi_min < rssi_min THEN excluded.rssi_min ELSE rssi_min END,
				rssi_max = CASE WHEN excluded.rssi_max > rssi_max THEN excluded.rssi_max ELSE rssi_max END,
				rssi_count = rssi_count + 1,
				rssi_sum = rssi_sum + excluded.rssi_sum,
				rssi_avg = (rssi_sum + excluded.rssi_sum) / (rssi_count + 1),
				last_seen_at = excluded.last_seen_at,
				last_seen_node = excluded.last_seen_node,
				is_wearable = excluded.is_wearable
		`, dev.Addr, deviceType, dev.Name, manufacturer, dev.MfrID, dev.MfrDataHex,
			dev.RSSIdBm, dev.RSSIdBm, dev.RSSIdBm, dev.RSSIdBm,
			now, now, nodeMAC, isWearable)

		if err != nil {
			log.Printf("[WARN] ble: failed to upsert device %s: %v", dev.Addr, err)
		}
	}

	return nil
}

// detectDeviceTypeAndManufacturer infers device type and manufacturer from BLE data.
func detectDeviceTypeAndManufacturer(mfrID int, mfrDataHex, name string) (DeviceType, string) {
	// Check manufacturer ID first
	if info, ok := ManufacturerInfo[mfrID]; ok {
		// For Apple, refine based on manufacturer data
		if mfrID == 0x004C {
			return detectAppleDevice(mfrDataHex, name)
		}
		return refineDeviceType(info.Type, name), info.Name
	}

	// Check name patterns if manufacturer ID unknown
	return detectDeviceTypeFromName(name), "Unknown"
}

// detectAppleDevice refines Apple device type based on manufacturer data.
func detectAppleDevice(mfrDataHex, name string) (DeviceType, string) {
	mfrData, err := hex.DecodeString(mfrDataHex)
	if err != nil || len(mfrData) < 4 {
		return refineDeviceType(DeviceTypeApplePhone, name), "Apple"
	}

	// Check for device indicators in name
	nameLower := strings.ToLower(name)
	if strings.Contains(nameLower, "airpod") || strings.Contains(nameLower, "airpods") {
		return DeviceTypeAppleEarbuds, "Apple"
	}
	if strings.Contains(nameLower, "watch") {
		return DeviceTypeAppleWatch, "Apple"
	}

	// Parse Apple-specific data segments
	i := 0
	for i < len(mfrData) {
		if i+1 >= len(mfrData) {
			break
		}
		segLen := int(mfrData[i])
		if segLen == 0 || i+1+segLen > len(mfrData) {
			break
		}
		segType := mfrData[i+1]

		switch segType {
		case 0x09: // Battery level - typically AirPods
			return DeviceTypeAppleEarbuds, "Apple"
		case 0x0C: // Find My - AirTag
			return DeviceTypeTile, "Apple" // Treat as tracker
		}

		i += 1 + segLen
	}

	return refineDeviceType(DeviceTypeApplePhone, name), "Apple"
}

// refineDeviceType refines device type based on name.
func refineDeviceType(baseType DeviceType, name string) DeviceType {
	nameLower := strings.ToLower(name)

	switch {
	case strings.Contains(nameLower, "watch") || strings.Contains(nameLower, "gear"):
		return DeviceTypeAppleWatch
	case strings.Contains(nameLower, "airpod") || strings.Contains(nameLower, "airpods") || strings.Contains(nameLower, "buds"):
		return DeviceTypeAppleEarbuds
	case strings.Contains(nameLower, "fitbit"):
		return DeviceTypeFitbit
	case strings.Contains(nameLower, "garmin"):
		return DeviceTypeGarmin
	case strings.Contains(nameLower, "tile") || strings.Contains(nameLower, "airtag"):
		return DeviceTypeTile
	}

	return baseType
}

// detectDeviceTypeFromName infers device type from name patterns.
func detectDeviceTypeFromName(name string) DeviceType {
	nameLower := strings.ToLower(name)

	switch {
	case strings.Contains(nameLower, "iphone") || strings.Contains(nameLower, "pixel") || strings.Contains(nameLower, "galaxy s"):
		return DeviceTypeApplePhone
	case strings.Contains(nameLower, "ipad") || strings.Contains(nameLower, "tablet") || strings.Contains(nameLower, "galaxy tab"):
		return DeviceTypeApplePhone
	case strings.Contains(nameLower, "watch") || strings.Contains(nameLower, "gear") || strings.Contains(nameLower, "fitbit"):
		return DeviceTypeFitbit
	case strings.Contains(nameLower, "tile") || strings.Contains(nameLower, "airtag") || strings.Contains(nameLower, "chipolo"):
		return DeviceTypeTile
	case strings.Contains(nameLower, "airpod") || strings.Contains(nameLower, "headphone") || strings.Contains(nameLower, "buds"):
		return DeviceTypeAppleEarbuds
	case strings.Contains(nameLower, "garmin"):
		return DeviceTypeGarmin
	case strings.Contains(nameLower, "ruuvi"):
		return DeviceTypeRuuvi
	default:
		return DeviceTypeUnknown
	}
}

// isLikelyWearable returns true if the device type suggests it's worn on the body.
func isLikelyWearable(deviceType DeviceType) bool {
	switch deviceType {
	case DeviceTypeAppleWatch, DeviceTypeFitbit, DeviceTypeGarmin:
		return true
	default:
		return false
	}
}

// GetDevices returns all devices, optionally including archived ones.
func (r *Registry) GetDevices(includeArchived bool) ([]DeviceRecord, error) {
	query := `
		SELECT d.mac, d.name, d.label, d.manufacturer, d.device_type, d.device_name,
		       d.mfr_id, d.mfr_data_hex, d.person_id, COALESCE(p.name, ''),
		       d.rssi_min, d.rssi_max, d.rssi_avg,
		       d.first_seen_at, d.last_seen_at, d.last_seen_node, d.is_archived, d.is_wearable, d.enabled,
		       d.last_x, d.last_y, d.last_z, d.last_confidence, d.last_loc_time
		FROM ble_devices d
		LEFT JOIN people p ON d.person_id = p.id
	`
	if !includeArchived {
		query += " WHERE d.is_archived = 0"
	}
	query += " ORDER BY d.is_archived, d.last_seen_at DESC"

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []DeviceRecord
	for rows.Next() {
		d, err := scanDeviceRow(rows)
		if err != nil {
			log.Printf("[WARN] ble: scan device: %v", err)
			continue
		}
		devices = append(devices, *d)
	}
	return devices, rows.Err()
}

// GetDevice returns a single device by MAC address.
func (r *Registry) GetDevice(mac string) (*DeviceRecord, error) {
	row := r.db.QueryRow(`
		SELECT d.mac, d.name, d.label, d.manufacturer, d.device_type, d.device_name,
		       d.mfr_id, d.mfr_data_hex, d.person_id, COALESCE(p.name, ''),
		       d.rssi_min, d.rssi_max, d.rssi_avg,
		       d.first_seen_at, d.last_seen_at, d.last_seen_node, d.is_archived, d.is_wearable, d.enabled,
		       d.last_x, d.last_y, d.last_z, d.last_confidence, d.last_loc_time
		FROM ble_devices d
		LEFT JOIN people p ON d.person_id = p.id
		WHERE d.mac = ?
	`, mac)
	return scanDeviceRow(row)
}

// UpdateLabel updates the user-assigned label for a device.
func (r *Registry) UpdateLabel(mac, label string) error {
	_, err := r.db.Exec(`UPDATE ble_devices SET name = ? WHERE mac = ?`, label, mac)
	return err
}

// UpdateDevice updates multiple fields for a device.
func (r *Registry) UpdateDevice(mac string, updates map[string]interface{}) error {
	if len(updates) == 0 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Build dynamic update query
	query := "UPDATE ble_devices SET "
	args := []interface{}{}
	first := true

	for field, value := range updates {
		if !first {
			query += ", "
		}
		query += field + " = ?"
		args = append(args, value)
		first = false
	}

	query += " WHERE mac = ?"
	args = append(args, mac)

	_, err := r.db.Exec(query, args...)
	return err
}

// AssignToPerson assigns a device to a person.
func (r *Registry) AssignToPerson(mac, personID string) error {
	return r.UpdateDevice(mac, map[string]interface{}{"person_id": personID})
}

// UnassignFromPerson removes a device from its person.
func (r *Registry) UnassignFromPerson(mac string) error {
	return r.UpdateDevice(mac, map[string]interface{}{"person_id": nil})
}

// ArchiveDevice marks a device as archived (soft delete).
func (r *Registry) ArchiveDevice(mac string) error {
	return r.UpdateDevice(mac, map[string]interface{}{"is_archived": 1})
}

// UnarchiveDevice marks a device as not archived.
func (r *Registry) UnarchiveDevice(mac string) error {
	return r.UpdateDevice(mac, map[string]interface{}{"is_archived": 0})
}

// DeleteDevice permanently removes a device from the registry.
func (r *Registry) DeleteDevice(mac string) error {
	_, err := r.db.Exec(`DELETE FROM ble_devices WHERE mac = ?`, mac)
	return err
}

// ArchiveStale marks devices not seen for longer than olderThan as archived.
func (r *Registry) ArchiveStale(olderThan time.Duration) (int64, error) {
	cutoff := time.Now().Add(-olderThan).UnixNano()
	result, err := r.db.Exec(`
		UPDATE ble_devices SET is_archived = 1
		WHERE last_seen_at < ? AND is_archived = 0
	`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// CreatePerson creates a new person.
func (r *Registry) CreatePerson(name, color string) (Person, error) {
	id := generateUUID()
	now := time.Now().UnixNano()

	_, err := r.db.Exec(`
		INSERT INTO people (id, name, color, created_at) VALUES (?, ?, ?, ?)
	`, id, name, color, now)
	if err != nil {
		return Person{}, err
	}

	return Person{
		ID:        id,
		Name:      name,
		Color:     color,
		CreatedAt: time.Unix(0, now),
	}, nil
}

// GetPeople returns all people.
func (r *Registry) GetPeople() ([]Person, error) {
	rows, err := r.db.Query(`SELECT id, name, color, created_at FROM people ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var people []Person
	for rows.Next() {
		var p Person
		var createdAtNS int64
		if err := rows.Scan(&p.ID, &p.Name, &p.Color, &createdAtNS); err != nil {
			continue
		}
		p.CreatedAt = time.Unix(0, createdAtNS)
		people = append(people, p)
	}
	return people, rows.Err()
}

// GetPerson returns a single person by ID.
func (r *Registry) GetPerson(id string) (*Person, error) {
	row := r.db.QueryRow(`SELECT id, name, color, created_at FROM people WHERE id = ?`, id)

	var p Person
	var createdAtNS int64
	err := row.Scan(&p.ID, &p.Name, &p.Color, &createdAtNS)
	if err != nil {
		return nil, err
	}
	p.CreatedAt = time.Unix(0, createdAtNS)
	return &p, nil
}

// UpdatePerson updates a person's name and/or color.
func (r *Registry) UpdatePerson(id, name, color string) error {
	if name != "" && color != "" {
		_, err := r.db.Exec(`UPDATE people SET name = ?, color = ? WHERE id = ?`, name, color, id)
		return err
	} else if name != "" {
		_, err := r.db.Exec(`UPDATE people SET name = ? WHERE id = ?`, name, id)
		return err
	} else if color != "" {
		_, err := r.db.Exec(`UPDATE people SET color = ? WHERE id = ?`, color, id)
		return err
	}
	return nil
}

// DeletePerson soft-deletes a person (retains historical data).
func (r *Registry) DeletePerson(id string) error {
	// Unassign all devices from this person first
	if _, err := r.db.Exec(`UPDATE ble_devices SET person_id = NULL WHERE person_id = ?`, id); err != nil {
		return err
	}
	_, err := r.db.Exec(`DELETE FROM people WHERE id = ?`, id)
	return err
}

// GetPersonDevices returns all devices assigned to a person.
func (r *Registry) GetPersonDevices(personID string) ([]DeviceRecord, error) {
	rows, err := r.db.Query(`
		SELECT d.mac, d.name, d.label, d.manufacturer, d.device_type, d.device_name,
		       d.mfr_id, d.mfr_data_hex, d.person_id, COALESCE(p.name, ''),
		       d.rssi_min, d.rssi_max, d.rssi_avg,
		       d.first_seen_at, d.last_seen_at, d.last_seen_node, d.is_archived, d.is_wearable, d.enabled,
		       d.last_x, d.last_y, d.last_z, d.last_confidence, d.last_loc_time
		FROM ble_devices d
		LEFT JOIN people p ON d.person_id = p.id
		WHERE d.person_id = ? AND d.is_archived = 0
		ORDER BY d.last_seen_at DESC
	`, personID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []DeviceRecord
	for rows.Next() {
		d, err := scanDeviceRow(rows)
		if err != nil {
			continue
		}
		devices = append(devices, *d)
	}
	return devices, rows.Err()
}

// GetPeopleWithDevices returns all people with their associated devices.
func (r *Registry) GetPeopleWithDevices() ([]map[string]interface{}, error) {
	people, err := r.GetPeople()
	if err != nil {
		return nil, err
	}

	result := make([]map[string]interface{}, len(people))
	for i, p := range people {
		devices, err := r.GetPersonDevices(p.ID)
		if err != nil {
			devices = nil
		}

		// Find most recent last_seen among devices
		var lastSeen time.Time
		for _, d := range devices {
			if d.LastSeenAt.After(lastSeen) {
				lastSeen = d.LastSeenAt
			}
		}

		result[i] = map[string]interface{}{
			"id":           p.ID,
			"name":         p.Name,
			"color":        p.Color,
			"created_at":   p.CreatedAt,
			"device_count": len(devices),
			"devices":      devices,
			"last_seen":    lastSeen,
		}
	}
	return result, nil
}

// DetectPossibleDuplicates finds device pairs that may be the same device with rotated MAC.
func (r *Registry) DetectPossibleDuplicates() ([]PossibleDuplicate, error) {
	devices, err := r.GetDevices(false)
	if err != nil {
		return nil, err
	}

	var duplicates []PossibleDuplicate

	for i := 0; i < len(devices); i++ {
		for j := i + 1; j < len(devices); j++ {
			d1, d2 := devices[i], devices[j]

			if d1.DeviceName == "" || d2.DeviceName == "" {
				continue
			}
			if d1.DeviceName != d2.DeviceName {
				continue
			}

			if !similarMfrData(d1.MfrDataHex, d2.MfrDataHex) {
				continue
			}

			confidence := 0.5
			if d1.MfrID == d2.MfrID && d1.MfrID != 0 {
				confidence = 0.7
			}

			timeDiff := d1.LastSeenAt.Sub(d2.LastSeenAt)
			if timeDiff < 0 {
				timeDiff = -timeDiff
			}
			if timeDiff < 30*time.Second {
				confidence *= 0.5
			}

			if confidence >= 0.3 {
				duplicates = append(duplicates, PossibleDuplicate{
					MAC1:       d1.Addr,
					MAC2:       d2.Addr,
					Name1:      d1.DeviceName,
					Name2:      d2.DeviceName,
					Reason:     "Same name and similar manufacturer data suggest MAC rotation",
					Confidence: confidence,
				})
			}
		}
	}

	return duplicates, nil
}

// similarMfrData checks if two manufacturer data hex strings have the same first 6 bytes.
func similarMfrData(hex1, hex2 string) bool {
	if len(hex1) < 12 || len(hex2) < 12 {
		return hex1 == hex2
	}
	return hex1[:12] == hex2[:12]
}

// MergeDevices merges two devices, keeping mac1 and removing mac2.
func (r *Registry) MergeDevices(mac1, mac2 string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	tx, err := r.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var device2Name, device2PersonID sql.NullString
	err = tx.QueryRow(`SELECT name, person_id FROM ble_devices WHERE mac = ?`, mac2).Scan(&device2Name, &device2PersonID)
	if err != nil {
		return err
	}

	if device2Name.Valid && device2Name.String != "" {
		tx.Exec(`UPDATE ble_devices SET name = ? WHERE mac = ? AND name = ''`, device2Name.String, mac1)
	}
	if device2PersonID.Valid && device2PersonID.String != "" {
		tx.Exec(`UPDATE ble_devices SET person_id = ? WHERE mac = ? AND person_id IS NULL`, device2PersonID.String, mac1)
	}

	if _, err := tx.Exec(`DELETE FROM ble_devices WHERE mac = ?`, mac2); err != nil {
		return err
	}

	return tx.Commit()
}

// UpdateLocation updates the last known location for a device.
func (r *Registry) UpdateLocation(addr string, loc Location) error {
	now := loc.Timestamp.UnixNano()
	_, err := r.db.Exec(`
		UPDATE ble_devices SET
			last_x = ?, last_y = ?, last_z = ?,
			last_confidence = ?, last_loc_time = ?
		WHERE mac = ?
	`, loc.X, loc.Y, loc.Z, loc.Confidence, now, addr)
	return err
}

// GetRSSICache returns the RSSI observation cache.
func (r *Registry) GetRSSICache() *RSSICache {
	return r.rssiCache
}

// GetAllPersonDevices returns all devices assigned to any person.
func (r *Registry) GetAllPersonDevices() ([]DeviceRecord, error) {
	rows, err := r.db.Query(`
		SELECT d.mac, d.name, d.label, d.manufacturer, d.device_type, d.device_name,
		       d.mfr_id, d.mfr_data_hex, d.person_id, COALESCE(p.name, ''),
		       d.rssi_min, d.rssi_max, d.rssi_avg,
		       d.first_seen_at, d.last_seen_at, d.last_seen_node, d.is_archived, d.is_wearable, d.enabled,
		       d.last_x, d.last_y, d.last_z, d.last_confidence, d.last_loc_time
		FROM ble_devices d
		LEFT JOIN people p ON d.person_id = p.id
		WHERE d.person_id IS NOT NULL AND d.is_archived = 0 AND d.enabled = 1
		ORDER BY d.last_seen_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var devices []DeviceRecord
	for rows.Next() {
		d, err := scanDeviceRow(rows)
		if err != nil {
			continue
		}
		devices = append(devices, *d)
	}
	return devices, rows.Err()
}

// RecordObservation records a BLE observation from a node (adapter for ingestion.BLEDevice).
func (r *Registry) RecordObservation(nodeMAC string, dev BLEObservation) error {
	return r.ProcessRelayMessage(nodeMAC, []BLEObservation{dev})
}

// RecordObservations records multiple BLE observations from a node (adapter for ingestion.BLEDevice slice).
func (r *Registry) RecordObservations(nodeMAC string, devices []BLEObservation) error {
	return r.ProcessRelayMessage(nodeMAC, devices)
}

// scanner matches both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

func scanDeviceRow(s scanner) (*DeviceRecord, error) {
	var d DeviceRecord
	var isArchived, isWearable, enabled int
	var firstNS, lastNS, locNS int64
	var personID sql.NullString

	err := s.Scan(
		&d.Addr, &d.Name, &d.Label, &d.Manufacturer, &d.DeviceType, &d.DeviceName,
		&d.MfrID, &d.MfrDataHex, &personID, &d.PersonName,
		&d.RSSIMin, &d.RSSIMax, &d.RSSIAvg,
		&firstNS, &lastNS, &d.LastSeenNode, &isArchived, &isWearable, &enabled,
		&d.LastLocation.X, &d.LastLocation.Y, &d.LastLocation.Z,
		&d.LastLocation.Confidence, &locNS,
	)
	if err != nil {
		return nil, err
	}

	if personID.Valid {
		d.PersonID = personID.String
	}
	d.IsArchived = isArchived != 0
	d.IsWearable = isWearable != 0
	d.Enabled = enabled != 0

	if firstNS > 0 {
		d.FirstSeenAt = time.Unix(0, firstNS)
	}
	if lastNS > 0 {
		d.LastSeenAt = time.Unix(0, lastNS)
	}
	if locNS > 0 {
		d.LastLocation.Timestamp = time.Unix(0, locNS)
	}

	return &d, nil
}

// generateUUID generates a simple UUID v4 string.
func generateUUID() string {
	b := make([]byte, 16)
	now := time.Now().UnixNano()
	b[0] = byte(now)
	b[1] = byte(now >> 8)
	b[2] = byte(now >> 16)
	b[3] = byte(now >> 24)
	b[4] = byte(now >> 32)
	b[5] = byte(now >> 40)
	b[6] = byte(now >> 48)
	b[7] = byte(now >> 56)
	for i := 8; i < 16; i++ {
		b[i] = byte(now + int64(i*31))
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
