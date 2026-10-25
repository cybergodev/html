package html_test

// regressions_test.go — regression tests for the D-002 fixes, verified
// through the public API (folded from the former p1_regression_test.go,
// p2_regression_test.go, dataurl_case_regression_test.go, and
// placeholder_regression_test.go):
//   - ResolveURL: scheme-bearing references (mailto:, tel:) are absolute and
//     must not be concatenated onto the base; protocol-relative bases must
//     survive /path resolution.
//   - hasEmbedPattern: embed-host patterns match only at domain boundaries,
//     so lookalike hosts (fakebilibili.com) are not embed videos.
//   - isHiddenByStyle: "display:none !important" hides content.
//   - findTagIgnoreCase: mixed-case tags (<IFRAME> before <iframe>) are found
//     in document order.
//   - extractTitle: the title fallback prefers the FIRST h1/h2.
//   - ExtractToMarkdown* forwards audit events and statistics to the parent
//     processor instead of dropping them.
//   - raw-HTML attribute values are entity-decoded so media URLs extracted
//     from sanitizer-removed tags match their DOM-extracted counterparts.
//   - the inline placeholder contract between the text pass and
//     extractImagesAndLinks: dispatched tables burn [IMAGE:n]/[LINK:n]
//     positions for their subtree, and enabling only one of
//     InlineImageFormat/InlineLinkFormat must not leak the other kind's
//     literal tokens into Text.

import (
	"strings"
	"testing"

	"github.com/cybergodev/html"
)

func TestSchemeLinksNotResolvedOntoBase(t *testing.T) {
	t.Parallel()

	cfg := html.DefaultConfig()
	cfg.BaseURL = "https://site.com/page"
	cfg.ResolveRelativeURLs = true
	p, err := html.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	links, err := p.ExtractAllLinks([]byte(`<html><body>
<a href="mailto:user@example.com">mail</a>
<a href="tel:+1-555-0100">call</a>
<a href="about.html">about</a>
</body></html>`))
	if err != nil {
		t.Fatal(err)
	}

	got := make(map[string]bool)
	for _, l := range links {
		got[l.URL] = true
	}
	if !got["mailto:user@example.com"] {
		t.Errorf("mailto link mangled or missing: %v", links)
	}
	if !got["tel:+1-555-0100"] {
		t.Errorf("tel link mangled or missing: %v", links)
	}
	if !got["https://site.com/about.html"] {
		t.Errorf("relative link not resolved: %v", links)
	}
}

