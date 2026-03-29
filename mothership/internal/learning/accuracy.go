// Package learning provides accuracy metric computation for detection
package learning

import (
	"context"
	"fmt"
	"log"
	"math"
	"sync"
	"time"
)

// AccuracyComputerConfig holds configuration for accuracy computation
type AccuracyComputerConfig struct {
	ComputeInterval time.Duration // How often to compute accuracy metrics
	HistoryWeeks    int           // Number of weeks to keep in history
}

// DefaultAccuracyComputerConfig returns default configuration
func DefaultAccuracyComputerConfig() AccuracyComputerConfig {
	return AccuracyComputerConfig{
		ComputeInterval: 24 * time.Hour, // Daily computation
		HistoryWeeks:    8,              // Keep 8 weeks of history
	}
}

// AccuracyComputer computes precision, recall, and F1 metrics
type AccuracyComputer struct {
	store  *FeedbackStore
	config AccuracyComputerConfig
	mu     sync.RWMutex
}

// NewAccuracyComputer creates a new accuracy computer
func NewAccuracyComputer(store *FeedbackStore, config AccuracyComputerConfig) *AccuracyComputer {
	return &AccuracyComputer{
		store:  store,
		config: config,
	}
}

// Run starts the background accuracy computation loop
func (a *AccuracyComputer) Run(ctx context.Context) {
	ticker := time.NewTicker(a.config.ComputeInterval)
	defer ticker.Stop()

	// Compute once at startup
	a.ComputeAll()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.ComputeAll()
		}
	}
}

// ComputeNow triggers an immediate accuracy computation
func (a *AccuracyComputer) ComputeNow() error {
	return a.ComputeAll()
}

// ComputeAll computes accuracy metrics for all scopes
func (a *AccuracyComputer) ComputeAll() error {
	// Get current week
	currentWeek := GetWeekString(time.Now())

	// Compute system-wide metrics
	if err := a.computeForScope(ScopeTypeSystem, ScopeIDSystem, currentWeek); err != nil {
		log.Printf("[WARN] Failed to compute system accuracy: %v", err)
	}

	// Compute per-link metrics
	if err := a.computePerLink(currentWeek); err != nil {
		log.Printf("[WARN] Failed to compute per-link accuracy: %v", err)
	}

	// Compute per-zone metrics
	if err := a.computePerZone(currentWeek); err != nil {
		log.Printf("[WARN] Failed to compute per-zone accuracy: %v", err)
	}

	return nil
}

// Scope types and IDs
const (
	ScopeTypeSystem = "system"
	ScopeTypeLink   = "link"
	ScopeTypeZone   = "zone"
	ScopeTypePerson = "person"

	ScopeIDSystem = "all"
)

// computeForScope computes accuracy metrics for a specific scope
func (a *AccuracyComputer) computeForScope(scopeType, scopeID, week string) error {
	tp, fp, fn, err := a.getCounts(scopeType, scopeID, week)
	if err != nil {
		return err
	}

	// Compute metrics
	precision := 0.0
	if tp+fp > 0 {
		precision = float64(tp) / float64(tp+fp)
	}

	recall := 0.0
	if tp+fn > 0 {
		recall = float64(tp) / float64(tp+fn)
	}

	f1 := 0.0
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}

	// Round to 4 decimal places
	precision = math.Round(precision*10000) / 10000
	recall = math.Round(recall*10000) / 10000
	f1 = math.Round(f1*10000) / 10000

	record := AccuracyRecord{
		Week:       week,
		ScopeType:  scopeType,
		ScopeID:    scopeID,
		Precision:  precision,
		Recall:     recall,
		F1:         f1,
		TPCount:    tp,
		FPCount:    fp,
		FNCount:    fn,
		ComputedAt: time.Now(),
	}

	return a.store.SaveAccuracyRecord(record)
}

// getCounts retrieves TP, FP, FN counts for a scope in a given week
func (a *AccuracyComputer) getCounts(scopeType, scopeID, week string) (tp, fp, fn int, err error) {
	// Get week start/end times
	weekStart, err := parseWeekString(week)
	if err != nil {
		return 0, 0, 0, err
	}
	weekEnd := weekStart.Add(7 * 24 * time.Hour)

	// Get all feedback in the week
	feedbacks, err := a.getFeedbackInTimeRange(weekStart, weekEnd)
	if err != nil {
		return 0, 0, 0, err
	}

	// Filter by scope and count
	for _, f := range feedbacks {
		// Check if feedback belongs to this scope
		if !a.matchesScope(f, scopeType, scopeID) {
			continue
		}

		switch f.FeedbackType {
		case TruePositive:
			tp++
		case FalsePositive:
			fp++
		case FalseNegative:
			fn++
		}
	}

	return tp, fp, fn, nil
}

// getFeedbackInTimeRange retrieves all feedback in a time range
func (a *AccuracyComputer) getFeedbackInTimeRange(start, end time.Time) ([]FeedbackRecord, error) {
	// This is a simplified implementation - in production you'd have a more efficient query
	stats, err := a.store.GetFeedbackStats()
	if err != nil {
		return nil, err
	}

	// For now, use the stats to get counts
	// A full implementation would query feedback by timestamp
	_ = stats
	_ = start
	_ = end

	// Return empty for now - actual implementation would query the database
	return nil, nil
}

