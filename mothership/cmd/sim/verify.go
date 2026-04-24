package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// CSVWriter writes ground truth data to a CSV file
type CSVWriter struct {
	file    *os.File
	writer  *csv.Writer
	created time.Time
}

// NewCSVWriter creates a new CSV writer with headers for walker positions and link deltaRMS
func NewCSVWriter(filename string) (*CSVWriter, error) {
	file, err := os.Create(filename)
	if err != nil {
		return nil, err
	}

	writer := csv.NewWriter(file)

	header := []string{
		"timestamp_ms",
		"walker_id",
		"x", "y", "z",
		"vx", "vy", "vz",
		"link_id",
		"delta_rms",
	}
	if err := writer.Write(header); err != nil {
		file.Close()
		return nil, err
	}

	return &CSVWriter{
		file:    file,
		writer:  writer,
		created: time.Now(),
	}, nil
}

// WriteRow writes a row of ground truth data including walker positions and per-link deltaRMS
func (w *CSVWriter) WriteRow(walkers []*Walker, nodes []*VirtualNode, walls []Wall) {
	timestamp := time.Since(w.created).Milliseconds()

	for _, walker := range walkers {
		// Write walker position row (no link-specific data)
		row := []string{
			fmt.Sprintf("%d", timestamp),
			fmt.Sprintf("%d", walker.ID),
			fmt.Sprintf("%.3f", walker.Position.X),
			fmt.Sprintf("%.3f", walker.Position.Y),
			fmt.Sprintf("%.3f", walker.Position.Z),
			fmt.Sprintf("%.3f", walker.Velocity.X),
			fmt.Sprintf("%.3f", walker.Velocity.Y),
			fmt.Sprintf("%.3f", walker.Velocity.Z),
			"",     // link_id — empty for position-only rows
			"",     // delta_rms — empty for position-only rows
		}
		if err := w.writer.Write(row); err != nil {
			log.Printf("[SIM] CSV write error: %v", err)
		}

		// Write deltaRMS rows for each node pair link
		for _, tx := range nodes {
			for _, rx := range nodes {
				if tx.ID >= rx.ID {
					continue // avoid duplicate link pairs
				}
				deltaRMS := computeWalkerDeltaRMS(tx.Position, rx.Position, walker.Position)
				linkID := fmt.Sprintf("%s:%s", macToString(tx.MAC), macToString(rx.MAC))
				linkRow := []string{
					fmt.Sprintf("%d", timestamp),
					fmt.Sprintf("%d", walker.ID),
					fmt.Sprintf("%.3f", walker.Position.X),
					fmt.Sprintf("%.3f", walker.Position.Y),
					fmt.Sprintf("%.3f", walker.Position.Z),
					"", "", "", // velocity empty for link rows
					linkID,
					fmt.Sprintf("%.6f", deltaRMS),
				}
				if err := w.writer.Write(linkRow); err != nil {
					log.Printf("[SIM] CSV write error: %v", err)
				}
			}
		}
	}
}

// Close flushes and closes the CSV file
func (w *CSVWriter) Close() error {
	w.writer.Flush()
	return w.file.Close()
}

// verifyBlobs verifies that the mothership detected the expected number of blobs.
// It queries GET /api/blobs and checks blob_count == walker_count within ±1 tolerance.
func verifyBlobs(expectedWalkers int, walkers []*Walker, space *Space) error {
	wsURL, err := url.Parse(*flagMothership)
	if err != nil {
		return fmt.Errorf("invalid mothership URL: %w", err)
	}

	httpURL := *wsURL
	if httpURL.Scheme == "ws" {
		httpURL.Scheme = "http"
	} else if httpURL.Scheme == "wss" {
		httpURL.Scheme = "https"
	}

	log.Printf("[SIM] Waiting 2 seconds for pipeline to settle...")
	time.Sleep(2 * time.Second)

	blobsURL := strings.TrimSuffix(httpURL.String(), "/ws")
	blobsURL = strings.TrimSuffix(blobsURL, "/")
	blobsURL += "/api/blobs"

	resp, err := http.Get(blobsURL)
	if err != nil {
		return fmt.Errorf("failed to query blobs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("blobs API returned status %d: %s", resp.StatusCode, string(body))
	}

	var blobs []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&blobs); err != nil {
		return fmt.Errorf("failed to decode blobs response: %w", err)
	}

	blobCount := len(blobs)

	// Check blob count within ±1 tolerance
	tolerance := 1
	minExpected := expectedWalkers - tolerance
	maxExpected := expectedWalkers + tolerance

	if blobCount < minExpected || blobCount > maxExpected {
		return fmt.Errorf("FAIL: expected %d blobs (±%d), got %d", expectedWalkers, tolerance, blobCount)
	}

	// If walkers are in room bounds, check each walker has a blob within 2m
	if allWalkersInRoom(walkers, space) && len(blobs) > 0 {
		matched := 0
		for _, walker := range walkers {
			for _, blob := range blobs {
				bx, _ := blob["x"].(float64)
				by, _ := blob["y"].(float64)
				bz, _ := blob["z"].(float64)
				dx := walker.Position.X - bx
				dy := walker.Position.Y - by
				dz := walker.Position.Z - bz
				if math.Sqrt(dx*dx+dy*dy+dz*dz) <= 2.0 {
					matched++
					break
				}
			}
		}
		log.Printf("[SIM] %d/%d walkers have a blob within 2m", matched, len(walkers))
	}

	log.Printf("[SIM] PASS: %d blobs detected for %d walkers", blobCount, expectedWalkers)
	return nil
}

// allWalkersInRoom checks if all walkers are within room bounds
func allWalkersInRoom(walkers []*Walker, space *Space) bool {
	for _, w := range walkers {
		if w.Position.X < 0 || w.Position.X > space.Width ||
			w.Position.Y < 0 || w.Position.Y > space.Depth ||
			w.Position.Z < 0 || w.Position.Z > space.Height {
			return false
		}
	}
	return true
}
