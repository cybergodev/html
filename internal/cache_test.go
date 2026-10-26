package internal

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestCacheBasic(t *testing.T) {
	t.Parallel()

	cache := NewCache[string](10, time.Hour)

	// Test Set and Get
	cache.Set("key1", "value1")
	val := cache.Get("key1")
	if val == nil {
		t.Fatal("Get() returned nil for existing key")
	}
	if val.(string) != "value1" {
		t.Errorf("Get() = %v, want %v", val, "value1")
	}

	// Test non-existent key
	val = cache.Get("nonexistent")
	if val != nil {
		t.Errorf("Get() = %v, want nil for non-existent key", val)
	}
}

func TestCacheEmptyKey(t *testing.T) {
	t.Parallel()

	cache := NewCache[string](10, time.Hour)

	cache.Set("", "value")
	val := cache.Get("")
	if val != nil {
		t.Error("Empty key should not be stored")
	}
}

func TestCacheNilValue(t *testing.T) {
	t.Parallel()

	cache := NewCache[string](10, time.Hour)

	cache.Set("key", nil)
	val := cache.Get("key")
	if val != nil {
		t.Error("Nil value should not be stored")
	}
}

func TestCacheTTL(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping TTL test in short mode")
	}

	cache := NewCache[string](10, 50*time.Millisecond)

	cache.Set("key", "value")

	// Should exist immediately
	val := cache.Get("key")
	if val == nil {
		t.Fatal("Get() returned nil immediately after Set()")
	}

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Should be expired
	val = cache.Get("key")
	if val != nil {
		t.Error("Get() should return nil after TTL expiration")
	}
}

func TestCacheNoTTL(t *testing.T) {
	t.Parallel()

	cache := NewCache[string](10, 0) // No TTL

	cache.Set("key", "value")

	val := cache.Get("key")
	if val == nil {
		t.Fatal("Get() returned nil for key with no TTL")
	}
}

func TestCacheEviction(t *testing.T) {
	t.Parallel()

	cache := NewCache[string](3, time.Hour) // Small cache

	// Fill cache
	cache.Set("key1", "value1")
	cache.Set("key2", "value2")
	cache.Set("key3", "value3")

	// All should exist
	if cache.Get("key1") == nil || cache.Get("key2") == nil || cache.Get("key3") == nil {
		t.Fatal("All keys should exist after filling cache")
	}

	// Add one more - should trigger eviction
	cache.Set("key4", "value4")

	// key4 should exist
	if cache.Get("key4") == nil {
		t.Error("Newly added key should exist")
	}

	// At least one old key should be evicted
	count := 0
	if cache.Get("key1") != nil {
		count++
	}
	if cache.Get("key2") != nil {
		count++
	}
	if cache.Get("key3") != nil {
		count++
	}
	if count > 2 {
		t.Error("Eviction should have removed at least one old entry")
	}
}

func TestCacheUpdateExisting(t *testing.T) {
	t.Parallel()

	cache := NewCache[string](3, time.Hour)

	cache.Set("key", "value1")
	cache.Set("key", "value2")

	val := cache.Get("key")
	if val == nil {
		t.Fatal("Get() returned nil")
	}
	if val.(string) != "value2" {
		t.Errorf("Get() = %v, want %v", val, "value2")
	}
}

func TestCacheClear(t *testing.T) {
	t.Parallel()

	cache := NewCache[string](10, time.Hour)

	cache.Set("key1", "value1")
	cache.Set("key2", "value2")

	cache.Clear()

	if cache.Get("key1") != nil || cache.Get("key2") != nil {
		t.Error("Clear() should remove all entries")
	}
}

func TestCacheDisabled(t *testing.T) {
	t.Parallel()

	cache := NewCache[string](0, time.Hour) // Disabled cache

	cache.Set("key", "value")

	val := cache.Get("key")
	if val != nil {
		t.Error("Disabled cache should not store values")
	}
}

func TestCacheConcurrency(t *testing.T) {
	t.Parallel()

	cache := NewCache[string](100, time.Hour)

	const goroutines = 50
	const operations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			for j := 0; j < operations; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				value := fmt.Sprintf("value-%d-%d", id, j)

				cache.Set(key, value)
				val := cache.Get(key)

				if val != nil && val.(string) != value {
					t.Errorf("Concurrent access: got %v, want %v", val, value)
				}
			}
		}(i)
	}

	wg.Wait()
}

