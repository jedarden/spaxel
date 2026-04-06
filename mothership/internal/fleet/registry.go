// Package fleet manages the node registry, role assignment, and fleet health.
package fleet

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// NodeRecord stores persistent node metadata.
type NodeRecord struct {
	MAC             string
	Name            string
	Role            string
	PreviousRole    string    // Role before disconnect, for reconnect grace period
	WentOfflineAt   time.Time // When the node went offline
	PosX            float64
	PosY            float64
	PosZ            float64
	Virtual         bool
	FirstSeenAt     time.Time
	LastSeenAt      time.Time
	FirmwareVersion string
	ChipModel       string
	HealthScore     float64 // Latest health score from ambient confidence
}

// RoomConfig stores room geometry.
type RoomConfig struct {
	ID      string
	Name    string
	Width   float64 // meters, X axis
	Depth   float64 // meters, Z axis
	Height  float64 // meters, Y axis
	OriginX float64
	OriginZ float64
}

// DefaultRoom is the initial room configuration inserted on first run.
var DefaultRoom = RoomConfig{
	ID:     "main",
	Name:   "Main",
	Width:  6.0,
	Depth:  5.0,
	Height: 2.5,
}

// Registry is a SQLite-backed store for nodes and room configuration.
type Registry struct {
	db *sql.DB
}

// NewRegistry opens (or creates) the SQLite database at dbPath.
func NewRegistry(dbPath string) (*Registry, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	conn.SetMaxOpenConns(1) // SQLite is single-writer

	r := &Registry{db: conn}
	if err := r.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return r, nil
}

func (r *Registry) migrate() error {
	_, err := r.db.Exec(`
		CREATE TABLE IF NOT EXISTS nodes (
			mac              TEXT PRIMARY KEY,
			name             TEXT    NOT NULL DEFAULT '',
			role             TEXT    NOT NULL DEFAULT 'rx',
			previous_role    TEXT    NOT NULL DEFAULT '',
			went_offline_at  INTEGER NOT NULL DEFAULT 0,
			pos_x            REAL    NOT NULL DEFAULT 0,
			pos_y            REAL    NOT NULL DEFAULT 0,
			pos_z            REAL    NOT NULL DEFAULT 0,
			virtual          INTEGER NOT NULL DEFAULT 0,
			first_seen_at    INTEGER NOT NULL DEFAULT 0,
			last_seen_at     INTEGER NOT NULL DEFAULT 0,
			firmware_version TEXT    NOT NULL DEFAULT '',
			chip_model       TEXT    NOT NULL DEFAULT '',
			health_score     REAL    NOT NULL DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS rooms (
			id       TEXT PRIMARY KEY,
			name     TEXT NOT NULL DEFAULT 'Main',
			width    REAL NOT NULL DEFAULT 6.0,
			depth    REAL NOT NULL DEFAULT 5.0,
			height   REAL NOT NULL DEFAULT 2.5,
			origin_x REAL NOT NULL DEFAULT 0,
			origin_z REAL NOT NULL DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS optimisation_history (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			timestamp        INTEGER NOT NULL,
			trigger_reason   TEXT    NOT NULL DEFAULT '',
			mean_gdop_before REAL    NOT NULL DEFAULT 0,
			mean_gdop_after  REAL    NOT NULL DEFAULT 0,
			coverage_delta   REAL    NOT NULL DEFAULT 0,
			nodes_before     TEXT    NOT NULL DEFAULT '',
			nodes_after      TEXT    NOT NULL DEFAULT ''
		);

		INSERT OR IGNORE INTO rooms (id, name, width, depth, height, origin_x, origin_z)
		VALUES ('main', 'Main', 6.0, 5.0, 2.5, 0, 0);
	`)
	if err != nil {
		return err
	}

	// Run migrations for new columns (ignore "duplicate column" errors)
	migrations := []string{
		"ALTER TABLE nodes ADD COLUMN previous_role TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE nodes ADD COLUMN went_offline_at INTEGER NOT NULL DEFAULT 0",
		"ALTER TABLE nodes ADD COLUMN health_score REAL NOT NULL DEFAULT 0",
	}
	for _, m := range migrations {
		_, _ = r.db.Exec(m) // Ignore errors (column may already exist)
	}
	return nil
}

// Close closes the database.
func (r *Registry) Close() error {
	return r.db.Close()
}

// UpsertNode inserts or updates a node record on connect.
func (r *Registry) UpsertNode(mac, firmware, chip string) error {
	now := time.Now().UnixNano()
	_, err := r.db.Exec(`
		INSERT INTO nodes (mac, firmware_version, chip_model, first_seen_at, last_seen_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(mac) DO UPDATE SET
			firmware_version = excluded.firmware_version,
			chip_model       = excluded.chip_model,
			last_seen_at     = excluded.last_seen_at
	`, mac, firmware, chip, now, now)
	return err
}

