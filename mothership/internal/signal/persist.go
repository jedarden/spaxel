// Package signal implements baseline persistence to SQLite
package signal

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// BaselineStore persists baseline and diurnal data to SQLite
type BaselineStore struct {
	mu   sync.RWMutex
	db   *sql.DB
	path string
}

// NewBaselineStore creates a new baseline persistence store
func NewBaselineStore(dbPath string) (*BaselineStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	store := &BaselineStore{
		db:   db,
		path: dbPath,
	}

	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

// initSchema creates the necessary tables
func (s *BaselineStore) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS baselines (
		link_id TEXT PRIMARY KEY,
		values_json TEXT NOT NULL,
		sample_time INTEGER NOT NULL,
		confidence REAL NOT NULL,
		updated_at INTEGER NOT NULL
	);

	CREATE TABLE IF NOT EXISTS diurnal_baselines (
		link_id TEXT NOT NULL,
		slot INTEGER NOT NULL,
		values_json TEXT NOT NULL,
		sample_count INTEGER NOT NULL,
		last_update INTEGER NOT NULL,
		PRIMARY KEY (link_id, slot)
	);

	CREATE TABLE IF NOT EXISTS diurnal_meta (
		link_id TEXT PRIMARY KEY,
		created_at INTEGER NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_baselines_updated ON baselines(updated_at);
	CREATE INDEX IF NOT EXISTS idx_diurnal_update ON diurnal_baselines(last_update);
	`

	_, err := s.db.Exec(schema)
	return err
}

// SaveBaseline saves an EMA baseline snapshot
func (s *BaselineStore) SaveBaseline(linkID string, snapshot *BaselineSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	valuesJSON, err := json.Marshal(snapshot.Values)
	if err != nil {
		return err
	}

	now := time.Now().Unix()
	_, err = s.db.Exec(`
		INSERT OR REPLACE INTO baselines (link_id, values_json, sample_time, confidence, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, linkID, string(valuesJSON), snapshot.SampleTime.Unix(), snapshot.Confidence, now)

	return err
}

// SaveAllBaselines saves all baseline snapshots
func (s *BaselineStore) SaveAllBaselines(baselines map[string]*BaselineSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	now := time.Now().Unix()
	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO baselines (link_id, values_json, sample_time, confidence, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for linkID, snapshot := range baselines {
		valuesJSON, err := json.Marshal(snapshot.Values)
		if err != nil {
			continue
		}

		_, err = stmt.Exec(linkID, string(valuesJSON), snapshot.SampleTime.Unix(), snapshot.Confidence, now)
		if err != nil {
			log.Printf("[WARN] Failed to save baseline for %s: %v", linkID, err)
		}
	}

	return tx.Commit()
}

// LoadBaseline loads a baseline snapshot for a link
func (s *BaselineStore) LoadBaseline(linkID string) (*BaselineSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var valuesJSON string
	var sampleTimeUnix int64
	var confidence float64

	err := s.db.QueryRow(`
		SELECT values_json, sample_time, confidence FROM baselines WHERE link_id = ?
	`, linkID).Scan(&valuesJSON, &sampleTimeUnix, &confidence)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	var values []float64
	if err := json.Unmarshal([]byte(valuesJSON), &values); err != nil {
		return nil, err
	}

	return &BaselineSnapshot{
		Values:     values,
		SampleTime: time.Unix(sampleTimeUnix, 0),
		Confidence: confidence,
	}, nil
}

// LoadAllBaselines loads all baseline snapshots
func (s *BaselineStore) LoadAllBaselines() (map[string]*BaselineSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`SELECT link_id, values_json, sample_time, confidence FROM baselines`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]*BaselineSnapshot)
	for rows.Next() {
		var linkID, valuesJSON string
		var sampleTimeUnix int64
		var confidence float64

		if err := rows.Scan(&linkID, &valuesJSON, &sampleTimeUnix, &confidence); err != nil {
			continue
		}

		var values []float64
		if err := json.Unmarshal([]byte(valuesJSON), &values); err != nil {
			continue
		}

		result[linkID] = &BaselineSnapshot{
			Values:     values,
			SampleTime: time.Unix(sampleTimeUnix, 0),
			Confidence: confidence,
		}
	}

	return result, nil
}