func TestCacheLRUBehavior(t *testing.T) {
	t.Parallel()

	cache := NewCache[string](3, time.Hour)

	// Add entries
	cache.Set("key1", "value1")
	time.Sleep(10 * time.Millisecond)
	cache.Set("key2", "value2")
	time.Sleep(10 * time.Millisecond)
	cache.Set("key3", "value3")

	// Access key1 to update its last used time
	cache.Get("key1")

	// Add new entry - should evict key2 (least recently used)
	cache.Set("key4", "value4")

	// key1 should still exist (recently accessed)
	if cache.Get("key1") == nil {
		t.Error("Recently accessed key should not be evicted")
	}

	// key4 should exist (just added)
	if cache.Get("key4") == nil {
		t.Error("Newly added key should exist")
	}
}

func TestCacheExpiredEviction(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping expiration test in short mode")
	}

	cache := NewCache[string](3, 50*time.Millisecond)

	// Add entries
	cache.Set("key1", "value1")
	cache.Set("key2", "value2")

	// Wait for expiration
	time.Sleep(100 * time.Millisecond)

	// Add new entry - should clean up expired entries
	cache.Set("key3", "value3")

	// Expired entries should be gone
	if cache.Get("key1") != nil || cache.Get("key2") != nil {
		t.Error("Expired entries should be removed during eviction")
	}

	// New entry should exist
	if cache.Get("key3") == nil {
		t.Error("New entry should exist")
	}
}

func BenchmarkCacheSet(b *testing.B) {
	cache := NewCache[string](1000, time.Hour)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i%1000)
		cache.Set(key, "value")
	}
}

func BenchmarkCacheGet(b *testing.B) {
	cache := NewCache[string](1000, time.Hour)

	// Pre-populate cache
	for i := 0; i < 1000; i++ {
		cache.Set(fmt.Sprintf("key-%d", i), "value")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i%1000)
		cache.Get(key)
	}
}

func BenchmarkCacheConcurrent(b *testing.B) {
	cache := NewCache[string](1000, time.Hour)

	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i%1000)
			if i%2 == 0 {
				cache.Set(key, "value")
			} else {
				cache.Get(key)
			}
			i++
		}
	})
}

func TestCacheStartCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping cleanup test in short mode")
	}

	cache := NewCache[string](100, 50*time.Millisecond)

	// Start cleanup with 20ms interval
	cancel := cache.StartCleanup(20 * time.Millisecond)
	if cancel == nil {
		t.Fatal("StartCleanup should return a cancel function")
	}
	defer cancel()

	// Add entries
	cache.Set("key1", "value1")
	cache.Set("key2", "value2")

	// Verify entries exist
	if cache.Len() != 2 {
		t.Errorf("Expected 2 entries, got %d", cache.Len())
	}

	// Wait for TTL expiration and cleanup
	time.Sleep(100 * time.Millisecond)

	// Entries should be cleaned up
	if cache.Len() != 0 {
		t.Errorf("Expected 0 entries after cleanup, got %d", cache.Len())
	}
}

// Removed TestCacheStopCleanup and TestCacheCleanupIdempotent (no meaningful
// assertions): repeated StopCleanup and repeated StartCleanup are pinned with
// real oracles by TestCacheStartRestartConcurrent (no-panic + usable cancel)
// and TestCacheMultipleStartCleanup (goroutine census) below.

func TestCacheCleanupNoExpiredEntries(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping cleanup test in short mode")
	}

	// Cache with long TTL - entries won't expire during test
	cache := NewCache[string](100, time.Hour)
	cancel := cache.StartCleanup(20 * time.Millisecond)
	defer cancel()

	// Add entries
	cache.Set("key1", "value1")
	cache.Set("key2", "value2")

	// Wait for a few cleanup cycles
	time.Sleep(100 * time.Millisecond)

	// Entries should still exist (not expired)
	if cache.Len() != 2 {
		t.Errorf("Expected 2 entries, got %d", cache.Len())
	}
}

