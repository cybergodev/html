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

// defaultTableProcessor is the singleton table processor with default accessor
// and walker. Processor holds only immutable references (the accessor and walker
// are stateless singletons), so a single shared instance is safe for concurrent
// use. Extracting a table previously allocated a new *table.Processor on every
// <table> element encountered; on table-heavy documents (financial reports,
// data pages) this is hundreds of tiny allocations per document.
var defaultTableProcessor = table.NewProcessor(defaultTableAccessor, defaultTableWalker)

// TableProcessor returns the table processor with default accessor and walker.
func TableProcessor() *table.Processor {
	return defaultTableProcessor
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

// getCellSpan extracts an integer span attribute (colspan or rowspan) from a
// table cell. Returns 1 when the attribute is absent or invalid. Values above
// maxCellSpan are clamped, matching the HTML spec's behavior.
func getCellSpan(n *html.Node, attrKey string) int {
	if n == nil {
		return 1
	}
	for _, attr := range n.Attr {
		if strings.ToLower(attr.Key) == attrKey {
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

// getColSpan extracts the colspan attribute value from a table cell.
func getColSpan(n *html.Node) int {
	return getCellSpan(n, "colspan")
}

// getRowSpan extracts the rowspan attribute value from a table cell.
func getRowSpan(n *html.Node) int {
	return getCellSpan(n, "rowspan")
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
// It checks both the width attribute and the style attribute in a single pass.
// The width attribute takes precedence over the style attribute's width property.
func getCellWidth(n *html.Node) string {
	if n == nil {
		return ""
	}
	// Scan attributes once: collect the first non-zero width attribute value
	// (preferred) and remember the style attribute for fallback. The width
	// attribute takes precedence over style per the HTML/CSS cascade for
	// presentational hints.
	var styleAttr string
	widthFound := false
	for _, attr := range n.Attr {
		switch strings.ToLower(attr.Key) {
		case "width":
			if widthFound {
				continue
			}
			widthVal := strings.TrimSpace(attr.Val)
			if !isZeroWidthValue(widthVal) {
				return widthVal
			}
		case "style":
			styleAttr = attr.Val
		}
	}
	// Check style attribute for width property
	if styleAttr != "" {
		// Find "width:" case-insensitively, requiring it to start at a CSS
		// boundary so it is not matched inside "border-width:", "max-width:",
		// or "min-width:". Searching the original rather than a strings.ToLower
		// copy keeps the index correct even when lowercasing would change byte
		// length (some non-ASCII runes fold to a different number of bytes),
		// and preserves the value's original case.
		if idx := asciiFoldIndexWord(styleAttr, "width:"); idx >= 0 {
			start := idx + len("width:")
			for start < len(styleAttr) && (styleAttr[start] == ' ' || styleAttr[start] == '\t') {
				start++
			}
			end := start
			for end < len(styleAttr) {
				c := styleAttr[end]
				if c == ';' || c == '"' || c == '\'' || c == '}' {
					break
				}
				end++
			}
			widthVal := strings.TrimSpace(styleAttr[start:end])
			if !isZeroWidthValue(widthVal) {
				return widthVal
			}
		}
	}
	return ""
}

// asciiFoldIndexWord returns the index of the first occurrence of substr in s
// (ASCII case-insensitive) that begins at a CSS word boundary — either at the
// start of s or immediately after a CSS boundary character. It mirrors the
// boundary discipline of hasWordBoundary but returns an index. getCellWidth
// uses it so "width:" is not matched inside "border-width:", "max-width:", or
// "min-width:" (which a plain substring search would catch), matching the
// boundary-aware behavior getCellAlign already uses for text-align.
func asciiFoldIndexWord(s, substr string) int {
	sl := len(substr)
	if sl == 0 {
		return 0
	}
	for i := 0; i+sl <= len(s); i++ {
		if !asciiFoldHasPrefix(s[i:], substr) {
			continue
		}
		if i > 0 && !isBoundaryChar(s[i-1], boundaryCSS) {
			continue
		}
		return i
	}
	return -1
}