// TouchNode updates last_seen_at for an existing node.
func (r *Registry) TouchNode(mac string) error {
	_, err := r.db.Exec(`UPDATE nodes SET last_seen_at=? WHERE mac=?`, time.Now().UnixNano(), mac)
	return err
}

// SetNodePosition updates the 3D position for a node.
func (r *Registry) SetNodePosition(mac string, x, y, z float64) error {
	_, err := r.db.Exec(`UPDATE nodes SET pos_x=?, pos_y=?, pos_z=? WHERE mac=?`, x, y, z, mac)
	return err
}

// SetNodeRole updates the role for a node.
func (r *Registry) SetNodeRole(mac, role string) error {
	_, err := r.db.Exec(`UPDATE nodes SET role=? WHERE mac=?`, role, mac)
	return err
}

// SetNodePreviousRole saves the current role as previous_role for reconnect grace period.
func (r *Registry) SetNodePreviousRole(mac, role string) error {
	_, err := r.db.Exec(`UPDATE nodes SET previous_role=? WHERE mac=?`, role, mac)
	return err
}

// SetNodeOffline marks a node as offline with timestamp.
func (r *Registry) SetNodeOffline(mac string) error {
	now := time.Now().UnixNano()
	_, err := r.db.Exec(`UPDATE nodes SET went_offline_at=? WHERE mac=?`, now, mac)
	return err
}

// ClearNodeOffline clears the offline timestamp.
func (r *Registry) ClearNodeOffline(mac string) error {
	_, err := r.db.Exec(`UPDATE nodes SET went_offline_at=0 WHERE mac=?`, mac)
	return err
}

// SetNodeHealthScore updates the health score for a node.
func (r *Registry) SetNodeHealthScore(mac string, score float64) error {
	_, err := r.db.Exec(`UPDATE nodes SET health_score=? WHERE mac=?`, score, mac)
	return err
}

// GetNodePreviousRole returns the previous role for a node.
func (r *Registry) GetNodePreviousRole(mac string) (string, error) {
	var role string
	err := r.db.QueryRow(`SELECT previous_role FROM nodes WHERE mac=?`, mac).Scan(&role)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return role, err
}

// GetNodeWentOfflineAt returns when a node went offline.
func (r *Registry) GetNodeWentOfflineAt(mac string) (time.Time, error) {
	var ns int64
	err := r.db.QueryRow(`SELECT went_offline_at FROM nodes WHERE mac=?`, mac).Scan(&ns)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if ns == 0 {
		return time.Time{}, nil
	}
	return time.Unix(0, ns), err
}

// SetNodeName updates the name for a node.
func (r *Registry) SetNodeName(mac, name string) error {
	_, err := r.db.Exec(`UPDATE nodes SET name=? WHERE mac=?`, name, mac)
	return err
}

// AddVirtualNode inserts or updates a virtual node for coverage planning.
func (r *Registry) AddVirtualNode(mac, name string, x, y, z float64) error {
	now := time.Now().UnixNano()
	_, err := r.db.Exec(`
		INSERT INTO nodes (mac, name, role, pos_x, pos_y, pos_z, virtual, first_seen_at, last_seen_at)
		VALUES (?, ?, 'virtual', ?, ?, ?, 1, ?, ?)
		ON CONFLICT(mac) DO UPDATE SET
			name = excluded.name,
			pos_x = excluded.pos_x,
			pos_y = excluded.pos_y,
			pos_z = excluded.pos_z,
			virtual = 1
	`, mac, name, x, y, z, now, now)
	return err
}

// DeleteNode removes a node record.
func (r *Registry) DeleteNode(mac string) error {
	_, err := r.db.Exec(`DELETE FROM nodes WHERE mac=?`, mac)
	return err
}

// GetNode returns a single node record.
func (r *Registry) GetNode(mac string) (*NodeRecord, error) {
	row := r.db.QueryRow(`
		SELECT mac, name, role, previous_role, went_offline_at, pos_x, pos_y, pos_z, virtual, first_seen_at, last_seen_at, firmware_version, chip_model, health_score
		FROM nodes WHERE mac=?`, mac)
	return scanNode(row)
}

// GetNodePosition returns the 3D position of a node for BLE triangulation.
// Implements ble.NodePositionAccessor interface.
func (r *Registry) GetNodePosition(mac string) (x, y, z float64, ok bool) {
	node, err := r.GetNode(mac)
	if err != nil || node == nil {
		return 0, 0, 0, false
	}
	return node.PosX, node.PosY, node.PosZ, true
}

