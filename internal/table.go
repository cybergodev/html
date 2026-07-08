package internal

import (
	"strconv"
	"strings"

	"github.com/cybergodev/html/internal/table"
	"golang.org/x/net/html"
)

// htmlCellAccessor implements table.CellAccessor using the existing helper functions.
type htmlCellAccessor struct{}

// GetAlignment implements table.CellAccessor.
func (a *htmlCellAccessor) GetAlignment(node *html.Node) table.CellAlignment {
	if node == nil {
		return table.AlignDefault
	}
	return getCellAlign(node)
}

// GetColSpan implements table.CellAccessor.
func (a *htmlCellAccessor) GetColSpan(node *html.Node) int {
	if node == nil {
		return 1
	}
	return getColSpan(node)
}

// GetRowSpan implements table.CellAccessor.
func (a *htmlCellAccessor) GetRowSpan(node *html.Node) int {
	if node == nil {
		return 1
	}
	return getRowSpan(node)
}

// GetWidth implements table.CellAccessor.
func (a *htmlCellAccessor) GetWidth(node *html.Node) string {
	if node == nil {
		return ""
	}
	return getCellWidth(node)
}

// GetTextContent implements table.CellAccessor.
func (a *htmlCellAccessor) GetTextContent(node *html.Node) string {
	if node == nil {
		return ""
	}
	return GetTextContent(node)
}

// htmlNodeWalker implements table.NodeWalker using the existing WalkNodes function.
type htmlNodeWalker struct{}

// Walk implements table.NodeWalker.
func (w *htmlNodeWalker) Walk(node *html.Node, callback func(*html.Node) bool) {
	WalkNodes(node, callback)
}

// defaultTableAccessor is the default accessor instance for table extraction.
var defaultTableAccessor = &htmlCellAccessor{}

// defaultTableWalker is the default walker instance for table extraction.
var defaultTableWalker = &htmlNodeWalker{}

// TableProcessor returns the table processor with default accessor and walker.
func TableProcessor() *table.Processor {
	return table.NewProcessor(defaultTableAccessor, defaultTableWalker)
}

// containsWord checks if text contains word with proper boundary detection.
// This is used for parsing CSS style attributes to ensure we match complete
// property names (e.g., "text-align:center" not just "align:center").
func containsWord(text, word string) bool {
	return hasWordBoundary(text, word, boundaryCSS)
}

// getCellAlign extracts the alignment from a table cell node.
// It first checks the align attribute, then the style attribute for text-align.
// Optimized to use a single loop through attributes.
func getCellAlign(n *html.Node) table.CellAlignment {
	if n == nil {
		return table.AlignDefault
	}

	var styleAttr string

	// Single pass through attributes - collect style and check align
	for _, attr := range n.Attr {
		attrKey := strings.ToLower(attr.Key)
		switch attrKey {
		case "align":
			alignVal := strings.ToLower(strings.TrimSpace(attr.Val))
			switch alignVal {
			case "left":
				return table.AlignLeft
			case "center":
				return table.AlignCenter
			case "right":
				return table.AlignRight
			case "justify":
				return table.AlignJustify
			}
		case "style":
			styleAttr = attr.Val
		}
	}

	// Check style attribute for text-align (only if found)
	if styleAttr != "" {
		style := strings.ToLower(styleAttr)
		// Normalize spaces around colons: both "text-align : justify" and
		// "text-align: justify" collapse to "text-align:justify", so only the
		// colon-space-free forms below can ever match.
		normalizedStyle := strings.ReplaceAll(style, " :", ":")
		normalizedStyle = strings.ReplaceAll(normalizedStyle, ": ", ":")
		// Check for text-align patterns with better boundary detection.
		if containsWord(normalizedStyle, "text-align:justify") {
			return table.AlignJustify
		}
		if containsWord(normalizedStyle, "text-align:right") {
			return table.AlignRight
		}
		if containsWord(normalizedStyle, "text-align:center") {
			return table.AlignCenter
		}
		if containsWord(normalizedStyle, "text-align:left") {
			return table.AlignLeft
		}
	}

	return table.AlignDefault
}

