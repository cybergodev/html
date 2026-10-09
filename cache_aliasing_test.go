package html

import (
	"sync"
	"testing"
)

// TestCacheMissResultNotAliasedWithCache guards the ownership contract between
// a returned *Result and the processor cache.
//
// On a cache miss, the freshly built result is stored in the cache. The value
// returned to the caller must be an independent copy: cache hits already deep
// clone via cloneResult to avoid aliasing, so the miss path must not hand back
// the same pointer the cache holds. Otherwise a caller that mutates its result
// races with concurrent cache-hit reads and silently corrupts the cache.
//
// This reproduces the defect deterministically without the race detector:
// mutate the first returned result, then confirm a cache hit does not observe
// the mutation.
func TestCacheMissResultNotAliasedWithCache(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 16
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	htmlBytes := []byte("<html><body><p>hello world</p></body></html>")

	r1, err := p.Extract(htmlBytes)
	if err != nil {
		t.Fatalf("first Extract: %v", err)
	}

	// The caller owns r1 and is free to mutate it.
	r1.Text = "MUTATED-BY-CALLER"
	r1.Title = "MUTATED-TITLE"

	// Second call hits the cache and must return an independent copy.
	r2, err := p.Extract(htmlBytes)
	if err != nil {
		t.Fatalf("second Extract: %v", err)
	}

	if r2.Text == "MUTATED-BY-CALLER" || r2.Title == "MUTATED-TITLE" {
		t.Fatalf("cache miss returned a *Result aliased with the cache entry; "+
			"caller mutation leaked into cached value (Text=%q Title=%q)",
			r2.Text, r2.Title)
	}
}

// TestConcurrentExtractResultOwnership stresses the same contract: many
// goroutines extract the same document and mutate their own result. Run under
// -race (where the race detector is available) to catch any residual aliasing.
func TestConcurrentExtractResultOwnership(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 64
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	htmlBytes := []byte("<html><body><p>concurrent ownership</p></body></html>")

	var wg sync.WaitGroup
	for range 64 {
		wg.Go(func() {
			r, err := p.Extract(htmlBytes)
			if err != nil {
				return
			}
			// Caller mutates its own result.
			r.Text = "x"
			r.WordCount = 1
		})
	}
	wg.Wait()
}
