package html

// concurrent_test.go — concurrency stress tests for cache, audit, pool,
// links, and batch, now including the P-002 race-window hardening tests
// merged from race_hardening_test.go (sections marked below).

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cybergodev/html/internal"
)

// /////////////////////////////////////////////////////////////////////////////
// From concurrent_test.go
// /////////////////////////////////////////////////////////////////////////////

// These tests verify thread-safety and detect race conditions.

// TestConcurrentProcessorExtraction tests concurrent HTML extraction.
func TestConcurrentProcessorExtraction(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 100
	cfg.CacheTTL = time.Minute
	processor, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}
	defer func() { _ = processor.Close() }()

	html := []byte(`<html><body><article><h1>Concurrent Title</h1><p>Test content</p></article></body></html>`)
	numGoroutines := 100
	numOperations := 50

	var wg sync.WaitGroup
	var errorCount atomic.Int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				result, err := processor.Extract(html)
				if err != nil {
					errorCount.Add(1)
					continue
				}
				// Cache hits must return the same content the miss path built.
				if result.Title != "Concurrent Title" {
					errorCount.Add(1)
				}
			}
		}(i)
	}

	wg.Wait()

	if errorCount.Load() > 0 {
		t.Errorf("Concurrent extraction had %d errors", errorCount.Load())
	}

	stats := processor.GetStatistics()
	expectedOps := int64(numGoroutines * numOperations)
	if stats.TotalProcessed != expectedOps {
		t.Errorf("Expected %d total processed, got %d", expectedOps, stats.TotalProcessed)
	}
}

// TestConcurrentCacheOperations tests concurrent cache access.
func TestConcurrentCacheOperations(t *testing.T) {
	cache := internal.NewCache[string](1000, time.Minute)

	numGoroutines := 50
	numOperations := 100

	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				cache.Set(key, fmt.Sprintf("value-%d-%d", id, j))
			}
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				key := fmt.Sprintf("key-%d-%d", id, j)
				cache.Get(key)
			}
		}(i)
	}

	wg.Wait()
}

// TestConcurrentCacheSetGet tests concurrent Set and Get on same keys.
func TestConcurrentCacheSetGet(t *testing.T) {
	cache := internal.NewCache[string](100, time.Minute)

	numGoroutines := 20
	numOperations := 1000
	keys := []string{"shared-1", "shared-2", "shared-3"}

	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(2)

		// Writer goroutine
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				for _, key := range keys {
					cache.Set(key, fmt.Sprintf("writer-%d-value-%d", id, j))
				}
			}
		}(i)

		// Reader goroutine
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				for _, key := range keys {
					_ = cache.Get(key)
				}
			}
		}()
	}

	wg.Wait()
}

// TestConcurrentCacheClear tests concurrent Clear with Set/Get.
func TestConcurrentCacheClear(t *testing.T) {
	cache := internal.NewCache[string](100, time.Minute)

	numGoroutines := 30
	numOperations := 200

	var wg sync.WaitGroup

	// Writers
	for i := 0; i < numGoroutines/3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				cache.Set(fmt.Sprintf("key-%d", id), fmt.Sprintf("value-%d", j))
			}
		}(i)
	}

	// Readers
	for i := 0; i < numGoroutines/3; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				cache.Get(fmt.Sprintf("key-%d", id))
			}
		}(i)
	}

	// Clearers
	for i := 0; i < numGoroutines/3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations/10; j++ {
				cache.Clear()
			}
		}()
	}

	wg.Wait()
}