// matchesScope checks if a feedback record matches the given scope
func (a *AccuracyComputer) matchesScope(f FeedbackRecord, scopeType, scopeID string) bool {
	if scopeType == ScopeTypeSystem && scopeID == ScopeIDSystem {
		return true // System scope matches everything
	}

	if f.Details == nil {
		return false
	}

	switch scopeType {
	case ScopeTypeLink:
		if linkID, ok := f.Details["link_id"].(string); ok {
			return linkID == scopeID
		}
	case ScopeTypeZone:
		if zoneID, ok := f.Details["zone_id"].(string); ok {
			return zoneID == scopeID
		}
	case ScopeTypePerson:
		if personID, ok := f.Details["person_id"].(string); ok {
			return personID == scopeID
		}
	}

	return false
}

// computePerLink computes accuracy for each link
func (a *AccuracyComputer) computePerLink(week string) error {
	// Get all unique link IDs from feedback
	linkIDs := a.getUniqueScopeIDs(ScopeTypeLink)

	for _, linkID := range linkIDs {
		if err := a.computeForScope(ScopeTypeLink, linkID, week); err != nil {
			log.Printf("[WARN] Failed to compute accuracy for link %s: %v", linkID, err)
		}
	}

	return nil
}

// computePerZone computes accuracy for each zone
func (a *AccuracyComputer) computePerZone(week string) error {
	zoneIDs := a.getUniqueScopeIDs(ScopeTypeZone)

	for _, zoneID := range zoneIDs {
		if err := a.computeForScope(ScopeTypeZone, zoneID, week); err != nil {
			log.Printf("[WARN] Failed to compute accuracy for zone %s: %v", zoneID, err)
		}
	}

	return nil
}

// getUniqueScopeIDs extracts unique scope IDs from feedback
func (a *AccuracyComputer) getUniqueScopeIDs(scopeType string) []string {
	// This would query distinct scope IDs from feedback
	// Simplified implementation for now
	return nil
}

// parseWeekString parses a week string (e.g., "2026-W13") into a time
func parseWeekString(week string) (time.Time, error) {
	var year, weekNum int
	_, err := fmt.Sscanf(week, "%d-W%d", &year, &weekNum)
	if err != nil {
		return time.Time{}, err
	}

	// Get the first day of the year
	t := time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC)

	// Add weeks (ISO weeks start on Monday)
	for t.Weekday() != time.Monday {
		t = t.AddDate(0, 0, -1)
	}

	// Add the week offset
	t = t.AddDate(0, 0, (weekNum-1)*7)

	return t, nil
}

// GetAccuracyHistory retrieves accuracy history for a scope
func (a *AccuracyComputer) GetAccuracyHistory(scopeType, scopeID string, weeks int) ([]AccuracyRecord, error) {
	return a.store.GetAccuracyHistory(scopeType, scopeID, weeks)
}

// GetCurrentAccuracy retrieves current week's accuracy for a scope
func (a *AccuracyComputer) GetCurrentAccuracy(scopeType, scopeID string) (*AccuracyRecord, error) {
	currentWeek := GetWeekString(time.Now())
	records, err := a.store.GetAccuracyHistory(scopeType, scopeID, 1)
	if err != nil || len(records) == 0 {
		return nil, err
	}

	// Find the current week's record
	for _, r := range records {
		if r.Week == currentWeek {
			return &r, nil
		}
	}

	return nil, nil
}

// GetImprovementStats calculates improvement statistics
func (a *AccuracyComputer) GetImprovementStats() (map[string]interface{}, error) {
	currentWeek := GetWeekString(time.Now())
	lastWeek := GetWeekString(time.Now().AddDate(0, 0, -7))

	currentRecords, err := a.store.GetAllAccuracyRecords(currentWeek)
	if err != nil {
		return nil, err
	}

	lastWeekRecords, err := a.store.GetAllAccuracyRecords(lastWeek)
	if err != nil {
		return nil, err
	}

	// Calculate average F1 for each week
	currentAvg := 0.0
	currentCount := 0
	for _, r := range currentRecords {
		if r.ScopeType == ScopeTypeSystem {
			currentAvg = r.F1
			currentCount = 1
			break
		}
	}

	lastAvg := 0.0
	lastCount := 0
	for _, r := range lastWeekRecords {
		if r.ScopeType == ScopeTypeSystem {
			lastAvg = r.F1
			lastCount = 1
			break
		}
	}

	// Calculate improvement percentage
	improvement := 0.0
	if lastCount > 0 && currentCount > 0 && lastAvg > 0 {
		improvement = ((currentAvg - lastAvg) / lastAvg) * 100
	}

	// Get feedback stats
	stats, err := a.store.GetFeedbackStats()
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"current_f1":        currentAvg,
		"last_week_f1":      lastAvg,
		"improvement_pct":   improvement,
		"total_feedback":    stats["total_count"],
		"this_week_feedback": stats["this_week_count"],
		"unprocessed_count": stats["unprocessed_count"],
	}, nil
}
