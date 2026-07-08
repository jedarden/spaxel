// Package analytics provides alert handling for anomaly detection.
package analytics

import (
	"testing"
	"time"

	"github.com/spaxel/mothership/internal/events"
)

// thumbBlob is the anonymous struct both NotificationService.GenerateFloorPlanThumbnail
// and Service.GenerateFloorPlanThumbnail accept.
type thumbBlob = struct {
	X, Y, Z  float64
	Identity string
	IsFall   bool
}

// fakeNotifyService captures what the analytics alert handler propagates downstream:
// the Notification it hands to Send (asserted to carry person_name in Data) and the
// blob list it hands to GenerateFloorPlanThumbnail (asserted to carry Identity).
type fakeNotifyService struct {
	lastNotif    Notification
	thumbBlobs   []thumbBlob
	thumbCalls   int
	sendErr      error
}

func (f *fakeNotifyService) Send(notif Notification) error {
	f.lastNotif = notif
	return f.sendErr
}

func (f *fakeNotifyService) GenerateFloorPlanThumbnail(width, height int, blobs []thumbBlob) ([]byte, error) {
	f.thumbCalls++
	f.thumbBlobs = blobs
	return []byte("fake-png"), nil
}

// TestAlertHandler_PropagatesResolvedIdentity verifies the analytics (alert
// handler) projection observes a NON-EMPTY identity at runtime and propagates it
// downstream: SendAlert copies event.PersonName into BOTH the notification's Data
// map (person_name / person_id) AND the floor-plan thumbnail blob's Identity
// field. This is the runtime identity-flow the bf-5151 unit-test structs guard
// but which was never exercised end-to-end. (bf-2v9g)
func TestAlertHandler_PropagatesResolvedIdentity(t *testing.T) {
	t.Parallel()
	fake := &fakeNotifyService{}
	h := NewNotificationAlertHandler(fake)

	event := events.AnomalyEvent{
		ID:          "anom-1",
		Type:        events.AnomalyUnusualDwell,
		Score:       0.91,
		Description: "Unusual dwell in Kitchen",
		ZoneID:      "zone-1",
		ZoneName:    "Kitchen",
		BlobID:      7,
		PersonID:    "person-alice",
		PersonName:  "Alice",
		Position:    events.Position{X: 3, Y: 2, Z: 1},
		Timestamp:   time.Now(),
	}

	if err := h.SendAlert(event, false); err != nil {
		t.Fatalf("SendAlert returned error: %v", err)
	}

	// (1) The notify Notification carries the resolved person identity.
	if got := fake.lastNotif.Data["person_name"]; got != "Alice" {
		t.Fatalf("notify projection observed empty identity: Data[person_name]=%v, want %q", got, "Alice")
	}
	if got := fake.lastNotif.Data["person_id"]; got != "person-alice" {
		t.Fatalf("Data[person_id]=%v, want %q", got, "person-alice")
	}

	// (2) The floor-plan thumbnail blob also carries the resolved identity.
	if fake.thumbCalls == 0 {
		t.Fatal("GenerateFloorPlanThumbnail was never called by SendAlert")
	}
	if len(fake.thumbBlobs) != 1 {
		t.Fatalf("thumbnail received %d blobs, want 1", len(fake.thumbBlobs))
	}
	if got := fake.thumbBlobs[0].Identity; got != "Alice" {
		t.Fatalf("notify thumbnail blob Identity=%q, want %q", got, "Alice")
	}
}

// TestAlertHandler_NilSafeWithUnidentifiedEvent verifies the analytics projection
// never dereferences an UNSET identity field at runtime: an AnomalyEvent with no
// resolved person (the unidentified-intruder / "where applicable" case) flows
// through SendAlert without panic, with empty person fields surfaced as such.
// (bf-2v9g)
func TestAlertHandler_NilSafeWithUnidentifiedEvent(t *testing.T) {
	t.Parallel()
	fake := &fakeNotifyService{}
	h := NewNotificationAlertHandler(fake)

	event := events.AnomalyEvent{
		ID:          "anom-2",
		Type:        events.AnomalyMotionDuringAway,
		Score:       0.88,
		Description: "Motion while away",
		ZoneName:    "Living Room",
		Position:    events.Position{X: 2, Y: 1, Z: 1},
		Timestamp:   time.Now(),
		// PersonID / PersonName intentionally empty (unidentified intruder).
	}

	if err := h.SendAlert(event, true); err != nil {
		t.Fatalf("SendAlert returned error on unidentified event: %v", err)
	}
	if v, ok := fake.lastNotif.Data["person_name"]; ok && v != "" {
		t.Fatalf("unidentified event surfaced non-empty person_name=%v", v)
	}
	// Thumbnail still produced for the unidentified blob (Identity="").
	if len(fake.thumbBlobs) != 1 || fake.thumbBlobs[0].Identity != "" {
		t.Fatalf("expected one unidentified thumbnail blob, got %+v", fake.thumbBlobs)
	}
}
