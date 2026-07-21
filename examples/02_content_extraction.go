//go:build examples

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"

	"github.com/cybergodev/html"
)

// This example demonstrates content extraction options and output formats.
// Learn how to customize extraction and produce different output formats.
//
// NOTE: For brevity, extraction errors are elided in this example;
// see 07_error_handling.go for proper error-handling patterns.
func main() {
	fmt.Println("=== Content Extraction & Output Formats ===")
	fmt.Println()

	sampleHTML := `
		<html>
			<head><title>Go Interfaces Guide</title></head>
			<body>
				<article>
					<h1>Understanding Go Interfaces</h1>
					<p>Interfaces provide a way to specify behavior.</p>
					<img src="diagram.jpg" alt="Interface Diagram" width="600">
					<a href="https://go.dev/tour/">Go Tour</a>
					<table>
						<tr><th>Feature</th><th>Benefit</th></tr>
						<tr><td>Goroutines</td><td>Lightweight concurrency</td></tr>
						<tr><td>Channels</td><td>Safe communication</td></tr>
					</table>
				</article>
			</body>
		</html>
	`

	// ============================================================
	// 1. Preset Configurations (Recommended)
	// ============================================================
	fmt.Println("1. Preset Configurations")
	fmt.Println("-----------------------")

	// Default: all media preserved
	fmt.Println("DefaultConfig():  All media preserved")

	// Text-only: simple preset
	textOnlyProcessor, err := html.New(html.TextOnlyConfig())
	if err != nil {
		log.Fatal(err)
	}
	defer textOnlyProcessor.Close()
	result, _ := textOnlyProcessor.Extract([]byte(sampleHTML))
	fmt.Printf("TextOnlyConfig(): %d chars, %d images\n\n", len(result.Text), len(result.Images))

	// Markdown: images as markdown
	mdProcessor, err := html.New(html.MarkdownConfig())
	if err != nil {
		log.Fatal(err)
	}
	defer mdProcessor.Close()
	mdResult, _ := mdProcessor.Extract([]byte(sampleHTML))
	fmt.Printf("MarkdownConfig(): %d chars, images inline as markdown\n\n", len(mdResult.Text))

	// ============================================================
	// 2. Custom Configuration
	// ============================================================
	fmt.Println("2. Custom Configuration")
	fmt.Println("-----------------------")

	// Start from preset, modify as needed
	cfg := html.DefaultConfig()
	cfg.PreserveImages = true
	cfg.PreserveLinks = true
	cfg.PreserveVideos = false
	cfg.PreserveAudios = false
	cfg.InlineImageFormat = "markdown"

	customProcessor, err := html.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer customProcessor.Close()
	result, _ = customProcessor.Extract([]byte(sampleHTML))
	fmt.Printf("Images: %d, Links: %d\n\n", len(result.Images), len(result.Links))

	// ============================================================
	// 3. Inline Format Options (Images & Links)
	// ============================================================
	fmt.Println("3. Inline Format Options (Images & Links)")
	fmt.Println("-------------------------------------------")

	// InlineImageFormat controls how <img> is rendered in Text.
	imageHTML := `<img src="photo.jpg" alt="Photo" width="800">`
	for _, format := range []string{"none", "markdown", "html", "placeholder"} {
		cfg := html.DefaultConfig()
		cfg.InlineImageFormat = format
		r, _ := html.Extract([]byte(imageHTML), cfg)
		fmt.Printf("  image %-11s: %s\n", format, r.Text)
	}

	// InlineLinkFormat controls how <a> is rendered in Text.
	linkHTML := `<p>See the <a href="https://go.dev">Go site</a> for docs.</p>`
	for _, format := range []string{"none", "markdown", "html"} {
		cfg := html.DefaultConfig()
		cfg.InlineLinkFormat = format
		r, _ := html.Extract([]byte(linkHTML), cfg)
		fmt.Printf("  link  %-11s: %s\n", format, r.Text)
	}
	fmt.Println()

	// ============================================================
	// 4. Table Format Options
	// ============================================================
	fmt.Println("4. Table Format Options")
	fmt.Println("-----------------------")

	// A table-only fixture keeps the rendering comparison free of unrelated
	// image/link noise. Markdown (default) is compact; html preserves the tags.
	tableHTML := "<table><tr><th>Lang</th><th>Year</th></tr><tr><td>Go</td><td>2009</td></tr></table>"
	for _, format := range []string{"markdown", "html"} {
		cfg := html.DefaultConfig()
		cfg.TableFormat = format
		r, _ := html.Extract([]byte(tableHTML), cfg)
		fmt.Printf("  TableFormat=%-9s:\n%s\n", format, r.Text)
	}
	fmt.Println()

	// ============================================================
	// 5. JSON Output
	// ============================================================
	fmt.Println("5. JSON Output")
	fmt.Println("--------------")

	processor, err := html.New()
	if err != nil {
		log.Fatal(err)
	}
	defer processor.Close()

	jsonBytes, err := processor.ExtractToJSON([]byte(sampleHTML))
	if err != nil {
		log.Fatal(err)
	}

	// Pretty-print without a lossy unmarshal/remarshal round-trip:
	// json.Indent reformats the bytes in place, preserving key order and
	// numeric precision exactly as the library produced them.
	var prettyBuf bytes.Buffer
	if err := json.Indent(&prettyBuf, jsonBytes, "", "  "); err != nil {
		log.Fatal(err)
	}
	pretty := prettyBuf.Bytes()
	if len(pretty) > 400 {
		fmt.Printf("%s...\n\n", string(pretty[:400]))
	} else {
		fmt.Printf("%s\n\n", pretty)
	}

	// ============================================================
	// 6. Markdown Output
	// ============================================================
	fmt.Println("6. Markdown Output")
	fmt.Println("------------------")

	markdown, _ := processor.ExtractToMarkdown([]byte(sampleHTML))
	if len(markdown) > 200 {
		fmt.Printf("%s...\n\n", markdown[:200])
	} else {
		fmt.Printf("%s\n\n", markdown)
	}

	// ============================================================
	// 7. File Operations
	// ============================================================
	fmt.Println("7. File Operations")
	fmt.Println("------------------")
	fmt.Println("  processor.ExtractFromFile(\"article.html\")")
	fmt.Println("  processor.ExtractToJSONFromFile(\"article.html\")")
	fmt.Println("  processor.ExtractToMarkdownFromFile(\"article.html\")")

	// ============================================================
	// 8. Encoding Support
	// ============================================================
	fmt.Println("\n8. Encoding Support")
	fmt.Println("-------------------")
	fmt.Println("  Auto-detects: UTF-8, GBK, Big5, Shift_JIS, Windows-1250/1251/1252, ISO-8859-*")
	fmt.Println("  Specify explicitly:")
	fmt.Println("    cfg := html.DefaultConfig()")
	fmt.Println("    cfg.Encoding = \"windows-1252\"")

	// ============================================================
	// Summary
	// ============================================================
	fmt.Println("\n=== Quick Reference ===")
	fmt.Println()
	fmt.Println("Preset configs (recommended):")
	fmt.Println("  html.DefaultConfig()      - Full features, all media")
	fmt.Println("  html.TextOnlyConfig()     - Plain text only")
	fmt.Println("  html.MarkdownConfig()     - Markdown output")
	fmt.Println("  html.HighSecurityConfig() - Strict limits, audit enabled")
	fmt.Println()
	fmt.Println("Customization pattern:")
	fmt.Println("  cfg := html.DefaultConfig()       // Start from preset")
	fmt.Println("  cfg.PreserveImages = false  // Modify as needed")
	fmt.Println("  processor, _ := html.New(cfg)")
	fmt.Println()
	fmt.Println("Image formats:  none | markdown | html | placeholder")
	fmt.Println("Link formats:   none | markdown | html")
	fmt.Println("Table formats:  markdown | html")
	fmt.Println("Output methods: Extract(), ExtractToJSON(), ExtractToMarkdown()")
	fmt.Println()
	fmt.Println("Scope: ExtractArticle (default true) auto-detects the main content node;")
	fmt.Println("       set false to extract the whole <body> instead.")
}
