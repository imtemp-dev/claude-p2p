package mcp

import (
	"errors"
	"sort"
	"sync"
)

// ErrResourceNotFound is returned when a resource URI is not registered.
var ErrResourceNotFound = errors.New("resource not found")

// ResourceReadHandler reads a resource and returns its contents.
type ResourceReadHandler func() (*ResourcesReadResult, error)

// registeredResource pairs a resource definition with its read handler.
type registeredResource struct {
	resource Resource
	handler  ResourceReadHandler
}

// ResourceRegistry manages MCP resources.
type ResourceRegistry struct {
	mu        sync.RWMutex
	resources map[string]registeredResource // keyed by URI
}

// NewResourceRegistry creates an empty resource registry.
func NewResourceRegistry() *ResourceRegistry {
	return &ResourceRegistry{
		resources: make(map[string]registeredResource),
	}
}

// Register adds a resource with its read handler.
func (r *ResourceRegistry) Register(resource Resource, handler ResourceReadHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.resources[resource.URI] = registeredResource{resource: resource, handler: handler}
}

// UpdateDescription updates the description and LastModified of an existing resource.
// If the resource is not found, this is a no-op.
func (r *ResourceRegistry) UpdateDescription(uri string, description string, lastModified string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	rr, ok := r.resources[uri]
	if !ok {
		return
	}
	rr.resource.Description = description
	if rr.resource.Annotations == nil {
		rr.resource.Annotations = &ResourceAnnotations{}
	}
	rr.resource.Annotations.LastModified = lastModified
	r.resources[uri] = rr
}

// List returns all registered resources sorted by URI.
func (r *ResourceRegistry) List() []Resource {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Resource, 0, len(r.resources))
	for _, rr := range r.resources {
		result = append(result, rr.resource)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].URI < result[j].URI
	})
	return result
}

// Has returns true if a resource with the given URI is registered.
func (r *ResourceRegistry) Has(uri string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.resources[uri]
	return ok
}

// Read looks up a resource by URI and calls its handler.
// The handler is called outside the lock to prevent blocking UpdateDescription.
func (r *ResourceRegistry) Read(uri string) (*ResourcesReadResult, error) {
	r.mu.RLock()
	rr, ok := r.resources[uri]
	r.mu.RUnlock()
	if !ok {
		return nil, ErrResourceNotFound
	}
	return rr.handler()
}
