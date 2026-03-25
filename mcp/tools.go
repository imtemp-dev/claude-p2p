package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
)

// ErrToolNotFound is returned when a tool name is not in the registry.
var ErrToolNotFound = errors.New("tool not found")

// ToolHandler is a function that handles a tool call.
type ToolHandler func(ctx context.Context, args json.RawMessage) (*ToolResult, error)

// ToolRegistry manages tool definitions and their handlers.
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]registeredTool
}

type registeredTool struct {
	definition Tool
	handler    ToolHandler
}

// NewToolRegistry creates an empty tool registry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{tools: make(map[string]registeredTool)}
}

// Register adds a tool to the registry.
func (r *ToolRegistry) Register(tool Tool, handler ToolHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[tool.Name] = registeredTool{definition: tool, handler: handler}
}

// List returns all registered tool definitions, sorted by name for deterministic ordering.
func (r *ToolRegistry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	tools := make([]Tool, 0, len(r.tools))
	for _, rt := range r.tools {
		tools = append(tools, rt.definition)
	}
	sort.Slice(tools, func(i, j int) bool {
		return tools[i].Name < tools[j].Name
	})
	return tools
}

// UpdateDescription updates the description of a registered tool.
// If the tool is not found, this is a no-op.
func (r *ToolRegistry) UpdateDescription(name string, description string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rt, ok := r.tools[name]
	if !ok {
		return
	}
	rt.definition.Description = description
	r.tools[name] = rt
}

// Call looks up a tool by name and calls its handler.
func (r *ToolRegistry) Call(ctx context.Context, name string, args json.RawMessage) (*ToolResult, error) {
	r.mu.RLock()
	rt, ok := r.tools[name]
	r.mu.RUnlock()
	if !ok {
		return nil, ErrToolNotFound
	}
	return rt.handler(ctx, args)
}
