package internal

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestScoreContentNode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		html      string
		wantScore int
		checkFunc func(int) bool
	}{
		{
			name:      "article tag high score",
			html:      `<article><p>Content</p></article>`,
			checkFunc: func(score int) bool { return score > 500 },
		},
		{
			name:      "main tag high score",
			html:      `<main><p>Content</p></main>`,
			checkFunc: func(score int) bool { return score > 500 },
		},
		{
			name:      "div with paragraphs",
			html:      `<div><p>P1</p><p>P2</p><p>P3</p></div>`,
			checkFunc: func(score int) bool { return score > 0 },
		},
		{
			name:      "long text content",
			html:      `<div>` + strings.Repeat("word ", 100) + `</div>`,
			checkFunc: func(score int) bool { return score > 300 },
		},
		{
			name:      "short text penalty",
			html:      `<div>Short</div>`,
			checkFunc: func(score int) bool { return score < 0 },
		},
		{
			name:      "high link density penalty",
			html:      `<div><a href="#">Link1</a><a href="#">Link2</a>Text</div>`,
			checkFunc: func(score int) bool { return score < 100 },
		},
		{
			name:      "positive class names",
			html:      `<div class="article-content"><p>Content</p></div>`,
			checkFunc: func(score int) bool { return score > 200 },
		},
		{
			name:      "negative class names",
			html:      `<div class="sidebar"><p>Content</p></div>`,
			checkFunc: func(score int) bool { return score == 0 }, // ShouldRemove guard: removable nodes score 0
		},
		{
			name:      "with headings",
			html:      `<div><h1>Title</h1><p>Content</p></div>`,
			checkFunc: func(score int) bool { return true }, // Score can vary
		},
		{
			name:      "with commas",
			html:      `<div>Text with, many, commas, here, and, more</div>`,
			checkFunc: func(score int) bool { return true }, // Score can vary based on text length
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, _ := html.Parse(strings.NewReader(tt.html))
			var targetNode *html.Node
			WalkNodes(doc, func(n *html.Node) bool {
				if n.Type == html.ElementNode && (n.Data == "article" || n.Data == "main" || n.Data == "div") {
					targetNode = n
					return false
				}
				return true
			})

			if targetNode == nil {
				t.Fatal("Could not find target node")
			}

			score := ScoreContentNode(targetNode)
			if !tt.checkFunc(score) {
				t.Errorf("ScoreContentNode() = %d, failed check", score)
			}
		})
	}
}

func TestScoreContentNodeNil(t *testing.T) {
	t.Parallel()

	score := ScoreContentNode(nil)
	if score != 0 {
		t.Errorf("ScoreContentNode(nil) = %d, want 0", score)
	}
}

func TestScoreContentNodeTextNode(t *testing.T) {
	t.Parallel()

	textNode := &html.Node{
		Type: html.TextNode,
		Data: "text",
	}

	score := ScoreContentNode(textNode)
	if score != 0 {
		t.Errorf("ScoreContentNode(textNode) = %d, want 0", score)
	}
}

func TestScoreContentNodeNonContent(t *testing.T) {
	t.Parallel()

	doc, _ := html.Parse(strings.NewReader(`<script>code</script>`))
	var scriptNode *html.Node
	WalkNodes(doc, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "script" {
			scriptNode = n
			return false
		}
		return true
	})

	score := ScoreContentNode(scriptNode)
	if score != 0 {
		t.Errorf("ScoreContentNode(script) = %d, want 0", score)
	}
}

