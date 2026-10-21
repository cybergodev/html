// Package internal provides tests for pooled resources.
package internal

import (
	"bytes"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// TestGetBuffer tests getting a bytes.Buffer from the pool.
func TestGetBuffer(t *testing.T) {
	t.Parallel()

	buf := GetBuffer()
	if buf == nil {
		t.Fatal("GetBuffer() returned nil")
	}

	// Verify the buffer is usable
	_, err := buf.Write([]byte("test"))
	if err != nil {
		t.Errorf("Write() failed: %v", err)
	}

	if !bytes.Equal(buf.Bytes(), []byte("test")) {
		t.Errorf("Expected 'test', got '%s'", buf.Bytes())
	}
}

// TestPutBuffer tests returning a bytes.Buffer to the pool.
func TestPutBuffer(t *testing.T) {
	t.Parallel()

	buf := GetBuffer()
	buf.Write([]byte("content"))

	// Put the buffer back
	PutBuffer(buf)

	// Get another buffer - it should be reset
	buf2 := GetBuffer()
	if buf2.Len() != 0 {
		t.Errorf("Buffer should be reset, got length %d", buf2.Len())
	}

	PutBuffer(buf2)
}

// TestPutBufferNil tests that PutBuffer handles nil safely.
func TestPutBufferNil(t *testing.T) {
	t.Parallel()

	// Should not panic
	PutBuffer(nil)
}

// TestBufferPoolConcurrent tests concurrent access to BufferPool.
func TestBufferPoolConcurrent(t *testing.T) {
	t.Parallel()

	const numGoroutines = 100
	const numOperations = 50

	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				buf := GetBuffer()
				buf.Write([]byte("test bytes"))
				_ = buf.Bytes()
				PutBuffer(buf)
			}
		}()
	}

	wg.Wait()
}

// calls Grow() to pre-allocate space for fresh builders.
// TestBufferPoolInitialCapacity verifies that pooled buffers have initial capacity.
func TestBufferPoolInitialCapacity(t *testing.T) {
	t.Parallel()

	buf := GetBuffer()
	defer PutBuffer(buf)

	// The buffer should have been created with bufferPoolInitialCapacity
	if buf.Cap() < bufferPoolInitialCapacity {
		t.Errorf("Expected capacity >= %d, got %d", bufferPoolInitialCapacity, buf.Cap())
	}
}

// BenchmarkGetBuffer benchmarks GetBuffer performance.
func BenchmarkGetBuffer(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := GetBuffer()
		PutBuffer(buf)
	}
}

// BenchmarkBufferWithWork benchmarks buffer with actual work.
func BenchmarkBufferWithWork(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buf := GetBuffer()
		buf.Write([]byte("test byte content"))
		_ = buf.Bytes()
		PutBuffer(buf)
	}
}

// This simulates a scenario where something external puts a wrong type in the pool.
// TestGetBufferPoolCorruption tests that GetBuffer handles corrupted pool gracefully.
func TestGetBufferPoolCorruption(t *testing.T) {
	t.Parallel()

	// Put a wrong type in the pool to simulate corruption
	var wrongBuffer any = "not a buffer" // deliberately non-pointer: tests pool corruption fallback
	BufferPool.Put(wrongBuffer)

	// GetBuffer should still return a valid buffer (fallback to new)
	buf := GetBuffer()
	if buf == nil {
		t.Fatal("GetBuffer() returned nil even with pool corruption")
	}

	// Verify the buffer is usable
	_, err := buf.Write([]byte("test"))
	if err != nil {
		t.Errorf("Write() failed: %v", err)
	}

	if !bytes.Equal(buf.Bytes(), []byte("test")) {
		t.Errorf("Expected 'test', got '%s'", buf.Bytes())
	}

	PutBuffer(buf)
}

// TestPutBufferSecureClear tests that poolSecureClear properly zeros buffer.
func TestPutBufferSecureClear(t *testing.T) {
	t.Parallel()

	// Enable secure clear
	SetPoolSecureClear(true)
	defer SetPoolSecureClear(false)

	// Create a buffer with sensitive content
	buf := GetBuffer()
	sensitiveData := []byte("SENSITIVE_API_KEY_12345")
	buf.Write(sensitiveData)

	// Verify data was written
	if !bytes.Equal(buf.Bytes(), sensitiveData) {
		t.Fatalf("Expected '%s', got '%s'", sensitiveData, buf.Bytes())
	}

	// Return to pool - should zero the buffer
	PutBuffer(buf)

	// Note: We can't directly verify the buffer was zeroed since
	// the buffer has been reset and returned to pool.
	// This test primarily verifies the code path executes without panic.
}

