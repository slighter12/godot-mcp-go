package types

import "sync"

const defaultReadProjectFileCacheCapacity = 128

type readProjectFileCacheKey struct {
	path            string
	size            int64
	modTimeUnixNano int64
}

type readProjectFileCache struct {
	mu       sync.Mutex
	capacity int
	entries  map[readProjectFileCacheKey][]byte
	order    []readProjectFileCacheKey
	hits     uint64
	misses   uint64
}

func newReadProjectFileCache(capacity int) *readProjectFileCache {
	if capacity <= 0 {
		capacity = defaultReadProjectFileCacheCapacity
	}
	return &readProjectFileCache{
		capacity: capacity,
		entries:  make(map[readProjectFileCacheKey][]byte, capacity),
		order:    make([]readProjectFileCacheKey, 0, capacity),
	}
}

func (c *readProjectFileCache) get(key readProjectFileCacheKey) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	data, ok := c.entries[key]
	if !ok {
		c.misses++
		return nil, false
	}
	c.hits++
	return cloneBytes(data), true
}

func (c *readProjectFileCache) set(key readProjectFileCacheKey, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if _, exists := c.entries[key]; !exists {
		c.order = append(c.order, key)
		if len(c.entries) >= c.capacity {
			evict := c.order[0]
			c.order = c.order[1:]
			delete(c.entries, evict)
		}
	}
	c.entries[key] = cloneBytes(data)
}

func (c *readProjectFileCache) stats() (uint64, uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hits, c.misses
}

func cloneBytes(in []byte) []byte {
	if len(in) == 0 {
		return nil
	}
	out := make([]byte, len(in))
	copy(out, in)
	return out
}

var projectFileReadCache = newReadProjectFileCache(defaultReadProjectFileCacheCapacity)

func resetProjectFileReadCacheForTest() {
	projectFileReadCache = newReadProjectFileCache(defaultReadProjectFileCacheCapacity)
}
