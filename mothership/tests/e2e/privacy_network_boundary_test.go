package e2e

// Live network-level verification of the local-only privacy boundary
// (bead spaxel-ee373362).
//
// The README promises "detection is local: data stays on the mothership,
// there is no cloud relay or remote access". Two earlier tests pin parts of
// that promise statically: internal/privacy's egress allowlist test proves
// the *source* contains no dial sites outside operator-configured
// integrations, and internal/auth's route-surface test proves every HTTP
// route carries the documented auth expectation. Neither observes the
// *running process*. This test closes that gap at the network level: while a
// simulated fleet streams CSI and the fusion pipeline is producing blobs,
// it watches the mothership process's actual sockets through /proc and
// asserts the boundary holds in practice.
//
// # The verified boundary (what this test pins for the default deployment)
//
//   - Exactly ONE TCP listener exists, at the configured SPAXEL_BIND_ADDR
//     (the harness binds 127.0.0.1, which is unreachable from off-box by
//     construction). Production binds a LAN interface so nodes can connect;
//     the API surface it exposes is what the route-surface test pins.
//   - No TCP socket in any state (sampled continuously for the whole run)
//     has a non-loopback peer. Zero non-loopback egress is tolerated: no
//     telemetry agent, CSI upload, or second integration can hide between
//     samples.
//   - No UDP socket is connected to a non-loopback unicast peer, except the
//     kernel resolver (remote port 53). Unconnected UDP sockets (remote
//     port 0) are exempt: that is how Go's DNS resolver and the mDNS
//     advertisement bind, and neither carries detection data.
//   - Detection data is actually flowing while all of the above holds
//     (blobs observed via /api/blobs), so the assertion covers the live
//     detection path, not an idle server.
//
// # The one known dialer, deflected to loopback
//
// Measured at HEAD, the ONLY thing a mothership dials outside the LAN is
// the GitHub API release-availability ping: startup phase 5 initializes the
// internal/github client (used for Kaniko release lookups) and pings its
// endpoint — api.github.com by default — even when completely unconfigured
// (observed: "[GITHUB] API ping successful (status 401)"). The ping carries
// no payload, and a failed ping is non-fatal (WARN + continue). The client
// endpoint is operator configuration (SPAXEL_GITHUB_API_URL): pointing
// releases at an internal mirror is a supported posture. This test uses
// exactly that config knob to set the endpoint to a closed loopback port,
// which does double duty:
//
//   - the known dialer is deflected onto loopback, so the strict
//     "zero non-loopback egress" assertion holds and, crucially, proves
//     NOTHING ELSE dials out — any second dialer would still show a
//     non-loopback socket and fail;
//   - it exercises the operator-config surface the boundary write-up
//     documents for airgapped deployments.
//
// (Matching api.github.com by resolved IP instead was tried and rejected:
// GitHub rotates its A records, so the test's resolution and the
// mothership's connection legitimately disagree and the allowlist flakes.)
//
// Note the honest corollary, documented rather than hidden: because that
// ping is unconditional, a stock unconfigured deployment is not fully
// airgapped — it contacts api.github.com once at startup. It carries no
// detection data, and SPAXEL_GITHUB_API_URL redirects it. See
// docs/notes/network-boundary.md for the full boundary write-up.
//
// Remaining bind-only LAN services, never relays: mDNS advertisement
// (UDP/5353 link-local multicast, SPAXEL_MDNS_ENABLED, default on) and the
// opt-in embedded SNTP responder (UDP/123, SPAXEL_NTP_LOCAL_ENABLED,
// default off).
//
// # Socket attribution
//
// /proc offers no atomic process→socket view, so each sample reads the
// process's fd inodes, then the /proc/net tables, then the fd inodes again,
// and only considers inodes present in BOTH fd snapshots. Without the
// second read, a mothership socket that closes mid-sample can have its
// inode immediately recycled by another process on this host, producing
// phantom violations. The double read costs one extra pass and excludes
// only sockets living shorter than a single sample; a real relay connection
// is long-lived and cannot hide that way. The sampling loop covers the
// entire run at 200 ms.
//
// /proc is Linux-only, so the test skips elsewhere; CI (spaxel-build's
// go-test leg) runs this suite on Linux.

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

