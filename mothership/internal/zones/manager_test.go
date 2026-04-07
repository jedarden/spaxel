package zones

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// testDB creates a temporary database file for testing.
func testDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "test.db")
}

// setupManager creates a Manager with a test database and pre-populated zones.
func setupManager(t *testing.T, tz *time.Location) (*Manager, func()) {
	t.Helper()
	if tz == nil {
		tz = time.UTC
	}
	dbPath := testDB(t)
	m, err := NewManager(dbPath, tz)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	cleanup := func() {
		m.Close()
		os.Remove(dbPath)
	}
	return m, cleanup
}

// --- reconcileOccupancy tests ---

func TestReconcileOccupancy_PersistedOnly(t *testing.T) {
	tests := []struct {
		name       string
		persisted  map[string]int // zone_id -> last_known_occupancy
		wantCount  map[string]int // zone_id -> expected reconciled count
		wantStatus map[string]OccupancyStatus
	}{
		{
			name:       "no persisted values",
			persisted:  map[string]int{},
			wantCount:  map[string]int{},
			wantStatus: map[string]OccupancyStatus{},
		},
		{
			name: "single zone with 2 people",
			persisted: map[string]int{
				"kitchen": 2,
			},
			wantCount: map[string]int{
				"kitchen": 2,
			},
			wantStatus: map[string]OccupancyStatus{
				"kitchen": OccupancyUncertain,
			},
		},
		{
			name: "multiple zones with various counts",
			persisted: map[string]int{
				"kitchen":  1,
				"bedroom":  0,
				"hallway":  3,
			},
			wantCount: map[string]int{
				"kitchen":  1,
				"bedroom":  0,
				"hallway":  3,
			},
			wantStatus: map[string]OccupancyStatus{
				"kitchen":  OccupancyUncertain,
				"bedroom":  OccupancyUncertain,
				"hallway":  OccupancyUncertain,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, cleanup := setupManager(t, time.UTC)
			defer cleanup()

			// Create zones and set persisted occupancy
			for zoneID := range tt.persisted {
				zone := &Zone{
					ID:     zoneID,
					Name:   zoneID,
					MinX: 0, MinY: 0, MinZ: 0,
					MaxX: 1, MaxY: 1, MaxZ: 1,
					Enabled: true,
				}
				if err := m.CreateZone(zone); err != nil {
					t.Fatalf("CreateZone: %v", err)
				}
				// Set persisted occupancy directly in DB
				m.db.Exec(`UPDATE zones SET last_known_occupancy = ? WHERE id = ?`, tt.persisted[zoneID], zoneID)
			}

			// Run reconciliation
			m.reconcileOccupancy()

			// Check results
			for zoneID, wantCount := range tt.wantCount {
				occ := m.GetZoneOccupancy(zoneID)
				if occ == nil {
					if wantCount != 0 {
						t.Errorf("zone %s: got nil occupancy, want count %d", zoneID, wantCount)
					}
					continue
				}
				if occ.Count != wantCount {
					t.Errorf("zone %s: got count %d, want %d", zoneID, occ.Count, wantCount)
				}
			}

			// Verify status for zones
			for zoneID, wantStatus := range tt.wantStatus {
				occ := m.GetZoneOccupancy(zoneID)
				if occ == nil {
					t.Errorf("zone %s: nil occupancy, want status %s", zoneID, wantStatus)
					continue
				}
				if occ.Status != wantStatus {
					t.Errorf("zone %s: got status %s, want %s", zoneID, occ.Status, wantStatus)
				}
			}
		})
	}
}

