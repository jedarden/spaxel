// Package ota provides tests for auto-update functionality.
package ota

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// scrapeAutoUpdateTriggerCounter GETs url — the same promhttp.Handler() that
// main.go mounts at /metrics — and returns the auto_update_triggers_total
// samples keyed by their label pairs, plus the response's content type.
func scrapeAutoUpdateTriggerCounter(t *testing.T, url string) (map[string]float64, string) {
	t.Helper()

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("scrape %s: %v", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("scrape %s: status = %d, want 200", url, resp.StatusCode)
	}

	sampleRe := regexp.MustCompile(`auto_update_triggers_total\{([^}]*)\}\s+([0-9.eE+-]+)`)

	samples := map[string]float64{}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read scrape body: %v", err)
	}
	body := string(raw)
	for _, m := range sampleRe.FindAllStringSubmatch(body, -1) {
		value, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			t.Fatalf("parse auto_update_triggers_total value %q: %v", m[2], err)
		}
		samples[m[1]] = value
	}
	return samples, resp.Header.Get("Content-Type")
}

// TestAutoUpdateTriggerCounterExposedForScraping verifies the metric a cycle
// writes is actually reachable by a scraper: the endpoint returns 200 in the
// Prometheus text exposition format, both label combinations are exported
// (also at zero, before any cycle has run), and a cycle that resolves drives
// the counter value a subsequent scrape reads back.
func TestAutoUpdateTriggerCounterExposedForScraping(t *testing.T) {
	ts := httptest.NewServer(promhttp.Handler())
	t.Cleanup(ts.Close)

	scrape := func(t *testing.T) map[string]float64 {
		t.Helper()
		samples, contentType := scrapeAutoUpdateTriggerCounter(t, ts.URL)
		if got := contentType; len(got) < 10 || got[:10] != "text/plain" {
			t.Errorf("content type = %q, want the Prometheus text exposition format", got)
		}
		return samples
	}

	// Label the two exported series carry, sorted for a stable error message.
	labels := func(samples map[string]float64) []string {
		keys := make([]string, 0, len(samples))
		for k := range samples {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys
	}

	// The series is exported before any cycle has resolved in this process.
	// A CounterVec with no children emits nothing, so this only holds because
	// both label combinations are pre-created at zero — the value is
	// package-global and other tests may already have moved it.
	first := scrape(t)
	want := []string{`result="failure",trigger_type="auto"`, `result="success",trigger_type="auto"`}
	if got := labels(first); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("exported label pairs = %v, want exactly %v", got, want)
	}

	srv := &Server{}
	autoMgr := NewAutoUpdateManager(srv, NewManager(srv, "http://localhost:8080"), time.UTC)
	firmware := &FirmwareMeta{
		Filename:  "spaxel-firmware-0.1.350.bin",
		Version:   "0.1.350",
		SHA256:    "abc123",
		SizeBytes: 1024,
	}

	// A failing cycle: no node provider, so selectCanaryNode finds nothing
	// and the cycle resolves as failure right after the trigger fired.
	failureBefore := triggerCounterValue(t, "failure")
	autoMgr.startUpdateCycle(context.Background(), firmware)

	// A completing cycle: the canary is the only connected node, so
	// fleetRollout has nothing left to deploy and finishes straight away.
	clearQuietWindow(t, autoMgr)
	nodes := newMockNodeProvider()
	nodes.addNodeWithFirmware("AA:BB:CC:DD:EE:01", "tx_rx", 0.9, "0.1.349")
	autoMgr.SetNodeProvider(nodes)
	autoMgr.mu.Lock()
	autoMgr.currentCanaryNode = "AA:BB:CC:DD:EE:01"
	autoMgr.updateState = StateFleetDeploy
	autoMgr.mu.Unlock()

	successBefore := triggerCounterValue(t, "success")
	autoMgr.wg.Add(1) // fleetRollout owns a matching wg.Done
	go autoMgr.fleetRollout(context.Background(), firmware)
	autoMgr.wg.Wait()

	after := scrape(t)
	if got := after[want[0]] - failureBefore; got != 1 {
		t.Errorf("scraped auto/failure delta = %v, want 1 (sample %v)", got, after[want[0]])
	}
	if got := after[want[1]] - successBefore; got != 1 {
		t.Errorf("scraped auto/success delta = %v, want 1 (sample %v)", got, after[want[1]])
	}
}
