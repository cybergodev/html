package html_test

// links_test.go — tests for the links API surface, gathered from the former
// single-topic files (group_links_test.go, fragment_link_test.go):
//
//   - GroupLinksByType's grouping contract (empty input, per-type buckets,
//     empty types folded to "unknown", grouping over extracted links);
//   - page-internal navigation (fragment-only and query-only hrefs) produces
//     no link entry — IsValidURL rejects "#section" and "?page=2", and the
//     doc comments on Result.Links, LinkInfo and LinkResource state that
//     explicitly — while a rejected href still consumes its [LINK:n]
//     position so placeholders stay aligned with Result.Links.

import (
	"strings"
	"testing"

	"github.com/cybergodev/html"
)

func TestGroupLinksByType(t *testing.T) {
	t.Parallel()

	t.Run("empty slice returns empty map", func(t *testing.T) {
		result := html.GroupLinksByType(nil)
		if len(result) != 0 {
			t.Errorf("expected empty map, got %d entries", len(result))
		}
		result = html.GroupLinksByType([]html.LinkResource{})
		if len(result) != 0 {
			t.Errorf("expected empty map, got %d entries", len(result))
		}
	})

	t.Run("groups links by type", func(t *testing.T) {
		links := []html.LinkResource{
			{URL: "https://a.com", Type: "stylesheet", Title: "CSS"},
			{URL: "https://b.com", Type: "stylesheet", Title: "CSS2"},
			{URL: "https://c.com", Type: "canonical", Title: "Canon"},
			{URL: "https://d.com", Type: "icon", Title: "Favicon"},
		}
		grouped := html.GroupLinksByType(links)
		if len(grouped) != 3 {
			t.Fatalf("expected 3 groups, got %d", len(grouped))
		}
		if len(grouped["stylesheet"]) != 2 {
			t.Errorf("expected 2 stylesheet links, got %d", len(grouped["stylesheet"]))
		}
		if len(grouped["canonical"]) != 1 {
			t.Errorf("expected 1 canonical link, got %d", len(grouped["canonical"]))
		}
		if len(grouped["icon"]) != 1 {
			t.Errorf("expected 1 icon link, got %d", len(grouped["icon"]))
		}
	})

	t.Run("links with empty type grouped as unknown", func(t *testing.T) {
		links := []html.LinkResource{
			{URL: "https://a.com", Type: "", Title: "No type"},
			{URL: "https://b.com", Type: "", Title: "Also no type"},
			{URL: "https://c.com", Type: "stylesheet", Title: "CSS"},
		}
		grouped := html.GroupLinksByType(links)
		if len(grouped) != 2 {
			t.Fatalf("expected 2 groups, got %d", len(grouped))
		}
		if len(grouped["unknown"]) != 2 {
			t.Errorf("expected 2 unknown links, got %d", len(grouped["unknown"]))
		}
	})

	t.Run("groups from extracted links", func(t *testing.T) {
		htmlContent := []byte(`<html><head>
			<link rel="stylesheet" href="https://example.com/style.css">
			<link rel="icon" href="https://example.com/favicon.ico">
			<link rel="canonical" href="https://example.com/page">
		</head><body>
			<a href="https://example.com/link1">Link 1</a>
			<a href="https://example.com/link2">Link 2</a>
		</body></html>`)

		links, err := html.ExtractAllLinks(htmlContent)
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		grouped := html.GroupLinksByType(links)

		if len(grouped) == 0 {
			t.Fatal("expected non-empty grouped result")
		}

		for typ, group := range grouped {
			if typ == "" {
				t.Error("group type should never be empty (empty types become 'unknown')")
			}
			for _, link := range group {
				if link.URL == "" {
					t.Error("grouped link should have a URL")
				}
			}
		}
	})
}

func TestExtractAllLinksNonHTTPBaseFallsBackToCanonical(t *testing.T) {
	t.Parallel()

	// A <base> whose value normalizes to nothing (about:blank) is not a
	// usable base: detection must fall through to the canonical/og:url ladder
	// instead of disabling URL resolution for the whole document.
	doc := []byte(`<html><head>
		<base href="about:blank">
		<link rel="canonical" href="https://example.com/page/">
	</head><body><a href="rel.html">rel</a></body></html>`)

	cfg := html.DefaultConfig()
	cfg.MaxCacheEntries = 0
	links, err := html.ExtractAllLinks(doc, cfg)
	if err != nil {
		t.Fatalf("ExtractAllLinks: %v", err)
	}
	// The ladder reduces the canonical to the origin root (NormalizeBaseURL's
	// documented behavior), so the relative link resolves against the host.
	var found bool
	for _, l := range links {
		if l.URL == "https://example.com/rel.html" {
			found = true
		}
	}
	if !found {
		t.Fatalf("relative link not resolved via canonical after unusable <base>: %+v", links)
	}
}

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

// TestEmbedLinkTitleFallbacks pins the embed link-title ladder through the
// public links API: an explicit title attribute wins, then the platform name
// recognized in the URL (YouTube / Vimeo / Dailymotion), then the generic
// "Embedded Video" for any other video URL.
func TestEmbedLinkTitleFallbacks(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name  string
		src   string
		title string // explicit title attribute; omitted when empty
		want  string
	}{
		{"explicit title", "https://www.youtube.com/embed/a", "Watch this", "Watch this"},
		{"youtube platform", "https://www.youtube.com/embed/a", "", "YouTube Video"},
		{"vimeo platform", "https://player.vimeo.com/video/1", "", "Vimeo Video"},
		{"dailymotion platform", "https://www.dailymotion.com/embed/video/x", "", "Dailymotion Video"},
		{"generic video fallback", "https://cdn.example.com/player.mp4", "", "Embedded Video"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			embed := `<embed src="` + tt.src + `"`
			if tt.title != "" {
				embed += ` title="` + tt.title + `"`
			}
			links, err := html.ExtractAllLinks([]byte(`<html><body>` + embed + `></body></html>`))
			if err != nil {
				t.Fatalf("ExtractAllLinks: %v", err)
			}
			for _, l := range links {
				if l.URL == tt.src {
					if l.Title != tt.want {
						t.Errorf("Title = %q, want %q", l.Title, tt.want)
					}
					if l.Type != "video" {
						t.Errorf("Type = %q, want %q", l.Type, "video")
					}
					return
				}
			}
			t.Errorf("embed URL %s missing from links: %+v", tt.src, links)
		})
	}
}
