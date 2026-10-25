// constants.go provides centralized constant definitions for internal use.
package internal

const (
	// URL validation limits
	MaxURLLength     = 2000   // Maximum URL length
	MaxDataURILength = 100000 // Maximum data URL length (100KB)

	// Scoring constants
	strongPositiveScore = 400
	mediumPositiveScore = 200
	strongNegativeScore = -400
	mediumNegativeScore = -200
	weakNegativeScore   = -100

	minParagraphsForBonus       = 3
	manyParagraphsMultiplier    = 150
	fewParagraphsMultiplier     = 80
	headingMultiplier           = 100
	veryLongTextThreshold       = 500
	longTextThreshold           = 200
	mediumTextThreshold         = 100
	shortTextThreshold          = 50
	veryLongTextBonusMultiplier = 10
	longTextBonusDivider        = 2
	mediumTextBonusDivider      = 3
	shortTextPenalty            = -300
	highLinkDensityThreshold    = 0.5
	mediumLinkDensityThreshold  = 0.3
	lowLinkDensityThreshold     = 0.15
	highContentDensityThreshold = 0.7
	lowContentDensityThreshold  = 0.3
	highDensityMultiplier       = 1.2
	lowDensityMultiplier        = 0.7
	highLinkDensityPenalty      = 0.2
	mediumLinkDensityPenalty    = 0.5
	lowLinkDensityPenalty       = 0.75
	// linkDensityPenaltyTextThreshold is the absolute text-length gate the
	// link-density penalty still respects: a genuinely long article body may be
	// link-dense legitimately, and must never be crushed by the density penalty
	// even when its links are short (a long list of short-linked entries is the
	// one nav-like shape that is also substantial). The primary navigation
	// signal is now navLinkTextThreshold (average link-text length), which
	// catches short tag clouds that the old text-length-only gate let through.
	linkDensityPenaltyTextThreshold = 500
	// charsPerBlockForFullDensity is the average text-per-block length (in
	// characters) that maps to a content density of 1.0. Density is
	// averageTextPerBlock / charsPerBlockForFullDensity, clamped to [0,1]. The
	// factor matches the original textLength/(tagCount*10) scale: ~10 chars of
	// text per block element is "full" density. Counting only block elements
	// (not inline formatting tags) is what changed — heavily-formatted prose is
	// no longer misread as low density the way a total-tag count misread it.
	charsPerBlockForFullDensity = 10
	// navLinkTextThreshold is the average link-text length (linkTextLength /
	// linkCount) below which a node's links are considered navigation-like.
	// Nav bars and tag clouds are made of many short links ("Home", "About"),
	// whereas main content that wraps prose or cards in <a> uses long links.
	navLinkTextThreshold    = 25
	commaBonusThreshold     = 5
	commaBonusMultiplier    = 10
	sentenceBonusThreshold  = 3
	sentenceBonusMultiplier = 15
	// Text-quality feature thresholds (applyScoringRules). highDigitRatioThreshold
	// is the digit-byte share above which long text reads as a data grid;
	// minProsePunctDensity is the comma+sentence-terminator share below which
	// long text reads as a label soup. Both penalties are deliberately mild
	// (±15%) so they tilt close calls without flipping clear selections.
	highDigitRatioThreshold = 0.4
	minProsePunctDensity    = 0.004
	dataGridPenalty         = 0.85
	labelSoupPenalty        = 0.85
	// Score-climb guard values (FindBestArticleNode): maxScoreClimbSteps bounds
	// the ancestor walk (the ≥50%-of-parent-text dominance rule already
	// bounds it logarithmically; this is a hard backstop).
	maxScoreClimbSteps = 8
	// Listing-page detection (FindListingContainer): the minimum number of
	// direct-child <article> elements and the aggregate link density above
	// which their parent is recognized as a card listing.
	minListingArticles        = 3
	listingArticleLinkDensity = mediumLinkDensityThreshold
	// layoutTableDirectTextThreshold is the minimum non-whitespace text length a
	// table with a nested <table> must carry OUTSIDE that nested table before it
	// is treated as a data table rather than a layout shell. Below this, the
	// outer table is visual scaffolding (Finviz-style nested layout tables) and
	// is flattened to its nested data tables; at or above, a cell's own prose
	// makes the outer table genuine data.
	layoutTableDirectTextThreshold = 25
)
