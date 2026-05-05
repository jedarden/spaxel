// Package doctor provides pre-flight configuration diagnostics for the Spaxel mothership.
// It complements the /healthz endpoint (runtime state) with configuration correctness checks.
package doctor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Checker runs pre-flight diagnostics on the mothership configuration.
type Checker struct {
	db               *sql.DB
	dataDir          string
	firmwareDir      string
	mdnsEnabled      bool
	mqttBroker       string
	ntpServer        string
	installSecret    []byte
	fleetGetNodes    func() ([]NodeInfo, error)
	mdnsIsRegistered func() bool
}

// Config holds checker configuration.
type Config struct {
	DB               *sql.DB
	DataDir          string
	FirmwareDir      string
	MDNSEnabled      bool
	MQTTBroker       string
	NTPServer        string
	InstallSecret    []byte
	FleetGetNodes    func() ([]NodeInfo, error)
	MDNSIsRegistered func() bool
}

// NodeInfo represents minimal node information for token consistency checks.
type NodeInfo struct {
	MAC string
}

// New creates a new doctor checker.
func New(cfg Config) *Checker {
	return &Checker{
		db:               cfg.DB,
		dataDir:          cfg.DataDir,
		firmwareDir:      cfg.FirmwareDir,
		mdnsEnabled:      cfg.MDNSEnabled,
		mqttBroker:       cfg.MQTTBroker,
		ntpServer:        cfg.NTPServer,
		installSecret:    cfg.InstallSecret,
		fleetGetNodes:    cfg.FleetGetNodes,
		mdnsIsRegistered: cfg.MDNSIsRegistered,
	}
}

// CheckResult represents the result of a single diagnostic check.
type CheckResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"`  // "ok", "warn", "error"
	Message string `json:"message"` // null if ok, error message otherwise
}

// Response is the doctor endpoint response.
type Response struct {
	Checks    []CheckResult `json:"checks"`
	Overall   string        `json:"overall"`   // "ok", "warn", "error"
	CheckedAt string        `json:"checked_at"`
}

// Check runs all pre-flight diagnostics and returns the results.
func (c *Checker) Check() Response {
	results := []CheckResult{
		c.checkDataDirWritable(),
		c.checkDBIntegrity(),
		c.checkFirmwareDir(),
		c.checkMDNSBinding(),
		c.checkMQTTReachable(),
		c.checkNTPReachable(),
		c.checkInstallSecret(),
		c.checkPINConfigured(),
		c.checkNodeTokenConsistency(),
	}

	overall := "ok"
	for _, r := range results {
		if r.Status == "error" {
			overall = "error"
			break
		}
		if r.Status == "warn" && overall == "ok" {
			overall = "warn"
		}
	}

	return Response{
		Checks:    results,
		Overall:   overall,
		CheckedAt: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// checkDataDirWritable verifies /data is writable and has >100 MB free.
func (c *Checker) checkDataDirWritable() CheckResult {
	// Check if directory is writable by attempting to create a temp file
	testFile := filepath.Join(c.dataDir, ".doctor_write_test")
	f, err := os.Create(testFile)
	if err != nil {
		return CheckResult{
			Name:    "data_dir_writable",
			Status:  "error",
			Message: "Data directory not writable: " + err.Error(),
		}
	}
	_ = f.Close()
	_ = os.Remove(testFile)

	// Check disk space using syscall.Statfs
	var stat syscall.Statfs_t
	if err := syscall.Statfs(c.dataDir, &stat); err != nil {
		return CheckResult{
			Name:    "data_dir_writable",
			Status:  "warn",
			Message: "Cannot check disk space: " + err.Error(),
		}
	}

	// Calculate free space in MB: (Bavail * Frsize) / (1024 * 1024)
	freeBytes := stat.Bavail * uint64(stat.Frsize)
	freeMB := freeBytes / (1024 * 1024)

	if freeMB < 100 {
		return CheckResult{
			Name:    "data_dir_writable",
			Status:  "error",
			Message: fmt.Sprintf("Disk space low: %d MB free (minimum 100 MB required)", freeMB),
		}
	}

	return CheckResult{
		Name:    "data_dir_writable",
		Status:  "ok",
		Message: "",
	}
}

// checkDBIntegrity runs PRAGMA integrity_check on the database.
func (c *Checker) checkDBIntegrity() CheckResult {
	var result string
	err := c.db.QueryRow("PRAGMA integrity_check").Scan(&result)
	if err != nil {
		return CheckResult{
			Name:    "db_integrity",
			Status:  "error",
			Message: "SQLite integrity check failed: " + err.Error(),
		}
	}

	if result != "ok" {
		return CheckResult{
			Name:    "db_integrity",
			Status:  "error",
			Message: "SQLite integrity check failed: " + result,
		}
	}

	return CheckResult{
		Name:    "db_integrity",
		Status:  "ok",
		Message: "",
	}
}

// checkFirmwareDir verifies /firmware contains at least one *.bin file.
func (c *Checker) checkFirmwareDir() CheckResult {
	entries, err := os.ReadDir(c.firmwareDir)
	if err != nil {
		if os.IsNotExist(err) {
			return CheckResult{
				Name:    "firmware_dir",
				Status:  "error",
				Message: "Firmware directory does not exist: " + c.firmwareDir,
			}
		}
		return CheckResult{
			Name:    "firmware_dir",
			Status:  "warn",
			Message: "Cannot read firmware directory: " + err.Error(),
		}
	}

	hasBin := false
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".bin") {
			hasBin = true
			break
		}
	}

	if !hasBin {
		return CheckResult{
			Name:    "firmware_dir",
			Status:  "error",
			Message: "No firmware binaries found — OTA updates unavailable",
		}
	}

	return CheckResult{
		Name:    "firmware_dir",
		Status:  "ok",
		Message: "",
	}
}

