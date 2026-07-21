package internal

import (
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// TestScoreArticleCandidatesMatchesNaiveLoop is the correctness guard for the
// article-scoring fast path. It asserts that ScoreArticleCandidates (the O(N)
// bottom-up fold) produces the identical candidate→score map as the
// pre-optimization loop — walk every non-inline element and call Score(n), which
// re-walks each subtree via collectContentMetrics. If this fails, the fast path
// must not ship: article selection would silently drift.
//
// The cases deliberately stress the parts where a bottom-up fold could diverge
// from a fresh per-node collectContentMetrics: skip-tag subtrees whose
// descendants are still scored (svg/math/nav/aside/footer), a block nested inside
// an <a> (linkTextLength), NBSP text, comma-rich prose, and deep nesting.
func TestScoreArticleCandidatesMatchesNaiveLoop(t *testing.T) {
	cases := []struct {
		name   string
		markup string
	}{
		{"plain article", `<html><body><article><h1>Title</h1><p>alpha, beta, gamma.</p><p>second paragraph here.</p></article></body></html>`},
		{"link dense nav", `<html><body><nav><a href="/a">A</a><a href="/b">B</a><a href="/c">C</a></nav><article><p>real content, here.</p></article></body></html>`},
		{"candidate inside anchor", `<html><body><a href="/x"><div><p>block inside a link</p><p>more text</p></div></a></body></html>`},
		{"nbsp text", "<html><body><p>non breaking space</p></body></html>"},
		{"skip tag subtrees with candidates", `<html><head><title>t</title></head><body><script>x</script><style>x</style><nav>n</nav><svg><text>s</text></svg><math><mi>m</mi></math><aside>a</aside><footer>f</footer><article><p>real, comma, content.</p></article></body></html>`},
		{"comma rich prose", `<html><body><article><p>one, two, three, four, five, six.</p><p>` + "，" + `fullwidth` + "，" + `test。</p></article></body></html>`},
		{"nested deep", `<html><body><div><div><div><section><p>deep, text.</p></section></div></div></div></body></html>`},
		{"empty body", `<html><body></body></html>`},
		{"mixed inline and block", `<html><body><article><h2>Head</h2><p>text with <a href="/l">a link</a> and <em>emphasis</em>.</p><ul><li>one</li><li>two</li></ul></article></body></html>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := parseMetricsDoc(t, tc.markup)
			ds := SharedDefaultScorer()

			// Naive: the pre-optimization per-node loop (extractArticleNode's old shape).
			naive := make(map[*html.Node]int)
			WalkNodes(doc, func(n *html.Node) bool {
				if n.Type == html.ElementNode && !IsInlineElement(n.Data) {
					if sc := ds.Score(n); sc > 0 {
						naive[n] = sc
					}
				}
				return true
			})

			fast := ds.ScoreArticleCandidates(doc)

			if len(naive) != len(fast) {
				// Report the differing nodes to make a regression actionable.
				missing, extra := diffCandidates(naive, fast)
				t.Fatalf("candidate count mismatch: naive=%d fast=%d\n  only-in-naive(score>0): %s\n  only-in-fast: %s",
					len(naive), len(fast), missing, extra)
			}
			for n, want := range naive {
				if got := fast[n]; got != want {
					t.Errorf("score mismatch for <%s>: naive=%d fast=%d", n.Data, want, got)
				}
			}
		})
	}
}

func parseMetricsDoc(t *testing.T, markup string) *html.Node {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(markup))
	if err != nil {
		t.Fatalf("html.Parse: %v", err)
	}
	return doc
}

// diffCandidates returns human-readable summaries of nodes present in one map but
// not the other, to help diagnose a candidate-set regression.
func diffCandidates(naive, fast map[*html.Node]int) (missing, extra string) {
	tags := func(n *html.Node) string { return "<" + n.Data + ">" }
	for n, sc := range naive {
		if _, ok := fast[n]; !ok {
			if missing != "" {
				missing += ", "
			}
			missing += tags(n) + "(" + strconv.Itoa(sc) + ")"
		}
	}
	for n := range fast {
		if _, ok := naive[n]; !ok {
			if extra != "" {
				extra += ", "
			}
			extra += tags(n)
		}
	}
	return missing, extra
}
