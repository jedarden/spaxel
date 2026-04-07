// Package loadshed implements adaptive load shedding for the mothership fusion pipeline.
// It monitors pipeline iteration timing and applies 4 shedding levels to keep the
// system responsive under CPU/memory pressure, especially for large fleets.
//
// Level 0 (normal):  rolling avg < 80 ms  — full pipeline
// Level 1 (light):   rolling avg >= 80 ms — suspend crowd flow accumulation
// Level 2 (moderate): rolling avg >= 90 ms — also suspend CSI replay buffer writes
// Level 3 (heavy):   rolling avg >= 95 ms — drop CSI frames when ingest channel > 50% full;
//                     push rate reduction config to all nodes (10 Hz cap)
//
// Recovery: when rolling avg < 60 ms for 10 consecutive iterations, step down one level.
//
// All reads of the shedding level are lock-free via atomic operations.
package loadshed

import (
	"log"
	"sync/atomic"
	"time"
)

// Level represents the current load shedding level (0-3).
type Level int32

const (
	// LevelNormal is the default: full pipeline, no shedding.
	LevelNormal Level = iota
	// LevelLight suspends crowd flow accumulation.
	LevelLight
	// LevelModerate also suspends CSI replay buffer writes.
	LevelModerate
	// LevelHeavy drops CSI frames on full channels and caps node rate at 10 Hz.
	LevelHeavy
)

// String returns a human-readable label for the shedding level.
func (l Level) String() string {
	switch l {
	case LevelNormal:
		return "NOMINAL"
	case LevelLight:
		return "LIGHT"
	case LevelModerate:
		return "MODERATE"
	case LevelHeavy:
		return "HIGH"
	default:
		return "UNKNOWN"
	}
}

// Thresholds for load shedding state transitions.
const (
	// Thresholds for escalating shedding levels.
	thresholdLevel1 = 80 * time.Millisecond
	thresholdLevel2 = 90 * time.Millisecond
	thresholdLevel3 = 95 * time.Millisecond

	// Recovery threshold: rolling avg must stay below this for recoveryCount
	// consecutive iterations before stepping down one level.
	recoveryThreshold = 60 * time.Millisecond
	recoveryCount     = 10

	// Level 3 CSI rate cap pushed to all nodes.
	level3RateCapHz = 10

	// Number of iterations in the rolling average window.
	rollingWindowSize = 5
)

// IngestChannelFull is a callback that reports whether the ingest processing
// channel is more than 50% full. Returns true if shedding should drop frames.
type IngestChannelFull func() bool

// RatePushCallback is called when Level 3 is entered or exited to push
// rate config changes to all connected nodes.
type RatePushCallback func(rateHz int)

// Stage represents a named pipeline stage for timing instrumentation.
type Stage struct {
	Name string
	// Start is set by BeginStage. Use StageDuration() to get elapsed time.
	start time.Time
}

// Shedder manages the load shedding state machine.
type Shedder struct {
	level         atomic.Int32 // Current shedding level (0-3), read lock-free.
	recoveryTicks atomic.Int32 // Consecutive iterations below recovery threshold.

	// Rolling average window (ring buffer).
	durations [rollingWindowSize]time.Duration
	durationsIdx int
	durationsFilled int // how many slots have been written (< rollingWindowSize on startup)

	// Pipeline stage timing for instrumentation.
	stages [8]Stage
	stageIdx int

	// Iteration timing.
	iterStart time.Time

	// External callbacks.
	ingestFull IngestChannelFull
	ratePush   RatePushCallback

	// Previous rate before Level 3 was entered, for restoration.
	prevRateHz atomic.Int32
	level3Active atomic.Bool
}

// New creates a new Shedder.
func New() *Shedder {
	return &Shedder{
		prevRateHz: atomic.Int32{}, // defaults to 0 (unknown)
	}
}

// SetIngestChannelFull sets the callback that reports whether the ingest
// channel is more than 50% full.
func (s *Shedder) SetIngestChannelFull(fn IngestChannelFull) {
	s.ingestFull = fn
}

// SetRatePushCallback sets the callback for pushing rate config to nodes.
func (s *Shedder) SetRatePushCallback(fn RatePushCallback) {
	s.ratePush = fn
}

// SetPreviousRate records the node rate that was active before Level 3
// was entered. Used to restore the rate on recovery.
func (s *Shedder) SetPreviousRate(hz int) {
	s.prevRateHz.Store(int32(hz))
}

// GetLevel returns the current shedding level (lock-free read).
func (s *Shedder) GetLevel() Level {
	return Level(s.level.Load())
}

// ShouldAccumulateCrowdFlow returns false when crowd flow accumulation
// should be suspended (Level >= 1).
func (s *Shedder) ShouldAccumulateCrowdFlow() bool {
	return s.level.Load() < int32(LevelLight)
}

// ShouldWriteReplay returns false when CSI replay buffer writes
// should be suspended (Level >= 2).
func (s *Shedder) ShouldWriteReplay() bool {
	return s.level.Load() < int32(LevelModerate)
}

