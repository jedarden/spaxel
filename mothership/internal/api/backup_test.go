package api

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// setupTestDB creates a WAL-mode database at dir/name and runs the provided
// SQL statements.
func setupTestDB(t *testing.T, dir, name, ddl string) {
	t.Helper()
	dsn := filepath.Join(dir, name) + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(ddl); err != nil {
		t.Fatalf("exec ddl: %v", err)
	}
	if _, err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		t.Fatalf("checkpoint: %v", err)
	}
}

// doBackupRequest creates a handler, runs a backup, and returns the response
// body bytes.
func doBackupRequest(t *testing.T, dir, version string) []byte {
	t.Helper()
	handler := NewBackupHandler(dir, version)
	req := httptest.NewRequest(http.MethodGet, "/api/backup", nil)
	rec := httptest.NewRecorder()
	handler.HandleBackup(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return body
}

// openZip returns a zip reader for the given bytes.
func openZip(t *testing.T, data []byte) *zip.Reader {
	t.Helper()
	rdr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	return rdr
}

// zipEntryNames returns a set of all entry names in the zip.
func zipEntryNames(t *testing.T, data []byte) map[string]bool {
	t.Helper()
	rdr := openZip(t, data)
	names := make(map[string]bool)
	for _, f := range rdr.File {
		names[f.Name] = true
	}
	return names
}

// readZipEntry reads and returns the contents of the named zip entry.
func readZipEntry(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	rdr := openZip(t, data)
	for _, f := range rdr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open zip entry %s: %v", name, err)
			}
			defer rc.Close()
			buf, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("read zip entry %s: %v", name, err)
			}
			return buf
		}
	}
	t.Fatalf("zip entry %q not found", name)
	return nil
}

func TestBackupHandler_Headers(t *testing.T) {
	dir := t.TempDir()
	setupTestDB(t, dir, "spaxel.db", "CREATE TABLE t(id INTEGER PRIMARY KEY);")

	handler := NewBackupHandler(dir, "1.0.0")
	req := httptest.NewRequest(http.MethodGet, "/api/backup", nil)
	rec := httptest.NewRecorder()
	handler.HandleBackup(rec, req)

	resp := rec.Result()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d; want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q; want application/zip", ct)
	}
	cd := resp.Header.Get("Content-Disposition")
	if !strings.HasPrefix(cd, `attachment; filename="spaxel-backup-`) {
		t.Errorf("Content-Disposition = %q; want attachment with spaxel-backup prefix", cd)
	}
}

func TestBackupHandler_ZipContents(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(t *testing.T, dir string)
		wantFiles []string
		noFiles   []string
	}{
		{
			name: "single database and version",
			setup: func(t *testing.T, dir string) {
				setupTestDB(t, dir, "spaxel.db",
					"CREATE TABLE nodes(mac TEXT PRIMARY KEY, name TEXT);"+
						"INSERT INTO nodes VALUES('AA:BB:CC:DD:EE:FF','Kitchen');")
			},
			wantFiles: []string{"spaxel.db", "VERSION"},
		},
		{
			name: "multiple databases with floor plan",
			setup: func(t *testing.T, dir string) {
				setupTestDB(t, dir, "spaxel.db",
					"CREATE TABLE nodes(mac TEXT PRIMARY KEY);"+
						"INSERT INTO nodes VALUES('AA:BB');")
				setupTestDB(t, dir, "ble.db",
					"CREATE TABLE devices(addr TEXT PRIMARY KEY);"+
						"INSERT INTO devices VALUES('11:22');")
				fpDir := filepath.Join(dir, "floor_plan")
				if err := os.MkdirAll(fpDir, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(fpDir, "image.png"), []byte("fake-png"), 0644); err != nil {
					t.Fatal(err)
				}
			},
			wantFiles: []string{"spaxel.db", "ble.db", "VERSION", "floor_plan/image.png"},
		},
		{
			name: "no floor plan directory",
			setup: func(t *testing.T, dir string) {
				setupTestDB(t, dir, "zones.db", "CREATE TABLE zones(id INTEGER PRIMARY KEY);")
			},
			wantFiles: []string{"zones.db", "VERSION"},
			noFiles:   []string{"floor_plan"},
		},
		{
			name: "empty data dir",
			setup: func(t *testing.T, dir string) {
				// no files created
			},
			wantFiles: []string{"VERSION"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.setup != nil {
				tc.setup(t, dir)
			}

			body := doBackupRequest(t, dir, "1.0.0")
			names := zipEntryNames(t, body)

			for _, want := range tc.wantFiles {
				if !names[want] {
					t.Errorf("zip missing entry %q; got %v", want, names)
				}
			}
			for _, no := range tc.noFiles {
				for name := range names {
					if strings.HasPrefix(name, no) {
						t.Errorf("zip should not contain %q entries, got %q", no, name)
					}
				}
			}
		})
	}
}

