// pool.go provides pooled resources for memory allocation optimization.
package internal

import (
	"bytes"
	"strings"
	"sync"
	"sync/atomic"

	"golang.org/x/net/html"

	"github.com/cybergodev/html/internal/table"
)

// Pool configuration constants
const (
	// builderPoolInitialCapacity is the initial capacity for pooled strings.Builder
	// Increased from 256 to 1024 to reduce reallocations for typical HTML content
	builderPoolInitialCapacity = 1024

	// bufferPoolInitialCapacity is the initial capacity for pooled bytes.Buffer
	bufferPoolInitialCapacity = 1024

	// maxPooledByteCap is the largest []byte/bytes.Buffer capacity that is returned
	// to a pool. Buffers that grew beyond this (e.g. a multi-MB extraction result)
	// are dropped on Put so the pool does not retain them indefinitely — the
	// classic sync.Pool retention footgun, where one oversized buffer is handed
	// back out for every subsequent small request and stays live per-P forever.
	// 64 KiB covers the hot paths (GetTextContent, charset transform chunks) while
	// shedding the pathological large-document buffers.
	maxPooledByteCap = 64 * 1024

	// maxPooledNodeSliceCap bounds retained []*html.Node slices (≈64 KiB of
	// pointers at 8 bytes each). Tree traversal stacks are normally tiny; a slice
	// this large indicates a pathologically wide document whose stack should not
	// be retained.
	maxPooledNodeSliceCap = 8192
)

// poolDebug enables debug logging for pool corruption detection.
// This is intentionally not exported - it's for internal debugging only.
// To enable, modify this variable to true before using the pool.
//
// Example (in test code):
//
//	internal.SetPoolDebug(true, log.Printf)
var poolDebug atomic.Bool

// poolSecureClear enables secure clearing of pooled objects before reuse.
// When enabled, buffers are zeroed before being returned to the pool,
// preventing potential data leakage from previous uses.
// This has a performance cost and is intended for security-sensitive environments.
var poolSecureClear atomic.Bool

// poolLogger is an optional logger for pool corruption warnings.
var poolLogger struct {
	mu     sync.Mutex
	logger func(format string, args ...any)
}

// SetPoolDebug enables or disables pool debug logging.
// When enabled, pool corruption events are logged to the provided logger.
// This function is intended for development and debugging purposes only.
//
// Note: This is an internal API and may change without notice.
func SetPoolDebug(enabled bool, logger func(format string, args ...any)) {
	poolLogger.mu.Lock()
	poolLogger.logger = logger
	poolLogger.mu.Unlock()
	poolDebug.Store(enabled)
}

// SetPoolSecureClear enables or disables secure clearing of pooled objects.
// When enabled, buffers are zeroed before being returned to the pool,
// preventing potential data leakage from previous uses.
//
// SECURITY: Enable this in security-sensitive environments where data
// isolation between operations is required.
//
// Performance: This option adds ~5-10% overhead to pool operations.
// Note: This is an internal API and may change without notice.
func SetPoolSecureClear(enabled bool) {
	poolSecureClear.Store(enabled)
}

// logPoolCorruption logs a pool corruption warning if debugging is enabled.
func logPoolCorruption(poolName, expectedType string, got any) {
	if !poolDebug.Load() {
		return
	}
	poolLogger.mu.Lock()
	defer poolLogger.mu.Unlock()
	if poolLogger.logger != nil {
		poolLogger.logger("POOL CORRUPTION: %s expected type %s but got %T", poolName, expectedType, got)
	}
}