func TestReconcileOccupancy_WithCrossings(t *testing.T) {
	tests := []struct {
		name         string
		persisted    map[string]int // zone_id -> last_known_occupancy
		crossings    []struct {
			zoneA   string
			zoneB   string
			dir     int // 1 = a_to_b, -1 = b_to_a
			tsMs    int64
		}
		wantCount    map[string]int
		wantStatus   map[string]OccupancyStatus
	}{
		{
			name: "one person left kitchen after midnight",
			persisted: map[string]int{
				"kitchen":  2,
				"hallway":  0,
			},
			crossings: []struct {
				zoneA string
				zoneB string
				dir   int
				tsMs  int64
			}{
				{zoneA: "kitchen", zoneB: "hallway", dir: 1, tsMs: nowMsSinceMidnight(1 * time.Hour)},
			},
			wantCount: map[string]int{
				"kitchen":  1,
				"hallway":  1,
			},
			wantStatus: map[string]OccupancyStatus{
				"kitchen":  OccupancyUncertain,
				"hallway":  OccupancyUncertain,
			},
		},
		{
			name: "person entered and left (net zero)",
			persisted: map[string]int{
				"kitchen":  1,
				"hallway":  0,
			},
			crossings: []struct {
				zoneA string
				zoneB string
				dir   int
				tsMs  int64
			}{
				{zoneA: "kitchen", zoneB: "hallway", dir: 1, tsMs: nowMsSinceMidnight(1 * time.Hour)},
				{zoneA: "hallway", zoneB: "kitchen", dir: 1, tsMs: nowMsSinceMidnight(2 * time.Hour)},
			},
			wantCount: map[string]int{
				"kitchen":  1,
				"hallway":  0,
			},
			wantStatus: map[string]OccupancyStatus{
				"kitchen":  OccupancyUncertain,
			},
		},
		{
			name: "net negative clamped to zero",
			persisted: map[string]int{
				"kitchen":  0,
				"hallway":  0,
			},
			crossings: []struct {
				zoneA string
				zoneB string
				dir   int
				tsMs  int64
			}{
				{zoneA: "kitchen", zoneB: "hallway", dir: 1, tsMs: nowMsSinceMidnight(1 * time.Hour)},
			},
			wantCount: map[string]int{
				"kitchen":  0, // clamped from -1
				"hallway":  1,
			},
			wantStatus: map[string]OccupancyStatus{
				"hallway": OccupancyUncertain,
			},
		},
		{
			name: "crossings before midnight ignored",
			persisted: map[string]int{
				"kitchen":  2,
				"hallway":  0,
			},
			crossings: []struct {
				zoneA string
				zoneB string
				dir   int
				tsMs  int64
			}{
				// This crossing is before midnight, should be ignored
				{zoneA: "kitchen", zoneB: "hallway", dir: 1, tsMs: nowMsSinceMidnight(-1 * time.Hour)},
			},
			wantCount: map[string]int{
				"kitchen":  2,
				"hallway":  0,
			},
			wantStatus: map[string]OccupancyStatus{
				"kitchen": OccupancyUncertain,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, cleanup := setupManager(t, time.UTC)
			defer cleanup()

			// Create zones
			allZoneIDs := make(map[string]bool)
			for zoneID := range tt.persisted {
				allZoneIDs[zoneID] = true
			}
			for _, c := range tt.crossings {
				allZoneIDs[c.zoneA] = true
				allZoneIDs[c.zoneB] = true
			}
			for zoneID := range allZoneIDs {
				zone := &Zone{
					ID: zoneID, Name: zoneID,
					MinX: 0, MinY: 0, MinZ: 0,
					MaxX: 1, MaxY: 1, MaxZ: 1,
					Enabled: true,
				}
				if err := m.CreateZone(zone); err != nil {
					t.Fatalf("CreateZone: %v", err)
				}
			}

			// Set persisted occupancy
			for zoneID, count := range tt.persisted {
				m.db.Exec(`UPDATE zones SET last_known_occupancy = ? WHERE id = ?`, count, zoneID)
			}

			// Insert crossing events
			for _, c := range tt.crossings {
				m.db.Exec(`
					INSERT INTO crossing_events (portal_id, blob_id, direction, from_zone, to_zone, timestamp)
					VALUES (?, ?, ?, ?, ?, ?)
				`, "portal_1", 1, c.dir, c.zoneA, c.zoneB, c.tsMs)
			}

			// Run reconciliation
			m.reconcileOccupancy()

			// Check results
			for zoneID, wantCount := range tt.wantCount {
				occ := m.GetZoneOccupancy(zoneID)
				if occ == nil {
					if wantCount != 0 {
						t.Errorf("zone %s: got nil occupancy, want count %d", zoneID, wantCount)
					}
					continue
				}
				if occ.Count != wantCount {
					t.Errorf("zone %s: got count %d, want %d", zoneID, occ.Count, wantCount)
				}
			}
			for zoneID, wantStatus := range tt.wantStatus {
				occ := m.GetZoneOccupancy(zoneID)
				if occ == nil {
					t.Errorf("zone %s: nil occupancy, want status %s", zoneID, wantStatus)
					continue
				}
				if occ.Status != wantStatus {
					t.Errorf("zone %s: got status %s, want %s", zoneID, occ.Status, wantStatus)
				}
			}
		})
	}
}

