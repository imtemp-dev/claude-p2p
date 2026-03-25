package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// helper: create a server with piped input/output
func newTestServer(input string) (*Server, *bytes.Buffer) {
	in := io.NopCloser(strings.NewReader(input))
	out := &bytes.Buffer{}
	registry := NewToolRegistry()
	registry.Register(Tool{
		Name:        "test_tool",
		Description: "A test tool",
		InputSchema: json.RawMessage(`{"type":"object","properties":{}}`),
	}, func(_ context.Context, _ json.RawMessage) (*ToolResult, error) {
		return &ToolResult{
			Content: []ContentItem{{Type: "text", Text: "test result"}},
		}, nil
	})
	server := NewServer(ServerInfo{Name: "test", Version: "0.1.0"}, registry, NewResourceRegistry(), in, out)
	return server, out
}

// helper: parse all JSON-RPC responses from output buffer
func parseResponses(t *testing.T, out *bytes.Buffer) []map[string]any {
	t.Helper()
	var results []map[string]any
	dec := json.NewDecoder(out)
	for dec.More() {
		var msg map[string]any
		if err := dec.Decode(&msg); err != nil {
			break
		}
		results = append(results, msg)
	}
	return results
}

// Scenario 1: Initialize handshake
func TestInitializeHandshake(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
`
	server, out := newTestServer(input)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := server.Run(ctx)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	responses := parseResponses(t, out)
	if len(responses) < 2 {
		t.Fatalf("expected at least 2 responses, got %d", len(responses))
	}

	// Check initialize response
	initResp := responses[0]
	result, ok := initResp["result"].(map[string]any)
	if !ok {
		t.Fatal("initialize response missing result")
	}
	if v, _ := result["protocolVersion"].(string); v != "2025-03-26" {
		t.Errorf("protocolVersion = %q, want %q", v, "2025-03-26")
	}
	serverInfo, _ := result["serverInfo"].(map[string]any)
	if name, _ := serverInfo["name"].(string); name != "test" {
		t.Errorf("serverInfo.name = %q, want %q", name, "test")
	}
	caps, _ := result["capabilities"].(map[string]any)
	tools, _ := caps["tools"].(map[string]any)
	if lc, _ := tools["listChanged"].(bool); !lc {
		t.Error("capabilities.tools.listChanged should be true")
	}
}

// Scenario 2: List tools
func TestListTools(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
`
	server, out := newTestServer(input)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	server.Run(ctx)
	responses := parseResponses(t, out)
	if len(responses) < 2 {
		t.Fatalf("expected at least 2 responses, got %d", len(responses))
	}

	listResp := responses[1]
	result, ok := listResp["result"].(map[string]any)
	if !ok {
		t.Fatal("tools/list response missing result")
	}
	toolsList, ok := result["tools"].([]any)
	if !ok {
		t.Fatal("result missing tools array")
	}
	if len(toolsList) != 1 {
		t.Errorf("expected 1 tool, got %d", len(toolsList))
	}
}

// Scenario 3: Call stub tool
func TestCallTool(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"test_tool","arguments":{}}}
`
	server, out := newTestServer(input)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	server.Run(ctx)
	responses := parseResponses(t, out)
	if len(responses) < 2 {
		t.Fatalf("expected at least 2 responses, got %d", len(responses))
	}

	callResp := responses[1]
	result, ok := callResp["result"].(map[string]any)
	if !ok {
		t.Fatal("tools/call response missing result")
	}
	content, ok := result["content"].([]any)
	if !ok || len(content) == 0 {
		t.Fatal("result missing content array")
	}
	item, _ := content[0].(map[string]any)
	if text, _ := item["text"].(string); text != "test result" {
		t.Errorf("content text = %q, want %q", text, "test result")
	}
}

// Scenario 4: Ping
func TestPing(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"ping"}
`
	server, out := newTestServer(input)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	server.Run(ctx)
	responses := parseResponses(t, out)
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	resp := responses[0]
	if resp["result"] == nil {
		t.Error("ping response should have non-nil result")
	}
	if resp["error"] != nil {
		t.Error("ping response should not have error")
	}
	if id, _ := resp["id"].(float64); id != 1 {
		t.Errorf("response id = %v, want 1", resp["id"])
	}
}

