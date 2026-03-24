package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
)

// Supported MCP protocol versions.
var supportedVersions = map[string]bool{
	"2024-11-05": true,
	"2025-03-26": true,
}

const preferredVersion = "2025-03-26"

// Server is an MCP JSON-RPC 2.0 server over stdio.
type Server struct {
	input            io.ReadCloser
	reader           *bufio.Scanner
	writer           *json.Encoder
	mu               sync.Mutex
	closeOnce        sync.Once
	tools            *ToolRegistry
	initResponseSent bool
	initialized      bool
	serverInfo       ServerInfo
	logger           *log.Logger
}

// NewServer creates a new MCP server with the given input/output streams.
func NewServer(info ServerInfo, registry *ToolRegistry, input io.ReadCloser, output io.Writer) *Server {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	return &Server{
		input:      input,
		reader:     scanner,
		writer:     json.NewEncoder(output),
		tools:      registry,
		serverInfo: info,
		logger:     log.New(os.Stderr, "[claude-p2p] ", log.LstdFlags),
	}
}

// Run starts the main message loop. It blocks until ctx is cancelled or stdin reaches EOF.
func (s *Server) Run(ctx context.Context) error {
	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()
	go func() {
		<-runCtx.Done()
		s.Close()
	}()

	for s.reader.Scan() {
		line := s.reader.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		if err := s.handleMessage(ctx, line); err != nil {
			return err
		}
	}

	if err := s.reader.Err(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		s.logger.Printf("scanner error: %v", err)
		return err
	}
	return nil
}

// Close closes the input stream to unblock the scanner. Idempotent.
func (s *Server) Close() error {
	var err error
	s.closeOnce.Do(func() {
		err = s.input.Close()
	})
	return err
}

func (s *Server) handleMessage(ctx context.Context, data []byte) error {
	// Batch request check
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		return s.sendError(nil, ErrCodeInvalidRequest, "Batch requests not supported")
	}

	// Parse into generic map to detect id/method presence
	var msg map[string]any
	if err := json.Unmarshal(data, &msg); err != nil {
		return s.sendError(nil, ErrCodeParseError, "Parse error: "+err.Error())
	}

	method, _ := msg["method"].(string)
	if method == "" {
		id, _ := msg["id"]
		return s.sendError(id, ErrCodeInvalidRequest, "Invalid Request: missing method")
	}

	// Determine if request (has id) or notification (no id field)
	id, hasID := msg["id"]

	if !hasID {
		// Notification
		return s.handleNotification(method)
	}

	// Request — check initialization state
	if !s.initialized && method != "initialize" && method != "ping" {
		return s.sendError(id, ErrCodeInvalidRequest, "Server not initialized")
	}

	var params json.RawMessage
	if raw, ok := msg["params"]; ok && raw != nil {
		params, _ = json.Marshal(raw)
	}

	switch method {
	case "initialize":
		return s.handleInitialize(id, params)
	case "tools/list":
		return s.handleToolsList(id, params)
	case "tools/call":
		return s.handleToolsCall(ctx, id, params)
	case "ping":
		return s.handlePing(id)
	default:
		return s.sendError(id, ErrCodeMethodNotFound, "Method not found: "+method)
	}
}

func (s *Server) handleNotification(method string) error {
	switch method {
	case "notifications/initialized":
		if s.initResponseSent {
			s.initialized = true
		}
	}
	// Unknown notifications are silently ignored
	return nil
}

func (s *Server) handleInitialize(id any, params json.RawMessage) error {
	// Re-initialize: reset handshake state
	if s.initialized {
		s.initialized = false
		s.initResponseSent = false
	}

	var p InitializeParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return s.sendError(id, ErrCodeInvalidParams, "Invalid params: "+err.Error())
		}
	}

	// Version negotiation
	version := p.ProtocolVersion
	if !supportedVersions[version] {
		version = preferredVersion
	}

	s.initResponseSent = true

	return s.sendResponse(&JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result: InitializeResult{
			ProtocolVersion: version,
			Capabilities: ServerCapabilities{
				Tools: &ToolsCapability{ListChanged: true},
			},
			ServerInfo:   s.serverInfo,
			Instructions: "P2P communication between Claude Code instances",
		},
	})
}

func (s *Server) handleToolsList(id any, _ json.RawMessage) error {
	return s.sendResponse(&JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  ToolsListResult{Tools: s.tools.List()},
	})
}

func (s *Server) handleToolsCall(ctx context.Context, id any, params json.RawMessage) error {
	var p ToolCallParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &p); err != nil {
			return s.sendError(id, ErrCodeInvalidParams, "Invalid params: "+err.Error())
		}
	}

	// Panic recovery
	defer func() {
		if r := recover(); r != nil {
			s.sendResponse(&JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      id,
				Result: ToolResult{
					Content: []ContentItem{{Type: "text", Text: fmt.Sprintf("tool panic: %v", r)}},
					IsError: true,
				},
			})
		}
	}()

	result, err := s.tools.Call(ctx, p.Name, p.Arguments)
	if err != nil {
		if errors.Is(err, ErrToolNotFound) {
			return s.sendError(id, ErrCodeInvalidParams, "Unknown tool: "+p.Name)
		}
		return s.sendResponse(&JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      id,
			Result: ToolResult{
				Content: []ContentItem{{Type: "text", Text: err.Error()}},
				IsError: true,
			},
		})
	}

	return s.sendResponse(&JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	})
}

func (s *Server) handlePing(id any) error {
	return s.sendResponse(&JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Result:  struct{}{},
	})
}

func (s *Server) sendResponse(resp *JSONRPCResponse) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writer.Encode(resp)
}

func (s *Server) sendError(id any, code int, message string) error {
	return s.sendResponse(&JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &JSONRPCError{Code: code, Message: message},
	})
}
