package apdetector

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spaxel/mothership/internal/db"
)

// openTestDB builds a real migrated schema for the detector to write against.
// upsertVirtualNode and emitAPChangeAlert both issue raw INSERTs into
// nodes/events, so the tests exercise the actual migrations rather than a
// hand-rolled subset of the schema.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dataDir := t.TempDir()
	migrator, err := db.NewMigrator(filepath.Join(dataDir, "test.db"), db.Config{DataDir: dataDir})
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	migrator.Register(db.AllMigrations()...)
	if err := migrator.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return migrator.DB()
}

func TestNormalizeBSSID(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"lowercase coloned", "4c:34:88:f4:df:8a", "4C:34:88:F4:DF:8A"},
		{"already normalized", "4C:34:88:F4:DF:8A", "4C:34:88:F4:DF:8A"},
		{"bare hex re-colonized", "4C3488F4DF8A", "4C:34:88:F4:DF:8A"},
		{"dash separated", "4c-34-88-f4-df-8a", "4C:34:88:F4:DF:8A"},
		{"empty stays empty", "", ""},
		{"non-mac passthrough", "not-a-mac", "not-a-mac"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeBSSID(tt.input); got != tt.want {
				t.Errorf("normalizeBSSID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestProcessHelloConsensus covers the agreement math around the 80%
// threshold. Consensus is evaluated incrementally: the very first hello (one
// node = 100% agreement) creates the AP immediately, and the threshold is what
// gates a *change* — a minority BSSID must be reported by at least 80% of all
// reporting nodes before the detected AP flips.
func TestProcessHelloConsensus(t *testing.T) {
	const apA = "4C:34:88:F4:DF:8A" // Intel Corporate
	const apB = "AA:BB:CC:11:22:33"

	type hello struct{ mac, bssid string }

	tests := []struct {
		name      string
		hellos    []hello
		wantAP    string // normalized BSSID GetCurrentAP must end on
		wantAgree float64
		wantTotal int // unique nodes at detection; 0 skips the stat checks
	}{
		{
			name:      "single node unanimous creates AP immediately",
			hellos:    []hello{{"AA:BB:CC:DD:EE:01", apA}},
			wantAP:    apA,
			wantAgree: 1.0,
			wantTotal: 1,
		},
		{
			name: "four of five exactly at threshold flips AP",
			hellos: []hello{
				{"AA:BB:CC:DD:EE:01", apA},
				{"AA:BB:CC:DD:EE:02", apB},
				{"AA:BB:CC:DD:EE:03", apB},
				{"AA:BB:CC:DD:EE:04", apB},
				{"AA:BB:CC:DD:EE:05", apB},
			},
			wantAP:    apB,
			wantAgree: 0.8,
			wantTotal: 5,
		},
		{
			name: "three of five below threshold keeps current AP",
			hellos: []hello{
				{"AA:BB:CC:DD:EE:01", apA},
				{"AA:BB:CC:DD:EE:02", apB},
				{"AA:BB:CC:DD:EE:03", apB},
				{"AA:BB:CC:DD:EE:04", apB},
			},
			wantAP: apA,
		},
		{
			name: "mesh split two of two never flips",
			hellos: []hello{
				{"AA:BB:CC:DD:EE:01", apA},
				{"AA:BB:CC:DD:EE:02", apB},
				{"AA:BB:CC:DD:EE:03", apB},
				{"AA:BB:CC:DD:EE:04", apA},
			},
			wantAP: apA,
		},
		{
			name: "mixed BSSID formatting reports one AP",
			hellos: []hello{
				{"AA:BB:CC:DD:EE:01", "4c:34:88:f4:df:8a"},
				{"AA:BB:CC:DD:EE:02", "4C3488F4DF8A"},
			},
			wantAP:    apA,
			wantAgree: 1.0,
			wantTotal: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			d := NewDetector(db)

			for _, h := range tt.hellos {
				if err := d.ProcessHello(h.mac, h.bssid, 6); err != nil {
					t.Fatalf("ProcessHello(%s, %s): %v", h.mac, h.bssid, err)
				}
			}

			ap := d.GetCurrentAP()
			if ap == nil {
				t.Fatal("GetCurrentAP() = nil, want detected AP")
			}
			if ap.BSSID != tt.wantAP {
				t.Errorf("AP.BSSID = %q, want %q", ap.BSSID, tt.wantAP)
			}
			if tt.wantTotal == 0 {
				return
			}
			if ap.AgreementPct != tt.wantAgree {
				t.Errorf("AP.AgreementPct = %v, want %v", ap.AgreementPct, tt.wantAgree)
			}
			if ap.TotalNodes != tt.wantTotal {
				t.Errorf("AP.TotalNodes = %d, want %d", ap.TotalNodes, tt.wantTotal)
			}

			// The virtual router node must exist in the DB under the
			// normalized BSSID with the ADR-003 node shape.
			var role string
			var virtual, apChannel int
			var nodeType, manufacturer, name string
			err := db.QueryRow(
				`SELECT role, virtual, node_type, ap_channel, manufacturer, name FROM nodes WHERE mac = ?`,
				tt.wantAP,
			).Scan(&role, &virtual, &nodeType, &apChannel, &manufacturer, &name)
			if err != nil {
				t.Fatalf("virtual router node row: %v", err)
			}
			if role != "ap" {
				t.Errorf("role = %q, want %q", role, "ap")
			}
			if virtual != 1 {
				t.Errorf("virtual = %d, want 1", virtual)
			}
			if nodeType != "ap" {
				t.Errorf("node_type = %q, want %q", nodeType, "ap")
			}
			if apChannel != 6 {
				t.Errorf("ap_channel = %d, want 6", apChannel)
			}
			if tt.wantAP == apA {
				if manufacturer != "Intel Corporate" {
					t.Errorf("manufacturer = %q, want %q (OUI 4C:34:88)", manufacturer, "Intel Corporate")
				}
				if name != "Intel Corporate Router" {
					t.Errorf("name = %q, want %q", name, "Intel Corporate Router")
				}
			}
		})
	}
}

// TestProcessHelloCountsUniqueNodes pins the denominator of the agreement
// fraction: repeated hellos from one node must not make a lone reporter look
// like a consensus, and the snapshot taken on an AP flip must count unique
// nodes (bf-4p0ne regression guard).
func TestProcessHelloCountsUniqueNodes(t *testing.T) {
	db := openTestDB(t)
	d := NewDetector(db)

	const apA = "4C:34:88:F4:DF:8A"
	const apB = "AA:BB:CC:11:22:33"

	// Three hellos from one node: still a single-node consensus.
	for i := 0; i < 3; i++ {
		if err := d.ProcessHello("AA:BB:CC:DD:EE:01", apA, 6); err != nil {
			t.Fatalf("ProcessHello repeat %d: %v", i, err)
		}
	}
	ap := d.GetCurrentAP()
	if ap == nil || ap.BSSID != apA {
		t.Fatalf("GetCurrentAP() = %+v, want AP %s", ap, apA)
	}
	if ap.TotalNodes != 1 {
		t.Errorf("TotalNodes = %d, want 1 (unique nodes, not hello count)", ap.TotalNodes)
	}

	// A second node reporting a different BSSID flips the AP; the new
	// snapshot must count the two unique nodes, not the four hellos.
	if err := d.ProcessHello("AA:BB:CC:DD:EE:02", apB, 6); err != nil {
		t.Fatalf("ProcessHello second node: %v", err)
	}
	if err := d.ProcessHello("AA:BB:CC:DD:EE:01", apB, 6); err != nil {
		t.Fatalf("ProcessHello first node re-report: %v", err)
	}

	ap = d.GetCurrentAP()
	if ap == nil || ap.BSSID != apB {
		t.Fatalf("GetCurrentAP() = %+v, want AP %s after flip", ap, apB)
	}
	if ap.TotalNodes != 2 {
		t.Errorf("TotalNodes = %d, want 2 (unique nodes, not hello count)", ap.TotalNodes)
	}
	if ap.ReportCount != 2 {
		t.Errorf("ReportCount = %d, want 2 (unique nodes agreeing)", ap.ReportCount)
	}
	if ap.AgreementPct != 1.0 {
		t.Errorf("AgreementPct = %v, want 1", ap.AgreementPct)
	}
}

// TestProcessHelloEmptyBSSIDIsNoOp: firmware builds without passive-radar
// support (and rx-role nodes) send hello without ap_bssid; they must not
// create reports or a virtual node.
func TestProcessHelloEmptyBSSIDIsNoOp(t *testing.T) {
	db := openTestDB(t)
	d := NewDetector(db)

	if err := d.ProcessHello("AA:BB:CC:DD:EE:01", "", 6); err != nil {
		t.Fatalf("ProcessHello empty BSSID: %v", err)
	}
	if len(d.reports) != 0 {
		t.Errorf("reports = %d entries, want 0", len(d.reports))
	}
	if d.GetCurrentAP() != nil {
		t.Error("GetCurrentAP() != nil, want nil")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM nodes WHERE role = 'ap'`).Scan(&n); err != nil {
		t.Fatalf("count ap nodes: %v", err)
	}
	if n != 0 {
		t.Errorf("nodes with role='ap' = %d, want 0", n)
	}
}

// TestProcessHelloAPChangeEmitsEvent: when the consensus moves to a different
// BSSID the detector records an ap_changed event and re-points the virtual
// node at the new router (ADR-003 router-replacement flow).
func TestProcessHelloAPChangeEmitsEvent(t *testing.T) {
	db := openTestDB(t)
	d := NewDetector(db)

	const oldBSSID = "4C:34:88:F4:DF:8A"
	const newBSSID = "4C:34:88:F4:DF:8B"

	if err := d.ProcessHello("AA:BB:CC:DD:EE:01", oldBSSID, 6); err != nil {
		t.Fatalf("ProcessHello initial AP: %v", err)
	}
	if ap := d.GetCurrentAP(); ap == nil || ap.BSSID != oldBSSID {
		t.Fatalf("GetCurrentAP() = %+v, want AP %s", ap, oldBSSID)
	}

	// Two nodes now report a different BSSID: consensus moves to the new AP.
	if err := d.ProcessHello("AA:BB:CC:DD:EE:02", newBSSID, 11); err != nil {
		t.Fatalf("ProcessHello new AP (node 2): %v", err)
	}
	if err := d.ProcessHello("AA:BB:CC:DD:EE:01", newBSSID, 11); err != nil {
		t.Fatalf("ProcessHello new AP (node 1): %v", err)
	}

	ap := d.GetCurrentAP()
	if ap == nil || ap.BSSID != newBSSID {
		t.Fatalf("GetCurrentAP() = %+v, want AP %s", ap, newBSSID)
	}

	var nodeChannel int
	if err := db.QueryRow(`SELECT ap_channel FROM nodes WHERE mac = ?`, newBSSID).Scan(&nodeChannel); err != nil {
		t.Fatalf("virtual node for new AP: %v", err)
	}
	if nodeChannel != 11 {
		t.Errorf("ap_channel = %d, want 11", nodeChannel)
	}

	var detailJSON string
	var severity, zone, eventType string
	err := db.QueryRow(
		`SELECT type, zone, severity, detail_json FROM events WHERE type = 'ap_changed'`,
	).Scan(&eventType, &zone, &severity, &detailJSON)
	if err != nil {
		t.Fatalf("ap_changed event: %v", err)
	}
	if zone != "system" {
		t.Errorf("event zone = %q, want %q", zone, "system")
	}
	if severity != "warning" {
		t.Errorf("event severity = %q, want %q", severity, "warning")
	}
	for _, want := range []string{`"old_bssid":"` + oldBSSID + `"`, `"new_bssid":"` + newBSSID + `"`} {
		if !strings.Contains(detailJSON, want) {
			t.Errorf("event detail_json = %s, want substring %s", detailJSON, want)
		}
	}
}
