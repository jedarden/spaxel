// Package shutdown implements the 10-step graceful shutdown sequence for Spaxel mothership.
package shutdown

import (
	"context"
	"database/sql"
	"log"
	"time"

	_ "modernc.org/sqlite"
)

// ShutdownMessage is sent to dashboard clients on shutdown.
type ShutdownMessage struct {
	Type          string `json:"type"`
	ReconnectInMS int    `json:"reconnect_in_ms"`
}

// BaselineFlusher can flush in-memory baselines to SQLite.
type BaselineFlusher interface {
	FlushBaselines(ctx context.Context) error
}

// RecordingSyncer can sync CSI recording buffer to disk.
type RecordingSyncer interface {
	Sync(ctx context.Context) error
}

// DashboardBroadcaster can broadcast shutdown messages to dashboard clients.
type DashboardBroadcaster interface {
	BroadcastShutdown(msg ShutdownMessage)
}

// NodeConnectionCloser can close node WebSocket connections.
type NodeConnectionCloser interface {
	CloseAllConnections() error
}

// EventWriter can write the system_stopped event.
type EventWriter interface {
	WriteSystemStoppedEvent() error
}

// Manager orchestrates the 10-step graceful shutdown sequence.
type Manager struct {
	baselineFlusher     BaselineFlusher
	processorManager     interface{} // *sigproc.ProcessorManager
	baselineStore        interface{} // *sigproc.BaselineStore
	recordingSyncer      RecordingSyncer
	dashboardBroadcaster DashboardBroadcaster
	nodeCloser           NodeConnectionCloser
	eventWriter          EventWriter
	db                   *sql.DB
	ingestionShutdowner  IngestionShutdowner
}

// IngestionShutdowner sets the ingestion server to shutting_down state.
type IngestionShutdowner interface {
	SetShuttingDown()
}

// NewManager creates a new shutdown manager.
func NewManager(db *sql.DB) *Manager {
	return &Manager{
		db: db,
	}
}

// SetBaselineFlusher sets the baseline flusher.
func (m *Manager) SetBaselineFlusher(f BaselineFlusher) {
	m.baselineFlusher = f
}

// SetBaselineComponents sets the processor manager and baseline store for proper flushing.
func (m *Manager) SetBaselineComponents(pm, baselineStore interface{}) {
	m.processorManager = pm
	m.baselineStore = baselineStore
}

// SetRecordingSyncer sets the recording syncer.
func (m *Manager) SetRecordingSyncer(r RecordingSyncer) {
	m.recordingSyncer = r
}

// SetDashboardBroadcaster sets the dashboard broadcaster.
func (m *Manager) SetDashboardBroadcaster(b DashboardBroadcaster) {
	m.dashboardBroadcaster = b
}

// SetNodeCloser sets the node connection closer.
func (m *Manager) SetNodeCloser(c NodeConnectionCloser) {
	m.nodeCloser = c
}

// SetEventWriter sets the event writer.
func (m *Manager) SetEventWriter(e EventWriter) {
	m.eventWriter = e
}

// SetIngestionShutdowner sets the ingestion shutdowner.
func (m *Manager) SetIngestionShutdowner(i IngestionShutdowner) {
	m.ingestionShutdowner = i
}

