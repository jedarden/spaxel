package recording

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"os"
	"testing"
	"time"
)

// Benchmark compression with realistic CSI frame patterns.
// CSI frames are highly compressible because:
// 1. Adjacent frames are correlated (motion is slow relative to 20 Hz sampling)
// 2. Idle periods dominate (most time, no one is moving)
// 3. Amplitude/phase values follow predictable patterns

func generateCSIFrame(seed uint32) []byte {
	r := rand.New(rand.NewSource(int64(seed)))

	// CSI frame header (24 bytes)
	buf := make([]byte, 24)

	// Node MAC (6 bytes) - use seed to generate realistic MAC
	for i := 0; i < 6; i++ {
		buf[i] = byte(r.Intn(256))
	}

	// Peer MAC (6 bytes)
	for i := 0; i < 6; i++ {
		buf[6+i] = byte(r.Intn(256))
	}

	// Timestamp (8 bytes) - microseconds
	binary.LittleEndian.PutUint64(buf[12:20], uint64(r.Intn(1_000_000)))

	// RSSI, noise floor, channel, n_sub (4 bytes)
	buf[20] = byte(r.Intn(256)) // RSSI
	buf[21] = byte(r.Intn(256)) // noise floor
	buf[22] = byte(r.Intn(14) + 1) // channel 1-14
	buf[23] = 64 // n_sub = 64 subcarriers

	// Payload: 64 subcarriers * 2 bytes (I + Q)
	payloadLen := 64 * 2
	payload := make([]byte, payloadLen)

	// Simulate idle vs active patterns
	// Idle: most values are similar (low variance)
	// Active: higher variance
	isActive := r.Float32() < 0.1 // 10% active frames

	if isActive {
		// Active frame: random values (higher entropy)
		for i := 0; i < len(payload); i++ {
			payload[i] = byte(r.Intn(256))
		}
	} else {
		// Idle frame: correlated with previous frames
		// Use a base value with small variations
		base := int(r.Intn(50) + 100) // Base amplitude
		for i := 0; i < len(payload); i += 2 {
			// I component: base + small variation
			variation := r.Intn(10) - 5 // -5 to +5
			iVal := base + variation
			if iVal < 0 {
				iVal = 0
			}
			if iVal > 255 {
				iVal = 255
			}
			payload[i] = byte(iVal)

			// Q component: similar pattern with phase offset
			qVal := base + variation + r.Intn(5)
			if qVal < 0 {
				qVal = 0
			}
			if qVal > 255 {
				qVal = 255
			}
			payload[i+1] = byte(qVal)
		}
	}

	buf = append(buf, payload...)
	return buf
}

