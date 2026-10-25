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
			// "widget" is deliberately absent: the token is ambiguous — it names
			// real ad/recirculation widgets, but also CMS content containers such
			// as Drupal/NIR "nir-widget" / "block--…__widget", which wrap the
			// primary content. Removing those stripped the entire page body (see
			// Enphase investor events page). "ad"/"advertisement"/"social" carry
			// the same signal for genuine widgets without the false positives.
			"related": mediumNegativeScore, "share": mediumNegativeScore,
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
			// "widget" is deliberately absent for the same reason as in
			// NegativeMediumPatterns above: CMS content blocks (nir-widget,
			// block--…__widget) are primary content, not removable chrome.
			"plugin": true,
			"promo":  true, "promotion": true,
			"banner": true, "sponsor": true,
			// Sitemap/site-map/site_map match standard delimited class/id values.
			// Prefixed/suffixed forms (divSiteMap, sitemap2) are caught by
			// SubstringRemovePatterns below, since word boundaries fail there.
			"sitemap": true, "site-map": true, "site_map": true,

			// Navigation trails and consent/subscription/promo chrome. Each
			// token is word-bounded and names page furniture that never holds
			// article prose; the riskier candidates ("premium", "skip") are
			// deliberately absent until eval evidence justifies them.
			"breadcrumb": true, "breadcrumbs": true,
			"cookie": true, "gdpr": true, "consent": true,
			"newsletter": true, "subscribe": true, "signup": true,
			"modal": true, "overlay": true, "popup": true, "toast": true,
			"trending": true, "popular": true, "recirc": true,
			"outbrain": true, "taboola": true, "disqus": true,
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
	config *ScoringConfig
	// patternIndex buckets every scoring pattern by its (case-folded) first
	// byte for fast pattern matching. An array rather than a map: the scorer
	// consults it per present character of every class/id attribute, and the
	// profiler showed the map lookups (hashing plus bucket search) as a major
	// cost on class-heavy pages.
	patternIndex [256][]patternScore
	// removeScanClass maps each byte to a dense bucket id (1+) for the
	// RemovePatterns buckets (built by buildRemoveWordIndex at construction),
	// or 0 when no removal pattern begins with that byte.
	// matchesRemovalPatterns consults this one-byte table per input byte
	// so the whole pattern set is evaluated in a single pass over the value —
	// the same idiom as mediaScanClass in media.go — instead of one
	// strings.Index scan per pattern plus a 256-bucket loop per attribute.
	// removeSubstrPatterns lists SubstringRemovePatterns flat. Both replace
	// per-attribute map iteration in ShouldRemove with deterministic,
	// prefix-filtered scans.
	removeScanClass      [256]uint8
	removeScanBuckets    [][]string
	removeSubstrPatterns []string
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
	return newDefaultScorerWithConfig(config)
}

// SharedDefaultScorer returns a shared singleton DefaultScorer.
// Use this instead of NewDefaultScorer() when the default configuration is acceptable
// to avoid repeated allocation of scoring maps and pattern indexes.
// The returned scorer is read-only and safe for concurrent use.
func SharedDefaultScorer() *DefaultScorer {
	return getDefaultScorer()
}

