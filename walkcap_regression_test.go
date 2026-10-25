package html

// walkcap_regression_test.go — regressions for the removed 50,000-node
// WalkNodes cap (D-002 P0 / C-2), gathered from the former single-topic files
// (walkcap_regression_test.go and walkcap_coverage_test.go):
//
//   - extractImagesAndLinks once used the capped WalkNodes while the
//     placeholder text pass (extractTextWithStructure) recursed uncapped. On
//     documents past the cap the collector silently dropped every img/a past
//     node 50,000, but the text pass kept emitting [IMAGE:n]/[LINK:n]
//     placeholders for them — the unmatched image tokens leaked into Text
//     verbatim and the link URLs were silently discarded;
//   - internal.WalkNodes itself stopped silently at 50,000 visited nodes, and
//     several extraction paths (media, links, <base> detection, the h1/h2
//     title fallback, GetTextContent, table rows, and the in-table
//     placeholder counters) were built on it, so large documents lost that
//     data with no error while other passes over the same tree ran uncapped.
//
// WalkNodes is now uncapped; each test below puts the interesting element
// past the old cap and asserts it survives.

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

func TestImagesLinksBeyondWalkCapCollected(t *testing.T) {
	t.Parallel()

	// Each "<p>filler</p>" contributes an element node plus a text node, so
	// 30,000 paragraphs put the DOM well past the old 50,000-node walk cap
	// before the tail paragraph — the image and link below land past the
	// point where the capped collector used to stop.
	var b strings.Builder
	b.WriteString("<html><body>")
	for i := 0; i < 30000; i++ {
		b.WriteString("<p>filler</p>")
	}
	b.WriteString(`<p><img src="/tail.png" alt="tail"><a href="/tail.html">tail link</a></p>`)
	b.WriteString("</body></html>")

	cfg := DefaultConfig()
	cfg.InlineImageFormat = "markdown"
	cfg.InlineLinkFormat = "markdown"
	cfg.MaxCacheEntries = 0

	res, err := Extract([]byte(b.String()), cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	foundImg := false
	for _, img := range res.Images {
		if img.URL == "/tail.png" {
			foundImg = true
			break
		}
	}
	if !foundImg {
		t.Errorf("image past the 50k-node cap was not collected (len(Images)=%d)", len(res.Images))
	}

	foundLink := false
	for _, l := range res.Links {
		if l.URL == "/tail.html" {
			foundLink = true
			break
		}
	}
	if !foundLink {
		t.Errorf("link past the 50k-node cap was not collected (len(Links)=%d)", len(res.Links))
	}

	// No unmatched placeholder token may survive into the final text.
	if strings.Contains(res.Text, "[IMAGE:") {
		t.Error("literal [IMAGE:n] placeholder leaked into Text")
	}
	if strings.Contains(res.Text, "[LINK:") {
		t.Error("literal [LINK:n] placeholder leaked into Text")
	}

	// The tail media must be formatted inline from its collected position.
	if !strings.Contains(res.Text, "![tail](/tail.png)") {
		t.Error("tail image was not formatted inline as markdown")
	}
	if !strings.Contains(res.Text, "[tail link](/tail.html)") {
		t.Error("tail link was not formatted inline as markdown")
	}
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

// TestLateDocumentFeaturesPastWalkCap table-drives the four walk-cap probes
// that shared one scaffold — build a document whose feature sits beyond the
// removed 50k-node walk cap, run the entry point that observes it, and scan
// the result for the expected value. Rows (the former
// TestAllLinksPastWalkCapExtracted / TestTitleFallbackPastWalkCap /
// TestBaseElementPastWalkCap / TestBaseURLMetaPastWalkCap):
//   - links and images past the cap still collected by ExtractAllLinks;
//   - the h1 title fallback past the cap (Processor.Extract);
//   - <base>-driven resolution — <base> is detected through
//     FindElementByTag and a relative link resolves only if the base is
//     found, so the row exercises both walks at once;
//   - og:url-driven resolution — no <base>, so detectBaseURL scans for
//     og:url / rel=canonical and NormalizeBaseURL reduces it to the origin.
func TestLateDocumentFeaturesPastWalkCap(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name       string
		tail       string
		useExtract bool
		wantSubstr []string // substring matches against link URLs
		wantExact  string   // exact resolved-URL match
		wantTitle  string   // Extract-only: res.Title
	}{
		{
			name:       "links and images collected",
			tail:       `<p><a href="/late.html">late</a><img src="/late.png" alt="late"></p></body></html>`,
			wantSubstr: []string{"late.html", "late.png"},
		},
		{
			name:       "h1 title fallback",
			tail:       "<h1>Late Heading</h1></body></html>",
			useExtract: true,
			wantTitle:  "Late Heading",
		},
		{
			name:      "base element resolution",
			tail:      `<a href="rel/page.html">rel</a><base href="https://late.example/dir/"></body></html>`,
			wantExact: "https://late.example/rel/page.html",
		},
		{
			name:      "og:url meta resolution",
			tail:      `<meta property="og:url" content="https://og.example/art/">` + `<a href="rel/page.html">rel</a></body></html>`,
			wantExact: "https://og.example/rel/page.html",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			doc := beyondWalkCapDoc("<html><body>", tt.tail)
			cfg := DefaultConfig()
			cfg.MaxCacheEntries = 0

			if tt.useExtract {
				res, err := Extract(doc, cfg)
				if err != nil {
					t.Fatalf("Extract: %v", err)
				}
				if tt.wantTitle != "" && res.Title != tt.wantTitle {
					t.Errorf("Title = %q, want %q", res.Title, tt.wantTitle)
				}
				return
			}

			links, err := ExtractAllLinks(doc, cfg)
			if err != nil {
				t.Fatalf("ExtractAllLinks: %v", err)
			}
			for _, want := range tt.wantSubstr {
				found := false
				for _, l := range links {
					if strings.Contains(l.URL, want) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("URL containing %q missing from result: %+v", want, links)
				}
			}
			if tt.wantExact != "" {
				found := false
				for _, l := range links {
					if l.URL == tt.wantExact {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("resolved URL %q missing from result: %+v", tt.wantExact, links)
				}
			}
		})
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
