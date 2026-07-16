package pathmtu

import (
	"net/netip"
	"sync"
	"time"
)

type PathKey struct {
	EndpointKind string
	EndpointID   string
	DeviceID     string
	Transport    string
	Remote       netip.AddrPort
}

type ConfirmedPath struct {
	Key         PathKey
	Route       RouteCandidate
	Probe       ConfirmResult
	Plan        DatagramPlan
	ConfirmedAt time.Time
}

// Cache stores only peer-confirmed immutable path plans. It has no expiry or
// polling loop; callers explicitly invalidate entries on route change,
// reconnect, or a validated path-MTU error.
type Cache struct {
	mu      sync.RWMutex
	entries map[PathKey]ConfirmedPath
}

func NewCache() *Cache {
	return &Cache{entries: make(map[PathKey]ConfirmedPath)}
}

func (c *Cache) Load(key PathKey) (ConfirmedPath, bool) {
	if c == nil {
		return ConfirmedPath{}, false
	}
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	return entry, ok
}

func (c *Cache) Store(entry ConfirmedPath) {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.entries == nil {
		c.entries = make(map[PathKey]ConfirmedPath)
	}
	c.entries[entry.Key] = entry
	c.mu.Unlock()
}

func (c *Cache) Invalidate(key PathKey) {
	if c == nil {
		return
	}
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

func (c *Cache) InvalidateEndpoint(kind, endpointID string) int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	removed := 0
	for key := range c.entries {
		if key.EndpointKind == kind && key.EndpointID == endpointID {
			delete(c.entries, key)
			removed++
		}
	}
	c.mu.Unlock()
	return removed
}

func (c *Cache) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	clear(c.entries)
	c.mu.Unlock()
}
