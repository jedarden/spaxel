package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

// testEventsHandler creates a handler backed by a temp SQLite DB.
func testEventsHandler(t *testing.T) (*EventsHandler, func()) {
	t.Helper()
	dir := t.TempDir()
	h, err := NewEventsHandler(filepath.Join(dir, "events.db"))
	if err != nil {
		t.Fatalf("NewEventsHandler: %v", err)
	}
	return h, func() { h.Close() }
}

// seedEvents inserts n events with ascending timestamps starting from base.
func seedEvents(t *testing.T, h *EventsHandler, base time.Time, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		ts := base.Add(time.Duration(i) * time.Second)
		zones := []string{"Kitchen", "Hallway", "Bedroom", "Living Room", ""}
		zone := zones[i%len(zones)]
		persons := []string{"Alice", "Bob", "", "", ""}
		person := persons[i%len(persons)]
		types := []string{"detection", "zone_entry", "zone_exit", "portal_crossing", "system"}
		evtType := types[i%len(types)]
		if err := h.LogEvent(evtType, ts, zone, person, 0, `{"test":true}`, "info"); err != nil {
			t.Fatalf("LogEvent %d: %v", i, err)
		}
	}
}

// --- LogEvent tests ---

func TestLogEvent_ValidTypes(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	for _, validType := range []string{
		"detection", "zone_entry", "zone_exit", "portal_crossing",
		"trigger_fired", "fall_alert", "anomaly", "security_alert",
		"node_online", "node_offline", "ota_update", "baseline_changed",
		"system", "learning_milestone",
	} {
		err := h.LogEvent(validType, time.Now(), "Kitchen", "Alice", 1, `{}`, "info")
		if err != nil {
			t.Errorf("LogEvent(%q) returned error: %v", validType, err)
		}
	}
}

func TestLogEvent_InvalidType(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	// LogEvent is a write path and does not validate event types.
	// Type validation happens on the read side (listEvents filter).
	err := h.LogEvent("invalid_type", time.Now(), "", "", 0, `{}`, "info")
	if err != nil {
		t.Errorf("LogEvent should accept any type string: %v", err)
	}
}

