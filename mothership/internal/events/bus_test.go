package events

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestEventBusSubscribePublish(t *testing.T) {
	bus := NewEventBus(10)

	ch := bus.Subscribe(BusMotionDetected)
	defer bus.Unsubscribe(BusMotionDetected, ch)

	payload := MotionDetectedPayload{
		Timestamp:  time.Now(),
		ZoneID:     "zone-1",
		ZoneName:   "Kitchen",
		BlobID:     1,
		Confidence: 0.85,
		Position:   Position{X: 1.0, Y: 2.0, Z: 0.9},
	}

	received := bus.Publish(payload)
	if received != 1 {
		t.Errorf("Publish() returned %d, want 1", received)
	}

	select {
	case event := <-ch:
		if got, ok := event.(MotionDetectedPayload); !ok {
			t.Errorf("received type %T, want MotionDetectedPayload", event)
		} else if got.ZoneID != "zone-1" {
			t.Errorf("ZoneID = %q, want zone-1", got.ZoneID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("timed out waiting for event")
	}
}

func TestEventBusMultipleSubscribers(t *testing.T) {
	bus := NewEventBus(10)

	ch1 := bus.Subscribe(BusMotionDetected)
	ch2 := bus.Subscribe(BusMotionDetected)
	ch3 := bus.Subscribe(BusMotionDetected)
	defer func() {
		bus.Unsubscribe(BusMotionDetected, ch1)
		bus.Unsubscribe(BusMotionDetected, ch2)
		bus.Unsubscribe(BusMotionDetected, ch3)
	}()

	payload := MotionDetectedPayload{
		Timestamp:  time.Now(),
		ZoneID:     "zone-1",
		ZoneName:   "Kitchen",
		BlobID:     1,
		Confidence: 0.85,
		Position:   Position{X: 1.0, Y: 2.0, Z: 0.9},
	}

	received := bus.Publish(payload)
	if received != 3 {
		t.Errorf("Publish() returned %d, want 3", received)
	}

	// All three subscribers should receive the event
	for i, ch := range []<-chan EventPayload{ch1, ch2, ch3} {
		select {
		case event := <-ch:
			if got, ok := event.(MotionDetectedPayload); !ok {
				t.Errorf("subscriber %d: received type %T, want MotionDetectedPayload", i, event)
			} else if got.ZoneID != "zone-1" {
				t.Errorf("subscriber %d: ZoneID = %q, want zone-1", i, got.ZoneID)
			}
		case <-time.After(100 * time.Millisecond):
			t.Errorf("subscriber %d: timed out waiting for event", i)
		}
	}
}

func TestEventBusPublishWithin10ms(t *testing.T) {
	bus := NewEventBus(100)

	// Subscribe with multiple subscribers
	const numSubscribers = 10
	var chans []<-chan EventPayload
	for i := 0; i < numSubscribers; i++ {
		ch := bus.Subscribe(BusFallDetected)
		chans = append(chans, ch)
		defer bus.Unsubscribe(BusFallDetected, ch)
	}

	payload := FallDetectedPayload{
		Timestamp:  time.Now(),
		ZoneID:     "zone-1",
		ZoneName:   "Bathroom",
		BlobID:     1,
		ZVelocity:  -2.5,
		Confidence: 0.95,
		Position:   Position{X: 1.0, Y: 1.0, Z: 0.3},
	}

	start := time.Now()
	received := bus.Publish(payload)
	elapsed := time.Since(start)

	if received != numSubscribers {
		t.Errorf("Publish() returned %d, want %d", received, numSubscribers)
	}

	// The publish itself should be very fast (non-blocking)
	if elapsed > 10*time.Millisecond {
		t.Errorf("Publish() took %v, want < 10ms", elapsed)
	}

	// Verify all subscribers received the event
	for i, ch := range chans {
		select {
		case <-ch:
			// Event received
		case <-time.After(100 * time.Millisecond):
			t.Errorf("subscriber %d: timed out waiting for event", i)
		}
	}
}

func TestEventBusDifferentEventTypes(t *testing.T) {
	bus := NewEventBus(10)

	motionCh := bus.Subscribe(BusMotionDetected)
	fallCh := bus.Subscribe(BusFallDetected)
	nodeCh := bus.Subscribe(BusNodeConnected)
	defer func() {
		bus.Unsubscribe(BusMotionDetected, motionCh)
		bus.Unsubscribe(BusFallDetected, fallCh)
		bus.Unsubscribe(BusNodeConnected, nodeCh)
	}()

	motionPayload := MotionDetectedPayload{
		Timestamp:  time.Now(),
		ZoneID:     "zone-1",
		ZoneName:   "Kitchen",
		BlobID:     1,
		Confidence: 0.85,
		Position:   Position{X: 1.0, Y: 2.0, Z: 0.9},
	}

	fallPayload := FallDetectedPayload{
		Timestamp:  time.Now(),
		ZoneID:     "zone-2",
		ZoneName:   "Bathroom",
		BlobID:     2,
		ZVelocity:  -2.5,
		Confidence: 0.95,
		Position:   Position{X: 0.5, Y: 0.5, Z: 0.3},
	}

	nodePayload := NodeConnectedPayload{
		Timestamp:   time.Now(),
		NodeMAC:     "AA:BB:CC:DD:EE:FF",
		NodeName:    "Kitchen North",
		FirmwareVer: "1.0.0",
		IPAddress:   "192.168.1.100",
	}

	// Publish different event types
	bus.Publish(motionPayload)
	bus.Publish(fallPayload)
	bus.Publish(nodePayload)

	// Verify motion subscriber only gets motion events
	select {
	case event := <-motionCh:
		if _, ok := event.(MotionDetectedPayload); !ok {
			t.Errorf("motionCh received type %T, want MotionDetectedPayload", event)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("motionCh timed out")
	}

	// Verify fall subscriber only gets fall events
	select {
	case event := <-fallCh:
		if _, ok := event.(FallDetectedPayload); !ok {
			t.Errorf("fallCh received type %T, want FallDetectedPayload", event)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("fallCh timed out")
	}

	// Verify node subscriber only gets node events
	select {
	case event := <-nodeCh:
		if _, ok := event.(NodeConnectedPayload); !ok {
			t.Errorf("nodeCh received type %T, want NodeConnectedPayload", event)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("nodeCh timed out")
	}
}

func TestEventBusUnsubscribe(t *testing.T) {
	bus := NewEventBus(10)

	ch := bus.Subscribe(BusMotionDetected)

	payload := MotionDetectedPayload{
		Timestamp:  time.Now(),
		ZoneID:     "zone-1",
		ZoneName:   "Kitchen",
		BlobID:     1,
		Confidence: 0.85,
		Position:   Position{X: 1.0, Y: 2.0, Z: 0.9},
	}

	// Publish before unsubscribe
	received := bus.Publish(payload)
	if received != 1 {
		t.Errorf("Publish() before unsubscribe returned %d, want 1", received)
	}

	// Consume the event
	<-ch

	// Unsubscribe
	bus.Unsubscribe(BusMotionDetected, ch)

	// Publish after unsubscribe
	received = bus.Publish(payload)
	if received != 0 {
		t.Errorf("Publish() after unsubscribe returned %d, want 0", received)
	}

	// Verify no more events
	select {
	case <-ch:
		t.Error("received event after unsubscribe")
	case <-time.After(50 * time.Millisecond):
		// Expected - no event should be received
	}
}

func TestEventBusChannelFull(t *testing.T) {
	bus := NewEventBus(1) // Very small buffer

	ch := bus.Subscribe(BusMotionDetected)
	defer bus.Unsubscribe(BusMotionDetected, ch)

	payload := MotionDetectedPayload{
		Timestamp:  time.Now(),
		ZoneID:     "zone-1",
		ZoneName:   "Kitchen",
		BlobID:     1,
		Confidence: 0.85,
		Position:   Position{X: 1.0, Y: 2.0, Z: 0.9},
	}

	// Fill the channel without consuming
	received := bus.Publish(payload)
	if received != 1 {
		t.Errorf("first Publish() returned %d, want 1", received)
	}

	// Second publish should be skipped (channel full)
	received = bus.Publish(payload)
	if received != 0 {
		t.Errorf("second Publish() with full channel returned %d, want 0", received)
	}

	// Consume the first event
	<-ch

	// Now publish should succeed again
	received = bus.Publish(payload)
	if received != 1 {
		t.Errorf("third Publish() returned %d, want 1", received)
	}
}

func TestEventBusPublishBlocking(t *testing.T) {
	bus := NewEventBus(1)

	ch := bus.Subscribe(BusMotionDetected)
	defer bus.Unsubscribe(BusMotionDetected, ch)

	payload := MotionDetectedPayload{
		Timestamp:  time.Now(),
		ZoneID:     "zone-1",
		ZoneName:   "Kitchen",
		BlobID:     1,
		Confidence: 0.85,
		Position:   Position{X: 1.0, Y: 2.0, Z: 0.9},
	}

	ctx := context.Background()
	received, err := bus.PublishBlocking(ctx, payload)
	if err != nil {
		t.Errorf("PublishBlocking() error = %v", err)
	}
	if received != 1 {
		t.Errorf("PublishBlocking() returned %d, want 1", received)
	}

	// Verify event was received
	select {
	case <-ch:
		// OK
	case <-time.After(100 * time.Millisecond):
		t.Error("timed out waiting for event")
	}
}

func TestEventBusPublishBlockingCancelled(t *testing.T) {
	bus := NewEventBus(1)

	// Fill the channel
	ch := bus.Subscribe(BusMotionDetected)
	defer bus.Unsubscribe(BusMotionDetected, ch)
	bus.Publish(MotionDetectedPayload{Timestamp: time.Now()})

	payload := MotionDetectedPayload{
		Timestamp:  time.Now(),
		ZoneID:     "zone-1",
		ZoneName:   "Kitchen",
		BlobID:     1,
		Confidence: 0.85,
		Position:   Position{X: 1.0, Y: 2.0, Z: 0.9},
	}

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	received, err := bus.PublishBlocking(ctx, payload)
	if err == nil {
		t.Error("PublishBlocking() with cancelled context should return error")
	}
	if received != 0 {
		t.Errorf("PublishBlocking() with cancelled context returned %d, want 0", received)
	}
}

func TestEventBusSubscriberCount(t *testing.T) {
	bus := NewEventBus(10)

	if count := bus.SubscriberCount(BusMotionDetected); count != 0 {
		t.Errorf("SubscriberCount() = %d, want 0", count)
	}

	ch1 := bus.Subscribe(BusMotionDetected)
	ch2 := bus.Subscribe(BusMotionDetected)

	if count := bus.SubscriberCount(BusMotionDetected); count != 2 {
		t.Errorf("SubscriberCount() = %d, want 2", count)
	}

	bus.Unsubscribe(BusMotionDetected, ch1)

	if count := bus.SubscriberCount(BusMotionDetected); count != 1 {
		t.Errorf("SubscriberCount() after unsubscribe = %d, want 1", count)
	}

	bus.Unsubscribe(BusMotionDetected, ch2)

	if count := bus.SubscriberCount(BusMotionDetected); count != 0 {
		t.Errorf("SubscriberCount() after all unsubscribe = %d, want 0", count)
	}
}

func TestEventBusClose(t *testing.T) {
	bus := NewEventBus(10)

	ch1 := bus.Subscribe(BusMotionDetected)
	ch2 := bus.Subscribe(BusFallDetected)

	bus.Close()

	// Channels should be closed
	select {
	case _, ok := <-ch1:
		if ok {
			t.Error("ch1 should be closed after Close()")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("ch1 should be closed immediately")
	}

	select {
	case _, ok := <-ch2:
		if ok {
			t.Error("ch2 should be closed after Close()")
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("ch2 should be closed immediately")
	}

	// Publish after close should deliver to no subscribers
	payload := MotionDetectedPayload{
		Timestamp:  time.Now(),
		ZoneID:     "zone-1",
		ZoneName:   "Kitchen",
		BlobID:     1,
		Confidence: 0.85,
		Position:   Position{X: 1.0, Y: 2.0, Z: 0.9},
	}

	if received := bus.Publish(payload); received != 0 {
		t.Errorf("Publish() after Close() returned %d, want 0", received)
	}
}

func TestEventBusConcurrentPublish(t *testing.T) {
	const numSubscribers = 10
	const numPublishers = 5
	const eventsPerPublisher = 100

	// Capacity must be >= total events published so non-blocking Publish doesn't drop
	bus := NewEventBus(numPublishers * eventsPerPublisher)

	var chans []<-chan EventPayload
	for i := 0; i < numSubscribers; i++ {
		ch := bus.Subscribe(BusMotionDetected)
		chans = append(chans, ch)
		defer bus.Unsubscribe(BusMotionDetected, ch)
	}

	var wg sync.WaitGroup
	wg.Add(numPublishers)

	// Start multiple publishers
	for p := 0; p < numPublishers; p++ {
		go func(publisherID int) {
			defer wg.Done()
			for i := 0; i < eventsPerPublisher; i++ {
				payload := MotionDetectedPayload{
					Timestamp:  time.Now(),
					ZoneID:     "zone-1",
					ZoneName:   "Kitchen",
					BlobID:     publisherID*eventsPerPublisher + i,
					Confidence: 0.85,
					Position:   Position{X: float64(i), Y: float64(i), Z: 0.9},
				}
				bus.Publish(payload)
			}
		}(p)
	}

	wg.Wait()

	// Count total received events across all subscribers
	var receivedWg sync.WaitGroup
	receivedCounts := make([]int, numSubscribers)

	for i, ch := range chans {
		receivedWg.Add(1)
		go func(subscriberID int, ch <-chan EventPayload) {
			defer receivedWg.Done()
			count := 0
			timeout := time.After(500 * time.Millisecond)
			for {
				select {
				case <-ch:
					count++
				case <-timeout:
					receivedCounts[subscriberID] = count
					return
				}
			}
		}(i, ch)
	}

	receivedWg.Wait()

	expectedTotal := numPublishers * eventsPerPublisher
	for i, count := range receivedCounts {
		if count != expectedTotal {
			t.Errorf("subscriber %d received %d events, want %d", i, count, expectedTotal)
		}
	}
}

// TestAllPayloadTypes verifies that all defined event types have a corresponding payload struct.
func TestAllPayloadTypes(t *testing.T) {
	payloads := []struct {
		name     string
		eventType BusEventType
		payload  EventPayload
	}{
		{"MotionDetected", BusMotionDetected, MotionDetectedPayload{Timestamp: time.Now()}},
		{"MotionStopped", BusMotionStopped, MotionStoppedPayload{Timestamp: time.Now()}},
		{"ZoneTransition", BusZoneTransition, ZoneTransitionPayload{Timestamp: time.Now()}},
		{"ZoneEntry", BusZoneEntry, ZoneEntryPayload{Timestamp: time.Now()}},
		{"ZoneExit", BusZoneExit, ZoneExitPayload{Timestamp: time.Now()}},
		{"FallDetected", BusFallDetected, FallDetectedPayload{Timestamp: time.Now()}},
		{"FallConfirmed", BusFallConfirmed, FallConfirmedPayload{Timestamp: time.Now()}},
		{"NodeConnected", BusNodeConnected, NodeConnectedPayload{Timestamp: time.Now()}},
		{"NodeDisconnected", BusNodeDisconnected, NodeDisconnectedPayload{Timestamp: time.Now()}},
		{"NodeReconnected", BusNodeReconnected, NodeReconnectedPayload{Timestamp: time.Now()}},
		{"NodeStale", BusNodeStale, NodeStalePayload{Timestamp: time.Now()}},
		{"SystemStarted", BusSystemStarted, SystemStartedPayload{Timestamp: time.Now()}},
		{"SystemShutdown", BusSystemShutdown, SystemShutdownPayload{Timestamp: time.Now()}},
		{"ConfigChanged", BusConfigChanged, ConfigChangedPayload{Timestamp: time.Now()}},
		{"TriggerFired", BusTriggerFired, TriggerFiredPayload{Timestamp: time.Now()}},
		{"TriggerCleared", BusTriggerCleared, TriggerClearedPayload{Timestamp: time.Now()}},
		{"BaselineUpdated", BusBaselineUpdated, BaselineUpdatedPayload{Timestamp: time.Now()}},
		{"ModelUpdated", BusModelUpdated, ModelUpdatedPayload{Timestamp: time.Now()}},
	}

	for _, tt := range payloads {
		t.Run(tt.name, func(t *testing.T) {
			// Verify EventType() returns the correct type
			if tt.payload.EventType() != tt.eventType {
				t.Errorf("EventType() = %v, want %v", tt.payload.EventType(), tt.eventType)
			}

			// Verify GetTimestamp() returns a non-zero time
			if tt.payload.GetTimestamp().IsZero() {
				t.Error("GetTimestamp() returned zero time")
			}

			// Verify payload can be published and received
			bus := NewEventBus(1)
			ch := bus.Subscribe(tt.eventType)
			defer bus.Unsubscribe(tt.eventType, ch)

			received := bus.Publish(tt.payload)
			if received != 1 {
				t.Errorf("Publish() returned %d, want 1", received)
			}

			select {
			case event := <-ch:
				// Verify we received the same type
				if event.EventType() != tt.eventType {
					t.Errorf("received EventType() = %v, want %v", event.EventType(), tt.eventType)
				}
			case <-time.After(100 * time.Millisecond):
				t.Error("timed out waiting for event")
			}
		})
	}
}

// TestPayloadInterfaces verifies that all payload structs implement EventPayload correctly.
func TestPayloadInterfaces(t *testing.T) {
	// This is a compile-time check that all payloads implement EventPayload
	var _ EventPayload = MotionDetectedPayload{}
	var _ EventPayload = MotionStoppedPayload{}
	var _ EventPayload = ZoneTransitionPayload{}
	var _ EventPayload = ZoneEntryPayload{}
	var _ EventPayload = ZoneExitPayload{}
	var _ EventPayload = FallDetectedPayload{}
	var _ EventPayload = FallConfirmedPayload{}
	var _ EventPayload = NodeConnectedPayload{}
	var _ EventPayload = NodeDisconnectedPayload{}
	var _ EventPayload = NodeReconnectedPayload{}
	var _ EventPayload = NodeStalePayload{}
	var _ EventPayload = SystemStartedPayload{}
	var _ EventPayload = SystemShutdownPayload{}
	var _ EventPayload = ConfigChangedPayload{}
	var _ EventPayload = TriggerFiredPayload{}
	var _ EventPayload = TriggerClearedPayload{}
	var _ EventPayload = BaselineUpdatedPayload{}
	var _ EventPayload = ModelUpdatedPayload{}
}

func TestEventBusZeroCapacity(t *testing.T) {
	bus := NewEventBus(0) // Unbuffered channels

	ch := bus.Subscribe(BusMotionDetected)
	defer bus.Unsubscribe(BusMotionDetected, ch)

	payload := MotionDetectedPayload{
		Timestamp:  time.Now(),
		ZoneID:     "zone-1",
		ZoneName:   "Kitchen",
		BlobID:     1,
		Confidence: 0.85,
		Position:   Position{X: 1.0, Y: 2.0, Z: 0.9},
	}

	// Publish should be skipped since there's no receiver waiting
	received := bus.Publish(payload)
	if received != 0 {
		t.Errorf("Publish() to unbuffered channel with no receiver returned %d, want 0", received)
	}

	// Now receive in a goroutine and publish
	done := make(chan struct{})
	go func() {
		<-ch
		close(done)
	}()

	time.Sleep(10 * time.Millisecond) // Let the goroutine block on receive

	received = bus.Publish(payload)
	if received != 1 {
		t.Errorf("Publish() to unbuffered channel with waiting receiver returned %d, want 1", received)
	}

	<-done
}
