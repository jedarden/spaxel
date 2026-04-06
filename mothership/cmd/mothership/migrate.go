//go:build ignore_migrate

// Command-line migration tool for Spaxel mothership.
// This is a standalone binary for running migrations without starting the full application.
//
// Build:
//   go build -tags ignore_migrate -o migrate ./cmd/mothership
//
// Usage:
//   ./migrate [options]
//   ./migrate --version
//   ./migrate --prune
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/spaxel/mothership/internal/db"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := runMigrate(ctx, os.Args[1:]); err != nil {
		log.Printf("[ERROR] %v", err)
		os.Exit(1)
	}
}

func runMigrate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	dataDir := fs.String("data", "/data", "Data directory")
	dbName := fs.String("db", "spaxel.db", "Database name")
	showVersion := fs.Bool("version", false, "Show current schema version and exit")
	prune := fs.Bool("prune", false, "Prune old backups and exit")
	help := fs.Bool("help", false, "Show help")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if *help {
		printMigrateHelp(fs)
		return nil
	}

	dbPath := filepath.Join(*dataDir, *dbName)

	// Show current version
	if *showVersion {
		current, err := db.CurrentVersion(*dataDir, *dbName)
		if err != nil {
			return fmt.Errorf("get current version: %w", err)
		}
		fmt.Printf("Current schema version: %d\n", current)
		return nil
	}

	// Prune old backups
	if *prune {
		migrator, err := db.NewMigrator(dbPath, db.Config{
			DataDir:         *dataDir,
			BackupRetention: 90 * 24 * time.Hour,
		})
		if err != nil {
			return fmt.Errorf("create migrator: %w", err)
		}
		defer migrator.Close()

		log.Printf("[INFO] Pruning old backups in %s", filepath.Join(*dataDir, "backups"))
		migrator.PruneOldBackups()
		return nil
	}

	// Run migrations
	log.Printf("[INFO] Starting database migration")
	log.Printf("[INFO] Database: %s", dbPath)
	log.Printf("[INFO] Data directory: %s", *dataDir)

	start := time.Now()
	if err := db.RunMigrations(*dataDir, *dbName); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	duration := time.Since(start)
	log.Printf("[INFO] Migration completed successfully in %v", duration)

	// Show final version
	current, err := db.CurrentVersion(*dataDir, *dbName)
	if err != nil {
		return err
	}
	log.Printf("[INFO] Schema version: %d", current)

	return nil
}

func printMigrateHelp(fs *flag.FlagSet) {
	fmt.Printf("Usage: migrate [options]\n\n")
	fmt.Printf("Options:\n")
	fs.PrintDefaults()
	fmt.Printf("\nExamples:\n")
	fmt.Printf("  migrate                                   # Run pending migrations\n")
	fmt.Printf("  migrate --version                        # Show current schema version\n")
	fmt.Printf("  migrate --prune                          # Prune old backups\n")
	fmt.Printf("  migrate --data /data/spaxel --db mothership.db\n")
}
