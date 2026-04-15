// Package db provides tests for the schema migration system.
package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

// TestMigrateIdempotent verifies that running migrations on an already-migrated
// database is a no-op and doesn't cause errors.
func TestMigrateIdempotent(t *testing.T) {
	t.Cleanup(func() {
		// Clean up test databases
		os.RemoveAll(t.TempDir())
	})

	ctx := context.Background()
	dataDir := t.TempDir()
	dbName := "test.db"

	// First migration - should create all tables
	migrator1, err := NewMigrator(filepath.Join(dataDir, dbName), Config{
		DataDir: dataDir,
	})
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	migrator1.Register(AllMigrations()...)

	if err := migrator1.Migrate(ctx); err != nil {
		t.Fatalf("First migrate: %v", err)
	}

	version1, err := migrator1.CurrentVersion(ctx)
	if err != nil {
		t.Fatalf("Get version after first migrate: %v", err)
	}

	expectedVersion := len(AllMigrations())
	if version1 != expectedVersion {
		t.Errorf("After first migrate: version = %d, want %d", version1, expectedVersion)
	}

	// Verify tables exist
	db := migrator1.DB()
	tables := []string{
		"schema_migrations", "settings", "auth", "sessions", "nodes",
		"link_weights", "baselines", "ble_devices", "floorplan", "zones",
		"portals", "portal_crossings", "triggers", "events", "events_archive",
		"feedback", "sleep_records", "firmware", "briefings", "notification_channels",
		"replay_sessions", "crowd_flow", "diurnal_baselines",
		"anomaly_patterns", "prediction_models", "ble_device_aliases",
	}

	for _, table := range tables {
		var name string
		err := db.QueryRowContext(ctx,
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?",
			table).Scan(&name)
		if err == sql.ErrNoRows {
			t.Errorf("Table %s does not exist after migration", table)
		} else if err != nil {
			t.Errorf("Check table %s: %v", table, err)
		}
	}

	// Close and re-open - second migration should be a no-op
	migrator1.Close()

	migrator2, err := NewMigrator(filepath.Join(dataDir, dbName), Config{
		DataDir: dataDir,
	})
	if err != nil {
		t.Fatalf("NewMigrator second time: %v", err)
	}
	migrator2.Register(AllMigrations()...)

	if err := migrator2.Migrate(ctx); err != nil {
		t.Fatalf("Second migrate: %v", err)
	}

	version2, err := migrator2.CurrentVersion(ctx)
	if err != nil {
		t.Fatalf("Get version after second migrate: %v", err)
	}

	if version2 != version1 {
		t.Errorf("After second migrate: version = %d, want %d (unchanged)", version2, version1)
	}

	migrator2.Close()
}

// TestMigrateFromV1 tests migrating from v1 to the current version.
func TestMigrateFromV1(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "test.db")

	// Manually create v1 database
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("Open sqlite: %v", err)
	}
	defer db.Close()

	// Create schema_migrations table and insert v1
	_, err = db.ExecContext(ctx, `
		CREATE TABLE schema_migrations (
			version     INTEGER PRIMARY KEY,
			applied_at  INTEGER NOT NULL,
			description TEXT
		);

		CREATE TABLE settings (
			key TEXT PRIMARY KEY,
			value_json TEXT NOT NULL
		);

		CREATE TABLE auth (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			install_secret BLOB NOT NULL,
			pin_bcrypt TEXT,
			updated_at INTEGER NOT NULL
		);

		INSERT INTO auth (id, install_secret, updated_at) VALUES (1, X'0000000000000000000000000000000000000000000000000000000000000000', strftime('%s', 'now') * 1000);

		INSERT INTO schema_migrations (version, applied_at, description)
		VALUES (1, strftime('%s', 'now') * 1000, 'initial schema');
	`)
	if err != nil {
		t.Fatalf("Create v1 schema: %v", err)
	}
	db.Close()

	// Now run migrations - should apply v2 through v5
	migrator, err := NewMigrator(dbPath, Config{
		DataDir: dataDir,
	})
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	migrator.Register(AllMigrations()...)

	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("Migrate from v1: %v", err)
	}

	version, err := migrator.CurrentVersion(ctx)
	if err != nil {
		t.Fatalf("Get version after migrate: %v", err)
	}

	expectedVersion := len(AllMigrations())
	if version != expectedVersion {
		t.Errorf("After migrate from v1: version = %d, want %d", version, expectedVersion)
	}

	// Verify new tables from v2-v5 exist
	newTables := []string{
		"diurnal_baselines", "anomaly_patterns", "prediction_models", "ble_device_aliases",
	}

	db = migrator.DB()
	for _, table := range newTables {
		var name string
		err := db.QueryRowContext(ctx,
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?",
			table).Scan(&name)
		if err == sql.ErrNoRows {
			t.Errorf("New table %s does not exist after migration", table)
		} else if err != nil {
			t.Errorf("Check new table %s: %v", table, err)
		}
	}

	migrator.Close()
}

