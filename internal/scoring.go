package internal

import (
	"strings"

	"golang.org/x/net/html"
)

// ShouldRemoveElement determines if a node should be removed from the content
// tree, delegating to the shared default Scorer. CleanContentNode calls it for
// every element, so it is production surface rather than a test entry point.
func ShouldRemoveElement(n *html.Node) bool {
	return getDefaultScorer().ShouldRemove(n)
}

// contentMetrics holds all metrics collected during a single DOM traversal.
type contentMetrics struct {
	paragraphCount int
	headingCount   int
	textLength     int
	linkTextLength int
	blockCount     int
	linkCount      int
	commaCount     int
	sentenceCount  int
	digitCount     int
}

// metricsSkipTags lists tags whose subtrees contain no extractable main-content
// text and must be excluded from content scoring, in addition to
// IsNonContentElement (script/style/noscript/nav/aside/footer/header). These
// are raw-text or embedded-content elements (svg, math, template) and document
// metadata (head, title) whose internal text never appears in extracted output.
// Skipping them keeps scoring consistent with extraction and decouples the
// score from whether SanitizeDOM has already removed them.
//
// Switch-based for the same hot-path reason as the element classifications in
// elements.go: this test runs per element on every scoring walk.
func metricsSkipTags(tag string) bool {
	switch tag {
	case "svg", "math", "template", "head", "title":
		return true
	}
	return false
}

// collectContentMetrics collects all scoring metrics in a single DOM traversal.
// This is more efficient than calling separate functions for each metric.
// Optimized with inline NBSP handling to avoid function call overhead.
//
// The subtree skip set (including ShouldRemoveElement) always reflects the
// SHARED default scorer's policy, even when a custom ScoringConfig is in
// play: that config's RemovePatterns affects only the node-level guard in
// scoreWithMetrics. NewDefaultScorerWithConfig is a test/calibration surface,
// so the mixed granularity is documented here rather than unified.
func collectContentMetrics(node *html.Node) contentMetrics {
	var metrics contentMetrics

	WalkNodes(node, func(n *html.Node) bool {
		if n.Type == html.ElementNode {
			// Skip subtrees that carry no extractable main-content text. This
			// mirrors ExtractTextWithStructureAndImages (which skips
			// IsNonContentElement) and prevents script/style/svg/nav/header
			// text from inflating a candidate's content metrics. Without it,
			// pages that inline large script payloads (e.g. VitePress SSR
			// data) score wildly differently depending on whether SanitizeDOM
			// has removed those tags, and the article extractor can pick the
			// wrong node. Returning false stops traversal of this subtree.
			//
			// ShouldRemoveElement is also checked so that class/id-based
			// removal targets (nav containers, ad slots, widgets, etc.) do
			// not inflate ancestor scores. Without this, an anonymous wrapper
			// div around a mega-menu can outscore the real article body; when
			// CleanContentNode then strips the wrapper's children, the
			// extraction yields empty output.
			if isSkipElement(n) || metricsSkipTags(n.Data) || ShouldRemoveElement(n) {
				return false
			}
			if isBlockElementForScoring(n.Data) {
				metrics.blockCount++
			}
			switch n.Data {
			case "p":
				metrics.paragraphCount++
			case "h1", "h2", "h3", "h4", "h5", "h6":
				metrics.headingCount++
			case "a":
				metrics.linkCount++
			}
		} else if n.Type == html.TextNode {
			// Inline NBSP normalization: Contains is the vectorized equivalent
			// of the byte-pair scan it replaces, and the conditional ReplaceAll
			// avoids the copy bytes.Replace would make on NBSP-free text.
			var textData string
			if data := n.Data; strings.Contains(data, "\u00a0") {
				textData = strings.ReplaceAll(data, "\u00a0", " ")
			} else {
				textData = data
			}

			text := strings.TrimSpace(textData)
			if text != "" {
				metrics.textLength += len(text)
				metrics.commaCount += strings.Count(text, ",") + strings.Count(text, "，")
				metrics.sentenceCount += countSentenceTerminators(text)
				metrics.digitCount += countDigitBytes(text)

				// Check if this text is inside a link
				for parent := n.Parent; parent != nil; parent = parent.Parent {
					if parent.Type == html.ElementNode && parent.Data == "a" {
						metrics.linkTextLength += len(text)
						break
					}
				}
			}
		}
		return true
	})

	return metrics
}

