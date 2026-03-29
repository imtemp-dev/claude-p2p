package p2p

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func newTestHistory(t *testing.T) *HistoryStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "history.jsonl")
	return NewHistoryStore(path, log.New(os.Stderr, "[test] ", log.LstdFlags))
}

func TestHistoryAppendAndLoad(t *testing.T) {
	h := newTestHistory(t)

	h.AppendReceived(InboxMessage{
		Message:    Message{ID: "m1", From: "peer-a", Content: "hello", Type: "direct"},
		ReceivedAt: "2026-03-27T00:00:00Z",
	})
	h.AppendSent(Message{ID: "m2", To: "peer-a", Content: "hi back", Type: "direct"})

	entries, err := h.Load(50, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}
	if entries[0].Direction != "recv" || entries[0].Message.ID != "m1" {
		t.Errorf("unexpected first entry: %+v", entries[0])
	}
	if entries[1].Direction != "sent" || entries[1].Message.ID != "m2" {
		t.Errorf("unexpected second entry: %+v", entries[1])
	}
}

func TestHistoryAppendReceived_SkipsMetadata(t *testing.T) {
	h := newTestHistory(t)

	h.AppendReceived(InboxMessage{
		Message: Message{ID: "m1", Topic: "_meta", Type: "broadcast"},
	})
	h.AppendReceived(InboxMessage{
		Message: Message{ID: "m2", Type: "metadata"},
	})
	h.AppendReceived(InboxMessage{
		Message: Message{ID: "m3", Content: "real message", Type: "direct"},
	})

	entries, err := h.Load(50, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry (metadata skipped), got %d", len(entries))
	}
	if entries[0].Message.ID != "m3" {
		t.Errorf("expected m3, got %s", entries[0].Message.ID)
	}
}

func TestHistoryLoadWithPeerFilter(t *testing.T) {
	h := newTestHistory(t)

	h.AppendReceived(InboxMessage{Message: Message{ID: "m1", From: "peer-a", Content: "from a", Type: "direct"}})
	h.AppendReceived(InboxMessage{Message: Message{ID: "m2", From: "peer-b", Content: "from b", Type: "direct"}})
	h.AppendSent(Message{ID: "m3", To: "peer-a", Content: "to a", Type: "direct"})

	entries, err := h.Load(50, "peer-a", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries for peer-a, got %d", len(entries))
	}
}

func TestHistoryLoadWithLimit(t *testing.T) {
	h := newTestHistory(t)

	for i := 0; i < 20; i++ {
		h.AppendSent(Message{ID: "m", Content: "msg", Type: "direct"})
	}

	entries, err := h.Load(5, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}
}

func TestHistoryLoadEmpty(t *testing.T) {
	h := newTestHistory(t)

	entries, err := h.Load(50, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if entries != nil {
		t.Errorf("expected nil for empty history, got %d entries", len(entries))
	}
}

func TestHistoryNilStore(t *testing.T) {
	var h *HistoryStore
	// Should not panic
	h.AppendReceived(InboxMessage{Message: Message{ID: "m1"}})
	h.AppendSent(Message{ID: "m2"})
	entries, err := h.Load(50, "", "")
	if err != nil || entries != nil {
		t.Errorf("nil store should return nil, nil")
	}
}

func TestHistoryConcurrentAppend(t *testing.T) {
	h := newTestHistory(t)
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				h.AppendSent(Message{ID: "m", Content: "msg", Type: "direct"})
			}
		}()
	}
	wg.Wait()

	entries, err := h.Load(200, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 100 {
		t.Errorf("expected 100 entries, got %d", len(entries))
	}
}
