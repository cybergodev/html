package html

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

import (
	"testing"
)

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

// TestExtractSourceLinksTypeCaseInsensitive pins the <source type> fix: a
// mixed-case MIME type must classify the resource. The src deliberately has
// no recognizable media extension, so only the type attribute can classify
// it — before the fix it fell through to the generic "media" class.
func TestExtractSourceLinksTypeCaseInsensitive(t *testing.T) {
	t.Parallel()

	p, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	doc := []byte(`<html><body><video><source src="/media/clip?id=1" type="VIDEO/MP4"></video></body></html>`)
	links, err := p.ExtractAllLinks(doc)
	if err != nil {
		t.Fatalf("ExtractAllLinks: %v", err)
	}

	for _, l := range links {
		if l.URL == "/media/clip?id=1" {
			if l.Type != "video" {
				t.Errorf("source with type=%q classified as %q, want %q", "VIDEO/MP4", l.Type, "video")
			}
			return
		}
	}
	t.Errorf("source URL /media/clip?id=1 missing from results: %+v", links)
}

// TestExtractLinkTagTypeCaseInsensitive pins the <link type> fallback fix: a
// <link> with no matching rel tokens must still be classified via a
// mixed-case type attribute. Before the fix the case-sensitive contains
// checks dropped the resource entirely.
func TestExtractLinkTagTypeCaseInsensitive(t *testing.T) {
	t.Parallel()

	p, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	doc := []byte(`<html><head><link type="text/CSS" href="/theme.css"></head></html>`)
	links, err := p.ExtractAllLinks(doc)
	if err != nil {
		t.Fatalf("ExtractAllLinks: %v", err)
	}

	for _, l := range links {
		if l.URL == "/theme.css" {
			if l.Type != "css" {
				t.Errorf("link with type=%q classified as %q, want %q", "text/CSS", l.Type, "css")
			}
			return
		}
	}
	t.Errorf("stylesheet /theme.css missing from results: %+v", links)
}
