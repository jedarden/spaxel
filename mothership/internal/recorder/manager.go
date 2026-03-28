package recorder

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	defaultRetentionHours  = 48
	defaultMaxBytesPerLink = int64(1 << 30) // 1 GB
	defaultBufferSize      = 1000
	defaultCleanupInterval = time.Hour
)

// Config holds recorder configuration.
type Config struct {
	DataDir         string        // Base directory for segment files
	RetentionHours  int           // Hours to retain segment files (default: 48)
	MaxBytesPerLink int64         // Max bytes per link as secondary guard (default: 1 GB)
	BufferSize      int           // Per-link buffered channel capacity (default: 1000)
	CleanupInterval time.Duration // Cleanup sweep interval (default: 1 hour)
}

// DefaultConfig returns a config with sensible defaults.
func DefaultConfig(dataDir string) Config {
	return Config{
		DataDir:         dataDir,
		RetentionHours:  defaultRetentionHours,
		MaxBytesPerLink: defaultMaxBytesPerLink,
		BufferSize:      defaultBufferSize,
		CleanupInterval: defaultCleanupInterval,
	}
}

// Manager manages per-link CSI frame recorders.
// It is safe for concurrent use.
type Manager struct {
	mu     sync.RWMutex
	config Config
	links  map[string]*linkRecorder
	done   chan struct{}
	wg     sync.WaitGroup
}

type linkRecorder struct {
	ch     chan writeReq
	linkID string
	dir    string
}

type writeReq struct {
	recvTimeNS int64
	frame      []byte
}

// NewManager creates a new recorder manager and starts the cleanup goroutine.
func NewManager(cfg Config) (*Manager, error) {
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("recorder: create data dir: %w", err)
	}
	if cfg.RetentionHours <= 0 {
		cfg.RetentionHours = defaultRetentionHours
	}
	if cfg.MaxBytesPerLink <= 0 {
		cfg.MaxBytesPerLink = defaultMaxBytesPerLink
	}
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = defaultBufferSize
	}
	if cfg.CleanupInterval <= 0 {
		cfg.CleanupInterval = defaultCleanupInterval
	}

	m := &Manager{
		config: cfg,
		links:  make(map[string]*linkRecorder),
		done:   make(chan struct{}),
	}

	m.wg.Add(1)
	go m.cleanupLoop()

	return m, nil
}

// Write writes a raw CSI frame for the given link.
// It does not block the caller. If the per-link buffer is full,
// the frame is dropped with a log warning.
func (m *Manager) Write(linkID string, frame []byte) {
	select {
	case <-m.done:
		return
	default:
	}

	lr := m.getOrCreateLink(linkID)

	req := writeReq{
		recvTimeNS: time.Now().UnixNano(),
		frame:      frame,
	}

	select {
	case lr.ch <- req:
	default:
		log.Printf("[WARN] Recorder buffer full for link %s, dropping frame", linkID)
	}
}

// ReadFrom returns a channel that yields raw CSI frames for the given link
// from the specified time onwards, in chronological order.
// The channel is closed when all historical frames have been sent.
func (m *Manager) ReadFrom(linkID string, since time.Time) <-chan []byte {
	ch := make(chan []byte, 100)

	go func() {
		defer close(ch)

		dir := filepath.Join(m.config.DataDir, linkDir(linkID))
		files, err := listSegmentFiles(dir)
		if err != nil {
			log.Printf("[WARN] Recorder ReadFrom: list segments for %s: %v", linkID, err)
			return
		}

		sinceNS := since.UnixNano()
		for _, f := range files {
			if err := ScanSegmentFrom(f, sinceNS, func(_ int64, frame []byte) bool {
				select {
				case ch <- frame:
				case <-m.done:
					return false
				}
				return true
			}); err != nil {
				log.Printf("[WARN] Recorder ReadFrom: read segment %s: %v", f, err)
				return
			}
		}
	}()

	return ch
}

