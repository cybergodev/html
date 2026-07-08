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

// TestGetPooledProcessorSafe_RecoversPoolNewPanic verifies the helper converts
// a panicking pool.New into ErrInternalPanic, preserving the panic value.
func TestGetPooledProcessorSafe_RecoversPoolNewPanic(t *testing.T) {
	withPanickingPool(t)

	p, err := getPooledProcessorSafe()
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

	t.Run("Extract", func(t *testing.T) {
		if _, err := Extract(htmlBytes); !errors.Is(err, ErrInternalPanic) {
			t.Fatalf("expected ErrInternalPanic, got: %v", err)
		}
	})

	t.Run("ExtractText", func(t *testing.T) {
		if _, err := ExtractText(htmlBytes); !errors.Is(err, ErrInternalPanic) {
			t.Fatalf("expected ErrInternalPanic, got: %v", err)
		}
	})

	t.Run("ExtractToMarkdown", func(t *testing.T) {
		if _, err := ExtractToMarkdown(htmlBytes); !errors.Is(err, ErrInternalPanic) {
			t.Fatalf("expected ErrInternalPanic, got: %v", err)
		}
	})

	t.Run("ExtractToJSON", func(t *testing.T) {
		if _, err := ExtractToJSON(htmlBytes); !errors.Is(err, ErrInternalPanic) {
			t.Fatalf("expected ErrInternalPanic, got: %v", err)
		}
	})

	t.Run("ExtractAllLinks", func(t *testing.T) {
		if _, err := ExtractAllLinks(htmlBytes); !errors.Is(err, ErrInternalPanic) {
			t.Fatalf("expected ErrInternalPanic, got: %v", err)
		}
	})

	t.Run("ExtractBatch", func(t *testing.T) {
		br := ExtractBatch([][]byte{htmlBytes, htmlBytes})
		if br.Failed != 2 {
			t.Fatalf("expected 2 failures, got %d (successes=%d)", br.Failed, br.Success)
		}
		if br.Success != 0 {
			t.Fatalf("expected 0 successes, got %d", br.Success)
		}
		for i, e := range br.Errors {
			if !errors.Is(e, ErrInternalPanic) {
				t.Errorf("item %d: expected ErrInternalPanic, got: %v", i, e)
			}
		}
	})
}
