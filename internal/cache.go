// Package internal provides caching functionality for content extraction results.
// It implements a thread-safe LRU cache with TTL support to improve performance
// for repeated extractions of the same content.
package internal

import (
	"context"
	"runtime"
	"sync"
	"time"
)

// Default cache cleanup configuration
const (
	DefaultCacheCleanupInterval = 5 * time.Minute
)

// Cache is a generic, thread-safe LRU cache with optional TTL support.
// It is parameterized by the key type K so callers can pick the most efficient
// key representation: the HTML Processor keys its cache by a 128-bit hash value
// ([16]byte) to avoid allocating a key string on every extraction, while tests
// and other callers may use plain string keys. K must be comparable (a
// requirement of Go maps, which back the cache).
//
// It uses a doubly-linked list for LRU ordering with sentinel nodes
// to simplify edge case handling.
//
// TTL Behavior:
//   - ttl > 0: Entries expire after the specified duration
//   - ttl = 0: Entries never expire based on time (only LRU eviction)
//   - ttl < 0: Treated as 0 (no time-based expiration)
//
// Thread Safety: All public methods are safe for concurrent use.
//
// The lock is a plain sync.Mutex rather than sync.RWMutex: Get() must promote
// the matched entry to the front of the LRU list (a write), so it takes the
// write lock anyway, and the only remaining reader was Len(). An RWMutex here
// bought no read concurrency in exchange for its larger word and RLock path,
// so Mutex is the simpler, honest choice.
//
// SECURITY: Cache entries may contain sensitive data. Use Clear() to remove
// all entries when processing sensitive content. Consider setting an appropriate
// TTL to limit data retention.
type Cache[K comparable] struct {
	mu         sync.Mutex
	entries    map[K]*cacheEntry[K]
	maxEntries int
	ttl        time.Duration
	head, tail *cacheEntry[K] // Sentinel nodes for doubly-linked list

	// Cleanup management
	cleanupMu     sync.Mutex         // Protects cleanupCancel and cleanupOnce coordination
	cleanupCancel context.CancelFunc // Function to stop background cleanup
	cleanupOnce   sync.Once          // Ensures cleanup goroutine starts only once
}

type cacheEntry[K comparable] struct {
	prev, next *cacheEntry[K]
	lastUsed   int64
	expiresAt  int64
	value      any
	key        K
}

func (e *cacheEntry[K]) isExpired(now int64) bool {
	return e.expiresAt > 0 && now > e.expiresAt
}

// NewCache creates a new LRU cache with the specified maximum entries and TTL.
// If maxEntries is 0 or negative, the cache is disabled (Set becomes a no-op).
// If ttl is 0 or negative, entries never expire based on time.
//
// The key type K must be specified at construction, e.g. NewCache[string](...)
// or NewCache[[16]byte](...).
func NewCache[K comparable](maxEntries int, ttl time.Duration) *Cache[K] {
	if maxEntries < 0 {
		maxEntries = 0
	}
	if ttl < 0 {
		ttl = 0
	}
	c := &Cache[K]{
		entries:    make(map[K]*cacheEntry[K], maxEntries),
		maxEntries: maxEntries,
		ttl:        ttl,
	}
	// Initialize sentinel nodes
	c.head = &cacheEntry[K]{}
	c.tail = &cacheEntry[K]{}
	c.head.next = c.tail
	c.tail.prev = c.head
	return c
}

// cacheKeyZero reports whether k is the zero value of its type. The zero value
// is treated as the "no key" sentinel by Get/Set (a real key must be non-zero),
// matching the legacy behavior where the empty string "" was rejected.
func cacheKeyZero[K comparable](k K) bool {
	var zero K
	return k == zero
}

func (c *Cache[K]) Get(key K) any {
	if cacheKeyZero(key) {
		return nil
	}
	now := time.Now().UnixNano()

	// Lock for the whole Get: a read-only check would race with a concurrent
	// Set over the same entry's LRU position, so Get promotes the entry to the
	// front under the write lock (see type doc for why Cache uses sync.Mutex).
	c.mu.Lock()
	defer c.mu.Unlock()

	entry := c.entries[key]
	if entry == nil {
		return nil
	}

	// Check expiration
	if entry.isExpired(now) {
		c.removeNode(entry)
		delete(c.entries, key)
		return nil
	}

	// Update LRU position
	entry.lastUsed = now
	c.moveToFront(entry)

	return entry.value
}

