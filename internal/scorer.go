// scorer.go contains the Scorer interface and default implementation for content scoring.
package internal

import (
	"strings"
	"sync"

	"golang.org/x/net/html"
)

// Scorer defines the interface for content scoring algorithms.
// Implementations can provide custom scoring logic for content extraction.
type Scorer interface {
	// Score calculates a relevance score for a content node.
	// Higher scores indicate more likely main content.
	Score(node *html.Node) int
	// ShouldRemove determines if a node should be removed from the content tree.
	ShouldRemove(node *html.Node) bool
}

// ScoringConfig holds the configuration for the default scorer.
type ScoringConfig struct {
	// PositiveStrongPatterns maps pattern strings to their strong positive scores.
	PositiveStrongPatterns map[string]int
	// PositiveMediumPatterns maps pattern strings to their medium positive scores.
	PositiveMediumPatterns map[string]int
	// NegativeStrongPatterns maps pattern strings to their strong negative scores.
	NegativeStrongPatterns map[string]int
	// NegativeMediumPatterns maps pattern strings to their medium negative scores.
	NegativeMediumPatterns map[string]int
	// NegativeWeakPatterns maps pattern strings to their weak negative scores.
	NegativeWeakPatterns map[string]int
	// RemovePatterns maps pattern strings to a boolean indicating removal.
	RemovePatterns map[string]bool
	// SubstringRemovePatterns maps patterns matched as plain substrings (no word
	// boundary) for removal. Reserve this for unambiguous navigation markers
	// whose real-world class/id forms defeat word-boundary matching — e.g.
	// "sitemap" appears in ids like "divSiteMap" and "sitemap2", where the
	// surrounding letters/digits prevent RemovePatterns from matching. Keep this
	// set small and high-confidence: substring matching is broader than
	// RemovePatterns, so a carelessly chosen token (e.g. "ad", "nav") would
	// cause widespread false positives.
	SubstringRemovePatterns map[string]bool
	// TagScores maps tag names to their base scores.
	TagScores map[string]int
}

// DefaultScoringConfig returns the default scoring configuration.
func DefaultScoringConfig() *ScoringConfig {
	return &ScoringConfig{
		PositiveStrongPatterns: map[string]int{
			"content": strongPositiveScore, "article": strongPositiveScore, "main": strongPositiveScore,
			"post": strongPositiveScore, "entry": strongPositiveScore, "text": strongPositiveScore,
			"body": strongPositiveScore, "story": strongPositiveScore,
		},
		PositiveMediumPatterns: map[string]int{
			"blog": mediumPositiveScore, "news": mediumPositiveScore,
			"detail": mediumPositiveScore, "page": mediumPositiveScore,
		},
		NegativeStrongPatterns: map[string]int{
			"comment": strongNegativeScore, "sidebar": strongNegativeScore, "nav": strongNegativeScore,
			"navigation": strongNegativeScore, "footer": strongNegativeScore, "header": strongNegativeScore,
			"menu": strongNegativeScore, "ad": strongNegativeScore, "advertisement": strongNegativeScore,
		},
		NegativeMediumPatterns: map[string]int{
			"widget": mediumNegativeScore, "related": mediumNegativeScore, "share": mediumNegativeScore,
			"social": mediumNegativeScore, "meta": mediumNegativeScore, "tag": mediumNegativeScore,
			"category": mediumNegativeScore,
		},
		NegativeWeakPatterns: map[string]int{
			"promo": weakNegativeScore, "banner": weakNegativeScore, "sponsor": weakNegativeScore,
		},
		RemovePatterns: map[string]bool{
			"nav": true, "navigation": true, "menu": true,
			"sidebar": true, "side-bar": true,
			"footer": true, "header": true,
			"comment": true, "comments": true,
			"ad": true, "ads": true, "advertisement": true,
			"social": true, "share": true, "sharing": true,
			"related": true, "recommend": true,
			"widget": true, "plugin": true,
			"promo": true, "promotion": true,
			"banner": true, "sponsor": true,
			// Sitemap/site-map/site_map match standard delimited class/id values.
			// Prefixed/suffixed forms (divSiteMap, sitemap2) are caught by
			// SubstringRemovePatterns below, since word boundaries fail there.
			"sitemap": true, "site-map": true, "site_map": true,
		},
		// Substring matches for navigation markers that reliably indicate
		// non-primary content regardless of surrounding characters. "sitemap"
		// blocks are large link directories (e.g. 9k+ chars, 100+ links) that
		// are never main article content.
		SubstringRemovePatterns: map[string]bool{
			"sitemap": true,
		},
		TagScores: map[string]int{
			"article": 1000,
			"main":    900,
			"section": 300,
			"body":    100,
			"div":     50,
			// "p" is intentionally absent: Score() short-circuits to 0 for <p>
			// via the node.Data == "p" early return, so a base score here would
			// be unreachable. getTagScore also defaults unknown tags to 0.
		},
	}
}

