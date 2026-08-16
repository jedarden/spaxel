//go:build io6_gate

// io6_gate_conclusion_test.go locks in the data-driven conclusion of
// CaptureIO6Diagnostics (bf-48juo). Zero blobs may enter bf-4q5w fusion triage
// ONLY when node geometry reached the DB distinctly (>=1 node, not all
// collapsed to the (0,0,1) schema default); an empty /api/nodes or all-at-origin
// admission must instead be attributed to an auth/provision failure and must
// not be misattributed to fusion. bf-5k1z found the old unconditional
// conclusion doing exactly that.
//
// The harness is driven against an httptest server returning controlled
// /api/nodes + /api/status payloads, so this needs no real mothership or
// simulator — it is a fast, deterministic check of the branching logic.
package e2e

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// statusAndNodesServer returns an httptest.Server serving /api/status and
// /api/nodes from the provided payloads.
func statusAndNodesServer(t *testing.T, status any, nodes any) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(status)
	})
	mux.HandleFunc("/api/nodes", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(nodes)
	})
	return httptest.NewServer(mux)
}

func TestCaptureIO6Diagnostics_ConclusionIsDataDriven(t *testing.T) {
	// /api/status is constant across cases: zero blobs is the RED state whose
	// root cause the conclusion must disambiguate from the node-admission data.
	status := StatusResponse{Nodes: 0, Blobs: 0, UptimeS: 5, DetectionQuality: 0}

	cases := []struct {
		name       string
		nodes      []NodeRecord
		wantAuth   bool // true => expect the auth/provision-failure conclusion
		wantWiring bool // true => expect the bf-4q5w wiring-gap conclusion
	}{
		{
			name:       "zero nodes admitted -> auth/provision failure, not bf-4q5w",
			nodes:      []NodeRecord{},
			wantAuth:   true,
			wantWiring: false,
		},
		{
			name: "all nodes collapsed to schema origin -> auth/provision failure",
			nodes: []NodeRecord{
				{MAC: "AA:00:00:00:00:01", Role: "tx_rx", PosX: 0, PosY: 0, PosZ: 1},
				{MAC: "AA:00:00:00:00:02", Role: "tx_rx", PosX: 0, PosY: 0, PosZ: 1},
				{MAC: "AA:00:00:00:00:03", Role: "tx_rx", PosX: 0, PosY: 0, PosZ: 1},
			},
			wantAuth:   true,
			wantWiring: false,
		},
		{
			name: "distinct corner geometry -> fusion triage",
			nodes: []NodeRecord{
				{MAC: "AA:00:00:00:00:01", Role: "tx_rx", PosX: 0.5, PosY: 0.5, PosZ: 2.0},
				{MAC: "AA:00:00:00:00:02", Role: "tx_rx", PosX: 5.5, PosY: 0.5, PosZ: 2.0},
				{MAC: "AA:00:00:00:00:03", Role: "tx_rx", PosX: 0.5, PosY: 4.5, PosZ: 0.3},
				{MAC: "AA:00:00:00:00:04", Role: "tx_rx", PosX: 5.5, PosY: 4.5, PosZ: 0.3},
			},
			wantAuth:   false,
			wantWiring: true,
		},
		{
			// atOrigin(1) < len(4) still counts as distinct geometry -> fusion triage,
			// not auth failure (one straggler at the schema default is tolerated).
			name:       "mostly distinct, one node at origin -> still fusion triage",
			wantWiring: true,
			nodes: []NodeRecord{
				{MAC: "AA:00:00:00:00:01", Role: "tx_rx", PosX: 0.5, PosY: 0.5, PosZ: 2.0},
				{MAC: "AA:00:00:00:00:02", Role: "tx_rx", PosX: 5.5, PosY: 4.5, PosZ: 0.3},
				{MAC: "AA:00:00:00:00:03", Role: "tx_rx", PosX: 0, PosY: 0, PosZ: 1},
				{MAC: "AA:00:00:00:00:04", Role: "tx_rx", PosX: 3.0, PosY: 2.5, PosZ: 1.5},
			},
		},
	}

	ctx := context.Background()
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			srv := statusAndNodesServer(t, status, tc.nodes)
			defer srv.Close()

			h := NewTestHarness(t)
			h.APIURL = srv.URL

			got := h.CaptureIO6Diagnostics(ctx, 0, 0)

			// Phrases that identify each conclusion family. The two are mutually
			// exclusive in every case, so asserting both directions catches any
			// future regression that re-conflates them.
			const (
				authPhrase   = "auth/provision failure"
				wiringPhrase = "fusion accumulation grid"
			)
			hasAuth := strings.Contains(got, authPhrase)
			hasWiring := strings.Contains(got, wiringPhrase)

			if hasAuth != tc.wantAuth {
				t.Errorf("conclusion auth-attribution: hasAuth=%v, want %v\n--- diagnostics ---\n%s",
					hasAuth, tc.wantAuth, got)
			}
			if hasWiring != tc.wantWiring {
				t.Errorf("conclusion wiring-attribution: hasWiring=%v, want %v\n--- diagnostics ---\n%s",
					hasWiring, tc.wantWiring, got)
			}
		})
	}
}