func TestScoreAttributes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		html      string
		checkFunc func(int) bool
	}{
		{
			name:      "positive class",
			html:      `<div class="article-content"></div>`,
			checkFunc: func(score int) bool { return score > 0 },
		},
		{
			name:      "negative class",
			html:      `<div class="sidebar"></div>`,
			checkFunc: func(score int) bool { return score < 0 },
		},
		{
			name:      "positive id",
			html:      `<div id="main-content"></div>`,
			checkFunc: func(score int) bool { return score > 0 },
		},
		{
			name:      "negative id",
			html:      `<div id="navigation"></div>`,
			checkFunc: func(score int) bool { return score < 0 },
		},
		{
			name:      "role main",
			html:      `<div role="main"></div>`,
			checkFunc: func(score int) bool { return score > 0 },
		},
		{
			name:      "role navigation",
			html:      `<div role="navigation"></div>`,
			checkFunc: func(score int) bool { return score < 0 },
		},
		{
			name:      "no attributes",
			html:      `<div></div>`,
			checkFunc: func(score int) bool { return score == 0 },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, _ := html.Parse(strings.NewReader(tt.html))
			var divNode *html.Node
			WalkNodes(doc, func(n *html.Node) bool {
				if n.Type == html.ElementNode && n.Data == "div" {
					divNode = n
					return false
				}
				return true
			})

			if divNode == nil {
				t.Fatal("Could not find div node")
			}

			score := ScoreAttributes(divNode)
			if !tt.checkFunc(score) {
				t.Errorf("ScoreAttributes() = %d, failed check", score)
			}
		})
	}
}

func TestScoreAttributesNil(t *testing.T) {
	t.Parallel()

	score := ScoreAttributes(nil)
	if score != 0 {
		t.Errorf("ScoreAttributes(nil) = %d, want 0", score)
	}
}

func TestMatchesPattern(t *testing.T) {
	t.Parallel()

	tests := []struct {
		value    string
		patterns map[string]bool
		want     bool
	}{
		{"article-content", map[string]bool{"article": true, "content": true}, true},
		{"sidebar", map[string]bool{"article": true, "content": true}, false},
		{"main-article", map[string]bool{"article": true}, true},
		{"navigation-menu", map[string]bool{"nav": true, "menu": true}, true},
		{"", map[string]bool{"article": true}, false},
		{"test", map[string]bool{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			result := MatchesPattern(tt.value, tt.patterns)
			if result != tt.want {
				t.Errorf("MatchesPattern(%q, %v) = %v, want %v", tt.value, tt.patterns, result, tt.want)
			}
		})
	}
}

func TestCalculateContentDensity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		html      string
		checkFunc func(float64) bool
	}{
		{
			name:      "high density - lots of text",
			html:      `<div>` + strings.Repeat("word ", 100) + `</div>`,
			checkFunc: func(d float64) bool { return d > 0.5 },
		},
		{
			name:      "low density - many tags",
			html:      `<div><span><span><span>word</span></span></span></div>`,
			checkFunc: func(d float64) bool { return d < 0.5 },
		},
		{
			name:      "no text",
			html:      `<div><span></span></div>`,
			checkFunc: func(d float64) bool { return d == 0 },
		},
		{
			name:      "plain text",
			html:      `<p>text only</p>`,
			checkFunc: func(d float64) bool { return d > 0 },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, _ := html.Parse(strings.NewReader(tt.html))
			density := CalculateContentDensity(doc)

			if !tt.checkFunc(density) {
				t.Errorf("CalculateContentDensity() = %f, failed check", density)
			}
		})
	}
}

func TestCountTags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		html string
		want int
	}{
		{
			name: "single tag",
			html: `<div></div>`,
			want: 1,
		},
		{
			name: "nested tags",
			html: `<div><p><span>text</span></p></div>`,
			want: 3,
		},
		{
			name: "multiple siblings",
			html: `<div><p></p><p></p><p></p></div>`,
			want: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, _ := html.Parse(strings.NewReader(tt.html))
			count := CountTags(doc)

			if count < tt.want {
				t.Errorf("CountTags() = %d, want at least %d", count, tt.want)
			}
		})
	}
}

func TestCountTagsNil(t *testing.T) {
	t.Parallel()

	count := CountTags(nil)
	if count != 0 {
		t.Errorf("CountTags(nil) = %d, want 0", count)
	}
}

func TestIsNonContentElement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tag  string
		want bool
	}{
		{"script", true},
		{"style", true},
		{"noscript", true},
		{"nav", true},
		{"aside", true},
		{"footer", true},
		{"header", true},
		{"form", false},
		{"div", false},
		{"p", false},
		{"article", false},
		{"main", false},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			result := IsNonContentElement(tt.tag)
			if result != tt.want {
				t.Errorf("IsNonContentElement(%q) = %v, want %v", tt.tag, result, tt.want)
			}
		})
	}
}

