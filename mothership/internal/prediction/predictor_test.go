package prediction

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func TestPredictor_PauseResumeUpdates(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "prediction.db")

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	store, err := NewModelStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create model store: %v", err)
	}

	predictor := NewPredictor(store)

	// Add some training data
	personID := "person1"
	zoneID := "zone1"
	hourOfWeek := HourOfWeek(time.Now())

	// Add transition samples
	for i := 0; i < 100; i++ {
		err := store.RecordTransition(ZoneTransition{
			PersonID:             personID,
			FromZoneID:           zoneID,
			ToZoneID:             zoneID,
			HourOfWeek:           hourOfWeek,
			DwellDurationMinutes: 10.0,
			Timestamp:            time.Now(),
		})
		if err != nil {
			t.Fatalf("Failed to add transition sample: %v", err)
		}
	}

	// Set up providers
	predictor.SetZoneProvider(&mockZoneProvider{
		zones: map[string]string{
			zoneID: "Living Room",
		},
	})

	predictor.SetPersonProvider(&mockPersonProvider{
		people: []struct {
			ID    string
			Name  string
			Color string
		}{
			{personID, "Person 1", "#ff0000"},
		},
	})

	predictor.SetPositionProvider(&mockPositionProvider{
		positions: map[string]struct {
			ZoneID    string
			EntryTime time.Time
		}{
			personID: {zoneID, time.Now().Add(-time.Minute)},
		},
	})

	// Initial update should work
	predictor.UpdatePredictions()
	predictions := predictor.GetPredictions()

	// Pause updates
	predictor.PauseUpdates()
	if !predictor.IsPaused() {
		t.Error("IsPaused should return true after PauseUpdates")
	}

	// UpdatePredictions while paused should be a no-op
	// This is hard to test directly since we can't observe the side effects,
	// but we can verify the method doesn't panic
	predictor.UpdatePredictions()

	// Resume updates
	predictor.ResumeUpdates()
	if predictor.IsPaused() {
		t.Error("IsPaused should return false after ResumeUpdates")
	}

	// UpdatePredictions after resume should work
	predictor.UpdatePredictions()
	newPredictions := predictor.GetPredictions()

	// The predictions should still be valid
	if len(newPredictions) != len(predictions) {
		t.Errorf("Prediction count changed: got %d, want %d", len(newPredictions), len(predictions))
	}
}

func TestPredictor_NilProviders(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "prediction.db")

	store, err := NewModelStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create model store: %v", err)
	}

	predictor := NewPredictor(store)

	// Pause and resume with nil providers should not panic
	predictor.PauseUpdates()
	predictor.UpdatePredictions()
	predictor.ResumeUpdates()
	predictor.UpdatePredictions()

	if predictor.IsPaused() {
		t.Error("Predictor should not be paused after resume")
	}
}

func TestPredictor_ConcurrentPauseUpdates(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "prediction.db")

	store, err := NewModelStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create model store: %v", err)
	}

	predictor := NewPredictor(store)

	// Set up minimal providers
	predictor.SetZoneProvider(&mockZoneProvider{
		zones: map[string]string{"zone1": "Zone 1"},
	})
	predictor.SetPersonProvider(&mockPersonProvider{
		people: []struct {
			ID    string
			Name  string
			Color string
		}{
			{"person1", "Person 1", "#ff0000"},
		},
	})
	predictor.SetPositionProvider(&mockPositionProvider{
		positions: map[string]struct {
			ZoneID    string
			EntryTime time.Time
		}{
			"person1": {ZoneID: "zone1", EntryTime: time.Now()},
		},
	})

	done := make(chan struct{})

	// Goroutine that constantly updates
	go func() {
		for i := 0; i < 100; i++ {
			predictor.UpdatePredictions()
			time.Sleep(time.Millisecond)
		}
		done <- struct{}{}
	}()

	// Goroutine that constantly pauses/resumes
	go func() {
		for i := 0; i < 50; i++ {
			predictor.PauseUpdates()
			time.Sleep(time.Millisecond)
			predictor.ResumeUpdates()
			time.Sleep(time.Millisecond)
		}
		done <- struct{}{}
	}()

	// Wait for both to complete
	<-done
	<-done

	// Final resume to ensure clean state
	predictor.ResumeUpdates()

	if predictor.IsPaused() {
		t.Error("Predictor should not be paused after concurrent operations")
	}
}

// Mock providers for testing

type mockZoneProvider struct {
	zones map[string]string
}

func (m *mockZoneProvider) GetZone(id string) (name string, ok bool) {
	name, ok = m.zones[id]
	return
}

type mockPersonProvider struct {
	people []struct {
		ID    string
		Name  string
		Color string
	}
}

func (m *mockPersonProvider) GetPerson(id string) (name, color string, ok bool) {
	for _, p := range m.people {
		if p.ID == id {
			return p.Name, p.Color, true
		}
	}
	return "", "", false
}

func (m *mockPersonProvider) GetAllPeople() ([]struct {
	ID    string
	Name  string
	Color string
}, error) {
	result := make([]struct {
		ID    string
		Name  string
		Color string
	}, len(m.people))
	copy(result, m.people)
	return result, nil
}

type mockPositionProvider struct {
	positions map[string]struct {
		ZoneID    string
		EntryTime time.Time
	}
}

func (m *mockPositionProvider) GetPersonPositions() map[string]struct {
	ZoneID    string
	EntryTime time.Time
} {
	return m.positions
}
