package html

// processor_pool_internal_test.go - SEC-003 regression coverage for the pooled
// processor path. Package-level functions (Extract, ExtractBatch, ...) reach
// the processor pool via withProcessor/withProcessorBatch, whose Get() invokes
// processorPool.New. That constructor panics only on a library-invariant
// violation (poolCfg failing validation), but it sits on the public-API path,
// so getPooledProcessorSafe must convert the panic into ErrInternalPanic.

import (
	"errors"
	"runtime"
	"strings"
	"testing"
)

// withPanickingPool swaps the global processorPool.New for one that always
// panics, draining any cached processors first so Get() is forced to invoke
// New. The original constructor is restored on cleanup. The test must not run
// in parallel with others while the swap is active, so the returned cleanup is
// deferred by the caller rather than relying on t.Parallel sequencing.
func withPanickingPool(t *testing.T) {
	t.Helper()
	orig := processorPool.New
	processorPool.New = func() any {
		panic("SEC-003: simulated pool invariant panic")
	}
	// sync.Pool drops all cached entries on GC, so this guarantees the next
	// Get() calls the (now-panicking) New rather than returning a warm entry.
	runtime.GC()
	t.Cleanup(func() {
		processorPool.New = orig
		// Drain again so a subsequent Get() rebuilds a valid processor with the
		// restored constructor rather than receiving a poisoned nil.
		runtime.GC()
	})
}

// drainPoolAndRetry calls fn until it reports the panicking-pool outcome.
// Parallel tests that use the package-level functions keep handing warm
// entries back to the shared processorPool, so a Get() issued right after a
// drain can still receive a warm entry and never invoke the swapped,
// panicking New. Draining (runtime.GC drops every cached entry) and retrying
// bounds that interference window instead of letting it flake the suite.
func drainPoolAndRetry(fn func() bool) bool {
	for range 100 {
		runtime.GC()
		if fn() {
			return true
		}
	}
	return false
}

// TestGetPooledProcessorSafe_RecoversPoolNewPanic verifies the helper converts
// a panicking pool.New into ErrInternalPanic, preserving the panic value.
func TestGetPooledProcessorSafe_RecoversPoolNewPanic(t *testing.T) {
	withPanickingPool(t)

	var p *Processor
	var err error
	if !drainPoolAndRetry(func() bool {
		p, err = getPooledProcessorSafe()
		return err != nil
	}) {
		t.Fatal("panicking pool.New never surfaced (warm entries kept winning the race)")
	}
	if p != nil {
		t.Fatalf("expected nil processor on pool panic, got %+v", p)
	}
	if !errors.Is(err, ErrInternalPanic) {
		t.Fatalf("expected ErrInternalPanic, got: %v", err)
	}
	if !strings.Contains(err.Error(), "simulated pool invariant panic") {
		t.Fatalf("error should preserve original panic value, got: %v", err)
	}
}

// TestPackageFunctionsSurvivePoolPanic verifies the end-to-end SEC-003 guarantee
// for package-level entry points: with the pooled processor's constructor
// panicking, every public package-level function returns ErrInternalPanic
// instead of crashing the process. Covers both the single-result path
// (withProcessor) and the batch path (withProcessorBatch).
func TestPackageFunctionsSurvivePoolPanic(t *testing.T) {
	withPanickingPool(t)

	htmlBytes := []byte("<html><body><p>hi</p></body></html>")

	single := func(t *testing.T, fn func() error) {
		t.Helper()
		if !drainPoolAndRetry(func() bool {
			err := fn()
			return errors.Is(err, ErrInternalPanic)
		}) {
			t.Fatal("panicking pool.New never surfaced (warm entries kept winning the race)")
		}
	}

	t.Run("Extract", func(t *testing.T) {
		single(t, func() error { _, err := Extract(htmlBytes); return err })
	})

	t.Run("ExtractText", func(t *testing.T) {
		single(t, func() error { _, err := ExtractText(htmlBytes); return err })
	})

	t.Run("ExtractToMarkdown", func(t *testing.T) {
		single(t, func() error { _, err := ExtractToMarkdown(htmlBytes); return err })
	})

	t.Run("ExtractToJSON", func(t *testing.T) {
		single(t, func() error { _, err := ExtractToJSON(htmlBytes); return err })
	})

	t.Run("ExtractAllLinks", func(t *testing.T) {
		single(t, func() error { _, err := ExtractAllLinks(htmlBytes); return err })
	})

	t.Run("ExtractBatch", func(t *testing.T) {
		if !drainPoolAndRetry(func() bool {
			br := ExtractBatch([][]byte{htmlBytes, htmlBytes})
			if br.Failed != 2 || br.Success != 0 {
				return false
			}
			for _, e := range br.Errors {
				if !errors.Is(e, ErrInternalPanic) {
					return false
				}
			}
			return true
		}) {
			t.Fatal("panicking pool.New never surfaced (warm entries kept winning the race)")
		}
	})
}

// TestGetPooledProcessorTypeFallback covers getPooledProcessor's type-corruption
// fallback branch (processor_pool.go:50, previously uncovered). A wrong type in
// the pool must not crash the caller: Get falls back to building a fresh,
// fully-initialized Processor via New(poolCfg).
func TestGetPooledProcessorTypeFallback(t *testing.T) {
	// Not parallel: this corrupts the shared processorPool, so it must not
	// overlap with other tests (or the package-level convenience functions)
	// that Get from the same pool.
	processorPool.Put("not a processor") //nolint:staticcheck // intentionally wrong type: exercises the corruption path

	p := getPooledProcessor()
	if p == nil {
		t.Fatal("getPooledProcessor returned nil after pool corruption")
	}
	// A fallback Processor must be fully usable, not a half-built struct.
	if _, err := p.Extract([]byte("<html><body><p>hi</p></body></html>")); err != nil {
		t.Fatalf("fallback processor unusable: %v", err)
	}
	_ = p.Close()
}

// TestPutPooledProcessorGuards covers putPooledProcessor's degenerate inputs:
// nil is a no-op, and a Processor that was closed during its use is discarded
// rather than resurrected (its cache sweeper is stopped and its audit sink
// closed — handing it back out would violate Close's no-reuse contract).
func TestPutPooledProcessorGuards(t *testing.T) {
	t.Parallel()

	putPooledProcessor(nil) // documented no-op; must not panic

	// A closed processor must be dropped by the discard branch.
	p, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := p.Extract([]byte("<html><body><p>hi</p></body></html>")); err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	putPooledProcessor(p) // closed: discarded, never reused

	// A live processor takes the normal reset-and-return path.
	p2, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New(2): %v", err)
	}
	if _, err := p2.Extract([]byte("<html><body><p>hi</p></body></html>")); err != nil {
		t.Fatalf("Extract(2): %v", err)
	}
	putPooledProcessor(p2)
	if got := p2.GetStatistics().TotalProcessed; got != 0 {
		t.Errorf("returned processor kept statistics: TotalProcessed=%d, want 0 after reset", got)
	}
}