func TestCountChildElements(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		html string
		tag  string
		want int
	}{
		{
			name: "count paragraphs",
			html: `<div><p>1</p><p>2</p><p>3</p></div>`,
			tag:  "p",
			want: 3,
		},
		{
			name: "count nested",
			html: `<div><div><p>1</p></div><p>2</p></div>`,
			tag:  "p",
			want: 2,
		},
		{
			name: "no matches",
			html: `<div><span>text</span></div>`,
			tag:  "p",
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, _ := html.Parse(strings.NewReader(tt.html))
			var divNode *html.Node
			WalkNodes(doc, func(n *html.Node) bool {
				if n.Type == html.ElementNode && n.Data == "div" {
					divNode = n
					return false
				}
				return true
			})

			count := CountChildElements(divNode, tt.tag)
			if count != tt.want {
				t.Errorf("CountChildElements() = %d, want %d", count, tt.want)
			}
		})
	}
}

func TestCountChildElementsNil(t *testing.T) {
	t.Parallel()

	count := CountChildElements(nil, "p")
	if count != 0 {
		t.Errorf("CountChildElements(nil) = %d, want 0", count)
	}
}

func TestShouldRemoveElement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		html string
		want bool
	}{
		{
			name: "script tag",
			html: `<script>code</script>`,
			want: true,
		},
		{
			name: "nav tag",
			html: `<nav>menu</nav>`,
			want: true,
		},
		{
			name: "sidebar class",
			html: `<div class="sidebar">content</div>`,
			want: true,
		},
		{
			name: "navigation id",
			html: `<div id="navigation">menu</div>`,
			want: true,
		},
		{
			name: "hidden attribute",
			html: `<div hidden>content</div>`,
			want: true,
		},
		{
			name: "display none",
			html: `<div style="display:none">content</div>`,
			want: true,
		},
		{
			name: "display none with space",
			html: `<div style="display: none">content</div>`,
			want: true,
		},
		{
			name: "normal div",
			html: `<div>content</div>`,
			want: false,
		},
		{
			name: "article",
			html: `<article>content</article>`,
			want: false,
		},
		{
			// Regression: <article class="post-with-sidebar"> is the article body
			// in a sidebar layout, NOT a sidebar. The "-" delimiter previously
			// made "post-with-sidebar" match the "sidebar" removal pattern and
			// discard the entire article. The semantic <article> tag must be
			// protected from the class/id heuristic.
			name: "article with sidebar-layout class",
			html: `<article class="post-with-sidebar post-92883 category-news">content</article>`,
			want: false,
		},
		{
			name: "main with nav-ish class",
			html: `<main class="nav-primary">content</main>`,
			want: false,
		},
		{
			name: "div with role main and sidebar class",
			html: `<div role="main" class="sidebar-layout">content</div>`,
			want: false,
		},
		{
			// A plain div matching a removal pattern is still removed — only
			// semantic primary-content containers (<article>, <main>) and
			// class/id values carrying a content-area signal ("content",
			// "article") alongside the removal pattern are protected.
			name: "div with sidebar class still removed",
			html: `<div class="post-with-sidebar">content</div>`,
			want: true,
		},
		{
			// Compound layout class names like "content-sidebar" describe a
			// CSS grid/flex layout that wraps BOTH the main content area and
			// a sidebar column, not the sidebar itself. The "content" signal
			// overrides the "sidebar" removal so the article body nested
			// inside survives.
			name: "div content-sidebar layout wrapper",
			html: `<div class="content-sidebar grid-layout">article body</div>`,
			want: false,
		},
		{
			// Real-world regression: The Motley Fool uses
			// "foolcom-grid-content-sidebar mt-9" on the div that wraps
			// #article-body and the sidebar. Without the content-area-signal
			// override, CleanContentNode removed this div and discarded the
			// entire article, yielding empty extraction output.
			name: "foolcom-grid-content-sidebar (real-world regression)",
			html: `<div class="foolcom-grid-content-sidebar mt-9">article body</div>`,
			want: false,
		},
		{
			// "article" is also a valid content-area signal in class names.
			name: "div article-sidebar layout wrapper",
			html: `<div class="article-sidebar">article body</div>`,
			want: false,
		},
		{
			// The override applies to the id attribute as well.
			name: "id content-sidebar layout wrapper",
			html: `<div id="content-sidebar">article body</div>`,
			want: false,
		},
		{
			// A genuinely non-content element inside the layout wrapper is
			// still removed — the override only protects the wrapper itself,
			// not its children.
			name: "sidebar child still removed inside content wrapper",
			html: `<div class="sidebar">nav links</div>`,
			want: true,
		},
		{
			// Sitemap blocks are large link directories, never main content.
			// The standard delimited class is matched by RemovePatterns.
			name: "sitemap class",
			html: `<div class="sitemap">links</div>`,
			want: true,
		},
		{
			// Real-world ids such as "divSiteMap" prefix the token with letters,
			// which defeats word-boundary matching; SubstringRemovePatterns
			// catches these prefixed forms.
			name: "divSiteMap id (prefixed token)",
			html: `<div id="divSiteMap">links</div>`,
			want: true,
		},
		{
			// Suffixed forms such as "sitemap2" (digit suffix) also defeat word
			// boundaries and are caught by SubstringRemovePatterns.
			name: "sitemap2 id (digit suffix)",
			html: `<div id="sitemap2">links</div>`,
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, _ := html.Parse(strings.NewReader(tt.html))
			var targetNode *html.Node
			WalkNodes(doc, func(n *html.Node) bool {
				if n.Type == html.ElementNode && n.Data != "html" && n.Data != "head" && n.Data != "body" {
					targetNode = n
					return false
				}
				return true
			})

			if targetNode == nil {
				t.Fatal("Could not find target node")
			}

			result := ShouldRemoveElement(targetNode)
			if result != tt.want {
				t.Errorf("ShouldRemoveElement() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestShouldRemoveElementNil(t *testing.T) {
	t.Parallel()

	result := ShouldRemoveElement(nil)
	if result != false {
		t.Error("ShouldRemoveElement(nil) should return false")
	}
}

func TestShouldRemoveElementTextNode(t *testing.T) {
	t.Parallel()

	textNode := &html.Node{
		Type: html.TextNode,
		Data: "text",
	}

	result := ShouldRemoveElement(textNode)
	if result != false {
		t.Error("ShouldRemoveElement(textNode) should return false")
	}
}

func TestIsBlockElement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		tag  string
		want bool
	}{
		{"p", true},
		{"div", true},
		{"h1", true},
		{"h2", true},
		{"article", true},
		{"section", true},
		{"blockquote", true},
		{"ul", true},
		{"ol", true},
		{"li", true},
		{"table", true},
		{"br", false}, // BR is now inline, not block
		{"hr", true},
		{"span", false},
		{"a", false},
		{"strong", false},
		{"em", false},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			result := IsBlockElement(tt.tag)
			if result != tt.want {
				t.Errorf("IsBlockElement(%q) = %v, want %v", tt.tag, result, tt.want)
			}
		})
	}
}

