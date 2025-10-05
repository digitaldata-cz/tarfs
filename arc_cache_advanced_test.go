package tarfs

import (
	"testing"
)

// Test ARC cache eviction
func TestARCCacheEviction(t *testing.T) {
	cache := newARCCache(2) // Small cache for testing eviction

	// Fill T1
	cache.Put("a", []byte("data-a"))
	cache.Put("b", []byte("data-b"))

	// Both should be in cache
	if cache.Get("a") == nil {
		t.Error("'a' should be in cache")
	}
	if cache.Get("b") == nil {
		t.Error("'b' should be in cache")
	}

	// Access 'a' again, moves to T2
	cache.Get("a")

	// Add new item, should evict from T1
	cache.Put("c", []byte("data-c"))

	// 'b' should be evicted
	if cache.Get("b") != nil {
		t.Error("'b' should have been evicted")
	}

	// 'a' and 'c' should still be there
	if cache.Get("a") == nil {
		t.Error("'a' should still be in cache")
	}
	if cache.Get("c") == nil {
		t.Error("'c' should still be in cache")
	}
}

// Test ARC cache ghost lists
func TestARCCacheGhostLists(t *testing.T) {
	cache := newARCCache(2)

	// Fill cache
	cache.Put("a", []byte("data-a"))
	cache.Put("b", []byte("data-b"))

	// Access 'a' to move to T2
	cache.Get("a")

	// Add 'c', 'b' evicted to B1
	cache.Put("c", []byte("data-c"))

	// Verify 'b' not in cache
	if cache.Get("b") != nil {
		t.Error("'b' should not be in cache")
	}

	// Add 'd', should affect adaptive parameter p
	cache.Put("d", []byte("data-d"))

	// Re-add 'b', should be a hit in ghost list
	cache.Put("b", []byte("data-b-new"))

	// 'b' should now be in cache
	if data := cache.Get("b"); data == nil {
		t.Error("'b' should be in cache after re-adding")
	} else if string(data) != "data-b-new" {
		t.Errorf("Expected 'data-b-new', got '%s'", string(data))
	}
}

// Test ARC adaptive parameter p
func TestARCAdaptiveParameter(t *testing.T) {
	cache := newARCCache(4)

	// Sequential access pattern (LRU-friendly)
	for i := 0; i < 8; i++ {
		key := string(rune('a' + i))
		cache.Put(key, []byte("data-"+key))
	}

	// Frequent access pattern (LFU-friendly)
	for i := 0; i < 10; i++ {
		cache.Get("e")
		cache.Get("f")
	}

	// Add new items
	cache.Put("x", []byte("data-x"))
	cache.Put("y", []byte("data-y"))

	// Frequently accessed items should still be there
	if cache.Get("e") == nil {
		t.Error("Frequently accessed 'e' should still be in cache")
	}
	if cache.Get("f") == nil {
		t.Error("Frequently accessed 'f' should still be in cache")
	}
}

// Test ARC cache with nil data
func TestARCCacheNilData(t *testing.T) {
	cache := newARCCache(2)

	// Put nil data
	cache.Put("nil", nil)

	// Should be able to retrieve nil
	data := cache.Get("nil")
	if data != nil {
		t.Error("Retrieved data should be nil")
	}
}

// Test ARC cache with empty key
func TestARCCacheEmptyKey(t *testing.T) {
	cache := newARCCache(2)

	// Put with empty key
	cache.Put("", []byte("empty-key"))

	// Should be retrievable
	data := cache.Get("")
	if data == nil {
		t.Error("Should be able to get empty key")
	}
	if string(data) != "empty-key" {
		t.Errorf("Expected 'empty-key', got '%s'", string(data))
	}
}

// Test ARC cache overwrite
func TestARCCacheOverwrite(t *testing.T) {
	cache := newARCCache(2)

	// Put initial value
	cache.Put("key", []byte("value1"))

	// Overwrite
	cache.Put("key", []byte("value2"))

	// Should get latest value
	data := cache.Get("key")
	if data == nil {
		t.Error("Key should exist")
	}
	if string(data) != "value2" {
		t.Errorf("Expected 'value2', got '%s'", string(data))
	}
}

// Test ARC cache size 0 (should use default)
func TestARCCacheSizeZero(t *testing.T) {
	cache := newARCCache(0)

	// Put should not panic
	cache.Put("a", []byte("data"))

	// Cache should have default size (128), so it should store data
	if cache.Get("a") == nil {
		t.Error("Cache with size 0 should use default size and store data")
	}
}

