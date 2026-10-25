package html_test

// article_test.go — tests for the article-selection model, gathered from the
// former single-topic files (article_cluster_regression_test.go,
// article_landmark_regression_test.go, article_selection_p2_test.go,
// hidden_class_regression_test.go, and the selection half of
// p3_features_test.go):
//
//   - P0-1 cluster widening: an article split across sibling blocks under one
//     content wrapper is widened to the parent so the whole article survives;
//     semantic <article>/<main> containers and body-level splits stay untouched;
//   - P0-2 landmarks: header/footer/aside are page chrome only OUTSIDE a
//     primary-content container; nested inside one they are article furniture
//     and survive. <nav> is hard non-content everywhere;
//   - P0-3 hiding: class/ARIA-based hiding (sr-only, d-none, aria-hidden)
//     strips content; responsive-visibility classes (hidden-sm) are preserved;
//   - P2 selection: the score-guarded ancestor climb excludes penalized
//     sibling chrome, captures headlines in outer wrappers, and recognizes
//     listing pages (card feeds) without misfiring on full articles;
//   - the ArticleFinder hook lets a custom Scorer bypass the Score
//     enumeration, and its nil verdict falls back to <body>;
//   - role=presentation tables are flattened into prose, not rendered as
//     Markdown data tables.

import (
	"strings"
	"testing"

	"github.com/cybergodev/html"
)

// extractArticleText extracts Text with the default config (ExtractArticle=true)
// and returns it, failing the test on error.
func extractArticleText(t *testing.T, markup string) string {
	t.Helper()
	p, err := html.New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = p.Close() }()

	result, err := p.Extract([]byte(markup))
	if err != nil {
		t.Fatalf("Extract() failed: %v", err)
	}
	return result.Text
}

// longParagraph builds filler prose of roughly n characters (>= 200) so an
// article node clears the confidentTextChars threshold.
func longParagraph(n int) string {
	base := "The measurement campaign ran for several months and produced consistent readings across every site, which the analysts took as strong evidence for the model. "
	var b strings.Builder
	for b.Len() < n {
		b.WriteString(base)
	}
	return b.String()
}

// ---------------------------------------------------------------------------
// P0-1 — split-article cluster widening
// ---------------------------------------------------------------------------

// TestSplitArticleSiblingsAreRejoined splits a real article into a headline div,
// a body div, and an author-note div under one wrapper. Each block is long
// enough to be a candidate, so before P0-1 only one block survived; now the
// wrapper is selected and all three survive.
func TestSplitArticleSiblingsAreRejoined(t *testing.T) {
	t.Parallel()

	text := extractArticleText(t, `<html><body>
		<div class="article-wrapper">
			<div class="article-head">
				<h1>A Complete Analysis of the Quarterly Results</h1>
				<p>The company reported revenue growth across all business segments this quarter.</p>
			</div>
			<div class="article-body">
				<p>Revenue grew twelve percent year over year, driven by the launch of the new product line.</p>
				<p>Operating margin expanded three hundred basis points as the company reduced structural costs.</p>
				<p>Management guided to continued double digit growth in the coming fiscal year.</p>
			</div>
			<div class="article-foot">
				<p>Jane Doe is a senior markets reporter covering technology and industrials.</p>
			</div>
		</div>
	</body></html>`)

	for _, fragment := range []string{
		"A Complete Analysis of the Quarterly Results",
		"Revenue grew twelve percent",
		"Operating margin expanded",
		"Management guided to continued",
		"Jane Doe is a senior markets reporter",
	} {
		if !strings.Contains(text, fragment) {
			t.Errorf("split article fragment %q missing from text:\n%s", fragment, text)
		}
	}
}

// TestSemanticArticleNotExpanded guards the boundary: an <article> container is
// returned as-is, so a sibling <aside> (even a content-bearing one) outside the
// <article> must not be pulled in.
func TestSemanticArticleNotExpanded(t *testing.T) {
	t.Parallel()

	text := extractArticleText(t, `<html><body>
		<article>
			<h1>Title</h1>
			<p>Body content one.</p>
			<p>Body content two.</p>
		</article>
		<aside><p>This related story is not part of the article body.</p></aside>
	</body></html>`)

	if strings.Contains(text, "This related story is not part") {
		t.Errorf("sibling aside outside <article> leaked into text:\n%s", text)
	}
}

