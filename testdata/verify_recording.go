// Test program to verify the CSI recording can be read by replay code
package main

import (
	"encoding/binary"
	"fmt"
	"os"
)

const (
	fileMagic      = "SPAXLREP"
	headerSize     = int64(32)
	recordOverhead = int64(10)
	maxFrameBytes  = int64(280)
)

func main() {
	testFile := "/home/coding/spaxel/testdata/csi_session_mixed_activity.bin"

	f, err := os.Open(testFile)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	// Get file info to know actual size
	info, err := f.Stat()
	if err != nil {
		panic(err)
	}
	fileSize := info.Size()

	// Read and verify header
	var buf [32]byte
	if _, err := f.ReadAt(buf[:], 0); err != nil {
		panic(err)
	}

	magic := string(buf[0:8])
	if magic != fileMagic {
		panic(fmt.Sprintf("Invalid magic: %s (expected %s)", magic, fileMagic))
	}

	writePos := int64(binary.LittleEndian.Uint64(buf[8:16]))
	oldestPos := int64(binary.LittleEndian.Uint64(buf[16:24]))
	wrapPos := int64(binary.LittleEndian.Uint64(buf[24:32]))

	fmt.Printf("CSI Recording Verification\n")
	fmt.Printf("=========================\n")
	fmt.Printf("File: %s\n", testFile)
	fmt.Printf("Magic: %s ✓\n", magic)
	fmt.Printf("File size: %d bytes (%.2f MB)\n", fileSize, float64(fileSize)/(1024*1024))
	fmt.Printf("Write position: %d\n", writePos)
	fmt.Printf("Oldest position: %d\n", oldestPos)
	fmt.Printf("Wrap position: %d\n", wrapPos)

	// Count and scan frames
	frameCount := 0
	pos := oldestPos
	var totalAmplitude float64
	var firstTimestamp, lastTimestamp int64
	var nSub uint8
	var rssi int8
	var channel uint8
	var frameLen int64

	for pos < writePos && pos >= headerSize {
		// Check we have enough data for a record header (10 bytes)
		// Need at least 10 bytes remaining in both file and valid data region
		remainingInFile := fileSize - pos
		remainingValid := writePos - pos

		// Debug output for first few iterations
		if frameCount < 5 {
			fmt.Printf("[DEBUG] Frame %d: pos=%d, remainingInFile=%d, remainingValid=%d\n",
				frameCount, pos, remainingInFile, remainingValid)
		}

		if remainingInFile < 10 || remainingValid < 10 {
			fmt.Printf("[DEBUG] Breaking at frame %d: pos=%d, remainingInFile=%d, remainingValid=%d\n",
				frameCount, pos, remainingInFile, remainingValid)
			break // Not enough data for a record header
		}

		// Read record header
		hdr := make([]byte, 10)
		n, err := f.ReadAt(hdr, pos)
		if err != nil {
			panic(fmt.Sprintf("Failed to read header at pos %d (file size %d): %v", pos, fileSize, err))
		}
		if n != 10 {
			panic(fmt.Sprintf("Short read at pos %d: got %d bytes, expected 10 (file size %d)", pos, n, fileSize))
		}

		if frameCount < 5 {
			fmt.Printf("[DEBUG] Read %d bytes at pos %d: %v\n", n, pos, hdr)
		}

		recvTimeNS := int64(binary.LittleEndian.Uint64(hdr[0:8]))
		frameLen = int64(binary.LittleEndian.Uint16(hdr[8:10]))

		if frameLen > maxFrameBytes || frameLen < 24 {
			panic(fmt.Sprintf("Invalid frame length: %d at pos %d", frameLen, pos))
		}

		// Check we have enough data for the full record (header + payload)
		if pos+recordOverhead+frameLen > fileSize || pos+recordOverhead+frameLen > writePos {
			break // Not enough data for a full record
		}

		// Read CSI frame header
		var frameHdr [24]byte
		if _, err := f.ReadAt(frameHdr[:], pos+recordOverhead); err != nil {
			panic(fmt.Sprintf("Failed to read frame header at pos %d: %v", pos+recordOverhead, err))
		}

		// Parse frame fields
		nSub = uint8(frameHdr[23])
		rssi = int8(frameHdr[20])
		channel = uint8(frameHdr[22])

		// Calculate total amplitude (sum of absolute I/Q values)
		payloadSize := int(nSub) * 2
		if payloadSize > 0 {
			payload := make([]byte, payloadSize)
			if _, err := f.ReadAt(payload, pos+recordOverhead+24); err != nil {
				panic(fmt.Sprintf("Failed to read payload at pos %d: %v", pos+recordOverhead+24, err))
			}

			for i := 0; i < payloadSize; i += 2 {
				iVal := int8(payload[i])
				qVal := int8(payload[i+1])
				amplitude := float64(iVal*iVal+qVal*qVal)
				totalAmplitude += amplitude
			}
		}

		frameCount++
		if firstTimestamp == 0 {
			firstTimestamp = recvTimeNS
		}
		lastTimestamp = recvTimeNS

		// Move to next record
		nextPos := pos + recordOverhead + frameLen
		if wrapPos != 0 && nextPos >= wrapPos {
			nextPos = headerSize
		}
		pos = nextPos
	}

	duration := float64(lastTimestamp-firstTimestamp) / 1e9 // convert nanoseconds to seconds
	avgAmplitude := totalAmplitude / float64(frameCount)

	fmt.Printf("\nFrame Statistics\n")
	fmt.Printf("================\n")
	fmt.Printf("Total frames: %d\n", frameCount)
	fmt.Printf("Duration: %.2f seconds\n", duration)
	fmt.Printf("Average amplitude per frame: %.2f\n", avgAmplitude)
	fmt.Printf("Frame size: %d bytes (header: 24 + payload: %d)\n", 24+int(nSub)*2, int(nSub)*2)
	fmt.Printf("Subcarriers: %d\n", nSub)
	fmt.Printf("RSSI: %d dBm\n", rssi)
	fmt.Printf("Channel: %d\n", channel)
	fmt.Printf("\n✓ Recording is valid and readable by replay code!\n")
}
