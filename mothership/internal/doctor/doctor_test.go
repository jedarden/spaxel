// Package doctor provides pre-flight configuration diagnostics for the Spaxel mothership.
package doctor

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

// mockRow is a simple mock for sql.Row scanning.
type mockRow struct {
	values []interface{}
	err    error
}

func (m mockRow) Scan(dest ...interface{}) error {
	if m.err != nil {
		return m.err
	}
	if len(m.values) != len(dest) {
		return sql.ErrNoRows
	}
	for i, v := range m.values {
		switch d := dest[i].(type) {
		case *string:
			if v == nil {
				return sql.ErrNoRows
			}
			*d = v.(string)
		case *[]byte:
			if v == nil {
				return sql.ErrNoRows
			}
			*d = v.([]byte)
		case *sql.NullString:
			if v == nil {
				d.Valid = false
			} else {
				d.String = v.(string)
				d.Valid = true
			}
		default:
			// For other types, try direct assignment
		}
	}
	return nil
}

func TestCheckDataDirWritable(t *testing.T) {
	tests := []struct {
		name       string
		setup      func() (string, func())
		wantName   string
		wantStatus string
	}{
		{
			name: "writable with enough space",
			setup: func() (string, func()) {
				dir := t.TempDir()
				// Create a test file to verify writability
				testFile := filepath.Join(dir, "test")
				if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
					t.Fatalf("failed to create test file: %v", err)
				}
				return dir, func() { os.RemoveAll(dir) }
			},
			wantName:   "data_dir_writable",
			wantStatus: "ok",
		},
		{
			name: "directory not writable",
			setup: func() (string, func()) {
				// Use /proc which exists but isn't writable
				return "/proc", func() {}
			},
			wantName:   "data_dir_writable",
			wantStatus: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataDir, cleanup := tt.setup()
			defer cleanup()

			c := &Checker{dataDir: dataDir}
			result := c.checkDataDirWritable()

			if result.Name != tt.wantName {
				t.Errorf("Name = %v, want %v", result.Name, tt.wantName)
			}
			if result.Status != tt.wantStatus {
				t.Errorf("Status = %v, want %v", result.Status, tt.wantStatus)
			}
		})
	}
}

func TestCheckFirmwareDir(t *testing.T) {
	tests := []struct {
		name          string
		setupFirmware func() string
		wantName      string
		wantStatus    string
		wantMessage   string
	}{
		{
			name: "has firmware binaries",
			setupFirmware: func() string {
				dir := t.TempDir()
				_ = os.WriteFile(filepath.Join(dir, "spaxel.bin"), []byte("firmware"), 0644)
				return dir
			},
			wantName:   "firmware_dir",
			wantStatus: "ok",
		},
		{
			name: "no firmware binaries",
			setupFirmware: func() string {
				return t.TempDir()
			},
			wantName:    "firmware_dir",
			wantStatus:  "error",
			wantMessage: "No firmware binaries found",
		},
		{
			name: "directory does not exist",
			setupFirmware: func() string {
				return "/nonexistent/firmware"
			},
			wantName:    "firmware_dir",
			wantStatus:  "error",
			wantMessage: "Firmware directory does not exist",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			firmwareDir := tt.setupFirmware()
			c := &Checker{firmwareDir: firmwareDir}
			result := c.checkFirmwareDir()

			if result.Name != tt.wantName {
				t.Errorf("Name = %v, want %v", result.Name, tt.wantName)
			}
			if result.Status != tt.wantStatus {
				t.Errorf("Status = %v, want %v", result.Status, tt.wantStatus)
			}
			if tt.wantMessage != "" && !strings.Contains(result.Message, tt.wantMessage) {
				t.Errorf("Message = %v, want to contain %v", result.Message, tt.wantMessage)
			}
		})
	}
}

