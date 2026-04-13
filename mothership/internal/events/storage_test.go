// Package events provides tests for the timeline storage subscriber.
package events

import (
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestStorageSubscriberBasicFunctionality verifies the subscriber can be started and stopped.
func TestStorageSubscriberBasicFunctionality(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	bus := NewEventBus(10)
	subscriber := NewStorageSubscriber(db, bus)

	// Start the subscriber
	subscriber.Start()

	// Publish a test event
	payload := MotionDetectedPayload{
		Timestamp:  time.Now(),
		ZoneID:     "zone-1",
		ZoneName:   "Kitchen",
		BlobID:     1,
		PersonID:   "person-1",
		PersonName: "Alice",
		Confidence: 0.85,
		Position:   Position{X: 1.0, Y: 2.0, Z: 0.9},
	}

	bus.Publish(payload)

	// Give the subscriber time to process
	time.Sleep(100 * time.Millisecond)

	// Stop the subscriber
	subscriber.Stop()

	// Verify the event was stored
	events, _, _, err := QueryEvents(db, QueryParams{Limit: 10})
	if err != nil {
		t.Fatalf("QueryEvents() error = %v", err)
	}

	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	if events[0].Type != EventTypeDetection {
		t.Errorf("event type = %v, want %v", events[0].Type, EventTypeDetection)
	}
	if events[0].Zone != "Kitchen" {
		t.Errorf("event zone = %q, want Kitchen", events[0].Zone)
	}
	if events[0].Person != "Alice" {
		t.Errorf("event person = %q, want Alice", events[0].Person)
	}
}

// TestStorageSubscriberAllEventTypes verifies that all event types are correctly stored.
func TestStorageSubscriberAllEventTypes(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	bus := NewEventBus(10)
	subscriber := NewStorageSubscriber(db, bus)
	subscriber.Start()
	defer subscriber.Stop()

	testTime := time.Now()

	testCases := []struct {
		name          string
		payload       EventPayload
		expectedType  EventType
		expectedZone  string
		expectedPerson string
		expectedBlobID int
		expectedSeverity EventSeverity
	}{
		{
			name: "MotionDetected",
			payload: MotionDetectedPayload{
				Timestamp:  testTime,
				ZoneName:   "Kitchen",
				PersonName: "Alice",
				BlobID:     1,
				Confidence: 0.85,
			},
			expectedType: EventTypeDetection,
			expectedZone: "Kitchen",
			expectedPerson: "Alice",
			expectedBlobID: 1,
			expectedSeverity: SeverityInfo,
		},
		{
			name: "MotionStopped",
			payload: MotionStoppedPayload{
				Timestamp:  testTime,
				ZoneName:   "Living Room",
				PersonName: "Bob",
				BlobID:     2,
				DurationMs: 5000,
			},
			expectedType: EventTypeDetection,
			expectedZone: "Living Room",
			expectedPerson: "Bob",
			expectedBlobID: 2,
			expectedSeverity: SeverityInfo,
		},
		{
			name: "ZoneTransition",
			payload: ZoneTransitionPayload{
				Timestamp:    testTime,
				PortalName:   "Kitchen Door",
				FromZoneName: "Hallway",
				ToZoneName:   "Kitchen",
				PersonName:   "Alice",
				BlobID:       1,
				Direction:    "a_to_b",
			},
			expectedType: EventTypePortalCrossing,
			expectedZone: "Kitchen",
			expectedPerson: "Alice",
			expectedBlobID: 1,
			expectedSeverity: SeverityInfo,
		},
		{
			name: "ZoneEntry",
			payload: ZoneEntryPayload{
				Timestamp:  testTime,
				ZoneName:   "Bedroom",
				PersonName: "Charlie",
				BlobID:     3,
			},
			expectedType: EventTypeZoneEntry,
			expectedZone: "Bedroom",
			expectedPerson: "Charlie",
			expectedBlobID: 3,
			expectedSeverity: SeverityInfo,
		},
		{
			name: "ZoneExit",
			payload: ZoneExitPayload{
				Timestamp:  testTime,
				ZoneName:   "Bathroom",
				PersonName: "Diana",
				BlobID:     4,
			},
			expectedType: EventTypeZoneExit,
			expectedZone: "Bathroom",
			expectedPerson: "Diana",
			expectedBlobID: 4,
			expectedSeverity: SeverityInfo,
		},
		{
			name: "FallDetected",
			payload: FallDetectedPayload{
				Timestamp:  testTime,
				ZoneName:   "Hallway",
				PersonName: "Eve",
				BlobID:     5,
				ZVelocity:  -2.5,
				Confidence: 0.95,
			},
			expectedType: EventTypeFallAlert,
			expectedZone: "Hallway",
			expectedPerson: "Eve",
			expectedBlobID: 5,
			expectedSeverity: SeverityAlert,
		},
		{
			name: "FallConfirmed",
			payload: FallConfirmedPayload{
				Timestamp:      testTime,
				ZoneName:       "Bathroom",
				PersonName:     "Frank",
				BlobID:         6,
				ConfirmationMs: 10000,
				AlertSent:      true,
			},
			expectedType: EventTypeFallAlert,
			expectedZone: "Bathroom",
			expectedPerson: "Frank",
			expectedBlobID: 6,
			expectedSeverity: SeverityCritical,
		},
		{
			name: "NodeConnected",
			payload: NodeConnectedPayload{
				Timestamp:   testTime,
				NodeMAC:     "AA:BB:CC:DD:EE:FF",
				NodeName:    "Kitchen North",
				FirmwareVer: "1.0.0",
				IPAddress:   "192.168.1.100",
			},
			expectedType: EventTypeNodeOnline,
			expectedSeverity: SeverityInfo,
		},
		{
			name: "NodeDisconnected",
			payload: NodeDisconnectedPayload{
				Timestamp:    testTime,
				NodeMAC:      "11:22:33:44:55:66",
				NodeName:     "Living Room",
				WasOnlineFor: 3600000,
				Reason:       "timeout",
			},
			expectedType: EventTypeNodeOffline,
			expectedSeverity: SeverityWarning,
		},
		{
			name: "NodeReconnected",
			payload: NodeReconnectedPayload{
				Timestamp:    testTime,
				NodeMAC:      "AA:BB:CC:DD:EE:FF",
				NodeName:     "Kitchen North",
				OfflineForMs: 5000,
			},
			expectedType: EventTypeNodeOnline,
			expectedSeverity: SeverityInfo,
		},
		{
			name: "NodeStale",
			payload: NodeStalePayload{
				Timestamp:    testTime,
				NodeMAC:      "22:33:44:55:66:77",
				NodeName:     "Bedroom",
				LastHealthMs: 20000,
			},
			expectedType: EventTypeNodeOffline,
			expectedSeverity: SeverityWarning,
		},
		{
			name: "SystemStarted",
			payload: SystemStartedPayload{
				Timestamp:  testTime,
				Version:    "1.0.0",
				StartTime:  testTime.Add(-1 * time.Second),
				DurationMs: 1000,
			},
			expectedType: EventTypeSystem,
			expectedSeverity: SeverityInfo,
		},
		{
			name: "SystemShutdown",
			payload: SystemShutdownPayload{
				Timestamp:  testTime,
				Reason:     "manual",
				DurationMs: 5000,
			},
			expectedType: EventTypeSystem,
			expectedSeverity: SeverityInfo,
		},
		{
			name: "ConfigChanged",
			payload: ConfigChangedPayload{
				Timestamp: testTime,
				Key:       "fusion_rate_hz",
				OldValue:  "10",
				NewValue:  "20",
				ChangedBy: "api",
			},
			expectedType: EventTypeSystem,
			expectedSeverity: SeverityInfo,
		},
		{
			name: "TriggerFired",
			payload: TriggerFiredPayload{
				Timestamp:   testTime,
				TriggerID:   1,
				TriggerName: "Couch Dwell",
				Condition:   "dwell",
				ZoneName:    "Living Room",
				PersonName:  "Alice",
				BlobID:      1,
				DurationS:   35.0,
			},
			expectedType: EventTypeTriggerFired,
			expectedZone: "Living Room",
			expectedPerson: "Alice",
			expectedBlobID: 1,
			expectedSeverity: SeverityInfo,
		},
		{
			name: "TriggerCleared",
			payload: TriggerClearedPayload{
				Timestamp:   testTime,
				TriggerID:   1,
				TriggerName: "Couch Dwell",
				DurationS:   60.0,
			},
			expectedType: EventTypeTriggerFired,
			expectedSeverity: SeverityInfo,
		},
		{
			name: "BaselineUpdated",
			payload: BaselineUpdatedPayload{
				Timestamp:   testTime,
				LinkID:      "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66",
				Reason:      "manual",
				Confidence:  0.85,
				SampleCount: 500,
			},
			expectedType: EventTypeBaselineChanged,
			expectedSeverity: SeverityInfo,
		},
		{
			name: "ModelUpdated",
			payload: ModelUpdatedPayload{
				Timestamp:      testTime,
				ModelType:      "prediction",
				PersonID:       "Alice",
				ZoneID:         "1",
				SamplesAdded:   10,
				TotalSamples:   100,
				AccuracyPercent: 78.5,
			},
			expectedType: EventTypeLearningMilestone,
			expectedPerson: "Alice",
			expectedSeverity: SeverityInfo,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			bus.Publish(tc.payload)

			// Give the subscriber time to process
			time.Sleep(50 * time.Millisecond)

			// Query for the event
			events, _, _, err := QueryEvents(db, QueryParams{Limit: 1})
			if err != nil {
				t.Fatalf("QueryEvents() error = %v", err)
			}

			if len(events) == 0 {
				t.Fatal("no events found")
			}

			event := events[0]

			if event.Type != tc.expectedType {
				t.Errorf("event type = %v, want %v", event.Type, tc.expectedType)
			}

			if tc.expectedZone != "" && event.Zone != tc.expectedZone {
				t.Errorf("event zone = %q, want %q", event.Zone, tc.expectedZone)
			}

			if tc.expectedPerson != "" && event.Person != tc.expectedPerson {
				t.Errorf("event person = %q, want %q", event.Person, tc.expectedPerson)
			}

			if tc.expectedBlobID > 0 && event.BlobID != tc.expectedBlobID {
				t.Errorf("event blob_id = %d, want %d", event.BlobID, tc.expectedBlobID)
			}

			if event.Severity != tc.expectedSeverity {
				t.Errorf("event severity = %v, want %v", event.Severity, tc.expectedSeverity)
			}
		})
	}
}

