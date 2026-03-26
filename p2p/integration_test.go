package p2p

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
)

// TestIntegrationP2PCommunication verifies end-to-end P2P messaging between two hosts.
func TestIntegrationP2PCommunication(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create two hosts on loopback
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

	// Set up P2P stack for both
	inbox1 := NewInbox(10)
	inbox2 := NewInbox(10)

	tracker1, err := NewPeerTracker(h1, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	tracker1.Start(ctx)
	defer tracker1.Close()

	tracker2, err := NewPeerTracker(h2, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	tracker2.Start(ctx)
	defer tracker2.Close()

	m1 := NewMessenger(h1, inbox1, testLogger())
	defer m1.Close()
	m2 := NewMessenger(h2, inbox2, testLogger())
	defer m2.Close()

	// Connect h1 to h2
	err = h1.Connect(ctx, peer.AddrInfo{ID: h2.ID(), Addrs: h2.Addrs()})
	if err != nil {
		t.Fatalf("connect failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Verify peer tracking
	peers1 := tracker1.Peers()
	if len(peers1) != 1 {
		t.Errorf("host1 expected 1 peer, got %d", len(peers1))
	}
	peers2 := tracker2.Peers()
	if len(peers2) != 1 {
		t.Errorf("host2 expected 1 peer, got %d", len(peers2))
	}

	// Send direct message from h1 to h2
	err = m1.SendDirect(ctx, h2.ID(), "hello from integration test", "")
	if err != nil {
		t.Fatalf("send direct failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Verify h2 received the message
	msgs := inbox2.Pop()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "hello from integration test" {
		t.Errorf("message content = %q, want %q", msgs[0].Content, "hello from integration test")
	}
	if msgs[0].Type != "direct" {
		t.Errorf("message type = %q, want %q", msgs[0].Type, "direct")
	}
	if msgs[0].From != h1.ID().String() {
		t.Errorf("message from = %q, want %q", msgs[0].From, h1.ID().String())
	}

	// Send message back from h2 to h1
	err = m2.SendDirect(ctx, h1.ID(), "reply from h2", "")
	if err != nil {
		t.Fatalf("send reply failed: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	msgs = inbox1.Pop()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 reply, got %d", len(msgs))
	}
	if msgs[0].Content != "reply from h2" {
		t.Errorf("reply content = %q, want %q", msgs[0].Content, "reply from h2")
	}
}

// TestIntegrationGracefulShutdown verifies clean shutdown of P2P components.
func TestIntegrationGracefulShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}

	inbox := NewInbox(10)
	tracker, err := NewPeerTracker(h, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	tracker.Start(ctx)

	m := NewMessenger(h, inbox, testLogger())

	// Cancel context to trigger shutdown
	cancel()
	time.Sleep(100 * time.Millisecond)

	// Close all components in order
	m.Close()
	tracker.Close()
	h.Close()

	// Verify tracker returns empty after close
	if tracker.Count() != 0 {
		t.Errorf("expected 0 peers after shutdown, got %d", tracker.Count())
	}
	if peers := tracker.Peers(); len(peers) != 0 {
		t.Errorf("expected empty peers after shutdown, got %d", len(peers))
	}
}