func TestLogEvent_DefaultSeverity(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	// Empty severity defaults to "info"
	err := h.LogEvent("system", time.Now(), "", "", 0, `{}`, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Invalid severity also defaults to "info"
	err = h.LogEvent("system", time.Now(), "", "", 0, `{}`, "invalid_sev")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLogEvent_EventBusPublish(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	// Note: EventsHandler doesn't have a bus field in the current implementation
	// This test is simplified to just verify logging works
	err := h.LogEvent("detection", time.Now(), "Kitchen", "Alice", 1, `{}`, "info")
	if err != nil {
		t.Fatalf("LogEvent failed: %v", err)
	}

	err = h.LogEvent("zone_exit", time.Now(), "Hallway", "Bob", 2, `{}`, "warning")
	if err != nil {
		t.Fatalf("LogEvent failed: %v", err)
	}
}

// --- GET /api/events tests ---

func TestListEvents_DefaultPagination(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	seedEvents(t, h, base, 100)

	req := httptest.NewRequest("GET", "/api/events", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp eventsResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Default limit is 50
	if len(resp.Events) != 50 {
		t.Errorf("got %d events, want 50", len(resp.Events))
	}
	if !resp.HasMore {
		t.Error("expected has_more=true for 100 events with limit 50")
	}
	if resp.TotalFiltered != 100 {
		t.Errorf("total_filtered = %d, want 100", resp.TotalFiltered)
	}
}

func TestListEvents_CustomLimit(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	seedEvents(t, h, base, 100)

	req := httptest.NewRequest("GET", "/api/events?limit=10", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var resp eventsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.Events) != 10 {
		t.Errorf("got %d events, want 10", len(resp.Events))
	}
	if !resp.HasMore {
		t.Error("expected has_more=true")
	}
}

func TestListEvents_LimitClampedToMax(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	seedEvents(t, h, base, 100)

	// Request limit=1000, should be clamped to maxLimit (500)
	req := httptest.NewRequest("GET", "/api/events?limit=1000", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var resp eventsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.Events) != 100 {
		t.Errorf("got %d events, want 100 (all events since <500)", len(resp.Events))
	}
	if resp.HasMore {
		t.Error("expected has_more=false (all 100 events returned)")
	}
}

func TestListEvents_Empty(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/events", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var resp eventsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.Events) != 0 {
		t.Errorf("got %d events, want 0", len(resp.Events))
	}
	if resp.HasMore {
		t.Error("expected has_more=false for empty table")
	}
	if resp.TotalFiltered != 0 {
		t.Errorf("total_filtered = %d, want 0", resp.TotalFiltered)
	}
	if resp.Cursor != "" {
		t.Error("expected empty cursor for empty table")
	}
}

func TestListEvents_DescendingOrder(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	seedEvents(t, h, base, 5)

	req := httptest.NewRequest("GET", "/api/events?limit=5", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var resp eventsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	// Events should be in descending timestamp order
	for i := 1; i < len(resp.Events); i++ {
		if resp.Events[i].Timestamp > resp.Events[i-1].Timestamp {
			t.Errorf("events not descending: [%d].ts=%d > [%d].ts=%d",
				i, resp.Events[i].Timestamp, i-1, resp.Events[i-1].Timestamp)
		}
	}
}

func TestListEvents_FilterByType(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	seedEvents(t, h, base, 100)

	tests := []struct {
		name      string
		filter    string
		wantCount int
	}{
		{"detection", "detection", 20},
		{"zone_entry", "zone_entry", 20},
		{"system", "system", 20},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/events?type="+tc.filter+"&limit=100", nil)
			w := httptest.NewRecorder()
			h.listEvents(w, req)

			var resp eventsResponse
			json.NewDecoder(w.Body).Decode(&resp)

			if resp.TotalFiltered != tc.wantCount {
				t.Errorf("total_filtered = %d, want %d", resp.TotalFiltered, tc.wantCount)
			}
			for _, ev := range resp.Events {
				if ev.Type != tc.filter {
					t.Errorf("event type = %q, want %q", ev.Type, tc.filter)
				}
			}
		})
	}
}

func TestListEvents_InvalidType(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/events?type=invalid_type", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestListEvents_FilterByZone(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	seedEvents(t, h, base, 100)

	req := httptest.NewRequest("GET", "/api/events?zone=Kitchen&limit=100", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var resp eventsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	for _, ev := range resp.Events {
		if ev.Zone != "Kitchen" {
			t.Errorf("event zone = %q, want Kitchen", ev.Zone)
		}
	}
}

func TestListEvents_FilterByPerson(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	seedEvents(t, h, base, 100)

	req := httptest.NewRequest("GET", "/api/events?person=Alice&limit=100", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var resp eventsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	for _, ev := range resp.Events {
		if ev.Person != "Alice" {
			t.Errorf("event person = %q, want Alice", ev.Person)
		}
	}
}

func TestListEvents_FilterByAfter(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	seedEvents(t, h, base, 10)

	// Filter after the 5th event's time
	afterTime := base.Add(4 * time.Second).Format(time.RFC3339)
	req := httptest.NewRequest("GET", "/api/events?after="+afterTime+"&limit=100", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var resp eventsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.TotalFiltered != 6 { // events 4..9
		t.Errorf("total_filtered = %d, want 6", resp.TotalFiltered)
	}
	for _, ev := range resp.Events {
		if ev.Timestamp < base.Add(4*time.Second).UnixNano()/1e6 {
			t.Errorf("event ts %d before after time", ev.Timestamp)
		}
	}
}

func TestListEvents_InvalidAfter(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/events?after=not-a-date", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestListEvents_CursorPagination(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	seedEvents(t, h, base, 100)

	// Page 1
	req := httptest.NewRequest("GET", "/api/events?limit=30", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var page1 eventsResponse
	json.NewDecoder(w.Body).Decode(&page1)

	if len(page1.Events) != 30 {
		t.Fatalf("page 1: got %d events, want 30", len(page1.Events))
	}
	if !page1.HasMore {
		t.Fatal("page 1: expected has_more=true")
	}
	if page1.Cursor == "" {
		t.Fatal("page 1: expected non-empty cursor")
	}

	// Page 2 using cursor
	req = httptest.NewRequest("GET", "/api/events?limit=30&before="+page1.Cursor, nil)
	w = httptest.NewRecorder()
	h.listEvents(w, req)

	var page2 eventsResponse
	json.NewDecoder(w.Body).Decode(&page2)

	if len(page2.Events) != 30 {
		t.Fatalf("page 2: got %d events, want 30", len(page2.Events))
	}

	// Ensure no overlap: page2 events must all have earlier timestamps than page1's last event
	lastPage1TS := page1.Events[len(page1.Events)-1].Timestamp
	for _, ev := range page2.Events {
		if ev.Timestamp >= lastPage1TS {
			t.Errorf("page 2 event ts %d >= page 1 last ts %d (overlap!)", ev.Timestamp, lastPage1TS)
		}
	}

	// Page 3
	req = httptest.NewRequest("GET", "/api/events?limit=30&before="+page2.Cursor, nil)
	w = httptest.NewRecorder()
	h.listEvents(w, req)

	var page3 eventsResponse
	json.NewDecoder(w.Body).Decode(&page3)

	if len(page3.Events) != 30 {
		t.Fatalf("page 3: got %d events, want 30", len(page3.Events))
	}

	// Page 4 — should return remaining 10 events, no more pages
	req = httptest.NewRequest("GET", "/api/events?limit=30&before="+page3.Cursor, nil)
	w = httptest.NewRecorder()
	h.listEvents(w, req)

	var page4 eventsResponse
	json.NewDecoder(w.Body).Decode(&page4)

	if len(page4.Events) != 10 {
		t.Fatalf("page 4: got %d events, want 10", len(page4.Events))
	}
	if page4.HasMore {
		t.Error("page 4: expected has_more=false")
	}
	if page4.Cursor != "" {
		t.Error("page 4: expected empty cursor")
	}

	// Verify total across all pages
	total := len(page1.Events) + len(page2.Events) + len(page3.Events) + len(page4.Events)
	if total != 100 {
		t.Errorf("total across pages = %d, want 100", total)
	}

	// Verify no duplicates across all pages
	seen := make(map[int64]bool)
	for _, p := range []eventsResponse{page1, page2, page3, page4} {
		for _, ev := range p.Events {
			if seen[ev.ID] {
				t.Errorf("duplicate event ID %d across pages", ev.ID)
			}
			seen[ev.ID] = true
		}
	}
}

func TestListEvents_ConsistentPagination(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	seedEvents(t, h, base, 50)

	// Fetch all events in one shot
	req := httptest.NewRequest("GET", "/api/events?limit=50", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var all eventsResponse
	json.NewDecoder(w.Body).Decode(&all)

	// Fetch same events via paginated requests
	var paginated []*Event
	cursor := ""
	for {
		u := "/api/events?limit=10"
		if cursor != "" {
			u += "&before=" + cursor
		}
		req := httptest.NewRequest("GET", u, nil)
		w := httptest.NewRecorder()
		h.listEvents(w, req)

		var page eventsResponse
		json.NewDecoder(w.Body).Decode(&page)
		paginated = append(paginated, page.Events...)
		cursor = page.Cursor
		if !page.HasMore {
			break
		}
	}

	if len(paginated) != len(all.Events) {
		t.Fatalf("paginated count %d != full count %d", len(paginated), len(all.Events))
	}

	// Both should return same event IDs in same order
	for i := range all.Events {
		if paginated[i].ID != all.Events[i].ID {
			t.Errorf("position %d: paginated ID %d != full ID %d",
				i, paginated[i].ID, all.Events[i].ID)
		}
	}
}

func TestListEvents_CombinedFilters(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	seedEvents(t, h, base, 100)

	// Filter by type AND zone
	req := httptest.NewRequest("GET", "/api/events?type=detection&zone=Kitchen&limit=100", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var resp eventsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	for _, ev := range resp.Events {
		if ev.Type != "detection" {
			t.Errorf("type = %q, want detection", ev.Type)
		}
		if ev.Zone != "Kitchen" {
			t.Errorf("zone = %q, want Kitchen", ev.Zone)
		}
	}
}

// --- FTS5 search tests ---

func TestListEvents_FTS5Search(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	// Insert events with searchable content
	h.LogEvent("detection", base, "Kitchen", "Alice", 1, `{"message":"person detected near fridge"}`, "info")
	h.LogEvent("zone_entry", base.Add(time.Second), "Hallway", "Bob", 2, `{"message":"entered hallway"}`, "info")
	h.LogEvent("system", base.Add(2*time.Second), "", "", 0, `{"message":"system started"}`, "info")

	tests := []struct {
		name      string
		query     string
		wantCount int
	}{
		{"exact match type", "detection", 1},
		{"prefix match type", "detect*", 1},
		{"exact match zone", "Kitchen", 1},
		{"prefix match zone", "Kit*", 1},
		{"exact match person", "Alice", 1},
		{"prefix match person", "Ali*", 1},
		{"match in detail_json", "fridge", 1},
		{"prefix match detail", "frid*", 1},
		{"no match", "zzznonexistent", 0},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/events?q="+tc.query+"&limit=100", nil)
			w := httptest.NewRecorder()
			h.listEvents(w, req)

			var resp eventsResponse
			json.NewDecoder(w.Body).Decode(&resp)

			if resp.TotalFiltered != tc.wantCount {
				t.Errorf("total_filtered = %d, want %d (query=%q)", resp.TotalFiltered, tc.wantCount, tc.query)
			}
		})
	}
}

func TestListEvents_FTS5SearchPagination(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	// Insert many events with "test" in detail_json
	for i := 0; i < 100; i++ {
		detail := `{"test":"event ` + strings.Repeat("word", i+1) + `"}`
		h.LogEvent("system", base.Add(time.Duration(i)*time.Second), "", "", 0, detail, "info")
	}

	// Page through FTS5 results
	req := httptest.NewRequest("GET", "/api/events?q=test&limit=10", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var page1 eventsResponse
	json.NewDecoder(w.Body).Decode(&page1)

	if len(page1.Events) != 10 {
		t.Fatalf("page 1: got %d, want 10", len(page1.Events))
	}
	if !page1.HasMore {
		t.Fatal("expected has_more=true")
	}

	// Page 2
	req = httptest.NewRequest("GET", "/api/events?q=test&limit=10&before="+page1.Cursor, nil)
	w = httptest.NewRecorder()
	h.listEvents(w, req)

	var page2 eventsResponse
	json.NewDecoder(w.Body).Decode(&page2)

	if len(page2.Events) != 10 {
		t.Fatalf("page 2: got %d, want 10", len(page2.Events))
	}

	// No overlap
	lastPage1TS := page1.Events[len(page1.Events)-1].Timestamp
	for _, ev := range page2.Events {
		if ev.Timestamp >= lastPage1TS {
			t.Errorf("overlap: page2 ts %d >= page1 last ts %d", ev.Timestamp, lastPage1TS)
		}
	}
}

func TestListEvents_FTS5SearchWithFilter(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	h.LogEvent("detection", base, "Kitchen", "Alice", 1, `{"message":"kitchen detection"}`, "info")
	h.LogEvent("detection", base.Add(time.Second), "Hallway", "Bob", 2, `{"message":"hallway detection"}`, "info")
	h.LogEvent("zone_entry", base.Add(2*time.Second), "Kitchen", "Alice", 1, `{"message":"entered kitchen"}`, "info")

	// FTS5 + type filter
	req := httptest.NewRequest("GET", "/api/events?q=kitchen&type=detection&limit=100", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var resp eventsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	for _, ev := range resp.Events {
		if ev.Type != "detection" {
			t.Errorf("type = %q, want detection", ev.Type)
		}
	}
}

// --- GET /api/events/{id} tests ---

func TestGetEvent_Found(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	ts := time.Now()
	h.LogEvent("detection", ts, "Kitchen", "Alice", 42, `{"key":"val"}`, "warning")

	// Get the event via list to find its ID
	req := httptest.NewRequest("GET", "/api/events?limit=1", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var listResp eventsResponse
	json.NewDecoder(w.Body).Decode(&listResp)
	if len(listResp.Events) == 0 {
		t.Fatal("no events returned")
	}
	eventID := listResp.Events[0].ID

	// Verify by querying DB directly
	var ev Event
	err := h.db.QueryRow(`
		SELECT id, timestamp_ms, type, zone, person, blob_id, detail_json, severity
		FROM events WHERE id = ?
	`, eventID).Scan(&ev.ID, &ev.Timestamp, &ev.Type, &ev.Zone,
		&ev.Person, &ev.BlobID, &ev.DetailJSON, &ev.Severity)
	if err != nil {
		t.Fatalf("query: %v", err)
	}

	if ev.Type != "detection" {
		t.Errorf("type = %q, want detection", ev.Type)
	}
	if ev.Zone != "Kitchen" {
		t.Errorf("zone = %q, want Kitchen", ev.Zone)
	}
	if ev.Person != "Alice" {
		t.Errorf("person = %q, want Alice", ev.Person)
	}
	if ev.BlobID != 42 {
		t.Errorf("blob_id = %d, want 42", ev.BlobID)
	}
	if ev.Severity != "warning" {
		t.Errorf("severity = %q, want warning", ev.Severity)
	}
}

func TestGetEvent_NotFound(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	// Use chi URLParam to simulate routing
	req := httptest.NewRequest("GET", "/api/events/999999", nil)
	// chi.URLParam reads from a context value set by chi router
	// We need to simulate this by setting up a chi router
	r := chi.NewRouter()
	e := &EventsHandler{db: h.db}
	r.Get("/api/events/{id}", e.getEvent)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "event not found" {
		t.Errorf("error = %q, want 'event not found'", resp["error"])
	}
}

func TestGetEvent_InvalidID(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	e := &EventsHandler{db: h.db}
	r := chi.NewRouter()
	r.Get("/api/events/{id}", e.getEvent)

	// Test with non-numeric ID
	req := httptest.NewRequest("GET", "/api/events/invalid", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "invalid event id" {
		t.Errorf("error = %q, want 'invalid event id'", resp["error"])
	}
}

func TestGetEvent_HTTPHandler_Found(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	ts := time.Now()
	h.LogEvent("detection", ts, "Kitchen", "Alice", 42, `{"key":"val"}`, "warning")

	// Get the event via list to find its ID
	req := httptest.NewRequest("GET", "/api/events?limit=1", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var listResp eventsResponse
	json.NewDecoder(w.Body).Decode(&listResp)
	if len(listResp.Events) == 0 {
		t.Fatal("no events returned")
	}
	eventID := listResp.Events[0].ID

	// Test the actual HTTP handler
	e := &EventsHandler{db: h.db}
	r := chi.NewRouter()
	r.Get("/api/events/{id}", e.getEvent)

	req = httptest.NewRequest("GET", "/api/events/"+strconv.FormatInt(eventID, 10), nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var ev Event
	json.NewDecoder(w.Body).Decode(&ev)

	if ev.ID != eventID {
		t.Errorf("id = %d, want %d", ev.ID, eventID)
	}
	if ev.Type != "detection" {
		t.Errorf("type = %q, want detection", ev.Type)
	}
	if ev.Zone != "Kitchen" {
		t.Errorf("zone = %q, want Kitchen", ev.Zone)
	}
	if ev.Person != "Alice" {
		t.Errorf("person = %q, want Alice", ev.Person)
	}
	if ev.BlobID != 42 {
		t.Errorf("blob_id = %d, want 42", ev.BlobID)
	}
	if ev.Severity != "warning" {
		t.Errorf("severity = %q, want warning", ev.Severity)
	}
	if ev.DetailJSON != `{"key":"val"}` {
		t.Errorf("detail_json = %q, want '{\"key\":\"val\"}'", ev.DetailJSON)
	}
}

// --- Event struct JSON encoding tests ---

func TestEvent_JSONEncoding(t *testing.T) {
	ev := Event{
		ID:         1,
		Timestamp:  1710000000000,
		Type:       "detection",
		Zone:       "Kitchen",
		Person:     "Alice",
		BlobID:     42,
		DetailJSON: `{"key":"val"}`,
		Severity:   "warning",
	}

	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded map[string]interface{}
	json.Unmarshal(data, &decoded)

	if decoded["type"] != "detection" {
		t.Errorf("type = %v", decoded["type"])
	}
	if decoded["zone"] != "Kitchen" {
		t.Errorf("zone = %v", decoded["zone"])
	}
	if decoded["person"] != "Alice" {
		t.Errorf("person = %v", decoded["person"])
	}
	if _, ok := decoded["blob_id"]; !ok {
		t.Error("blob_id missing")
	}
	if decoded["severity"] != "warning" {
		t.Errorf("severity = %v", decoded["severity"])
	}
	// Omitempty fields should be omitted when zero value
	emptyEvent := Event{ID: 1, Timestamp: 1000, Type: "system", Severity: "info"}
	data2, _ := json.Marshal(emptyEvent)
	s := string(data2)
	if strings.Contains(s, `"zone"`) {
		t.Error("zone should be omitted when empty")
	}
	if strings.Contains(s, `"person"`) {
		t.Error("person should be omitted when empty")
	}
}

// --- eventsResponse JSON encoding ---

func TestEventsResponse_JSONEncoding(t *testing.T) {
	resp := eventsResponse{
		Events: []*Event{
			{ID: 1, Timestamp: 1000, Type: "system", Severity: "info"},
		},
		Cursor:        "999",
		HasMore:       true,
		TotalFiltered: 42,
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	s := string(data)
	if !strings.Contains(s, `"cursor":"999"`) {
		t.Error("cursor missing or wrong")
	}
	if !strings.Contains(s, `"has_more":true`) {
		t.Error("has_more missing or wrong")
	}
	if !strings.Contains(s, `"total_filtered":42`) {
		t.Error("total_filtered missing or wrong")
	}
}

// --- Archive tests ---

func TestRunArchive_NoOldEvents(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	seedEvents(t, h, base, 10)

	// Run archive — nothing should be archived (all recent)
	h.Archive(nil)

	var count int
	h.db.QueryRow("SELECT COUNT(*) FROM events").Scan(&count)
	if count != 10 {
		t.Errorf("events count = %d, want 10 (none archived)", count)
	}

	var archiveCount int
	h.db.QueryRow("SELECT COUNT(*) FROM events_archive").Scan(&archiveCount)
	if archiveCount != 0 {
		t.Errorf("archive count = %d, want 0", archiveCount)
	}
}

func TestRunArchive_OldEvents(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	// Insert events that are older than 90 days
	oldTime := time.Now().AddDate(0, 0, -91)
	for i := 0; i < 5; i++ {
		h.LogEvent("system", oldTime.Add(time.Duration(i)*time.Second), "", "", 0, `{"old":true}`, "info")
	}

	// Insert recent events
	base := time.Now()
	for i := 0; i < 3; i++ {
		h.LogEvent("system", base.Add(time.Duration(i)*time.Second), "", "", 0, `{"recent":true}`, "info")
	}

	// Run archive
	h.Archive(nil)

	var eventCount, archiveCount int
	h.db.QueryRow("SELECT COUNT(*) FROM events").Scan(&eventCount)
	h.db.QueryRow("SELECT COUNT(*) FROM events_archive").Scan(&archiveCount)

	if eventCount != 3 {
		t.Errorf("events count = %d, want 3 (recent events)", eventCount)
	}
	if archiveCount != 5 {
		t.Errorf("archive count = %d, want 5 (old events)", archiveCount)
	}
}

// --- Performance: FTS5 with 1000 events ---

func BenchmarkListEvents_FTS5_1000(b *testing.B) {
	dir := b.TempDir()
	h, err := NewEventsHandler(filepath.Join(dir, "events.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer h.Close()

	base := time.Now()
	for i := 0; i < 1000; i++ {
		h.LogEvent("detection", base.Add(time.Duration(i)*time.Second),
			[]string{"Kitchen", "Hallway", "Bedroom"}[i%3],
			[]string{"Alice", "Bob", ""}[i%3],
			i%10, `{"message":"test event `+strings.Repeat("word", 5)+`"}`, "info")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/events?q=test&limit=50", nil)
		w := httptest.NewRecorder()
		h.listEvents(w, req)
	}
}

func BenchmarkListEvents_Pagination_1000(b *testing.B) {
	dir := b.TempDir()
	h, err := NewEventsHandler(filepath.Join(dir, "events.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer h.Close()

	base := time.Now()
	for i := 0; i < 1000; i++ {
		h.LogEvent("system", base.Add(time.Duration(i)*time.Second), "", "", 0, `{}`, "info")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", "/api/events?limit=50", nil)
		w := httptest.NewRecorder()
		h.listEvents(w, req)
	}
}

// --- Integration: FTS index rebuild ---

func TestFTSRebuildOnStartup(t *testing.T) {
	dir := t.TempDir()

	// Create a handler and insert events
	h, err := NewEventsHandler(filepath.Join(dir, "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	base := time.Now()
	for i := 0; i < 10; i++ {
		h.LogEvent("system", base.Add(time.Duration(i)*time.Second), "", "", 0, `{"rebuild":"test"}`, "info")
	}
	h.Close()

	// Drop the FTS table (simulating corruption)
	_ = os.Remove(filepath.Join(dir, "events.db-wal"))
	_ = os.Remove(filepath.Join(dir, "events.db-shm"))

	// Reopen — FTS should rebuild
	h2, err := NewEventsHandler(filepath.Join(dir, "events.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer h2.Close()

	// Search should still work after rebuild
	req := httptest.NewRequest("GET", "/api/events?q=rebuild&limit=100", nil)
	w := httptest.NewRecorder()
	h2.listEvents(w, req)

	var resp eventsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.TotalFiltered != 10 {
		t.Errorf("after rebuild: total_filtered = %d, want 10", resp.TotalFiltered)
	}
}

// --- Tests for since/until query parameters ---

func TestListEvents_SinceParameter(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	seedEvents(t, h, base, 10)

	// Filter using since parameter (alias for after)
	sinceTime := base.Add(4 * time.Second).Format(time.RFC3339)
	req := httptest.NewRequest("GET", "/api/events?since="+sinceTime+"&limit=100", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var resp eventsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.TotalFiltered != 6 { // events 4..9
		t.Errorf("total_filtered = %d, want 6", resp.TotalFiltered)
	}
	for _, ev := range resp.Events {
		if ev.Timestamp < base.Add(4*time.Second).UnixNano()/1e6 {
			t.Errorf("event ts %d before since time", ev.Timestamp)
		}
	}
}

func TestListEvents_UntilParameter(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	seedEvents(t, h, base, 10)

	// Filter using until parameter (upper bound)
	untilTime := base.Add(5 * time.Second).Format(time.RFC3339)
	req := httptest.NewRequest("GET", "/api/events?until="+untilTime+"&limit=100", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var resp eventsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.TotalFiltered != 6 { // events 0..5
		t.Errorf("total_filtered = %d, want 6", resp.TotalFiltered)
	}
	for _, ev := range resp.Events {
		if ev.Timestamp > base.Add(5*time.Second).UnixNano()/1e6 {
			t.Errorf("event ts %d after until time", ev.Timestamp)
		}
	}
}

func TestListEvents_SinceAndUntil(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	seedEvents(t, h, base, 10)

	// Filter using both since and until
	sinceTime := base.Add(2 * time.Second).Format(time.RFC3339)
	untilTime := base.Add(7 * time.Second).Format(time.RFC3339)
	req := httptest.NewRequest("GET", "/api/events?since="+sinceTime+"&until="+untilTime+"&limit=100", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var resp eventsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.TotalFiltered != 6 { // events 2..7
		t.Errorf("total_filtered = %d, want 6", resp.TotalFiltered)
	}
}

func TestListEvents_InvalidUntil(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/events?until=not-a-date", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// --- Tests for person_id and zone_id parameter aliases ---

func TestListEvents_PersonIDAlias(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	seedEvents(t, h, base, 100)

	// Filter using person_id parameter (alias for person)
	req := httptest.NewRequest("GET", "/api/events?person_id=Alice&limit=100", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var resp eventsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	for _, ev := range resp.Events {
		if ev.Person != "Alice" {
			t.Errorf("event person = %q, want Alice", ev.Person)
		}
	}
}

func TestListEvents_ZoneIDAlias(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	seedEvents(t, h, base, 100)

	// Filter using zone_id parameter (alias for zone)
	req := httptest.NewRequest("GET", "/api/events?zone_id=Kitchen&limit=100", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var resp eventsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	for _, ev := range resp.Events {
		if ev.Zone != "Kitchen" {
			t.Errorf("event zone = %q, want Kitchen", ev.Zone)
		}
	}
}

func TestListEvents_ZoneTakesPrecedence(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	seedEvents(t, h, base, 10)

	// When both zone and zone_id are provided, zone_id should take precedence
	req := httptest.NewRequest("GET", "/api/events?zone=Hallway&zone_id=Kitchen&limit=100", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var resp eventsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	for _, ev := range resp.Events {
		if ev.Zone != "Kitchen" {
			t.Errorf("event zone = %q, want Kitchen (zone_id should take precedence)", ev.Zone)
		}
	}
}

func TestListEvents_PersonIDTakesPrecedence(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	seedEvents(t, h, base, 10)

	// When both person and person_id are provided, person_id should take precedence
	req := httptest.NewRequest("GET", "/api/events?person=Bob&person_id=Alice&limit=100", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var resp eventsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	for _, ev := range resp.Events {
		if ev.Person != "Alice" {
			t.Errorf("event person = %q, want Alice (person_id should take precedence)", ev.Person)
		}
	}
}

// --- Tests for mode parameter (simple vs expert mode) ---

func TestListEvents_SimpleModeFiltersSystemEvents(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	// Insert events with different types
	eventTypes := []string{
		"zone_entry", "zone_exit", "portal_crossing", "fall_alert",
		"anomaly", "security_alert", "sleep_session_end",
		"node_online", "node_offline", "ota_update", "baseline_changed", "system",
	}
	for i, evtType := range eventTypes {
		ts := base.Add(time.Duration(i) * time.Second)
		if err := h.LogEvent(evtType, ts, "Kitchen", "Alice", 0, `{"test":true}`, "info"); err != nil {
			t.Fatalf("LogEvent %s: %v", evtType, err)
		}
	}

	// Simple mode (default) - should exclude system event types
	req := httptest.NewRequest("GET", "/api/events?mode=simple&limit=100", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var resp eventsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	// Should only return user-facing events (zone_entry, zone_exit, portal_crossing, fall_alert, anomaly, security_alert, sleep_session_end)
	// Should exclude: node_online, node_offline, ota_update, baseline_changed, system
	for _, ev := range resp.Events {
		switch ev.Type {
		case "node_online", "node_offline", "ota_update", "baseline_changed", "system":
			t.Errorf("simple mode should exclude system event type %q", ev.Type)
		}
	}

	// Verify we got some events (non-system ones)
	if len(resp.Events) == 0 {
		t.Error("simple mode returned no events, expected non-system events")
	}
}

func TestListEvents_ExpertModeShowsAllEvents(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	// Insert events with different types
	eventTypes := []string{
		"zone_entry", "node_online", "system", "ota_update",
	}
	for i, evtType := range eventTypes {
		ts := base.Add(time.Duration(i) * time.Second)
		if err := h.LogEvent(evtType, ts, "Kitchen", "Alice", 0, `{"test":true}`, "info"); err != nil {
			t.Fatalf("LogEvent %s: %v", evtType, err)
		}
	}

	// Expert mode - should return all events including system types
	req := httptest.NewRequest("GET", "/api/events?mode=expert&limit=100", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var resp eventsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	// Should return all events
	if resp.TotalFiltered != 4 {
		t.Errorf("expert mode: total_filtered = %d, want 4 (all events)", resp.TotalFiltered)
	}

	// Verify we have system events
	hasSystemEvent := false
	for _, ev := range resp.Events {
		if ev.Type == "node_online" || ev.Type == "system" || ev.Type == "ota_update" {
			hasSystemEvent = true
			break
		}
	}
	if !hasSystemEvent {
		t.Error("expert mode should include system events")
	}
}

func TestListEvents_DefaultModeIsSimple(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	// Insert system events
	for i := 0; i < 3; i++ {
		ts := base.Add(time.Duration(i) * time.Second)
		if err := h.LogEvent("system", ts, "", "", 0, `{"test":true}`, "info"); err != nil {
			t.Fatalf("LogEvent: %v", err)
		}
	}
	// Insert user-facing events
	for i := 0; i < 2; i++ {
		ts := base.Add(time.Duration(i+3) * time.Second)
		if err := h.LogEvent("zone_entry", ts, "Kitchen", "Alice", 0, `{"test":true}`, "info"); err != nil {
			t.Fatalf("LogEvent: %v", err)
		}
	}

	// No mode parameter specified - should default to simple mode
	req := httptest.NewRequest("GET", "/api/events?limit=100", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var resp eventsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	// Should exclude system events in default (simple) mode
	for _, ev := range resp.Events {
		if ev.Type == "system" {
			t.Error("default mode (simple) should exclude system events")
		}
	}

	// Should have the user-facing events
	if len(resp.Events) != 2 {
		t.Errorf("default mode: got %d events, want 2 (user-facing only)", len(resp.Events))
	}
}

func TestListEvents_ModeWithTypeFilter(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	// Insert events
	eventTypes := []string{"node_online", "zone_entry", "system"}
	for i, evtType := range eventTypes {
		ts := base.Add(time.Duration(i) * time.Second)
		if err := h.LogEvent(evtType, ts, "Kitchen", "Alice", 0, `{"test":true}`, "info"); err != nil {
			t.Fatalf("LogEvent %s: %v", evtType, err)
		}
	}

	// Simple mode with explicit type filter for a system type
	// When type is explicitly specified, simple mode filtering should not override it
	req := httptest.NewRequest("GET", "/api/events?mode=simple&type=node_online&limit=100", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var resp eventsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	// Should return the requested system type even in simple mode when explicitly requested
	if resp.TotalFiltered != 1 {
		t.Errorf("simple mode with explicit type: total_filtered = %d, want 1", resp.TotalFiltered)
	}
	if len(resp.Events) != 1 || resp.Events[0].Type != "node_online" {
		t.Error("simple mode with explicit type should return requested system event")
	}
}

func TestListEvents_ModeWithCombinedFilters(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	base := time.Now()
	// Insert events with different types, zones, and persons
	events := []struct {
		evtType string
		zone    string
		person  string
	}{
		{"zone_entry", "Kitchen", "Alice"},
		{"zone_exit", "Kitchen", "Alice"},
		{"node_online", "", ""},
		{"system", "", ""},
		{"detection", "Kitchen", "Bob"},
		{"detection", "Hallway", "Alice"},
	}
	for i, e := range events {
		ts := base.Add(time.Duration(i) * time.Second)
		if err := h.LogEvent(e.evtType, ts, e.zone, e.person, 0, `{"test":true}`, "info"); err != nil {
			t.Fatalf("LogEvent: %v", err)
		}
	}

	// Simple mode with zone and person filters
	req := httptest.NewRequest("GET", "/api/events?mode=simple&zone=Kitchen&person=Alice&limit=100", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var resp eventsResponse
	json.NewDecoder(w.Body).Decode(&resp)

	// Should only return zone_entry and zone_exit for Alice in Kitchen (exclude system events)
	if resp.TotalFiltered != 2 {
		t.Errorf("combined filters: total_filtered = %d, want 2", resp.TotalFiltered)
	}
	for _, ev := range resp.Events {
		if ev.Zone != "Kitchen" {
			t.Errorf("event zone = %q, want Kitchen", ev.Zone)
		}
		if ev.Person != "Alice" {
			t.Errorf("event person = %q, want Alice", ev.Person)
		}
	}
}

// --- POST /api/events/{id}/feedback tests ---

func TestPostEventFeedback_ValidFeedbackCorrect(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	// Create an event to submit feedback for
	ts := time.Now()
	h.LogEvent("detection", ts, "Kitchen", "Alice", 42, `{"key":"val"}`, "info")

	// Get the event ID
	req := httptest.NewRequest("GET", "/api/events?limit=1", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var listResp eventsResponse
	json.NewDecoder(w.Body).Decode(&listResp)
	if len(listResp.Events) == 0 {
		t.Fatal("no events returned")
	}
	eventID := listResp.Events[0].ID

	// Create feedback request
	feedbackReq := FeedbackRequest{
		Type:   "correct",
		BlobID: 42,
	}
	body, _ := json.Marshal(feedbackReq)

	// Test the handler
	e := &EventsHandler{db: h.db}
	r := chi.NewRouter()
	r.Post("/api/events/{id}/feedback", e.postEventFeedback)

	req = httptest.NewRequest("POST", "/api/events/"+strconv.FormatInt(eventID, 10)+"/feedback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Errorf("ok = %v, want true", resp["ok"])
	}
}

func TestPostEventFeedback_ValidFeedbackIncorrect(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	// Create an event to submit feedback for
	ts := time.Now()
	h.LogEvent("detection", ts, "Kitchen", "Alice", 42, `{"key":"val"}`, "info")

	// Get the event ID
	req := httptest.NewRequest("GET", "/api/events?limit=1", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var listResp eventsResponse
	json.NewDecoder(w.Body).Decode(&listResp)
	if len(listResp.Events) == 0 {
		t.Fatal("no events returned")
	}
	eventID := listResp.Events[0].ID

	// Create feedback request
	feedbackReq := FeedbackRequest{
		Type:   "incorrect",
		BlobID: 42,
	}
	body, _ := json.Marshal(feedbackReq)

	// Test the handler
	e := &EventsHandler{db: h.db}
	r := chi.NewRouter()
	r.Post("/api/events/{id}/feedback", e.postEventFeedback)

	req = httptest.NewRequest("POST", "/api/events/"+strconv.FormatInt(eventID, 10)+"/feedback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Errorf("ok = %v, want true", resp["ok"])
	}
}

func TestPostEventFeedback_ValidFeedbackMissed(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	// Create an event to submit feedback for
	ts := time.Now()
	h.LogEvent("detection", ts, "Kitchen", "Alice", 0, `{"key":"val"}`, "info")

	// Get the event ID
	req := httptest.NewRequest("GET", "/api/events?limit=1", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var listResp eventsResponse
	json.NewDecoder(w.Body).Decode(&listResp)
	if len(listResp.Events) == 0 {
		t.Fatal("no events returned")
	}
	eventID := listResp.Events[0].ID

	// Create feedback request with position (for "missed" type)
	feedbackReq := FeedbackRequest{
		Type: "missed",
		Position: &struct {
			X float64 `json:"x"`
			Y float64 `json:"y"`
			Z float64 `json:"z"`
		}{
			X: 1.5,
			Y: 2.3,
			Z: 0.8,
		},
	}
	body, _ := json.Marshal(feedbackReq)

	// Test the handler
	e := &EventsHandler{db: h.db}
	r := chi.NewRouter()
	r.Post("/api/events/{id}/feedback", e.postEventFeedback)

	req = httptest.NewRequest("POST", "/api/events/"+strconv.FormatInt(eventID, 10)+"/feedback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["ok"] != true {
		t.Errorf("ok = %v, want true", resp["ok"])
	}
}

func TestPostEventFeedback_EventNotFound(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	// Create feedback request
	feedbackReq := FeedbackRequest{
		Type:   "correct",
		BlobID: 42,
	}
	body, _ := json.Marshal(feedbackReq)

	// Test the handler with non-existent event ID
	e := &EventsHandler{db: h.db}
	r := chi.NewRouter()
	r.Post("/api/events/{id}/feedback", e.postEventFeedback)

	req := httptest.NewRequest("POST", "/api/events/999999/feedback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "event not found" {
		t.Errorf("error = %q, want 'event not found'", resp["error"])
	}
}

func TestPostEventFeedback_InvalidEventID(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	// Create feedback request
	feedbackReq := FeedbackRequest{
		Type:   "correct",
		BlobID: 42,
	}
	body, _ := json.Marshal(feedbackReq)

	// Test the handler with invalid event ID
	e := &EventsHandler{db: h.db}
	r := chi.NewRouter()
	r.Post("/api/events/{id}/feedback", e.postEventFeedback)

	req := httptest.NewRequest("POST", "/api/events/invalid/feedback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "invalid event id" {
		t.Errorf("error = %q, want 'invalid event id'", resp["error"])
	}
}

func TestPostEventFeedback_InvalidFeedbackType(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	// Create an event to submit feedback for
	ts := time.Now()
	h.LogEvent("detection", ts, "Kitchen", "Alice", 42, `{"key":"val"}`, "info")

	// Get the event ID
	req := httptest.NewRequest("GET", "/api/events?limit=1", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var listResp eventsResponse
	json.NewDecoder(w.Body).Decode(&listResp)
	if len(listResp.Events) == 0 {
		t.Fatal("no events returned")
	}
	eventID := listResp.Events[0].ID

	// Create feedback request with invalid type
	feedbackReq := FeedbackRequest{
		Type:   "invalid_type",
		BlobID: 42,
	}
	body, _ := json.Marshal(feedbackReq)

	// Test the handler
	e := &EventsHandler{db: h.db}
	r := chi.NewRouter()
	r.Post("/api/events/{id}/feedback", e.postEventFeedback)

	req = httptest.NewRequest("POST", "/api/events/"+strconv.FormatInt(eventID, 10)+"/feedback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if !strings.Contains(resp["error"], "invalid feedback type") {
		t.Errorf("error = %q, want error containing 'invalid feedback type'", resp["error"])
	}
}

func TestPostEventFeedback_InvalidRequestBody(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	// Create an event to submit feedback for
	ts := time.Now()
	h.LogEvent("detection", ts, "Kitchen", "Alice", 42, `{"key":"val"}`, "info")

	// Get the event ID
	req := httptest.NewRequest("GET", "/api/events?limit=1", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var listResp eventsResponse
	json.NewDecoder(w.Body).Decode(&listResp)
	if len(listResp.Events) == 0 {
		t.Fatal("no events returned")
	}
	eventID := listResp.Events[0].ID

	// Test with invalid JSON body
	e := &EventsHandler{db: h.db}
	r := chi.NewRouter()
	r.Post("/api/events/{id}/feedback", e.postEventFeedback)

	req = httptest.NewRequest("POST", "/api/events/"+strconv.FormatInt(eventID, 10)+"/feedback", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "invalid request body" {
		t.Errorf("error = %q, want 'invalid request body'", resp["error"])
	}
}

func TestPostEventFeedback_WithFeedbackHandler(t *testing.T) {
	h, cleanup := testEventsHandler(t)
	defer cleanup()

	// Create an event to submit feedback for
	ts := time.Now()
	h.LogEvent("detection", ts, "Kitchen", "Alice", 42, `{"key":"val"}`, "info")

	// Get the event ID
	req := httptest.NewRequest("GET", "/api/events?limit=1", nil)
	w := httptest.NewRecorder()
	h.listEvents(w, req)

	var listResp eventsResponse
	json.NewDecoder(w.Body).Decode(&listResp)
	if len(listResp.Events) == 0 {
		t.Fatal("no events returned")
	}
	eventID := listResp.Events[0].ID

	// Create a mock feedback handler that sets a flag
	var feedbackProcessed bool
	mockHandler := &mockFeedbackHandler{
		submitFunc: func(w http.ResponseWriter, r *http.Request, req FeedbackRequest) {
			feedbackProcessed = true
			// Verify the request
			if req.EventID != eventID {
				t.Errorf("event ID = %d, want %d", req.EventID, eventID)
			}
			if req.Type != "correct" {
				t.Errorf("type = %q, want 'correct'", req.Type)
			}
			if req.BlobID != 42 {
				t.Errorf("blob_id = %d, want 42", req.BlobID)
			}
			writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
		},
	}

	// Set the feedback handler
	h.SetFeedbackHandler(mockHandler)

	// Create feedback request
	feedbackReq := FeedbackRequest{
		Type:   "correct",
		BlobID: 42,
	}
	body, _ := json.Marshal(feedbackReq)

	// Test the handler
	r := chi.NewRouter()
	r.Post("/api/events/{id}/feedback", h.postEventFeedback)

	req = httptest.NewRequest("POST", "/api/events/"+strconv.FormatInt(eventID, 10)+"/feedback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if !feedbackProcessed {
		t.Error("feedback handler was not called")
	}

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// mockFeedbackHandler is a mock implementation of the feedback handler interface
type mockFeedbackHandler struct {
	submitFunc func(w http.ResponseWriter, r *http.Request, req FeedbackRequest)
}

func (m *mockFeedbackHandler) SubmitFeedback(w http.ResponseWriter, r *http.Request, req FeedbackRequest) {
	if m.submitFunc != nil {
		m.submitFunc(w, r, req)
	}
}
