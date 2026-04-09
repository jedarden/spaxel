// Package events provides an internal typed event bus for subsystem communication.
package events

import (
	"context"
	"sync"
	"time"
)

// BusEventType represents the type of event being published on the internal bus.
type BusEventType string

const (
	// Motion events
	BusMotionDetected BusEventType = "motion_detected"
	BusMotionStopped  BusEventType = "motion_stopped"

	// Zone events
	BusZoneTransition BusEventType = "zone_transition"
	BusZoneEntry      BusEventType = "zone_entry"
	BusZoneExit       BusEventType = "zone_exit"

	// Safety events
	BusFallDetected  BusEventType = "fall_detected"
	BusFallConfirmed BusEventType = "fall_confirmed"

	// Node lifecycle events
	BusNodeConnected    BusEventType = "node_connected"
	BusNodeDisconnected BusEventType = "node_disconnected"
	BusNodeReconnected  BusEventType = "node_reconnected"
	BusNodeStale        BusEventType = "node_stale"

	// System events
	BusSystemStarted  BusEventType = "system_started"
	BusSystemShutdown BusEventType = "system_shutdown"
	BusConfigChanged  BusEventType = "config_changed"

	// Automation events
	BusTriggerFired   BusEventType = "trigger_fired"
	BusTriggerCleared BusEventType = "trigger_cleared"

	// Learning events
	BusBaselineUpdated BusEventType = "baseline_updated"
	BusModelUpdated    BusEventType = "model_updated"
)

// EventPayload is the interface that all event payloads must implement.
type EventPayload interface {
	EventType() BusEventType
	GetTimestamp() time.Time
}

// MotionDetectedPayload is emitted when motion is first detected after a period of stillness.
type MotionDetectedPayload struct {
	Timestamp  time.Time `json:"timestamp"`
	ZoneID     string    `json:"zone_id"`
	ZoneName   string    `json:"zone_name"`
	BlobID     int       `json:"blob_id"`
	PersonID   string    `json:"person_id,omitempty"`
	PersonName string    `json:"person_name,omitempty"`
	Confidence float64   `json:"confidence"`
	Position   Position  `json:"position"`
}

func (m MotionDetectedPayload) EventType() BusEventType { return BusMotionDetected }
func (m MotionDetectedPayload) GetTimestamp() time.Time   { return m.Timestamp }

// MotionStoppedPayload is emitted when motion ceases in a zone.
type MotionStoppedPayload struct {
	Timestamp  time.Time `json:"timestamp"`
	ZoneID     string    `json:"zone_id"`
	ZoneName   string    `json:"zone_name"`
	BlobID     int       `json:"blob_id"`
	DurationMs int64     `json:"duration_ms"`
	PersonID   string    `json:"person_id,omitempty"`
	PersonName string    `json:"person_name,omitempty"`
}

func (m MotionStoppedPayload) EventType() BusEventType { return BusMotionStopped }
func (m MotionStoppedPayload) GetTimestamp() time.Time   { return m.Timestamp }

// ZoneTransitionPayload is emitted when a blob crosses a portal between zones.
type ZoneTransitionPayload struct {
	Timestamp    time.Time `json:"timestamp"`
	PortalID     int       `json:"portal_id"`
	PortalName   string    `json:"portal_name"`
	FromZoneID   string    `json:"from_zone_id"`
	FromZoneName string    `json:"from_zone_name"`
	ToZoneID     string    `json:"to_zone_id"`
	ToZoneName   string    `json:"to_zone_name"`
	BlobID       int       `json:"blob_id"`
	PersonID     string    `json:"person_id,omitempty"`
	PersonName   string    `json:"person_name,omitempty"`
	Position     Position  `json:"position"`
	Direction    string    `json:"direction"` // "a_to_b" or "b_to_a"
}

func (z ZoneTransitionPayload) EventType() BusEventType { return BusZoneTransition }
func (z ZoneTransitionPayload) GetTimestamp() time.Time   { return z.Timestamp }

// ZoneEntryPayload is emitted when a blob enters a zone (not via portal).
type ZoneEntryPayload struct {
	Timestamp  time.Time `json:"timestamp"`
	ZoneID     string    `json:"zone_id"`
	ZoneName   string    `json:"zone_name"`
	BlobID     int       `json:"blob_id"`
	PersonID   string    `json:"person_id,omitempty"`
	PersonName string    `json:"person_name,omitempty"`
	Position   Position  `json:"position"`
}

