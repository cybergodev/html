// example_test.go holds the package's testable godoc examples. Each Example
// function below is executed by `go test`, so every // Output block doubles
// as a regression test for the documented behavior shown on pkg.go.dev.
package html_test

import (
	"fmt"
	"log"
	"maps"
	"slices"

	"github.com/cybergodev/html"
)

// ExampleExtract demonstrates the one-call extraction API returning the
// article text plus metadata.
func ExampleExtract() {
	doc := []byte(`<html><body>
		<nav>Site navigation</nav>
		<article>
			<h1>Go 1.26 released</h1>
			<p>The new release is available now.</p>
		</article>
	</body></html>`)

	result, err := html.Extract(doc)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Title)
	fmt.Printf("%q\n", result.Text)
	// Output:
	// Go 1.26 released
	// "# Go 1.26 released\n\nThe new release is available now."
}

// ExampleExtractText demonstrates the one-liner plain-text API. Navigation,
// scripts, and boilerplate are removed automatically.
func ExampleExtractText() {
	doc := []byte(`<html><body>
		<nav>Navigation Bar</nav>
		<article><h1>Hello World</h1><p>Content here...</p></article>
		<footer>Footer</footer>
	</body></html>`)

	text, err := html.ExtractText(doc)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%q\n", text)
	// Output:
	// "# Hello World\n\nContent here..."
}

// ExampleNew demonstrates configuring a Processor via DefaultConfig for
// repeated extractions. Close releases the processor's cache and background
// goroutines.
func ExampleNew() {
	cfg := html.DefaultConfig()
	cfg.MaxInputSize = 10 * 1024 * 1024 // limit input to 10 MB

	processor, err := html.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = processor.Close() }()

	result, err := processor.Extract([]byte(`<html><body>
		<article><h1>Title</h1><p>Body text.</p></article>
	</body></html>`))
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(result.Title, "/", result.WordCount)
	// Output:
	// Title / 3
}

// ExampleExtractAllLinks demonstrates enumerating every link resource
// (anchors, images, scripts, stylesheets, ...). Results are sorted by URL,
// so the output order is deterministic.
func ExampleExtractAllLinks() {
	doc := []byte(`<html><head>
		<link rel="stylesheet" href="https://cdn.example.com/style.css">
	</head><body>
		<a href="https://example.com/page">Example</a>
		<img src="https://cdn.example.com/logo.png" alt="Logo">
		<script src="https://cdn.example.com/app.js"></script>
	</body></html>`)

	links, err := html.ExtractAllLinks(doc)
	if err != nil {
		log.Fatal(err)
	}
	for _, link := range links {
		fmt.Printf("%s %s\n", link.Type, link.URL)
	}
	// Output:
	// js https://cdn.example.com/app.js
	// image https://cdn.example.com/logo.png
	// css https://cdn.example.com/style.css
	// link https://example.com/page
}

// ExampleGroupLinksByType demonstrates grouping extracted links by resource
// type. Keys are iterated in sorted order for deterministic output.
func ExampleGroupLinksByType() {
	doc := []byte(`<html><body>
		<a href="https://example.com/a">A</a>
		<a href="https://example.com/b">B</a>
		<img src="https://cdn.example.com/logo.png" alt="Logo">
		<script src="https://cdn.example.com/app.js"></script>
	</body></html>`)

	links, err := html.ExtractAllLinks(doc)
	if err != nil {
		log.Fatal(err)
	}
	grouped := html.GroupLinksByType(links)
	for _, kind := range slices.Sorted(maps.Keys(grouped)) {
		fmt.Printf("%s: %d\n", kind, len(grouped[kind]))
	}
	// Output:
	// image: 1
	// js: 1
	// link: 2
}
