package p2p

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/connmgr"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

const (
	// ConnLowWatermark is the connection count below which pruning stops.
	ConnLowWatermark = 50
	// ConnHighWatermark is the connection count above which aggressive pruning begins.
	ConnHighWatermark = 100
	// ConnGracePeriod is how long new connections are immune from pruning.
	ConnGracePeriod = time.Minute
)

// Host wraps a libp2p host with discovery, topic management, messaging, and peer tracking.
type Host struct {
	host         host.Host
	discovery    *Discovery
	topicManager *TopicManager
	peerTracker  *PeerTracker
	messenger       *Messenger
	metadataManager *MetadataManager
	inbox           *Inbox
	ps              *pubsub.PubSub
	logger          *log.Logger
}

// NewHost creates a libp2p host with all subsystems.
func NewHost(ctx context.Context, logger *log.Logger, getLastToolCall func() time.Time) (*Host, error) {
	cm, err := connmgr.NewConnManager(ConnLowWatermark, ConnHighWatermark,
		connmgr.WithGracePeriod(ConnGracePeriod))
	if err != nil {
		return nil, fmt.Errorf("connection manager: %w", err)
	}

	h, err := libp2p.New(
		libp2p.ListenAddrStrings(
			"/ip4/0.0.0.0/tcp/0",
			"/ip4/0.0.0.0/udp/0/quic-v1",
			"/ip6/::/tcp/0",
			"/ip6/::/udp/0/quic-v1",
		),
		libp2p.NATPortMap(),
		libp2p.EnableAutoNATv2(),
		libp2p.EnableHolePunching(),
		libp2p.EnableRelayService(),
		libp2p.ConnectionManager(cm),
	)
	if err != nil {
		return nil, err
	}

	peerTracker, err := NewPeerTracker(h, logger)
	if err != nil {
		h.Close()
		return nil, err
	}
	peerTracker.Start(ctx)

	disc := NewDiscovery(h, peerTracker, logger)

	if err := disc.StartMDNS("claude-p2p"); err != nil {
		logger.Printf("mDNS failed to start: %v (continuing without mDNS)", err)
	}

	if err := disc.StartDHT(ctx); err != nil {
		logger.Printf("DHT failed to start: %v (continuing without DHT)", err)
	}

	inbox := NewInbox(DefaultInboxCapacity)

	// Create GossipSub
	var ps *pubsub.PubSub
	ps, err = pubsub.NewGossipSub(ctx, h)
	if err != nil {
		logger.Printf("GossipSub failed to start: %v (continuing without broadcast)", err)
	}

	tm := NewTopicManager(ctx, h, disc, peerTracker, inbox, logger)
	if ps != nil {
		tm.SetPubSub(ps)
	}

	messenger := NewMessenger(h, inbox, logger)

	// Join metadata topic for work context sharing
	if err := tm.JoinLocal(MetadataTopicName); err != nil {
		logger.Printf("metadata topic join failed: %v (continuing without metadata sharing)", err)
	}

	// Create MetadataManager
	metadataManager := NewMetadataManager(ctx, h, peerTracker, tm, logger, getLastToolCall)

	// Route GossipSub messages: metadata to MetadataManager, others to inbox
	tm.SetMessageHandler(func(topic string, msg Message, from peer.ID) {
		if topic == MetadataTopicName {
			metadataManager.HandleMetadataMessage(msg, from)
		} else {
			inbox.Push(InboxMessage{
				Message:    msg,
				ReceivedAt: time.Now().UTC().Format(time.RFC3339),
			})
		}
	})

	logger.Printf("libp2p peer ID: %s", h.ID())
	for _, addr := range h.Addrs() {
		logger.Printf("listening on: %s/p2p/%s", addr, h.ID())
	}

	return &Host{
		host:            h,
		discovery:       disc,
		topicManager:    tm,
		peerTracker:     peerTracker,
		messenger:       messenger,
		metadataManager: metadataManager,
		inbox:           inbox,
		ps:              ps,
		logger:          logger,
	}, nil
}

// NewHostForTest creates a minimal Host with loopback networking for testing.
// No mDNS, DHT, or GossipSub — only PeerTracker, Inbox, Messenger, and TopicManager.
func NewHostForTest(ctx context.Context, logger *log.Logger) (*Host, error) {
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		return nil, fmt.Errorf("create libp2p host: %w", err)
	}

	tracker, err := NewPeerTracker(h, logger)
	if err != nil {
		h.Close()
		return nil, fmt.Errorf("create peer tracker: %w", err)
	}
	tracker.Start(ctx)

	inbox := NewInbox(DefaultInboxCapacity)
	disc := NewDiscovery(h, tracker, logger)
	tm := NewTopicManager(ctx, h, disc, tracker, inbox, logger)
	messenger := NewMessenger(h, inbox, logger)

	mm := NewMetadataManager(ctx, h, tracker, tm, logger, func() time.Time { return time.Now() })

	return &Host{
		host:            h,
		discovery:       disc,
		topicManager:    tm,
		peerTracker:     tracker,
		messenger:       messenger,
		metadataManager: mm,
		inbox:           inbox,
		logger:          logger,
	}, nil
}

// ID returns the peer ID as a string.
func (h *Host) ID() string {
	return h.host.ID().String()
}

// Addrs returns the listen addresses as strings.
func (h *Host) Addrs() []string {
	addrs := h.host.Addrs()
	result := make([]string, len(addrs))
	for i, addr := range addrs {
		result[i] = addr.String()
	}
	return result
}

// Close shuts down all components in order.
func (h *Host) Close() error {
	var errs []error
	if h.messenger != nil {
		if err := h.messenger.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if h.metadataManager != nil {
		if err := h.metadataManager.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if h.topicManager != nil {
		if err := h.topicManager.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if h.discovery != nil {
		if err := h.discovery.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if h.peerTracker != nil {
		if err := h.peerTracker.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if err := h.host.Close(); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

// LibP2PHost returns the underlying libp2p host.
func (h *Host) LibP2PHost() host.Host {
	return h.host
}

// PeerTracker returns the peer tracker.
func (h *Host) PeerTracker() *PeerTracker {
	return h.peerTracker
}

// TopicManager returns the topic manager.
func (h *Host) TopicManager() *TopicManager {
	return h.topicManager
}

// Discovery returns the discovery coordinator.
func (h *Host) Discovery() *Discovery {
	return h.discovery
}

// Messenger returns the messenger.
func (h *Host) Messenger() *Messenger {
	return h.messenger
}

// Inbox returns the shared inbox.
func (h *Host) Inbox() *Inbox {
	return h.inbox
}

// MetadataManager returns the metadata manager.
func (h *Host) MetadataManager() *MetadataManager {
	return h.metadataManager
}