// Shutdown executes the 10-step graceful shutdown sequence with a 30s deadline.
// Returns true if all steps completed within the deadline, false otherwise.
func (m *Manager) Shutdown(ctx context.Context, cancelContext context.CancelFunc) bool {
	startTime := time.Now()
	deadline := 30 * time.Second

	log.Printf("[INFO] [SHUTDOWN] Initiating graceful shutdown sequence (30s deadline)")

	// Step 1/10: Set shutting_down=true; ingestion server returns HTTP 503
	log.Printf("[INFO] [SHUTDOWN] Step 1/10 — Setting ingestion server to shutting down (HTTP 503 for new connections)")
	if m.ingestionShutdowner != nil {
		m.ingestionShutdowner.SetShuttingDown()
	}
	elapsed := time.Since(startTime)
	log.Printf("[INFO] [SHUTDOWN] Step 1/10 completed in %v", elapsed)

	// Step 2/10: Broadcast shutdown message to dashboard WebSocket clients
	log.Printf("[INFO] [SHUTDOWN] Step 2/10 — Broadcasting shutdown message to dashboard clients")
	if m.dashboardBroadcaster != nil {
		msg := ShutdownMessage{
			Type:          "shutdown",
			ReconnectInMS: 30000,
		}
		m.dashboardBroadcaster.BroadcastShutdown(msg)
	}
	elapsed = time.Since(startTime)
	log.Printf("[INFO] [SHUTDOWN] Step 2/10 completed in %v", elapsed)

	// Check deadline
	if elapsed > deadline {
		log.Printf("[ERROR] [SHUTDOWN] Deadline exceeded after step 2 (%v > %v)", elapsed, deadline)
		return false
	}

	// Step 3/10: Cancel fusion loop context
	log.Printf("[INFO] [SHUTDOWN] Step 3/10 — Canceling fusion loop context")
	if cancelContext != nil {
		cancelContext()
	}
	elapsed = time.Since(startTime)
	log.Printf("[INFO] [SHUTDOWN] Step 3/10 completed in %v", elapsed)

	// Step 4/10: Drain signal processing pipeline (max 2s)
	log.Printf("[INFO] [SHUTDOWN] Step 4/10 — Draining signal processing pipeline (max 2s)")
	drainStart := time.Now()
	drainDeadline := 2 * time.Second

	// Give goroutines a chance to finish processing
	// We sleep briefly to allow in-flight frames to be processed
	select {
	case <-time.After(100 * time.Millisecond):
	case <-ctx.Done():
	}

	// Wait a bit more for the pipeline to drain
	drainElapsed := time.Since(drainStart)
	if drainElapsed < drainDeadline {
		select {
		case <-time.After(drainDeadline - drainElapsed):
		case <-ctx.Done():
		}
	}
	elapsed = time.Since(startTime)
	log.Printf("[INFO] [SHUTDOWN] Step 4/10 completed in %v", drainElapsed)

	// Check deadline
	if elapsed > deadline {
		log.Printf("[ERROR] [SHUTDOWN] Deadline exceeded after step 4 (%v > %v)", elapsed, deadline)
		return false
	}

	// Step 5/10: Flush in-memory baselines to SQLite in a single transaction
	log.Printf("[INFO] [SHUTDOWN] Step 5/10 — Flushing in-memory baselines to SQLite")

	// Try direct baselineStore flush first (preferred method)
	if m.baselineStore != nil && m.processorManager != nil {
		// Use type assertion to access the saveAll method
		type baselineSaver interface {
			SaveAllBaselines(map[string]interface{}) error
			SaveAllDiurnal(map[string]interface{}) error
		}
		type baselinesGetter interface {
			GetAllBaselines() map[string]interface{}
			GetAllDiurnalSnapshots() map[string]interface{}
		}

		if bs, ok := m.baselineStore.(baselineSaver); ok {
			if pm, ok := m.processorManager.(baselinesGetter); ok {
				baselines := pm.GetAllBaselines()
				diurnals := pm.GetAllDiurnalSnapshots()

				log.Printf("[INFO] Flushing %d EMA baselines and %d diurnal baselines to SQLite",
					len(baselines), len(diurnals))

				_, flushCancel := context.WithTimeout(ctx, 5*time.Second)
				defer flushCancel()

				if len(baselines) > 0 {
					if err := bs.SaveAllBaselines(baselines); err != nil {
						log.Printf("[ERROR] [SHUTDOWN] Failed to flush EMA baselines: %v", err)
					} else {
						log.Printf("[INFO] [SHUTDOWN] EMA baselines flushed successfully")
					}
				}

				if len(diurnals) > 0 {
					if err := bs.SaveAllDiurnal(diurnals); err != nil {
						log.Printf("[ERROR] [SHUTDOWN] Failed to flush diurnal baselines: %v", err)
					} else {
						log.Printf("[INFO] [SHUTDOWN] Diurnal baselines flushed successfully")
					}
				}
			}
		}
	} else if m.baselineFlusher != nil {
		// Fall back to baseline flusher interface
		flushCtx, flushCancel := context.WithTimeout(ctx, 5*time.Second)
		defer flushCancel()
		if err := m.baselineFlusher.FlushBaselines(flushCtx); err != nil {
			log.Printf("[ERROR] [SHUTDOWN] Failed to flush baselines: %v", err)
		} else {
			log.Printf("[INFO] [SHUTDOWN] Baselines flushed successfully")
		}
	}

	elapsed = time.Since(startTime)
	log.Printf("[INFO] [SHUTDOWN] Step 5/10 completed in %v", elapsed)

	// Step 6/10: Sync CSI recording buffer to disk
	log.Printf("[INFO] [SHUTDOWN] Step 6/10 — Syncing CSI recording buffer to disk")
	if m.recordingSyncer != nil {
		syncCtx, syncCancel := context.WithTimeout(ctx, 5*time.Second)
		defer syncCancel()
		if err := m.recordingSyncer.Sync(syncCtx); err != nil {
			log.Printf("[ERROR] [SHUTDOWN] Failed to sync recording buffer: %v", err)
		} else {
			log.Printf("[INFO] [SHUTDOWN] Recording buffer synced successfully")
		}
	}
	elapsed = time.Since(startTime)
	log.Printf("[INFO] [SHUTDOWN] Step 6/10 completed in %v", elapsed)

	// Check deadline
	if elapsed > deadline {
		log.Printf("[ERROR] [SHUTDOWN] Deadline exceeded after step 6 (%v > %v)", elapsed, deadline)
		return false
	}

	// Step 7/10: Close all node WebSocket connections with normal close frame
	log.Printf("[INFO] [SHUTDOWN] Step 7/10 — Closing all node WebSocket connections")
	if m.nodeCloser != nil {
		if err := m.nodeCloser.CloseAllConnections(); err != nil {
			log.Printf("[ERROR] [SHUTDOWN] Failed to close node connections: %v", err)
		} else {
			log.Printf("[INFO] [SHUTDOWN] Node connections closed successfully")
		}
	}
	elapsed = time.Since(startTime)
	log.Printf("[INFO] [SHUTDOWN] Step 7/10 completed in %v", elapsed)

	// Step 8/10: Write system_stopped event to events table
	log.Printf("[INFO] [SHUTDOWN] Step 8/10 — Writing system_stopped event")
	if m.eventWriter != nil {
		if err := m.eventWriter.WriteSystemStoppedEvent(); err != nil {
			log.Printf("[ERROR] [SHUTDOWN] Failed to write system_stopped event: %v", err)
		} else {
			log.Printf("[INFO] [SHUTDOWN] System stopped event written successfully")
		}
	}
	elapsed = time.Since(startTime)
	log.Printf("[INFO] [SHUTDOWN] Step 8/10 completed in %v", elapsed)

	// Step 9/10: PRAGMA wal_checkpoint(FULL) to collapse WAL into main DB file
	log.Printf("[INFO] [SHUTDOWN] Step 9/10 — Collapsing WAL into main DB file")
	if m.db != nil {
		checkpointCtx, checkpointCancel := context.WithTimeout(ctx, 5*time.Second)
		defer checkpointCancel()

		var walFrames int
		var checkpointed int
		err := m.db.QueryRowContext(checkpointCtx, "PRAGMA wal_checkpoint(FULL)").Scan(&walFrames, &checkpointed, &checkpointed)
		if err != nil {
			log.Printf("[ERROR] [SHUTDOWN] WAL checkpoint failed: %v", err)
		} else {
			log.Printf("[INFO] [SHUTDOWN] WAL checkpoint complete: %d frames, %d checkpointed", walFrames, checkpointed)
		}
	}
	elapsed = time.Since(startTime)
	log.Printf("[INFO] [SHUTDOWN] Step 9/10 completed in %v", elapsed)

	// Step 10/10: Close SQLite
	log.Printf("[INFO] [SHUTDOWN] Step 10/10 — Closing SQLite database")
	if m.db != nil {
		if err := m.db.Close(); err != nil {
			log.Printf("[ERROR] [SHUTDOWN] Failed to close database: %v", err)
		} else {
			log.Printf("[INFO] [SHUTDOWN] Database closed successfully")
		}
	}
	totalElapsed := time.Since(startTime)
	log.Printf("[INFO] [SHUTDOWN] Step 10/10 completed in %v", totalElapsed-elapsed)

	// Check if we completed within the deadline
	if totalElapsed <= deadline {
		log.Printf("[INFO] [SHUTDOWN] Graceful shutdown complete in %v (within %v deadline)", totalElapsed, deadline)
		return true
	}

	log.Printf("[ERROR] [SHUTDOWN] Graceful shutdown exceeded deadline (%v > %v)", totalElapsed, deadline)
	return false
}
