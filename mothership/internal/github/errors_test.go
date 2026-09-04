// Tests for the typed error surface: how a failure is classified into an
// ErrorKind, what the APIError carries, and what callers can branch on without
// matching message text.
package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestAPIErrorMessage covers the rendering of each error kind. The status
// code, the request it belongs to, and the API's own message are all part of
// the text so a log line is enough to see what came back.
func TestAPIErrorMessage(t *testing.T) {
	tests := []struct {
		name    string
		err     *APIError
		wantSub []string
	}{
		{
			name: "http error carries method, path and status",
			err: &APIError{
				Kind:       ErrorKindHTTP,
				Method:     http.MethodGet,
				Path:       "/repos/o/r/releases",
				StatusCode: http.StatusNotFound,
				Message:    "Not Found",
			},
			wantSub: []string{"GET /repos/o/r/releases", "404", "Not Found"},
		},
		{
			name: "rate limit error reports the exhausted window",
			err: &APIError{
				Kind:       ErrorKindRateLimit,
				Method:     http.MethodGet,
				Path:       "/repos/o/r/releases",
				StatusCode: http.StatusForbidden,
				RateLimit:  RateLimit{Limit: 60, Remaining: 0, Reset: 1700000000},
			},
			wantSub: []string{"403", "rate limit exhausted", "0 of 60 requests remaining"},
		},
		{
			name: "parse error names the offending path",
			err: &APIError{
				Kind:       ErrorKindParse,
				Method:     http.MethodGet,
				Path:       "/repos/o/r/releases",
				StatusCode: http.StatusOK,
				Body:       "<html>bad gateway</html>",
			},
			wantSub: []string{"could not be parsed", "<html>bad gateway</html>"},
		},
		{
			name: "transport error wraps the underlying cause",
			err: &APIError{
				Kind:   ErrorKindTransport,
				Method: http.MethodGet,
				Path:   "/repos/o/r/releases",
				Err:    context.DeadlineExceeded,
			},
			wantSub: []string{"request failed", "context deadline exceeded"},
		},
		{
			name: "body is rendered when the payload has no message",
			err: &APIError{
				Kind:       ErrorKindHTTP,
				StatusCode: http.StatusInternalServerError,
				Body:       "upstream exploded",
			},
			wantSub: []string{"500", "upstream exploded"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			for _, sub := range tt.wantSub {
				if !strings.Contains(got, sub) {
					t.Errorf("Error() = %q, want it to contain %q", got, sub)
				}
			}
		})
	}
}

// TestAPIErrorClassification covers the helpers a caller branches on, per
// kind, instead of matching message text.
func TestAPIErrorClassification(t *testing.T) {
	tests := []struct {
		name          string
		err           *APIError
		wantNotFound  bool
		wantRateLimit bool
		wantTemporary bool
	}{
		{name: "404 is not found", err: &APIError{Kind: ErrorKindHTTP, StatusCode: http.StatusNotFound},
			wantNotFound: true},
		{name: "403 without exhausted quota is not rate limited", err: &APIError{Kind: ErrorKindHTTP, StatusCode: http.StatusForbidden}},
		{name: "exhausted quota is rate limited and temporary",
			err:           &APIError{Kind: ErrorKindRateLimit, StatusCode: http.StatusForbidden, RateLimit: RateLimit{Limit: 60}},
			wantRateLimit: true, wantTemporary: true},
		{name: "parse failure is not temporary", err: &APIError{Kind: ErrorKindParse, StatusCode: http.StatusOK}},
		{name: "transport failure is temporary", err: &APIError{Kind: ErrorKindTransport},
			wantTemporary: true},
		{name: "5xx is temporary", err: &APIError{Kind: ErrorKindHTTP, StatusCode: http.StatusBadGateway},
			wantTemporary: true},
		{name: "429 is temporary", err: &APIError{Kind: ErrorKindHTTP, StatusCode: http.StatusTooManyRequests},
			wantTemporary: true},
		{name: "4xx is not temporary", err: &APIError{Kind: ErrorKindHTTP, StatusCode: http.StatusUnauthorized}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.IsNotFound(); got != tt.wantNotFound {
				t.Errorf("IsNotFound() = %v, want %v", got, tt.wantNotFound)
			}
			if got := tt.err.IsRateLimited(); got != tt.wantRateLimit {
				t.Errorf("IsRateLimited() = %v, want %v", got, tt.wantRateLimit)
			}
			if got := tt.err.Temporary(); got != tt.wantTemporary {
				t.Errorf("Temporary() = %v, want %v", got, tt.wantTemporary)
			}
		})
	}
}

