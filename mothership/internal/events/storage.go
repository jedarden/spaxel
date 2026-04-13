// Package events provides a timeline storage subscriber for persisting events to SQLite.
package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// bufferSize is the capacity of the event queue.
const bufferSize = 1000

// StorageSubscriber subscribes to EventBus and persists events to SQLite.
// It uses a buffered queue with drop-oldest behavior to ensure it never blocks publishers.
// Shutdown is two-phase: forwarders drain their EventBus channels first, then the worker
// drains the internal queue. This guarantees in-flight events are not silently dropped.
type StorageSubscriber struct {
	db              *sql.DB
	bus             *EventBus
	queue           chan EventPayload
	dropped         int64 // Counter for dropped events
	dropWarn        int64 // Counter for when we last logged a drop warning
	workerCtx       context.Context
	workerCancel    context.CancelFunc
	forwarderCtx    context.Context
	forwarderCancel context.CancelFunc
	forwarderWg     sync.WaitGroup // tracks forwarder goroutines
	workerWg        sync.WaitGroup // tracks worker goroutine
	mu              sync.Mutex
}

// NewStorageSubscriber creates a new timeline storage subscriber.
// The subscriber runs in a background goroutine until Stop() is called.
func NewStorageSubscriber(db *sql.DB, bus *EventBus) *StorageSubscriber {
	wCtx, wCancel := context.WithCancel(context.Background())
	fCtx, fCancel := context.WithCancel(context.Background())
	return &StorageSubscriber{
		db:              db,
		bus:             bus,
		queue:           make(chan EventPayload, bufferSize),
		workerCtx:       wCtx,
		workerCancel:    wCancel,
		forwarderCtx:    fCtx,
		forwarderCancel: fCancel,
	}
}

// Start begins consuming events from the EventBus and writing them to SQLite.
// It subscribes to all event types and returns immediately.
// The subscriber runs in a background goroutine until Stop() is called.
func (s *StorageSubscriber) Start() {
	// Subscribe to all event types
	eventTypes := []BusEventType{
		BusMotionDetected,
		BusMotionStopped,
		BusZoneTransition,
		BusZoneEntry,
		BusZoneExit,
		BusFallDetected,
		BusFallConfirmed,
		BusNodeConnected,
		BusNodeDisconnected,
		BusNodeReconnected,
		BusNodeStale,
		BusSystemStarted,
		BusSystemShutdown,
		BusConfigChanged,
		BusTriggerFired,
		BusTriggerCleared,
		BusBaselineUpdated,
		BusModelUpdated,
	}

	// Subscribe to each event type and forward to our queue
	for _, eventType := range eventTypes {
		ch := s.bus.Subscribe(eventType)
		s.forwarderWg.Add(1)
		go s.forwarder(ch)
	}

	// Start the storage worker
	s.workerWg.Add(1)
	go s.worker()
}

// forwarder reads events from a subscriber channel and forwards them to the queue.
// If the queue is full, it drops the oldest event (drop-oldest behavior).
// On shutdown (forwarderCtx cancelled), it drains any remaining buffered events from
// the EventBus channel before exiting so they are not silently lost.
func (s *StorageSubscriber) forwarder(ch <-chan EventPayload) {
	defer s.forwarderWg.Done()

	for {
		select {
		case <-s.forwarderCtx.Done():
			// Drain remaining events buffered in the EventBus channel before exiting
			for {
				select {
				case payload, ok := <-ch:
					if !ok {
						return
					}
					s.enqueue(payload)
				default:
					return
				}
			}
		case payload, ok := <-ch:
			if !ok {
				return
			}
			s.enqueue(payload)
		}
	}
}

// enqueue adds an event to the queue with drop-oldest behavior on overflow.
func (s *StorageSubscriber) enqueue(payload EventPayload) {
	s.mu.Lock()
	defer s.mu.Unlock()

	select {
	case s.queue <- payload:
		// Event queued successfully
	default:
		// Queue is full - drop oldest and log warning
		select {
		case <-s.queue:
			// Dropped one event
			s.dropped++
			s.dropWarn++
			// Log warning at most once per 100 drops to avoid spam
			if s.dropWarn%100 == 1 {
				log.Printf("[WARN] Timeline storage queue full (%d events), dropping oldest (total dropped: %d)",
					len(s.queue), s.dropped)
			}
		default:
			// Queue became empty between checks, should be rare
		}
		// Now enqueue the new event
		s.queue <- payload
	}
}