// Scenario 6: Invalid JSON
func TestInvalidJSON(t *testing.T) {
	input := "{broken\n"
	server, out := newTestServer(input)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	server.Run(ctx)
	responses := parseResponses(t, out)
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	resp := responses[0]
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatal("expected error response")
	}
	code, _ := errObj["code"].(float64)
	if code != -32700 {
		t.Errorf("error code = %v, want -32700", code)
	}
}

// Scenario 7: Unknown method
func TestUnknownMethod(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"unknown/method"}
`
	server, out := newTestServer(input)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	server.Run(ctx)
	responses := parseResponses(t, out)
	if len(responses) < 2 {
		t.Fatalf("expected at least 2 responses, got %d", len(responses))
	}

	resp := responses[1]
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatal("expected error response for unknown method")
	}
	code, _ := errObj["code"].(float64)
	if code != -32601 {
		t.Errorf("error code = %v, want -32601", code)
	}
}

// Scenario 8: Pre-init request
func TestPreInitRequest(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}
`
	server, out := newTestServer(input)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	server.Run(ctx)
	responses := parseResponses(t, out)
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	resp := responses[0]
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatal("expected error response for pre-init request")
	}
	code, _ := errObj["code"].(float64)
	if code != -32600 {
		t.Errorf("error code = %v, want -32600", code)
	}
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "not initialized") {
		t.Errorf("error message = %q, should mention not initialized", msg)
	}
}

// Scenario 9: Unknown tool
func TestUnknownTool(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"nonexistent","arguments":{}}}
`
	server, out := newTestServer(input)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	server.Run(ctx)
	responses := parseResponses(t, out)
	if len(responses) < 2 {
		t.Fatalf("expected at least 2 responses, got %d", len(responses))
	}

	resp := responses[1]
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatal("expected error response for unknown tool")
	}
	code, _ := errObj["code"].(float64)
	if code != -32602 {
		t.Errorf("error code = %v, want -32602", code)
	}
}

// Scenario 11: stdin EOF
func TestStdinEOF(t *testing.T) {
	input := "" // empty = immediate EOF
	server, _ := newTestServer(input)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := server.Run(ctx)
	if err != nil {
		t.Errorf("stdin EOF should return nil, got: %v", err)
	}
}

// Scenario 12: String vs int id
func TestStringID(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":"abc","method":"ping"}
`
	server, out := newTestServer(input)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	server.Run(ctx)
	responses := parseResponses(t, out)
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	resp := responses[0]
	if id, _ := resp["id"].(string); id != "abc" {
		t.Errorf("response id = %v, want %q", resp["id"], "abc")
	}
}

// Scenario 13: Null params
func TestNullParams(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
`
	server, out := newTestServer(input)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	server.Run(ctx)
	responses := parseResponses(t, out)
	if len(responses) < 2 {
		t.Fatalf("expected at least 2 responses, got %d", len(responses))
	}
	// Should not crash, should return tools
	resp := responses[1]
	if resp["error"] != nil {
		t.Error("tools/list with no cursor param should succeed")
	}
}

// Edge case: Empty line on stdin
func TestEmptyLine(t *testing.T) {
	input := "\n\n{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"ping\"}\n"
	server, out := newTestServer(input)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	server.Run(ctx)
	responses := parseResponses(t, out)
	if len(responses) != 1 {
		t.Fatalf("expected 1 response (empty lines skipped), got %d", len(responses))
	}
}

// Edge case: Batch request rejected
func TestBatchRequest(t *testing.T) {
	input := `[{"jsonrpc":"2.0","id":1,"method":"ping"}]
`
	server, out := newTestServer(input)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	server.Run(ctx)
	responses := parseResponses(t, out)
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	resp := responses[0]
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatal("expected error response for batch request")
	}
	code, _ := errObj["code"].(float64)
	if code != -32600 {
		t.Errorf("error code = %v, want -32600", code)
	}
}

// Version negotiation: unsupported version
func TestVersionNegotiation(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"1999-01-01","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
`
	server, out := newTestServer(input)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	server.Run(ctx)
	responses := parseResponses(t, out)
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	result, ok := responses[0]["result"].(map[string]any)
	if !ok {
		t.Fatal("expected result in initialize response")
	}
	if v, _ := result["protocolVersion"].(string); v != "2025-03-26" {
		t.Errorf("protocolVersion = %q, want %q (server preferred)", v, "2025-03-26")
	}
}