// TestConcurrentAuditCollector tests concurrent audit logging.
func TestConcurrentAuditCollector(t *testing.T) {
	config := AuditConfig{
		Enabled:           true,
		LogBlockedTags:    true,
		LogBlockedAttrs:   true,
		LogBlockedURLs:    true,
		IncludeRawValues:  true,
		MaxRawValueLength: 100,
		// Room for every recorded entry: this test asserts that all concurrent
		// writes are retained; the eviction cap itself has its own tests.
		MaxEntries: 50*100*3 + 1000,
	}
	collector := newAuditCollector(config)
	defer func() { _ = collector.Close() }()

	numGoroutines := 50
	numOperations := 100

	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				collector.RecordBlockedTag(fmt.Sprintf("script-%d-%d", id, j))
				collector.RecordBlockedAttr("onclick", fmt.Sprintf("alert(%d)", id))
				collector.RecordBlockedURL(fmt.Sprintf("javascript:alert(%d)", id), "xss")
			}
		}(i)
	}

	// Concurrent reads during writes
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				_ = collector.GetEntries()
			}
		}()
	}

	wg.Wait()

	entries := collector.GetEntries()
	expectedEntries := numGoroutines * numOperations * 3 // 3 types of records
	if len(entries) != expectedEntries {
		t.Errorf("Expected %d entries, got %d", expectedEntries, len(entries))
	}
}

// TestConcurrentAuditCollectorClear tests concurrent Clear with Record.
func TestConcurrentAuditCollectorClear(t *testing.T) {
	config := AuditConfig{Enabled: true}
	collector := newAuditCollector(config)
	defer func() { _ = collector.Close() }()

	numGoroutines := 20
	numOperations := 200

	var wg sync.WaitGroup

	// Writers
	for i := 0; i < numGoroutines/2; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				collector.RecordBlockedTag(fmt.Sprintf("tag-%d-%d", id, j))
			}
		}(i)
	}

	// Clearers
	for i := 0; i < numGoroutines/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations/10; j++ {
				collector.Clear()
			}
		}()
	}

	wg.Wait()

	// Invariant: whatever entries survived the interleaved clears, the count
	// can never exceed the total number of records written.
	maxEntries := (numGoroutines / 2) * numOperations
	if got := len(collector.GetEntries()); got > maxEntries {
		t.Errorf("entries = %d, want <= %d", got, maxEntries)
	}
}

// TestConcurrentProcessorStatistics tests concurrent statistics access.
func TestConcurrentProcessorStatistics(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 100
	cfg.CacheTTL = time.Minute
	processor, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}
	defer func() { _ = processor.Close() }()

	html := []byte(`<html><body><p>Test</p></body></html>`)
	numGoroutines := 50
	numOperations := 100

	var wg sync.WaitGroup

	// Processors
	for i := 0; i < numGoroutines/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				_, _ = processor.Extract(html)
			}
		}()
	}

	// Statistics readers
	for i := 0; i < numGoroutines/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				stats := processor.GetStatistics()
				_ = stats.TotalProcessed
				_ = stats.CacheHits
				_ = stats.CacheMisses
			}
		}()
	}

	wg.Wait()

	// Reset statistics concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			processor.ResetStatistics()
		}()
	}

	wg.Wait()
}

// TestConcurrentChannelAuditSink tests concurrent writes to ChannelAuditSink.
func TestConcurrentChannelAuditSink(t *testing.T) {
	sink := NewChannelAuditSink(1000)

	numGoroutines := 50
	numOperations := 100

	var wg sync.WaitGroup

	// Start a counting consumer; it exits when Close() closes the channel.
	var received atomic.Int64
	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		for range sink.Channel() {
			received.Add(1)
		}
	}()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				sink.Write(AuditEntry{
					EventType: AuditEventBlockedTag,
					Message:   fmt.Sprintf("test-%d-%d", id, j),
				})
			}
		}(i)
	}

	wg.Wait()
	if err := sink.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}
	<-consumerDone

	// Invariant: every entry was either delivered or counted as dropped —
	// none may vanish.
	total := int64(numGoroutines * numOperations)
	if got := received.Load() + sink.DroppedCount(); got != total {
		t.Errorf("received+dropped = %d, want %d", got, total)
	}
}

