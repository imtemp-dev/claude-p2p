package node

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jlim/claude-p2p/mcp"
	"github.com/jlim/claude-p2p/p2p"
)

func testLogger() *log.Logger {
	return log.New(os.Stderr, "[node-test] ", log.LstdFlags)
}

func newTestMCPServer(registry *mcp.ToolRegistry, resources *mcp.ResourceRegistry) *mcp.Server {
	in := io.NopCloser(strings.NewReader(""))
	out := &bytes.Buffer{}
	return mcp.NewServer(mcp.ServerInfo{Name: "test", Version: "0.1.0"}, registry, resources, in, out)
}

func newTestNode(t *testing.T, withP2P bool) (*Node, func()) {
	t.Helper()
	logger := testLogger()
	registry := mcp.NewToolRegistry()
	resources := mcp.NewResourceRegistry()
	server := newTestMCPServer(registry, resources)

	var p2pHost *p2p.Host
	if withP2P {
		ctx := context.Background()
		var err error
		p2pHost, err = p2p.NewHostForTest(ctx, logger)
		if err != nil {
			t.Fatalf("create test p2p host: %v", err)
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

	cleanup := func() {
		if p2pHost != nil {
			p2pHost.Close()
		}
	}
	return n, cleanup
}

func callTool(n *Node, name string, args any) (*mcp.ToolResult, error) {
	var rawArgs json.RawMessage
	if args != nil {
		data, err := json.Marshal(args)
		if err != nil {
			return nil, err
		}
		rawArgs = data
	}
	return n.registry.Call(context.Background(), name, rawArgs)
}

// --- list_peers tests ---

func TestHandleListPeers_P2PDisabled(t *testing.T) {
	n, cleanup := newTestNode(t, false)
	defer cleanup()

	result, err := callTool(n, "list_peers", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error result")
	}
	if result.Content[0].Text != "P2P is disabled" {
		t.Errorf("unexpected text: %s", result.Content[0].Text)
	}
}

func TestHandleListPeers_NoPeers(t *testing.T) {
	n, cleanup := newTestNode(t, true)
	defer cleanup()

	result, err := callTool(n, "list_peers", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, `"count":0`) {
		t.Errorf("expected count 0, got: %s", result.Content[0].Text)
	}
}

// --- send_message tests ---

func TestHandleSendMessage_P2PDisabled(t *testing.T) {
	n, cleanup := newTestNode(t, false)
	defer cleanup()

	result, err := callTool(n, "send_message", map[string]string{
		"peer_id": "dummy", "message": "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.Content[0].Text != "P2P is disabled" {
		t.Errorf("expected P2P disabled error, got: %s", result.Content[0].Text)
	}
}

func TestHandleSendMessage_EmptyMessage(t *testing.T) {
	n, cleanup := newTestNode(t, true)
	defer cleanup()

	result, err := callTool(n, "send_message", map[string]string{
		"peer_id": "dummy", "message": "",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.Content[0].Text != "message is required" {
		t.Errorf("expected 'message is required', got: %s", result.Content[0].Text)
	}
}

func TestHandleSendMessage_BothPeerAndTopic(t *testing.T) {
	n, cleanup := newTestNode(t, true)
	defer cleanup()

	result, err := callTool(n, "send_message", map[string]string{
		"peer_id": "some-peer", "topic": "some-topic", "message": "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.Content[0].Text != "provide either peer_id or topic, not both" {
		t.Errorf("unexpected: %s", result.Content[0].Text)
	}
}

func TestHandleSendMessage_NeitherPeerNorTopic(t *testing.T) {
	n, cleanup := newTestNode(t, true)
	defer cleanup()

	result, err := callTool(n, "send_message", map[string]string{
		"message": "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.Content[0].Text != "either peer_id or topic is required" {
		t.Errorf("unexpected: %s", result.Content[0].Text)
	}
}

func TestHandleSendMessage_InvalidPeerID(t *testing.T) {
	n, cleanup := newTestNode(t, true)
	defer cleanup()

	result, err := callTool(n, "send_message", map[string]string{
		"peer_id": "not-a-valid-peer-id", "message": "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content[0].Text, "peer not found") {
		t.Errorf("expected peer not found error, got: %s", result.Content[0].Text)
	}
}

func TestHandleSendMessage_OversizeMessage(t *testing.T) {
	n, cleanup := newTestNode(t, true)
	defer cleanup()

	bigMsg := strings.Repeat("x", p2p.MaxMessageSize+1)
	result, err := callTool(n, "send_message", map[string]string{
		"peer_id": "12D3KooWDpJ7As7BWAwRMfu1VU2WCqNjvq387JEYKDBj4kx6nXTN",
		"message": bigMsg,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content[0].Text, "message too large") {
		t.Errorf("expected message too large error, got: %s", result.Content[0].Text)
	}
}

func TestHandleSendMessage_DirectSuccess(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	host1, err := p2p.NewHostForTest(ctx, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer host1.Close()

	host2, err := p2p.NewHostForTest(ctx, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer host2.Close()

	// Connect host1 to host2
	h1 := host1.LibP2PHost()
	h2 := host2.LibP2PHost()
	h1.Peerstore().AddAddrs(h2.ID(), h2.Addrs(), time.Hour)
	if err := h1.Connect(ctx, h2.Peerstore().PeerInfo(h2.ID())); err != nil {
		t.Fatalf("connect: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Create node with host1
	registry := mcp.NewToolRegistry()
	resources := mcp.NewResourceRegistry()
	server := newTestMCPServer(registry, resources)
	n := &Node{
		mcpServer: server,
		p2pHost:   host1,
		registry:  registry,
		resources: resources,
		logger:    logger,
	}
	n.registerTools()
	n.registerResources()

	result, err := callTool(n, "send_message", map[string]string{
		"peer_id": h2.ID().String(),
		"message": "hello from test",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "Message sent to") {
		t.Errorf("unexpected result: %s", result.Content[0].Text)
	}

	// Verify message arrived in host2's inbox
	time.Sleep(100 * time.Millisecond)
	msgs := host2.Inbox().Pop()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message in host2 inbox, got %d", len(msgs))
	}
	if msgs[0].Content != "hello from test" {
		t.Errorf("unexpected content: %s", msgs[0].Content)
	}
}

// --- join_topic tests ---

func TestHandleJoinTopic_P2PDisabled(t *testing.T) {
	n, cleanup := newTestNode(t, false)
	defer cleanup()

	result, err := callTool(n, "join_topic", map[string]string{"topic": "test"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.Content[0].Text != "P2P is disabled" {
		t.Errorf("expected P2P disabled error, got: %s", result.Content[0].Text)
	}
}

func TestHandleJoinTopic_EmptyTopic(t *testing.T) {
	n, cleanup := newTestNode(t, true)
	defer cleanup()

	result, err := callTool(n, "join_topic", map[string]string{"topic": ""})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.Content[0].Text != "topic is required" {
		t.Errorf("expected 'topic is required', got: %s", result.Content[0].Text)
	}
}

// --- set_summary tests ---

func TestHandleSetSummary_P2PDisabled(t *testing.T) {
	n, cleanup := newTestNode(t, false)
	defer cleanup()

	result, err := callTool(n, "set_summary", map[string]string{"summary": "test"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.Content[0].Text != "P2P is disabled" {
		t.Errorf("expected P2P disabled error, got: %s", result.Content[0].Text)
	}
}

func TestHandleSetSummary_EmptyString(t *testing.T) {
	n, cleanup := newTestNode(t, true)
	defer cleanup()

	result, err := callTool(n, "set_summary", map[string]string{"summary": ""})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.Content[0].Text != "summary is required" {
		t.Errorf("expected 'summary is required', got: %s", result.Content[0].Text)
	}
}

func TestHandleSetSummary_Success(t *testing.T) {
	n, cleanup := newTestNode(t, true)
	defer cleanup()

	result, err := callTool(n, "set_summary", map[string]string{"summary": "working on auth"})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "Summary updated") {
		t.Errorf("unexpected result: %s", result.Content[0].Text)
	}
}

// --- get_messages tests ---

func TestHandleGetMessages_P2PDisabled(t *testing.T) {
	n, cleanup := newTestNode(t, false)
	defer cleanup()

	result, err := callTool(n, "get_messages", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || result.Content[0].Text != "P2P is disabled" {
		t.Errorf("expected P2P disabled error, got: %s", result.Content[0].Text)
	}
}

func TestHandleGetMessages_EmptyInbox(t *testing.T) {
	n, cleanup := newTestNode(t, true)
	defer cleanup()

	result, err := callTool(n, "get_messages", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, `"messages":[]`) {
		t.Errorf("expected empty messages array, got: %s", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, `"count":0`) {
		t.Errorf("expected count 0, got: %s", result.Content[0].Text)
	}
}

func TestHandleGetMessages_WithMessages(t *testing.T) {
	n, cleanup := newTestNode(t, true)
	defer cleanup()

	// Push a message directly into the inbox
	n.p2pHost.Inbox().Push(p2p.InboxMessage{
		Message: p2p.Message{
			ID:      "test-1",
			From:    "peer-abc",
			Content: "hello world",
			Type:    "direct",
		},
		ReceivedAt: time.Now().UTC().Format(time.RFC3339),
	})

	result, err := callTool(n, "get_messages", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, `"count":1`) {
		t.Errorf("expected count 1, got: %s", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "hello world") {
		t.Errorf("expected message content, got: %s", result.Content[0].Text)
	}

	// After pop, inbox should be empty
	if n.p2pHost.Inbox().Len() != 0 {
		t.Errorf("expected empty inbox after pop, got %d", n.p2pHost.Inbox().Len())
	}
}

func TestHandleGetMessages_Peek(t *testing.T) {
	n, cleanup := newTestNode(t, true)
	defer cleanup()

	n.p2pHost.Inbox().Push(p2p.InboxMessage{
		Message: p2p.Message{
			ID:      "test-1",
			From:    "peer-abc",
			Content: "peek me",
			Type:    "direct",
		},
		ReceivedAt: time.Now().UTC().Format(time.RFC3339),
	})

	result, err := callTool(n, "get_messages", map[string]bool{"peek": true})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "peek me") {
		t.Errorf("expected message content, got: %s", result.Content[0].Text)
	}

	// After peek, inbox should still have the message
	if n.p2pHost.Inbox().Len() != 1 {
		t.Errorf("expected 1 message after peek, got %d", n.p2pHost.Inbox().Len())
	}
}

// --- NewForTest tests ---

func TestNewForTest_NilP2P(t *testing.T) {
	logger := testLogger()
	server := newTestMCPServer(mcp.NewToolRegistry(), mcp.NewResourceRegistry())
	n := NewForTest(server, nil, logger)
	if n == nil {
		t.Fatal("expected non-nil node")
	}

	// Tools should be registered even without P2P
	tools := n.registry.List()
	if len(tools) != 6 {
		t.Errorf("expected 5 tools, got %d", len(tools))
	}

	// All tools should return P2P disabled
	for _, tool := range tools {
		result, err := n.registry.Call(context.Background(), tool.Name, nil)
		if err != nil {
			t.Errorf("tool %s: unexpected error: %v", tool.Name, err)
			continue
		}
		if !result.IsError {
			t.Errorf("tool %s: expected error result with nil P2P", tool.Name)
		}
	}
}

func TestNewForTest_WithP2P(t *testing.T) {
	logger := testLogger()
	ctx := context.Background()

	p2pHost, err := p2p.NewHostForTest(ctx, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer p2pHost.Close()

	server := newTestMCPServer(mcp.NewToolRegistry(), mcp.NewResourceRegistry())
	n := NewForTest(server, p2pHost, logger)
	if n == nil {
		t.Fatal("expected non-nil node")
	}

	// Resources should be registered with P2P
	resources := n.resources.List()
	found := false
	for _, r := range resources {
		if r.URI == "p2p://inbox" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected p2p://inbox resource to be registered")
	}
}

// --- nickname resolve tests ---

func TestHandleSendMessage_ResolveByName(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	host1, err := p2p.NewHostForTest(ctx, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer host1.Close()

	host2, err := p2p.NewHostForTest(ctx, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer host2.Close()

	// Connect
	h1 := host1.LibP2PHost()
	h2 := host2.LibP2PHost()
	h1.Peerstore().AddAddrs(h2.ID(), h2.Addrs(), time.Hour)
	if err := h1.Connect(ctx, h2.Peerstore().PeerInfo(h2.ID())); err != nil {
		t.Fatalf("connect: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	// Set metadata with display name on host1's tracker for host2
	host1.PeerTracker().SetMetadata(h2.ID(), p2p.PeerMetadata{
		DisplayName: "test-swift-fox",
	})

	// Create node with host1
	registry := mcp.NewToolRegistry()
	resources := mcp.NewResourceRegistry()
	server := newTestMCPServer(registry, resources)
	n := &Node{
		mcpServer: server, p2pHost: host1,
		registry: registry, resources: resources, logger: logger,
	}
	n.registerTools()
	n.registerResources()

	// Send message using display name
	result, err := callTool(n, "send_message", map[string]string{
		"peer_id": "test-swift-fox",
		"message": "hello by name",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}
	if !strings.Contains(result.Content[0].Text, "Message sent to") {
		t.Errorf("unexpected result: %s", result.Content[0].Text)
	}

	// Verify message arrived
	time.Sleep(100 * time.Millisecond)
	msgs := host2.Inbox().Pop()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Content != "hello by name" {
		t.Errorf("unexpected content: %s", msgs[0].Content)
	}
}

func TestHandleSendMessage_NameNotFound(t *testing.T) {
	n, cleanup := newTestNode(t, true)
	defer cleanup()

	result, err := callTool(n, "send_message", map[string]string{
		"peer_id": "nonexistent-name",
		"message": "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content[0].Text, "peer not found") {
		t.Errorf("expected 'peer not found', got: %s", result.Content[0].Text)
	}
}

func TestHandleSendMessage_DuplicateName(t *testing.T) {
	ctx := context.Background()
	logger := testLogger()

	host1, err := p2p.NewHostForTest(ctx, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer host1.Close()

	host2, err := p2p.NewHostForTest(ctx, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer host2.Close()

	host3, err := p2p.NewHostForTest(ctx, logger)
	if err != nil {
		t.Fatal(err)
	}
	defer host3.Close()

	h1 := host1.LibP2PHost()
	h2 := host2.LibP2PHost()
	h3 := host3.LibP2PHost()

	// Connect host1 to host2 and host3
	h1.Peerstore().AddAddrs(h2.ID(), h2.Addrs(), time.Hour)
	h1.Connect(ctx, h2.Peerstore().PeerInfo(h2.ID()))
	h1.Peerstore().AddAddrs(h3.ID(), h3.Addrs(), time.Hour)
	h1.Connect(ctx, h3.Peerstore().PeerInfo(h3.ID()))
	time.Sleep(200 * time.Millisecond)

	// Set same display name for both
	host1.PeerTracker().SetMetadata(h2.ID(), p2p.PeerMetadata{DisplayName: "duplicate-name"})
	host1.PeerTracker().SetMetadata(h3.ID(), p2p.PeerMetadata{DisplayName: "duplicate-name"})

	registry := mcp.NewToolRegistry()
	resources := mcp.NewResourceRegistry()
	server := newTestMCPServer(registry, resources)
	n := &Node{
		mcpServer: server, p2pHost: host1,
		registry: registry, resources: resources, logger: logger,
	}
	n.registerTools()
	n.registerResources()

	result, err := callTool(n, "send_message", map[string]string{
		"peer_id": "duplicate-name",
		"message": "hello",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || !strings.Contains(result.Content[0].Text, "multiple peers named") {
		t.Errorf("expected duplicate name error, got: %s", result.Content[0].Text)
	}
}

// setupSendTest creates two connected p2p hosts with a Node wrapping the first.
func setupSendTest(t *testing.T) (n *Node, receiver *p2p.Host, cleanup func()) {
	t.Helper()
	ctx := context.Background()
	logger := testLogger()

	sender, err := p2p.NewHostForTest(ctx, logger)
	if err != nil {
		t.Fatal(err)
	}
	recv, err := p2p.NewHostForTest(ctx, logger)
	if err != nil {
		sender.Close()
		t.Fatal(err)
	}

	sh := sender.LibP2PHost()
	rh := recv.LibP2PHost()
	sh.Peerstore().AddAddrs(rh.ID(), rh.Addrs(), time.Hour)
	if err := sh.Connect(ctx, rh.Peerstore().PeerInfo(rh.ID())); err != nil {
		sender.Close()
		recv.Close()
		t.Fatalf("connect: %v", err)
	}
	time.Sleep(200 * time.Millisecond)

	registry := mcp.NewToolRegistry()
	resources := mcp.NewResourceRegistry()
	server := newTestMCPServer(registry, resources)
	node := &Node{
		mcpServer: server,
		p2pHost:   sender,
		registry:  registry,
		resources: resources,
		logger:    logger,
	}
	node.registerTools()
	node.registerResources()

	return node, recv, func() {
		sender.Close()
		recv.Close()
	}
}

func TestHandleSendMessage_CodeContentType(t *testing.T) {
	n, receiver, cleanup := setupSendTest(t)
	defer cleanup()

	result, err := callTool(n, "send_message", map[string]string{
		"peer_id":      receiver.LibP2PHost().ID().String(),
		"message":      `func hello() { fmt.Println("hello") }`,
		"content_type": "code",
		"filename":     "hello.go",
		"language":     "go",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}

	time.Sleep(100 * time.Millisecond)
	msgs := receiver.Inbox().Pop()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message in receiver inbox, got %d", len(msgs))
	}
	if msgs[0].ContentType != "code" {
		t.Errorf("content_type = %q, want %q", msgs[0].ContentType, "code")
	}
	if msgs[0].Filename != "hello.go" {
		t.Errorf("filename = %q, want %q", msgs[0].Filename, "hello.go")
	}
	if msgs[0].Language != "go" {
		t.Errorf("language = %q, want %q", msgs[0].Language, "go")
	}
}

func TestHandleSendMessage_PlainTextBackwardCompat(t *testing.T) {
	n, receiver, cleanup := setupSendTest(t)
	defer cleanup()

	// Send without content_type — should still work (backward compat)
	result, err := callTool(n, "send_message", map[string]string{
		"peer_id": receiver.LibP2PHost().ID().String(),
		"message": "plain text message",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Content[0].Text)
	}

	time.Sleep(100 * time.Millisecond)
	msgs := receiver.Inbox().Pop()
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].ContentType != "" {
		t.Errorf("expected empty content_type for plain text, got %q", msgs[0].ContentType)
	}
	if msgs[0].Content != "plain text message" {
		t.Errorf("content = %q, want %q", msgs[0].Content, "plain text message")
	}
}

func TestHandleSendMessage_InvalidContentType(t *testing.T) {
	n, cleanup := newTestNode(t, true)
	defer cleanup()

	result, err := callTool(n, "send_message", map[string]string{
		"peer_id":      "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N",
		"message":      "hello",
		"content_type": "binary",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid content_type")
	}
	if !strings.Contains(result.Content[0].Text, "invalid content_type") {
		t.Errorf("unexpected error message: %s", result.Content[0].Text)
	}
}

func TestHandleSendMessage_FilenameWithoutCodeType(t *testing.T) {
	n, cleanup := newTestNode(t, true)
	defer cleanup()

	result, err := callTool(n, "send_message", map[string]string{
		"peer_id":  "QmYyQSo1c1Ym7orWxLYvCrM2EmxFTANf8wXmmE7DWjhx5N",
		"message":  "hello",
		"filename": "hello.go",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("expected error when filename given without content_type=code")
	}
	if !strings.Contains(result.Content[0].Text, "filename and language") {
		t.Errorf("unexpected error message: %s", result.Content[0].Text)
	}
}

// Suppress unused import warning for fmt
var _ = fmt.Sprintf
