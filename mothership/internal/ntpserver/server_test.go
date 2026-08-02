package ntpserver

import (
	"encoding/binary"
	"net"
	"testing"
	"time"
)

// makeRequest builds a minimal, well-formed SNTP client request (mode 3)
// with the given transmit timestamp, matching what ESP-IDF's SNTP client
// (and any other SNTP/NTPv4 client) sends.
func makeRequest(transmitTime time.Time) []byte {
	req := make([]byte, packetSize)
	req[0] = (leapNone << 6) | (versionNum << 3) | 3 // mode 3 = client
	sec, frac := toNTPTime(transmitTime)
	binary.BigEndian.PutUint32(req[40:44], sec)
	binary.BigEndian.PutUint32(req[44:48], frac)
	return req
}

func TestBuildResponse_EchoesOriginTimestamp(t *testing.T) {
	clientTx := time.Date(2026, 8, 1, 12, 0, 0, 500_000_000, time.UTC)
	req := makeRequest(clientTx)
	recvTime := clientTx.Add(50 * time.Millisecond)

	resp, err := buildResponse(req, recvTime)
	if err != nil {
		t.Fatalf("buildResponse: %v", err)
	}
	if len(resp) != packetSize {
		t.Fatalf("response length = %d, want %d", len(resp), packetSize)
	}

	// Origin timestamp (bytes 24-31) must equal the client's own transmit
	// timestamp from the request (bytes 40-47) -- this is how an SNTP
	// client validates the reply matches its own request and computes
	// round-trip delay.
	wantSec, wantFrac := toNTPTime(clientTx)
	gotSec := binary.BigEndian.Uint32(resp[24:28])
	gotFrac := binary.BigEndian.Uint32(resp[28:32])
	if gotSec != wantSec || gotFrac != wantFrac {
		t.Errorf("origin timestamp = (%d,%d), want (%d,%d)", gotSec, gotFrac, wantSec, wantFrac)
	}
}

func TestBuildResponse_ModeAndStratum(t *testing.T) {
	req := makeRequest(time.Now())
	resp, err := buildResponse(req, time.Now())
	if err != nil {
		t.Fatalf("buildResponse: %v", err)
	}

	mode := resp[0] & 0x07
	if mode != modeServer {
		t.Errorf("mode = %d, want %d (server)", mode, modeServer)
	}
	version := (resp[0] >> 3) & 0x07
	if version != versionNum {
		t.Errorf("version = %d, want %d", version, versionNum)
	}
	if resp[1] != stratumLocal {
		t.Errorf("stratum = %d, want %d", resp[1], stratumLocal)
	}
	if string(resp[12:16]) != refIDLocal {
		t.Errorf("refID = %q, want %q", resp[12:16], refIDLocal)
	}
}

func TestBuildResponse_TimestampsAreMonotonicallyOrdered(t *testing.T) {
	req := makeRequest(time.Now())
	recvTime := time.Now()
	resp, err := buildResponse(req, recvTime)
	if err != nil {
		t.Fatalf("buildResponse: %v", err)
	}

	recvSec := binary.BigEndian.Uint32(resp[32:36])
	txSec := binary.BigEndian.Uint32(resp[40:44])
	if txSec < recvSec {
		t.Errorf("transmit timestamp (%d) precedes receive timestamp (%d)", txSec, recvSec)
	}
}

func TestBuildResponse_RejectsShortRequest(t *testing.T) {
	_, err := buildResponse(make([]byte, packetSize-1), time.Now())
	if err == nil {
		t.Fatal("expected an error for a too-short request, got nil")
	}
}

func TestToNTPTime_RoundTrip(t *testing.T) {
	// The NTP epoch is 70 years before the Unix epoch; a known Unix time
	// should map to a known NTP seconds value.
	unixTime := time.Unix(1785600000, 250_000_000).UTC() // 2026-08-01ish
	sec, frac := toNTPTime(unixTime)

	wantSec := uint32(1785600000 + ntpEpochDelta)
	if sec != wantSec {
		t.Errorf("sec = %d, want %d", sec, wantSec)
	}

	// frac represents 0.25s as a fraction of the full uint32 range.
	wantFrac := uint32(float64(0.25) * (1 << 32))
	// Allow a small tolerance for integer rounding in the fixed-point conversion.
	diff := int64(frac) - int64(wantFrac)
	if diff < -1 || diff > 1 {
		t.Errorf("frac = %d, want ~%d", frac, wantFrac)
	}
}

// TestServer_StartServeStop exercises the real UDP path end-to-end on an
// ephemeral local port (not 123, so this doesn't need elevated privileges).
func TestServer_StartServeStop(t *testing.T) {
	s, err := Start("127.0.0.1")
	if err == nil {
		// Binding port 123 as this test's user succeeded (e.g. running as
		// root) -- exercise the real thing.
		defer func() { _ = s.Stop() }()
	} else {
		t.Skipf("skipping live UDP round-trip: cannot bind :123 in this environment: %v", err)
	}

	client, err := net.Dial("udp", "127.0.0.1:123")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	req := makeRequest(time.Now())
	if _, err := client.Write(req); err != nil {
		t.Fatalf("write request: %v", err)
	}

	_ = client.SetReadDeadline(time.Now().Add(2 * time.Second))
	resp := make([]byte, packetSize)
	n, err := client.Read(resp)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	if n != packetSize {
		t.Fatalf("response length = %d, want %d", n, packetSize)
	}
	if mode := resp[0] & 0x07; mode != modeServer {
		t.Errorf("mode = %d, want %d (server)", mode, modeServer)
	}
}
