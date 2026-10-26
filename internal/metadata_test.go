package internal

// metadata_test.go — unit tests for CollectMetadata at the internal layer.
// The root package exercises it end-to-end through Extract/ReadPage (title
// suffix stripping, quality scoring); these rows pin each source's
// precedence and parsing rules directly: article-typed JSON-LD blocks over
// OpenGraph over classic <meta>, the author property's three shapes, the
// WebPage-name last resort, canonical over og:url, and malformed or
// non-ld+json scripts leaving fields empty.

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestCollectMetadata(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		doc  string
		want Metadata
	}{
		{
			name: "open graph over classic meta",
			doc: `<html><head>
				<meta property="og:title" content="OG Title">
				<meta property="og:description" content="OG blurb">
				<meta property="og:site_name" content="OG Site">
				<meta property="og:url" content="https://e.com/og">
				<meta property="article:published_time" content="2026-03-04">
				<meta name="author" content="Meta Author">
				<meta name="description" content="Meta blurb">
			</head><body></body></html>`,
			want: Metadata{
				Title:         "OG Title",
				Author:        "Meta Author",
				PublishedTime: "2026-03-04",
				Description:   "OG blurb",
				SiteName:      "OG Site",
				CanonicalURL:  "https://e.com/og",
			},
		},
		{
			name: "article json-ld wins scoped fields, og still owns description",
			doc: `<html><head>
				<meta property="og:title" content="OG Title">
				<meta property="og:description" content="OG blurb">
				<meta name="author" content="Meta Author">
				<script type="application/ld+json">
					{"@type":"NewsArticle","headline":"LD Head","author":{"name":"Ada Lovelace"},
					 "datePublished":"2026-01-02","description":"LD desc","publisher":{"name":"PubCo"}}
				</script>
			</head><body></body></html>`,
			want: Metadata{
				Title:         "LD Head",
				Author:        "Ada Lovelace",
				PublishedTime: "2026-01-02",
				Description:   "OG blurb",
				SiteName:      "PubCo",
			},
		},
		{
			name: "website name is only the title of last resort",
			doc: `<html><head>
				<script type="application/ld+json">{"@type":"WebSite","name":"Site Wide"}</script>
				<script type="application/ld+json">{"@type":"Article","headline":"Art Head"}</script>
			</head><body></body></html>`,
			want: Metadata{
				Title:    "Art Head",
				SiteName: "Site Wide",
			},
		},
		{
			name: "author array joins display names",
			doc: `<html><head>
				<script type="application/ld+json">
					{"@type":"Article","headline":"H","author":["First",{"@type":"Person","name":"Second"}]}
				</script>
			</head><body></body></html>`,
			want: Metadata{Title: "H", Author: "First, Second"},
		},
		{
			name: "malformed ld block falls back to open graph",
			doc: `<html><head>
				<meta property="og:title" content="OG Fallback">
				<script type="application/ld+json">{"headline": not json</script>
			</head><body></body></html>`,
			want: Metadata{Title: "OG Fallback"},
		},
		{
			name: "canonical link beats og:url, non-ld scripts ignored",
			doc: `<html><head>
				<meta property="og:url" content="https://e.com/og">
				<link rel="canonical" href="https://e.com/canonical">
				<script type="application/json">{"headline":"Ignored"}</script>
			</head><body></body></html>`,
			want: Metadata{CanonicalURL: "https://e.com/canonical"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc, err := parseHTML(tt.doc)
			if err != nil {
				t.Fatalf("parseHTML: %v", err)
			}
			got := CollectMetadata(doc)
			if got != tt.want {
				t.Errorf("CollectMetadata =\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}

	if got := CollectMetadata(nil); got != (Metadata{}) {
		t.Errorf("CollectMetadata(nil) = %+v, want zero Metadata", got)
	}
}

// ---------------------------------------------------------------------------
// scriptText boundaries ([FIX-001] boundary additions)
// ---------------------------------------------------------------------------

// TestScriptTextLimits pins scriptText's byte-cap and multi-node behavior:
// text past the limit is truncated (mid-rune bytes included, byte-counted like
// production), non-text children are skipped, and a single long text node
// stops exactly at the limit.
func TestScriptTextLimits(t *testing.T) {
	t.Parallel()

	script := &html.Node{Type: html.ElementNode, Data: "script"}
	script.AppendChild(&html.Node{Type: html.CommentNode, Data: "not text"})
	script.AppendChild(&html.Node{Type: html.TextNode, Data: "abcdefghij"})
	script.AppendChild(&html.Node{Type: html.TextNode, Data: "KLMNOP"})

	if got, want := scriptText(script, 5), "abcde"; got != want {
		t.Errorf("scriptText(limit=5) = %q, want %q (truncated at the cap)", got, want)
	}
	if got, want := scriptText(script, 15), "abcdefghijKLMNO"; got != want {
		t.Errorf("scriptText(limit=15) = %q, want %q (cap lands in the second node)", got, want)
	}
	if got, want := scriptText(script, 100), "abcdefghijKLMNOP"; got != want {
		t.Errorf("scriptText(limit=100) = %q, want %q (no truncation)", got, want)
	}
	if got := scriptText(&html.Node{Type: html.ElementNode, Data: "script"}, 10); got != "" {
		t.Errorf("scriptText(empty script) = %q, want empty", got)
	}
	if !strings.Contains(scriptText(script, 100), "abcdefghij") {
		t.Error("comment child was not skipped")
	}
}