// BuilderPool is a sync.Pool for strings.Builder instances.
// Use this for functions that build strings incrementally to reduce allocations.
//
// For most use cases, prefer the helper functions GetBuilder() and PutBuilder():
//
//	sb := internal.GetBuilder()
//	defer internal.PutBuilder(sb)
//	sb.Grow(estimatedSize)
//	// ... use sb ...
//	return sb.String()
//
// Direct pool access is also available for advanced use cases:
//
//	sbPtr := internal.BuilderPool.Get().(*strings.Builder)
//	sb := *sbPtr
//	defer func() {
//	    sb.Reset()
//	    internal.BuilderPool.Put(sbPtr)
//	}()
var BuilderPool = sync.Pool{
	New: func() any {
		sb := &strings.Builder{}
		sb.Grow(builderPoolInitialCapacity)
		return sb
	},
}

// BufferPool is a sync.Pool for bytes.Buffer instances.
// Use this for functions that work with byte slices to reduce allocations.
//
// For most use cases, prefer the helper functions GetBuffer() and PutBuffer():
//
//	buf := internal.GetBuffer()
//	defer internal.PutBuffer(buf)
//	buf.Grow(estimatedSize)
//	// ... use buf ...
//	return buf.Bytes()
//
// Direct pool access is also available for advanced use cases:
//
//	bufPtr := internal.BufferPool.Get().(*bytes.Buffer)
//	buf := *bufPtr
//	defer func() {
//	    buf.Reset()
//	    internal.BufferPool.Put(bufPtr)
//	}()
var BufferPool = sync.Pool{
	New: func() any {
		return bytes.NewBuffer(make([]byte, 0, bufferPoolInitialCapacity))
	},
}

// GetBuilder gets a strings.Builder from the pool.
// The returned builder has been reset and is ready for use.
// Call PutBuilder when done to return it to the pool.
//
// IMPORTANT: Callers MUST ensure PutBuilder is called even on error paths.
// Use defer immediately after GetBuilder to guarantee cleanup:
//
//	sb := internal.GetBuilder()
//	defer internal.PutBuilder(sb)
//	// ... use sb ...
//
// Failure to return the builder to the pool will not cause memory leaks
// (the GC will collect it), but will reduce the effectiveness of the pool.
func GetBuilder() *strings.Builder {
	v := BuilderPool.Get()
	sb, ok := v.(*strings.Builder)
	if !ok {
		// Log pool corruption if debugging is enabled
		logPoolCorruption("BuilderPool", "*strings.Builder", v)
		// Fallback: return a new builder if pool is corrupted
		sb = &strings.Builder{}
		sb.Grow(builderPoolInitialCapacity)
	}
	return sb
}

// PutBuilder returns a strings.Builder to the pool.
// The builder is reset before being returned to the pool.
// It is safe to call PutBuilder with a nil pointer (no-op).
//
// SECURITY: When poolSecureClear is enabled, the builder is NOT returned to
// the pool. Unlike bytes.Buffer, strings.Builder exposes no mutable accessor
// for its internal []byte (String() shares immutable memory, and the Write*
// methods only append), so the buffer cannot be zeroed in place without
// coupling to its unexported, version-specific layout via reflect/unsafe.
// Dropping the builder instead of repooling still honors the secure-clear
// intent: the builder and its buffer are never reused, so no prior content can
// leak into a later GetBuilder call. The cost is reduced pooling effectiveness,
// which is the accepted trade-off for an opt-in paranoid mode.
func PutBuilder(sb *strings.Builder) {
	if sb == nil {
		return
	}
	sb.Reset()
	if poolSecureClear.Load() {
		// Do not return to the pool; let the GC reclaim the builder and its
		// buffer so the sensitive content is not retained for reuse.
		return
	}
	BuilderPool.Put(sb)
}

