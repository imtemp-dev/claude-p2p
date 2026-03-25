package mcp

import (
	"testing"
)

func TestResourceRegistryListEmpty(t *testing.T) {
	r := NewResourceRegistry()
	list := r.List()
	if len(list) != 0 {
		t.Errorf("expected 0 resources, got %d", len(list))
	}
}

func TestResourceRegistryRegisterAndList(t *testing.T) {
	r := NewResourceRegistry()
	r.Register(Resource{URI: "p2p://inbox", Name: "Inbox"}, func() (*ResourcesReadResult, error) {
		return &ResourcesReadResult{Contents: []ResourceContents{{URI: "p2p://inbox", Text: "test"}}}, nil
	})

	list := r.List()
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}
	if list[0].URI != "p2p://inbox" {
		t.Errorf("URI = %q, want %q", list[0].URI, "p2p://inbox")
	}
	if list[0].Name != "Inbox" {
		t.Errorf("Name = %q, want %q", list[0].Name, "Inbox")
	}
}

func TestResourceRegistryListSorted(t *testing.T) {
	r := NewResourceRegistry()
	r.Register(Resource{URI: "z://z", Name: "Z"}, func() (*ResourcesReadResult, error) { return nil, nil })
	r.Register(Resource{URI: "a://a", Name: "A"}, func() (*ResourcesReadResult, error) { return nil, nil })

	list := r.List()
	if len(list) != 2 {
		t.Fatalf("expected 2, got %d", len(list))
	}
	if list[0].URI != "a://a" || list[1].URI != "z://z" {
		t.Errorf("expected sorted order, got %q, %q", list[0].URI, list[1].URI)
	}
}

func TestResourceRegistryHas(t *testing.T) {
	r := NewResourceRegistry()
	r.Register(Resource{URI: "p2p://inbox", Name: "Inbox"}, func() (*ResourcesReadResult, error) { return nil, nil })

	if !r.Has("p2p://inbox") {
		t.Error("expected Has('p2p://inbox') = true")
	}
	if r.Has("p2p://unknown") {
		t.Error("expected Has('p2p://unknown') = false")
	}
}

func TestResourceRegistryRead(t *testing.T) {
	r := NewResourceRegistry()
	r.Register(Resource{URI: "p2p://inbox", Name: "Inbox"}, func() (*ResourcesReadResult, error) {
		return &ResourcesReadResult{Contents: []ResourceContents{{URI: "p2p://inbox", Text: `{"count":0}`}}}, nil
	})

	result, err := r.Read("p2p://inbox")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content, got %d", len(result.Contents))
	}
	if result.Contents[0].URI != "p2p://inbox" {
		t.Errorf("content URI = %q, want %q", result.Contents[0].URI, "p2p://inbox")
	}
}

func TestResourceRegistryReadNotFound(t *testing.T) {
	r := NewResourceRegistry()
	_, err := r.Read("p2p://unknown")
	if err == nil {
		t.Fatal("expected error for unknown URI")
	}
	if err != ErrResourceNotFound {
		t.Errorf("expected ErrResourceNotFound, got %v", err)
	}
}

func TestResourceRegistryUpdateDescription(t *testing.T) {
	r := NewResourceRegistry()
	r.Register(Resource{URI: "p2p://inbox", Name: "Inbox", Description: "Empty"}, func() (*ResourcesReadResult, error) { return nil, nil })

	r.UpdateDescription("p2p://inbox", "3 unread", "2026-03-25T10:00:00Z")

	list := r.List()
	if list[0].Description != "3 unread" {
		t.Errorf("description = %q, want %q", list[0].Description, "3 unread")
	}
	if list[0].Annotations == nil {
		t.Fatal("expected annotations to be set")
	}
	if list[0].Annotations.LastModified != "2026-03-25T10:00:00Z" {
		t.Errorf("lastModified = %q, want %q", list[0].Annotations.LastModified, "2026-03-25T10:00:00Z")
	}
}

func TestResourceRegistryUpdateDescriptionNilAnnotations(t *testing.T) {
	r := NewResourceRegistry()
	r.Register(Resource{URI: "test://r", Name: "Test"}, func() (*ResourcesReadResult, error) { return nil, nil })

	// Annotations is nil — should not panic
	r.UpdateDescription("test://r", "updated", "2026-01-01T00:00:00Z")

	list := r.List()
	if list[0].Annotations == nil {
		t.Fatal("expected annotations to be initialized")
	}
	if list[0].Annotations.LastModified != "2026-01-01T00:00:00Z" {
		t.Errorf("lastModified = %q", list[0].Annotations.LastModified)
	}
}

func TestResourceRegistryUpdateDescriptionNotFound(t *testing.T) {
	r := NewResourceRegistry()
	// Should not panic
	r.UpdateDescription("nonexistent", "desc", "2026-01-01T00:00:00Z")
}