func (z ZoneEntryPayload) EventType() BusEventType { return BusZoneEntry }
func (z ZoneEntryPayload) GetTimestamp() time.Time   { return z.Timestamp }

// ZoneExitPayload is emitted when a blob exits a zone (not via portal).
type ZoneExitPayload struct {
	Timestamp  time.Time `json:"timestamp"`
	ZoneID     string    `json:"zone_id"`
	ZoneName   string    `json:"zone_name"`
	BlobID     int       `json:"blob_id"`
	PersonID   string    `json:"person_id,omitempty"`
	PersonName string    `json:"person_name,omitempty"`
	Position   Position  `json:"position"`
}

func (z ZoneExitPayload) EventType() BusEventType { return BusZoneExit }
func (z ZoneExitPayload) GetTimestamp() time.Time   { return z.Timestamp }

// FallDetectedPayload is emitted when a potential fall is detected.
type FallDetectedPayload struct {
	Timestamp  time.Time `json:"timestamp"`
	ZoneID     string    `json:"zone_id"`
	ZoneName   string    `json:"zone_name"`
	BlobID     int       `json:"blob_id"`
	PersonID   string    `json:"person_id,omitempty"`
	PersonName string    `json:"person_name,omitempty"`
	Position   Position  `json:"position"`
	ZVelocity  float64   `json:"z_velocity"`
	Confidence float64   `json:"confidence"`
}

func (f FallDetectedPayload) EventType() BusEventType { return BusFallDetected }
func (f FallDetectedPayload) GetTimestamp() time.Time   { return f.Timestamp }

// FallConfirmedPayload is emitted when a fall is confirmed after the confirmation window.
type FallConfirmedPayload struct {
	Timestamp      time.Time `json:"timestamp"`
	ZoneID         string    `json:"zone_id"`
	ZoneName       string    `json:"zone_name"`
	BlobID         int       `json:"blob_id"`
	PersonID       string    `json:"person_id,omitempty"`
	PersonName     string    `json:"person_name,omitempty"`
	Position       Position  `json:"position"`
	ConfirmationMs int64     `json:"confirmation_ms"`
	AlertSent      bool      `json:"alert_sent"`
}

func (f FallConfirmedPayload) EventType() BusEventType { return BusFallConfirmed }
func (f FallConfirmedPayload) GetTimestamp() time.Time   { return f.Timestamp }

// NodeConnectedPayload is emitted when a node connects for the first time or after a long absence.
type NodeConnectedPayload struct {
	Timestamp   time.Time `json:"timestamp"`
	NodeMAC     string    `json:"node_mac"`
	NodeName    string    `json:"node_name"`
	FirmwareVer string    `json:"firmware_version"`
	IPAddress   string    `json:"ip_address"`
}

func (n NodeConnectedPayload) EventType() BusEventType { return BusNodeConnected }
func (n NodeConnectedPayload) GetTimestamp() time.Time   { return n.Timestamp }

// NodeDisconnectedPayload is emitted when a node disconnects unexpectedly.
type NodeDisconnectedPayload struct {
	Timestamp    time.Time `json:"timestamp"`
	NodeMAC      string    `json:"node_mac"`
	NodeName     string    `json:"node_name"`
	WasOnlineFor int64     `json:"was_online_for_ms"`
	Reason       string    `json:"reason,omitempty"` // "timeout", "error", "shutdown"
}

func (n NodeDisconnectedPayload) EventType() BusEventType { return BusNodeDisconnected }
func (n NodeDisconnectedPayload) GetTimestamp() time.Time   { return n.Timestamp }

// NodeReconnectedPayload is emitted when a node reconnects after a brief disconnection.
type NodeReconnectedPayload struct {
	Timestamp    time.Time `json:"timestamp"`
	NodeMAC      string    `json:"node_mac"`
	NodeName     string    `json:"node_name"`
	OfflineForMs int64     `json:"offline_for_ms"`
}

func (n NodeReconnectedPayload) EventType() BusEventType { return BusNodeReconnected }
func (n NodeReconnectedPayload) GetTimestamp() time.Time   { return n.Timestamp }