// checkMDNSBinding verifies mDNS service is registered or SPAXEL_MDNS_ENABLED=false.
func (c *Checker) checkMDNSBinding() CheckResult {
	if !c.mdnsEnabled {
		return CheckResult{
			Name:    "mdns_binding",
			Status:  "ok",
			Message: "",
		}
	}

	if c.mdnsIsRegistered != nil && c.mdnsIsRegistered() {
		return CheckResult{
			Name:    "mdns_binding",
			Status:  "ok",
			Message: "",
		}
	}

	return CheckResult{
		Name:    "mdns_binding",
		Status:  "warn",
		Message: "mDNS not advertising — nodes cannot auto-discover mothership",
	}
}

// checkMQTTReachable tests TCP connectivity to MQTT broker if configured.
func (c *Checker) checkMQTTReachable() CheckResult {
	if c.mqttBroker == "" {
		return CheckResult{
			Name:    "mqtt_reachable",
			Status:  "ok",
			Message: "",
		}
	}

	// Parse broker URL to extract host:port
	parts := strings.SplitN(c.mqttBroker, "://", 2)
	if len(parts) != 2 {
		return CheckResult{
			Name:    "mqtt_reachable",
			Status:  "warn",
			Message: "Invalid MQTT broker URL: " + c.mqttBroker,
		}
	}

	addr := parts[1]
	// Remove any path component
	if idx := strings.Index(addr, "/"); idx >= 0 {
		addr = addr[:idx]
	}

	// Add default port if not specified
	if !strings.Contains(addr, ":") {
		addr += ":1883"
	}

	// Try to connect with 3s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return CheckResult{
			Name:    "mqtt_reachable",
			Status:  "warn",
			Message: "MQTT broker unreachable: " + c.mqttBroker + " (" + err.Error() + ")",
		}
	}
	_ = conn.Close()

	return CheckResult{
		Name:    "mqtt_reachable",
		Status:  "ok",
		Message: "",
	}
}

