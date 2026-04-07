// Package api provides the backup streaming endpoint.
package api

import (
	"archive/zip"
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"modernc.org/sqlite"
)

// BackupHandler handles GET /api/backup — streams a zip of all databases
// and supporting files directly to the HTTP response without temp files.
type BackupHandler struct {
	dataDir string
	version string
}

// NewBackupHandler creates a backup handler that will archive every .db
// file found inside dataDir, plus optional floor_plan/ and a VERSION file.
func NewBackupHandler(dataDir, version string) *BackupHandler {
	return &BackupHandler{dataDir: dataDir, version: version}
}

// HandleBackup streams a zip archive to w.
//
// Zip layout:
//
//	spaxel-backup-<timestamp>.zip
//	├── *.db               — one entry per database file found in dataDir
//	├── floor_plan/        — if the directory exists
//	│   └── ...
//	└── VERSION            — mothership version string
func (h *BackupHandler) HandleBackup(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	timestamp := start.UTC().Format("2006-01-02")

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf(`attachment; filename="spaxel-backup-%s.zip"`, timestamp))

	// We write directly into the response — no temp file on disk.
	zw := zip.NewWriter(w)
	defer zw.Close()

	// 1. Back up every .db file found in dataDir using the Online Backup API.
	if err := h.backupDatabases(zw); err != nil {
		log.Printf("[ERROR] backup: database backup failed: %v", err)
		http.Error(w, "backup failed", http.StatusInternalServerError)
		return
	}

	// 2. Include floor_plan/ directory if it exists.
	if err := h.backupDirectory(zw, "floor_plan"); err != nil {
		log.Printf("[WARN] backup: floor_plan backup skipped: %v", err)
	}

	// 3. Include VERSION file.
	if fw, err := zw.Create("VERSION"); err == nil {
		fw.Write([]byte(h.version + "\n"))
	}

	if err := zw.Close(); err != nil {
		log.Printf("[ERROR] backup: zip close failed: %v", err)
		return
	}

	log.Printf("[INFO] backup completed in %s", time.Since(start))
}

// backupDatabases finds all .db files in dataDir, uses the SQLite Online
// Backup API to create a consistent snapshot of each, and adds the snapshot
// to the zip.
func (h *BackupHandler) backupDatabases(zw *zip.Writer) error {
	entries, err := os.ReadDir(h.dataDir)
	if err != nil {
		return fmt.Errorf("read data dir: %w", err)
	}

	// Collect .db files and sort for deterministic zip ordering.
	var dbFiles []string
	for _, e := range entries {
		if !e.Type().IsRegular() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".db") {
			dbFiles = append(dbFiles, e.Name())
		}
	}
	sort.Strings(dbFiles)

	for _, name := range dbFiles {
		dbPath := filepath.Join(h.dataDir, name)
		if err := h.backupOneDB(zw, dbPath, name); err != nil {
			log.Printf("[WARN] backup: skipping %s: %v", name, err)
			continue
		}
	}

	return nil
}

// backupOneDB uses the SQLite Online Backup API to produce a consistent
// snapshot of the database at dbPath, then writes the serialized bytes into
// the zip entry named zipName.
//
// The Online Backup API copies page-by-page; readers and writers continue
// uninterrupted. No temp file is written — the backup is serialized from
// an in-memory copy directly to the zip stream.
func (h *BackupHandler) backupOneDB(zw *zip.Writer, dbPath, zipName string) error {
	dsn := dbPath + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer db.Close()

	conn, err := db.Conn(context.Background())
	if err != nil {
		return fmt.Errorf("conn: %w", err)
	}
	defer conn.Close()

	var backupBytes []byte

	err = conn.Raw(func(driverConn any) error {
		// Assert the backuper interface to access the Online Backup API.
		bp, ok := driverConn.(interface {
			NewBackup(dstUri string) (*sqlite.Backup, error)
		})
		if !ok {
			return fmt.Errorf("driver does not support online backup")
		}

		// Create an in-memory destination and initialise the backup.
		bck, err := bp.NewBackup(":memory:")
		if err != nil {
			return fmt.Errorf("backup init: %w", err)
		}

		// Copy pages 100 at a time until done.
		const pagesPerStep = 100
		for {
			more, err := bck.Step(pagesPerStep)
			if err != nil {
				bck.Finish()
				return fmt.Errorf("backup step: %w", err)
			}
			if !more {
				break
			}
		}

		// Finish the backup but keep the destination connection open.
		dstConn, err := bck.Commit()
		if err != nil {
			return fmt.Errorf("backup commit: %w", err)
		}
		defer dstConn.Close()

		// Serialize the in-memory database to bytes.
		ser, ok := dstConn.(interface {
			Serialize() ([]byte, error)
		})
		if !ok {
			return fmt.Errorf("driver does not support serialize")
		}

		backupBytes, err = ser.Serialize()
		return err
	})
	if err != nil {
		return err
	}

	if len(backupBytes) == 0 {
		return fmt.Errorf("empty database backup")
	}

	fw, err := zw.Create(zipName)
	if err != nil {
		return fmt.Errorf("zip create: %w", err)
	}
	if _, err := fw.Write(backupBytes); err != nil {
		return fmt.Errorf("zip write: %w", err)
	}

	log.Printf("[DEBUG] backup: %s (%d bytes)", zipName, len(backupBytes))
	return nil
}

// backupDirectory adds every file under dirName (relative to dataDir) into
// the zip, preserving directory structure.  Silently skips if the directory
// does not exist.
func (h *BackupHandler) backupDirectory(zw *zip.Writer, dirName string) error {
	dirPath := filepath.Join(h.dataDir, dirName)
	info, err := os.Stat(dirPath)
	if err != nil || !info.IsDir() {
		return nil // not present — skip silently
	}

	return filepath.WalkDir(dirPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(h.dataDir, path)
		if err != nil {
			return err
		}
		// zip paths must use forward slashes.
		rel = filepath.ToSlash(rel)

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", rel, err)
		}

		fw, err := zw.Create(rel)
		if err != nil {
			return fmt.Errorf("zip create %s: %w", rel, err)
		}
		if _, err := fw.Write(data); err != nil {
			return fmt.Errorf("zip write %s: %w", rel, err)
		}
		return nil
	})
}
