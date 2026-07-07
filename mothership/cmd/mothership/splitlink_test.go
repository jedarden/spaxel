package main

import (
	"reflect"
	"testing"
	"time"

	sigproc "github.com/spaxel/mothership/internal/signal"
)

// splitLinkIDTestCases documents the link-ID formats the fusion path must
// accept. The producer (CSIFrame.LinkID in internal/ingestion) emits the
// colon-joined form; the dash-joined form is the legacy documented format. Both
// are exactly 35 chars (17 + 1 separator + 17) with the MAC boundary at index
// 17. See bf-561zr / bf-20q9o.
var splitLinkIDTestCases = []struct {
	name    string
	linkID  string
	want    []string
}{
	{
		name:   "colon-joined producer format (uppercase, LinkID output)",
		linkID: "AA:BB:CC:DD:EE:FF:11:22:33:44:55:66",
		want:   []string{"AA:BB:CC:DD:EE:FF", "11:22:33:44:55:66"},
	},
	{
		name:   "colon-joined producer format (lowercase, sim/real edge case)",
		linkID: "aa:bb:cc:dd:ee:ff:11:22:33:44:55:66",
		want:   []string{"aa:bb:cc:dd:ee:ff", "11:22:33:44:55:66"},
	},
	{
		name:   "dash-joined documented format",
		linkID: "AA:BB:CC:DD:EE:FF-11:22:33:44:55:66",
		want:   []string{"AA:BB:CC:DD:EE:FF", "11:22:33:44:55:66"},
	},
	{
		// Single MAC (17 chars) is not a valid two-MAC link ID.
		name:   "single MAC (no separator) -> nil",
		linkID: "AA:BB:CC:DD:EE:FF",
		want:   nil,
	},
	{
		name:   "empty string -> nil",
		linkID: "",
		want:   nil,
	},
	{
		// Defensive fallback: a non-canonical dash-joined form still splits
		// on the rightmost dash rather than being dropped.
		name:   "non-canonical dash form falls back to dash scan",
		linkID: "AB:CD-EF:GH",
		want:   []string{"AB:CD", "EF:GH"},
	},
}

func TestSplitLinkID(t *testing.T) {
	for _, tc := range splitLinkIDTestCases {
		t.Run(tc.name, func(t *testing.T) {
			got := splitLinkID(tc.linkID)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("splitLinkID(%q) = %v, want %v", tc.linkID, got, tc.want)
			}
		})
	}
}

// TestSplitLinkIDProducerContract verifies the exact contract CSIFrame.LinkID
// relies on: splitting the real producer output recovers the original node and
// peer MACs. This guards against a regression where the in-memory link key
// diverges from what the fusion path can parse (the bf-20q9o root cause:
// 0 active links -> 0 peaks -> 0 blobs).
func TestSplitLinkIDProducerContract(t *testing.T) {
	// Mirrors internal/ingestion CSIFrame.LinkID(): "%s:%s" of two
	// 17-char uppercase colon-separated MACs.
	const nodeMAC = "AA:BB:CC:DD:EE:FF"
	const peerMAC = "11:22:33:44:55:66"
	linkID := nodeMAC + ":" + peerMAC

	parts := splitLinkID(linkID)
	if len(parts) != 2 {
		t.Fatalf("splitLinkID(producer LinkID %q) returned %d parts, want 2", linkID, len(parts))
	}
	if parts[0] != nodeMAC {
		t.Errorf("parts[0] = %q, want node MAC %q", parts[0], nodeMAC)
	}
	if parts[1] != peerMAC {
		t.Errorf("parts[1] = %q, want peer MAC %q", parts[1], peerMAC)
	}
}

// TestGatherFusionLinksProducerLinkID proves acceptance criterion #2 at the
// function level: when the signal pipeline holds a link registered under the
// producer's actual colon-joined LinkID (CSIFrame.LinkID, fed straight through
// by ingestion/server.go pm.Process), gatherFusionLinks returns a non-empty
// links slice with the two MACs split correctly. That slice is exactly what the
// 3D engine's Fuse receives, so a non-empty result here == Fuse receiving
// links>=1 (the [FUSE-DBG] log in internal/fusion would mirror len(links)).
//
// Before the bf-561zr fix, splitLinkID scanned only for '-' and returned nil for
// this colon-joined linkID, so this assertion would have produced 0 links.
func TestGatherFusionLinksProducerLinkID(t *testing.T) {
	const nodeMAC = "AA:BB:CC:DD:EE:FF"
	const peerMAC = "11:22:33:44:55:66"
	linkID := nodeMAC + ":" + peerMAC // colon-joined producer form (35 chars)

	const nSub = 64
	pm := sigproc.NewProcessorManager(sigproc.ProcessorManagerConfig{
		NSub:       nSub,
		FusionRate: 10,
		Tau:        30,
	})

	// Feed a few synthetic CSI frames (128 int8 = nSub*2 I/Q pairs) on the
	// colon-joined link so a processor + motion state is registered, exactly
	// as ingestion/server.go does on pm.Process(linkID, ...).
	payload := make([]int8, nSub*2)
	for i := range payload {
		payload[i] = 10
	}
	base := time.Unix(1_700_000_000, 0)
	for i := 0; i < 5; i++ {
		if _, err := pm.Process(linkID, payload, -50, nSub, base.Add(time.Duration(i)*100*time.Millisecond)); err != nil {
			t.Fatalf("pm.Process(%q): %v", linkID, err)
		}
	}

	links := gatherFusionLinks(pm)
	if len(links) < 1 {
		t.Fatalf("gatherFusionLinks returned %d links, want >=1 (Fuse would receive an empty slice -> 0 peaks -> 0 blobs)", len(links))
	}
	got := links[0]
	if got.NodeMAC != nodeMAC {
		t.Errorf("links[0].NodeMAC = %q, want %q", got.NodeMAC, nodeMAC)
	}
	if got.PeerMAC != peerMAC {
		t.Errorf("links[0].PeerMAC = %q, want %q", got.PeerMAC, peerMAC)
	}
}