// SaveDiurnal saves a diurnal baseline snapshot
func (s *BaselineStore) SaveDiurnal(linkID string, snapshot *DiurnalSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Save meta
	_, err = tx.Exec(`
		INSERT OR REPLACE INTO diurnal_meta (link_id, created_at)
		VALUES (?, ?)
	`, linkID, snapshot.Created.Unix())
	if err != nil {
		return err
	}

	// Save each slot
	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO diurnal_baselines (link_id, slot, values_json, sample_count, last_update)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for slot := 0; slot < DiurnalSlots; slot++ {
		valuesJSON, err := json.Marshal(snapshot.SlotValues[slot])
		if err != nil {
			continue
		}

		_, err = stmt.Exec(linkID, slot, string(valuesJSON), snapshot.SlotCounts[slot], snapshot.SlotTimes[slot].Unix())
		if err != nil {
			log.Printf("[WARN] Failed to save diurnal slot %d for %s: %v", slot, linkID, err)
		}
	}

	return tx.Commit()
}

// SaveAllDiurnal saves all diurnal snapshots
func (s *BaselineStore) SaveAllDiurnal(diurnals map[string]*DiurnalSnapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for linkID, snapshot := range diurnals {
		if err := s.saveDiurnalTx(linkID, snapshot); err != nil {
			log.Printf("[WARN] Failed to save diurnal for %s: %v", linkID, err)
		}
	}

	return nil
}

// saveDiurnalTx saves a diurnal snapshot within a transaction
func (s *BaselineStore) saveDiurnalTx(linkID string, snapshot *DiurnalSnapshot) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Save meta
	_, err = tx.Exec(`
		INSERT OR REPLACE INTO diurnal_meta (link_id, created_at)
		VALUES (?, ?)
	`, linkID, snapshot.Created.Unix())
	if err != nil {
		return err
	}

	// Save each slot
	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO diurnal_baselines (link_id, slot, values_json, sample_count, last_update)
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for slot := 0; slot < DiurnalSlots; slot++ {
		valuesJSON, err := json.Marshal(snapshot.SlotValues[slot])
		if err != nil {
			continue
		}

		_, err = stmt.Exec(linkID, slot, string(valuesJSON), snapshot.SlotCounts[slot], snapshot.SlotTimes[slot].Unix())
		if err != nil {
			continue
		}
	}

	return tx.Commit()
}

