// extraction.go provides content extraction, table processing, and text manipulation
// functionality that is not part of the public API.
package internal

import (
	"strconv"
	"strings"

	"github.com/cybergodev/html/internal/table"
	"golang.org/x/net/html"
)

// writeInt writes a non-negative integer to the builder without allocating a string.
func writeInt(tb *table.TrackedBuilder, n int) {
	if n < 10 {
		_ = tb.WriteByte(byte('0' + n))
		return
	}
	var buf [20]byte
	tb.Write(strconv.AppendInt(buf[:0], int64(n), 10))
}

// ExtractTextWithStructureAndImages extracts text content from an HTML node tree
// while preserving document structure (headings, paragraphs, lists, tables). It
// writes into tb, a capacity-retaining TrackedBuilder the caller obtains from
// GetTrackedBuilder (and returns with PutTrackedBuilder) so the per-Extract
// document buffer is reused across calls instead of re-grown from zero each time.
func ExtractTextWithStructureAndImages(node *html.Node, tb *table.TrackedBuilder, imageCounter *int, linkCounter *int, tableFormat string) {
	if node == nil {
		return
	}
	if node.Type == html.ElementNode && IsNonContentElement(node.Data) {
		return
	}

	extractTextWithStructure(node, tb, imageCounter, linkCounter, tableFormat, nil, 0)
}

func extractTextWithStructure(node *html.Node, tb *table.TrackedBuilder, imageCounter *int, linkCounter *int, tableFormat string, parentBlock *html.Node, depth int) {
	if node == nil {
		return
	}
	if node.Type == html.ElementNode && IsNonContentElement(node.Data) {
		return
	}
	if node.Type == html.TextNode {
		// Single-pass text normalization: handles NBSP, entities, and line breaks
		textData := normalizeText(node.Data)

		// Check if we're inside an inline/namespace element
		isInsideInline := false
		if parentBlock != nil && parentBlock.Type == html.ElementNode {
			isInsideInline = IsInlineElement(parentBlock.Data) || IsNamespaceTag(parentBlock.Data)
		}

		if isInsideInline {
			// Inside inline elements, handle trailing space based on next sibling
			hasTrailingSpace := strings.HasSuffix(textData, " ") || strings.HasSuffix(textData, "\t")
			content := strings.TrimSpace(textData)
			if content != "" {
				tb.WriteString(content)
				// Preserve trailing space UNLESS next sibling is a namespace tag
				// Namespace tags (ix:*, xbrl:*, etc.) should be concatenated without spaces
				if hasTrailingSpace {
					shouldPreserveSpace := true
					if node.NextSibling != nil && node.NextSibling.Type == html.ElementNode {
						// Check if next sibling is a namespace tag
						nextTag := node.NextSibling.Data
						if IsNamespaceTag(nextTag) || IsKnownInlineNamespacePrefix(GetNamespacePrefix(nextTag)) {
							shouldPreserveSpace = false
						}
					}
					if shouldPreserveSpace {
						_ = tb.WriteByte(' ')
					}
				}
			}
		} else {
			// For regular text nodes, check for trailing space and preserve it
			hasTrailingSpace := strings.HasSuffix(textData, " ") || strings.HasSuffix(textData, "\t")
			content := strings.TrimSpace(textData)
			if content != "" {
				table.EnsureSpacing(tb, ' ')
				tb.WriteString(content)
				// Preserve trailing space from original HTML
				if hasTrailingSpace {
					_ = tb.WriteByte(' ')
				}
			}
		}
		return
	}
	if node.Type == html.ElementNode {
		if node.Data == "img" && imageCounter != nil {
			// Only emit a placeholder for imgs with a usable src, so the count
			// stays in lockstep with extractImagesAndLinks (which assigns
			// positions only to such imgs) and no unmatched [IMAGE:n] token can
			// leak into output.
			if !imgHasValidSrc(node) {
				return
			}
			*imageCounter++
			table.EnsureNewline(tb)
			tb.WriteString("[IMAGE:")
			writeInt(tb, *imageCounter)
			tb.WriteString("]\n")
			return
		}
		if node.Data == "a" && linkCounter != nil {
			*linkCounter++
			tb.WriteString("[LINK:")
			writeInt(tb, *linkCounter)
			tb.WriteString("]")
			// Continue processing children for link text
		}
		if node.Data == "br" {
			// BR creates a single line break, not paragraph spacing
			// Only add newline if we have content and don't already have one
			if tb.Len() > 0 && tb.LastChar != '\n' {
				_ = tb.WriteByte('\n')
			}
			return
		}
		if node.Data == "table" {
			if containsNestedTable(node) {
				// Layout table: this <table> wraps other <table> elements in its
				// cells purely for visual layout (common on financial sites such
				// as Finviz, where the whole page is nested layout tables).
				// Rendering it as a Markdown table would capture each outer cell
				// via GetTextContent, which recursively flattens every nested
				// data table into unstructured run-on text. Fall through to the
				// generic block handling below so each nested data table is
				// dispatched and rendered on its own terms.
			} else {
				// Use the table processor for table extraction
				TableProcessor().Extract(node, tb, tableFormat)
				return
			}
		}
		// Check if this is a paragraph-level block element that needs double newlines
		// Elements like li, br, hr, tr, td, th should not add extra spacing
		isParagraphBlock := IsParagraphLevelBlockElement(node.Data)

		// Structure-aware: for unknown tags, dynamically determine if they should be treated as block elements
		isBlockElement := IsBlockElement(node.Data)
		if !isBlockElement && !isParagraphBlock {
			isBlockElement = ShouldTreatAsBlockElement(node)
			// If dynamically determined to be a block, also treat as paragraph block
			if isBlockElement {
				isParagraphBlock = true
			}
		}

		startLen := tb.Len()
		if isBlockElement && startLen > 0 {
			table.EnsureNewline(tb)
			// Add Markdown list/indentation prefix (list markers for <li>,
			// padding-left based indentation for other indented blocks).
			if listPrefix := blockListPrefix(node); listPrefix != "" {
				tb.WriteString(listPrefix)
			}
			startLen = tb.Len()
		} else if isBlockElement && startLen == 0 {
			// First element - add list/indentation prefix if applicable.
			if listPrefix := blockListPrefix(node); listPrefix != "" {
				tb.WriteString(listPrefix)
				startLen = tb.Len()
			}
		}

		for child := node.FirstChild; child != nil; child = child.NextSibling {
			extractTextWithStructure(child, tb, imageCounter, linkCounter, tableFormat, node, depth+1)
		}
		// Add closing link tag after processing children
		if node.Data == "a" && linkCounter != nil {
			tb.WriteString("[/LINK]")
		}
		hasContent := tb.Len() > startLen
		if isBlockElement && hasContent {
			table.EnsureNewline(tb)
			// Add an extra newline for paragraph-level blocks to create paragraph spacing in Markdown
			if isParagraphBlock && tb.LastChar == '\n' {
				_ = tb.WriteByte('\n')
			}
		}
		// Add spacing for non-root inline elements (depth > 0)
		// This ensures proper spacing between inline elements at the same level
		if !isBlockElement && hasContent && node.NextSibling != nil && depth > 0 {
			table.EnsureSpacing(tb, ' ')
		}
	} else {
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			extractTextWithStructure(child, tb, imageCounter, linkCounter, tableFormat, parentBlock, depth+1)
		}
	}
}

