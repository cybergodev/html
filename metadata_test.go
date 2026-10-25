package html_test

// metadata_test.go — covers the P1 structured-metadata channel and the Result
// enrichment derived from it, gathered from the former single-topic files
// (confidence_test.go, title_strip_site_name_regression_test.go, and the
// heading/reading-time half of p3_features_test.go):
//
//   - JSON-LD (object, array, @graph), OpenGraph and classic <meta> tags,
//     rel=canonical, the metadata-based title priority, and the
//     Config.ExtractMetadata off switch;
//   - the "Title - Site" site-name suffix strip on <title>-derived titles;
//   - the extraction-confidence tiers and the ExtractionMethod marker;
//   - Markdown heading markers in Text (which must not inflate WordCount) and
//     the per-script reading-time rate.

import (
	"strings"
	"testing"
	"time"

	"github.com/cybergodev/html"
)

// extractFor runs Extract with the default config and returns the Result,
// failing the test on error.
func extractFor(t *testing.T, markup string) *html.Result {
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
	return result
}

func TestMetadataJSONLDArticle(t *testing.T) {
	t.Parallel()

	result := extractFor(t, `<html><head>
<title>Ignored Title Tag - Example.com</title>
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"NewsArticle",
 "headline":"Real Headline from JSON-LD",
 "author":{"@type":"Person","name":"Jane Doe"},
 "datePublished":"2026-10-25T10:30:00Z",
 "description":"A summary from JSON-LD.",
 "publisher":{"@type":"Organization","name":"Example News"}}
</script>
</head><body>
<article><h1>DOM Headline</h1><p>Body text long enough to be scored as the article node of this page.</p></article>
</body></html>`)

	if result.Title != "Real Headline from JSON-LD" {
		t.Errorf("Title = %q, want JSON-LD headline (beats <title> and DOM h1)", result.Title)
	}
	if result.Author != "Jane Doe" {
		t.Errorf("Author = %q, want %q", result.Author, "Jane Doe")
	}
	if want := time.Date(2026, 10, 25, 10, 30, 0, 0, time.UTC); !result.PublishedTime.Equal(want) {
		t.Errorf("PublishedTime = %v, want %v", result.PublishedTime, want)
	}
	if result.Description != "A summary from JSON-LD." {
		t.Errorf("Description = %q, want %q", result.Description, "A summary from JSON-LD.")
	}
	if result.SiteName != "Example News" {
		t.Errorf("SiteName = %q, want publisher name %q", result.SiteName, "Example News")
	}
}

func TestMetadataJSONLDGraphAndAuthorArray(t *testing.T) {
	t.Parallel()

	result := extractFor(t, `<html><head>
<script type="application/ld+json">
{"@context":"https://schema.org","@graph":[
 {"@type":"Organization","name":"Example Corp","url":"https://example.com"},
 {"@type":"BlogPosting",
  "headline":"Graph Headline",
  "author":[{"@type":"Person","name":"Ada"},{"@type":"Person","name":"Lin"}],
  "datePublished":"2026-09-01"}]}
</script>
</head><body><article><h1>h</h1><p>Body paragraph with enough words to score as the article node here.</p></article></body></html>`)

	if result.Title != "Graph Headline" {
		t.Errorf("Title = %q, want headline from @graph article node", result.Title)
	}
	if result.Author != "Ada, Lin" {
		t.Errorf("Author = %q, want %q", result.Author, "Ada, Lin")
	}
	if want := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC); !result.PublishedTime.Equal(want) {
		t.Errorf("PublishedTime = %v, want %v (date-only layout)", result.PublishedTime, want)
	}
	if result.SiteName != "Example Corp" {
		t.Errorf("SiteName = %q, want %q", result.SiteName, "Example Corp")
	}
}

func TestMetadataOpenGraphPriority(t *testing.T) {
	t.Parallel()

	result := extractFor(t, `<html><head>
<title>Page Title Tag - Example.com</title>
<meta property="og:title" content="OG Title">
<meta property="og:description" content="OG description">
<meta property="og:site_name" content="OG Site">
<meta property="og:url" content="https://example.com/og-url">
<meta name="description" content="meta description">
</head><body>
<article><h1>x</h1><p>Article body text that is comfortably long enough for the scorer to pick this node.</p></article>
</body></html>`)

	if result.Title != "OG Title" {
		t.Errorf("Title = %q, want og:title over <title> tag", result.Title)
	}
	if result.Description != "OG description" {
		t.Errorf("Description = %q, want og:description over meta description", result.Description)
	}
	if result.SiteName != "OG Site" {
		t.Errorf("SiteName = %q, want og:site_name", result.SiteName)
	}
	if result.CanonicalURL != "https://example.com/og-url" {
		t.Errorf("CanonicalURL = %q, want og:url fallback", result.CanonicalURL)
	}
}