// GetAllNodes returns all node records ordered by first_seen_at.
func (r *Registry) GetAllNodes() ([]NodeRecord, error) {
	rows, err := r.db.Query(`
		SELECT mac, name, role, previous_role, went_offline_at, pos_x, pos_y, pos_z, virtual, first_seen_at, last_seen_at, firmware_version, chip_model, health_score
		FROM nodes ORDER BY first_seen_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []NodeRecord
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			log.Printf("[WARN] fleet: scan node: %v", err)
			continue
		}
		nodes = append(nodes, *n)
	}
	return nodes, rows.Err()
}

// OptimisationHistoryRecord stores historical optimisation events
type OptimisationHistoryRecord struct {
	ID              int64
	Timestamp       time.Time
	TriggerReason   string
	MeanGDOPBefore  float64
	MeanGDOPAfter   float64
	CoverageDelta   float64
	NodesBeforeJSON string
	NodesAfterJSON  string
}

// AddOptimisationHistory adds an optimisation event to the history
func (r *Registry) AddOptimisationHistory(rec OptimisationHistoryRecord) error {
	_, err := r.db.Exec(`
		INSERT INTO optimisation_history (timestamp, trigger_reason, mean_gdop_before, mean_gdop_after, coverage_delta, nodes_before, nodes_after)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, rec.Timestamp.UnixNano(), rec.TriggerReason, rec.MeanGDOPBefore, rec.MeanGDOPAfter, rec.CoverageDelta, rec.NodesBeforeJSON, rec.NodesAfterJSON)
	return err
}

// GetOptimisationHistory returns recent optimisation history
func (r *Registry) GetOptimisationHistory(limit int) ([]OptimisationHistoryRecord, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.db.Query(`
		SELECT id, timestamp, trigger_reason, mean_gdop_before, mean_gdop_after, coverage_delta, nodes_before, nodes_after
		FROM optimisation_history
		ORDER BY timestamp DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []OptimisationHistoryRecord
	for rows.Next() {
		var rec OptimisationHistoryRecord
		var ns int64
		if err := rows.Scan(&rec.ID, &ns, &rec.TriggerReason, &rec.MeanGDOPBefore, &rec.MeanGDOPAfter, &rec.CoverageDelta, &rec.NodesBeforeJSON, &rec.NodesAfterJSON); err != nil {
			continue
		}
		rec.Timestamp = time.Unix(0, ns)
		records = append(records, rec)
	}
	return records, rows.Err()
}

// GetRoom returns the main room configuration.
func (r *Registry) GetRoom() (*RoomConfig, error) {
	row := r.db.QueryRow(`SELECT id, name, width, depth, height, origin_x, origin_z FROM rooms WHERE id='main'`)
	return scanRoom(row)
}

// SetRoom updates the main room configuration.
func (r *Registry) SetRoom(room RoomConfig) error {
	_, err := r.db.Exec(`
		INSERT INTO rooms (id, name, width, depth, height, origin_x, origin_z)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name     = excluded.name,
			width    = excluded.width,
			depth    = excluded.depth,
			height   = excluded.height,
			origin_x = excluded.origin_x,
			origin_z = excluded.origin_z
	`, room.ID, room.Name, room.Width, room.Depth, room.Height, room.OriginX, room.OriginZ)
	return err
}

// scanner matches both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

func scanNode(s scanner) (*NodeRecord, error) {
	var n NodeRecord
	var virtual int
	var firstNS, lastNS, offlineNS int64
	err := s.Scan(
		&n.MAC, &n.Name, &n.Role, &n.PreviousRole, &offlineNS,
		&n.PosX, &n.PosY, &n.PosZ,
		&virtual, &firstNS, &lastNS,
		&n.FirmwareVersion, &n.ChipModel, &n.HealthScore,
	)
	if err != nil {
		return nil, err
	}
	n.Virtual = virtual != 0
	if firstNS > 0 {
		n.FirstSeenAt = time.Unix(0, firstNS)
	}
	if lastNS > 0 {
		n.LastSeenAt = time.Unix(0, lastNS)
	}
	if offlineNS > 0 {
		n.WentOfflineAt = time.Unix(0, offlineNS)
	}
	return &n, nil
}

func scanRoom(s scanner) (*RoomConfig, error) {
	var rc RoomConfig
	err := s.Scan(&rc.ID, &rc.Name, &rc.Width, &rc.Depth, &rc.Height, &rc.OriginX, &rc.OriginZ)
	if err != nil {
		return nil, err
	}
	return &rc, nil
}
