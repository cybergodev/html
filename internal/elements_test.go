package internal

// elements_test.go — unit tests for the element-classification helpers in
// elements.go (hasPresentationalRole, hasPrimaryContentClassToken,
// insidePrimaryContent). These back the P0-2 landmark rule ("header/footer/
// aside are page chrome only outside a primary-content container"); the
// end-to-end behavior is pinned by the root package's article_test.go, and
// these direct tests keep every token/role branch observable on its own.

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// parseElement parses markup and returns the first element carrying the given
// class, so each case can address the node it is about.
func parseElement(t *testing.T, markup, class string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(markup))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
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
	walk(doc)
	if found == nil {
		t.Fatalf("no element with class %q in markup", class)
	}
	return found
}

func TestHasPresentationalRole(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		markup string
		want   bool
	}{
		{"role presentation", `<table role="presentation"><tr><td>x</td></tr></table>`, true},
		{"role none", `<table role="none"><tr><td>x</td></tr></table>`, true},
		{"role case and spaces normalized", `<table role=" Presentation "><tr><td>x</td></tr></table>`, true},
		{"other role", `<table role="grid"><tr><td>x</td></tr></table>`, false},
		{"no role attribute", `<table><tr><td>x</td></tr></table>`, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			doc, err := html.Parse(strings.NewReader(tt.markup))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			var table *html.Node
			WalkNodes(doc, func(n *html.Node) bool {
				if table == nil && n.Type == html.ElementNode && n.Data == "table" {
					table = n
				}
				return true
			})
			if table == nil {
				t.Fatal("no <table> in markup")
			}
			if got := hasPresentationalRole(table); got != tt.want {
				t.Errorf("hasPresentationalRole(%s) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestHasPrimaryContentClassToken(t *testing.T) {
	t.Parallel()

	t.Run("positive tokens match word-bounded", func(t *testing.T) {
		t.Parallel()
		for _, class := range []string{"article", "post-body", "story", "entry-content", "body", "main-view"} {
			n := parseElement(t, `<div class="`+class+`"><p>x</p></div>`, class)
			if !hasPrimaryContentClassToken(n) {
				t.Errorf("class %q should be a primary-content token", class)
			}
		}
	})

	t.Run("non-tokens rejected", func(t *testing.T) {
		t.Parallel()
		for _, class := range []string{"page-content", "articleX", " Xarticle", "blog-nav", "layout"} {
			n := parseElement(t, `<div class="`+class+`"><p>x</p></div>`, class)
			if hasPrimaryContentClassToken(n) {
				t.Errorf("class %q must not be a primary-content token", class)
			}
		}
	})

	t.Run("id attribute counts too", func(t *testing.T) {
		t.Parallel()
		doc, err := html.Parse(strings.NewReader(`<div id="main-post"><p>x</p></div>`))
		if err != nil {
			t.Fatal(err)
		}
		var div *html.Node
		WalkNodes(doc, func(n *html.Node) bool {
			if n.Type == html.ElementNode && n.Data == "div" {
				div = n
			}
			return true
		})
		if !hasPrimaryContentClassToken(div) {
			t.Error("id with a primary-content token must count as a content wrapper")
		}
	})

	t.Run("nil and non-element nodes", func(t *testing.T) {
		t.Parallel()
		if hasPrimaryContentClassToken(nil) {
			t.Error("nil node must be false")
		}
		text := &html.Node{Type: html.TextNode, Data: "article"}
		if hasPrimaryContentClassToken(text) {
			t.Error("text node must be false")
		}
	})
}

func TestInsidePrimaryContent(t *testing.T) {
	t.Parallel()

	t.Run("footer inside positive-token wrapper", func(t *testing.T) {
		t.Parallel()
		n := parseElement(t, `<div class="entry"><p>a</p><footer class="foot"><p>b</p></footer></div>`, "foot")
		if !insidePrimaryContent(n) {
			t.Error("footer inside a primary-content wrapper must be inside primary content")
		}
	})

	t.Run("footer inside semantic article", func(t *testing.T) {
		t.Parallel()
		n := parseElement(t, `<article><p>a</p><footer class="foot"><p>b</p></footer></article>`, "foot")
		if !insidePrimaryContent(n) {
			t.Error("footer inside <article> must be inside primary content")
		}
	})

	t.Run("bare footer at page level", func(t *testing.T) {
		t.Parallel()
		n := parseElement(t, `<body><p>a</p><footer class="foot"><p>b</p></footer></body>`, "foot")
		if insidePrimaryContent(n) {
			t.Error("page-level footer must not be inside primary content")
		}
	})

	t.Run("landmark between node and wrapper stops the walk", func(t *testing.T) {
		t.Parallel()
		// The soft landmark (<header>) sits BETWEEN the footer and the
		// positive-token wrapper, so the upward walk hits the landmark first
		// and must stop: the wrapper above it is page chrome territory.
		n := parseElement(t, `<div class="entry"><header class="page-head"><footer class="foot"><p>b</p></footer></header></div>`, "foot")
		if insidePrimaryContent(n) {
			t.Error("walk continued past a soft landmark to the wrapper above it")
		}
	})
}
