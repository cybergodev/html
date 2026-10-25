package internal

// scoring_p2_test.go — unit tests for the P2 text-quality features in
// applyScoringRules (data-grid and label-soup penalties), exercising the
// scoring rules directly with hand-built metrics.

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// scoreDivWithMetrics applies the default scorer's rules to a parsed <div>
// with the given metrics, isolating the rule under test from markup effects.
func scoreDivWithMetrics(t *testing.T, m contentMetrics) int {
	t.Helper()
	doc, err := parseHTML(`<div></div>`)
	if err != nil {
		t.Fatalf("parseHTML: %v", err)
	}
	var div *html.Node
	WalkNodes(doc, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "div" && div == nil {
			div = n
		}
		return true
	})
	if div == nil {
		t.Fatal("no div in parsed document")
	}
	scorer := NewDefaultScorer()
	return scorer.applyScoringRules(div, m)
}

func TestApplyScoringRulesDataGridPenalty(t *testing.T) {
	t.Parallel()

	prose := contentMetrics{
		paragraphCount: 4, headingCount: 0, blockCount: 10,
		textLength: 400, digitCount: 40, sentenceCount: 8, commaCount: 10,
	}
	grid := prose
	grid.digitCount = 200 // 50% digits — screener/stat-card shape

	if gridScore := scoreDivWithMetrics(t, grid); gridScore >= scoreDivWithMetrics(t, prose) {
		t.Errorf("digit-heavy metrics (%d digits/%d chars) must score below prose metrics", grid.digitCount, grid.textLength)
	}
}

func TestApplyScoringRulesLabelSoupPenalty(t *testing.T) {
	t.Parallel()

	prose := contentMetrics{
		paragraphCount: 4, blockCount: 10,
		textLength: 400, sentenceCount: 12, commaCount: 10,
	}
	labels := prose
	labels.sentenceCount = 0
	labels.commaCount = 0 // 400 chars, zero punctuation — tag-cloud shape

	if labelScore := scoreDivWithMetrics(t, labels); labelScore >= scoreDivWithMetrics(t, prose) {
		t.Errorf("punctuation-free metrics must score below punctuated prose metrics")
	}
}

func TestApplyScoringRulesShortTextExemptFromQualityPenalties(t *testing.T) {
	t.Parallel()

	// Short fragments (captions, headings, small table rows) must not be
	// penalized even when digit-heavy or punctuation-free: the features are
	// gated on longTextThreshold.
	short := contentMetrics{textLength: 120, digitCount: 110, blockCount: 2}
	if shortScore := scoreDivWithMetrics(t, short); shortScore == 0 {
		t.Errorf("short digit-heavy fragment scored 0; quality penalties must not gate on short text")
	}
}

func TestFindListingContainer(t *testing.T) {
	t.Parallel()

	doc, err := parseHTML(`<body><div class="feed">
	<article><h2><a href="/1">Card headline one</a></h2><p>Excerpt one with some more words here.</p></article>
	<article><h2><a href="/2">Card headline two</a></h2><p>Excerpt two with some more words here.</p></article>
	<article><h2><a href="/3">Card headline three</a></h2><p>Excerpt three with some more words here.</p></article>
	</div></body>`)
	if err != nil {
		t.Fatalf("parseHTML: %v", err)
	}
	if got := FindListingContainer(doc); got == nil {
		t.Fatal("FindListingContainer = nil for a 3-card link-dense feed")
	}
}

func TestFindListingContainerRejectsChromeWrapper(t *testing.T) {
	t.Parallel()

	doc, err := parseHTML(`<body><div class="related">
	<article><a href="/1">Card one</a></article>
	<article><a href="/2">Card two</a></article>
	<article><a href="/3">Card three</a></article>
	</div></body>`)
	if err != nil {
		t.Fatalf("parseHTML: %v", err)
	}
	if got := FindListingContainer(doc); got != nil {
		t.Fatalf("FindListingContainer returned a removal-marked wrapper (%v), want nil", got)
	}
}

func TestFindListingContainerNeedsThreeArticles(t *testing.T) {
	t.Parallel()

	doc, err := parseHTML(`<body><div class="feed">
	<article><a href="/1">Card one</a></article>
	<article><a href="/2">Card two</a></article>
	</div></body>`)
	if err != nil {
		t.Fatalf("parseHTML: %v", err)
	}
	if got := FindListingContainer(doc); got != nil {
		t.Fatalf("FindListingContainer returned a container for 2 cards, want nil (minimum 3)")
	}
}

// findNodeByClass returns the first element with the given class attribute.
func findNodeByClass(t *testing.T, root *html.Node, class string) *html.Node {
	t.Helper()
	var found *html.Node
	WalkNodes(root, func(n *html.Node) bool {
		if found != nil {
			return false
		}
		if n.Type == html.ElementNode {
			for _, attr := range n.Attr {
				if attr.Key == "class" && attr.Val == class {
					found = n
					return false
				}
			}
		}
		return true
	})
	if found == nil {
		t.Fatalf("no element with class %q", class)
	}
	return found
}

