package html

// regressions_internal_test.go — white-box regression suite, consolidated
// from the seven former single-topic D-002 regression files (section headers
// below preserve each file’s original documentation):
//   regressions_media_test.go, regressions_table_test.go,
//   regressions_url_test.go, regressions_timeout_test.go,
//   cache_regression_test.go, walkcap_regression_test.go,
//   minor_fixes_regression_test.go.

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// /////////////////////////////////////////////////////////////////////////////
// From regressions_media_test.go
// /////////////////////////////////////////////////////////////////////////////

// regressions_media_test.go — regressions for the D-002 media extraction and
// ownership fixes, gathered from the former single-topic files:
//
//   - iframe/embed <source> attribute scanning no longer lets a decoy value
//     inside another attribute's quoted value hijack the real src, nor does a
//     literal '>' inside a quoted value end the tag scan early;
//   - the inline-image HTML formatter keeps the title attribute;
//   - media URLs retained in the Result (and the cached Result) never alias the
//     caller's input buffer, on the zero-copy decode path.

func TestIframeSourceNotHijackedByQuotedTitle(t *testing.T) {
	t.Parallel()

	hasVideo := func(res *Result, substr string) bool {
		for _, v := range res.Videos {
			if strings.Contains(v.URL, substr) {
				return true
			}
		}
		return false
	}

	// Decoy attribute value that reads like an unquoted source; pre-fix, the
	// scanner returned `here"` from inside the title, failed validation, and
	// returned — the real src was never read.
	doc := `<html><body><iframe title="video src=here" src="https://cdn.example.com/embed/real.mp4"></iframe></body></html>`
	res, err := Extract([]byte(doc))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !hasVideo(res, "cdn.example.com/embed/real.mp4") {
		t.Errorf("real iframe src not extracted (videos: %v)", res.Videos)
	}

	// Decoy containing a fully valid-looking video URL inside the title:
	// pre-fix the decoy won the first-match race and real2 was lost entirely.
	doc2 := `<html><body><iframe title="demo src=https://evil.example.com/x.mp4 here" src="https://cdn.example.com/embed/real2.mp4"></iframe></body></html>`
	res2, err := Extract([]byte(doc2))
	if err != nil {
		t.Fatalf("Extract(2): %v", err)
	}
	if !hasVideo(res2, "cdn.example.com/embed/real2.mp4") {
		t.Errorf("real iframe src lost to a quoted-value decoy (videos: %v)", res2.Videos)
	}
}

// mediaAliasingDoc carries every media URL shape that reaches the result:
// an iframe (raw-HTML attribute path), an embed and an object element (both
// raw-HTML), a <video src>, an <audio src>, and an mp3 that appears only as
// untagged text inside a script body (the ScanMediaURLs path).
const mediaAliasingDoc = `<html><body>
<iframe src="https://www.youtube.com/embed/abc123"></iframe>
<embed src="https://cdn.example.com/clip.webm">
<object data="https://cdn.example.com/obj.mp4"></object>
<video src="https://cdn.example.com/movie.mp4"></video>
<audio src="https://cdn.example.com/song.ogg"></audio>
<script>var cfg = {"voice":"https://cdn.example.com/narration.mp3"};</script>
</body></html>`

// scrubInput overwrites every ASCII letter of buf, which is what a caller
// recycling an input buffer does.
func scrubInput(buf []byte) {
	for i := range buf {
		if c := buf[i]; (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			buf[i] = 'Z'
		}
	}
}

func mediaAliasingProcessor(t *testing.T) *Processor {
	t.Helper()
	// Cache off: with it on, the second extraction could be served from the
	// cache and the assertion would test cloneResult instead of the buffer
	// ownership contract this file is about. ProcessingTimeout off keeps the
	// zero-copy decode path (see detectEncoding); with a timeout the decoded
	// document is already a private copy and the media paths could not alias
	// the caller's buffer, so the clone these tests guard would go unexercised.
	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	cfg.ProcessingTimeout = 0
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

// TestMediaURLsNotAliasedWithInputBuffer pins the ownership contract: mutating
// the input buffer after Extract must not change any URL in the Result.
func TestMediaURLsNotAliasedWithInputBuffer(t *testing.T) {
	t.Parallel()

	p := mediaAliasingProcessor(t)

	buf := []byte(mediaAliasingDoc)
	result, err := p.Extract(buf)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(result.Videos) == 0 || len(result.Audios) == 0 {
		t.Fatalf("test document produced no media: videos=%d audios=%d",
			len(result.Videos), len(result.Audios))
	}

	videosBefore := make([]string, len(result.Videos))
	for i, v := range result.Videos {
		// strings.Clone is required here: copying the string header alone would
		// leave the snapshot pointing at the very bytes the scrub overwrites,
		// so the comparison below would compare a value with itself.
		videosBefore[i] = strings.Clone(v.URL)
	}
	audiosBefore := make([]string, len(result.Audios))
	for i, a := range result.Audios {
		audiosBefore[i] = strings.Clone(a.URL)
	}

	scrubInput(buf)

	for i, v := range result.Videos {
		if v.URL != videosBefore[i] {
			t.Errorf("Video[%d].URL changed after the input buffer was reused: %q -> %q",
				i, videosBefore[i], v.URL)
		}
		if v.Type == "" {
			t.Errorf("Video[%d].Type unexpectedly empty", i)
		}
	}
	for i, a := range result.Audios {
		if a.URL != audiosBefore[i] {
			t.Errorf("Audio[%d].URL changed after the input buffer was reused: %q -> %q",
				i, audiosBefore[i], a.URL)
		}
	}
}

// TestCachedMediaURLsNotAliasedWithInputBuffer covers the second consequence:
// the cached entry must not point into the caller's buffer either. A cache hit
// serves a clone of the cached Result, so if the cached URLs aliased the first
// caller's input, the clone would too.
func TestCachedMediaURLsNotAliasedWithInputBuffer(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 16
	// ProcessingTimeout off for the same reason as mediaAliasingProcessor: the
	// aliasing under test only exists on the zero-copy decode path, and a cache
	// hit would otherwise mask it with cloneResult's fresh strings.
	cfg.ProcessingTimeout = 0
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })

	buf := []byte(mediaAliasingDoc)
	first, err := p.Extract(buf)
	if err != nil {
		t.Fatalf("first Extract: %v", err)
	}
	if len(first.Videos) == 0 {
		t.Fatal("test document produced no videos")
	}
	// Cloned for the same reason as above: a bare header copy would alias the
	// bytes the next statement overwrites.
	before := strings.Clone(first.Videos[0].URL)

	// The caller reuses its buffer exactly as the API permits.
	scrubInput(buf)

	// Same bytes as the first call, so this is a cache hit.
	second, err := p.Extract(append([]byte(nil), mediaAliasingDoc...))
	if err != nil {
		t.Fatalf("second Extract: %v", err)
	}
	if len(second.Videos) == 0 {
		t.Fatal("second extraction produced no videos")
	}
	if got := second.Videos[0].URL; got != before {
		t.Fatalf("cached media URL aliased the first caller's input buffer: %q -> %q", before, got)
	}
}

