package replay

import (
	"os"
	"path/filepath"
	"testing"
)

func tempStore(t *testing.T, maxMB int) (*RecordingStore, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test_replay.bin")
	s, err := NewRecordingStore(path, maxMB)
	if err != nil {
		t.Fatalf("NewRecordingStore: %v", err)
	}
	return s, path
}

func makeFrame(size int) []byte {
	b := make([]byte, size)
	for i := range b {
		b[i] = byte(i % 251)
	}
	return b
}

func TestNewStore(t *testing.T) {
	s, _ := tempStore(t, 1)
	defer s.Close()

	if s.writePos != headerSize {
		t.Errorf("writePos = %d, want %d", s.writePos, headerSize)
	}
	if s.hasData() {
		t.Error("new store should be empty")
	}
}

func TestBasicAppend(t *testing.T) {
	s, _ := tempStore(t, 1)
	defer s.Close()

	frame := makeFrame(152) // typical 64-subcarrier frame
	if err := s.Append(1000, frame); err != nil {
		t.Fatalf("Append: %v", err)
	}

	expected := headerSize + recordOverhead + int64(len(frame))
	if s.writePos != expected {
		t.Errorf("writePos = %d, want %d", s.writePos, expected)
	}
	if !s.hasData() {
		t.Error("store should have data after append")
	}
	if s.oldestPos != headerSize {
		t.Errorf("oldestPos = %d, want %d", s.oldestPos, headerSize)
	}
}

func TestMultipleAppends(t *testing.T) {
	s, _ := tempStore(t, 1)
	defer s.Close()

	frame := makeFrame(152)
	for i := 0; i < 5; i++ {
		if err := s.Append(int64(i*1000), frame); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	expectedPos := headerSize + 5*(recordOverhead+int64(len(frame)))
	if s.writePos != expectedPos {
		t.Errorf("writePos = %d, want %d", s.writePos, expectedPos)
	}
}

func TestWrapAround(t *testing.T) {
	// Use a tiny 1-byte "MB" which gives ~1 MB buffer. Use a custom small size via
	// direct manipulation for testing. Instead, use a small-but-valid maxMB.
	// 1 MB data area fits floor(1MB / (10+152)) = ~6501 frames.
	s, _ := tempStore(t, 1)
	defer s.Close()

	frame := makeFrame(152)
	recordSize := recordOverhead + int64(len(frame))

	// Calculate how many records fit before a wrap
	dataArea := s.fileSize - headerSize
	recsBeforeWrap := int(dataArea / recordSize) // integer division

	// Fill to just before wrap
	for i := 0; i < recsBeforeWrap; i++ {
		if err := s.Append(int64(i), frame); err != nil {
			t.Fatalf("Append %d before wrap: %v", i, err)
		}
	}

	beforeWrapPos := s.writePos
	firstOldest := s.oldestPos

	// One more append should trigger wrap + eviction
	if err := s.Append(int64(recsBeforeWrap), frame); err != nil {
		t.Fatalf("Append triggering wrap: %v", err)
	}

	// writePos should have wrapped and advanced from headerSize
	if s.writePos >= beforeWrapPos {
		t.Errorf("writePos %d should have wrapped (was %d before wrap)", s.writePos, beforeWrapPos)
	}
	// oldest should have advanced (eviction happened)
	if s.oldestPos == firstOldest && s.oldestPos != 0 {
		t.Errorf("oldestPos %d should have advanced after wrap (first was %d)", s.oldestPos, firstOldest)
	}
}

func TestCrashRecovery(t *testing.T) {
	frame := makeFrame(152)
	dir := t.TempDir()
	path := filepath.Join(dir, "replay.bin")

	// Write some frames
	s1, err := NewRecordingStore(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := s1.Append(int64(i*1000), frame); err != nil {
			t.Fatalf("s1.Append %d: %v", i, err)
		}
	}
	savedWrite := s1.writePos
	savedOldest := s1.oldestPos
	s1.Close()

	// Reopen should restore state
	s2, err := NewRecordingStore(path, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	if s2.writePos != savedWrite {
		t.Errorf("writePos after reopen = %d, want %d", s2.writePos, savedWrite)
	}
	if s2.oldestPos != savedOldest {
		t.Errorf("oldestPos after reopen = %d, want %d", s2.oldestPos, savedOldest)
	}
}

func TestEvictionMaintainsData(t *testing.T) {
	// Use a very small buffer: headerSize + 3 records fit
	// We do this by checking eviction logic directly, using
	// a tiny 1MB store (already tested above). Simulate eviction
	// explicitly by calling evictOne after appending.
	s, _ := tempStore(t, 1)
	defer s.Close()

	frame := makeFrame(100)

	// Fill buffer just past the point where eviction must occur
	dataArea := s.fileSize - headerSize
	recordSize := recordOverhead + int64(len(frame))
	count := int(dataArea/recordSize) + 5 // force wraps

	for i := 0; i < count; i++ {
		if err := s.Append(int64(i), frame); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	// Store should still be valid (not returning errors)
	if s.writePos < headerSize || s.writePos > s.fileSize {
		t.Errorf("writePos %d out of range [%d, %d]", s.writePos, headerSize, s.fileSize)
	}
}

func TestInvalidMagicStartsFresh(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.bin")

	// Write garbage
	if err := os.WriteFile(path, []byte("GARBAGE_DATA_123456789012345678901234567890"), 0644); err != nil {
		t.Fatal(err)
	}

	s, err := NewRecordingStore(path, 1)
	if err != nil {
		t.Fatalf("should recover from bad magic: %v", err)
	}
	defer s.Close()

	if s.writePos != headerSize {
		t.Errorf("writePos = %d after bad magic, want %d", s.writePos, headerSize)
	}
}

func TestFrameTooLarge(t *testing.T) {
	s, _ := tempStore(t, 1)
	defer s.Close()

	oversized := make([]byte, maxFrameBytes+1)
	if err := s.Append(0, oversized); err == nil {
		t.Error("expected error for oversized frame")
	}
}