// DefaultScorer is the default implementation of the Scorer interface.
type DefaultScorer struct {
	config          *ScoringConfig
	patternPrefixes map[byte][]patternScore // Pre-computed prefix index for fast pattern matching
}

// patternScore holds a pattern and its score for prefix-based filtering.
type patternScore struct {
	pattern string
	score   int
}

// NewDefaultScorer creates a new DefaultScorer with the default configuration.
// For repeated use, prefer SharedDefaultScorer() to avoid repeated allocation.
func NewDefaultScorer() *DefaultScorer {
	config := DefaultScoringConfig()
	return &DefaultScorer{
		config:          config,
		patternPrefixes: buildPatternPrefixIndex(config),
	}
}

// SharedDefaultScorer returns a shared singleton DefaultScorer.
// Use this instead of NewDefaultScorer() when the default configuration is acceptable
// to avoid repeated allocation of scoring maps and pattern indexes.
// The returned scorer is read-only and safe for concurrent use.
func SharedDefaultScorer() *DefaultScorer {
	return getDefaultScorer()
}

// buildPatternPrefixIndex creates a prefix-based index for fast pattern matching.
// Patterns are grouped by their first character to enable early filtering.
func buildPatternPrefixIndex(config *ScoringConfig) map[byte][]patternScore {
	// Estimate capacity: most patterns start with unique characters
	index := make(map[byte][]patternScore)

	// Add all pattern categories to the index
	addPatternsToIndex(index, config.PositiveStrongPatterns)
	addPatternsToIndex(index, config.PositiveMediumPatterns)
	addPatternsToIndex(index, config.NegativeStrongPatterns)
	addPatternsToIndex(index, config.NegativeMediumPatterns)
	addPatternsToIndex(index, config.NegativeWeakPatterns)

	return index
}

// addPatternsToIndex adds patterns to the prefix index grouped by their first character.
func addPatternsToIndex(index map[byte][]patternScore, patterns map[string]int) {
	for pattern, score := range patterns {
		if len(pattern) == 0 {
			continue
		}
		firstChar := pattern[0]
		// Convert to lowercase for case-insensitive matching
		if firstChar >= 'A' && firstChar <= 'Z' {
			firstChar += 32
		}
		index[firstChar] = append(index[firstChar], patternScore{
			pattern: pattern,
			score:   score,
		})
	}
}

// NewDefaultScorerWithConfig creates a new DefaultScorer with custom configuration.
// If config is nil, the default configuration is used.
func NewDefaultScorerWithConfig(config *ScoringConfig) *DefaultScorer {
	if config == nil {
		config = DefaultScoringConfig()
	}
	return &DefaultScorer{
		config:          config,
		patternPrefixes: buildPatternPrefixIndex(config),
	}
}

// Score calculates a relevance score for a content node.
func (s *DefaultScorer) Score(node *html.Node) int {
	if node == nil || node.Type != html.ElementNode || IsNonContentElement(node.Data) || node.Data == "p" {
		return 0
	}
	// Collect all metrics in a single traversal
	return s.scoreWithMetrics(node, collectContentMetrics(node))
}