func (c *Cache[K]) Set(key K, value any) {
	if value == nil || cacheKeyZero(key) || c.maxEntries == 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().UnixNano()

	// Check if key already exists (update case)
	if entry, exists := c.entries[key]; exists {
		entry.value = value
		entry.lastUsed = now
		if c.ttl > 0 {
			entry.expiresAt = now + c.ttl.Nanoseconds()
		}
		c.moveToFront(entry)
		return
	}

	// Need to add new entry - check capacity
	if len(c.entries) >= c.maxEntries {
		c.evictOne(now)
	}

	// Create new entry
	entry := &cacheEntry[K]{
		value:     value,
		lastUsed:  now,
		key:       key,
		expiresAt: 0,
	}
	if c.ttl > 0 {
		entry.expiresAt = now + c.ttl.Nanoseconds()
	}

	// Add to front of list and map
	c.entries[key] = entry
	c.addToFront(entry)
}

// moveToFront moves an entry to the front (most recently used position)
func (c *Cache[K]) moveToFront(entry *cacheEntry[K]) {
	if entry == nil || entry == c.head || entry == c.tail {
		return
	}
	// Remove from current position
	entry.prev.next = entry.next
	entry.next.prev = entry.prev
	// Add to front
	c.addToFront(entry)
}

// addToFront adds an entry right after head (most recently used position)
func (c *Cache[K]) addToFront(entry *cacheEntry[K]) {
	if entry == nil {
		return
	}
	entry.prev = c.head
	entry.next = c.head.next
	c.head.next.prev = entry
	c.head.next = entry
}

// removeNode removes an entry from the doubly-linked list
func (c *Cache[K]) removeNode(entry *cacheEntry[K]) {
	if entry == nil || entry == c.head || entry == c.tail {
		return
	}
	entry.prev.next = entry.next
	entry.next.prev = entry.prev
	entry.prev = nil
	entry.next = nil
}

func (c *Cache[K]) evictOne(nowNano int64) {
	// First, try to remove an expired entry. This scan only makes sense when
	// entries can expire: with ttl == 0 every entry is created with
	// expiresAt == 0, so isExpired always returns false and the full O(n) map
	// scan would run on every capacity-exceeding Set without ever matching.
	// Skipping it avoids serializing that scan against concurrent Gets.
	if c.ttl > 0 {
		for key, entry := range c.entries {
			if entry.isExpired(nowNano) {
				c.removeNode(entry)
				delete(c.entries, key)
				return
			}
		}
	}

	// If no expired entries, evict the least recently used (tail.prev)
	// The tail.prev node is the LRU entry (right before tail sentinel)
	if c.tail.prev != c.head {
		lruEntry := c.tail.prev
		c.removeNode(lruEntry)
		delete(c.entries, lruEntry.key)
	}
}

func (c *Cache[K]) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	// Clear all entries and break references to help GC
	for key, entry := range c.entries {
		if entry != nil {
			entry.prev = nil
			entry.next = nil
		}
		delete(c.entries, key)
	}
	// Reset linked list
	c.head.next = c.tail
	c.tail.prev = c.head
}

