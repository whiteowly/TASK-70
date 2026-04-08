package cache

import (
	"container/list"
	"sync"
	"time"
)

// LRU is a thread-safe in-memory cache with a maximum capacity and per-entry TTL.
type LRU struct {
	mu       sync.Mutex
	capacity int
	ttl      time.Duration
	items    map[string]*entry
	order    *list.List
}

type entry struct {
	key       string
	value     interface{}
	expiresAt time.Time
	el        *list.Element
}

// NewLRU creates a new LRU cache with the given capacity and TTL.
func NewLRU(capacity int, ttl time.Duration) *LRU {
	return &LRU{
		capacity: capacity,
		ttl:      ttl,
		items:    make(map[string]*entry),
		order:    list.New(),
	}
}

// Get returns the cached value and true if found and not expired. If the entry
// is expired it is removed and (nil, false) is returned.
func (c *LRU) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	e, ok := c.items[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(e.expiresAt) {
		c.removeLocked(e)
		return nil, false
	}
	c.order.MoveToFront(e.el)
	return e.value, true
}

// Set stores a value in the cache. If the cache is at capacity the
// least-recently-used entry is evicted.
func (c *LRU) Set(key string, value interface{}) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if e, ok := c.items[key]; ok {
		e.value = value
		e.expiresAt = time.Now().Add(c.ttl)
		c.order.MoveToFront(e.el)
		return
	}

	if len(c.items) >= c.capacity {
		c.evictLocked()
	}

	e := &entry{
		key:       key,
		value:     value,
		expiresAt: time.Now().Add(c.ttl),
	}
	e.el = c.order.PushFront(e)
	c.items[key] = e
}

// Invalidate removes a single key from the cache.
func (c *LRU) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if e, ok := c.items[key]; ok {
		c.removeLocked(e)
	}
}

// Clear removes all entries from the cache.
func (c *LRU) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*entry)
	c.order.Init()
}

// Len returns the number of entries in the cache.
func (c *LRU) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.items)
}

func (c *LRU) removeLocked(e *entry) {
	c.order.Remove(e.el)
	delete(c.items, e.key)
}

func (c *LRU) evictLocked() {
	back := c.order.Back()
	if back == nil {
		return
	}
	e := back.Value.(*entry)
	c.removeLocked(e)
}
