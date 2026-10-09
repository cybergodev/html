// Package html provides resource leak detection tests.
// These tests verify that goroutines, memory, and other resources are properly cleaned up.
package html

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestGoroutineLeakInBatchProcessing tests that batch processing doesn't leak goroutines.
func TestGoroutineLeakInBatchProcessing(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 100
	cfg.CacheTTL = time.Minute
	processor, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}
	defer func() { _ = processor.Close() }()

	html := []byte(`<html><body><p>Test content</p></body></html>`)
	contents := make([][]byte, 100)
	for i := range contents {
		contents[i] = html
	}

	// Get initial goroutine count
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d", initialGoroutines)

	// Run batch extraction multiple times
	for i := 0; i < 10; i++ {
		br := processor.ExtractBatch(contents)
		if br.Failed > 0 {
			t.Fatalf("Batch extraction failed: %v", br.Errors[0])
		}
	}

	// Allow goroutines to settle
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	t.Logf("Final goroutines: %d", finalGoroutines)

	// Allow some tolerance for background goroutines
	if finalGoroutines > initialGoroutines+5 {
		t.Errorf("Potential goroutine leak: initial=%d, final=%d", initialGoroutines, finalGoroutines)
	}
}

// TestGoroutineLeakInBatchWithContext tests that batch processing with context doesn't leak goroutines.
func TestGoroutineLeakInBatchWithContext(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 100
	cfg.CacheTTL = time.Minute
	processor, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}
	defer func() { _ = processor.Close() }()

	html := []byte(`<html><body><p>Test content</p></body></html>`)
	contents := make([][]byte, 100)
	for i := range contents {
		contents[i] = html
	}

	// Get initial goroutine count
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d", initialGoroutines)

	// Run batch extraction with context multiple times
	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_ = processor.ExtractBatchWithContext(ctx, contents)
		cancel()
	}

	// Allow goroutines to settle
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	t.Logf("Final goroutines: %d", finalGoroutines)

	if finalGoroutines > initialGoroutines+5 {
		t.Errorf("Potential goroutine leak: initial=%d, final=%d", initialGoroutines, finalGoroutines)
	}
}

// TestGoroutineLeakInCancelledContext tests goroutine cleanup when context is cancelled.
func TestGoroutineLeakInCancelledContext(t *testing.T) {
	processor, err := New()
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}
	defer func() { _ = processor.Close() }()

	// Large content slice to process
	contents := make([][]byte, 1000)
	for i := range contents {
		contents[i] = []byte(`<html><body><p>Test</p></body></html>`)
	}

	// Get initial goroutine count
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d", initialGoroutines)

	// Start batch with short timeout - will cancel many goroutines
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	_ = processor.ExtractBatchWithContext(ctx, contents)
	cancel()

	// Allow goroutines to settle
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	t.Logf("Final goroutines after cancellation: %d", finalGoroutines)

	if finalGoroutines > initialGoroutines+10 {
		t.Errorf("Potential goroutine leak after cancellation: initial=%d, final=%d", initialGoroutines, finalGoroutines)
	}
}

// TestGoroutineLeakInAuditCollector tests that audit collector properly cleans up goroutines.
func TestGoroutineLeakInAuditCollector(t *testing.T) {
	config := AuditConfig{
		Enabled:           true,
		LogBlockedTags:    true,
		LogBlockedAttrs:   true,
		LogBlockedURLs:    true,
		IncludeRawValues:  true,
		MaxRawValueLength: 100,
	}

	// Get initial goroutine count
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d", initialGoroutines)

	// Create and use multiple collectors
	for i := 0; i < 10; i++ {
		collector := newAuditCollector(config)
		for j := 0; j < 100; j++ {
			collector.RecordBlockedTag("script")
			collector.RecordBlockedAttr("onclick", "alert(1)")
			collector.RecordBlockedURL("javascript:alert(1)", "xss")
		}
		_ = collector.Close()
	}

	// Allow goroutines to settle
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	t.Logf("Final goroutines: %d", finalGoroutines)

	if finalGoroutines > initialGoroutines+2 {
		t.Errorf("Potential goroutine leak in audit collector: initial=%d, final=%d", initialGoroutines, finalGoroutines)
	}
}