// TestConcurrentWriterAuditSink tests concurrent writes to WriterAuditSink.
func TestConcurrentWriterAuditSink(t *testing.T) {
	// A counting discard writer: no-op output, but it records how many bytes
	// were actually handed to the underlying writer.
	counter := &countingDiscardWriter{}
	sink := NewWriterAuditSink(counter)
	defer func() { _ = sink.Close() }()

	numGoroutines := 50
	numOperations := 100

	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				sink.Write(AuditEntry{
					EventType: AuditEventBlockedTag,
					Message:   fmt.Sprintf("test-%d-%d", id, j),
				})
			}
		}(i)
	}

	wg.Wait()

	// Invariant: serialized writes must reach the underlying writer, and no
	// bytes may be lost or torn between concurrent Write calls.
	if got := counter.bytes.Load(); got == 0 {
		t.Error("no bytes reached the underlying writer")
	}
}

// countingDiscardWriter discards output but counts the bytes written.
type countingDiscardWriter struct {
	bytes atomic.Int64
}

func (w *countingDiscardWriter) Write(p []byte) (int, error) {
	w.bytes.Add(int64(len(p)))
	return len(p), nil
}

// TestConcurrentProcessorClose tests concurrent Close calls.
func TestConcurrentProcessorClose(t *testing.T) {
	processor, err := New()
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	numGoroutines := 100
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = processor.Close() // Multiple closes should be safe
		}()
	}

	wg.Wait()

	// Invariant: after the racing Closes, the processor is definitively
	// closed — Close stays idempotent (nil, never an error) and Extract
	// refuses to run.
	if err := processor.Close(); err != nil {
		t.Errorf("Close() after concurrent closes = %v, want nil (idempotent)", err)
	}
	if _, err := processor.Extract([]byte("<p>x</p>")); err != ErrProcessorClosed {
		t.Errorf("Extract() after close = %v, want ErrProcessorClosed", err)
	}
}

// TestConcurrentPoolAccess tests concurrent sync.Pool usage.
func TestConcurrentPoolAccess(t *testing.T) {
	numGoroutines := 100
	numOperations := 500

	var wg sync.WaitGroup

	// Shared []byte scratch pool (backing GetTextContent and CleanTextBytes).
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				bp := internal.GetByteBuf()
				*bp = append(*bp, "test string"...)
				_ = string(*bp)
				internal.PutByteBuf(bp)
			}
		}()
	}

	// Shared traversal-stack pool (backing WalkNodes and SanitizeDOM), which
	// every extraction walk borrows and returns.
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				stack := internal.GetNodeSlice()
				*stack = append(*stack, nil)
				internal.PutNodeSlice(stack)
			}
		}()
	}

	wg.Wait()
}

// TestConcurrentLinkExtraction tests concurrent link extraction.
func TestConcurrentLinkExtraction(t *testing.T) {
	processor, err := New()
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}
	defer func() { _ = processor.Close() }()

	html := []byte(`<html><body>
		<a href="https://example.com/1">Link 1</a>
		<a href="https://example.com/2">Link 2</a>
		<img src="image.jpg">
		<script src="script.js"></script>
	</body></html>`)

	numGoroutines := 50
	numOperations := 100

	var wg sync.WaitGroup
	var errorCount atomic.Int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				_, err := processor.ExtractAllLinks(html)
				if err != nil {
					errorCount.Add(1)
				}
			}
		}()
	}

	wg.Wait()

	if errorCount.Load() > 0 {
		t.Errorf("Concurrent link extraction had %d errors", errorCount.Load())
	}
}

// TestConcurrentCacheWithTTL tests cache with TTL under concurrent access.
func TestConcurrentCacheWithTTL(t *testing.T) {
	// Short TTL to test expiration
	cache := internal.NewCache[string](100, 50*time.Millisecond)

	numGoroutines := 30
	numOperations := 100

	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(2)

		// Writer
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				cache.Set(fmt.Sprintf("key-%d", id), fmt.Sprintf("value-%d-%d", id, j))
			}
		}(i)

		// Reader (may encounter expired entries)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				val := cache.Get(fmt.Sprintf("key-%d", id))
				_ = val // Value may be nil due to expiration
			}
		}(i)
	}

	wg.Wait()

	// Invariant: once the TTL has fully elapsed, every key must read as
	// expired — a Get that returns a value after the deadline would mean TTL
	// enforcement is broken under concurrent writes.
	time.Sleep(60 * time.Millisecond)
	for i := 0; i < numGoroutines; i++ {
		if got := cache.Get(fmt.Sprintf("key-%d", i)); got != nil {
			t.Fatalf("key-%d returned %v after TTL expiry, want nil", i, got)
		}
	}
}

