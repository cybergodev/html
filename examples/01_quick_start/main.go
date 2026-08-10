package main

import (
	"fmt"
	"log"

	"github.com/cybergodev/html"
)

// This example demonstrates the fastest way to get started with the library.
// Perfect for first-time users who want to see immediate results.
func main() {
	fmt.Println("=== Quick Start ===")
	fmt.Println()

	// ============================================================
	// 1. Extract plain text (simplest approach)
	// ============================================================
	htmlContent := `
		<html>
			<body>
				<h1>Welcome to Go</h1>
				<p>Go is a powerful programming language.</p>
			</body>
		</html>
	`

	text, err := html.ExtractText([]byte(htmlContent))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("1. Plain text:\n   %s\n\n", text)

	// ============================================================
	// 2. Extract with metadata (title, word count, etc.)
	// ============================================================
	result, err := html.Extract([]byte(htmlContent))
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("2. With metadata:\n")
	fmt.Printf("   Title: %s\n", result.Title)
	fmt.Printf("   Words: %d\n", result.WordCount)
	fmt.Printf("   Reading time: %v\n\n", result.ReadingTime)

	// ============================================================
	// 3. Reuse processor for multiple documents (efficient)
	// ============================================================
	processor, err := html.New()
	if err != nil {
		log.Fatal(err)
	}
	defer processor.Close()

	docs := []string{
		`<article><h1>First Post</h1><p>Content 1</p></article>`,
		`<article><h1>Second Post</h1><p>Content 2</p></article>`,
	}

	fmt.Println("3. Process multiple documents:")
	for i, doc := range docs {
		result, err := processor.Extract([]byte(doc))
		if err != nil {
			continue
		}
		fmt.Printf("   Doc %d: %s (%d words)\n", i+1, result.Title, result.WordCount)
	}

	// ============================================================
	// 4. ExtractArticle mode (default: true)
	// ============================================================
	// When ExtractArticle is true (the default), the library identifies the
	// primary content node (e.g. <article>) and extracts only that subtree.
	// Setting it to false extracts the entire <body>, which is useful when
	// you need surrounding context (related sections, comments, etc.).
	// Note: nav, aside, footer, and script are always removed regardless
	// of this setting.
	fmt.Println("\n4. ExtractArticle mode:")
	pageWithExtra := `<html><body>
		<div>
			<h2>Related Articles</h2>
			<p>See also: Understanding Go Channels.</p>
		</div>
		<article>
			<h1>Main Article</h1>
			<p>This is the primary content.</p>
		</article>
	</body></html>`

	articleResult, _ := processor.Extract([]byte(pageWithExtra))
	fmt.Printf("   ExtractArticle=true (default):\n     %s\n", articleResult.Text)

	fullBodyCfg := html.DefaultConfig()
	fullBodyCfg.ExtractArticle = false
	fullBodyProc, _ := html.New(fullBodyCfg)
	defer fullBodyProc.Close()
	fullResult, _ := fullBodyProc.Extract([]byte(pageWithExtra))
	fmt.Printf("   ExtractArticle=false:\n     %s\n", fullResult.Text)

	// ============================================================
	// Summary
	// ============================================================
	fmt.Println("\n=== Quick Reference ===")
	fmt.Println("One-shot extraction:")
	fmt.Println("  html.ExtractText([]byte(html))     - Plain text only")
	fmt.Println("  html.Extract([]byte(html))         - Full result with metadata")
	fmt.Println()
	fmt.Println("Reusable processor:")
	fmt.Println("  processor, _ := html.New()")
	fmt.Println("  defer processor.Close()")
	fmt.Println("  result, _ := processor.Extract([]byte(html))")
}
