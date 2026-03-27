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
	PosX            float64
	PosY            float64
	PosZ            float64
	Virtual         bool
	FirstSeenAt     time.Time
	LastSeenAt      time.Time
	FirmwareVersion string
	ChipModel       string
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
			pos_x            REAL    NOT NULL DEFAULT 0,
			pos_y            REAL    NOT NULL DEFAULT 0,
			pos_z            REAL    NOT NULL DEFAULT 0,
			virtual          INTEGER NOT NULL DEFAULT 0,
			first_seen_at    INTEGER NOT NULL DEFAULT 0,
			last_seen_at     INTEGER NOT NULL DEFAULT 0,
			firmware_version TEXT    NOT NULL DEFAULT '',
			chip_model       TEXT    NOT NULL DEFAULT ''
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

		INSERT OR IGNORE INTO rooms (id, name, width, depth, height, origin_x, origin_z)
		VALUES ('main', 'Main', 6.0, 5.0, 2.5, 0, 0);
	`)
	return err
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
		SELECT mac, name, role, pos_x, pos_y, pos_z, virtual, first_seen_at, last_seen_at, firmware_version, chip_model
		FROM nodes WHERE mac=?`, mac)
	return scanNode(row)
}

// GetAllNodes returns all node records ordered by first_seen_at.
func (r *Registry) GetAllNodes() ([]NodeRecord, error) {
	rows, err := r.db.Query(`
		SELECT mac, name, role, pos_x, pos_y, pos_z, virtual, first_seen_at, last_seen_at, firmware_version, chip_model
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
	var firstNS, lastNS int64
	err := s.Scan(
		&n.MAC, &n.Name, &n.Role,
		&n.PosX, &n.PosY, &n.PosZ,
		&virtual, &firstNS, &lastNS,
		&n.FirmwareVersion, &n.ChipModel,
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
