// Package eventbus provides an internal publish/subscribe event bus
// so any package can emit events without direct dependency on other packages.
package eventbus

import (
	"sync"
)

// Event represents a timeline event published on the bus.
type Event struct {
	Type       string      // detection, zone_entry, zone_exit, etc.
	Zone       string      // optional zone name
	Person     string      // optional person name (BLE-identified)
	BlobID     int         // optional associated blob ID
	Detail     interface{} // optional detail payload (will be JSON-encoded by subscribers)
	Severity   string      // info, warning, alert, critical
}

// Subscriber receives events published on the bus.
// The callback receives the raw Event struct. Implementations may
// persist to SQLite, broadcast to WebSocket, etc.
type Subscriber func(Event)

// Bus is an internal publish/subscribe mechanism for timeline events.
// It is safe for concurrent use.
type Bus struct {
	mu          sync.RWMutex
	subscribers []Subscriber
}

// New creates a new event bus.
func New() *Bus {
	return &Bus{}
}

// Subscribe registers a callback that will be called for every published event.
// Subscriptions are permanent for the lifetime of the bus.
func (b *Bus) Subscribe(fn Subscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers = append(b.subscribers, fn)
}

// Publish sends an event to all subscribers.
// This is non-blocking: each subscriber is called in a separate goroutine.
func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	subs := make([]Subscriber, len(b.subscribers))
	copy(subs, b.subscribers)
	b.mu.RUnlock()

	for _, fn := range subs {
		go fn(e)
	}
}

// PublishSync sends an event to all subscribers, blocking until all complete.
// Use this when ordering matters (e.g., tests).
func (b *Bus) PublishSync(e Event) {
	b.mu.RLock()
	subs := make([]Subscriber, len(b.subscribers))
	copy(subs, b.subscribers)
	b.mu.RUnlock()

	for _, fn := range subs {
		fn(e)
	}
}
