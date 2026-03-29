// Package learning provides feedback processing for detection accuracy
package learning

import (
	"context"
	"log"
	"sync"
	"time"
)

// ProcessorConfig holds configuration for the feedback processor
type ProcessorConfig struct {
	ProcessInterval time.Duration // How often to process unprocessed feedback
	RetentionWindow time.Duration // How long to keep false positive/negative frames
}

// DefaultProcessorConfig returns default configuration
func DefaultProcessorConfig() ProcessorConfig {
	return ProcessorConfig{
		ProcessInterval: 6 * time.Hour,
		RetentionWindow: 30 * 24 * time.Hour, // 30 days
	}
}

// Processor handles background processing of detection feedback
type Processor struct {
	store   *FeedbackStore
	config  ProcessorConfig
	mu      sync.RWMutex
	running bool

	// Callbacks for extending processor behavior
	onFalsePositive func(feedback FeedbackRecord, details map[string]interface{})
	onFalseNegative func(feedback FeedbackRecord, details map[string]interface{})
}

// NewProcessor creates a new feedback processor
func NewProcessor(store *FeedbackStore, config ProcessorConfig) *Processor {
	return &Processor{
		store:  store,
		config: config,
	}
}

// SetOnFalsePositive sets a callback for false positive processing
func (p *Processor) SetOnFalsePositive(fn func(feedback FeedbackRecord, details map[string]interface{})) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onFalsePositive = fn
}

// SetOnFalseNegative sets a callback for false negative processing
func (p *Processor) SetOnFalseNegative(fn func(feedback FeedbackRecord, details map[string]interface{})) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onFalseNegative = fn
}

// Run starts the background processing loop
func (p *Processor) Run(ctx context.Context) {
	p.mu.Lock()
	p.running = true
	p.mu.Unlock()

	ticker := time.NewTicker(p.config.ProcessInterval)
	defer ticker.Stop()

	// Process once at startup
	p.processBatch()

	for {
		select {
		case <-ctx.Done():
			p.mu.Lock()
			p.running = false
			p.mu.Unlock()
			return
		case <-ticker.C:
			p.processBatch()
		}
	}
}

// ProcessNow triggers an immediate processing cycle
func (p *Processor) ProcessNow() error {
	return p.processBatch()
}

// processBatch processes all unprocessed feedback
func (p *Processor) processBatch() error {
	feedbacks, err := p.store.GetUnprocessedFeedback()
	if err != nil {
		log.Printf("[WARN] Failed to get unprocessed feedback: %v", err)
		return err
	}

	if len(feedbacks) == 0 {
		return nil
	}

	log.Printf("[INFO] Processing %d unprocessed feedback entries", len(feedbacks))

	var processedIDs []string

	for _, feedback := range feedbacks {
		if err := p.processFeedback(feedback); err != nil {
			log.Printf("[WARN] Failed to process feedback %s: %v", feedback.ID, err)
			continue
		}
		processedIDs = append(processedIDs, feedback.ID)
	}

	// Mark as processed
	if len(processedIDs) > 0 {
		if err := p.store.MarkFeedbackProcessed(processedIDs); err != nil {
			log.Printf("[WARN] Failed to mark feedback as processed: %v", err)
			return err
		}
		log.Printf("[INFO] Marked %d feedback entries as processed", len(processedIDs))
	}

	return nil
}

// processFeedback handles a single feedback entry
func (p *Processor) processFeedback(feedback FeedbackRecord) error {
	switch feedback.FeedbackType {
	case FalsePositive:
		return p.processFalsePositive(feedback)
	case FalseNegative:
		return p.processFalseNegative(feedback)
	case TruePositive:
		// True positives don't need special processing, just mark as processed
		return nil
	case WrongIdentity, WrongZone:
		// These feedback types are informational for now
		// Future: could be used to adjust identity/zone thresholds
		return nil
	default:
		log.Printf("[WARN] Unknown feedback type: %s", feedback.FeedbackType)
		return nil
	}
}

// processFalsePositive handles false positive feedback
func (p *Processor) processFalsePositive(feedback FeedbackRecord) error {
	// Extract CSI-related details if available
	details := feedback.Details
	if details == nil {
		details = make(map[string]interface{})
	}

	// Call extension callback if set
	p.mu.RLock()
	callback := p.onFalsePositive
	p.mu.RUnlock()

	if callback != nil {
		callback(feedback, details)
	}

	// If we have link_id and delta_rms, store as a false positive frame
	if linkID, ok := details["link_id"].(string); ok {
		deltaRMS := 0.0
		if d, ok := details["delta_rms"].(float64); ok {
			deltaRMS = d
		}

		frame := FalsePositiveFrame{
			LinkID:    linkID,
			Timestamp: feedback.Timestamp,
			DeltaRMS:  deltaRMS,
			Context:   details,
		}

		if err := p.store.AddFalsePositiveFrame(frame); err != nil {
			return err
		}
	}

	return nil
}

// processFalseNegative handles false negative feedback
func (p *Processor) processFalseNegative(feedback FeedbackRecord) error {
	details := feedback.Details
	if details == nil {
		details = make(map[string]interface{})
	}

	// Call extension callback if set
	p.mu.RLock()
	callback := p.onFalseNegative
	p.mu.RUnlock()

	if callback != nil {
		callback(feedback, details)
	}

	// If we have position and link_id, store as a false negative frame
	if linkID, ok := details["link_id"].(string); ok {
		posX, _ := details["position_x"].(float64)
		posY, _ := details["position_y"].(float64)
		posZ, _ := details["position_z"].(float64)

		frame := FalseNegativeFrame{
			LinkID:            linkID,
			Timestamp:         feedback.Timestamp,
			ExpectedPositionX: posX,
			ExpectedPositionY: posY,
			ExpectedPositionZ: posZ,
			Context:           details,
		}

		if err := p.store.AddFalseNegativeFrame(frame); err != nil {
			return err
		}
	}

	return nil
}

// IsRunning returns whether the processor is running
func (p *Processor) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.running
}