// TestAPIErrorUnwrap keeps the original cause reachable, so a caller can use
// errors.Is on sentinel errors such as context.DeadlineExceeded.
func TestAPIErrorUnwrap(t *testing.T) {
	sentinel := context.DeadlineExceeded
	err := error(&APIError{Kind: ErrorKindTransport, Err: sentinel})

	if !errors.Is(err, sentinel) {
		t.Errorf("errors.Is(err, %v) = false, want true", sentinel)
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatal("errors.As(err, *APIError) = false, want true")
	}
	if apiErr.Kind != ErrorKindTransport {
		t.Errorf("Kind = %q, want %q", apiErr.Kind, ErrorKindTransport)
	}
}

// TestRateLimitFields covers the quota helpers the error message and callers
// rely on.
func TestRateLimitFields(t *testing.T) {
	tests := []struct {
		name          string
		rl            RateLimit
		wantExhausted bool
		wantReset     time.Time
	}{
		{
			name:          "zero remaining with a limit is exhausted",
			rl:            RateLimit{Limit: 60, Remaining: 0, Reset: 1700000000},
			wantExhausted: true,
			wantReset:     time.Unix(1700000000, 0),
		},
		{
			name:      "remaining budget is not exhausted",
			rl:        RateLimit{Limit: 60, Remaining: 1, Reset: 1700000000},
			wantReset: time.Unix(1700000000, 0),
		},
		{
			name: "no reported limit is not exhausted",
			rl:   RateLimit{Remaining: 0},
		},
		{
			name:      "absent reset header reports the zero time",
			rl:        RateLimit{Limit: 60, Remaining: 59},
			wantReset: time.Time{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.rl.Exhausted(); got != tt.wantExhausted {
				t.Errorf("Exhausted() = %v, want %v", got, tt.wantExhausted)
			}
			if got := tt.rl.ResetsAt(); !got.Equal(tt.wantReset) {
				t.Errorf("ResetsAt() = %v, want %v", got, tt.wantReset)
			}
		})
	}
}

// TestGetJSONErrorKinds drives the typed methods against responses that fail
// in each distinct way and asserts the kind that comes back, so the
// classification is checked end to end rather than only on hand-built errors.
func TestGetJSONErrorKinds(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		headers     map[string]string
		body        string
		wantKind    ErrorKind
		wantStatus  int
		wantMessage string
	}{
		{
			name:        "not found is an http error",
			status:      http.StatusNotFound,
			body:        `{"message":"Not Found","documentation_url":"https://docs.github.com/rest"}`,
			wantKind:    ErrorKindHTTP,
			wantStatus:  http.StatusNotFound,
			wantMessage: "Not Found",
		},
		{
			name:        "exhausted quota is a rate limit error",
			status:      http.StatusForbidden,
			headers:     map[string]string{"X-RateLimit-Remaining": "0", "X-RateLimit-Limit": "60", "X-RateLimit-Reset": "1700000000"},
			body:        `{"message":"API rate limit exceeded"}`,
			wantKind:    ErrorKindRateLimit,
			wantStatus:  http.StatusForbidden,
			wantMessage: "API rate limit exceeded",
		},
		{
			name:       "unparsable success body is a parse error",
			status:     http.StatusOK,
			body:       `<html>not json</html>`,
			wantKind:   ErrorKindParse,
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for k, v := range tt.headers {
					w.Header().Set(k, v)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			client := NewClientFromConfig(GitHubConfig{BaseURL: srv.URL, Timeout: time.Second})
			_, err := client.GetReleases(context.Background(), "o", "r")
			if err == nil {
				t.Fatal("GetReleases returned nil error, want an *APIError")
			}

			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("error is %T (%v), want *APIError", err, err)
			}
			if apiErr.Kind != tt.wantKind {
				t.Errorf("Kind = %q, want %q", apiErr.Kind, tt.wantKind)
			}
			if apiErr.StatusCode != tt.wantStatus {
				t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, tt.wantStatus)
			}
			if apiErr.Message != tt.wantMessage {
				t.Errorf("Message = %q, want %q", apiErr.Message, tt.wantMessage)
			}
			if apiErr.Path != "/repos/o/r/releases" {
				t.Errorf("Path = %q, want %q", apiErr.Path, "/repos/o/r/releases")
			}
		})
	}
}