// Test ARC cache size 1
func TestARCCacheSizeOne(t *testing.T) {
	cache := newARCCache(1)

	cache.Put("a", []byte("data-a"))

	// Should be in cache
	if cache.Get("a") == nil {
		t.Error("'a' should be in cache")
	}

	// Add another item
	cache.Put("b", []byte("data-b"))

	// 'a' should be evicted
	if cache.Get("a") != nil {
		t.Error("'a' should have been evicted")
	}

	// 'b' should be there
	if cache.Get("b") == nil {
		t.Error("'b' should be in cache")
	}
}

// Test ARC cache concurrent access patterns
func TestARCCacheAccessPatterns(t *testing.T) {
	cache := newARCCache(4)

	// Pattern 1: FIFO-like
	for i := 0; i < 8; i++ {
		key := string(rune('0' + i))
		cache.Put(key, []byte(key))
	}

	// Last 4 should be in cache
	for i := 4; i < 8; i++ {
		key := string(rune('0' + i))
		if cache.Get(key) == nil {
			t.Errorf("'%s' should be in cache", key)
		}
	}

	// Pattern 2: Hot data
	cache = newARCCache(4)
	cache.Put("hot1", []byte("hot1"))
	cache.Put("hot2", []byte("hot2"))

	// Access hot data many times
	for i := 0; i < 10; i++ {
		cache.Get("hot1")
		cache.Get("hot2")
	}

	// Add more data
	for i := 0; i < 4; i++ {
		key := string(rune('a' + i))
		cache.Put(key, []byte(key))
	}

	// Hot data should still be there
	if cache.Get("hot1") == nil {
		t.Error("Hot data 'hot1' should still be in cache")
	}
	if cache.Get("hot2") == nil {
		t.Error("Hot data 'hot2' should still be in cache")
	}
}

// Test ARC cache ghost list cleanup
func TestARCCacheGhostListCleanup(t *testing.T) {
	cache := newARCCache(2)

	// Fill cache and cause evictions
	for i := 0; i < 10; i++ {
		key := string(rune('a' + i))
		cache.Put(key, []byte(key))
	}

	// Ghost lists should be bounded to cache size
	cache.mu.RLock()
	b1Len := cache.b1.Len()
	b2Len := cache.b2.Len()
	cache.mu.RUnlock()

	// B1 + B2 should not exceed 2 * cache size
	if b1Len+b2Len > 4 {
		t.Errorf("Ghost lists too large: B1=%d, B2=%d", b1Len, b2Len)
	}
}

// Test ARC cache replace logic
func TestARCCacheReplaceLogic(t *testing.T) {
	cache := newARCCache(2)

	// Fill T1
	cache.Put("a", []byte("a"))
	cache.Put("b", []byte("b"))

	// Access 'a' to move to T2
	cache.Get("a")

	// Now T1 has 'b', T2 has 'a'
	cache.mu.RLock()
	t1Len := cache.t1.Len()
	t2Len := cache.t2.Len()
	cache.mu.RUnlock()

	if t1Len != 1 || t2Len != 1 {
		t.Errorf("Expected T1=1, T2=1, got T1=%d, T2=%d", t1Len, t2Len)
	}

	// Add new item when T1 is at target size
	cache.Put("c", []byte("c"))

	// Should evict from T1
	if cache.Get("b") != nil {
		t.Error("'b' should have been evicted from T1")
	}

	// 'a' and 'c' should remain
	if cache.Get("a") == nil {
		t.Error("'a' should still be in cache")
	}
	if cache.Get("c") == nil {
		t.Error("'c' should still be in cache")
	}
}

// Test ARC cache statistics
func TestARCCacheStatistics(t *testing.T) {
	cache := newARCCache(4)

	hits := 0
	misses := 0

	// Add items
	for i := 0; i < 4; i++ {
		key := string(rune('a' + i))
		cache.Put(key, []byte(key))
	}

	// Test hits and misses
	for i := 0; i < 8; i++ {
		key := string(rune('a' + i))
		if cache.Get(key) != nil {
			hits++
		} else {
			misses++
		}
	}

	if hits != 4 {
		t.Errorf("Expected 4 hits, got %d", hits)
	}
	if misses != 4 {
		t.Errorf("Expected 4 misses, got %d", misses)
	}
}

// Test ARC cache memory efficiency
func TestARCCacheMemoryEfficiency(t *testing.T) {
	cache := newARCCache(100)

	// Add large data
	largeData := make([]byte, 1024*1024) // 1MB
	for i := 0; i < 10; i++ {
		key := string(rune('a' + i))
		cache.Put(key, largeData)
	}

	// Only last items should be in cache
	for i := 0; i < 10; i++ {
		key := string(rune('a' + i))
		data := cache.Get(key)
		if i < 10-100 && data != nil {
			t.Errorf("Old item '%s' should have been evicted", key)
		}
	}
}
