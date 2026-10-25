package html

// cache_regression_test.go — regressions for the processor cache, gathered
// from the former single-topic files (cache_aliasing_test.go,
// cachekey_coverage_test.go):
//
//   - ownership: on a cache miss the returned *Result must not alias the
//     cache entry, so a caller mutating its result cannot corrupt entries a
//     concurrent cache hit would observe;
//   - key quality (D-002 C-1): generateCacheKey once sampled only five
//     ~819-byte windows of documents larger than 64KB, so two same-length
//     documents differing only in the unsampled middle collided and the
//     second extraction was served the first document's cached Result.

import (
	"strings"
	"sync"
	"testing"
)

// ---------------------------------------------------------------------------
// Ownership — returned Results must not alias the cache
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Cache-key quality — every byte region participates in the key
// ---------------------------------------------------------------------------

// TestCacheKeyDistinguishesUnsampledMiddle pins the property the old sampling
// scheme violated: changing bytes anywhere in a large document must produce a
// different key.
func TestCacheKeyDistinguishesUnsampledMiddle(t *testing.T) {
	p, err := New(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	const size = 150000
	base := []byte(strings.Repeat("a", size))
	// 80000 sits between the old scheme's sampled windows (0..819,
	// ~37.5KB, ~75KB, ~112.5KB, and the tail).
	mutated := append([]byte(nil), base...)
	copy(mutated[80000:], "ZZZZ")

	if p.generateCacheKey(base) == p.generateCacheKey(mutated) {
		t.Fatal("cache key is blind to bytes in the document middle")
	}
}

// TestCacheKeyEveryRegion participates in the key: flipping a marker across
// the whole document must change the key at every position.
func TestCacheKeyEveryRegion(t *testing.T) {
	p, err := New(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	const size = 200000
	base := []byte(strings.Repeat("q", size))
	want := p.generateCacheKey(base)

	for pos := 0; pos+4 <= size; pos += 9973 {
		mutated := append([]byte(nil), base...)
		copy(mutated[pos:], "MARK")
		if p.generateCacheKey(mutated) == want {
			t.Fatalf("cache key unchanged after mutating offset %d", pos)
		}
	}
}

// TestExtractLargeDocsWithSameLengthMiddleDiff is the end-to-end guard: a
// second document that differs only in the middle of a >64KB body must be
// extracted afresh, never answered from the first document's cache entry.
func TestExtractLargeDocsWithSameLengthMiddleDiff(t *testing.T) {
	p, err := New(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	prefix := "<html><body><p>"
	pad := strings.Repeat("a", 150000)
	docA := prefix + pad
	docB := prefix + pad
	const markerAt = 80000
	docA = docA[:markerAt] + "AAAA" + docA[markerAt+4:]
	docB = docB[:markerAt] + "BBBB" + docB[markerAt+4:]
	if len(docA) != len(docB) {
		t.Fatalf("test documents differ in length: %d vs %d", len(docA), len(docB))
	}

	resA, err := p.Extract([]byte(docA))
	if err != nil {
		t.Fatal(err)
	}
	resB, err := p.Extract([]byte(docB))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(resA.Text, "AAAA") {
		t.Fatalf("first document lost its marker: %.80q", resA.Text)
	}
	if strings.Contains(resB.Text, "AAAA") || !strings.Contains(resB.Text, "BBBB") {
		t.Fatalf("second document was served the first document's cached result: %.80q", resB.Text)
	}
	if hits := p.GetStatistics().CacheHits; hits != 0 {
		t.Fatalf("CacheHits = %d, want 0 (documents differ)", hits)
	}
}

// TestCacheKeyDistinguishesSameContentDifferentLength keeps the length mixing
// contract: appending a byte must change the key.
func TestCacheKeyDistinguishesSameContentDifferentLength(t *testing.T) {
	p, err := New(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	short := []byte(strings.Repeat("a", 70000))
	long := append(append([]byte(nil), short...), 'b')
	if p.generateCacheKey(short) == p.generateCacheKey(long) {
		t.Fatal("cache key does not distinguish content length")
	}
}