// NodeStalePayload is emitted when a node hasn't sent health updates within the expected interval.
type NodeStalePayload struct {
	Timestamp    time.Time `json:"timestamp"`
	NodeMAC      string    `json:"node_mac"`
	NodeName     string    `json:"node_name"`
	LastHealthMs int64     `json:"last_health_ms"`
}

func (n NodeStalePayload) EventType() BusEventType { return BusNodeStale }
func (n NodeStalePayload) GetTimestamp() time.Time   { return n.Timestamp }

// SystemStartedPayload is emitted when the mothership completes startup.
type SystemStartedPayload struct {
	Timestamp  time.Time `json:"timestamp"`
	Version    string    `json:"version"`
	StartTime  time.Time `json:"start_time"`
	DurationMs int64     `json:"duration_ms"`
}

func (s SystemStartedPayload) EventType() BusEventType { return BusSystemStarted }
func (s SystemStartedPayload) GetTimestamp() time.Time   { return s.Timestamp }

// SystemShutdownPayload is emitted when the mothership begins graceful shutdown.
type SystemShutdownPayload struct {
	Timestamp  time.Time `json:"timestamp"`
	Reason     string    `json:"reason,omitempty"`
	DurationMs int64     `json:"duration_ms"`
}

func (s SystemShutdownPayload) EventType() BusEventType { return BusSystemShutdown }
func (s SystemShutdownPayload) GetTimestamp() time.Time   { return s.Timestamp }

// ConfigChangedPayload is emitted when a configuration value changes.
type ConfigChangedPayload struct {
	Timestamp time.Time `json:"timestamp"`
	Key       string    `json:"key"`
	OldValue  string    `json:"old_value,omitempty"`
	NewValue  string    `json:"new_value,omitempty"`
	ChangedBy string    `json:"changed_by,omitempty"` // "api", "migration", etc.
}

func (c ConfigChangedPayload) EventType() BusEventType { return BusConfigChanged }
func (c ConfigChangedPayload) GetTimestamp() time.Time   { return c.Timestamp }

// TriggerFiredPayload is emitted when an automation trigger condition is met.
type TriggerFiredPayload struct {
	Timestamp   time.Time `json:"timestamp"`
	TriggerID   int       `json:"trigger_id"`
	TriggerName string    `json:"trigger_name"`
	Condition   string    `json:"condition"` // "enter", "leave", "dwell", "vacant", "count"
	ZoneID      string    `json:"zone_id,omitempty"`
	ZoneName    string    `json:"zone_name,omitempty"`
	BlobID      int       `json:"blob_id,omitempty"`
	PersonID    string    `json:"person_id,omitempty"`
	PersonName  string    `json:"person_name,omitempty"`
	Position    Position  `json:"position,omitempty"`
	DurationS   float64   `json:"duration_s,omitempty"` // For dwell conditions
}

func (t TriggerFiredPayload) EventType() BusEventType { return BusTriggerFired }
func (t TriggerFiredPayload) GetTimestamp() time.Time   { return t.Timestamp }

// TriggerClearedPayload is emitted when a trigger condition is no longer met.
type TriggerClearedPayload struct {
	Timestamp   time.Time `json:"timestamp"`
	TriggerID   int       `json:"trigger_id"`
	TriggerName string    `json:"trigger_name"`
	DurationS   float64   `json:"duration_s"`
}

func (t TriggerClearedPayload) EventType() BusEventType { return BusTriggerCleared }
func (t TriggerClearedPayload) GetTimestamp() time.Time   { return t.Timestamp }

// BaselineUpdatedPayload is emitted when a link baseline is updated.
type BaselineUpdatedPayload struct {
	Timestamp    time.Time `json:"timestamp"`
	LinkID       string    `json:"link_id"`
	Reason       string    `json:"reason"` // "manual", "drift", "schedule"
	Confidence   float64   `json:"confidence"`
	SampleCount  int       `json:"sample_count"`
}

func (b BaselineUpdatedPayload) EventType() BusEventType { return BusBaselineUpdated }
func (b BaselineUpdatedPayload) GetTimestamp() time.Time   { return b.Timestamp }

