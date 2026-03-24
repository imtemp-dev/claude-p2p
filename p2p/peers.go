package p2p

import (
	"context"
	"log"
	"sort"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/event"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

// TrackedPeer holds information about a currently connected peer.
type TrackedPeer struct {
	ID          peer.ID
	Addrs       []multiaddr.Multiaddr
	ConnectedAt time.Time
	Source      string
	Metadata    *PeerMetadata
}

// PeerTracker maintains a thread-safe set of connected peers via the event bus.
type PeerTracker struct {
	host           host.Host
	mu             sync.RWMutex
	peers          map[peer.ID]*TrackedPeer
	pendingSources map[peer.ID]string
	sub            event.Subscription
	done           chan struct{}
	closed         bool
	logger         *log.Logger
}

// NewPeerTracker creates a peer tracker and subscribes to connection events.
func NewPeerTracker(h host.Host, logger *log.Logger) (*PeerTracker, error) {
	sub, err := h.EventBus().Subscribe(new(event.EvtPeerConnectednessChanged))
	if err != nil {
		return nil, err
	}
	return &PeerTracker{
		host:           h,
		peers:          make(map[peer.ID]*TrackedPeer),
		pendingSources: make(map[peer.ID]string),
		sub:            sub,
		done:           make(chan struct{}),
		logger:         logger,
	}, nil
}

// Start begins processing connection events in a background goroutine.
func (pt *PeerTracker) Start(_ context.Context) {
	go func() {
		defer close(pt.done)
		for evt := range pt.sub.Out() {
			e, ok := evt.(event.EvtPeerConnectednessChanged)
			if !ok {
				continue
			}
			pt.mu.Lock()
			if e.Connectedness == network.Connected {
				source := "direct"
				if s, ok := pt.pendingSources[e.Peer]; ok {
					source = s
					delete(pt.pendingSources, e.Peer)
				}
				pt.peers[e.Peer] = &TrackedPeer{
					ID:          e.Peer,
					ConnectedAt: time.Now(),
					Source:      source,
				}
			} else if e.Connectedness == network.NotConnected {
				delete(pt.peers, e.Peer)
				delete(pt.pendingSources, e.Peer)
			}
			pt.mu.Unlock()
		}
	}()
}

// Close stops the event processing goroutine and marks the tracker as closed.
func (pt *PeerTracker) Close() error {
	pt.sub.Close()
	<-pt.done
	pt.mu.Lock()
	pt.closed = true
	pt.mu.Unlock()
	return nil
}

const maxPendingSources = 1000

// AddPending registers a discovery source for a peer before connection is established.
// If the peer is already connected, updates the source directly.
func (pt *PeerTracker) AddPending(id peer.ID, source string) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if p, ok := pt.peers[id]; ok {
		p.Source = source
		return
	}
	if len(pt.pendingSources) >= maxPendingSources {
		return
	}
	pt.pendingSources[id] = source
}

// Peers returns a snapshot of all connected peers, sorted by ID.
func (pt *PeerTracker) Peers() []TrackedPeer {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	if pt.closed {
		return nil
	}
	result := make([]TrackedPeer, 0, len(pt.peers))
	for _, p := range pt.peers {
		tp := *p
		tp.Addrs = pt.host.Peerstore().Addrs(p.ID)
		result = append(result, tp)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID.String() < result[j].ID.String()
	})
	return result
}

// PeersBySource returns peers filtered by discovery source.
func (pt *PeerTracker) PeersBySource(source string) []TrackedPeer {
	all := pt.Peers()
	filtered := make([]TrackedPeer, 0)
	for _, p := range all {
		if p.Source == source {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// Count returns the number of connected peers. Returns 0 after Close.
func (pt *PeerTracker) Count() int {
	pt.mu.RLock()
	defer pt.mu.RUnlock()
	if pt.closed {
		return 0
	}
	return len(pt.peers)
}

// SetMetadata stores metadata for a connected peer. No-op if peer not found.
func (pt *PeerTracker) SetMetadata(id peer.ID, meta PeerMetadata) {
	pt.mu.Lock()
	defer pt.mu.Unlock()
	if p, ok := pt.peers[id]; ok {
		pt.peers[id] = &TrackedPeer{
			ID:          p.ID,
			Addrs:       p.Addrs,
			ConnectedAt: p.ConnectedAt,
			Source:      p.Source,
			Metadata:    &meta,
		}
	}
}
