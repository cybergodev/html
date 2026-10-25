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
