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

	extractTextWithStructure(node, tb, imageCounter, linkCounter, tableFormat, nil, 0, false, false)
}

// ExtractChromeTextWithStructure extracts the visible text of a page-chrome
// region (a nav/header/aside/footer section node) into tb. It differs from
// ExtractTextWithStructureAndImages in two ways, both required by reader-mode
// section extraction:
//
//   - The section root itself is exempt from the skip check. The root is a
//     nav/header/footer/aside element, which the content pass's isSkipElement
//     predicate would reject before emitting anything.
//   - Descendants use the chrome skip policy: only raw-text elements
//     (script/style/noscript/template) are skipped. The content pass also
//     skips nested nav and out-of-article landmarks, but a header's <nav> menu
//     IS the region's substance and must survive.
//
// The [IMAGE:n]/[LINK:n] counters obey the same rules as the content pass, so
// a collector that prunes only raw-text subtrees (see the root package's
// extractImagesAndLinks chrome mode) stays in lockstep with the emitted tokens.
func ExtractChromeTextWithStructure(node *html.Node, tb *table.TrackedBuilder, imageCounter *int, linkCounter *int, tableFormat string) {
	if node == nil {
		return
	}
	// Children are visited with node as their parent block and depth 1,
	// matching how the content entry point recurses into the root's children.
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		extractTextWithStructure(child, tb, imageCounter, linkCounter, tableFormat, node, 1, false, true)
	}
}

func extractTextWithStructure(node *html.Node, tb *table.TrackedBuilder, imageCounter *int, linkCounter *int, tableFormat string, parentBlock *html.Node, depth int, inPre bool, chromeMode bool) {
	if node == nil {
		return
	}
	if chromeMode {
		// Chrome regions skip only raw-text subtrees; nested nav and landmark
		// elements are the region's content, not chrome-within-chrome.
		if node.Type == html.ElementNode && IsRawTextElement(node.Data) {
			return
		}
	} else if isSkipElement(node) {
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
				// With counters active the table's own text carries
				// [LINK:n]/[IMAGE:n] tokens (tableTokenText below), so table
				// links keep their URLs through the caller's formatInline*
				// substitution instead of collapsing to bare labels. The old
				// silent counter burn is gone — the cells themselves advance
				// the counters in the same document order the section-wide
				// collector numbered them.
				cellText := func(c *html.Node) string {
					return tableTokenText(c, imageCounter, linkCounter, chromeMode)
				}
				useTokens := imageCounter != nil || linkCounter != nil
				// A <caption> is visible rendered content, but the row-based
				// table extractor below collects only <tr> subtrees and this
				// dispatch returns without walking the subtree again —
				// without this, the caption text vanished from every output
				// format. Emit it as its own line ahead of the table
				// (pipe-table Markdown has no caption element). Token-aware
				// when counters are active so caption links survive too, and
				// their positions line up (a caption precedes the rows in DOM
				// order, matching the collector's numbering).
				captionEmitted := false
				for child := node.FirstChild; child != nil; child = child.NextSibling {
					if child.Type != html.ElementNode || child.Data != "caption" {
						continue
					}
					if !captionEmitted {
						captionEmitted = true
						var text string
						if useTokens {
							text = cellText(child)
						} else {
							text = GetTextContent(child)
						}
						if text != "" {
							if tb.Len() > 0 && tb.LastChar != '\n' {
								_ = tb.WriteByte('\n')
							}
							_, _ = tb.WriteString(text)
							_ = tb.WriteByte('\n')
						}
						continue
					}
					// Only the FIRST <caption> is the table's caption and is
					// rendered; a second one is a parse artifact the HTML
					// parser keeps as a sibling (the spec closes the first
					// caption on a second start tag). The section-wide
					// collector still numbers its anchors/images, so when
					// tokens are active walk it with the text discarded — the
					// same counter-burn idiom appendRowCells uses for
					// non-cell row children — or every later token in this
					// section misses by this caption's count.
					if useTokens {
						_ = cellText(child)
					}
				}
				// Use the table processor for table extraction
				if useTokens {
					TableProcessor().ExtractWithCellText(node, tb, tableFormat, cellText)
				} else {
					TableProcessor().Extract(node, tb, tableFormat)
				}
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
			extractTextWithStructure(child, tb, imageCounter, linkCounter, tableFormat, node, depth+1, childInPre, chromeMode)
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
			extractTextWithStructure(child, tb, imageCounter, linkCounter, tableFormat, parentBlock, depth+1, inPre, chromeMode)
		}
	}
}

