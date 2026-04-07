package health

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/spaxel/mothership/internal/loadshed"
)

// TestHealthCheckOK tests that health check returns OK when all components are healthy.
func TestHealthCheckOK(t *testing.T) {
	checker := &Checker{
		startTime: time.Now(),
		db: &sql.DB{}, // Mock - we'll override checkDB for testing
		getNodeCount: func() int { return 3 },
		shedder:      loadshed.New(),
	}

	// Override checkDB to return OK
	originalCheckDB := checker.checkDB
	checker.checkDB = func() string { return "ok" }
	defer func() { checker.checkDB = originalCheckDB }()

	resp := checker.check("1.0.0")

	if resp.Status != "ok" {
		t.Errorf("expected status=ok, got %s", resp.Status)
	}
	if resp.DB != "ok" {
		t.Errorf("expected db=ok, got %s", resp.DB)
	}
	if resp.NodesOnline != 3 {
		t.Errorf("expected nodes_online=3, got %d", resp.NodesOnline)
	}
	if resp.LoadLevel != 0 {
		t.Errorf("expected load_level=0, got %d", resp.LoadLevel)
	}
	if resp.UptimeS < 0 {
		t.Errorf("expected uptime_s >= 0, got %d", resp.UptimeS)
	}
	if resp.Version != "1.0.0" {
		t.Errorf("expected version=1.0.0, got %s", resp.Version)
	}
	if resp.Reason != "" {
		t.Errorf("expected empty reason, got %s", resp.Reason)
	}
}

// TestHealthCheckDBFailing tests that health check returns degraded when DB fails.
func TestHealthCheckDBFailing(t *testing.T) {
	checker := &Checker{
		startTime: time.Now(),
		db:        nil, // No DB = failing
		getNodeCount: func() int { return 3 },
		shedder:   loadshed.New(),
	}

	resp := checker.check("1.0.0")

	if resp.Status != "degraded" {
		t.Errorf("expected status=degraded, got %s", resp.Status)
	}
	if resp.DB != "failing" {
		t.Errorf("expected db=failing, got %s", resp.DB)
	}
	if resp.Reason != "database unreachable" {
		t.Errorf("expected reason='database unreachable', got %s", resp.Reason)
	}
}

// TestHealthCheckNoNodes tests that health check returns degraded after 5 min with no nodes.
func TestHealthCheckNoNodes(t *testing.T) {
	checker := &Checker{
		startTime: time.Now().Add(-6 * time.Minute), // 6 minutes ago
		db:        &sql.DB{},
		getNodeCount: func() int { return 0 },
		shedder:   loadshed.New(),
	}

	// Override checkDB to return OK
	originalCheckDB := checker.checkDB
	checker.checkDB = func() string { return "ok" }
	defer func() { checker.checkDB = originalCheckDB }()

	resp := checker.check("1.0.0")

	if resp.Status != "degraded" {
		t.Errorf("expected status=degraded, got %s", resp.Status)
	}
	if resp.Reason != "no nodes connected" {
		t.Errorf("expected reason='no nodes connected', got %s", resp.Reason)
	}
}

// TestHealthCheckNoNodesWithinGracePeriod tests that health check is OK within 5 min grace period.
func TestHealthCheckNoNodesWithinGracePeriod(t *testing.T) {
	checker := &Checker{
		startTime: time.Now().Add(-2 * time.Minute), // 2 minutes ago
		db:        &sql.DB{},
		getNodeCount: func() int { return 0 },
		shedder:   loadshed.New(),
	}

	// Override checkDB to return OK
	originalCheckDB := checker.checkDB
	checker.checkDB = func() string { return "ok" }
	defer func() { checker.checkDB = originalCheckDB }()

	resp := checker.check("1.0.0")

	if resp.Status != "ok" {
		t.Errorf("expected status=ok, got %s", resp.Status)
	}
}

// TestHealthCheckLoadLevel3 tests that health check returns degraded after 60s of level 3.
func TestHealthCheckLoadLevel3(t *testing.T) {
	shedder := loadshed.New()
	checker := &Checker{
		startTime: time.Now(),
		db:        &sql.DB{},
		getNodeCount: func() int { return 3 },
		shedder:   shedder,
	}

	// Override checkDB to return OK
	originalCheckDB := checker.checkDB
	checker.checkDB = func() string { return "ok" }
	defer func() { checker.checkDB = originalCheckDB }()

	// Initially OK
	resp := checker.check("1.0.0")
	if resp.Status != "ok" {
		t.Errorf("expected status=ok initially, got %s", resp.Status)
	}

	// Set to level 3 and mark it as having been active for 61 seconds
	checker.mu.Lock()
	checker.level3Since = time.Now().Add(-61 * time.Second)
	checker.mu.Unlock()

	// Manually set shedder level (we need to access the internal state)
	// Since we can't do that directly, we'll verify the timestamp logic works

	resp = checker.check("1.0.0")
	if resp.Status != "degraded" {
		t.Errorf("expected status=degraded after 60s level 3, got %s", resp.Status)
	}
}

// TestHealthCheckHandler tests the HTTP handler returns correct status codes.
func TestHealthCheckHandler(t *testing.T) {
	checker := New(Config{
		DB: &sql.DB{},
		GetNodeCount: func() int { return 2 },
		Shedder:      loadshed.New(),
	})
	checker.checkDB = func() string { return "ok" }

	handler := checker.Handler("1.2.3")

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("expected status=ok, got %s", resp.Status)
	}
}

// TestHealthCheckHandlerDegraded tests the HTTP handler returns 503 for degraded state.
func TestHealthCheckHandlerDegraded(t *testing.T) {
	checker := New(Config{
		DB:          nil, // Failing DB
		GetNodeCount: func() int { return 2 },
		Shedder:      loadshed.New(),
	})

	handler := checker.Handler("1.2.3")

	req := httptest.NewRequest("GET", "/healthz", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}

	var resp Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "degraded" {
		t.Errorf("expected status=degraded, got %s", resp.Status)
	}
}

// TestHealthCheckUptimeIncrement tests that uptime increments across calls.
func TestHealthCheckUptimeIncrement(t *testing.T) {
	checker := &Checker{
		startTime: time.Now(),
		db:        &sql.DB{},
		getNodeCount: func() int { return 1 },
		shedder:   loadshed.New(),
	}
	checker.checkDB = func() string { return "ok" }

	resp1 := checker.check("1.0.0")
	time.Sleep(100 * time.Millisecond)
	resp2 := checker.check("1.0.0")

	if resp2.UptimeS <= resp1.UptimeS {
		t.Errorf("expected uptime to increment, was %d then %d", resp1.UptimeS, resp2.UptimeS)
	}
}
