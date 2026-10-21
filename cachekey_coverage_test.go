package html

// cachekey_coverage_test.go — regression for D-002 C-1: generateCacheKey
// sampled only five ~819-byte windows of documents larger than 64KB, making
// the key blind to every byte outside those windows. Two same-length
// documents differing only in the unsampled middle collided, so the second
// extraction was served the first document's cached Result.

import (
	"strings"
	"testing"
)

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
