// Package diskspace provides runtime disk-space monitoring for the /data filesystem.
// It implements the plan's Disk Full Handling spec: at <100 MB free, stop CSI replay
// buffer writes and emit a system alert; at <20 MB free, also pause crowd flow
// accumulation and prediction model updates.
package diskspace

import (
	"context"
	"log"
	"sync"
	"syscall"
	"time"

	"github.com/spaxel/mothership/internal/eventbus"
)

const (
	// WarningThresholdMB is the free space threshold for warnings (100 MB).
	WarningThresholdMB = 100

	// CriticalThresholdMB is the free space threshold for critical mode (20 MB).
	CriticalThresholdMB = 20

	// DefaultCheckInterval is how often to poll disk space.
	DefaultCheckInterval = 60 * time.Second
)

// WritePauser is an interface for components that can pause/resume writes.
type WritePauser interface {
	PauseWrites()
	ResumeWrites()
}

// UpdatePauser is an interface for components that can pause/resume updates.
type UpdatePauser interface {
	PauseUpdates()
	ResumeUpdates()
}

// State represents the current disk-space state.
type State int

const (
	StateNormal State = iota
	StateWarning
	StateCritical
)

func (s State) String() string {
	switch s {
	case StateNormal:
		return "normal"
	case StateWarning:
		return "warning"
	case StateCritical:
		return "critical"
	}
	return "unknown"
}

// Monitor polls disk space and coordinates pausing writes when low.
type Monitor struct {
	mu       sync.RWMutex
	dataDir  string
	interval time.Duration

	// Components to control
	recorder       WritePauser  // CSI replay buffer
	flowAccumulator WritePauser // Crowd flow accumulation
	predictor      UpdatePauser // Prediction model updates

	// State
	state      State
	freeMB     uint64
	lastCheck  time.Time

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Config holds monitor configuration.
type Config struct {
	DataDir         string        // Path to /data filesystem
	CheckInterval   time.Duration // Poll interval (default: 60s)
	Recorder        WritePauser   // CSI replay buffer manager
	FlowAccumulator WritePauser   // Crowd flow accumulator
	Predictor       UpdatePauser  // Prediction predictor
}

// New creates a new disk-space monitor.
func New(cfg Config) *Monitor {
	interval := cfg.CheckInterval
	if interval <= 0 {
		interval = DefaultCheckInterval
	}

	ctx, cancel := context.WithCancel(context.Background())

	m := &Monitor{
		dataDir:         cfg.DataDir,
		interval:        interval,
		recorder:        cfg.Recorder,
		flowAccumulator: cfg.FlowAccumulator,
		predictor:       cfg.Predictor,
		state:           StateNormal,
		ctx:             ctx,
		cancel:          cancel,
	}

	// Initial check
	m.checkDiskSpace()

	return m
}

// Start begins the disk-space monitoring goroutine.
func (m *Monitor) Start() {
	m.wg.Add(1)
	go m.run()
}

// Stop gracefully stops the monitor.
func (m *Monitor) Stop() {
	m.cancel()
	m.wg.Wait()
}

// run is the monitor goroutine.
func (m *Monitor) run() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.checkDiskSpace()
		case <-m.ctx.Done():
			return
		}
	}
}

// checkDiskSpace polls disk space and updates state.
func (m *Monitor) checkDiskSpace() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lastCheck = time.Now()

	var stat syscall.Statfs_t
	if err := syscall.Statfs(m.dataDir, &stat); err != nil {
		log.Printf("[ERROR] diskspace: statfs failed: %v", err)
		return
	}

	// Calculate free space in MB: (Bavail * Frsize) / (1024 * 1024)
	freeBytes := stat.Bavail * uint64(stat.Frsize)
	freeMB := freeBytes / (1024 * 1024)
	m.freeMB = freeMB

	oldState := m.state

	// Determine new state
	if freeMB < CriticalThresholdMB {
		m.state = StateCritical
	} else if freeMB < WarningThresholdMB {
		m.state = StateWarning
	} else {
		m.state = StateNormal
	}

	// Log state transition
	if oldState != m.state {
		log.Printf("[INFO] diskspace: state transition %s -> %s (%d MB free)", oldState, m.state, freeMB)
	}

	// Apply state changes
	switch m.state {
	case StateCritical:
		m.enterCriticalState(oldState)
	case StateWarning:
		m.enterWarningState(oldState)
	case StateNormal:
		m.enterNormalState(oldState)
	}

	// Emit event to timeline
	m.emitEvent()
}

