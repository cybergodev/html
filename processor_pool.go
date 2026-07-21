package html

import (
	"fmt"
	"sync"
)

// poolCfg is the configuration used by the package-level processor pool. It is
// identical to DefaultConfig except caching is fully disabled.
//
// Pooled processors are returned to the pool via putPooledProcessor, which
// calls ClearCache on every return, so a pooled processor always begins an
// extraction with an empty cache. With caching enabled, every package-level
// extraction would therefore pay the cache-key hash plus a map insert/remove
// that can never pay off (Get always misses), and New would additionally start
// a background cleanup goroutine sweeping a cache that is always empty. Zeroing
// MaxCacheEntries short-circuits the cache in Extract (no key generation, no
// Get/Set), and zeroing CacheTTL/CacheCleanup prevents the cleanup goroutine
// from starting. Callers that benefit from caching construct their own
// Processor via New, which honors their Config.
var poolCfg = func() Config {
	c := DefaultConfig()
	c.MaxCacheEntries = 0
	c.CacheTTL = 0
	c.CacheCleanup = 0
	return c
}()

// processorPool is a sync.Pool for Processor instances.
// Used by package-level functions to reduce allocation overhead.
var processorPool = sync.Pool{
	New: func() any {
		// poolCfg is derived from DefaultConfig and is valid by construction, so
		// New cannot fail here. If it ever does, that is a library invariant
		// violation (like regexp.MustCompile on a compile-time pattern): fail
		// fast rather than return a half-constructed Processor. The previous
		// fallback hand-built a Processor with nil stats/scorer/audit, which
		// would have nil-dereferenced on the very next Extract.
		p, err := New(poolCfg)
		if err != nil {
			panic(fmt.Sprintf("html: default processor config failed validation: %v", err))
		}
		return p
	},
}

// getPooledProcessor gets a Processor from the pool.
// Call putPooledProcessor when done to return it to the pool.
// Includes type check with fallback to prevent panic if pool is corrupted.
func getPooledProcessor() *Processor {
	v := processorPool.Get()
	p, ok := v.(*Processor)
	if !ok {
		// Pool corruption detected: create a new processor as fallback. This
		// builds a fully initialized Processor (New sets stats/scorer/audit),
		// unlike the old pool.New fallback which hand-built an incomplete one.
		p, _ = New(poolCfg)
	}
	return p
}

// getPooledProcessorSafe returns a pooled Processor, converting a panic from
// processorPool.New into an ErrInternalPanic error.
//
// processorPool.New panics only when poolCfg fails validation — a library
// invariant violation, since poolCfg is derived from DefaultConfig() and is
// valid by construction (see the comment on processorPool). That panic sits on
// the public-API path: every package-level function reaches the pool through
// withProcessor/withProcessorBatch. Per SEC-003 (no panic escapes the public
// API), this wrapper recovers such a panic and returns it wrapped in
// ErrInternalPanic, preserving the original value in the message. sync.Pool
// does not cache the result of a panicking New, so a later Get retries fresh
// rather than handing out a poisoned entry.
func getPooledProcessorSafe() (p *Processor, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: processor pool initialization failed: %v", ErrInternalPanic, r)
		}
	}()
	p = getPooledProcessor()
	return p, nil
}

// putPooledProcessor returns a Processor to the pool.
// The processor's statistics, audit log, and cache are reset before returning
// to prevent memory accumulation from cached results across pool uses.
func putPooledProcessor(p *Processor) {
	if p == nil {
		return
	}
	// If the processor was closed during this use, discard it instead of
	// resurrecting it. Pooled processors are never closed on the normal path
	// (withProcessor Close()s only the non-pooled branch), so this only fires
	// under misuse — and a closed processor must not be handed back out: its
	// cache cleanup goroutine has been stopped and its audit sink closed.
	// Previously this code did closed.Swap(false) to un-close the processor and
	// then RestartCleanup, but that branch was dead on every real path (poolCfg
	// zeroes CacheTTL/CacheCleanup) and silently broke Close's "do not reuse"
	// contract. sync.Pool tolerates the missing Put; its New rebuilds on next Get.
	if p.closed.Load() {
		return
	}
	p.ResetStatistics()
	// Sink writes are synchronous, so by the time the previous user's Extract
	// returned, every audit entry was already handed to its sink. Wait() is a
	// no-op safety hook kept here to mark the "audit work for this use is done"
	// point before clearing entries and returning the processor to the pool.
	if p.audit != nil {
		p.audit.Wait()
	}
	p.ClearAuditLog()
	p.ClearCache()
	processorPool.Put(p)
}

// resolveConfig resolves the configuration from optional variadic Config parameter.
// Returns the resolved config, a boolean indicating whether to use the processor pool,
// and an error. When no config is provided, returns DefaultConfig() with pooled=true.
// When one config is provided, returns it with pooled=false.
// Returns ErrMultipleConfigs if more than one config is provided.
func resolveConfig(cfg ...Config) (Config, bool, error) {
	switch len(cfg) {
	case 0:
		return DefaultConfig(), true, nil
	case 1:
		return cfg[0], false, nil
	default:
		return Config{}, false, ErrMultipleConfigs
	}
}

// withProcessor executes a function with a processor.
// When pooled is true, reuses a pooled processor (DefaultConfig) for efficiency.
// When pooled is false, creates a temporary processor with the given config.
func withProcessor[T any](pooled bool, cfg Config, fn func(*Processor) (T, error)) (T, error) {
	if pooled {
		p, err := getPooledProcessorSafe()
		if err != nil {
			var zero T
			return zero, err
		}
		defer putPooledProcessor(p)
		return fn(p)
	}
	p, err := New(cfg)
	if err != nil {
		var zero T
		return zero, err
	}
	defer func() { _ = p.Close() }()
	return fn(p)
}

// withProcessorBatch executes a batch function with a processor.
// On processor creation failure, returns a BatchResult with all items marked as failed.
func withProcessorBatch(pooled bool, cfg Config, itemCount int, fn func(*Processor) *BatchResult) *BatchResult {
	var p *Processor
	if pooled {
		var err error
		p, err = getPooledProcessorSafe()
		if err != nil {
			// Propagate the invariant failure to every item, mirroring the
			// New-failure path in the else branch below.
			return uniformErrorBatch(itemCount, err)
		}
		defer putPooledProcessor(p)
	} else {
		var err error
		p, err = New(cfg)
		if err != nil {
			return uniformErrorBatch(itemCount, err)
		}
		defer func() { _ = p.Close() }()
	}
	return fn(p)
}
