package mcp

import "encoding/json"

// JSON-RPC error codes.
const (
	ErrCodeParseError     = -32700
	ErrCodeInvalidRequest = -32600
	ErrCodeMethodNotFound = -32601
	ErrCodeInvalidParams  = -32602
	ErrCodeInternal          = -32603
	ErrCodeResourceNotFound  = -32002
)

// JSONRPCRequest is an incoming JSON-RPC 2.0 request (has id).
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse is an outgoing JSON-RPC 2.0 response.
type JSONRPCResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      any           `json:"id"`
	Result  any           `json:"result,omitempty"`
	Error   *JSONRPCError `json:"error,omitempty"`
}

// JSONRPCError is a JSON-RPC 2.0 error object.
type JSONRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// JSONRPCNotification is a JSON-RPC 2.0 notification (no id).
type JSONRPCNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// InitializeParams is the params for the initialize request.
type InitializeParams struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      ClientInfo         `json:"clientInfo"`
}

// ClientCapabilities describes client capabilities.
type ClientCapabilities struct {
	Roots    *RootsCapability `json:"roots,omitempty"`
	Sampling *struct{}        `json:"sampling,omitempty"`
}

// RootsCapability describes the roots capability.
type RootsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ClientInfo identifies the client.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeResult is the result for the initialize response.
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
	Instructions    string             `json:"instructions,omitempty"`
}

// ServerCapabilities describes server capabilities.
type ServerCapabilities struct {
	Tools        *ToolsCapability     `json:"tools,omitempty"`
	Resources    *ResourcesCapability `json:"resources,omitempty"`
	Experimental map[string]any       `json:"experimental,omitempty"`
}

// ChannelNotificationParams is the params for notifications/claude/channel.
type ChannelNotificationParams struct {
	Content string            `json:"content"`
	Meta    map[string]string `json:"meta,omitempty"`
}

// ResourcesCapability describes the resources capability.
type ResourcesCapability struct {
	Subscribe   bool `json:"subscribe,omitempty"`
	ListChanged bool `json:"listChanged,omitempty"`
}

// ToolsCapability describes the tools capability.
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ServerInfo identifies the server.
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Tool is an MCP tool definition.
type Tool struct {
	Name        string           `json:"name"`
	Description string           `json:"description,omitempty"`
	InputSchema json.RawMessage  `json:"inputSchema"`
	Annotations *ToolAnnotations `json:"annotations,omitempty"`
}

// ToolAnnotations provides hints about tool behavior.
type ToolAnnotations struct {
	Title           string `json:"title,omitempty"`
	ReadOnlyHint    *bool  `json:"readOnlyHint,omitempty"`
	DestructiveHint *bool  `json:"destructiveHint,omitempty"`
	IdempotentHint  *bool  `json:"idempotentHint,omitempty"`
	OpenWorldHint   *bool  `json:"openWorldHint,omitempty"`
}

// ToolCallParams is the params for tools/call.
type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// ToolResult is the result for tools/call.
type ToolResult struct {
	Content []ContentItem  `json:"content"`
	IsError bool           `json:"isError,omitempty"`
	Meta    map[string]any `json:"_meta,omitempty"`
}

// ContentItem is a content block in a tool result.
type ContentItem struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ToolsListParams is the params for tools/list.
type ToolsListParams struct {
	Cursor string `json:"cursor,omitempty"`
}

// ToolsListResult is the result for tools/list.
type ToolsListResult struct {
	Tools      []Tool `json:"tools"`
	NextCursor string `json:"nextCursor,omitempty"`
}

// Resource is an MCP resource definition.
type Resource struct {
	URI         string               `json:"uri"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	MimeType    string               `json:"mimeType,omitempty"`
	Annotations *ResourceAnnotations `json:"annotations,omitempty"`
}

// ResourceAnnotations provides hints about the resource.
type ResourceAnnotations struct {
	Audience     []string `json:"audience,omitempty"`
	Priority     *float64 `json:"priority,omitempty"`
	LastModified string   `json:"lastModified,omitempty"`
}

// ResourceContents is the content of a resource.
type ResourceContents struct {
	URI      string `json:"uri"`
	MimeType string `json:"mimeType,omitempty"`
	Text     string `json:"text,omitempty"`
}

// ResourcesListParams is the params for resources/list.
type ResourcesListParams struct {
	Cursor string `json:"cursor,omitempty"`
}

// ResourcesListResult is the result for resources/list.
type ResourcesListResult struct {
	Resources  []Resource `json:"resources"`
	NextCursor string     `json:"nextCursor,omitempty"`
}

// ResourcesReadParams is the params for resources/read.
type ResourcesReadParams struct {
	URI string `json:"uri"`
}

// ResourcesReadResult is the result for resources/read.
type ResourcesReadResult struct {
	Contents []ResourceContents `json:"contents"`
}

// ResourcesSubscribeParams is the params for resources/subscribe and resources/unsubscribe.
type ResourcesSubscribeParams struct {
	URI string `json:"uri"`
}

// ResourcesUpdatedParams is the params for notifications/resources/updated.
type ResourcesUpdatedParams struct {
	URI string `json:"uri"`
}

// BoolPtr returns a pointer to a bool value.
func BoolPtr(b bool) *bool {
	return &b
}

// Float64Ptr returns a pointer to a float64 value.
func Float64Ptr(f float64) *float64 {
	return &f
}
