// Package localization provides weight persistence for self-improving localization
package localization

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// WeightStore persists learned weights to SQLite
type WeightStore struct {
	mu   sync.RWMutex
	db   *sql.DB
	path string
}

// NewWeightStore creates a new weight persistence store
func NewWeightStore(path string) (*WeightStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	store := &WeightStore{
		db:   db,
		path: path,
	}

	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, err
	}

	return store, nil
}

// initSchema creates the database schema
func (s *WeightStore) initSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS link_weights (
			link_id TEXT PRIMARY KEY,
			weight REAL NOT NULL DEFAULT 1.0,
			sigma REAL NOT NULL DEFAULT 0.0,
			observation_count INTEGER NOT NULL DEFAULT 0,
			correct_count INTEGER NOT NULL DEFAULT 0,
			error_sum REAL NOT NULL DEFAULT 0,
			error_sum_squared REAL NOT NULL DEFAULT 0,
			last_error REAL NOT NULL DEFAULT 0,
			weight_adjustments INTEGER NOT NULL DEFAULT 0,
			last_adjustment_time TIMESTAMP,
			updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_link_weights_updated ON link_weights(updated_at);

		CREATE TABLE IF NOT EXISTS learning_metadata (
			key TEXT PRIMARY KEY,
			value TEXT
		);
	`)
	return err
}

// SaveWeights persists all learned weights to the database
func (s *WeightStore) SaveWeights(weights *LearnedWeights) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	weights.mu.RLock()
	defer weights.mu.RUnlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO link_weights
		(link_id, weight, sigma, observation_count, correct_count, error_sum,
		 error_sum_squared, last_error, weight_adjustments, last_adjustment_time, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now()

	for linkID, stats := range weights.linkStats {
		weight := weights.linkWeights[linkID]
		sigma := weights.linkSigmas[linkID]

		_, err := stmt.Exec(
			linkID,
			weight,
			sigma,
			stats.ObservationCount,
			stats.CorrectCount,
			stats.ErrorSum,
			stats.ErrorSumSquared,
			stats.LastError,
			stats.WeightAdjustments,
			stats.LastAdjustmentTime,
			now,
		)
		if err != nil {
			return err
		}
	}

	// Update metadata
	metaStmt, err := tx.Prepare(`INSERT OR REPLACE INTO learning_metadata (key, value) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer metaStmt.Close()

	_, err = metaStmt.Exec("last_save", now.Format(time.RFC3339))
	if err != nil {
		return err
	}

	return tx.Commit()
}

// LoadWeights restores learned weights from the database
func (s *WeightStore) LoadWeights() (*LearnedWeights, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	weights := NewLearnedWeights()

	rows, err := s.db.Query(`
		SELECT link_id, weight, sigma, observation_count, correct_count, error_sum,
		       error_sum_squared, last_error, weight_adjustments, last_adjustment_time
		FROM link_weights
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	weights.mu.Lock()
	defer weights.mu.Unlock()

	for rows.Next() {
		var linkID string
		var weight, sigma, errorSum, errorSumSquared, lastError float64
		var obsCount, correctCount, adjustments int64
		var lastAdjTime sql.NullTime

		err := rows.Scan(
			&linkID, &weight, &sigma, &obsCount, &correctCount, &errorSum,
			&errorSumSquared, &lastError, &adjustments, &lastAdjTime,
		)
		if err != nil {
			return nil, err
		}

		weights.linkWeights[linkID] = weight
		weights.linkSigmas[linkID] = sigma
		weights.linkStats[linkID] = &LinkLearningStats{
			ObservationCount:   obsCount,
			CorrectCount:       correctCount,
			ErrorSum:           errorSum,
			ErrorSumSquared:    errorSumSquared,
			LastError:          lastError,
			WeightAdjustments:  adjustments,
			LastAdjustmentTime: lastAdjTime.Time,
		}
	}

	weights.lastUpdate = time.Now()

	log.Printf("[INFO] Loaded %d learned link weights from %s", len(weights.linkWeights), s.path)

	return weights, rows.Err()
}

// GetLinkCount returns the number of links with stored weights
func (s *WeightStore) GetLinkCount() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM link_weights`).Scan(&count)
	return count, err
}

// GetLastSaveTime returns the time of the last successful save
func (s *WeightStore) GetLastSaveTime() (time.Time, error) {
	var lastSaveStr string
	err := s.db.QueryRow(`SELECT value FROM learning_metadata WHERE key = 'last_save'`).Scan(&lastSaveStr)
	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339, lastSaveStr)
}

// Close closes the database connection
func (s *WeightStore) Close() error {
	return s.db.Close()
}

// StartPeriodicSave starts a goroutine that periodically saves weights
func (s *WeightStore) StartPeriodicSave(ctx interface{ Done() <-chan struct{} }, learner *WeightLearner, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		lastObsCount := int64(0)

		for {
			select {
			case <-ctx.Done():
				// Final save on shutdown
				if learner != nil {
					weights := learner.GetLearnedWeights()
					if err := s.SaveWeights(weights); err != nil {
						log.Printf("[WARN] Failed to save weights on shutdown: %v", err)
					} else {
						log.Printf("[INFO] Saved learned weights on shutdown")
					}
				}
				return
			case <-ticker.C:
				if learner == nil {
					continue
				}

				// Only save if there are new observations
				progress := learner.GetLearningProgress()
				obsCount, _ := progress["total_observations"].(int64)
				if obsCount > lastObsCount {
					weights := learner.GetLearnedWeights()
					if err := s.SaveWeights(weights); err != nil {
						log.Printf("[WARN] Failed to save weights: %v", err)
					} else {
						lastObsCount = obsCount
					}
				}
			}
		}
	}()

	log.Printf("[INFO] Periodic weight save started (interval: %v)", interval)
}
