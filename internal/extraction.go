// extraction.go provides content extraction, table processing, and text manipulation
// functionality that is not part of the public API.
package internal

import (
	"strconv"
	"strings"

	"github.com/cybergodev/html/internal/table"
	"golang.org/x/net/html"
)

// TrackedBuilder Write* methods satisfy io.Writer but always return nil errors (plain byte-slice append); the discards in this file are deliberate.
// writeInt writes a non-negative integer to the builder without allocating a string.
func writeInt(tb *table.TrackedBuilder, n int) {
	if n < 10 {
		_ = tb.WriteByte(byte('0' + n))
		return
	}
	var buf [20]byte
	_, _ = tb.Write(strconv.AppendInt(buf[:0], int64(n), 10))
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
	if isSkipElement(node) {
		return
	}

	extractTextWithStructure(node, tb, imageCounter, linkCounter, tableFormat, nil, 0, false)
}

func extractTextWithStructure(node *html.Node, tb *table.TrackedBuilder, imageCounter *int, linkCounter *int, tableFormat string, parentBlock *html.Node, depth int, inPre bool) {
	if node == nil {
		return
	}
	if isSkipElement(node) {
		return
	}
	if node.Type == html.TextNode {
		if inPre {
			// Inside <pre> (and its <code> subtree) newlines and internal
			// whitespace are structural — code blocks and ASCII diagrams. Write
			// the text verbatim so line breaks survive; only NBSP is folded to a
			// regular space (a raw U+00A0 would be visually misleading in code).
			// CleanTextBytes later keeps the line structure while trimming each
			// line's trailing spaces.
			if data := strings.ReplaceAll(node.Data, nbspString, " "); data != "" {
				_, _ = tb.WriteString(data)
			}
			return
		}
		// A whitespace-only text node (the newlines and indentation between
		// pretty-printed tags) contributes nothing downstream: normalization
		// maps it to ASCII spaces, which the TrimSpace below reduces to "", so
		// neither branch writes anything. TrimSpace treats NBSP (U+00A0) as
		// whitespace, matching every byte the normalizer could produce from
		// such input, so checking it first is behavior-identical and skips
		// normalizeText's pooled-buffer pass plus its fresh string per node —
		// on pretty-printed pages that is one allocation per inter-tag gap.
		if strings.TrimSpace(node.Data) == "" {
			return
		}

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
				_, _ = tb.WriteString(content)
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
				_, _ = tb.WriteString(content)
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
			_, _ = tb.WriteString("[IMAGE:")
			writeInt(tb, *imageCounter)
			_, _ = tb.WriteString("]\n")
			return
		}
		if node.Data == "a" && linkCounter != nil {
			*linkCounter++
			_, _ = tb.WriteString("[LINK:")
			writeInt(tb, *linkCounter)
			_, _ = tb.WriteString("]")
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
			if isLayoutTable(node) {
				// Layout table: this <table> wraps other <table> elements in its
				// cells purely for visual layout (common on financial sites such
				// as Finviz, where the whole page is nested layout tables).
				// Rendering it as a Markdown table would capture each outer cell
				// via GetTextContent, which recursively flattens every nested
				// data table into unstructured run-on text. Fall through to the
				// generic block handling below so each nested data table is
				// dispatched and rendered on its own terms.
			} else {
				// Burn placeholder positions for the subtree the table
				// processor is about to consume. extractImagesAndLinks (root
				// package) numbers every valid-src <img> and every <a> in the
				// whole document, table cells included; the dispatch below
				// returns without walking this table's children, so without
				// these silent increments the counters fall out of lockstep
				// and every [IMAGE:n]/[LINK:n] after the table would be
				// substituted with the wrong entry.
				if imageCounter != nil || linkCounter != nil {
					WalkNodes(node, func(c *html.Node) bool {
						if c.Type != html.ElementNode {
							return true
						}
						switch c.Data {
						case "img":
							if imageCounter != nil && imgHasValidSrc(c) {
								*imageCounter++
							}
						case "a":
							if linkCounter != nil {
								*linkCounter++
							}
						}
						return true
					})
				}
				// A <caption> is visible rendered content, but the row-based
				// table extractor below collects only <tr> subtrees and this
				// dispatch returns without walking the subtree again —
				// without this, the caption text vanished from every output
				// format. Emit it as its own line ahead of the table
				// (pipe-table Markdown has no caption element).
				for child := node.FirstChild; child != nil; child = child.NextSibling {
					if child.Type == html.ElementNode && child.Data == "caption" {
						if text := GetTextContent(child); text != "" {
							if tb.Len() > 0 && tb.LastChar != '\n' {
								_ = tb.WriteByte('\n')
							}
							_, _ = tb.WriteString(text)
							_ = tb.WriteByte('\n')
						}
						break
					}
				}
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
				_, _ = tb.WriteString(listPrefix)
			}
			startLen = tb.Len()
		} else if isBlockElement && startLen == 0 {
			// First element - add list/indentation prefix if applicable.
			if listPrefix := blockListPrefix(node); listPrefix != "" {
				_, _ = tb.WriteString(listPrefix)
				startLen = tb.Len()
			}
		}

		childInPre := inPre || node.Data == "pre"
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			extractTextWithStructure(child, tb, imageCounter, linkCounter, tableFormat, node, depth+1, childInPre)
		}
		// Add closing link tag after processing children
		if node.Data == "a" && linkCounter != nil {
			_, _ = tb.WriteString("[/LINK]")
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
			extractTextWithStructure(child, tb, imageCounter, linkCounter, tableFormat, parentBlock, depth+1, inPre)
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

// isLayoutTable reports whether tableNode should be treated as a visual-layout
// shell rather than a data table.
//
// Two independent signals:
//
//   - an explicit ARIA role of presentation/none declares the table structure
//     presentational (screen readers skip it), which is the strongest signal a
//     page can give — it applies with or without nested tables;
//   - a nested <table> AND no meaningful direct text of its own (outside the
//     nested tables): a cell that carries its own prose next to an embedded
//     table is a genuine data table and must still render.
//
// Legacy presentational attributes (border="0", cellpadding, cellspacing) are
// deliberately NOT signals: borderless styling is the modern default for real
// data tables, so those attributes cannot distinguish layout from data.
func isLayoutTable(tableNode *html.Node) bool {
	if hasPresentationalRole(tableNode) {
		return true
	}
	if !containsNestedTable(tableNode) {
		return false
	}
	// Collect the direct text of tableNode that lies OUTSIDE any nested <table>
	// subtree. If there is a non-trivial amount, the outer table is data.
	var directText int
	WalkNodes(tableNode, func(n *html.Node) bool {
		if n != tableNode && n.Type == html.ElementNode && n.Data == "table" {
			return false // skip nested table subtrees
		}
		if n.Type == html.TextNode {
			directText += len(strings.TrimSpace(n.Data))
		}
		return true
	})
	return directText < layoutTableDirectTextThreshold
}

// hasPresentationalRole reports whether node carries role="presentation" or
// role="none" (case-insensitive), the ARIA declaration that an element's
// structure is purely visual.
func hasPresentationalRole(node *html.Node) bool {
	for _, attr := range node.Attr {
		if attr.Key != "role" {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(attr.Val)) {
		case "presentation", "none":
			return true
		}
	}
	return false
}

// CleanContentNode removes non-content elements from the node tree.
// Uses iterative traversal with explicit stack to avoid potential stack overflow
// on deeply nested documents and improve cache locality.
func CleanContentNode(node *html.Node) *html.Node {
	if node == nil {
		return nil
	}

	// Use pooled stack to avoid allocation
	stackPtr := GetNodeSlice()
	defer PutNodeSlice(stackPtr)
	stack := *stackPtr

	stack = append(stack, node)

	// Iterative traversal using explicit stack. Marked children are detached
	// immediately (saving next before RemoveChild, which clears the child's
	// sibling links) instead of being collected into a toRemove slice for a
	// second pass — that slice was one allocation per call. Detached subtrees
	// are not pushed, so their descendants are never visited; the deferred
	// removal loop this replaces achieved the same effect only incidentally,
	// by removing nodes whose parents lived in already-detached subtrees.
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		for child := n.FirstChild; child != nil; {
			next := child.NextSibling
			if child.Type == html.ElementNode && ShouldRemoveElement(child) {
				n.RemoveChild(child)
			} else {
				stack = append(stack, child)
			}
			child = next
		}
	}

	*stackPtr = stack
	return node
}
