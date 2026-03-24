package p2p

import (
	"context"
	"strings"
	"testing"

	"github.com/libp2p/go-libp2p"
)

func newTestTopicManager(t *testing.T) (*TopicManager, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())

	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })

	tracker, err := NewPeerTracker(h, testLogger())
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	tracker.Start(ctx)
	t.Cleanup(func() { tracker.Close() })

	disc := NewDiscovery(h, tracker, testLogger())
	// Don't start DHT (no internet in tests) — RoutingDiscovery will be nil
	t.Cleanup(func() { disc.Close() })

	tm := NewTopicManager(ctx, h, disc, tracker, NewInbox(DefaultInboxCapacity), testLogger())
	t.Cleanup(func() { tm.Close() })

	return tm, cancel
}

// Scenario 7: join_topic with no DHT returns error
func TestJoinTopicNoDHT(t *testing.T) {
	tm, cancel := newTestTopicManager(t)
	defer cancel()

	err := tm.Join("my-team")
	if err == nil {
		t.Fatal("expected error when DHT not available")
	}
	if !strings.Contains(err.Error(), "DHT not available") {
		t.Errorf("error = %q, should mention DHT not available", err.Error())
	}
}

// Scenario 8: join_topic with empty topic
func TestJoinTopicEmpty(t *testing.T) {
	tm, cancel := newTestTopicManager(t)
	defer cancel()

	err := tm.Join("")
	if err == nil {
		t.Fatal("expected error for empty topic")
	}
	if !strings.Contains(err.Error(), "topic is required") {
		t.Errorf("error = %q, should mention topic is required", err.Error())
	}
}

// Test join_topic with too-long topic
func TestJoinTopicTooLong(t *testing.T) {
	tm, cancel := newTestTopicManager(t)
	defer cancel()

	longTopic := strings.Repeat("a", MaxTopicLength+1)
	err := tm.Join(longTopic)
	if err == nil {
		t.Fatal("expected error for too-long topic")
	}
	if !strings.Contains(err.Error(), "too long") {
		t.Errorf("error = %q, should mention too long", err.Error())
	}
}

// Scenario 11: Re-join topic is idempotent
func TestReJoinTopicIdempotent(t *testing.T) {
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
	// Start DHT for this test
	if err := disc.StartDHT(ctx); err != nil {
		t.Skipf("DHT failed to start (may need network): %v", err)
	}
	defer disc.Close()

	tm := NewTopicManager(ctx, h, disc, tracker, NewInbox(DefaultInboxCapacity), testLogger())
	defer tm.Close()

	err = tm.Join("test-topic")
	if err != nil {
		t.Fatalf("first join failed: %v", err)
	}

	// Second join should be no-op
	err = tm.Join("test-topic")
	if err != nil {
		t.Fatalf("re-join failed: %v", err)
	}

	topics := tm.Topics()
	if len(topics) != 1 {
		t.Errorf("expected 1 topic, got %d", len(topics))
	}
}

// Scenario 13: Multiple topics
func TestMultipleTopics(t *testing.T) {
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
	if err := disc.StartDHT(ctx); err != nil {
		t.Skipf("DHT failed to start: %v", err)
	}
	defer disc.Close()

	tm := NewTopicManager(ctx, h, disc, tracker, NewInbox(DefaultInboxCapacity), testLogger())
	defer tm.Close()

	tm.Join("topic-a")
	tm.Join("topic-b")

	topics := tm.Topics()
	if len(topics) != 2 {
		t.Errorf("expected 2 topics, got %d", len(topics))
	}
	if topics[0] != "topic-a" || topics[1] != "topic-b" {
		t.Errorf("topics = %v, want [topic-a, topic-b]", topics)
	}
}

// Scenario 5: Leave topic
func TestLeaveTopic(t *testing.T) {
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
	if err := disc.StartDHT(ctx); err != nil {
		t.Skipf("DHT failed to start: %v", err)
	}
	defer disc.Close()

	tm := NewTopicManager(ctx, h, disc, tracker, NewInbox(DefaultInboxCapacity), testLogger())
	defer tm.Close()

	tm.Join("temp-topic")
	if len(tm.Topics()) != 1 {
		t.Fatal("expected 1 topic after join")
	}

	tm.Leave("temp-topic")
	if len(tm.Topics()) != 0 {
		t.Error("expected 0 topics after leave")
	}
}

// Test Leave unknown topic is no-op
func TestLeaveUnknownTopic(t *testing.T) {
	tm, cancel := newTestTopicManager(t)
	defer cancel()

	// Should not panic
	tm.Leave("nonexistent")
}

// Test max topics limit
func TestMaxTopicsLimit(t *testing.T) {
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
	if err := disc.StartDHT(ctx); err != nil {
		t.Skipf("DHT failed to start: %v", err)
	}
	defer disc.Close()

	tm := NewTopicManager(ctx, h, disc, tracker, NewInbox(DefaultInboxCapacity), testLogger())
	defer tm.Close()

	for i := 0; i < MaxTopics; i++ {
		if err := tm.Join(strings.Repeat("t", i+1)); err != nil {
			t.Fatalf("join %d failed: %v", i, err)
		}
	}

	err = tm.Join("one-too-many")
	if err == nil {
		t.Fatal("expected error when max topics reached")
	}
	if !strings.Contains(err.Error(), "too many topics") {
		t.Errorf("error = %q, should mention too many topics", err.Error())
	}
}