// TestGoroutineLeakInChannelAuditSink tests ChannelAuditSink goroutine cleanup.
func TestGoroutineLeakInChannelAuditSink(t *testing.T) {
	// Get initial goroutine count
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d", initialGoroutines)

	// Create sinks with consumers
	for i := 0; i < 5; i++ {
		sink := NewChannelAuditSink(100)

		// Start consumer goroutine
		consumerDone := make(chan struct{})
		go func() {
			defer close(consumerDone)
			for range sink.Channel() {
				// Drain channel
			}
		}()

		// Write some entries
		for j := 0; j < 50; j++ {
			sink.Write(AuditEntry{EventType: AuditEventBlockedTag, Message: "test"})
		}

		// Close sink - this should close the channel and stop the consumer
		_ = sink.Close()

		// Wait for consumer to finish
		select {
		case <-consumerDone:
			// Consumer finished properly
		case <-time.After(time.Second):
			t.Error("Consumer goroutine did not finish after sink close")
		}
	}

	// Allow goroutines to settle
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	t.Logf("Final goroutines: %d", finalGoroutines)

	if finalGoroutines > initialGoroutines+2 {
		t.Errorf("Potential goroutine leak in channel sink: initial=%d, final=%d", initialGoroutines, finalGoroutines)
	}
}

// TestMemoryLeakInCache tests that cache doesn't cause memory leaks.
func TestMemoryLeakInCache(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 1000
	cfg.CacheTTL = time.Minute
	processor, err := New(cfg)
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}
	defer func() { _ = processor.Close() }()

	// Generate unique HTML content to bypass cache
	baseTime := time.Now()
	generateHTML := func(id int) []byte {
		return []byte(`<html><body><p>Unique content ` + baseTime.String() + ` ID:` + string(rune('0'+id)) + `</p></body></html>`)
	}

	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// Process many unique documents
	for i := 0; i < 10000; i++ {
		_, _ = processor.Extract(generateHTML(i))
	}

	runtime.GC()
	runtime.ReadMemStats(&m2)

	// Check memory growth (allowing some growth but not unbounded)
	heapGrowth := int64(m2.HeapAlloc) - int64(m1.HeapAlloc)
	t.Logf("Heap growth: %d bytes (%.2f MB)", heapGrowth, float64(heapGrowth)/(1024*1024))

	// Memory should not grow unboundedly (allow up to 50MB growth for this test)
	if heapGrowth > 50*1024*1024 {
		t.Errorf("Potential memory leak: heap grew by %d bytes", heapGrowth)
	}
}

// TestMemoryLeakInProcessorClose tests that Close() properly releases resources.
func TestMemoryLeakInProcessorClose(t *testing.T) {
	var m1, m2 runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m1)

	// Create and close many processors
	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 1000
	cfg.CacheTTL = time.Minute

	for i := 0; i < 100; i++ {
		processor, err := New(cfg)
		if err != nil {
			t.Fatalf("Failed to create processor: %v", err)
		}

		// Use the processor
		for j := 0; j < 100; j++ {
			_, _ = processor.Extract([]byte(`<html><body><p>Test</p></body></html>`))
		}

		// Close the processor
		if err := processor.Close(); err != nil {
			t.Fatalf("Failed to close processor: %v", err)
		}
	}

	runtime.GC()
	runtime.ReadMemStats(&m2)

	heapGrowth := int64(m2.HeapAlloc) - int64(m1.HeapAlloc)
	t.Logf("Heap growth after processor cycles: %d bytes", heapGrowth)

	// Memory should not grow significantly after proper Close() calls
	if heapGrowth > 10*1024*1024 {
		t.Errorf("Potential memory leak: heap grew by %d bytes after processor cycles", heapGrowth)
	}
}

// TestWithTimeoutGoroutineCleanup tests that withTimeout properly cleans up goroutines.
// This test verifies that when timeout occurs, the background goroutine is eventually cleaned up.
//
// IMPORTANT: The withTimeout function has a known limitation - when timeout occurs,
// the background goroutine continues running until fn() completes. This test ensures
// that after fn() completes, goroutines are properly cleaned up.
func TestWithTimeoutGoroutineCleanup(t *testing.T) {
	// Get initial goroutine count
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()
	t.Logf("Initial goroutines: %d", initialGoroutines)

	// Run many quick operations - these should NOT timeout
	cfg := DefaultConfig()
	cfg.ProcessingTimeout = 5 * time.Second

	for i := 0; i < 50; i++ {
		processor, err := New(cfg)
		if err != nil {
			t.Fatalf("Failed to create processor: %v", err)
		}

		// Small content that completes quickly
		smallHTML := []byte(`<html><body><p>Test</p></body></html>`)

		_, err = processor.Extract(smallHTML)
		_ = processor.Close()

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
		}
	}

	// Allow goroutines to settle
	runtime.GC()
	time.Sleep(200 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	t.Logf("Final goroutines after quick ops: %d", finalGoroutines)

	// Allow some tolerance for GC and timing variations
	if finalGoroutines > initialGoroutines+5 {
		t.Errorf("Potential goroutine leak after quick operations: initial=%d, final=%d", initialGoroutines, finalGoroutines)
	}
}

