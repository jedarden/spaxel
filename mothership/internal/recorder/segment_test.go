package recorder

import (
	"bytes"
	"encoding/binary"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestSegmentFileName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input time.Time
		want  string
	}{
		{time.Date(2026, 3, 27, 14, 30, 0, 0, time.UTC), "20260327-14.csi"},
		{time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), "20260101-00.csi"},
		{time.Date(2026, 12, 31, 23, 59, 59, 0, time.UTC), "20261231-23.csi"},
		// Non-UTC input should be converted to UTC.
		{time.Date(2026, 3, 27, 15, 0, 0, 0, time.FixedZone("EST", -5*3600)), "20260327-20.csi"},
	}
	for _, tt := range tests {
		got := segmentFileName(tt.input)
		if got != tt.want {
			t.Errorf("segmentFileName(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestParseSegmentTime(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		want    time.Time
		wantErr bool
	}{
		{"20260327-14.csi", time.Date(2026, 3, 27, 14, 0, 0, 0, time.UTC), false},
		{"20260101-00.csi", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), false},
		{"20261231-23.csi", time.Date(2026, 12, 31, 23, 0, 0, 0, time.UTC), false},
		{"not-a-segment.csi", time.Time{}, true},
		{"20260327.csi", time.Time{}, true},
		{"20260327-14.dat", time.Time{}, true},
	}
	for _, tt := range tests {
		got, err := parseSegmentTime(tt.name)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseSegmentTime(%q): expected error, got nil", tt.name)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseSegmentTime(%q): unexpected error: %v", tt.name, err)
			continue
		}
		if !got.Equal(tt.want) {
			t.Errorf("parseSegmentTime(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestSegmentHour(t *testing.T) {
	t.Parallel()
	input := time.Date(2026, 3, 27, 14, 30, 45, 123456789, time.UTC)
	got := segmentHour(input)
	want := time.Date(2026, 3, 27, 14, 0, 0, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("segmentHour(%v) = %v, want %v", input, got, want)
	}

	// Non-UTC input should be converted.
	input2 := time.Date(2026, 3, 27, 14, 30, 45, 0, time.FixedZone("EST", -5*3600))
	got2 := segmentHour(input2)
	want2 := time.Date(2026, 3, 27, 19, 0, 0, 0, time.UTC) // 14:30 EST = 19:30 UTC
	if !got2.Equal(want2) {
		t.Errorf("segmentHour(%v) = %v, want %v", input2, got2, want2)
	}
}

func TestLinkDir(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"AA:BB:CC:DD:EE:FF:11:22:33:44:55:66", "AA:BB:CC:DD:EE:FF-11:22:33:44:55:66"},
		{"AA:BB:CC:DD:EE:FF:AA:BB:CC:DD:EE:FF", "AA:BB:CC:DD:EE:FF-AA:BB:CC:DD:EE:FF"},
		{"short", "short"},
	}
	for _, tt := range tests {
		got := linkDir(tt.input)
		if got != tt.want {
			t.Errorf("linkDir(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestWriteAndScan(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "20260327-14.csi")

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		t.Fatal(err)
	}

	// Write three records with different timestamps.
	records := []struct {
		ns    int64
		frame []byte
	}{
		{1000, []byte("frame1")},
		{2000, []byte("frame2-data")},
		{3000, []byte("frame3-longer-data-here")},
	}

	for _, r := range records {
		if err := WriteRecord(f, r.ns, r.frame); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()

	// Scan all records.
	var scanned []struct {
		ns    int64
		frame []byte
	}
	err = ScanSegment(path, func(ns int64, frame []byte) bool {
		scanned = append(scanned, struct {
			ns    int64
			frame []byte
		}{ns, frame})
		return true
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(scanned) != 3 {
		t.Fatalf("expected 3 records, got %d", len(scanned))
	}
	for i, r := range records {
		if scanned[i].ns != r.ns {
			t.Errorf("record %d: ns = %d, want %d", i, scanned[i].ns, r.ns)
		}
		if string(scanned[i].frame) != string(r.frame) {
			t.Errorf("record %d: frame = %q, want %q", i, scanned[i].frame, r.frame)
		}
	}
}

func TestScanSegmentFrom(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "20260327-14.csi")

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		t.Fatal(err)
	}

	WriteRecord(f, 1000, []byte("a"))
	WriteRecord(f, 2000, []byte("b"))
	WriteRecord(f, 3000, []byte("c"))
	f.Close()

	// Scan from 2000 — should get "b" and "c".
	var result [][]byte
	err = ScanSegmentFrom(path, 2000, func(_ int64, frame []byte) bool {
		result = append(result, frame)
		return true
	})
	if err != nil {
		t.Fatal(err)
	}

	if len(result) != 2 {
		t.Fatalf("expected 2 records, got %d", len(result))
	}
	if string(result[0]) != "b" {
		t.Errorf("first frame = %q, want %q", result[0], "b")
	}
	if string(result[1]) != "c" {
		t.Errorf("second frame = %q, want %q", result[1], "c")
	}
}

func TestScanStopEarly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "20260327-14.csi")

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		t.Fatal(err)
	}
	WriteRecord(f, 1000, []byte("a"))
	WriteRecord(f, 2000, []byte("b"))
	WriteRecord(f, 3000, []byte("c"))
	f.Close()

	count := 0
	err = ScanSegment(path, func(_ int64, _ []byte) bool {
		count++
		return count < 2 // stop after 2
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Errorf("expected 2 records scanned, got %d", count)
	}
}

func TestScanEmptyFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "20260327-14.csi")

	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	count := 0
	err = ScanSegment(path, func(_ int64, _ []byte) bool {
		count++
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("expected 0 records from empty file, got %d", count)
	}
}

func TestScanCorruptRecord(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "20260327-14.csi")

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		t.Fatal(err)
	}
	// Write a valid record.
	WriteRecord(f, 1000, []byte("ok"))
	// Write truncated data (4-byte length says 100 bytes payload but only write 2).
	var buf [4]byte
	binary.BigEndian.PutUint32(buf[:], 100)
	f.Write(buf[:])
	f.Write([]byte{0xFF, 0xFF})
	f.Close()

	err = ScanSegment(path, func(_ int64, _ []byte) bool {
		return true
	})
	if err == nil {
		t.Error("expected error on corrupt record")
	}
}

func TestListSegmentFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create some segment files and a non-segment file.
	segments := []string{"20260327-13.csi", "20260327-14.csi", "20260327-15.csi"}
	for _, name := range segments {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	os.WriteFile(filepath.Join(dir, "other.txt"), nil, 0644)
	os.MkdirAll(filepath.Join(dir, "subdir"), 0755)

	files, err := listSegmentFiles(dir)
	if err != nil {
		t.Fatal(err)
	}

	if len(files) != 3 {
		t.Fatalf("expected 3 segment files, got %d", len(files))
	}

	// Should be sorted chronologically.
	var names []string
	for _, f := range files {
		names = append(names, filepath.Base(f))
	}
	if !sort.StringsAreSorted(names) {
		t.Errorf("segment files not sorted: %v", names)
	}
}

func TestListSegmentFilesNonExistentDir(t *testing.T) {
	t.Parallel()
	files, err := listSegmentFiles("/nonexistent/path")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Errorf("expected 0 files for nonexistent dir, got %d", len(files))
	}
}

func TestScanReaderWithBytesBuffer(t *testing.T) {
	t.Parallel()
	// Build a record in memory and scan it.
	var buf bytes.Buffer
	WriteRecordToBuffer(&buf, 42, []byte("hello"))

	count := 0
	err := scanReader(&buf, func(ns int64, frame []byte) bool {
		count++
		if ns != 42 {
			t.Errorf("ns = %d, want 42", ns)
		}
		if string(frame) != "hello" {
			t.Errorf("frame = %q, want %q", frame, "hello")
		}
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("expected 1 record, got %d", count)
	}
}

// WriteRecordToBuffer writes a record to a bytes.Buffer (for testing scanReader).
func WriteRecordToBuffer(buf *bytes.Buffer, recvTimeNS int64, frame []byte) {
	payloadLen := uint32(timestampSize + len(frame))
	var hdr [recordOverhead]byte
	binary.BigEndian.PutUint32(hdr[:4], payloadLen)
	binary.BigEndian.PutUint64(hdr[4:12], uint64(recvTimeNS))
	buf.Write(hdr[:])
	buf.Write(frame)
}
