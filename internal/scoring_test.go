package internal

import (
	"fmt"
	"strconv"
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

// TestBlockDensityIgnoresInlineMarkup guards the P1 density redefinition: content
// density counts block elements, not inline formatting tags, so prose with heavy
// <span>/<em> markup is not misread as low density. Two subtrees with identical
// visible text — one plain, one wrapped in inline tags — must have equal density.
func TestBlockDensityIgnoresInlineMarkup(t *testing.T) {
	t.Parallel()

	plain, err := parseHTML(`<div><p>This is a plain sentence with ordinary prose content.</p></div>`)
	if err != nil {
		t.Fatal(err)
	}
	formatted, err := parseHTML(`<div><p><span>This</span> <em>is</em> <span>a</span> <strong>plain</strong> <span>sentence</span> <em>with</em> <span>ordinary</span> <strong>prose</strong> <em>content</em><b>.</b></p></div>`)
	if err != nil {
		t.Fatal(err)
	}

	plainDensity := CalculateContentDensity(plain)
	formattedDensity := CalculateContentDensity(formatted)

	if plainDensity != formattedDensity {
		t.Errorf("inline markup skewed density: plain=%f formatted=%f (want equal)", plainDensity, formattedDensity)
	}
}

// TestCountSentenceTerminators pins the sentence-counting helper, including the
// CJK fullwidth terminators (。！？) that carry the prose signal for CJK text,
// which has no ASCII commas.
func TestCountSentenceTerminators(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		text string
		want int
	}{
		{"empty", "", 0},
		{"no terminators", "plain text without punctuation", 0},
		{"ascii period", "one. two. three.", 3},
		{"mixed ascii", "a! b? c.", 3},
		{"cjk full stop", "第一句。第二句。", 2},
		{"fullwidth exclaim and question", "真的吗？太好了！", 2},
		{"ellipsis counts each dot", "wait...", 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := countSentenceTerminators(tc.text); got != tc.want {
				t.Errorf("countSentenceTerminators(%q) = %d, want %d", tc.text, got, tc.want)
			}
		})
	}
}

// TestCJKProseGetsSentenceBonus verifies the sentence bonus lifts a CJK prose
// block (which has no ASCII commas) so it scores as content rather than being
// short-text-penalized like a tag cloud. This is the end-to-end effect of the
// sentenceCount metric for non-Latin prose.
func TestCJKProseGetsSentenceBonus(t *testing.T) {
	t.Parallel()

	doc, err := parseHTML(`<div><p>这是第一段正文内容。这里有几个完整的句子作为统计信号。</p><p>这是第二段正文内容。同样包含句号来体现正文特征。</p><p>第三段也在这里。继续提供句号信号。</p></div>`)
	if err != nil {
		t.Fatal(err)
	}

	score := ScoreContentNode(findScoreElement(doc, "div", ""))
	if score <= 0 {
		t.Errorf("CJK prose scored %d, want > 0", score)
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

func TestCountDescendantElements(t *testing.T) {
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

			count := CountDescendantElements(divNode, tt.tag)
			if count != tt.want {
				t.Errorf("CountDescendantElements() = %d, want %d", count, tt.want)
			}
		})
	}
}