// worker processes events from the queue and writes them to SQLite.
func (s *StorageSubscriber) worker() {
	defer s.workerWg.Done()

	for {
		select {
		case <-s.workerCtx.Done():
			// Drain remaining events before exiting (forwarders have already finished)
			s.drain()
			return
		case payload := <-s.queue:
			if err := s.storeEvent(payload); err != nil {
				log.Printf("[ERROR] Failed to store event in timeline: %v", err)
			}
		}
	}
}

// storeEvent converts an EventBus payload to an Event record and inserts it into SQLite.
func (s *StorageSubscriber) storeEvent(payload EventPayload) error {
	event := s.convertPayload(payload)
	_, err := InsertEvent(s.db, event)
	return err
}

// convertPayload converts an EventBus payload to an Event record for storage.
func (s *StorageSubscriber) convertPayload(payload EventPayload) Event {
	base := Event{
		TimestampMs: payload.GetTimestamp().UnixMilli(),
		DetailJSON:  marshalDetail(payload),
		Severity:    SeverityInfo,
	}

	switch p := payload.(type) {
	case MotionDetectedPayload:
		base.Type = EventTypeDetection
		base.Zone = p.ZoneName
		base.Person = p.PersonName
		base.BlobID = p.BlobID
		base.Severity = SeverityInfo

	case MotionStoppedPayload:
		base.Type = EventTypeDetection
		base.Zone = p.ZoneName
		base.Person = p.PersonName
		base.BlobID = p.BlobID
		base.Severity = SeverityInfo

	case ZoneTransitionPayload:
		base.Type = EventTypePortalCrossing
		base.Zone = p.ToZoneName
		base.Person = p.PersonName
		base.BlobID = p.BlobID
		base.Severity = SeverityInfo

	case ZoneEntryPayload:
		base.Type = EventTypeZoneEntry
		base.Zone = p.ZoneName
		base.Person = p.PersonName
		base.BlobID = p.BlobID
		base.Severity = SeverityInfo

	case ZoneExitPayload:
		base.Type = EventTypeZoneExit
		base.Zone = p.ZoneName
		base.Person = p.PersonName
		base.BlobID = p.BlobID
		base.Severity = SeverityInfo

	case FallDetectedPayload:
		base.Type = EventTypeFallAlert
		base.Zone = p.ZoneName
		base.Person = p.PersonName
		base.BlobID = p.BlobID
		base.Severity = SeverityAlert

	case FallConfirmedPayload:
		base.Type = EventTypeFallAlert
		base.Zone = p.ZoneName
		base.Person = p.PersonName
		base.BlobID = p.BlobID
		base.Severity = SeverityCritical

	case NodeConnectedPayload:
		base.Type = EventTypeNodeOnline
		base.DetailJSON = marshalDetail(map[string]interface{}{
			"node_mac":        p.NodeMAC,
			"node_name":       p.NodeName,
			"firmware_version": p.FirmwareVer,
			"ip_address":      p.IPAddress,
		})
		base.Severity = SeverityInfo

	case NodeDisconnectedPayload:
		base.Type = EventTypeNodeOffline
		base.DetailJSON = marshalDetail(map[string]interface{}{
			"node_mac":      p.NodeMAC,
			"node_name":     p.NodeName,
			"was_online_ms": p.WasOnlineFor,
			"reason":        p.Reason,
		})
		base.Severity = SeverityWarning

	case NodeReconnectedPayload:
		base.Type = EventTypeNodeOnline
		base.DetailJSON = marshalDetail(map[string]interface{}{
			"node_mac":       p.NodeMAC,
			"node_name":      p.NodeName,
			"offline_for_ms": p.OfflineForMs,
		})
		base.Severity = SeverityInfo

	case NodeStalePayload:
		base.Type = EventTypeNodeOffline
		base.DetailJSON = marshalDetail(map[string]interface{}{
			"node_mac":      p.NodeMAC,
			"node_name":     p.NodeName,
			"last_health_ms": p.LastHealthMs,
		})
		base.Severity = SeverityWarning

	case SystemStartedPayload:
		base.Type = EventTypeSystem
		base.DetailJSON = marshalDetail(map[string]interface{}{
			"message":  "System started",
			"version":  p.Version,
			"start_time": p.StartTime.Format(time.RFC3339),
			"duration_ms": p.DurationMs,
		})
		base.Severity = SeverityInfo

	case SystemShutdownPayload:
		base.Type = EventTypeSystem
		base.DetailJSON = marshalDetail(map[string]interface{}{
			"message":     "System shutdown",
			"reason":      p.Reason,
			"duration_ms": p.DurationMs,
		})
		base.Severity = SeverityInfo

	case ConfigChangedPayload:
		base.Type = EventTypeSystem
		base.DetailJSON = marshalDetail(map[string]interface{}{
			"message":    "Configuration changed",
			"key":        p.Key,
			"old_value":  p.OldValue,
			"new_value":  p.NewValue,
			"changed_by": p.ChangedBy,
		})
		base.Severity = SeverityInfo

	case TriggerFiredPayload:
		base.Type = EventTypeTriggerFired
		base.Zone = p.ZoneName
		base.Person = p.PersonName
		base.BlobID = p.BlobID
		base.DetailJSON = marshalDetail(map[string]interface{}{
			"trigger_id":   p.TriggerID,
			"trigger_name": p.TriggerName,
			"condition":    p.Condition,
			"duration_s":   p.DurationS,
			"position":     p.Position,
		})
		base.Severity = SeverityInfo

	case TriggerClearedPayload:
		base.Type = EventTypeTriggerFired
		base.DetailJSON = marshalDetail(map[string]interface{}{
			"trigger_id":   p.TriggerID,
			"trigger_name": p.TriggerName,
			"duration_s":   p.DurationS,
		})
		base.Severity = SeverityInfo

	case BaselineUpdatedPayload:
		base.Type = EventTypeBaselineChanged
		base.DetailJSON = marshalDetail(map[string]interface{}{
			"link_id":      p.LinkID,
			"reason":       p.Reason,
			"confidence":   p.Confidence,
			"sample_count": p.SampleCount,
		})
		base.Severity = SeverityInfo

	case ModelUpdatedPayload:
		base.Type = EventTypeLearningMilestone
		base.Person = p.PersonID
		base.DetailJSON = marshalDetail(map[string]interface{}{
			"model_type":        p.ModelType,
			"zone_id":           p.ZoneID,
			"samples_added":     p.SamplesAdded,
			"total_samples":     p.TotalSamples,
			"accuracy_percent":  p.AccuracyPercent,
		})
		base.Severity = SeverityInfo

	default:
		base.Type = EventTypeSystem
		base.DetailJSON = marshalDetail(map[string]interface{}{
			"message": fmt.Sprintf("Unknown event type: %T", payload),
		})
		base.Severity = SeverityWarning
	}

	return base
}

