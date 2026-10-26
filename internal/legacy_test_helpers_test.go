package internal

// legacy_test_helpers_test.go — test-only entry points that used to live in
// production files.
//
// These functions had zero production callers: they existed so tests could
// exercise an unexported stage in isolation. Keeping them in the shipped
// package meant the module carried scaffolding (and its godoc) as if it were
// API. They are defined here instead, so the test coverage they support stays
// identical while production code carries only what it calls.
//
// Every helper is a thin wrapper over the real production path; none adds
// behavior, so a test that passes here passes against the production code.

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

// CleanText is CleanTextBytes over a string. Production reaches CleanTextBytes
// directly with the builder's byte buffer, so the string form is test-only.
func CleanText(text string) string {
	return CleanTextBytes([]byte(text))
}

// ScoreContentNode exposes DefaultScorer.Score through the shared default
// scorer.
func ScoreContentNode(node *html.Node) int {
	return getDefaultScorer().Score(node)
}

// ScoreAttributes exposes DefaultScorer.scoreAttributes through the shared
// default scorer.
func ScoreAttributes(n *html.Node) int {
	return getDefaultScorer().scoreAttributes(n)
}

// MatchesPattern reports whether value contains any pattern with word
// boundaries.
func MatchesPattern(value string, patterns map[string]bool) bool {
	for pattern := range patterns {
		if hasWordBoundary(value, pattern, boundaryStandard) {
			return true
		}
	}
	return false
}

// CalculateContentDensity returns the text-to-tag ratio of a subtree.
func CalculateContentDensity(n *html.Node) float64 {
	if n == nil {
		return 0
	}
	return calculateDensityFromMetrics(collectContentMetrics(n))
}

// CountTags counts all element nodes in the subtree.
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

// CountDescendantElements counts DESCENDANT elements of the given tag at any
// depth below n (n itself excluded).
func CountDescendantElements(n *html.Node, tag string) int {
	count := 0
	WalkNodes(n, func(node *html.Node) bool {
		if node != n && node.Type == html.ElementNode && node.Data == tag {
			count++
		}
		return true
	})
	return count
}

// SanitizeHTML sanitizes an HTML string and returns the rendered body content.
func SanitizeHTML(htmlContent string) string {
	return SanitizeHTMLWithAudit(htmlContent, NoOpAuditRecorder{})
}

// sanitizeNodeWithAudit sanitizes n and its entire subtree in place, with the
// Extract-path semantics (no button unwrap). SanitizeHTMLWithAudit's string
// round-trip goes through it; production goes through SanitizeDOM /
// SanitizeDOMReader, so the wrapper lives here with its only caller.
func sanitizeNodeWithAudit(n *html.Node, audit AuditRecorder) {
	sanitizeNodeWithAuditMode(n, audit, false)
}

// SanitizeHTMLWithAudit sanitizes an HTML string and records security events.
//
// Production sanitizes the already-parsed tree in place (SanitizeDOM), so the
// render/re-parse round trip here is test-only: it lets a test assert on the
// sanitized markup as a string.
func SanitizeHTMLWithAudit(htmlContent string, audit AuditRecorder) string {
	if htmlContent == "" {
		return ""
	}

	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return ""
	}

	sanitizeNodeWithAudit(doc, audit)

	body := findBodyElement(doc)
	if body == nil {
		return ""
	}

	var buf bytes.Buffer

	for child := body.FirstChild; child != nil; child = child.NextSibling {
		if err := html.Render(&buf, child); err != nil {
			continue
		}
	}

	return buf.String()
}

// findBodyElement locates the body element in a parsed HTML document.
// html.Parse nests <body> under <html>, so it is a grandchild of the document
// node; FindElementByTag descends to find it. Non-document input yields nil.
func findBodyElement(doc *html.Node) *html.Node {
	if doc.Type != html.DocumentNode {
		return nil
	}
	return FindElementByTag(doc, "body")
}