// tableTokenText returns node's flattened visible text with [LINK:n]…[/LINK]
// and inline [IMAGE:n] placeholders, for table cells and captions on the
// token-emitting path: the section-wide formatInlineLinks/formatInlineImages
// pass runs over the table's rendered output too, so a cell anchor reaches
// the final text as a real link instead of a bare label.
//
// Alignment contract: it increments the counters for exactly the anchors the
// section-wide collector numbers — in chrome mode the collector prunes
// raw-text subtrees (IsRawTextElement, template included), so the emitter
// skips them too; in content mode the collector counts everything, and the
// emitter mirrors GetTextContent's skip set (script/style/noscript carry no
// anchor elements after parsing; template content is real parsed nodes and
// is included).
//
// Unlike the linear text pass, this flattener never emits newlines (a
// markdown table row is one line); whitespace between fragments collapses to
// single spaces.
func tableTokenText(node *html.Node, imageCounter, linkCounter *int, chromeMode bool) string {
	if node == nil {
		return ""
	}
	var b strings.Builder
	b.Grow(64)
	writeTokenText(&b, node, imageCounter, linkCounter, chromeMode)
	return strings.TrimSpace(b.String())
}

// writeTokenText is tableTokenText's recursive worker.
func writeTokenText(b *strings.Builder, node *html.Node, imageCounter, linkCounter *int, chromeMode bool) {
	switch node.Type {
	case html.TextNode:
		if text := strings.TrimSpace(node.Data); text != "" {
			ensureTokenSpace(b)
			b.WriteString(text)
		}
		return
	case html.CommentNode, html.DoctypeNode:
		return
	case html.ElementNode:
		switch node.Data {
		case "script", "style", "noscript":
			return
		case "template":
			// Chrome mode: the collector prunes it, so skip entirely.
			// Content mode: real parsed children — include, matching
			// GetTextContent and the unpruned collector.
			if chromeMode {
				return
			}
		case "a":
			if linkCounter != nil {
				*linkCounter++
				ensureTokenSpace(b)
				b.WriteString("[LINK:")
				b.WriteString(strconv.Itoa(*linkCounter))
				b.WriteString("]")
				for child := node.FirstChild; child != nil; child = child.NextSibling {
					writeTokenText(b, child, imageCounter, linkCounter, chromeMode)
				}
				b.WriteString("[/LINK]")
				return
			}
		case "img":
			if imageCounter != nil && imgHasValidSrc(node) {
				*imageCounter++
				ensureTokenSpace(b)
				b.WriteString("[IMAGE:")
				b.WriteString(strconv.Itoa(*imageCounter))
				b.WriteString("]")
				return
			}
			return
		case "br":
			ensureTokenSpace(b)
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			writeTokenText(b, child, imageCounter, linkCounter, chromeMode)
		}
	}
}

// ensureTokenSpace appends a single separating space when needed so flattened
// fragments never run together and never double-space.
func ensureTokenSpace(b *strings.Builder) {
	if b.Len() > 0 {
		last := b.String()[b.Len()-1]
		if last != ' ' {
			b.WriteByte(' ')
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
	return pruneDescendants(node, ShouldRemoveElement)
}

// RemoveHiddenElements removes hidden elements from node's subtree in place:
// the hidden attribute, aria-hidden="true", a display:none/visibility:hidden
// style, or a hidden class token (hidden, sr-only, d-none, ...). It is the
// light-weight cleaner for page-chrome sections in reader mode: CleanContentNode
// applies the content scorer's class removal, which would strip a nav region's
// own "nav-menu" children along with the ads, so chrome regions clear only
// what is invisible. The root node itself is never removed (callers decide its
// fate; see IsHiddenElement).
func RemoveHiddenElements(node *html.Node) *html.Node {
	return pruneDescendants(node, IsHiddenElement)
}

// pruneDescendants detaches every element child for which remove returns
// true, in place, and returns node. It is the single implementation of the
// detach-while-iterating walk CleanContentNode and RemoveHiddenElements
// share. Marked children are detached immediately — saving next BEFORE
// RemoveChild, which clears the child's sibling links — instead of being
// collected into a toRemove slice for a second pass; detached subtrees are
// never pushed, so their descendants are never visited. The root itself is
// never tested; callers decide its fate.
func pruneDescendants(node *html.Node, remove func(*html.Node) bool) *html.Node {
	if node == nil {
		return nil
	}

	// Use pooled stack to avoid allocation
	stackPtr := GetNodeSlice()
	defer PutNodeSlice(stackPtr)
	stack := *stackPtr

	stack = append(stack, node)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		for child := n.FirstChild; child != nil; {
			next := child.NextSibling
			if child.Type == html.ElementNode && remove(child) {
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