// scoreWithMetrics applies the DefaultScorer scoring rules to node using
// precomputed subtree metrics. It holds the full post-guard body that Score used
// to inline; the guards are re-checked so it is safe to call directly.
// ScoreArticleCandidates calls this with metrics folded by a single bottom-up
// pass (foldAndScore), avoiding the O(N²) per-candidate subtree re-walk that
// collectContentMetrics would otherwise repeat for every candidate.
func (s *DefaultScorer) scoreWithMetrics(node *html.Node, metrics contentMetrics) int {
	if node == nil || node.Type != html.ElementNode || IsNonContentElement(node.Data) || node.Data == "p" {
		return 0
	}

	// Nodes that CleanContentNode would remove (class/id matching nav, menu,
	// sidebar, ad, etc., or hidden via style/attribute) must never be selected
	// as the article root. Without this guard, a text-rich mega-menu navigation
	// container can outscore the real article body — but when CleanContentNode
	// then strips the nav children (which also carry nav/menu classes), the
	// extraction yields empty output. Returning 0 here excludes such nodes
	// from the candidate set so a non-removable ancestor or sibling is chosen
	// instead. ShouldRemove also honors the contentAreaSignal and
	// isPrimaryContentContainer overrides, so layout wrappers like
	// content-sidebar and semantic <article>/<main> are still eligible.
	if s.ShouldRemove(node) {
		return 0
	}

	score := s.getTagScore(node.Data) + s.scoreAttributes(node)

	// Score based on paragraph count
	if metrics.paragraphCount >= minParagraphsForBonus {
		score += metrics.paragraphCount * manyParagraphsMultiplier
	} else if metrics.paragraphCount > 0 {
		score += metrics.paragraphCount * fewParagraphsMultiplier
	}

	// Score based on heading count
	if metrics.headingCount > 0 {
		score += metrics.headingCount * headingMultiplier
	}

	// Score based on text length
	textLength := metrics.textLength
	switch {
	case textLength > veryLongTextThreshold:
		score += veryLongTextThreshold + (textLength-veryLongTextThreshold)/veryLongTextBonusMultiplier
	case textLength > longTextThreshold:
		score += textLength / longTextBonusDivider
	case textLength > mediumTextThreshold:
		score += textLength / mediumTextBonusDivider
	case textLength < shortTextThreshold:
		score += shortTextPenalty
	}

	// Apply content density multiplier
	contentDensity := calculateDensityFromMetrics(metrics)
	if contentDensity > highContentDensityThreshold {
		score = int(float64(score) * highDensityMultiplier)
	} else if contentDensity < lowContentDensityThreshold {
		score = int(float64(score) * lowDensityMultiplier)
	}

	// Penalize high link density (likely navigation/menu/sitemap). The penalty
	// is gated on absolute text length: nav bars and tag clouds are link-dense
	// AND short, whereas main content that legitimately wraps prose or cards
	// in <a> (landing pages, portfolio grids) is link-dense but substantial.
	// Without this gate, a card-layout container is crushed by
	// highLinkDensityPenalty and the article extractor picks a tiny sibling
	// (e.g. a hero block) instead, discarding most of the page.
	linkDensity := calculateLinkDensityFromMetrics(metrics)
	if metrics.textLength < linkDensityPenaltyTextThreshold {
		if linkDensity > highLinkDensityThreshold {
			score = int(float64(score) * highLinkDensityPenalty)
		} else if linkDensity > mediumLinkDensityThreshold {
			score = int(float64(score) * mediumLinkDensityPenalty)
		} else if linkDensity > lowLinkDensityThreshold {
			score = int(float64(score) * lowLinkDensityPenalty)
		}
	}

	// Bonus for comma-rich content (likely prose)
	if metrics.commaCount > commaBonusThreshold {
		score += metrics.commaCount * commaBonusMultiplier
	}

	return score
}