// tcpStates maps the /proc/net/tcp numeric state column (hex) to a short
// name for violation messages. Only LISTEN is treated specially by the scan.
var tcpStates = map[int]string{
	0x01: "ESTABLISHED", 0x02: "SYN_SENT", 0x03: "SYN_RECV",
	0x04: "FIN_WAIT1", 0x05: "FIN_WAIT2", 0x06: "TIME_WAIT",
	0x07: "CLOSE", 0x08: "CLOSE_WAIT", 0x09: "LAST_ACK",
	0x0A: "LISTEN", 0x0B: "CLOSING",
}

const (
	tcpStateListen = 0x0A
	udpPortDNS     = 53 // kernel resolver; exempted, see the exceptions comment
)

// procNetEntry is one socket row from a /proc/net/{tcp,udp}{,6} table, with
// the row's hex addresses and ports decoded.
type procNetEntry struct {
	local      *net.TCPAddr // local address (ip + decimal port)
	remote     *net.TCPAddr // remote address; port 0 = unconnected
	state      int          // TCP state code; UDP rows carry 07 (CLOSE)
	localIsUDP bool
}

// parseHexIP decodes a /proc/net hex host address. Both tables print each
// 4-byte word of the address as a little-endian u32, so the bytes of every
// word are reversed relative to network order ("0100007F" is 127.0.0.1, not
// 1.0.0.127).
func parseHexIP(hexIP string) net.IP {
	raw, err := hex.DecodeString(hexIP)
	if err != nil {
		return nil
	}
	for i := 0; i+4 <= len(raw); i += 4 {
		raw[i], raw[i+3] = raw[i+3], raw[i]
		raw[i+1], raw[i+2] = raw[i+2], raw[i+1]
	}
	return net.IP(raw)
}

// parseProcAddr decodes one /proc/net ADDRESS column ("0100007F:1F90") into
// an IP and a decimal port. Both halves are hex in every table.
func parseProcAddr(addr string) (*net.TCPAddr, error) {
	hostHex, portHex, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("malformed address %q: %w", addr, err)
	}
	ip := parseHexIP(hostHex)
	if ip == nil {
		return nil, fmt.Errorf("malformed hex host %q in %q", hostHex, addr)
	}
	port, err := strconv.ParseInt(portHex, 16, 32)
	if err != nil {
		return nil, fmt.Errorf("malformed hex port %q in %q: %w", portHex, addr, err)
	}
	return &net.TCPAddr{IP: ip, Port: int(port)}, nil
}

// parseProcNet parses /proc/net/<table> (tcp, tcp6, udp or udp6) into socket
// entries keyed by inode. The header row and rows with an unparseable shape
// are skipped; a table that is merely absent (e.g. no IPv6) is not an error.
func parseProcNet(table string) (map[string]procNetEntry, error) {
	data, err := os.ReadFile("/proc/net/" + table)
	if os.IsNotExist(err) {
		return map[string]procNetEntry{}, nil
	}
	if err != nil {
		return nil, err
	}
	entries := make(map[string]procNetEntry)
	isUDP := strings.HasPrefix(table, "udp")
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		// sl local rem st tx:rx tr:when retrnsmt uid timeout inode ...
		if len(fields) < 10 || fields[0] == "sl" {
			continue
		}
		local, err := parseProcAddr(fields[1])
		if err != nil {
			continue
		}
		remote, err := parseProcAddr(fields[2])
		if err != nil {
			continue
		}
		// The state column is hex ("0A" = LISTEN); decimal parsing would
		// silently drop every LISTEN/CLOSING row.
		st, err := strconv.ParseInt(fields[3], 16, 32)
		if err != nil {
			continue
		}
		entries[fields[9]] = procNetEntry{
			local:      local,
			remote:     remote,
			state:      int(st),
			localIsUDP: isUDP,
		}
	}
	return entries, nil
}