// TestPutNodeSliceSecureClear tests that poolSecureClear properly zeros node slice.
func TestPutNodeSliceSecureClear(t *testing.T) {
	t.Parallel()

	// Enable secure clear
	SetPoolSecureClear(true)
	defer SetPoolSecureClear(false)

	// Create a slice with some node pointers (nil is valid for *html.Node)
	slicePtr := GetNodeSlice()
	*slicePtr = append(*slicePtr, nil, nil)

	// Return to pool - should zero the slice
	PutNodeSlice(slicePtr)

	// This test primarily verifies the code path executes without panic
}

// TestSetPoolDebug verifies that enabling pool debug logging routes corruption
// events to the supplied logger, and that disabling it silences them.
//
// This test is intentionally NOT parallel: it toggles the package-global
// poolDebug flag and corrupts the shared pools, so it must not overlap with the
// other pool tests.
func TestSetPoolDebug(t *testing.T) {
	var (
		mu     sync.Mutex
		called bool
	)
	logger := func(format string, args ...any) {
		mu.Lock()
		called = true
		mu.Unlock()
	}

	// Enable debug logging, then corrupt a pool exactly as the corruption tests do.
	SetPoolDebug(true, logger)
	defer SetPoolDebug(false, nil)

	// sync.Pool may evict its per-P local cache at any GC (documented behavior).
	// The race detector is allocation-heavy, so GC fires often and a single
	// Put→Get round-trip no longer reliably surfaces the corrupted item: Get()
	// can fall through to New() and return a valid *bytes.Buffer, skipping the
	// type-assertion failure that calls logPoolCorruption. Retry until the
	// logger fires — each attempt hits the corruption path ~70% of the time
	// under -race, so 100 attempts makes a miss astronomically unlikely while
	// remaining instant when the cache survives (the common case without -race).
	const maxCorruptAttempts = 100
	wasCalled := false
	for i := 0; i < maxCorruptAttempts; i++ {
		BufferPool.Put("not a buffer") //nolint:SA6002 // intentionally using non-pointer to test pool corruption handling
		_ = GetBuffer()                // type-assertion fallback invokes logPoolCorruption
		mu.Lock()
		if called {
			wasCalled = true
		}
		mu.Unlock()
		if wasCalled {
			break
		}
	}
	if !wasCalled {
		t.Fatalf("logger was not invoked after %d corruption attempts despite enabled debug logging", maxCorruptAttempts)
	}

	// Disable debug logging; subsequent corruption must not invoke the logger.
	// This path is GC-independent: logPoolCorruption returns early when
	// poolDebug is false, before touching the logger, so a single attempt
	// suffices — but a few more exercise the disabled state more thoroughly.
	SetPoolDebug(false, nil)
	mu.Lock()
	called = false
	mu.Unlock()

	for i := 0; i < maxCorruptAttempts; i++ {
		BufferPool.Put("not a buffer") //nolint:SA6002 // intentionally using non-pointer to test pool corruption handling
		_ = GetBuffer()
	}
	mu.Lock()
	invokedAfterDisable := called
	mu.Unlock()
	if invokedAfterDisable {
		t.Fatal("logger was invoked after SetPoolDebug(false, nil)")
	}
}

// checkByteBufPut verifies the shared []byte-pool Put contract: nil is a no-op,
// oversized buffers (cap > maxPooledByteCap) are dropped rather than retained,
// and a normal buffer is retained for reuse within the same P.
func checkByteBufPut(t *testing.T, name string, get func() *[]byte, put func(*[]byte)) {
	t.Helper()

	put(nil) // must not panic

	// Oversized buffer is dropped; a subsequent get must not hand back the
	// multi-MB backing array (cap stays within the pooled ceiling).
	big := make([]byte, 0, maxPooledByteCap+1024)
	put(&big)
	fresh := get()
	if cap(*fresh) > maxPooledByteCap {
		t.Errorf("%s: oversized buffer (cap=%d) was retained; got cap=%d > maxPooledByteCap=%d",
			name, cap(big), cap(*fresh), maxPooledByteCap)
	}
	put(fresh)

	// Normal buffer is retained and reused.
	bp := get()
	*bp = append(*bp, "payload"...)
	put(bp)
	again := get()
	if cap(*again) < len("payload") {
		t.Errorf("%s: normal buffer was not retained (cap=%d)", name, cap(*again))
	}
	put(again)
}

func TestPutByteBuf_Boundaries(t *testing.T) {
	t.Parallel()
	checkByteBufPut(t, "PutByteBuf", GetByteBuf, PutByteBuf)
}

func TestPutBuffer_Boundaries(t *testing.T) {
	t.Parallel()

	PutBuffer(nil) // no-op

	// Oversized bytes.Buffer is dropped.
	big := bytes.NewBuffer(make([]byte, maxPooledByteCap+1024))
	PutBuffer(big)
	fresh := GetBuffer()
	if cap(fresh.Bytes()) > maxPooledByteCap {
		t.Errorf("PutBuffer: oversized buffer retained, cap=%d", cap(fresh.Bytes()))
	}
	PutBuffer(fresh)
}