// --- ReconcileTick tests ---

func TestReconcileTick_BlobCountOverride(t *testing.T) {
	tests := []struct {
		name           string
		initialCount   int
		blobCount      int
		ticks          int // number of ReconcileTick calls
		wantFinalCount int
		wantReconciled bool
	}{
		{
			name:           "no discrepancy",
			initialCount:   2,
			blobCount:      2,
			ticks:          2,
			wantFinalCount: 2,
			wantReconciled: true, // agrees after 2 checks
		},
		{
			name:           "off by 1 is ok",
			initialCount:   2,
			blobCount:      1,
			ticks:          2,
			wantFinalCount: 2, // still uncertain after 2 checks (diff=1 not >1)
			wantReconciled: false,
		},
		{
			name:           "off by 2 triggers override after 2 ticks",
			initialCount:   3,
			blobCount:      1,
			ticks:          2,
			wantFinalCount: 1, // blob count wins
			wantReconciled: false,
		},
		{
			name:           "off by 5 triggers override after 2 ticks",
			initialCount:   5,
			blobCount:      0,
			ticks:          2,
			wantFinalCount: 0,
			wantReconciled: false,
		},
		{
			name:           "single tick with large discrepancy does not override",
			initialCount:   3,
			blobCount:      0,
			ticks:          1,
			wantFinalCount: 3, // needs 2 consecutive discrepancies
			wantReconciled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, cleanup := setupManager(t, time.UTC)
			defer cleanup()

			// Create zone
			zone := &Zone{
				ID: "test_zone", Name: "Test",
				MinX: 0, MinY: 0, MinZ: 0,
				MaxX: 10, MaxY: 10, MaxZ: 3,
				Enabled: true,
			}
			if err := m.CreateZone(zone); err != nil {
				t.Fatalf("CreateZone: %v", err)
			}

			// Set initial occupancy as uncertain
			m.mu.Lock()
			m.occupancy["test_zone"] = &ZoneOccupancy{
				ZoneID: "test_zone",
				Count:  tt.initialCount,
				Status: OccupancyUncertain,
			}
			m.reconciled = false // reset — constructor set true when no zones existed
			m.mu.Unlock()

			// Place blobs to simulate live blob count
			m.mu.Lock()
			for i := 0; i < tt.blobCount; i++ {
				m.blobPositions[i+100] = struct {
					X, Y, Z     float64
					ZoneID      string
					LastUpdated time.Time
				}{X: 1, Y: 1, Z: 1, ZoneID: "test_zone", LastUpdated: time.Now()}
			}
			m.mu.Unlock()

			// Run ticks
			for i := 0; i < tt.ticks; i++ {
				m.ReconcileTick()
			}

			occ := m.GetZoneOccupancy("test_zone")
			if occ == nil {
				t.Fatalf("nil occupancy for test_zone")
			}
			if occ.Count != tt.wantFinalCount {
				t.Errorf("got count %d, want %d", occ.Count, tt.wantFinalCount)
			}
			if m.IsReconciled() != tt.wantReconciled {
				t.Errorf("got reconciled %v, want %v", m.IsReconciled(), tt.wantReconciled)
			}
		})
	}
}