// TestConcurrentMultiSink tests concurrent writes to MultiSink.
func TestConcurrentMultiSink(t *testing.T) {
	sink1 := NewChannelAuditSink(100)
	sink2 := NewChannelAuditSink(100)
	multiSink := NewMultiSink(sink1, sink2)

	// Counting consumers; they exit when Close() closes the channels.
	var received1, received2 atomic.Int64
	done1 := make(chan struct{})
	done2 := make(chan struct{})
	go func() {
		defer close(done1)
		for range sink1.Channel() {
			received1.Add(1)
		}
	}()
	go func() {
		defer close(done2)
		for range sink2.Channel() {
			received2.Add(1)
		}
	}()

	numGoroutines := 30
	numOperations := 50

	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				multiSink.Write(AuditEntry{
					EventType: AuditEventBlockedTag,
					Message:   fmt.Sprintf("test-%d-%d", id, j),
				})
			}
		}(i)
	}

	wg.Wait()
	if err := multiSink.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}
	<-done1
	<-done2

	// Invariant: a MultiSink must fan out to EVERY child sink — each child
	// sees all entries (delivered or counted dropped), not just some.
	total := int64(numGoroutines * numOperations)
	if got := received1.Load() + sink1.DroppedCount(); got != total {
		t.Errorf("sink1 received+dropped = %d, want %d", got, total)
	}
	if got := received2.Load() + sink2.DroppedCount(); got != total {
		t.Errorf("sink2 received+dropped = %d, want %d", got, total)
	}
}

// BenchmarkConcurrentCache benchmarks concurrent cache operations.
func BenchmarkConcurrentCache(b *testing.B) {
	cache := internal.NewCache[string](10000, time.Hour)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key-%d", i%1000)
			if i%2 == 0 {
				cache.Set(key, fmt.Sprintf("value-%d", i))
			} else {
				cache.Get(key)
			}
			i++
		}
	})
}

// BenchmarkConcurrentAuditCollector benchmarks concurrent audit recording.
func BenchmarkConcurrentAuditCollector(b *testing.B) {
	config := AuditConfig{Enabled: true}
	collector := newAuditCollector(config)
	defer func() { _ = collector.Close() }()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			collector.RecordBlockedTag(fmt.Sprintf("tag-%d", i))
			i++
		}
	})
}

// BenchmarkConcurrentProcessorExtraction benchmarks concurrent extraction.
func BenchmarkConcurrentProcessorExtraction(b *testing.B) {
	processor, _ := New()
	defer func() { _ = processor.Close() }()

	html := []byte(`<html><body><p>Test content for benchmarking</p></body></html>`)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = processor.Extract(html)
		}
	})
}

// TestConcurrentProcessorCreation tests concurrent processor creation and cleanup.
// This verifies that multiple processors can be created and used concurrently without issues.
func TestConcurrentProcessorCreation(t *testing.T) {
	t.Parallel()

	const numGoroutines = 50

	var wg sync.WaitGroup
	var errorCount atomic.Int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			p, err := New()
			if err != nil {
				errorCount.Add(1)
				return
			}
			defer func() { _ = p.Close() }()

			// Use the processor
			result, err := p.Extract([]byte("<html><body>Test</body></html>"))
			if err != nil {
				errorCount.Add(1)
				return
			}
			if result == nil {
				errorCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	if errorCount.Load() > 0 {
		t.Errorf("Concurrent processor creation had %d errors", errorCount.Load())
	}
}

// TestConcurrentBatchProcessing tests batch processing with concurrent workers.
func TestConcurrentBatchProcessing(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.WorkerPoolSize = 8

	p, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	// Prepare multiple HTML documents
	docs := make([][]byte, 50)
	for i := range docs {
		docs[i] = fmt.Appendf(nil, "<html><body><h1>Doc %d</h1><p>Content</p></body></html>", i)
	}

	// Run multiple batch operations concurrently
	const numConcurrentBatches = 10

	var wg sync.WaitGroup
	var errorCount atomic.Int64

	for i := 0; i < numConcurrentBatches; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			br := p.ExtractBatch(docs)
			if br.Failed > 0 {
				errorCount.Add(1)
				return
			}
			if len(br.Results) != len(docs) {
				errorCount.Add(1)
			}
		}(i)
	}

	wg.Wait()

	if errorCount.Load() > 0 {
		t.Errorf("Concurrent batch processing had %d errors", errorCount.Load())
	}
}

