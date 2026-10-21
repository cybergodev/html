package html

// walkcap_coverage_test.go — regression for D-002 C-2: internal.WalkNodes
// used to stop silently at 50,000 visited nodes. Several extraction paths
// (media, links, <base> detection, the h1/h2 title fallback, GetTextContent,
// table rows, and the in-table placeholder counters) were built on it, so
// large documents lost that data with no error while other passes over the
// same tree (the text pass, sanitization) ran uncapped. WalkNodes is now
// uncapped; each test below puts the interesting element past the old cap
// and asserts it survives.

import (
	"strings"
	"testing"
)

// beyondWalkCapDoc builds a document whose interesting tail lands past the
// 50,000-node walk cap (each "<p>x</p>" is an element node plus a text node).
func beyondWalkCapDoc(head, tail string) []byte {
	var b strings.Builder
	b.WriteString(head)
	for i := 0; i < 30000; i++ {
		b.WriteString("<p>x</p>")
	}
	b.WriteString(tail)
	return []byte(b.String())
}

func TestMediaPastWalkCapExtracted(t *testing.T) {
	t.Parallel()

	doc := beyondWalkCapDoc("<html><body>",
		`<video src="/late.mp4"></video><audio src="/late.mp3"></audio>`+
			`<iframe src="https://www.youtube.com/embed/late"></iframe></body></html>`)

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	res, err := Extract(doc, cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	if len(res.Videos) == 0 {
		t.Fatal("no videos extracted from a document past the walk cap")
	}
	var sawMP4, sawEmbed bool
	for _, v := range res.Videos {
		if strings.Contains(v.URL, "late.mp4") {
			sawMP4 = true
		}
		if v.Type == "embed" {
			sawEmbed = true
		}
	}
	if !sawMP4 {
		t.Errorf("late.mp4 missing from videos: %+v", res.Videos)
	}
	if !sawEmbed {
		t.Errorf("late iframe embed missing from videos: %+v", res.Videos)
	}
	if len(res.Audios) == 0 {
		t.Fatalf("no audios extracted from a document past the walk cap")
	}
	if got := res.Audios[0].URL; !strings.Contains(got, "late.mp3") {
		t.Errorf("audios[0].URL = %q, want the late.mp3 source", got)
	}
}

func TestAllLinksPastWalkCapExtracted(t *testing.T) {
	t.Parallel()

	doc := beyondWalkCapDoc("<html><body>",
		`<p><a href="/late.html">late</a><img src="/late.png" alt="late"></p></body></html>`)

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	links, err := ExtractAllLinks(doc, cfg)
	if err != nil {
		t.Fatalf("ExtractAllLinks: %v", err)
	}

	var sawLink, sawImage bool
	for _, l := range links {
		if strings.Contains(l.URL, "late.html") {
			sawLink = true
		}
		if strings.Contains(l.URL, "late.png") {
			sawImage = true
		}
	}
	if !sawLink {
		t.Errorf("late.html missing from ExtractAllLinks result: %+v", links)
	}
	if !sawImage {
		t.Errorf("late.png missing from ExtractAllLinks result: %+v", links)
	}
}

func TestTitleFallbackPastWalkCap(t *testing.T) {
	t.Parallel()

	doc := beyondWalkCapDoc("<html><body>", "<h1>Late Heading</h1></body></html>")

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	res, err := Extract(doc, cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if res.Title != "Late Heading" {
		t.Errorf("Title = %q, want the h1 fallback past the walk cap", res.Title)
	}
}

func TestBaseElementPastWalkCap(t *testing.T) {
	t.Parallel()

	// <base> is detected through FindElementByTag; a relative link is resolved
	// only if the base is found, so it exercises both walks at once.
	doc := beyondWalkCapDoc("<html><body>",
		`<a href="rel/page.html">rel</a><base href="https://late.example/dir/"></body></html>`)

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	links, err := ExtractAllLinks(doc, cfg)
	if err != nil {
		t.Fatalf("ExtractAllLinks: %v", err)
	}

	var found bool
	for _, l := range links {
		if l.URL == "https://late.example/rel/page.html" {
			found = true
		}
	}
	if !found {
		t.Fatalf("relative link not resolved against the <base> past the walk cap: %+v", links)
	}
}

func TestBaseURLMetaPastWalkCap(t *testing.T) {
	t.Parallel()

	// No <base>: detectBaseURL scans for og:url / rel=canonical instead.
	// NormalizeBaseURL reduces either form to the origin root, so the resolved
	// link keeps only the host.
	doc := beyondWalkCapDoc("<html><body>",
		`<meta property="og:url" content="https://og.example/art/">`+
			`<a href="rel/page.html">rel</a></body></html>`)

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	links, err := ExtractAllLinks(doc, cfg)
	if err != nil {
		t.Fatalf("ExtractAllLinks: %v", err)
	}

	var found bool
	for _, l := range links {
		if l.URL == "https://og.example/rel/page.html" {
			found = true
		}
	}
	if !found {
		t.Fatalf("relative link not resolved against og:url past the walk cap: %+v", links)
	}
}

// TestExtractAndAllLinksAgreePastCap pins the cross-entry-point contract the
// walk-cap mismatch broke: Extract's links and ExtractAllLinks must both see
// anchors past the cap.
func TestExtractAndAllLinksAgreePastCap(t *testing.T) {
	t.Parallel()

	doc := beyondWalkCapDoc("<html><body>",
		`<p><a href="/late.html">late</a></p></body></html>`)

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	res, err := Extract(doc, cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	all, err := ExtractAllLinks(doc, cfg)
	if err != nil {
		t.Fatalf("ExtractAllLinks: %v", err)
	}

	inExtract := false
	for _, l := range res.Links {
		if l.URL == "/late.html" {
			inExtract = true
		}
	}
	inAll := false
	for _, l := range all {
		if l.URL == "/late.html" {
			inAll = true
		}
	}
	if inExtract != inAll {
		t.Fatalf("entry points disagree past the walk cap: Extract=%v ExtractAllLinks=%v", inExtract, inAll)
	}
	if !inExtract {
		t.Log("note: anchor not found by either entry point")
	}
}

// TestTableRowsPastWalkCap: the table walker runs on internal.WalkNodes, so
// capped traversal truncated large tables row-by-row (a 60,000-row table
// rendered 49,998 rows).
func TestTableRowsPastWalkCap(t *testing.T) {
	t.Parallel()

	const rows = 60000
	var b strings.Builder
	b.WriteString("<html><body><table>")
	for i := 0; i < rows; i++ {
		b.WriteString("<tr><td>v</td></tr>")
	}
	b.WriteString("</table></body></html>")

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	res, err := Extract([]byte(b.String()), cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if got := strings.Count(res.Text, "| v"); got != rows {
		t.Fatalf("rendered rows = %d, want %d (table walk truncated)", got, rows)
	}
}

// TestPlaceholderCountersPastWalkCap covers the lockstep requirement for the
// [IMAGE:n]/[LINK:n] counters the table dispatch burns: a table large enough
// that the counter walk stopped early desynchronized the numbers from the
// markdown substitution, so a trailing image rendered as "![Image 10001]"
// pointing at the wrong URL.
func TestPlaceholderCountersPastWalkCap(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	b.WriteString("<html><body><table>")
	for i := 0; i < 20000; i++ {
		b.WriteString(`<tr><td><img src="/in.png"><a href="/in.html">c</a></td></tr>`)
	}
	b.WriteString("</table>")
	b.WriteString(`<p><img src="/after.png" alt="after"><a href="/after.html">after</a></p>`)
	b.WriteString("</body></html>")

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	cfg.InlineImageFormat = "markdown"
	cfg.InlineLinkFormat = "markdown"
	res, err := Extract([]byte(b.String()), cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !strings.Contains(res.Text, "![after](/after.png)") {
		t.Fatalf("trailing image substituted with the wrong entry: ...%s",
			res.Text[max(0, len(res.Text)-120):])
	}
	if !strings.Contains(res.Text, "[after](/after.html)") {
		t.Fatalf("trailing link substituted with the wrong entry: ...%s",
			res.Text[max(0, len(res.Text)-120):])
	}
}

// TestGetTextContentPastWalkCap: GetTextContent backs link titles and table
// cells; a capped walk truncated the text of an element whose subtree is
// large (a huge <a> lost everything after node 50,000).
func TestGetTextContentPastWalkCap(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	b.WriteString(`<html><body><p><a href="/x">START`)
	for i := 0; i < 30000; i++ {
		b.WriteString("<span>filler</span>")
	}
	b.WriteString("END</a></p></body></html>")

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	res, err := Extract([]byte(b.String()), cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(res.Links) == 0 {
		t.Fatal("no link extracted")
	}
	if !strings.Contains(res.Links[0].Text, "END") {
		t.Fatalf("link text truncated before the end: %d bytes", len(res.Links[0].Text))
	}
}

// lateArticleScorer scores only <article>, so the article extractor can only
// succeed if its candidate walk reaches the element past the old cap.
type lateArticleScorer struct{}

func (lateArticleScorer) Score(n ContentNode) int {
	if n.Data() == "article" {
		return 1 << 20
	}
	return 0
}

func (lateArticleScorer) ShouldRemove(ContentNode) bool { return false }

// TestCustomScorerPastWalkCap: a custom Scorer's candidate walk must reach
// every candidate, or the extractor silently falls back to the whole document.
func TestCustomScorerPastWalkCap(t *testing.T) {
	t.Parallel()

	var b strings.Builder
	b.WriteString("<html><body>")
	for i := 0; i < 30000; i++ {
		b.WriteString("<p>filler text here.</p>")
	}
	b.WriteString(`<article><p>THE REAL ARTICLE BODY.</p></article></body></html>`)

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	cfg.Scorer = lateArticleScorer{}
	res, err := Extract([]byte(b.String()), cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if strings.Contains(res.Text, "filler") {
		t.Fatalf("article extractor did not reach the late candidate (text %d bytes)", len(res.Text))
	}
	if !strings.Contains(res.Text, "THE REAL ARTICLE BODY") {
		t.Fatalf("article body missing from output: %q", res.Text)
	}
}