func TestProtocolRelativeBasePreserved(t *testing.T) {
	t.Parallel()

	cfg := html.DefaultConfig()
	cfg.BaseURL = "//cdn.example.com/assets/"
	cfg.ResolveRelativeURLs = true
	p, err := html.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	links, err := p.ExtractAllLinks([]byte(`<html><body><a href="/clip.mp4">c</a></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].URL != "//cdn.example.com/clip.mp4" {
		t.Errorf("protocol-relative base dropped: %+v", links)
	}
}

func TestEmbedHostBoundaryRejectsLookalikes(t *testing.T) {
	t.Parallel()

	p, err := html.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	res, err := p.Extract([]byte(`<html><body>
<embed src="https://fakebilibili.com/page">
<embed src="https://www.bilibili.com/video/BV1x">
</body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	var urls []string
	for _, v := range res.Videos {
		urls = append(urls, v.URL)
	}
	joined := strings.Join(urls, ";")
	if strings.Contains(joined, "fakebilibili.com") {
		t.Errorf("lookalike host classified as embed video: %v", urls)
	}
	if !strings.Contains(joined, "www.bilibili.com/video/BV1x") {
		t.Errorf("genuine subdomain embed not detected: %v", urls)
	}
}

func TestHiddenByStyleImportant(t *testing.T) {
	t.Parallel()

	p, err := html.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	res, err := p.Extract([]byte(`<html><body><main>
<p>Visible article text.</p>
<div style="display:none !important">HIDDEN_BY_DISPLAY</div>
<div style="visibility:hidden!important">HIDDEN_BY_VISIBILITY</div>
<div style="display: none">HIDDEN_PLAIN</div>
</main></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"HIDDEN_BY_DISPLAY", "HIDDEN_BY_VISIBILITY", "HIDDEN_PLAIN"} {
		if strings.Contains(res.Text, secret) {
			t.Errorf("hidden content leaked into Text: %q", res.Text)
		}
	}
	if !strings.Contains(res.Text, "Visible article text.") {
		t.Errorf("visible content lost: %q", res.Text)
	}
}

func TestMixedCaseMediaTagsFoundInOrder(t *testing.T) {
	t.Parallel()

	p, err := html.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	res, err := p.Extract([]byte(`<html><body>
<IFRAME SRC="https://www.youtube.com/embed/UPPER1"></IFRAME>
<iframe src="https://www.youtube.com/embed/lower1"></iframe>
</body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	var urls []string
	for _, v := range res.Videos {
		urls = append(urls, v.URL)
	}
	if len(urls) != 2 {
		t.Fatalf("expected both iframes extracted, got %v", urls)
	}
	if !strings.HasSuffix(urls[0], "UPPER1") || !strings.HasSuffix(urls[1], "lower1") {
		t.Errorf("mixed-case iframes missing or out of document order: %v", urls)
	}
}

func TestTitleFallbackPrefersFirstHeading(t *testing.T) {
	t.Parallel()

	p, err := html.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	// Empty <title> (no text) forces the h1 fallback; the FIRST h1 must win.
	res, err := p.Extract([]byte(`<html><head><title></title></head><body>
<h1>First heading</h1><p>para</p><h1>Second heading</h1>
</body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if res.Title != "First heading" {
		t.Errorf("Title = %q, want %q (first h1)", res.Title, "First heading")
	}

	// No <title> at all and no h1: the first h2 wins.
	res2, err := p.Extract([]byte(`<html><body><h2>Early</h2><p>p</p><h2>Late</h2></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if res2.Title != "Early" {
		t.Errorf("Title = %q, want %q (first h2)", res2.Title, "Early")
	}
}

func TestRawHTMLMediaURLsEntityDecoded(t *testing.T) {
	t.Parallel()

	p, err := html.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	res, err := p.Extract([]byte(`<html><body>
<iframe src="https://cdn.example.com/embed/a.mp4?x=1&amp;y=2"></iframe>
</body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range res.Videos {
		if strings.Contains(v.URL, "&amp;") {
			t.Errorf("raw-extracted media URL kept undecoded entity: %q", v.URL)
		}
		if strings.Contains(v.URL, "x=1&y=2") {
			return // found the decoded iframe URL
		}
	}
	t.Errorf("iframe media URL with decoded query not found: %+v", res.Videos)
}

func TestExtractToMarkdownForwardsAudit(t *testing.T) {
	t.Parallel()

	cfg := html.DefaultConfig()
	cfg.Audit.Enabled = true
	p, err := html.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	_, err = p.ExtractToMarkdown([]byte(`<html><body><p>hi</p><script>evil()</script></body></html>`))
	if err != nil {
		t.Fatal(err)
	}

	var sawBlockedTag bool
	for _, e := range p.GetAuditLog() {
		if e.EventType == html.AuditEventBlockedTag && e.Tag == "script" {
			sawBlockedTag = true
		}
	}
	if !sawBlockedTag {
		t.Error("ExtractToMarkdown sanitize events missing from GetAuditLog (audit blind spot)")
	}

	stats := p.GetStatistics()
	if stats.TotalProcessed == 0 {
		t.Error("ExtractToMarkdown run not reflected in GetStatistics")
	}

	// And the plain Extract path still audits, so coverage is now uniform.
	p.ClearAuditLog()
	if _, err := p.Extract([]byte(`<html><body><p>x</p><script>evil()</script></body></html>`)); err != nil {
		t.Fatal(err)
	}
	sawBlockedTag = false
	for _, e := range p.GetAuditLog() {
		if e.EventType == html.AuditEventBlockedTag && strings.EqualFold(e.Tag, "script") {
			sawBlockedTag = true
		}
	}
	if !sawBlockedTag {
		t.Error("Extract sanitize events missing from GetAuditLog")
	}
}

// The RFC 3986 data:-scheme case regressions (DATA:text/html rejected,
// DATA:image/png kept) moved to security_test.go: they pin the same
// safeMediaTypes whitelist as TestExtractAllLinksRejectsScriptableDataURLs
// and TestDataURLInjection, through the same entry points with only the
// letter case differing.

// ---------------------------------------------------------------------------
// Inline placeholder contract (from placeholder_regression_test.go)
// ---------------------------------------------------------------------------

// TestTableDoesNotShiftInlinePlaceholders extracts a document whose table
// cells contain an image and a link between two paragraph-level elements.
// The original bug: the table's elements were numbered by
// extractImagesAndLinks but never by the text pass, so every placeholder
// after the table substituted the wrong entry (the outro image rendered the
// table's image, and the table's link never appeared). Since the token-aware
// table path landed, cell anchors and images emit their own placeholders, so
// the cells render real [T](URL)/![B](url) constructs — and the numbering
// discipline this test was born from still holds: the outro elements must
// resolve to their own entries.
func TestTableDoesNotShiftInlinePlaceholders(t *testing.T) {
	t.Parallel()

	cfg := html.DefaultConfig()
	cfg.InlineImageFormat = "markdown"
	cfg.InlineLinkFormat = "markdown"
	p, err := html.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	doc := `<html><body>
<p>Intro <img src="/a.png" alt="A"></p>
<table><tr><td><img src="/b.png" alt="B"></td><td><a href="https://table.example/t">T</a></td></tr></table>
<p>Outro <img src="/c.png" alt="C"> <a href="https://outro.example/o">O</a></p>
</body></html>`
	res, err := p.Extract([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}

	// The outro elements must reference C and O — not the table's B/T.
	if !strings.Contains(res.Text, "![C](/c.png)") {
		t.Errorf("outro image markdown missing: %q", res.Text)
	}
	if !strings.Contains(res.Text, "[O](https://outro.example/o)") {
		t.Errorf("outro link markdown missing: %q", res.Text)
	}
	// Table cells now carry their own placeholders through the renderer, so
	// the table's image and link appear inside the rendered rows.
	if !strings.Contains(res.Text, "![B](/b.png)") {
		t.Errorf("table-cell image markdown missing: %q", res.Text)
	}
	if !strings.Contains(res.Text, "[T](https://table.example/t)") {
		t.Errorf("table-cell link markdown missing: %q", res.Text)
	}

	// Positions in the structured results must still cover the table cells.
	var sawB, sawT bool
	for _, img := range res.Images {
		if img.URL == "/b.png" {
			sawB = true
		}
	}
	for _, link := range res.Links {
		if link.URL == "https://table.example/t" {
			sawT = true
		}
	}
	if !sawB || !sawT {
		t.Errorf("table-cell image/link missing from structured results: images=%v links=%v", res.Images, res.Links)
	}
}

// TestMixedInlineFormatsDoNotLeakPlaceholders guards the literal-token leak:
// with only one format enabled, the other kind's [IMAGE:n]/[LINK:n] tokens
// had no formatter to consume them and survived verbatim into Text.
func TestMixedInlineFormatsDoNotLeakPlaceholders(t *testing.T) {
	t.Parallel()

	doc := `<html><body><p>Hello <a href="https://go.dev">Go</a> <img src="a.png" alt="pic"></p></body></html>`

	t.Run("image markdown only", func(t *testing.T) {
		t.Parallel()
		cfg := html.DefaultConfig()
		cfg.InlineImageFormat = "markdown" // InlineLinkFormat stays "none"
		p, err := html.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = p.Close() }()
		res, err := p.Extract([]byte(doc))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(res.Text, "[LINK:") || strings.Contains(res.Text, "[/LINK]") {
			t.Errorf("literal link placeholder leaked: %q", res.Text)
		}
		if !strings.Contains(res.Text, "![pic](a.png)") {
			t.Errorf("image markdown missing: %q", res.Text)
		}
		if !strings.Contains(res.Text, "Go") {
			t.Errorf("link text lost: %q", res.Text)
		}
	})

	t.Run("link markdown only", func(t *testing.T) {
		t.Parallel()
		cfg := html.DefaultConfig()
		cfg.InlineLinkFormat = "markdown" // InlineImageFormat stays "none"
		p, err := html.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = p.Close() }()
		res, err := p.Extract([]byte(doc))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(res.Text, "[IMAGE:") {
			t.Errorf("literal image placeholder leaked: %q", res.Text)
		}
		if !strings.Contains(res.Text, "[Go](https://go.dev)") {
			t.Errorf("link markdown missing: %q", res.Text)
		}
	})
}