// TestBodyLevelSplitNotExpanded guards the conservative boundary: when the
// winning node sits directly under <body> (no semantic or content wrapper), the
// expansion is skipped — a competing body-level sibling is more likely unrelated
// than a split of the same article. The article div wins here because its
// class carries a positive "article/content" bonus that <body> lacks, so the
// scorer picks the div rather than <body>, and the sibling must stay out.
func TestBodyLevelSplitNotExpanded(t *testing.T) {
	t.Parallel()

	text := extractArticleText(t, `<html><body>
		<div class="article-content">
			<p>Main article body paragraph one with enough length to win the scorer.</p>
			<p>Main article body paragraph two with enough length to win the scorer.</p>
			<p>Main article body paragraph three with enough length to win the scorer.</p>
		</div>
		<div><p>This is a separate callout box that should not be merged into the article.</p></div>
	</body></html>`)

	if !strings.Contains(text, "Main article body paragraph one") {
		t.Errorf("article body missing:\n%s", text)
	}
	if strings.Contains(text, "separate callout box") {
		t.Errorf("body-level sibling was merged into the article:\n%s", text)
	}
}

// ---------------------------------------------------------------------------
// P0-2 — landmarks inside vs outside primary content
// ---------------------------------------------------------------------------

func TestArticleInnerHeaderPreserved(t *testing.T) {
	t.Parallel()

	text := extractArticleText(t, `<html><body>
		<article>
			<header><h1>Headline</h1><p class="byline">By Jane Doe</p></header>
			<p>Body paragraph one.</p>
			<p>Body paragraph two.</p>
		</article>
	</body></html>`)

	if !strings.Contains(text, "By Jane Doe") {
		t.Errorf("article-inner <header> byline was stripped:\n%s", text)
	}
	if !strings.Contains(text, "Body paragraph two") {
		t.Errorf("article body missing:\n%s", text)
	}
}

func TestArticleInnerFooterPreserved(t *testing.T) {
	t.Parallel()

	text := extractArticleText(t, `<html><body>
		<article>
			<h1>Title</h1>
			<p>Body content.</p>
			<footer><p>About the author: a staff writer.</p></footer>
		</article>
	</body></html>`)

	if !strings.Contains(text, "About the author") {
		t.Errorf("article-inner <footer> author note was stripped:\n%s", text)
	}
}

func TestPageLevelFooterStillRemoved(t *testing.T) {
	t.Parallel()

	text := extractArticleText(t, `<html><body>
		<article>
			<h1>Title</h1>
			<p>Body content.</p>
		</article>
		<footer><p>Copyright 2026 Example Corp. All rights reserved.</p></footer>
	</body></html>`)

	if strings.Contains(text, "Copyright 2026") {
		t.Errorf("page-level <footer> leaked into article text:\n%s", text)
	}
	if !strings.Contains(text, "Body content") {
		t.Errorf("article body missing:\n%s", text)
	}
}

func TestArticleInnerAsidePreserved(t *testing.T) {
	t.Parallel()

	text := extractArticleText(t, `<html><body>
		<article>
			<h1>Title</h1>
			<p>Body content.</p>
			<aside><p>A pull-quote worth keeping.</p></aside>
		</article>
	</body></html>`)

	if !strings.Contains(text, "pull-quote worth keeping") {
		t.Errorf("article-inner <aside> pull-quote was stripped:\n%s", text)
	}
}

func TestMainInnerFooterPreserved(t *testing.T) {
	t.Parallel()

	text := extractArticleText(t, `<html><body>
		<main>
			<h1>Documentation</h1>
			<p>Body content.</p>
			<footer><p>Last updated 2026-10-25.</p></footer>
		</main>
	</body></html>`)

	if !strings.Contains(text, "Last updated") {
		t.Errorf("<main>-inner <footer> was stripped:\n%s", text)
	}
}

// TestDivWrapperInnerLandmarksPreserved covers the div-based extension of the
// P0-2 rule: a non-semantic div whose class carries a primary-content token
// (article/post/story/entry/body/main) acts as a content wrapper, so a
// byline <header> and author <footer> nested inside it survive.
func TestDivWrapperInnerLandmarksPreserved(t *testing.T) {
	t.Parallel()

	text := extractArticleText(t, `<html><body>
		<div class="entry-content">
			<header><p>By Jane Doe, staff reporter</p></header>
			<p>This is the first paragraph of body content.</p>
			<p>This is the second paragraph of body content.</p>
			<footer><p>About the author notes here.</p></footer>
		</div>
	</body></html>`)

	for _, fragment := range []string{
		"By Jane Doe, staff reporter",
		"This is the first paragraph",
		"About the author notes here",
	} {
		if !strings.Contains(text, fragment) {
			t.Errorf("div-wrapper inner landmark/body fragment %q missing:\n%s", fragment, text)
		}
	}
}

