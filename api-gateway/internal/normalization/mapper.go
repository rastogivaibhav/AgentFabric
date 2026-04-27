package normalization

import (
	"fmt"
	"sync"
)

// EventMapper defines the interface for mapping vendor-specific events to canonical format
type EventMapper interface {
	Map(event interface{}) (*CanonicalEvent, error)
	Accepts(sourceVendor, sourceProduct string) bool
}

// MapperRegistry manages vendor-specific event mappers using factory pattern
type MapperRegistry struct {
	mu      sync.RWMutex
	mappers map[string]EventMapper
}

// NewMapperRegistry creates a new registry of event mappers
func NewMapperRegistry() *MapperRegistry {
	return &MapperRegistry{
		mappers: make(map[string]EventMapper),
	}
}

// Register adds a mapper for a specific vendor
func (r *MapperRegistry) Register(sourceVendor string, mapper EventMapper) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.mappers[sourceVendor] = mapper
}

// Map returns a canonical event using the appropriate registered mapper
func (r *MapperRegistry) Map(sourceVendor string, event interface{}) (*CanonicalEvent, error) {
	r.mu.RLock()
	mapper, ok := r.mappers[sourceVendor]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("no mapper registered for vendor: %s", sourceVendor)
	}

	return mapper.Map(event)
}

// Get returns a mapper for a specific vendor
func (r *MapperRegistry) Get(sourceVendor string) (EventMapper, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	mapper, ok := r.mappers[sourceVendor]
	if !ok {
		return nil, fmt.Errorf("no mapper found for vendor: %s", sourceVendor)
	}
	return mapper, nil
}

// ListVendors returns all registered vendor names
func (r *MapperRegistry) ListVendors() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	vendors := make([]string, 0, len(r.mappers))
	for vendor := range r.mappers {
		vendors = append(vendors, vendor)
	}
	return vendors
}

// Global registry instance
var globalRegistry = NewMapperRegistry()

// RegisterMapper adds a mapper to the global registry
func RegisterMapper(sourceVendor string, mapper EventMapper) {
	globalRegistry.Register(sourceVendor, mapper)
}

// GetRegistry returns the global mapper registry
func GetRegistry() *MapperRegistry {
	return globalRegistry
}
