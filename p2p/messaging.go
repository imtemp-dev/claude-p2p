package p2p

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/libp2p/go-msgio"
)

const (
	// ProtocolID is the libp2p protocol identifier for direct messages.
	ProtocolID = "/claude-p2p/msg/1.0.0"

	// MaxMessageSize is the maximum message size in bytes (64KB).
	MaxMessageSize = 65536
)

// Message is the wire format for both direct and broadcast messages.
type Message struct {
	ID        string `json:"id"`
	From      string `json:"from"`
	To        string `json:"to,omitempty"`
	Content   string `json:"content"`
	Type      string `json:"type"`
	Topic     string `json:"topic,omitempty"`
	Timestamp string `json:"timestamp"`
}

// Messenger handles sending and receiving messages over libp2p streams.
type Messenger struct {
	host   host.Host
	inbox  *Inbox
	logger *log.Logger
	seq    atomic.Uint64
}

// NewMessenger creates a messenger and registers the stream handler.
func NewMessenger(h host.Host, inbox *Inbox, logger *log.Logger) *Messenger {
	m := &Messenger{host: h, inbox: inbox, logger: logger}
	h.SetStreamHandler(protocol.ID(ProtocolID), m.handleStream)
	return m
}

// SendDirect sends a direct message to a specific peer.
func (m *Messenger) SendDirect(ctx context.Context, peerID peer.ID, content string) error {
	stream, err := m.host.NewStream(ctx, peerID, protocol.ID(ProtocolID))
	if err != nil {
		return fmt.Errorf("open stream: %w", err)
	}
	defer stream.Close()

	msg := Message{
		ID:        GenerateMessageID(m.host.ID(), &m.seq),
		From:      m.host.ID().String(),
		To:        peerID.String(),
		Content:   content,
		Type:      "direct",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(msg)
	if err != nil {
		stream.Reset()
		return err
	}
	if len(data) > MaxMessageSize {
		stream.Reset()
		return fmt.Errorf("message too large (%d bytes, max %d)", len(data), MaxMessageSize)
	}
	writer := msgio.NewWriter(stream)
	if err := writer.WriteMsg(data); err != nil {
		stream.Reset()
		return err
	}
	stream.CloseWrite()
	return nil
}

func (m *Messenger) handleStream(s network.Stream) {
	defer s.Close()
	reader := msgio.NewReaderSize(s, MaxMessageSize)
	data, err := reader.ReadMsg()
	if err != nil {
		m.logger.Printf("message read error from %s: %v", s.Conn().RemotePeer(), err)
		s.Reset()
		return
	}
	defer reader.ReleaseMsg(data)

	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		m.logger.Printf("message unmarshal error: %v", err)
		return
	}
	m.inbox.Push(InboxMessage{
		Message:    msg,
		ReceivedAt: time.Now().UTC().Format(time.RFC3339),
	})
	m.logger.Printf("received direct message from %s", msg.From)
}

// Close removes the stream handler.
func (m *Messenger) Close() error {
	m.host.RemoveStreamHandler(protocol.ID(ProtocolID))
	return nil
}

// GenerateMessageID creates a unique message ID.
func GenerateMessageID(hostID peer.ID, seq *atomic.Uint64) string {
	s := seq.Add(1)
	idStr := hostID.String()
	short := idStr
	if len(idStr) > 8 {
		short = idStr[:8]
	}
	return fmt.Sprintf("%s-%d-%d", short, time.Now().UnixMilli(), s)
}
