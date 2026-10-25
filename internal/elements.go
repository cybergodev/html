package internal

import (
	"strings"

	"golang.org/x/net/html"
)

// The element classifications below are switch-based rather than map-based.
// These predicates run once or more per node on every tree walk (extraction,
// scoring, sanitization, cleaning — ~7 walks per Extract), and the profiler
// showed string-map hashing (mapaccess1_faststr) at ~8% of Extract CPU, with
// IsNonContentElement alone responsible for ~45% of those lookups. A string
// switch compiles to length + byte comparisons with no hashing, and reads the
// same as the map literal it replaces.

// knownInlineNamespacePrefixes contains namespace prefixes that are typically
// used for inline data markers in structured documents like XBRL/SEC filings.
var knownInlineNamespacePrefixes = map[string]bool{
	"ix":      true, // Inline XBRL - used for inline facts in documents
	"xbrl":    true, // XBRL core elements
	"dei":     true, // Document and Entity Information
	"us-gaap": true, // US GAAP taxonomy
	"ifrs":    true, // IFRS taxonomy
	"link":    true, // XLink elements (often inline)
	"xlink":   true, // Alternative XLink namespace
}

// IsKnownInlineNamespacePrefix checks if the prefix is a known inline namespace prefix.
func IsKnownInlineNamespacePrefix(prefix string) bool {
	return knownInlineNamespacePrefixes[prefix]
}

// isBlockElementForScoring reports whether a tag is a block-level element that
// should count toward a candidate's blockCount. It is deliberately narrower than
// IsBlockElement: table-cell wrappers (td/th/tr) and structural shells
// (body/html/head) contribute no block worth dividing text by, and counting them
// skewed the density denominator on table-heavy and deeply-nested pages.
//
// (The doc comment that used to sit here described IsBlockElement but was
// physically attached to this function; it has moved to IsBlockElement below.)
func isBlockElementForScoring(tag string) bool {
	switch tag {
	case "p", "div", "pre", "blockquote",
		"h1", "h2", "h3", "h4", "h5", "h6",
		"article", "section", "main",
		"ul", "ol", "li", "dl", "dt", "dd",
		"figure", "figcaption", "address", "form", "fieldset",
		"details", "summary", "center", "canvas":
		return true
	}
	return false
}

// IsBlockElement reports whether the tag is a known block-level element.
// Block elements add newlines and paragraph spacing.
func IsBlockElement(tag string) bool {
	switch tag {
	// Text containers; headings; semantic HTML5 sections; lists; tables; forms;
	// interactive elements; other block elements; structural elements (low
	// priority, rarely appear in content extraction); deprecated elements;
	// media/interactive elements.
	case "p", "div", "pre", "blockquote",
		"h1", "h2", "h3", "h4", "h5", "h6",
		"article", "section", "main", "nav", "aside",
		"header", "footer", "figure", "figcaption",
		"ul", "ol", "li", "dl", "dt", "dd",
		"table", "thead", "tbody", "tfoot", "tr", "td", "th",
		"form", "fieldset",
		"details", "summary", "dialog",
		"hr", "address",
		"body", "html", "head",
		"center",
		"canvas":
		return true
	}
	return false
}

// IsInlineElement returns true if the tag is a known inline element.
// Inline elements should not add newlines or paragraph spacing.
// These elements flow with text on the same line.
func IsInlineElement(tag string) bool {
	switch tag {
	// Text formatting (presentational); semantic inline; media and embedded;
	// form controls; line break (special inline); metadata (no layout effect).
	//
	// canvas is intentionally NOT here: it is a block-level element (see
	// IsBlockElement / IsParagraphLevelBlockElement) and survives sanitization,
	// so classifying it inline too made IsInlineElement and IsBlockElement both
	// return true and caused the article scorer to skip <canvas> subtrees
	// (extract.go extractArticleNode) while extraction treated them as blocks.
	case "font", "b", "i", "u", "s", "strike",
		"del", "ins", "strong", "em",
		"mark", "small", "sub", "sup",
		"big", "tt",
		"span", "a", "code", "kbd", "samp",
		"var", "abbr", "cite", "q", "dfn",
		"time", "data", "ruby", "rt", "rp",
		"bdi", "wbr",
		"img", "svg", "picture",
		"video", "audio",
		"object", "embed", "iframe",
		"map",
		"input", "button", "select",
		"textarea", "label", "output",
		"br",
		"script", "style", "link", "meta", "title":
		return true
	}
	return false
}

