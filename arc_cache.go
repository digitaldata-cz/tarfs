package tarfs

import (
	"container/list"
	"sync"
)

// arcCache implements an Adaptive Replacement Cache (ARC) algorithm.
// ARC balances between recency (LRU) and frequency (LFU) to provide
// better hit rates than simple LRU caches.
//
// The cache maintains four lists:
// - T1: recent cache entries (LRU)
// - T2: frequent cache entries (LFU)
// - B1: ghost entries evicted from T1
// - B2: ghost entries evicted from T2
type arcCache struct {
	mu sync.RWMutex

	// Maximum cache size
	capacity int

	// Target size for T1 (adaptive)
	p int

	// Cache lists
	t1       *list.List               // Recent items
	t2       *list.List               // Frequent items
	b1       *list.List               // Ghost entries from T1
	b2       *list.List               // Ghost entries from T2
	cache    map[string]*arcEntry     // Actual cached data
	ghosts   map[string]*list.Element // Ghost entries (metadata only)
	elements map[string]*list.Element // Map to list elements
	inT2     map[string]bool          // Track which elements are in T2 (optimization)
}

type arcEntry struct {
	key  string
	data []byte
}

// newARCCache creates a new ARC cache with the specified capacity
func newARCCache(capacity int) *arcCache {
	if capacity <= 0 {
		capacity = 128
	}
	return &arcCache{
		capacity: capacity,
		p:        0,
		t1:       list.New(),
		t2:       list.New(),
		b1:       list.New(),
		b2:       list.New(),
		cache:    make(map[string]*arcEntry),
		ghosts:   make(map[string]*list.Element),
		elements: make(map[string]*list.Element),
		inT2:     make(map[string]bool),
	}
}

// Get retrieves data from cache, returns nil if not found
func (c *arcCache) Get(key string) []byte {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if in cache
	if entry, ok := c.cache[key]; ok {
		// Move to T2 (frequent list) or update position
		if elem, ok := c.elements[key]; ok {
			// Check if in T1 or T2 using our tracking map (O(1) instead of O(n))
			if !c.inT2[key] {
				// In T1, move to T2
				c.t1.Remove(elem)
				delete(c.elements, key)
				newElem := c.t2.PushFront(entry)
				c.elements[key] = newElem
				c.inT2[key] = true
			} else {
				// Already in T2, move to front
				c.t2.MoveToFront(elem)
			}
		}
		return entry.data
	}

	return nil
}

