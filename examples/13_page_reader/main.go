package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/cybergodev/html"
)

// This example demonstrates reader mode (ReadPage / ReadPageToMarkdown): a
// full-page, structure-labeled reading view whose primary use is feeding a
// "clean page" to LLMs and other text-only consumers.
//
// Unlike Extract — which selects the article and discards navigation and
// footers — reader mode keeps every page region (header, navigation, main,
// aside, footer) as a labeled section. HTML, CSS, and JS markup is removed;
// hyperlinks stay inline in the text, rendered as Markdown [text](URL) by
// default or HTML <a> anchors via Config.InlineLinkFormat (links are always
// preserved — "none" is treated as "markdown" in reader mode).
func main() {
	fmt.Println("=== Reader Mode: Clean Pages for LLMs ===")
	fmt.Println()

	processor, err := html.New()
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = processor.Close() }()

	// A page exercising every reader behavior: a header with a nested nav
	// (the canonical masthead shape), a hidden mobile-duplicate nav, an ad
	// slot inside the article, a related-posts residual, and a <base> for
	// relative-URL resolution.
	pageHTML := `<html>
<head>
	<title>Quarterly Results - Example News</title>
	<base href="https://news.example.com/quarterly/">
</head>
<body>
	<header>
		<nav class="mobile-only d-none"><a href="/">Home</a><a href="/m1">M1</a><a href="/m2">M2</a></nav>
		<nav class="site-nav"><a href="/">Home</a> <a href="/tech">Tech</a> <a href="/finance">Finance</a></nav>
	</header>
	<main>
		<article>
			<h1>Quarterly Results Announced</h1>
			<p>The company posted strong quarterly results on Tuesday, beating every analyst estimate by a wide margin as revenue grew across all segments. <a href="analysis.html">Full analysis</a> is available separately.</p>
			<p>Executives attributed the growth to sustained demand in the enterprise division and said guidance would be raised next week.</p>
			<div class="ad-slot"><a href="/buy1">Buy now</a><a href="/buy2">Deal</a><a href="/buy3">Offer</a></div>
		</article>
	</main>
	<aside><p>Sidebar: market data summary and index performance.</p></aside>
	<div class="related-posts"><a href="/p1">Post 1</a><a href="/p2">Post 2</a><a href="/p3">Post 3</a></div>
	<footer><p>© 2026 Example News <a href="/privacy">Privacy</a> <a href="/tos">Terms</a></p></footer>
</body>
</html>`
	doc := []byte(pageHTML)

	// ============================================================
	// 1. The one-call LLM forms: whole page as one Markdown document,
	//    with links (ToMarkdown) or without (ToMarkdownText)
	// ============================================================
	fmt.Println("1. ReadPageToMarkdown / ReadPageToMarkdownText")
	fmt.Println("----------------------------------------------")

	markdown, err := processor.ReadPageToMarkdown(doc)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(markdown)
	fmt.Println("Note what is gone: the hidden mobile nav, the ad slot, the")
	fmt.Println("related-posts block, and all HTML/CSS/JS markup. Relative URLs")
	fmt.Println("are resolved against the detected base (document origin).")

	// The same document without inline links — structure (headings, lists,
	// tables) and link labels stay, the [text](URL) constructs do not:
	plainDoc, err := processor.ReadPageToMarkdownText(doc)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println()
	fmt.Println("ReadPageToMarkdownText (link-free) — the navigation section:")
	fmt.Println("------------------------------------------------------------")
	for line := range strings.SplitSeq(plainDoc, "\n") {
		if strings.Contains(line, "Home") || strings.Contains(line, "Tech") {
			fmt.Println(line)
		}
	}

	// ============================================================
	// 2. Structured sections: callers decide what to feed
	// ============================================================
	fmt.Println()
	fmt.Println("2. ReadPage — labeled sections")
	fmt.Println("-----------------------------")

	page, err := processor.ReadPage(doc)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Title: %s\n", page.Title)
	for i := range page.Sections {
		sec := &page.Sections[i]
		fmt.Printf("  %-10s %3d words %2d links\n", sec.Kind, sec.WordCount, len(sec.Links))
	}

	// Feed only the main content to the LLM when the chrome is noise:
	fmt.Println()
	fmt.Println("Main section only:")
	for i := range page.Sections {
		if page.Sections[i].Kind == html.SectionMain {
			fmt.Println(strings.TrimRight(page.Sections[i].Text, "\n"))
		}
	}

	// Or compose a subset ReadResult and flatten it with Render() (links
	// inline) / RenderText() (link-free) — the same rendering the one-call
	// ReadPageToMarkdown forms use, applied to sections you chose:
	contentOnly := html.ReadResult{Title: page.Title}
	for i := range page.Sections {
		if k := page.Sections[i].Kind; k == html.SectionMain || k == html.SectionAside {
			contentOnly.Sections = append(contentOnly.Sections, page.Sections[i])
		}
	}
	fmt.Println()
	fmt.Println("Render() over Main+Aside only:")
	fmt.Println(strings.TrimRight(contentOnly.Render(), "\n"))

	// ============================================================
	// 3. Link style is config-selected: "html" keeps <a> anchors
	// ============================================================
	fmt.Println()
	fmt.Println(`3. InlineLinkFormat = "html" — anchors in the text`)
	fmt.Println("------------------------------------------------")

	cfg := html.DefaultConfig()
	cfg.InlineLinkFormat = "html"
	cfg.MaxCacheEntries = 0
	anchorPage, err := html.ReadPage(doc, cfg)
	if err != nil {
		log.Fatal(err)
	}
	for i := range anchorPage.Sections {
		if anchorPage.Sections[i].Kind == html.SectionHeader {
			fmt.Println(strings.TrimRight(anchorPage.Sections[i].Text, "\n"))
			break
		}
	}

	// ============================================================
	// 4. File input: the FromFile variants
	// ============================================================
	fmt.Println()
	fmt.Println("4. ReadPageFromFile / ReadPageFromFileToMarkdown")
	fmt.Println("-----------------------------------------------")

	path := filepath.Join(os.TempDir(), "html-reader-example.html")
	if err := os.WriteFile(path, doc, 0o600); err != nil {
		log.Fatal(err)
	}
	defer func() { _ = os.Remove(path) }()

	fromFile, err := html.ReadPageFromFileToMarkdown(path)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("ReadPageFromFileToMarkdown(%q): %d bytes, identical to the bytes form: %v\n",
		filepath.Base(path), len(fromFile), fromFile == markdown)

	structured, err := processor.ReadPageFromFile(path)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("ReadPageFromFile(%q): %d sections, title %q\n",
		filepath.Base(path), len(structured.Sections), structured.Title)
}
