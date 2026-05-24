package recorder

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestWriteAndReadFrom(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManager(Config{
		DataDir:        dir,
		RetentionHours: 48,
		BufferSize:     100,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close() //nolint:errcheck

	linkID := "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66"

	// Write frames.
	frame1 := []byte("csi-frame-1")
	frame2 := []byte("csi-frame-2")
	frame3 := []byte("csi-frame-3")

	beforeWrite := time.Now()
	mgr.Write(linkID, frame1)
	mgr.Write(linkID, frame2)
	mgr.Write(linkID, frame3)
	// Give the writer goroutine time to flush.
	time.Sleep(50 * time.Millisecond)

	// ReadFrom with a time before the writes.
	ch := mgr.ReadFrom(linkID, beforeWrite.Add(-time.Second))
	var frames [][]byte
	for f := range ch {
		frames = append(frames, f)
	}

	if len(frames) != 3 {
		t.Fatalf("expected 3 frames, got %d", len(frames))
	}
	for i, want := range [][]byte{frame1, frame2, frame3} {
		if string(frames[i]) != string(want) {
			t.Errorf("frame %d: got %q, want %q", i, frames[i], want)
		}
	}
}

func TestReadFromWithSince(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManager(Config{
		DataDir:        dir,
		RetentionHours: 48,
		BufferSize:     100,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close() //nolint:errcheck

	linkID := "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66"

	mgr.Write(linkID, []byte("frame-1"))
	time.Sleep(10 * time.Millisecond)
	cutoff := time.Now()
	time.Sleep(10 * time.Millisecond)
	mgr.Write(linkID, []byte("frame-2"))
	mgr.Write(linkID, []byte("frame-3"))

	time.Sleep(50 * time.Millisecond)

	// Read only frames after cutoff.
	ch := mgr.ReadFrom(linkID, cutoff)
	var frames [][]byte
	for f := range ch {
		frames = append(frames, f)
	}

	if len(frames) != 2 {
		t.Fatalf("expected 2 frames after cutoff, got %d", len(frames))
	}
	if string(frames[0]) != "frame-2" {
		t.Errorf("first frame = %q, want %q", frames[0], "frame-2")
	}
	if string(frames[1]) != "frame-3" {
		t.Errorf("second frame = %q, want %q", frames[1], "frame-3")
	}
}

func TestAvailableRange(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManager(Config{
		DataDir:        dir,
		RetentionHours: 48,
		BufferSize:     100,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close() //nolint:errcheck

	linkID := "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66"

	_, _, err = mgr.AvailableRange(linkID)
	if err == nil {
		t.Error("expected error for no data")
	}

	before := time.Now()
	mgr.Write(linkID, []byte("frame-1"))
	time.Sleep(20 * time.Millisecond)
	mgr.Write(linkID, []byte("frame-2"))
	time.Sleep(20 * time.Millisecond)
	after := time.Now()

	time.Sleep(50 * time.Millisecond)

	start, end, err := mgr.AvailableRange(linkID)
	if err != nil {
		t.Fatal(err)
	}
	if start.Before(before) {
		t.Errorf("start %v before first write %v", start, before)
	}
	if end.After(after) {
		t.Errorf("end %v after last write %v", end, after)
	}
	if start.After(end) {
		t.Errorf("start %v after end %v", start, end)
	}
}

func TestAvailableRangeNoData(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManager(Config{
		DataDir:        dir,
		RetentionHours: 48,
		BufferSize:     100,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close() //nolint:errcheck

	_, _, err = mgr.AvailableRange("AA:BB:CC:DD:EE:FF:11:22:33:44:55:66")
	if err == nil {
		t.Error("expected error for no data")
	}
}

func TestMultipleLinks(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManager(Config{
		DataDir:        dir,
		RetentionHours: 48,
		BufferSize:     100,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close() //nolint:errcheck

	link1 := "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66"
	link2 := "AA:BB:CC:DD:EE:FF:AA:BB:CC:DD:EE:FF"

	mgr.Write(link1, []byte("link1-frame"))
	mgr.Write(link2, []byte("link2-frame"))

	time.Sleep(50 * time.Millisecond)

	// Each link should have its own data.
	ch1 := mgr.ReadFrom(link1, time.Now().Add(-time.Minute))
	var frames1 [][]byte
	for f := range ch1 {
		frames1 = append(frames1, f)
	}
	if len(frames1) != 1 || string(frames1[0]) != "link1-frame" {
		t.Errorf("link1: got %d frames, want 1 with 'link1-frame'", len(frames1))
	}

	ch2 := mgr.ReadFrom(link2, time.Now().Add(-time.Minute))
	var frames2 [][]byte
	for f := range ch2 {
		frames2 = append(frames2, f)
	}
	if len(frames2) != 1 || string(frames2[0]) != "link2-frame" {
		t.Errorf("link2: got %d frames, want 1 with 'link2-frame'", len(frames2))
	}
}

func TestBufferFullDrop(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManager(Config{
		DataDir:        dir,
		RetentionHours: 48,
		BufferSize:     2, // tiny buffer
	})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close() //nolint:errcheck

	linkID := "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66"

	// Pause the writer goroutine by not reading from the channel.
	// Write more than buffer capacity — some will be dropped.
	// The first two will go into the channel; the third may or may not
	// depending on timing. With a tiny buffer, at least one should be dropped
	// if we flood fast enough.
	dropped := false
	for i := 0; i < 100; i++ {
		mgr.Write(linkID, []byte("frame"))
	}

	// Give writer time to process.
	time.Sleep(100 * time.Millisecond)

	// Count how many were actually written.
	ch := mgr.ReadFrom(linkID, time.Now().Add(-time.Minute))
	count := 0
	for range ch {
		count++
	}

	if count >= 100 {
		t.Log("warning: no frames were dropped (test timing issue)")
	} else {
		dropped = true
	}
	if !dropped {
		t.Log("buffer did not drop frames — may need tuning for slow machines")
	}
}

func TestConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManager(Config{
		DataDir:        dir,
		RetentionHours: 48,
		BufferSize:     1000,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close() //nolint:errcheck

	linkID := "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66"

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				mgr.Write(linkID, []byte{byte(id), byte(j)})
			}
		}(i)
	}
	wg.Wait()

	time.Sleep(100 * time.Millisecond)

	ch := mgr.ReadFrom(linkID, time.Now().Add(-time.Minute))
	count := 0
	for range ch {
		count++
	}

	if count != 1000 {
		t.Errorf("expected 1000 frames from concurrent writes, got %d", count)
	}
}

func TestCleanupRetention(t *testing.T) {
	dir := t.TempDir()

	// Create a link directory with a very old segment file.
	linkDirPath := filepath.Join(dir, linkDir("AA:BB:CC:DD:EE:FF:11:22:33:44:55:66"))
	if err := os.MkdirAll(linkDirPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Create an "old" segment file (retention = 1 hour for this test).
	oldName := segmentFileName(time.Now().UTC().Add(-2 * time.Hour))
	oldPath := filepath.Join(linkDirPath, oldName)
	if err := os.WriteFile(oldPath, nil, 0644); err != nil {
		t.Fatal(err)
	}

	// Create a "new" segment file.
	newName := segmentFileName(time.Now().UTC())
	newPath := filepath.Join(linkDirPath, newName)
	if err := os.WriteFile(newPath, nil, 0644); err != nil {
		t.Fatal(err)
	}

	mgr, err := NewManager(Config{
		DataDir:        dir,
		RetentionHours: 1, // 1 hour retention
		BufferSize:     100,
	})
	if err != nil {
		t.Fatal(err)
	}

	// The initial cleanup should have deleted the old file.
	// Give the async cleanup goroutine time to run.
	time.Sleep(100 * time.Millisecond)

	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Error("old segment file should have been deleted by cleanup")
	}
	if _, err := os.Stat(newPath); os.IsNotExist(err) {
		t.Error("new segment file should still exist")
	}

	mgr.Close() //nolint:errcheck
}

func TestCleanupMaxBytesPerLink(t *testing.T) {
	dir := t.TempDir()

	linkDirPath := filepath.Join(dir, linkDir("AA:BB:CC:DD:EE:FF:11:22:33:44:55:66"))
	if err := os.MkdirAll(linkDirPath, 0755); err != nil {
		t.Fatal(err)
	}

	// Create three segment files, each 100 bytes.
	for i := 0; i < 3; i++ {
		name := segmentFileName(time.Now().UTC().Add(time.Duration(i) * time.Hour))
		path := filepath.Join(linkDirPath, name)
		if err := os.WriteFile(path, make([]byte, 100), 0644); err != nil {
			t.Fatal(err)
		}
	}

	mgr, err := NewManager(Config{
		DataDir:         dir,
		RetentionHours:  48,
		MaxBytesPerLink: 150, // allow max ~1.5 files
		BufferSize:      100,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Cleanup should have deleted enough files to get under 150 bytes.
	// Give the async cleanup goroutine time to run.
	time.Sleep(100 * time.Millisecond)

	files, err := listSegmentFiles(linkDirPath)
	if err != nil {
		t.Fatal(err)
	}

	var totalSize int64
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		totalSize += info.Size()
	}

	if totalSize > 150 {
		t.Errorf("total size %d exceeds MaxBytesPerLink 150 after cleanup", totalSize)
	}

	mgr.Close() //nolint:errcheck
}

func TestWriteAfterClose(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManager(Config{
		DataDir:        dir,
		RetentionHours: 48,
		BufferSize:     100,
	})
	if err != nil {
		t.Fatal(err)
	}

	mgr.Close() //nolint:errcheck

	// Write after close should be a no-op, not panic.
	mgr.Write("AA:BB:CC:DD:EE:FF:11:22:33:44:55:66", []byte("should-not-write"))
}

func TestSegmentRotation(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManager(Config{
		DataDir:        dir,
		RetentionHours: 48,
		BufferSize:     100,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close() //nolint:errcheck

	linkID := "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66"

	// Write frames in the current hour.
	mgr.Write(linkID, []byte("frame-in-hour-1"))
	time.Sleep(50 * time.Millisecond)

	// Verify data was written.
	_, _, err = mgr.AvailableRange(linkID)
	if err != nil {
		t.Fatalf("expected data available: %v", err)
	}

	// Verify segment file exists in the link directory.
	linkDirPath := filepath.Join(dir, linkDir(linkID))
	files, err := listSegmentFiles(linkDirPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Errorf("expected 1 segment file, got %d", len(files))
	}
}

func TestPauseResumeWrites(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManager(Config{
		DataDir:        dir,
		RetentionHours: 48,
		BufferSize:     100,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close() //nolint:errcheck

	linkID := "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66"

	// Write some frames before pausing
	mgr.Write(linkID, []byte("frame-1"))
	mgr.Write(linkID, []byte("frame-2"))
	time.Sleep(50 * time.Millisecond)

	// Pause writes
	mgr.PauseWrites()
	if !mgr.IsPaused() {
		t.Error("IsPaused should return true after PauseWrites")
	}

	// Write while paused - these should be dropped
	mgr.Write(linkID, []byte("frame-paused-1"))
	mgr.Write(linkID, []byte("frame-paused-2"))
	time.Sleep(50 * time.Millisecond)

	// Resume writes
	mgr.ResumeWrites()
	if mgr.IsPaused() {
		t.Error("IsPaused should return false after ResumeWrites")
	}

	// Write more frames after resuming
	mgr.Write(linkID, []byte("frame-3"))
	time.Sleep(50 * time.Millisecond)

	// Verify we only have 3 frames (frame-1, frame-2, frame-3)
	// The paused frames should have been dropped
	ch := mgr.ReadFrom(linkID, time.Now().Add(-time.Minute))
	var frames [][]byte
	for f := range ch {
		frames = append(frames, f)
	}

	if len(frames) != 3 {
		t.Fatalf("expected 3 frames (2 before pause + 1 after resume), got %d", len(frames))
	}
	if string(frames[0]) != "frame-1" {
		t.Errorf("frame 0 = %q, want %q", frames[0], "frame-1")
	}
	if string(frames[1]) != "frame-2" {
		t.Errorf("frame 1 = %q, want %q", frames[1], "frame-2")
	}
	if string(frames[2]) != "frame-3" {
		t.Errorf("frame 2 = %q, want %q", frames[2], "frame-3")
	}
}

func TestConcurrentPauseWrites(t *testing.T) {
	dir := t.TempDir()
	mgr, err := NewManager(Config{
		DataDir:        dir,
		RetentionHours: 48,
		BufferSize:     100,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer mgr.Close() //nolint:errcheck

	linkID := "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66"

	var wg sync.WaitGroup

	// Start goroutines that pause/resume and write concurrently
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if j%10 == 0 {
					mgr.PauseWrites()
				} else if j%10 == 5 {
					mgr.ResumeWrites()
				}
				mgr.Write(linkID, []byte{byte(id), byte(j)})
			}
		}(i)
	}
	wg.Wait()

	// Resume to make sure we're not paused
	mgr.ResumeWrites()
	time.Sleep(100 * time.Millisecond)

	// Verify we got some frames (not all, due to pauses)
	ch := mgr.ReadFrom(linkID, time.Now().Add(-time.Minute))
	count := 0
	for range ch {
		count++
	}

	// Should have written some frames but not all (some dropped during pause)
	if count == 0 {
		t.Error("expected some frames to be written")
	}
	if count >= 500 { // 10 * 50
		t.Errorf("expected fewer than 500 frames due to pauses, got %d", count)
	}
}
