// Test program to verify the CSI recording using the actual replay store code
package replay

import (
	"fmt"
	"testing"

	"github.com/spaxel/mothership/internal/ingestion"
)

func TestVerifyRecording(t *testing.T) {
	testFile := "../../../testdata/csi_session_mixed_activity.bin"

	// Open the recording store using the actual replay code
	store, err := NewRecordingStore(testFile, 0)
	if err != nil {
		t.Fatalf("Failed to open recording store: %v", err)
	}
	defer store.Close()

	stats := store.Stats()

	fmt.Printf("CSI Recording Verification (using replay store)\n")
	fmt.Printf("==================================================\n")
	fmt.Printf("File: %s\n", testFile)
	fmt.Printf("Has data: %v\n", stats.HasData)
	fmt.Printf("Write position: %d\n", stats.WritePos)
	fmt.Printf("Oldest position: %d\n", stats.OldestPos)
	fmt.Printf("File size: %d bytes (%.2f MB)\n", stats.FileSize, float64(stats.FileSize)/(1024*1024))

	// Scan all frames and collect statistics
	frameCount := 0
	var totalAmplitude float64
	var firstTimestamp, lastTimestamp int64
	var nSub uint8
	var rssi int8
	var channel uint8

	err = store.Scan(func(recvTimeNS int64, frame []byte) bool {
		// Parse the CSI frame
		csiFrame, parseErr := ingestion.ParseFrame(frame)
		if parseErr != nil {
			t.Fatalf("Failed to parse CSI frame %d: %v", frameCount, parseErr)
		}

		// Update statistics
		frameCount++
		if firstTimestamp == 0 {
			firstTimestamp = recvTimeNS
		}
		lastTimestamp = recvTimeNS

		// Calculate total amplitude (sum of I²+Q²)
		for i := 0; i < len(csiFrame.Payload); i += 2 {
			if i+1 < len(csiFrame.Payload) {
				iVal := csiFrame.Payload[i]
				qVal := csiFrame.Payload[i+1]
				amplitude := float64(iVal*iVal + qVal*qVal)
				totalAmplitude += amplitude
			}
		}

		// Keep track of frame parameters
		nSub = csiFrame.NSub
		rssi = csiFrame.RSSI
		channel = csiFrame.Channel

		return true // continue scanning
	})

	if err != nil {
		panic(fmt.Sprintf("Error during scan: %v", err))
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
