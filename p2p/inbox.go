package p2p

import "sync"

// DefaultInboxCapacity is the default maximum number of messages in the inbox.
const DefaultInboxCapacity = 100

// InboxMessage wraps a Message with reception metadata.
type InboxMessage struct {
	Message
	ReceivedAt string `json:"received_at"`
}

// Inbox is a bounded in-memory FIFO queue for received messages.
type Inbox struct {
	mu       sync.Mutex
	messages []InboxMessage
	capacity int
}

// NewInbox creates an inbox with the given capacity.
// If capacity <= 0, DefaultInboxCapacity is used.
func NewInbox(capacity int) *Inbox {
	if capacity <= 0 {
		capacity = DefaultInboxCapacity
	}
	return &Inbox{
		messages: make([]InboxMessage, 0, capacity),
		capacity: capacity,
	}
}

// Push adds a message to the inbox. If at capacity, the oldest message is dropped.
func (i *Inbox) Push(msg InboxMessage) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if len(i.messages) >= i.capacity {
		newMsgs := make([]InboxMessage, len(i.messages)-1, i.capacity)
		copy(newMsgs, i.messages[1:])
		i.messages = newMsgs
	}
	i.messages = append(i.messages, msg)
}

// Pop returns all messages and clears the inbox. Returns nil if empty.
func (i *Inbox) Pop() []InboxMessage {
	i.mu.Lock()
	defer i.mu.Unlock()
	if len(i.messages) == 0 {
		return nil
	}
	result := make([]InboxMessage, len(i.messages))
	copy(result, i.messages)
	i.messages = make([]InboxMessage, 0, i.capacity)
	return result
}

// Peek returns a copy of all messages without clearing the inbox.
func (i *Inbox) Peek() []InboxMessage {
	i.mu.Lock()
	defer i.mu.Unlock()
	result := make([]InboxMessage, len(i.messages))
	copy(result, i.messages)
	return result
}

// Len returns the count of messages in the inbox.
func (i *Inbox) Len() int {
	i.mu.Lock()
	defer i.mu.Unlock()
	return len(i.messages)
}
