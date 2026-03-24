package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/jlim/claude-p2p/mcp"
	"github.com/jlim/claude-p2p/p2p"
)

// Node orchestrates the MCP server and P2P host lifecycle.
type Node struct {
	mcpServer *mcp.Server
	p2pHost   *p2p.Host
	registry  *mcp.ToolRegistry
	logger    *log.Logger
}

// New creates a new Node with a P2P host and MCP server.
func New(ctx context.Context) (*Node, error) {
	logger := log.New(os.Stderr, "[claude-p2p] ", log.LstdFlags)

	p2pHost, err := p2p.NewHost(ctx, logger)
	if err != nil {
		logger.Printf("P2P host failed: %v. Running without P2P.", err)
	}

	// Auto-join topic from env var
	if topic := os.Getenv("CLAUDE_P2P_TOPIC"); topic != "" && p2pHost != nil {
		if err := p2pHost.TopicManager().Join(topic); err != nil {
			logger.Printf("auto-join topic %q failed: %v", topic, err)
		} else {
			logger.Printf("auto-joined topic: %s", topic)
		}
	}

	registry := mcp.NewToolRegistry()
	server := mcp.NewServer(
		mcp.ServerInfo{Name: "claude-p2p", Version: "0.1.0"},
		registry, os.Stdin, os.Stdout,
	)

	n := &Node{
		mcpServer: server,
		p2pHost:   p2pHost,
		registry:  registry,
		logger:    logger,
	}
	n.registerTools()
	return n, nil
}

// Run starts the MCP server. It blocks until stdin EOF or ctx cancel.
func (n *Node) Run(ctx context.Context) error {
	if n.p2pHost != nil {
		n.logger.Printf("claude-p2p started, peer ID: %s", n.p2pHost.ID())
	} else {
		n.logger.Println("claude-p2p started (P2P disabled)")
	}
	return n.mcpServer.Run(ctx)
}

