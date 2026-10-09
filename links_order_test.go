package html

import "testing"

// TestExtractAllLinksOrderDeterministic guards against the previous
// map-iteration-based ordering of ExtractAllLinks, which returned links in a
// randomized order on every call. The result must be stable across repeated
// calls on identical input.
func TestExtractAllLinksOrderDeterministic(t *testing.T) {
	t.Parallel()

	const htmlContent = `<html><body>
		<a href="https://example.com/a">A</a>
		<a href="https://example.com/b">B</a>
		<a href="https://example.com/c">C</a>
		<img src="https://example.com/img/d.png">
		<link rel="stylesheet" href="https://example.com/e.css">
		<script src="https://example.com/f.js"></script>
	</body></html>`

	p, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = p.Close() }()

	first, err := p.ExtractAllLinks([]byte(htmlContent))
	if err != nil {
		t.Fatalf("ExtractAllLinks() failed: %v", err)
	}
	if len(first) < 2 {
		t.Fatalf("expected several links, got %d", len(first))
	}

	for run := 1; run < 20; run++ {
		got, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() run %d failed: %v", run, err)
		}
		if len(got) != len(first) {
			t.Fatalf("run %d: link count changed %d -> %d", run, len(first), len(got))
		}
		for i := range got {
			if got[i].URL != first[i].URL {
				t.Fatalf("run %d: non-deterministic order at index %d: %q vs %q",
					run, i, got[i].URL, first[i].URL)
			}
		}
	}
}
