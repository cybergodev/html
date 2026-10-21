package html

// regressions_url_test.go — regressions for the D-002 link-URL fixes, gathered
// from the former single-topic files:
//
//   - a literal '@' in a URL path ("/@alice" profile URLs) is no longer scanned
//     as userinfo, so same-site links survive IncludeExternalLinks=false;
//   - the <source> dispatch gate honors per-type Include* flags (a video
//     <source> must not appear when only audios are enabled, and vice versa);
//   - rel token matching (stylesheet/icon, case-insensitive, multi-token).

import (
	"testing"
)

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