// IsNonContentElement returns true if the tag is typically not part of main content.
// Note: <form> is intentionally excluded. Server-side frameworks (ASP.NET
// WebForms, JSF, JSP) wrap the entire page body in a single <form>; marking it
// non-content would cause ShouldRemove/CleanContentNode and the text extractor
// to drop the whole page body.
//
// The header/footer/aside entries here are the *categorical* classification:
// they are page chrome by default. Whether a specific <header>/<footer>/<aside>
// node is actually dropped is decided by isSkipElement, which keeps them when
// they nest inside a primary-content container (<article>/<main>) where they
// are article furniture (byline, author footer, pull-quote) rather than chrome.
func IsNonContentElement(tag string) bool {
	switch tag {
	case "script", "style", "noscript", "nav", "aside", "footer", "header":
		return true
	}
	return false
}

// IsRawTextElement reports whether the tag's subtree is raw text (script/
// style code), inert metadata, or a template fragment — content that must be
// skipped in EVERY extraction mode. It is the complete skip policy for
// page-chrome regions (ExtractChromeTextWithStructure), where content-oriented
// skipping must not apply: a header's nested <nav> menu is the region's
// substance, but a stray <script> is noise in a nav bar just as it is in an
// article.
func IsRawTextElement(tag string) bool {
	switch tag {
	case "script", "style", "noscript", "template":
		return true
	}
	return false
}

// hardNonContent reports whether tag is categorically non-content and must be
// skipped on every extraction/scoring walk regardless of position: raw-text and
// embedding elements plus <nav>. <nav> stays hard even though it can nest inside
// an <article>: menus there are still navigation, not prose.
func hardNonContent(tag string) bool {
	switch tag {
	case "script", "style", "noscript", "nav":
		return true
	}
	return false
}

// isSoftLandmark reports whether tag is a semantic landmark whose content may be
// article furniture (byline <header>, author <footer>, pull-quote <aside>) when
// it sits inside a primary-content container, but is page chrome when it does not.
func isSoftLandmark(tag string) bool {
	switch tag {
	case "header", "footer", "aside":
		return true
	}
	return false
}

// primaryContentClassTokens are the positive class/id tokens that mark a
// non-semantic element as a CMS content wrapper in the spirit of <article>.
// They are deliberately narrower than the scorer's full positive set: "content",
// "text", "page", and "blog"/"news"/"detail" are excluded because they also name
// page-level chrome ("page-header", "main-content-wrapper" around a whole layout,
// "blog-nav"), where treating the wrapper as primary content would wrongly keep
// a page-level <footer>/<header>. The tokens kept here are the ones that almost
// always name the article body itself: "article", "post", "story", "entry",
// "body", "main".
var primaryContentClassTokens = [...]string{
	"article", "post", "story", "entry", "body", "main",
}

// hasPrimaryContentClassToken reports whether node's class or id attribute
// contains any primaryContentClassToken as a word-bounded match. It backs the
// div-based extension of insidePrimaryContent.
func hasPrimaryContentClassToken(node *html.Node) bool {
	if node == nil || node.Type != html.ElementNode {
		return false
	}
	for _, attr := range node.Attr {
		if attr.Key != "class" && attr.Key != "id" {
			continue
		}
		lowerVal := strings.ToLower(attr.Val)
		for _, token := range primaryContentClassTokens {
			if hasWordBoundary(lowerVal, token, boundaryStandard) {
				return true
			}
		}
	}
	return false
}

// isPositiveContentContainer reports whether node is a non-semantic element
// whose class/id carries a primary-content token. It is the div-based analogue
// of isPrimaryContentContainer: semantic tags and ARIA roles are checked there;
// class tokens are checked here so insidePrimaryContent can stop at the right
// wrapper.
func isPositiveContentContainer(node *html.Node) bool {
	return hasPrimaryContentClassToken(node)
}

// insidePrimaryContent reports whether node is nested inside a primary-content
// container: a semantic <article>/<main> (or role=main/article), or a
// non-semantic element whose class/id carries a primary-content token
// (article/post/story/entry/body/main). The walk stops at the first
// hard-non-content or soft-landmark ancestor, so a <footer> inside a page-level
// <header> is not mistaken for article furniture, and a positive-token wrapper
// that lies inside a page-level landmark is not reached.
func insidePrimaryContent(node *html.Node) bool {
	for p := node.Parent; p != nil; p = p.Parent {
		if p.Type != html.ElementNode {
			continue
		}
		if hardNonContent(p.Data) || isSoftLandmark(p.Data) {
			return false
		}
		if isPrimaryContentContainer(p) || isPositiveContentContainer(p) {
			return true
		}
	}
	return false
}