// socketInodes lists the inodes of every socket the process holds open, by
// readlink-ing /proc/<pid>/fd. Only "socket:[<inode>]" links are returned.
func socketInodes(pid int) (map[string]bool, error) {
	fds, err := os.ReadDir(fmt.Sprintf("/proc/%d/fd", pid))
	if err != nil {
		return nil, err
	}
	inodes := make(map[string]bool)
	for _, fd := range fds {
		link, err := os.Readlink(fmt.Sprintf("/proc/%d/fd/%s", pid, fd.Name()))
		if err != nil {
			continue // fd closed between readdir and readlink
		}
		if inode, ok := strings.CutPrefix(link, "socket:["); ok {
			inodes[strings.TrimSuffix(inode, "]")] = true
		}
	}
	return inodes, nil
}

// boundaryFindings is what the socket samples observed about the process.
type boundaryFindings struct {
	listenAddrs   map[string]bool // distinct TCP LISTEN local addresses seen
	violations    []string        // human-readable boundary violations
	estabLoopback int             // cumulative loopback-peer TCP sockets observed
	samples       int
}

// sampleProcessSockets takes one instantaneous sample of pid's sockets and
// folds it into f. TCP sockets in any non-LISTEN state must have a loopback
// peer; connected UDP sockets must have a loopback peer or be DNS.
// Unconnected UDP sockets (remote port 0) are exempt — see the file comment
// for why.
func sampleProcessSockets(pid int, f *boundaryFindings) {
	// Double fd read around the table reads: an inode must be held by the
	// process at BOTH snapshots to be attributed to it. See the file comment
	// ("Socket attribution") for the inode-recycling race this closes.
	inodesBefore, err := socketInodes(pid)
	if err != nil {
		f.violations = append(f.violations, fmt.Sprintf("could not read /proc/%d/fd: %v", pid, err))
		return
	}

	tableEntries := make([]map[string]procNetEntry, 0, 4)
	var tableErrs []string
	for _, table := range []string{"tcp", "tcp6", "udp", "udp6"} {
		entries, err := parseProcNet(table)
		if err != nil {
			tableErrs = append(tableErrs, fmt.Sprintf("could not read /proc/net/%s: %v", table, err))
			continue
		}
		tableEntries = append(tableEntries, entries)
	}

	inodesAfter, err := socketInodes(pid)
	if err != nil {
		f.violations = append(f.violations, fmt.Sprintf("could not re-read /proc/%d/fd: %v", pid, err))
		return
	}
	owned := func(inode string) bool { return inodesBefore[inode] && inodesAfter[inode] }

	f.samples++
	f.violations = append(f.violations, tableErrs...)

	for _, entries := range tableEntries {
		for inode, e := range entries {
			if !owned(inode) {
				continue // not (provably) the mothership's socket
			}
			kind := "TCP"
			if e.localIsUDP {
				kind = "UDP"
			}
			switch {
			case kind == "TCP" && e.state == tcpStateListen:
				f.listenAddrs[e.local.String()] = true
			case e.remote.Port == 0:
				// Unconnected: DNS resolver, mDNS advertisement bind.
			case e.remote.IP.IsLoopback():
				if kind == "TCP" {
					f.estabLoopback++
				}
			case kind == "UDP" && e.remote.Port == udpPortDNS:
				// Kernel resolver looking up an operator-configured
				// endpoint; infrastructure, carries no detection data.
			default:
				f.violations = append(f.violations, fmt.Sprintf(
					"%s socket with non-loopback peer (%s): %s -> %s",
					kind, tcpStates[e.state], e.local, e.remote))
			}
		}
	}
}

// resolveAPIEndpointIPs resolves the GitHub API hostname the mothership's
// release-availability ping targets, so the egress allowlist pins the
// documented endpoint rather than "any TCP/443". An unresolvable host (an
// offline CI builder) yields an empty allowlist: the ping then also fails
// inside the mothership, no such socket exists, and the assertion degrades
// safely — any observed non-loopback egress still fails the test.
func resolveAPIEndpointIPs(t *testing.T, host string) map[string]bool {
	t.Helper()
	ips, err := net.LookupHost(host)
	if err != nil {
		t.Logf("could not resolve %s (%v); the documented-egress allowlist is empty for this run", host, err)
		return map[string]bool{}
	}
	out := make(map[string]bool, len(ips))
	for _, ip := range ips {
		out[ip] = true
	}
	return out
}