func TestBackupHandler_DBIntegrity(t *testing.T) {
	dir := t.TempDir()

	// Create a database, write data, then write MORE data that lives in the WAL.
	dsn := filepath.Join(dir, "spaxel.db") + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.Exec("CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT)")
	db.Exec("INSERT INTO t VALUES(1,'before-backup')")

	// Don't close db — simulate the mothership still writing while backup runs.
	db.Exec("INSERT INTO t VALUES(2,'in-wal')")

	body := doBackupRequest(t, dir, "1.0.0")

	// Extract spaxel.db from zip and verify integrity.
	dbBytes := readZipEntry(t, body, "spaxel.db")

	// Write to a temp file so sqlite can open it.
	tmp := filepath.Join(t.TempDir(), "restored.db")
	if err := os.WriteFile(tmp, dbBytes, 0644); err != nil {
		t.Fatal(err)
	}

	rdb, err := sql.Open("sqlite", tmp)
	if err != nil {
		t.Fatalf("open restored db: %v", err)
	}
	defer rdb.Close()

	var ok string
	if err := rdb.QueryRow("PRAGMA quick_check(1)").Scan(&ok); err != nil {
		t.Fatalf("integrity check failed: %v", err)
	}
	if ok != "ok" {
		t.Fatalf("integrity check: %s", ok)
	}

	// Verify both rows are present (WAL data was included in backup).
	var count int
	if err := rdb.QueryRow("SELECT count(*) FROM t").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("row count = %d; want 2 (WAL data should be included)", count)
	}

	db.Close()
}

func TestBackupHandler_SimultaneousWrite(t *testing.T) {
	dir := t.TempDir()

	// Create a database with initial data.
	dsn := filepath.Join(dir, "spaxel.db") + "?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	db.Exec("CREATE TABLE t(id INTEGER PRIMARY KEY, v TEXT)")
	db.Exec("INSERT INTO t VALUES(1,'original')")

	// Run the backup while writing concurrently.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 2; i <= 20; i++ {
			db.Exec(fmt.Sprintf("INSERT INTO t VALUES(%d,'concurrent-%d')", i, i))
		}
	}()

	body := doBackupRequest(t, dir, "1.0.0")
	<-done

	// Verify the backup is a valid database.
	dbBytes := readZipEntry(t, body, "spaxel.db")
	tmp := filepath.Join(t.TempDir(), "concurrent.db")
	if err := os.WriteFile(tmp, dbBytes, 0644); err != nil {
		t.Fatal(err)
	}

	rdb, err := sql.Open("sqlite", tmp)
	if err != nil {
		t.Fatalf("open restored db: %v", err)
	}
	defer rdb.Close()

	var ok string
	if err := rdb.QueryRow("PRAGMA quick_check(1)").Scan(&ok); err != nil {
		t.Fatalf("integrity check failed: %v", err)
	}
	if ok != "ok" {
		t.Fatalf("integrity check during concurrent writes: %s", ok)
	}
}

func TestBackupHandler_BackupSize(t *testing.T) {
	dir := t.TempDir()

	var rows strings.Builder
	rows.WriteString("CREATE TABLE data(v TEXT);")
	for i := 0; i < 100; i++ {
		rows.WriteString(fmt.Sprintf("INSERT INTO data VALUES('row-%04d-some-data-here');", i))
	}
	setupTestDB(t, dir, "analytics.db", rows.String())

	body := doBackupRequest(t, dir, "1.0.0")

	if len(body) == 0 {
		t.Error("backup size = 0 bytes; want non-empty")
	}
	if len(body) > 1<<20 {
		t.Errorf("backup size = %d bytes; want < 1 MB", len(body))
	}
}

func TestBackupHandler_VersionFile(t *testing.T) {
	dir := t.TempDir()
	setupTestDB(t, dir, "spaxel.db", "CREATE TABLE t(id INTEGER PRIMARY KEY);")

	version := "2.5.0-rc1"
	body := doBackupRequest(t, dir, version)

	content := readZipEntry(t, body, "VERSION")
	got := strings.TrimSpace(string(content))
	if got != version {
		t.Errorf("VERSION = %q; want %q", got, version)
	}
}
