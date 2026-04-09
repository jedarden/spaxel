// Package timeline provides persistent storage for timeline events.
// It subscribes to the EventBus and writes events to SQLite asynchronously
// using a buffered queue with drop-oldest behavior to ensure publishers are never blocked.
package timeline

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/spaxel/mothership/internal/eventbus"
)

const (
	// queueSize is the buffer size for the event queue.
	queueSize = 1000

	// flushInterval is how often to flush queued events to SQLite.
	flushInterval = 100 * time.Millisecond

	// flushBatchSize is the maximum number of events to write in one transaction.
	flushBatchSize = 100
)

// Event represents an event queued for storage.
type Event struct {
	Type        string
	TimestampMs int64
	Zone        string
	Person      string
	BlobID      int
	Detail      interface{}
	Severity    string
}

// Storage provides asynchronous timeline event storage.
// It subscribes to the EventBus and writes events to SQLite
// without blocking publishers.
type Storage struct {
	db        *sql.DB
	queue     chan Event
	done      chan struct{}
	wg        sync.WaitGroup
	mu        sync.Mutex
	dropped   int // Counter for dropped events (for metrics)
	lastWarn  time.Time
}

// New creates a new timeline storage subscriber.
// It subscribes to the default EventBus and starts a goroutine
// that writes events to SQLite.
func New(db *sql.DB) *Storage {
	s := &Storage{
		db:    db,
		queue: make(chan Event, queueSize),
		done:  make(chan struct{}),
	}

	// Subscribe to all events from the EventBus
	eventbus.SubscribeDefault(func(e eventbus.Event) {
		s.enqueue(e)
	})

	// Start the flush goroutine
	s.wg.Add(1)
	go s.flusher()

	return s
}

// enqueue adds an event to the queue, dropping oldest if full.
// This is called from the EventBus subscriber callback and must never block.
func (s *Storage) enqueue(e eventbus.Event) {
	select {
	case s.queue <- Event{
		Type:        e.Type,
		TimestampMs: e.TimestampMs,
		Zone:        e.Zone,
		Person:      e.Person,
		BlobID:      e.BlobID,
		Detail:      e.Detail,
		Severity:    e.Severity,
	}:
		// Event queued successfully
	default:
		// Queue is full, drop oldest event
		s.mu.Lock()
		s.dropped++
		s.mu.Unlock()

		// Warn about overflow at most once per minute
		now := time.Now()
		s.mu.Lock()
		shouldWarn := now.Sub(s.lastWarn) > time.Minute
		if shouldWarn {
			s.lastWarn = now
		}
		s.mu.Unlock()

		if shouldWarn {
			log.Printf("[WARN] Timeline storage queue overflow (dropped oldest event, %d total dropped)", s.dropped)
		}

		// Drop oldest by receiving and discarding one, then sending the new one
		<-s.queue
		s.queue <- Event{
			Type:        e.Type,
			TimestampMs: e.TimestampMs,
			Zone:        e.Zone,
			Person:      e.Person,
			BlobID:      e.BlobID,
			Detail:      e.Detail,
			Severity:    e.Severity,
		}
	}
}

// flusher runs in a goroutine and periodically flushes queued events to SQLite.
func (s *Storage) flusher() {
	defer s.wg.Done()

	ticker := time.NewTicker(flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.flush()
		case <-s.done:
			// Flush remaining events before shutdown
			s.flush()
			return
		}
	}
}

// flush writes up to flushBatchSize events from the queue to SQLite.
func (s *Storage) flush() {
	batch := make([]Event, 0, flushBatchSize)

	// Drain up to batch size from queue
	for i := 0; i < flushBatchSize; i++ {
		select {
		case e := <-s.queue:
			batch = append(batch, e)
		default:
			// Queue is empty
			break
		}
	}

	if len(batch) == 0 {
		return
	}

	// Write batch to SQLite in a single transaction
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		log.Printf("[ERROR] Timeline storage begin transaction: %v", err)
		return
	}

	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO events (timestamp_ms, type, zone, person, blob_id, detail_json, severity)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		log.Printf("[ERROR] Timeline storage prepare statement: %v", err)
		return
	}
	defer stmt.Close()

	for _, e := range batch {
		var detailJSON string
		if e.Detail != nil {
			// Detail may be a string (from eventbus) or a map/interface{}
			switch v := e.Detail.(type) {
			case string:
				detailJSON = v
			case []byte:
				detailJSON = string(v)
			default:
				j, err := json.Marshal(e.Detail)
				if err != nil {
					log.Printf("[ERROR] Timeline storage marshal detail: %v", err)
					detailJSON = "{}"
				} else {
					detailJSON = string(j)
				}
			}
		}

		_, err := stmt.Exec(e.TimestampMs, e.Type, e.Zone, e.Person, e.BlobID, detailJSON, e.Severity)
		if err != nil {
			log.Printf("[ERROR] Timeline storage insert event: %v", err)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[ERROR] Timeline storage commit: %v", err)
		return
	}
}

// Close gracefully shuts down the storage, flushing all remaining events.
func (s *Storage) Close() error {
	close(s.done)
	s.wg.Wait()

	// Final flush to ensure all events are written
	s.flush()

	return nil
}

// Stats returns statistics about the storage queue.
func (s *Storage) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()

	return Stats{
		Queued:  len(s.queue),
		Dropped: s.dropped,
	}
}

// Stats represents storage queue statistics.
type Stats struct {
	Queued  int // Number of events currently in queue
	Dropped int // Total number of events dropped due to overflow
}