// maxCellSpan caps colspan/rowspan values. The HTML spec clamps both to 1000
// (browsers ignore anything larger), and oversized values are almost always
// malformed; capping also bounds the placeholder cells expandColspanCells
// allocates (colspan-1 per cell), which would otherwise let a single
// <td colspan="N"> exhaust memory.
const maxCellSpan = 1000

// getColSpan extracts the colspan attribute value from a table cell.
// Returns 1 if no colspan attribute is present or if the value is invalid.
// Values above maxCellSpan are clamped to maxCellSpan.
func getColSpan(n *html.Node) int {
	if n == nil {
		return 1
	}
	for _, attr := range n.Attr {
		if strings.ToLower(attr.Key) == "colspan" {
			if val, err := strconv.Atoi(strings.TrimSpace(attr.Val)); err == nil && val > 0 {
				if val > maxCellSpan {
					return maxCellSpan
				}
				return val
			}
		}
	}
	return 1
}

// getRowSpan extracts the rowspan attribute value from a table cell.
// Returns 1 if no rowspan attribute is present or if the value is invalid.
// Values above maxCellSpan are clamped to maxCellSpan.
func getRowSpan(n *html.Node) int {
	if n == nil {
		return 1
	}
	for _, attr := range n.Attr {
		if strings.ToLower(attr.Key) == "rowspan" {
			if val, err := strconv.Atoi(strings.TrimSpace(attr.Val)); err == nil && val > 0 {
				if val > maxCellSpan {
					return maxCellSpan
				}
				return val
			}
		}
	}
	return 1
}

// isZeroWidthValue reports whether a width value represents zero width ("",
// "0", "0px", "0%", case-insensitive). Such values carry no usable column-width
// information and are dropped so structure-row detection and style emission
// treat the cell as widthless. Shared by the width-attribute and style-attribute
// branches of getCellWidth so both apply the same filter.
func isZeroWidthValue(s string) bool {
	switch strings.ToLower(s) {
	case "", "0", "0px", "0%":
		return true
	}
	return false
}

// getCellWidth extracts the width from a table cell node.
// It checks both the width attribute and the style attribute.
func getCellWidth(n *html.Node) string {
	if n == nil {
		return ""
	}
	// First check width attribute
	for _, attr := range n.Attr {
		if strings.ToLower(attr.Key) == "width" {
			widthVal := strings.TrimSpace(attr.Val)
			if !isZeroWidthValue(widthVal) {
				return widthVal
			}
		}
	}
	// Then check style attribute
	for _, attr := range n.Attr {
		if strings.ToLower(attr.Key) == "style" {
			style := attr.Val
			// Find "width:" case-insensitively with an index valid for the
			// original style string. Searching the original rather than a
			// strings.ToLower copy keeps the index correct even when lowercasing
			// would change byte length (some non-ASCII runes fold to a different
			// number of bytes), and preserves the value's original case.
			if idx := asciiFoldIndex(style, "width:"); idx >= 0 {
				start := idx + len("width:")
				for start < len(style) && (style[start] == ' ' || style[start] == '\t') {
					start++
				}
				end := start
				for end < len(style) {
					c := style[end]
					if c == ';' || c == '"' || c == '\'' || c == '}' {
						break
					}
					end++
				}
				widthVal := strings.TrimSpace(style[start:end])
				if !isZeroWidthValue(widthVal) {
					return widthVal
				}
			}
		}
	}
	return ""
}

// asciiFoldIndex returns the byte index of the first ASCII-case-insensitive
// occurrence of substr within s, or -1 if none. The returned index is valid for
// slicing the original s: unlike strings.Index on a strings.ToLower copy, it
// stays correct when lowercasing would change the byte length of s (some
// non-ASCII runes fold to a different number of bytes). substr must be non-empty
// and lowercase ASCII (the "width:" caller satisfies both).
func asciiFoldIndex(s, substr string) int {
	if len(substr) == 0 {
		return 0
	}
	for i := 0; i+len(substr) <= len(s); i++ {
		if asciiFoldHasPrefix(s[i:], substr) {
			return i
		}
	}
	return -1
}