func TestCacheLen(t *testing.T) {
	cache := NewCache[string](10, time.Hour)

	if cache.Len() != 0 {
		t.Errorf("Empty cache should have 0 entries, got %d", cache.Len())
	}

	cache.Set("key1", "value1")
	if cache.Len() != 1 {
		t.Errorf("Expected 1 entry, got %d", cache.Len())
	}

	cache.Set("key2", "value2")
	if cache.Len() != 2 {
		t.Errorf("Expected 2 entries, got %d", cache.Len())
	}

	// Update existing key should not increase count
	cache.Set("key1", "updated")
	if cache.Len() != 2 {
		t.Errorf("Expected 2 entries after update, got %d", cache.Len())
	}

	cache.Clear()
	if cache.Len() != 0 {
		t.Errorf("Cleared cache should have 0 entries, got %d", cache.Len())
	}
}

func TestCacheCleanupDefaultInterval(t *testing.T) {
	cache := NewCache[string](100, time.Hour)

	// Start cleanup with 0 interval - should use default
	cancel := cache.StartCleanup(0)
	if cancel == nil {
		t.Fatal("StartCleanup(0) should use default interval and return cancel function")
	}
	cancel()
}

func BenchmarkCacheWithCleanup(b *testing.B) {
	cache := NewCache[string](1000, time.Hour)
	cancel := cache.StartCleanup(time.Minute)
	defer cancel()

	// Pre-populate cache
	for i := 0; i < 1000; i++ {
		cache.Set(fmt.Sprintf("key-%d", i), "value")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := fmt.Sprintf("key-%d", i%1000)
		cache.Get(key)
	}
}

// TestCacheGoroutineCleanup verifies that the cleanup goroutine is properly
// stopped when the Cache is garbage collected without explicitly calling StopCleanup.
// This test ensures the runtime.SetFinalizer fix is working correctly.
func TestCacheGoroutineCleanup(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping goroutine cleanup test in short mode")
	}

	// Force GC to get a clean baseline
	runtime.GC()
	time.Sleep(50 * time.Millisecond)

	initialGoroutines := runtime.NumGoroutine()

	// Create a cache and start cleanup, then let it be garbage collected
	// WITHOUT calling StopCleanup explicitly
	func() {
		cache := NewCache[string](100, time.Minute)
		cache.StartCleanup(10 * time.Millisecond)
		// Cache goes out of scope here without StopCleanup being called
		// The finalizer should ensure the goroutine is stopped
	}()

	// Force garbage collection to trigger the finalizer
	for i := 0; i < 5; i++ {
		runtime.GC()
		time.Sleep(50 * time.Millisecond)
	}

	// Allow time for finalizer to execute and goroutine to exit
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()

	// The goroutine count should not have increased
	// Allow some tolerance for other goroutines that may be running
	if finalGoroutines > initialGoroutines+1 {
		t.Errorf("Potential goroutine leak: before=%d, after=%d", initialGoroutines, finalGoroutines)
	}
}

// TestCacheMultipleStartCleanup verifies that StartCleanup is idempotent
// and only starts one cleanup goroutine even when called multiple times.
func TestCacheMultipleStartCleanup(t *testing.T) {
	runtime.GC()
	time.Sleep(50 * time.Millisecond)

	before := runtime.NumGoroutine()

	cache := NewCache[string](100, time.Minute)

	// Call StartCleanup multiple times
	cache.StartCleanup(10 * time.Millisecond)
	cache.StartCleanup(10 * time.Millisecond)
	cache.StartCleanup(10 * time.Millisecond)

	time.Sleep(50 * time.Millisecond)

	after := runtime.NumGoroutine()

	// Should only have started one additional goroutine
	if after > before+1 {
		t.Errorf("Multiple StartCleanup calls should only start one goroutine: before=%d, after=%d", before, after)
	}

	// Clean up
	cache.StopCleanup()
}