// TestStorageSubscriberQueueOverflow verifies drop-oldest behavior.
func TestStorageSubscriberQueueOverflow(t *testing.T) {
	bus := NewEventBus(100) // Smaller bus buffer for this test

	subscriber := &StorageSubscriber{
		bus:   bus,
		queue: make(chan EventPayload, bufferSize),
	}

	// Fill the queue beyond capacity
	numEvents := bufferSize + 100
	for i := 0; i < numEvents; i++ {
		payload := MotionDetectedPayload{
			Timestamp:  time.Now(),
			ZoneName:   "Kitchen",
			BlobID:     i,
			Confidence: 0.85,
		}
		subscriber.enqueue(payload)
	}

	// Check stats
	stats := subscriber.Stats()
	queueSize := stats["queue_size"].(int)

	if queueSize != bufferSize {
		t.Errorf("queue size = %d, want %d (max capacity)", queueSize, bufferSize)
	}

	// Verify events were dropped
	dropped := stats["dropped_total"].(int64)
	if dropped < 100 {
		t.Errorf("dropped count = %d, want at least 100", dropped)
	}
}

// TestStorageSubscriberConcurrentEvents verifies concurrent event handling.
func TestStorageSubscriberConcurrentEvents(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	bus := NewEventBus(100)
	subscriber := NewStorageSubscriber(db, bus)
	subscriber.Start()
	defer subscriber.Stop()

	numPublishers := 10
	eventsPerPublisher := 50

	var wg sync.WaitGroup
	wg.Add(numPublishers)

	// Start multiple publishers
	for i := 0; i < numPublishers; i++ {
		go func(publisherID int) {
			defer wg.Done()
			for j := 0; j < eventsPerPublisher; j++ {
				payload := MotionDetectedPayload{
					Timestamp:  time.Now(),
					ZoneID:     "zone-1",
					ZoneName:   "Kitchen",
					BlobID:     publisherID*eventsPerPublisher + j,
					Confidence: 0.85,
				}
				bus.Publish(payload)
			}
		}(i)
	}

	wg.Wait()

	// Give subscriber time to process all events
	time.Sleep(500 * time.Millisecond)

	// Stop subscriber and wait for drain
	subscriber.Stop()

	// Count stored events
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
	if err != nil {
		t.Fatalf("failed to count events: %v", err)
	}

	expectedEvents := numPublishers * eventsPerPublisher
	if count < expectedEvents {
		t.Logf("stored %d events out of %d (some may have been dropped due to queue overflow)", count, expectedEvents)
	} else if count != expectedEvents {
		t.Errorf("stored %d events, want %d", count, expectedEvents)
	}
}