// containsNestedTable reports whether tableNode has another <table> element
// anywhere in its subtree. Such tables are almost always used for visual
// layout — wrapping real data tables inside <td> cells — rather than carrying
// data themselves.
//
// The walk short-circuits as soon as a nested <table> is found: once `found`
// is set, the callback returns false for every subsequent node, which makes
// WalkNodes skip those subtrees, so the cost on layout tables is minimal. A
// genuine data table (no nested table) pays one full subtree walk, which is
// unavoidable to prove the absence of a nested table.
func containsNestedTable(tableNode *html.Node) bool {
	if tableNode == nil {
		return false
	}
	found := false
	WalkNodes(tableNode, func(n *html.Node) bool {
		if found {
			return false
		}
		if n != tableNode && n.Type == html.ElementNode && n.Data == "table" {
			found = true
			return false
		}
		return true
	})
	return found
}

// CleanContentNode removes non-content elements from the node tree.
// Uses iterative traversal with explicit stack to avoid potential stack overflow
// on deeply nested documents and improve cache locality.
func CleanContentNode(node *html.Node) *html.Node {
	if node == nil {
		return nil
	}

	toRemove := make([]*html.Node, 0, 8)

	// Use pooled stack to avoid allocation
	stackPtr := GetNodeSlice()
	defer PutNodeSlice(stackPtr)
	stack := *stackPtr

	stack = append(stack, node)

	// Iterative traversal using explicit stack
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if child.Type == html.ElementNode && ShouldRemoveElement(child) {
				toRemove = append(toRemove, child)
			} else {
				stack = append(stack, child)
			}
		}
	}

	// Remove marked nodes
	for _, n := range toRemove {
		if n.Parent != nil {
			n.Parent.RemoveChild(n)
		}
	}

	*stackPtr = stack
	return node
}