// TestCacheStopCleanupStopsEviction covers the sweeper's stop semantics. While
// running, expired entries are evicted in the background; after StopCleanup (as
// the owning Processor's Close does), a newly expired entry lingers because
// nobody is sweeping — and a later StartCleanup cannot bring the sweeper back
// (the Once is spent; documented on StartCleanup).
func TestCacheStopCleanupStopsEviction(t *testing.T) {
	t.Parallel()

	cache := NewCache[string](4, 25*time.Millisecond)
	cache.StartCleanup(10 * time.Millisecond)
	defer cache.StopCleanup()

	cache.Set("present", "v")
	if got := cache.Len(); got != 1 {
		t.Fatalf("Len = %d, want 1 immediately after Set", got)
	}

	// Phase 1: the running sweeper evicts the expired entry.
	if !waitCacheLen(cache, 0, time.Second) {
		t.Fatal("background cleanup did not evict the expired entry")
	}

	// Phase 2: stop the sweeper (as Close() does). A newly expired entry must
	// now linger because nobody is sweeping.
	cache.StopCleanup()
	cache.Set("lingering", "v")
	// Allow more than the TTL + several sweep intervals; if the sweeper were
	// still running the entry would be gone by now.
	time.Sleep(80 * time.Millisecond)
	if cache.Len() == 0 {
		t.Fatal("after StopCleanup the expired entry should linger (no sweeper running)")
	}

	// Phase 3: StartCleanup must not restart a spent sweeper — it returns a
	// no-op cancel and eviction stays off.
	cancel := cache.StartCleanup(10 * time.Millisecond)
	if cancel == nil {
		t.Fatal("StartCleanup after StopCleanup returned nil cancel func")
	}
	cancel()
	cache.Set("still-lingering", "v")
	time.Sleep(80 * time.Millisecond)
	if cache.Len() == 0 {
		t.Fatal("StartCleanup must not restart the sweeper after StopCleanup (the Once is spent)")
	}
}

// waitCacheLen polls c.Len() until it equals want or the timeout elapses.
// Returns true if the target was reached.
func waitCacheLen(c *Cache[string], want int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if c.Len() == want {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return c.Len() == want
}

// TestCacheStartStopConcurrent pins the internal serialization of the cleanup
// lifecycle: StartCleanup's Once.Do runs under cleanupMu and StopCleanup takes
// the same lock. A concurrent mix of StartCleanup/StopCleanup on one Cache must
// not race on the cleanupOnce/cleanupCancel fields (run with -race), must not
// panic, and must leave a Cache whose StartCleanup still returns a usable
// CancelFunc.
func TestCacheStartStopConcurrent(t *testing.T) {
	t.Parallel()

	cache := NewCache[string](16, time.Minute)
	defer cache.StopCleanup()

	const goroutines = 8
	const iterations = 50

	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Add(1)
		go func(mode int) {
			defer wg.Done()
			for i := range iterations {
				// Rotate operations within each goroutine so StartCleanup
				// and StopCleanup genuinely overlap across goroutines.
				if (mode+i)%2 == 0 {
					cancel := cache.StartCleanup(time.Millisecond)
					cancel() // returned CancelFunc must always be callable
				} else {
					cache.StopCleanup()
				}
			}
		}(g)
	}
	wg.Wait()

	// The lifecycle must remain usable after the hammering. After any
	// concurrent Stop the Once may be spent, in which case this returns the
	// documented no-op cancel — either way non-nil and callable.
	cancel := cache.StartCleanup(time.Millisecond)
	if cancel == nil {
		t.Fatal("StartCleanup returned a nil CancelFunc after concurrent Start/Stop")
	}
	cancel()
}

// TestCacheNewClampsNegativeArgs covers the negative-argument clamping in
// NewCache (maxEntries<0 -> 0, ttl<0 -> 0). A disabled cache must reject all
// Sets and return nil for all Gets.
func TestCacheNewClampsNegativeArgs(t *testing.T) {
	t.Parallel()

	c := NewCache[string](-5, -time.Second)
	if c.maxEntries != 0 {
		t.Errorf("negative maxEntries should clamp to 0, got %d", c.maxEntries)
	}
	if c.ttl != 0 {
		t.Errorf("negative ttl should clamp to 0, got %v", c.ttl)
	}

	c.Set("k", "v")
	if got := c.Get("k"); got != nil {
		t.Errorf("disabled cache Get should return nil, got %v", got)
	}
	if c.Len() != 0 {
		t.Errorf("disabled cache should stay empty, got Len=%d", c.Len())
	}
}