// GetBuffer gets a bytes.Buffer from the pool.
// The returned buffer has been reset and is ready for use.
// Call PutBuffer when done to return it to the pool.
//
// IMPORTANT: Callers MUST ensure PutBuffer is called even on error paths.
// Use defer immediately after GetBuffer to guarantee cleanup:
//
//	buf := internal.GetBuffer()
//	defer internal.PutBuffer(buf)
//	// ... use buf ...
//
// Failure to return the buffer to the pool will not cause memory leaks
// (the GC will collect it), but will reduce the effectiveness of the pool.
func GetBuffer() *bytes.Buffer {
	v := BufferPool.Get()
	buf, ok := v.(*bytes.Buffer)
	if !ok {
		// Log pool corruption if debugging is enabled
		logPoolCorruption("BufferPool", "*bytes.Buffer", v)
		// Fallback: return a new buffer if pool is corrupted
		buf = bytes.NewBuffer(make([]byte, 0, bufferPoolInitialCapacity))
	}
	return buf
}

// PutBuffer returns a bytes.Buffer to the pool.
// The buffer is reset before being returned to the pool.
// It is safe to call PutBuffer with a nil pointer (no-op).
//
// SECURITY: When poolSecureClear is enabled, the buffer's internal data
// is zeroed before being returned to prevent data leakage.
func PutBuffer(buf *bytes.Buffer) {
	if buf == nil {
		return
	}
	// Drop oversized buffers so the pool does not retain a multi-MB backing array
	// indefinitely (see maxPooledByteCap rationale).
	if cap(buf.Bytes()) > maxPooledByteCap {
		return
	}
	// SECURITY: Optionally clear sensitive data before returning to pool
	if poolSecureClear.Load() {
		// Zero out the buffer contents before reset
		b := buf.Bytes()
		for i := range b {
			b[i] = 0
		}
	}
	buf.Reset()
	BufferPool.Put(buf)
}

// byteBufPool pools reusable []byte scratch buffers.
//
// Unlike BuilderPool, whose *strings.Builder loses its backing array on every
// Reset() (strings.Builder.Reset sets buf = nil), a pooled []byte retains its
// capacity across uses because it is reset with [:0]. On hot paths that build a
// string per call (e.g. GetTextContent, invoked once per <a> and per table cell),
// this avoids re-growing a buffer from zero on every call. The profiler showed
// GetTextContent's sb.Grow as the single largest allocator (~23% of bytes) for
// precisely this reason; pooling a retaining []byte removes it.
//
// Usage:
//
//	bp := internal.GetByteBuf()
//	defer internal.PutByteBuf(bp)
//	*bp = append(*bp, "..."...)
//	return string(*bp) // copies; safe because the buffer is reused after return
var byteBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, 0, 256)
		return &b
	},
}

// GetByteBuf returns a zero-length []byte backed by a capacity-retaining pooled
// buffer. Append to *bp and convert the result with string(*bp) before returning
// the buffer with PutByteBuf. The returned slice must not be retained past
// PutByteBuf (the backing array is reused).
func GetByteBuf() *[]byte {
	bp := byteBufPool.Get().(*[]byte)
	*bp = (*bp)[:0]
	return bp
}

// PutByteBuf returns a buffer obtained from GetByteBuf to the pool, retaining its
// capacity for reuse. Safe to call with nil.
func PutByteBuf(bp *[]byte) {
	if bp == nil {
		return
	}
	// Drop oversized buffers so the pool does not retain a multi-MB backing array
	// indefinitely (see maxPooledByteCap rationale).
	if cap(*bp) > maxPooledByteCap {
		return
	}
	*bp = (*bp)[:0]
	byteBufPool.Put(bp)
}

// trackedBuilderPool pools *table.TrackedBuilder instances for the text-extraction
// hot path. A TrackedBuilder is []byte-backed, so its Reset (buf[:0]) retains the
// backing capacity across uses — unlike BuilderPool's *strings.Builder, whose
// Reset nils the buffer. extractTextContent builds a document-length string per
// Extract() call, so reusing a buffer that is already large enough after warmup
// removes the per-call growth allocations and the GC churn they cause.
var trackedBuilderPool = sync.Pool{
	New: func() any { return table.NewTrackedBuilder() },
}

