// Package main provides verification logic for the CSI simulator.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"time"
)

// VerificationResult represents the result of a verification check
type VerificationResult struct {
	Passed       bool      `json:"passed"`
	Message      string    `json:"message"`
	BlobCount    int       `json:"blob_count"`
	WalkerCount  int       `json:"walker_count"`
	ExpectedBlobs int      `json:"expected_blobs"`
	BlobDetails  []Blob    `json:"blob_details"`
	Timestamp    time.Time `json:"timestamp"`
}

// Blob represents a detected blob from the mothership API
type Blob struct {
	ID         int     `json:"id"`
	X          float64 `json:"x"`
	Y          float64 `json:"y"`
	Z          float64 `json:"z"`
	Confidence float64 `json:"confidence"`
	Person     string  `json:"person,omitempty"`
}

// Verifier handles blob count verification
type Verifier struct {
	mothershipURL string
	httpClient    *http.Client
	tolerance     int // ±1 tolerance for blob count
	distance      float64 // Max distance for walker-blob matching (meters)
}

// NewVerifier creates a new verifier
func NewVerifier(mothershipURL string) *Verifier {
	return &Verifier{
		mothershipURL: mothershipURL,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		tolerance: 1,
		distance:  2.0,
	}
}

// SetTolerance sets the blob count tolerance
func (v *Verifier) SetTolerance(tolerance int) {
	v.tolerance = tolerance
}

// SetDistance sets the max distance for walker-blob matching
func (v *Verifier) SetDistance(distance float64) {
	v.distance = distance
}

// Verify performs the verification check
func (v *Verifier) Verify(walkerCount int, walkerPositions [][3]float64) (*VerificationResult, error) {
	// Wait 2 seconds for pipeline to settle
	time.Sleep(2 * time.Second)

	// Fetch current blobs from mothership
	blobs, err := v.fetchBlobs()
	if err != nil {
		return nil, fmt.Errorf("failed to fetch blobs: %w", err)
	}

	result := &VerificationResult{
		BlobCount:    len(blobs),
		WalkerCount:  walkerCount,
		ExpectedBlobs: walkerCount,
		BlobDetails:  blobs,
		Timestamp:    time.Now(),
	}

	// Check blob count against walker count (with tolerance)
	minExpected := walkerCount - v.tolerance
	maxExpected := walkerCount + v.tolerance

	if len(blobs) < minExpected {
		result.Passed = false
		result.Message = fmt.Sprintf("FAIL: expected %d±%d blobs, got %d",
			walkerCount, v.tolerance, len(blobs))
		return result, nil
	}

	if len(blobs) > maxExpected {
		result.Passed = false
		result.Message = fmt.Sprintf("FAIL: expected %d±%d blobs, got %d",
			walkerCount, v.tolerance, len(blobs))
		return result, nil
	}

	// If walkers are within room bounds, check all walkers have a blob within distance
	if v.allWalkersInBounds(walkerPositions) {
		if !v.walkersHaveNearbyBlobs(walkerPositions, blobs) {
			result.Passed = false
			result.Message = fmt.Sprintf("FAIL: %d blobs detected but not all walkers within %.1fm",
				len(blobs), v.distance)
			return result, nil
		}
	}

	result.Passed = true
	result.Message = fmt.Sprintf("PASS: %d blobs detected for %d walkers",
		len(blobs), walkerCount)

	return result, nil
}

// fetchBlobs fetches the current list of blobs from the mothership
func (v *Verifier) fetchBlobs() ([]Blob, error) {
	// Convert ws:// URL to http:// for REST API
	apiURL := wsToHTTP(v.mothershipURL) + "/api/blobs"

	resp, err := v.httpClient.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var blobs []Blob
	if err := json.Unmarshal(body, &blobs); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return blobs, nil
}

// allWalkersInBounds checks if all walkers are within room bounds
func (v *Verifier) allWalkersInBounds(positions [][3]float64) bool {
	// Assume room bounds from space dimensions
	// Default bounds: 0-6m X, 0-5m Y, 0-2.5m Z
	for _, pos := range positions {
		if pos[0] < 0 || pos[0] > 6 || pos[1] < 0 || pos[1] > 5 || pos[2] < 0 || pos[2] > 2.5 {
			return false
		}
	}
	return true
}

