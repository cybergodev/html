package html_test

// regressions_test.go — regression tests for the D-002 fixes, verified
// through the public API (folded from the former p1_regression_test.go and
// p2_regression_test.go):
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
