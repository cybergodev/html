package internal

import (
	"strings"

	"golang.org/x/net/html"
)

// HTML5 inline elements - elements that should NOT add newlines or paragraph spacing.
// These elements flow with text on the same line.
var inlineElements = map[string]bool{
	// Text formatting (presentational)
	"font": true, "b": true, "i": true, "u": true, "s": true, "strike": true,
	"del": true, "ins": true, "strong": true, "em": true,
	"mark": true, "small": true, "sub": true, "sup": true,
	"big": true, "tt": true,

	// Semantic inline
	"span": true, "a": true, "code": true, "kbd": true, "samp": true,
	"var": true, "abbr": true, "cite": true, "q": true, "dfn": true,
	"time": true, "data": true, "ruby": true, "rt": true, "rp": true,
	"bdi": true, "wbr": true,

	// Media and embedded. canvas is intentionally NOT here: it is a block-level
	// element (see blockElements / IsParagraphLevelBlockElement) and surviving
	// sanitization, so classifying it inline too made IsInlineElement and
	// IsBlockElement both return true and caused the article scorer to skip
	// <canvas> subtrees (extract.go extractArticleNode) while extraction treated
	// them as blocks.
	"img": true, "svg": true, "picture": true,
	"video": true, "audio": true,
	"object": true, "embed": true, "iframe": true,
	"map": true,

	// Form controls
	"input": true, "button": true, "select": true,
	"textarea": true, "label": true, "output": true,

	// Line break (special inline)
	"br": true,

	// Metadata (should not affect layout)
	"script": true, "style": true, "link": true, "meta": true, "title": true,
}

// HTML5 block elements - elements that should add newlines and paragraph spacing.
// Organized by category for better maintainability.
var blockElements = map[string]bool{
	// Text containers
	"p": true, "div": true, "pre": true, "blockquote": true,

	// Headings
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,

	// Semantic HTML5 sections (high priority)
	"article": true, "section": true, "main": true, "nav": true, "aside": true,
	"header": true, "footer": true, "figure": true, "figcaption": true,

	// Lists
	"ul": true, "ol": true, "li": true, "dl": true, "dt": true, "dd": true,

	// Tables
	"table": true, "thead": true, "tbody": true, "tfoot": true, "tr": true, "td": true, "th": true,

	// Forms
	"form": true, "fieldset": true,

	// Interactive elements
	"details": true, "summary": true, "dialog": true,

	// Other block elements
	"hr": true, "address": true,

	// Structural elements (low priority, rarely appear in content extraction)
	"body": true, "html": true, "head": true,

	// Deprecated elements
	"center": true,

	// Media/Interactive elements
	"canvas": true,
}

// nonContentTags contains tags that are typically not part of the main content.
// Note: <form> is intentionally excluded. Server-side frameworks (ASP.NET
// WebForms, JSF, JSP) wrap the entire page body in a single <form>; marking it
// non-content would cause ShouldRemove/CleanContentNode and the text extractor
// to drop the whole page body.
var nonContentTags = map[string]bool{
	"script": true, "style": true, "noscript": true, "nav": true,
	"aside": true, "footer": true, "header": true,
}

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

// IsBlockElement returns true if the tag is a known block-level element.
func IsBlockElement(tag string) bool {
	return blockElements[tag]
}

// IsInlineElement returns true if the tag is a known inline element.
// Inline elements should not add newlines or paragraph spacing.
func IsInlineElement(tag string) bool {
	return inlineElements[tag]
}

// IsNonContentElement returns true if the tag is typically not part of main content.
func IsNonContentElement(tag string) bool {
	return nonContentTags[tag]
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
			// Count newlines in original text (before trimming)
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
				// Count newlines in original text (before trimming)
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