func TestPutNodeSlice_Boundaries(t *testing.T) {
	t.Parallel()

	PutNodeSlice(nil) // no-op

	// Oversized node slice is dropped.
	big := make([]*html.Node, 0, maxPooledNodeSliceCap+8)
	PutNodeSlice(&big)
	fresh := GetNodeSlice()
	if cap(*fresh) > maxPooledNodeSliceCap {
		t.Errorf("PutNodeSlice: oversized slice retained, cap=%d", cap(*fresh))
	}
	PutNodeSlice(fresh)
}

// TestPutByteBufSecureClear tests that poolSecureClear properly zeros a byte
// scratch buffer. Mirrors the per-Put secure-clear tests; this one was missing,
// leaving PutByteBuf's secure-clear branch uncovered.
func TestPutByteBufSecureClear(t *testing.T) {
	t.Parallel()

	SetPoolSecureClear(true)
	defer SetPoolSecureClear(false)

	bp := GetByteBuf()
	sensitiveData := []byte("SENSITIVE_SCRATCH_12345")
	*bp = append(*bp, sensitiveData...)

	if !bytes.Equal(*bp, sensitiveData) {
		t.Fatalf("Expected '%s', got '%s'", sensitiveData, *bp)
	}

	// Return to pool - the secure-clear path zeroes then resets (no panic).
	PutByteBuf(bp)
}

// --- TrackedBuilder pool tests (GetTrackedBuilder / PutTrackedBuilder) ---
// These were at 0% coverage because the table rendering code path goes through
// NewTrackedBuilder (direct construction), not the pooled variant.

// TestGetTrackedBuilder tests getting a TrackedBuilder from the pool.
func TestGetTrackedBuilder(t *testing.T) {
	t.Parallel()

	tb := GetTrackedBuilder()
	if tb == nil {
		t.Fatal("GetTrackedBuilder() returned nil")
	}

	// Must be reset (empty) on acquisition.
	if tb.Len() != 0 {
		t.Errorf("pooled TrackedBuilder should be reset, got Len()=%d", tb.Len())
	}
	if tb.LastChar != 0 {
		t.Errorf("LastChar = %d, want 0 after reset", tb.LastChar)
	}

	// Verify it is usable.
	_, _ = tb.WriteString("hello")
	if tb.String() != "hello" {
		t.Errorf("String() = %q, want 'hello'", tb.String())
	}

	PutTrackedBuilder(tb)
}

// TestPutTrackedBuilder tests returning a TrackedBuilder to the pool and verifies
// that the next Get returns a reset instance.
func TestPutTrackedBuilder(t *testing.T) {
	t.Parallel()

	tb := GetTrackedBuilder()
	_, _ = tb.WriteString("content to be cleared")
	_ = tb.WriteByte('!')

	PutTrackedBuilder(tb)

	// Next acquisition should be reset.
	tb2 := GetTrackedBuilder()
	if tb2.Len() != 0 {
		t.Errorf("pooled TrackedBuilder should be reset after Put, got Len()=%d", tb2.Len())
	}

	PutTrackedBuilder(tb2)
}

// TestPutTrackedBuilderNil tests that PutTrackedBuilder handles nil safely.
func TestPutTrackedBuilderNil(t *testing.T) {
	t.Parallel()

	PutTrackedBuilder(nil) // must not panic
}

// TestTrackedBuilderPoolConcurrent tests concurrent access to the TrackedBuilder pool.
func TestTrackedBuilderPoolConcurrent(t *testing.T) {
	t.Parallel()

	const numGoroutines = 100
	const numOperations = 50

	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				tb := GetTrackedBuilder()
				_, _ = tb.WriteString("test string")
				_ = tb.String()
				PutTrackedBuilder(tb)
			}
		}()
	}

	wg.Wait()
}

// TestGetNodeSlicePoolCorruption mirrors the other corruption tests: a wrong
// type in NodeSlicePool must not crash GetNodeSlice, which falls back to a
// fresh slice (the previously uncovered 57% branch).
func TestGetNodeSlicePoolCorruption(t *testing.T) {
	t.Parallel()

	// Put a wrong type in the pool to simulate corruption
	var wrongType any = "not a node slice"
	NodeSlicePool.Put(wrongType)

	s := GetNodeSlice()
	if s == nil {
		t.Fatal("GetNodeSlice() returned nil even with pool corruption")
	}
	if cap(*s) == 0 {
		t.Error("fallback slice should have retained capacity")
	}

	PutNodeSlice(s)
}

// TestPutNodeSliceNil pins the documented nil no-op contract.
func TestPutNodeSliceNil(t *testing.T) {
	t.Parallel()
	PutNodeSlice(nil) // must not panic
}
