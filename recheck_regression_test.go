package html

// recheck_regression_test.go — regressions for the D-002 re-check round:
// rel token matching (case-insensitive, multi-token), quoted-'>' tag
// boundaries in the raw media scanner, and the inline image title attribute
// in the html format.

import (
	"strings"
	"testing"
)

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
