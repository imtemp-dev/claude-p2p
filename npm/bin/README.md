# claude-p2p

A peer-to-peer MCP server for direct communication between Claude Code instances.

[한국어](README.ko.md) | [中文](README.zh.md) | [日本語](README.ja.md)

Team members running separate Claude Code sessions can discover each other, exchange messages, and share work context in real-time — without any external infrastructure.

## Features

- **Zero infrastructure** — No central server, broker, or external service
- **LAN auto-discovery** — mDNS finds peers on the same network automatically
- **Internet connectivity** — Connect remote peers via shared topic codes (DHT-based)
- **Real-time messaging** — Direct 1:1 messages + topic broadcast
- **Work context sharing** — Share current task summary, git repo/branch auto-detected
- **End-to-end encryption** — Noise protocol (built into libp2p)
- **NAT traversal** — Hole punching + Circuit Relay v2 fallback
- **Single binary** — Built with Go, cross-platform (macOS/Linux/Windows)

## Architecture

```
Developer A (Seoul)                    Developer B (US)
┌──────────────┐                      ┌──────────────┐
│ Claude Code  │                      │ Claude Code  │
│     ↕ stdio  │                      │     ↕ stdio  │
│ claude-p2p   │ ←── libp2p ────────→ │ claude-p2p   │
│  (single bin)│    Noise encryption  │  (single bin)│
└──────────────┘    NAT hole-punch    └──────────────┘
```

## Quick Start

### 1. Build

```bash
git clone https://github.com/jlim/claude-p2p.git
cd claude-p2p
go build -o claude-p2p .
```

### 2. Register with Claude Code

Global registration (recommended — works from any directory):

```bash
claude mcp add claude-p2p /path/to/claude-p2p -s user
```

Or per-project via `.mcp.json`:

```json
{
  "mcpServers": {
    "claude-p2p": {
      "command": "/path/to/claude-p2p"
    }
  }
}
```

### 3. Run

Start Claude Code — `claude-p2p` launches automatically as an MCP server.

## Local Network Testing Guide

You can test P2P communication between two Claude Code sessions on the same machine.

### Setup

```bash
# Build
go build -o claude-p2p .

# Register globally
claude mcp add claude-p2p $(pwd)/claude-p2p -s user
```

### Option 1: Two Claude Code Sessions

**Terminal A** — in any project directory:

```bash
claude
```

**Terminal B** — in a different directory:

```bash
claude
```

Both sessions start with claude-p2p as an MCP server. On the same LAN, **peers are discovered automatically via mDNS**.

In either session, ask Claude:

```
Show me connected peers
```

Claude calls `list_peers` and shows the other session.

### Option 2: Manual MCP Testing

Test the MCP protocol directly without Claude Code:

**Terminal A:**

```bash
./claude-p2p
```

You'll see logs on stderr:

```
[claude-p2p] libp2p peer ID: 12D3KooW...
[claude-p2p] listening on: /ip4/192.168.0.10/tcp/54321/p2p/12D3KooW...
[claude-p2p] mDNS discovery started
[claude-p2p] DHT started
```

**Terminal B:**

```bash
./claude-p2p
```

After a few seconds, both sides show:

```
[claude-p2p] mDNS: discovered peer 12D3KooW...
```

Send MCP commands via stdin:

```json
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
```

```json
{"jsonrpc":"2.0","method":"notifications/initialized"}
```

```json
{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"list_peers","arguments":{}}}
```

### Option 3: Go Tests

Integration tests are included:

```bash
# All tests (42, with race detector)
go test ./... -race -v

# Integration tests only
go test ./p2p/ -run TestIntegration -v

# P2P messaging test
go test ./p2p/ -run TestSendDirectMessage -v
```

### Usage Examples

Once two Claude Code sessions are connected, use natural language:

```
> "Show me connected peers"
> "Send a message to the other session: please review PR #42"
> "Broadcast to my-team topic: pushed to main"
> "Check for new messages"
> "Set my summary to: refactoring auth module"
> "Join topic my-team"
```

Claude automatically calls the appropriate MCP tools.

## MCP Tools

| Tool | Description |
|------|-------------|
| `list_peers` | List connected peers (scope: local/topic/all) |
| `send_message` | Send direct message or topic broadcast |
| `get_messages` | Retrieve received messages (pop or peek mode) |
| `set_summary` | Set work summary visible to peers |
| `join_topic` | Join a team/project topic |

## Connectivity

### LAN (Automatic)

Peers on the same network are discovered via mDNS. No configuration needed.

### Internet (Topic Code)

```bash
# Set topic via environment variable
CLAUDE_P2P_TOPIC=my-team-abc claude

# Or use the tool from within Claude Code
join_topic(topic="my-team-abc")
```

All peers using the same topic code connect via DHT rendezvous.

## Cross-Platform Builds

```bash
# Install goreleaser (https://goreleaser.com/install/)
# macOS:
brew install goreleaser

# Build for all platforms (macOS/Linux/Windows x amd64/arm64)
goreleaser build --snapshot --clean

# Output: 6 binaries in dist/
```

## Tech Stack

| Layer | Technology |
|-------|------------|
| Language | Go |
| P2P | [go-libp2p](https://github.com/libp2p/go-libp2p) |
| Discovery | mDNS (LAN) + Kademlia DHT (Internet) |
| Messaging | libp2p streams (1:1) + GossipSub (broadcast) |
| Encryption | Noise protocol |
| NAT Traversal | Hole punching + Circuit Relay v2 |
| MCP | JSON-RPC 2.0 over stdio |
| Build | goreleaser |

## Project Structure

```
claude-p2p/
├── main.go                 # Entry point
├── mcp/
│   ├── server.go           # MCP JSON-RPC server
│   ├── types.go            # JSON-RPC + MCP type definitions
│   └── tools.go            # Tool registry
├── node/
│   └── node.go             # MCP server + P2P host orchestrator
├── p2p/
│   ├── host.go             # libp2p host (NAT, connection management)
│   ├── discovery.go        # mDNS + DHT peer discovery
│   ├── topic.go            # Topic management + GossipSub
│   ├── messaging.go        # Direct messaging (libp2p streams)
│   ├── inbox.go            # Message inbox (bounded FIFO queue)
│   ├── metadata.go         # Work context sharing
│   ├── peers.go            # Peer tracker
│   └── types.go            # P2P types
└── .goreleaser.yaml        # Cross-platform build config
```

## License

MIT
