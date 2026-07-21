package internal

import (
	"strconv"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// findScoreElement returns the first element matching tag and (optionally) the
// exact class attribute, walking the parsed tree in document order. An empty
// class matches any element of tag.
func findScoreElement(doc *html.Node, tag, class string) *html.Node {
	var found *html.Node
	WalkNodes(doc, func(n *html.Node) bool {
		if found != nil {
			return false
		}
		if n.Type != html.ElementNode || n.Data != tag {
			return true
		}
		if class != "" {
			for _, a := range n.Attr {
				if a.Key == "class" && a.Val == class {
					found = n
					return false
				}
			}
			return true
		}
		found = n
		return false
	})
	return found
}

// TestCollectMetricsIgnoresScriptText verifies that text inside <script> is not
// counted toward a candidate's content metrics. Without the skip, a page that
// inlines a large script payload (e.g. VitePress SSR data) would have its
// container's score driven by script text, not real content.
func TestCollectMetricsIgnoresScriptText(t *testing.T) {
	t.Parallel()

	plain := `<div><p>Real article content goes here.</p></div>`
	withScript := `<div><p>Real article content goes here.</p><script>` +
		strings.Repeat("var data = 1; ", 1000) + `</script></div>`

	plainDoc, err := parseHTML(plain)
	if err != nil {
		t.Fatal(err)
	}
	scriptDoc, err := parseHTML(withScript)
	if err != nil {
		t.Fatal(err)
	}

	plainScore := ScoreContentNode(findScoreElement(plainDoc, "div", ""))
	scriptScore := ScoreContentNode(findScoreElement(scriptDoc, "div", ""))

	if scriptScore != plainScore {
		t.Errorf("script text leaked into content metrics: plain=%d withScript=%d (want equal)",
			plainScore, scriptScore)
	}
}

// TestCollectMetricsIgnoresNavAndSvgText verifies that nav/header/svg subtrees
// do not inflate content metrics.
func TestCollectMetricsIgnoresNavAndSvgText(t *testing.T) {
	t.Parallel()

	plain := `<div><p>Body content here now.</p></div>`
	withNoise := `<div><p>Body content here now.</p>` +
		`<nav>` + strings.Repeat("menu link ", 200) + `</nav>` +
		`<header>` + strings.Repeat("site header ", 200) + `</header>` +
		`<svg><text>` + strings.Repeat("glyph ", 200) + `</text></svg></div>`

	plainDoc, err := parseHTML(plain)
	if err != nil {
		t.Fatal(err)
	}
	noiseDoc, err := parseHTML(withNoise)
	if err != nil {
		t.Fatal(err)
	}

	plainScore := ScoreContentNode(findScoreElement(plainDoc, "div", ""))
	noiseScore := ScoreContentNode(findScoreElement(noiseDoc, "div", ""))

	if noiseScore != plainScore {
		t.Errorf("nav/header/svg text leaked into content metrics: plain=%d noisy=%d (want equal)",
			plainScore, noiseScore)
	}
}

// TestLinkDensityPenaltyGatedByTextLength verifies the link-density penalty
// applies to short link-dense nodes (navigation) but is skipped for substantial
// link-wrapped content (card/portfolio grids).
func TestLinkDensityPenaltyGatedByTextLength(t *testing.T) {
	t.Parallel()

	scorer := NewDefaultScorer()

	// Small, link-dense node: a handful of short links. Nav-like, MUST be
	// penalized.
	smallDoc, err := parseHTML(`<div><a href="#">Link1</a><a href="#">Link2</a>Text</div>`)
	if err != nil {
		t.Fatal(err)
	}

	// Large, link-wrapped content: well over the 500-char threshold, all text
	// inside <a>, as on a project-card grid.
	var cards strings.Builder
	cards.WriteString(`<div>`)
	for i := 0; i < 30; i++ {
		cards.WriteString(`<a href="#p` + strconv.Itoa(i) + `">` +
			`Project card description with enough prose to exceed the threshold.` +
			`</a> `)
	}
	cards.WriteString(`</div>`)
	largeDoc, err := parseHTML(cards.String())
	if err != nil {
		t.Fatal(err)
	}

	smallScore := scorer.Score(findScoreElement(smallDoc, "div", ""))
	largeScore := scorer.Score(findScoreElement(largeDoc, "div", ""))

	if smallScore >= 100 {
		t.Errorf("small link-dense node should be penalized, got %d", smallScore)
	}
	// The large card grid must clear a score the x0.2 penalty would prevent.
	if largeScore < 400 {
		t.Errorf("large link-wrapped content should not be penalized, got %d", largeScore)
	}
	if largeScore <= smallScore {
		t.Errorf("large content must out-score small nav: large=%d small=%d", largeScore, smallScore)
	}
}

// TestScorePrefersCardContentOverHero is the end-to-end regression for the
// reported issue: a VitePress-style landing page where a small "main" hero
// block sits next to a larger set of link-wrapped project cards. Before the
// fix, the cards container was crushed by the link-density penalty and the hero
// (boosted by its "main" class) won article selection, discarding every card.
func TestScorePrefersCardContentOverHero(t *testing.T) {
	t.Parallel()

	hero := `<div class="main"><h1>CyberGo</h1><p>tagline here</p></div>`

	cardDesc := "这是一个项目卡片的描述文本，需要足够长以便在统计内容指标时超过文本阈值，" +
		"从而验证链接密度惩罚不会误伤被锚点元素包裹的真实正文内容。"
	var projects strings.Builder
	projects.WriteString(`<div class="home-projects">`)
	for i := 0; i < 6; i++ {
		projects.WriteString(`<div class="project-card"><a class="card-main" href="#p` +
			strconv.Itoa(i) + `">` + cardDesc + `</a></div>`)
	}
	projects.WriteString(`</div>`)

	doc, err := parseHTML(hero + projects.String())
	if err != nil {
		t.Fatal(err)
	}

	heroNode := findScoreElement(doc, "div", "main")
	projNode := findScoreElement(doc, "div", "home-projects")
	if heroNode == nil || projNode == nil {
		t.Fatalf("could not locate nodes: hero=%v projects=%v", heroNode != nil, projNode != nil)
	}

	heroScore := ScoreContentNode(heroNode)
	projScore := ScoreContentNode(projNode)

	if projScore <= heroScore {
		t.Errorf("project cards should out-score hero after fix: hero=%d projects=%d",
			heroScore, projScore)
	}
}
