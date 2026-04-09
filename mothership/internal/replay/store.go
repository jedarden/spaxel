// Package replay provides a disk-backed circular buffer for raw CSI frames.
//
// File layout:
//
//	Header (32 bytes):
//	  magic[8]     "SPAXLREP"
//	  writePos[8]  uint64 LE — absolute file offset of next write
//	  oldestPos[8] uint64 LE — absolute file offset of oldest valid record (0 = empty)
//	  wrapPos[8]   uint64 LE — writePos at last wrap point (0 = no pending wrap)
//
//	Record (10 + frameLen bytes):
//	  recvTimeNS[8] int64 LE — Unix nanosecond receive timestamp
//	  frameLen[2]   uint16 LE — length of following frame bytes
//	  frameData[N]  raw CSI frame bytes
//
// Eviction: when the oldest record's successor would reach wrapPos, oldest wraps
// to headerSize. New writes evict oldest records as needed to make room.
package replay

import (
	"encoding/binary"
	"errors"
	"os"
	"sync"
)

const (
	fileMagic      = "SPAXLREP"
	headerSize     = int64(32)  // magic(8) + writePos(8) + oldestPos(8) + wrapPos(8)
	recordOverhead = int64(10)  // recvTimeNS(8) + frameLen(2)
	maxFrameBytes  = int64(280) // per plan: max CSI frame = 24 + 128*2

	// DefaultMaxMB is the default recording buffer capacity in megabytes (~48h at 20 Hz, 20 links).
	DefaultMaxMB = 360
)

// RecordingStore is a disk-backed circular buffer for raw CSI frames.
// It is safe for concurrent use.
type RecordingStore struct {
	mu       sync.Mutex
	f        *os.File
	fileSize int64 // total file size including header
	writePos int64 // absolute file offset of next write
	oldestPos int64 // absolute file offset of oldest valid record (0 = empty)
	wrapPos  int64 // writePos at time of last wrap (0 = no pending wrap)
}

// NewRecordingStore opens or creates a recording store at path.
// maxMB is the data capacity; pass 0 to use DefaultMaxMB.
func NewRecordingStore(path string, maxMB int) (*RecordingStore, error) {
	if maxMB <= 0 {
		maxMB = DefaultMaxMB
	}
	fileSize := headerSize + int64(maxMB)*1024*1024

	if fileSize-headerSize < maxFrameBytes+recordOverhead {
		return nil, errors.New("replay: maxMB too small for a single record")
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}

	s := &RecordingStore{
		f:        f,
		fileSize: fileSize,
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}

	if info.Size() >= headerSize {
		if herr := s.readHeader(); herr == nil && s.headerValid() {
			// Grow file to new size if needed
			if info.Size() < fileSize {
				if terr := f.Truncate(fileSize); terr != nil {
					f.Close()
					return nil, terr
				}
			}
			return s, nil
		}
	}

	// Fresh store
	s.writePos = headerSize
	s.oldestPos = 0
	s.wrapPos = 0
	if err := f.Truncate(fileSize); err != nil {
		f.Close()
		return nil, err
	}
	if err := s.syncHeader(); err != nil {
		f.Close()
		return nil, err
	}
	return s, nil
}

