package webhook

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"
)

// PostJSON sends one JSON HTTP POST against a user-configured webhook URL and
// reports the response status code and request latency.
//
// This is the shared low-level egress point for user-configured webhook
// integrations: the event Publisher in this package and the trigger-volume
// webhook actions relayed from internal/api. The URL always originates from
// user configuration — a trigger action or a notification channel — so the
// mothership only ever connects to a destination the operator explicitly
// pointed it at. Code outside these user-configured integration packages must
// not open outbound connections at all (enforced by internal/privacy).
//
// client must be non-nil; the caller owns its timeout policy. headers are set
// after the default Content-Type, so a caller-supplied header can override it.
// Error strings preserve the historical contract: "create request: ..." when
// the request cannot be built, "request failed: ..." on transport failure.
func PostJSON(client *http.Client, url string, headers map[string]string, body []byte) (statusCode int, latencyMs int64, err error) {
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, 0, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Add caller-supplied headers (trigger action params / channel config)
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := client.Do(req)
	latencyMs = time.Since(start).Milliseconds()

	if err != nil {
		return 0, latencyMs, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	// Drain body to allow connection reuse
	_, _ = io.Copy(io.Discard, resp.Body) //nolint:errcheck // best-effort drain

	return resp.StatusCode, latencyMs, nil
}
