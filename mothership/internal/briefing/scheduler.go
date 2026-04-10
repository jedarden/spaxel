// Package briefing provides scheduling for morning briefing push notifications.
package briefing

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// Scheduler handles automatic briefing generation and push notifications.
type Scheduler struct {
	generator      *Generator
	notifyService  NotifyService
	mu             sync.RWMutex
	config         SchedulerConfig
	ticker         *time.Ticker
	stopChan       chan struct{}
	running        bool
}

// SchedulerConfig holds scheduling configuration.
type SchedulerConfig struct {
	Enabled          bool
	Time             string // HH:MM format, e.g., "07:00"
	PushNotification bool
	AutoGenerate     bool
	Timezone         string // IANA timezone name
}

// NotifyService is the interface for sending push notifications.
type NotifyService interface {
	Send(notification Notification) error
}

// Notification represents a push notification.
type Notification struct {
	Title     string
	Body      string
	Priority  int
	Tags      []string
	Image     []byte
	ImageType string
	Timestamp time.Time
}

// NewScheduler creates a new briefing scheduler.
func NewScheduler(gen *Generator, notify NotifyService, config SchedulerConfig) *Scheduler {
	if config.Time == "" {
		config.Time = "07:00" // Default 7 AM
	}
	if config.Timezone == "" {
		config.Timezone = "Local"
	}

	return &Scheduler{
		generator:     gen,
		notifyService: notify,
		config:        config,
		stopChan:      make(chan struct{}),
	}
}

// Start begins the scheduling loop.
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()

	// Start ticker to check every minute
	s.ticker = time.NewTicker(1 * time.Minute)

	go func() {
		defer s.ticker.Stop()

		// Initial check on start
		s.checkAndGenerate()

		for {
			select {
			case <-s.ticker.C:
				s.checkAndGenerate()
			case <-s.stopChan:
				return
			case <-ctx.Done():
				return
			}
		}
	}()

	log.Printf("[INFO] Briefing scheduler started (time: %s, push: %v)",
		s.config.Time, s.config.PushNotification)
}

// Stop stops the scheduling loop.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return
	}

	s.running = false
	close(s.stopChan)

	if s.ticker != nil {
		s.ticker.Stop()
	}

	log.Printf("[INFO] Briefing scheduler stopped")
}

// SetConfig updates the scheduler configuration.
func (s *Scheduler) SetConfig(config SchedulerConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()

	oldConfig := s.config
	s.config = config

	// If time changed, reset ticker to trigger sooner
	if oldConfig.Time != config.Time {
		if s.ticker != nil {
			s.ticker.Reset(1 * time.Minute)
		}
	}
}

// GetConfig returns the current configuration.
func (s *Scheduler) GetConfig() SchedulerConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.config
}

// checkAndGenerate checks if it's time to generate and send a briefing.
func (s *Scheduler) checkAndGenerate() {
	s.mu.RLock()
	config := s.config
	s.mu.RUnlock()

	if !config.Enabled || !config.AutoGenerate {
		return
	}

	// Parse configured time
	hour, minute, err := parseTime(config.Time)
	if err != nil {
		log.Printf("[ERROR] Failed to parse briefing time %q: %v", config.Time, err)
		return
	}

	// Get current time in configured timezone
	now := s.nowInTimezone()

	// Check if we're at the configured time (within 1 minute window)
	if now.Hour() != hour || now.Minute() != minute {
		return
	}

	// Get today's date
	date := now.Format("2006-01-02")

	// Check if briefing was already generated today
	if !s.generator.ShouldGenerate(date, "") {
		log.Printf("[DEBUG] Briefing already generated for %s, skipping", date)
		return
	}

	// Generate briefing
	b, err := s.generator.Generate(date, "")
	if err != nil {
		log.Printf("[ERROR] Failed to generate briefing for %s: %v", date, err)
		return
	}

	// Save briefing
	if err := s.generator.Save(b); err != nil {
		log.Printf("[ERROR] Failed to save briefing for %s: %v", date, err)
	}

	log.Printf("[INFO] Morning briefing generated for %s", date)

	// Send push notification if enabled
	if config.PushNotification && s.notifyService != nil {
		s.sendNotification(b)
	}
}

// sendNotification sends a push notification for the briefing.
func (s *Scheduler) sendNotification(b *Briefing) {
	notification := Notification{
		Title:     "Morning Briefing",
		Body:       s.formatNotificationBody(b),
		Priority:  1, // Low priority for morning briefings
		Tags:      []string{"briefing", "morning"},
		Timestamp: time.Now(),
	}

	if err := s.notifyService.Send(notification); err != nil {
		log.Printf("[ERROR] Failed to send briefing notification: %v", err)
	} else {
		log.Printf("[INFO] Morning briefing notification sent for %s", b.Date)
	}
}

// formatNotificationBody formats the briefing content for push notifications.
// Truncates to a reasonable length for push notifications.
func (s *Scheduler) formatNotificationBody(b *Briefing) string {
	// Use the first 200 characters of the briefing content
	maxLen := 200
	content := b.Content
	if len(content) > maxLen {
		content = content[:maxLen] + "..."
	}
	return content
}

// nowInTimezone returns the current time in the configured timezone.
func (s *Scheduler) nowInTimezone() time.Time {
	s.mu.RLock()
	timezone := s.config.Timezone
	s.mu.RUnlock()

	if timezone == "Local" || timezone == "" {
		return time.Now()
	}

	loc, err := time.LoadLocation(timezone)
	if err != nil {
		log.Printf("[WARN] Failed to load timezone %q, using local time: %v", timezone, err)
		return time.Now()
	}

	return time.Now().In(loc)
}

// parseTime parses a time string in HH:MM format.
func parseTime(s string) (hour, minute int, err error) {
	var h, m int
	_, err = fmt.Sscanf(s, "%d:%d", &h, &m)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid time format: %w", err)
	}
	if h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, fmt.Errorf("invalid time values: %d:%d", h, m)
	}
	return h, m, nil
}

// ShouldGenerateNow checks if a briefing should be generated at this moment.
// This is useful for testing and manual triggers.
func (s *Scheduler) ShouldGenerateNow() bool {
	s.mu.RLock()
	config := s.config
	s.mu.RUnlock()

	if !config.Enabled || !config.AutoGenerate {
		return false
	}

	hour, minute, err := parseTime(config.Time)
	if err != nil {
		return false
	}

	now := s.nowInTimezone()
	return now.Hour() == hour && now.Minute() == minute
}

// TriggerNow manually triggers briefing generation and notification.
func (s *Scheduler) TriggerNow(date string) error {
	s.mu.RLock()
	config := s.config
	s.mu.RUnlock()

	if date == "" {
		now := s.nowInTimezone()
		date = now.Format("2006-01-02")
	}

	// Generate briefing
	b, err := s.generator.Generate(date, "")
	if err != nil {
		return err
	}

	// Save briefing
	if err := s.generator.Save(b); err != nil {
		return err
	}

	log.Printf("[INFO] Manual briefing trigger for %s", date)

	// Send push notification if enabled
	if config.PushNotification && s.notifyService != nil {
		s.sendNotification(b)
	}

	return nil
}
