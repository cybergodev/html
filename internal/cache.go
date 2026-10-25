// Package internal provides caching functionality for content extraction results.
// It implements a thread-safe LRU cache with TTL support to improve performance
// for repeated extractions of the same content.
package internal

import (
	"context"
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
	// The entries map carries no size hint: NewCache's callers pass a capacity
	// LIMIT, not an expected population, and hinting with it made every
	// Processor (default MaxCacheEntries: 2000) preallocate ~100KB of buckets
	// at New() — paid in full by the common create-extract-Close processor and
	// by pooled one-shot processors that never store an entry. The map grows
	// amortized on demand instead; a cache actually filled to its limit pays a
	// handful of rehashes over its lifetime, bounded by the entry cap.
	c := &Cache[K]{
		entries:    make(map[K]*cacheEntry[K]),
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

// maxEvictProbe bounds how many entries evictOne inspects for expiry before
// falling back to plain LRU eviction. Probing a bounded sample of the map
// (iteration order is unspecified, so the sample is effectively random)
// keeps a capacity-exceeding Set cheap even when the cache is full of fresh
// entries — the steady state — where the previous unbounded scan visited
// every entry on every Set. Expired entries the probe misses are still
// removed by the background sweeper (when configured), by lazy expiry in
// Get, or by a later probe.
const maxEvictProbe = 32

func (c *Cache[K]) evictOne(nowNano int64) {
	// First, try to remove an expired entry from a bounded sample. This only
	// makes sense when entries can expire: with ttl == 0 every entry is
	// created with expiresAt == 0, so isExpired always returns false and the
	// scan would never match. Skipping it entirely avoids serializing the
	// probe against concurrent Gets.
	if c.ttl > 0 {
		probed := 0
		for key, entry := range c.entries {
			if probed >= maxEvictProbe {
				break
			}
			probed++
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
// The cleanup goroutine runs at the specified interval until StopCleanup is
// called. If interval is 0, DefaultCacheCleanupInterval is used.
//
// This method is idempotent - calling it multiple times has no additional effect.
//
// Concurrency: StartCleanup, StopCleanup, and RestartCleanup are serialized
// per Cache instance (cleanupMu covers the Once.Do below and the Once reset
// in RestartCleanup), so they are safe to call concurrently with each other.
//
// IMPORTANT: The cleanup goroutine holds a reference to the Cache, so the
// Cache can never be garbage-collected (and no finalizer could ever run)
// while the goroutine is alive. Call StopCleanup() to release the goroutine
// deterministically; the owning Processor does this in Close(). A
// runtime.SetFinalizer-based safety net previously documented here could not
// fire under that reference and has been removed.
//
// Usage:
//
//	cache := NewCache[string](1000, time.Hour)
//	cache.StartCleanup(5 * time.Minute)
//	defer cache.StopCleanup()
func (c *Cache[K]) StartCleanup(interval time.Duration) context.CancelFunc {
	c.cleanupMu.Lock()
	defer c.cleanupMu.Unlock()
	return c.startCleanupLocked(interval)
}

// startCleanupLocked starts the cleanup goroutine; c.cleanupMu must be held.
// The interval is clamped here so every public entry point shares one policy.
//
// cleanupMu is held across the Once.Do as well: RestartCleanup resets
// cleanupOnce under this same lock, so anchoring Do() inside the critical
// section is what makes a concurrent StartCleanup/RestartCleanup pair
// race-free (Do would otherwise read the cleanupOnce field while
// RestartCleanup writes it). The Do closure runs with the lock already
// held and therefore must not re-acquire it.
func (c *Cache[K]) startCleanupLocked(interval time.Duration) context.CancelFunc {
	if interval <= 0 {
		interval = DefaultCacheCleanupInterval
	}

	c.cleanupOnce.Do(func() {
		ctx, cancelFunc := context.WithCancel(context.Background())
		c.cleanupCancel = cancelFunc

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

	// Always return the stored cancel func, even on repeated calls. After a
	// StopCleanup the sync.Once is spent and no new goroutine starts, so the
	// stored cancel is nil — return a no-op instead so a caller doing
	// `defer cancel()` does not panic on a nil function.
	if c.cleanupCancel == nil {
		return func() {}
	}
	return c.cleanupCancel
}

// StopCleanup stops the background cleanup goroutine if it was started.
// It is safe to call this method multiple times.
func (c *Cache[K]) StopCleanup() {
	c.cleanupMu.Lock()
	defer c.cleanupMu.Unlock()
	c.stopCleanupLocked()
}

// stopCleanupLocked cancels a running cleanup goroutine; c.cleanupMu must be
// held. Cancel runs under the lock (the previous public method canceled after
// unlocking) so a RestartCleanup holding the lock observes a fully stopped
// cleaner before it resets the Once. The cleanup goroutine itself never takes
// cleanupMu, so canceling under it cannot deadlock.
func (c *Cache[K]) stopCleanupLocked() {
	if cancel := c.cleanupCancel; cancel != nil {
		c.cleanupCancel = nil
		cancel()
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
// Concurrency: StartCleanup, StopCleanup, and RestartCleanup are serialized
// per Cache instance via cleanupMu (which covers both the cleanupOnce reset
// below and StartCleanup's Once.Do), so they are safe to call concurrently
// with each other on the same Cache.
func (c *Cache[K]) RestartCleanup(interval time.Duration) {
	c.cleanupMu.Lock()
	defer c.cleanupMu.Unlock()
	// Atomic under the lock: stop, reset the Once, start. The previous
	// three-step sequence released the lock between the steps, so a
	// concurrent StartCleanup could slip in between the stop and the Once
	// reset — its goroutine's cancel func was then overwritten by the start
	// below, and that first goroutine (holding its Cache reference) leaked
	// for good. Holding cleanupMu across all three steps makes the restart
	// one indivisible transition, honoring the documented "safe to call
	// concurrently" contract.
	c.stopCleanupLocked()
	c.cleanupOnce = sync.Once{}
	c.startCleanupLocked(interval)
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
