package eventbus

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestPublishSync(t *testing.T) {
	bus := New()

	var received []Event
	bus.Subscribe(func(e Event) {
		received = append(received, e)
	})

	bus.PublishSync(Event{Type: "detection", Zone: "Kitchen"})
	bus.PublishSync(Event{Type: "zone_exit", Person: "Alice"})

	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}
	if received[0].Type != "detection" || received[0].Zone != "Kitchen" {
		t.Errorf("event 0 mismatch: %+v", received[0])
	}
	if received[1].Type != "zone_exit" || received[1].Person != "Alice" {
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
