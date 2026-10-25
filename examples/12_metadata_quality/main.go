package main

import (
	"fmt"
	"log"
	"time"

	"github.com/cybergodev/html"
)

// This example demonstrates structured-metadata extraction (JSON-LD,
// OpenGraph, classic <meta>, rel=canonical) and the extraction-quality
// signals Result.Confidence and Result.ExtractionMethod.
//
// Metadata collection is on by default (Config.ExtractMetadata) and also
// upgrades the Title priority: a JSON-LD headline or og:title outranks the
// <title> tag.
func main() {
	fmt.Println("=== Metadata & Quality Signals ===")
	fmt.Println()

	processor, err := html.New()
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = processor.Close() }()

	// ============================================================
	// 1. JSON-LD (schema.org)
	// ============================================================
	fmt.Println("1. JSON-LD (schema.org)")
	fmt.Println("-----------------------")

	// The JSON-LD block populates every metadata field, and its headline
	// outranks both the <title> tag and the DOM <h1>.
	jsonldHTML := `<html><head>
<title>Legacy Title Tag - Example News</title>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"NewsArticle",
 "headline":"Real Headline from JSON-LD",
 "author":{"@type":"Person","name":"Jane Doe"},
 "datePublished":"2026-10-25T10:30:00Z",
 "description":"A summary from JSON-LD.",
 "publisher":{"@type":"Organization","name":"Example News"}}
</script>
</head><body>
<article><h1>DOM Headline</h1><p>Body text of the story.</p></article>
</body></html>`

	result, err := processor.Extract([]byte(jsonldHTML))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  Title:       %s\n", result.Title)
	fmt.Printf("  Author:      %s\n", result.Author)
	fmt.Printf("  Published:   %s\n", formatTime(result.PublishedTime))
	fmt.Printf("  Description: %s\n", result.Description)
	fmt.Printf("  SiteName:    %s\n\n", result.SiteName)

	// ============================================================
	// 2. OpenGraph + classic <meta> + rel=canonical
	// ============================================================
	fmt.Println("2. OpenGraph, <meta>, rel=canonical")
	fmt.Println("-----------------------------------")

	ogHTML := `<html><head>
<title>OG Page - My Site</title>
<meta property="og:title" content="OpenGraph Title">
<meta property="og:description" content="An OpenGraph description.">
<meta property="og:site_name" content="My Site">
<meta property="article:published_time" content="2026-10-20T08:00:00Z">
<meta name="author" content="Grace Hopper">
<link rel="canonical" href="https://example.com/canonical-page">
</head><body><article><h1>OG Page</h1><p>Body text of the page.</p></article></body></html>`

	result, err = processor.Extract([]byte(ogHTML))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  Title:        %s   (og:title beats <title>)\n", result.Title)
	fmt.Printf("  Author:       %s   (<meta name=author>)\n", result.Author)
	fmt.Printf("  Published:    %s\n", formatTime(result.PublishedTime))
	fmt.Printf("  Description:  %s\n", result.Description)
	fmt.Printf("  SiteName:     %s\n", result.SiteName)
	fmt.Printf("  CanonicalURL: %s   (rel=canonical beats og:url)\n\n", result.CanonicalURL)

	// ============================================================
	// 3. Turning metadata off (Config.ExtractMetadata)
	// ============================================================
	fmt.Println("3. Turning Metadata Off")
	fmt.Println("-----------------------")

	// ExtractMetadata=false skips collection entirely: the metadata fields
	// stay empty and Title falls back to the <title> tag (the trailing
	// " - Site" suffix is still stripped).
	noMetaCfg := html.DefaultConfig()
	noMetaCfg.ExtractMetadata = false
	noMetaResult, err := html.Extract([]byte(jsonldHTML), noMetaCfg)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("  Author:      %q (empty: nothing was collected)\n", noMetaResult.Author)
	fmt.Printf("  Description: %q\n", noMetaResult.Description)
	fmt.Printf("  Title:       %q (<title> fallback, suffix stripped)\n\n", noMetaResult.Title)

	// ============================================================
	// 4. Quality Signals: Confidence & ExtractionMethod
	// ============================================================
	fmt.Println("4. Quality Signals")
	fmt.Println("-----------------")

	// Confidence is a coarse triage signal in [0, 1] — route anything below
	// 1.0 to review or a fallback strategy; it is not a probability.
	// ExtractionMethod reports where Text came from: "article" (main-content
	// node), "listing" (a card feed), or "body" (full-body fallback).
	fixtures := []struct {
		label string
		page  string
		note  string
	}{
		{
			label: "full article",
			page:  `<html><body><article><h1>Go Concurrency Patterns</h1><p>` + articleBody + `</p></article></body></html>`,
			note:  "article node with substantial text",
		},
		{
			label: "card feed",
			page:  listingHTML,
			note:  "listing: Text is the whole card list, not one article",
		},
		{
			label: "paywall stub",
			page:  `<html><body><article><h1>Premium Story</h1><p>Subscribe to continue reading this story. This preview is all anonymous visitors can see.</p></article></body></html>`,
			note:  "consent/paywall boilerplate captured instead of content",
		},
	}

	for _, f := range fixtures {
		r, err := processor.Extract([]byte(f.page))
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("  %-13s Confidence=%.1f  ExtractionMethod=%q\n", f.label, r.Confidence, r.ExtractionMethod)
		fmt.Printf("                %s\n", f.note)
	}

	// ============================================================
	// Summary
	// ============================================================
	fmt.Println("\n=== Quick Reference ===")
	fmt.Println("Metadata priority, per field:")
	fmt.Println("  Title:         JSON-LD headline > og:title > <title>")
	fmt.Println("  Author:        JSON-LD author > <meta name=author>")
	fmt.Println("  PublishedTime: article:published_time > JSON-LD datePublished")
	fmt.Println("  Description:   og:description > JSON-LD description > <meta name=description>")
	fmt.Println("  SiteName:      og:site_name > JSON-LD publisher")
	fmt.Println("  CanonicalURL:  <link rel=canonical> > og:url")
	fmt.Println()
	fmt.Println("Quality signals:")
	fmt.Println("  Confidence: 1.0 clean article | 0.6 listing or short article | 0.4 thin/paywall")
	fmt.Println("  ExtractionMethod: article | listing | body (fallback)")
	fmt.Println("  Config.ExtractMetadata=false disables collection")
}

