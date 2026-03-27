package node

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"

	"github.com/jlim/claude-p2p/mcp"
	"github.com/jlim/claude-p2p/p2p"
)

const (
	getMessagesToolName    = "get_messages"
	getMessagesDefaultDesc = "Get pending messages from inbox"
)

// Node orchestrates the MCP server and P2P host lifecycle.
type Node struct {
	mcpServer *mcp.Server
	p2pHost   *p2p.Host
	registry  *mcp.ToolRegistry
	resources *mcp.ResourceRegistry
	logger    *log.Logger
}

// New creates a new Node with a P2P host and MCP server.
func New(ctx context.Context) (*Node, error) {
	logger := log.New(os.Stderr, "[claude-p2p] ", log.LstdFlags)

	// Create MCP server first (P2P host needs its LastToolCallTime callback)
	registry := mcp.NewToolRegistry()
	resources := mcp.NewResourceRegistry()
	server := mcp.NewServer(
		mcp.ServerInfo{Name: "claude-p2p", Version: "0.1.0"},
		registry, resources, os.Stdin, os.Stdout,
	)

	p2pHost, err := p2p.NewHost(ctx, logger, server.LastToolCallTime)
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

	n := &Node{
		mcpServer: server,
		p2pHost:   p2pHost,
		registry:  registry,
		resources: resources,
		logger:    logger,
	}
	n.registerTools()
	n.registerResources()
	return n, nil
}

