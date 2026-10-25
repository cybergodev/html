package internal

// metadata_test.go — unit tests for CollectMetadata at the internal layer.
// The root package exercises it end-to-end through Extract/ReadPage (title
// suffix stripping, quality scoring); these rows pin each source's
// precedence and parsing rules directly: article-typed JSON-LD blocks over
// OpenGraph over classic <meta>, the author property's three shapes, the
// WebPage-name last resort, canonical over og:url, and malformed or
// non-ld+json scripts leaving fields empty.

import (
	"testing"
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
