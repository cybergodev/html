package internal

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// textLengthCorpus holds documents chosen to hit every branch where
// TextContentLength could drift from len(GetTextContent): NBSP folding, CR
// dropping, inter-node space insertion, whitespace-only text nodes, raw-text
// subtree skips, CJK/emoji multi-byte text, and nested inline spacing.
var textLengthCorpus = []string{
	// Compact article with links (inter-node space inside split text runs).
	`<html><body><article><h1>Title</h1><p>Paragraph with <a href="/x">a link</a> and text.</p></article></body></html>`,

	// Pretty-printed markup: whitespace-only text nodes everywhere, each
	// carrying newlines, tabs, and CRs the normalizer must fold.
	"<html>\r\n  <body>\r\n    <div>\n\t  <p>one</p>\n\n\n      <p>two</p>\n    </div>\r\n  </body>\n</html>",

	// NBSP in every position: interior, leading, trailing, doubled.
	"<p> lead</p><p>trail </p><p>a b  c</p><p>  </p>",

	// CR-only normalization triggers and CR+LF mixes.
	"<div>a\rb\nc\r\nd</div><span> e\r </span>",

	// Raw-text subtrees are skipped entirely (script/style/noscript).
	`<div>visible<script>evil() {};</script><style>.x{color:red}</style><noscript>hidden</noscript>after</div>`,

	// Table cells (single text child fast path) and a structure-only table.
	`<table><tr><td>a</td><td> b&nbsp;c </td></tr><tr><th>H</th><td>x y</td></tr></table>`,

	// CJK and emoji: multi-byte text where byte lengths matter, no spaces.
	`<p>第1段包含链接以及正文文字内容，中文之间没有空格。</p><p>😀🎉🌍 emoji</p>`,

	// Nested inline elements with significant spaces around them.
	`<a>alpha <b>beta</b> gamma</a><span>x <em>y</em> <strong>z</strong></span>`,

	// Empty and whitespace-edge shapes.
	`<p></p><p> </p><p>&nbsp;</p><div><span></span></div>`,

	// Head/meta content that metricsSkipTags-style logic might see.
	`<html><head><title> T </title><meta charset="utf-8"></head><body><svg>ignore</svg>text</body></html>`,
}

// TestTextContentLengthMatchesGetTextContent pins the equivalence contract of
// TextContentLength: for every node of every corpus document, the computed
// length must equal the length of the string GetTextContent actually builds.
// GetTextContent is the behavior-defining implementation; this test fails
// whenever the two drift apart.
func TestTextContentLengthMatchesGetTextContent(t *testing.T) {
	checked := 0
	for _, doc := range textLengthCorpus {
		root, err := html.Parse(strings.NewReader(doc))
		if err != nil {
			t.Fatalf("parse corpus doc %q: %v", doc, err)
		}
		WalkNodes(root, func(n *html.Node) bool {
			want := len(GetTextContent(n))
			if got := TextContentLength(n); got != want {
				t.Errorf("TextContentLength(%s %q) = %d, want len(GetTextContent) = %d",
					nodeKind(n), n.Data, got, want)
			}
			checked++
			return true
		})
	}
	if checked == 0 {
		t.Fatal("corpus produced no nodes")
	}
}

// TestTextContentLengthEmptyInput covers the nil/empty degenerate inputs,
// which WalkNodes cannot reach from a parsed document.
func TestTextContentLengthEmptyInput(t *testing.T) {
	if got := TextContentLength(nil); got != 0 {
		t.Errorf("TextContentLength(nil) = %d, want 0", got)
	}
	empty := &html.Node{Type: html.ElementNode, Data: "p"}
	if got := TextContentLength(empty); got != 0 {
		t.Errorf("TextContentLength(empty p) = %d, want 0", got)
	}
}

// nodeKind labels a node for failure messages.
func nodeKind(n *html.Node) string {
	switch n.Type {
	case html.ElementNode:
		return "element"
	case html.TextNode:
		return "text"
	case html.DocumentNode:
		return "document"
	case html.CommentNode:
		return "comment"
	case html.DoctypeNode:
		return "doctype"
	default:
		return "other"
	}
}