// ShouldDropFrames returns true when CSI frames should be dropped because
// the ingest channel is more than 50% full (Level 3 only).
func (s *Shedder) ShouldDropFrames() bool {
	return s.level.Load() >= int32(LevelHeavy) && s.ingestFull != nil && s.ingestFull()
}

// IsLevel3Active returns true when Level 3 shedding is active.
func (s *Shedder) IsLevel3Active() bool {
	return s.level3Active.Load()
}

// GetLevel3RateCap returns the rate cap applied during Level 3 (10 Hz).
func (s *Shedder) GetLevel3RateCap() int {
	return level3RateCapHz
}

// BeginIteration marks the start of a pipeline iteration. Call this at the
// beginning of each fusion tick.
func (s *Shedder) BeginIteration() {
	s.iterStart = time.Now()
	s.stageIdx = 0
}

// BeginStage starts timing a named pipeline stage. Returns a Stage handle
// whose duration is captured on EndIteration.
func (s *Shedder) BeginStage(name string) Stage {
	st := Stage{Name: name, start: time.Now()}
	if s.stageIdx < len(s.stages) {
		s.stages[s.stageIdx] = st
	}
	return st
}

// EndStage marks the end of a pipeline stage.
func (s *Shedder) EndStage(st Stage) {
	_ = st // duration computed lazily in GetStageDurations
}

// GetStageDurations returns the durations of all stages from the most recent
// completed iteration.
func (s *Shedder) GetStageDurations() []time.Duration {
	n := s.stageIdx
	if n > len(s.stages) {
		n = len(s.stages)
	}
	result := make([]time.Duration, n)
	for i := 0; i < n; i++ {
		result[i] = time.Since(s.stages[i].start)
	}
	return result
}

// EndIteration marks the end of a pipeline iteration, updates the rolling
// average, and evaluates the shedding state machine.
func (s *Shedder) EndIteration() {
	elapsed := time.Since(s.iterStart)

	// Update rolling average window.
	s.durations[s.durationsIdx] = elapsed
	s.durationsIdx = (s.durationsIdx + 1) % rollingWindowSize
	if s.durationsFilled < rollingWindowSize {
		s.durationsFilled++
	}

	avg := s.rollingAvg()

	// Evaluate state machine.
	prevLevel := Level(s.level.Load())
	var newLevel Level

	if avg >= thresholdLevel3 {
		newLevel = LevelHeavy
	} else if avg >= thresholdLevel2 {
		newLevel = LevelModerate
	} else if avg >= thresholdLevel1 {
		newLevel = LevelLight
	} else {
		// Below all escalation thresholds — check recovery.
		if avg < recoveryThreshold {
			ticks := s.recoveryTicks.Add(1)
			if ticks >= recoveryCount && prevLevel > LevelNormal {
				newLevel = prevLevel - 1
				s.recoveryTicks.Store(0)
			} else {
				newLevel = prevLevel
			}
		} else {
			// Between recovery threshold and Level 1 — hold current level.
			s.recoveryTicks.Store(0)
			newLevel = prevLevel
		}
	}

	// Level can only go UP directly to the new level (no gradual escalation),
	// but recovery steps down one level at a time.
	if newLevel > prevLevel {
		// Escalate directly.
		s.setLevel(newLevel)
	} else if newLevel < prevLevel {
		// Recovery step down.
		s.setLevel(newLevel)
	} else {
		// Reset recovery counter if we didn't step down this tick
		// and we're not in recovery mode.
		if avg >= recoveryThreshold {
			s.recoveryTicks.Store(0)
		}
	}
}

// setLevel applies a level change and logs it.
func (s *Shedder) setLevel(new Level) {
	prev := Level(s.level.Swap(int32(new)))
	if prev == new {
		return
	}
	log.Printf("[INFO] Load shedding level changed: %s (%d) → %s (%d)", prev, prev, new, new)

	// Level 3 enter/exit: push rate config to nodes.
	if new == LevelHeavy && prev < LevelHeavy {
		// Entering Level 3.
		s.level3Active.Store(true)
		if s.ratePush != nil {
			s.ratePush(level3RateCapHz)
		}
	} else if prev == LevelHeavy && new < LevelHeavy {
		// Exiting Level 3: restore previous rate.
		s.level3Active.Store(false)
		if s.ratePush != nil {
			prevRate := int(s.prevRateHz.Load())
			if prevRate <= 0 {
				prevRate = 20 // sensible default
			}
			s.ratePush(prevRate)
		}
	}
}

// rollingAvg computes the average iteration duration over the rolling window.
func (s *Shedder) rollingAvg() time.Duration {
	n := s.durationsFilled
	if n == 0 {
		return 0
	}
	var sum time.Duration
	for i := 0; i < n; i++ {
		sum += s.durations[i]
	}
	return sum / time.Duration(n)
}

// RollingAvg returns the current rolling average iteration time (for diagnostics).
func (s *Shedder) RollingAvg() time.Duration {
	return s.rollingAvg()
}
