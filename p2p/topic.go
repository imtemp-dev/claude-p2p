package p2p

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	dutil "github.com/libp2p/go-libp2p/p2p/discovery/util"
)

const (
	TopicNamespace    = "claude-p2p"
	FindPeersInterval = 30 * time.Second
	MaxTopicLength    = 128
	MaxTopics         = 10

	// BroadcastRateLimit is the maximum sustained broadcast rate per topic (messages/sec).
	BroadcastRateLimit = 10
	// BroadcastBurstLimit is the maximum burst size for broadcasts per topic.
	BroadcastBurstLimit = 20
)

// rateBucket implements a simple token bucket rate limiter.
type rateBucket struct {
	tokens     float64
	maxTokens  float64
	refillRate float64 // tokens per second
	lastRefill time.Time
}

func newRateBucket(rate, burst float64) *rateBucket {
	return &rateBucket{
		tokens:     burst,
		maxTokens:  burst,
		refillRate: rate,
		lastRefill: time.Now(),
	}
}

func (rb *rateBucket) allow() bool {
	now := time.Now()
	elapsed := now.Sub(rb.lastRefill).Seconds()
	rb.tokens += elapsed * rb.refillRate
	if rb.tokens > rb.maxTokens {
		rb.tokens = rb.maxTokens
	}
	rb.lastRefill = now
	if rb.tokens >= 1 {
		rb.tokens--
		return true
	}
	return false
}

// TopicManager manages topic-based peer grouping via DHT rendezvous and GossipSub.
type TopicManager struct {
	ctx          context.Context
	host         host.Host
	discovery    *Discovery
	peerTracker  *PeerTracker
	inbox        *Inbox
	pubsub       *pubsub.PubSub
	mu           sync.RWMutex
	topics       map[string]context.CancelFunc
	pubsubTopics map[string]*pubsub.Topic
	pubsubSubs   map[string]*pubsub.Subscription
	wg             sync.WaitGroup
	seq            atomic.Uint64
	messageHandler func(topic string, msg Message, from peer.ID)
	rateMu         sync.Mutex
	rateBuckets    map[string]*rateBucket
	logger         *log.Logger
}

// NewTopicManager creates a topic manager with a long-lived context.
func NewTopicManager(ctx context.Context, h host.Host, disc *Discovery, tracker *PeerTracker, inbox *Inbox, logger *log.Logger) *TopicManager {
	return &TopicManager{
		ctx:          ctx,
		host:         h,
		discovery:    disc,
		peerTracker:  tracker,
		inbox:        inbox,
		topics:       make(map[string]context.CancelFunc),
		pubsubTopics: make(map[string]*pubsub.Topic),
		pubsubSubs:   make(map[string]*pubsub.Subscription),
		rateBuckets:  make(map[string]*rateBucket),
		logger:       logger,
	}
}

// SetPubSub sets the GossipSub instance.
func (tm *TopicManager) SetPubSub(ps *pubsub.PubSub) {
	tm.pubsub = ps
}