// TestMigrationRollback verifies that a failed migration rolls back
// and doesn't leave the database in a partially-migrated state.
func TestMigrationRollback(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "test.db")

	// Create a migrator with a failing migration
	failingMigrations := []Migration{
		{
			Version:     1,
			Description: "initial schema",
			Up:          migration_001_initial_schema,
		},
		{
			Version:     2,
			Description: "failing migration",
			Up: func(tx *sql.Tx) error {
				// Create a table
				if _, err := tx.Exec("CREATE TABLE test_table (id INTEGER)"); err != nil {
					return err
				}
				// Return error to trigger rollback
				return sql.ErrTxDone
			},
		},
	}

	migrator, err := NewMigrator(dbPath, Config{
		DataDir: dataDir,
	})
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	migrator.Register(failingMigrations...)

	// Apply first migration (should succeed)
	if err := migrator.applyMigration(ctx, failingMigrations[0]); err != nil {
		t.Fatalf("Apply first migration: %v", err)
	}

	version, _ := migrator.CurrentVersion(ctx)
	if version != 1 {
		t.Errorf("After first migration: version = %d, want 1", version)
	}

	// Try to apply failing migration
	err = migrator.applyMigration(ctx, failingMigrations[1])
	if err == nil {
		t.Error("Failing migration should have returned an error")
	}

	// Verify version is still 1 (rollback occurred)
	version, err = migrator.CurrentVersion(ctx)
	if err != nil {
		t.Fatalf("Get version after failed migration: %v", err)
	}
	if version != 1 {
		t.Errorf("After failed migration: version = %d, want 1 (rollback)", version)
	}

	// Verify test_table doesn't exist (rollback worked)
	db := migrator.DB()
	var tableName string
	err = db.QueryRowContext(ctx,
		"SELECT name FROM sqlite_master WHERE type='table' AND name='test_table'",
	).Scan(&tableName)
	if err != sql.ErrNoRows {
		t.Error("test_table should not exist after rollback")
	}

	migrator.Close()
}

// TestPendingMigrations verifies that pending migrations are correctly identified.
func TestPendingMigrations(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "test.db")

	migrator, err := NewMigrator(dbPath, Config{
		DataDir: dataDir,
	})
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	migrator.Register(AllMigrations()...)

	// Before any migrations, all should be pending
	pending, err := migrator.Pending(ctx)
	if err != nil {
		t.Fatalf("Get pending migrations: %v", err)
	}

	totalMigrations := len(AllMigrations())
	if len(pending) != totalMigrations {
		t.Errorf("Before migrations: pending count = %d, want %d", len(pending), totalMigrations)
	}

	// Apply first migration
	if err := migrator.applyMigration(ctx, AllMigrations()[0]); err != nil {
		t.Fatalf("Apply first migration: %v", err)
	}

	// Now one fewer should be pending
	pending, err = migrator.Pending(ctx)
	if err != nil {
		t.Fatalf("Get pending after first: %v", err)
	}

	expectedPending := totalMigrations - 1
	if len(pending) != expectedPending {
		t.Errorf("After first migration: pending count = %d, want %d", len(pending), expectedPending)
	}

	// Verify pending version numbers are correct
	for i, p := range pending {
		expectedVersion := i + 2 // v1 was applied
		if p.Version != expectedVersion {
			t.Errorf("Pending migration %d: version = %d, want %d", i, p.Version, expectedVersion)
		}
	}

	migrator.Close()
}

// TestPreMigrationBackup verifies that a backup is created before migration.
func TestPreMigrationBackup(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	dbPath := filepath.Join(dataDir, "test.db")

	// Create initial database with some data
	db, err := sql.Open("sqlite", dbPath+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatalf("Open sqlite: %v", err)
	}
	defer db.Close()

	_, err = db.ExecContext(ctx, `
		CREATE TABLE schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at INTEGER,
			description TEXT
		);
		CREATE TABLE test_data (id INTEGER, value TEXT);
		INSERT INTO test_data VALUES (1, 'test');
		INSERT INTO schema_migrations VALUES (1, strftime('%s', 'now') * 1000, 'initial');
	`)
	if err != nil {
		t.Fatalf("Create initial schema: %v", err)
	}
	db.Close()

	// Run migration - should create backup
	migrator, err := NewMigrator(dbPath, Config{
		DataDir: dataDir,
	})
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	migrator.Register(AllMigrations()...)

	// Check backups directory exists
	backupDir := filepath.Join(dataDir, "backups")
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		t.Error("Backup directory should exist after migration")
	}

	// Run migrations
	if err := migrator.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Check backup file was created
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("Read backup dir: %v", err)
	}

	if len(entries) == 0 {
		t.Error("No backup file created")
	}

	// Verify backup has correct naming pattern
	foundBackup := false
	for _, e := range entries {
		if e.Name()[:13] == "pre-upgrade-v" {
			foundBackup = true
			break
		}
	}
	if !foundBackup {
		t.Error("No pre-upgrade backup file found")
	}

	migrator.Close()
}

