package html

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// readerDoc is a full semantic page used by several reader-mode tests:
// header with a nested nav (the canonical masthead shape), main article with
// an ad div inside, aside, footer, plus a related-links block the scorer
// removes outright.
const readerDoc = `<html><head><title>Example News - Example.com</title></head><body>
	<header>
		<nav class="site-nav"><a href="/">Home</a> <a href="/tech">Tech</a> <a href="/finance">Finance</a></nav>
	</header>
	<main>
		<article>
			<h1>Quarterly Results Announced</h1>
			<p>The company posted strong quarterly results, beating every analyst estimate by a wide margin. Revenue grew across all segments. <a href="/news/related">Related coverage</a> is available separately.</p>
			<div class="ad-slot"><a href="/buy1">Buy</a><a href="/buy2">Buy2</a><a href="/buy3">Buy3</a></div>
		</article>
	</main>
	<aside><p>Sidebar: market data summary.</p></aside>
	<div class="related-posts"><a href="/p1">Post 1</a><a href="/p2">Post 2</a><a href="/p3">Post 3</a></div>
	<footer><p>© 2026 Example.com <a href="/privacy">Privacy</a></p></footer>
</body></html>`

// readerSectionKinds returns the ordered section kinds of a ReadResult.
func readerSectionKinds(t *testing.T, page *ReadResult) []SectionKind {
	t.Helper()
	kinds := make([]SectionKind, len(page.Sections))
	for i := range page.Sections {
		kinds[i] = page.Sections[i].Kind
	}
	return kinds
}

func TestReadPageSections(t *testing.T) {
	t.Parallel()

	page, err := ReadPage([]byte(readerDoc))
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}

	want := []SectionKind{SectionHeader, SectionMain, SectionAside, SectionFooter}
	if got := readerSectionKinds(t, page); !slices.Equal(got, want) {
		t.Fatalf("section kinds = %v, want %v", got, want)
	}
	if page.Title != "Example News" {
		t.Fatalf("Title = %q, want %q (site-name suffix stripped)", page.Title, "Example News")
	}

	// The header section keeps its nested nav's links as markdown — the
	// default InlineLinkFormat "none" is treated as "markdown" in reader
	// mode, so links never silently vanish from the default output.
	header := page.Sections[0]
	for _, want := range []string{"[Home](/)", "[Tech](/tech)", "[Finance](/finance)"} {
		if !strings.Contains(header.Text, want) {
			t.Errorf("header text missing nav link %q: %q", want, header.Text)
		}
	}
	if strings.Contains(header.Text, "<a") {
		t.Errorf("default config leaked HTML anchors into header text: %q", header.Text)
	}

	// The main section keeps the article, drops the ad div inside it, and
	// carries the h1 marker.
	main := page.Sections[1]
	if !strings.Contains(main.Text, "# Quarterly Results Announced") {
		t.Errorf("main text missing heading marker: %q", main.Text)
	}
	if strings.Contains(main.Text, "Buy") {
		t.Errorf("ad div inside article leaked into main text: %q", main.Text)
	}
	if !strings.Contains(main.Text, "Related coverage") {
		t.Errorf("inline link text missing from main: %q", main.Text)
	}

	// The related-posts block (removal-classed residual) is gone entirely;
	// the footer keeps its privacy link.
	for _, sec := range page.Sections {
		if strings.Contains(sec.Text, "Post 1") {
			t.Errorf("related-posts residual survived as %s section", sec.Kind)
		}
	}
	if !strings.Contains(page.Sections[3].Text, "Privacy") {
		t.Errorf("footer text missing privacy link: %q", page.Sections[3].Text)
	}
}

func TestReadPageLinkFormatMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		linkFormat string
		wantInNav  string
		wantAbsent string
	}{
		{"markdown", "markdown", "[Home](https://cdn.example/)", ""},
		{"html", "html", `<a href="https://cdn.example/">Home</a>`, ""},
		// "none" (the DefaultConfig value, right for Extract) is normalized
		// to markdown in reader mode: hyperlinks are the reading view's
		// contract and must never silently vanish from the output.
		{"none treated as markdown", "none", "[Home](https://cdn.example/)", "<a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := DefaultConfig()
			cfg.InlineLinkFormat = tt.linkFormat
			cfg.MaxCacheEntries = 0
			// A <base> tag makes the resolution deterministic and exercises
			// the absolute-URL requirement for text-only consumers.
			doc := `<html><head><base href="https://cdn.example/dir/"></head><body>
				<nav><a href="/">Home</a></nav>
				<main><p>Article prose with enough words to anchor the page.</p></main>
				</body></html>`

			page, err := ReadPage([]byte(doc), cfg)
			if err != nil {
				t.Fatalf("ReadPage: %v", err)
			}
			nav := page.Sections[0]
			if !strings.Contains(nav.Text, tt.wantInNav) {
				t.Fatalf("nav text = %q, want it to contain %q", nav.Text, tt.wantInNav)
			}
			if tt.wantAbsent != "" && strings.Contains(nav.Text, tt.wantAbsent) {
				t.Fatalf("nav text = %q, want it to NOT contain %q", nav.Text, tt.wantAbsent)
			}
			if len(nav.Links) == 0 || nav.Links[0].URL != "https://cdn.example/" {
				t.Fatalf("nav links = %+v, want resolved absolute URL", nav.Links)
			}
		})
	}
}

