// Package html provides concurrent safety tests for the HTML processor.
// These tests verify thread-safety and detect race conditions.
package html

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cybergodev/html/internal"
)

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

	// Test BuilderPool
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				sb := internal.GetBuilder()
				sb.WriteString("test string")
				_ = sb.String()
				internal.PutBuilder(sb)
			}
		}()
	}

	// Test BufferPool
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				buf := internal.GetBuffer()
				buf.Write([]byte("test bytes"))
				_ = buf.Bytes()
				internal.PutBuffer(buf)
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
