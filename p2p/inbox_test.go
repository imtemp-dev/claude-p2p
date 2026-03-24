package p2p

import (
	"sync"
	"testing"
	"time"
)

func TestInboxPushPop(t *testing.T) {
	inbox := NewInbox(10)

	inbox.Push(InboxMessage{
		Message:    Message{ID: "1", Content: "hello", Type: "direct"},
		ReceivedAt: time.Now().Format(time.RFC3339),
	})
	inbox.Push(InboxMessage{
		Message:    Message{ID: "2", Content: "world", Type: "direct"},
		ReceivedAt: time.Now().Format(time.RFC3339),
	})

	if inbox.Len() != 2 {
		t.Errorf("expected len 2, got %d", inbox.Len())
	}

	msgs := inbox.Pop()
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Content != "hello" {
		t.Errorf("first message = %q, want %q", msgs[0].Content, "hello")
	}
	if msgs[1].Content != "world" {
		t.Errorf("second message = %q, want %q", msgs[1].Content, "world")
	}

	// Inbox should be empty after Pop
	if inbox.Len() != 0 {
		t.Errorf("expected len 0 after pop, got %d", inbox.Len())
	}
	if msgs := inbox.Pop(); msgs != nil {
		t.Errorf("expected nil from empty pop, got %v", msgs)
	}
}

func TestInboxPeek(t *testing.T) {
	inbox := NewInbox(10)
	inbox.Push(InboxMessage{
		Message: Message{ID: "1", Content: "test"},
	})

	msgs := inbox.Peek()
	if len(msgs) != 1 {
		t.Fatalf("peek expected 1, got %d", len(msgs))
	}

	// Peek should not clear
	if inbox.Len() != 1 {
		t.Errorf("inbox should still have 1 after peek, got %d", inbox.Len())
	}
}

func TestInboxOverflow(t *testing.T) {
	inbox := NewInbox(3)

	for i := 0; i < 5; i++ {
		inbox.Push(InboxMessage{
			Message: Message{ID: string(rune('a' + i)), Content: string(rune('A' + i))},
		})
	}

	if inbox.Len() != 3 {
		t.Errorf("expected 3 (capped), got %d", inbox.Len())
	}

	msgs := inbox.Pop()
	// Oldest 2 dropped, remaining: c, d, e
	if len(msgs) != 3 {
		t.Fatalf("expected 3, got %d", len(msgs))
	}
	if msgs[0].Content != "C" {
		t.Errorf("oldest should be C (a,b dropped), got %q", msgs[0].Content)
	}
}

func TestInboxDefaultCapacity(t *testing.T) {
	inbox := NewInbox(0)
	// Should use default
	for i := 0; i < DefaultInboxCapacity+5; i++ {
		inbox.Push(InboxMessage{Message: Message{ID: "x"}})
	}
	if inbox.Len() != DefaultInboxCapacity {
		t.Errorf("expected %d, got %d", DefaultInboxCapacity, inbox.Len())
	}
}

func TestInboxConcurrent(t *testing.T) {
	inbox := NewInbox(100)
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				inbox.Push(InboxMessage{Message: Message{ID: "x"}})
			}
		}(i)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			inbox.Pop()
			inbox.Peek()
			inbox.Len()
		}
	}()

	wg.Wait()
}

func TestInboxEmptyPeek(t *testing.T) {
	inbox := NewInbox(10)
	msgs := inbox.Peek()
	if len(msgs) != 0 {
		t.Errorf("expected empty peek, got %d", len(msgs))
	}
}