// NewForTest creates a Node with injected dependencies for testing.
func NewForTest(mcpServer *mcp.Server, p2pHost *p2p.Host, logger *log.Logger) *Node {
	registry := mcp.NewToolRegistry()
	resources := mcp.NewResourceRegistry()
	n := &Node{
		mcpServer: mcpServer,
		p2pHost:   p2pHost,
		registry:  registry,
		resources: resources,
		logger:    logger,
	}
	n.registerTools()
	n.registerResources()
	return n
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
		InputSchema: json.RawMessage(`{"type":"object","properties":{"peer_id":{"type":"string","description":"Target peer ID or display name (for direct message)"},"message":{"type":"string","description":"Message content"},"topic":{"type":"string","description":"Topic to broadcast to (alternative to peer_id)"},"reply_to":{"type":"string","description":"Message ID to reply to (for conversation threading)"}},"required":["message"]}`),
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
		Name:        getMessagesToolName,
		Description: getMessagesDefaultDesc,
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
			pi.DisplayName = tp.Metadata.DisplayName
			pi.Status = tp.Metadata.Status
			pi.IdleSince = tp.Metadata.IdleSince
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
		ReplyTo string `json:"reply_to"`
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

	if len(params.Message) > p2p.MaxMessageSize {
		return &mcp.ToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: fmt.Sprintf("message too large (%d bytes, max %d)", len(params.Message), p2p.MaxMessageSize)}},
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
		if err := n.p2pHost.TopicManager().Broadcast(ctx, params.Topic, params.Message, params.ReplyTo); err != nil {
			return &mcp.ToolResult{
				Content: []mcp.ContentItem{{Type: "text", Text: err.Error()}},
				IsError: true,
			}, nil
		}
		return &mcp.ToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: fmt.Sprintf("Broadcast sent to topic: %s", params.Topic)}},
		}, nil
	}

	// Direct message path — resolve peer ID or display name
	decodedPeerID, err := peer.Decode(params.PeerID)
	if err != nil {
		// Not a valid peer ID, try name resolution
		matches := n.p2pHost.PeerTracker().FindByName(params.PeerID)
		switch len(matches) {
		case 0:
			return &mcp.ToolResult{
				Content: []mcp.ContentItem{{Type: "text", Text: fmt.Sprintf("peer not found: '%s'", params.PeerID)}},
				IsError: true,
			}, nil
		case 1:
			decodedPeerID = matches[0]
		default:
			ids := make([]string, len(matches))
			for i, id := range matches {
				ids[i] = id.String()
			}
			return &mcp.ToolResult{
				Content: []mcp.ContentItem{{Type: "text", Text: fmt.Sprintf("multiple peers named '%s', use peer_id instead: [%s]", params.PeerID, strings.Join(ids, ", "))}},
				IsError: true,
			}, nil
		}
	}

	if err := n.p2pHost.Messenger().SendDirect(ctx, decodedPeerID, params.Message, params.ReplyTo); err != nil {
		errMsg := err.Error()
		if params.PeerID != decodedPeerID.String() {
			errMsg = fmt.Sprintf("failed to reach '%s' (%s): %v", params.PeerID, decodedPeerID, err)
		}
		return &mcp.ToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: errMsg}},
			IsError: true,
		}, nil
	}

	successMsg := fmt.Sprintf("Message sent to %s", params.PeerID)
		if params.PeerID != decodedPeerID.String() {
			successMsg = fmt.Sprintf("Message sent to %s (%s)", params.PeerID, decodedPeerID)
		}
		return &mcp.ToolResult{
			Content: []mcp.ContentItem{{Type: "text", Text: successMsg}},
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
		// Sync resource description after clearing inbox and notify client
		if len(messages) > 0 {
			n.syncInboxDescription(0, nil)
			if n.mcpServer.IsInitialized() {
				if n.mcpServer.IsSubscribed("p2p://inbox") {
					if err := n.mcpServer.SendNotification("notifications/resources/updated",
						mcp.ResourcesUpdatedParams{URI: "p2p://inbox"}); err != nil {
						n.logger.Printf("send resources/updated notification: %v", err)
					}
				}
				if err := n.mcpServer.SendNotification("notifications/resources/list_changed", nil); err != nil {
					n.logger.Printf("send resources/list_changed notification: %v", err)
				}
			}
			// Reset tool description to default
			n.registry.UpdateDescription(getMessagesToolName, getMessagesDefaultDesc)
			if n.mcpServer.IsInitialized() {
				if err := n.mcpServer.SendNotification("notifications/tools/list_changed", nil); err != nil {
					n.logger.Printf("send tools/list_changed notification: %v", err)
				}
			}
		}
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

func (n *Node) registerResources() {
	if n.p2pHost == nil {
		return
	}

	n.resources.Register(mcp.Resource{
		URI:         "p2p://inbox",
		Name:        "P2P Inbox",
		Description: "No unread messages",
		MimeType:    "application/json",
		Annotations: &mcp.ResourceAnnotations{
			Audience: []string{"assistant"},
			Priority: mcp.Float64Ptr(1.0),
		},
	}, n.readInboxResource)

	n.p2pHost.Inbox().SetOnPush(n.onInboxPush)
}

func (n *Node) readInboxResource() (*mcp.ResourcesReadResult, error) {
	messages := n.p2pHost.Inbox().Peek()

	var latestMsg *p2p.InboxMessage
	if len(messages) > 0 {
		latestMsg = &messages[len(messages)-1]
	}
	n.syncInboxDescription(len(messages), latestMsg)

	data, err := json.Marshal(struct {
		Messages []p2p.InboxMessage `json:"messages"`
		Count    int                `json:"count"`
	}{
		Messages: messages,
		Count:    len(messages),
	})
	if err != nil {
		return nil, fmt.Errorf("serialize inbox: %w", err)
	}

	return &mcp.ResourcesReadResult{
		Contents: []mcp.ResourceContents{{
			URI:      "p2p://inbox",
			MimeType: "application/json",
			Text:     string(data),
		}},
	}, nil
}

func (n *Node) syncInboxDescription(count int, latestMsg *p2p.InboxMessage) {
	var desc string
	if count == 0 {
		desc = "No unread messages"
	} else if latestMsg != nil {
		from := latestMsg.From
		if len(from) > 12 {
			from = from[:12] + "..."
		}
		desc = fmt.Sprintf("%d unread message(s), latest from %s", count, from)
	} else {
		desc = fmt.Sprintf("%d unread message(s)", count)
	}
	n.resources.UpdateDescription("p2p://inbox", desc, time.Now().UTC().Format(time.RFC3339))
}

func (n *Node) onInboxPush(msg p2p.InboxMessage) {
	count := n.p2pHost.Inbox().Len()
	n.syncInboxDescription(count, &msg)

	// Real message notifications (skip _meta/metadata)
	if msg.Topic != "_meta" && msg.Type != "metadata" && count > 0 {
		// Tool description update
		from := msg.From
		if len(from) > 12 {
			from = from[:12] + "..."
		}
		desc := fmt.Sprintf("⚠ %d unread message(s) (latest from %s). %s", count, from, getMessagesDefaultDesc)
		n.registry.UpdateDescription(getMessagesToolName, desc)

		if n.mcpServer.IsInitialized() {
			if err := n.mcpServer.SendNotification("notifications/tools/list_changed", nil); err != nil {
				n.logger.Printf("send tools/list_changed notification: %v", err)
			}

			// Channel notification — pushes message directly into Claude's context
			// Skip in lazy mode: let Claude poll via get_messages instead
			if os.Getenv("CLAUDE_P2P_MODE") != "lazy" {
				meta := map[string]string{
					"from":       msg.From,
					"type":       msg.Type,
					"message_id": msg.ID,
				}
				if msg.Topic != "" {
					meta["topic"] = msg.Topic
				}
				if msg.ReplyTo != "" {
					meta["reply_to"] = msg.ReplyTo
				}
				if err := n.mcpServer.SendNotification("notifications/claude/channel",
					mcp.ChannelNotificationParams{Content: msg.Content, Meta: meta}); err != nil {
					n.logger.Printf("send channel notification: %v", err)
				}
			}
		}
	}

	if !n.mcpServer.IsInitialized() {
		return
	}

	if count == 0 {
		return
	}

	// Resource notifications — also skip _meta to avoid flooding every 60s
	if msg.Topic != "_meta" && msg.Type != "metadata" {
		if n.mcpServer.IsSubscribed("p2p://inbox") {
			if err := n.mcpServer.SendNotification("notifications/resources/updated",
				mcp.ResourcesUpdatedParams{URI: "p2p://inbox"}); err != nil {
				n.logger.Printf("send resources/updated notification: %v", err)
			}
		}

		if err := n.mcpServer.SendNotification("notifications/resources/list_changed", nil); err != nil {
			n.logger.Printf("send resources/list_changed notification: %v", err)
		}
	}

	// OS desktop notification (has its own _meta filter inside)
	n.sendDesktopNotification(msg)
}

func (n *Node) sendDesktopNotification(msg p2p.InboxMessage) {
	// Skip metadata broadcasts — only notify for real messages
	if msg.Topic == "_meta" || msg.Type == "metadata" {
		return
	}

	from := msg.From
	if len(from) > 12 {
		from = from[:12] + "..."
	}

	title := fmt.Sprintf("claude-p2p: %s", from)
	body := msg.Content
	if len(body) > 100 {
		body = body[:100] + "..."
	}

	switch runtime.GOOS {
	case "darwin":
		go exec.Command("osascript", "-e",
			fmt.Sprintf(`display notification %q with title %q`, body, title),
		).Run()
	case "linux":
		go exec.Command("notify-send", title, body).Run()
	}
}
