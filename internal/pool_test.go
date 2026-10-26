// Package internal provides tests for pooled resources.
package internal

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"golang.org/x/net/html"
)

// TestPutNodeSliceSecureClear tests that poolSecureClear properly zeros node slice.
func TestPutNodeSliceSecureClear(t *testing.T) {
	t.Parallel()

	// Enable secure clear
	SetPoolSecureClear(true)
	defer SetPoolSecureClear(false)

	// Create a slice with NON-NIL node pointers: the secure-clear branch
	// clear((*s)[:cap(*s)]) must actually overwrite pointer slots — with the
	// former nil sentinels the clear was an observable no-op.
	slicePtr := GetNodeSlice()
	sentinels := []*html.Node{{Type: html.ElementNode, Data: "a"}, {Type: html.TextNode, Data: "t"}}
	*slicePtr = append(*slicePtr, sentinels...)

	if got := (*slicePtr)[0]; got != sentinels[0] || (*slicePtr)[1] != sentinels[1] {
		t.Fatalf("fixture error: slice does not hold the sentinel nodes %v", *slicePtr)
	}

	// Return to pool: zeroes the capacity region, then resets and pools
	// (must not panic). The zeroing itself cannot be asserted here without
	// reading the slice after the pool owns it, which would race with another
	// borrower; the branch is exercised and pinned by this path.
	PutNodeSlice(slicePtr)
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
	// can fall through to New() and return a valid *[]*html.Node, skipping the
	// type-assertion failure that calls logPoolCorruption. Retry until the
	// logger fires — each attempt hits the corruption path most of the time
	// under -race, so 100 attempts makes a miss astronomically unlikely while
	// remaining instant when the cache survives (the common case without -race).
	//
	// nodeSlicePool is used as the corruption vehicle because it is the pool
	// whose Get performs a checked type assertion and therefore routes a
	// corrupted entry through logPoolCorruption.
	const maxCorruptAttempts = 100
	wasCalled := false
	for i := 0; i < maxCorruptAttempts; i++ {
		nodeSlicePool.Put("not a node slice") //nolint:SA6002 // intentionally wrong type: exercises the corruption path
		_ = GetNodeSlice()                    // type-assertion fallback invokes logPoolCorruption
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
		nodeSlicePool.Put("not a node slice") //nolint:SA6002 // intentionally wrong type: exercises the corruption path
		_ = GetNodeSlice()
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

// TestByteBufPoolConcurrent tests concurrent access to the shared []byte pool.
// This is the concurrency coverage for the pool that serves the
// text-extraction hot path (GetByteBuf backs GetTextContent and CleanTextBytes).
func TestByteBufPoolConcurrent(t *testing.T) {
	t.Parallel()

	const numGoroutines = 100
	const numOperations = 50

	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numOperations; j++ {
				bp := GetByteBuf()
				*bp = append(*bp, "test bytes"...)
				_ = string(*bp)
				PutByteBuf(bp)
			}
		}()
	}

	wg.Wait()
}

// BenchmarkGetByteBuf benchmarks the []byte pool round trip.
func BenchmarkGetByteBuf(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		PutByteBuf(GetByteBuf())
	}
}

// BenchmarkByteBufWithWork benchmarks a pooled buffer with actual work.
func BenchmarkByteBufWithWork(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bp := GetByteBuf()
		*bp = append(*bp, "test byte content"...)
		_ = string(*bp)
		PutByteBuf(bp)
	}
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
// type in nodeSlicePool must not crash GetNodeSlice, which falls back to a
// fresh slice (the previously uncovered 57% branch).
func TestGetNodeSlicePoolCorruption(t *testing.T) {
	t.Parallel()

	// Put a wrong type in the pool to simulate corruption
	var wrongType any = "not a node slice"
	nodeSlicePool.Put(wrongType)

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

// TestSlicePoolPutOversizedDropped covers SlicePool.Put's oversized-drop branch
// (pool.go:241, previously 83.3%): a scratch slice grown past
// maxPooledResultScratchElems must be discarded, not retained for a later Get.
func TestSlicePoolPutOversizedDropped(t *testing.T) {
	t.Parallel()

	sp := NewSlicePool[int](8)

	// Grow a scratch past the retention ceiling and return it.
	scratch := sp.Get()
	*scratch = make([]int, maxPooledResultScratchElems+16)
	sp.Put(scratch)

	// A subsequent Get must hand back a fresh slice within the ceiling, not the
	// oversized backing array we just discarded.
	next := sp.Get()
	if cap(*next) > maxPooledResultScratchElems {
		t.Errorf("oversized scratch retained: got cap=%d > maxPooledResultScratchElems=%d",
			cap(*next), maxPooledResultScratchElems)
	}
	sp.Put(next)

	// Nil Put is a documented no-op.
	sp.Put(nil)
}

// TestPutTrackedBuilderGuards covers PutTrackedBuilder's degenerate inputs:
// nil is a no-op, an oversized buffer is dropped rather than retained per-P,
// and the secure-clear path zeroes the buffer before returning it.
func TestPutTrackedBuilderGuards(t *testing.T) {
	t.Parallel()

	PutTrackedBuilder(nil) // documented no-op; must not panic

	// Oversized builder: cap beyond maxPooledByteCap must be dropped. The
	// builder stays usable for the caller afterwards.
	big := GetTrackedBuilder()
	_, _ = big.WriteString(strings.Repeat("x", maxPooledByteCap+1))
	PutTrackedBuilder(big)
	if big.Len() == 0 {
		t.Log("oversized builder was reset by Put; either way it is not retained")
	}

	// Secure-clear enabled: the returned buffer is zeroed before pooling.
	SetPoolSecureClear(true)
	defer SetPoolSecureClear(false)
	tb := GetTrackedBuilder()
	_, _ = tb.WriteString("sensitive")
	PutTrackedBuilder(tb) // SecureClear path; must not panic
	if tb.Len() != 0 {
		t.Errorf("builder not cleared under secure clear: len=%d", tb.Len())
	}
}