// Close shuts down the MCP server and P2P host.
func (n *Node) Close() error {
	var errs []error
	if err := n.mcpServer.Close(); err != nil {
		errs = append(errs, err)
	}
	if n.p2pHost != nil {
		if err := n.p2pHost.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (n *Node) registerTools() {
	// Real list_peers handler
	n.registry.Register(mcp.Tool{
		Name:        "list_peers",
		Description: "List connected peers",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"scope":{"type":"string","enum":["local","repo","topic"],"description":"Discovery scope"}},"required":[]}`),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    mcp.BoolPtr(true),
			DestructiveHint: mcp.BoolPtr(false),
			OpenWorldHint:   mcp.BoolPtr(true),
		},
	}, n.handleListPeers)

	// Real send_message handler (direct + broadcast)
	n.registry.Register(mcp.Tool{
		Name:        "send_message",
		Description: "Send a message to a specific peer or broadcast to a topic",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"peer_id":{"type":"string","description":"Target peer ID (for direct message)"},"message":{"type":"string","description":"Message content"},"topic":{"type":"string","description":"Topic to broadcast to (alternative to peer_id)"}},"required":["message"]}`),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    mcp.BoolPtr(false),
			DestructiveHint: mcp.BoolPtr(false),
			OpenWorldHint:   mcp.BoolPtr(true),
		},
	}, n.handleSendMessage)

	n.registry.Register(mcp.Tool{
		Name:        "set_summary",
		Description: "Set current work summary visible to peers",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"summary":{"type":"string","description":"Current work summary"}},"required":["summary"]}`),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    mcp.BoolPtr(false),
			DestructiveHint: mcp.BoolPtr(false),
			OpenWorldHint:   mcp.BoolPtr(true),
		},
	}, n.handleSetSummary)

	// Real join_topic handler
	n.registry.Register(mcp.Tool{
		Name:        "join_topic",
		Description: "Join a team/project topic by shared code",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"topic":{"type":"string","description":"Topic code to join"}},"required":["topic"]}`),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    mcp.BoolPtr(false),
			DestructiveHint: mcp.BoolPtr(false),
			OpenWorldHint:   mcp.BoolPtr(true),
		},
	}, n.handleJoinTopic)

	// Real get_messages handler
	n.registry.Register(mcp.Tool{
		Name:        "get_messages",
		Description: "Get pending messages from inbox",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"peek":{"type":"boolean","description":"If true, don't clear inbox after reading","default":false}},"required":[]}`),
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint:    mcp.BoolPtr(false),
			DestructiveHint: mcp.BoolPtr(false),
			IdempotentHint:  mcp.BoolPtr(false),
			OpenWorldHint:   mcp.BoolPtr(true),
		},
	}, n.handleGetMessages)
}

func (n *Node) handleListPeers(_ context.Context, args json.RawMessage) (*mcp.ToolResult, error) {
	if n.p2pHost == nil {
		return &mcp.ToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: "P2P is disabled"}},
			IsError: true,
		}, nil
	}

	var params struct {
		Scope string `json:"scope"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &params); err != nil {
			return &mcp.ToolResult{
				Content: []mcp.ContentItem{{Type: "text", Text: "invalid arguments: " + err.Error()}},
				IsError: true,
			}, nil
		}
	}

	var peers []p2p.TrackedPeer
	switch params.Scope {
	case "local":
		peers = n.p2pHost.PeerTracker().PeersBySource("mdns")
	case "topic":
		peers = n.p2pHost.PeerTracker().PeersBySource("dht")
	default:
		peers = n.p2pHost.PeerTracker().Peers()
	}

	peerInfos := make([]p2p.PeerInfo, len(peers))
	for i, tp := range peers {
		addrs := make([]string, len(tp.Addrs))
		for j, a := range tp.Addrs {
			addrs[j] = a.String()
		}
		pi := p2p.PeerInfo{
			ID:          tp.ID.String(),
			Addrs:       addrs,
			ConnectedAt: tp.ConnectedAt.Format(time.RFC3339),
			Source:      tp.Source,
		}
		if tp.Metadata != nil {
			pi.Summary = tp.Metadata.Summary
			pi.Username = tp.Metadata.Username
			pi.Repo = tp.Metadata.Repo
			pi.Branch = tp.Metadata.Branch
		}
		peerInfos[i] = pi
	}

	response := struct {
		Peers []p2p.PeerInfo `json:"peers"`
		Count int            `json:"count"`
	}{
		Peers: peerInfos,
		Count: len(peerInfos),
	}

	data, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize peer list: %w", err)
	}
	return &mcp.ToolResult{
		Content: []mcp.ContentItem{{Type: "text", Text: string(data)}},
	}, nil
}