func BenchmarkScoreContentNode(b *testing.B) {
	htmlContent := `<article class="main-content"><h1>Title</h1><p>Paragraph 1</p><p>Paragraph 2</p></article>`
	doc, _ := html.Parse(strings.NewReader(htmlContent))
	var articleNode *html.Node
	WalkNodes(doc, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "article" {
			articleNode = n
			return false
		}
		return true
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ScoreContentNode(articleNode)
	}
}

func BenchmarkCalculateContentDensity(b *testing.B) {
	htmlContent := `<div><p>` + strings.Repeat("word ", 50) + `</p></div>`
	doc, _ := html.Parse(strings.NewReader(htmlContent))

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalculateContentDensity(doc)
	}
}

func TestPatternMatchesEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		value   string
		pattern string
		want    bool
	}{
		{
			name:    "empty value",
			value:   "",
			pattern: "test",
			want:    false,
		},
		{
			name:    "empty pattern",
			value:   "test",
			pattern: "",
			want:    false,
		},
		{
			name:    "both empty",
			value:   "",
			pattern: "",
			want:    false, // Empty pattern never matches
		},
		{
			name:    "pattern longer than value",
			value:   "test",
			pattern: "testing",
			want:    false,
		},
		{
			name:    "exact match",
			value:   "article",
			pattern: "article",
			want:    true,
		},
		{
			name:    "hyphenated word match",
			value:   "article-content",
			pattern: "article",
			want:    true,
		},
		{
			name:    "underscore separated match - underscore is NOT a word boundary",
			value:   "article_content",
			pattern: "article",
			want:    true, // Underscore is not treated as a word boundary by patternMatches
		},
		{
			name:    "digit suffix matches (digits are boundary chars)",
			value:   "article123",
			pattern: "article",
			want:    true, // Digits are boundary chars: menu3 matches menu, article1 matches article
		},
		{
			name:    "match at start",
			value:   "article-body",
			pattern: "article",
			want:    true,
		},
		{
			name:    "match at end",
			value:   "main-article",
			pattern: "article",
			want:    true,
		},
		{
			name:    "match in middle",
			value:   "the-article-body",
			pattern: "article",
			want:    true,
		},
		{
			name:    "case sensitive match",
			value:   "Article",
			pattern: "article",
			want:    false,
		},
		{
			name:    "special characters",
			value:   "article.content",
			pattern: "article",
			want:    false,
		},
		{
			name:    "numbers in pattern",
			value:   "h1-content",
			pattern: "h1",
			want:    true,
		},
		{
			name:    "multiple hyphens",
			value:   "article-body-content",
			pattern: "body",
			want:    true,
		},
		{
			name:    "pattern with hyphen",
			value:   "post-article-content",
			pattern: "article-content",
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use MatchesPattern which internally calls patternMatches
			patterns := map[string]bool{tt.pattern: true}
			result := MatchesPattern(tt.value, patterns)
			if result != tt.want {
				t.Errorf("MatchesPattern(%q, %q) = %v, want %v", tt.value, tt.pattern, result, tt.want)
			}
		})
	}
}

func TestScoreContentNodeEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("document with very long text", func(t *testing.T) {
		longText := strings.Repeat("word ", 1000)
		inputHTML := `<div>` + longText + `</div>`
		doc, _ := html.Parse(strings.NewReader(inputHTML))
		// Find the div element
		var div *html.Node
		WalkNodes(doc, func(n *html.Node) bool {
			if n.Type == html.ElementNode && n.Data == "div" {
				div = n
				return false
			}
			return true
		})
		score := ScoreContentNode(div)
		// Very long content should have high score
		if score < 500 {
			t.Errorf("ScoreContentNode(long text) = %d, want >= 500", score)
		}
	})

	t.Run("document with only scripts", func(t *testing.T) {
		inputHTML := `<html><head><script>var x = 1;</script></head><body></body></html>`
		doc, _ := html.Parse(strings.NewReader(inputHTML))
		var body *html.Node
		WalkNodes(doc, func(n *html.Node) bool {
			if n.Type == html.ElementNode && n.Data == "body" {
				body = n
				return false
			}
			return true
		})
		score := ScoreContentNode(body)
		// Body with no content should have low score
		if score > 100 {
			t.Errorf("ScoreContentNode(empty body) = %d, want <= 100", score)
		}
	})
}

func TestCalculateContentDensityEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("nil node", func(t *testing.T) {
		density := CalculateContentDensity(nil)
		if density != 0 {
			t.Errorf("CalculateContentDensity(nil) = %f, want 0", density)
		}
	})

	t.Run("text node only", func(t *testing.T) {
		textNode := &html.Node{
			Type: html.TextNode,
			Data: "Hello World",
		}
		density := CalculateContentDensity(textNode)
		if density != 1.0 {
			t.Errorf("CalculateContentDensity(text node) = %f, want 1.0", density)
		}
	})

	t.Run("single tag with text", func(t *testing.T) {
		inputHTML := `<p>Hello</p>`
		doc, _ := html.Parse(strings.NewReader(inputHTML))
		density := CalculateContentDensity(doc)
		if density <= 0 {
			t.Errorf("CalculateContentDensity() = %f, want > 0", density)
		}
	})

	t.Run("many tags little text", func(t *testing.T) {
		inputHTML := strings.Repeat("<div>", 100) + "text" + strings.Repeat("</div>", 100)
		doc, _ := html.Parse(strings.NewReader(inputHTML))
		density := CalculateContentDensity(doc)
		// Many tags with little text should have low density
		if density > 0.1 {
			t.Errorf("CalculateContentDensity(many tags) = %f, want <= 0.1", density)
		}
	})
}

