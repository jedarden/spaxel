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
	"syscall"
	"time"

	"github.com/spaxel/mothership/internal/startup"
)

// OpenDB initializes the database with full startup sequence.
// It runs migrations, creates backups, and returns a ready-to-use database connection.
// The startup sequence is:
//   1. Data directory: verify /data is writable; acquire flock() lock
//   2. SQLite: open database with WAL mode, busy_timeout=5000
//   3. Schema migration: apply pending migrations with backup
//   4. Config & secrets: load/generate install secret
//
// If any phase fails, the function returns an error and the caller should
// exit without serving traffic.
func OpenDB(dataDir, dbName string) (*sql.DB, error) {
	ctx, cancel := context.WithTimeout(context.Background(), startup.TotalTimeout)
	defer cancel()

	// Phase 1: Data directory + flock
	done := startup.Phase(1, "Data directory")
	dbPath := filepath.Join(dataDir, dbName)
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create data dir: %w", err)
	}

	// Acquire exclusive flock to prevent duplicate instances
	lockPath := filepath.Join(dataDir, ".lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("create lock file: %w", err)
	}
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lockFile.Close()
		return nil, fmt.Errorf("acquire flock on %s (another instance running?): %w", lockPath, err)
	}
	done()

	// Phase 2: SQLite open
	done = startup.Phase(2, "SQLite")
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)")
	if err != nil {
		lockFile.Close()
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite is single-writer

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		lockFile.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// Integrity check
	var integrityResult string
	err = db.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrityResult)
	if err != nil || integrityResult != "ok" {
		corruptPath := dbPath + ".corrupt." + time.Now().Format("20060102-150405")
		log.Printf("[WARN] SQLite integrity check failed (%s), moving to %s and starting fresh", integrityResult, corruptPath)
		db.Close()
		if renameErr := os.Rename(dbPath, corruptPath); renameErr != nil {
			lockFile.Close()
			return nil, fmt.Errorf("move corrupt database: %w", renameErr)
		}
		db, err = sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)")
		if err != nil {
			lockFile.Close()
			return nil, fmt.Errorf("open fresh sqlite: %w", err)
		}
		db.SetMaxOpenConns(1)
	}
	done()

	// Phase 3: Schema migration
	done = startup.Phase(3, "Schema migrations")
	migrator, err := NewMigrator(dbPath, Config{
		DataDir:         dataDir,
		BackupRetention: 90 * 24 * time.Hour,
	})
	if err != nil {
		db.Close()
		lockFile.Close()
		return nil, fmt.Errorf("create migrator: %w", err)
	}
	migrator.Register(AllMigrations()...)

	current, err := migrator.CurrentVersion(ctx)
	if err != nil {
		db.Close()
		lockFile.Close()
		return nil, fmt.Errorf("get current version: %w", err)
	}

	if current == 0 {
		log.Printf("[INFO] Initializing new database")
	} else {
		log.Printf("[INFO] Current schema version %d", current)
	}

	if err := migrator.Migrate(ctx); err != nil {
		db.Close()
		lockFile.Close()
		return nil, fmt.Errorf("apply migrations: %w", err)
	}

	latest := len(AllMigrations())
	log.Printf("[INFO] Schema migration complete (version %d)", latest)
	done()

	// Phase 4: Config & secrets
	done = startup.Phase(4, "Config & secrets")
	if err := ensureInstallSecret(ctx, db); err != nil {
		db.Close()
		lockFile.Close()
		return nil, fmt.Errorf("ensure install secret: %w", err)
	}
	done()

	return db, nil
}

// ensureInstallSecret ensures the install secret exists, generating one if needed.
func ensureInstallSecret(ctx context.Context, db *sql.DB) error {
	var existingSecret []byte
	err := db.QueryRowContext(ctx, "SELECT install_secret FROM auth WHERE id = 1").Scan(&existingSecret)
	if err == nil && len(existingSecret) == 32 {
		return nil
	}

	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return fmt.Errorf("generate secret: %w", err)
	}

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
func RunMigrations(dataDir, dbName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), startup.TotalTimeout)
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