// Append writes a raw CSI frame to the store.
func (s *RecordingStore) Append(recvTimeNS int64, rawFrame []byte) error {
	frameLen := int64(len(rawFrame))
	if frameLen > maxFrameBytes {
		return errors.New("replay: frame exceeds maximum size")
	}
	recordSize := recordOverhead + frameLen
	if recordSize > s.fileSize-headerSize {
		return errors.New("replay: buffer too small for record")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Wrap writePos if record won't fit before end of file.
	if s.writePos+recordSize > s.fileSize {
		s.wrapPos = s.writePos
		s.writePos = headerSize
	}

	// Evict oldest records that fall within [writePos, writePos+recordSize).
	for s.hasData() && s.oldestPos >= s.writePos && s.oldestPos < s.writePos+recordSize {
		if err := s.evictOne(); err != nil {
			return err
		}
	}

	wasEmpty := !s.hasData()

	// Build record.
	buf := make([]byte, recordSize)
	binary.LittleEndian.PutUint64(buf[0:8], uint64(recvTimeNS))
	binary.LittleEndian.PutUint16(buf[8:10], uint16(frameLen))
	copy(buf[10:], rawFrame)

	if _, err := s.f.WriteAt(buf, s.writePos); err != nil {
		return err
	}

	if wasEmpty {
		s.oldestPos = s.writePos
	}
	s.writePos += recordSize

	return s.syncHeader()
}

// Scan reads all stored records from oldest to newest, calling fn for each.
// fn receives the receive timestamp (Unix nanoseconds) and the raw frame bytes.
// Returning false from fn stops the scan early.
// The store is held under lock for the entire scan — callers must not call
// Append or other mutating methods from within fn.
func (s *RecordingStore) Scan(fn func(recvTimeNS int64, frame []byte) bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.hasData() {
		return nil
	}

	pos := s.oldestPos
	for {
		if pos == s.writePos {
			break
		}

		// Read record header: recvTimeNS(8) + frameLen(2)
		var hdr [10]byte
		if _, err := s.f.ReadAt(hdr[:], pos); err != nil {
			return err
		}
		recvTimeNS := int64(binary.LittleEndian.Uint64(hdr[0:8]))
		frameLen := int64(binary.LittleEndian.Uint64(hdr[8:10]))
		if frameLen > maxFrameBytes {
			return errors.New("replay: corrupt record during scan")
		}

		frame := make([]byte, frameLen)
		if _, err := s.f.ReadAt(frame, pos+recordOverhead); err != nil {
			return err
		}

		if !fn(recvTimeNS, frame) {
			break
		}

		nextPos := pos + recordOverhead + frameLen
		// Wrap: if we just read the last record before the wrap point, jump to data start.
		if s.wrapPos != 0 && nextPos >= s.wrapPos {
			nextPos = headerSize
		}
		pos = nextPos
	}
	return nil
}

// ScanRange reads records within a time range [fromNS, toNS], calling fn for each.
// fn receives the receive timestamp (Unix nanoseconds) and the raw frame bytes.
// Returning false from fn stops the scan early.
// The store is held under lock for the entire scan — callers must not call
// Append or other mutating methods from within fn.
func (s *RecordingStore) ScanRange(fromNS, toNS int64, fn func(recvTimeNS int64, frame []byte) bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.hasData() {
		return nil
	}

	pos := s.oldestPos
	for {
		if pos == s.writePos {
			break
		}

		// Read record header: recvTimeNS(8) + frameLen(2)
		var hdr [10]byte
		if _, err := s.f.ReadAt(hdr[:], pos); err != nil {
			return err
		}
		recvTimeNS := int64(binary.LittleEndian.Uint64(hdr[0:8]))
		frameLen := int64(binary.LittleEndian.Uint64(hdr[8:10]))
		if frameLen > maxFrameBytes {
			return errors.New("replay: corrupt record during scan")
		}

		// Skip records before the time range
		if recvTimeNS < fromNS {
			nextPos := pos + recordOverhead + frameLen
			if s.wrapPos != 0 && nextPos >= s.wrapPos {
				nextPos = headerSize
			}
			pos = nextPos
			continue
		}

		// Stop if we've passed the time range
		if recvTimeNS > toNS {
			break
		}

		frame := make([]byte, frameLen)
		if _, err := s.f.ReadAt(frame, pos+recordOverhead); err != nil {
			return err
		}

		if !fn(recvTimeNS, frame) {
			break
		}

		nextPos := pos + recordOverhead + frameLen
		// Wrap: if we just read the last record before the wrap point, jump to data start.
		if s.wrapPos != 0 && nextPos >= s.wrapPos {
			nextPos = headerSize
		}
		pos = nextPos
	}
	return nil
}

// Stats returns summary statistics about the recording store.
type Stats struct {
	HasData   bool
	WritePos  int64
	OldestPos int64
	FileSize  int64
}

func (s *RecordingStore) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Stats{
		HasData:   s.hasData(),
		WritePos:  s.writePos,
		OldestPos: s.oldestPos,
		FileSize:  s.fileSize,
	}
}

// WritePos returns the current write position (for diagnostics).
func (s *RecordingStore) WritePos() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writePos
}

// Close closes the underlying file.
func (s *RecordingStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.f.Close()
}

// hasData reports whether there are any valid records.
func (s *RecordingStore) hasData() bool {
	return s.oldestPos != 0
}

// evictOne advances oldestPos past the oldest record.
func (s *RecordingStore) evictOne() error {
	if !s.hasData() {
		return nil
	}

	var lenBuf [2]byte
	if _, err := s.f.ReadAt(lenBuf[:], s.oldestPos+8); err != nil {
		return err
	}
	frameLen := int64(binary.LittleEndian.Uint16(lenBuf[:]))
	if frameLen > maxFrameBytes {
		// Corrupted record; reset state to recover.
		s.oldestPos = 0
		s.wrapPos = 0
		return nil
	}

	nextPos := s.oldestPos + recordOverhead + frameLen

	// When oldest has consumed the right arc, wrap it to the data start.
	if s.wrapPos != 0 && nextPos >= s.wrapPos {
		nextPos = headerSize
		s.wrapPos = 0
	}

	if nextPos == s.writePos {
		s.oldestPos = 0 // buffer is now empty
	} else {
		s.oldestPos = nextPos
	}
	return nil
}

func (s *RecordingStore) headerValid() bool {
	return s.writePos >= headerSize && s.writePos <= s.fileSize &&
		(s.oldestPos == 0 || (s.oldestPos >= headerSize && s.oldestPos <= s.fileSize)) &&
		(s.wrapPos == 0 || (s.wrapPos >= headerSize && s.wrapPos <= s.fileSize))
}

func (s *RecordingStore) readHeader() error {
	var buf [32]byte
	if _, err := s.f.ReadAt(buf[:], 0); err != nil {
		return err
	}
	if string(buf[0:8]) != fileMagic {
		return errors.New("replay: invalid magic")
	}
	s.writePos = int64(binary.LittleEndian.Uint64(buf[8:16]))
	s.oldestPos = int64(binary.LittleEndian.Uint64(buf[16:24]))
	s.wrapPos = int64(binary.LittleEndian.Uint64(buf[24:32]))
	return nil
}

func (s *RecordingStore) syncHeader() error {
	var buf [32]byte
	copy(buf[0:8], fileMagic)
	binary.LittleEndian.PutUint64(buf[8:16], uint64(s.writePos))
	binary.LittleEndian.PutUint64(buf[16:24], uint64(s.oldestPos))
	binary.LittleEndian.PutUint64(buf[24:32], uint64(s.wrapPos))
	_, err := s.f.WriteAt(buf[:], 0)
	return err
}
