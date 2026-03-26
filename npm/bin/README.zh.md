# claude-p2p

Claude Code 实例间 P2P 直接通信 MCP 服务器。

[English](README.md) | [한국어](README.ko.md) | [日本語](README.ja.md)

团队成员各自运行 Claude Code 会话时，无需任何外部基础设施即可直接连接，实时交换消息和共享工作上下文。

## 特性

- **零基础设施** — 无中央服务器、代理或外部服务
- **局域网自动发现** — mDNS 自动检测同一网络上的节点
- **互联网连接** — 通过共享主题代码连接远程节点（基于 DHT）
- **实时消息** — 1:1 直接消息 + 主题广播
- **工作上下文共享** — 共享当前任务摘要，自动检测 git 仓库/分支
- **端到端加密** — Noise 协议（libp2p 内置）
- **NAT 穿透** — 打洞 + Circuit Relay v2 自动回退
- **单一二进制文件** — Go 构建，跨平台（macOS/Linux/Windows）

## 架构

```
开发者 A（上海）                        开发者 B（美国）
┌──────────────┐                      ┌──────────────┐
│ Claude Code  │                      │ Claude Code  │
│     ↕ stdio  │                      │     ↕ stdio  │
│ claude-p2p   │ ←── libp2p ────────→ │ claude-p2p   │
│ （单一二进制） │    Noise 加密        │ （单一二进制） │
└──────────────┘    NAT 打洞           └──────────────┘
```

## 快速开始

### 1. 构建

```bash
git clone https://github.com/jlim/claude-p2p.git
cd claude-p2p
go build -o claude-p2p .
```

### 2. 注册到 Claude Code

全局注册（推荐 — 在任何目录下都可使用）：

```bash
claude mcp add claude-p2p /path/to/claude-p2p -s user
```

或通过项目级 `.mcp.json`：

```json
{
  "mcpServers": {
    "claude-p2p": {
      "command": "/path/to/claude-p2p"
    }
  }
}
```

### 3. 运行

使用 development channels 标志启动 Claude Code：

```bash
claude --dangerously-load-development-channels server:claude-p2p
```

## 局域网测试指南

可以在同一台机器上使用两个 Claude Code 会话测试 P2P 通信。

### 准备

```bash
# 构建
go build -o claude-p2p .

# 全局注册
claude mcp add claude-p2p $(pwd)/claude-p2p -s user
```

### 方法 1：两个 Claude Code 会话

**终端 A** — 在任意项目目录：

```bash
claude --dangerously-load-development-channels server:claude-p2p
```

**终端 B** — 在不同目录：

```bash
claude --dangerously-load-development-channels server:claude-p2p
```

两个会话都以 claude-p2p 作为 MCP 服务器启动。在同一局域网中，**通过 mDNS 自动发现**。

在任一会话中告诉 Claude：

```
显示已连接的节点
```

Claude 会调用 `list_peers` 并显示另一个会话。

### 方法 2：Go 测试

包含集成测试：

```bash
# 所有测试（42个，含竞态检测）
go test ./... -race -v

# 仅集成测试
go test ./p2p/ -run TestIntegration -v
```

### 使用示例

两个 Claude Code 会话连接后，使用自然语言：

```
> "显示已连接的节点"
> "给另一个会话发消息：请审查 PR #42"
> "向 my-team 主题广播：已推送到 main"
> "查看新消息"
> "设置工作摘要：正在重构 auth 模块"
> "加入 my-team 主题"
```

Claude 会自动调用相应的 MCP 工具。

## MCP 工具

| 工具 | 说明 |
|------|------|
| `list_peers` | 列出已连接节点（scope: local/topic/all） |
| `send_message` | 发送直接消息或主题广播 |
| `get_messages` | 获取收到的消息（pop 或 peek 模式） |
| `set_summary` | 设置对节点可见的工作摘要 |
| `join_topic` | 加入团队/项目主题 |

## 连接方式

### 局域网（自动）

同一网络上的节点通过 mDNS 自动发现。无需配置。

### 互联网（主题代码）

```bash
# 通过环境变量设置主题
CLAUDE_P2P_TOPIC=my-team-abc claude --dangerously-load-development-channels server:claude-p2p

# 或在 Claude Code 中调用工具
join_topic(topic="my-team-abc")
```

使用相同主题代码的所有节点通过 DHT 汇合连接。

## 跨平台构建

```bash
# 安装 goreleaser (https://goreleaser.com/install/)
brew install goreleaser

# 构建（macOS/Linux/Windows × amd64/arm64）
goreleaser build --snapshot --clean
```

## 技术栈

| 层 | 技术 |
|----|------|
| 语言 | Go |
| P2P | [go-libp2p](https://github.com/libp2p/go-libp2p) |
| 节点发现 | mDNS（局域网）+ Kademlia DHT（互联网） |
| 消息 | libp2p streams（1:1）+ GossipSub（广播） |
| 加密 | Noise protocol |
| NAT 穿透 | 打洞 + Circuit Relay v2 |
| MCP | JSON-RPC 2.0 over stdio |
| 构建 | goreleaser |

## 项目结构

```
claude-p2p/
├── main.go                 # 入口点
├── mcp/
│   ├── server.go           # MCP JSON-RPC 服务器
│   ├── types.go            # JSON-RPC + MCP 类型定义
│   └── tools.go            # 工具注册表
├── node/
│   └── node.go             # MCP 服务器 + P2P 主机编排器
├── p2p/
│   ├── host.go             # libp2p 主机（NAT、连接管理）
│   ├── discovery.go        # mDNS + DHT 节点发现
│   ├── topic.go            # 主题管理 + GossipSub
│   ├── messaging.go        # 直接消息（libp2p streams）
│   ├── inbox.go            # 消息收件箱（有界 FIFO 队列）
│   ├── metadata.go         # 工作上下文共享
│   ├── peers.go            # 节点追踪器
│   └── types.go            # P2P 类型
└── .goreleaser.yaml        # 跨平台构建配置
```

## 许可证

MIT
