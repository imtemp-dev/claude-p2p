package p2p

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Scenario 10: Self-discovery filtered
func TestHandlePeerFoundSkipsSelf(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	tracker, err := NewPeerTracker(h, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	tracker.Start(ctx)
	defer tracker.Close()

	disc := NewDiscovery(h, tracker, testLogger())

	// Call HandlePeerFound with self — should be ignored
	disc.HandlePeerFound(peer.AddrInfo{ID: h.ID(), Addrs: h.Addrs()})
	time.Sleep(50 * time.Millisecond)

	if tracker.Count() != 0 {
		t.Errorf("expected 0 peers (self should be filtered), got %d", tracker.Count())
	}
}

// Test HandlePeerFound with context fallback (no DHT started)
func TestHandlePeerFoundContextFallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h1, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	defer h1.Close()

	h2, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	defer h2.Close()

	tracker, err := NewPeerTracker(h1, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	tracker.Start(ctx)
	defer tracker.Close()

	disc := NewDiscovery(h1, tracker, testLogger())
	// DHT NOT started — ctx is nil, should fall back to context.Background()

	disc.HandlePeerFound(peer.AddrInfo{ID: h2.ID(), Addrs: h2.Addrs()})
	time.Sleep(200 * time.Millisecond)

	peers := tracker.Peers()
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer after HandlePeerFound, got %d", len(peers))
	}
	if peers[0].Source != "mdns" {
		t.Errorf("source = %q, want %q", peers[0].Source, "mdns")
	}
}

// Test Discovery.Close with nil services
func TestDiscoveryCloseNil(t *testing.T) {
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	tracker, err := NewPeerTracker(h, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	disc := NewDiscovery(h, tracker, testLogger())
	// Neither mDNS nor DHT started — Close should not panic
	if err := disc.Close(); err != nil {
		t.Errorf("Close with nil services should not error: %v", err)
	}
}

// Test RoutingDiscovery returns nil when DHT not started
func TestRoutingDiscoveryNilWithoutDHT(t *testing.T) {
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	tracker, err := NewPeerTracker(h, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	disc := NewDiscovery(h, tracker, testLogger())
	if disc.RoutingDiscovery() != nil {
		t.Error("RoutingDiscovery should be nil without DHT started")
	}
}