// calculateDensityFromMetrics calculates content density from collected metrics.
// Density is the average text length per block element, scaled so that
// charsPerBlockForFullDensity characters per block maps to 1.0 and clamped to
// [0,1]. Only block-level elements are counted (not inline formatting tags), so
// prose with heavy <span>/<em> markup is not misread as low density the way a
// total-tag count did.
func calculateDensityFromMetrics(m contentMetrics) float64 {
	if m.textLength == 0 {
		return 0
	}
	if m.blockCount == 0 {
		return 1.0
	}
	density := float64(m.textLength) / float64(m.blockCount) / charsPerBlockForFullDensity
	if density > 1.0 {
		return 1.0
	}
	return density
}

// countSentenceTerminators counts sentence-ending punctuation in text: the ASCII
// sentence terminators plus the CJK forms — ideographic full stop 。(U+3002,
// E3 80 82) and fullwidth ！(U+FF01, EF BC 81) ？(U+FF1F, EF BC 9F) — so CJK
// prose, which carries no commas but is rich in full stops, still scores as
// prose. It is a byte scan matching ASCII terminators directly and the CJK
// terminators by their distinct UTF-8 lead/prefix bytes.
func countSentenceTerminators(text string) int {
	count := 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '.', '!', '?':
			count++
		case 0xE3:
			// 。 U+3002 = E3 80 82.
			if i+2 < len(text) && text[i+1] == 0x80 && text[i+2] == 0x82 {
				count++
			}
			i += 2
		case 0xEF:
			// ！ U+FF01 = EF BC 81, ？ U+FF1F = EF BC 9F.
			if i+2 < len(text) && text[i+1] == 0xBC {
				switch text[i+2] {
				case 0x81, 0x9F:
					count++
				}
			}
			i += 2
		}
	}
	return count
}

// countDigitBytes counts ASCII digit bytes in text — the raw material for the
// data-grid penalty in applyScoringRules.
func countDigitBytes(text string) int {
	count := 0
	for i := 0; i < len(text); i++ {
		if text[i] >= '0' && text[i] <= '9' {
			count++
		}
	}
	return count
}

// FindListingContainer returns the outermost element that is a listing/index
// page's card container: at least minListingArticles direct-child <article>
// elements whose combined link density exceeds listingArticleLinkDensity, in a
// container that is not itself removal-marked chrome (a .related wrapper of
// article cards is chrome, not the page's listing). Nil when no such container
// exists — the common case on article pages.
//
// Card listings mark each card up as its own <article> (blog indexes, news
// feeds, search results), so every card carries the +1000 article tag score
// and plain candidate selection picks a single card. Recognizing the shape
// lets the caller extract the whole list and report it as a listing instead.
func FindListingContainer(root *html.Node) *html.Node {
	var found *html.Node
	WalkNodes(root, func(n *html.Node) bool {
		if found != nil {
			return false
		}
		if n.Type != html.ElementNode || !isListingContainer(n) {
			return true
		}
		found = n
		return false
	})
	return found
}

// isListingContainer reports whether node itself is a listing container per
// FindListingContainer's contract.
func isListingContainer(node *html.Node) bool {
	articles := 0
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "article" {
			articles++
		}
	}
	if articles < minListingArticles {
		return false
	}
	// Chrome wrappers (related/recirc sections) also wrap article cards;
	// they are removed later and must not be selected as the listing.
	if ShouldRemoveElement(node) {
		return false
	}

	linkText, totalText := 0, 0
	for c := node.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || c.Data != "article" {
			continue
		}
		m := collectContentMetrics(c)
		linkText += m.linkTextLength
		totalText += m.textLength
	}
	return totalText > 0 && float64(linkText)/float64(totalText) > listingArticleLinkDensity
}

// calculateLinkDensityFromMetrics returns the share of a node's text that
// sits inside links: linkTextLength / textLength. Zero when there is no text.
func calculateLinkDensityFromMetrics(m contentMetrics) float64 {
	if m.textLength == 0 {
		return 0.0
	}
	return float64(m.linkTextLength) / float64(m.textLength)
}
