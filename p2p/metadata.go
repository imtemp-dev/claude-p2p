package p2p

import (
	"context"
	"encoding/json"
	"log"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

const (
	// MetadataTopicName is the GossipSub topic for metadata broadcast.
	MetadataTopicName = "_meta"

	// MetadataBroadcastInterval is how often to re-broadcast metadata.
	MetadataBroadcastInterval = 60 * time.Second

	// Field length limits for metadata validation.
	MaxSummaryLength  = 500
	MaxUsernameLength = 100
	MaxRepoLength     = 500
	MaxBranchLength   = 200
)

// PeerMetadata holds work context information about a peer.
type PeerMetadata struct {
	PeerID      string `json:"peer_id"`
	DisplayName string `json:"display_name"`
	Status      string `json:"status,omitempty"`
	IdleSince   string `json:"idle_since,omitempty"`
	Summary     string `json:"summary"`
	Username    string `json:"username"`
	Repo        string `json:"repo"`
	Branch      string `json:"branch"`
	UpdatedAt   string `json:"updated_at"`
}

const (
	StatusActive  = "active"
	StatusIdle    = "idle"
	IdleThreshold = 2 * time.Minute
)

// truncateField truncates a string to maxLen bytes if it exceeds the limit.
func truncateField(s string, maxLen int) string {
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}

// MetadataManager manages local metadata and broadcasts it to peers.
type MetadataManager struct {
	host            host.Host
	peerTracker     *PeerTracker
	topicManager    *TopicManager
	getLastToolCall func() time.Time
	mu              sync.RWMutex
	local           PeerMetadata
	cancel          context.CancelFunc
	wg              sync.WaitGroup
	logger          *log.Logger
}

// NewMetadataManager creates a manager, auto-detects git context, and starts periodic broadcast.
func NewMetadataManager(ctx context.Context, h host.Host, tracker *PeerTracker, tm *TopicManager, logger *log.Logger, getLastToolCall func() time.Time) *MetadataManager {
	if getLastToolCall == nil {
		getLastToolCall = func() time.Time { return time.Time{} }
	}
	username := "unknown"
	if u, err := user.Current(); err == nil {
		username = u.Username
	}

	repo := truncateField(sanitizeRepoURL(detectGit("remote", "get-url", "origin")), MaxRepoLength)
	branch := truncateField(detectGit("rev-parse", "--abbrev-ref", "HEAD"), MaxBranchLength)

	// Generate or load display name
	dir, err := os.Getwd()
	if err != nil {
		dir = "unknown"
	}
	dirBase := filepath.Base(dir)
	displayName := os.Getenv("CLAUDE_P2P_NAME")
	if displayName == "" {
		displayName = loadOrGenerateDisplayName(dirBase)
	} else {
		displayName = sanitizeDisplayName(displayName)
	}
	displayName = truncateFieldUTF8(displayName, MaxDisplayNameLength)

	broadcastCtx, cancel := context.WithCancel(ctx)

	mm := &MetadataManager{
		host:            h,
		peerTracker:     tracker,
		topicManager:    tm,
		getLastToolCall: getLastToolCall,
		local: PeerMetadata{
			PeerID:      h.ID().String(),
			DisplayName: displayName,
			Summary:     "",
			Username:    username,
			Repo:        repo,
			Branch:      branch,
			UpdatedAt:   time.Now().UTC().Format(time.RFC3339),
		},
		cancel: cancel,
		logger: logger,
	}

	mm.wg.Add(1)
	go mm.broadcastLoop(broadcastCtx)
	return mm
}

func detectGit(args ...string) string {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// sanitizeRepoURL strips credentials from a git remote URL.
func sanitizeRepoURL(raw string) string {
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.User == nil {
		return raw
	}
	u.User = nil
	return u.String()
}

// SetSummary updates the local work summary.
func (mm *MetadataManager) SetSummary(summary string) {
	mm.mu.Lock()
	mm.local.Summary = truncateField(summary, MaxSummaryLength)
	mm.local.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	mm.mu.Unlock()
}

// SetDisplayName changes the local peer's display name at runtime.
// The name is sanitized and truncated, then broadcast to peers on the next metadata cycle.
func (mm *MetadataManager) SetDisplayName(name string) {
	mm.mu.Lock()
	mm.local.DisplayName = truncateFieldUTF8(sanitizeDisplayName(name), MaxDisplayNameLength)
	mm.local.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	mm.mu.Unlock()
}

// LocalMetadata returns a copy of the local metadata.
func (mm *MetadataManager) LocalMetadata() PeerMetadata {
	mm.mu.RLock()
	defer mm.mu.RUnlock()
	return mm.local
}

// HandleMetadataMessage processes an incoming metadata broadcast from a remote peer.
// The `from` parameter is the authenticated peer ID from GossipSub transport (not the JSON payload).
func (mm *MetadataManager) HandleMetadataMessage(msg Message, from peer.ID) {
	var meta PeerMetadata
	if err := json.Unmarshal([]byte(msg.Content), &meta); err != nil {
		mm.logger.Printf("metadata unmarshal error: %v", err)
		return
	}

	meta.DisplayName = truncateFieldUTF8(meta.DisplayName, MaxDisplayNameLength)
	meta.Summary = truncateField(meta.Summary, MaxSummaryLength)
	meta.Username = truncateField(meta.Username, MaxUsernameLength)
	meta.Repo = truncateField(meta.Repo, MaxRepoLength)
	meta.Branch = truncateField(meta.Branch, MaxBranchLength)

	mm.peerTracker.SetMetadata(from, meta)
}

func (mm *MetadataManager) broadcastLoop(ctx context.Context) {
	defer mm.wg.Done()

	// Broadcast immediately on start
	mm.broadcast(ctx)

	ticker := time.NewTicker(MetadataBroadcastInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			mm.broadcast(ctx)
		}
	}
}

func (mm *MetadataManager) broadcast(ctx context.Context) {
	// Update status + marshal atomically under write lock
	mm.mu.Lock()
	lastTC := mm.getLastToolCall()
	if lastTC.IsZero() || time.Since(lastTC) > IdleThreshold {
		mm.local.Status = StatusIdle
		if !lastTC.IsZero() {
			mm.local.IdleSince = lastTC.Add(IdleThreshold).Format(time.RFC3339)
		} else {
			mm.local.IdleSince = ""
		}
	} else {
		mm.local.Status = StatusActive
		mm.local.IdleSince = ""
	}
	data, err := json.Marshal(mm.local)
	mm.mu.Unlock()
	if err != nil {
		mm.logger.Printf("metadata marshal error: %v", err)
		return
	}
	if _, err := mm.topicManager.Broadcast(ctx, MetadataTopicName, string(data), MessageOptions{}); err != nil {
		// Don't log on context cancellation
		if ctx.Err() == nil {
			mm.logger.Printf("metadata broadcast error: %v", err)
		}
	}
}

// Close stops the broadcast goroutine.
func (mm *MetadataManager) Close() error {
	mm.cancel()
	mm.wg.Wait()
	return nil
}