// LoadDiurnal loads a diurnal baseline snapshot for a link
func (s *BaselineStore) LoadDiurnal(linkID string, nSub int) (*DiurnalSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Get meta
	var createdAt int64
	err := s.db.QueryRow(`SELECT created_at FROM diurnal_meta WHERE link_id = ?`, linkID).Scan(&createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	snapshot := &DiurnalSnapshot{
		LinkID:    linkID,
		Created:   time.Unix(createdAt, 0),
	}

	// Initialize slots
	for i := 0; i < DiurnalSlots; i++ {
		snapshot.SlotValues[i] = make([]float64, nSub)
		snapshot.SlotTimes[i] = time.Time{}
	}

	// Get all slots
	rows, err := s.db.Query(`
		SELECT slot, values_json, sample_count, last_update
		FROM diurnal_baselines WHERE link_id = ?
	`, linkID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var slot int
		var valuesJSON string
		var sampleCount int
		var lastUpdate int64

		if err := rows.Scan(&slot, &valuesJSON, &sampleCount, &lastUpdate); err != nil {
			continue
		}

		if slot >= 0 && slot < DiurnalSlots {
			var values []float64
			if err := json.Unmarshal([]byte(valuesJSON), &values); err == nil {
				snapshot.SlotValues[slot] = values
			}
			snapshot.SlotCounts[slot] = sampleCount
			snapshot.SlotTimes[slot] = time.Unix(lastUpdate, 0)
		}
	}

	return snapshot, nil
}

// LoadAllDiurnal loads all diurnal baseline snapshots
func (s *BaselineStore) LoadAllDiurnal(nSub int) (map[string]*DiurnalSnapshot, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Get all link IDs from meta
	rows, err := s.db.Query(`SELECT link_id, created_at FROM diurnal_meta`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	linkMetas := make(map[string]time.Time)
	for rows.Next() {
		var linkID string
		var createdAt int64
		if err := rows.Scan(&linkID, &createdAt); err != nil {
			continue
		}
		linkMetas[linkID] = time.Unix(createdAt, 0)
	}

	// Load each diurnal
	result := make(map[string]*DiurnalSnapshot)
	for linkID, created := range linkMetas {
		snapshot, err := s.LoadDiurnal(linkID, nSub)
		if err != nil {
			log.Printf("[WARN] Failed to load diurnal for %s: %v", linkID, err)
			continue
		}
		if snapshot != nil {
			snapshot.Created = created
			result[linkID] = snapshot
		}
	}

	return result, nil
}

// StartPeriodicSave starts a goroutine that periodically saves all baselines
func (s *BaselineStore) StartPeriodicSave(ctx context.Context, pm *ProcessorManager, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				// Final save on shutdown
				s.saveAll(pm)
				return
			case <-ticker.C:
				s.saveAll(pm)
			}
		}
	}()
}

func (s *BaselineStore) saveAll(pm *ProcessorManager) {
	// Save EMA baselines
	baselines := pm.GetAllBaselines()
	if len(baselines) > 0 {
		if err := s.SaveAllBaselines(baselines); err != nil {
			log.Printf("[WARN] Failed to save baselines: %v", err)
		}
	}

	// Save diurnal baselines
	diurnals := pm.GetAllDiurnalSnapshots()
	if len(diurnals) > 0 {
		if err := s.SaveAllDiurnal(diurnals); err != nil {
			log.Printf("[WARN] Failed to save diurnal baselines: %v", err)
		}
	}
}

// RestoreAll restores all saved baselines to a processor manager
func (s *BaselineStore) RestoreAll(pm *ProcessorManager, nSub int) error {
	// Restore EMA baselines
	baselines, err := s.LoadAllBaselines()
	if err != nil {
		return err
	}
	for linkID, snapshot := range baselines {
		pm.RestoreBaseline(linkID, snapshot)
	}

	// Restore diurnal baselines
	diurnals, err := s.LoadAllDiurnal(nSub)
	if err != nil {
		return err
	}
	for linkID, snapshot := range diurnals {
		pm.RestoreDiurnal(linkID, snapshot)
	}

	log.Printf("[INFO] Restored %d EMA baselines and %d diurnal baselines", len(baselines), len(diurnals))
	return nil
}

// Close closes the database connection
func (s *BaselineStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.Close()
}

// DeleteBaseline removes a baseline from the store
func (s *BaselineStore) DeleteBaseline(linkID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`DELETE FROM baselines WHERE link_id = ?`, linkID)
	return err
}

// DeleteDiurnal removes a diurnal baseline from the store
func (s *BaselineStore) DeleteDiurnal(linkID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`DELETE FROM diurnal_meta WHERE link_id = ?`, linkID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`DELETE FROM diurnal_baselines WHERE link_id = ?`, linkID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

// PruneStale removes baselines older than the specified age
func (s *BaselineStore) PruneStale(maxAge time.Duration) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-maxAge).Unix()

	result, err := s.db.Exec(`DELETE FROM baselines WHERE updated_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}

	deleted, _ := result.RowsAffected()

	// Also prune old diurnal slots
	_, err = s.db.Exec(`DELETE FROM diurnal_baselines WHERE last_update < ?`, cutoff)
	if err != nil {
		return int(deleted), err
	}

	return int(deleted), nil
}
