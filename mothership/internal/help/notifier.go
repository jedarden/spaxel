// Package help provides feature discovery notification management.
// It tracks one-time notifications for features that become available and ensures
// they fire only once per feature, respecting quiet hours.
package help

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	_ "modernc.org/sqlite"
)

// Notifier manages feature discovery notifications.
type Notifier struct {
	mu   sync.RWMutex
	db   *sql.DB
	quietHours *QuietHours
}

// QuietHours defines when notifications should be suppressed.
type QuietHours struct {
	Enabled    bool
	StartHour  int // 0-23
	StartMin   int
	EndHour    int
	EndMin     int
	DaysMask   int // Bitmask for days (0=Sun, 1=Mon, ..., 6=Sat)
}

// FeatureNotification represents a one-time notification for a feature.
type FeatureNotification struct {
	EventID        string    `json:"event_id"`         // Unique identifier
	Title          string    `json:"title"`
	Message        string    `json:"message"`
	ActionLabel    string    `json:"action_label,omitempty"` // Button text
	ActionURL      string    `json:"action_url,omitempty"`   // Link for button
	DismissedAt    *time.Time `json:"dismissed_at,omitempty"`
	FiredAt        time.Time `json:"fired_at"`
}

// Predefined feature notification events
const (
	EventDiurnalBaselineActivated   = "diurnal_baseline_activated"
	EventFirstSleepSessionComplete   = "first_sleep_session_complete"
	EventWeightUpdateApproved        = "weight_update_approved"
	EventAutomationFirstFired        = "automation_first_fired"
	EventPredictionModelReady        = "prediction_model_ready"
)

// NewNotifier creates a new feature notification manager.
// The feature_notifications table must already exist via migration.
func NewNotifier(db *sql.DB) (*Notifier, error) {
	return &Notifier{
		db:         db,
		quietHours: &QuietHours{},
	}, nil
}

// NewNotifierWithQuietHours creates a new feature notification manager with quiet hours configured.
func NewNotifierWithQuietHours(db *sql.DB, qh *QuietHours) (*Notifier, error) {
	return &Notifier{
		db:         db,
		quietHours: qh,
	}, nil
}

// LoadQuietHoursFromSettings loads quiet hours configuration from settings map.
// Expects settings to contain "quiet_hours_start" and "quiet_hours_end" in "HH:MM" format.
// Also looks for "quiet_hours_enabled" (bool) and "quiet_hours_days_mask" (int).
func (n *Notifier) LoadQuietHoursFromSettings(settings map[string]interface{}) error {
	n.mu.Lock()
	defer n.mu.Unlock()

	qh := &QuietHours{}

	// Check if quiet hours are enabled
	if enabled, ok := settings["quiet_hours_enabled"].(bool); ok {
		qh.Enabled = enabled
	} else {
		// If not explicitly set, enable if start/end times are configured
		qh.Enabled = false
	}

	// Parse start time (HH:MM format)
	if startStr, ok := settings["quiet_hours_start"].(string); ok && startStr != "" {
		if h, m, err := parseHM(startStr); err == nil {
			qh.StartHour = h
			qh.StartMin = m
			qh.Enabled = true // Enable if start time is set
		}
	}

	// Parse end time (HH:MM format)
	if endStr, ok := settings["quiet_hours_end"].(string); ok && endStr != "" {
		if h, m, err := parseHM(endStr); err == nil {
			qh.EndHour = h
			qh.EndMin = m
			qh.Enabled = true // Enable if end time is set
		}
	}

	// Parse days mask (0-127 bitmask, 0=Sunday)
	if daysMask, ok := settings["quiet_hours_days_mask"].(int); ok {
		qh.DaysMask = daysMask
	}
	// Also check float64 (JSON numbers)
	if daysMaskFloat, ok := settings["quiet_hours_days_mask"].(float64); ok {
		qh.DaysMask = int(daysMaskFloat)
	}

	n.quietHours = qh
	return nil
}

