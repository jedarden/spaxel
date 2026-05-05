package acceptance

import (
	"context"
	"encoding/json"
	"net/http"
)

// Helper functions shared across acceptance tests

// checkMothershipHealth verifies the mothership is running.
func checkMothershipHealth(ctx context.Context, baseURL string) bool {
	req, _ := http.NewRequestWithContext(ctx, "GET", baseURL+"/healthz", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// getBlobsResponse fetches the current blobs from the API.
func getBlobsResponse(t testingT, baseURL string) []map[string]interface{} {
	t.Helper()

	resp, err := http.Get(baseURL + "/api/blobs")
	if err != nil {
		t.Logf("Failed to get blobs: %v", err)
		return []map[string]interface{}{}
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	blobs, _ := result["blobs"].([]map[string]interface{})
	return blobs
}

// getNodesResponse fetches the current nodes from the API.
func getNodesResponse(t testingT, baseURL string) []map[string]interface{} {
	t.Helper()

	resp, err := http.Get(baseURL + "/api/nodes")
	if err != nil {
		t.Logf("Failed to get nodes: %v", err)
		return []map[string]interface{}{}
	}
	defer resp.Body.Close()

	var result []map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	return result
}

// getEventsByType fetches events of a specific type.
func getEventsByType(t testingT, baseURL, eventType string) []map[string]interface{} {
	t.Helper()

	resp, err := http.Get(baseURL + "/api/events?type=" + eventType)
	if err != nil {
		t.Logf("Failed to get events: %v", err)
		return nil
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	events, _ := result["events"].([]map[string]interface{})
	return events
}

// testingT is a subset of testing.T interface for helpers.
type testingT interface {
	Helper()
	Logf(string, ...interface{})
	Log(...interface{})
}

// blobsResponse represents the /api/blobs response.
type blobsResponse struct {
	Blobs []map[string]interface{} `json:"blobs"`
}