// TestClimbToContentWrapperGuards drives the climb's individual stop
// conditions directly. The end-to-end selection is pinned by the root
// package's article tests; here every guard branch (nil parent, body/html
// ceiling, removal-marked parent, dominance, score regression) is reached
// deterministically by passing bestScore explicitly.
func TestClimbToContentWrapperGuards(t *testing.T) {
	t.Parallel()

	scorer := NewDefaultScorer()
	climb := scorer.climbToContentWrapper

	t.Run("node directly under body stays", func(t *testing.T) {
		t.Parallel()
		doc, err := parseHTML(`<body><div class="prose"><p>body text here</p></div></body>`)
		if err != nil {
			t.Fatal(err)
		}
		best := findNodeByClass(t, doc, "prose")
		if got := climb(best, 100); got != best {
			t.Error("node under <body> was climbed past the body ceiling")
		}
	})

	t.Run("detached node stays", func(t *testing.T) {
		t.Parallel()
		doc, err := parseHTML(`<body><div class="prose"><p>text</p></div></body>`)
		if err != nil {
			t.Fatal(err)
		}
		best := findNodeByClass(t, doc, "prose")
		best.Parent.RemoveChild(best) // best becomes root: parent nil
		if got := climb(best, 100); got != best {
			t.Error("detached node was climbed")
		}
	})

	t.Run("removal-marked parent stops the climb", func(t *testing.T) {
		t.Parallel()
		doc, err := parseHTML(`<body><nav><div class="prose"><p>nav text content</p></div></nav></body>`)
		if err != nil {
			t.Fatal(err)
		}
		best := findNodeByClass(t, doc, "prose")
		if got := climb(best, 1); got != best {
			t.Error("climb entered a removal-marked parent (<nav>)")
		}
	})

	t.Run("dominance guard stops the climb", func(t *testing.T) {
		t.Parallel()
		// The sibling holds far more text than best, so best does not carry
		// at least half the parent's text.
		doc, err := parseHTML(`<body><div class="wrap"><div class="prose"><p>tiny</p></div><div><p>` +
			repeatWord("rival ", 300) + `</p></div></div></body>`)
		if err != nil {
			t.Fatal(err)
		}
		best := findNodeByClass(t, doc, "prose")
		if got := climb(best, 1); got != best {
			t.Error("climb widened past a rival sibling despite the dominance guard")
		}
	})

	t.Run("score regression stops the climb", func(t *testing.T) {
		t.Parallel()
		// A bestScore no ancestor can match (1<<30) must stop the first step.
		doc, err := parseHTML(`<body><div class="wrap"><div class="prose"><p>` + repeatWord("body ", 60) + `</p></div></div></body>`)
		if err != nil {
			t.Fatal(err)
		}
		best := findNodeByClass(t, doc, "prose")
		if got := climb(best, 1<<30); got != best {
			t.Error("climb continued although the parent scored below best")
		}
	})

	t.Run("dominating parent with a healthy score is climbed to", func(t *testing.T) {
		t.Parallel()
		// bestScore of 1 is matched by the wrapper (positive "post" class,
		// prose content), and best holds most of the parent's text.
		doc, err := parseHTML(`<body><div class="post"><div class="prose"><p>` + repeatWord("body ", 60) + `</p></div><p>tail</p></div></body>`)
		if err != nil {
			t.Fatal(err)
		}
		best := findNodeByClass(t, doc, "prose")
		parent := findNodeByClass(t, doc, "post")
		got := climb(best, 1)
		if got != parent {
			gotData := "<nil>"
			if got != nil {
				gotData = got.Data
			}
			t.Errorf("climb = %s, want the dominating wrapper .post (score gate should pass)", gotData)
		}
	})
}

// repeatWord builds s repeated n times (test-local convenience for long text).
func repeatWord(s string, n int) string {
	var b strings.Builder
	for range n {
		b.WriteString(s)
	}
	return b.String()
}

// TestAsciiTrimSpaceLower pins the single-pass trim+lowercase helper,
// including its empty result and non-ASCII fallback.
func TestAsciiTrimSpaceLower(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"   ", ""},
		{" \t\r\n ", ""},
		{"abc", "abc"},
		{"  Mixed Case  ", "mixed case"},
		{"\tTabs\tAnd\nNewlines\r", "tabs\tand\nnewlines"},
		// Only ASCII letters are lowercased: pure non-ASCII input takes the
		// no-allocation fast path and passes through unchanged.
		{"ÁÉÍÓÚ", "ÁÉÍÓÚ"},
		// ASCII uppercase inside non-ASCII input forces the strings.ToLower
		// fallback, which lowercases everything it can.
		{"ÁBC", "ábc"},
	}
	for _, tt := range tests {
		if got := asciiTrimSpaceLower(tt.in); got != tt.want {
			t.Errorf("asciiTrimSpaceLower(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

// TestGetTagScoreDegenerateReceivers pins the nil guards; unknown tags score
// zero on a configured scorer.
func TestGetTagScoreDegenerateReceivers(t *testing.T) {
	t.Parallel()

	var nilScorer *DefaultScorer
	if got := nilScorer.getTagScore("article"); got != 0 {
		t.Errorf("nil scorer getTagScore = %d, want 0", got)
	}
	if got := (&DefaultScorer{}).getTagScore("article"); got != 0 {
		t.Errorf("nil-config getTagScore = %d, want 0", got)
	}
	if got := NewDefaultScorer().getTagScore("definitely-not-a-real-tag"); got != 0 {
		t.Errorf("unknown tag score = %d, want 0", got)
	}
}
