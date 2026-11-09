package api

import (
	"container/list"
	"sync"
)

const (
	faceCacheBudget  = 128 << 20 // total cached bytes
	faceCacheMaxItem = 32 << 20  // larger faces are served uncached
)

// faceKey identifies one extracted face; mtime and size make a stale entry
// miss when the file changes on disk.
type faceKey struct {
	id    int64
	mtime int64
	size  int64
}

// faceLRU is a byte-budgeted cache of extracted TTC/OTC faces, so scrolling
// a list of collection faces doesn't re-read and rebuild the file per row.
// Concurrent misses for one key share a single build.
type faceLRU struct {
	mu       sync.Mutex
	budget   int
	maxItem  int
	used     int
	order    *list.List // front = most recent; values are *faceEntry
	byKey    map[faceKey]*list.Element
	inflight map[faceKey]*faceCall
}

type faceEntry struct {
	key  faceKey
	data []byte
}

type faceCall struct {
	done chan struct{}
	data []byte
	err  error
}

func newFaceLRU(budget, maxItem int) *faceLRU {
	return &faceLRU{
		budget: budget, maxItem: maxItem, order: list.New(),
		byKey: make(map[faceKey]*list.Element), inflight: make(map[faceKey]*faceCall),
	}
}

// faceCache is shared by all FontFileMiddleware instances.
var faceCache = newFaceLRU(faceCacheBudget, faceCacheMaxItem)

// get returns the cached face for k, or builds it (once, however many
// callers ask at the same time), caches it if small enough, and returns it.
func (c *faceLRU) get(k faceKey, build func() ([]byte, error)) ([]byte, error) {
	c.mu.Lock()
	if el, ok := c.byKey[k]; ok {
		c.order.MoveToFront(el)
		data := el.Value.(*faceEntry).data
		c.mu.Unlock()
		return data, nil
	}
	if call, ok := c.inflight[k]; ok {
		c.mu.Unlock()
		<-call.done
		return call.data, call.err
	}
	call := &faceCall{done: make(chan struct{})}
	c.inflight[k] = call
	c.mu.Unlock()

	call.data, call.err = build()

	c.mu.Lock()
	delete(c.inflight, k)
	if call.err == nil && len(call.data) <= c.maxItem {
		c.byKey[k] = c.order.PushFront(&faceEntry{key: k, data: call.data})
		c.used += len(call.data)
		for c.used > c.budget && c.order.Len() > 0 {
			last := c.order.Back()
			e := last.Value.(*faceEntry)
			c.order.Remove(last)
			delete(c.byKey, e.key)
			c.used -= len(e.data)
		}
	}
	c.mu.Unlock()
	close(call.done)
	return call.data, call.err
}
