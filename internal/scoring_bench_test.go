package internal

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// benchArticleDoc builds a rich article document comparable to the realistic
// extraction workload: nav, many sections (heading + paragraph + link), images,
// and a table. This is the input shape that made extractArticleNode's per-candidate
// collectContentMetrics re-walks O(N²).
func benchArticleDoc(b *testing.B) *html.Node {
	b.Helper()
	var sb strings.Builder
	sb.WriteString(`<html><head><title>Bench</title></head><body>`)
	sb.WriteString(`<nav><a href="/home">Home</a><a href="/about">About</a></nav>`)
	sb.WriteString(`<article><h1>Main Title</h1>`)
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&sb, "<h2>Section %d</h2>", i)
		fmt.Fprintf(&sb, `<p>Paragraph %d has <a href="/p/%d">a link</a> and prose, with commas, here.</p>`, i, i)
		if i%5 == 0 {
			fmt.Fprintf(&sb, `<img src="/img/%d.jpg" alt="Image %d">`, i, i)
		}
	}
	sb.WriteString(`<table>`)
	for r := 0; r < 10; r++ {
		sb.WriteString("<tr><td>a</td><td>b</td><td>c</td></tr>")
	}
	sb.WriteString(`</table></article><aside>x</aside><footer>f</footer></body></html>`)

	doc, err := html.Parse(strings.NewReader(sb.String()))
	if err != nil {
		b.Fatalf("parse: %v", err)
	}
	return doc
}

// BenchmarkArticleScoring_Naive reproduces the pre-optimization loop: walk the
// tree and call Score(n) per non-inline element, where each Score re-walks n's
// subtree via collectContentMetrics. This is the baseline for the O(N²) cost.
func BenchmarkArticleScoring_Naive(b *testing.B) {
	doc := benchArticleDoc(b)
	ds := SharedDefaultScorer()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		candidates := make(map[*html.Node]int, 32)
		WalkNodes(doc, func(n *html.Node) bool {
			if n.Type == html.ElementNode && !IsInlineElement(n.Data) {
				if sc := ds.Score(n); sc > 0 {
					candidates[n] = sc
				}
			}
			return true
		})
		_ = candidates
	}
}

// BenchmarkArticleScoring_Fast measures ScoreArticleCandidates: a single bottom-up
// fold that scores each candidate from its just-computed metrics, with no subtree
// re-walk. Run alongside the naive benchmark so both share the same machine state —
// the fast/naive ratio is the speedup, independent of absolute load.
func BenchmarkArticleScoring_Fast(b *testing.B) {
	doc := benchArticleDoc(b)
	ds := SharedDefaultScorer()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = ds.ScoreArticleCandidates(doc)
	}
}