func TestMetadataCanonicalLinkWinsOverOGURL(t *testing.T) {
	t.Parallel()

	result := extractFor(t, `<html><head>
<meta property="og:url" content="https://example.com/og">
<link rel="canonical" href="https://example.com/canonical">
</head><body><article><h1>t</h1><p>Body copy long enough for the article scorer to settle on this article element.</p></article></body></html>`)

	if result.CanonicalURL != "https://example.com/canonical" {
		t.Errorf("CanonicalURL = %q, want rel=canonical over og:url", result.CanonicalURL)
	}
}

func TestMetadataClassicMetaFallbacks(t *testing.T) {
	t.Parallel()

	result := extractFor(t, `<html><head>
<meta name="description" content="classic description">
<meta name="author" content="Classic Author">
</head><body><article><h1>t</h1><p>Some body text so the article node is identifiable by the scoring heuristics used here.</p></article></body></html>`)

	if result.Description != "classic description" {
		t.Errorf("Description = %q, want meta name=description fallback", result.Description)
	}
	if result.Author != "Classic Author" {
		t.Errorf("Author = %q, want meta name=author fallback", result.Author)
	}
}

func TestMetadataMalformedJSONLDIgnored(t *testing.T) {
	t.Parallel()

	result := extractFor(t, `<html><head>
<title>Fallback Title - Example.com</title>
<script type="application/ld+json">{"headline": "broken"</script>
<script type="application/ld+json">not json at all</script>
</head><body><article><h1>t</h1><p>Body text that the article scoring pass will accept as this page's main content.</p></article></body></html>`)

	if result.Title != "Fallback Title" {
		t.Errorf("Title = %q, want <title> fallback when JSON-LD is malformed (site name stripped)", result.Title)
	}
	if result.Author != "" {
		t.Errorf("Author = %q, want empty for malformed JSON-LD", result.Author)
	}
}

func TestMetadataDisabledByConfig(t *testing.T) {
	t.Parallel()

	cfg := html.DefaultConfig()
	cfg.ExtractMetadata = false
	p, err := html.New(cfg)
	if err != nil {
		t.Fatalf("New(cfg) failed: %v", err)
	}
	defer func() { _ = p.Close() }()

	result, err := p.Extract([]byte(`<html><head>
<title>Title Tag - Example.com</title>
<meta property="og:title" content="OG Title">
<meta property="og:site_name" content="OG Site">
</head><body><article><h1>t</h1><p>Body copy that the scorer can find as the article of this small page without trouble.</p></article></body></html>`))
	if err != nil {
		t.Fatalf("Extract() failed: %v", err)
	}

	if result.Title != "Title Tag" {
		t.Errorf("Title = %q, want <title>-based ladder when metadata disabled", result.Title)
	}
	if result.Author != "" || result.SiteName != "" || result.CanonicalURL != "" || result.Description != "" {
		t.Errorf("metadata fields not empty with ExtractMetadata=false: %+v", result)
	}
	if !result.PublishedTime.IsZero() {
		t.Errorf("PublishedTime = %v, want zero with ExtractMetadata=false", result.PublishedTime)
	}
}

func TestMetadataSiteNameSuffixStrippedOnlyOnExactMatch(t *testing.T) {
	t.Parallel()

	// The suffix is stripped when it exactly matches the declared site name.
	stripped := extractFor(t, `<html><head>
<meta property="og:title" content="An Article Title - Example News">
<meta property="og:site_name" content="Example News">
</head><body><article><h1>t</h1><p>Body content that is definitely long enough to be chosen as this page's article.</p></article></body></html>`)
	if stripped.Title != "An Article Title" {
		t.Errorf("Title = %q, want exact-matching site-name suffix stripped", stripped.Title)
	}

	// A " - " subtitle whose tail does not match the site name must survive:
	// metadata headlines legitimately carry subtitles.
	subtitle := extractFor(t, `<html><head>
<meta property="og:title" content="GDP Falls 0.3% - Worst Quarter Since 2020">
<meta property="og:site_name" content="Example News">
</head><body><article><h1>t</h1><p>Body content that is definitely long enough to be chosen as this page's article.</p></article></body></html>`)
	if subtitle.Title != "GDP Falls 0.3% - Worst Quarter Since 2020" {
		t.Errorf("Title = %q, want the subtitle preserved", subtitle.Title)
	}
}

