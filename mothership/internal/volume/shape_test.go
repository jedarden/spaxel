// Package volume provides tests for 3D trigger volume geometry and point-in-volume testing.
package volume

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// TestZoneBounds tests the GetZoneByName method with various scenarios.
func TestZoneBounds(t *testing.T) {
	// Create a temporary database
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create a Store instance (which initializes the zones table with the correct schema)
	store, err := NewStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Get the underlying database to insert test zones directly
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Insert test zones using the correct schema (INTEGER AUTOINCREMENT id)
	_, err = db.Exec(`
		INSERT INTO zones (name, min_x, min_y, min_z, max_x, max_y, max_z, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, "Kitchen", 0.0, 0.0, 0.0, 4.0, 3.0, 2.5, 1)
	if err != nil {
		t.Fatalf("Failed to insert Kitchen zone: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO zones (name, min_x, min_y, min_z, max_x, max_y, max_z, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, "Bedroom", 5.0, 0.0, 0.0, 9.0, 4.0, 2.5, 1)
	if err != nil {
		t.Fatalf("Failed to insert Bedroom zone: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO zones (name, min_x, min_y, min_z, max_x, max_y, max_z, enabled)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, "Hallway", 4.0, 3.0, 0.0, 5.0, 8.0, 2.5, 0)
	if err != nil {
		t.Fatalf("Failed to insert Hallway zone: %v", err)
	}

	// Test 1: Get an existing enabled zone
	t.Run("existing enabled zone", func(t *testing.T) {
		geom, err := store.GetZoneByName("Kitchen")
		if err != nil {
			t.Fatalf("Expected no error, got: %v", err)
		}
		if geom == nil {
			t.Fatal("Expected geometry, got nil")
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
