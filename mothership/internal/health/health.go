// Package health provides comprehensive health checking for the mothership.
package health

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/spaxel/mothership/internal/loadshed"
)

// Checker provides health check functionality for the mothership.
type Checker struct {
	mu            sync.RWMutex
	startTime     time.Time
	db            *sql.DB
	getNodeCount  func() int
	shedder       *loadshed.Shedder
	level3Since   time.Time // When level 3 shedding started
}

// Config holds configuration for the health checker.
type Config struct {
	DB           *sql.DB
	GetNodeCount func() int
	Shedder      *loadshed.Shedder
}

// New creates a new health checker.
func New(cfg Config) *Checker {
	return &Checker{
		startTime:    time.Now(),
		db:          cfg.DB,
		getNodeCount: cfg.GetNodeCount,
		shedder:     cfg.Shedder,
	}
}

// Response is the health check response JSON structure.
type Response struct {
	Status      string  `json:"status"`      // "ok" or "degraded"
	UptimeS     int64   `json:"uptime_s"`    // seconds since start
	Version     string  `json:"version"`     // mothership version
	NodesOnline int     `json:"nodes_online"` // count of connected nodes
	DB          string  `json:"db"`          // "ok" or "failing"
	LoadLevel   int     `json:"load_level"`  // 0-3, current load shedding level
	Reason      string  `json:"reason,omitempty"` // explanation of degradation (only when status=degraded)
}

// Handler returns an http.HandlerFunc that performs the health check.
func (c *Checker) Handler(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp := c.check(version)

		if resp.Status == "ok" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}
}

// check performs the health check and returns the response.
func (c *Checker) check(version string) Response {
	c.mu.Lock()
	defer c.mu.Unlock()

	uptime := int64(time.Since(c.startTime).Seconds())

	// Check database health with 100ms timeout
	dbStatus := c.checkDB()

	// Get node count
	nodesOnline := 0
	if c.getNodeCount != nil {
		nodesOnline = c.getNodeCount()
	}

	// Get load level (0-3)
	loadLevel := 0
	if c.shedder != nil {
		loadLevel = int(c.shedder.GetLevel())
	}

	// Determine degraded conditions
	status := "ok"
	var reason string

	// Condition 1: DB failing
	if dbStatus == "failing" {
		status = "degraded"
		reason = "database unreachable"
	}

	// Condition 2: Load level 3 for > 60 seconds
	if loadLevel == 3 {
		if c.level3Since.IsZero() {
			c.level3Since = time.Now()
		}
		level3Duration := time.Since(c.level3Since)

		if level3Duration > 60*time.Second {
			status = "degraded"
			if reason == "" {
				reason = "sustained high load (level 3)"
			}
		}
	} else {
		// Reset level 3 timestamp when not in level 3
		c.level3Since = time.Time{}
	}

	// Condition 3: No nodes online after 5 minutes uptime
	if nodesOnline == 0 && uptime > 300 {
		status = "degraded"
		if reason == "" {
			reason = "no nodes connected"
		}
	}

	return Response{
		Status:      status,
		UptimeS:     uptime,
		Version:     version,
		NodesOnline: nodesOnline,
		DB:          dbStatus,
		LoadLevel:   loadLevel,
		Reason:      reason,
	}
}

// checkDB runs a simple query with a 100ms timeout to verify database health.
func (c *Checker) checkDB() string {
	if c.db == nil {
		return "failing"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	var result int
	err := c.db.QueryRowContext(ctx, "SELECT 1").Scan(&result)
	if err != nil {
		return "failing"
	}
	return "ok"
}