// Context cancellation
func TestContextCancel(t *testing.T) {
	// Use a pipe so we can control when EOF happens
	pr, pw := io.Pipe()
	out := &bytes.Buffer{}
	registry := NewToolRegistry()
	server := NewServer(ServerInfo{Name: "test", Version: "0.1.0"}, registry, NewResourceRegistry(), pr, out)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- server.Run(ctx)
	}()

	// Cancel the context
	cancel()

	select {
	case err := <-done:
		if err != nil && err != context.Canceled {
			t.Errorf("expected nil or context.Canceled, got: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}

	pw.Close()
}

// Coverage gap 1: notifications/initialized before initialize (should be ignored)
func TestInitializedBeforeInitialize(t *testing.T) {
	input := `{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":1,"method":"tools/list"}
`
	server, out := newTestServer(input)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	server.Run(ctx)
	responses := parseResponses(t, out)
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	// tools/list should be rejected because initialized was never actually set
	resp := responses[0]
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatal("expected error: server should not be initialized")
	}
	code, _ := errObj["code"].(float64)
	if code != -32600 {
		t.Errorf("error code = %v, want -32600", code)
	}
}

// Coverage gap 2: Re-initialize after full handshake
func TestReInitialize(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
{"jsonrpc":"2.0","id":3,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"2.0"}}}
{"jsonrpc":"2.0","id":4,"method":"tools/list"}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":5,"method":"tools/list"}
`
	server, out := newTestServer(input)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	server.Run(ctx)
	responses := parseResponses(t, out)
	if len(responses) < 5 {
		t.Fatalf("expected at least 5 responses, got %d", len(responses))
	}

	// Response 0: first initialize -> success
	if responses[0]["error"] != nil {
		t.Error("first initialize should succeed")
	}
	// Response 1: tools/list -> success (initialized)
	if responses[1]["error"] != nil {
		t.Error("tools/list after init should succeed")
	}
	// Response 2: second initialize -> success (re-init)
	if responses[2]["error"] != nil {
		t.Error("re-initialize should succeed")
	}
	// Response 3: tools/list -> rejected (re-init reset initialized flag)
	errObj, ok := responses[3]["error"].(map[string]any)
	if !ok {
		t.Fatal("tools/list after re-init should be rejected (not yet initialized)")
	}
	code, _ := errObj["code"].(float64)
	if code != -32600 {
		t.Errorf("error code = %v, want -32600", code)
	}
	// Response 4: tools/list -> success (after second notifications/initialized)
	if responses[4]["error"] != nil {
		t.Error("tools/list after second handshake should succeed")
	}
}

// Coverage gap 3: Tool handler panic recovery
func TestToolHandlerPanic(t *testing.T) {
	in := io.NopCloser(strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"panicker","arguments":{}}}
{"jsonrpc":"2.0","id":3,"method":"ping"}
`))
	out := &bytes.Buffer{}
	registry := NewToolRegistry()
	registry.Register(Tool{
		Name:        "panicker",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(_ context.Context, _ json.RawMessage) (*ToolResult, error) {
		panic("intentional panic")
	})
	server := NewServer(ServerInfo{Name: "test", Version: "0.1.0"}, registry, NewResourceRegistry(), in, out)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := server.Run(ctx)
	if err != nil {
		t.Fatalf("Run should not return error after panic recovery: %v", err)
	}

	responses := parseResponses(t, out)
	if len(responses) < 3 {
		t.Fatalf("expected at least 3 responses, got %d", len(responses))
	}

	// Response 1 (index 1): tool call with panic -> should return ToolResult with isError
	callResp := responses[1]
	result, ok := callResp["result"].(map[string]any)
	if !ok {
		t.Fatal("panic recovery should return result, not error")
	}
	isError, _ := result["isError"].(bool)
	if !isError {
		t.Error("result.isError should be true after panic")
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("result.content should not be empty")
	}
	item, _ := content[0].(map[string]any)
	text, _ := item["text"].(string)
	if !strings.Contains(text, "panic") {
		t.Errorf("error text should mention panic, got: %q", text)
	}

	// Response 2 (index 2): ping -> server still alive
	pingResp := responses[2]
	if pingResp["error"] != nil {
		t.Error("ping after panic should succeed (server should continue)")
	}
}

