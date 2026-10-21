package internal

// textcontent_script_regression_test.go — regression for the D-002 P3 fix:
// GetTextContent concatenated every text node in the subtree, including those
// inside <script>/<style>/<noscript>, whose contents are code rather than
// visible text. It now skips those subtrees, matching the main extraction
// pass (IsNonContentElement).

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestGetTextContentSkipsScriptStyle(t *testing.T) {
	t.Parallel()

	doc, err := html.Parse(strings.NewReader(
		`<html><body><a href="x"><script>evil()</script>click me<noscript>noscript text</noscript></a></body></html>`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	a := FindElementByTag(doc, "a")
	if a == nil {
		t.Fatal("<a> not found")
	}
	if got := GetTextContent(a); got != "click me" {
		t.Errorf("GetTextContent = %q, want %q", got, "click me")
	}

	// Mixed content: text around a style block still joins per the source's
	// own whitespace (a space is emitted only where the source had one).
	doc2, err := html.Parse(strings.NewReader(
		`<html><body><a href="x">alpha <style>.s{}</style> beta</a></body></html>`))
	if err != nil {
		t.Fatalf("Parse(2): %v", err)
	}
	a2 := FindElementByTag(doc2, "a")
	if got := GetTextContent(a2); got != "alpha beta" {
		t.Errorf("GetTextContent(2) = %q, want %q", got, "alpha beta")
	}
}