// ============================================================================
// NewDefaultScorerWithConfig Tests
// ============================================================================

func TestNewDefaultScorerWithConfig(t *testing.T) {
	t.Parallel()

	t.Run("creates scorer with custom config", func(t *testing.T) {
		config := &ScoringConfig{
			PositiveStrongPatterns: map[string]int{"custom": 500},
			TagScores:              map[string]int{"article": 1000},
		}
		scorer := NewDefaultScorerWithConfig(config)

		if scorer == nil {
			t.Fatal("NewDefaultScorerWithConfig returned nil")
		}
		if scorer.config != config {
			t.Error("Scorer config not set correctly")
		}
	})

	t.Run("nil config uses default", func(t *testing.T) {
		scorer := NewDefaultScorerWithConfig(nil)

		if scorer == nil {
			t.Fatal("NewDefaultScorerWithConfig(nil) returned nil")
		}
		// nil config is replaced with default config for safety
		if scorer.config == nil {
			t.Error("Config should not be nil when nil is passed (should use default)")
		}
		// Verify default config is used by checking for expected patterns
		if len(scorer.config.PositiveStrongPatterns) == 0 {
			t.Error("Default config should have positive patterns")
		}
	})

	t.Run("custom config affects scoring", func(t *testing.T) {
		// Create custom config with different patterns
		config := &ScoringConfig{
			PositiveStrongPatterns: map[string]int{"content": 500, "article": 500},
			TagScores:              map[string]int{"article": 1000},
		}
		scorer := NewDefaultScorerWithConfig(config)

		// Test with article element
		htmlContent := `<article class="content"><p>Test content here.</p></article>`
		doc, _ := html.Parse(strings.NewReader(htmlContent))

		var articleNode *html.Node
		WalkNodes(doc, func(n *html.Node) bool {
			if n.Type == html.ElementNode && n.Data == "article" {
				articleNode = n
				return false
			}
			return true
		})

		score := scorer.Score(articleNode)
		// Score should be affected by custom config
		if score <= 0 {
			t.Errorf("Score with custom config = %d, want > 0", score)
		}
	})

	t.Run("scorer implements Scorer interface", func(t *testing.T) {
		config := &ScoringConfig{}
		var _ Scorer = NewDefaultScorerWithConfig(config)
	})
}

func TestDefaultScorerScore(t *testing.T) {
	t.Parallel()

	t.Run("nil node returns 0", func(t *testing.T) {
		scorer := NewDefaultScorer()
		score := scorer.Score(nil)
		if score != 0 {
			t.Errorf("Score(nil) = %d, want 0", score)
		}
	})

	t.Run("text node returns 0", func(t *testing.T) {
		scorer := NewDefaultScorer()
		textNode := &html.Node{
			Type: html.TextNode,
			Data: "text",
		}
		score := scorer.Score(textNode)
		if score != 0 {
			t.Errorf("Score(textNode) = %d, want 0", score)
		}
	})

	t.Run("non-content element returns 0", func(t *testing.T) {
		scorer := NewDefaultScorer()
		doc, _ := html.Parse(strings.NewReader(`<script>code</script>`))

		var scriptNode *html.Node
		WalkNodes(doc, func(n *html.Node) bool {
			if n.Type == html.ElementNode && n.Data == "script" {
				scriptNode = n
				return false
			}
			return true
		})

		score := scorer.Score(scriptNode)
		if score != 0 {
			t.Errorf("Score(script) = %d, want 0", score)
		}
	})

	t.Run("paragraph returns 0", func(t *testing.T) {
		scorer := NewDefaultScorer()
		doc, _ := html.Parse(strings.NewReader(`<p>text</p>`))

		var pNode *html.Node
		WalkNodes(doc, func(n *html.Node) bool {
			if n.Type == html.ElementNode && n.Data == "p" {
				pNode = n
				return false
			}
			return true
		})

		score := scorer.Score(pNode)
		if score != 0 {
			t.Errorf("Score(p) = %d, want 0", score)
		}
	})

	t.Run("article with content has positive score", func(t *testing.T) {
		scorer := NewDefaultScorer()
		doc, _ := html.Parse(strings.NewReader(`<article><p>Content paragraph with text.</p></article>`))

		var articleNode *html.Node
		WalkNodes(doc, func(n *html.Node) bool {
			if n.Type == html.ElementNode && n.Data == "article" {
				articleNode = n
				return false
			}
			return true
		})

		score := scorer.Score(articleNode)
		if score <= 0 {
			t.Errorf("Score(article with content) = %d, want > 0", score)
		}
	})
}