// Coverage gap 4+5: Version negotiation edge cases
func TestEmptyProtocolVersion(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
`
	server, out := newTestServer(input)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	server.Run(ctx)
	responses := parseResponses(t, out)
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}
	result, _ := responses[0]["result"].(map[string]any)
	if v, _ := result["protocolVersion"].(string); v != "2025-03-26" {
		t.Errorf("empty version should get preferred %q, got %q", "2025-03-26", v)
	}
}

func TestEchoSupportedVersion(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
`
	server, out := newTestServer(input)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	server.Run(ctx)
	responses := parseResponses(t, out)
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}
	result, _ := responses[0]["result"].(map[string]any)
	if v, _ := result["protocolVersion"].(string); v != "2024-11-05" {
		t.Errorf("supported version should be echoed, want %q, got %q", "2024-11-05", v)
	}
}

// Coverage gap 8: Missing method field
func TestMissingMethodField(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1}
`
	server, out := newTestServer(input)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	server.Run(ctx)
	responses := parseResponses(t, out)
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}

	errObj, ok := responses[0]["error"].(map[string]any)
	if !ok {
		t.Fatal("expected error for missing method")
	}
	code, _ := errObj["code"].(float64)
	if code != -32600 {
		t.Errorf("error code = %v, want -32600", code)
	}
	msg, _ := errObj["message"].(string)
	if !strings.Contains(msg, "missing method") {
		t.Errorf("error message should mention missing method, got: %q", msg)
	}
}

// Coverage gap 9: Tool handler error at server level
func TestToolHandlerErrorAtServerLevel(t *testing.T) {
	in := io.NopCloser(strings.NewReader(
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"failing","arguments":{}}}
`))
	out := &bytes.Buffer{}
	registry := NewToolRegistry()
	registry.Register(Tool{
		Name:        "failing",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(_ context.Context, _ json.RawMessage) (*ToolResult, error) {
		return nil, errors.New("handler failed")
	})
	server := NewServer(ServerInfo{Name: "test", Version: "0.1.0"}, registry, NewResourceRegistry(), in, out)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	server.Run(ctx)
	responses := parseResponses(t, out)
	if len(responses) < 2 {
		t.Fatalf("expected at least 2 responses, got %d", len(responses))
	}

	// The tool error should be returned as ToolResult with isError, NOT a JSON-RPC error
	callResp := responses[1]
	if callResp["error"] != nil {
		t.Fatal("tool handler error should NOT produce JSON-RPC error response")
	}
	result, ok := callResp["result"].(map[string]any)
	if !ok {
		t.Fatal("expected result in response")
	}
	isError, _ := result["isError"].(bool)
	if !isError {
		t.Error("result.isError should be true")
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatal("result.content should not be empty")
	}
	item, _ := content[0].(map[string]any)
	text, _ := item["text"].(string)
	if text != "handler failed" {
		t.Errorf("content text = %q, want %q", text, "handler failed")
	}
}

// Coverage gap 11: id:null treated as request
func TestNullID(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":null,"method":"ping"}
`
	server, out := newTestServer(input)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	server.Run(ctx)
	responses := parseResponses(t, out)
	if len(responses) != 1 {
		t.Fatalf("expected 1 response (null id is a request), got %d", len(responses))
	}

	resp := responses[0]
	if resp["error"] != nil {
		t.Error("ping with null id should succeed")
	}
	// id should be null in response
	if resp["id"] != nil {
		t.Errorf("response id should be null, got %v", resp["id"])
	}
}

// helper: create an initialized server with resources
func newResourceTestServer(input string) (*Server, *bytes.Buffer, *ResourceRegistry) {
	initPrefix := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
{"jsonrpc":"2.0","method":"notifications/initialized"}
`
	in := io.NopCloser(strings.NewReader(initPrefix + input))
	out := &bytes.Buffer{}
	registry := NewToolRegistry()
	resources := NewResourceRegistry()
	server := NewServer(ServerInfo{Name: "test", Version: "0.1.0"}, registry, resources, in, out)
	return server, out, resources
}

