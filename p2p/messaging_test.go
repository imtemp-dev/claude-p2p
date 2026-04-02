package p2p

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/peer"
)

func TestSendDirectMessage(t *testing.T) {
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

	inbox1 := NewInbox(10)
	inbox2 := NewInbox(10)
	m1 := NewMessenger(h1, inbox1, testLogger())
	defer m1.Close()
	m2 := NewMessenger(h2, inbox2, testLogger())
	defer m2.Close()

	// Connect h1 to h2
	h1.Connect(ctx, peer.AddrInfo{ID: h2.ID(), Addrs: h2.Addrs()})

	// Send message from h1 to h2
	_, err = m1.SendDirect(ctx, h2.ID(), "hello from h1", "", "", "", "")
	if err != nil {
		t.Fatalf("SendDirect failed: %v", err)
	}

	// Wait for message to arrive
	time.Sleep(200 * time.Millisecond)

	msgs := inbox2.Pop()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message in h2 inbox, got %d", len(msgs))
	}
	if msgs[0].Content != "hello from h1" {
		t.Errorf("message content = %q, want %q", msgs[0].Content, "hello from h1")
	}
	if msgs[0].Type != "direct" {
		t.Errorf("message type = %q, want %q", msgs[0].Type, "direct")
	}
	if msgs[0].From != h1.ID().String() {
		t.Errorf("message from = %q, want %q", msgs[0].From, h1.ID().String())
	}
}

func TestSendToDisconnectedPeer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h1, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	defer h1.Close()

	inbox := NewInbox(10)
	m := NewMessenger(h1, inbox, testLogger())
	defer m.Close()

	// Try to send to a random peer ID (not connected)
	fakePeerID, _ := peer.Decode("QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N")
	_, err = m.SendDirect(ctx, fakePeerID, "hello", "", "", "", "")
	if err == nil {
		t.Fatal("expected error sending to disconnected peer")
	}
}

func TestGenerateMessageID(t *testing.T) {
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	var seq atomic.Uint64
	id1 := GenerateMessageID(h.ID(), &seq)
	id2 := GenerateMessageID(h.ID(), &seq)

	if id1 == "" {
		t.Error("message ID should not be empty")
	}
	if id1 == id2 {
		t.Error("sequential message IDs should be unique")
	}
}

func TestMessengerClose(t *testing.T) {
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	inbox := NewInbox(10)
	m := NewMessenger(h, inbox, testLogger())

	err = m.Close()
	if err != nil {
		t.Errorf("Close should not return error: %v", err)
	}
}