func TestReconcileTick_ForceReconcileAfter60s(t *testing.T) {
	m, cleanup := setupManager(t, time.UTC)
	defer cleanup()

	// Create zone
	zone := &Zone{
		ID: "test_zone", Name: "Test",
		MinX: 0, MinY: 0, MinZ: 0,
		MaxX: 10, MaxY: 10, MaxZ: 3,
		Enabled: true,
	}
	if err := m.CreateZone(zone); err != nil {
		t.Fatalf("CreateZone: %v", err)
	}

	// Set initial occupancy as uncertain with wrong count
	m.mu.Lock()
	m.occupancy["test_zone"] = &ZoneOccupancy{
		ZoneID: "test_zone",
		Count:  5,
		Status: OccupancyUncertain,
	}
	m.reconciled = false
	m.startedAt = time.Now().Add(-61 * time.Second) // simulate 61s elapsed
	m.mu.Unlock()

	// Run tick — should force-reconcile even though there are no blobs
	m.ReconcileTick()

	occ := m.GetZoneOccupancy("test_zone")
	if occ == nil {
		t.Fatalf("nil occupancy")
	}
	if occ.Status != OccupancyReconciled {
		t.Errorf("got status %s, want reconciled", occ.Status)
	}
	if occ.Count != 0 {
		t.Errorf("got count %d, want 0 (no blobs)", occ.Count)
	}
	if !m.IsReconciled() {
		t.Error("expected IsReconciled=true after 60s force")
	}
}

// --- PersistOccupancy tests ---

func TestPersistOccupancy(t *testing.T) {
	tests := []struct {
		name      string
		occupancy map[string]int
	}{
		{
			name: "single zone",
			occupancy: map[string]int{
				"kitchen": 2,
			},
		},
		{
			name: "multiple zones",
			occupancy: map[string]int{
				"kitchen":  1,
				"bedroom":  0,
				"hallway":  3,
			},
		},
		{
			name:      "empty",
			occupancy: map[string]int{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, cleanup := setupManager(t, time.UTC)
			defer cleanup()

			// Create zones
			for zoneID, count := range tt.occupancy {
				zone := &Zone{
					ID: zoneID, Name: zoneID,
					MinX: 0, MinY: 0, MinZ: 0,
					MaxX: 1, MaxY: 1, MaxZ: 1,
					Enabled: true,
				}
				if err := m.CreateZone(zone); err != nil {
					t.Fatalf("CreateZone: %v", err)
				}
				m.occupancy[zoneID] = &ZoneOccupancy{
					ZoneID: zoneID,
					Count:  count,
					Status: OccupancyReconciled,
				}
			}

			if err := m.PersistOccupancy(); err != nil {
				t.Fatalf("PersistOccupancy: %v", err)
			}

			// Verify values in DB
			for zoneID, wantCount := range tt.occupancy {
				var gotCount int
				err := m.db.QueryRow(`SELECT last_known_occupancy FROM zones WHERE id = ?`, zoneID).Scan(&gotCount)
				if err != nil {
					t.Errorf("failed to query zone %s: %v", zoneID, err)
					continue
				}
				if gotCount != wantCount {
					t.Errorf("zone %s: got persisted count %d, want %d", zoneID, gotCount, wantCount)
				}
			}
		})
	}
}

func TestPersistOccupancy_OnBlobUpdate(t *testing.T) {
	m, cleanup := setupManager(t, time.UTC)
	defer cleanup()

	zone := &Zone{
		ID: "kitchen", Name: "Kitchen",
		MinX: 0, MinY: 0, MinZ: 0,
		MaxX: 10, MaxY: 10, MaxZ: 3,
		Enabled: true,
	}
	if err := m.CreateZone(zone); err != nil {
		t.Fatalf("CreateZone: %v", err)
	}

	// Update blob positions — should persist occupancy
	m.UpdateBlobPositions([]struct {
		ID     int
		X, Y, Z float64
	}{
		{ID: 1, X: 5, Y: 5, Z: 1},
	})

	// Verify persisted in DB
	var gotCount int
	err := m.db.QueryRow(`SELECT last_known_occupancy FROM zones WHERE id = 'kitchen'`).Scan(&gotCount)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if gotCount != 1 {
		t.Errorf("got persisted count %d, want 1", gotCount)
	}

	// Add second blob
	m.UpdateBlobPositions([]struct {
		ID     int
		X, Y, Z float64
	}{
		{ID: 1, X: 5, Y: 5, Z: 1},
		{ID: 2, X: 6, Y: 6, Z: 1},
	})

	err = m.db.QueryRow(`SELECT last_known_occupancy FROM zones WHERE id = 'kitchen'`).Scan(&gotCount)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if gotCount != 2 {
		t.Errorf("got persisted count %d, want 2", gotCount)
	}
}