// Put adds data to cache
func (c *arcCache) Put(key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Already in cache - update and move to T2
	if entry, ok := c.cache[key]; ok {
		entry.data = data
		if elem, ok := c.elements[key]; ok {
			if !c.inT2[key] {
				// In T1, move to T2
				c.t1.Remove(elem)
				delete(c.elements, key)
				newElem := c.t2.PushFront(entry)
				c.elements[key] = newElem
				c.inT2[key] = true
			} else {
				// Already in T2, move to front
				c.t2.MoveToFront(elem)
			}
		}
		return
	}

	// Check if in ghost lists
	inB1 := false
	inB2 := false
	if _, ok := c.ghosts[key]; ok {
		if c.ghostInB1(key) {
			inB1 = true
		} else {
			inB2 = true
		}
	}

	// Case 1: Cache hit in B1
	if inB1 {
		// Adapt: increase p
		delta := 1
		if c.b2.Len() > 0 && c.b1.Len() >= c.b2.Len() {
			delta = c.b1.Len() / c.b2.Len()
			if delta < 1 {
				delta = 1
			}
		}
		c.p = min(c.p+delta, c.capacity)
		c.replace(key, false)
		c.removeGhost(key)

		entry := &arcEntry{key: key, data: data}
		elem := c.t2.PushFront(entry)
		c.cache[key] = entry
		c.elements[key] = elem
		c.inT2[key] = true
		return
	}

	// Case 2: Cache hit in B2
	if inB2 {
		// Adapt: decrease p
		delta := 1
		if c.b1.Len() > 0 && c.b2.Len() >= c.b1.Len() {
			delta = c.b2.Len() / c.b1.Len()
			if delta < 1 {
				delta = 1
			}
		}
		c.p = max(c.p-delta, 0)
		c.replace(key, true)
		c.removeGhost(key)

		entry := &arcEntry{key: key, data: data}
		elem := c.t2.PushFront(entry)
		c.cache[key] = entry
		c.elements[key] = elem
		c.inT2[key] = true
		return
	}

	// Case 3: Cache miss
	// Check if we need to evict
	if c.t1.Len()+c.b1.Len() == c.capacity {
		if c.t1.Len() < c.capacity {
			// Delete LRU from B1
			if c.b1.Len() > 0 {
				elem := c.b1.Back()
				if elem != nil {
					ghostKey := elem.Value.(string)
					c.removeGhost(ghostKey)
				}
			}
			c.replace(key, false)
		} else {
			// Delete LRU from T1
			if c.t1.Len() > 0 {
				elem := c.t1.Back()
				if elem != nil {
					entry := elem.Value.(*arcEntry)
					c.t1.Remove(elem)
					delete(c.cache, entry.key)
					delete(c.elements, entry.key)
				}
			}
		}
	} else if c.t1.Len()+c.t2.Len()+c.b1.Len()+c.b2.Len() >= c.capacity {
		if c.t1.Len()+c.t2.Len()+c.b1.Len()+c.b2.Len() == 2*c.capacity {
			// Delete LRU from B2
			if c.b2.Len() > 0 {
				elem := c.b2.Back()
				if elem != nil {
					ghostKey := elem.Value.(string)
					c.removeGhost(ghostKey)
				}
			}
		}
		c.replace(key, false)
	}

	// Add to T1
	entry := &arcEntry{key: key, data: data}
	elem := c.t1.PushFront(entry)
	c.cache[key] = entry
	c.elements[key] = elem
	c.inT2[key] = false // Mark as being in T1
}

// replace handles eviction logic
func (c *arcCache) replace(key string, inB2 bool) {
	if c.t1.Len() > 0 && ((c.t1.Len() > c.p) || (inB2 && c.t1.Len() == c.p)) {
		// Evict from T1 to B1
		elem := c.t1.Back()
		if elem != nil {
			entry := elem.Value.(*arcEntry)
			c.t1.Remove(elem)
			delete(c.cache, entry.key)
			delete(c.elements, entry.key)
			delete(c.inT2, entry.key) // Remove T2 tracking

			// Add to B1 ghosts
			ghostElem := c.b1.PushFront(entry.key)
			c.ghosts[entry.key] = ghostElem
		}
	} else if c.t2.Len() > 0 {
		// Evict from T2 to B2
		elem := c.t2.Back()
		if elem != nil {
			entry := elem.Value.(*arcEntry)
			c.t2.Remove(elem)
			delete(c.cache, entry.key)
			delete(c.elements, entry.key)
			delete(c.inT2, entry.key) // Remove T2 tracking

			// Add to B2 ghosts
			ghostElem := c.b2.PushFront(entry.key)
			c.ghosts[entry.key] = ghostElem
		}
	}
}

// Helper functions
func (c *arcCache) ghostInB1(key string) bool {
	if elem, ok := c.ghosts[key]; ok {
		for e := c.b1.Front(); e != nil; e = e.Next() {
			if e == elem {
				return true
			}
		}
	}
	return false
}

func (c *arcCache) removeGhost(key string) {
	if elem, ok := c.ghosts[key]; ok {
		delete(c.ghosts, key)
		// Try to remove from B1
		for e := c.b1.Front(); e != nil; e = e.Next() {
			if e == elem {
				c.b1.Remove(e)
				return
			}
		}
		// Try to remove from B2
		for e := c.b2.Front(); e != nil; e = e.Next() {
			if e == elem {
				c.b2.Remove(e)
				return
			}
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Len returns the number of items in the cache
func (c *arcCache) Len() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.cache)
}

// Clear removes all items from the cache
func (c *arcCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t1.Init()
	c.t2.Init()
	c.b1.Init()
	c.b2.Init()
	c.cache = make(map[string]*arcEntry)
	c.ghosts = make(map[string]*list.Element)
	c.elements = make(map[string]*list.Element)
	c.inT2 = make(map[string]bool) // Reset T2 tracking map
	c.p = 0
}
