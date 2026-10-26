package internal

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// benchClassHeavyHTML is the class/id-dense shape ShouldRemove sees on
// real-world CMS output: every element carries multi-token class values that
// force the fold + hidden-token + removal-pattern scans.
var benchClassHeavyHTML = `<div class="site-header header-wrapper" id="page-header">` +
	`<nav class="main-navigation nav-menu"><ul class="menu-list">` +
	strings.Repeat(`<li class="menu-item item-a"><a class="nav-link" href="/n">Nav</a></li>`, 40) +
	`</ul></nav></div>` +
	`<main class="content-area main-content" role="main">` +
	strings.Repeat(`<section class="section-block"><h2 class="section-title">H</h2>`+
		`<div class="text-wrapper prose"><p class="paragraph text-body">Body text, with commas, and more.</p></div></section>`, 60) +
	`</main>` +
	`<footer class="site-footer footer-bottom"><p class="copyright">© 2026</p></footer>`

// benchElements parses doc and returns its element nodes in document order.
func benchElements(b *testing.B, doc string) []*html.Node {
	b.Helper()
	parsed, err := html.Parse(strings.NewReader(doc))
	if err != nil {
		b.Fatal(err)
	}
	var elems []*html.Node
	WalkNodes(parsed, func(n *html.Node) bool {
		if n.Type == html.ElementNode {
			elems = append(elems, n)
		}
		return true
	})
	return elems
}

// BenchmarkShouldRemoveClassHeavy measures the removal decision the scoring
// fold and content cleaning make once per element on every extraction. The
// class-heavy shape is the worst case for the class/id scanning path.
func BenchmarkShouldRemoveClassHeavy(b *testing.B) {
	s := getDefaultScorer()
	elems := benchElements(b, benchClassHeavyHTML)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		remove := false
		for _, e := range elems {
			if s.ShouldRemove(e) {
				remove = !remove
			}
		}
		_ = remove
	}
}

// BenchmarkFoldAndScoreClassHeavy measures the full bottom-up scoring pass
// (per-element ShouldRemove + per-text-node metric folding) that selects the
// article node on every ExtractArticle extraction.
func BenchmarkFoldAndScoreClassHeavy(b *testing.B) {
	s := getDefaultScorer()
	parsed, err := html.Parse(strings.NewReader(benchClassHeavyHTML))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cc := &candidateCollector{bestScore: -1}
		s.foldAndScore(parsed, false, cc)
	}
}
