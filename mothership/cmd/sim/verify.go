package main

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"time"
)

// CSVWriter writes ground truth data to a CSV file
type CSVWriter struct {
	file    *os.File
	writer  *csv.Writer
	created time.Time
}

// NewCSVWriter creates a new CSV writer
func NewCSVWriter(filename string) (*CSVWriter, error) {
	file, err := os.Create(filename)
	if err != nil {
		return nil, err
	}

	writer := csv.NewWriter(file)

	// Write header
	header := []string{
		"timestamp_ms",
		"walker_id",
		"x", "y", "z",
		"vx", "vy", "vz",
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

// WriteRow writes a row of ground truth data
func (w *CSVWriter) WriteRow(walkers []*Walker, nodes []*VirtualNode) {
	timestamp := time.Since(w.created).Milliseconds()

	for _, walker := range walkers {
		row := []string{
			fmt.Sprintf("%d", timestamp),
			fmt.Sprintf("%d", walker.ID),
			fmt.Sprintf("%.3f", walker.Position.X),
			fmt.Sprintf("%.3f", walker.Position.Y),
			fmt.Sprintf("%.3f", walker.Position.Z),
			fmt.Sprintf("%.3f", walker.Velocity.X),
			fmt.Sprintf("%.3f", walker.Velocity.Y),
			fmt.Sprintf("%.3f", walker.Velocity.Z),
		}

		if err := w.writer.Write(row); err != nil {
			fmt.Printf("[SIM] CSV write error: %v\n", err)
		}
	}
}

// Close flushes and closes the CSV file
func (w *CSVWriter) Close() error {
	w.writer.Flush()
	if err := w.file.Close(); err != nil {
		return err
	}
	return nil
}

// verifyWalkersInRoom checks if all walkers are within room bounds
func verifyWalkersInRoom(walkers []*Walker, space *Space) bool {
	for _, walker := range walkers {
		if walker.Position.X < 0 || walker.Position.X > space.Width {
			return false
		}
		if walker.Position.Y < 0 || walker.Position.Y > space.Depth {
			return false
		}
		if walker.Position.Z < 0 || walker.Position.Z > space.Height {
			return false
		}
	}
	return true
}

// computeBlobAccuracy checks if blobs are within expected distance of walkers
func computeBlobAccuracy(blobs []map[string]interface{}, walkers []*Walker) (bool, float64) {
	if len(blobs) == 0 || len(walkers) == 0 {
		return false, 0
	}

	maxDistance := 0.0
	matched := 0

	for _, blob := range blobs {
		blobX, _ := blob["x"].(float64)
		blobY, _ := blob["y"].(float64)
		blobZ, _ := blob["z"].(float64)

		minDist := math.MaxFloat64
		for _, walker := range walkers {
			dx := blobX - walker.Position.X
			dy := blobY - walker.Position.Y
			dz := blobZ - walker.Position.Z
			dist := math.Sqrt(dx*dx + dy*dy + dz*dz)
			if dist < minDist {
				minDist = dist
			}
		}

		if minDist <= 2.0 { // Within 2 meters
			matched++
		}
		if minDist > maxDistance {
			maxDistance = minDist
		}
	}

	// At least 50% of walkers should have a blob within 2m
	accuracy := float64(matched) / float64(len(walkers))
	return accuracy >= 0.5, maxDistance
}
