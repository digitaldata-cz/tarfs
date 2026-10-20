package tarfs

import (
	"container/list"
	"sync"
)

// ARC holds decoded immutable payloads. T1/T2 are resident recent/frequent
// entries; B1/B2 retain only evicted keys. All transitions and ghost removals
// use indexed list elements, with no scans proportional to cache capacity.
type arcCache struct {
	mu             sync.RWMutex
	capacity       int
	p              int
	t1, t2, b1, b2 *list.List
	cache          map[string]*arcEntry
	ghosts         map[string]*list.Element
	ghostList      map[string]*list.List
	elements       map[string]*list.Element
	inT2           map[string]bool
}

type arcEntry struct {
	key  string
	data []byte
}

func newARCCache(capacity int) *arcCache {
	if capacity <= 0 {
		capacity = 128
	}
	return &arcCache{
		capacity: capacity, t1: list.New(), t2: list.New(), b1: list.New(), b2: list.New(),
		cache: make(map[string]*arcEntry), ghosts: make(map[string]*list.Element),
		ghostList: make(map[string]*list.List), elements: make(map[string]*list.Element), inT2: make(map[string]bool),
	}
}

func (cache *arcCache) Get(key string) []byte {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	entry, exists := cache.cache[key]
	if !exists {
		return nil
	}
	cache.promote(key)
	return entry.data
}

func (cache *arcCache) promote(key string) {
	element := cache.elements[key]
	if cache.inT2[key] {
		cache.t2.MoveToFront(element)
		return
	}
	cache.t1.Remove(element)
	cache.elements[key] = cache.t2.PushFront(cache.cache[key])
	cache.inT2[key] = true
}

func (cache *arcCache) Put(key string, data []byte) {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	if entry, exists := cache.cache[key]; exists {
		entry.data = data
		cache.promote(key)
		return
	}
	if owner, exists := cache.ghostList[key]; exists {
		inB2 := owner == cache.b2
		if inB2 {
			cache.p = max(0, cache.p-max(1, cache.b1.Len()/cache.b2.Len()))
		} else {
			cache.p = min(cache.capacity, cache.p+max(1, cache.b2.Len()/cache.b1.Len()))
		}
		cache.replace(inB2)
		cache.removeGhost(key)
		cache.add(key, data, true)
		return
	}
	if cache.t1.Len()+cache.b1.Len() == cache.capacity {
		if cache.t1.Len() < cache.capacity {
			cache.removeGhost(cache.b1.Back().Value.(string))
			cache.replace(false)
		} else {
			cache.removeResident(cache.t1, cache.t1.Back())
		}
	} else if cache.t1.Len()+cache.t2.Len()+cache.b1.Len()+cache.b2.Len() >= cache.capacity {
		if cache.t1.Len()+cache.t2.Len()+cache.b1.Len()+cache.b2.Len() == 2*cache.capacity {
			cache.removeGhost(cache.b2.Back().Value.(string))
		}
		cache.replace(false)
	}
	cache.add(key, data, false)
}

func (cache *arcCache) add(key string, data []byte, frequent bool) {
	entry := &arcEntry{key: key, data: data}
	cache.cache[key] = entry
	cache.inT2[key] = frequent
	if frequent {
		cache.elements[key] = cache.t2.PushFront(entry)
	} else {
		cache.elements[key] = cache.t1.PushFront(entry)
	}
}

func (cache *arcCache) removeResident(owner *list.List, element *list.Element) string {
	key := element.Value.(*arcEntry).key
	owner.Remove(element)
	delete(cache.cache, key)
	delete(cache.elements, key)
	delete(cache.inT2, key)
	return key
}

func (cache *arcCache) replace(inB2 bool) {
	var key string
	var ghost *list.List
	if cache.t1.Len() > 0 && (cache.t1.Len() > cache.p || (inB2 && cache.t1.Len() == cache.p)) {
		key = cache.removeResident(cache.t1, cache.t1.Back())
		ghost = cache.b1
	} else if cache.t2.Len() > 0 {
		key = cache.removeResident(cache.t2, cache.t2.Back())
		ghost = cache.b2
	} else {
		return
	}
	cache.ghosts[key] = ghost.PushFront(key)
	cache.ghostList[key] = ghost
}

func (cache *arcCache) ghostInB1(key string) bool { return cache.ghostList[key] == cache.b1 }

func (cache *arcCache) removeGhost(key string) {
	if owner, exists := cache.ghostList[key]; exists {
		owner.Remove(cache.ghosts[key])
		delete(cache.ghosts, key)
		delete(cache.ghostList, key)
	}
}

func (cache *arcCache) Len() int { cache.mu.RLock(); defer cache.mu.RUnlock(); return len(cache.cache) }

func (cache *arcCache) Clear() {
	cache.mu.Lock()
	defer cache.mu.Unlock()
	cache.t1.Init()
	cache.t2.Init()
	cache.b1.Init()
	cache.b2.Init()
	clear(cache.cache)
	clear(cache.elements)
	clear(cache.inT2)
	clear(cache.ghosts)
	clear(cache.ghostList)
	cache.p = 0
}
