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
	// linkDensityPenaltyTextThreshold is the minimum content-text length below
	// which the link-density penalty applies. Navigation bars, tag clouds, and
	// sitemaps are link-dense AND short; main content that legitimately wraps
	// prose or cards in <a> (landing pages, portfolio grids) is link-dense but
	// substantial. Gating the penalty on text length prevents the article
	// extractor from crushing a real content container and falling back to a
	// tiny sibling (e.g. a hero block).
	linkDensityPenaltyTextThreshold = 500
	commaBonusThreshold             = 5
	commaBonusMultiplier            = 10
)