// articleBody is filler prose long enough (>= 200 chars) for an article-node
// extraction to reach the top confidence tier.
const articleBody = "Go's concurrency model is built around goroutines and channels. " +
	"Goroutines are lightweight threads managed by the Go runtime, and channels " +
	"are typed pipes for safe communication between them. Together they make " +
	"concurrent designs explicit: data flows through channels, ownership stays " +
	"clear, and data races become hard to write by accident."

// listingHTML is a card-feed page: link-dense sibling <article> cards dominate
// the body, so the page is recognized as a listing rather than one article.
const listingHTML = `<html><head><title>Tech News - Example Wire</title></head><body>
<nav><a href="/">Home</a> <a href="/tech">Tech</a></nav>
<div class="feed">
  <article><h2><a href="/1">First card headline about quantum chips</a></h2><p>Excerpt text for the first card, summarizing the story.</p></article>
  <article><h2><a href="/2">Second card headline about climate models</a></h2><p>Excerpt text for the second card, summarizing the story.</p></article>
  <article><h2><a href="/3">Third card headline about market moves</a></h2><p>Excerpt text for the third card, summarizing the story.</p></article>
  <article><h2><a href="/4">Fourth card headline about space launches</a></h2><p>Excerpt text for the fourth card, summarizing the story.</p></article>
</div>
<footer><p>© Example Wire</p></footer>
</body></html>`

// formatTime renders PublishedTime, treating the zero time as unknown.
func formatTime(t time.Time) string {
	if t.IsZero() {
		return "(unknown)"
	}
	return t.Format(time.RFC3339)
}