// TestGetJSONRateLimitFields checks the quota headers survive onto the error,
// so a caller can back off until ResetsAt rather than retry immediately.
func TestGetJSONRateLimitFields(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Reset", "1700000000")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded for 1.2.3.4"}`))
	}))
	defer srv.Close()

	client := NewClientFromConfig(GitHubConfig{BaseURL: srv.URL, Timeout: time.Second})
	_, err := client.GetReleases(context.Background(), "o", "r")

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is %T (%v), want *APIError", err, err)
	}
	if !apiErr.IsRateLimited() {
		t.Errorf("IsRateLimited() = false, want true (err = %v)", err)
	}
	if !apiErr.Temporary() {
		t.Error("Temporary() = false, want true for an exhausted quota")
	}
	if got, want := apiErr.RateLimit, (RateLimit{Limit: 5000, Remaining: 0, Reset: 1700000000}); got != want {
		t.Errorf("RateLimit = %+v, want %+v", got, want)
	}
	if apiErr.RateLimit.ResetsAt().Unix() != 1700000000 {
		t.Errorf("ResetsAt().Unix() = %d, want %d", apiErr.RateLimit.ResetsAt().Unix(), int64(1700000000))
	}
}

// TestGetJSONTransportErrorKind keeps transport failures distinct from HTTP
// failures — a caller retrying a refused connection must not treat it as a
// 4xx.
func TestGetJSONTransportErrorKind(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))
	url := srv.URL
	srv.Close() // the port is now closed, so the request cannot be delivered

	client := NewClientFromConfig(GitHubConfig{BaseURL: url, Timeout: time.Second})
	_, err := client.GetReleases(context.Background(), "o", "r")
	if err == nil {
		t.Fatal("GetReleases returned nil error against a closed server")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is %T (%v), want *APIError", err, err)
	}
	if apiErr.Kind != ErrorKindTransport {
		t.Errorf("Kind = %q, want %q", apiErr.Kind, ErrorKindTransport)
	}
	if !apiErr.Temporary() {
		t.Error("Temporary() = false, want true for a transport failure")
	}
}

// TestTransportErrorExposesSentinel checks the configured timeout stays
// reachable through the typed error, which the existing "Client.Timeout"
// contract in TestGetTransportErrors depends on.
func TestTransportErrorExposesSentinel(t *testing.T) {
	client := NewClientFromConfig(GitHubConfig{BaseURL: "http://127.0.0.1:1", Timeout: 50 * time.Millisecond})
	_, err := client.Get(context.Background(), "/slow")
	if err == nil {
		t.Fatal("Get returned nil error against an unroutable port")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("error is %T (%v), want *APIError", err, err)
	}
	if apiErr.Err == nil {
		t.Error("Err is nil, want the wrapped transport failure")
	}
	if !strings.Contains(err.Error(), "GET /slow") {
		t.Errorf("Error() = %q, want it to name the request", err.Error())
	}
}

// TestTruncateBodyBoundsErrorPayloads bounds what a pathological response can
// store in, and render through, an APIError.
func TestTruncateBodyBoundsErrorPayloads(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		wantMax  int
		wantTail string
	}{
		{name: "short body is kept verbatim", body: []byte("  {\"ok\":true}  "), wantMax: len(`{"ok":true}`)},
		{name: "oversized body is capped", body: []byte(strings.Repeat("x", maxErrorBodyBytes+100)),
			wantMax: maxErrorBodyBytes + len(" ... (truncated)"), wantTail: " ... (truncated)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateBody(tt.body)
			if len(got) > tt.wantMax {
				t.Errorf("truncateBody() length = %d, want <= %d", len(got), tt.wantMax)
			}
			if tt.wantTail != "" && !strings.HasSuffix(got, tt.wantTail) {
				t.Errorf("truncateBody() = %q, want it to end with %q", got, tt.wantTail)
			}
		})
	}
}