// enterCriticalState applies critical mode actions.
func (m *Monitor) enterCriticalState(oldState State) {
	// Pause CSI replay buffer writes
	if m.recorder != nil {
		if oldState != StateCritical {
			log.Printf("[WARN] diskspace: Pausing CSI replay buffer writes (disk critically low: %d MB)", m.freeMB)
			m.recorder.PauseWrites()
		}
	}

	// Pause crowd flow accumulation writes
	if m.flowAccumulator != nil {
		if oldState != StateCritical {
			log.Printf("[WARN] diskspace: Pausing crowd flow accumulation writes (disk critically low: %d MB)", m.freeMB)
			m.flowAccumulator.PauseWrites()
		}
	}

	// Pause prediction model updates
	if m.predictor != nil {
		if oldState != StateCritical {
			log.Printf("[WARN] diskspace: Pausing prediction model updates (disk critically low: %d MB)", m.freeMB)
			m.predictor.PauseUpdates()
		}
	}
}

// enterWarningState applies warning mode actions.
func (m *Monitor) enterWarningState(oldState State) {
	// Pause CSI replay buffer writes
	if m.recorder != nil {
		if oldState == StateNormal {
			log.Printf("[WARN] diskspace: Pausing CSI replay buffer writes (disk low: %d MB)", m.freeMB)
			m.recorder.PauseWrites()
		}
	}

	// In warning mode, crowd flow and prediction continue
	// Resume them if we're coming from critical state
	if oldState == StateCritical {
		if m.flowAccumulator != nil {
			log.Printf("[INFO] diskspace: Resuming crowd flow accumulation writes")
			m.flowAccumulator.ResumeWrites()
		}
		if m.predictor != nil {
			log.Printf("[INFO] diskspace: Resuming prediction model updates")
			m.predictor.ResumeUpdates()
		}
	}
}

// enterNormalState applies normal mode actions.
func (m *Monitor) enterNormalState(oldState State) {
	// Resume all writes
	if oldState != StateNormal {
		if m.recorder != nil {
			log.Printf("[INFO] diskspace: Resuming CSI replay buffer writes")
			m.recorder.ResumeWrites()
		}
		if m.flowAccumulator != nil {
			log.Printf("[INFO] diskspace: Resuming crowd flow accumulation writes")
			m.flowAccumulator.ResumeWrites()
		}
		if m.predictor != nil {
			log.Printf("[INFO] diskspace: Resuming prediction model updates")
			m.predictor.ResumeUpdates()
		}
	}
}

// emitEvent publishes a disk_space event to the timeline.
func (m *Monitor) emitEvent() {
	detail := map[string]interface{}{
		"state":       m.state.String(),
		"free_mb":     m.freeMB,
		"warning_mb":  WarningThresholdMB,
		"critical_mb": CriticalThresholdMB,
	}

	severity := eventbus.SeverityInfo
	if m.state == StateWarning {
		severity = eventbus.SeverityWarning
	} else if m.state == StateCritical {
		severity = eventbus.SeverityCritical
	}

	eventbus.PublishDefault(eventbus.Event{
		Type:        "disk_space",
		TimestampMs: time.Now().UnixNano() / 1e6,
		Severity:    severity,
		Detail:      detail,
	})
}

// GetState returns the current monitor state.
func (m *Monitor) GetState() (state State, freeMB uint64, lastCheck time.Time) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state, m.freeMB, m.lastCheck
}

// Stats returns disk-space statistics for the dashboard.
type Stats struct {
	State       State    `json:"state"`
	FreeMB      uint64   `json:"free_mb"`
	WarningMB   uint64   `json:"warning_mb"`
	CriticalMB  uint64   `json:"critical_mb"`
	LastCheck   time.Time `json:"last_check"`
}

// GetStats returns current statistics for dashboard display.
func (m *Monitor) GetStats() Stats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return Stats{
		State:      m.state,
		FreeMB:     m.freeMB,
		WarningMB:  WarningThresholdMB,
		CriticalMB: CriticalThresholdMB,
		LastCheck:  m.lastCheck,
	}
}