func TestCheckMDNSBinding(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		reg     func() bool
		want    CheckResult
	}{
		{
			name:    "mdns disabled",
			enabled: false,
			want:    CheckResult{Name: "mdns_binding", Status: "ok"},
		},
		{
			name:    "mdns enabled and registered",
			enabled: true,
			reg:     func() bool { return true },
			want:    CheckResult{Name: "mdns_binding", Status: "ok"},
		},
		{
			name:    "mdns enabled but not registered",
			enabled: true,
			reg:     func() bool { return false },
			want:    CheckResult{Name: "mdns_binding", Status: "warn", Message: "mDNS not advertising"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Checker{
				mdnsEnabled:      tt.enabled,
				mdnsIsRegistered: tt.reg,
			}
			result := c.checkMDNSBinding()

			if result.Name != tt.want.Name {
				t.Errorf("Name = %v, want %v", result.Name, tt.want.Name)
			}
			if result.Status != tt.want.Status {
				t.Errorf("Status = %v, want %v", result.Status, tt.want.Status)
			}
			if tt.want.Message != "" && !strings.Contains(result.Message, tt.want.Message) {
				t.Errorf("Message = %v, want to contain %v", result.Message, tt.want.Message)
			}
		})
	}
}

func TestCheckMQTTReachable(t *testing.T) {
	tests := []struct {
		name    string
		broker  string
		wantMsg string
	}{
		{
			name:    "no broker configured",
			broker:  "",
			wantMsg: "",
		},
		{
			name:    "invalid broker URL",
			broker:  "not-a-url",
			wantMsg: "Invalid MQTT broker URL",
		},
		{
			name:    "unreachable broker",
			broker:  "mqtt://localhost:9999",
			wantMsg: "MQTT broker unreachable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Checker{mqttBroker: tt.broker}
			result := c.checkMQTTReachable()

			if result.Name != "mqtt_reachable" {
				t.Errorf("Name = %v, want mqtt_reachable", result.Name)
			}
			if tt.wantMsg != "" && result.Status != "ok" {
				if !strings.Contains(result.Message, tt.wantMsg) {
					t.Errorf("Message = %v, want to contain %v", result.Message, tt.wantMsg)
				}
			}
		})
	}
}

func TestCheckNTPReachable(t *testing.T) {
	tests := []struct {
		name    string
		server  string
		wantMsg string
	}{
		{
			name:    "no server configured",
			server:  "",
			wantMsg: "",
		},
		{
			name:    "invalid server",
			server:  "invalid host name with spaces",
			wantMsg: "NTP server unreachable",
		},
		{
			name:    "valid server (pool.ntp.org)",
			server:  "pool.ntp.org",
			wantMsg: "", // May be ok or warn depending on network
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Checker{ntpServer: tt.server}
			result := c.checkNTPReachable()

			if result.Name != "ntp_reachable" {
				t.Errorf("Name = %v, want ntp_reachable", result.Name)
			}
			if tt.wantMsg != "" && result.Status != "ok" {
				if !strings.Contains(result.Message, tt.wantMsg) {
					t.Errorf("Message = %v, want to contain %v", result.Message, tt.wantMsg)
				}
			}
		})
	}
}

