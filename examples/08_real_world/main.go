package main

import (
	"fmt"
	"log"
	"unicode/utf8"

	"github.com/cybergodev/html"
)

// This example demonstrates real-world use cases.
// Perfect for understanding practical patterns.
//
// NOTE: For brevity, extraction errors are elided in most use cases below;
// see 07_error_handling for proper error-handling patterns.
func main() {
	fmt.Println("=== Real-World Use Cases ===")
	fmt.Println()

	processor, err := html.New()
	if err != nil {
		log.Fatal(err)
	}
	defer processor.Close()

	// ============================================================
	// Use Case 1: Blog Article Extraction
	// ============================================================
	fmt.Println("Use Case 1: Blog Article Extraction")
	fmt.Println("------------------------------------")

	blogHTML := `
		<html>
			<head><title>Understanding Go Generics</title></head>
			<body>
				<nav>Menu | Home | About</nav>
				<main>
					<article>
						<h1>Understanding Go Generics</h1>
						<p>Generics allow writing flexible code that works with multiple types.</p>
						<p>They were introduced in Go 1.18 and have become a powerful tool.</p>
					</article>
				</main>
				<footer>© 2024</footer>
			</body>
		</html>
	`

	result, _ := processor.Extract([]byte(blogHTML))
	fmt.Printf("Title: %s\n", result.Title)
	fmt.Printf("Content: %s\n", truncateText(result.Text, 100))
	fmt.Printf("Words: %d\n", result.WordCount)
	fmt.Printf("Reading time: %v\n\n", result.ReadingTime)

	// ============================================================
	// Use Case 2: Newsletter Processing
	// ============================================================
	fmt.Println("Use Case 2: Newsletter Content")
	fmt.Println("------------------------------")

	newsletterHTML := `
		<html>
			<body>
				<p style="display:none">Unsubscribe | View in browser</p>
				<table width="600">
					<tr><td><h1>Weekly Tech Digest</h1></td></tr>
					<tr><td><p>This week in tech news and updates.</p></td></tr>
					<tr><td><p>Subscribe for more tutorials.</p></td></tr>
				</table>
			</body>
		</html>
	`

	result, _ = processor.Extract([]byte(newsletterHTML))
	fmt.Printf("Title: %s\n", result.Title)
	fmt.Printf("Content length: %d chars\n", len(result.Text))
	fmt.Println("  ✓ Ignored hidden content")
	fmt.Println("  ✓ Extracted main content from tables")
	fmt.Println()

	// ============================================================
	// Use Case 3: RSS Feed Item Processing
	// ============================================================
	fmt.Println("Use Case 3: RSS Feed Items")
	fmt.Println("--------------------------")

	rssItems := []struct {
		title string
		desc  string
	}{
		{"Go 1.22 Released", "<p>New features include enhanced for loops.</p>"},
		{"Go 1.21 Released", "<p>Bug fixes and performance improvements.</p>"},
		{"Go 1.20 Released", "<p>PGO support and coverage tooling.</p>"},
	}

	fmt.Println("Processing RSS feed items:")
	for i, item := range rssItems {
		content := item.desc
		result, err := processor.Extract([]byte(content))
		if err != nil {
			continue
		}
		fmt.Printf("  [%d] %s (%d words)\n", i+1, item.title, result.WordCount)
	}
	fmt.Println()

	// ============================================================
	// Use Case 4: Documentation Content
	// ============================================================
	fmt.Println("Use Case 4: Documentation Extraction")
	fmt.Println("------------------------------------")

	docsHTML := `
		<html>
			<head><title>API Reference</title></head>
			<body>
				<aside>Sidebar navigation</aside>
				<main>
					<h1>API Reference</h1>
					<p>This document describes the available API endpoints.</p>
					<pre><code>func New(cfg Config) (*Processor, error)</code></pre>
					<p>Creates a new processor with the given configuration.</p>
				</main>
			</body>
		</html>
	`

	result, _ = processor.Extract([]byte(docsHTML))
	fmt.Printf("Title: %s\n", result.Title)
	fmt.Printf("Content length: %d chars\n", len(result.Text))
	fmt.Println("  ✓ Removed sidebar navigation")
	fmt.Println("  ✓ Preserved code blocks")
	fmt.Println()

	// ============================================================
	// Use Case 5: Batch Content Extraction
	// ============================================================
	fmt.Println("Use Case 5: Batch Content Extraction")
	fmt.Println("-------------------------------------")

	// Simulate multiple HTML pages
	pages := [][]byte{
		[]byte(`<html><head><title>Page 1</title></head><body><article><h1>First</h1><p>Content 1</p></article></body></html>`),
		[]byte(`<html><head><title>Page 2</title></head><body><article><h1>Second</h1><p>Content 2</p></article></body></html>`),
		[]byte(`<html><head><title>Page 3</title></head><body><article><h1>Third</h1><p>Content 3</p></article></body></html>`),
	}

	batchResult := processor.ExtractBatch(pages)
	fmt.Printf("Processed %d pages (%d success, %d failed):\n",
		len(batchResult.Results), batchResult.Success, batchResult.Failed)
	for i, r := range batchResult.Results {
		if r != nil {
			fmt.Printf("  [%d] %s (%d words)\n", i+1, r.Title, r.WordCount)
		}
	}
	fmt.Println()

	// ============================================================
	// Use Case 6: Link Crawler Pattern
	// ============================================================
	fmt.Println("Use Case 6: Link Crawler Pattern")
	fmt.Println("---------------------------------")

	pageWithLinks := `
		<html>
			<head><base href="https://example.com/blog/"></head>
			<body>
				<a href="/post/1">Post 1</a>
				<a href="/post/2">Post 2</a>
				<a href="https://external.com">External</a>
				<img src="image.jpg">
			</body>
		</html>
	`

	links, _ := processor.ExtractAllLinks([]byte(pageWithLinks))
	fmt.Printf("Found %d links:\n", len(links))

	// Group by type using GroupLinksByType
	grouped := html.GroupLinksByType(links)
	for linkType, group := range grouped {
		fmt.Printf("  %s: %d\n", linkType, len(group))
	}
	fmt.Println()

	// ============================================================
	// Use Case 7: Markdown Conversion
	// ============================================================
	fmt.Println("Use Case 7: Markdown Conversion")
	fmt.Println("--------------------------------")

	htmlToConvert := `
		<html>
			<body>
				<h1>Title</h1>
				<p>Paragraph with <strong>bold</strong> text.</p>
				<img src="photo.jpg" alt="Photo">
				<a href="https://example.com">Link</a>
			</body>
		</html>
	`

	markdown, _ := processor.ExtractToMarkdown([]byte(htmlToConvert))
	fmt.Printf("Markdown output:\n%s\n\n", truncateText(markdown, 150))

	// ============================================================
	// Use Case 8: JSON API Response
	// ============================================================
	fmt.Println("Use Case 8: JSON API Response")
	fmt.Println("------------------------------")

	jsonData, _ := processor.ExtractToJSON([]byte(htmlToConvert))
	fmt.Printf("JSON output (%d bytes)\n", len(jsonData))
	fmt.Println("  Use for REST API responses")
	fmt.Println("  Includes all metadata (title, word count, etc.)")
	fmt.Println()

	// ============================================================
	// Summary
	// ============================================================
	fmt.Println("=== Use Case Summary ===")
	fmt.Println("1. Blog extraction: Clean content, noise removal")
	fmt.Println("2. Newsletter: Table content, hidden elements")
	fmt.Println("3. RSS feeds: Batch processing of descriptions")
	fmt.Println("4. Documentation: Code blocks, sidebars")
	fmt.Println("5. Batch extraction: Parallel processing")
	fmt.Println("6. Link crawler: URL resolution and filtering")
	fmt.Println("7. Markdown: Content conversion")
	fmt.Println("8. JSON API: Structured output")
}

// truncateText shortens text for display, respecting multi-byte characters.
func truncateText(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	if len(runes) > maxLen {
		return string(runes[:maxLen]) + "..."
	}
	return s
}
