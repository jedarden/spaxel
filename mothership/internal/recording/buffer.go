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
	"fmt"
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

	// Record type markers
	recordTypeUncompressed = 0
	recordTypeCompressed   = 1
)

// Buffer is a disk-backed circular buffer for raw CSI frames with time-based
// retention. It is safe for concurrent use.
type Buffer struct {
	mu           sync.Mutex
	f            *os.File
	fileSize     int64
	writePos     int64
	oldestPos    int64
	wrapPos      int64
	retention    time.Duration
	compression  bool
	compressor   *Compressor
	chunkSize    int
	pendingChunk []byte
}

// NewBuffer opens or creates a recording buffer at path.
// maxMB is the data capacity; pass 0 to use DefaultMaxMB.
// retention is the time-based retention period; pass 0 to use DefaultRetention.
// enableCompression enables zstd compression; pass false for uncompressed mode.
// chunkSize is the target size for compressed chunks; pass 0 to use DefaultChunkSize.
// The SPAXEL_RECORDING_RETENTION environment variable overrides the retention
// parameter when set.
func NewBuffer(path string, maxMB int, retention time.Duration, enableCompression bool, chunkSize int) (*Buffer, error) {
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
		f:           f,
		fileSize:    fileSize,
		retention:   retention,
		compression: enableCompression,
		chunkSize:   chunkSize,
	}

	// Initialize compressor if compression is enabled
	if enableCompression {
		var err error
		b.compressor, err = NewCompressor(chunkSize)
		if err != nil {
			f.Close() //nolint:errcheck
			return nil, fmt.Errorf("create compressor: %w", err)
		}
	}

	info, err := f.Stat()
	if err != nil {
		f.Close() //nolint:errcheck
		return nil, err
	}

	if info.Size() >= headerSize {
		if herr := b.readHeader(); herr == nil && b.headerValid() {
			if info.Size() < fileSize {
				if terr := f.Truncate(fileSize); terr != nil {
					f.Close() //nolint:errcheck
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
		f.Close() //nolint:errcheck
		return nil, err
	}
	if err := b.syncHeader(); err != nil {
		f.Close() //nolint:errcheck
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

	b.mu.Lock()
	defer b.mu.Unlock()

	// Prune time-expired records before writing.
	cutoff := recvTimeNS - b.retention.Nanoseconds()
	if err := b.pruneOlderThan(cutoff); err != nil {
		return err
	}

	if b.compression && b.compressor != nil {
		return b.appendCompressed(recvTimeNS, rawFrame)
	}
	return b.appendUncompressed(recvTimeNS, rawFrame)
}

// appendUncompressed writes a frame in the legacy uncompressed format.
// Must be called with b.mu held.
func (b *Buffer) appendUncompressed(recvTimeNS int64, rawFrame []byte) error {
	frameLen := int64(len(rawFrame))
	recordSize := recordOverhead + frameLen
	if recordSize > b.fileSize-headerSize {
		return errors.New("recording: buffer too small for record")
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

// appendCompressed writes a frame using chunked compression.
// Frames are batched into chunks and compressed when the chunk size is reached.
// Must be called with b.mu held.
func (b *Buffer) appendCompressed(recvTimeNS int64, rawFrame []byte) error {
	// Add frame to compressor with timestamp
	chunk, err := b.compressor.AddFrame(recvTimeNS, rawFrame)
	if err != nil {
		return fmt.Errorf("compress frame: %w", err)
	}

	// If chunk is ready, write it
	if chunk != nil {
		return b.writeCompressedChunk(chunk)
	}

	return nil
}

// writeCompressedChunk writes a compressed chunk to the buffer.
// Must be called with b.mu held.
func (b *Buffer) writeCompressedChunk(chunk []byte) error {
	recordSize := int64(len(chunk))
	if recordSize > b.fileSize-headerSize {
		return errors.New("recording: compressed chunk too large")
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

	if _, err := b.f.WriteAt(chunk, b.writePos); err != nil {
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

// SeekToTimestamp finds the CSI frame closest to the target timestamp.
// Returns the frame data and its exact timestamp, or an error if no data is available.
// This is optimized for replay seeking with O(n) scan where n is the number of frames
// between oldest and target. For a 1-hour segment at 50 Hz, this is at most 180,000 frames.
func (b *Buffer) SeekToTimestamp(target time.Time) ([]byte, int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	targetNS := target.UnixNano()

	if !b.hasData() {
		return nil, 0, errors.New("recording: no data available")
	}

	// Find oldest timestamp
	oldestNS, err := b.oldestTimestamp()
	if err != nil {
		return nil, 0, err
	}

	// If target is before oldest data, return oldest frame
	if targetNS < oldestNS {
		frame, ts, err := b.readFrameAt(b.oldestPos)
		if err != nil {
			return nil, 0, err
		}
		return frame, ts, nil
	}

	// Scan for frame closest to target
	var closestFrame []byte
	var closestTimeNS int64
	minDiff := int64(1 << 62) // Very large value

	found := b.scan(func(recvTimeNS int64, frame []byte) bool {
		diff := recvTimeNS - targetNS
		if diff < 0 {
			diff = -diff
		}

		if diff < minDiff {
			minDiff = diff
			closestFrame = frame
			closestTimeNS = recvTimeNS
		}

		// Stop if we've passed the target and are moving away
		if recvTimeNS > targetNS && minDiff < 100_000_000 { // Within 100ms
			return false
		}

		return true
	})

	if found != nil {
		return nil, 0, found
	}

	if closestFrame == nil {
		return nil, 0, errors.New("recording: no frame found")
	}

	return closestFrame, closestTimeNS, nil
}

// readFrameAt reads the frame at the specified file position.
// Must be called with b.mu held.
func (b *Buffer) readFrameAt(pos int64) ([]byte, int64, error) {
	var hdr [10]byte
	if _, err := b.f.ReadAt(hdr[:], pos); err != nil {
		return nil, 0, err
	}

	recvTimeNS := int64(binary.LittleEndian.Uint64(hdr[0:8]))
	frameLen := int64(binary.LittleEndian.Uint16(hdr[8:10]))
	if frameLen > maxFrameBytes {
		return nil, 0, errors.New("recording: corrupt record")
	}

	frame := make([]byte, frameLen)
	if _, err := b.f.ReadAt(frame, pos+recordOverhead); err != nil {
		return nil, 0, err
	}

	return frame, recvTimeNS, nil
}

// GetTimestampRange returns the oldest and newest timestamps in the buffer.
// Useful for determining the valid replay range.
func (b *Buffer) GetTimestampRange() (oldest, newest time.Time, err error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.hasData() {
		return time.Time{}, time.Time{}, errors.New("recording: no data available")
	}

	oldestNS, err := b.oldestTimestamp()
	if err != nil {
		return time.Time{}, time.Time{}, err
	}

	// Scan to find newest timestamp
	var newestNS int64
	b.scan(func(recvTimeNS int64, frame []byte) bool {
		newestNS = recvTimeNS
		return true // Continue to find absolute newest
	})

	return time.Unix(0, oldestNS), time.Unix(0, newestNS), nil
}

// Close closes the underlying file and releases compression resources.
// Any pending compressed data is flushed before closing.
func (b *Buffer) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Flush any pending compressed data
	if b.compressor != nil {
		finalChunk, err := b.compressor.Flush()
		if err != nil {
			b.compressor.Close()
			b.f.Close()
			return fmt.Errorf("flush compressor: %w", err)
		}
		if finalChunk != nil {
			if err := b.writeCompressedChunk(finalChunk); err != nil {
				b.compressor.Close()
				b.f.Close()
				return fmt.Errorf("write final compressed chunk: %w", err)
			}
		}
		b.compressor.Close()
	}

	return b.f.Close()
}

// FlushCompressed flushes any pending compressed data to disk.
// This is called automatically on Close, but can be called explicitly
// to ensure data is durability.
func (b *Buffer) FlushCompressed() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.compressor == nil {
		return nil // No compression enabled
	}

	chunk, err := b.compressor.Flush()
	if err != nil {
		return fmt.Errorf("flush compressor: %w", err)
	}
	if chunk != nil {
		return b.writeCompressedChunk(chunk)
	}
	return nil
}

// CompressionEnabled returns true if compression is enabled for this buffer.
func (b *Buffer) CompressionEnabled() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.compression
}

// EffectiveRetention estimates the actual retention period achievable with
// the current disk budget, based on recent data compression ratio.
// Returns 0 if insufficient data for estimation.
func (b *Buffer) EffectiveRetention() time.Duration {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.compression || !b.hasData() {
		// No compression or no data - use configured retention
		return b.retention
	}

	// Scan data to estimate compression ratio
	var uncompressedFrames int64
	var totalCompressedBytes int64
	var totalUncompressedBytes int64

	pos := b.oldestPos
	iteration := 0
	maxSamples := 10000 // Sample up to 10000 frames for estimate

	for pos < b.writePos && iteration < 100000 && uncompressedFrames < int64(maxSamples) {
		iteration++

		// Peek at record type
		if pos+2 > b.writePos {
			break
		}
		var lenBuf [2]byte
		if _, err := b.f.ReadAt(lenBuf[:], pos); err != nil {
			break
		}

		// Check if this might be a compressed chunk
		dataLen := int64(binary.LittleEndian.Uint16(lenBuf[:]))
		chunkLen := 2 + dataLen

		// Verify it's actually a compressed chunk by checking the magic
		isCompressed := false
		if chunkLen >= 18 && pos+chunkLen <= b.writePos {
			var magicBuf [4]byte
			if _, err := b.f.ReadAt(magicBuf[:], pos+2); err == nil {
				if string(magicBuf[:]) == chunkMagic {
					isCompressed = true
				}
			}
		}

		if isCompressed {
			// Compressed chunk
			chunkLen, err := b.getCompressedChunkSize(pos)
			if err != nil {
				break
			}
			if chunkLen <= 0 {
				break
			}

			// Read chunk to get frame count and uncompressed size
			chunkData := make([]byte, chunkLen)
			if _, err := b.f.ReadAt(chunkData, pos); err != nil {
				break
			}

			// Parse header (skip 2-byte length prefix)
			if len(chunkData) >= 18 {
				frameCount := int64(binary.LittleEndian.Uint16(chunkData[7:9]))
				uncompLen := int64(binary.LittleEndian.Uint32(chunkData[9:13]))
				uncompressedFrames += int64(frameCount)
				totalUncompressedBytes += int64(uncompLen)
				totalCompressedBytes += int64(chunkLen)
			}

			// Advance past chunk
			nextPos := pos + chunkLen
			if b.wrapPos != 0 && nextPos >= b.wrapPos {
				nextPos = headerSize
			}
			pos = nextPos
		} else {
			// Uncompressed record
			if pos+10 > b.writePos {
				break
			}
			var hdr [10]byte
			if _, err := b.f.ReadAt(hdr[:], pos); err != nil {
				break
			}
			frameLen := int64(binary.LittleEndian.Uint16(hdr[8:10]))
			uncompressedFrames++
			totalUncompressedBytes += frameLen + recordOverhead
			totalCompressedBytes += frameLen + recordOverhead

			nextPos := pos + recordOverhead + frameLen
			if b.wrapPos != 0 && nextPos >= b.wrapPos {
				nextPos = headerSize
			}
			pos = nextPos
		}
	}

	if uncompressedFrames == 0 {
		return b.retention
	}

	// Calculate compression ratio
	ratio := float64(totalCompressedBytes) / float64(totalUncompressedBytes)
	if ratio > 0 && ratio < 1 {
		// Compression working - extend retention proportionally
		// If we achieve 8:1 compression (ratio=0.125), we get 8x the retention
		extended := time.Duration(float64(b.retention) / ratio)
		return extended
	}

	// Fallback to configured retention
	return b.retention
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
	iteration := 0
	for {
		if pos >= b.writePos {
			break
		}

		iteration++
		// Safety check to prevent infinite loop
		if iteration > 100000 {
			return errors.New("scan: too many iterations - possible infinite loop")
		}

		// Peek at the record to determine type
		// Try to read a compressed chunk header first
		var lenBuf [2]byte
		_, err := b.f.ReadAt(lenBuf[:], pos)
		if err != nil {
			return err
		}

		// Check if this might be a compressed chunk
		// Format: [2-byte length prefix][16-byte header: "CSCZ"...][compressed data]
		dataLen := int64(binary.LittleEndian.Uint16(lenBuf[:]))
		chunkLen := 2 + dataLen

		// Verify it's actually a compressed chunk by checking the magic
		isCompressed := false
		if chunkLen >= 18 && pos+chunkLen <= b.writePos {
			// Read the 4-byte magic marker (offset 2, after the 2-byte prefix)
			var magicBuf [4]byte
			if _, err := b.f.ReadAt(magicBuf[:], pos+2); err == nil {
				if string(magicBuf[:]) == chunkMagic {
					isCompressed = true
				}
			}
		}

		if isCompressed {
			// Get chunk size first so we can advance after scanning
			chunkLen, err := b.getCompressedChunkSize(pos)
			if err != nil {
				return err
			}
			if chunkLen <= 0 {
				return fmt.Errorf("scan: invalid chunkLen=%d at pos %d", chunkLen, pos)
			}
			if !b.scanCompressedChunk(pos, fn) {
				break
			}
			// Advance past compressed chunk
			nextPos := pos + chunkLen
			if b.wrapPos != 0 && nextPos >= b.wrapPos {
				nextPos = headerSize
			}
			pos = nextPos
			continue
		}

		// Uncompressed record
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

// scanCompressedChunk scans a compressed chunk and calls fn for each frame.
// Returns false if fn returns false (stop scan early), true otherwise.
// Must be called with b.mu held.
func (b *Buffer) scanCompressedChunk(pos int64, fn func(recvTimeNS int64, frame []byte) bool) bool {
	// Read the chunk header to get size
	chunkLen, err := b.getCompressedChunkSize(pos)
	if err != nil {
		return true
	}

	// Check if we have enough data to read this chunk
	if pos+chunkLen > b.writePos {
		// Chunk would read past writePos - incomplete write, stop scan
		return true
	}

	// Read entire chunk
	chunkData := make([]byte, chunkLen)
	if _, err := b.f.ReadAt(chunkData, pos); err != nil {
		return true
	}

	// Skip the 2-byte length prefix - DecompressChunk expects data to start with header
	chunkDataWithoutPrefix := chunkData[2:]

	// Decompress and iterate frames
	decomp, err := NewDecompressor()
	if err != nil {
		return true
	}
	defer decomp.Close()

	stopped := false
	err = decomp.DecompressChunk(chunkDataWithoutPrefix, func(recvTimeNS int64, frame []byte) error {
		if !fn(recvTimeNS, frame) {
			stopped = true
			return errors.New("scan stopped")
		}
		return nil
	})
	if err != nil {
		// Decompression error - stop scan
		return true
	}

	// Return false if scan stopped early
	if stopped {
		return false
	}
	return true
}

// getCompressedChunkSize returns the total size of a compressed chunk record.
// Must be called with b.mu held.
func (b *Buffer) getCompressedChunkSize(pos int64) (int64, error) {
	// Check if we have at least 2 bytes to read
	if pos+2 > b.writePos {
		return 0, errors.New("not enough data for chunk length")
	}

	// Read chunk length prefix (first 2 bytes)
	var lenBuf [2]byte
	if _, err := b.f.ReadAt(lenBuf[:], pos); err != nil {
		return 0, err
	}
	dataLen := int64(binary.LittleEndian.Uint16(lenBuf[:])) // Length of header+data (not including prefix)
	chunkLen := 2 + dataLen                                  // Total chunk length including prefix

	// Verify it's a compressed chunk magic
	if chunkLen >= 18 && pos+chunkLen <= b.writePos { // At least 2+16 bytes
		magicBuf := make([]byte, 4)
		if _, err := b.f.ReadAt(magicBuf, pos+2); err != nil {
			return 0, err
		}
		if string(magicBuf) == "CSCZ" {
			return chunkLen, nil
		}
	}

	// Not a compressed chunk, return error
	return 0, errors.New("not a compressed chunk")
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
