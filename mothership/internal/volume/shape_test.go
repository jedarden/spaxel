// Package volume provides tests for 3D trigger volume geometry and point-in-volume testing.
package volume

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestZoneBounds tests the GetZoneByName method with various scenarios.
func TestZoneBounds(t *testing.T) {
	// Create a temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Open the database and initialize zones table
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		t.Fatalf("Failed to enable foreign keys: %v", err)
	}

	// Create zones table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS zones (
			id             TEXT PRIMARY KEY,
			name           TEXT    NOT NULL,
			color          TEXT,
			min_x          REAL NOT NULL,
			min_y          REAL NOT NULL,
			min_z          REAL NOT NULL,
			max_x          REAL NOT NULL,
			max_y          REAL NOT NULL,
			max_z          REAL NOT NULL,
			enabled        INTEGER NOT NULL DEFAULT 1,
			zone_type      TEXT    NOT NULL DEFAULT 'normal',
			is_children_zone INTEGER NOT NULL DEFAULT 0,
			created_at      INTEGER
		);
	`)
	if err != nil {
		t.Fatalf("Failed to create zones table: %v", err)
	}

	// Insert test zones
	now := time.Now().UnixNano()
	testZones := []struct {
		id          string
		name        string
		color       string
		minX, minY, minZ float64
		maxX, maxY, maxZ float64
		enabled     bool
	}{
		{
			id:     "zone_kitchen",
			name:   "Kitchen",
			color:  "#ff5722",
			minX:   0.0, minY: 0.0, minZ: 0.0,
			maxX:   4.0, maxY: 3.0, maxZ: 2.5,
			enabled: true,
		},
		{
			id:     "zone_bedroom",
			name:   "Bedroom",
			color:  "#4fc3f7",
			minX:   5.0, minY: 0.0, minZ: 0.0,
			maxX:   9.0, maxY: 4.0, maxZ: 2.5,
			enabled: true,
		},
		{
			id:     "zone_hallway",
			name:   "Hallway",
			color:  "#4caf50",
			minX:   4.0, minY: 3.0, minZ: 0.0,
			maxX:   5.0, maxY: 8.0, maxZ: 2.5,
			enabled: false, // Disabled zone
		},
	}

	for _, z := range testZones {
		enabled := 0
		if z.enabled {
			enabled = 1
		}
		_, err := db.Exec(`
			INSERT INTO zones (id, name, color, min_x, min_y, min_z, max_x, max_y, max_z, enabled, zone_type, is_children_zone, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, z.id, z.name, z.color, z.minX, z.minY, z.minZ, z.maxX, z.maxY, z.maxZ, enabled, "normal", 0, now)
		if err != nil {
			t.Fatalf("Failed to insert test zone: %v", err)
		}
	}

	// Create a Store instance
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Test 1: Get an existing enabled zone
	t.Run("existing enabled zone", func(t *testing.T) {
		geom, err := store.GetZoneByName("Kitchen")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if geom == nil {
			t.Fatal("Expected geometry, got nil")
		}
		if geom.ID != "zone_kitchen" {
			t.Errorf("Expected ID 'zone_kitchen', got '%s'", geom.ID)
		}
		if geom.Name != "Kitchen" {
			t.Errorf("Expected name 'Kitchen', got '%s'", geom.Name)
		}
		if geom.MinX != 0.0 || geom.MinY != 0.0 || geom.MinZ != 0.0 {
			t.Errorf("Expected Min (0, 0, 0), got (%.1f, %.1f, %.1f)", geom.MinX, geom.MinY, geom.MinZ)
		}
		if geom.MaxX != 4.0 || geom.MaxY != 3.0 || geom.MaxZ != 2.5 {
			t.Errorf("Expected Max (4, 3, 2.5), got (%.1f, %.1f, %.1f)", geom.MaxX, geom.MaxY, geom.MaxZ)
		}
		if !geom.Enabled {
			t.Error("Expected Enabled=true, got false")
		}
	})

	// Test 2: Get another existing enabled zone
	t.Run("another enabled zone", func(t *testing.T) {
		geom, err := store.GetZoneByName("Bedroom")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if geom == nil {
			t.Fatal("Expected geometry, got nil")
		}
		if geom.ID != "zone_bedroom" {
			t.Errorf("Expected ID 'zone_bedroom', got '%s'", geom.ID)
		}
		if geom.Name != "Bedroom" {
			t.Errorf("Expected name 'Bedroom', got '%s'", geom.Name)
		}
		if geom.MinX != 5.0 || geom.MinY != 0.0 || geom.MinZ != 0.0 {
			t.Errorf("Expected Min (5, 0, 0), got (%.1f, %.1f, %.1f)", geom.MinX, geom.MinY, geom.MinZ)
		}
		if geom.MaxX != 9.0 || geom.MaxY != 4.0 || geom.MaxZ != 2.5 {
			t.Errorf("Expected Max (9, 4, 2.5), got (%.1f, %.1f, %.1f)", geom.MaxX, geom.MaxY, geom.MaxZ)
		}
	})

	// Test 3: Zone not found
	t.Run("zone not found", func(t *testing.T) {
		geom, err := store.GetZoneByName("NonExistent")
		if err == nil {
			t.Error("Expected error for non-existent zone, got nil")
		}
		if geom != nil {
			t.Error("Expected nil geometry for not found, got geometry")
		}
		if err.Error() != "zone not found: NonExistent" {
			t.Errorf("Expected 'zone not found: NonExistent' error, got: %v", err)
		}
	})

	// Test 4: Disabled zone should return error
	t.Run("disabled zone", func(t *testing.T) {
		geom, err := store.GetZoneByName("Hallway")
		if err == nil {
			t.Error("Expected error for disabled zone, got nil")
		}
		if geom != nil {
			t.Error("Expected nil geometry for disabled zone, got geometry")
		}
		if err.Error() != "zone is disabled: Hallway" {
			t.Errorf("Expected 'zone is disabled: Hallway' error, got: %v", err)
		}
	})
}