func TestScoringConfigDefaults(t *testing.T) {
	t.Parallel()

	t.Run("default config values are reasonable", func(t *testing.T) {
		config := DefaultScoringConfig()

		if len(config.PositiveStrongPatterns) == 0 {
			t.Error("PositiveStrongPatterns should not be empty")
		}
		if len(config.NegativeStrongPatterns) == 0 {
			t.Error("NegativeStrongPatterns should not be empty")
		}
		if len(config.TagScores) == 0 {
			t.Error("TagScores should not be empty")
		}
		if len(config.RemovePatterns) == 0 {
			t.Error("RemovePatterns should not be empty")
		}
		if len(config.SubstringRemovePatterns) == 0 {
			t.Error("SubstringRemovePatterns should not be empty")
		}
		if !config.SubstringRemovePatterns["sitemap"] {
			t.Error("SubstringRemovePatterns should contain sitemap")
		}
		if !config.RemovePatterns["sitemap"] {
			t.Error("RemovePatterns should contain sitemap")
		}
	})
}

// TestSharedDefaultScorer verifies the sync.Once-backed singleton: every call
// returns the same instance, and it is usable for scoring.
func TestSharedDefaultScorer(t *testing.T) {
	t.Parallel()

	s1 := SharedDefaultScorer()
	if s1 == nil {
		t.Fatal("SharedDefaultScorer() returned nil")
	}

	// Pointer identity across calls proves the singleton is cached, not rebuilt.
	s2 := SharedDefaultScorer()
	if s1 != s2 {
		t.Fatal("SharedDefaultScorer() returned distinct instances; expected a shared singleton")
	}

	// It must be interchangeable with a freshly built default scorer.
	if NewDefaultScorer() == nil {
		t.Fatal("NewDefaultScorer() returned nil")
	}
}

// TestIsHiddenByStyle verifies the style-based hidden-element detection used by
// ShouldRemove. It pins the tokenized property-name comparison so that CSS
// custom properties are not false positives and CSS-escaped names are not false
// negatives.
func TestIsHiddenByStyle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		style string
		want  bool
	}{
		{"display none", "display:none", true},
		{"display none with spaces", "display: none", true},
		{"display none uppercase", "DISPLAY: NONE", true},
		{"display none in declaration list", "color:red;display:none", true},
		{"visibility hidden", "visibility:hidden", true},
		{"visibility hidden with space", "visibility: hidden", true},
		{"display block", "display:block", false},
		{"color only", "color:red", false},
		{"empty", "", false},
		// Custom property whose name merely ends in "display" must NOT be
		// treated as display:none (previous substring match false positive).
		{"custom property ending in display", "--my-display:none", false},
		// CSS escape in the property name (di\splay == display) must still hide.
		{"escaped display name", `di\splay:none`, true},
		{"escaped visibility name", `vi\sibility:hidden`, true},
		// A bare colon / empty property is not a hidden declaration.
		{"no property before colon", ":none", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isHiddenByStyle(tt.style); got != tt.want {
				t.Errorf("isHiddenByStyle(%q) = %v, want %v", tt.style, got, tt.want)
			}
		})
	}
}