// candidateCollector receives scored candidates from foldAndScore. When the map
// field is non-nil, every scored node is recorded into it (the ScoreArticleCandidates
// path). When it is nil, only the single highest-scoring node is tracked (the
// FindBestArticleNode path), eliminating the map allocation entirely — a
// make(map[*html.Node]int, 32) was the sole allocation on the scoring hot path,
// attributed ~2.2% of total bytes by the profiler.
type candidateCollector struct {
	candidates map[*html.Node]int
	bestNode   *html.Node
	bestScore  int
}

// record stores (or tracks) a scored candidate. Ties in the best-node path
// prefer the existing node (first-wins), matching SelectBestCandidate's behavior
// when the map iteration encounters the equal-scoring node first.
func (c *candidateCollector) record(node *html.Node, score int) {
	if c.candidates != nil {
		c.candidates[node] = score
		return
	}
	if score > c.bestScore {
		c.bestNode = node
		c.bestScore = score
	}
}

// ScoreArticleCandidates scores every plausible article-root element under root
// and returns the candidate→score map (score > 0 only). It is the O(N)
// replacement for the previous pattern — used by extractArticleNode — of walking
// the tree and calling Score(n) per non-inline element, where each Score re-walked
// n's entire subtree via collectContentMetrics.
//
// Instead the traversal folds each element's subtree metrics once (post-order) and
// scores the element immediately from those metrics, so no subtree is walked more
// than once and no per-node metrics need to be stored — only the returned candidate
// map is allocated (the same map the previous code built).
//
// Selection behavior is identical to scoring each non-inline element with Score:
// scoreWithMetrics applies the same guards (IsNonContentElement, "p") and rules,
// and the folded metrics match collectContentMetrics(n). Equivalence is locked by
// TestScoreArticleCandidatesMatchesNaiveLoop.
func (s *DefaultScorer) ScoreArticleCandidates(root *html.Node) map[*html.Node]int {
	candidates := make(map[*html.Node]int, 32)
	s.foldAndScore(root, false, &candidateCollector{candidates: candidates})
	return candidates
}

// FindBestArticleNode scores every plausible article-root element under root and
// returns the one with the highest score (or nil if none scores above 0). It
// folds best-candidate tracking directly into the bottom-up scoring pass, so the
// map allocation that ScoreArticleCandidates requires is never made — the caller
// (extractArticleNode) only needs the winning node, never the full candidate set.
func (s *DefaultScorer) FindBestArticleNode(root *html.Node) *html.Node {
	cc := &candidateCollector{bestScore: -1}
	s.foldAndScore(root, false, cc)
	return cc.bestNode
}

// foldAndScore computes node's subtree metrics bottom-up and records a score for
// each non-inline element candidate using its just-folded metrics, then returns
// node's subtree metrics so the parent can fold them in. insideLink indicates node
// sits within an <a> ancestor, so its text counts toward linkTextLength.
//
// The pass always descends into every subtree so each element is scored against the
// metrics collectContentMetrics would produce for it (which starts fresh at that
// node, regardless of whether an ancestor is a skip tag). Contributions are folded
// into a parent only when the parent is not itself a skip tag, so a skip tag's own
// metrics are zero — matching collectContentMetrics returning false before
// incrementing — while its non-skip descendants are still scored correctly.
func (s *DefaultScorer) foldAndScore(node *html.Node, insideLink bool, cc *candidateCollector) contentMetrics {
	if node == nil {
		return contentMetrics{}
	}

	isSkip := node.Type == html.ElementNode && (IsNonContentElement(node.Data) || metricsSkipTags[node.Data] || s.ShouldRemove(node))

	var m contentMetrics
	if node.Type == html.ElementNode && !isSkip {
		m.tagCount++
		switch node.Data {
		case "p":
			m.paragraphCount++
		case "h1", "h2", "h3", "h4", "h5", "h6":
			m.headingCount++
		}
		if node.Data == "a" {
			insideLink = true
		}
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		cm := s.foldAndScore(child, insideLink, cc)
		if !isSkip {
			m.tagCount += cm.tagCount
			m.paragraphCount += cm.paragraphCount
			m.headingCount += cm.headingCount
			m.textLength += cm.textLength
			m.totalTextLength += cm.totalTextLength
			m.linkTextLength += cm.linkTextLength
			m.commaCount += cm.commaCount
		}
	}

	if node.Type == html.TextNode {
		// Mirrors collectContentMetrics' text-node handling verbatim so the two
		// paths cannot diverge (guarded by TestScoreArticleCandidatesMatchesNaiveLoop).
		data := node.Data
		dataLen := len(data)
		hasNBSP := false
		for i := 0; i+1 < dataLen; i++ {
			if data[i] == 0xC2 && data[i+1] == 0xA0 {
				hasNBSP = true
				break
			}
		}
		if hasNBSP {
			data = strings.ReplaceAll(data, " ", " ")
		}
		if text := strings.TrimSpace(data); text != "" {
			textLen := len(text)
			m.textLength += textLen
			m.totalTextLength += textLen
			m.commaCount += strings.Count(text, ",") + strings.Count(text, "，")
			if insideLink {
				m.linkTextLength += textLen
			}
		}
	}

	// m now holds node's complete subtree metrics; score it if it is a candidate.
	if node.Type == html.ElementNode && !isSkip && !IsInlineElement(node.Data) {
		if score := s.scoreWithMetrics(node, m); score > 0 {
			cc.record(node, score)
		}
	}

	return m
}

