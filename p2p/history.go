package p2p

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"
)

const (
	maxHistoryLines   = 10000
	rotateKeepLines   = 7500
	defaultHistoryLimit = 50
	maxHistoryLimit     = 200
)

// HistoryEntry represents a single message log entry (sent or received).
type HistoryEntry struct {
	Direction  string  `json:"direction"`            // "sent" or "recv"
	Message    Message `json:"message"`              // message content
	ReceivedAt string  `json:"received_at,omitempty"` // only for recv
	At         string  `json:"at"`                   // when this entry was logged
}

// HistoryStore persists conversation history to a JSONL file.
type HistoryStore struct {
	mu       sync.Mutex
	filePath string
	logger   *log.Logger
}

// NewHistoryStore creates a history store at the given file path.
func NewHistoryStore(filePath string, logger *log.Logger) *HistoryStore {
	return &HistoryStore{filePath: filePath, logger: logger}
}

// AppendReceived logs a received message. Skips metadata messages.
func (h *HistoryStore) AppendReceived(msg InboxMessage) {
	if h == nil {
		return
	}
	if msg.Topic == MetadataTopicName || msg.Type == "metadata" {
		return
	}
	h.append(HistoryEntry{
		Direction:  "recv",
		Message:    msg.Message,
		ReceivedAt: msg.ReceivedAt,
		At:         time.Now().UTC().Format(time.RFC3339),
	})
}

// AppendSent logs a sent message. Skips metadata messages.
func (h *HistoryStore) AppendSent(msg Message) {
	if h == nil {
		return
	}
	if msg.Topic == MetadataTopicName || msg.Type == "metadata" {
		return
	}
	h.append(HistoryEntry{
		Direction: "sent",
		Message:   msg,
		At:        time.Now().UTC().Format(time.RFC3339),
	})
}

func (h *HistoryStore) append(entry HistoryEntry) {
	h.mu.Lock()
	defer h.mu.Unlock()

	f, err := os.OpenFile(h.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		h.logger.Printf("history write error: %v", err)
		return
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		h.logger.Printf("history marshal error: %v", err)
		return
	}
	f.Write(data)
	f.Write([]byte("\n"))

	h.maybeRotate()
}

// Load returns recent history entries with optional filters.
func (h *HistoryStore) Load(limit int, peerID string, topic string) ([]HistoryEntry, error) {
	if h == nil {
		return nil, nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()

	if limit <= 0 {
		limit = defaultHistoryLimit
	}
	if limit > maxHistoryLimit {
		limit = maxHistoryLimit
	}

	f, err := os.Open(h.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var all []HistoryEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		var entry HistoryEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if peerID != "" {
			if entry.Message.From != peerID && entry.Message.To != peerID {
				continue
			}
		}
		if topic != "" {
			if entry.Message.Topic != topic {
				continue
			}
		}
		all = append(all, entry)
	}

	// Return last `limit` entries
	if len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all, nil
}

// maybeRotate truncates the file if it exceeds maxHistoryLines.
// Must be called under lock.
func (h *HistoryStore) maybeRotate() {
	f, err := os.Open(h.filePath)
	if err != nil {
		return
	}
	defer f.Close()

	var lineCount int
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	for scanner.Scan() {
		lineCount++
	}

	if lineCount <= maxHistoryLines {
		return
	}

	// Re-read and keep last rotateKeepLines
	f.Seek(0, 0)
	scanner = bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) > rotateKeepLines {
		lines = lines[len(lines)-rotateKeepLines:]
	}

	tmpPath := h.filePath + ".tmp"
	tmp, err := os.Create(tmpPath)
	if err != nil {
		return
	}
	for _, line := range lines {
		tmp.Write([]byte(line))
		tmp.Write([]byte("\n"))
	}
	tmp.Close()
	os.Rename(tmpPath, h.filePath)
}
