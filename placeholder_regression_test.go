package html_test

// placeholder_regression_test.go — regression tests for the inline
// placeholder contract between the text pass (ExtractTextWithStructureAndImages)
// and extractImagesAndLinks:
//   - dispatched tables must burn [IMAGE:n]/[LINK:n] positions for their
//     subtree so numbering stays in lockstep (table cells contain <img>/<a>
//     that the text walker never visits), and
//   - enabling only one of InlineImageFormat/InlineLinkFormat must not leak
//     the other kind's literal tokens into Text.

import (
	"strings"
	"testing"

	"github.com/cybergodev/html"
)

// TestTableDoesNotShiftInlinePlaceholders extracts a document whose table
// cells contain an image and a link between two paragraph-level elements.
// Before the fix, the table's elements were numbered by
// extractImagesAndLinks but never by the text pass, so every placeholder
// after the table substituted the wrong entry (the outro image rendered the
// table's image, and the table's link never appeared).
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
	if strings.Contains(res.Text, "![B](/b.png)") {
		t.Errorf("table-cell image substituted into inline text: %q", res.Text)
	}
	if strings.Contains(res.Text, "[T](https://table.example/t)") {
		t.Errorf("table-cell link substituted into inline text: %q", res.Text)
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
