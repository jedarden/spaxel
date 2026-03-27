package recording

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func tempBuffer(t *testing.T, maxMB int, retention time.Duration) (*Buffer, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test_recording.bin")
	b, err := NewBuffer(path, maxMB, retention)
	if err != nil {
		t.Fatalf("NewBuffer: %v", err)
	}
	return b, path
}

func makeFrame(size int) []byte {
	b := make([]byte, size)
	for i := range b {
		b[i] = byte(i % 251)
	}
	return b
}

func TestNewBuffer(t *testing.T) {
	b, _ := tempBuffer(t, 1, time.Hour)
	defer b.Close()

	if b.writePos != headerSize {
		t.Errorf("writePos = %d, want %d", b.writePos, headerSize)
	}
	if b.hasData() {
		t.Error("new buffer should be empty")
	}
	if b.Retention() != time.Hour {
		t.Errorf("retention = %v, want %v", b.Retention(), time.Hour)
	}
}

func TestAppendAndScan(t *testing.T) {
	b, _ := tempBuffer(t, 1, time.Hour)
	defer b.Close()

	now := time.Now().UnixNano()
	frame := makeFrame(152)
	for i := 0; i < 5; i++ {
		ts := now + int64(i)*int64(time.Millisecond)
		if err := b.Append(ts, frame); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	var count int
	if err := b.Scan(func(_ int64, f []byte) bool {
		count++
		if len(f) != len(frame) {
			t.Errorf("frame %d: got %d bytes, want %d", count, len(f), len(frame))
		}
		return true
	}); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if count != 5 {
		t.Errorf("scan count = %d, want 5", count)
	}
}

func TestScanPreservesOrder(t *testing.T) {
	b, _ := tempBuffer(t, 1, time.Hour)
	defer b.Close()

	base := time.Now().UnixNano()
	frame := makeFrame(50)
	const n = 10
	for i := 0; i < n; i++ {
		if err := b.Append(base+int64(i), frame); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	var prev int64 = -1
	i := 0
	b.Scan(func(recvTimeNS int64, _ []byte) bool {
		if recvTimeNS <= prev {
			t.Errorf("out-of-order at position %d: %d <= %d", i, recvTimeNS, prev)
		}
		prev = recvTimeNS
		i++
		return true
	})
	if i != n {
		t.Errorf("scanned %d records, want %d", i, n)
	}
}

func TestTimeBasedPruningOnAppend(t *testing.T) {
	b, _ := tempBuffer(t, 1, time.Hour)
	defer b.Close()

	// Write three frames with timestamps 2 hours ago.
	old := time.Now().Add(-2 * time.Hour).UnixNano()
	frame := makeFrame(100)
	for i := 0; i < 3; i++ {
		if err := b.Append(old+int64(i), frame); err != nil {
			t.Fatalf("old Append %d: %v", i, err)
		}
	}

	// Appending a fresh frame should prune the 2-hour-old frames
	// (cutoff = now - 1h, so frames at now-2h are evicted).
	nowNS := time.Now().UnixNano()
	if err := b.Append(nowNS, frame); err != nil {
		t.Fatalf("new Append: %v", err)
	}

	var count int
	if err := b.Scan(func(_ int64, _ []byte) bool {
		count++
		return true
	}); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if count != 1 {
		t.Errorf("after time-based pruning, scan count = %d, want 1", count)
	}
}

func TestExplicitPrune(t *testing.T) {
	b, _ := tempBuffer(t, 1, time.Hour)
	defer b.Close()

	// Write frames with timestamps 2 hours ago.
	old := time.Now().Add(-2 * time.Hour).UnixNano()
	frame := makeFrame(100)
	for i := 0; i < 3; i++ {
		if err := b.Append(old+int64(i), frame); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	if err := b.Prune(); err != nil {
		t.Fatalf("Prune: %v", err)
	}

	var count int
	b.Scan(func(_ int64, _ []byte) bool {
		count++
		return true
	})
	if count != 0 {
		t.Errorf("after explicit Prune, count = %d, want 0", count)
	}
	if b.hasData() {
		t.Error("hasData should be false after pruning all records")
	}
}

func TestScanRange(t *testing.T) {
	// Use 24h retention so no frames get pruned during the test.
	b, _ := tempBuffer(t, 1, 24*time.Hour)
	defer b.Close()

	// Use a fixed base time so the test is deterministic.
	base := time.Unix(1_000_000, 0)
	frame := makeFrame(100)
	const n = 10
	for i := 0; i < n; i++ {
		ts := base.Add(time.Duration(i) * time.Hour)
		if err := b.Append(ts.UnixNano(), frame); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	// Query hours [2, 5] inclusive — expect 4 records.
	from := base.Add(2 * time.Hour)
	to := base.Add(5 * time.Hour)

	var got []int64
	if err := b.ScanRange(from, to, func(recvTimeNS int64, _ []byte) bool {
		got = append(got, recvTimeNS)
		return true
	}); err != nil {
		t.Fatalf("ScanRange: %v", err)
	}

	if len(got) != 4 {
		t.Fatalf("ScanRange count = %d, want 4", len(got))
	}
	for i, ts := range got {
		want := base.Add(time.Duration(i+2) * time.Hour).UnixNano()
		if ts != want {
			t.Errorf("got[%d] = %d, want %d", i, ts, want)
		}
	}
}

func TestScanRangeBeforeData(t *testing.T) {
	b, _ := tempBuffer(t, 1, 24*time.Hour)
	defer b.Close()

	base := time.Unix(1_000_000, 0)
	frame := makeFrame(50)
	for i := 0; i < 3; i++ {
		b.Append(base.Add(time.Duration(i)*time.Minute).UnixNano(), frame)
	}

	// Query entirely before the data.
	var count int
	b.ScanRange(base.Add(-10*time.Minute), base.Add(-1*time.Minute), func(_ int64, _ []byte) bool {
		count++
		return true
	})
	if count != 0 {
		t.Errorf("count for query before data = %d, want 0", count)
	}
}

func TestScanRangeAfterData(t *testing.T) {
	b, _ := tempBuffer(t, 1, 24*time.Hour)
	defer b.Close()

	base := time.Unix(1_000_000, 0)
	frame := makeFrame(50)
	for i := 0; i < 3; i++ {
		b.Append(base.Add(time.Duration(i)*time.Minute).UnixNano(), frame)
	}

	// Query entirely after the data.
	var count int
	b.ScanRange(base.Add(10*time.Minute), base.Add(20*time.Minute), func(_ int64, _ []byte) bool {
		count++
		return true
	})
	if count != 0 {
		t.Errorf("count for query after data = %d, want 0", count)
	}
}

func TestScanRangeInvalidRange(t *testing.T) {
	b, _ := tempBuffer(t, 1, time.Hour)
	defer b.Close()

	now := time.Now()
	err := b.ScanRange(now.Add(time.Hour), now, func(_ int64, _ []byte) bool { return true })
	if err == nil {
		t.Error("expected error for from > to")
	}
}

func TestScanRangeEarlyStop(t *testing.T) {
	b, _ := tempBuffer(t, 1, 24*time.Hour)
	defer b.Close()

	base := time.Unix(1_000_000, 0)
	frame := makeFrame(50)
	for i := 0; i < 5; i++ {
		b.Append(base.Add(time.Duration(i)*time.Minute).UnixNano(), frame)
	}

	// Stop after first record.
	var count int
	b.ScanRange(base, base.Add(10*time.Minute), func(_ int64, _ []byte) bool {
		count++
		return count < 2 // stop after second record
	})
	if count != 2 {
		t.Errorf("early-stop scan returned %d records, want 2", count)
	}
}

func TestCrashRecovery(t *testing.T) {
	frame := makeFrame(152)
	dir := t.TempDir()
	path := filepath.Join(dir, "recording.bin")

	b1, err := NewBuffer(path, 1, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixNano()
	for i := 0; i < 3; i++ {
		if err := b1.Append(now+int64(i), frame); err != nil {
			t.Fatalf("b1.Append %d: %v", i, err)
		}
	}
	savedWrite := b1.writePos
	savedOldest := b1.oldestPos
	b1.Close()

	// Reopen should restore state from the header.
	b2, err := NewBuffer(path, 1, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	defer b2.Close()

	if b2.writePos != savedWrite {
		t.Errorf("writePos after reopen = %d, want %d", b2.writePos, savedWrite)
	}
	if b2.oldestPos != savedOldest {
		t.Errorf("oldestPos after reopen = %d, want %d", b2.oldestPos, savedOldest)
	}

	// Data should be readable after reopen.
	var count int
	b2.Scan(func(_ int64, _ []byte) bool {
		count++
		return true
	})
	if count != 3 {
		t.Errorf("after reopen, scan count = %d, want 3", count)
	}
}

func TestWrapAround(t *testing.T) {
	b, _ := tempBuffer(t, 1, 48*time.Hour)
	defer b.Close()

	frame := makeFrame(152)
	recordSize := recordOverhead + int64(len(frame))
	dataArea := b.fileSize - headerSize
	recsBeforeWrap := int(dataArea / recordSize)

	now := time.Now().UnixNano()
	for i := 0; i < recsBeforeWrap; i++ {
		if err := b.Append(now+int64(i), frame); err != nil {
			t.Fatalf("Append %d before wrap: %v", i, err)
		}
	}

	beforeWrapPos := b.writePos

	if err := b.Append(now+int64(recsBeforeWrap), frame); err != nil {
		t.Fatalf("Append triggering wrap: %v", err)
	}

	if b.writePos >= beforeWrapPos {
		t.Errorf("writePos %d should have wrapped (was %d before wrap)", b.writePos, beforeWrapPos)
	}
}

func TestStorageBounded(t *testing.T) {
	b, _ := tempBuffer(t, 1, 48*time.Hour)
	defer b.Close()

	frame := makeFrame(100)
	recordSize := recordOverhead + int64(len(frame))
	dataArea := b.fileSize - headerSize
	count := int(dataArea/recordSize) + 100 // force many wraps + evictions

	now := time.Now().UnixNano()
	for i := 0; i < count; i++ {
		if err := b.Append(now+int64(i), frame); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	if b.writePos < headerSize || b.writePos > b.fileSize {
		t.Errorf("writePos %d out of range [%d, %d]", b.writePos, headerSize, b.fileSize)
	}
}

func TestRetentionEnvVar(t *testing.T) {
	t.Setenv(RetentionEnvVar, "24h")

	// Pass 0 so that the env var is the only non-default source.
	b, _ := tempBuffer(t, 1, 0)
	defer b.Close()

	if b.Retention() != 24*time.Hour {
		t.Errorf("retention = %v, want 24h (from env var)", b.Retention())
	}
}

func TestRetentionEnvVarInvalidFallsBack(t *testing.T) {
	t.Setenv(RetentionEnvVar, "not-a-duration")

	b, _ := tempBuffer(t, 1, time.Hour)
	defer b.Close()

	// Invalid env var should fall back to the parameter value.
	if b.Retention() != time.Hour {
		t.Errorf("retention = %v, want 1h (fallback to parameter)", b.Retention())
	}
}

func TestInvalidMagicStartsFresh(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.bin")

	if err := os.WriteFile(path, []byte("GARBAGE_DATA_123456789012345678901234567890"), 0644); err != nil {
		t.Fatal(err)
	}

	b, err := NewBuffer(path, 1, time.Hour)
	if err != nil {
		t.Fatalf("should recover from bad magic: %v", err)
	}
	defer b.Close()

	if b.writePos != headerSize {
		t.Errorf("writePos = %d after bad magic, want %d", b.writePos, headerSize)
	}
	if b.hasData() {
		t.Error("buffer from bad-magic file should start empty")
	}
}

func TestFrameTooLarge(t *testing.T) {
	b, _ := tempBuffer(t, 1, time.Hour)
	defer b.Close()

	oversized := make([]byte, maxFrameBytes+1)
	if err := b.Append(0, oversized); err == nil {
		t.Error("expected error for oversized frame")
	}
}

func TestScanReadBackData(t *testing.T) {
	b, _ := tempBuffer(t, 1, time.Hour)
	defer b.Close()

	base := time.Now().UnixNano()
	frames := [][]byte{
		makeFrame(24),  // header-only (0 subcarriers)
		makeFrame(152), // 64 subcarriers
		makeFrame(280), // 128 subcarriers (max)
	}

	for i, f := range frames {
		if err := b.Append(base+int64(i), f); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	var idx int
	if err := b.Scan(func(recvTimeNS int64, f []byte) bool {
		wantTS := base + int64(idx)
		if recvTimeNS != wantTS {
			t.Errorf("record %d: recvTimeNS = %d, want %d", idx, recvTimeNS, wantTS)
		}
		if len(f) != len(frames[idx]) {
			t.Errorf("record %d: len = %d, want %d", idx, len(f), len(frames[idx]))
		}
		for j := range f {
			if f[j] != frames[idx][j] {
				t.Errorf("record %d byte %d: got %d, want %d", idx, j, f[j], frames[idx][j])
				break
			}
		}
		idx++
		return true
	}); err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if idx != len(frames) {
		t.Errorf("scanned %d records, want %d", idx, len(frames))
	}
}