// GetTrackedBuilder returns a reset TrackedBuilder from the pool. Return it with
// PutTrackedBuilder. The returned buffer retains capacity across uses.
func GetTrackedBuilder() *table.TrackedBuilder {
	tb := trackedBuilderPool.Get().(*table.TrackedBuilder)
	tb.Reset()
	return tb
}

// PutTrackedBuilder returns a TrackedBuilder to the pool. Oversized buffers are
// dropped (see maxPooledByteCap) so a single huge document cannot make the pool
// retain a multi-MB backing array indefinitely.
func PutTrackedBuilder(tb *table.TrackedBuilder) {
	if tb == nil {
		return
	}
	if tb.Cap() > maxPooledByteCap {
		return
	}
	tb.Reset()
	trackedBuilderPool.Put(tb)
}

// TransformBufferPool is a sync.Pool for byte slices used in encoding transformation.
// These buffers are used for charset conversion operations.
var TransformBufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, 8192)
		return &buf
	},
}

// GetTransformBuffer gets a byte slice from the transform buffer pool.
// The returned slice has zero length but retained capacity.
func GetTransformBuffer() *[]byte {
	v := TransformBufferPool.Get()
	buf, ok := v.(*[]byte)
	if !ok {
		// Log pool corruption if debugging is enabled
		logPoolCorruption("TransformBufferPool", "*[]byte", v)
		// Fallback: return a new buffer if pool is corrupted
		newBuf := make([]byte, 0, 8192)
		return &newBuf
	}
	return buf
}

// PutTransformBuffer returns a byte slice to the transform buffer pool.
// The slice is reset to zero length before being returned.
// It is safe to call PutTransformBuffer with a nil pointer (no-op).
//
// SECURITY: When poolSecureClear is enabled, the buffer's data is zeroed
// before being returned to prevent data leakage.
func PutTransformBuffer(buf *[]byte) {
	if buf == nil {
		return
	}
	// Drop oversized buffers so the pool does not retain a multi-MB backing array
	// indefinitely (see maxPooledByteCap rationale).
	if cap(*buf) > maxPooledByteCap {
		return
	}
	// SECURITY: Optionally clear sensitive data before returning to pool
	if poolSecureClear.Load() {
		for i := range *buf {
			(*buf)[i] = 0
		}
	}
	*buf = (*buf)[:0]
	TransformBufferPool.Put(buf)
}

// NodeSlicePool is a sync.Pool for HTML node slices used in tree traversal.
// This reduces allocations in WalkNodes and similar traversal functions.
var NodeSlicePool = sync.Pool{
	New: func() any {
		s := make([]*html.Node, 0, 64)
		return &s
	},
}

// GetNodeSlice gets a node slice from the pool.
// The returned slice has zero length but retained capacity.
// Call PutNodeSlice when done to return it to the pool.
func GetNodeSlice() *[]*html.Node {
	v := NodeSlicePool.Get()
	s, ok := v.(*[]*html.Node)
	if !ok {
		logPoolCorruption("NodeSlicePool", "*[]*html.Node", v)
		newS := make([]*html.Node, 0, 64)
		return &newS
	}
	return s
}

// PutNodeSlice returns a node slice to the pool.
// The slice is reset to zero length before being returned.
// It is safe to call PutNodeSlice with a nil pointer (no-op).
//
// SECURITY: When poolSecureClear is enabled, the slice is zeroed
// before being returned to prevent potential pointer leakage.
func PutNodeSlice(s *[]*html.Node) {
	if s == nil {
		return
	}
	// Drop oversized slices so the pool does not retain a pathologically wide
	// traversal stack indefinitely (see maxPooledNodeSliceCap rationale).
	if cap(*s) > maxPooledNodeSliceCap {
		return
	}
	// SECURITY: Optionally clear node pointers to prevent potential
	// stale pointer access or information leakage
	if poolSecureClear.Load() {
		for i := range *s {
			(*s)[i] = nil
		}
	}
	*s = (*s)[:0]
	NodeSlicePool.Put(s)
}