func TestPersistOccupancy_OnBlobRemoval(t *testing.T) {
	m, cleanup := setupManager(t, time.UTC)
	defer cleanup()

	zone := &Zone{
		ID: "kitchen", Name: "Kitchen",
		MinX: 0, MinY: 0, MinZ: 0,
		MaxX: 10, MaxY: 10, MaxZ: 3,
		Enabled: true,
	}
	if err := m.CreateZone(zone); err != nil {
		t.Fatalf("CreateZone: %v", err)
	}

	// Add blobs
	m.UpdateBlobPositions([]struct {
		ID     int
		X, Y, Z float64
	}{
		{ID: 1, X: 5, Y: 5, Z: 1},
		{ID: 2, X: 6, Y: 6, Z: 1},
	})

	// Simulate blob timeout by manipulating LastUpdated directly
	m.mu.Lock()
	for id := range m.blobPositions {
		pos := m.blobPositions[id]
		pos.LastUpdated = time.Now().Add(-15 * time.Second)
		m.blobPositions[id] = pos
	}
	m.mu.Unlock()

	// Trigger cleanup by updating with no blobs (empty update still cleans up)
	m.UpdateBlobPositions(nil)

	// Verify persisted count is 0
	var gotCount int
	err := m.db.QueryRow(`SELECT last_known_occupancy FROM zones WHERE id = 'kitchen'`).Scan(&gotCount)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if gotCount != 0 {
		t.Errorf("got persisted count %d, want 0 after blob timeout", gotCount)
	}
}

// --- End-to-end: reconcile after restart simulation ---

func TestEndToEnd_RestoreOccupancyAfterRestart(t *testing.T) {
	// Simulate: zone has 2 people, shutdown (persist), "restart" (reconcile)
	dbPath := testDB(t)
	tz := time.UTC

	// Phase 1: Initial run — create zone, set occupancy, persist
	m1, err := NewManager(dbPath, tz)
	if err != nil {
		t.Fatalf("NewManager (phase 1): %v", err)
	}

	zone := &Zone{
		ID: "kitchen", Name: "Kitchen",
		MinX: 0, MinY: 0, MinZ: 0,
		MaxX: 10, MaxY: 10, MaxZ: 3,
		Enabled: true,
	}
	if err := m1.CreateZone(zone); err != nil {
		t.Fatalf("CreateZone: %v", err)
	}

	// Simulate 2 people in kitchen
	m1.UpdateBlobPositions([]struct {
		ID     int
		X, Y, Z float64
	}{
		{ID: 1, X: 5, Y: 5, Z: 1},
		{ID: 2, X: 6, Y: 6, Z: 1},
	})

	// Persist on "shutdown"
	if err := m1.PersistOccupancy(); err != nil {
		t.Fatalf("PersistOccupancy: %v", err)
	}
	m1.Close()

	// Phase 2: "Restart" — open same DB, reconcile
	m2, err := NewManager(dbPath, tz)
	if err != nil {
		t.Fatalf("NewManager (phase 2): %v", err)
	}
	defer m2.Close()

	occ := m2.GetZoneOccupancy("kitchen")
	if occ == nil {
		t.Fatal("expected occupancy for kitchen, got nil")
	}
	if occ.Count != 2 {
		t.Errorf("got count %d, want 2", occ.Count)
	}
	if occ.Status != OccupancyUncertain {
		t.Errorf("got status %s, want uncertain", occ.Status)
	}
}

