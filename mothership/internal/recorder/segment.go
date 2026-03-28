// Package recorder implements per-link CSI frame recording with 1-hour
// append-only segment files and configurable time-based retention.
//
// Segment file layout (per link directory):
//
//	data/{nodeMAC}-{peerMAC}/{YYYYMMDD-HH}.csi
//
// Record format within each segment file:
//
//	[4-byte BE uint32: payloadLen][payloadLen bytes]
//	  payloadLen = 8 + len(rawCSIframe)
//	  payload = [8-byte BE int64: recvTimeNS Unix nanoseconds][raw CSI frame bytes]
//
// Records are appended in chronological order. Segment files are rotated
// hourly. Background cleanup deletes files older than RetentionHours and
// enforces MaxBytesPerLink as a secondary guard.
package recorder

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	lengthPrefixSize = 4
	timestampSize    = 8
	// recordOverhead is the per-record overhead: length prefix + timestamp.
	recordOverhead = lengthPrefixSize + timestampSize
	// maxFrameBytes is the maximum raw CSI frame size (24 header + 128*2 payload).
	maxFrameBytes = 280
	segmentExt    = ".csi"
)

// segmentFileName returns the segment file name for the given time.
// Format: YYYYMMDD-HH.csi (UTC).
func segmentFileName(t time.Time) string {
	return fmt.Sprintf("%s-%02d%s", t.UTC().Format("20060102"), t.UTC().Hour(), segmentExt)
}

// parseSegmentTime parses a segment file name and returns the segment start time (UTC).
func parseSegmentTime(name string) (time.Time, error) {
	base := strings.TrimSuffix(name, segmentExt)
	if len(base) != 11 || base[8] != '-' {
		return time.Time{}, fmt.Errorf("recorder: invalid segment file name: %s", name)
	}
	t, err := time.ParseInLocation("20060102-15", base, time.UTC)
	if err != nil {
		return time.Time{}, fmt.Errorf("recorder: invalid segment file name: %s: %w", name, err)
	}
	return t, nil
}

// segmentHour returns the start of the hour for the given time in UTC.
func segmentHour(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), u.Hour(), 0, 0, 0, time.UTC)
}

// linkDir converts a linkID to a directory name.
// "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66" -> "AA:BB:CC:DD:EE:FF-11:22:33:44:55:66"
func linkDir(linkID string) string {
	if len(linkID) == 35 && linkID[17] == ':' {
		return linkID[:17] + "-" + linkID[18:]
	}
	return strings.ReplaceAll(linkID, ":", "-")
}

// WriteRecord appends a record to f.
// Record: [4-byte BE payloadLen][8-byte BE recvTimeNS][raw frame bytes].
func WriteRecord(f *os.File, recvTimeNS int64, frame []byte) error {
	payloadLen := uint32(timestampSize + len(frame))
	var hdr [recordOverhead]byte
	binary.BigEndian.PutUint32(hdr[:4], payloadLen)
	binary.BigEndian.PutUint64(hdr[4:12], uint64(recvTimeNS))
	if _, err := f.Write(hdr[:]); err != nil {
		return err
	}
	_, err := f.Write(frame)
	return err
}

// ScanSegment reads all records from a segment file in order.
// Calls fn for each record. If fn returns false, scanning stops.
func ScanSegment(path string, fn func(recvTimeNS int64, frame []byte) bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return scanReader(f, fn)
}

// ScanSegmentFrom reads records with recvTimeNS >= sinceNS from a segment file.
func ScanSegmentFrom(path string, sinceNS int64, fn func(recvTimeNS int64, frame []byte) bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return scanReader(f, func(recvTimeNS int64, frame []byte) bool {
		if recvTimeNS < sinceNS {
			return true // skip
		}
		return fn(recvTimeNS, frame)
	})
}

// scanReader reads records from r, calling fn for each.
func scanReader(r io.Reader, fn func(recvTimeNS int64, frame []byte) bool) error {
	var lenBuf [lengthPrefixSize]byte
	var tsBuf [timestampSize]byte

	for {
		if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}

		payloadLen := int(binary.BigEndian.Uint32(lenBuf[:]))
		if payloadLen < timestampSize {
			return fmt.Errorf("recorder: invalid record: payload length %d < %d", payloadLen, timestampSize)
		}

		if _, err := io.ReadFull(r, tsBuf[:]); err != nil {
			return err
		}
		recvTimeNS := int64(binary.BigEndian.Uint64(tsBuf[:]))

		frameLen := payloadLen - timestampSize
		if frameLen > maxFrameBytes {
			return fmt.Errorf("recorder: frame too large: %d bytes", frameLen)
		}

		frame := make([]byte, frameLen)
		if frameLen > 0 {
			if _, err := io.ReadFull(r, frame); err != nil {
				return err
			}
		}

		if !fn(recvTimeNS, frame) {
			return nil
		}
	}
}

// listSegmentFiles lists segment files in dir, sorted chronologically by name.
func listSegmentFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), segmentExt) {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	sort.Strings(files)
	return files, nil
}