// isSkipElement reports whether extraction and scoring must skip node entirely:
// it is hard non-content, or a soft landmark (header/footer/aside) that lies
// outside a primary-content container. This is the single contextual skip
// predicate shared by the text extraction, the scoring metric fold, and
// ShouldRemove, so a landmark inside <article> is preserved consistently by
// every pass. Previously IsNonContentElement dropped header/footer/aside
// unconditionally, silently discarding bylines, author footers, and pull-quotes
// inside real articles.
func isSkipElement(node *html.Node) bool {
	if node == nil || node.Type != html.ElementNode {
		return false
	}
	if hardNonContent(node.Data) {
		return true
	}
	return isSoftLandmark(node.Data) && !insidePrimaryContent(node)
}

// IsParagraphLevelBlockElement returns true if the element is a block element that should
// be separated by paragraph spacing (double newlines) in the output.
//
// Paragraph-level block elements create visual separation with blank lines in Markdown:
//   - Text containers: p, div, pre, blockquote
//   - Headings: h1-h6
//   - Semantic sections: article, section, main, figure, figcaption, address
//   - Lists: ul, ol
//   - Definitions: dd (separates term/definition pairs; dt stays tight to its dd)
//   - Tables: table
//   - Forms: fieldset
//   - Interactive: details, summary, dialog
//   - Media: canvas
//
// Block elements WITHOUT paragraph spacing (treated as inline blocks):
//   - List items: li, dt
//   - Definition container: dl (its dd children already emit the paragraph spacing
//     that separates term/definition pairs; marking dl too would double the blank
//     line after the final dd)
//   - Table structure: thead, tbody, tfoot, tr, td, th
//   - Self-closing: hr
//   - Structural: body, html, head
//   - Semantic (non-content): nav, aside, header, footer, form
func IsParagraphLevelBlockElement(tag string) bool {
	switch tag {
	// Paragraph-level blocks (add double newlines)
	case "p", "div", "h1", "h2", "h3", "h4", "h5", "h6",
		"article", "section", "main", "blockquote", "pre",
		"ul", "ol", "table",
		"figure", "figcaption", "address",
		"fieldset", "details", "summary", "dialog",
		"canvas", "dd":
		return true

	// Block elements but no paragraph spacing (compact layout)
	case "li", "dt", "dl",
		"thead", "tbody", "tfoot", "tr", "td", "th",
		"hr",
		"body", "html", "head",
		"nav", "aside", "header", "footer", "form",
		"center":
		return false

	default:
		// For unknown elements, use IsBlockElement as fallback
		return IsBlockElement(tag)
	}
}

// IsNamespaceTag checks if a tag is a namespaced tag (contains ':').
// Examples: ix:nonnumeric, xbrl:value, dei:CityAreaCode
func IsNamespaceTag(tag string) bool {
	return strings.Contains(tag, ":")
}

// GetNamespacePrefix extracts the namespace prefix from a namespaced tag.
// For "ix:nonnumeric", it returns "ix".
func GetNamespacePrefix(tag string) string {
	parts := strings.SplitN(tag, ":", 2)
	if len(parts) == 2 {
		return parts[0]
	}
	return ""
}

// ShouldTreatNamespaceTagAsInline determines if a namespaced tag should be
// treated as an inline element based on context, content, and namespace.
func ShouldTreatNamespaceTagAsInline(node *html.Node) bool {
	if node == nil || node.Type != html.ElementNode {
		return false
	}

	// Rule 1: Analyze content structure first (highest priority)
	// This ensures that content characteristics override namespace assumptions
	hasElementChildren := false
	textLength := 0
	textNodeCount := 0
	newlineCount := 0

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		switch child.Type {
		case html.ElementNode:
			hasElementChildren = true
		case html.TextNode:
			text := strings.TrimSpace(child.Data)
			if text != "" {
				textNodeCount++
				textLength += len(text)
			}
			// Count newlines in original text (before trimming). Deliberately
			// OUTSIDE the text != "" guard — see the matching loop in
			// ShouldTreatAsBlockElement for the intentional asymmetry.
			newlineCount += strings.Count(child.Data, "\n")
		}
	}

	// Tags with element children are NOT inline
	if hasElementChildren {
		return false
	}

	// Tags with multi-line content are NOT inline
	if newlineCount > 0 {
		return false
	}

	// Tags with long content are NOT inline
	if textLength > 50 {
		return false
	}

	// Tags with multiple text nodes are NOT inline
	if textNodeCount > 1 {
		return false
	}

	// Rule 2: Check if the parent is an inline element
	// Tags inside inline containers (span, a, font, etc.) should be inline
	if node.Parent != nil && node.Parent.Type == html.ElementNode {
		if IsInlineElement(node.Parent.Data) {
			return true
		}
	}

	// Rule 3: Known inline namespaces are inline by default
	// Only apply this if content characteristics don't suggest otherwise
	tag := node.Data
	prefix := GetNamespacePrefix(tag)
	return knownInlineNamespacePrefixes[prefix]
}