// contentAreaSignals lists class/id tokens that, when present alongside a
// removal pattern in the same attribute value, indicate a CSS layout wrapper
// (e.g. foolcom-grid-content-sidebar, article-with-sidebar) rather than the
// non-content region itself. Only patterns that unambiguously denote a
// primary-content area are included; patterns such as "main", "post", and
// "text" are excluded because they also appear in non-content class names
// (main-nav, post-nav, nav-text) where an override would be harmful.
var contentAreaSignals = map[string]bool{
	"content": true,
	"article": true,
}

// hasContentAreaSignal reports whether value (a lowercased class or id
// attribute) contains any contentAreaSignals pattern as a word-bounded match.
// ShouldRemove uses this to exempt layout wrappers from the class/id-based
// removal heuristic so the main content nested inside them is not discarded.
func hasContentAreaSignal(value string) bool {
	for pattern := range contentAreaSignals {
		if hasWordBoundary(value, pattern, boundaryStandard) {
			return true
		}
	}
	return false
}

// ShouldRemove determines if a node should be removed from the content tree.
func (s *DefaultScorer) ShouldRemove(node *html.Node) bool {
	if node == nil || node.Type != html.ElementNode {
		return false
	}

	if s == nil || s.config == nil {
		return false
	}

	if IsNonContentElement(node.Data) {
		return true
	}

	// Semantic main-content containers (<article>, <main>, or an explicit
	// ARIA role of main/article) unambiguously denote primary content. They
	// must NOT be stripped by the class/id substring heuristic: a class such
	// as "post-with-sidebar" denotes an article rendered in a sidebar layout,
	// not a sidebar itself, yet the "-" delimiter made it match the "sidebar"
	// removal pattern and discard the entire article body. Hidden/style
	// signals below are still honored because they are unambiguous.
	primaryContent := isPrimaryContentContainer(node)

	for _, attr := range node.Attr {
		switch attr.Key {
		case "class", "id":
			if primaryContent {
				continue
			}
			lowerVal := strings.ToLower(attr.Val)
			// A non-semantic element (div, section, ...) whose class/id
			// contains both a content-area signal ("content", "article")
			// and a removal pattern ("sidebar", "nav", ...) describes a CSS
			// layout wrapper that encloses the main content AND a non-content
			// region — e.g. foolcom-grid-content-sidebar on The Motley Fool
			// wraps #article-body and a sidebar column. Removing it would
			// discard the article body along with the sidebar, yielding empty
			// output. Skip removal-pattern matching for this attribute so the
			// content subtree survives. The heuristic for truly non-content
			// elements still applies via their own class/id (e.g. a child
			// <div class="sidebar"> is still stripped).
			if hasContentAreaSignal(lowerVal) {
				continue
			}
			for pattern := range s.config.RemovePatterns {
				if hasWordBoundary(lowerVal, pattern, boundaryStandard) {
					return true
				}
			}
			for pattern := range s.config.SubstringRemovePatterns {
				if strings.Contains(lowerVal, pattern) {
					return true
				}
			}
		case "style":
			if isHiddenByStyle(attr.Val) {
				return true
			}
		case "hidden":
			return true
		}
	}
	return false
}