// BenchmarkUncompressedWrite benchmarks writing without compression.
func BenchmarkUncompressedWrite(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "buffer_test")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	buf, err := NewBuffer(tmpDir+"/test.bin", 10, 0, false, 0)
	if err != nil {
		b.Fatal(err)
	}
	defer buf.Close()

	b.ResetTimer()
	now := time.Now()

	for i := 0; i < b.N; i++ {
		frame := generateCSIFrame(uint32(i))
		recvTimeNS := now.Add(time.Duration(i) * time.Second).UnixNano()
		if err := buf.Append(recvTimeNS, frame); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCompressedWrite benchmarks writing with zstd compression.
func BenchmarkCompressedWrite(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "buffer_test")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	buf, err := NewBuffer(tmpDir+"/test.bin", 10, 0, true, DefaultChunkSize)
	if err != nil {
		b.Fatal(err)
	}
	defer buf.Close()

	b.ResetTimer()
	now := time.Now()

	for i := 0; i < b.N; i++ {
		frame := generateCSIFrame(uint32(i))
		recvTimeNS := now.Add(time.Duration(i) * time.Second).UnixNano()
		if err := buf.Append(recvTimeNS, frame); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCompressionRatio measures the compression ratio achieved.
func BenchmarkCompressionRatio(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "buffer_test")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Simulate 1 hour of data at 20 Hz with 8 nodes
	frameCount := 20 * 60 * 60 // 72,000 frames

	b.ReportMetric(float64(frameCount), "frames")

	// Uncompressed
	bufUncompressed, err := NewBuffer(tmpDir+"/test_uncompressed.bin", 10, 0, false, 0)
	if err != nil {
		b.Fatal(err)
	}

	now := time.Now()
	for i := 0; i < frameCount; i++ {
		frame := generateCSIFrame(uint32(i))
		recvTimeNS := now.Add(time.Duration(i) * 50 * time.Millisecond).UnixNano() // 20 Hz = 50ms intervals
		if err := bufUncompressed.Append(recvTimeNS, frame); err != nil {
			b.Fatal(err)
		}
	}

	statsUncompressed := bufUncompressed.Stats()
	bufUncompressed.Close()

	uncompressedBytes := statsUncompressed.WritePos - headerSize

	// Compressed - track only compressed chunks written
	bufCompressed, err := NewBuffer(tmpDir+"/test_compressed.bin", 10, 0, true, DefaultChunkSize)
	if err != nil {
		b.Fatal(err)
	}

	now = time.Now()
	for i := 0; i < frameCount; i++ {
		frame := generateCSIFrame(uint32(i))
		recvTimeNS := now.Add(time.Duration(i) * 50 * time.Millisecond).UnixNano()
		if err := bufCompressed.Append(recvTimeNS, frame); err != nil {
			b.Fatal(err)
		}
	}

	// Flush pending data to get final compressed size
	if err := bufCompressed.FlushCompressed(); err != nil {
		b.Fatal(err)
	}

	statsCompressed := bufCompressed.Stats()
	bufCompressed.Close()

	compressedBytes := statsCompressed.WritePos - headerSize

	// Calculate actual compression ratio
	ratio := float64(compressedBytes) / float64(uncompressedBytes)
	b.ReportMetric(float64(uncompressedBytes), "uncompressed_bytes")
	b.ReportMetric(float64(compressedBytes), "compressed_bytes")
	b.ReportMetric(ratio, "compression_ratio")
	b.ReportMetric((1.0-ratio)*100.0, "space_saved_pct")
}

// BenchmarkScanUncompressed benchmarks scanning uncompressed records.
func BenchmarkScanUncompressed(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "buffer_test")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Pre-populate buffer
	buf, err := NewBuffer(tmpDir+"/test.bin", 10, 0, false, 0)
	if err != nil {
		b.Fatal(err)
	}
	defer buf.Close()

	frameCount := 1000
	now := time.Now()
	for i := 0; i < frameCount; i++ {
		frame := generateCSIFrame(uint32(i))
		recvTimeNS := now.Add(time.Duration(i) * time.Second).UnixNano()
		if err := buf.Append(recvTimeNS, frame); err != nil {
			b.Fatal(err)
		}
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		scanned := 0
		err := buf.Scan(func(recvTimeNS int64, frame []byte) bool {
			scanned++
			return true
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkScanCompressed benchmarks scanning compressed records.
func BenchmarkScanCompressed(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "buffer_test")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Pre-populate buffer with compression
	buf, err := NewBuffer(tmpDir+"/test.bin", 10, 0, true, DefaultChunkSize)
	if err != nil {
		b.Fatal(err)
	}
	defer buf.Close()

	frameCount := 1000
	now := time.Now()
	for i := 0; i < frameCount; i++ {
		frame := generateCSIFrame(uint32(i))
		recvTimeNS := now.Add(time.Duration(i) * time.Second).UnixNano()
		if err := buf.Append(recvTimeNS, frame); err != nil {
			b.Fatal(err)
		}
	}

	// Flush pending data
	if err := buf.FlushCompressed(); err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		scanned := 0
		err := buf.Scan(func(recvTimeNS int64, frame []byte) bool {
			scanned++
			return true
		})
		if err != nil {
			b.Fatal(err)
		}
	}
}

// TestCompressedRoundTrip verifies that compressed chunks can be read back correctly.
func TestCompressedRoundTrip(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "buffer_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	buf, err := NewBuffer(tmpDir+"/test.bin", 10, 0, true, DefaultChunkSize)
	if err != nil {
		t.Fatal(err)
	}
	defer buf.Close()

	// Write test frames
	now := time.Now()
	writtenFrames := make(map[int64][]byte)
	for i := 0; i < 100; i++ {
		frame := generateCSIFrame(uint32(i))
		recvTimeNS := now.Add(time.Duration(i) * time.Second).UnixNano()

		if err := buf.Append(recvTimeNS, frame); err != nil {
			t.Fatalf("Append frame %d: %v", i, err)
		}
		writtenFrames[recvTimeNS] = frame
	}

	// Flush pending data
	if err := buf.FlushCompressed(); err != nil {
		t.Fatalf("Flush compressed: %v", err)
	}

	// Read back and verify
	readFrames := make(map[int64][]byte)
	err = buf.Scan(func(recvTimeNS int64, frame []byte) bool {
		readFrames[recvTimeNS] = frame
		return true
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	// Verify all frames match
	if len(readFrames) != len(writtenFrames) {
		t.Errorf("Frame count mismatch: got %d, want %d", len(readFrames), len(writtenFrames))
	}

	for ts, written := range writtenFrames {
		read, ok := readFrames[ts]
		if !ok {
			t.Errorf("Missing timestamp %d", ts)
			continue
		}
		if !bytes.Equal(written, read) {
			t.Errorf("Frame mismatch at timestamp %d", ts)
		}
	}
}

// TestBackwardCompatibility verifies that an uncompressed buffer can be read.
func TestBackwardCompatibility(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "buffer_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Write uncompressed data
	buf, err := NewBuffer(tmpDir+"/test.bin", 10, 0, false, 0)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	writtenFrames := make(map[int64][]byte)
	for i := 0; i < 50; i++ {
		frame := generateCSIFrame(uint32(i))
		recvTimeNS := now.Add(time.Duration(i) * time.Second).UnixNano()

		if err := buf.Append(recvTimeNS, frame); err != nil {
			t.Fatalf("Append frame %d: %v", i, err)
		}
		writtenFrames[recvTimeNS] = frame
	}
	buf.Close()

	// Reopen and verify (should read uncompressed format)
	buf2, err := NewBuffer(tmpDir+"/test.bin", 10, 0, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer buf2.Close()

	readFrames := make(map[int64][]byte)
	err = buf2.Scan(func(recvTimeNS int64, frame []byte) bool {
		readFrames[recvTimeNS] = frame
		return true
	})
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}

	if len(readFrames) != len(writtenFrames) {
		t.Errorf("Frame count mismatch: got %d, want %d", len(readFrames), len(writtenFrames))
	}

	for ts, written := range writtenFrames {
		read, ok := readFrames[ts]
		if !ok {
			t.Errorf("Missing timestamp %d", ts)
		}
		if !bytes.Equal(written, read) {
			t.Errorf("Frame mismatch at timestamp %d", ts)
		}
	}
}