// TestPlainDivInnerLandmarksStillRemoved guards the boundary of the div-based
// extension: a div with NO primary-content token is not a content wrapper, so a
// <footer> inside it is still page chrome and stripped.
func TestPlainDivInnerLandmarksStillRemoved(t *testing.T) {
	t.Parallel()

	text := extractArticleText(t, `<html><body>
		<div class="layout">
			<p>Body content one.</p>
			<footer><p>Copyright 2026 Example Corp.</p></footer>
		</div>
	</body></html>`)

	if strings.Contains(text, "Copyright 2026") {
		t.Errorf("plain-div-inner <footer> leaked into text:\n%s", text)
	}
}

// TestArticleInnerNavStillRemoved guards the other half of the contract: <nav>
// is hard non-content regardless of position — a menu inside an <article> is
// still navigation, not prose.
func TestArticleInnerNavStillRemoved(t *testing.T) {
	t.Parallel()

	text := extractArticleText(t, `<html><body>
		<article>
			<nav><a href="/prev">Previous</a> <a href="/next">Next</a></nav>
			<p>Body content.</p>
		</article>
	</body></html>`)

	if strings.Contains(text, "Previous") || strings.Contains(text, "Next") {
		t.Errorf("article-inner <nav> leaked into article text:\n%s", text)
	}
}

// ---------------------------------------------------------------------------
// P0-3 — class/ARIA-based hiding
// ---------------------------------------------------------------------------

func TestHiddenClassContentStripped(t *testing.T) {
	t.Parallel()

	text := extractArticleText(t, `<html><body>
		<article>
			<h1>Title</h1>
			<p>Visible body content.</p>
			<div class="sr-only">Screen reader only filler.</div>
			<div class="d-none">Bootstrap hidden filler.</div>
			<div aria-hidden="true">ARIA hidden filler.</div>
		</article>
	</body></html>`)

	if !strings.Contains(text, "Visible body content") {
		t.Errorf("visible content missing:\n%s", text)
	}
	for _, filler := range []string{"Screen reader only", "Bootstrap hidden", "ARIA hidden"} {
		if strings.Contains(text, filler) {
			t.Errorf("hidden class/ARIA content %q leaked into text:\n%s", filler, text)
		}
	}
}

func TestResponsiveHiddenClassPreserved(t *testing.T) {
	t.Parallel()

	text := extractArticleText(t, `<html><body>
		<article>
			<h1>Title</h1>
			<p>Visible body content.</p>
			<div class="hidden-sm">Visible on larger screens.</div>
		</article>
	</body></html>`)

	if !strings.Contains(text, "Visible on larger screens") {
		t.Errorf("responsive-visibility class was wrongly stripped:\n%s", text)
	}
}

// ---------------------------------------------------------------------------
// P2 — score-guarded climb and listing pages
// ---------------------------------------------------------------------------

// TestScoreClimbExcludesPenalizedSiblingChrome locks the climb's score guard.
// The wrapper holds the story and an unmarked short-link block; the junk
// pushes the wrapper's link density over the penalty threshold, so the
// wrapper scores below the story body and the selection must stay on the
// story body rather than widen into the junk.
func TestScoreClimbExcludesPenalizedSiblingChrome(t *testing.T) {
	t.Parallel()

	var links strings.Builder
	for _, label := range []string{"Markets", "Earnings", "Deals", "Tech", "Funds", "Rates", "Crypto", "Energy", "Retail", "IPOs", "FX", "Bonds"} {
		links.WriteString(`<a href="/x">` + label + `</a> `)
	}

	text := extractArticleText(t, `<html><body>
<div class="post-wrap">
  <div class="story-body">
    <h1>The Story Headline</h1>
    <p>`+longParagraph(220)+`</p>
    <p>`+longParagraph(180)+`</p>
  </div>
  <div class="coverage">`+links.String()+`</div>
</div>
</body></html>`)

	if !strings.Contains(text, "The Story Headline") || !strings.Contains(text, "measurement campaign") {
		t.Errorf("story body missing from extraction:\n%s", text)
	}
	for _, junk := range []string{"Markets", "Earnings", "Crypto"} {
		if strings.Contains(text, junk) {
			t.Errorf("penalized link-chrome sibling leaked into extraction:\n%s", text)
		}
	}
}