func (n *Node) handleSendMessage(ctx context.Context, args json.RawMessage) (*mcp.ToolResult, error) {
	if n.p2pHost == nil {
		return &mcp.ToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: "P2P is disabled"}},
			IsError: true,
		}, nil
	}

	var params struct {
		PeerID  string `json:"peer_id"`
		Message string `json:"message"`
		Topic   string `json:"topic"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &params); err != nil {
			return &mcp.ToolResult{
				Content: []mcp.ContentItem{{Type: "text", Text: "invalid arguments: " + err.Error()}},
				IsError: true,
			}, nil
		}
	}

	if params.Message == "" {
		return &mcp.ToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: "message is required"}},
			IsError: true,
		}, nil
	}

	if params.PeerID != "" && params.Topic != "" {
		return &mcp.ToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: "provide either peer_id or topic, not both"}},
			IsError: true,
		}, nil
	}

	if params.PeerID == "" && params.Topic == "" {
		return &mcp.ToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: "either peer_id or topic is required"}},
			IsError: true,
		}, nil
	}

	// Broadcast path
	if params.Topic != "" {
		if err := n.p2pHost.TopicManager().Broadcast(ctx, params.Topic, params.Message); err != nil {
			return &mcp.ToolResult{
				Content: []mcp.ContentItem{{Type: "text", Text: err.Error()}},
				IsError: true,
			}, nil
		}
		return &mcp.ToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: fmt.Sprintf("Broadcast sent to topic: %s", params.Topic)}},
		}, nil
	}

	// Direct message path
	decodedPeerID, err := peer.Decode(params.PeerID)
	if err != nil {
		return &mcp.ToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: "invalid peer ID: " + err.Error()}},
			IsError: true,
		}, nil
	}

	if err := n.p2pHost.Messenger().SendDirect(ctx, decodedPeerID, params.Message); err != nil {
		return &mcp.ToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.ToolResult{
		Content: []mcp.ContentItem{{Type: "text", Text: fmt.Sprintf("Message sent to %s", params.PeerID)}},
	}, nil
}

func (n *Node) handleJoinTopic(_ context.Context, args json.RawMessage) (*mcp.ToolResult, error) {
	if n.p2pHost == nil {
		return &mcp.ToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: "P2P is disabled"}},
			IsError: true,
		}, nil
	}

	var params struct {
		Topic string `json:"topic"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &params); err != nil {
			return &mcp.ToolResult{
				Content: []mcp.ContentItem{{Type: "text", Text: "invalid arguments: " + err.Error()}},
				IsError: true,
			}, nil
		}
	}

	if params.Topic == "" {
		return &mcp.ToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: "topic is required"}},
			IsError: true,
		}, nil
	}

	if err := n.p2pHost.TopicManager().Join(params.Topic); err != nil {
		return &mcp.ToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: err.Error()}},
			IsError: true,
		}, nil
	}

	return &mcp.ToolResult{
		Content: []mcp.ContentItem{{Type: "text", Text: fmt.Sprintf("Joined topic: %s. Use list_peers to see connected peers.", params.Topic)}},
	}, nil
}

func (n *Node) handleGetMessages(_ context.Context, args json.RawMessage) (*mcp.ToolResult, error) {
	if n.p2pHost == nil {
		return &mcp.ToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: "P2P is disabled"}},
			IsError: true,
		}, nil
	}

	var params struct {
		Peek bool `json:"peek"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &params); err != nil {
			return &mcp.ToolResult{
				Content: []mcp.ContentItem{{Type: "text", Text: "invalid arguments: " + err.Error()}},
				IsError: true,
			}, nil
		}
	}

	var messages []p2p.InboxMessage
	if params.Peek {
		messages = n.p2pHost.Inbox().Peek()
	} else {
		messages = n.p2pHost.Inbox().Pop()
	}

	if messages == nil {
		messages = []p2p.InboxMessage{} // ensure empty array in JSON, not null
	}

	response := struct {
		Messages []p2p.InboxMessage `json:"messages"`
		Count    int                `json:"count"`
	}{
		Messages: messages,
		Count:    len(messages),
	}

	data, err := json.Marshal(response)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize messages: %w", err)
	}
	return &mcp.ToolResult{
		Content: []mcp.ContentItem{{Type: "text", Text: string(data)}},
	}, nil
}

func (n *Node) handleSetSummary(_ context.Context, args json.RawMessage) (*mcp.ToolResult, error) {
	if n.p2pHost == nil {
		return &mcp.ToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: "P2P is disabled"}},
			IsError: true,
		}, nil
	}

	var params struct {
		Summary string `json:"summary"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &params); err != nil {
			return &mcp.ToolResult{
				Content: []mcp.ContentItem{{Type: "text", Text: "invalid arguments: " + err.Error()}},
				IsError: true,
			}, nil
		}
	}

	if params.Summary == "" {
		return &mcp.ToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: "summary is required"}},
			IsError: true,
		}, nil
	}

	n.p2pHost.MetadataManager().SetSummary(params.Summary)

	return &mcp.ToolResult{
		Content: []mcp.ContentItem{{Type: "text", Text: fmt.Sprintf("Summary updated: %s", params.Summary)}},
	}, nil
}
