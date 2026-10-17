package service

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
)

func TestStreamChunkCacheBudgetKeepsAsManyFullChunksAsFit(t *testing.T) {
	budget := int64(3) * UploadChunkSize
	cache := newStreamChunkCacheWithBudget(budget)
	if cache.maxBytes != budget {
		t.Fatalf("maxBytes: got %d, want %d", cache.maxBytes, budget)
	}
	if cache.maxEntries != 4 {
		t.Fatalf("maxEntries: got %d, want 4 (three full chunks plus one)", cache.maxEntries)
	}

	fileID := uuid.New()
	full := bytes.Repeat([]byte("x"), UploadChunkSize)
	for position := int16(0); position < 3; position++ {
		cache.set(streamChunkCacheKey{fileID: fileID, position: position}, full)
	}
	for position := int16(0); position < 3; position++ {
		if _, ok := cache.get(streamChunkCacheKey{fileID: fileID, position: position}); !ok {
			t.Fatalf("chunk %d should fit inside the budget", position)
		}
	}

	cache.set(streamChunkCacheKey{fileID: fileID, position: 3}, full)
	if _, ok := cache.get(streamChunkCacheKey{fileID: fileID, position: 0}); ok {
		t.Fatal("the least recently used chunk must be evicted once the budget is exceeded")
	}
	if _, ok := cache.get(streamChunkCacheKey{fileID: fileID, position: 3}); !ok {
		t.Fatal("the newest chunk must be cached")
	}
}

func TestStreamChunkCacheBudgetDefaultsWhenUnset(t *testing.T) {
	for _, budget := range []int64{0, -1} {
		cache := newStreamChunkCacheWithBudget(budget)
		if cache.maxBytes != defaultStreamChunkCacheMaxBytes || cache.maxEntries != defaultStreamChunkCacheMaxEntries {
			t.Fatalf("budget %d: got entries=%d bytes=%d, want defaults", budget, cache.maxEntries, cache.maxBytes)
		}
	}
}