// TestMultiLevelWrapperHeadlineCaptured locks that a headline sitting outside
// the innermost content block (two wrapper levels up, as a sibling of the
// block's parent) still reaches the output.
func TestMultiLevelWrapperHeadlineCaptured(t *testing.T) {
	t.Parallel()

	text := extractArticleText(t, `<html><body>
<div class="page"><div class="entry">
  <div class="headline-block"><h1>Deep Wrapper Headline</h1></div>
  <div class="entry-inner"><div class="prose">
    <p>`+longParagraph(240)+`</p>
    <p>`+longParagraph(200)+`</p>
  </div></div>
</div></div>
</body></html>`)

	if !strings.Contains(text, "Deep Wrapper Headline") {
		t.Errorf("headline outside the innermost block missing:\n%s", text)
	}
	if !strings.Contains(text, "measurement campaign") {
		t.Errorf("body missing:\n%s", text)
	}
}

func TestListingPageDetected(t *testing.T) {
	t.Parallel()

	result := extractFor(t, `<html><head><title>Tech News - Example Wire</title></head><body>
<nav><a href="/">Home</a> <a href="/tech">Tech</a></nav>
<div class="feed">
  <article><h2><a href="/1">First card headline about quantum chips</a></h2><p>Excerpt text for the first card, summarizing the story in a couple of sentences.</p></article>
  <article><h2><a href="/2">Second card headline about climate models</a></h2><p>Excerpt text for the second card, summarizing the story in a couple of sentences.</p></article>
  <article><h2><a href="/3">Third card headline about market moves</a></h2><p>Excerpt text for the third card, summarizing the story in a couple of sentences.</p></article>
  <article><h2><a href="/4">Fourth card headline about space launches</a></h2><p>Excerpt text for the fourth card, summarizing the story in a couple of sentences.</p></article>
</div>
<footer><p>© Example Wire</p></footer>
</body></html>`)

	if result.ExtractionMethod != "listing" {
		t.Errorf("ExtractionMethod = %q, want %q for a card feed page", result.ExtractionMethod, "listing")
	}
	for _, want := range []string{"quantum chips", "climate models", "market moves", "space launches"} {
		if !strings.Contains(result.Text, want) {
			t.Errorf("card %q missing from listing extraction:\n%s", want, result.Text)
		}
	}
	if strings.Contains(result.Text, "Home") {
		t.Errorf("nav leaked into listing extraction:\n%s", result.Text)
	}
	if result.Confidence != 0.6 {
		t.Errorf("Confidence = %v, want 0.6 for listing extraction", result.Confidence)
	}
}

func TestListingNotTriggeredForFullArticles(t *testing.T) {
	t.Parallel()

	result := extractFor(t, `<html><body>
<div class="multi">
  <article><h2>First Full Post</h2><p>`+longParagraph(300)+`</p><p>`+longParagraph(200)+`</p></article>
  <article><h2>Second Full Post</h2><p>`+longParagraph(300)+`</p><p>`+longParagraph(200)+`</p></article>
  <article><h2>Third Full Post</h2><p>`+longParagraph(300)+`</p><p>`+longParagraph(200)+`</p></article>
</div>
</body></html>`)

	if result.ExtractionMethod != "article" {
		t.Errorf("ExtractionMethod = %q, want %q: full-text posts are not a card listing", result.ExtractionMethod, "article")
	}
}

func TestListingNotTriggeredWhenCardsNotDominant(t *testing.T) {
	t.Parallel()

	result := extractFor(t, `<html><body>
<article class="main-post"><h1>The Real Article</h1><p>`+longParagraph(300)+`</p><p>`+longParagraph(250)+`</p></article>
<div class="more">
  <article><a href="/a">Card one headline</a></article>
  <article><a href="/b">Card two headline</a></article>
  <article><a href="/c">Card three headline</a></article>
</div>
</body></html>`)

	if result.ExtractionMethod != "article" {
		t.Errorf("ExtractionMethod = %q, want %q: a stray card section must not turn an article page into a listing", result.ExtractionMethod, "article")
	}
	if !strings.Contains(result.Text, "The Real Article") || !strings.Contains(result.Text, "measurement campaign") {
		t.Errorf("main article missing:\n%s", result.Text)
	}
	if strings.Contains(result.Text, "Card one headline") {
		t.Errorf("non-dominant card section leaked:\n%s", result.Text)
	}
}

