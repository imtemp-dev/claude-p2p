package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestToolRegistryRegisterAndList(t *testing.T) {
	r := NewToolRegistry()

	r.Register(Tool{
		Name:        "tool_a",
		Description: "Tool A",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(_ context.Context, _ json.RawMessage) (*ToolResult, error) {
		return &ToolResult{Content: []ContentItem{{Type: "text", Text: "a"}}}, nil
	})

	r.Register(Tool{
		Name:        "tool_b",
		Description: "Tool B",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(_ context.Context, _ json.RawMessage) (*ToolResult, error) {
		return &ToolResult{Content: []ContentItem{{Type: "text", Text: "b"}}}, nil
	})

	tools := r.List()
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	names := map[string]bool{}
	for _, tool := range tools {
		names[tool.Name] = true
	}
	if !names["tool_a"] || !names["tool_b"] {
		t.Errorf("expected tool_a and tool_b, got %v", names)
	}
}

func TestToolRegistryCallSuccess(t *testing.T) {
	r := NewToolRegistry()
	r.Register(Tool{
		Name:        "echo",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(_ context.Context, args json.RawMessage) (*ToolResult, error) {
		return &ToolResult{
			Content: []ContentItem{{Type: "text", Text: string(args)}},
		}, nil
	})

	result, err := r.Call(context.Background(), "echo", json.RawMessage(`{"msg":"hello"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Content) != 1 {
		t.Fatal("expected 1 content item")
	}
	if result.Content[0].Text != `{"msg":"hello"}` {
		t.Errorf("got text %q", result.Content[0].Text)
	}
}

func TestToolRegistryCallNotFound(t *testing.T) {
	r := NewToolRegistry()
	result, err := r.Call(context.Background(), "nonexistent", nil)
	if result != nil {
		t.Error("expected nil result for not-found tool")
	}
	if !errors.Is(err, ErrToolNotFound) {
		t.Errorf("expected ErrToolNotFound, got: %v", err)
	}
}

func TestToolRegistryCallHandlerError(t *testing.T) {
	r := NewToolRegistry()
	r.Register(Tool{
		Name:        "failing",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(_ context.Context, _ json.RawMessage) (*ToolResult, error) {
		return nil, errors.New("handler failed")
	})

	result, err := r.Call(context.Background(), "failing", nil)
	if result != nil {
		t.Error("expected nil result on handler error")
	}
	if err == nil || err.Error() != "handler failed" {
		t.Errorf("expected 'handler failed' error, got: %v", err)
	}
}

func TestToolRegistryUpdateDescription(t *testing.T) {
	r := NewToolRegistry()
	r.Register(Tool{
		Name:        "test_tool",
		Description: "original",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	}, func(_ context.Context, _ json.RawMessage) (*ToolResult, error) {
		return nil, nil
	})

	r.UpdateDescription("test_tool", "updated description")
	tools := r.List()
	if tools[0].Description != "updated description" {
		t.Errorf("description = %q, want %q", tools[0].Description, "updated description")
	}
}

func TestToolRegistryUpdateDescriptionNotFound(t *testing.T) {
	r := NewToolRegistry()
	// Should not panic
	r.UpdateDescription("nonexistent", "test")
}

func TestBoolPtr(t *testing.T) {
	truePtr := BoolPtr(true)
	falsePtr := BoolPtr(false)
	if *truePtr != true {
		t.Error("BoolPtr(true) should return pointer to true")
	}
	if *falsePtr != false {
		t.Error("BoolPtr(false) should return pointer to false")
	}
}