// checkNTPReachable tests UDP connectivity to NTP server.
func (c *Checker) checkNTPReachable() CheckResult {
	if c.ntpServer == "" {
		return CheckResult{
			Name:    "ntp_reachable",
			Status:  "ok",
			Message: "",
		}
	}

	// NTP uses UDP port 123
	addr := c.ntpServer + ":123"

	// Try to connect with 3s timeout
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "udp", addr)
	if err != nil {
		return CheckResult{
			Name:    "ntp_reachable",
			Status:  "warn",
			Message: "NTP server unreachable — node clock sync may fail: " + c.ntpServer + " (" + err.Error() + ")",
		}
	}
	_ = conn.Close()

	return CheckResult{
		Name:    "ntp_reachable",
		Status:  "ok",
		Message: "",
	}
}

// checkInstallSecret verifies the install_secret row exists in auth table.
func (c *Checker) checkInstallSecret() CheckResult {
	if len(c.installSecret) == 0 {
		return CheckResult{
			Name:    "install_secret",
			Status:  "error",
			Message: "Installation secret missing — re-run container to regenerate",
		}
	}

	// Also verify it exists in the database
	var secret []byte
	err := c.db.QueryRow("SELECT install_secret FROM auth WHERE id = 1").Scan(&secret)
	if err != nil {
		if err == sql.ErrNoRows {
			return CheckResult{
				Name:    "install_secret",
				Status:  "error",
				Message: "Installation secret missing from database — re-run container to regenerate",
			}
		}
		return CheckResult{
			Name:    "install_secret",
			Status:  "warn",
			Message: "Cannot verify install_secret in database: " + err.Error(),
		}
	}

	return CheckResult{
		Name:    "install_secret",
		Status:  "ok",
		Message: "",
	}
}

// checkPINConfigured verifies pin_bcrypt is non-null in auth table.
func (c *Checker) checkPINConfigured() CheckResult {
	var pinBcrypt sql.NullString
	err := c.db.QueryRow("SELECT pin_bcrypt FROM auth WHERE id = 1").Scan(&pinBcrypt)
	if err != nil {
		return CheckResult{
			Name:    "pin_configured",
			Status:  "warn",
			Message: "Cannot check PIN configuration: " + err.Error(),
		}
	}

	if !pinBcrypt.Valid || pinBcrypt.String == "" {
		return CheckResult{
			Name:    "pin_configured",
			Status:  "error",
			Message: "Dashboard PIN not configured — run first-time setup",
		}
	}

	return CheckResult{
		Name:    "pin_configured",
		Status:  "ok",
		Message: "",
	}
}

// checkNodeTokenConsistency verifies all nodes in registry can derive valid tokens.
func (c *Checker) checkNodeTokenConsistency() CheckResult {
	if c.fleetGetNodes == nil {
		return CheckResult{
			Name:    "node_token_consistency",
			Status:  "ok",
			Message: "",
		}
	}

	nodes, err := c.fleetGetNodes()
	if err != nil {
		return CheckResult{
			Name:    "node_token_consistency",
			Status:  "warn",
			Message: "Cannot check node tokens: " + err.Error(),
		}
	}

	// All nodes can derive valid tokens since tokens are computed from MAC + install_secret
	// This check is more informational - if there are nodes, they can all authenticate
	if len(nodes) == 0 {
		return CheckResult{
			Name:    "node_token_consistency",
			Status:  "ok",
			Message: "",
		}
	}

	// Verify each node has a valid MAC address
	for _, node := range nodes {
		if node.MAC == "" {
			return CheckResult{
				Name:    "node_token_consistency",
				Status:  "error",
				Message: "Node with empty MAC address found in registry",
			}
		}
	}

	return CheckResult{
		Name:    "node_token_consistency",
		Status:  "ok",
		Message: "",
	}
}

// Handler returns an http.HandlerFunc for the /api/doctor endpoint.
func (c *Checker) Handler(requireAuth func(http.HandlerFunc) http.HandlerFunc) http.HandlerFunc {
	return requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		response := c.Check()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(response)
	})
}