// parseHM parses a time string in "HH:MM" format.
func parseHM(s string) (hour, min int, err error) {
	var h, m int
	_, err = fmt.Sscanf(s, "%d:%d", &h, &m)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid time format: %q", s)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("invalid time values: %q", s)
	}
	return h, m, nil
}

// SetQuietHours sets the quiet hours configuration.
func (n *Notifier) SetQuietHours(qh *QuietHours) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.quietHours = qh
}

// isQuietHours checks if current time is within quiet hours.
func (n *Notifier) isQuietHours(t time.Time) bool {
	n.mu.RLock()
	defer n.mu.RUnlock()

	if !n.quietHours.Enabled {
		return false
	}

	// Check day of week
	dow := int(t.Weekday())
	if n.quietHours.DaysMask != 0 && (n.quietHours.DaysMask & (1 << dow)) == 0 {
		return false
	}

	// Check time range
	currentMin := t.Hour()*60 + t.Minute()
	startMin := n.quietHours.StartHour*60 + n.quietHours.StartMin
	endMin := n.quietHours.EndHour*60 + n.quietHours.EndMin

	// Handle overnight ranges (e.g., 22:00 to 06:00)
	if startMin <= endMin {
		return currentMin >= startMin && currentMin < endMin
	}
	// Overnight range
	return currentMin >= startMin || currentMin < endMin
}

// FireNotification fires a feature discovery notification if it hasn't been fired before.
// Returns true if the notification was fired, false if it was already fired or suppressed.
func (n *Notifier) FireNotification(eventID, title, message string) bool {
	return n.FireNotificationWithAction(eventID, title, message, "", "")
}

// FireNotificationWithAction fires a notification with an action button.
func (n *Notifier) FireNotificationWithAction(eventID, title, message, actionLabel, actionURL string) bool {
	// Check if quiet hours
	if n.isQuietHours(time.Now()) {
		log.Printf("[DEBUG] help: notification %s suppressed during quiet hours", eventID)
		return false
	}

	// Check if already fired
	var firedAt int64
	err := n.db.QueryRow("SELECT fired_at FROM feature_notifications WHERE event_id = ?", eventID).Scan(&firedAt)
	if err == nil {
		// Already fired
		log.Printf("[DEBUG] help: notification %s already fired at %v", eventID, time.Unix(firedAt, 0))
		return false
	}
	if err != sql.ErrNoRows {
		log.Printf("[ERROR] help: failed to check notification status: %v", err)
		return false
	}

	// Fire the notification
	now := time.Now()
	_, err = n.db.Exec("INSERT INTO feature_notifications (event_id, fired_at) VALUES (?, ?)",
		eventID, now.Unix())
	if err != nil {
		log.Printf("[ERROR] help: failed to record notification: %v", err)
		return false
	}

	log.Printf("[INFO] help: fired feature notification %s: %s", eventID, title)
	return true
}