// StartCleanup starts a background goroutine that periodically cleans up expired entries.
// This is useful when TTL is enabled and the cache receives many one-time accesses,
// as expired entries would otherwise only be cleaned when accessed or during eviction.
//
// The cleanup goroutine runs at the specified interval until StopCleanup is called
// or the cache is garbage collected. If interval is 0, DefaultCacheCleanupInterval is used.
//
// This method is idempotent - calling it multiple times has no additional effect.
//
// IMPORTANT: While runtime.SetFinalizer ensures cleanup when the Cache is garbage collected,
// it is still recommended to call StopCleanup() explicitly for deterministic resource release,
// especially in long-running applications.
//
// Usage:
//
//	cache := NewCache[string](1000, time.Hour)
//	cache.StartCleanup(5 * time.Minute)
//	defer cache.StopCleanup()
func (c *Cache[K]) StartCleanup(interval time.Duration) context.CancelFunc {
	if interval <= 0 {
		interval = DefaultCacheCleanupInterval
	}

	c.cleanupOnce.Do(func() {
		ctx, cancelFunc := context.WithCancel(context.Background())

		c.cleanupMu.Lock()
		c.cleanupCancel = cancelFunc
		c.cleanupMu.Unlock()

		// Set finalizer to ensure cleanup goroutine cleanup when Cache is garbage collected.
		// This prevents goroutine leaks if StopCleanup() is not called explicitly.
		runtime.SetFinalizer(c, func(cache *Cache[K]) {
			cache.StopCleanup()
		})

		go func() {
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					// Recover per cleanup pass so a single failing pass cannot
					// terminate the background cleaner (and crash the process).
					// cleanupExpired operates only on internal data, so a panic
					// here would be a library bug — but it still must not escape
					// a goroutine spawned by the public API (SEC-003). The value
					// is discarded because the cache has no error channel.
					func() {
						defer func() {
							_ = recover() // keep the cleaner alive on panic
						}()
						c.cleanupExpired()
					}()
				}
			}
		}()
	})

	// Always return the stored cancel func, even on repeated calls
	c.cleanupMu.Lock()
	cancel := c.cleanupCancel
	c.cleanupMu.Unlock()
	return cancel
}

// StopCleanup stops the background cleanup goroutine if it was started.
// It is safe to call this method multiple times.
// This method also clears the finalizer to prevent double cleanup.
func (c *Cache[K]) StopCleanup() {
	c.cleanupMu.Lock()
	cancel := c.cleanupCancel
	c.cleanupCancel = nil
	c.cleanupMu.Unlock()

	if cancel != nil {
		cancel()
		runtime.SetFinalizer(c, nil)
	}
}

// RestartCleanup stops any running cleanup goroutine and starts a new one. It
// is the public lifecycle primitive for a Cache whose sweeper has been stopped
// (e.g. via StopCleanup, or Close on an owning Processor) but whose expired
// entries should keep being evicted.
//
// The internal processor pool previously called this when returning a processor
// to the pool after Close(); that path was dead on every real execution and has
// been removed, so RestartCleanup currently has no in-module production caller.
// It is retained as part of the Cache API for callers that manage the cleanup
// lifecycle directly (and is exercised by TestCacheRestartCleanup).
//
// Concurrency: callers MUST serialize StartCleanup/StopCleanup/RestartCleanup
// against each other for a given Cache. The cleanupOnce reset below is taken
// under cleanupMu so it cannot race with a concurrent StartCleanup's Do(), but
// the broader start→stop→restart sequence is not internally serialized.
func (c *Cache[K]) RestartCleanup(interval time.Duration) {
	c.StopCleanup()
	// This assignment is NOT made race-free by cleanupMu alone: StartCleanup
	// reads cleanupOnce via Do() WITHOUT that lock (it only takes cleanupMu
	// inside the Do closure and again afterward to read cleanupCancel). Safety
	// against a concurrent StartCleanup therefore rests entirely on the caller
	// honoring the "MUST serialize" contract in the method doc above. cleanupMu
	// is still taken here to keep this write ordered with the cleanupCancel
	// reads/writes in StartCleanup/StopCleanup.
	c.cleanupMu.Lock()
	c.cleanupOnce = sync.Once{}
	c.cleanupMu.Unlock()
	c.StartCleanup(interval)
}

// cleanupExpired removes all expired entries from the cache.
// This is called periodically by the background cleanup goroutine.
func (c *Cache[K]) cleanupExpired() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().UnixNano()
	for key, entry := range c.entries {
		if entry.isExpired(now) {
			c.removeNode(entry)
			delete(c.entries, key)
		}
	}
}

// Len returns the current number of entries in the cache.
// This is useful for monitoring and debugging.
func (c *Cache[K]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}