// TestCurrentVersion tests getting the current schema version.
func TestCurrentVersion(t *testing.T) {
	tests := []struct {
		name           string
		setupFunc      func(*sql.DB, *testing.T) error
		wantVersion    int
		wantErr        bool
	}{
		{
			name: "no migrations table",
			setupFunc: func(db *sql.DB, t *testing.T) error {
				// Empty database
				return nil
			},
			wantVersion: 0,
			wantErr:     false,
		},
		{
			name: "migrations table empty",
			setupFunc: func(db *sql.DB, t *testing.T) error {
				_, err := db.Exec("CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY)")
				return err
			},
			wantVersion: 0,
			wantErr:     false,
		},
		{
			name: "single migration applied",
			setupFunc: func(db *sql.DB, t *testing.T) error {
				_, err := db.Exec(`
					CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY);
					INSERT INTO schema_migrations VALUES (1)
				`)
				return err
			},
			wantVersion: 1,
			wantErr:     false,
		},
		{
			name: "multiple migrations applied",
			setupFunc: func(db *sql.DB, t *testing.T) error {
				_, err := db.Exec(`
					CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY);
					INSERT INTO schema_migrations VALUES (1), (2), (3)
				`)
				return err
			},
			wantVersion: 3,
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			dataDir := t.TempDir()
			dbPath := filepath.Join(dataDir, "test.db")

			db, err := sql.Open("sqlite", dbPath)
			if err != nil {
				t.Fatalf("Open sqlite: %v", err)
			}
			defer db.Close()

			if err := tt.setupFunc(db, t); err != nil {
				t.Fatalf("setupFunc: %v", err)
			}
			db.Close()

			migrator, err := NewMigrator(dbPath, Config{DataDir: dataDir})
			if err != nil {
				t.Fatalf("NewMigrator: %v", err)
			}
			defer migrator.Close()

			version, err := migrator.CurrentVersion(ctx)
			if (err != nil) != tt.wantErr {
				t.Errorf("CurrentVersion() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if version != tt.wantVersion {
				t.Errorf("CurrentVersion() = %d, want %d", version, tt.wantVersion)
			}
		})
	}
}

// TestBackupPruning verifies that old backups are pruned correctly.
func TestBackupPruning(t *testing.T) {
	dataDir := t.TempDir()
	backupDir := filepath.Join(dataDir, "backups")

	// Create some backup files with different ages
	now := time.Now()
	oldFile := filepath.Join(backupDir, "pre-upgrade-v1-to-v2-20200101-120000.sqlite")
	recentFile := filepath.Join(backupDir, "pre-upgrade-v1-to-v2-20250101-120000.sqlite")

	if err := os.MkdirAll(backupDir, 0755); err != nil {
		t.Fatalf("Create backup dir: %v", err)
	}

	// Create old file
	if err := os.WriteFile(oldFile, []byte("old"), 0644); err != nil {
		t.Fatalf("Create old file: %v", err)
	}
	// Set modtime to 100 days ago
	oldTime := now.Add(-100 * 24 * time.Hour)
	if err := os.Chtimes(oldFile, oldTime, oldTime); err != nil {
		t.Fatalf("Set old file time: %v", err)
	}

	// Create recent file
	if err := os.WriteFile(recentFile, []byte("recent"), 0644); err != nil {
		t.Fatalf("Create recent file: %v", err)
	}

	// Create migrator with short retention and prune
	migrator, err := NewMigrator(filepath.Join(dataDir, "test.db"), Config{
		DataDir:         dataDir,
		BackupRetention: 90 * 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}

	migrator.PruneOldBackups()

	// Check old file was deleted
	if _, err := os.Stat(oldFile); !os.IsNotExist(err) {
		t.Error("Old backup file should have been deleted")
	}

	// Check recent file still exists
	if _, err := os.Stat(recentFile); os.IsNotExist(err) {
		t.Error("Recent backup file should still exist")
	}

	migrator.Close()
}

// TestOpenDBFullSequence tests the full OpenDB startup sequence.
func TestOpenDBFullSequence(t *testing.T) {
	dataDir := t.TempDir()

	db, err := OpenDB(nil, dataDir, "spaxel.db")
	if err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	defer db.Close()

	// Verify database is usable
	var version int
	err = db.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version)
	if err != nil {
		t.Errorf("Query schema_migrations: %v", err)
	}

	expectedVersion := len(AllMigrations())
	if version != expectedVersion {
		t.Errorf("Schema version = %d, want %d", version, expectedVersion)
	}

	// Verify install secret exists
	var secret []byte
	err = db.QueryRow("SELECT install_secret FROM auth WHERE id = 1").Scan(&secret)
	if err != nil {
		t.Errorf("Query install secret: %v", err)
	}
	if len(secret) != 32 {
		t.Errorf("Install secret length = %d, want 32", len(secret))
	}
}
