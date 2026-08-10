package internal

import (
	"strings"

	"golang.org/x/net/html"
)

// ScoreContentNode calculates a relevance score for content extraction.
// Higher scores indicate more likely main content. Negative scores suggest non-content elements.
// Exported for testing only.
func ScoreContentNode(node *html.Node) int {
	return getDefaultScorer().Score(node)
}

// ShouldRemoveElement determines if a node should be removed from the content tree.
// This function delegates to the default Scorer implementation.
func ShouldRemoveElement(n *html.Node) bool {
	return getDefaultScorer().ShouldRemove(n)
}

// ScoreAttributes calculates a score based on element attributes.
// Exported for testing only.
func ScoreAttributes(n *html.Node) int {
	return getDefaultScorer().ScoreAttributes(n)
}

// contentMetrics holds all metrics collected during a single DOM traversal.
type contentMetrics struct {
	paragraphCount  int
	headingCount    int
	textLength      int
	linkTextLength  int
	totalTextLength int
	tagCount        int
	commaCount      int
}

// metricsSkipTags lists tags whose subtrees contain no extractable main-content
// text and must be excluded from content scoring, in addition to
// IsNonContentElement (script/style/noscript/nav/aside/footer/header). These
// are raw-text or embedded-content elements (svg, math, template) and document
// metadata (head, title) whose internal text never appears in extracted output.
// Skipping them keeps scoring consistent with extraction and decouples the
// score from whether SanitizeDOM has already removed them.
var metricsSkipTags = map[string]bool{
	"svg": true, "math": true, "template": true,
	"head": true, "title": true,
}

// collectContentMetrics collects all scoring metrics in a single DOM traversal.
// This is more efficient than calling separate functions for each metric.
// Optimized with inline NBSP handling to avoid function call overhead.
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
			if IsNonContentElement(n.Data) || metricsSkipTags[n.Data] || ShouldRemoveElement(n) {
				return false
			}
			metrics.tagCount++
			switch n.Data {
			case "p":
				metrics.paragraphCount++
			case "h1", "h2", "h3", "h4", "h5", "h6":
				metrics.headingCount++
			}
		} else if n.Type == html.TextNode {
			// Inline NBSP normalization - avoid function call overhead
			data := n.Data
			dataLen := len(data)

			// Fast path: check if any NBSP present (UTF-8: 0xC2 0xA0).
			// The i+1 < dataLen boundary (rather than i < dataLen-1) reads more
			// naturally as "there is a pair starting at i" and sidesteps the
			// dataLen == 0 edge case without a separate guard.
			hasNBSP := false
			for i := 0; i+1 < dataLen; i++ {
				if data[i] == 0xC2 && data[i+1] == 0xA0 {
					hasNBSP = true
					break
				}
			}

			var textData string
			if hasNBSP {
				textData = strings.ReplaceAll(data, "\u00a0", " ")
			} else {
				textData = data
			}

			text := strings.TrimSpace(textData)
			if text != "" {
				metrics.textLength += len(text)
				metrics.totalTextLength += len(text)
				metrics.commaCount += strings.Count(text, ",") + strings.Count(text, "，")

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
func calculateDensityFromMetrics(m contentMetrics) float64 {
	if m.textLength == 0 {
		return 0
	}
	if m.tagCount == 0 {
		return 1.0
	}
	density := float64(m.textLength) / (float64(m.tagCount) * 10)
	if density > 1.0 {
		return 1.0
	}
	return density
}

// calculateLinkDensityFromMetrics calculates link density from collected metrics.
func calculateLinkDensityFromMetrics(m contentMetrics) float64 {
	if m.totalTextLength == 0 {
		return 0.0
	}
	return float64(m.linkTextLength) / float64(m.totalTextLength)
}

// MatchesPattern checks if value contains any pattern from the map with word boundaries.
// This is exported for testing purposes.
func MatchesPattern(value string, patterns map[string]bool) bool {
	for pattern := range patterns {
		if hasWordBoundary(value, pattern, boundaryStandard) {
			return true
		}
	}
	return false
}

// CalculateContentDensity calculates text-to-tag ratio.
// Exported for testing only.
func CalculateContentDensity(n *html.Node) float64 {
	if n == nil {
		return 0
	}
	metrics := collectContentMetrics(n)
	return calculateDensityFromMetrics(metrics)
}

// CountTags counts all element nodes in the subtree.
// Exported for testing only.
func CountTags(n *html.Node) int {
	count := 0
	WalkNodes(n, func(node *html.Node) bool {
		if node.Type == html.ElementNode {
			count++
		}
		return true
	})
	return count
}

// CountChildElements counts child elements of specific tag type.
// Exported for testing only.
func CountChildElements(n *html.Node, tag string) int {
	count := 0
	WalkNodes(n, func(node *html.Node) bool {
		if node != n && node.Type == html.ElementNode && node.Data == tag {
			count++
		}
		return true
	})
	return count
}
