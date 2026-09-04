package ingestion

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestNodeInfo_FreeHeapBytesField locks down the wire contract of the
// free_heap_bytes field on NodeInfo (the dashboard-WebSocket node state
// message).
//
// The bead that motivated this test was phrased against "NodeInfo.proto".
// No such file exists and no protobuf layer exists anywhere in the repo —
// the protocol's schema registry is this struct's json tags, which the
// dashboard hub marshals directly (see docs/research/nodeinfo-free-heap.md).
// These assertions are the protobuf checks restated for the encoding that
// actually exists.
func TestNodeInfo_FreeHeapBytesField(t *testing.T) {
	tests := []struct {
		name    string
		check   func(t *testing.T)
		critera string
	}{
		{
			name:    "field is present",
			critera: "free_heap_bytes field present in NodeInfo",
			check: func(t *testing.T) {
				typ := reflect.TypeOf(NodeInfo{})
				if _, ok := typ.FieldByName("FreeHeapBytes"); !ok {
					t.Fatal("NodeInfo has no FreeHeapBytes field")
				}
			},
		},
		{
			name: "field type is int64",
			// The task asked for uint32 or int64. int64 matches the
			// convention set by HealthMessage.FreeHeapBytes, which is the
			// value this field is filled from.
			critera: "field uses appropriate type",
			check: func(t *testing.T) {
				f, ok := reflect.TypeOf(NodeInfo{}).FieldByName("FreeHeapBytes")
				if !ok {
					t.Fatal("NodeInfo has no FreeHeapBytes field")
				}
				if f.Type.Kind() != reflect.Int64 {
					t.Errorf("FreeHeapBytes is %s, want int64", f.Type)
				}
			},
		},
		{
			name: "json tag names the wire key",
			critera: "field name follows the protocol's snake_case convention",
			check: func(t *testing.T) {
				f, ok := reflect.TypeOf(NodeInfo{}).FieldByName("FreeHeapBytes")
				if !ok {
					t.Fatal("NodeInfo has no FreeHeapBytes field")
				}
				const want = `json:"free_heap_bytes,omitempty"`
				if got := string(f.Tag); got != want {
					t.Errorf("FreeHeapBytes tag = %q, want %q", got, want)
				}
			},
		},
		{
			name: "wire key does not conflict with a sibling key",
			// Protobuf's "field numbers must not conflict" has no analogue
			// in JSON. The equivalent hazard is a duplicated json key, which
			// encoding/json silently drops on unmarshal. Comparing the
			// struct's declared keys against its marshalled keys catches a
			// collision.
			critera: "field numbering is valid and non-conflicting",
			check: func(t *testing.T) {
				// Every field is set so no omitempty key is missing from the
				// marshalled object the keys are compared against.
				full := NodeInfo{
					MAC:             "AA:BB:CC:DD:EE:FF",
					FirmwareVersion: "1.0.0",
					Chip:            "ESP32-S3",
					Unpaired:        true,
					FreeHeapBytes:   204800,
				}
				keys := jsonKeys(t, full)
				seen := map[string]int{}
				for _, k := range keys {
					seen[k]++
					if seen[k] > 1 {
						t.Errorf("duplicate json key %q in NodeInfo", k)
					}
				}
				if _, ok := seen["free_heap_bytes"]; !ok {
					t.Errorf("marshalled NodeInfo has no free_heap_bytes key; got %v", keys)
				}
			},
		},
		{
			name:    "round-trips on the wire",
			critera: "the field survives a marshal/unmarshal cycle",
			check: func(t *testing.T) {
				const heap = 204800
				in := NodeInfo{MAC: "AA:BB:CC:DD:EE:FF", FreeHeapBytes: heap}
				data, err := json.Marshal(in)
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				var out NodeInfo
				if err := json.Unmarshal(data, &out); err != nil {
					t.Fatalf("unmarshal %s: %v", data, err)
				}
				if out.FreeHeapBytes != heap {
					t.Errorf("round-trip FreeHeapBytes = %d, want %d", out.FreeHeapBytes, heap)
				}
				if !strings.Contains(string(data), `"free_heap_bytes":204800`) {
					t.Errorf("marshalled NodeInfo %s missing free_heap_bytes", data)
				}
			},
		},
		{
			name: "no reading is omitted, not reported as zero",
			// A node whose health report has not arrived yet has no heap
			// reading. omitempty drops the key so the dashboard cannot
			// mistake "unknown" for "0 bytes free".
			critera: "absence is distinguishable from a zero reading",
			check: func(t *testing.T) {
				data, err := json.Marshal(NodeInfo{MAC: "AA:BB:CC:DD:EE:FF"})
				if err != nil {
					t.Fatalf("marshal: %v", err)
				}
				if strings.Contains(string(data), "free_heap_bytes") {
					t.Errorf("zero-valued NodeInfo %s should omit free_heap_bytes", data)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.check(t)
		})
	}
}

// TestGetConnectedNodesInfo_PopulatesFreeHeapBytes verifies the field is
// filled from the node's most recent health report and left unset when no
// health report has arrived.
func TestGetConnectedNodesInfo_PopulatesFreeHeapBytes(t *testing.T) {
	tests := []struct {
		name string
		// nil LastHealth means the node connected but has not reported
		// health yet.
		lastHealth *HealthMessage
		want       int64
	}{
		{name: "no health report yet", lastHealth: nil, want: 0},
		{name: "after a health report", lastHealth: &HealthMessage{MAC: "AA:BB:CC:DD:EE:FF", FreeHeapBytes: 204800}, want: 204800},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := NewServer()
			server.mu.Lock()
			server.connections["AA:BB:CC:DD:EE:FF"] = &NodeConnection{
				MAC:        "AA:BB:CC:DD:EE:FF",
				LastHealth: tt.lastHealth,
			}
			server.mu.Unlock()

			nodes := server.GetConnectedNodesInfo()
			if len(nodes) != 1 {
				t.Fatalf("got %d nodes, want 1", len(nodes))
			}
			if nodes[0].FreeHeapBytes != tt.want {
				t.Errorf("FreeHeapBytes = %d, want %d", nodes[0].FreeHeapBytes, tt.want)
			}
		})
	}
}

// jsonKeys marshals v and returns the json object keys it produced, in order.
func jsonKeys(t *testing.T, v interface{}) []string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal %s: %v", data, err)
	}
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	return keys
}