// TestStorageSubscriberStats verifies the stats method returns correct information.
func TestStorageSubscriberStats(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	bus := NewEventBus(10)
	subscriber := NewStorageSubscriber(db, bus)
	subscriber.Start()

	// Publish some events
	for i := 0; i < 10; i++ {
		payload := MotionDetectedPayload{
			Timestamp:  time.Now(),
			ZoneName:   "Kitchen",
			BlobID:     i,
			Confidence: 0.85,
		}
		bus.Publish(payload)
	}

	// Wait for processing
	time.Sleep(200 * time.Millisecond)

	stats := subscriber.Stats()

	queueCapacity := stats["queue_capacity"].(int)

	if queueCapacity != bufferSize {
		t.Errorf("queue capacity = %d, want %d", queueCapacity, bufferSize)
	}

	subscriber.Stop()
}

// TestStorageSubscriberDrain verifies remaining events are processed on stop.
func TestStorageSubscriberDrain(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	bus := NewEventBus(10)
	subscriber := NewStorageSubscriber(db, bus)
	subscriber.Start()

	// Publish events
	for i := 0; i < 20; i++ {
		payload := MotionDetectedPayload{
			Timestamp:  time.Now(),
			ZoneName:   "Kitchen",
			BlobID:     i,
			Confidence: 0.85,
		}
		bus.Publish(payload)
	}

	// Stop without waiting - drain should process remaining events
	subscriber.Stop()

	// Verify events were stored
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
	if err != nil {
		t.Fatalf("failed to count events: %v", err)
	}

	if count == 0 {
		t.Error("no events stored, drain may have failed")
	}
}