// walkersHaveNearbyBlobs checks if each walker has a blob within distance
func (v *Verifier) walkersHaveNearbyBlobs(walkerPositions [][3]float64, blobs []Blob) bool {
	for _, walkerPos := range walkerPositions {
		hasNearbyBlob := false
		for _, blob := range blobs {
			dist := distance3D(walkerPos, [3]float64{blob.X, blob.Y, blob.Z})
			if dist <= v.distance {
				hasNearbyBlob = true
				break
			}
		}
		if !hasNearbyBlob {
			return false
		}
	}
	return true
}

// distance3D computes 3D Euclidean distance
func distance3D(a, b [3]float64) float64 {
	dx := a[0] - b[0]
	dy := a[1] - b[1]
	dz := a[2] - b[2]
	return math.Sqrt(dx*dx + dy*dy + dz*dz)
}

// wsToHTTP converts a WebSocket URL to HTTP URL
func wsToHTTP(wsURL string) string {
	// Simple string replacement
	// ws://localhost:8080/ws/node -> http://localhost:8080
	if len(wsURL) >= 5 && wsURL[:5] == "ws://" {
		return "http://" + wsURL[5:]
	}
	if len(wsURL) >= 6 && wsURL[:6] == "wss://" {
		return "https://" + wsURL[6:]
	}

	// If URL doesn't start with ws:// or wss://, assume it's already http/https
	return wsURL
}

// PrintResult prints the verification result to stdout
func (v *Verifier) PrintResult(result *VerificationResult) {
	fmt.Printf("\n=== Verification Result ===\n")
	fmt.Printf("Status: %s\n", result.Message)
	fmt.Printf("Blob count: %d\n", result.BlobCount)
	fmt.Printf("Walker count: %d\n", result.WalkerCount)
	fmt.Printf("Expected blobs: %d±%d\n", result.WalkerCount, v.tolerance)
	fmt.Printf("Timestamp: %s\n", result.Timestamp.Format(time.RFC3339))

	if len(result.BlobDetails) > 0 {
		fmt.Printf("\nBlob details:\n")
		for _, blob := range result.BlobDetails {
			person := blob.Person
			if person == "" {
				person = "Unknown"
			}
			fmt.Printf("  ID %d: (%.2f, %.2f, %.2f) confidence=%.2f person=%s\n",
				blob.ID, blob.X, blob.Y, blob.Z, blob.Confidence, person)
		}
	}
	fmt.Printf("============================\n\n")
}

// ExitWithResult exits with appropriate exit code based on verification result
func (v *Verifier) ExitWithResult(result *VerificationResult) {
	v.PrintResult(result)

	if result.Passed {
		os.Exit(0)
	}
	os.Exit(1)
}

// RunVerification runs the full verification workflow
func RunVerification(mothershipURL string, walkerCount int, walkerPositions [][3]float64, tolerance int) error {
	verifier := NewVerifier(mothershipURL)
	verifier.SetTolerance(tolerance)

	result, err := verifier.Verify(walkerCount, walkerPositions)
	if err != nil {
		return fmt.Errorf("verification failed: %w", err)
	}

	verifier.ExitWithResult(result)
	return nil
}

// ProvisionToken attempts to provision a new token from the mothership
func ProvisionToken(mothershipURL string) (string, error) {
	apiURL := wsToHTTP(mothershipURL) + "/api/provision"

	// Create request body with synthetic credentials
	reqBody := []byte(`{"mac":"AA:BB:CC:DD:EE:FF"}`)

	resp, err := http.Post(apiURL, "application/json", nil)
	if err != nil {
		return "", fmt.Errorf("provision request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("provision returned HTTP %d: %s", resp.StatusCode, string(body))
	}

	// Read response - this should return the token
	token, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read token: %w", err)
	}

	return string(token), nil
}

// GetMothershipStatus fetches the mothership status
func GetMothershipStatus(mothershipURL string) (map[string]interface{}, error) {
	apiURL := wsToHTTP(mothershipURL) + "/api/status"

	resp, err := http.Get(apiURL)
	if err != nil {
		return nil, fmt.Errorf("status request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status returned HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read status: %w", err)
	}

	var status map[string]interface{}
	if err := json.Unmarshal(body, &status); err != nil {
		return nil, fmt.Errorf("failed to parse status: %w", err)
	}

	return status, nil
}