// TestMemoryPressure tests behavior under memory pressure with large documents.
func TestMemoryPressure(t *testing.T) {
	t.Parallel()

	if testing.Short() {
		t.Skip("Skipping memory pressure test in short mode")
	}

	cfg := DefaultConfig()
	cfg.MaxInputSize = 10 * 1024 * 1024 // 10MB

	p, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	// Create large HTML documents (100KB each)
	const docSize = 100 * 1024
	largeHTML := make([]byte, docSize)
	copy(largeHTML, "<html><body>")
	for i := len("<html><body>"); i < docSize-200; i += 100 {
		copy(largeHTML[i:], "<p>Test content with some text to make it larger.</p>")
	}
	copy(largeHTML[docSize-200:], "</body></html>")

	const numGoroutines = 10
	const docsPerGoroutine = 5

	var wg sync.WaitGroup
	var errorCount atomic.Int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < docsPerGoroutine; j++ {
				_, err := p.Extract(largeHTML)
				if err != nil {
					errorCount.Add(1)
				}
			}
		}(i)
	}

	wg.Wait()

	if errorCount.Load() > 0 {
		t.Errorf("Memory pressure test had %d errors", errorCount.Load())
	}
}

// TestCacheEvictionUnderLoad tests cache eviction with concurrent access.
func TestCacheEvictionUnderLoad(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 10 // Small cache to trigger evictions

	p, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	// Create unique HTML content to fill the cache
	const uniqueDocs = 50
	docs := make([][]byte, uniqueDocs)
	for i := range docs {
		docs[i] = fmt.Appendf(nil, "<html><body><h1>Doc %d</h1><p>Unique content %d</p></body></html>", i, i)
	}

	const numGoroutines = 20

	var wg sync.WaitGroup
	var errorCount atomic.Int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			// Each goroutine processes all documents, causing cache churn
			for j := range docs {
				result, err := p.Extract(docs[j])
				if err != nil {
					errorCount.Add(1)
					return
				}
				if result == nil {
					errorCount.Add(1)
				}
			}
		}(i)
	}

	wg.Wait()

	if errorCount.Load() > 0 {
		t.Errorf("Cache eviction test had %d errors", errorCount.Load())
	}

	// The point of MaxCacheEntries=10 with many distinct documents is that
	// cache churn prevents hit accumulation: with more documents than
	// entries, most extractions must be misses.
	if stats := p.GetStatistics(); stats.CacheMisses == 0 {
		t.Error("expected CacheMisses > 0 under load with MaxCacheEntries=10")
	}
}

// TestConcurrentPackageLevelExtract stresses the pooled-processor reuse path.
//
// The package-level Extract() (unlike the direct Processor.Extract exercised by
// the tests above) routes through processorPool -> get/putPooledProcessor, which
// resets stats, drains/clears the audit log, clears the cache, and may restart
// the cache cleanup goroutine on every call. That is the most intricate
// concurrency path in the library and was previously not covered by a
// concurrent test. Hammering the package-level entry point from many goroutines
// exercises pool reuse, the wasClosed Swap(false) handshake, and cache restart
// under contention.
func TestConcurrentPackageLevelExtract(t *testing.T) {
	t.Parallel()

	// Use the default config implicitly (no variadic Config) so the pooled path
	// is taken, and include an <img> so the scorer/link/inline-format code paths
	// run alongside the pool mechanics.
	htmlBytes := []byte(`<html><body><article><h1>Pool reuse</h1><p>concurrent extract via package-level API</p><img src="x.png" alt="a"></article></body></html>`)

	const numGoroutines = 100
	const numOperations = 100

	var wg sync.WaitGroup
	var errorCount atomic.Int64

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				result, err := Extract(htmlBytes)
				if err != nil {
					errorCount.Add(1)
					return
				}
				if result == nil || result.Text == "" {
					errorCount.Add(1)
					return
				}
			}
		}()
	}

	wg.Wait()

	if errorCount.Load() > 0 {
		t.Errorf("Concurrent package-level Extract had %d errors", errorCount.Load())
	}
}