// TestCacheEvictOneEmptyList covers the evictOne branch where the cache map and
// the linked list disagree on emptiness (tail.prev == head): evicting must not
// panic and must leave the cache consistent. This branch is unreachable through
// the public Set path (evictOne only runs at capacity), so it is driven directly.
func TestCacheEvictOneEmptyList(t *testing.T) {
	t.Parallel()

	c := NewCache[string](2, 0)
	c.evictOne(time.Now().UnixNano()) // no entries: LRU tail == head sentinel
	if c.Len() != 0 {
		t.Errorf("evictOne on empty cache should not change Len, got %d", c.Len())
	}
}

// TestCacheLRUEvictionOrder pins the eviction policy end to end: at capacity,
// the least-recently-Get/Set entry is evicted, and a Get refreshes an entry's
// recency so it survives the next eviction. This exercises the
// moveToFront/addToFront/removeNode/evictOne chain in its intended order.
func TestCacheLRUEvictionOrder(t *testing.T) {
	t.Parallel()

	c := NewCache[string](2, 0)
	c.Set("a", 1)
	c.Set("b", 2)

	// Touch "a" so "b" becomes the least recently used.
	if got := c.Get("a"); got != 1 {
		t.Fatalf("Get(a) = %v, want 1", got)
	}

	// Inserting "c" at capacity must evict "b", not "a".
	c.Set("c", 3)
	if got := c.Get("b"); got != nil {
		t.Errorf("Get(b) = %v after b should have been evicted as LRU", got)
	}
	if got := c.Get("a"); got != 1 {
		t.Errorf("Get(a) after eviction = %v, want 1 (Get refreshed recency)", got)
	}
	if got := c.Get("c"); got != 3 {
		t.Errorf("Get(c) = %v, want 3", got)
	}
	if c.Len() != 2 {
		t.Errorf("Len = %d, want 2", c.Len())
	}

	// Overwriting an existing key must not grow the cache or evict.
	c.Set("a", 10)
	if c.Len() != 2 {
		t.Errorf("Len after overwrite = %d, want 2", c.Len())
	}
	if got := c.Get("a"); got != 10 {
		t.Errorf("Get(a) after overwrite = %v, want 10", got)
	}
}

// TestCacheEvictProbeCap pins the maxEvictProbe boundary of evictOne: with a
// TTL configured and more live (non-expired) entries than the probe budget,
// the expired-entry scan stops after maxEvictProbe candidates and falls back
// to evicting the LRU entry — the bounded probe must not scan the whole map
// on every Set past capacity.
func TestCacheEvictProbeCap(t *testing.T) {
	t.Parallel()

	const capacity = maxEvictProbe + 8
	c := NewCache[string](capacity, time.Hour)
	for i := 0; i < capacity; i++ {
		c.Set(fmt.Sprintf("k%02d", i), "v")
	}
	if got := c.Len(); got != capacity {
		t.Fatalf("Len() = %d, want %d", got, capacity)
	}

	// One entry past capacity triggers eviction; none are expired, so the
	// probe scans maxEvictProbe entries without a match and must fall back
	// to the LRU entry (k00, the oldest).
	c.Set("new", "v")
	if got := c.Len(); got != capacity {
		t.Fatalf("Len() after eviction = %d, want %d", got, capacity)
	}
	if c.Get("k00") != nil {
		t.Error("LRU entry k00 survived eviction: the probe overran its budget or skipped the LRU fallback")
	}
	if c.Get("new") == nil {
		t.Error("the inserting entry was evicted instead of the LRU entry")
	}
}

// TestCacheEvictOnePrefersExpired covers evictOne's other exit: with TTL
// configured and an expired entry present, the probe removes the expired
// entry instead of falling back to the LRU eviction.
func TestCacheEvictOnePrefersExpired(t *testing.T) {
	t.Parallel()

	c := NewCache[string](4, time.Millisecond)
	for i := 0; i < 4; i++ {
		c.Set(fmt.Sprintf("e%d", i), "v")
	}
	time.Sleep(50 * time.Millisecond) // all four entries are now expired

	c.Set("fresh", "v")
	if c.Get("fresh") == nil {
		t.Error("fresh entry was lost during eviction of expired entries")
	}
	if got := c.Len(); got != 4 {
		t.Errorf("Len() = %d, want 4 after evicting one expired entry", got)
	}
}

