# claude-p2p

Claude Code インスタンス間の P2P 直接通信 MCP サーバー。

[English](README.md) | [한국어](README.ko.md) | [中文](README.zh.md)

チームメンバーがそれぞれ Claude Code セッションを開いて作業する際、外部インフラなしでセッション同士を直接接続し、リアルタイムでメッセージ交換やワークコンテキスト共有ができます。

## 特徴

- **ゼロインフラ** — 中央サーバー、ブローカー、外部サービス不要
- **LAN 自動検出** — mDNS で同一ネットワーク上のピアを自動検出
- **インターネット接続** — 共有トピックコードでリモートピアに接続（DHT ベース）
- **リアルタイムメッセージング** — 1:1 ダイレクトメッセージ + トピックブロードキャスト
- **ワークコンテキスト共有** — 現在の作業概要、git リポ/ブランチ自動検出
- **エンドツーエンド暗号化** — Noise プロトコル（libp2p 内蔵）
- **NAT トラバーサル** — ホールパンチング + Circuit Relay v2 自動フォールバック
- **シングルバイナリ** — Go でビルド、クロスプラットフォーム（macOS/Linux/Windows）

## アーキテクチャ

```
開発者 A（東京）                        開発者 B（米国）
┌──────────────┐                      ┌──────────────┐
│ Claude Code  │                      │ Claude Code  │
│     ↕ stdio  │                      │     ↕ stdio  │
│ claude-p2p   │ ←── libp2p ────────→ │ claude-p2p   │
│（シングルバイナリ）│  Noise 暗号化     │（シングルバイナリ）│
└──────────────┘    NAT ホールパンチ    └──────────────┘
```

## インストール

### 方法 A：go install（Go が必要）

```bash
go install github.com/imtemp-dev/claude-p2p@latest
claude mcp add claude-p2p $(go env GOPATH)/bin/claude-p2p -s user
```

### 方法 B：バイナリをダウンロード

[GitHub Releases](https://github.com/imtemp-dev/claude-p2p/releases) からダウンロード後：

```bash
chmod +x claude-p2p
claude mcp add claude-p2p /path/to/claude-p2p -s user
```

### 方法 C：ソースからビルド

```bash
git clone https://github.com/imtemp-dev/claude-p2p.git
cd claude-p2p
go build -o claude-p2p .
claude mcp add claude-p2p $(pwd)/claude-p2p -s user
```

### アップデート

```bash
cd /path/to/claude-p2p
git pull
go build -o claude-p2p .
```

次の Claude Code セッションから更新されたバイナリが使用されます。再登録は不要です。

### 実行

Claude Code を起動すると、`claude-p2p` が MCP サーバーとして自動的に起動します。

## ローカルネットワークテストガイド

同じマシン上で 2 つの Claude Code セッションによる P2P 通信をテストできます。

### 準備

```bash
# ビルド
go build -o claude-p2p .

# グローバル登録
claude mcp add claude-p2p $(pwd)/claude-p2p -s user
```

### 方法 1：2 つの Claude Code セッション

**ターミナル A** — 任意のプロジェクトディレクトリで：

```bash
claude
```

**ターミナル B** — 別のディレクトリで：

```bash
claude
```

両セッションとも claude-p2p が MCP サーバーとして起動します。同一 LAN 内であれば **mDNS で自動検出**されます。

いずれかのセッションで Claude に：

```
接続中のピアを表示して
```

Claude が `list_peers` を呼び出し、もう一方のセッションを表示します。

### 方法 2：Go テスト

統合テストが含まれています：

```bash
# 全テスト（42個、レースディテクター付き）
go test ./... -race -v

# 統合テストのみ
go test ./p2p/ -run TestIntegration -v
```

### 使用例

2 つの Claude Code セッションが接続された後、自然言語で：

```
> "接続中のピアを表示して"
> "別のセッションにメッセージを送って：PR #42 のレビューお願いします"
> "my-team トピックにブロードキャスト：main にプッシュしました"
> "新しいメッセージを確認して"
> "作業概要を設定：auth モジュールをリファクタリング中"
> "my-team トピックに参加して"
```

Claude が自動的に適切な MCP ツールを呼び出します。

## MCP ツール

| ツール | 説明 |
|--------|------|
| `list_peers` | 接続中のピア一覧（scope: local/topic/all） |
| `send_message` | ダイレクトメッセージまたはトピックブロードキャスト |
| `get_messages` | 受信メッセージの取得（pop または peek モード） |
| `set_summary` | ピアに公開される作業概要を設定 |
| `join_topic` | チーム/プロジェクトトピックに参加 |

## 接続方式

### LAN（自動）

同一ネットワーク上のピアは mDNS で自動検出。設定不要。

### インターネット（トピックコード）

```bash
# 環境変数でトピックを設定
CLAUDE_P2P_TOPIC=my-team-abc claude

# または Claude Code 内でツールを呼び出し
join_topic(topic="my-team-abc")
```

同じトピックコードを使用するすべてのピアが DHT ランデブーで接続されます。

## クロスプラットフォームビルド

```bash
# goreleaser のインストール (https://goreleaser.com/install/)
brew install goreleaser

# ビルド（macOS/Linux/Windows × amd64/arm64）
goreleaser build --snapshot --clean
```

## 技術スタック

| レイヤー | 技術 |
|----------|------|
| 言語 | Go |
| P2P | [go-libp2p](https://github.com/libp2p/go-libp2p) |
| ピア検出 | mDNS（LAN）+ Kademlia DHT（インターネット） |
| メッセージング | libp2p streams（1:1）+ GossipSub（ブロードキャスト） |
| 暗号化 | Noise protocol |
| NAT トラバーサル | ホールパンチング + Circuit Relay v2 |
| MCP | JSON-RPC 2.0 over stdio |
| ビルド | goreleaser |

## プロジェクト構造

```
claude-p2p/
├── main.go                 # エントリーポイント
├── mcp/
│   ├── server.go           # MCP JSON-RPC サーバー
│   ├── types.go            # JSON-RPC + MCP 型定義
│   └── tools.go            # ツールレジストリ
├── node/
│   └── node.go             # MCP サーバー + P2P ホストオーケストレーター
├── p2p/
│   ├── host.go             # libp2p ホスト（NAT、接続管理）
│   ├── discovery.go        # mDNS + DHT ピア検出
│   ├── topic.go            # トピック管理 + GossipSub
│   ├── messaging.go        # ダイレクトメッセージング（libp2p streams）
│   ├── inbox.go            # メッセージインボックス（有界 FIFO キュー）
│   ├── metadata.go         # ワークコンテキスト共有
│   ├── peers.go            # ピアトラッカー
│   └── types.go            # P2P 型
└── .goreleaser.yaml        # クロスプラットフォームビルド設定
```

## Built with claude-forge

このプロジェクトは [claude-forge](https://github.com/imtemp-dev/claude-forge) を使用して構築されました。claude-forge は Claude Code のためのドキュメントファースト AI 開発ツールです。すべての仕様、実装、レビュー、テストが forge アダプティブループを通じて行われました。

## ライセンス

MIT