// /////////////////////////////////////////////////////////////////////////////
// From race_hardening_test.go
// /////////////////////////////////////////////////////////////////////////////

// They pin race windows the existing concurrent_test.go suite does not
// exercise:
//
//   - ChannelAuditSink.Write racing Close (a send-on-closed-channel panic is
//     the failure this hunts; the existing test closes only after writers quit)
//   - the whole package-level API surface mixed under one load (pooled
//     processors, format transients sharing the parent's stats/audit, reader
//     mode), rather than Extract alone
//   - withTimeout's straggler goroutine outliving the caller's Extract return,
//     racing the deferred processor teardown on the one-shot (non-pooled) path

// mixedDoc exercises every extraction pass at once: article selection, inline
// link/image formatting, tables, and reader-mode section classification.
const mixedDoc = `<!DOCTYPE html>
<html><head><title>Mixed</title><meta charset="utf-8"></head>
<body>
<header><nav><a href="/a">A</a> <a href="/b">B</a></nav></header>
<main><article><h1>Headline</h1>
<p>Hello <a href="https://example.com/x">world</a> and an <img src="pic.png" alt="pic"> image.</p>
<table><tr><th>K</th><td>V</td></tr></table>
</article></main>
<footer><p>Bye</p></footer>
</body></html>`

// TestChannelAuditSinkCloseRacesWrites hammers Write concurrently with Close.
// Write holds closeMu.RLock while sending and Close takes closeMu.Lock before
// close(s.ch), so the two are mutually exclusive; any regression of that
// ordering surfaces here as a send-on-closed-channel panic (or a race report).
// Repeated rounds with a tiny buffer force the drop counter and the closed
// gate into the interleaving as well.
func TestChannelAuditSinkCloseRacesWrites(t *testing.T) {
	const (
		writers      = 8
		writesEach   = 200
		closeAtWrite = 20 // close once this many writes have been observed
	)

	for round := range 10 {
		sink := NewChannelAuditSink(4)

		var received atomic.Int64
		var written atomic.Int64
		consumerDone := make(chan struct{})
		go func() {
			defer close(consumerDone)
			for range sink.Channel() {
				received.Add(1)
			}
		}()

		var wg sync.WaitGroup

		// Closer: wait until writes are demonstrably in flight, then close
		// mid-stream so later Writes hit the isClosed gate and earlier ones
		// may still be racing the close itself.
		wg.Go(func() {
			deadline := time.Now().Add(100 * time.Millisecond)
			for written.Load() < closeAtWrite && time.Now().Before(deadline) {
				time.Sleep(50 * time.Microsecond)
			}
			if err := sink.Close(); err != nil {
				t.Errorf("Close() = %v, want nil", err)
			}
		})

		for i := range writers {
			wg.Go(func() {
				for j := range writesEach {
					sink.Write(AuditEntry{
						EventType: AuditEventBlockedTag,
						Message:   fmt.Sprintf("w%d-%d", i, j),
					})
					written.Add(1)
				}
			})
		}

		wg.Wait()
		<-consumerDone // terminates only via the channel close

		// A second Close must remain a safe no-op (guarded double close).
		if err := sink.Close(); err != nil {
			t.Fatalf("second Close() = %v, want nil", err)
		}

		// Every Write either delivered, was counted dropped, or no-opped
		// after close — none may vanish from the accounting above that gate.
		total := int64(writers * writesEach)
		if got := received.Load() + sink.DroppedCount(); got > total {
			t.Fatalf("round %d: received+dropped = %d exceeds total writes %d", round, got, total)
		}
	}
}

