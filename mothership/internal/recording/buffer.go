// Package recording implements a disk-backed circular buffer for CSI frame recording.
//
// Binary file layout:
//
//	Header (32 bytes):
//	  magic[8]      "SPAXLREC"
//	  writePos[8]   uint64 LE — absolute file offset of next write
//	  oldestPos[8]  uint64 LE — absolute file offset of oldest valid record (0 = empty)
//	  wrapPos[8]    uint64 LE — writePos at last wrap point (0 = no pending wrap)
//
//	Record (10 + frameLen bytes):
//	  recvTimeNS[8]  int64 LE  — Unix nanosecond receive timestamp
//	  frameLen[2]    uint16 LE — length of following frame bytes
//	  frameData[N]   raw CSI frame bytes (same format as WebSocket)
//
// Records are stored in chronological order. The buffer evicts the oldest
// records when either (a) the write pointer runs out of space or (b) records
// are older than the configured retention period.
//
// The retention period is configurable via the SPAXEL_RECORDING_RETENTION
// environment variable (e.g. "24h", "72h"). This is the foundation for the
// Phase 8 time-travel replay feature.
package recording

import (
	"encoding/binary"
	"errors"
	"os"
	"sync"
	"time"
)

const (
	fileMagic      = "SPAXLREC"
	headerSize     = int64(32)
	recordOverhead = int64(10)  // recvTimeNS(8) + frameLen(2)
	maxFrameBytes  = int64(280) // 24-byte header + 128*2 payload

	// DefaultRetention is the default time-based retention period.
	DefaultRetention = 48 * time.Hour

	// DefaultMaxMB is the default recording buffer capacity in megabytes.
	// Acts as a hard cap to prevent disk exhaustion; time-based pruning is
	// the primary retention mechanism.
	DefaultMaxMB = 512

	// RetentionEnvVar is the environment variable for configuring retention.
	// Accepts any value parseable by time.ParseDuration (e.g. "24h", "72h").
	RetentionEnvVar = "SPAXEL_RECORDING_RETENTION"
)

// Buffer is a disk-backed circular buffer for raw CSI frames with time-based
// retention. It is safe for concurrent use.
type Buffer struct {
	mu        sync.Mutex
	f         *os.File
	fileSize  int64
	writePos  int64
	oldestPos int64
	wrapPos   int64
	retention time.Duration
}

