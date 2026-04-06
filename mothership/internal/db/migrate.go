// Package db provides a versioned schema migration framework for SQLite databases.
// It supports pre-migration backups, transactional migration execution, and backup pruning.
package db

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Migration represents a single schema migration.
type Migration struct {
	Version     int
	Description string
	Up          func(*sql.Tx) error
}

// Migrator handles schema migrations for a database.
type Migrator struct {
	db              *sql.DB
	migrations      []Migration
	dataDir         string
	backupDir       string
	backupRetention time.Duration
	mu              sync.Mutex
}

// Config holds migrator configuration.
type Config struct {
	DataDir         string        // Base data directory
	BackupDir       string        // Backup directory (default: DataDir/backups)
	BackupRetention time.Duration // Backup retention (default: 90 days)
}

// NewMigrator creates a new migrator for the database at dbPath.
func NewMigrator(dbPath string, cfg Config) (*Migrator, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	backupDir := cfg.BackupDir
	if backupDir == "" {
		backupDir = filepath.Join(cfg.DataDir, "backups")
	}
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return nil, fmt.Errorf("create backup dir: %w", err)
	}

	retention := cfg.BackupRetention
	if retention == 0 {
		retention = 90 * 24 * time.Hour // 90 days default
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite is single-writer

	return &Migrator{
		db:              db,
		dataDir:         cfg.DataDir,
		backupDir:       backupDir,
		backupRetention: retention,
	}, nil
}

// Register registers migrations. Migrations must be registered in order by version.
func (m *Migrator) Register(migrations ...Migration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.migrations = migrations
}

// CurrentVersion returns the current schema version from the database.
func (m *Migrator) CurrentVersion(ctx context.Context) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if schema_migrations table exists
	var tableName string
	err := m.db.QueryRowContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name='schema_migrations'",
	).Scan(&tableName)
	if err == sql.ErrNoRows {
		// Table doesn't exist, no migrations applied yet
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("check schema_migrations table: %w", err)
	}

	// Get the max version applied
	var version int
	err = m.db.QueryRowContext(ctx,
		"SELECT COALESCE(MAX(version), 0) FROM schema_migrations",
	).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("get current version: %w", err)
	}

	return version, nil
}

// Pending returns the list of pending migrations that haven't been applied yet.
func (m *Migrator) Pending(ctx context.Context) ([]Migration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	current, err := m.CurrentVersion(ctx)
	if err != nil {
		return nil, err
	}

	var pending []Migration
	for _, m := range m.migrations {
		if m.Version > current {
			pending = append(pending, m)
		}
	}

	return pending, nil
}

// Migrate runs all pending migrations. Each migration runs in its own transaction.
// A pre-migration backup is created before any schema changes.
func (m *Migrator) Migrate(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	current, err := m.CurrentVersion(ctx)
	if err != nil {
		return fmt.Errorf("get current version: %w", err)
	}

	var pending []Migration
	for _, mig := range m.migrations {
		if mig.Version > current {
			pending = append(pending, mig)
		}
	}

	if len(pending) == 0 {
		log.Printf("[INFO] Database schema is up to date (version %d)", current)
		return nil
	}

	// Create pre-migration backup
	backupPath, err := m.createBackup(ctx, current, pending[len(pending)-1].Version)
	if err != nil {
		return fmt.Errorf("create pre-migration backup: %w", err)
	}
	log.Printf("[INFO] Pre-migration backup created: %s", backupPath)

	// Apply each pending migration
	for _, mig := range pending {
		log.Printf("[INFO] Applying migration %d: %s", mig.Version, mig.Description)

		if err := m.applyMigration(ctx, mig); err != nil {
			// Migration failed - backup is preserved for recovery
			return fmt.Errorf("migration %d failed: %w (backup preserved at %s)", mig.Version, err, backupPath)
		}

		log.Printf("[INFO] Migration %d applied successfully", mig.Version)
	}

	log.Printf("[INFO] All migrations applied successfully (version %d → %d)",
		current, pending[len(pending)-1].Version)

	// Prune old backups
	go m.pruneOldBackups()

	return nil
}

// applyMigration applies a single migration within a transaction.
func (m *Migrator) applyMigration(ctx context.Context, mig Migration) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	defer func() {
		if tx != nil {
			tx.Rollback()
		}
	}()

	// Run the migration Up function
	if err := mig.Up(tx); err != nil {
		return fmt.Errorf("execute migration: %w", err)
	}

	// Record the migration
	now := time.Now().UnixMilli()
	if _, err := tx.ExecContext(ctx,
		"INSERT INTO schema_migrations (version, applied_at, description) VALUES (?, ?, ?)",
		mig.Version, now, mig.Description,
	); err != nil {
		return fmt.Errorf("record migration: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	tx = nil // Mark as committed
	return nil
}

// createBackup creates a backup of the database before migration.
// Uses SQLite Online Backup API via SQL commands.
func (m *Migrator) createBackup(ctx context.Context, oldVersion, newVersion int) (string, error) {
	timestamp := time.Now().Format("20060102-150405")
	backupPath := filepath.Join(m.backupDir,
		fmt.Sprintf("pre-upgrade-v%d-to-v%d-%s.sqlite", oldVersion, newVersion, timestamp))

	// Use the backup API via SQL backup command
	if _, err := m.db.ExecContext(ctx,
		fmt.Sprintf("VACUUM INTO '%s'", backupPath)); err != nil {
		return "", fmt.Errorf("vacuum into backup: %w", err)
	}

	return backupPath, nil
}

// pruneOldBackups removes backups older than the retention period.
// Runs in the background after successful migration.
func (m *Migrator) pruneOldBackups() {
	// Find and delete old backup files
	cutoff := time.Now().Add(-m.backupRetention)

	entries, err := os.ReadDir(m.backupDir)
	if err != nil {
		log.Printf("[WARN] Failed to read backup directory for pruning: %v", err)
		return
	}

	pruned := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Parse backup filename format: pre-upgrade-vX-to-vY-TIMESTAMP.sqlite
		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			path := filepath.Join(m.backupDir, entry.Name())
			if err := os.Remove(path); err != nil {
				log.Printf("[WARN] Failed to delete old backup %s: %v", path, err)
			} else {
				pruned++
			}
		}
	}

	if pruned > 0 {
		log.Printf("[INFO] Pruned %d old backups (older than %s)", pruned, m.backupRetention)
	}
}

// PruneOldBackups manually triggers backup pruning.
func (m *Migrator) PruneOldBackups() {
	m.pruneOldBackups()
}

// Close closes the database connection.
func (m *Migrator) Close() error {
	return m.db.Close()
}

// DB returns the underlying database connection.
// This allows the migrator to be used as the database handle after migrations.
func (m *Migrator) DB() *sql.DB {
	return m.db
}