// GetPendingNotifications returns all fired but unacknowledged notifications.
func (n *Notifier) GetPendingNotifications() ([]FeatureNotification, error) {
	rows, err := n.db.Query(`
		SELECT event_id, fired_at, acknowledged_at
		FROM feature_notifications
		WHERE acknowledged_at IS NULL
		ORDER BY fired_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var notifications []FeatureNotification
	for rows.Next() {
		var fn FeatureNotification
		var acknowledgedAt sql.NullInt64
		err := rows.Scan(&fn.EventID, &fn.FiredAt, &acknowledgedAt)
		if err != nil {
			continue
		}
		if acknowledgedAt.Valid {
			fn.DismissedAt = func() *time.Time {
				t := time.Unix(acknowledgedAt.Int64, 0)
				return &t
			}()
		}
		fn.Title = getNotificationTitle(fn.EventID)
		fn.Message = getNotificationMessage(fn.EventID)
		fn.ActionLabel = getNotificationActionLabel(fn.EventID)
		fn.ActionURL = getNotificationActionURL(fn.EventID)
		notifications = append(notifications, fn)
	}

	return notifications, nil
}

// AcknowledgeNotification marks a notification as acknowledged.
func (n *Notifier) AcknowledgeNotification(eventID string) error {
	_, err := n.db.Exec("UPDATE feature_notifications SET acknowledged_at = ? WHERE event_id = ?",
		time.Now().Unix(), eventID)
	return err
}

// RegisterRoutes registers API routes for the notifier.
func (n *Notifier) RegisterRoutes(r chi.Router) {
	r.Get("/api/help/notifications", n.handleGetNotifications)
	r.Post("/api/help/notifications/{eventID}/acknowledge", n.handleAcknowledge)
	r.Post("/api/help/notifications/test", n.handleTest)
}

// handleGetNotifications returns pending feature discovery notifications.
func (n *Notifier) handleGetNotifications(w http.ResponseWriter, r *http.Request) {
	notifications, err := n.GetPendingNotifications()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"notifications": notifications,
	})
}

// handleAcknowledge marks a notification as acknowledged.
func (n *Notifier) handleAcknowledge(w http.ResponseWriter, r *http.Request) {
	eventID := chi.URLParam(r, "eventID")
	if eventID == "" {
		http.Error(w, "event_id required", http.StatusBadRequest)
		return
	}

	if err := n.AcknowledgeNotification(eventID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok": true,
	})
}

// handleTest fires a test notification for development.
func (n *Notifier) handleTest(w http.ResponseWriter, r *http.Request) {
	// Use a test event ID
	testEventID := "test_notification_" + time.Now().Format("20060102_150405")
	fired := n.FireNotificationWithAction(
		testEventID,
		"Test Notification",
		"This is a test feature discovery notification.",
		"Dismiss",
		"",
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":    true,
		"fired": fired,
		"event_id": testEventID,
	})
}

// Notification content helpers

func getNotificationTitle(eventID string) string {
	titles := map[string]string{
		EventDiurnalBaselineActivated: "Your system has learned your home's daily patterns",
		EventFirstSleepSessionComplete: "Your first sleep session was tracked overnight",
		EventWeightUpdateApproved:     "Localization accuracy improved",
		EventAutomationFirstFired:     "Your first automation just ran",
		EventPredictionModelReady:     "Presence predictions are now available",
	}
	if title, ok := titles[eventID]; ok {
		return title
	}
	return "New Feature Available"
}

func getNotificationMessage(eventID string) string {
	messages := map[string]string{
		EventDiurnalBaselineActivated: "Detection accuracy should improve starting today. The system now understands the daily RF patterns in your home.",
		EventFirstSleepSessionComplete: "Tap to see your sleep summary. Sleep data will accumulate over the coming nights for more detailed reports.",
		EventWeightUpdateApproved:      "Median position error decreased based on your BLE device positions. The system is adapting to your space.",
		EventAutomationFirstFired:      " automations are now active. You can view automation history in the Automations panel.",
		EventPredictionModelReady:      "The system has learned when people are typically in each room. Predictions appear in the Predictions panel.",
	}
	if msg, ok := messages[eventID]; ok {
		return msg
	}
	return "A new feature is now available in your Spaxel system."
}

func getNotificationActionLabel(eventID string) string {
	labels := map[string]string{
		EventDiurnalBaselineActivated: "View Diurnal Baseline",
		EventFirstSleepSessionComplete: "View Sleep Summary",
		EventWeightUpdateApproved:      "View Accuracy Trends",
		EventAutomationFirstFired:      "View Automations Log",
		EventPredictionModelReady:      "View Predictions",
	}
	if label, ok := labels[eventID]; ok {
		return label
	}
	return ""
}

func getNotificationActionURL(eventID string) string {
	urls := map[string]string{
		EventDiurnalBaselineActivated: "#/settings/diurnal",
		EventFirstSleepSessionComplete: "#/sleep",
		EventWeightUpdateApproved:      "#/accuracy",
		EventAutomationFirstFired:      "#/automations",
		EventPredictionModelReady:      "#/predictions",
	}
	if url, ok := urls[eventID]; ok {
		return url
	}
	return ""
}