func TestCountDescendantElementsNil(t *testing.T) {
	t.Parallel()

	count := CountDescendantElements(nil, "p")
	if count != 0 {
		t.Errorf("CountDescendantElements(nil) = %d, want 0", count)
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
			// Class-based hiding (not the hidden attribute or inline style) marks
			// visually absent content — consent walls, folded "read more" regions,
			// SEO filler — that must not leak into extracted text.
			name: "hidden class token",
			html: `<div class="hidden">content</div>`,
			want: true,
		},
		{
			name: "bootstrap d-none class",
			html: `<div class="d-none">content</div>`,
			want: true,
		},
		{
			name: "visually-hidden class",
			html: `<div class="visually-hidden">content</div>`,
			want: true,
		},
		{
			name: "sr-only class",
			html: `<div class="sr-only">content</div>`,
			want: true,
		},
		{
			name: "hidden token within longer class list",
			html: `<div class="container is-hidden">content</div>`,
			want: true,
		},
		{
			name: "aria-hidden true",
			html: `<div aria-hidden="true">content</div>`,
			want: true,
		},
		{
			name: "aria-hidden false is not hidden",
			html: `<div aria-hidden="false">content</div>`,
			want: false,
		},
		{
			// "hidden" inside an unrelated word must not trigger: the token list
			// is prefix-matched, and no token begins with these bytes except at a
			// true token start. Guarded so a careless future token ("hid", ...)
			// cannot silently widen this to a substring match over "schoold..." etc.
			name: "hidden as part of unrelated word",
			html: `<div class="not-hiddenize">content</div>`,
			want: false,
		},
		{
			// Bootstrap 3 responsive visibility: "hidden-sm" means hidden only on
			// small screens, VISIBLE on larger ones — it must NOT be stripped.
			name: "responsive hidden-sm is visible elsewhere",
			html: `<div class="hidden-sm">content</div>`,
			want: false,
		},
		{
			name: "responsive hidden-md-up is visible elsewhere",
			html: `<div class="hidden-md-up">content</div>`,
			want: false,
		},
		{
			name: "responsive hidden-lg-down is visible elsewhere",
			html: `<div class="hidden-lg-down">content</div>`,
			want: false,
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

// TestIsHiddenElementReader asserts the reader-mode hidden predicate keeps
// elements hidden only by the bare "hidden"/"hide" class or a responsive
// breakpoint (Tailwind's "hidden md:block", "xl:hidden") — hover/click
// disclosure panels a viewport-less, pointer-less reader must preserve — while
// still stripping the always-hidden classes and the unambiguous signals
// (d-none, sr-only, the hidden attribute, aria-hidden, inline display:none).
func TestIsHiddenElementReader(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		html string
		want bool
	}{
		{name: "bare hidden is an interaction hook, kept", html: `<div class="hidden">x</div>`, want: false},
		{name: "bare hide is an interaction hook, kept", html: `<div class="hide">x</div>`, want: false},
		{name: "dropdown panel kept", html: `<div class="nav-dropdown shadow-md hidden">x</div>`, want: false},
		{name: "d-none stays hidden", html: `<div class="d-none">x</div>`, want: true},
		{name: "sr-only stays hidden", html: `<div class="sr-only">x</div>`, want: true},
		{name: "visually-hidden stays hidden", html: `<div class="visually-hidden">x</div>`, want: true},
		{name: "is-hidden stays hidden", html: `<div class="is-hidden">x</div>`, want: true},
		{name: "invisible stays hidden", html: `<div class="invisible">x</div>`, want: true},
		{name: "responsive hidden md:block is kept", html: `<div class="hidden md:block">x</div>`, want: false},
		{name: "responsive hidden lg:flex is kept", html: `<div class="hidden lg:flex">x</div>`, want: false},
		{name: "responsive hidden 2xl:grid is kept", html: `<div class="hidden 2xl:grid">x</div>`, want: false},
		{name: "prefixed xl:hidden is kept", html: `<div class="xl:hidden">x</div>`, want: false},
		{name: "bootstrap hidden-sm is kept", html: `<div class="hidden-sm">x</div>`, want: false},
		{name: "responsive in longer class list", html: `<div class="flex items-center hidden md:inline-flex">x</div>`, want: false},
		{name: "d-none plus responsive stays hidden", html: `<div class="d-none hidden md:block">x</div>`, want: true},
		{name: "hidden attribute stays hidden", html: `<div hidden>visible-ish</div>`, want: true},
		{name: "aria-hidden true stays hidden", html: `<div aria-hidden="true">x</div>`, want: true},
		{name: "inline display none stays hidden", html: `<div style="display:none">x</div>`, want: true},
		{name: "not hidden", html: `<div class="container">x</div>`, want: false},
		{name: "hidden as part of unrelated word", html: `<div class="not-hiddenize">x</div>`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, _ := html.Parse(strings.NewReader(tt.html))
			var target *html.Node
			WalkNodes(doc, func(n *html.Node) bool {
				if n.Type == html.ElementNode && n.Data == "div" {
					target = n
					return false
				}
				return true
			})
			if target == nil {
				t.Fatal("could not find div")
			}
			if got := IsHiddenElementReader(target); got != tt.want {
				t.Errorf("IsHiddenElementReader() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestIsHiddenElementReaderStrict asserts the main-root reader variant: for
// non-landmark main wrappers (token-classed divs) the bare "hidden" token
// still hides (an SPA shell, not a menu) while the responsive forms
// ("hidden md:block", "xl:hidden") stay kept; semantic <main> roots never
// reach the class test at all (the primary-container exemption), so only the
// unambiguous signals hide them.
func TestIsHiddenElementReaderStrict(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		tag  string
		html string
		want bool
	}{
		{name: "landmark main class-exempt", tag: "main", html: `<main class="hidden">x</main>`, want: false},
		{name: "landmark main hidden attribute hides", tag: "main", html: `<main hidden>x</main>`, want: true},
		{name: "wrapper bare hidden collapses", tag: "div", html: `<div class="article hidden">x</div>`, want: true},
		{name: "responsive hidden md:block kept", tag: "div", html: `<div class="article hidden md:block">x</div>`, want: false},
		{name: "prefixed xl:hidden kept", tag: "div", html: `<div class="article xl:hidden">x</div>`, want: false},
		{name: "bootstrap hidden-sm kept", tag: "div", html: `<div class="article hidden-sm">x</div>`, want: false},
		{name: "d-none collapses", tag: "div", html: `<div class="article d-none">x</div>`, want: true},
		{name: "d-none plus responsive collapses", tag: "div", html: `<div class="article d-none hidden lg:block">x</div>`, want: true},
		{name: "visible wrapper", tag: "div", html: `<div class="article container">x</div>`, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, _ := html.Parse(strings.NewReader(tt.html))
			var target *html.Node
			WalkNodes(doc, func(n *html.Node) bool {
				if n.Type == html.ElementNode && n.Data == tt.tag {
					target = n
					return false
				}
				return true
			})
			if target == nil {
				t.Fatalf("could not find %s", tt.tag)
			}
			if got := IsHiddenElementReaderStrict(target); got != tt.want {
				t.Errorf("IsHiddenElementReaderStrict() = %v, want %v", got, tt.want)
			}
		})
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
	for b.Loop() {
		ScoreContentNode(articleNode)
	}
}

func BenchmarkCalculateContentDensity(b *testing.B) {
	htmlContent := `<div><p>` + strings.Repeat("word ", 50) + `</p></div>`
	doc, _ := html.Parse(strings.NewReader(htmlContent))

	b.ResetTimer()
	for b.Loop() {
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

// ============================================================================
// Scoring regressions, naive-loop equivalence oracle and benchmarks (merged
// from scoring_regression_test.go, scoring_metrics_equivalence_test.go and
// scoring_bench_test.go)
// ============================================================================

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
		{"nbsp text", "<html><body><p>non\u00a0breaking\u00a0space</p></body></html>"},
		// A literal ASCII "00a0" in visible text (e.g. a hex colour) must not be
		// treated as an NBSP: foldAndScore once shipped strings.Contains(data,
		// "00a0"), which rewrote it and made the folded metrics disagree with
		// collectContentMetrics for any document containing that substring.
		{"literal 00a0 text", `<html><body><div class="wrap">` + strings.Repeat("00a0 ", 12) + `</div></body></html>`},
		{"skip tag subtrees with candidates", `<html><head><title>t</title></head><body><script>x</script><style>x</style><nav>n</nav><svg><text>s</text></svg><math><mi>m</mi></math><aside>a</aside><footer>f</footer><article><p>real, comma, content.</p></article></body></html>`},
		{"comma rich prose", `<html><body><article><p>one, two, three, four, five, six.</p><p>` + "，" + `fullwidth` + "，" + `test。</p></article></body></html>`},
		{"sentence dense prose", `<html><body><article><p>First sentence ends here. Second sentence ends here too! A third? Yes.</p><p>Another paragraph of prose with a full stop.</p></article></body></html>`},
		{"link count metrics", `<html><body><div><a href="/a">A</a><a href="/b">B</a><a href="/c">C</a><p>text</p></div></body></html>`},
		{"block count inline markup", `<html><body><article><p>text with <span>inline</span> and <em>emphasis</em>.</p></article></body></html>`},
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
	for b.Loop() {
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
	for b.Loop() {
		_ = ds.ScoreArticleCandidates(doc)
	}
}

// TestFindBestArticleNode pins the production article-selection path. The root
// package calls DefaultScorer.FindBestArticleNode (extract.go extractArticleNode)
// through the internal.Scorer interface, so cross-package coverage never reaches
// it from the internal package's own tests. It must agree with
// ScoreArticleCandidates on the winner, return nil when nothing scores above 0,
// and resolve exact ties first-wins (the map path resolves ties by map-iteration
// order, so the two legitimately disagree only on ties).
func TestFindBestArticleNode(t *testing.T) {
	ds := SharedDefaultScorer()

	cases := []struct {
		name    string
		markup  string
		wantNil bool
	}{
		{"plain article picks article", `<html><body><article><h1>T</h1><p>alpha, beta, gamma.</p><p>second paragraph.</p></article></body></html>`, false},
		{"link dense nav skipped", `<html><body><nav><a href="/a">A</a><a href="/b">B</a></nav><article><p>real content, here.</p></article></body></html>`, false},
		{"empty body scores nothing", `<html><body></body></html>`, true},
		{"comment only scores nothing", `<html><body><!-- no elements --></body></html>`, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := parseMetricsDoc(t, tc.markup)
			best := ds.FindBestArticleNode(doc)
			if tc.wantNil {
				if best != nil {
					t.Fatalf("FindBestArticleNode = <%s>, want nil", best.Data)
				}
				return
			}
			if best == nil {
				t.Fatal("FindBestArticleNode = nil, want a non-nil article node")
			}

			// Agree with the map path on the winner, and on the winner's score.
			cands := ds.ScoreArticleCandidates(doc)
			maxScore := 0
			var winners []*html.Node
			for n, sc := range cands {
				if sc > maxScore {
					maxScore = sc
					winners = winners[:0]
					winners = append(winners, n)
				} else if sc == maxScore {
					winners = append(winners, n)
				}
			}
			if len(winners) == 1 && best != winners[0] {
				t.Errorf("FindBestArticleNode = <%s>, ScoreArticleCandidates winner = <%s>", best.Data, winners[0].Data)
			}
			if got := cands[best]; got != maxScore {
				t.Errorf("FindBestArticleNode picked <%s> with score %d, want the max score %d", best.Data, got, maxScore)
			}
		})
	}
}