// TestProcessorDoubleClose tests that double Close() calls are safe.
func TestProcessorDoubleClose(t *testing.T) {
	processor, err := New()
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	// First close
	err = processor.Close()
	if err != nil {
		t.Errorf("First close failed: %v", err)
	}

	// Second close should be safe
	err = processor.Close()
	if err != nil {
		t.Errorf("Second close failed: %v", err)
	}

	// Third close should also be safe
	err = processor.Close()
	if err != nil {
		t.Errorf("Third close failed: %v", err)
	}
}

// TestConcurrentCloseAndExtract tests safety of concurrent Close() and Extract() calls.
func TestConcurrentCloseAndExtract(t *testing.T) {
	processor, err := New()
	if err != nil {
		t.Fatalf("Failed to create processor: %v", err)
	}

	var extractOk atomic.Bool
	extractOk.Store(true)

	var wg sync.WaitGroup

	// Start multiple goroutines that extract
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			_, err := processor.Extract([]byte(`<html><body><p>Test</p></body></html>`))
			if err != nil && err != ErrProcessorClosed {
				extractOk.Store(false)
			}
			time.Sleep(time.Millisecond)
		}
	}()

	// Start multiple goroutines that close
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			_ = processor.Close()
			time.Sleep(5 * time.Millisecond)
		}
	}()

	// Wait for completion
	wg.Wait()

	if !extractOk.Load() {
		t.Error("Extract operations returned unexpected errors (not ErrProcessorClosed)")
	}
}

// TestMultiSinkCloseGoroutineLeak tests that MultiSink properly closes all underlying sinks
// and doesn't leak consumer goroutines.
func TestMultiSinkCloseGoroutineLeak(t *testing.T) {
	// Get initial goroutine count
	runtime.GC()
	time.Sleep(10 * time.Millisecond)
	initialGoroutines := runtime.NumGoroutine()

	// Create multiple channel sinks
	sinks := make([]AuditSink, 5)
	for i := range sinks {
		sinks[i] = NewChannelAuditSink(100)
	}

	multiSink := NewMultiSink(sinks...)

	// Start consumers for all channel sinks
	var consumerWg sync.WaitGroup
	for _, s := range sinks {
		sink := s.(*ChannelAuditSink)
		consumerWg.Add(1)
		go func() {
			defer consumerWg.Done()
			for range sink.Channel() {
			}
		}()
	}

	// Write some entries
	for i := 0; i < 100; i++ {
		multiSink.Write(AuditEntry{EventType: AuditEventBlockedTag})
	}

	// Close multi sink
	err := multiSink.Close()
	if err != nil {
		t.Errorf("MultiSink.Close() failed: %v", err)
	}

	// Wait for consumers to finish
	consumerWg.Wait()

	// Allow goroutines to settle
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	finalGoroutines := runtime.NumGoroutine()
	t.Logf("Goroutines - initial: %d, final: %d", initialGoroutines, finalGoroutines)

	// All consumer goroutines should have stopped
	if finalGoroutines > initialGoroutines+2 {
		t.Errorf("Potential goroutine leak in MultiSink: initial=%d, final=%d", initialGoroutines, finalGoroutines)
	}
}

// BenchmarkProcessorCreationWithClose benchmarks processor creation and close cycle.
func BenchmarkProcessorCreationWithClose(b *testing.B) {
	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 100
	cfg.CacheTTL = time.Minute
	for b.Loop() {
		processor, _ := New(cfg)
		_, _ = processor.Extract([]byte(`<html><body><p>Test</p></body></html>`))
		_ = processor.Close()
	}
}

// BenchmarkBatchProcessingMemory benchmarks memory usage during batch processing.
func BenchmarkBatchProcessingMemory(b *testing.B) {
	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 100
	cfg.CacheTTL = time.Minute
	processor, _ := New(cfg)
	defer func() { _ = processor.Close() }()

	contents := make([][]byte, 100)
	for i := range contents {
		contents[i] = []byte(`<html><body><p>Test content</p></body></html>`)
	}

	b.ResetTimer()
	for b.Loop() {
		_ = processor.ExtractBatch(contents)
	}
}
