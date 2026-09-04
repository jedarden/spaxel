package github

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// ErrorKind classifies the failure an APIError describes, so a caller can
// branch on the class of problem rather than match on message text.
type ErrorKind string

const (
	// ErrorKindHTTP is a response the API sent with a non-2xx status.
	ErrorKindHTTP ErrorKind = "http"

	// ErrorKindRateLimit is a non-2xx response that reports the request quota
	// exhausted — a 403 carrying X-RateLimit-Remaining: 0. The same fields as
	// ErrorKindHTTP are populated, so a caller that only looks at the status
	// code still sees 403.
	ErrorKindRateLimit ErrorKind = "rate_limit"

	// ErrorKindParse is a 2xx response whose body would not decode into the
	// shape the method asked for.
	ErrorKindParse ErrorKind = "parse"

	// ErrorKindTransport is a failure before a complete response existed:
	// connection refused, DNS lookup failure, the configured timeout, or a
	// body that failed partway through being read.
	ErrorKindTransport ErrorKind = "transport"
)

// maxErrorBodyBytes caps how much of an erroring response body is kept in the
// APIError and rendered in its message. GitHub error bodies are small JSON
// documents in practice; the cap only bounds a pathological response.
const maxErrorBodyBytes = 4 << 10

// APIError is the error every failing client method returns, whether the
// request never completed, the API answered with an error status, or a body
// could not be parsed. Callers branch on Kind, or on the helpers
// IsNotFound/IsRateLimited/Temporary, instead of matching message text; the
// wrapped Err keeps the original cause reachable through errors.Is/As.
type APIError struct {
	// Kind is the class of failure — see the ErrorKind constants.
	Kind ErrorKind

	// Method and Path identify the request that failed. Path is the API path
	// only, never the base URL, so a configured token or host is not echoed
	// into logs.
	Method string
	Path   string

	// StatusCode is the HTTP status the API returned. It is 0 for a transport
	// failure, and the 2xx status on a parse failure.
	StatusCode int

	// Message is the human-readable message GitHub puts on its error payload
	// ("Not Found", "Bad credentials"). Empty when the body was not a GitHub
	// error object.
	Message string

	// DocumentationURL is the documentation_url field of GitHub's error
	// payload, when present.
	DocumentationURL string

	// Body is the response body the error was built from, truncated to
	// maxErrorBodyBytes. It is what Error() renders when the body carried no
	// message.
	Body string

	// RateLimit describes the quota window the response reported. GitHub
	// sends these headers on every response, exhausted or not.
	RateLimit RateLimit

	// Err is the underlying cause — the transport error or the JSON decode
	// error. Nil for errors built purely from a response.
	Err error
}

// RateLimit is the quota state GitHub reports in the X-RateLimit-* response
// headers. Reset is a Unix timestamp in seconds.
type RateLimit struct {
	Limit     int
	Remaining int
	Reset     int64
}

// apiErrorPayload is the error object GitHub returns on a failed request.
type apiErrorPayload struct {
	Message          string `json:"message"`
	DocumentationURL string `json:"documentation_url"`
}

// Error renders the failure with the request it belongs to and the cause the
// API reported. The status code and the response body are always part of the
// text, so a log line is enough to see what came back.
func (e *APIError) Error() string {
	var b strings.Builder
	b.WriteString("GitHub API")
	if loc := strings.TrimSpace(e.Method + " " + e.Path); loc != "" {
		b.WriteString(" " + loc)
	}

	switch e.Kind {
	case ErrorKindTransport:
		b.WriteString(" request failed")
	case ErrorKindParse:
		b.WriteString(" response could not be parsed")
	default:
		fmt.Fprintf(&b, " returned status %d", e.StatusCode)
		if e.RateLimit.Exhausted() {
			fmt.Fprintf(&b, " (rate limit exhausted: %d of %d requests remaining, resets %s)",
				e.RateLimit.Remaining, e.RateLimit.Limit,
				e.RateLimit.ResetsAt().UTC().Format(time.RFC3339))
		}
	}

	if e.Message != "" {
		fmt.Fprintf(&b, ": %s", e.Message)
	} else if e.Body != "" {
		fmt.Fprintf(&b, ": %s", e.Body)
	}
	if e.Err != nil {
		fmt.Fprintf(&b, ": %v", e.Err)
	}
	return b.String()
}

// Unwrap exposes the underlying transport or decode error to errors.Is and
// errors.As.
func (e *APIError) Unwrap() error { return e.Err }

// IsNotFound reports whether the API answered 404 — for the release methods,
// a repository that does not exist or a repository with no releases.
func (e *APIError) IsNotFound() bool {
	return e.Kind == ErrorKindHTTP && e.StatusCode == http.StatusNotFound
}

// IsRateLimited reports whether the failure is the request quota being
// exhausted rather than a refused request.
func (e *APIError) IsRateLimited() bool {
	return e.Kind == ErrorKindRateLimit
}

// Temporary reports whether retrying the same request later can succeed: the
// quota window rolls over, a 5xx is transient, and a transport failure often
// is. A 4xx other than 403/429 is a request problem and will fail again.
func (e *APIError) Temporary() bool {
	switch e.Kind {
	case ErrorKindRateLimit, ErrorKindTransport:
		return true
	case ErrorKindParse:
		return false
	}
	return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
}

// Exhausted reports whether the quota the response reported is used up.
func (r RateLimit) Exhausted() bool {
	return r.Remaining == 0 && r.Limit > 0
}

// ResetsAt converts the reset timestamp to a time. A response that carried no
// parsable reset header reports the zero time.
func (r RateLimit) ResetsAt() time.Time {
	if r.Reset == 0 {
		return time.Time{}
	}
	return time.Unix(r.Reset, 0)
}

// newHTTPError builds the APIError for a response the caller cannot proceed
// on. It reads and closes the body, and classifies a quota-exhausted 403 as
// ErrorKindRateLimit using the same header rule as the package-level
// IsRateLimited.
func newHTTPError(req *http.Request, resp *http.Response) *APIError {
	body, _ := drainBody(resp)

	remaining, reset, _ := GetRateLimitInfo(resp)
	kind := ErrorKindHTTP
	if IsRateLimited(resp) {
		kind = ErrorKindRateLimit
	}

	e := &APIError{
		Kind:       kind,
		Method:     req.Method,
		Path:       req.URL.Path,
		StatusCode: resp.StatusCode,
		RateLimit: RateLimit{
			Limit:     headerInt(resp.Header, "X-RateLimit-Limit"),
			Remaining: remaining,
			Reset:     reset,
		},
	}

	var payload apiErrorPayload
	if err := json.Unmarshal(body, &payload); err == nil {
		e.Message = payload.Message
		e.DocumentationURL = payload.DocumentationURL
	}
	e.Body = truncateBody(body)
	return e
}

// headerInt reads an integer response header. A value that does not parse as
// an integer reads as zero, matching GetRateLimitInfo's treatment of the
// remaining/reset headers.
func headerInt(h http.Header, name string) int {
	var v int
	if _, err := fmt.Sscanf(h.Get(name), "%d", &v); err != nil {
		return 0
	}
	return v
}

// truncateBody trims and caps a response body for storage in an APIError.
func truncateBody(body []byte) string {
	s := strings.TrimSpace(string(body))
	if len(s) > maxErrorBodyBytes {
		s = s[:maxErrorBodyBytes] + " ... (truncated)"
	}
	return s
}
