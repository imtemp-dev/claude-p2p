package p2p

import (
	"context"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
)

func testLogger() *log.Logger {
	return log.New(os.Stderr, "[test] ", log.LstdFlags)
}

// Scenario 3: list_peers returns correct peer info with source
func TestPeerTrackerConnectDisconnect(t *testing.T) {
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

	// Connect h1 to h2
	h1.Connect(ctx, peer.AddrInfo{ID: h2.ID(), Addrs: h2.Addrs()})

	// Wait for event bus to process
	time.Sleep(100 * time.Millisecond)

	peers := tracker.Peers()
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(peers))
	}
	if peers[0].ID != h2.ID() {
		t.Errorf("peer ID = %s, want %s", peers[0].ID, h2.ID())
	}
	if peers[0].Source != "direct" {
		t.Errorf("source = %q, want %q", peers[0].Source, "direct")
	}
	if peers[0].ConnectedAt.IsZero() {
		t.Error("ConnectedAt should not be zero")
	}

	// Scenario 12: Peer disconnect removes from tracker
	h2.Close()
	time.Sleep(200 * time.Millisecond)

	peers = tracker.Peers()
	if len(peers) != 0 {
		t.Errorf("expected 0 peers after disconnect, got %d", len(peers))
	}
}

// Test AddPending sets source correctly
func TestPeerTrackerAddPending(t *testing.T) {
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

	// Register pending source before connect
	tracker.AddPending(h2.ID(), "mdns")

	// Connect
	h1.Connect(ctx, peer.AddrInfo{ID: h2.ID(), Addrs: h2.Addrs()})
	time.Sleep(100 * time.Millisecond)

	peers := tracker.Peers()
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(peers))
	}
	if peers[0].Source != "mdns" {
		t.Errorf("source = %q, want %q (from AddPending)", peers[0].Source, "mdns")
	}
}

// Test AddPending updates source for already-connected peer
func TestPeerTrackerAddPendingAlreadyConnected(t *testing.T) {
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

	// Connect first (source will be "direct")
	h1.Connect(ctx, peer.AddrInfo{ID: h2.ID(), Addrs: h2.Addrs()})
	time.Sleep(100 * time.Millisecond)

	// Now update source
	tracker.AddPending(h2.ID(), "dht")
	time.Sleep(50 * time.Millisecond)

	peers := tracker.Peers()
	if len(peers) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(peers))
	}
	if peers[0].Source != "dht" {
		t.Errorf("source = %q, want %q (updated)", peers[0].Source, "dht")
	}
}

// Test PeersBySource filtering
func TestPeersBySource(t *testing.T) {
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

	h3, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	defer h3.Close()

	tracker, err := NewPeerTracker(h1, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	tracker.Start(ctx)
	defer tracker.Close()

	tracker.AddPending(h2.ID(), "mdns")
	tracker.AddPending(h3.ID(), "dht")

	h1.Connect(ctx, peer.AddrInfo{ID: h2.ID(), Addrs: h2.Addrs()})
	h1.Connect(ctx, peer.AddrInfo{ID: h3.ID(), Addrs: h3.Addrs()})
	time.Sleep(200 * time.Millisecond)

	mdnsPeers := tracker.PeersBySource("mdns")
	if len(mdnsPeers) != 1 {
		t.Errorf("expected 1 mdns peer, got %d", len(mdnsPeers))
	}
	dhtPeers := tracker.PeersBySource("dht")
	if len(dhtPeers) != 1 {
		t.Errorf("expected 1 dht peer, got %d", len(dhtPeers))
	}
	allPeers := tracker.Peers()
	if len(allPeers) != 2 {
		t.Errorf("expected 2 total peers, got %d", len(allPeers))
	}
}

// Test PeerTracker returns empty after Close
func TestPeerTrackerClosedReturnsEmpty(t *testing.T) {
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

	h1.Connect(ctx, peer.AddrInfo{ID: h2.ID(), Addrs: h2.Addrs()})
	time.Sleep(100 * time.Millisecond)

	tracker.Close()

	peers := tracker.Peers()
	if len(peers) != 0 {
		t.Errorf("expected 0 peers after Close, got %d", len(peers))
	}
}

// Test Count returns 0 after Close
func TestPeerTrackerCountAfterClose(t *testing.T) {
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

	h1.Connect(ctx, peer.AddrInfo{ID: h2.ID(), Addrs: h2.Addrs()})
	time.Sleep(100 * time.Millisecond)

	if tracker.Count() == 0 {
		t.Error("expected non-zero count before close")
	}

	tracker.Close()

	if tracker.Count() != 0 {
		t.Errorf("expected 0 count after close, got %d", tracker.Count())
	}
}

// Scenario 14: Concurrent peer tracker access
func TestPeerTrackerConcurrentAccess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h1, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	defer h1.Close()

	tracker, err := NewPeerTracker(h1, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	tracker.Start(ctx)
	defer tracker.Close()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.Peers()
			tracker.Count()
			tracker.AddPending(peer.ID("fake"), "test")
		}()
	}
	wg.Wait()
	// No race detector panics = pass
}
