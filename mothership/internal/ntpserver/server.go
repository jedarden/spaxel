// Package ntpserver implements a minimal SNTP (RFC 4330) responder so nodes
// on an internet-isolated deployment can still get wall-clock time from the
// mothership itself, instead of an unreachable pool.ntp.org.
//
// This is deliberately not a full NTP peer implementation: no clock
// discipline, no upstream synchronization, no leap-second table. It answers
// every request with the host's own system clock, stamped as stratum 10
// ("locally significant, not verified against an external reference") --
// the same convention chrony's and ntpd's undisciplined-local-clock drivers
// use. Spaxel's own CSI timestamps are already monotonic and don't depend on
// this; it exists purely so an internet-isolated deployment gets sane
// wall-clock timestamps in the dashboard and event log instead of a node
// clock that never syncs at all.
package ntpserver

import (
	"context"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"time"
)

const (
	packetSize    = 48
	ntpEpochDelta = 2208988800 // seconds between the NTP epoch (1900-01-01) and the Unix epoch (1970-01-01)
	leapNone      = 0
	versionNum    = 4
	modeServer    = 4
	stratumLocal  = 10     // "locally significant, not synchronized to an external reference"
	refIDLocal    = "LOCL" // conventional REFID for an undisciplined local clock
	pollLog2Sec   = 6      // advisory poll interval hint (2^6 = 64s); we don't enforce it
	// precision: log2 seconds, two's complement in the packet's signed byte
	// field. -20 (~1us) is a conservative claim for a Go system clock read.
	// Go forbids converting a negative untyped constant straight to byte
	// (unsigned) even via an int8 constant conversion -- the value below is
	// int8(-20) written as its unsigned bit pattern.
	precisionByte = 0xEC
)

// Server is a minimal stateless SNTP responder bound to a single UDP socket.
type Server struct {
	conn *net.UDPConn
}

// Start binds UDP port 123 on bindHost (use "" or "0.0.0.0" for all
// interfaces) and begins serving in a background goroutine. It returns as
// soon as the bind succeeds; the caller is expected to Stop() it on
// shutdown. A bind failure is returned directly rather than logged, since
// this is a non-critical, opt-in feature that must never affect the rest of
// startup -- callers should log it as a warning and continue.
func Start(bindHost string) (*Server, error) {
	udpAddr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(bindHost, "123"))
	if err != nil {
		return nil, fmt.Errorf("resolve NTP bind address: %w", err)
	}
	conn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return nil, fmt.Errorf("bind UDP 123 (needs CAP_NET_BIND_SERVICE if not running as root): %w", err)
	}

	s := &Server{conn: conn}
	go s.serve()
	return s, nil
}

// Stop closes the UDP socket, ending the serve loop.
func (s *Server) Stop() error {
	return s.conn.Close()
}

// StopWithContext closes the socket when ctx is cancelled -- convenience for
// wiring into a context-based shutdown sequence.
func (s *Server) StopWithContext(ctx context.Context) {
	go func() {
		<-ctx.Done()
		_ = s.conn.Close()
	}()
}

func (s *Server) serve() {
	buf := make([]byte, packetSize)
	for {
		n, clientAddr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return // socket closed (shutdown) or a fatal read error either way
		}
		if n < packetSize {
			continue // too short to be a real NTP/SNTP request; ignore
		}

		resp, err := buildResponse(buf[:packetSize], time.Now())
		if err != nil {
			continue
		}
		if _, err := s.conn.WriteToUDP(resp, clientAddr); err != nil {
			log.Printf("[WARN] ntpserver: write to %s failed: %v", clientAddr, err)
		}
	}
}

// buildResponse constructs a 48-byte SNTP server response for a client
// request, using recvTime as both the receive timestamp and (after a second
// time.Now() call for the transmit field) the basis for the transmit
// timestamp. Split out from serve() so it's unit-testable without a socket.
func buildResponse(request []byte, recvTime time.Time) ([]byte, error) {
	if len(request) < packetSize {
		return nil, fmt.Errorf("request too short: %d bytes", len(request))
	}

	// The client's own Transmit Timestamp (bytes 40-47 of the REQUEST) is
	// echoed back as this response's Origin Timestamp, so the client can
	// compute round-trip delay.
	var origin [8]byte
	copy(origin[:], request[40:48])

	resp := make([]byte, packetSize)
	resp[0] = (leapNone << 6) | (versionNum << 3) | modeServer
	resp[1] = stratumLocal
	resp[2] = pollLog2Sec
	resp[3] = precisionByte

	// Root delay / root dispersion (bytes 4-11) left at zero: this server
	// IS the reference for the network it's answering on.
	copy(resp[12:16], []byte(refIDLocal))

	refSec, refFrac := toNTPTime(recvTime)
	putNTPTime(resp[16:24], refSec, refFrac) // reference timestamp: "when this clock was last set" == now, we have no history

	copy(resp[24:32], origin[:])

	rSec, rFrac := toNTPTime(recvTime)
	putNTPTime(resp[32:40], rSec, rFrac)

	tSec, tFrac := toNTPTime(time.Now())
	putNTPTime(resp[40:48], tSec, tFrac)

	return resp, nil
}

// toNTPTime converts a time.Time into the 64-bit NTP timestamp format: whole
// seconds since the NTP epoch, plus a fractional-second field where the
// full uint32 range represents one second.
func toNTPTime(t time.Time) (sec uint32, frac uint32) {
	sec = uint32(t.Unix() + ntpEpochDelta)
	frac = uint32((uint64(t.Nanosecond()) << 32) / 1e9)
	return sec, frac
}

func putNTPTime(b []byte, sec, frac uint32) {
	binary.BigEndian.PutUint32(b[0:4], sec)
	binary.BigEndian.PutUint32(b[4:8], frac)
}