// Join joins a topic by advertising, finding peers, and optionally subscribing via GossipSub.
func (tm *TopicManager) Join(topic string) error {
	if topic == "" {
		return fmt.Errorf("topic is required")
	}
	if len(topic) > MaxTopicLength {
		return fmt.Errorf("topic too long (max %d bytes)", MaxTopicLength)
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, ok := tm.topics[topic]; ok {
		return nil
	}
	if len(tm.topics) >= MaxTopics {
		return fmt.Errorf("too many topics (max %d)", MaxTopics)
	}

	rd := tm.discovery.RoutingDiscovery()
	if rd == nil {
		return fmt.Errorf("DHT not available: cannot join internet topics. Check network connectivity")
	}

	namespace := TopicNamespace + "/" + topic
	topicCtx, cancel := context.WithCancel(tm.ctx)
	tm.topics[topic] = cancel

	dutil.Advertise(topicCtx, rd, namespace)

	// Background peer finder
	tm.wg.Add(1)
	go func() {
		defer tm.wg.Done()
		ticker := time.NewTicker(FindPeersInterval)
		defer ticker.Stop()

		tm.findPeers(topicCtx, namespace)

		for {
			select {
			case <-topicCtx.Done():
				return
			case <-ticker.C:
				tm.findPeers(topicCtx, namespace)
			}
		}
	}()

	// GossipSub subscription
	if tm.pubsub != nil {
		psTopic, err := tm.pubsub.Join(namespace)
		if err != nil {
			tm.logger.Printf("GossipSub topic join failed for %s: %v", topic, err)
		} else {
			tm.pubsubTopics[topic] = psTopic
			sub, err := psTopic.Subscribe()
			if err != nil {
				tm.logger.Printf("GossipSub subscribe failed for %s: %v", topic, err)
				psTopic.Close()
				delete(tm.pubsubTopics, topic)
			} else {
				tm.pubsubSubs[topic] = sub
				tm.wg.Add(1)
				go func() {
					defer tm.wg.Done()
					tm.readSubscription(topicCtx, sub, topic)
				}()
			}
		}
	}

	tm.logger.Printf("joined topic: %s", topic)
	return nil
}

func (tm *TopicManager) readSubscription(ctx context.Context, sub *pubsub.Subscription, topic string) {
	for {
		msg, err := sub.Next(ctx)
		if err != nil {
			return
		}
		if msg.ReceivedFrom == tm.host.ID() || msg.Local {
			continue
		}
		var m Message
		if err := json.Unmarshal(msg.Data, &m); err != nil {
			tm.logger.Printf("GossipSub message unmarshal error on %s: %v", topic, err)
			continue
		}
		// MAJ-005: Drop oversized messages
		if len(m.Content) > MaxMessageSize {
			tm.logger.Printf("dropping oversized GossipSub message from %s (%d bytes)", msg.ReceivedFrom, len(m.Content))
			continue
		}
		// MAJ-001: Read handler under lock to prevent race
		tm.mu.RLock()
		handler := tm.messageHandler
		tm.mu.RUnlock()
		if handler != nil {
			handler(topic, m, msg.ReceivedFrom)
		} else {
			tm.inbox.Push(InboxMessage{
				Message:    m,
				ReceivedAt: time.Now().UTC().Format(time.RFC3339),
			})
		}
	}
}

// SetMessageHandler sets a custom handler for GossipSub messages.
// If set, messages are routed here instead of the inbox.
func (tm *TopicManager) SetMessageHandler(handler func(topic string, msg Message, from peer.ID)) {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	tm.messageHandler = handler
}

// JoinLocal joins a GossipSub topic without DHT rendezvous. For system topics like _meta.
func (tm *TopicManager) JoinLocal(topic string) error {
	if topic == "" {
		return fmt.Errorf("topic is required")
	}
	if len(topic) > MaxTopicLength {
		return fmt.Errorf("topic too long (max %d bytes)", MaxTopicLength)
	}

	tm.mu.Lock()
	defer tm.mu.Unlock()

	if _, ok := tm.topics[topic]; ok {
		return nil
	}

	if tm.pubsub == nil {
		return fmt.Errorf("GossipSub not available")
	}

	namespace := TopicNamespace + "/" + topic
	topicCtx, cancel := context.WithCancel(tm.ctx)
	tm.topics[topic] = cancel

	psTopic, err := tm.pubsub.Join(namespace)
	if err != nil {
		cancel()
		delete(tm.topics, topic)
		return fmt.Errorf("GossipSub topic join failed: %w", err)
	}
	tm.pubsubTopics[topic] = psTopic

	sub, err := psTopic.Subscribe()
	if err != nil {
		psTopic.Close()
		cancel()
		delete(tm.topics, topic)
		delete(tm.pubsubTopics, topic)
		return fmt.Errorf("GossipSub subscribe failed: %w", err)
	}
	tm.pubsubSubs[topic] = sub

	tm.wg.Add(1)
	go func() {
		defer tm.wg.Done()
		tm.readSubscription(topicCtx, sub, topic)
	}()

	tm.logger.Printf("joined local topic: %s", topic)
	return nil
}

// Broadcast publishes a broadcast message to a topic via GossipSub.
func (tm *TopicManager) Broadcast(ctx context.Context, topic string, content string, replyTo string) (Message, error) {
	if tm.pubsub == nil {
		return Message{}, fmt.Errorf("GossipSub not available")
	}

	// Rate limit check
	tm.rateMu.Lock()
	bucket, ok := tm.rateBuckets[topic]
	if !ok {
		bucket = newRateBucket(BroadcastRateLimit, BroadcastBurstLimit)
		tm.rateBuckets[topic] = bucket
	}
	allowed := bucket.allow()
	tm.rateMu.Unlock()
	if !allowed {
		return Message{}, fmt.Errorf("broadcast rate limit exceeded for topic %q (max %d/sec)", topic, BroadcastRateLimit)
	}

	tm.mu.RLock()
	psTopic, ok := tm.pubsubTopics[topic]
	tm.mu.RUnlock()
	if !ok {
		return Message{}, fmt.Errorf("not joined to topic: %s", topic)
	}

	msg := Message{
		ID:        GenerateMessageID(tm.host.ID(), &tm.seq),
		From:      tm.host.ID().String(),
		Content:   content,
		Type:      "broadcast",
		Topic:     topic,
		ReplyTo:   replyTo,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(msg)
	if err != nil {
		return Message{}, err
	}
	if len(data) > MaxMessageSize {
		return Message{}, fmt.Errorf("broadcast message too large (%d bytes, max %d)", len(data), MaxMessageSize)
	}
	if err := psTopic.Publish(ctx, data); err != nil {
		return Message{}, err
	}
	return msg, nil
}

func (tm *TopicManager) findPeers(ctx context.Context, namespace string) {
	rd := tm.discovery.RoutingDiscovery()
	if rd == nil {
		return
	}
	peerChan, err := rd.FindPeers(ctx, namespace)
	if err != nil {
		tm.logger.Printf("FindPeers error for %s: %v", namespace, err)
		return
	}
	for pi := range peerChan {
		if ctx.Err() != nil {
			return
		}
		if pi.ID == tm.host.ID() {
			continue
		}
		tm.peerTracker.AddPending(pi.ID, "dht")
		if err := tm.host.Connect(ctx, pi); err != nil {
			// Best-effort
		}
	}
}

// Leave leaves a topic, stopping advertising, peer finding, and GossipSub.
func (tm *TopicManager) Leave(topic string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	cancel, ok := tm.topics[topic]
	if !ok {
		return
	}
	cancel()
	delete(tm.topics, topic)

	if sub, ok := tm.pubsubSubs[topic]; ok {
		sub.Cancel()
		delete(tm.pubsubSubs, topic)
	}
	if psTopic, ok := tm.pubsubTopics[topic]; ok {
		psTopic.Close()
		delete(tm.pubsubTopics, topic)
	}

	tm.logger.Printf("left topic: %s", topic)
}

// Topics returns a sorted list of active topic names.
func (tm *TopicManager) Topics() []string {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	result := make([]string, 0, len(tm.topics))
	for t := range tm.topics {
		result = append(result, t)
	}
	sort.Strings(result)
	return result
}

// Close cancels all topics and waits for background goroutines.
func (tm *TopicManager) Close() error {
	// Collect items under lock, then release before calling GossipSub (MAJ-004)
	tm.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(tm.topics))
	subs := make([]*pubsub.Subscription, 0, len(tm.pubsubSubs))
	psTopics := make([]*pubsub.Topic, 0, len(tm.pubsubTopics))
	for _, cancel := range tm.topics {
		cancels = append(cancels, cancel)
	}
	for _, sub := range tm.pubsubSubs {
		subs = append(subs, sub)
	}
	for _, pst := range tm.pubsubTopics {
		psTopics = append(psTopics, pst)
	}
	tm.topics = make(map[string]context.CancelFunc)
	tm.pubsubSubs = make(map[string]*pubsub.Subscription)
	tm.pubsubTopics = make(map[string]*pubsub.Topic)
	tm.mu.Unlock()

	for _, c := range cancels {
		c()
	}
	for _, s := range subs {
		s.Cancel()
	}
	for _, p := range psTopics {
		p.Close()
	}
	tm.wg.Wait()
	return nil
}
