# claude-p2p

P2P MCP server for direct communication between Claude Code instances.

## Usage

```json
{
  "mcpServers": {
    "claude-p2p": {
      "command": "npx",
      "args": ["-y", "claude-p2p"]
    }
  }
}
```

Or register globally:

```bash
npx claude-p2p  # downloads binary automatically
claude mcp add claude-p2p npx -- -y claude-p2p -s user
```

See [GitHub](https://github.com/imtemp-dev/claude-p2p) for full documentation.