// Scenario 1: Resource list returns inbox resource
func TestResourcesList(t *testing.T) {
	server, out, resources := newResourceTestServer(
		`{"jsonrpc":"2.0","id":10,"method":"resources/list"}
`)
	resources.Register(Resource{URI: "p2p://inbox", Name: "P2P Inbox", MimeType: "application/json"}, func() (*ResourcesReadResult, error) {
		return &ResourcesReadResult{Contents: []ResourceContents{{URI: "p2p://inbox", Text: `{}`}}}, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server.Run(ctx)

	responses := parseResponses(t, out)
	// responses[0] = initialize, responses[1] = resources/list
	var listResp map[string]any
	for _, r := range responses {
		if id, ok := r["id"].(float64); ok && id == 10 {
			listResp = r
			break
		}
	}
	if listResp == nil {
		t.Fatal("no response for resources/list (id=10)")
	}
	result, _ := listResp["result"].(map[string]any)
	resList, _ := result["resources"].([]any)
	if len(resList) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resList))
	}
	res, _ := resList[0].(map[string]any)
	if res["uri"] != "p2p://inbox" {
		t.Errorf("URI = %v, want p2p://inbox", res["uri"])
	}
}

// Scenario 2: Resource read returns inbox contents
func TestResourcesRead(t *testing.T) {
	server, out, resources := newResourceTestServer(
		`{"jsonrpc":"2.0","id":10,"method":"resources/read","params":{"uri":"p2p://inbox"}}
`)
	resources.Register(Resource{URI: "p2p://inbox", Name: "P2P Inbox"}, func() (*ResourcesReadResult, error) {
		return &ResourcesReadResult{Contents: []ResourceContents{{URI: "p2p://inbox", MimeType: "application/json", Text: `{"messages":[],"count":0}`}}}, nil
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server.Run(ctx)

	responses := parseResponses(t, out)
	var readResp map[string]any
	for _, r := range responses {
		if id, ok := r["id"].(float64); ok && id == 10 {
			readResp = r
			break
		}
	}
	if readResp == nil {
		t.Fatal("no response for resources/read")
	}
	result, _ := readResp["result"].(map[string]any)
	contents, _ := result["contents"].([]any)
	if len(contents) != 1 {
		t.Fatalf("expected 1 content block, got %d", len(contents))
	}
}

// Scenario 6: Read unknown resource URI
func TestResourcesReadUnknown(t *testing.T) {
	server, out, _ := newResourceTestServer(
		`{"jsonrpc":"2.0","id":10,"method":"resources/read","params":{"uri":"p2p://unknown"}}
`)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server.Run(ctx)

	responses := parseResponses(t, out)
	var readResp map[string]any
	for _, r := range responses {
		if id, ok := r["id"].(float64); ok && id == 10 {
			readResp = r
			break
		}
	}
	if readResp == nil {
		t.Fatal("no response for resources/read")
	}
	errObj, _ := readResp["error"].(map[string]any)
	if errObj == nil {
		t.Fatal("expected error response for unknown resource")
	}
	code, _ := errObj["code"].(float64)
	if int(code) != ErrCodeResourceNotFound {
		t.Errorf("error code = %v, want %d", code, ErrCodeResourceNotFound)
	}
}

// Scenario 7: Subscribe to unknown resource
func TestResourcesSubscribeUnknown(t *testing.T) {
	server, out, _ := newResourceTestServer(
		`{"jsonrpc":"2.0","id":10,"method":"resources/subscribe","params":{"uri":"p2p://unknown"}}
`)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server.Run(ctx)

	responses := parseResponses(t, out)
	var resp map[string]any
	for _, r := range responses {
		if id, ok := r["id"].(float64); ok && id == 10 {
			resp = r
			break
		}
	}
	if resp == nil {
		t.Fatal("no response")
	}
	errObj, _ := resp["error"].(map[string]any)
	if errObj == nil {
		t.Fatal("expected error for unknown subscribe")
	}
	code, _ := errObj["code"].(float64)
	if int(code) != ErrCodeResourceNotFound {
		t.Errorf("error code = %v, want %d", code, ErrCodeResourceNotFound)
	}
}

// Scenario 8: Read with empty URI
func TestResourcesReadEmptyURI(t *testing.T) {
	server, out, _ := newResourceTestServer(
		`{"jsonrpc":"2.0","id":10,"method":"resources/read","params":{"uri":""}}
`)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server.Run(ctx)

	responses := parseResponses(t, out)
	var resp map[string]any
	for _, r := range responses {
		if id, ok := r["id"].(float64); ok && id == 10 {
			resp = r
			break
		}
	}
	if resp == nil {
		t.Fatal("no response")
	}
	errObj, _ := resp["error"].(map[string]any)
	if errObj == nil {
		t.Fatal("expected error for empty URI")
	}
	code, _ := errObj["code"].(float64)
	if int(code) != ErrCodeInvalidParams {
		t.Errorf("error code = %v, want %d", code, ErrCodeInvalidParams)
	}
}

// Scenario 13: Resource requests before initialization rejected
func TestResourcesBeforeInit(t *testing.T) {
	in := io.NopCloser(strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"resources/list"}
`))
	out := &bytes.Buffer{}
	server := NewServer(ServerInfo{Name: "test", Version: "0.1.0"}, NewToolRegistry(), NewResourceRegistry(), in, out)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server.Run(ctx)

	responses := parseResponses(t, out)
	if len(responses) != 1 {
		t.Fatalf("expected 1 response, got %d", len(responses))
	}
	errObj, _ := responses[0]["error"].(map[string]any)
	if errObj == nil {
		t.Fatal("expected error for pre-init request")
	}
}

// Test SendNotification
func TestSendNotification(t *testing.T) {
	in := io.NopCloser(strings.NewReader(""))
	out := &bytes.Buffer{}
	server := NewServer(ServerInfo{Name: "test", Version: "0.1.0"}, NewToolRegistry(), NewResourceRegistry(), in, out)

	err := server.SendNotification("notifications/resources/list_changed", nil)
	if err != nil {
		t.Fatalf("SendNotification failed: %v", err)
	}

	var notif map[string]any
	if err := json.NewDecoder(out).Decode(&notif); err != nil {
		t.Fatalf("decode notification: %v", err)
	}
	if notif["method"] != "notifications/resources/list_changed" {
		t.Errorf("method = %v, want notifications/resources/list_changed", notif["method"])
	}
	if _, hasID := notif["id"]; hasID {
		t.Error("notification should not have id field")
	}
}

// Test SendNotification with params
func TestSendNotificationWithParams(t *testing.T) {
	in := io.NopCloser(strings.NewReader(""))
	out := &bytes.Buffer{}
	server := NewServer(ServerInfo{Name: "test", Version: "0.1.0"}, NewToolRegistry(), NewResourceRegistry(), in, out)

	err := server.SendNotification("notifications/resources/updated", ResourcesUpdatedParams{URI: "p2p://inbox"})
	if err != nil {
		t.Fatalf("SendNotification failed: %v", err)
	}

	var notif map[string]any
	if err := json.NewDecoder(out).Decode(&notif); err != nil {
		t.Fatalf("decode: %v", err)
	}
	params, _ := notif["params"].(map[string]any)
	if params["uri"] != "p2p://inbox" {
		t.Errorf("params.uri = %v, want p2p://inbox", params["uri"])
	}
}

// Test IsSubscribed / subscribe / unsubscribe
func TestSubscribeUnsubscribe(t *testing.T) {
	server, out, resources := newResourceTestServer(
		`{"jsonrpc":"2.0","id":10,"method":"resources/subscribe","params":{"uri":"p2p://inbox"}}
{"jsonrpc":"2.0","id":11,"method":"resources/unsubscribe","params":{"uri":"p2p://inbox"}}
`)
	resources.Register(Resource{URI: "p2p://inbox", Name: "Inbox"}, func() (*ResourcesReadResult, error) { return nil, nil })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server.Run(ctx)

	// After subscribe, IsSubscribed should be true
	// After unsubscribe, IsSubscribed should be false
	// Both should have succeeded (no error in response)
	responses := parseResponses(t, out)
	for _, r := range responses {
		if id, ok := r["id"].(float64); ok && (id == 10 || id == 11) {
			if r["error"] != nil {
				t.Errorf("response %v had error: %v", id, r["error"])
			}
		}
	}

	// After both operations, should not be subscribed
	if server.IsSubscribed("p2p://inbox") {
		t.Error("expected not subscribed after unsubscribe")
	}
}

// Scenario: Read handler panic recovery
func TestResourcesReadPanicRecovery(t *testing.T) {
	server, out, resources := newResourceTestServer(
		`{"jsonrpc":"2.0","id":10,"method":"resources/read","params":{"uri":"p2p://panic"}}
`)
	resources.Register(Resource{URI: "p2p://panic", Name: "Panic"}, func() (*ResourcesReadResult, error) {
		panic("test panic in handler")
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server.Run(ctx)

	responses := parseResponses(t, out)
	var resp map[string]any
	for _, r := range responses {
		if id, ok := r["id"].(float64); ok && id == 10 {
			resp = r
			break
		}
	}
	if resp == nil {
		t.Fatal("no response")
	}
	errObj, _ := resp["error"].(map[string]any)
	if errObj == nil {
		t.Fatal("expected error response for panicking handler")
	}
	code, _ := errObj["code"].(float64)
	if int(code) != ErrCodeInternal {
		t.Errorf("error code = %v, want %d", code, ErrCodeInternal)
	}
}

// Scenario: Subscribe with empty URI
func TestResourcesSubscribeEmptyURI(t *testing.T) {
	server, out, _ := newResourceTestServer(
		`{"jsonrpc":"2.0","id":10,"method":"resources/subscribe","params":{"uri":""}}
`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server.Run(ctx)

	responses := parseResponses(t, out)
	var resp map[string]any
	for _, r := range responses {
		if id, ok := r["id"].(float64); ok && id == 10 {
			resp = r
			break
		}
	}
	if resp == nil {
		t.Fatal("no response")
	}
	errObj, _ := resp["error"].(map[string]any)
	if errObj == nil {
		t.Fatal("expected error for empty URI subscribe")
	}
	code, _ := errObj["code"].(float64)
	if int(code) != ErrCodeInvalidParams {
		t.Errorf("error code = %v, want %d", code, ErrCodeInvalidParams)
	}
}

// Scenario: Unsubscribe with empty URI
func TestResourcesUnsubscribeEmptyURI(t *testing.T) {
	server, out, _ := newResourceTestServer(
		`{"jsonrpc":"2.0","id":10,"method":"resources/unsubscribe","params":{"uri":""}}
`)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server.Run(ctx)

	responses := parseResponses(t, out)
	var resp map[string]any
	for _, r := range responses {
		if id, ok := r["id"].(float64); ok && id == 10 {
			resp = r
			break
		}
	}
	if resp == nil {
		t.Fatal("no response")
	}
	errObj, _ := resp["error"].(map[string]any)
	if errObj == nil {
		t.Fatal("expected error for empty URI unsubscribe")
	}
}

// Scenario: Empty resource list
func TestResourcesListEmpty(t *testing.T) {
	server, out, _ := newResourceTestServer(
		`{"jsonrpc":"2.0","id":10,"method":"resources/list"}
`)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server.Run(ctx)

	responses := parseResponses(t, out)
	var resp map[string]any
	for _, r := range responses {
		if id, ok := r["id"].(float64); ok && id == 10 {
			resp = r
			break
		}
	}
	if resp == nil {
		t.Fatal("no response")
	}
	result, _ := resp["result"].(map[string]any)
	resList, _ := result["resources"].([]any)
	if len(resList) != 0 {
		t.Errorf("expected 0 resources, got %d", len(resList))
	}
}

// Test Initialize includes resources capability
func TestInitializeIncludesResourcesCapability(t *testing.T) {
	input := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
`
	server, out := newTestServer(input)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	server.Run(ctx)

	responses := parseResponses(t, out)
	if len(responses) == 0 {
		t.Fatal("no response")
	}
	result, _ := responses[0]["result"].(map[string]any)
	caps, _ := result["capabilities"].(map[string]any)
	resCap, _ := caps["resources"].(map[string]any)
	if resCap == nil {
		t.Fatal("expected resources capability in init response")
	}
	if sub, ok := resCap["subscribe"].(bool); !ok || !sub {
		t.Error("expected resources.subscribe = true")
	}
	if lc, ok := resCap["listChanged"].(bool); !ok || !lc {
		t.Error("expected resources.listChanged = true")
	}
}
