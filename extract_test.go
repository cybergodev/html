package html

import (
	"context"
	"errors"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// TestContainsASCIIFold pins the boundary behavior of the unexported
// containsASCIIFold helper. The function ASCII-case-folds the *haystack* and
// compares against the needle verbatim, so per its sole production caller
// (extract.go, needle "nofollow") the needle must be supplied lower-case. These
// cases exercise that contract at its edges: case folding, no-match, and the
// empty / over-long needle boundaries.
func TestContainsASCIIFold(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		s      string
		substr string
		want   bool
	}{
		{"exact lowercase match", "rel=nofollow", "nofollow", true},
		{"mixed-case haystack", "rel=NoFollow", "nofollow", true},
		{"uppercase haystack", "REL=NOFOLLOW", "nofollow", true},
		{"substring embedded", "a nofollow b", "nofollow", true},
		{"no match different word", "rel=dofollow", "nofollow", false},
		{"needle longer than haystack", "nf", "nofollow", false},
		{"empty needle matches anywhere", "anything", "", true},
		{"empty haystack empty needle", "", "", true},
		{"empty haystack non-empty needle", "", "x", false},
		{"ascii prefix before multibyte tail", "caf\xc3\xa9", "caf", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := containsASCIIFold(tt.s, tt.substr); got != tt.want {
				t.Errorf("containsASCIIFold(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}

// TestPackageLevelWrappers_ConfigErrors drives the two shared error branches
// that all package-level Extract*/ExtractText* convenience wrappers funnel
// through:
//
//  1. resolveConfig rejecting >=2 variadic configs (ErrMultipleConfigs) — the
//     "multiple configs" subtests. Covers resolveConfig's default branch and
//     each wrapper's `if err != nil` early return.
//  2. withProcessor failing when New(cfg) rejects an invalid single config —
//     the "invalid config" subtests (processor_pool.go: the New(cfg) error
//     path). A negative MaxInputSize is rejected by Config.Validate, so the
//     processor is never built.
//
// Both paths return before any I/O or context work, so the file path passed to
// the *FromFile variants need not exist.
func TestPackageLevelWrappers_ConfigErrors(t *testing.T) {
	t.Parallel()

	valid := DefaultConfig()
	invalid := DefaultConfig()
	invalid.MaxInputSize = -1 // Config.Validate rejects this
	ctx := context.Background()
	htmlBytes := []byte("<p>x</p>")

	wrappers := []struct {
		name       string
		multi      func() error // passes two configs -> ErrMultipleConfigs
		oneInvalid func() error // passes one invalid config -> New() failure
	}{
		{"Extract", func() error {
			_, err := Extract(htmlBytes, valid, valid)
			return err
		}, func() error {
			_, err := Extract(htmlBytes, invalid)
			return err
		}},
		{"ExtractFromFile", func() error {
			_, err := ExtractFromFile("does-not-exist.html", valid, valid)
			return err
		}, func() error {
			_, err := ExtractFromFile("does-not-exist.html", invalid)
			return err
		}},
		{"ExtractText", func() error {
			_, err := ExtractText(htmlBytes, valid, valid)
			return err
		}, func() error {
			_, err := ExtractText(htmlBytes, invalid)
			return err
		}},
		{"ExtractTextFromFile", func() error {
			_, err := ExtractTextFromFile("does-not-exist.html", valid, valid)
			return err
		}, func() error {
			_, err := ExtractTextFromFile("does-not-exist.html", invalid)
			return err
		}},
		{"ExtractWithContext", func() error {
			_, err := ExtractWithContext(ctx, htmlBytes, valid, valid)
			return err
		}, func() error {
			_, err := ExtractWithContext(ctx, htmlBytes, invalid)
			return err
		}},
		{"ExtractFromFileWithContext", func() error {
			_, err := ExtractFromFileWithContext(ctx, "does-not-exist.html", valid, valid)
			return err
		}, func() error {
			_, err := ExtractFromFileWithContext(ctx, "does-not-exist.html", invalid)
			return err
		}},
		{"ExtractTextWithContext", func() error {
			_, err := ExtractTextWithContext(ctx, htmlBytes, valid, valid)
			return err
		}, func() error {
			_, err := ExtractTextWithContext(ctx, htmlBytes, invalid)
			return err
		}},
		{"ExtractTextFromFileWithContext", func() error {
			_, err := ExtractTextFromFileWithContext(ctx, "does-not-exist.html", valid, valid)
			return err
		}, func() error {
			_, err := ExtractTextFromFileWithContext(ctx, "does-not-exist.html", invalid)
			return err
		}},
	}

	for _, w := range wrappers {
		t.Run(w.name+"/multiple configs", func(t *testing.T) {
			t.Parallel()
			if err := w.multi(); err == nil {
				t.Errorf("%s with two configs: expected ErrMultipleConfigs, got nil", w.name)
			} else if !errors.Is(err, ErrMultipleConfigs) {
				t.Errorf("%s with two configs: expected ErrMultipleConfigs, got %v", w.name, err)
			}
		})
		t.Run(w.name+"/invalid config", func(t *testing.T) {
			t.Parallel()
			if err := w.oneInvalid(); err == nil {
				t.Errorf("%s with invalid config: expected validation error, got nil", w.name)
			}
		})
	}
}

// TestEscapeMarkdownText pins both branches of escapeMarkdownText. The fast path
// (input contains none of '\', '[', ']') returns the string unchanged without
// invoking the replacer; otherwise the replacer escapes those three characters so
// they cannot break Markdown link/image syntax. Link/image text almost never
// contains them, so the fast path is the hot one — both must be correct.
func TestEscapeMarkdownText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		// fast path: no escapable characters
		{"empty", "", ""},
		{"plain ascii", `plain text`, `plain text`},
		{"digits and spaces", `page 1 of 10`, `page 1 of 10`},
		{"unicode no escapable", `Hello 世界 café`, `Hello 世界 café`},
		// replacer path
		{"backslash", `a\b`, `a\\b`},
		{"open bracket", `a[b`, `a\[b`},
		{"close bracket", `a]b`, `a\]b`},
		{"all three", `a\b[c]d`, `a\\b\[c\]d`},
		{"only brackets", `[]`, `\[\]`},
		{"only backslash", `\`, `\\`},
		{"link-like text", `See [Section 5] \ref`, `See \[Section 5\] \\ref`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeMarkdownText(tt.in); got != tt.want {
				t.Errorf("escapeMarkdownText(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestEscapeMarkdownURLDest pins both branches of the Markdown destination
// escaper: destinations without parentheses pass through untouched, and any
// '(' / ')' is percent-encoded so it cannot terminate the destination early.
func TestEscapeMarkdownURLDest(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no parentheses passes through", "https://example.com/a/b.html", "https://example.com/a/b.html"},
		{"empty destination", "", ""},
		{"open paren encoded", "https://example.com/(a", "https://example.com/%28a"},
		{"close paren encoded", "https://example.com/a)", "https://example.com/a%29"},
		{"wikipedia-style parens", "https://en.wikipedia.org/wiki/Foo_(bar)", "https://en.wikipedia.org/wiki/Foo_%28bar%29"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := escapeMarkdownURLDest(tt.in); got != tt.want {
				t.Errorf("escapeMarkdownURLDest(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestProcessorNilGuardMethods pins the nil-receiver guards of the Processor
// convenience methods (processor.go): a nil Processor must be a silent no-op /
// nil return, never a panic.
func TestProcessorNilGuardMethods(t *testing.T) {
	t.Parallel()

	var p *Processor
	if got := p.GetAuditLog(); got != nil {
		t.Errorf("GetAuditLog() on nil = %v, want nil", got)
	}
	p.ClearAuditLog()   // must not panic
	p.ClearCache()      // must not panic
	p.ResetStatistics() // must not panic
}

// findElementByClass walks root depth-first and returns the first element
// node whose class attribute equals class.
func findElementByClass(root *html.Node, class string) *html.Node {
	var found *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found != nil || n == nil {
			return
		}
		if n.Type == html.ElementNode {
			for _, attr := range n.Attr {
				if attr.Key == "class" && attr.Val == class {
					found = n
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return found
}

// TestExpandToArticleClusterGuards drives the cluster-widening guard chain
// (extract.go) directly on parsed nodes. The end-to-end behavior is pinned by
// article_test.go; this unit test reaches every individual guard branch so a
// future edit cannot silently disable one of them.
func TestExpandToArticleClusterGuards(t *testing.T) {
	t.Parallel()

	p, err := New(DefaultConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = p.Close() }()

	parse := func(t *testing.T, markup string) *html.Node {
		t.Helper()
		doc, err := html.Parse(strings.NewReader(markup))
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		return doc
	}

	t.Run("nil node returns nil", func(t *testing.T) {
		if got := p.expandToArticleCluster(nil); got != nil {
			t.Errorf("expandToArticleCluster(nil) = %v, want nil", got)
		}
	})

	t.Run("role=article container returned unchanged", func(t *testing.T) {
		doc := parse(t, `<body><div role="article" class="best"><p>text</p></div></body>`)
		best := findElementByClass(doc, "best")
		if got := p.expandToArticleCluster(best); got != best {
			t.Error("role=article node was widened although it already delimits the article")
		}
	})

	t.Run("body-level parent not widened", func(t *testing.T) {
		doc := parse(t, `<body><div class="best"><p>`+strings.Repeat("word ", 30)+`</p></div></body>`)
		best := findElementByClass(doc, "best")
		if got := p.expandToArticleCluster(best); got != best {
			t.Error("node directly under <body> was widened")
		}
	})

	t.Run("chrome parent not widened", func(t *testing.T) {
		doc := parse(t, `<body><nav><div class="best"><p>`+strings.Repeat("word ", 30)+`</p></div></nav></body>`)
		best := findElementByClass(doc, "best")
		if got := p.expandToArticleCluster(best); got != best {
			t.Error("node with a removal-marked parent (<nav>) was widened")
		}
	})

	t.Run("dominance guard blocks rival sibling", func(t *testing.T) {
		// The sibling holds far more text than best, so best does not dominate
		// the parent: competing sections, not a split article.
		doc := parse(t, `<body><div class="wrap"><div class="best"><p>tiny block</p></div>`+
			`<div><p>`+strings.Repeat("rival ", 200)+`</p></div></div></body>`)
		best := findElementByClass(doc, "best")
		if got := p.expandToArticleCluster(best); got != best {
			t.Error("node widened although a rival sibling holds most of the parent text")
		}
	})

	t.Run("short or inline siblings block widening", func(t *testing.T) {
		// The only sibling is a short block (< minArticleSiblingChars of text)
		// plus an inline <span>: neither qualifies, so no widening.
		doc := parse(t, `<body><div class="wrap"><div class="best"><p>`+strings.Repeat("word ", 30)+`</p></div>`+
			`<div><p>short</p></div><span>inline</span></div></body>`)
		best := findElementByClass(doc, "best")
		if got := p.expandToArticleCluster(best); got != best {
			t.Error("node widened although no content-bearing block sibling exists")
		}
	})

	t.Run("content sibling under wrapper widens to parent", func(t *testing.T) {
		doc := parse(t, `<body><div class="wrap"><div class="best"><p>`+strings.Repeat("word ", 60)+`</p></div>`+
			`<div><p>`+strings.Repeat("sibling ", 10)+`</p></div></div></body>`)
		best := findElementByClass(doc, "best")
		parent := findElementByClass(doc, "wrap")
		if got := p.expandToArticleCluster(best); got != parent {
			t.Error("split-article shape was not widened to the content wrapper")
		}
	})
}

// TestContainsLowConfidencePhrase drives the paywall/consent phrase scan
// directly, including the rune-boundary back-off on inputs longer than
// paywallScanChars (800) that the public API only reaches incidentally.
func TestContainsLowConfidencePhrase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		text string
		want bool
	}{
		{"short matching phrase", "Please subscribe to continue reading this article.", true},
		{"short non-matching text", "The quarterly report covers three regions.", false},
		{"empty text", "", false},
		{"phrase beyond the scan window of a long text", strings.Repeat("x", 900) + " we use cookies and track you", false},
		{"phrase inside the scan window of a long text", strings.Repeat("x", 700) + " accept to continue " + strings.Repeat("y", 200), true},
		// A CJK lead-in longer than the byte cap forces the rune-boundary
		// back-off loop before the phrase is scanned.
		{"multibyte lead-in beyond the window", strings.Repeat("中文内容", 250) + " subscribe to read more", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := containsLowConfidencePhrase(tt.text); got != tt.want {
				t.Errorf("containsLowConfidencePhrase(len=%d) = %v, want %v", len(tt.text), got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Position lookup and body-length boundaries ([FIX-001] boundary additions)
// ---------------------------------------------------------------------------

// TestLinkByPositionBoundaries pins linkByPosition's guard and fallback
// branches: non-positive positions stay not-found, skipped positions in an
// ascending slice miss, and a non-ascending (hand-built) slice takes the
// last-wins linear scan instead of the binary search.
func TestLinkByPositionBoundaries(t *testing.T) {
	t.Parallel()

	ascending := []LinkInfo{
		{URL: "a", Position: 1},
		{URL: "b", Position: 2},
		{URL: "c", Position: 4}, // ascending, position 3 skipped
	}
	if l, ok := linkByPosition(ascending, 2); !ok || l.URL != "b" {
		t.Errorf("linkByPosition(ascending, 2) = (%v, %v), want (b, true)", l.URL, ok)
	}
	if _, ok := linkByPosition(ascending, 3); ok {
		t.Error("linkByPosition(ascending, 3) found a skipped position, want not-found")
	}
	if _, ok := linkByPosition(ascending, 9); ok {
		t.Error("linkByPosition(ascending, 9) = found, want not-found past the end")
	}
	for _, pos := range []int{0, -1} {
		if _, ok := linkByPosition(ascending, pos); ok {
			t.Errorf("linkByPosition(ascending, %d) = found, want not-found for non-positive positions", pos)
		}
	}

	manual := []LinkInfo{
		{URL: "first", Position: 2},
		{URL: "second", Position: 1},
		{URL: "dup-late", Position: 2},
	}
	if l, ok := linkByPosition(manual, 2); !ok || l.URL != "dup-late" {
		t.Errorf("linkByPosition(non-ascending, 2) = (%v, %v), want last-wins (dup-late, true)", l.URL, ok)
	}
	if _, ok := linkByPosition(manual, 3); ok {
		t.Error("linkByPosition(non-ascending, 3) = found, want not-found")
	}
}

// TestBodyTextLengthBoundaries pins bodyTextLength's no-body fallback: a
// fragment node with no <body> descendant measures zero instead of panicking,
// while a full document measures its body text.
func TestBodyTextLengthBoundaries(t *testing.T) {
	t.Parallel()

	p, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	fragment := &html.Node{Type: html.ElementNode, Data: "div"}
	if got := p.bodyTextLength(fragment); got != 0 {
		t.Errorf("bodyTextLength(body-less fragment) = %d, want 0", got)
	}

	doc, err := html.Parse(strings.NewReader(`<html><body><p>hello world</p></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if got := p.bodyTextLength(doc); got == 0 {
		t.Error("bodyTextLength(full document) = 0, want the body's text length")
	}
}

// TestExtractInlineDataURLImage pins C-2 end to end: a base64 inline image
// between MaxURLLength and MaxDataURILength is collected. IsValidURL's flat
// MaxURLLength gate used to reject every real inline image before its data:
// branch (and its 100KB ceiling) could ever run, silently dropping it from
// Result.Images while the DOM sanitizer kept the attribute.
func TestExtractInlineDataURLImage(t *testing.T) {
	t.Parallel()

	src := "data:image/png;base64," + strings.Repeat("A", 3000) // > MaxURLLength (2000), < MaxDataURILength (100000)
	doc := `<html><body><article><h1>Title</h1><img src="` + src + `" alt="chart"><p>Body text long enough for stable article selection by the scoring pass.</p></article></body></html>`

	res, err := Extract([]byte(doc))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if len(res.Images) != 1 {
		t.Fatalf("images = %d, want 1 (a data URL within MaxDataURILength must be collected)", len(res.Images))
	}
	if res.Images[0].URL != src {
		t.Fatalf("image URL = %.40s…, want the data URL verbatim", res.Images[0].URL)
	}
}

// TestExtractOverflowHiddenCardKept pins C-3 end to end: Tailwind's
// overflow-hidden wrapper (a paint clip, fully visible content) must not
// remove the card subtree from the extraction. Before the visible-compound
// exemption, matchesHiddenClassToken matched the trailing "hidden" of
// "overflow-hidden" and ShouldRemove dropped the whole card.
func TestExtractOverflowHiddenCardKept(t *testing.T) {
	t.Parallel()

	doc := `<html><body><nav>Home News About</nav><article><h1>Card demo</h1><div class="rounded-xl overflow-hidden shadow"><img src="/shot.png" alt="screenshot"><p>CARD BODY TEXT with plenty of words so the card carries substantial visible content for the check.</p></div><p>Trailing paragraph outside the card, also with a fair number of words for selection stability.</p></article><footer>Copyright</footer></body></html>`

	res, err := Extract([]byte(doc))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !strings.Contains(res.Text, "CARD BODY TEXT") {
		t.Fatalf("text = %q, want the visible overflow-hidden card kept", res.Text)
	}
	if len(res.Images) != 1 {
		t.Fatalf("images = %d, want 1 (the card's image must survive with it)", len(res.Images))
	}
}

// TestExtractTableTemplateRowsNotRendered pins M-2: rows nested inside a
// <template> between table rows are inert content — never rendered by a
// browser — and must not appear in the rendered table on either path. The
// plain path (both inline formats "none", the DefaultConfig) used to descend
// into the template and render its rows while the token path dropped them, so
// an unrelated format knob changed the table's content.
func TestExtractTableTemplateRowsNotRendered(t *testing.T) {
	t.Parallel()

	doc := `<html><body><article>
		<table>
			<tr><th>Name</th></tr>
			<template><tr><td>TPLROW</td></tr></template>
			<tr><td>RealRow</td></tr>
		</table>
		<p>Tail paragraph with enough text for article selection to be stable.</p>
		</article></body></html>`

	plain, err := ExtractText([]byte(doc))
	if err != nil {
		t.Fatalf("ExtractText: %v", err)
	}
	if strings.Contains(plain, "TPLROW") {
		t.Fatalf("plain path rendered the template row: %q", plain)
	}
	if !strings.Contains(plain, "RealRow") {
		t.Fatalf("plain path lost the real row: %q", plain)
	}

	md, err := ExtractToMarkdown([]byte(doc))
	if err != nil {
		t.Fatalf("ExtractToMarkdown: %v", err)
	}
	if strings.Contains(md, "TPLROW") {
		t.Fatalf("token path rendered the template row: %q", md)
	}
	if !strings.Contains(md, "RealRow") {
		t.Fatalf("token path lost the real row: %q", md)
	}
}
