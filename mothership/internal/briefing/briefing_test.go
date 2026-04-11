// Package briefing provides tests for the morning briefing generator.
package briefing

import (
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// mockZoneProvider implements ZoneProvider for testing.
type mockZoneProvider struct {
	zones     map[int]string
	occupancy map[int]int
	people    map[int][]string
}

func (m *mockZoneProvider) GetZoneName(id int) string {
	if m.zones == nil {
		return ""
	}
	return m.zones[id]
}

func (m *mockZoneProvider) GetZoneOccupancy(zoneID int) int {
	if m.occupancy == nil {
		return 0
	}
	return m.occupancy[zoneID]
}

func (m *mockZoneProvider) GetPeopleInZone(zoneID int) []string {
	if m.people == nil {
		return nil
	}
	return m.people[zoneID]
}

// mockPersonProvider implements PersonProvider for testing.
type mockPersonProvider struct {
	peopleHome []string
	lastSeen   map[string]time.Time
	zones      map[string]string
}

func (m *mockPersonProvider) GetPeopleHome() []string {
	if m.peopleHome == nil {
		return nil
	}
	return m.peopleHome
}

func (m *mockPersonProvider) GetPersonLastSeen(person string) time.Time {
	if m.lastSeen == nil {
		return time.Time{}
	}
	return m.lastSeen[person]
}

func (m *mockPersonProvider) GetPersonZone(person string) string {
	if m.zones == nil {
		return ""
	}
	return m.zones[person]
}

// mockPredictionProvider implements PredictionProvider for testing.
type mockPredictionProvider struct {
	predictions  map[string]mockPrediction
	daysComplete map[string]int
	modelReady   map[string]bool
}

type mockPrediction struct {
	zone        string
	probability float64
}

func (m *mockPredictionProvider) GetPrediction(person string, horizonMinutes int) (string, float64, bool) {
	if m.predictions == nil {
		return "", 0, false
	}
	p, ok := m.predictions[person]
	if !ok {
		return "", 0, false
	}
	return p.zone, p.probability, true
}

func (m *mockPredictionProvider) GetDaysComplete(person string) int {
	if m.daysComplete == nil {
		return 0
	}
	return m.daysComplete[person]
}

func (m *mockPredictionProvider) IsModelReady(person string) bool {
	if m.modelReady == nil {
		return false
	}
	return m.modelReady[person]
}

// mockHealthProvider implements HealthProvider for testing.
type mockHealthProvider struct {
	quality       float64
	online        int
	total         int
	accuracyDelta float64
	feedbackCount int
}

func (m *mockHealthProvider) GetDetectionQuality() float64 {
	return m.quality
}

func (m *mockHealthProvider) GetNodeCount() (int, int) {
	return m.online, m.total
}

func (m *mockHealthProvider) GetAccuracyDelta() (float64, int) {
	return m.accuracyDelta, m.feedbackCount
}

func setupTestDB(t *testing.T) (*sql.DB, string) {
	f, err := os.CreateTemp("", "briefing-test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	dbPath := f.Name()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}

	// Create schema
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS briefings (
			date TEXT PRIMARY KEY,
			person TEXT,
			content TEXT NOT NULL,
			generated_at INTEGER NOT NULL,
			sections_json TEXT
		);

		CREATE TABLE IF NOT EXISTS sleep_records (
			id INTEGER PRIMARY KEY,
			person TEXT,
			zone_id INTEGER,
			date TEXT NOT NULL,
			duration_min INTEGER,
			onset_latency_min REAL,
			restlessness REAL,
			breathing_rate_avg REAL,
			breathing_regularity REAL,
			breathing_anomaly INTEGER,
			breathing_samples_json TEXT,
			summary_json TEXT
		);

		CREATE TABLE IF NOT EXISTS events (
			id INTEGER PRIMARY KEY,
			timestamp_ms INTEGER NOT NULL,
			type TEXT NOT NULL,
			zone TEXT,
			person TEXT,
			blob_id INTEGER,
			detail_json TEXT,
			severity TEXT NOT NULL DEFAULT 'info'
		);
	`)
	if err != nil {
		t.Fatal(err)
	}

	return db, dbPath
}

func TestBriefing_GenerateEmpty(t *testing.T) {
	db, dbPath := setupTestDB(t)
	defer db.Close()
	defer os.Remove(dbPath)

	g, err := NewGenerator(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	b, err := g.Generate("2024-03-15", "")
	if err != nil {
		t.Fatal(err)
	}

	if b.Content == "" {
		t.Error("expected non-empty content for degenerate case")
	}

	if b.Date != "2024-03-15" {
		t.Errorf("expected date 2024-03-15, got %s", b.Date)
	}
}

func TestBriefing_GenerateWithSleep(t *testing.T) {
	db, dbPath := setupTestDB(t)
	defer db.Close()
	defer os.Remove(dbPath)

	// Insert a sleep record
	_, err := db.Exec(`
		INSERT INTO sleep_records (date, person, duration_min, restlessness, breathing_rate_avg, breathing_regularity)
		VALUES ('2024-03-15', 'Alice', 480, 0.5, 14.0, 0.08)
	`)
	if err != nil {
		t.Fatal(err)
	}

	g, err := NewGenerator(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	b, err := g.Generate("2024-03-15", "Alice")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(b.Content, "slept") {
		t.Error("expected content to mention sleep")
	}
}

func TestBriefing_SaveAndGet(t *testing.T) {
	db, dbPath := setupTestDB(t)
	defer db.Close()
	defer os.Remove(dbPath)

	g, err := NewGenerator(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	b := &Briefing{
		Date:        "2024-03-15",
		Person:      "Alice",
		Content:     "Test briefing content",
		GeneratedAt: time.Now().UnixMilli(),
	}

	err = g.Save(b)
	if err != nil {
		t.Fatal(err)
	}

	retrieved, err := g.Get("2024-03-15", "Alice")
	if err != nil {
		t.Fatal(err)
	}

	if retrieved.Content != b.Content {
		t.Errorf("expected content %q, got %q", b.Content, retrieved.Content)
	}
}

func TestBriefing_ShouldGenerate(t *testing.T) {
	db, dbPath := setupTestDB(t)
	defer db.Close()
	defer os.Remove(dbPath)

	g, err := NewGenerator(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	// Initially should generate
	if !g.ShouldGenerate("2024-03-15", "") {
		t.Error("expected ShouldGenerate to return true for new date")
	}

	// After saving, should not generate
	b := &Briefing{
		Date:        "2024-03-15",
		Content:     "Test",
		GeneratedAt: time.Now().UnixMilli(),
	}
	if err := g.Save(b); err != nil {
		t.Fatal(err)
	}

	if g.ShouldGenerate("2024-03-15", "") {
		t.Error("expected ShouldGenerate to return false after saving")
	}
}

func TestBriefing_GenerateWithAlerts(t *testing.T) {
	db, dbPath := setupTestDB(t)
	defer db.Close()
	defer os.Remove(dbPath)

	// Insert a fall alert event
	nightStart := time.Date(2024, 3, 14, 18, 0, 0, 0, time.Local)
	t.Logf("Inserting event at %s (%d)", nightStart, nightStart.UnixMilli())
	_, err := db.Exec(`
		INSERT INTO events (timestamp_ms, type, zone, person, severity)
		VALUES (?, 'fall_alert', 'Bedroom', 'Alice', 'alert')
	`, nightStart.UnixMilli())
	if err != nil {
		t.Fatal(err)
	}

	// Verify the event was inserted
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("Events in database: %d", count)

	g, err := NewGenerator(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()

	b, err := g.Generate("2024-03-15", "Alice")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(b.Content, "Fall") {
		t.Logf("Generated briefing content: %q", b.Content)
		t.Error("expected content to mention fall alert")
	}
}
