// Package db provides the main database initialization with migration support.
package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// StartupPhase represents a phase in the database startup sequence.
type StartupPhase int

const (
	PhaseDataDir StartupPhase = iota + 1
	PhaseOpenDB
	PhaseIntegrityCheck
	PhaseSchemaMigration
	PhaseConfigSecrets
	PhaseSubsystems
	PhaseReady
)

// PhaseLogger is called during each startup phase.
type PhaseLogger func(phase StartupPhase, message string)

// DefaultPhaseLogger logs to stdout.
func DefaultPhaseLogger(phase StartupPhase, message string) {
	log.Printf("[PHASE %d/7] %s", phase, message)
}

// OpenDB initializes the database with full startup sequence.
// It runs migrations, creates backups, and returns a ready-to-use database connection.
// The startup sequence is:
//   1. Data directory: verify /data is writable
//   2. SQLite: open database with WAL mode
//   3. Integrity check: verify database integrity
//   4. Schema migration: apply pending migrations with backup
//   5. Config & secrets: load/generate install secret
//   6. Subsystems: ready for other subsystems to use
//   7. Ready: database is ready for use
//
// If any phase fails, the function returns an error and the caller should
// exit without serving traffic.
func OpenDB(dataDir, dbName string, logger PhaseLogger) (*sql.DB, error) {
	if logger == nil {
		logger = DefaultPhaseLogger
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Phase 1: Data directory
	logger(PhaseDataDir, "Data directory: verify writable")
	dbPath := filepath.Join(dataDir, dbName)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	// Acquire file lock to prevent duplicate instances
	lockPath := filepath.Join(dataDir, ".lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("create lock file: %w", err)
	}
	defer lockFile.Close()
	// Note: proper flock would require platform-specific code
	// For now, we rely on SQLite's single-writer mode

	// Phase 2: SQLite open
	logger(PhaseOpenDB, fmt.Sprintf("SQLite: opening %s", dbPath))
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite is single-writer

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// Phase 3: Integrity check
	logger(PhaseIntegrityCheck, "SQLite: running integrity check")
	var integrityOK bool
	err = db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrityOK)
	if err == nil && integrityOK {
		logger(PhaseIntegrityCheck, "SQLite: integrity check passed")
	} else {
		// Database may be corrupt
		corruptPath := dbPath + ".corrupt." + time.Now().Format("20060102-150405")
		logger(PhaseIntegrityCheck, fmt.Sprintf("SQLite: integrity check failed, moving to %s and starting fresh", corruptPath))
		db.Close()
		if err := os.Rename(dbPath, corruptPath); err != nil {
			return nil, fmt.Errorf("move corrupt database: %w", err)
		}
		// Reopen with fresh database
		db, err = sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)")
		if err != nil {
			return nil, fmt.Errorf("open fresh sqlite: %w", err)
		}
		db.SetMaxOpenConns(1)
	}

	// Phase 4: Schema migration
	logger(PhaseSchemaMigration, "Schema migration: checking pending migrations")

	migrator, err := NewMigrator(dbPath, Config{
		DataDir:         dataDir,
		BackupRetention: 90 * 24 * time.Hour,
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("create migrator: %w", err)
	}
	migrator.Register(AllMigrations()...)

	current, err := migrator.CurrentVersion(ctx)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("get current version: %w", err)
	}

	if current == 0 {
		logger(PhaseSchemaMigration, "Schema migration: initializing new database")
	} else {
		logger(PhaseSchemaMigration, fmt.Sprintf("Schema migration: current version %d", current))
	}

	if err := migrator.Migrate(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("apply migrations: %w", err)
	}

	latest := len(AllMigrations())
	logger(PhaseSchemaMigration, fmt.Sprintf("Schema migration: complete (version %d)", latest))

	// Phase 5: Config & secrets
	logger(PhaseConfigSecrets, "Config & secrets: loading/generating install secret")
	if err := ensureInstallSecret(ctx, db); err != nil {
		db.Close()
		return nil, fmt.Errorf("ensure install secret: %w", err)
	}

	// Phase 6: Subsystems ready
	logger(PhaseSubsystems, "Subsystems: database ready for initialization")

	// Phase 7: Ready
	logger(PhaseReady, "Database ready")
	return db, nil
}

// ensureInstallSecret ensures the install secret exists, generating one if needed.
func ensureInstallSecret(ctx context.Context, db *sql.DB) error {
	// Check if secret exists
	var existingSecret []byte
	err := db.QueryRowContext(ctx, "SELECT install_secret FROM auth WHERE id = 1").Scan(&existingSecret)
	if err == nil && len(existingSecret) == 32 {
		return nil // Secret exists
	}

	// Generate new secret
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return fmt.Errorf("generate secret: %w", err)
	}

	// Insert or update
	_, err = db.ExecContext(ctx, `
		INSERT INTO auth (id, install_secret) VALUES (1, ?)
		ON CONFLICT(id) DO UPDATE SET install_secret = excluded.install_secret
	`, secret[:])
	if err != nil {
		return fmt.Errorf("store install secret: %w", err)
	}

	log.Printf("[INFO] Installation secret generated (shown once): %s", hex.EncodeToString(secret))
	return nil
}

// RunMigrations is a convenience function to run migrations on an existing database.
// This is useful for the migrate.go command or for testing.
func RunMigrations(dataDir, dbName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dbPath := filepath.Join(dataDir, dbName)

	migrator, err := NewMigrator(dbPath, Config{
		DataDir:         dataDir,
		BackupRetention: 90 * 24 * time.Hour,
	})
	if err != nil {
		return err
	}
	defer migrator.Close()

	migrator.Register(AllMigrations()...)

	return migrator.Migrate(ctx)
}

// CurrentVersion returns the current schema version of the database.
func CurrentVersion(dataDir, dbName string) (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	dbPath := filepath.Join(dataDir, dbName)

	migrator, err := NewMigrator(dbPath, Config{
		DataDir: dataDir,
	})
	if err != nil {
		return 0, err
	}
	defer migrator.Close()

	return migrator.CurrentVersion(ctx)
}