// NewBuffer opens or creates a recording buffer at path.
// maxMB is the data capacity; pass 0 to use DefaultMaxMB.
// retention is the time-based retention period; pass 0 to use DefaultRetention.
// The SPAXEL_RECORDING_RETENTION environment variable overrides the retention
// parameter when set.
func NewBuffer(path string, maxMB int, retention time.Duration) (*Buffer, error) {
	if maxMB <= 0 {
		maxMB = DefaultMaxMB
	}
	if retention <= 0 {
		retention = DefaultRetention
	}

	// Environment variable takes precedence.
	if envVal := os.Getenv(RetentionEnvVar); envVal != "" {
		if d, err := time.ParseDuration(envVal); err == nil && d > 0 {
			retention = d
		}
	}

	fileSize := headerSize + int64(maxMB)*1024*1024
	if fileSize-headerSize < maxFrameBytes+recordOverhead {
		return nil, errors.New("recording: maxMB too small for a single record")
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}

	b := &Buffer{
		f:         f,
		fileSize:  fileSize,
		retention: retention,
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	if info.Size() >= headerSize {
		if herr := b.readHeader(); herr == nil && b.headerValid() {
			if info.Size() < fileSize {
				if terr := f.Truncate(fileSize); terr != nil {
					f.Close()
					return nil, terr
				}
			}
			return b, nil
		}
	}

	// Fresh buffer.
	b.writePos = headerSize
	b.oldestPos = 0
	b.wrapPos = 0
	if err := f.Truncate(fileSize); err != nil {
		f.Close()
		return nil, err
	}
	if err := b.syncHeader(); err != nil {
		f.Close()
		return nil, err
	}
	return b, nil
}

// Append writes a raw CSI frame to the buffer, then prunes any records older
// than the retention period relative to recvTimeNS.
func (b *Buffer) Append(recvTimeNS int64, rawFrame []byte) error {
	frameLen := int64(len(rawFrame))
	if frameLen > maxFrameBytes {
		return errors.New("recording: frame exceeds maximum size")
	}
	recordSize := recordOverhead + frameLen
	if recordSize > b.fileSize-headerSize {
		return errors.New("recording: buffer too small for record")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Prune time-expired records before writing.
	cutoff := recvTimeNS - b.retention.Nanoseconds()
	if err := b.pruneOlderThan(cutoff); err != nil {
		return err
	}

	// Wrap writePos if the record won't fit before the end of file.
	if b.writePos+recordSize > b.fileSize {
		b.wrapPos = b.writePos
		b.writePos = headerSize
	}

	// Space-evict oldest records that overlap the write window.
	for b.hasData() && b.oldestPos >= b.writePos && b.oldestPos < b.writePos+recordSize {
		if err := b.evictOne(); err != nil {
			return err
		}
	}

	wasEmpty := !b.hasData()

	// Encode and write the record.
	buf := make([]byte, recordSize)
	binary.LittleEndian.PutUint64(buf[0:8], uint64(recvTimeNS))
	binary.LittleEndian.PutUint16(buf[8:10], uint16(frameLen))
	copy(buf[10:], rawFrame)

	if _, err := b.f.WriteAt(buf, b.writePos); err != nil {
		return err
	}

	if wasEmpty {
		b.oldestPos = b.writePos
	}
	b.writePos += recordSize

	return b.syncHeader()
}

// Prune removes all records older than the current retention period relative
// to wall-clock time. This is called automatically on each Append, but can
// also be triggered explicitly (e.g. during idle periods).
func (b *Buffer) Prune() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	cutoff := time.Now().Add(-b.retention).UnixNano()
	if err := b.pruneOlderThan(cutoff); err != nil {
		return err
	}
	return b.syncHeader()
}

// Scan reads all stored records from oldest to newest, calling fn for each.
// fn receives the receive timestamp (Unix nanoseconds) and raw frame bytes.
// Returning false from fn stops the scan early.
// The buffer is held under lock for the duration — callers must not call
// Append or other mutating methods from within fn.
func (b *Buffer) Scan(fn func(recvTimeNS int64, frame []byte) bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.scan(fn)
}

// ScanRange reads records whose recvTimeNS falls within [from, to] (inclusive).
// Records are delivered oldest-first. Returning false from fn stops the scan early.
// Returns an error if from is after to.
func (b *Buffer) ScanRange(from, to time.Time, fn func(recvTimeNS int64, frame []byte) bool) error {
	fromNS := from.UnixNano()
	toNS := to.UnixNano()
	if fromNS > toNS {
		return errors.New("recording: from must be before or equal to to")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	return b.scan(func(recvTimeNS int64, frame []byte) bool {
		if recvTimeNS < fromNS {
			return true // before range; keep scanning
		}
		if recvTimeNS > toNS {
			return false // past range; stop
		}
		return fn(recvTimeNS, frame)
	})
}

// Stats is a snapshot of the buffer's internal state.
type Stats struct {
	HasData   bool
	WritePos  int64
	OldestPos int64
	FileSize  int64
	Retention time.Duration
}

// Stats returns a snapshot of the buffer's internal state.
func (b *Buffer) Stats() Stats {
	b.mu.Lock()
	defer b.mu.Unlock()
	return Stats{
		HasData:   b.hasData(),
		WritePos:  b.writePos,
		OldestPos: b.oldestPos,
		FileSize:  b.fileSize,
		Retention: b.retention,
	}
}

// Retention returns the configured retention period.
func (b *Buffer) Retention() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.retention
}

// Close closes the underlying file.
func (b *Buffer) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.f.Close()
}

// pruneOlderThan evicts records with recvTimeNS < cutoff.
// Must be called with b.mu held.
func (b *Buffer) pruneOlderThan(cutoff int64) error {
	for b.hasData() {
		ts, err := b.oldestTimestamp()
		if err != nil {
			return err
		}
		if ts >= cutoff {
			break
		}
		if err := b.evictOne(); err != nil {
			return err
		}
	}
	return nil
}