// isHiddenByStyle reports whether a style attribute hides its element
// (display:none or visibility:hidden). It scans the declaration list inline
// (without allocating a []string from strings.Split) and compares complete
// property names so that:
//   - CSS custom properties (--my-display:none) are not confused with display.
//     The previous substring test ("contains display:none") stripped any element
//     whose custom-property name merely ended in "display".
//   - CSS escape sequences in the property name (di\splay:none, the valid CSS
//     spelling of display:none) are decoded before comparison, so a hidden
//     element can no longer evade removal.
//
// Note: hex CSS escapes (\64...) in property names are not decoded; they are
// vanishingly rare in real style attributes and out of scope for this
// content-extraction heuristic.
func isHiddenByStyle(style string) bool {
	pos := 0
	n := len(style)
	for pos < n {
		// Find the next ';' or end of string to delimit a declaration.
		declStart := pos
		for pos < n && style[pos] != ';' {
			pos++
		}
		raw := style[declStart:pos]
		pos++ // skip ';'

		colon := strings.IndexByte(raw, ':')
		if colon <= 0 {
			continue // empty declaration or no property name before the colon
		}
		prop := decodeCSSEcapeInline(raw[:colon])
		val := asciiTrimSpaceLower(raw[colon+1:])
		switch {
		case prop == "display" && val == "none":
			return true
		case prop == "visibility" && val == "hidden":
			return true
		}
	}
	return false
}

// asciiTrimSpaceLower trims leading/trailing ASCII whitespace and lowercases
// ASCII letters in a single pass, returning a substring of s (no allocation)
// when possible. For values containing non-ASCII bytes it falls back to
// strings.ToLower + strings.TrimSpace.
func asciiTrimSpaceLower(s string) string {
	// Trim leading whitespace
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	// Trim trailing whitespace
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	if start >= end {
		return ""
	}
	// Check if already lowercase ASCII (common case — no allocation)
	for i := start; i < end; i++ {
		if s[i] >= 'A' && s[i] <= 'Z' {
			return strings.ToLower(s[start:end])
		}
	}
	return s[start:end]
}

// decodeCSSEcapeInline decodes CSS backslash escapes and lowercases ASCII in a
// single pass. For property names without backslashes or uppercase (the common
// case), it returns a substring of the input with no allocation.
func decodeCSSEcapeInline(s string) string {
	// Trim leading/trailing whitespace first
	start := 0
	for start < len(s) && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	end := len(s)
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	if start >= end {
		return ""
	}
	s = s[start:end]

	hasBackslash := false
	hasUpper := false
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' {
			hasBackslash = true
		}
		if s[i] >= 'A' && s[i] <= 'Z' {
			hasUpper = true
		}
	}
	if !hasBackslash && !hasUpper {
		return s
	}
	return decodeCSSEscape(strings.ToLower(s))
}

// decodeCSSEscape decodes CSS backslash escapes in a style property name: a
// backslash followed by a single character yields that character (covering
// escapes like \s in di\splay → display). This prevents hiding a property name
// behind escapes. It does not decode hex escapes (see isHiddenByStyle note).
func decodeCSSEscape(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		c := s[i]
		if c != '\\' || i+1 >= len(s) {
			b.WriteByte(c)
			i++
			continue
		}
		// backslash + next literal char
		b.WriteByte(s[i+1])
		i += 2
	}
	return b.String()
}