// AvailableRange returns the time range of available frames for a link.
// Returns the oldest and newest frame timestamps.
func (m *Manager) AvailableRange(linkID string) (start, end time.Time, err error) {
	dir := filepath.Join(m.config.DataDir, linkDir(linkID))
	files, err := listSegmentFiles(dir)
	if err != nil || len(files) == 0 {
		return time.Time{}, time.Time{}, fmt.Errorf("recorder: no data for link %s", linkID)
	}

	var firstNS, lastNS int64
	foundFirst, foundLast := false, false

	if err := ScanSegment(files[0], func(ns int64, _ []byte) bool {
		firstNS = ns
		foundFirst = true
		return false
	}); err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("recorder: read first segment: %w", err)
	}

	if err := ScanSegment(files[len(files)-1], func(ns int64, _ []byte) bool {
		lastNS = ns
		foundLast = true
		return true
	}); err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("recorder: read last segment: %w", err)
	}

	if !foundFirst || !foundLast {
		return time.Time{}, time.Time{}, fmt.Errorf("recorder: no data for link %s", linkID)
	}

	return time.Unix(0, firstNS), time.Unix(0, lastNS), nil
}

// Close gracefully shuts down the manager, flushing all pending writes
// and stopping the cleanup goroutine.
func (m *Manager) Close() {
	close(m.done)

	m.mu.Lock()
	for _, lr := range m.links {
		close(lr.ch)
	}
	m.mu.Unlock()

	m.wg.Wait()
}

func (m *Manager) getOrCreateLink(linkID string) *linkRecorder {
	m.mu.RLock()
	lr, ok := m.links[linkID]
	m.mu.RUnlock()
	if ok {
		return lr
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if lr, ok = m.links[linkID]; ok {
		return lr
	}

	dir := filepath.Join(m.config.DataDir, linkDir(linkID))
	lr = &linkRecorder{
		ch:     make(chan writeReq, m.config.BufferSize),
		linkID: linkID,
		dir:    dir,
	}
	m.links[linkID] = lr

	m.wg.Add(1)
	go m.linkWriter(lr)

	return lr
}

func (m *Manager) linkWriter(lr *linkRecorder) {
	defer m.wg.Done()

	var writer *os.File
	var curHour time.Time

	flush := func() {
		if writer != nil {
			writer.Sync()
			writer.Close()
			writer = nil
		}
	}
	defer flush()

	for req := range lr.ch {
		t := time.Unix(0, req.recvTimeNS)
		hr := segmentHour(t)

		if writer == nil || hr != curHour {
			flush()
			curHour = hr

			if err := os.MkdirAll(lr.dir, 0755); err != nil {
				log.Printf("[ERROR] Recorder mkdir %s: %v", lr.dir, err)
				continue
			}

			path := filepath.Join(lr.dir, segmentFileName(hr))
			var err error
			writer, err = os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
			if err != nil {
				log.Printf("[ERROR] Recorder open %s: %v", path, err)
				continue
			}
		}

		if err := WriteRecord(writer, req.recvTimeNS, req.frame); err != nil {
			log.Printf("[ERROR] Recorder write to %s: %v", filepath.Join(lr.dir, segmentFileName(curHour)), err)
		}
	}
}

func (m *Manager) cleanupLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.CleanupInterval)
	defer ticker.Stop()

	m.cleanup()

	for {
		select {
		case <-ticker.C:
			m.cleanup()
		case <-m.done:
			return
		}
	}
}

func (m *Manager) cleanup() {
	cutoff := time.Now().UTC().Add(-time.Duration(m.config.RetentionHours) * time.Hour)

	entries, err := os.ReadDir(m.config.DataDir)
	if err != nil {
		return
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		linkDirPath := filepath.Join(m.config.DataDir, e.Name())

		files, err := listSegmentFiles(linkDirPath)
		if err != nil {
			continue
		}

		// Delete segment files older than retention period.
		for _, f := range files {
			name := filepath.Base(f)
			st, err := parseSegmentTime(name)
			if err != nil {
				continue
			}
			if st.Before(cutoff) {
				os.Remove(f)
			}
		}

		// Enforce MaxBytesPerLink: delete oldest files until under limit.
		files, err = listSegmentFiles(linkDirPath)
		if err != nil {
			continue
		}

		var totalSize int64
		fileSizes := make(map[string]int64, len(files))
		for _, f := range files {
			info, err := os.Stat(f)
			if err != nil {
				continue
			}
			fileSizes[f] = info.Size()
			totalSize += info.Size()
		}

		for _, f := range files {
			if totalSize <= m.config.MaxBytesPerLink {
				break
			}
			sz := fileSizes[f]
			if err := os.Remove(f); err == nil {
				totalSize -= sz
			}
		}
	}
}