func TestMetadataOGTitleBeatsWebsiteNodeName(t *testing.T) {
	t.Parallel()

	// A WebSite graph node's name is the SITE's name, not the page's title;
	// og:title is page-scoped and must win over it.
	result := extractFor(t, `<html><head>
<meta property="og:title" content="The Actual Page Title">
<script type="application/ld+json">
{"@context":"https://schema.org","@type":"WebSite","name":"Example News","url":"https://example.com"}
</script>
</head><body><article><h1>t</h1><p>Body text that is comfortably long enough for the article scorer on this small page.</p></article></body></html>`)

	if result.Title != "The Actual Page Title" {
		t.Errorf("Title = %q, want og:title over the WebSite node name", result.Title)
	}
	if result.SiteName != "Example News" {
		t.Errorf("SiteName = %q, want the WebSite node name as site name", result.SiteName)
	}
}

func TestResultJSONIncludesMetadataFields(t *testing.T) {
	t.Parallel()

	p, err := html.New()
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer func() { _ = p.Close() }()

	data, err := p.ExtractToJSON([]byte(`<html><head>
<meta property="og:title" content="JSON Fields">
<meta property="article:published_time" content="2026-10-25T08:00:00Z">
</head><body><article><h1>t</h1><p>Enough body text for the article scoring pass to select this node as the page content.</p></article></body></html>`))
	if err != nil {
		t.Fatalf("ExtractToJSON() failed: %v", err)
	}
	out := string(data)
	for _, want := range []string{`"title":"JSON Fields"`, `"published_time":"2026-10-25T08:00:00Z"`, `"confidence"`, `"extraction_method":"article"`} {
		if !strings.Contains(out, want) {
			t.Errorf("JSON output missing %s:\n%s", want, out)
		}
	}
	// Zero/empty metadata must be omitted, not serialized as zero values.
	if strings.Contains(out, "0001-01-01") || strings.Contains(out, `"author":""`) {
		t.Errorf("JSON output serializes empty metadata:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// <title> site-name suffix strip (from title_strip_site_name_regression_test.go)
// ---------------------------------------------------------------------------

func TestTitleStripsSiteName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		title string
		want  string
	}{
		{"hyphen separator", "My Article Title - Example News", "My Article Title"},
		{"pipe separator", "Quarterly Results | Acme Corp", "Quarterly Results"},
		{"middot separator", "A Guide to Go · Developer Blog", "A Guide to Go"},
		{"en dash separator", "Release Notes – Product Site", "Release Notes"},
		{"em dash separator", "Announcement — Company", "Announcement"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := extractFor(t, `<html><head><title>`+tc.title+`</title></head><body><article><p>content</p></article></body></html>`).Title
			if got != tc.want {
				t.Errorf("Title = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestTitleKeepsLongSeparatorParts(t *testing.T) {
	t.Parallel()

	// The tail after the separator is longer than the 30-char cap, so this is a
	// genuine two-part title, not "title + site name", and must stay intact.
	long := "An Extended Discussion of the Topic - With a Substantially Longer Second Part"
	got := extractFor(t, `<html><head><title>`+long+`</title></head><body><article><p>c</p></article></body></html>`).Title
	if got != long {
		t.Errorf("long two-part title was mutilated: got %q, want %q", got, long)
	}
}

func TestTitleKeepsPlainTitle(t *testing.T) {
	t.Parallel()

	got := extractFor(t, `<html><head><title>Simple Title</title></head><body><article><p>c</p></article></body></html>`).Title
	if got != "Simple Title" {
		t.Errorf("Title = %q, want 'Simple Title'", got)
	}
}

// ---------------------------------------------------------------------------
// Extraction-confidence tiers (from confidence_test.go)
// ---------------------------------------------------------------------------

func TestConfidenceArticleFull(t *testing.T) {
	t.Parallel()

	body := longParagraph(600)
	result := extractFor(t, `<html><body><nav><a href="/">Home</a> <a href="/x">X</a></nav>
<article><h1>Confident Extraction</h1><p>`+body+`</p></article>
<footer>Copyright</footer></body></html>`)

	if result.ExtractionMethod != "article" {
		t.Errorf("ExtractionMethod = %q, want %q", result.ExtractionMethod, "article")
	}
	if result.Confidence != 1.0 {
		t.Errorf("Confidence = %v, want 1.0 for a substantial article-node extraction", result.Confidence)
	}
}

func TestConfidenceBodyFallbackShortText(t *testing.T) {
	t.Parallel()

	// Nothing here scores above zero (no headings, one tiny paragraph), so
	// extraction falls back to <body> with very little text.
	result := extractFor(t, `<html><body><p>Hello</p></body></html>`)

	if result.ExtractionMethod != "body" {
		t.Errorf("ExtractionMethod = %q, want %q", result.ExtractionMethod, "body")
	}
	if result.Confidence != 0.2 {
		t.Errorf("Confidence = %v, want 0.2 for a body fallback with <50 chars", result.Confidence)
	}
}

func TestConfidencePaywallBoilerplate(t *testing.T) {
	t.Parallel()

	result := extractFor(t, `<html><body>
<div class="metered"><p>Subscribe to continue reading this article. Subscribe for full access.</p></div>
</body></html>`)

	if result.Confidence != 0.4 {
		t.Errorf("Confidence = %v, want 0.4 for short extraction opening with paywall boilerplate", result.Confidence)
	}
}

func TestConfidenceConsentWallBoilerplate(t *testing.T) {
	t.Parallel()

	result := extractFor(t, `<html><body>
<div class="gate"><p>We use cookies and similar technologies to personalize content. Accept to continue.</p></div>
</body></html>`)

	if result.Confidence != 0.4 {
		t.Errorf("Confidence = %v, want 0.4 for short extraction opening with consent boilerplate", result.Confidence)
	}
}

func TestConfidenceLongArticleMentionsSubscribingUnaffected(t *testing.T) {
	t.Parallel()

	// A long, genuine article whose body merely mentions subscribing must not
	// be downgraded: the paywall tier applies only to short extractions.
	body := longParagraph(800)
	result := extractFor(t, `<html><body><article><h1>Normal Article</h1><p>`+body+`</p><p>Readers can subscribe to continue reading coverage like this in our weekly edition.</p></article></body></html>`)

	if result.ExtractionMethod != "article" {
		t.Errorf("ExtractionMethod = %q, want %q", result.ExtractionMethod, "article")
	}
	if result.Confidence != 1.0 {
		t.Errorf("Confidence = %v, want 1.0 for a long article that merely mentions subscribing", result.Confidence)
	}
}

func TestConfidenceEmptyExtraction(t *testing.T) {
	t.Parallel()

	result := extractFor(t, `<html><body></body></html>`)

	if result.Confidence != 0 {
		t.Errorf("Confidence = %v, want 0 for empty extraction", result.Confidence)
	}
}

// ---------------------------------------------------------------------------
// Heading markers and reading time (from p3_features_test.go)
// ---------------------------------------------------------------------------

func TestHeadingMarkersInText(t *testing.T) {
	t.Parallel()

	result := extractFor(t, `<html><body><article>
<h1>Top Title</h1>
<p>Intro paragraph.</p>
<h3>Subsection</h3>
<p>Subsection body.</p>
</article></body></html>`)

	if !strings.Contains(result.Text, "# Top Title") {
		t.Errorf("h1 marker missing:\n%q", result.Text)
	}
	if !strings.Contains(result.Text, "### Subsection") {
		t.Errorf("h3 marker missing:\n%q", result.Text)
	}
	// The ATX markers must not inflate WordCount.
	if result.WordCount != 7 { // Top Title Intro paragraph. Subsection Subsection body.
		t.Errorf("WordCount = %d, want 7 (heading markers are not words)", result.WordCount)
	}
}

func TestReadingTimeUsesCJKRate(t *testing.T) {
	t.Parallel()

	// 660 CJK characters in the paragraph plus 4 in the h1, at 350 chars/min;
	// the old single rate (200 words/min for the same 664 "words") would
	// overestimate the time by 75%.
	cjkParagraph := strings.Repeat("经济增长平稳", 100) // 600 chars
	cjkParagraph += strings.Repeat("政策持续发力", 10)  // +60 chars

	result := extractFor(t, `<html><body><article><h1>经济数据</h1><p>`+cjkParagraph+`</p></article></body></html>`)

	if result.WordCount != 664 { // 660 CJK paragraph chars + h1 经济数据 (4)
		t.Fatalf("WordCount = %d, want 664", result.WordCount)
	}
	want := time.Duration(664 * int(time.Minute) / 350)
	tolerance := 10 * time.Second
	if d := result.ReadingTime - want; d < -tolerance || d > tolerance {
		t.Errorf("ReadingTime = %v, want ≈ %v (664 CJK chars at 350 chars/min)", result.ReadingTime, want)
	}
}