// isPrimaryContentContainer reports whether the node unambiguously represents
// primary/main content via its semantic tag or ARIA role. Such nodes are
// excluded from the class/id-based removal heuristic to prevent false
// positives such as <article class="post-with-sidebar"> matching the "sidebar"
// pattern. Note: this only governs the heuristic; a genuinely hidden node
// (hidden attribute or display:none) is still removed.
func isPrimaryContentContainer(node *html.Node) bool {
	switch node.Data {
	case "article", "main":
		return true
	}
	for _, attr := range node.Attr {
		if attr.Key == "role" {
			switch strings.ToLower(attr.Val) {
			case "main", "article":
				return true
			}
		}
	}
	return false
}

// getTagScore returns the base score for a tag name.
func (s *DefaultScorer) getTagScore(tag string) int {
	if s == nil || s.config == nil {
		return 0
	}
	if score, ok := s.config.TagScores[tag]; ok {
		return score
	}
	return 0
}

// ScoreAttributes calculates a score based on element attributes.
// This is the public version for external use.
func (s *DefaultScorer) ScoreAttributes(n *html.Node) int {
	return s.scoreAttributes(n)
}

// scoreAttributes calculates a score based on element attributes.
func (s *DefaultScorer) scoreAttributes(n *html.Node) int {
	if n == nil || s == nil || s.config == nil {
		return 0
	}

	score := 0
	for _, attr := range n.Attr {
		switch attr.Key {
		case "class", "id":
			lowerVal := strings.ToLower(attr.Val)
			score += s.calculatePatternScore(lowerVal, s.config.PositiveStrongPatterns)
			score += s.calculatePatternScore(lowerVal, s.config.PositiveMediumPatterns)
			score += s.calculatePatternScore(lowerVal, s.config.NegativeStrongPatterns)
			score += s.calculatePatternScore(lowerVal, s.config.NegativeMediumPatterns)
			score += s.calculatePatternScore(lowerVal, s.config.NegativeWeakPatterns)
		case "role":
			lowerVal := strings.ToLower(attr.Val)
			switch lowerVal {
			case "main", "article":
				score += 500
			case "navigation", "complementary":
				score -= 400
			}
		}
	}
	return score
}

// calculatePatternScore calculates score based on pattern matching.
// Optimized with sparse character tracking instead of fixed array iteration,
// and prefix filtering to only check patterns whose first character appears in value.
// Uses fixed-size array to avoid heap allocation.
func (s *DefaultScorer) calculatePatternScore(value string, patterns map[string]int) int {
	if len(value) == 0 || len(patterns) == 0 {
		return 0
	}

	score := 0

	// Use fixed-size arrays on stack to avoid heap allocation.
	// presentChars covers all possible first characters of scoring patterns:
	// 26 lowercase letters + 10 digits = 36 possible values.
	var valueChars [128]bool
	var presentChars [36]byte
	charCount := 0

	for i := 0; i < len(value); i++ {
		c := value[i]
		// Convert to lowercase for case-insensitive matching
		if c >= 'A' && c <= 'Z' {
			c += 32
		}
		// Only consider alphanumeric characters as potential pattern starts
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			if !valueChars[c] {
				valueChars[c] = true
				if charCount < 36 {
					presentChars[charCount] = c
					charCount++
				}
			}
		}
	}

	// Only check patterns whose first character appears in value
	// Iterate only through present characters, not all 128
	for i := 0; i < charCount; i++ {
		char := presentChars[i]
		if candidates, ok := s.patternPrefixes[char]; ok {
			for _, ps := range candidates {
				// Only check patterns that belong to the input patterns map
				if _, exists := patterns[ps.pattern]; exists {
					if hasWordBoundary(value, ps.pattern, boundaryStandard) {
						score += ps.score
					}
				}
			}
		}
	}

	return score
}

// getDefaultScorer returns a shared DefaultScorer instance, lazily initialized
// on first call via sync.OnceValue. This is an optimization for cases where
// multiple processors use the default scorer, reducing memory allocation by
// sharing a single read-only instance.
var getDefaultScorer = sync.OnceValue(func() *DefaultScorer {
	return NewDefaultScorer()
})