func TestEndToEnd_RestoreWithCrossings(t *testing.T) {
	dbPath := testDB(t)
	tz := time.UTC

	// Phase 1: Create zone, set occupancy, add crossing, persist
	m1, err := NewManager(dbPath, tz)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	zone := &Zone{
		ID: "kitchen", Name: "Kitchen",
		MinX: 0, MinY: 0, MinZ: 0,
		MaxX: 10, MaxY: 10, MaxZ: 3,
		Enabled: true,
	}
	if err := m1.CreateZone(zone); err != nil {
		t.Fatalf("CreateZone: %v", err)
	}

	hallway := &Zone{
		ID: "hallway", Name: "Hallway",
		MinX: 10, MinY: 0, MinZ: 0,
		MaxX: 20, MaxY: 10, MaxZ: 3,
		Enabled: true,
	}
	if err := m1.CreateZone(hallway); err != nil {
		t.Fatalf("CreateZone: %v", err)
	}

	// Simulate 2 people in kitchen, persist
	m1.UpdateBlobPositions([]struct {
		ID     int
		X, Y, Z float64
	}{
		{ID: 1, X: 5, Y: 5, Z: 1},
		{ID: 2, X: 6, Y: 6, Z: 1},
	})
	m1.PersistOccupancy()

	// Simulate a crossing event (one person left kitchen to hallway after midnight)
	now := time.Now()
	m1.recordCrossing(CrossingEvent{
		PortalID:  "portal_1",
		BlobID:    1,
		Direction: 1, // a_to_b
		FromZone:  "kitchen",
		ToZone:    "hallway",
		Timestamp: now,
	})
	m1.Close()

	// Phase 2: Restart and reconcile
	m2, err := NewManager(dbPath, tz)
	if err != nil {
		t.Fatalf("NewManager (restart): %v", err)
	}
	defer m2.Close()

	kitchenOcc := m2.GetZoneOccupancy("kitchen")
	if kitchenOcc == nil {
		t.Fatal("nil kitchen occupancy")
	}
	if kitchenOcc.Count != 1 {
		t.Errorf("kitchen: got count %d, want 1 (was 2, one left via portal)", kitchenOcc.Count)
	}

	hallwayOcc := m2.GetZoneOccupancy("hallway")
	if hallwayOcc == nil {
		t.Fatal("nil hallway occupancy")
	}
	if hallwayOcc.Count != 1 {
		t.Errorf("hallway: got count %d, want 1 (entered via portal)", hallwayOcc.Count)
	}
}

// --- GetOccupancyStatus tests ---

func TestGetOccupancyStatus(t *testing.T) {
	m, cleanup := setupManager(t, time.UTC)
	defer cleanup()

	zone := &Zone{
		ID: "kitchen", Name: "Kitchen",
		MinX: 0, MinY: 0, MinZ: 0,
		MaxX: 10, MaxY: 10, MaxZ: 3,
		Enabled: true,
	}
	if err := m.CreateZone(zone); err != nil {
		t.Fatalf("CreateZone: %v", err)
	}

	m.mu.Lock()
	m.occupancy["kitchen"] = &ZoneOccupancy{
		ZoneID: "kitchen",
		Count:  2,
		Status: OccupancyUncertain,
	}
	m.mu.Unlock()

	status := m.GetOccupancyStatus()
	if status["kitchen"] != OccupancyUncertain {
		t.Errorf("got %s, want uncertain", status["kitchen"])
	}
}

// --- IsReconciled tests ---

func TestIsReconciled_NoZones(t *testing.T) {
	m, cleanup := setupManager(t, time.UTC)
	defer cleanup()

	// No zones, no occupancy — should be reconciled (nothing to reconcile)
	if !m.IsReconciled() {
		t.Error("expected reconciled with no zones")
	}
}

// --- Helper ---

// nowMsSinceMidnight returns a Unix ms timestamp the given duration after midnight today.
func nowMsSinceMidnight(d time.Duration) int64 {
	now := time.Now().UTC()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	return midnight.Add(d).UnixMilli()
}