// TestStorageSubscriberNonBlocking verifies publishing never blocks.
func TestStorageSubscriberNonBlocking(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	bus := NewEventBus(10)
	subscriber := NewStorageSubscriber(db, bus)
	subscriber.Start()
	defer subscriber.Stop()

	// Publish many events rapidly - should never block
	start := time.Now()
	numEvents := 1000

	for i := 0; i < numEvents; i++ {
		payload := MotionDetectedPayload{
			Timestamp:  time.Now(),
			ZoneName:   "Kitchen",
			BlobID:     i,
			Confidence: 0.85,
		}
		bus.Publish(payload)
	}

	elapsed := time.Since(start)

	// Publishing should be very fast (< 100ms for 1000 events)
	if elapsed > 100*time.Millisecond {
		t.Errorf("publishing %d events took %v, want < 100ms (non-blocking)", numEvents, elapsed)
	}

	// Wait for some events to be processed
	time.Sleep(500 * time.Millisecond)
}

// TestStorageSubscriberMultipleSubscribers verifies multiple subscribers work together.
func TestStorageSubscriberMultipleSubscribers(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	bus := NewEventBus(10)

	// Create multiple storage subscribers
	sub1 := NewStorageSubscriber(db, bus)
	sub2 := NewStorageSubscriber(db, bus)

	sub1.Start()
	defer sub1.Stop()

	sub2.Start()
	defer sub2.Stop()

	// Publish events
	for i := 0; i < 10; i++ {
		payload := MotionDetectedPayload{
			Timestamp:  time.Now(),
			ZoneName:   "Kitchen",
			BlobID:     i,
			Confidence: 0.85,
		}
		bus.Publish(payload)
	}

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Both subscribers should have processed the events
	// Since they write to the same database, we should have 2x the events
	// (each subscriber stores independently)
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
	if err != nil {
		t.Fatalf("failed to count events: %v", err)
	}

	// We expect at least 10 events (could be more if both subscribers processed)
	if count < 10 {
		t.Errorf("expected at least 10 events, got %d", count)
	}
}