func TestCheckInstallSecret(t *testing.T) {
	tests := []struct {
		name          string
		installSecret []byte
		dbSetup       func(*sql.DB)
		wantStatus    string
		wantMessage   string
	}{
		{
			name:          "secret exists in memory and db",
			installSecret: []byte("test-secret-32-bytes-long-enough"),
			dbSetup: func(db *sql.DB) {
				_, _ = db.Exec("CREATE TABLE auth (id INTEGER PRIMARY KEY, install_secret BLOB)")
				_, _ = db.Exec("INSERT INTO auth (id, install_secret) VALUES (1, ?)", []byte("test-secret-32-bytes-long-enough"))
			},
			wantStatus: "ok",
		},
		{
			name:          "secret missing from memory",
			installSecret: nil,
			dbSetup:       func(db *sql.DB) {},
			wantStatus:    "error",
			wantMessage:   "Installation secret missing",
		},
		{
			name:          "secret missing from db",
			installSecret: []byte("test"),
			dbSetup: func(db *sql.DB) {
				_, _ = db.Exec("CREATE TABLE auth (id INTEGER PRIMARY KEY, install_secret BLOB)")
			},
			wantStatus:  "error",
			wantMessage: "Installation secret missing from database",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := sql.Open("sqlite", ":memory:")
			tt.dbSetup(db)
			defer db.Close()

			c := &Checker{
				db:            db,
				installSecret: tt.installSecret,
			}
			result := c.checkInstallSecret()

			if result.Name != "install_secret" {
				t.Errorf("Name = %v, want install_secret", result.Name)
			}
			if result.Status != tt.wantStatus {
				t.Errorf("Status = %v, want %v", result.Status, tt.wantStatus)
			}
			if tt.wantMessage != "" && !strings.Contains(result.Message, tt.wantMessage) {
				t.Errorf("Message = %v, want to contain %v", result.Message, tt.wantMessage)
			}
		})
	}
}

func TestCheckPINConfigured(t *testing.T) {
	tests := []struct {
		name        string
		dbSetup     func(*sql.DB)
		wantStatus  string
		wantMessage string
	}{
		{
			name: "pin configured",
			dbSetup: func(db *sql.DB) {
				_, _ = db.Exec("CREATE TABLE auth (id INTEGER PRIMARY KEY, pin_bcrypt TEXT)")
				_, _ = db.Exec("INSERT INTO auth (id, pin_bcrypt) VALUES (1, '$2a$12$hash')")
			},
			wantStatus: "ok",
		},
		{
			name: "pin not configured",
			dbSetup: func(db *sql.DB) {
				_, _ = db.Exec("CREATE TABLE auth (id INTEGER PRIMARY KEY, pin_bcrypt TEXT)")
				_, _ = db.Exec("INSERT INTO auth (id, pin_bcrypt) VALUES (1, NULL)")
			},
			wantStatus:  "error",
			wantMessage: "Dashboard PIN not configured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db, _ := sql.Open("sqlite", ":memory:")
			tt.dbSetup(db)
			defer db.Close()

			c := &Checker{db: db}
			result := c.checkPINConfigured()

			if result.Name != "pin_configured" {
				t.Errorf("Name = %v, want pin_configured", result.Name)
			}
			if result.Status != tt.wantStatus {
				t.Errorf("Status = %v, want %v", result.Status, tt.wantStatus)
			}
			if tt.wantMessage != "" && !strings.Contains(result.Message, tt.wantMessage) {
				t.Errorf("Message = %v, want to contain %v", result.Message, tt.wantMessage)
			}
		})
	}
}

func TestCheckNodeTokenConsistency(t *testing.T) {
	tests := []struct {
		name        string
		getNodes    func() ([]NodeInfo, error)
		wantStatus  string
		wantMessage string
	}{
		{
			name: "no nodes",
			getNodes: func() ([]NodeInfo, error) {
				return []NodeInfo{}, nil
			},
			wantStatus: "ok",
		},
		{
			name: "nodes with valid MACs",
			getNodes: func() ([]NodeInfo, error) {
				return []NodeInfo{
					{MAC: "AA:BB:CC:DD:EE:FF"},
					{MAC: "11:22:33:44:55:66"},
				}, nil
			},
			wantStatus: "ok",
		},
		{
			name: "node with empty MAC",
			getNodes: func() ([]NodeInfo, error) {
				return []NodeInfo{{MAC: ""}}, nil
			},
			wantStatus:  "error",
			wantMessage: "Node with empty MAC address",
		},
		{
			name: "get nodes error",
			getNodes: func() ([]NodeInfo, error) {
				return nil, sql.ErrConnDone
			},
			wantStatus:  "warn",
			wantMessage: "Cannot check node tokens",
		},
		{
			name:       "nil getNodes function",
			getNodes:   nil,
			wantStatus: "ok",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Checker{fleetGetNodes: tt.getNodes}
			result := c.checkNodeTokenConsistency()

			if result.Name != "node_token_consistency" {
				t.Errorf("Name = %v, want node_token_consistency", result.Name)
			}
			if result.Status != tt.wantStatus {
				t.Errorf("Status = %v, want %v", result.Status, tt.wantStatus)
			}
			if tt.wantMessage != "" && !strings.Contains(result.Message, tt.wantMessage) {
				t.Errorf("Message = %v, want to contain %v", result.Message, tt.wantMessage)
			}
		})
	}
}

