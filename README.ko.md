# claude-p2p

Claude Code 인스턴스 간 P2P 직접 통신 MCP 서버.

[English](README.md) | [中文](README.zh.md) | [日本語](README.ja.md)

같은 팀의 개발자들이 각자 Claude Code 세션을 열고 작업할 때, 별도 인프라 없이 서로의 세션에 직접 연결하여 메시지 교환과 작업 컨텍스트 공유를 실시간으로 할 수 있습니다.

## 특징

- **제로 인프라** — 중앙 서버, 브로커, 외부 서비스 없음
- **LAN 자동 발견** — mDNS로 같은 네트워크 피어 자동 감지
- **인터넷 연결** — 공유 토픽 코드로 원격 피어 연결 (DHT 기반)
- **실시간 메시징** — 1:1 직접 메시지 + 토픽 브로드캐스트
- **작업 컨텍스트 공유** — 현재 작업 요약, git 레포/브랜치 자동 감지
- **종단간 암호화** — Noise 프로토콜 (libp2p 내장)
- **NAT 통과** — 홀펀칭 + Circuit Relay v2 자동 폴백
- **단일 바이너리** — Go로 빌드, 크로스 플랫폼 (macOS/Linux/Windows)

## 아키텍처

```
개발자 A (서울)                         개발자 B (미국)
┌──────────────┐                      ┌──────────────┐
│ Claude Code  │                      │ Claude Code  │
│     ↕ stdio  │                      │     ↕ stdio  │
│ claude-p2p   │ ←── libp2p ────────→ │ claude-p2p   │
│ (단일 바이너리)│    Noise 암호화       │ (단일 바이너리)│
└──────────────┘    NAT 홀펀칭         └──────────────┘
```

## 설치

### 방법 A: go install (Go 필요)

```bash
go install github.com/imtemp-dev/claude-p2p@latest
claude mcp add claude-p2p $(go env GOPATH)/bin/claude-p2p -s user
```

### 방법 B: 바이너리 다운로드

[GitHub Releases](https://github.com/imtemp-dev/claude-p2p/releases)에서 다운로드 후:

```bash
chmod +x claude-p2p
claude mcp add claude-p2p /path/to/claude-p2p -s user
```

### 방법 C: 소스에서 빌드

```bash
git clone https://github.com/imtemp-dev/claude-p2p.git
cd claude-p2p
go build -o claude-p2p .
claude mcp add claude-p2p $(pwd)/claude-p2p -s user
```

### 실행

Claude Code를 시작하면 `claude-p2p`가 자동으로 MCP 서버로 실행됩니다.

## 로컬 네트워크 테스트 가이드

같은 머신에서 두 개의 Claude Code 세션으로 P2P 통신을 테스트할 수 있습니다.

### 준비

```bash
# 빌드
go build -o claude-p2p .

# 글로벌 등록
claude mcp add claude-p2p $(pwd)/claude-p2p -s user
```

### 방법 1: 두 개의 Claude Code 세션

**터미널 A** — 아무 프로젝트 디렉토리에서:

```bash
claude
```

**터미널 B** — 다른 디렉토리에서:

```bash
claude
```

두 세션 모두 claude-p2p가 MCP 서버로 시작되고, 같은 LAN이면 **mDNS로 자동 발견**됩니다.

어느 세션에서든 Claude에게:

```
연결된 피어 보여줘
```

Claude가 `list_peers`를 호출하여 상대 세션을 표시합니다.

### 방법 2: 수동 MCP 테스트

Claude Code 없이 MCP 프로토콜을 직접 테스트:

**터미널 A:**

```bash
./claude-p2p
```

stderr에 로그가 출력됩니다:

```
[claude-p2p] libp2p peer ID: 12D3KooW...
[claude-p2p] listening on: /ip4/192.168.0.10/tcp/54321/p2p/12D3KooW...
[claude-p2p] mDNS discovery started
[claude-p2p] DHT started
```

**터미널 B:**

```bash
./claude-p2p
```

몇 초 후 양쪽에서:

```
[claude-p2p] mDNS: discovered peer 12D3KooW...
```

### 방법 3: Go 테스트

통합 테스트가 포함되어 있습니다:

```bash
# 전체 테스트 (42개, race detector 포함)
go test ./... -race -v

# 통합 테스트만
go test ./p2p/ -run TestIntegration -v
```

### 사용 예시

두 Claude Code 세션이 연결된 후 자연어로 사용:

```
> "연결된 피어 보여줘"
> "다른 세션에 메시지 보내줘: PR #42 리뷰 부탁"
> "my-team 토픽으로 브로드캐스트: main에 푸시했습니다"
> "새 메시지 확인해줘"
> "작업 요약 설정: auth 모듈 리팩토링 중"
> "my-team 토픽 참여해줘"
```

Claude가 자동으로 적절한 MCP 도구를 호출합니다.

## MCP 도구

| 도구 | 설명 |
|------|------|
| `list_peers` | 연결된 피어 목록 (scope: local/topic/all) |
| `send_message` | 직접 메시지 또는 토픽 브로드캐스트 |
| `get_messages` | 받은 메시지 확인 (pop 또는 peek 모드) |
| `set_summary` | 현재 작업 요약 설정 (피어에게 자동 공유) |
| `join_topic` | 팀/프로젝트 토픽 참여 |

## 연결 방식

### LAN (자동)

같은 네트워크에 있으면 mDNS로 자동 발견. 설정 불필요.

### 인터넷 (토픽 코드)

```bash
# 환경변수로 토픽 설정
CLAUDE_P2P_TOPIC=my-team-abc claude

# 또는 Claude Code에서 도구 호출
join_topic(topic="my-team-abc")
```

같은 토픽 코드를 사용하는 모든 피어가 DHT를 통해 연결됩니다.

## 크로스 플랫폼 빌드

```bash
# goreleaser 설치 (https://goreleaser.com/install/)
brew install goreleaser

# 빌드 (macOS/Linux/Windows × amd64/arm64)
goreleaser build --snapshot --clean
```

## 기술 스택

| 계층 | 기술 |
|------|------|
| 언어 | Go |
| P2P | [go-libp2p](https://github.com/libp2p/go-libp2p) |
| 피어 발견 | mDNS (LAN) + Kademlia DHT (인터넷) |
| 메시징 | libp2p streams (1:1) + GossipSub (브로드캐스트) |
| 암호화 | Noise protocol |
| NAT 통과 | 홀펀칭 + Circuit Relay v2 |
| MCP | JSON-RPC 2.0 over stdio |
| 빌드 | goreleaser |

## 프로젝트 구조

```
claude-p2p/
├── main.go                 # 진입점
├── mcp/
│   ├── server.go           # MCP JSON-RPC 서버
│   ├── types.go            # JSON-RPC + MCP 타입 정의
│   └── tools.go            # 도구 레지스트리
├── node/
│   └── node.go             # MCP 서버 + P2P 호스트 오케스트레이터
├── p2p/
│   ├── host.go             # libp2p 호스트 (NAT, 연결 관리)
│   ├── discovery.go        # mDNS + DHT 피어 발견
│   ├── topic.go            # 토픽 관리 + GossipSub
│   ├── messaging.go        # 직접 메시징 (libp2p streams)
│   ├── inbox.go            # 메시지 인박스 (FIFO 큐)
│   ├── metadata.go         # 작업 컨텍스트 공유
│   ├── peers.go            # 피어 추적기
│   └── types.go            # P2P 타입
└── .goreleaser.yaml        # 크로스 플랫폼 빌드 설정
```

## 라이선스

MIT
