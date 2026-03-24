package p2p

import (
	"context"
	"errors"
	"log"
	"sync"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/discovery/mdns"
	drouting "github.com/libp2p/go-libp2p/p2p/discovery/routing"

	dht "github.com/libp2p/go-libp2p-kad-dht"
)

// Discovery coordinates mDNS (LAN) and DHT (internet) peer discovery.
type Discovery struct {
	mu               sync.RWMutex
	ctx              context.Context
	host             host.Host
	mdnsService      mdns.Service
	dht              *dht.IpfsDHT
	routingDiscovery *drouting.RoutingDiscovery
	peerTracker      *PeerTracker
	logger           *log.Logger
}

// NewDiscovery creates a discovery coordinator without starting any services.
func NewDiscovery(h host.Host, tracker *PeerTracker, logger *log.Logger) *Discovery {
	return &Discovery{
		host:        h,
		peerTracker: tracker,
		logger:      logger,
	}
}

// HandlePeerFound implements mdns.Notifee. Called when mDNS discovers a peer.
func (d *Discovery) HandlePeerFound(pi peer.AddrInfo) {
	if pi.ID == d.host.ID() {
		return
	}
	d.logger.Printf("mDNS: discovered peer %s", pi.ID.String())
	d.peerTracker.AddPending(pi.ID, "mdns")

	d.mu.RLock()
	ctx := d.ctx
	d.mu.RUnlock()
	if ctx == nil {
		ctx = context.Background()
	}
	if err := d.host.Connect(ctx, pi); err != nil {
		d.logger.Printf("mDNS: failed to connect to %s: %v", pi.ID.String(), err)
	}
}

// StartMDNS starts mDNS discovery with the given service name.
func (d *Discovery) StartMDNS(serviceName string) error {
	s := mdns.NewMdnsService(d.host, serviceName, d)
	if err := s.Start(); err != nil {
		return err
	}
	d.mdnsService = s
	d.logger.Println("mDNS discovery started")
	return nil
}

// StartDHT starts the Kademlia DHT, bootstraps, and creates routing discovery.
func (d *Discovery) StartDHT(ctx context.Context) error {
	kademliaDHT, err := dht.New(ctx, d.host, dht.Mode(dht.ModeAutoServer))
	if err != nil {
		return err
	}

	if err := kademliaDHT.Bootstrap(ctx); err != nil {
		d.logger.Printf("DHT bootstrap warning: %v (DHT may work later via peer exchange)", err)
	}

	rd := drouting.NewRoutingDiscovery(kademliaDHT)

	d.mu.Lock()
	d.ctx = ctx
	d.dht = kademliaDHT
	d.routingDiscovery = rd
	d.mu.Unlock()

	d.logger.Println("DHT started")
	return nil
}

// RoutingDiscovery returns the DHT-based routing discovery. Returns nil if DHT not started.
func (d *Discovery) RoutingDiscovery() *drouting.RoutingDiscovery {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.routingDiscovery
}

// Close stops mDNS and DHT services.
func (d *Discovery) Close() error {
	var errs []error
	if d.mdnsService != nil {
		if err := d.mdnsService.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	if d.dht != nil {
		if err := d.dht.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
