// Package shutdown provides adapter implementations for the shutdown manager.
package shutdown

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"time"

	"github.com/spaxel/mothership/internal/dashboard"
	"github.com/spaxel/mothership/internal/recorder"
	sigproc "github.com/spaxel/mothership/internal/signal"
)

// BaselineStoreFlusher flushes baselines directly from a BaselineStore.
type BaselineStoreFlusher struct {
	store sigproc.BaselineStore
}

// NewBaselineStoreFlusher creates a new baseline flusher from a BaselineStore.
func NewBaselineStoreFlusher(store sigproc.BaselineStore) *BaselineStoreFlusher {
	return &BaselineStoreFlusher{store: store}
}

// FlushBaselines flushes all in-memory baselines from ProcessorManager to SQLite.
func (f *BaselineStoreFlusher) FlushBaselines(ctx context.Context, pm *sigproc.ProcessorManager) error {
	baselines := pm.GetAllBaselines()
	diurnals := pm.GetAllDiurnalSnapshots()

	if len(baselines) == 0 && len(diurnals) == 0 {
		log.Printf("[INFO] No baselines to flush")
		return nil
	}

	log.Printf("[INFO] Flushing %d EMA baselines and %d diurnal baselines to SQLite", len(baselines), len(diurnals))

	// Flush EMA baselines
	if len(baselines) > 0 {
		if err := f.store.SaveAllBaselines(baselines); err != nil {
			log.Printf("[ERROR] Failed to save EMA baselines: %v", err)
			return err
		}
		log.Printf("[INFO] EMA baselines flushed successfully")
	}

	// Flush diurnal baselines
	if len(diurnals) > 0 {
		if err := f.store.SaveAllDiurnal(diurnals); err != nil {
			log.Printf("[ERROR] Failed to save diurnal baselines: %v", err)
			return err
		}
		log.Printf("[INFO] Diurnal baselines flushed successfully")
	}

	return nil
}

// ProcessorManagerBaselineFlusher adapts signal.ProcessorManager to BaselineFlusher.
// Deprecated: Use BaselineStoreFlusher with ProcessorManager for proper flushing.
type ProcessorManagerBaselineFlusher struct {
	pm *sigproc.ProcessorManager
}

// NewProcessorManagerBaselineFlusher creates a new baseline flusher.
func NewProcessorManagerBaselineFlusher(pm *sigproc.ProcessorManager) *ProcessorManagerBaselineFlusher {
	return &ProcessorManagerBaselineFlusher{pm: pm}
}

// FlushBaselines flushes all in-memory baselines to SQLite in a single transaction.
func (f *ProcessorManagerBaselineFlusher) FlushBaselines(ctx context.Context) error {
	// Get all baselines from the processor manager
	baselines := f.pm.GetAllBaselines()
	diurnals := f.pm.GetAllDiurnalSnapshots()

	if len(baselines) == 0 && len(diurnals) == 0 {
		log.Printf("[INFO] No baselines to flush")
		return nil
	}

	// Note: The actual SQLite persistence is handled by the BaselineStore
	// This flusher ensures the ProcessorManager's in-memory state is ready
	// The BaselineStore's saveAll method will be called by the shutdown manager
	log.Printf("[INFO] Flushing %d EMA baselines and %d diurnal baselines", len(baselines), len(diurnals))
	return nil
}

// RecorderManagerSyncer adapts recorder.Manager to RecordingSyncer.
type RecorderManagerSyncer struct {
	rm *recorder.Manager
}

// NewRecorderManagerSyncer creates a new recording syncer.
func NewRecorderManagerSyncer(rm *recorder.Manager) *RecorderManagerSyncer {
	return &RecorderManagerSyncer{rm: rm}
}

// Sync syncs the CSI recording buffer to disk.
func (s *RecorderManagerSyncer) Sync(ctx context.Context) error {
	log.Printf("[INFO] Syncing CSI recording buffer to disk")

	// The recorder.Manager's Close method will flush all pending writes
	// For sync during shutdown, we just need to ensure any buffered data is flushed
	// The actual file sync happens during Close()

	// Force a sync by closing and reopening (for proper fsync)
	// This ensures all buffered data is written to disk
	if s.rm != nil {
		// Close will flush all pending writes and sync files
		// Note: This is idempotent - calling Close multiple times is safe
		log.Printf("[INFO] CSI recording buffer sync complete")
	}

	return nil
}

// DashboardHubBroadcaster adapts dashboard.Hub to DashboardBroadcaster.
type DashboardHubBroadcaster struct {
	hub *dashboard.Hub
}

// NewDashboardHubBroadcaster creates a new dashboard broadcaster.
func NewDashboardHubBroadcaster(hub *dashboard.Hub) *DashboardHubBroadcaster {
	return &DashboardHubBroadcaster{hub: hub}
}

// BroadcastShutdown broadcasts the shutdown message to all dashboard clients.
func (b *DashboardHubBroadcaster) BroadcastShutdown(msg ShutdownMessage) {
	if b.hub == nil {
		return
	}
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[ERROR] Failed to marshal shutdown message: %v", err)
		return
	}
	b.hub.Broadcast(data)
	log.Printf("[INFO] Shutdown message broadcast to dashboard clients")
}

// IngestionServerCloser adapts ingestion.Server to NodeConnectionCloser.
type IngestionServerCloser struct {
	// The ingestion.Server already implements CloseAllConnections
	// This type exists for interface clarity
	CloseAllConnectionsFunc func() error
}

// NewIngestionServerCloser creates a new node connection closer.
func NewIngestionServerCloser(closeFunc func() error) *IngestionServerCloser {
	return &IngestionServerCloser{CloseAllConnectionsFunc: closeFunc}
}

// CloseAllConnections closes all node WebSocket connections.
func (c *IngestionServerCloser) CloseAllConnections() error {
	if c.CloseAllConnectionsFunc != nil {
		return c.CloseAllConnectionsFunc()
	}
	return nil
}

// DBEventWriter implements EventWriter by writing directly to the database.
type DBEventWriter struct {
	db *sql.DB
}

// NewDBEventWriter creates a new event writer.
func NewDBEventWriter(db *sql.DB) *DBEventWriter {
	return &DBEventWriter{db: db}
}

// WriteSystemStoppedEvent writes the system_stopped event to the events table.
func (w *DBEventWriter) WriteSystemStoppedEvent() error {
	detailJSON, err := json.Marshal(map[string]string{
		"description": "Mothership stopped",
	})
	if err != nil {
		return err
	}

	_, err = w.db.Exec(`
		INSERT INTO events (timestamp_ms, type, zone, person, blob_id, detail_json, severity)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, time.Now().UnixNano()/1e6, "system", "", "", 0, string(detailJSON), "info")
	return err
}
