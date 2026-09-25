package webhook

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// serverURLMarker in a test case's url field means "spin up a local
// httptest server and substitute its URL".
const serverURLMarker = "@server@"

func TestPostJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		client         *http.Client
		url            string
		headers        map[string]string
		body           string
		handlerStatus  int
		wantStatus     int
		wantErrText    string
		wantHTTPHeader map[string]string // header -> expected request-side value
	}{
		{
			name:          "posts body with default content type",
			url:           serverURLMarker,
			body:          `{"event":"fall"}`,
			handlerStatus: 200,
			wantStatus:    200,
			wantHTTPHeader: map[string]string{
				"Content-Type": "application/json",
			},
		},
		{
			name: "caller header overrides default content type",
			url:  serverURLMarker,
			body: "raw",
			headers: map[string]string{
				"Content-Type": "text/plain",
				"X-Secret":     "abc",
			},
			handlerStatus: 200,
			wantStatus:    200,
			wantHTTPHeader: map[string]string{
				"Content-Type": "text/plain",
				"X-Secret":     "abc",
			},
		},
		{
			name:          "handler 500 is reported as status, not error",
			url:           serverURLMarker,
			body:          `{}`,
			handlerStatus: 500,
			wantStatus:    500,
		},
		{
			name:        "unroutable url yields request-failed error",
			client:      &http.Client{Timeout: 2 * time.Second},
			url:         "http://127.0.0.1:1/hook", // port 1: nothing listens, fails fast
			body:        `{}`,
			wantErrText: "request failed:",
		},
		{
			name:        "unparseable url yields create-request error",
			url:         "http://192.168.0.%31",
			body:        `{}`,
			wantErrText: "create request:",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var gotHeaders http.Header
			var gotBody []byte
			url := tt.url
			if url == serverURLMarker {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					b, err := io.ReadAll(r.Body)
					if err != nil {
						t.Errorf("reading request body: %v", err)
					}
					gotBody = b
					gotHeaders = r.Header.Clone()
					w.WriteHeader(tt.handlerStatus)
				}))
				defer srv.Close()
				url = srv.URL
			}

			client := tt.client
			if client == nil {
				client = &http.Client{Timeout: 5 * time.Second}
			}

			gotStatus, latencyMs, err := PostJSON(client, url, tt.headers, []byte(tt.body))

			if tt.wantErrText != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErrText) {
					t.Fatalf("PostJSON() error = %v, want containing %q", err, tt.wantErrText)
				}
				return
			}
			if err != nil {
				t.Fatalf("PostJSON() unexpected error: %v", err)
			}
			if gotStatus != tt.wantStatus {
				t.Errorf("PostJSON() status = %d, want %d", gotStatus, tt.wantStatus)
			}
			if latencyMs < 0 {
				t.Errorf("PostJSON() latency = %d ms, want >= 0", latencyMs)
			}
			if string(gotBody) != tt.body {
				t.Errorf("request body = %q, want %q", gotBody, tt.body)
			}
			for k, want := range tt.wantHTTPHeader {
				if got := gotHeaders.Get(k); got != want {
					t.Errorf("request header %q = %q, want %q", k, got, want)
				}
			}
		})
	}
}

func TestPostJSONNilClientUsesDefault(t *testing.T) {
	t.Parallel()

	// A nil client must not panic: fall back to http.DefaultClient. Point the
	// request at a closed server so the call fails fast but deterministically.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	_, _, err := PostJSON(nil, srv.URL, nil, []byte(`{}`))
	if err == nil {
		t.Fatal("PostJSON(nil client) against a dead server: want error, got nil")
	}
	if !strings.Contains(err.Error(), "request failed:") {
		t.Errorf("PostJSON(nil client) error = %v, want transport failure", err)
	}
}