// ModelUpdatedPayload is emitted when a prediction model is updated.
type ModelUpdatedPayload struct {
	Timestamp       time.Time `json:"timestamp"`
	ModelType       string    `json:"model_type"` // "prediction", "anomaly", "weights"
	PersonID        string    `json:"person_id,omitempty"`
	ZoneID          string    `json:"zone_id,omitempty"`
	SamplesAdded    int       `json:"samples_added"`
	TotalSamples    int       `json:"total_samples"`
	AccuracyPercent float64   `json:"accuracy_percent,omitempty"`
}

func (m ModelUpdatedPayload) EventType() BusEventType { return BusModelUpdated }
func (m ModelUpdatedPayload) GetTimestamp() time.Time   { return m.Timestamp }

// EventBus provides a typed publish/subscribe mechanism for internal events.
// It supports multiple subscribers per event type with fan-out delivery.
type EventBus struct {
	mu         sync.RWMutex
	subscribers map[BusEventType][]chan EventPayload
	capacity   int // Buffer capacity for subscriber channels
}

// NewEventBus creates a new EventBus with the specified channel buffer capacity.
// A capacity of 0 creates unbuffered (synchronous) channels.
// Recommended capacity is 100-1000 for most use cases.
func NewEventBus(capacity int) *EventBus {
	return &EventBus{
		subscribers: make(map[BusEventType][]chan EventPayload),
		capacity:   capacity,
	}
}

// Subscribe registers a channel to receive events of the specified type.
// The channel will receive events via fan-out; each subscriber gets its own copy.
// Returns the channel that the caller should read from.
// The channel is buffered with the bus's capacity.
// It is the caller's responsibility to close the channel when done.
func (b *EventBus) Subscribe(eventType BusEventType) <-chan EventPayload {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan EventPayload, b.capacity)
	b.subscribers[eventType] = append(b.subscribers[eventType], ch)
	return ch
}

// Unsubscribe removes a channel from receiving events of the specified type.
// After calling Unsubscribe, the channel will no longer receive events
// and should be closed by the caller.
func (b *EventBus) Unsubscribe(eventType BusEventType, ch <-chan EventPayload) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs := b.subscribers[eventType]
	for i, sub := range subs {
		if sub == ch {
			// Remove the channel from the slice
			b.subscribers[eventType] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
}

// Publish sends an event payload to all subscribers of its type.
// The send is non-blocking; if a subscriber's channel is full, the event is skipped.
// This prevents a slow subscriber from blocking the entire system.
// Returns the number of subscribers that received the event.
func (b *EventBus) Publish(payload EventPayload) int {
	eventType := payload.EventType()

	b.mu.RLock()
	subs := b.subscribers[eventType]
	// Make a shallow copy to avoid holding the lock during sends
	subsCopy := make([]chan EventPayload, len(subs))
	copy(subsCopy, subs)
	b.mu.RUnlock()

	count := 0
	for _, ch := range subsCopy {
		select {
		case ch <- payload:
			count++
		default:
			// Channel is full, skip this subscriber
			// This prevents blocking on slow consumers
		}
	}
	return count
}

// PublishBlocking sends an event payload to all subscribers of its type,
// blocking until all subscribers have received the event or ctx is cancelled.
// Use this for critical events where delivery must be guaranteed.
// Returns the number of subscribers that received the event and any error.
func (b *EventBus) PublishBlocking(ctx context.Context, payload EventPayload) (int, error) {
	eventType := payload.EventType()

	b.mu.RLock()
	subs := b.subscribers[eventType]
	subsCopy := make([]chan EventPayload, len(subs))
	copy(subsCopy, subs)
	b.mu.RUnlock()

	count := 0
	for _, ch := range subsCopy {
		select {
		case ch <- payload:
			count++
		case <-ctx.Done():
			return count, ctx.Err()
		}
	}
	return count, nil
}

// SubscriberCount returns the number of active subscribers for the given event type.
func (b *EventBus) SubscriberCount(eventType BusEventType) int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.subscribers[eventType])
}

// Close closes all subscriber channels and releases resources.
// After Close, the bus should not be used.
func (b *EventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, subs := range b.subscribers {
		for _, ch := range subs {
			close(ch)
		}
	}
	b.subscribers = make(map[BusEventType][]chan EventPayload)
}