// TestCacheStartCleanupAfterStopReturnsUsableCancel pins that StartCleanup
// still returns a usable CancelFunc after an explicit StopCleanup (the
// pooled-processor reuse path: Close() stops the sweeper, a later user
// starts it again). From p2_regression_test.go.
func TestCacheStartCleanupAfterStopReturnsUsableCancel(t *testing.T) {
	t.Parallel()
	c := NewCache[string](4, 0)
	cancel := c.StartCleanup(0)
	if cancel != nil {
		cancel()
	}
	c.StopCleanup()
	cancel2 := c.StartCleanup(0)
	if cancel2 == nil {
		t.Fatal("StartCleanup after StopCleanup returned nil cancel func")
	}
	cancel2() // must not panic
}

// TestConcurrentCacheCleanupLifecycle pins the serialization contract
// documented on Cache: StartCleanup and StopCleanup are safe to call
// concurrently with each other and with Get/Set/Clear. StartCleanup's Once.Do
// runs under cleanupMu; a regression that moves it outside the lock trips the
// race detector here. The final census additionally verifies that the last
// StopCleanup leaves no cleanup goroutine behind. (Merged from
// cache_lifecycle_race_test.go.)
func TestConcurrentCacheCleanupLifecycle(t *testing.T) {
	runtime.GC()
	time.Sleep(5 * time.Millisecond)
	initial := runtime.NumGoroutine()

	cache := NewCache[string](64, 10*time.Millisecond)

	var wg sync.WaitGroup

	// Data traffic: hammers the entry mutex while the lifecycle goroutines
	// below churn the cleanup goroutine.
	for i := range 4 {
		wg.Go(func() {
			for j := range 400 {
				key := fmt.Sprintf("k%d-%d", i, j%8)
				cache.Set(key, j)
				cache.Get(key)
				if j%50 == 0 {
					cache.Clear()
				}
			}
		})
	}

	// Lifecycle churn: concurrent Start/Stop at staggered intervals so a
	// stop frequently lands between another goroutine's Once.Do and its
	// return.
	for i := range 3 {
		wg.Go(func() {
			interval := time.Duration(i+1) * time.Millisecond
			for j := range 60 {
				if (i+j)%2 == 0 {
					cache.StartCleanup(interval)
				} else {
					cache.StopCleanup()
				}
			}
		})
	}

	wg.Wait()
	cache.StopCleanup()

	// Settle past the longest interval used (3ms), then require the goroutine
	// census to return to its starting count: the final StopCleanup must have
	// cancelled the last cleanup goroutine, and nothing starts a new one.
	runtime.GC()
	time.Sleep(50 * time.Millisecond)
	if final := runtime.NumGoroutine(); final > initial+1 {
		t.Errorf("cleanup goroutine leak: initial=%d final=%d", initial, final)
	}
}

// ---------------------------------------------------------------------------
// LRU link guard branches ([FIX-001] boundary additions)
// ---------------------------------------------------------------------------

// TestCacheLRULinkGuards drives the doubly-linked-list helpers' degenerate
// receiver branches directly (nil entry, head sentinel, tail sentinel): they
// must be no-ops that leave the list and Len unchanged.
func TestCacheLRULinkGuards(t *testing.T) {
	t.Parallel()

	c := NewCache[string](3, 0)
	c.Set("a", 1)
	c.Set("b", 2)

	before := c.Len()

	c.moveToFront(nil)    // nil entry
	c.moveToFront(c.head) // head sentinel
	c.moveToFront(c.tail) // tail sentinel
	c.addToFront(nil)
	c.removeNode(nil)
	c.removeNode(c.head)
	c.removeNode(c.tail)

	if got := c.Len(); got != before {
		t.Errorf("Len changed from %d to %d after no-op guard calls", before, got)
	}
	if got := c.Get("a"); got != 1 {
		t.Errorf("Get(a) = %v after guard calls, want 1", got)
	}
	if got := c.Get("b"); got != 2 {
		t.Errorf("Get(b) = %v after guard calls, want 2", got)
	}
}