// marshalDetail converts a payload to JSON detail, falling back to a simple map on error.
func marshalDetail(payload interface{}) string {
	// For complex payloads, marshal to JSON
	if detail, ok := payload.(interface{ MarshalJSON() ([]byte, error) }); ok {
		if data, err := detail.MarshalJSON(); err == nil && len(data) > 0 {
			return string(data)
		}
	}

	// Fallback to JSON marshaling
	data, err := json.Marshal(payload)
	if err != nil {
		// Last resort: return a simple error message
		return fmt.Sprintf(`{"error":"marshal failed: %s"}`, err)
	}
	return string(data)
}

// drain processes any remaining events in the queue after Stop() is called.
func (s *StorageSubscriber) drain() {
	s.mu.Lock()
	defer s.mu.Unlock()

	remaining := len(s.queue)
	if remaining == 0 {
		return
	}

	log.Printf("[INFO] Timeline storage draining %d remaining events", remaining)

	// Process all remaining events with a timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			log.Printf("[WARN] Timeline storage drain timeout, %d events remaining", len(s.queue))
			return
		case payload := <-s.queue:
			if err := s.storeEvent(payload); err != nil {
				log.Printf("[ERROR] Failed to store event during drain: %v", err)
			}
		default:
			// Queue is empty
			return
		}
	}
}

// Stop gracefully shuts down the storage subscriber.
// It uses two-phase shutdown: first all forwarders are stopped so they can drain
// their EventBus channels into the internal queue, then the worker is stopped so
// it can drain the internal queue to SQLite. This ensures no in-flight events are lost.
func (s *StorageSubscriber) Stop() {
	// Phase 1: stop forwarders and let them drain their EventBus channels
	s.forwarderCancel()
	s.forwarderWg.Wait()

	// Phase 2: stop worker (it will drain the internal queue)
	s.workerCancel()
	s.workerWg.Wait()

	log.Printf("[INFO] Timeline storage stopped (total events dropped: %d)", s.dropped)
}

// Stats returns statistics about the storage subscriber.
func (s *StorageSubscriber) Stats() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()

	return map[string]interface{}{
		"queue_size":    len(s.queue),
		"queue_capacity": bufferSize,
		"dropped_total": s.dropped,
	}
}