// ShouldTreatAsBlockElement dynamically determines if an unknown/custom tag
// should be treated as a block-level element based on its structure and content.
// This enables proper handling of custom tag formats like SEC documents.
func ShouldTreatAsBlockElement(node *html.Node) bool {
	if node == nil || node.Type != html.ElementNode {
		return false
	}

	// Check if this is a namespaced tag (e.g., ix:nonnumeric, xbrl:value)
	// These require special handling as they're often inline data markers
	if IsNamespaceTag(node.Data) {
		// Use specialized logic for namespace tags based on context and content
		return !ShouldTreatNamespaceTagAsInline(node)
	}

	// Known inline elements should never be treated as block elements
	// This prevents bugs where long text in inline elements (like <font>)
	// triggers the text length heuristic
	if IsInlineElement(node.Data) {
		return false
	}

	// Analyze the node's structure and content
	hasElementChildren := false
	hasTextContent := false
	textLength := 0
	newlineCount := 0
	childCount := 0

	for child := node.FirstChild; child != nil; child = child.NextSibling {
		childCount++

		switch child.Type {
		case html.ElementNode:
			hasElementChildren = true
		case html.TextNode:
			text := strings.TrimSpace(child.Data)
			if text != "" {
				hasTextContent = true
				textLength += len(text)
				// Count newlines in original text (before trimming). Deliberately
				// INSIDE the text != "" guard, unlike
				// ShouldTreatNamespaceTagAsInline's loop: a block decision should
				// not hinge on line breaks that live only in whitespace-only text
				// nodes, while an inline decision treats any newline as
				// multi-line content.
				newlineCount += strings.Count(child.Data, "\n")
			}
		}
	}

	// Decision rules for treating as block element:

	// Rule 1: Container tags with multiple children are likely block-level
	if childCount > 1 || hasElementChildren {
		return true
	}

	// Rule 2: Tags with substantial text content are likely block-level
	// This catches custom tags that wrap meaningful content
	if hasTextContent && textLength > 50 {
		return true
	}

	// Rule 3: Tags containing multi-line text are likely block-level
	if newlineCount > 0 {
		return true
	}

	// Rule 4: Tags with uppercase names and hyphens (common in structured data formats like SEC)
	// Examples: <SEC-DOCUMENT>, <ACCEPTANCE-DATETIME>, <SEC-HEADER>
	tag := node.Data
	if isStructuredDataTag(tag) {
		return true
	}

	// Rule 5: Check if parent is a block element - children of blocks tend to be blocks
	// This handles nested structures
	if node.Parent != nil && node.Parent.Type == html.ElementNode {
		parentTag := node.Parent.Data
		if isStructuredDataTag(parentTag) {
			// Children of structured data tags are typically block-level
			return true
		}
	}

	return false
}

// isStructuredDataTag checks if a tag name matches patterns used in structured data formats.
// These patterns include:
//   - Tags with hyphens or underscores (sec-document, ACCEPTANCE_DATETIME)
//   - Long tag names suggesting metadata fields
//
// Note: HTML parser converts all tag names to lowercase, so we check for lowercase patterns
func isStructuredDataTag(tag string) bool {
	if tag == "" {
		return false
	}

	// Tags with hyphens or underscores are common in structured data formats
	// (HTML parser converts to lowercase, so we check for lowercase patterns)
	if strings.Contains(tag, "-") || strings.Contains(tag, "_") {
		return true
	}

	// Long tag names are typically metadata/structural fields
	if len(tag) > 8 {
		return true
	}

	return false
}