// ---------------------------------------------------------------------------
// Custom-scorer ArticleFinder hook (from p3_features_test.go)
// ---------------------------------------------------------------------------

// pickScorer is a custom Scorer that delegates selection to an ArticleFinder
// hook returning a fixed node, proving the hook bypasses the per-element
// Score enumeration.
type pickScorer struct{}

func (pickScorer) Score(html.ContentNode) int         { return 0 }
func (pickScorer) ShouldRemove(html.ContentNode) bool { return false }

func (pickScorer) FindArticleNode(root html.ContentNode) html.ContentNode {
	var found html.ContentNode
	var walk func(html.ContentNode)
	walk = func(n html.ContentNode) {
		if n == nil || found != nil {
			return
		}
		if n.Type() == "element" && n.AttrValue("id") == "custom-pick" {
			found = n
			return
		}
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			walk(c)
		}
	}
	walk(root)
	return found
}

func TestArticleFinderHookSelectsCustomNode(t *testing.T) {
	t.Parallel()

	cfg := html.DefaultConfig()
	cfg.Scorer = pickScorer{}
	p, err := html.New(cfg)
	if err != nil {
		t.Fatalf("New(cfg) failed: %v", err)
	}
	defer func() { _ = p.Close() }()

	result, err := p.Extract([]byte(`<html><body>
<div id="other"><p>Unrelated block that must not be extracted.</p></div>
<div id="custom-pick"><h2>Picked Section</h2><p>The custom scorer chose this node.</p></div>
</body></html>`))
	if err != nil {
		t.Fatalf("Extract() failed: %v", err)
	}

	if result.ExtractionMethod != "article" {
		t.Errorf("ExtractionMethod = %q, want %q", result.ExtractionMethod, "article")
	}
	if !strings.Contains(result.Text, "The custom scorer chose this node.") {
		t.Errorf("hook-selected node missing from extraction:\n%s", result.Text)
	}
	if strings.Contains(result.Text, "Unrelated block") {
		t.Errorf("non-selected sibling leaked into extraction:\n%s", result.Text)
	}
}

// nilPickScorer's hook always returns nil: the scorer's "no article" verdict
// must fall back to <body> rather than running the Score enumeration.
type nilPickScorer struct{}

func (nilPickScorer) Score(html.ContentNode) int         { return 999 }
func (nilPickScorer) ShouldRemove(html.ContentNode) bool { return false }
func (nilPickScorer) FindArticleNode(html.ContentNode) html.ContentNode {
	return nil
}

func TestArticleFinderHookNilFallsBackToBody(t *testing.T) {
	t.Parallel()

	cfg := html.DefaultConfig()
	cfg.Scorer = nilPickScorer{}
	p, err := html.New(cfg)
	if err != nil {
		t.Fatalf("New(cfg) failed: %v", err)
	}
	defer func() { _ = p.Close() }()

	result, err := p.Extract([]byte(`<html><body><div><p>Body fallback content.</p></div></body></html>`))
	if err != nil {
		t.Fatalf("Extract() failed: %v", err)
	}
	if result.ExtractionMethod != "body" {
		t.Errorf("ExtractionMethod = %q, want %q (hook's nil verdict must not re-run enumeration)", result.ExtractionMethod, "body")
	}
}

// ---------------------------------------------------------------------------
// role=presentation layout tables (from p3_features_test.go)
// ---------------------------------------------------------------------------

func TestPresentationRoleTableFlattened(t *testing.T) {
	t.Parallel()

	const table = `<table%role%><tr><td>Label One</td><td>Value One</td></tr><tr><td>Label Two</td><td>Value Two</td></tr></table>`

	presented := extractArticleText(t, strings.ReplaceAll(table, "%role%", ` role="presentation"`))
	for _, cell := range []string{"Label One", "Value One", "Label Two", "Value Two"} {
		if !strings.Contains(presented, cell) {
			t.Errorf("cell %q missing from flattened layout table:\n%s", cell, presented)
		}
	}
	if strings.Contains(presented, "|") {
		t.Errorf("role=presentation table rendered as a Markdown data table:\n%s", presented)
	}

	// Control: the same table without the role still renders as a data table.
	data := extractArticleText(t, strings.ReplaceAll(table, "%role%", ""))
	if !strings.Contains(data, "|") {
		t.Errorf("table without presentational role lost its Markdown table rendering:\n%s", data)
	}
}