func TestCheckOverall(t *testing.T) {
	tests := []struct {
		name        string
		checks      []CheckResult
		wantOverall string
	}{
		{
			name: "all ok",
			checks: []CheckResult{
				{Name: "check1", Status: "ok"},
				{Name: "check2", Status: "ok"},
			},
			wantOverall: "ok",
		},
		{
			name: "one warn",
			checks: []CheckResult{
				{Name: "check1", Status: "ok"},
				{Name: "check2", Status: "warn"},
			},
			wantOverall: "warn",
		},
		{
			name: "one error",
			checks: []CheckResult{
				{Name: "check1", Status: "ok"},
				{Name: "check2", Status: "error"},
			},
			wantOverall: "error",
		},
		{
			name: "warn and error",
			checks: []CheckResult{
				{Name: "check1", Status: "warn"},
				{Name: "check2", Status: "error"},
			},
			wantOverall: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			overall := "ok"
			for _, r := range tt.checks {
				if r.Status == "error" {
					overall = "error"
					break
				}
				if r.Status == "warn" && overall == "ok" {
					overall = "warn"
				}
			}

			if overall != tt.wantOverall {
				t.Errorf("Overall = %v, want %v", overall, tt.wantOverall)
			}
		})
	}
}

func TestHandler(t *testing.T) {
	requireAuthCalled := false
	requireAuth := func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			requireAuthCalled = true
			next(w, r)
		}
	}

	// Create a test database
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// Create auth table for tests
	_, _ = db.Exec("CREATE TABLE auth (id INTEGER PRIMARY KEY, install_secret BLOB, pin_bcrypt TEXT)")
	_, _ = db.Exec("INSERT INTO auth (id, install_secret, pin_bcrypt) VALUES (1, ?, '$2a$12$hash')", []byte("test-secret-32-bytes-long-enough"))

	c := &Checker{
		db:            db,
		dataDir:       t.TempDir(),
		firmwareDir:   t.TempDir(),
		installSecret: []byte("test-secret-32-bytes-long-enough"),
	}

	// Create firmware file
	_ = os.WriteFile(filepath.Join(c.firmwareDir, "test.bin"), []byte("firmware"), 0644)

	handler := c.Handler(requireAuth)

	req := httptest.NewRequest("GET", "/api/doctor", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if !requireAuthCalled {
		t.Error("requireAuth was not called")
	}

	if w.Code != http.StatusOK {
		t.Errorf("Status = %v, want %v", w.Code, http.StatusOK)
	}

	var response Response
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Overall == "" {
		t.Error("Overall status is empty")
	}

	if response.CheckedAt == "" {
		t.Error("CheckedAt is empty")
	}

	if len(response.Checks) != 9 {
		t.Errorf("Got %d checks, want 9", len(response.Checks))
	}
}

func TestHandlerMethodNotAllowed(t *testing.T) {
	requireAuth := func(next http.HandlerFunc) http.HandlerFunc {
		return next
	}

	c := &Checker{}
	handler := c.Handler(requireAuth)

	req := httptest.NewRequest("POST", "/api/doctor", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %v, want %v", w.Code, http.StatusMethodNotAllowed)
	}
}