func TestReadPageToMarkdown(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	doc := `<html><head><title>My Page - Site</title></head><body>
		<nav><a href="/a">Alpha</a> <a href="/b">Beta</a></nav>
		<main><h1>Head</h1><p>Body text with <a href="sec/page.html">a link</a> inside. Plus enough extra prose that the main region is unambiguous.</p></main>
		<footer><p>© Site</p></footer>
		</body></html>`
	// og:url sets the document base; NormalizeBaseURL reduces it to the origin.
	doc = strings.Replace(doc, "<head>", `<head><meta property="og:url" content="https://www.example.com/articles/1">`, 1)

	out, err := ReadPageToMarkdown([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("ReadPageToMarkdown: %v", err)
	}

	for _, want := range []string{
		"# My Page",                          // title heading
		"# Navigation",                       // section heading
		"[Alpha](https://www.example.com/a)", // resolved relative link, markdown style
		"# Main Content",
		"# Head", // content h1
		"[a link](https://www.example.com/sec/page.html)",
		"# Footer",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "[LINK:") || strings.Contains(out, "[IMAGE:") {
		t.Errorf("placeholder token leaked into output:\n%s", out)
	}
}

func TestReadPageURLResolutionToggle(t *testing.T) {
	t.Parallel()

	doc := `<html><head><base href="https://base.example/d/"></head><body>
		<nav><a href="x.html">X</a></nav>
		<main><p>Prose that makes this region the page's main content beyond any doubt.</p></main>
		</body></html>`

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	cfg.BaseURL = "https://cfg.example/dir/"
	page, err := ReadPage([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	if got := page.Sections[0].Links[0].URL; got != "https://cfg.example/dir/x.html" {
		t.Fatalf("resolved URL = %q, want BaseURL to win over <base>", got)
	}

	cfg.ResolveRelativeURLs = false
	page, err = ReadPage([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("ReadPage (no resolve): %v", err)
	}
	if got := page.Sections[0].Links[0].URL; got != "x.html" {
		t.Fatalf("raw URL = %q, want unchanged relative form", got)
	}
}

func TestReadPageHiddenSectionsDropped(t *testing.T) {
	t.Parallel()

	// The responsive-duplicate mobile nav is d-none hidden and must vanish;
	// the desktop nav stays. The hidden root check runs on landmark sections,
	// which element-level cleaning never removes.
	doc := `<html><body>
		<nav class="mobile-only d-none"><a href="/">Home</a><a href="/x">X</a><a href="/y">Y</a></nav>
		<nav class="desktop"><a href="/">Home</a><a href="/z">Z</a></nav>
		<main><p>Article prose dominating the page text by a comfortable margin for the classifier.</p></main>
		</body></html>`

	page, err := ReadPage([]byte(doc))
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	navCount := 0
	for _, sec := range page.Sections {
		if sec.Kind == SectionNavigation {
			navCount++
			if !strings.Contains(sec.Text, "Z") {
				t.Errorf("desktop nav lost its link: %q", sec.Text)
			}
		}
		if strings.Contains(sec.Text, "X") && strings.Contains(sec.Text, "Y") && !strings.Contains(sec.Text, "Z") {
			t.Errorf("hidden mobile nav survived: %q", sec.Text)
		}
	}
	if navCount != 1 {
		t.Fatalf("navigation sections = %d, want 1", navCount)
	}
}

func TestReadPageChromeKeepsNavClassChildren(t *testing.T) {
	t.Parallel()

	// The regression this file exists to pin: content cleaning would strip a
	// nav's own "nav-menu" children (class matches the scorer's removal
	// vocabulary), so chrome sections get the hidden-only cleaner. The main
	// region keeps full article-grade cleaning.
	doc := `<html><body>
		<nav><ul class="nav-menu"><li><a href="/">Home</a></li><li><a href="/a">A</a></li></ul></nav>
		<main><p>Real article content, several words long, clearly the dominant text of the page for classification.</p></main>
		</body></html>`

	page, err := ReadPage([]byte(doc))
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	nav := page.Sections[0]
	if !strings.Contains(nav.Text, "Home") || !strings.Contains(nav.Text, "A") {
		t.Fatalf("nav-menu children were stripped from the nav section: %q", nav.Text)
	}
	if len(nav.Links) != 2 {
		t.Fatalf("nav links = %+v, want 2 entries", nav.Links)
	}
	if nav.Links[0].Position != 1 || nav.Links[1].Position != 2 {
		t.Fatalf("nav link positions = %d,%d, want 1,2 (per-section numbering)",
			nav.Links[0].Position, nav.Links[1].Position)
	}
}

func TestReadPageAdjacentSectionMerge(t *testing.T) {
	t.Parallel()

	doc := `<html><body>
		<nav><a href="/a">A</a></nav>
		<nav><a href="/b">B</a></nav>
		<main><p>One main region with enough words to dominate and be selected without ambiguity whatsoever.</p></main>
		<footer><p>© 2026</p></footer>
		</body></html>`

	page, err := ReadPage([]byte(doc))
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	want := []SectionKind{SectionNavigation, SectionMain, SectionFooter}
	if got := readerSectionKinds(t, page); !slices.Equal(got, want) {
		t.Fatalf("kinds = %v, want %v (adjacent navs must merge)", got, want)
	}
	nav := page.Sections[0]
	if !strings.Contains(nav.Text, "A") || !strings.Contains(nav.Text, "B") {
		t.Fatalf("merged nav text = %q, want both menus", nav.Text)
	}
	if len(nav.Links) != 2 {
		t.Fatalf("merged nav links = %d, want 2", len(nav.Links))
	}
}

func TestReadPageEmptyAndErrors(t *testing.T) {
	t.Parallel()

	page, err := ReadPage([]byte("   \n\t "))
	if err != nil {
		t.Fatalf("blank input: %v", err)
	}
	if page == nil || len(page.Sections) != 0 || page.RenderText() != "" {
		t.Fatalf("blank input produced %+v", page)
	}

	p, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := p.ReadPage([]byte("<p>x</p>")); !errors.Is(err, ErrProcessorClosed) {
		t.Fatalf("closed processor error = %v, want ErrProcessorClosed", err)
	}

	cfg := DefaultConfig()
	cfg.MaxInputSize = 16
	cfg.MaxCacheEntries = 0
	if _, err := ReadPage([]byte("<html><body>way more than sixteen bytes of content here</body></html>"), cfg); !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("oversize error = %v, want ErrInputTooLarge", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadPageWithContext(ctx, []byte("<p>x</p>")); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled context error = %v, want context.Canceled", err)
	}
}

func TestReadPageCustomScorerFallback(t *testing.T) {
	t.Parallel()

	// A custom scorer that nominates the div soup "article" via Score: the
	// findArticle adapter must route reader-mode fallback through the same
	// selection path, so the custom pick becomes the main region.
	doc := `<html><body>
		<div><a href="/">Home</a></div>
		<div data-marker="article"><p>THE ARTICLE BODY the custom scorer picks, with a reasonable amount of text inside it for the test.</p></div>
		<div><p>Tail notes.</p></div>
		</body></html>`

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	cfg.Scorer = bodyDivScorer{}

	page, err := ReadPage([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	found := false
	for _, sec := range page.Sections {
		if sec.Kind == SectionMain && strings.Contains(sec.Text, "THE ARTICLE BODY") {
			found = true
		}
	}
	if !found {
		t.Fatalf("custom-scorer article not promoted to main: %+v", readerSectionKinds(t, page))
	}
}

// bodyDivScorer scores only divs whose text mentions the marker sentence, so
// the fallback path must select the article div through the custom scorer.
type bodyDivScorer struct{}

func (bodyDivScorer) Score(n ContentNode) int {
	if n.Data() == "div" && strings.Contains(n.AttrValue("data-marker"), "article") {
		return 1 << 20
	}
	return 0
}

func (bodyDivScorer) ShouldRemove(ContentNode) bool { return false }

func TestReadPagePreserveLinksOff(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	cfg.PreserveLinks = false
	cfg.InlineLinkFormat = "none"

	page, err := ReadPage([]byte(readerDoc), cfg)
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	for i, sec := range page.Sections {
		if sec.Links != nil {
			t.Fatalf("section %d carries links with PreserveLinks=false: %+v", i, sec.Links)
		}
	}
}

func TestReadPageProcessingTimeRecorded(t *testing.T) {
	t.Parallel()

	start := time.Now()
	page, err := ReadPage([]byte(readerDoc))
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	// Lower bound is >= 0, not > 0: Go's monotonic clock on Windows reads the
	// interrupt-time page, whose granularity is the system timer tick — a
	// sub-tick extraction legitimately measures 0 there. The upper bound is
	// the real invariant: the recorded time cannot exceed the wall-clock span
	// of the call.
	if page.ProcessingTime < 0 || page.ProcessingTime > time.Since(start)+time.Second {
		t.Fatalf("ProcessingTime = %v, want a plausible elapsed duration", page.ProcessingTime)
	}
}

func TestReadPageChromeTableCounterAlignment(t *testing.T) {
	t.Parallel()

	// Sanitization is OFF so <template> survives: unlike <noscript> (whose
	// content x/net/html parses as raw text, so it can hold no anchor
	// elements), a template's children are real parsed nodes. In chrome mode
	// the collector prunes raw-text subtrees and the token-aware table cell
	// emitter must prune them identically — otherwise the counters drift and
	// the link after the table resolves to a nonexistent position, losing its
	// URL in the rendered text.
	doc := `<html><body>
		<nav>
			<a href="/first">First</a>
			<table><tr><td><a href="/in-table">InTable</a> <template><a href="/tpl">T</a></template></td></tr></table>
			<a href="/last">Last</a>
		</nav>
		<main><p>Article prose of a comfortable length so the two regions classify cleanly.</p></main>
		</body></html>`

	cfg := DefaultConfig()
	cfg.EnableSanitization = false
	cfg.InlineLinkFormat = "markdown"
	cfg.MaxCacheEntries = 0

	page, err := ReadPage([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	var nav *PageSection
	for i := range page.Sections {
		if page.Sections[i].Kind == SectionNavigation {
			nav = &page.Sections[i]
			break
		}
	}
	if nav == nil {
		t.Fatal("no navigation section found")
	}
	if strings.Contains(nav.Text, "[LINK:") || strings.Contains(nav.Text, "[IMAGE:") {
		t.Errorf("placeholder token leaked into nav text: %q", nav.Text)
	}
	if !strings.Contains(nav.Text, "[First](/first)") {
		t.Errorf("link before table lost its markdown form: %q", nav.Text)
	}
	if !strings.Contains(nav.Text, "[Last](/last)") {
		t.Errorf("link after table lost its URL (counter desync): %q", nav.Text)
	}
	if strings.Contains(nav.Text, "/tpl") {
		t.Errorf("template anchor URL leaked into nav text: %q", nav.Text)
	}
}

func TestReadPageFromFileToMarkdown(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(path, []byte(readerDoc), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	// Package-level, config-passing form.
	out, err := ReadPageFromFileToMarkdown(path)
	if err != nil {
		t.Fatalf("ReadPageFromFileToMarkdown: %v", err)
	}
	for _, want := range []string{
		"# Example News",
		"# Header", // the nav is nested in the header, so the section label is Header
		"[Tech](/tech)",
		"# Main Content",
		"# Quarterly Results Announced",
		"# Footer",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}

	// Processor form on the same file.
	p, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()
	out2, err := p.ReadPageFromFileToMarkdown(path)
	if err != nil {
		t.Fatalf("Processor.ReadPageFromFileToMarkdown: %v", err)
	}
	if out2 != out {
		t.Errorf("processor and package-level outputs differ (%d vs %d bytes)", len(out2), len(out))
	}
}

func TestReadPageFromFile(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(path, []byte(readerDoc), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	page, err := ReadPageFromFile(path)
	if err != nil {
		t.Fatalf("ReadPageFromFile: %v", err)
	}
	want := []SectionKind{SectionHeader, SectionMain, SectionAside, SectionFooter}
	if got := readerSectionKinds(t, page); !slices.Equal(got, want) {
		t.Fatalf("section kinds = %v, want %v", got, want)
	}
	if page.Title != "Example News" {
		t.Fatalf("Title = %q, want %q", page.Title, "Example News")
	}

	// Processor form produces the same view.
	p, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()
	page2, err := p.ReadPageFromFile(path)
	if err != nil {
		t.Fatalf("Processor.ReadPageFromFile: %v", err)
	}
	if page2.Title != page.Title || page2.RenderText() != page.RenderText() {
		t.Errorf("processor and package-level results differ")
	}
}

// TestReadPageFromFileErrors table-drives the error contract shared by every
// reader-mode FromFile entry point — missing file, empty path, path
// traversal, a context cancelled before the read, and a closed processor. It
// merges the former TestReadPageFromFileErrors /
// TestReadPageFromFileToMarkdownErrors pair, whose checks ran byte-for-byte
// in parallel modulo the function under test, and extends the same contract
// to the Text variant.
func TestReadPageFromFileErrors(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "missing.html")
	calls := []struct {
		name string
		call func(path string) error
	}{
		{"ReadPageFromFile", func(path string) error { _, err := ReadPageFromFile(path); return err }},
		{"ReadPageFromFileToMarkdown", func(path string) error { _, err := ReadPageFromFileToMarkdown(path); return err }},
		{"ReadPageFromFileToMarkdownText", func(path string) error { _, err := ReadPageFromFileToMarkdownText(path); return err }},
	}
	badPaths := []struct {
		name     string
		path     string
		wantErr  error  // sentinel checked via errors.Is
		wantText string // substring of err.Error() when no sentinel applies
	}{
		{"missing file", missing, ErrFileNotFound, ""},
		{"empty path", "", ErrInvalidFilePath, ""},
		// Forward slashes on purpose: filepath.Clean accepts them on Windows
		// and treats them as separators, so the fixture is a traversal on
		// every supported platform (a "..\\.." literal is a single filename
		// on Linux).
		{"path traversal", "../../evil.html", nil, "path traversal"},
	}
	for _, c := range calls {
		for _, bad := range badPaths {
			err := c.call(bad.path)
			switch {
			case bad.wantErr != nil:
				if !errors.Is(err, bad.wantErr) {
					t.Errorf("%s / %s: error = %v, want %v", c.name, bad.name, err, bad.wantErr)
				}
			case err == nil || !strings.Contains(err.Error(), bad.wantText):
				t.Errorf("%s / %s: error = %v, want a rejection containing %q", c.name, bad.name, err, bad.wantText)
			}
		}
	}

	// A cancelled context wins over the file error: the early check inside
	// each WithContext variant observes ctx before the uncancellable file
	// read, matching the ExtractFromFileWithContext precedence contract.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, c := range []struct {
		name string
		call func(context.Context) error
	}{
		{"ReadPageFromFileWithContext", func(ctx context.Context) error {
			_, err := ReadPageFromFileWithContext(ctx, missing)
			return err
		}},
		{"ReadPageFromFileToMarkdownWithContext", func(ctx context.Context) error {
			_, err := ReadPageFromFileToMarkdownWithContext(ctx, missing)
			return err
		}},
		{"ReadPageFromFileToMarkdownTextWithContext", func(ctx context.Context) error {
			_, err := ReadPageFromFileToMarkdownTextWithContext(ctx, missing)
			return err
		}},
	} {
		if err := c.call(ctx); !errors.Is(err, context.Canceled) {
			t.Errorf("%s: cancelled-context error = %v, want context.Canceled", c.name, err)
		}
	}

	// Every FromFile form — and the byte-based markdown renderers, whose
	// only remaining guard is this one — reports a closed processor.
	p, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := p.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	for _, c := range []struct {
		name string
		call func() error
	}{
		{"Processor.ReadPageFromFile", func() error { _, err := p.ReadPageFromFile(missing); return err }},
		{"Processor.ReadPageFromFileToMarkdown", func() error { _, err := p.ReadPageFromFileToMarkdown(missing); return err }},
		{"Processor.ReadPageFromFileToMarkdownText", func() error { _, err := p.ReadPageFromFileToMarkdownText(missing); return err }},
		{"Processor.ReadPageToMarkdown", func() error { _, err := p.ReadPageToMarkdown([]byte(readerDoc)); return err }},
		{"Processor.ReadPageToMarkdownText", func() error { _, err := p.ReadPageToMarkdownText([]byte(readerDoc)); return err }},
	} {
		if err := c.call(); !errors.Is(err, ErrProcessorClosed) {
			t.Errorf("%s: closed-processor error = %v, want ErrProcessorClosed", c.name, err)
		}
	}
}

// TestReadPageContextVariants table-drives the context contract of every
// ReadPage*WithContext entry point — all six Processor methods and all five
// package-level wrappers. Before this table the suite cancelled only three
// package-level calls; the six Processor methods and the package-level
// ReadPageToMarkdownWithContext / ReadPageToMarkdownTextWithContext /
// ReadPageFromFileToMarkdownTextWithContext had no test at all, and no
// ReadPage* path had ever observed a deadline. Each entry runs three times:
// an already-cancelled context and an expired deadline must fail with their
// context error before any work starts, and a live context must succeed with
// non-empty output.
func TestReadPageContextVariants(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(path, []byte(readerDoc), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	p, err := New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	calls := []struct {
		name string
		run  func(ctx context.Context) (string, error)
	}{
		{"Processor.ReadPageWithContext", func(ctx context.Context) (string, error) {
			page, err := p.ReadPageWithContext(ctx, []byte(readerDoc))
			if err != nil {
				return "", err
			}
			return page.RenderText(), nil
		}},
		{"Processor.ReadPageFromFileWithContext", func(ctx context.Context) (string, error) {
			page, err := p.ReadPageFromFileWithContext(ctx, path)
			if err != nil {
				return "", err
			}
			return page.RenderText(), nil
		}},
		{"Processor.ReadPageToMarkdownWithContext", func(ctx context.Context) (string, error) {
			return p.ReadPageToMarkdownWithContext(ctx, []byte(readerDoc))
		}},
		{"Processor.ReadPageToMarkdownTextWithContext", func(ctx context.Context) (string, error) {
			return p.ReadPageToMarkdownTextWithContext(ctx, []byte(readerDoc))
		}},
		{"Processor.ReadPageFromFileToMarkdownWithContext", func(ctx context.Context) (string, error) {
			return p.ReadPageFromFileToMarkdownWithContext(ctx, path)
		}},
		{"Processor.ReadPageFromFileToMarkdownTextWithContext", func(ctx context.Context) (string, error) {
			return p.ReadPageFromFileToMarkdownTextWithContext(ctx, path)
		}},
		{"ReadPageWithContext", func(ctx context.Context) (string, error) {
			page, err := ReadPageWithContext(ctx, []byte(readerDoc))
			if err != nil {
				return "", err
			}
			return page.RenderText(), nil
		}},
		{"ReadPageToMarkdownWithContext", func(ctx context.Context) (string, error) {
			return ReadPageToMarkdownWithContext(ctx, []byte(readerDoc))
		}},
		{"ReadPageToMarkdownTextWithContext", func(ctx context.Context) (string, error) {
			return ReadPageToMarkdownTextWithContext(ctx, []byte(readerDoc))
		}},
		{"ReadPageFromFileToMarkdownWithContext", func(ctx context.Context) (string, error) {
			return ReadPageFromFileToMarkdownWithContext(ctx, path)
		}},
		{"ReadPageFromFileToMarkdownTextWithContext", func(ctx context.Context) (string, error) {
			return ReadPageFromFileToMarkdownTextWithContext(ctx, path)
		}},
	}

	for _, tc := range calls {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := tc.run(ctx); !errors.Is(err, context.Canceled) {
				t.Errorf("cancelled context: error = %v, want context.Canceled", err)
			}

			deadlineCtx, deadlineCancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			defer deadlineCancel()
			if _, err := tc.run(deadlineCtx); !errors.Is(err, context.DeadlineExceeded) {
				t.Errorf("expired deadline: error = %v, want context.DeadlineExceeded", err)
			}

			out, err := tc.run(context.Background())
			if err != nil {
				t.Fatalf("live context: %v", err)
			}
			if out == "" {
				t.Errorf("live context: output is empty")
			}
		})
	}
}

func TestReadPageDedupesDuplicateChrome(t *testing.T) {
	t.Parallel()

	// Two visible copies of the same menu (plain and div-wrapped — the
	// whitespace-stripped signature must treat them alike), a different
	// menu that must survive, duplicated text-only asides, and duplicated
	// footers.
	doc := `<html><body>
		<nav><a href="/h">Home</a> <a href="/t">Tech</a></nav>
		<nav><div><a href="/h">Home</a></div><div><a href="/t">Tech</a></div></nav>
		<nav><a href="/a">About</a> <a href="/c">Contact</a></nav>
		<main><p>Article prose long enough to be the dominant region of the page by a comfortable margin.</p></main>
		<aside><p>Stock ticker widget.</p></aside>
		<aside><p>Stock ticker widget.</p></aside>
		<footer><p>© 2026 Example</p></footer>
		<footer><p>© 2026 Example</p></footer>
		</body></html>`

	page, err := ReadPage([]byte(doc))
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	// The duplicate Home/Tech copy is dropped; the distinct About/Contact
	// menu survives and then merges with the kept copy (adjacent same-kind
	// merge is by design), so one navigation section holds both menus — with
	// Home appearing exactly once.
	want := []SectionKind{SectionNavigation, SectionMain, SectionAside, SectionFooter}
	if got := readerSectionKinds(t, page); !slices.Equal(got, want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
	nav := page.Sections[0].Text
	if n := strings.Count(nav, "Home"); n != 1 {
		t.Errorf("nav text contains Home %d times, want 1 (exact duplicate dropped):\n%s", n, nav)
	}
	if !strings.Contains(nav, "About") || !strings.Contains(nav, "Contact") {
		t.Errorf("nav text lost the distinct About/Contact menu:\n%s", nav)
	}
	if n := strings.Count(page.Sections[2].Text, "Stock ticker widget."); n != 1 {
		t.Errorf("aside text appears %d times, want 1 (text-only duplicate dropped)", n)
	}
	if n := strings.Count(page.Sections[3].Text, "© 2026"); n != 1 {
		t.Errorf("footer text appears %d times, want 1 (duplicate footer dropped)", n)
	}
}

func TestReadPageNeverDedupesMain(t *testing.T) {
	t.Parallel()

	// Repeated main regions are unusual but are content; dropping one would
	// be data loss, so identical mains are never deduplicated. They do merge
	// (adjacent same-kind merge is by design), which concatenates the texts —
	// the invariant is that BOTH copies' text survives.
	doc := `<html><body>
		<main><p>Identical body prose repeated by some framework glitch, still long enough to matter.</p></main>
		<main><p>Identical body prose repeated by some framework glitch, still long enough to matter.</p></main>
		</body></html>`

	page, err := ReadPage([]byte(doc))
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	if len(page.Sections) != 1 || page.Sections[0].Kind != SectionMain {
		t.Fatalf("sections = %+v, want a single merged main section", readerSectionKinds(t, page))
	}
	if n := strings.Count(page.Sections[0].Text, "Identical body prose"); n != 2 {
		t.Fatalf("main text holds %d copies of the prose, want 2 (content is never dropped):\n%s",
			n, page.Sections[0].Text)
	}
}

func TestReadPageToMarkdownDedupesInFlatOutput(t *testing.T) {
	t.Parallel()

	doc := `<html><body>
		<nav><a href="/h">Home</a></nav>
		<nav><a href="/h">Home</a></nav>
		<main><p>Prose that makes the main region unambiguous for the classifier.</p></main>
		</body></html>`

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	out, err := ReadPageToMarkdown([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("ReadPageToMarkdown: %v", err)
	}
	if n := strings.Count(out, "[Home](/h)"); n != 1 {
		t.Errorf("flat output contains the menu %d times, want exactly 1:\n%s", n, out)
	}
}

func TestReadPageDedupKeepsTextlessIconMenus(t *testing.T) {
	t.Parallel()

	// Icon-only menus: the anchors carry no text (the icons were <svg>,
	// which sanitization removes). With reader mode's always-on link
	// formatting the anchors render their "Link N" placeholder labels, so the
	// two sections' texts already differ; the empty-signature guard below the
	// dedup remains as defense for the truly-empty case. The invariant under
	// test: both menus' distinct link sets survive.
	doc := `<html><body>
		<nav><a href="/alpha"></a><a href="/beta"></a></nav>
		<nav><a href="/gamma"></a><a href="/delta"></a></nav>
		<main><p>Article prose long enough to dominate and classify cleanly.</p></main>
		</body></html>`

	page, err := ReadPage([]byte(doc))
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	// The two empty-text navs merge (adjacent same-kind merge is by design),
	// so the count is 1 — the invariant under test is that BOTH menus' links
	// survive: before the empty-signature guard, the second nav was dropped
	// wholesale and only two URLs remained.
	var navs []PageSection
	for _, sec := range page.Sections {
		if sec.Kind == SectionNavigation {
			navs = append(navs, sec)
		}
	}
	if len(navs) != 1 {
		t.Fatalf("navigation sections = %d, want 1 merged section", len(navs))
	}
	var urls []string
	for _, l := range navs[0].Links {
		urls = append(urls, l.URL)
	}
	slices.Sort(urls)
	want := []string{"/alpha", "/beta", "/delta", "/gamma"}
	if !slices.Equal(urls, want) {
		t.Fatalf("nav link URLs = %v, want all four distinct targets %v (second menu was dropped)", urls, want)
	}
}

func TestReadPageRenderVariants(t *testing.T) {
	t.Parallel()

	page, err := ReadPage([]byte(readerDoc))
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}

	// Render: linked flat document.
	linked := page.Render()
	for _, want := range []string{"# Header", "# Main Content", "[Tech](/tech)", "[Privacy](/privacy)"} {
		if !strings.Contains(linked, want) {
			t.Errorf("Render() missing %q:\n%s", want, linked)
		}
	}

	// RenderText: same headings and words, no link constructs.
	plain := page.RenderText()
	for _, want := range []string{"# Header", "# Main Content", "Tech", "Privacy"} {
		if !strings.Contains(plain, want) {
			t.Errorf("RenderText() missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "](/tech)") || strings.Contains(plain, "](/privacy)") ||
		strings.Contains(plain, "<a href") || strings.Contains(plain, "[IMAGE:") {
		t.Errorf("RenderText() leaked link/image constructs:\n%s", plain)
	}
}

func TestReadPageToMarkdownText(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	doc := `<html><head><title>My Page - Site</title></head><body>
		<nav><a href="/a">Alpha</a> <a href="/b">Beta</a></nav>
		<main><h1>Head</h1><p>Body text with <a href="sec/page.html">a link</a> inside. Plus enough extra prose that the main region is unambiguous.</p></main>
		<footer><p>© Site</p></footer>
		</body></html>`

	out, err := ReadPageToMarkdownText([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("ReadPageToMarkdownText: %v", err)
	}
	for _, want := range []string{"# My Page", "# Navigation", "Alpha", "Beta", "# Main Content", "# Head", "a link", "# Footer"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	for _, banned := range []string{"](/a)", "](/b)", "](sec/page.html)", "<a href", "[LINK:", "[IMAGE:"} {
		if strings.Contains(out, banned) {
			t.Errorf("link-free output leaked %q:\n%s", banned, out)
		}
	}

	// The linked sibling on the same document carries the constructs.
	linked, err := ReadPageToMarkdown([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("ReadPageToMarkdown: %v", err)
	}
	if !strings.Contains(linked, "[Alpha](/a)") || !strings.Contains(linked, "[a link](sec/page.html)") {
		t.Errorf("linked form missing its links:\n%s", linked)
	}
}

func TestReadPageFromFileToMarkdownText(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "page.html")
	if err := os.WriteFile(path, []byte(readerDoc), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	out, err := ReadPageFromFileToMarkdownText(path)
	if err != nil {
		t.Fatalf("ReadPageFromFileToMarkdownText: %v", err)
	}
	if !strings.Contains(out, "# Header") || !strings.Contains(out, "Tech") {
		t.Errorf("output missing structure/labels:\n%s", out)
	}
	if strings.Contains(out, "](/tech)") {
		t.Errorf("link-free file form leaked links:\n%s", out)
	}
}

func TestReadPageLinkLabelImages(t *testing.T) {
	t.Parallel()

	// The header shapes from html_doc/18.html: a logo-only link (two images,
	// no text) and an icon+text menu item (empty-alt image). With images
	// rendered inline, the logo link takes its alts as the label and the
	// decorative icon is dropped from the text item's label.
	doc := `<html><body>
		<header>
			<div class="header-mobile-logo"><a href="https://enphase.com/">
				<img src="/logo-white.svg" alt="Enphase logo default">
				<img src="/logo-white.svg" alt="Enphase logo white">
			</a></div>
			<a class="menu-dropdown__box-item" href="https://link.enphase.com/get-enphase" aria-label="Home Energy System">
				<img src="/home-energy-system.svg" alt="" class="svg-icon">
				<span>Home Energy System</span>
			</a>
		</header>
		<main><p>Article prose long enough to anchor the page's region classification.</p></main>
		</body></html>`

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	out, err := ReadPageToMarkdown([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("ReadPageToMarkdown: %v", err)
	}
	if !strings.Contains(out, "[Enphase logo default Enphase logo white](https://enphase.com/)") {
		t.Errorf("images-only label did not become the alt texts:\n%s", out)
	}
	if strings.Contains(out, "![Enphase logo") {
		t.Errorf("nested/escaped image leaked into the logo link label:\n%s", out)
	}
	if !strings.Contains(out, "[Home Energy System](https://link.enphase.com/get-enphase)") {
		t.Errorf("icon+text label not rendered as plain text label:\n%s", out)
	}
	if strings.Contains(out, "/home-energy-system.svg") {
		t.Errorf("decorative icon image survived in the label:\n%s", out)
	}
	if strings.Contains(out, "[IMAGE:") || strings.Contains(out, "[LINK:") {
		t.Errorf("placeholder token leaked into output:\n%s", out)
	}
}

func TestReadPagePlainTextStripsInlinedImageTokens(t *testing.T) {
	t.Parallel()

	// The table flattener inlines a cell's [IMAGE:n] token into a
	// single-line row, and an image inside running prose lands mid-sentence —
	// a line-scoped stripper leaked both into PlainText/RenderText. Tokens
	// must be removed wherever they occur.
	doc := `<html><body><main>
		<table>
			<tr><th>Client</th></tr>
			<tr><td><a href="/x"><img src="/i.png" alt="Acme logo"></a></td></tr>
		</table>
		<p>Caption text <img src="/c.png" alt="shot"> stays.</p>
	</main></body></html>`

	cfg := DefaultConfig()
	cfg.InlineImageFormat = "markdown"
	cfg.MaxCacheEntries = 0
	res, err := ReadPage([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	for _, sec := range res.Sections {
		if strings.Contains(sec.PlainText, "[IMAGE:") {
			t.Errorf("image token leaked into PlainText:\n%q", sec.PlainText)
		}
	}
	if strings.Contains(res.RenderText(), "[IMAGE:") {
		t.Errorf("image token leaked into RenderText:\n%s", res.RenderText())
	}
}

func TestReadPageSectionImages(t *testing.T) {
	t.Parallel()

	// An images-only region is kept because the section-wide collector saw
	// its images — before PageSection.Images existed that keep-check was the
	// images' ONLY effect, leaving an information-free husk section. The
	// collected images are now exposed like Links (gated on PreserveImages).
	doc := `<html><body>
		<aside><img src="/b.png" alt="Widget"></aside>
		<main><p>Article prose long enough to anchor the page's region classification.</p></main>
	</body></html>`

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	res, err := ReadPage([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	var sawImage bool
	for _, sec := range res.Sections {
		if sec.Kind == SectionAside {
			if len(sec.Images) == 1 && sec.Images[0].URL == "/b.png" && sec.Images[0].Alt == "Widget" {
				sawImage = true
			}
		}
	}
	if !sawImage {
		t.Errorf("aside section does not carry its image: %+v", res.Sections)
	}

	// PreserveImages=false keeps rendering-internal collection (no tokens at
	// the default "none" format) but must not expose the field.
	cfg.PreserveImages = false
	res, err = ReadPage([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	for _, sec := range res.Sections {
		if len(sec.Images) != 0 {
			t.Errorf("PreserveImages=false exposed section.Images: %+v", sec.Images)
		}
	}
}

func TestEmptyAnchorLeavesNoDoubledSpace(t *testing.T) {
	t.Parallel()

	// An empty anchor (no href, no text) between two block divs becomes a
	// zero-width substitution; the two spaces flanking its token (block
	// boundaries, normalized to single spaces) must not survive as a double
	// space in the linked form — the plain form re-cleans after stripping, so
	// the two disagreed. Found on dev_test/html_doc/13.html's quote table.
	doc := `<html><body><main><table>
		<tr><th>Quote</th></tr>
		<tr><td><div>Label</div><div><a id="x"></a></div><div>2,350.000</div></td></tr>
	</table></main></body></html>`

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	cfg.InlineLinkFormat = "markdown"
	res, err := ReadPage([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	for _, sec := range res.Sections {
		if strings.Contains(sec.Text, "  ") {
			t.Errorf("linked form carries a double space:\n%q", sec.Text)
		}
		if !strings.Contains(sec.Text, "Label 2,350.000") {
			t.Errorf("label/value junction not single-spaced:\n%q", sec.Text)
		}
	}

	// The Extract path shares the substitution machinery and the same
	// post-substitution re-clean.
	out, err := ExtractToMarkdown([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("ExtractToMarkdown: %v", err)
	}
	if strings.Contains(out, "  ") {
		t.Errorf("Extract markdown carries a double space:\n%s", out)
	}
}

func TestReadPageTableTemplateBetweenRows(t *testing.T) {
	t.Parallel()

	// <template> is the one element the HTML parser keeps between table
	// rows (phrasing strays are foster-parented out). Its anchors are
	// numbered by the section-wide collector, so the table walk must burn
	// them — text discarded — or the link AFTER the table resolves against a
	// shifted position.
	doc := `<html><body><main>
		<table>
			<tr><td><a href="/cell">Cell link</a></td></tr>
			<template><a href="/tpl">T</a></template>
			<tr><td>second row</td></tr>
		</table>
		<p>After: <a href="/after">after link</a>.</p>
	</main></body></html>`

	out, err := ReadPageToMarkdown([]byte(doc))
	if err != nil {
		t.Fatalf("ReadPageToMarkdown: %v", err)
	}
	if !strings.Contains(out, "[Cell link](/cell)") {
		t.Errorf("cell link missing its URL:\n%s", out)
	}
	if !strings.Contains(out, "[after link](/after)") {
		t.Errorf("post-table link resolved to a shifted entry (counter desync):\n%s", out)
	}
	if strings.Contains(out, "/tpl") {
		t.Errorf("between-rows template anchor leaked into output:\n%s", out)
	}
}

func TestReadPageIconListItemSingleLineLabel(t *testing.T) {
	t.Parallel()

	// The "icon-list-item" shape from html_doc/18.html: one anchor wrapping
	// an icon block and a text span. The label captured between the
	// placeholder markers carried the block children's newlines, rendering
	// "[\n  Email alerts\n\n](url)" — a link construct split across lines.
	// collapseInlineSpace must flatten it to one line.
	doc := `<html><body>
		<footer><ul>
			<li class="icon-list-item"><a href="https://investor.enphase.com/email-alerts">
				<span class="icon"><svg></svg></span>
				<span>Email alerts</span>
			</a></li>
		</ul></footer>
		<main><p>Article prose long enough to anchor the page classification.</p></main>
		</body></html>`

	out, err := ReadPageToMarkdown([]byte(doc))
	if err != nil {
		t.Fatalf("ReadPageToMarkdown: %v", err)
	}
	want := "[Email alerts](https://investor.enphase.com/email-alerts)"
	var oneLine bool
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(line, "Email alerts") && strings.Contains(line, want) {
			oneLine = true
		}
	}
	if !oneLine {
		t.Errorf("link label not rendered on one line (want %q in a single line):\n%s", want, out)
	}
}

func TestReadPageTableLinks(t *testing.T) {
	t.Parallel()

	// The events-table shape from html_doc/18.html: links inside table cells
	// used to collapse to bare labels (the table renderer's GetTextContent
	// flattened anchors). The token-aware table path now carries
	// [LINK:n]/[IMAGE:n] through the cells, so formatInlineLinks restores the
	// URLs — and the link AFTER the table must still resolve to its own
	// entry. The template cell pins content-mode counter alignment: the
	// emitter descends into templates there, exactly like the unpruned
	// collector.
	doc := `<html><body>
		<main>
			<table class="nirtable collapse-table events-tbl">
				<tr><th>Date</th><th>Event</th></tr>
				<tr><td>Nov 5, 2026</td><td><a href="/events/q3-call">Q3 2026 Financial Results Conference Call</a></td></tr>
				<tr><td>Hidden</td><td><template><a href="/tpl">T</a></template></td></tr>
			</table>
			<p>After the table: <a href="/after">an after link</a>.</p>
		</main>
		</body></html>`

	out, err := ReadPageToMarkdown([]byte(doc))
	if err != nil {
		t.Fatalf("ReadPageToMarkdown: %v", err)
	}
	if !strings.Contains(out, "[Q3 2026 Financial Results Conference Call](/events/q3-call)") {
		t.Errorf("table-cell link missing its URL:\n%s", out)
	}
	if !strings.Contains(out, "[an after link](/after)") {
		t.Errorf("post-table link resolved to the wrong entry (counter desync):\n%s", out)
	}
	if strings.Contains(out, "[LINK:") || strings.Contains(out, "[IMAGE:") {
		t.Errorf("placeholder token leaked into output:\n%s", out)
	}

	// The plain variant keeps the cell labels without the constructs.
	page, err := ReadPage([]byte(doc))
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	for _, sec := range page.Sections {
		if sec.Kind != SectionMain {
			continue
		}
		if !strings.Contains(sec.PlainText, "Q3 2026 Financial Results Conference Call") {
			t.Errorf("plain text lost the table-cell label:\n%s", sec.PlainText)
		}
		if strings.Contains(sec.PlainText, "](/events/q3-call)") {
			t.Errorf("plain text leaked a link construct:\n%s", sec.PlainText)
		}
	}
}
