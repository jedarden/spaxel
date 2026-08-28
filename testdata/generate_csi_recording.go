// Command to generate test CSI recording dataset for compression benchmarking
// Usage: go run generate_csi_recording.go
package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"math/rand"
	"os"
	"time"
)

const (
	// Replay buffer format constants (active implementation)
	fileMagic    = "SPAXLREC"
	headerSize   = 32
	recordOverhead = 10 // recvTimeNS(8) + frameLen(2)

	// CSI frame format constants
	csiHeaderSize = 24
	maxSubcarriers = 64
)

// writeHeader writes the replay store file header
func writeHeader(f *os.File, writePos, oldestPos, wrapPos uint64) error {
	header := make([]byte, headerSize)
	copy(header[0:8], []byte(fileMagic))
	binary.LittleEndian.PutUint64(header[8:16], writePos)
	binary.LittleEndian.PutUint64(header[16:24], oldestPos)
	binary.LittleEndian.PutUint64(header[24:32], wrapPos)
	_, err := f.WriteAt(header, 0)
	return err
}

// writeCSIRecord writes a single CSI frame record to the replay store
func writeCSIRecord(f *os.File, recvTimeNS int64, frameData []byte, writePos *uint64) error {
	frameLen := uint16(len(frameData))
	recordSize := recordOverhead + int64(frameLen)

	// Build record
	buf := make([]byte, recordSize)
	binary.LittleEndian.PutUint64(buf[0:8], uint64(recvTimeNS))
	binary.LittleEndian.PutUint16(buf[8:10], frameLen)
	copy(buf[10:], frameData)

	_, err := f.WriteAt(buf, int64(*writePos))
	if err != nil {
		return err
	}

	*writePos += uint64(recordSize)
	return nil
}

// generateCSIFrame generates a CSI frame with realistic characteristics
func generateCSIFrame(timestampUS uint64, activityLevel float64, nodeMAC, peerMAC [6]byte) []byte {
	frame := make([]byte, csiHeaderSize+maxSubcarriers*2)

	// Copy MAC addresses
	copy(frame[0:6], nodeMAC[:])
	copy(frame[6:12], peerMAC[:])

	// Timestamp
	binary.LittleEndian.PutUint64(frame[12:20], timestampUS)

	// RSSI and noise floor (more negative during activity due to body absorption)
	rssi := int8(-40 - int8(activityLevel*20))   // -40 (idle) to -60 (active)
	noiseFloor := int8(-90)
	frame[20] = byte(uint8(rssi))     // store as unsigned byte (will be read back as int8)
	frame[21] = byte(uint8(noiseFloor)) // store as unsigned byte (will be read back as int8)
	frame[22] = byte(6)   // channel 6
	frame[23] = byte(maxSubcarriers)

	// Generate CSI payload (I,Q pairs)
	// Idle: low variance, stable phase
	// Walking: high variance, phase shifts
	for k := 0; k < maxSubcarriers; k++ {
		var i, q int8
		var amplitude, phase float64

		if activityLevel < 0.3 {
			// Idle period: low amplitude variance, stable phase
			amplitude = 50.0 + 10.0*math.Sin(float64(k)*0.2)
			phase = 0.3 * float64(k)

			// Add small noise
			amplitude += (rand.Float64() - 0.5) * 2.0
			phase += (rand.Float64() - 0.5) * 0.1
		} else {
			// Walking period: high amplitude variance, dynamic phase
			// Simulate multipath interference and body reflection
			amplitude = 50.0 + 30.0*activityLevel*math.Abs(math.Sin(float64(k)*0.5+float64(timestampUS)*0.0001))
			phase = 0.3*float64(k) + activityLevel*math.Sin(float64(timestampUS)*0.01)

			// Add substantial noise
			amplitude += (rand.Float64() - 0.5) * 15.0
			phase += (rand.Float64() - 0.5) * 0.5
		}

		i = int8(amplitude * math.Cos(phase))
		q = int8(amplitude * math.Sin(phase))

		frame[csiHeaderSize+k*2] = byte(i)
		frame[csiHeaderSize+k*2+1] = byte(q)
	}

	return frame
}

func main() {
	outputPath := "/home/coding/spaxel/testdata/csi_session_mixed_activity.bin"

	// Parameters for a realistic session:
	// - 60 seconds total
	// - 20 Hz sampling (typical active rate)
	// - Idle periods (0-20s, 40-60s): activityLevel = 0.1
	// - Walking periods (20-40s): activityLevel = 0.8
	// Total frames: 60s × 20Hz = 1200 frames

	duration := time.Minute
	samplingRate := 20 // Hz
	totalFrames := int(duration.Seconds()) * samplingRate

	// Create file
	f, err := os.Create(outputPath)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	// Initialize MAC addresses
	var nodeMAC, peerMAC [6]byte
	copy(nodeMAC[:], []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE, 0xFF})
	copy(peerMAC[:], []byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66})

	// Write initial header
	writePos := uint64(headerSize)
	oldestPos := uint64(0) // Will be set after first write
	wrapPos := uint64(0)

	err = writeHeader(f, writePos, oldestPos, wrapPos)
	if err != nil {
		panic(err)
	}

	// Generate frames
	startTime := time.Now()
	fmt.Printf("Generating %d CSI frames (%.1f seconds at %d Hz)...\n",
		totalFrames, duration.Seconds(), samplingRate)

	for i := 0; i < totalFrames; i++ {
		timestampUS := uint64(i) * 1000000 / uint64(samplingRate) // microseconds
		recvTimeNS := startTime.Add(time.Duration(i) * time.Second / time.Duration(samplingRate)).UnixNano()

		// Determine activity level based on time
		elapsed := float64(i) / float64(samplingRate) // seconds
		var activityLevel float64

		// Idle period (0-20s)
		if elapsed < 20 {
			activityLevel = 0.1
		// Walking period (20-40s)
		} else if elapsed < 40 {
			activityLevel = 0.8
		// Idle period (40-60s)
		} else {
			activityLevel = 0.1
		}

		// Generate CSI frame
		frameData := generateCSIFrame(timestampUS, activityLevel, nodeMAC, peerMAC)

		// Write record
		err = writeCSIRecord(f, recvTimeNS, frameData, &writePos)
		if err != nil {
			panic(err)
		}

		// Set oldestPos after first write
		if oldestPos == 0 {
			oldestPos = uint64(headerSize)
			err = writeHeader(f, writePos, oldestPos, wrapPos)
			if err != nil {
				panic(err)
			}
		}

		// Progress every 200 frames
		if (i+1)%200 == 0 {
			fmt.Printf("  Generated %d/%d frames (%.1f%%)\n", i+1, totalFrames, 100*float64(i+1)/float64(totalFrames))
		}
	}

	// Final header update
	err = writeHeader(f, writePos, oldestPos, wrapPos)
	if err != nil {
		panic(err)
	}

	// Get file size
	info, err := f.Stat()
	if err != nil {
		panic(err)
	}

	fmt.Printf("\nTest CSI recording created successfully!\n")
	fmt.Printf("  Path: %s\n", outputPath)
	fmt.Printf("  Size: %.2f MB\n", float64(info.Size())/(1024*1024))
	fmt.Printf("  Duration: %.1f seconds\n", duration.Seconds())
	fmt.Printf("  Total frames: %d\n", totalFrames)
	fmt.Printf("  Sampling rate: %d Hz\n", samplingRate)
	fmt.Printf("  Activity pattern: 20s idle → 20s walking → 20s idle\n")
}