// TestMothershipNetworkBoundaryStaysLocal streams simulated CSI through a
// live mothership and watches the process's sockets for the whole window,
// asserting the local-only boundary holds while detection data flows.
func TestMothershipNetworkBoundaryStaysLocal(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping network-boundary e2e in short mode")
	}
	if runtime.GOOS != "linux" {
		t.Skipf("the socket-level boundary check reads /proc; skipping on %s", runtime.GOOS)
	}

	ctx, cancel := context.WithTimeout(context.Background(), TestTimeout)
	defer cancel()

	// Deflect the one known outbound dialer (the GitHub release-availability
	// ping) to a closed loopback port via its operator-config endpoint knob —
	// see the file comment ("The one known dialer, deflected to loopback").
	// The harness inherits the test process's environment in Start(), and no
	// test in this package runs in parallel, so a scoped os.Setenv is safe.
	defer os.Unsetenv("SPAXEL_GITHUB_API_URL")
	os.Setenv("SPAXEL_GITHUB_API_URL", "http://127.0.0.1:9") // closed discard port

	h := NewTestHarness(t)
	if err := h.Start(ctx); err != nil {
		t.Fatalf("start mothership: %v", err)
	}
	defer h.Stop()
	pid := h.MothershipCmd.Process.Pid

	// Default integration env: no MQTT broker, no notification channels, no
	// OTA auto-update — the harness env already configures none of these, so
	// the run exercises the default zero-integration deployment posture.
	if err := h.RunSimulator(ctx, 3, 1, 20, 12*time.Second); err != nil {
		t.Fatalf("start simulator: %v", err)
	}

	findings := &boundaryFindings{listenAddrs: make(map[string]bool)}
	maxBlobs := 0

	// Sample for the sim window plus a short grace period so post-run
	// lingering connections (final API polls, WS teardown) are covered too.
	sampleWindow := 14 * time.Second
	deadline := time.Now().Add(sampleWindow)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			t.Fatalf("context expired mid-sample: %v", ctx.Err())
		default:
		}
		sampleProcessSockets(pid, findings)
		if n, err := h.GetBlobCount(ctx); err == nil && n > maxBlobs {
			maxBlobs = n
		}
		time.Sleep(200 * time.Millisecond)
	}

	// Sanity guards, in the same spirit as the static egress test's: a broken
	// scanner must fail loudly, never pass vacuously.
	if findings.samples < 20 {
		t.Fatalf("sanity: only %d socket samples taken in %s; the sampling loop is broken", findings.samples, sampleWindow)
	}
	if findings.estabLoopback == 0 {
		t.Fatal("sanity: never observed a loopback TCP connection; the sim and API traffic should have produced many — the inode matching is broken")
	}

	// The boundary assertions proper.
	if len(findings.violations) > 0 {
		t.Errorf("mothership held %d socket sample(s) violating the local-only boundary across %d samples:\n  %s",
			len(findings.violations), findings.samples, strings.Join(uniqueSorted(findings.violations), "\n  "))
	}

	var sawListeners []string
	for addr := range findings.listenAddrs {
		sawListeners = append(sawListeners, addr)
	}
	if len(findings.listenAddrs) != 1 || !findings.listenAddrs[h.BindAddr] {
		t.Errorf("expected exactly one TCP listener at the configured bind address %s, saw %d distinct listener address(es): %v",
			h.BindAddr, len(findings.listenAddrs), uniqueSorted(sawListeners))
	}

	// Detection data must have been produced while the wire was being
	// watched — otherwise the boundary was verified against an idle process.
	if maxBlobs == 0 {
		t.Fatal("no blobs were produced during the sampling window: the fusion pipeline emitted nothing, so the boundary was not verified against flowing detection data")
	}

	endpoints := uniqueSorted(sawListeners)
	t.Logf("boundary held: %d samples, %d loopback connections, listener exactly {%s}, zero non-loopback egress, max concurrent blobs %d",
		findings.samples, findings.estabLoopback, strings.Join(endpoints, ", "), maxBlobs)
}

// uniqueSorted dedupes and sorts strings so a repeated offender is reported
// once and output stays deterministic.
func uniqueSorted(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