func TestIframeSourceAfterQuotedGT(t *testing.T) {
	t.Parallel()

	// A '>' inside an earlier quoted attribute value previously ended the
	// tag scan early, so the real src after it was never read.
	doc := `<html><body><iframe title="a > b" src="https://cdn.example.com/embed/q.mp4"></iframe></body></html>`
	res, err := Extract([]byte(doc))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	found := false
	for _, v := range res.Videos {
		if strings.Contains(v.URL, "cdn.example.com/embed/q.mp4") {
			found = true
		}
	}
	if !found {
		t.Errorf("iframe src after a quoted '>' not extracted (videos: %v)", res.Videos)
	}
}

func TestInlineImageHTMLTitle(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.InlineImageFormat = "html"

	doc := `<html><body><p><img src="a.png" alt="pic" title="Photo of X"></p></body></html>`
	res, err := Extract([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if want := `<img src="a.png" alt="pic" title="Photo of X">`; !strings.Contains(res.Text, want) {
		t.Errorf("inline html image missing title attribute; Text = %q", res.Text)
	}
}

// /////////////////////////////////////////////////////////////////////////////
// From regressions_table_test.go
// /////////////////////////////////////////////////////////////////////////////

// regressions_table_test.go — regressions for the D-002 table-rendering fixes,
// gathered from the former single-topic files:
//
//   - a <caption> child is now emitted as its own line ahead of the table, in
//     both the Markdown and HTML table formats;
//   - Config.Validate accepts TableFormat case-insensitively and the pipeline
//     honors it end-to-end, so "HTML"/"Markdown" render as their lowercase
//     canonical forms.

const captionDoc = "<html><body>" +
	"<table><caption>Quarterly Results</caption>" +
	"<tr><th>Region</th><th>Revenue</th></tr>" +
	"<tr><td>EMEA</td><td>42</td></tr>" +
	"</table></body></html>"

func TestTableCaptionEmitted(t *testing.T) {
	t.Parallel()

	res, err := Extract([]byte(captionDoc))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !strings.Contains(res.Text, "Quarterly Results") {
		t.Errorf("caption text lost from Text: %q", res.Text)
	}
	// The table itself must still render after the caption line.
	if !strings.Contains(res.Text, "|") {
		t.Errorf("table rows lost from Text: %q", res.Text)
	}

	// Same guarantee for the HTML table format.
	cfg := DefaultConfig()
	cfg.TableFormat = "html"
	res2, err := Extract([]byte(captionDoc), cfg)
	if err != nil {
		t.Fatalf("Extract(html): %v", err)
	}
	if !strings.Contains(res2.Text, "Quarterly Results") {
		t.Errorf("caption text lost from HTML-format output: %q", res2.Text)
	}
}

const mixedCaseTableDoc = "<html><body><h1>Report</h1>" +
	"<table><tr><th colspan=\"2\">H1</th></tr><tr><td>a</td><td>b</td></tr></table>" +
	"</body></html>"

func TestTableLinkURLPipeDoesNotSplitCells(t *testing.T) {
	t.Parallel()

	// A '|' inside a substituted link URL would end the Markdown table cell
	// at substitution time (the table escaper only sees the pre-substitution
	// [LINK:n] text): the URL is percent-encoded so the row keeps its shape.
	doc := `<html><body><article><table>
		<tr><th>Link</th><th>Note</th></tr>
		<tr><td><a href="/x?a=1|b=2">L</a></td><td>tail</td></tr>
	</table></article></body></html>`

	cfg := DefaultConfig()
	cfg.InlineLinkFormat = "markdown"
	cfg.MaxCacheEntries = 0
	out, err := ExtractToMarkdown([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("ExtractToMarkdown: %v", err)
	}
	if !strings.Contains(out, "[L](/x?a=1%7Cb=2)") {
		t.Errorf("pipe in URL not percent-encoded:\n%s", out)
	}
	if strings.Contains(out, "a=1|b=2") {
		t.Errorf("raw pipe survived into the rendered row:\n%s", out)
	}
}

func TestTableSecondCaptionBurnsCounters(t *testing.T) {
	t.Parallel()

	// A second <caption> is a parse artifact (the spec closes the first on a
	// second start tag) that is NOT rendered, but the section-wide collector
	// still numbered its anchors. Without the counter-burn walk over it, the
	// link AFTER the table received the caption's number and rendered with
	// the caption's URL.
	doc := `<html><body><article>
		<table>
			<caption>First caption</caption>
			<caption><a href="/cap-link">caplink</a></caption>
			<tr><td>cell</td></tr>
		</table>
		<p>See <a href="/after">after</a> the table.</p>
	</article></body></html>`

	cfg := DefaultConfig()
	cfg.InlineLinkFormat = "markdown"
	cfg.MaxCacheEntries = 0
	out, err := ExtractToMarkdown([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("ExtractToMarkdown: %v", err)
	}
	if !strings.Contains(out, "[after](/after)") {
		t.Errorf("post-table link rendered with the burned caption's number:\n%s", out)
	}
	if strings.Contains(out, "[after](/cap-link)") {
		t.Errorf("post-table link bound to the second caption's URL:\n%s", out)
	}
}

func TestExtractTableFormatMixedCaseHTML(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.TableFormat = "HTML" // Validate accepts it; the pipeline must honor it

	res, err := Extract([]byte(mixedCaseTableDoc), cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// The HTML renderer emits a real <table> with the colspan attribute
	// intact (the HTML data path never expands spans).
	if !strings.Contains(res.Text, "<table>") {
		t.Errorf("TableFormat %q did not render HTML: %q", "HTML", res.Text)
	}
	if !strings.Contains(res.Text, `colspan="2"`) {
		t.Errorf("HTML output lost the colspan attribute: %q", res.Text)
	}

	// The document contains no pipe characters, so any Markdown pipe table
	// in the output proves the renderer mismatch is back.
	if strings.Contains(res.Text, "|") {
		t.Errorf("TableFormat %q rendered a Markdown pipe table: %q", "HTML", res.Text)
	}
}

func TestExtractTableFormatMixedCaseMarkdown(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.TableFormat = "Markdown"

	res, err := Extract([]byte(mixedCaseTableDoc), cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// Mixed-case "Markdown" must render the same pipe table as the
	// canonical lowercase value: header row expanded to two columns.
	if !strings.Contains(res.Text, "|") {
		t.Errorf("TableFormat %q did not render a Markdown table: %q", "Markdown", res.Text)
	}
	if strings.Contains(res.Text, "<table>") {
		t.Errorf("TableFormat %q rendered HTML output: %q", "Markdown", res.Text)
	}
}

// /////////////////////////////////////////////////////////////////////////////
// From regressions_url_test.go
// /////////////////////////////////////////////////////////////////////////////

// regressions_url_test.go — regressions for the D-002 link-URL fixes, gathered
// from the former single-topic files (plus links_order_test.go and the two
// type-case tests from d002_quality_regression_test.go):
//
//   - a literal '@' in a URL path ("/@alice" profile URLs) is no longer scanned
//     as userinfo, so same-site links survive IncludeExternalLinks=false;
//   - the <source> dispatch gate honors per-type Include* flags (a video
//     <source> must not appear when only audios are enabled, and vice versa);
//   - rel token matching (stylesheet/icon, case-insensitive, multi-token);
//   - ExtractAllLinks output order is deterministic (the result was once
//     built through map iteration and reshuffled on every call);
//   - the type attribute of <source> and <link> is matched
//     case-insensitively: MIME types are case-insensitive markup
//     ("VIDEO/MP4", "text/CSS"), and the case-sensitive checks silently
//     dropped mixed-case declarations into the wrong resource class or out
//     of the result.

// TestExtractAllLinksOrderDeterministic guards against the previous
// map-iteration-based ordering of ExtractAllLinks, which returned links in a
// randomized order on every call. The result must be stable across repeated
// calls on identical input.
func TestExtractAllLinksOrderDeterministic(t *testing.T) {
	t.Parallel()

	const htmlContent = `<html><body>
		<a href="https://example.com/a">A</a>
		<a href="https://example.com/b">B</a>
		<a href="https://example.com/c">C</a>
		<img src="https://example.com/img/d.png">
		<link rel="stylesheet" href="https://example.com/e.css">
		<script src="https://example.com/f.js"></script>
	</body></html>`

	p, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = p.Close() }()

	first, err := p.ExtractAllLinks([]byte(htmlContent))
	if err != nil {
		t.Fatalf("ExtractAllLinks() failed: %v", err)
	}
	if len(first) < 2 {
		t.Fatalf("expected several links, got %d", len(first))
	}

	for run := 1; run < 20; run++ {
		got, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() run %d failed: %v", run, err)
		}
		if len(got) != len(first) {
			t.Fatalf("run %d: link count changed %d -> %d", run, len(first), len(got))
		}
		for i := range got {
			if got[i].URL != first[i].URL {
				t.Fatalf("run %d: non-deterministic order at index %d: %q vs %q",
					run, i, got[i].URL, first[i].URL)
			}
		}
	}
}

func TestSameSitePathUserLinkKeptWhenExternalFiltered(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.BaseURL = "https://example.com/page"
	cfg.IncludeExternalLinks = false

	doc := `<html><body>
		<a href="/@alice">alice profile</a>
		<a href="/about">about</a>
		<a href="https://other.example.net/x">external</a>
	</body></html>`

	res, err := ExtractAllLinks([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("ExtractAllLinks: %v", err)
	}

	has := func(url string) bool {
		for _, l := range res {
			if l.URL == url {
				return true
			}
		}
		return false
	}

	if !has("https://example.com/@alice") {
		t.Errorf("same-site /@alice link was dropped (got %d links)", len(res))
	}
	if !has("https://example.com/about") {
		t.Errorf("same-site /about link was dropped (got %d links)", len(res))
	}
	if has("https://other.example.net/x") {
		t.Error("external link was kept despite IncludeExternalLinks=false")
	}
}

const sourceFlagDoc = `<html><body>
	<video controls><source src="/movie.mp4" type="video/mp4"></video>
	<audio controls><source src="/theme.mp3" type="audio/mpeg"></audio>
</body></html>`

func TestSourceRespectsPerTypeIncludeFlags(t *testing.T) {
	t.Parallel()

	findByURL := func(links []LinkResource, url string) *LinkResource {
		for i := range links {
			if links[i].URL == url {
				return &links[i]
			}
		}
		return nil
	}

	// Videos disabled, audios enabled: the video <source> must not appear,
	// the audio <source> must.
	cfg := DefaultConfig()
	cfg.IncludeVideos = false
	cfg.IncludeAudios = true
	res, err := ExtractAllLinks([]byte(sourceFlagDoc), cfg)
	if err != nil {
		t.Fatalf("ExtractAllLinks: %v", err)
	}
	if m := findByURL(res, "/movie.mp4"); m != nil {
		t.Errorf("video <source> leaked despite IncludeVideos=false: %+v", m)
	}
	if a := findByURL(res, "/theme.mp3"); a == nil {
		t.Errorf("audio <source> missing despite IncludeAudios=true (got %d links)", len(res))
	} else if a.Type != "audio" {
		t.Errorf("audio <source> typed %q, want %q", a.Type, "audio")
	}

	// Symmetric case: videos enabled, audios disabled.
	cfg2 := DefaultConfig()
	cfg2.IncludeVideos = true
	cfg2.IncludeAudios = false
	res2, err := ExtractAllLinks([]byte(sourceFlagDoc), cfg2)
	if err != nil {
		t.Fatalf("ExtractAllLinks(2): %v", err)
	}
	if m := findByURL(res2, "/movie.mp4"); m == nil {
		t.Errorf("video <source> missing despite IncludeVideos=true (got %d links)", len(res2))
	} else if m.Type != "video" {
		t.Errorf("video <source> typed %q, want %q", m.Type, "video")
	}
	if a := findByURL(res2, "/theme.mp3"); a != nil {
		t.Errorf("audio <source> leaked despite IncludeAudios=false: %+v", a)
	}
}

func TestRelTokenMatching(t *testing.T) {
	t.Parallel()

	doc := `<html><head>
		<link rel="StyleSheet" href="/main.css">
		<link rel="shortcut icon" href="/favicon.ico">
		<link rel="preload stylesheet" href="/pre.css">
		<link rel="ICON" href="/icon2.ico">
	</head><body></body></html>`

	res, err := ExtractAllLinks([]byte(doc), DefaultConfig())
	if err != nil {
		t.Fatalf("ExtractAllLinks: %v", err)
	}

	got := map[string]string{}
	for _, l := range res {
		got[l.URL] = l.Type
	}
	want := map[string]string{
		"/main.css":    "css",
		"/favicon.ico": "icon",
		"/pre.css":     "css",
		"/icon2.ico":   "icon",
	}
	for url, typ := range want {
		if got[url] != typ {
			t.Errorf("link %s typed %q, want %q (all: %v)", url, got[url], typ, got)
		}
	}
}

// TestExtractTypeAttributeCaseInsensitive merges the former
// TestExtractSourceLinksTypeCaseInsensitive / TestExtractLinkTagTypeCaseInsensitive
// pair, whose scaffolds were byte-identical: MIME types are case-insensitive
// markup, so a mixed-case type attribute must still classify the resource.
// Each src deliberately has no recognizable extension, so only the type
// attribute can classify it — before the fix the case-sensitive checks
// dropped the resource into the wrong class or out of the result entirely.
func TestExtractTypeAttributeCaseInsensitive(t *testing.T) {
	t.Parallel()

	p, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	for _, tt := range []struct {
		name     string
		doc      string
		url      string
		declared string
		want     string
	}{
		{
			name:     "source type classifies video",
			doc:      `<html><body><video><source src="/media/clip?id=1" type="VIDEO/MP4"></video></body></html>`,
			url:      "/media/clip?id=1",
			declared: "VIDEO/MP4",
			want:     "video",
		},
		{
			name:     "link type classifies stylesheet",
			doc:      `<html><head><link type="text/CSS" href="/theme.css"></head></html>`,
			url:      "/theme.css",
			declared: "text/CSS",
			want:     "css",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			links, err := p.ExtractAllLinks([]byte(tt.doc))
			if err != nil {
				t.Fatalf("ExtractAllLinks: %v", err)
			}
			for _, l := range links {
				if l.URL == tt.url {
					if l.Type != tt.want {
						t.Errorf("resource with type=%q classified as %q, want %q", tt.declared, l.Type, tt.want)
					}
					return
				}
			}
			t.Errorf("URL %s missing from results: %+v", tt.url, links)
		})
	}
}

// TestExtractMarkdownLinkLabelIdioms table-drives the ExtractToMarkdown
// link-label rendering regressions that used to live as five single-scenario
// tests inside read_test.go (born there from the reader-mode label work, but
// exercising the Extract path, not ReadPage). Rows pin:
//   - the linked-image idiom for mixed labels, image-first or text-first
//     (an alt-bearing image is part of the label whichever side it sits on,
//     and the old visit-time hasText check dropped image-first tokens);
//   - page text that looks like a well-formed [IMAGE:999] token but resolves
//     to no image stays literal (the unpaired-token policy);
//   - a multiline alt breaks the inline ![label](url) construct exactly as a
//     newline-bearing link label does, and collapses to one line;
//   - a document whose every href is rejected by the URL validator has an
//     empty links list while the text pass still burned [LINK:n] positions —
//     no token may leak, and the anchor words must survive.
func TestExtractMarkdownLinkLabelIdioms(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name    string
		doc     string
		wants   []string
		forbids []string
	}{
		{
			name: "text-first mixed label nests the image idiom",
			doc:  `<html><body><article><p>View the <a href="/report">full report <img src="/chart.png" alt="Q3 chart"></a> for details.</p></article></body></html>`,
			wants: []string{
				"[full report ![Q3 chart](/chart.png)](/report)",
			},
		},
		{
			name: "image-first mixed label keeps the idiom",
			doc:  `<html><body><article><p>View the <a href="/report"><img src="/chart.png" alt="Q3 chart"> full report</a> for details.</p></article></body></html>`,
			wants: []string{
				"[![Q3 chart](/chart.png) full report](/report)",
			},
		},
		{
			name: "literal look-alike token stays verbatim",
			doc: `<html><body><article>
				<p><img src="/real.png" alt="real"></p>
				<p>See <a href="/notes">the [IMAGE:999] notes</a> now.</p>
			</article></body></html>`,
			wants: []string{
				"the [IMAGE:999] notes",
			},
		},
		{
			name: "multiline alt collapses to one line",
			doc:  "<html><body><article><p>Chart: <img src=\"/c.png\" alt=\"revenue\nchart 2026\"></p></article></body></html>",
			wants: []string{
				"![revenue chart 2026](/c.png)",
			},
		},
		{
			name:  "all-invalid hrefs leak no tokens",
			doc:   `<html><body><article><p>Click <a href="javascript:alert(1)">Bad</a> or <a href="vbscript:x">Worse</a> now.</p></article></body></html>`,
			wants: []string{"Bad", "Worse"},
			forbids: []string{
				"[LINK:",
				"[IMAGE:",
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.InlineImageFormat = "markdown"
			cfg.InlineLinkFormat = "markdown"
			cfg.MaxCacheEntries = 0
			out, err := ExtractToMarkdown([]byte(tt.doc), cfg)
			if err != nil {
				t.Fatalf("ExtractToMarkdown: %v", err)
			}
			for _, want := range tt.wants {
				if !strings.Contains(out, want) {
					t.Errorf("output missing %q:\n%s", want, out)
				}
			}
			for _, forbid := range tt.forbids {
				if strings.Contains(out, forbid) {
					t.Errorf("forbidden %q leaked into output:\n%s", forbid, out)
				}
			}
		})
	}
}

// /////////////////////////////////////////////////////////////////////////////
// From regressions_timeout_test.go
// /////////////////////////////////////////////////////////////////////////////

// regressions_timeout_test.go — regressions for the D-002/P-002 timeout and
// ownership fixes, gathered from the former single-topic files (plus the
// timeout-audit parity test from d002_quality_regression_test.go):
//
//   - any context.DeadlineExceeded was once rewritten to ErrProcessingTimeout
//     whenever ProcessingTimeout > 0, losing the caller's own earlier deadline
//     identity; the *WithContext entry points must preserve it;
//   - with a processing timeout configured, detectEncoding must decode into a
//     private copy (work runs on another goroutine), while the synchronous path
//     keeps the zero-copy decode;
//   - a processing timeout on the links path records an AuditEventTimeout
//     entry, matching the Extract path (extractAllLinksCore previously bumped
//     only the error counter, leaving an audit blind spot);
//   - a UTF-8 BOM must be stripped identically on the encoding fast path and
//     the forced-encoding slow path (moved here from the former
//     bom_fastpath_regression_test.go — another fast-vs-slow-path parity
//     regression alongside the detectEncoding pair);
//   - the remaining *FromFileWithContext entry points observe a cancelled
//     context before their uncancellable file read, matching the
//     ExtractFromFileWithContext precedence contract.

// userDeadlineDoc is large enough that full processing reliably outlives a
// ~1ms caller deadline (each <p> adds an element plus a text node).
func userDeadlineDoc() []byte {
	var b strings.Builder
	b.WriteString("<html><body>")
	for i := 0; i < 20000; i++ {
		b.WriteString("<p>filler text</p>")
	}
	b.WriteString("</body></html>")
	return []byte(b.String())
}

func TestUserDeadlineIdentityPreserved(t *testing.T) {
	t.Parallel()

	doc := userDeadlineDoc()

	// Cache off: the sanity run below would otherwise cache the result and
	// the deadline run would return instantly from the cache with nil error.
	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0

	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	// Sanity: without a user deadline the document processes fine.
	if _, err := p.Extract(doc); err != nil {
		t.Fatalf("Extract(no deadline): %v", err)
	}

	// A caller deadline well inside the default 30s ProcessingTimeout must
	// surface as context.DeadlineExceeded, not ErrProcessingTimeout.
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	_, err = p.ExtractWithContext(ctx, doc)
	if err == nil {
		t.Fatal("ExtractWithContext(user deadline): expected an error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error = %v, want context.DeadlineExceeded identity", err)
	}
	if errors.Is(err, ErrProcessingTimeout) {
		t.Errorf("user deadline was rewritten to ErrProcessingTimeout: %v", err)
	}
}

// decodedAliasesInput reports whether the string returned by detectEncoding
// shares storage with htmlBytes, by mutating a byte of the input and checking
// whether the decoded text changes with it.
func decodedAliasesInput(t *testing.T, p *Processor, doc []byte) (decoded string, aliased bool) {
	t.Helper()

	decoded, err := p.detectEncoding(doc)
	if err != nil {
		t.Fatalf("detectEncoding: %v", err)
	}
	// Snapshot before mutating: a copy is unaffected by the mutation, an
	// aliased view changes with it.
	snapshot := strings.Clone(decoded)
	doc[0] ^= 0x20 // flip case of the first '<'-side byte
	if decoded != snapshot {
		doc[0] ^= 0x20
		return snapshot, true
	}
	doc[0] ^= 0x20
	return decoded, false
}

// TestDetectEncodingCopiesWhenTimeoutConfigured covers the processing-timeout
// branch: the default configuration runs document work on another goroutine, so
// the decoded document must not point into the caller's buffer.
func TestDetectEncodingCopiesWhenTimeoutConfigured(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	cfg.ProcessingTimeout = time.Minute // any positive value runs the worker
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	doc := []byte("<html><body><p>ascii only</p></body></html>")
	decoded, aliased := decodedAliasesInput(t, p, doc)
	if aliased {
		t.Fatal("decoded document aliases the caller's buffer although a processing timeout runs the work on another goroutine")
	}
	if want := "<html><body><p>ascii only</p></body></html>"; decoded != want {
		t.Fatalf("decoded = %q, want %q", decoded, want)
	}
}

// TestDetectEncodingKeepsZeroCopyWhenSynchronous guards the fast path: with no
// processing timeout, runWithTimeout calls the work inline, nothing outlives the
// call, and the zero-copy decode is retained (with its output unchanged).
func TestDetectEncodingKeepsZeroCopyWhenSynchronous(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	cfg.ProcessingTimeout = 0
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	doc := []byte("<html><body><p>ascii only</p></body></html>")
	decoded, aliased := decodedAliasesInput(t, p, doc)
	if !aliased {
		t.Log("decoded document is a copy; the zero-copy fast path no longer applies (behavior is still correct)")
	}
	if want := "<html><body><p>ascii only</p></body></html>"; decoded != want {
		t.Fatalf("decoded = %q, want %q", decoded, want)
	}
}

// TestExtractAllLinksTimeoutAudited pins the audit parity fix: a document
// whose link extraction exceeds ProcessingTimeout must surface
// ErrProcessingTimeout AND record a timeout audit entry, exactly as the
// Extract path does. The deadline is 1µs so the timeout fires before the
// parse of a many-element document can complete, independent of machine speed.
func TestExtractAllLinksTimeoutAudited(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	cfg.ProcessingTimeout = time.Microsecond
	cfg.Audit.Enabled = true
	cfg.Audit.LogTimeouts = true

	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	_, err = p.ExtractAllLinks(userDeadlineDoc())
	if !errors.Is(err, ErrProcessingTimeout) {
		t.Fatalf("ExtractAllLinks error = %v, want ErrProcessingTimeout", err)
	}

	for _, e := range p.GetAuditLog() {
		if e.EventType == AuditEventTimeout {
			return
		}
	}
	t.Errorf("no %s audit entry recorded for a links-path processing timeout; entries: %+v",
		AuditEventTimeout, p.GetAuditLog())
}

// TestExtractStripsUTF8BOM is the end-to-end regression for the D-002 P0 fix:
// a UTF-8 BOM used to survive the encoding fast path into Result.Text
// (observed: a stray U+FEFF plus a spurious leading newline in front of
// "hello world"), while the same bytes forced down the slow path via
// Config.Encoding = "utf-8" came back clean.
func TestExtractStripsUTF8BOM(t *testing.T) {
	t.Parallel()

	bom := []byte{0xEF, 0xBB, 0xBF}
	body := []byte("<html><body><p>hello world</p></body></html>")

	in := append(append([]byte{}, bom...), body...)

	// Default route: fast path (no forced encoding).
	got, err := Extract(in)
	if err != nil {
		t.Fatalf("Extract(BOM doc): %v", err)
	}
	if strings.ContainsRune(got.Text, rune(0xFEFF)) {
		t.Errorf("default route leaked a UTF-8 BOM into Text: %q", got.Text)
	}
	if want := "hello world"; got.Text != want {
		t.Errorf("Text = %q, want %q", got.Text, want)
	}

	// Slow route (forced encoding): must agree byte-for-byte with the fast
	// route on the same input.
	cfg := DefaultConfig()
	cfg.Encoding = "utf-8"
	slow, err := Extract(in, cfg)
	if err != nil {
		t.Fatalf("Extract(BOM doc, forced utf-8): %v", err)
	}
	if slow.Text != got.Text {
		t.Errorf("forced-encoding route Text = %q, default route = %q; want identical", slow.Text, got.Text)
	}
}

// TestFromFileContextCancellationPrecedence pins the early-cancellation
// ordering for the remaining *FromFileWithContext entry points whose
// cancelled-context branch had no test: ExtractAllLinksFromFileWithContext
// and ExtractToMarkdownFromFileWithContext (the ReadPage FromFile family is
// covered by TestReadPageFromFileErrors / TestReadPageContextVariants in
// read_test.go). A cancelled context must win over the file error, because
// the file read itself is not cancellable.
func TestFromFileContextCancellationPrecedence(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "missing.html")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	if _, err := p.ExtractAllLinksFromFileWithContext(ctx, missing); !errors.Is(err, context.Canceled) {
		t.Errorf("ExtractAllLinksFromFileWithContext: cancelled-context error = %v, want context.Canceled", err)
	}
	if _, err := p.ExtractToMarkdownFromFileWithContext(ctx, missing); !errors.Is(err, context.Canceled) {
		t.Errorf("ExtractToMarkdownFromFileWithContext: cancelled-context error = %v, want context.Canceled", err)
	}
}

// /////////////////////////////////////////////////////////////////////////////
// From cache_regression_test.go
// /////////////////////////////////////////////////////////////////////////////

// cache_regression_test.go — regressions for the processor cache, gathered
// from the former single-topic files (cache_aliasing_test.go,
// cachekey_coverage_test.go):
//
//   - ownership: on a cache miss the returned *Result must not alias the
//     cache entry, so a caller mutating its result cannot corrupt entries a
//     concurrent cache hit would observe;
//   - key quality (D-002 C-1): generateCacheKey once sampled only five
//     ~819-byte windows of documents larger than 64KB, so two same-length
//     documents differing only in the unsampled middle collided and the
//     second extraction was served the first document's cached Result.

// ---------------------------------------------------------------------------
// Ownership — returned Results must not alias the cache
// ---------------------------------------------------------------------------

// TestCacheMissResultNotAliasedWithCache guards the ownership contract between
// a returned *Result and the processor cache.
//
// On a cache miss, the freshly built result is stored in the cache. The value
// returned to the caller must be an independent copy: cache hits already deep
// clone via cloneResult to avoid aliasing, so the miss path must not hand back
// the same pointer the cache holds. Otherwise a caller that mutates its result
// races with concurrent cache-hit reads and silently corrupts the cache.
//
// This reproduces the defect deterministically without the race detector:
// mutate the first returned result, then confirm a cache hit does not observe
// the mutation.
func TestCacheMissResultNotAliasedWithCache(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 16
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	htmlBytes := []byte("<html><body><p>hello world</p></body></html>")

	r1, err := p.Extract(htmlBytes)
	if err != nil {
		t.Fatalf("first Extract: %v", err)
	}

	// The caller owns r1 and is free to mutate it.
	r1.Text = "MUTATED-BY-CALLER"
	r1.Title = "MUTATED-TITLE"

	// Second call hits the cache and must return an independent copy.
	r2, err := p.Extract(htmlBytes)
	if err != nil {
		t.Fatalf("second Extract: %v", err)
	}

	if r2.Text == "MUTATED-BY-CALLER" || r2.Title == "MUTATED-TITLE" {
		t.Fatalf("cache miss returned a *Result aliased with the cache entry; "+
			"caller mutation leaked into cached value (Text=%q Title=%q)",
			r2.Text, r2.Title)
	}
}

// TestConcurrentExtractResultOwnership stresses the same contract: many
// goroutines extract the same document and mutate their own result. Run under
// -race (where the race detector is available) to catch any residual aliasing.
func TestConcurrentExtractResultOwnership(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 64
	p, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	htmlBytes := []byte("<html><body><p>concurrent ownership</p></body></html>")

	var wg sync.WaitGroup
	for range 64 {
		wg.Go(func() {
			r, err := p.Extract(htmlBytes)
			if err != nil {
				return
			}
			// Caller mutates its own result.
			r.Text = "x"
			r.WordCount = 1
		})
	}
	wg.Wait()
}

// ---------------------------------------------------------------------------
// Cache-key quality — every byte region participates in the key
// ---------------------------------------------------------------------------

// TestCacheKeyDistinguishesUnsampledMiddle pins the property the old sampling
// scheme violated: changing bytes anywhere in a large document must produce a
// different key.
func TestCacheKeyDistinguishesUnsampledMiddle(t *testing.T) {
	p, err := New(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	const size = 150000
	base := []byte(strings.Repeat("a", size))
	// 80000 sits between the old scheme's sampled windows (0..819,
	// ~37.5KB, ~75KB, ~112.5KB, and the tail).
	mutated := append([]byte(nil), base...)
	copy(mutated[80000:], "ZZZZ")

	if p.generateCacheKey(base) == p.generateCacheKey(mutated) {
		t.Fatal("cache key is blind to bytes in the document middle")
	}
}

// TestCacheKeyEveryRegion participates in the key: flipping a marker across
// the whole document must change the key at every position.
func TestCacheKeyEveryRegion(t *testing.T) {
	p, err := New(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	const size = 200000
	base := []byte(strings.Repeat("q", size))
	want := p.generateCacheKey(base)

	for pos := 0; pos+4 <= size; pos += 9973 {
		mutated := append([]byte(nil), base...)
		copy(mutated[pos:], "MARK")
		if p.generateCacheKey(mutated) == want {
			t.Fatalf("cache key unchanged after mutating offset %d", pos)
		}
	}
}

// TestExtractLargeDocsWithSameLengthMiddleDiff is the end-to-end guard: a
// second document that differs only in the middle of a >64KB body must be
// extracted afresh, never answered from the first document's cache entry.
func TestExtractLargeDocsWithSameLengthMiddleDiff(t *testing.T) {
	p, err := New(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	prefix := "<html><body><p>"
	pad := strings.Repeat("a", 150000)
	docA := prefix + pad
	docB := prefix + pad
	const markerAt = 80000
	docA = docA[:markerAt] + "AAAA" + docA[markerAt+4:]
	docB = docB[:markerAt] + "BBBB" + docB[markerAt+4:]
	if len(docA) != len(docB) {
		t.Fatalf("test documents differ in length: %d vs %d", len(docA), len(docB))
	}

	resA, err := p.Extract([]byte(docA))
	if err != nil {
		t.Fatal(err)
	}
	resB, err := p.Extract([]byte(docB))
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(resA.Text, "AAAA") {
		t.Fatalf("first document lost its marker: %.80q", resA.Text)
	}
	if strings.Contains(resB.Text, "AAAA") || !strings.Contains(resB.Text, "BBBB") {
		t.Fatalf("second document was served the first document's cached result: %.80q", resB.Text)
	}
	if hits := p.GetStatistics().CacheHits; hits != 0 {
		t.Fatalf("CacheHits = %d, want 0 (documents differ)", hits)
	}
}

// TestCacheKeyDistinguishesSameContentDifferentLength keeps the length mixing
// contract: appending a byte must change the key.
func TestCacheKeyDistinguishesSameContentDifferentLength(t *testing.T) {
	p, err := New(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	short := []byte(strings.Repeat("a", 70000))
	long := append(append([]byte(nil), short...), 'b')
	if p.generateCacheKey(short) == p.generateCacheKey(long) {
		t.Fatal("cache key does not distinguish content length")
	}
}

// /////////////////////////////////////////////////////////////////////////////
// From walkcap_regression_test.go
// /////////////////////////////////////////////////////////////////////////////

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

// pastWalkCapConfig returns the config shared by the PastWalkCap family:
// cache disabled and the processing watchdog off. These tests push 30k–60k
// synthetic nodes through Extract to verify walk completeness — under -race
// with the full suite contending for CPU, that legitimate workload can
// outlast the 30s wall-clock default and flake (observed 2026-10-27,
// GEN-001), which has nothing to do with what they verify.
func pastWalkCapConfig() Config {
	cfg := DefaultConfig()
	cfg.MaxCacheEntries = 0
	cfg.ProcessingTimeout = 0
	return cfg
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

	cfg := pastWalkCapConfig()
	cfg.InlineImageFormat = "markdown"
	cfg.InlineLinkFormat = "markdown"

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

	cfg := pastWalkCapConfig()
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
			cfg := pastWalkCapConfig()

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

	cfg := pastWalkCapConfig()
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
	// WalkNodes is uncapped now, so BOTH entry points must see the anchor.
	// The former assertion only required agreement (inExtract == inAll), so a
	// regression that dropped the anchor on both paths passed with a t.Log.
	if !inExtract || !inAll {
		t.Fatalf("anchor past the former walk cap not collected: Extract=%v ExtractAllLinks=%v, want both true", inExtract, inAll)
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

	cfg := pastWalkCapConfig()
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

	cfg := pastWalkCapConfig()
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

	cfg := pastWalkCapConfig()
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

	cfg := pastWalkCapConfig()
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

// /////////////////////////////////////////////////////////////////////////////
// From minor_fixes_regression_test.go
// /////////////////////////////////////////////////////////////////////////////

// minor_fixes_regression_test.go — regressions for the D-002 P3 behavior
// fixes: percent-encoded spaces in Markdown link/image destinations, the
// ErrBatchTooLarge sentinel for oversized batches, TrimSpace-aligned format
// validation, case-normalized LevelFilteredSink thresholds, Metadata maps no
// longer aliased by GetEntries, Windows OS error wordings mapped to their
// sanitized categories, and link titles free of script/style text.

func TestMarkdownURLDestSpacesEncoded(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.InlineImageFormat = "markdown"
	cfg.InlineLinkFormat = "markdown"

	doc := `<html><body><p><img src="my file.png" alt="pic"> and <a href="docs my page.html">docs</a></p></body></html>`
	res, err := Extract([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !strings.Contains(res.Text, "![pic](my%20file.png)") {
		t.Errorf("image destination not space-escaped: %q", res.Text)
	}
	if !strings.Contains(res.Text, "[docs](docs%20my%20page.html)") {
		t.Errorf("link destination not space-escaped: %q", res.Text)
	}
}

func TestBatchTooLargeSentinel(t *testing.T) {
	t.Parallel()

	inputs := make([][]byte, maxBatchSize+1)
	for i := range inputs {
		inputs[i] = []byte("<p>x</p>")
	}
	br := ExtractBatch(inputs)
	if len(br.Errors) != len(inputs) {
		t.Fatalf("got %d errors, want %d", len(br.Errors), len(inputs))
	}
	for i, err := range br.Errors {
		if !errors.Is(err, ErrBatchTooLarge) {
			t.Fatalf("Errors[%d] = %v, want ErrBatchTooLarge identity", i, err)
		}
	}
}

func TestValidateFormatTrimsSpace(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.InlineImageFormat = " markdown " // padded, mixed case
	if err := cfg.Validate(); err != nil {
		t.Fatalf("Validate rejected a padded format it would normalize: %v", err)
	}

	res, err := Extract([]byte(`<html><body><p><img src="a.png" alt="x"></p></body></html>`), cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !strings.Contains(res.Text, "![x](a.png)") {
		t.Errorf("padded format did not normalize to markdown: %q", res.Text)
	}
}

// p3RecordingSink captures written entries for level-filter assertions.
// Removed TestLevelFilteredSinkNormalizesCase (and its p3RecordingSink twin of
// audit_sink_test.go's recordingSink): the capitalized-threshold row now lives
// in TestLevelFilteredSink's table in audit_sink_test.go.

func TestGetEntriesMetadataNotAliased(t *testing.T) {
	t.Parallel()

	c := newAuditCollector(AuditConfig{
		Enabled:           true,
		LogEncodingIssues: true,
	})
	c.RecordEncodingIssue("utf-8", "boom")

	a := c.GetEntries()
	b := c.GetEntries()
	if len(a) != 1 || a[0].Metadata == nil {
		t.Fatalf("expected one entry with Metadata, got %+v", a)
	}
	a[0].Metadata["injected"] = "via alias"
	if _, ok := b[0].Metadata["injected"]; ok {
		t.Error("GetEntries returned Metadata maps aliased with the collector's entries")
	}
}

func TestFileErrorWindowsWordingsSanitized(t *testing.T) {
	t.Parallel()

	cases := []struct {
		raw  string
		want string
	}{
		{"open C:\\dir\\file.html: The system cannot find the file specified.", "file not found"},
		{"open C:\\dir\\file.html: Access is denied.", "permission denied"},
		{"read C:\\dir\\sub: The process cannot access the file because it is being used by another process.", "file operation failed"},
	}
	for _, c := range cases {
		fe := newFileError("ReadFile", `C:\dir\file.html`, errors.New(c.raw))
		if !strings.Contains(fe.Error(), c.want) {
			t.Errorf("sanitized message = %q, want it to contain %q (raw %q)", fe.Error(), c.want, c.raw)
		}
	}
}

func TestLinkTitleExcludesScriptText(t *testing.T) {
	t.Parallel()

	doc := `<html><body><a href="https://x.com/page"><script>evil()</script>click me<style>.x{color:red}</style></a></body></html>`
	res, err := ExtractAllLinks([]byte(doc))
	if err != nil {
		t.Fatalf("ExtractAllLinks: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("got %d links, want 1", len(res))
	}
	if got := res[0].Title; got != "click me" {
		t.Errorf("link Title = %q, want %q (script/style bodies must not leak)", got, "click me")
	}
}