// oldestTimestamp reads the recvTimeNS of the oldest record.
// Must be called with b.mu held and hasData() == true.
func (b *Buffer) oldestTimestamp() (int64, error) {
	var buf [8]byte
	if _, err := b.f.ReadAt(buf[:], b.oldestPos); err != nil {
		return 0, err
	}
	return int64(binary.LittleEndian.Uint64(buf[:])), nil
}

// scan iterates records from oldestPos to writePos in chronological order,
// calling fn for each. Must be called with b.mu held.
func (b *Buffer) scan(fn func(recvTimeNS int64, frame []byte) bool) error {
	if !b.hasData() {
		return nil
	}

	pos := b.oldestPos
	for {
		if pos == b.writePos {
			break
		}

		var hdr [10]byte
		if _, err := b.f.ReadAt(hdr[:], pos); err != nil {
			return err
		}
		recvTimeNS := int64(binary.LittleEndian.Uint64(hdr[0:8]))
		frameLen := int64(binary.LittleEndian.Uint16(hdr[8:10]))
		if frameLen > maxFrameBytes {
			return errors.New("recording: corrupt record during scan")
		}

		frame := make([]byte, frameLen)
		if _, err := b.f.ReadAt(frame, pos+recordOverhead); err != nil {
			return err
		}

		if !fn(recvTimeNS, frame) {
			break
		}

		nextPos := pos + recordOverhead + frameLen
		// After consuming the right arc, follow the wrap to data start.
		if b.wrapPos != 0 && nextPos >= b.wrapPos {
			nextPos = headerSize
		}
		pos = nextPos
	}
	return nil
}

// hasData reports whether there are any valid records.
func (b *Buffer) hasData() bool {
	return b.oldestPos != 0
}

// evictOne advances oldestPos past the oldest record.
// Must be called with b.mu held.
func (b *Buffer) evictOne() error {
	if !b.hasData() {
		return nil
	}

	var lenBuf [2]byte
	if _, err := b.f.ReadAt(lenBuf[:], b.oldestPos+8); err != nil {
		return err
	}
	frameLen := int64(binary.LittleEndian.Uint16(lenBuf[:]))
	if frameLen > maxFrameBytes {
		// Corrupted record; reset to recover gracefully.
		b.oldestPos = 0
		b.wrapPos = 0
		return nil
	}

	nextPos := b.oldestPos + recordOverhead + frameLen
	if b.wrapPos != 0 && nextPos >= b.wrapPos {
		nextPos = headerSize
		b.wrapPos = 0
	}

	if nextPos == b.writePos {
		b.oldestPos = 0 // buffer is now empty
	} else {
		b.oldestPos = nextPos
	}
	return nil
}

func (b *Buffer) headerValid() bool {
	return b.writePos >= headerSize && b.writePos <= b.fileSize &&
		(b.oldestPos == 0 || (b.oldestPos >= headerSize && b.oldestPos <= b.fileSize)) &&
		(b.wrapPos == 0 || (b.wrapPos >= headerSize && b.wrapPos <= b.fileSize))
}

func (b *Buffer) readHeader() error {
	var buf [32]byte
	if _, err := b.f.ReadAt(buf[:], 0); err != nil {
		return err
	}
	if string(buf[0:8]) != fileMagic {
		return errors.New("recording: invalid magic")
	}
	b.writePos = int64(binary.LittleEndian.Uint64(buf[8:16]))
	b.oldestPos = int64(binary.LittleEndian.Uint64(buf[16:24]))
	b.wrapPos = int64(binary.LittleEndian.Uint64(buf[24:32]))
	return nil
}

func (b *Buffer) syncHeader() error {
	var buf [32]byte
	copy(buf[0:8], fileMagic)
	binary.LittleEndian.PutUint64(buf[8:16], uint64(b.writePos))
	binary.LittleEndian.PutUint64(buf[16:24], uint64(b.oldestPos))
	binary.LittleEndian.PutUint64(buf[24:32], uint64(b.wrapPos))
	_, err := b.f.WriteAt(buf[:], 0)
	return err
}
