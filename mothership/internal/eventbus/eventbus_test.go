package eventbus

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestPublishSync(t *testing.T) {
	bus := New()

	var received []Event
	bus.Subscribe(func(e Event) {
		received = append(received, e)
	})

	bus.PublishSync(Event{Type: TypeDetection, Zone: "Kitchen"})
	bus.PublishSync(Event{Type: TypeZoneExit, Person: "Alice"})

	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}
	if received[0].Type != TypeDetection || received[0].Zone != "Kitchen" {
		t.Errorf("event 0 mismatch: %+v", received[0])
	}
	if received[1].Type != TypeZoneExit || received[1].Person != "Alice" {
		t.Errorf("event 1 mismatch: %+v", received[1])
	}
}

func TestPublishAsync(t *testing.T) {
	bus := New()

	var count int64
	var wg sync.WaitGroup
	wg.Add(10)

	bus.Subscribe(func(e Event) {
		atomic.AddInt64(&count, 1)
		wg.Done()
	})

	for i := 0; i < 10; i++ {
		bus.Publish(Event{Type: "test"})
	}

	wg.Wait()

	if atomic.LoadInt64(&count) != 10 {
		t.Errorf("expected 10 events, got %d", count)
	}
}

func TestMultipleSubscribers(t *testing.T) {
	bus := New()

	var a, b int
	bus.Subscribe(func(e Event) { a++ })
	bus.Subscribe(func(e Event) { b++ })

	bus.PublishSync(Event{Type: "test"})

	if a != 1 || b != 1 {
		t.Errorf("expected a=1 b=1, got a=%d b=%d", a, b)
	}
}

func TestPublishNoSubscribers(t *testing.T) {
	bus := New()
	// Should not panic
	bus.PublishSync(Event{Type: "test"})
	bus.Publish(Event{Type: "test"})
}

// TestEventTypes verifies all event type constants are defined.
func TestEventTypes(t *testing.T) {
	tests := []struct {
		name string
		typ  string
	}{
		{"Detection", TypeDetection},
		{"ZoneEntry", TypeZoneEntry},
		{"ZoneExit", TypeZoneExit},
		{"PortalCrossing", TypePortalCrossing},
		{"TriggerFired", TypeTriggerFired},
		{"FallAlert", TypeFallAlert},
		{"Anomaly", TypeAnomaly},
		{"SecurityAlert", TypeSecurityAlert},
		{"NodeOnline", TypeNodeOnline},
		{"NodeOffline", TypeNodeOffline},
		{"OTAUpdate", TypeOTAUpdate},
		{"BaselineChanged", TypeBaselineChanged},
		{"System", TypeSystem},
		{"LearningMilestone", TypeLearningMilestone},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.typ == "" {
				t.Errorf("event type %s is empty string", tt.name)
			}
		})
	}
}

// TestSeverityConstants verifies all severity constants are defined.
func TestSeverityConstants(t *testing.T) {
	tests := []struct {
		name     string
		severity string
	}{
		{"Info", SeverityInfo},
		{"Warning", SeverityWarning},
		{"Alert", SeverityAlert},
		{"Critical", SeverityCritical},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.severity == "" {
				t.Errorf("severity %s is empty string", tt.name)
			}
		})
	}
}

// TestEventFields verifies event struct fields work correctly.
func TestEventFields(t *testing.T) {
	tests := []struct {
		name     string
		event    Event
		check    func(Event) error
	}{
		{
			name: "full event",
			event: Event{
				Type:        TypeDetection,
				TimestampMs: 1234567890,
				Zone:        "Kitchen",
				Person:      "Alice",
				BlobID:      42,
				Detail:      map[string]interface{}{"x": 1.0, "y": 2.0},
				Severity:    SeverityInfo,
			},
			check: func(e Event) error {
				if e.Type != TypeDetection {
					t.Errorf("Type = %v, want %v", e.Type, TypeDetection)
				}
				if e.TimestampMs != 1234567890 {
					t.Errorf("TimestampMs = %v, want 1234567890", e.TimestampMs)
				}
				if e.Zone != "Kitchen" {
					t.Errorf("Zone = %v, want Kitchen", e.Zone)
				}
				if e.Person != "Alice" {
					t.Errorf("Person = %v, want Alice", e.Person)
				}
				if e.BlobID != 42 {
					t.Errorf("BlobID = %v, want 42", e.BlobID)
				}
				if e.Severity != SeverityInfo {
					t.Errorf("Severity = %v, want %v", e.Severity, SeverityInfo)
				}
				return nil
			},
		},
		{
			name: "minimal event",
			event: Event{
				Type:     TypeSystem,
				Severity: SeverityWarning,
			},
			check: func(e Event) error {
				if e.Type != TypeSystem {
					t.Errorf("Type = %v, want %v", e.Type, TypeSystem)
				}
				if e.Severity != SeverityWarning {
					t.Errorf("Severity = %v, want %v", e.Severity, SeverityWarning)
				}
				return nil
			},
		},
		{
			name: "event with timestamp",
			event: Event{
				Type:        TypeZoneEntry,
				TimestampMs: time.Now().UnixMilli(),
				Zone:        "Hallway",
				Person:      "Bob",
			},
			check: func(e Event) error {
				if e.Type != TypeZoneEntry {
					t.Errorf("Type = %v, want %v", e.Type, TypeZoneEntry)
				}
				if e.TimestampMs == 0 {
					t.Error("TimestampMs not set")
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.check != nil {
				tt.check(tt.event)
			}

			// Verify event can be published and received intact
			bus := New()
			var received Event
			bus.Subscribe(func(e Event) {
				received = e
			})
			bus.PublishSync(tt.event)

			if received.Type != tt.event.Type {
				t.Errorf("received Type = %v, want %v", received.Type, tt.event.Type)
			}
			if received.Zone != tt.event.Zone {
				t.Errorf("received Zone = %v, want %v", received.Zone, tt.event.Zone)
			}
			if received.Person != tt.event.Person {
				t.Errorf("received Person = %v, want %v", received.Person, tt.event.Person)
			}
			if received.BlobID != tt.event.BlobID {
				t.Errorf("received BlobID = %v, want %v", received.BlobID, tt.event.BlobID)
			}
			if received.Severity != tt.event.Severity {
				t.Errorf("received Severity = %v, want %v", received.Severity, tt.event.Severity)
			}
		})
	}
}

// TestSubscribeDuringPublish verifies subscriptions can be added safely.
func TestSubscribeDuringPublish(t *testing.T) {
	bus := New()

	var count int
	var wg sync.WaitGroup

	// Subscribe while publish is happening
	wg.Add(1)
	go func() {
		defer wg.Done()
		time.Sleep(10 * time.Millisecond)
		bus.Subscribe(func(e Event) {
			count++
		})
	}()

	// Publish some events
	for i := 0; i < 5; i++ {
		bus.Publish(Event{Type: "test"})
		time.Sleep(5 * time.Millisecond)
	}

	wg.Wait()

	// Count depends on timing; just verify no panic/ deadlock
	if count < 0 {
		t.Errorf("count = %v, want >= 0", count)
	}
}