// buildPatternPrefixIndex creates a prefix-based index for fast pattern matching.
// Patterns are grouped by their (case-folded) first character to enable early
// filtering. The five scoring categories are merged into one index: a pattern
// appearing in several categories of a custom config would have been matched
// once per category by the old per-category passes, each adding its own score,
// so the merged entry carries the categories' pre-summed score and a single
// match adds the same total.
func buildPatternPrefixIndex(config *ScoringConfig) [256][]patternScore {
	// Sum scores across the categories first.
	sums := make(map[string]int, 64)
	for _, patterns := range []map[string]int{
		config.PositiveStrongPatterns,
		config.PositiveMediumPatterns,
		config.NegativeStrongPatterns,
		config.NegativeMediumPatterns,
		config.NegativeWeakPatterns,
	} {
		for pattern, score := range patterns {
			if len(pattern) > 0 {
				sums[pattern] += score
			}
		}
	}

	var index [256][]patternScore
	for pattern, score := range sums {
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
	return index
}

// buildRemoveWordIndex buckets RemovePatterns by their (case-folded) first
// character so ShouldRemove checks only the patterns that could possibly occur
// in the attribute value (a pattern whose first byte is absent from the value
// can never be found by hasWordBoundary).
func buildRemoveWordIndex(config *ScoringConfig) [256][]string {
	var index [256][]string
	for pattern := range config.RemovePatterns {
		if len(pattern) == 0 {
			continue
		}
		firstChar := pattern[0]
		if firstChar >= 'A' && firstChar <= 'Z' {
			firstChar += 32
		}
		index[firstChar] = append(index[firstChar], pattern)
	}
	return index
}

// buildRemoveSubstrPatterns flattens SubstringRemovePatterns into a slice for
// deterministic iteration without map overhead (the set is tiny by contract).
func buildRemoveSubstrPatterns(config *ScoringConfig) []string {
	patterns := make([]string, 0, len(config.SubstringRemovePatterns))
	for pattern := range config.SubstringRemovePatterns {
		if len(pattern) > 0 {
			patterns = append(patterns, pattern)
		}
	}
	return patterns
}

// NewDefaultScorerWithConfig creates a new DefaultScorer with custom configuration.
// If config is nil, the default configuration is used.
//
// Test/calibration surface: production code always builds the default
// configuration; this constructor is exercised by scoring_test.go to tune
// ScoringConfig against the fixture corpus.
func NewDefaultScorerWithConfig(config *ScoringConfig) *DefaultScorer {
	if config == nil {
		config = DefaultScoringConfig()
	}
	return newDefaultScorerWithConfig(config)
}

// newDefaultScorerWithConfig assembles a DefaultScorer from a non-nil config,
// including the derived scan tables, so every construction path produces the
// same indexed state.
func newDefaultScorerWithConfig(config *ScoringConfig) *DefaultScorer {
	removeWordIndex := buildRemoveWordIndex(config)
	removeScanClass, removeScanBuckets := densifyFirstByteIndex(removeWordIndex)
	return &DefaultScorer{
		config:               config,
		patternIndex:         buildPatternPrefixIndex(config),
		removeScanClass:      removeScanClass,
		removeScanBuckets:    removeScanBuckets,
		removeSubstrPatterns: buildRemoveSubstrPatterns(config),
	}
}

// densifyFirstByteIndex converts a sparse [256][]string first-byte index into a
// one-byte class table plus a dense bucket slice, mirroring mediaScanClass /
// mediaScanBuckets in media.go: the scanning hot path pays one table load per
// input byte instead of iterating all 256 (mostly empty) buckets per call.
func densifyFirstByteIndex(index [256][]string) (cls [256]uint8, buckets [][]string) {
	for b, bucket := range &index {
		if len(bucket) > 0 {
			cls[b] = uint8(len(buckets) + 1)
			buckets = append(buckets, bucket)
		}
	}
	return cls, buckets
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

	return s.applyScoringRules(node, metrics)
}

// applyScoringRules is scoreWithMetrics without the ShouldRemove guard. Both
// call sites have already excluded removed nodes by the time they get here —
// scoreWithMetrics checks it directly, and foldAndScore folds it into isSkip —
// so skipping the (pattern-matching, per-attribute) removal pass here avoids
// evaluating ShouldRemove twice per element on the article-scoring walk. The
// remaining guards are cheap string comparisons and are kept so a direct call
// cannot score an ineligible node.
func (s *DefaultScorer) applyScoringRules(node *html.Node, metrics contentMetrics) int {
	if node == nil || node.Type != html.ElementNode || IsNonContentElement(node.Data) || node.Data == "p" {
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

	// Penalize high link density when the links are navigation-like: many
	// short links. The old gate on absolute text length (textLength < 500)
	// both crushed link-wrapped card grids whose card text stayed under 500
	// chars and let through long tag clouds. Gating on average link-text length
	// instead — nav bars and tag clouds are many short links ("Home", "About"),
	// card grids wrap long prose — preserves the card-grid fix without the
	// absolute-length blind spot. The penalty is applied only when the node has
	// any links and is still gated on text length so a genuinely long article
	// body (which may legitimately be link-dense) is never crushed.
	linkDensity := calculateLinkDensityFromMetrics(metrics)
	if metrics.linkCount > 0 && linkDensity > lowLinkDensityThreshold {
		navLike := metrics.linkTextLength/metrics.linkCount < navLinkTextThreshold
		if navLike || metrics.textLength < linkDensityPenaltyTextThreshold {
			if linkDensity > highLinkDensityThreshold {
				score = int(float64(score) * highLinkDensityPenalty)
			} else if linkDensity > mediumLinkDensityThreshold {
				score = int(float64(score) * mediumLinkDensityPenalty)
			} else if linkDensity > lowLinkDensityThreshold {
				score = int(float64(score) * lowLinkDensityPenalty)
			}
		}
	}

	// Text-quality features (P2). Both are gated on substantial text so short
	// fragments (a heading, a caption, a table cell row) are never penalized.
	if textLength >= longTextThreshold {
		// Data-grid penalty: text that is mostly digits is tabular or numeric
		// chrome (price grids, stat cards, screener tables), not prose. Real
		// prose rarely exceeds 20% digits; the threshold leaves headroom for
		// finance or data journalism.
		if float64(metrics.digitCount)/float64(textLength) > highDigitRatioThreshold {
			score = int(float64(score) * dataGridPenalty)
		}
		// Label-soup penalty: 200+ characters with (nearly) no sentence
		// punctuation or commas is a run of labels and titles — tag clouds,
		// keyword lists, link-card titles — rather than running prose. CJK
		// prose is covered too: 。！？ feed sentenceCount.
		if punct := metrics.commaCount + metrics.sentenceCount; punct == 0 ||
			float64(punct)/float64(textLength) < minProsePunctDensity {
			score = int(float64(score) * labelSoupPenalty)
		}
	}

	// Bonus for comma-rich content (likely prose)
	if metrics.commaCount > commaBonusThreshold {
		score += metrics.commaCount * commaBonusMultiplier
	}

	// Bonus for sentence-dense content (prose with real sentence structure).
	// This strengthens the weak comma-count prose signal, especially for CJK
	// text, which carries no ASCII commas but terminates sentences with fullwidth
	// full stops (。！？).
	if metrics.sentenceCount > sentenceBonusThreshold {
		score += metrics.sentenceCount * sentenceBonusMultiplier
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
// prefer the existing node (first-wins), so this path picks a candidate
// deterministically; SelectBestCandidate instead resolves ties by map
// iteration order (nondeterministic), and the two paths can legitimately
// disagree on exact ties.
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
//
// Test/calibration surface: production extraction goes through
// FindBestArticleNode (the map-free single pass); this method survives as the
// equivalence anchor for that test and for scorer tuning.
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
//
// The raw winner is then widened by climbToContentWrapper: a winning block
// whose parent scores at least as well, is not chrome, and whose text the
// block dominates is almost always one fragment of an article split across
// siblings under that wrapper — the wrapper is the article. This generalizes
// the single-level expandToArticleCluster patch to any depth.
func (s *DefaultScorer) FindBestArticleNode(root *html.Node) *html.Node {
	cc := &candidateCollector{bestScore: -1}
	s.foldAndScore(root, false, cc)
	if cc.bestNode == nil {
		return nil
	}
	return s.climbToContentWrapper(cc.bestNode, cc.bestScore)
}

// climbToContentWrapper widens best upward while every step is safe. A step to
// the parent requires ALL of:
//
//   - the parent is an element other than <body>/<html> (the body fallback is
//     the caller's decision, never a scored selection) and is not itself
//     removal-marked chrome;
//   - dominance: best already holds at least half the parent's text — the
//     parent adds at most a sibling fragment of the same article, not a
//     competing section (this also bounds the chain logarithmically: each
//     step can at most double the text, so the hard step cap is a backstop);
//   - the parent does not score below the current node: wrappers dragged
//     down by their extra chrome (link density, label-soup penalties) stop
//     the climb, and equal scores climb — ties prefer the wider ancestor,
//     whose chrome CleanContentNode strips afterwards.
//
// Ancestor scores are computed lazily (collectContentMetrics + scoreWithMetrics
// per step) rather than via a candidates map, keeping the scoring fold
// allocation-free; nested ancestor subtrees shrink, so the whole climb costs
// a small multiple of the final subtree.
func (s *DefaultScorer) climbToContentWrapper(best *html.Node, bestScore int) *html.Node {
	for range maxScoreClimbSteps {
		parent := best.Parent
		if parent == nil || parent.Type != html.ElementNode {
			break
		}
		switch parent.Data {
		case "body", "html":
			return best
		}
		if s.ShouldRemove(parent) {
			break
		}
		// Length-only dominance test: TextContentLength is the allocation-free
		// twin of len(GetTextContent(...)) (see helpers.go), and each climb step
		// previously materialized two whole-subtree strings just to compare them.
		if TextContentLength(best)*2 < TextContentLength(parent) {
			break
		}
		parentScore := s.scoreWithMetrics(parent, collectContentMetrics(parent))
		if parentScore <= 0 || parentScore < bestScore {
			break
		}
		best, bestScore = parent, parentScore
	}
	return best
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

	isSkip := node.Type == html.ElementNode && (isSkipElement(node) || metricsSkipTags(node.Data) || s.ShouldRemove(node))

	var m contentMetrics
	if node.Type == html.ElementNode && !isSkip {
		if isBlockElementForScoring(node.Data) {
			m.blockCount++
		}
		switch node.Data {
		case "p":
			m.paragraphCount++
		case "h1", "h2", "h3", "h4", "h5", "h6":
			m.headingCount++
		case "a":
			m.linkCount++
		}
		if node.Data == "a" {
			insideLink = true
		}
	}

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		cm := s.foldAndScore(child, insideLink, cc)
		if !isSkip {
			m.blockCount += cm.blockCount
			m.paragraphCount += cm.paragraphCount
			m.headingCount += cm.headingCount
			m.linkCount += cm.linkCount
			m.textLength += cm.textLength
			m.linkTextLength += cm.linkTextLength
			m.commaCount += cm.commaCount
			m.sentenceCount += cm.sentenceCount
			m.digitCount += cm.digitCount
		}
	}

	if node.Type == html.TextNode {
		// Mirrors collectContentMetrics' text-node handling verbatim so the two
		// paths cannot diverge (guarded by TestScoreArticleCandidatesMatchesNaiveLoop).
		// The NBSP literal is spelled with the \u00a0 escape: a raw U+00A0 byte
		// in source is visually indistinguishable from a plain space, and an
		// earlier version of this block used the ASCII string "00a0" — which
		// rewrote ordinary text such as "#00a0ff" and made the folded metrics
		// disagree with collectContentMetrics on any document containing it.
		data := node.Data
		if strings.Contains(data, "\u00a0") {
			data = strings.ReplaceAll(data, "\u00a0", " ")
		}
		if text := strings.TrimSpace(data); text != "" {
			textLen := len(text)
			m.textLength += textLen
			m.commaCount += strings.Count(text, ",") + strings.Count(text, "，")
			m.sentenceCount += countSentenceTerminators(text)
			m.digitCount += countDigitBytes(text)
			if insideLink {
				m.linkTextLength += textLen
			}
		}
	}

	// m now holds node's complete subtree metrics; score it if it is a candidate.
	// The "p" guard mirrors scoreWithMetrics' (a <p> always scores 0); calling
	// applyScoringRules instead skips the redundant ShouldRemove pass that
	// isSkip has already evaluated for this node.
	if node.Type == html.ElementNode && !isSkip && !IsInlineElement(node.Data) && node.Data != "p" {
		if score := s.applyScoringRules(node, m); score > 0 {
			cc.record(node, score)
		}
	}

	return m
}

// contentAreaSignalList lists class/id tokens that, when present alongside a
// removal pattern in the same attribute value, indicate a CSS layout wrapper
// (e.g. foolcom-grid-content-sidebar, article-with-sidebar) rather than the
// non-content region itself. Only patterns that unambiguously denote a
// primary-content area are included; patterns such as "main", "post", and
// "text" are excluded because they also appear in non-content class names
// (main-nav, post-nav, nav-text) where an override would be harmful.
// A fixed slice rather than a map: ShouldRemove consults it per class/id
// attribute, and map iteration carries hashing and random-order overhead the
// deterministic scan does not.
var contentAreaSignalList = [...]string{
	"content",
	"article",
}

// hasContentAreaSignal reports whether value (a lowercased class or id
// attribute) contains any contentAreaSignalList pattern as a word-bounded
// match. ShouldRemove uses this to exempt layout wrappers from the class/id-based
// removal heuristic so the main content nested inside them is not discarded.
func hasContentAreaSignal(value string) bool {
	for _, pattern := range &contentAreaSignalList {
		if hasWordBoundary(value, pattern, boundaryStandard) {
			return true
		}
	}
	return false
}

// matchesRemovalPatterns reports whether lowerVal (a lowercased class or id
// attribute value) matches any configured removal pattern: word-bounded
// RemovePatterns via the first-byte scan tables, and SubstringRemovePatterns
// via plain substring scans.
//
// The word-bounded check is a single forward pass over lowerVal: at each byte,
// the class table dispatches to the (few) patterns whose first byte matches,
// and hasWordBoundaryAt checks whether one occurs at exactly that position with
// valid boundary characters. An occurrence can only start where the pattern's
// first byte equals the value's byte, so this visits the same (position,
// pattern) occurrence pairs the previous implementation — one strings.Index
// scan per pattern across the whole value, plus a 256-bucket presence loop per
// attribute — tested, and returns the same boolean. The profiler attributed
// ~10% of class-heavy-document CPU to those per-pattern scans.
func (s *DefaultScorer) matchesRemovalPatterns(lowerVal string) bool {
	for i := 0; i < len(lowerVal); i++ {
		class := s.removeScanClass[lowerVal[i]]
		if class == 0 {
			continue
		}
		for _, pattern := range s.removeScanBuckets[class-1] {
			if hasWordBoundaryAt(lowerVal, i, pattern) {
				return true
			}
		}
	}
	for _, pattern := range s.removeSubstrPatterns {
		if strings.Contains(lowerVal, pattern) {
			return true
		}
	}
	return false
}

var hiddenClassTokens = [...]string{
	"hidden",
	"hide",
	"is-hidden",
	"sr-only",
	"visually-hidden",
	"d-none",
	"invisible",
}

// hiddenResponsiveSuffixes are the Bootstrap 3 responsive visibility breakpoint
// suffixes: class="hidden-sm" means "hidden only on small screens" and is VISIBLE
// on larger ones, so stripping it would drop legitimately visible content. The
// "-up"/"-down" forms ("hidden-md-up" = hidden on medium and up) are the same
// family and are included alongside the plain breakpoint. When the token
// "hidden" (or "hide") is immediately followed by one of these as a bounded
// token, it is a responsive class, not an always-hidden class.
var hiddenResponsiveSuffixes = [...]string{
	"xs", "sm", "md", "lg", "xl", "print",
	"xs-up", "sm-up", "md-up", "lg-up", "xl-up",
	"xs-down", "sm-down", "md-down", "lg-down", "xl-down",
}

// isResponsiveHiddenSuffix reports whether lowerVal, at the position just past a
// matched "hidden"/"hide" token, continues with a responsive-visibility suffix
// ("-sm", "-md", ...). The suffix must itself be bounded so "-small" (a real word
// that merely starts with "sm") is not confused with the breakpoint "sm".
func isResponsiveHiddenSuffix(lowerVal string, end int) bool {
	if end+2 > len(lowerVal) || lowerVal[end] != '-' {
		return false
	}
	rest := lowerVal[end+1:]
	for _, suffix := range hiddenResponsiveSuffixes {
		if len(rest) < len(suffix) {
			continue
		}
		if rest[:len(suffix)] != suffix {
			continue
		}
		// The suffix must be bounded: end of string, or a non-alphanumeric
		// boundary ("hidden-sm ", "hidden-md-up"). A following letter/digit would
		// mean a longer word ("hidden-smaller") that is not a breakpoint.
		after := len(suffix)
		if after >= len(rest) {
			return true
		}
		c := rest[after]
		if !isAlphaNumericByte(c) {
			return true
		}
	}
	return false
}

// hiddenClassScanClass maps each byte to a dense bucket id (1+) for the
// hiddenClassTokens buckets, or 0 when no token begins with that byte. It is the
// same one-byte dispatch idiom as removeScanClass (and mediaScanClass in
// media.go): ShouldRemove consults it per byte of a class/id value so the whole
// hidden-token set is evaluated in one pass, without a strings.Index scan per
// token per attribute.
var hiddenClassScanClass, hiddenClassScanBuckets = func() ([256]uint8, [][]string) {
	var index [256][]string
	for _, token := range hiddenClassTokens {
		if token == "" {
			continue
		}
		index[token[0]] = append(index[token[0]], token)
	}
	return densifyFirstByteIndex(index)
}()

// matchesHiddenClassToken reports whether lowerVal (a lowercased class or id
// attribute value) contains a class token that reliably denotes a hidden
// element. Unlike matchesRemovalPatterns, no leading word boundary is required:
// tokens such as "d-none" (Bootstrap) and "sr-only" are complete class names.
// A trailing boundary IS required (end-of-string or a non-alphanumeric byte), so
// a token cannot fire inside a longer word ("hiddenize" is not "hidden"). The
// "hidden"/"hide" tokens are additionally exempted when they carry a Bootstrap 3
// responsive-visibility suffix (hidden-sm, ...), which means "hidden only on
// that breakpoint" — visible elsewhere.
func matchesHiddenClassToken(lowerVal string) bool {
	for i := 0; i < len(lowerVal); i++ {
		class := hiddenClassScanClass[lowerVal[i]]
		if class == 0 {
			continue
		}
		for _, token := range hiddenClassScanBuckets[class-1] {
			if !strings.HasPrefix(lowerVal[i:], token) {
				continue
			}
			end := i + len(token)
			if end < len(lowerVal) && isAlphaNumericByte(lowerVal[end]) {
				continue // token continues into a longer word ("hiddenize")
			}
			if (token == "hidden" || token == "hide") && isResponsiveHiddenSuffix(lowerVal, end) {
				continue
			}
			return true
		}
	}
	return false
}

// isAlphaNumericByte reports whether c is an ASCII letter or digit.
func isAlphaNumericByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// ShouldRemove determines if a node should be removed from the content tree.
func (s *DefaultScorer) ShouldRemove(node *html.Node) bool {
	if node == nil || node.Type != html.ElementNode {
		return false
	}

	if s == nil || s.config == nil {
		return false
	}

	if hardNonContent(node.Data) {
		return true
	}
	// A soft landmark (header/footer/aside) outside a primary-content container
	// is page chrome and must be stripped; inside <article>/<main> it is article
	// furniture (byline, author footer, pull-quote) and is kept. This mirrors
	// isSkipElement, which the extraction and scoring walks use for the same
	// distinction, so ShouldRemove cannot strip a node that extraction keeps.
	if isSoftLandmark(node.Data) && !insidePrimaryContent(node) {
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
			// A hidden-element class token (hidden, sr-only, d-none, ...) marks
			// content that is visually absent — consent walls, folded "read more"
			// regions, SEO filler — and must be stripped. On primary-content
			// containers (<article>/<main>) this class check is skipped by the
			// guard above; the unambiguous hidden ATTRIBUTE and style cases below
			// still fire on them. It is independent of the content-area-signal
			// guard below: a plain content wrapper with an explicit hidden class
			// is hidden even when its class also carries a content-area signal.
			// It runs as a substring test (no word boundary) so hyphenated tokens
			// like "d-none" and camelCase "isHidden" forms are caught.
			if matchesHiddenClassToken(lowerVal) {
				return true
			}
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
			if s.matchesRemovalPatterns(lowerVal) {
				return true
			}
		case "style":
			if isHiddenByStyle(attr.Val) {
				return true
			}
		case "hidden":
			return true
		case "aria-hidden":
			if strings.EqualFold(strings.TrimSpace(attr.Val), "true") {
				return true
			}
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
		// "!important" is a CSS priority marker, not part of the value, and
		// "display:none !important" is the most common spelling in SPA/CMS
		// markup for hidden elements. Strip it so the exact comparisons below
		// still recognize the declaration as hiding the element.
		val = strings.TrimSpace(strings.TrimSuffix(val, "!important"))
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
			attributeScore := s.calculatePatternScore(lowerVal)
			if attr.Key == "id" {
				// An id is page-unique and names the element's role; a class
				// is one of many styling hooks on the element. Weight id
				// matches double (the Readability convention): id="content"
				// says more about the node than class="content-utility-text".
				attributeScore *= 2
			}
			score += attributeScore
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

// calculatePatternScore calculates score based on pattern matching against the
// merged, prefix-bucketed pattern index. One pass over the value's distinct
// (alphanumeric, case-folded) characters replaces the five per-category passes
// this function used to make: every pattern's score is pre-summed across the
// categories at scorer construction, so a single match adds the same total the
// per-category matches would have added together. Uses fixed-size arrays to
// avoid heap allocation.
//
// CONSTRAINT: only patterns whose first byte is an ASCII letter or digit are
// reachable — buckets are consulted solely for the alphanumeric characters
// present in value. Scoring patterns in a custom ScoringConfig must therefore
// start with an ASCII alphanumeric byte (the default configuration's all do);
// a pattern starting with any other byte — "-", "…", or a CJK lead byte such
// as the "导" of "导航" — is silently never matched. The removal-pattern path
// (matchesRemovalPatterns) dispatches on every byte and has no such limit.
func (s *DefaultScorer) calculatePatternScore(value string) int {
	if len(value) == 0 {
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
		for _, ps := range s.patternIndex[presentChars[i]] {
			if hasWordBoundary(value, ps.pattern, boundaryStandard) {
				score += ps.score
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
