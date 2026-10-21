package html_test

// fragment_link_test.go — pins the documented contract that page-internal
// navigation (fragment-only and query-only hrefs) produces no link entry.
// IsValidURL rejects "#section" and "?page=2", and the doc comments on
// Result.Links, LinkInfo and LinkResource now state that explicitly: reporting
// them would require resolving them against the document base, which invents
// a URL the markup never contained.

import (
	"strings"
	"testing"

	"github.com/cybergodev/html"
)

func TestFragmentAndQueryHrefsExcluded(t *testing.T) {
	t.Parallel()

	doc := []byte(`<html><body>
		<a href="#top">fragment</a>
		<a href="?page=2">query</a>
		<a href="#">empty fragment</a>
		<a href="/page.html">real link</a>
		<a href="https://example.com/x">absolute</a>
	</body></html>`)

	cfg := html.DefaultConfig()
	cfg.MaxCacheEntries = 0

	res, err := html.Extract(doc, cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	for _, l := range res.Links {
		switch l.URL {
		case "#top", "?page=2", "#":
			t.Errorf("Result.Links contains page-internal navigation %q", l.URL)
		}
	}
	if len(res.Links) != 2 {
		t.Fatalf("Result.Links = %d entries, want 2 (the two real destinations): %+v", len(res.Links), res.Links)
	}

	links, err := html.ExtractAllLinks(doc, cfg)
	if err != nil {
		t.Fatalf("ExtractAllLinks: %v", err)
	}
	for _, l := range links {
		switch l.URL {
		case "#top", "?page=2", "#":
			t.Errorf("ExtractAllLinks contains page-internal navigation %q", l.URL)
		}
	}
}

// TestFragmentHrefStillConsumesPosition documents the counter contract: a
// rejected href still consumes its position number, so the [LINK:n]
// placeholders emitted into Result.Text stay aligned with Result.Links.
func TestFragmentHrefStillConsumesPosition(t *testing.T) {
	t.Parallel()

	doc := []byte(`<html><body><p>` +
		`<a href="#frag">first</a> ` +
		`<a href="/real.html">second</a>` +
		`</p></body></html>`)

	cfg := html.DefaultConfig()
	cfg.MaxCacheEntries = 0
	cfg.InlineLinkFormat = "markdown"

	res, err := html.Extract(doc, cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(res.Links) != 1 {
		t.Fatalf("Result.Links = %d, want 1: %+v", len(res.Links), res.Links)
	}
	// The surviving link was emitted second in the document, so it keeps
	// position 2 — and its markdown must still carry its own URL, not the
	// fragment's.
	if res.Links[0].Position != 2 {
		t.Errorf("link Position = %d, want 2 (rejected href still consumes a number)", res.Links[0].Position)
	}
	if !strings.Contains(res.Text, "[second](/real.html)") {
		t.Errorf("markdown output lost the link: %q", res.Text)
	}
	if strings.Contains(res.Text, "[LINK:") || strings.Contains(res.Text, "[/LINK]") {
		t.Errorf("unmatched placeholder leaked into text: %q", res.Text)
	}
}
