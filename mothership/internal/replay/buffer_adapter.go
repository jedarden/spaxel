// Package replay provides time-travel debugging capabilities for CSI data.
//
// This file implements an adapter that wraps recording.Buffer to implement
// the RecordingStore interface used by the replay worker.
package replay

import (
	"time"

	"github.com/spaxel/mothership/internal/recording"
)

// BufferAdapter wraps a recording.Buffer to implement the RecordingStore interface.
// This allows the replay worker to read from the same CSI recording buffer
// that the ingestion server writes to.
type BufferAdapter struct {
	buf *recording.Buffer
}

// NewBufferAdapter creates a new adapter from a recording.Buffer.
func NewBufferAdapter(buf *recording.Buffer) *BufferAdapter {
	return &BufferAdapter{buf: buf}
}

// Stats returns statistics about the recording buffer.
func (a *BufferAdapter) Stats() Stats {
	stats := a.buf.Stats()
	return Stats{
		HasData:   stats.HasData,
		WritePos:  stats.WritePos,
		OldestPos: stats.OldestPos,
		FileSize:  stats.FileSize,
	}
}

// Scan reads all stored CSI frames from oldest to newest.
// The recording.Buffer's Scan method signature matches what we need.
func (a *BufferAdapter) Scan(fn func(recvTimeNS int64, frame []byte) bool) error {
	return a.buf.Scan(fn)
}

// ScanRange reads records whose recvTimeNS falls within [fromNS, toNS] (inclusive).
// Records are delivered oldest-first. Returning false from fn stops the scan early.
func (a *BufferAdapter) ScanRange(fromNS, toNS int64, fn func(recvTimeNS int64, frame []byte) bool) error {
	from := time.Unix(0, fromNS)
	to := time.Unix(0, toNS)
	return a.buf.ScanRange(from, to, fn)
}

// Close closes the underlying recording buffer.
func (a *BufferAdapter) Close() error {
	return a.buf.Close()
}