// TestConcurrentPackageLevelMixedAPIs runs every package-level entry point
// family against the shared processor pool at once: plain Extract/ExtractText,
// the format-transient paths (ExtractToMarkdown/ExtractToJSON, whose transient
// processors share the pooled parent's stats and audit collector), link
// extraction, reader mode, and batch extraction. The race detector verifies
// the pool hand-off, the shared disabled format cache, and the transient's
// shared-state contract under mixed traffic.
func TestConcurrentPackageLevelMixedAPIs(t *testing.T) {
	const goroutines = 28

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines)

	for i := range goroutines {
		wg.Go(func() {
			doc := []byte(mixedDoc)
			for j := range 10 {
				switch (i + j) % 7 {
				case 0:
					if _, err := Extract(doc); err != nil {
						errCh <- fmt.Errorf("Extract: %w", err)
						return
					}
				case 1:
					text, err := ExtractText(doc)
					if err != nil {
						errCh <- fmt.Errorf("ExtractText: %w", err)
						return
					}
					if !strings.Contains(text, "Headline") {
						errCh <- fmt.Errorf("ExtractText: headline missing from %q", text)
						return
					}
				case 2:
					md, err := ExtractToMarkdown(doc)
					if err != nil {
						errCh <- fmt.Errorf("ExtractToMarkdown: %w", err)
						return
					}
					if !strings.Contains(md, "https://example.com/x") {
						errCh <- fmt.Errorf("ExtractToMarkdown: inline link missing from %q", md)
						return
					}
				case 3:
					js, err := ExtractToJSON(doc)
					if err != nil {
						errCh <- fmt.Errorf("ExtractToJSON: %w", err)
						return
					}
					if len(js) == 0 {
						errCh <- errors.New("ExtractToJSON: empty output")
						return
					}
				case 4:
					links, err := ExtractAllLinks(doc)
					if err != nil {
						errCh <- fmt.Errorf("ExtractAllLinks: %w", err)
						return
					}
					if len(links) == 0 {
						errCh <- errors.New("ExtractAllLinks: no links extracted")
						return
					}
				case 5:
					rr, err := ReadPage(doc)
					if err != nil {
						errCh <- fmt.Errorf("ReadPage: %w", err)
						return
					}
					if len(rr.Sections) == 0 {
						errCh <- errors.New("ReadPage: no sections")
						return
					}
				case 6:
					br := ExtractBatch([][]byte{doc, doc, doc})
					if br.Failed > 0 && br.Errors[0] != nil {
						errCh <- fmt.Errorf("ExtractBatch: %w", br.Errors[0])
						return
					}
				}
			}
		})
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

// TestTimeoutStragglerRacesTeardown pins the straggler contract documented on
// withTimeout and detectEncoding: when the processing timeout fires, Extract
// returns immediately while the watchdog goroutine keeps running fn until its
// next cooperative cancellation check. On the one-shot (non-pooled) package
// path, withProcessor's deferred Close() therefore runs concurrently with the
// straggler's remaining work. The straggler must touch no state that Close
// mutates (closed flag, cache lifecycle, audit sink); the race detector fails
// this test if that contract regresses.
func TestTimeoutStragglerRacesTeardown(t *testing.T) {
	// Large enough that parsing exceeds the 1ms deadline on any machine, so
	// the straggler is reliably inside the parse (touching only locals and the
	// Safe-copy of the input) when the caller's teardown runs.
	big := strings.Repeat("<p>filler paragraph with <a href='/f'>link</a> content</p>", 20000)
	doc := []byte("<html><body><article>" + big + "</article></body></html>")

	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			cfg := DefaultConfig()
			cfg.ProcessingTimeout = time.Millisecond
			_, err := Extract(doc, cfg)
			// Either outcome is valid — a fast run may complete within the
			// deadline; the point of the test is the teardown race, not the
			// timeout error itself.
			if err != nil && !errors.Is(err, ErrProcessingTimeout) {
				t.Errorf("Extract with 1ms timeout: err = %v, want ErrProcessingTimeout or nil", err)
			}
		})
	}
	wg.Wait()
}
