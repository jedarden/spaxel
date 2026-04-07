// Package eventbus provides an internal publish/subscribe event bus
// so any package can emit events without direct dependency on other packages.
package eventbus

import (
	"sync"
)

// Event type constants. These match the EventType values from the events package.
const (
	TypeDetection        = "detection"
	TypeZoneEntry        = "zone_entry"
	TypeZoneExit         = "zone_exit"
	TypePortalCrossing   = "portal_crossing"
	TypeTriggerFired     = "trigger_fired"
	TypeFallAlert        = "fall_alert"
	TypeAnomaly          = "anomaly"
	TypeSecurityAlert    = "security_alert"
	TypeNodeOnline       = "node_online"
	TypeNodeOffline      = "node_offline"
	TypeOTAUpdate        = "ota_update"
	TypeBaselineChanged  = "baseline_changed"
	TypeSystem           = "system"
	TypeLearningMilestone = "learning_milestone"
)

// Severity level constants.
const (
	SeverityInfo     = "info"
	SeverityWarning  = "warning"
	SeverityAlert    = "alert"
	SeverityCritical = "critical"
)

// Event represents a timeline event published on the bus.
type Event struct {
	Type        string      // detection, zone_entry, zone_exit, etc.
	TimestampMs int64       // Unix milliseconds timestamp
	Zone        string      // optional zone name
	Person      string      // optional person name (BLE-identified)
	BlobID      int         // optional associated blob ID
	Detail      interface{} // optional detail payload (will be JSON-encoded by subscribers)
	Severity    string      // info, warning, alert, critical
}

// Subscriber receives events published on the bus.
// The callback receives the raw Event struct. Implementations may
// persist to SQLite, broadcast to WebSocket, etc.
type Subscriber func(Event)

// defaultBus is the global default event bus.
// Packages can call Default() to get a shared bus instance.
var (
	defaultBus *Bus
	once       sync.Once
)

// Default returns the global default event bus instance.
// It is safe to call from any goroutine.
func Default() *Bus {
	once.Do(func() {
		defaultBus = New()
	})
	return defaultBus
}

// PublishDefault is a convenience function that publishes to the default bus.
func PublishDefault(e Event) {
	Default().Publish(e)
}

// PublishDefaultSync is a convenience function that publishes to the default bus synchronously.
func PublishDefaultSync(e Event) {
	Default().PublishSync(e)
}

// SubscribeDefault is a convenience function that subscribes to the default bus.
func SubscribeDefault(fn Subscriber) {
	Default().Subscribe(fn)
}

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
