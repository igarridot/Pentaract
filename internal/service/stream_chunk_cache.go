package service

import (
	"container/list"
	"sync"

	"github.com/google/uuid"
)

const (
	defaultStreamChunkCacheMaxEntries = 6
	defaultStreamChunkCacheMaxBytes   = 96 * 1024 * 1024
)

type streamChunkCacheKey struct {
	fileID   uuid.UUID
	position int16
}

type streamChunkCacheEntry struct {
	key  streamChunkCacheKey
	data []byte
	size int64
}

type streamChunkCache struct {
	mu         sync.Mutex
	entries    map[streamChunkCacheKey]*list.Element
	order      *list.List
	totalBytes int64
	maxBytes   int64
	maxEntries int
}

func newStreamChunkCache(maxEntries int, maxBytes int64) *streamChunkCache {
	if maxEntries <= 0 {
		maxEntries = defaultStreamChunkCacheMaxEntries
	}
	if maxBytes <= 0 {
		maxBytes = defaultStreamChunkCacheMaxBytes
	}

	return &streamChunkCache{
		entries:    make(map[streamChunkCacheKey]*list.Element, maxEntries),
		order:      list.New(),
		maxBytes:   maxBytes,
		maxEntries: maxEntries,
	}
}

func (c *streamChunkCache) get(key streamChunkCacheKey) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	element, ok := c.entries[key]
	if !ok {
		return nil, false
	}

	c.order.MoveToFront(element)
	entry := element.Value.(*streamChunkCacheEntry)
	return entry.data, true
}

func (c *streamChunkCache) set(key streamChunkCacheKey, data []byte) {
	if len(data) == 0 || int64(len(data)) > c.maxBytes {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if element, ok := c.entries[key]; ok {
		entry := element.Value.(*streamChunkCacheEntry)
		c.totalBytes -= entry.size
		entry.data = data
		entry.size = int64(len(data))
		c.totalBytes += entry.size
		c.order.MoveToFront(element)
		c.trimLocked()
		return
	}

	entry := &streamChunkCacheEntry{
		key:  key,
		data: data,
		size: int64(len(data)),
	}
	element := c.order.PushFront(entry)
	c.entries[key] = element
	c.totalBytes += entry.size
	c.trimLocked()
}

func (c *streamChunkCache) trimLocked() {
	for len(c.entries) > c.maxEntries || c.totalBytes > c.maxBytes {
		tail := c.order.Back()
		if tail == nil {
			return
		}
		entry := tail.Value.(*streamChunkCacheEntry)
		delete(c.entries, entry.key)
		c.totalBytes -= entry.size
		c.order.Remove(tail)
	}
}
