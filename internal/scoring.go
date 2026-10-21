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
	tagCount       int
	commaCount     int
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
			if IsNonContentElement(n.Data) || metricsSkipTags(n.Data) || ShouldRemoveElement(n) {
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
// The denominator is textLength: the two were kept as separate always-equal
// fields (a historical "total" distinction never implemented), and the
// link-density read was the only consumer of the redundant one.
func calculateLinkDensityFromMetrics(m contentMetrics) float64 {
	if m.textLength == 0 {
		return 0.0
	}
	return float64(m.linkTextLength) / float64(m.textLength)
}
