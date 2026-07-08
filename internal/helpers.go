package internal

import (
	htmlstd "html"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// Pre-compiled regex patterns for text processing.
// These are compile-time constants and will panic on initialization if invalid.
var (
	paddingLeftRegex = regexp.MustCompile(`padding-left:\s*(\d+(?:\.\d+)?)\s*pt`)
)

// Clean text and builder sizing constants are now in constants.go

// boundaryCharSets defines different character sets for word boundary detection
type boundaryCharSet int

const (
	// boundaryStandard uses standard word boundary characters (-, _, space, tab)
	boundaryStandard boundaryCharSet = iota
	// boundaryCSS uses CSS-specific boundary characters (;, :, space, tab, {, }, ")
	boundaryCSS
)

// hasWordBoundary checks if a pattern appears with proper word boundaries.
// The allowed boundary characters are determined by the boundaryCharSet parameter.
func hasWordBoundary(text, pattern string, charSet boundaryCharSet) bool {
	idx := strings.Index(text, pattern)
	if idx == -1 {
		return false
	}

	// Check character before the match
	if idx > 0 {
		before := text[idx-1]
		if !isBoundaryChar(before, charSet) {
			return false
		}
	}

	// Check character after the match
	endIdx := idx + len(pattern)
	if endIdx < len(text) {
		after := text[endIdx]
		if !isBoundaryChar(after, charSet) {
			return false
		}
	}

	return true
}

// isBoundaryChar checks if a character is a valid boundary character
func isBoundaryChar(c byte, charSet boundaryCharSet) bool {
	switch charSet {
	case boundaryCSS:
		return c == ';' || c == ':' || c == ' ' || c == '\t' ||
			c == '{' || c == '}' || c == '"'
	case boundaryStandard:
		return c == '-' || c == '_' || c == ' ' || c == '\t'
	default:
		return false
	}
}

// normalizeNonBreakingSpaces replaces all non-breaking spaces (\u00a0) with regular spaces.
// This ensures consistent text processing across different HTML sources.
// Optimized with early exit to avoid allocation when no NBSP is present.
func normalizeNonBreakingSpaces(s string) string {
	// Fast path: early exit if no non-breaking spaces present
	// This avoids the allocation overhead of strings.ReplaceAll for common case
	if !strings.Contains(s, "\u00a0") {
		return s
	}
	return strings.ReplaceAll(s, "\u00a0", " ")
}

// textNeedsNormalization reports whether s contains any byte sequence that
// GetTextContent's normalization pass would rewrite: a newline/CR, an '&'
// (potential entity), or an NBSP (UTF-8 0xC2 0xA0). When it returns false, the
// text passes through unchanged apart from edge trimming, so GetTextContent can
// skip its pooled buffer and tree walk entirely.
func textNeedsNormalization(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\n', '\r', '&':
			return true
		case 0xC2:
			if i+1 < len(s) && s[i+1] == 0xA0 {
				return true
			}
		}
	}
	return false
}

// normalizeText performs text normalization in a single pass.
// It handles NBSP replacement, line break normalization, and HTML entity replacement.
// This is more efficient than calling the individual functions sequentially.
func normalizeText(s string) string {
	if len(s) == 0 {
		return s
	}

	// Single scan to detect what processing is needed
	n := len(s)
	hasNBSP := false
	hasNewline := false
	hasAmpersand := false
	firstMod := -1

	for i := 0; i < n; i++ {
		c := s[i]
		switch {
		case c == '\n' || c == '\r':
			if firstMod == -1 {
				firstMod = i
			}
			hasNewline = true
		case c == '&':
			if firstMod == -1 {
				firstMod = i
			}
			hasAmpersand = true
		case c == 0xC2 && i+1 < n && s[i+1] == 0xA0:
			// UTF-8 encoding of NBSP (U+00A0)
			if firstMod == -1 {
				firstMod = i
			}
			hasNBSP = true
		}
	}

	// Fast path: no processing needed
	if firstMod == -1 {
		return s
	}

	// If only ampersands present (no NBSP or newlines), delegate to entity replacement
	if hasAmpersand && !hasNBSP && !hasNewline {
		return ReplaceHTMLEntities(s)
	}

	// Single-pass processing for NBSP and newlines
	sb := GetBuilder()
	defer PutBuilder(sb)
	sb.Grow(n)

	// Copy unchanged prefix
	if firstMod > 0 {
		sb.WriteString(s[:firstMod])
	}

	// Process from first modification point
	i := firstMod
	for i < n {
		c := s[i]
		switch {
		case c == '\n':
			sb.WriteByte(' ')
			i++
		case c == '\r':
			// Skip carriage returns
			i++
		case c == 0xC2 && i+1 < n && s[i+1] == 0xA0:
			// UTF-8 encoding of NBSP (U+00A0) - replace with space
			sb.WriteByte(' ')
			i += 2
		case c == '&':
			// Handle entity at this position
			replaced, consumed := replaceEntityAt(s, i)
			sb.WriteString(replaced)
			i += consumed
		default:
			sb.WriteByte(c)
			i++
		}
	}

	return sb.String()
}

// replaceEntityAt handles an HTML entity starting at position pos.
// Returns the replacement string and the number of bytes consumed.
func replaceEntityAt(text string, pos int) (string, int) {
	textLen := len(text)
	if pos >= textLen || text[pos] != '&' {
		return "&", 1
	}

	// Find the end of the entity (semicolon or end of string)
	end := pos + 1
	if end >= textLen {
		return "&", 1
	}

	// Check for common entities first (most frequent case)
	remainingLen := textLen - pos
	switch {
	case remainingLen >= 5 && text[pos:pos+5] == "&amp;":
		return "&", 5
	case remainingLen >= 6 && text[pos:pos+6] == "&nbsp;":
		return " ", 6
	case remainingLen >= 4 && text[pos:pos+4] == "&lt;":
		return "<", 4
	case remainingLen >= 4 && text[pos:pos+4] == "&gt;":
		return ">", 4
	case remainingLen >= 6 && text[pos:pos+6] == "&quot;":
		return "\"", 6
	case remainingLen >= 6 && text[pos:pos+6] == "&apos;":
		return "'", 6
	case remainingLen >= 6 && text[pos:pos+6] == "&copy;":
		return "©", 6
	case remainingLen >= 5 && text[pos:pos+5] == "&reg;":
		return "®", 5
	case remainingLen >= 7 && text[pos:pos+7] == "&mdash;":
		return "—", 7
	case remainingLen >= 7 && text[pos:pos+7] == "&ndash;":
		return "–", 7
	}

	// Check for numeric entity
	if text[end] == '#' {
		return replaceNumericEntity(text, pos)
	}

	// Find semicolon for named entity
	semi := strings.IndexByte(text[pos:], ';')
	if semi == -1 {
		return "&", 1
	}
	semi += pos

	// Extract entity name
	entityName := text[pos+1 : semi]
	if !isValidEntityName(entityName) {
		return "&", 1
	}

	// Use standard library for other entities
	decoded := htmlstd.UnescapeString(text[pos : semi+1])
	return decoded, semi - pos + 1
}

var unwantedCharReplacer = strings.NewReplacer(
	"☒", "[X]",
	"☐", "[ ]",
	"☑", "[X]",
)

func CleanText(text string, whitespaceRegex *regexp.Regexp) string {
	if text == "" {
		return ""
	}

	// Fast path: check if processing is needed
	n := len(text)
	hasNewlines := false
	hasMultipleSpaces := false
	hasNBSP := false
	hasUnwanted := false
	hasAmpersand := false
	prevSpace := false

	for i := 0; i < n; i++ {
		c := text[i]
		switch {
		case c == '\n':
			hasNewlines = true
		case c == '\t':
			hasMultipleSpaces = true
		case c == ' ':
			if prevSpace {
				hasMultipleSpaces = true
			}
			prevSpace = true
			continue
		case c == 0xC2 && i+1 < n && text[i+1] == 0xA0:
			hasNBSP = true
		case c == '&':
			hasAmpersand = true
		case c == 0xE2 && i+2 < n:
			if text[i+1] == 0x98 && (text[i+2] == 0x92 || text[i+2] == 0x90 || text[i+2] == 0x91) {
				hasUnwanted = true
			}
		}
		prevSpace = false
	}

	if !hasNewlines && !hasMultipleSpaces && !hasNBSP && !hasUnwanted {
		if hasAmpersand {
			return ReplaceHTMLEntities(text)
		}
		return text
	}

	sb := GetBuilder()
	defer PutBuilder(sb)

	if n > builderPoolInitialCapacity {
		sb.Grow(n)
	}
	start := 0
	previousWasEmpty := false

	for i := 0; i <= n; i++ {
		if i == n || text[i] == '\n' {
			rawLine := text[start:i]
			isEmpty := true

			if rawLine != "" {
				firstNonSpace := 0
				lineLen := len(rawLine)
				for firstNonSpace < lineLen && rawLine[firstNonSpace] == ' ' {
					firstNonSpace++
				}

				var indent, contentPart string
				if firstNonSpace < lineLen {
					indent = rawLine[:firstNonSpace]
					contentPart = rawLine[firstNonSpace:]
				} else {
					contentPart = rawLine
				}

				contentLen := len(contentPart)
				if contentLen > 0 {
					// Scan for compression need
					needsCompress := false
					prevSp := false
					for j := 0; j < contentLen; j++ {
						c := contentPart[j]
						if c == '\t' {
							needsCompress = true
							break
						}
						if c == ' ' && prevSp {
							needsCompress = true
							break
						}
						prevSp = c == ' '
					}

					// Trim trailing spaces/tabs
					contentEnd := contentLen
					for contentEnd > 0 && (contentPart[contentEnd-1] == ' ' || contentPart[contentEnd-1] == '\t') {
						contentEnd--
					}

					if contentEnd > 0 {
						if sb.Len() > 0 {
							if previousWasEmpty {
								sb.WriteByte('\n')
							}
							sb.WriteByte('\n')
						}
						sb.WriteString(indent)
						if needsCompress {
							inSpace := false
							for j := 0; j < contentEnd; j++ {
								c := contentPart[j]
								if c == ' ' || c == '\t' {
									if !inSpace {
										sb.WriteByte(' ')
										inSpace = true
									}
								} else {
									sb.WriteByte(c)
									inSpace = false
								}
							}
						} else {
							sb.WriteString(contentPart[:contentEnd])
						}
						isEmpty = false
					}
				}
			}

			previousWasEmpty = isEmpty
			start = i + 1
		}
	}

	result := sb.String()

	if hasUnwanted {
		result = unwantedCharReplacer.Replace(result)
	}
	if hasAmpersand {
		return ReplaceHTMLEntities(result)
	}
	return result
}

// maxWalkDepth limits the maximum traversal depth to prevent memory exhaustion
// from deeply nested or malformed HTML documents.
// SECURITY: This limit prevents potential DoS attacks through deeply nested structures.
const maxWalkDepth = 50000

// WalkNodes traverses the HTML node tree iteratively using an explicit stack
// to avoid potential stack overflow on deeply nested documents.
// The fn callback is called for each node. If fn returns false, traversal
// stops for that branch (node's children are not visited).
// Optimized with pooled stack slice to reduce allocations.
//
// SECURITY: Traversal is limited to maxWalkDepth (50,000) nodes to prevent
// memory exhaustion attacks through deeply nested or recursive structures.
// If the limit is exceeded, traversal stops early without notification.
// For applications that need to know if traversal was truncated, use WalkNodesWithTruncation.
func WalkNodes(node *html.Node, fn func(*html.Node) bool) {
	_, _ = WalkNodesWithTruncation(node, fn)
}

// WalkNodesWithTruncation traverses the HTML node tree iteratively using an explicit stack
// to avoid potential stack overflow on deeply nested documents.
// The fn callback is called for each node. If fn returns false, traversal
// stops for that branch (node's children are not visited).
//
// Returns:
//   - truncated: true if traversal was stopped due to exceeding maxWalkDepth limit
//   - visited: the number of nodes visited before completion or truncation
//
// SECURITY: Traversal is limited to maxWalkDepth (50,000) nodes to prevent
// memory exhaustion attacks through deeply nested or recursive structures.
//
// Optimized with pooled stack slice to reduce allocations.
func WalkNodesWithTruncation(node *html.Node, fn func(*html.Node) bool) (truncated bool, visited int) {
	if node == nil || fn == nil {
		return false, 0
	}

	// Use pooled stack to avoid allocation
	stackPtr := GetNodeSlice()
	defer PutNodeSlice(stackPtr)
	stack := *stackPtr

	stack = append(stack, node)

	// SECURITY: Track visited count to detect potential infinite loops
	// or extremely deep structures
	visitedCount := 0

	for len(stack) > 0 {
		// SECURITY: Check depth limit to prevent memory exhaustion
		visitedCount++
		if visitedCount > maxWalkDepth {
			// Stop traversal to prevent memory exhaustion
			// Update the pointer for pool return before returning
			*stackPtr = stack
			return true, visitedCount - 1 // Return truncation status
		}

		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if !fn(n) {
			continue
		}

		// Push children so the first child is processed next. The child list is a
		// singly-linked list (FirstChild -> NextSibling), so append them in document
		// order to the end of the stack, then reverse just that segment in place.
		// This yields correct document order when popped without an intermediate
		// buffer allocation.
		segStart := len(stack)
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			stack = append(stack, child)
		}
		for i, j := segStart, len(stack)-1; i < j; i, j = i+1, j-1 {
			stack[i], stack[j] = stack[j], stack[i]
		}
	}

	// Update the pointer for pool return
	*stackPtr = stack
	return false, visitedCount
}

func FindElementByTag(doc *html.Node, tagName string) *html.Node {
	var result *html.Node
	WalkNodes(doc, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == tagName {
			result = n
			return false
		}
		return true
	})
	return result
}

func GetTextContent(node *html.Node) string {
	if node == nil {
		return ""
	}

	// Fast path: an element whose only child is a single text node that needs no
	// normalization (no newline, '&', or NBSP) yields just the trimmed text — no
	// tree walk, pooled buffer, or closure. GetTextContent is the hottest
	// allocator in extraction (invoked once per <a>, table cell, and title
	// candidate), and the common <a>link</a> / <td>cell</td> shape hits this path,
	// skipping a closure heap-escape and two pool round-trips per call with zero
	// allocations. Output is identical to the general path for this subtree shape.
	if node.Type == html.ElementNode {
		if c := node.FirstChild; c != nil && c.NextSibling == nil && c.Type == html.TextNode {
			if data := c.Data; !textNeedsNormalization(data) {
				return strings.TrimSpace(data)
			}
		}
	}

	// Use a capacity-retaining pooled []byte rather than BuilderPool: a pooled
	// []byte keeps its backing array across calls (reset with [:0]), whereas
	// strings.Builder.Reset() drops the buffer, forcing a fresh allocation on
	// every call. GetTextContent is invoked once per <a>/table cell, so this is
	// a hot allocation point.
	bp := GetByteBuf()
	defer PutByteBuf(bp)

	prevEndedWithSpace := false

	WalkNodes(node, func(n *html.Node) bool {
		if n.Type == html.TextNode {
			data := n.Data
			dataLen := len(data)
			if dataLen == 0 {
				return true
			}

			// Single-pass: trim, normalize NBSP+newlines, replace entities
			// Find first non-whitespace and last non-whitespace in one scan
			// Whitespace includes: space, tab, newline, CR, and NBSP (0xC2 0xA0)
			start := 0
			for start < dataLen {
				c := data[start]
				if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
					start++
				} else if c == 0xC2 && start+1 < dataLen && data[start+1] == 0xA0 {
					start += 2
				} else {
					break
				}
			}
			if start >= dataLen {
				prevEndedWithSpace = true
				return true
			}
			end := dataLen - 1
			for end > start {
				c := data[end]
				if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
					end--
				} else if c == 0xA0 && end > start && data[end-1] == 0xC2 {
					end -= 2
				} else {
					break
				}
			}

			// Track leading/trailing spaces for inter-node spacing
			startedWithSpace := start > 0
			endedWithSpace := end < dataLen-1

			// Extract trimmed content and apply normalizations in one pass
			trimmed := data[start : end+1]

			// Fast check: does the trimmed content need any processing?
			hasNBSP := false
			hasNewline := false
			hasAmp := false
			for i := 0; i < len(trimmed); i++ {
				c := trimmed[i]
				if c == '\n' || c == '\r' {
					hasNewline = true
				} else if c == '&' {
					hasAmp = true
				} else if c == 0xC2 && i+1 < len(trimmed) && trimmed[i+1] == 0xA0 {
					hasNBSP = true
				}
			}

			var text string
			if !hasNBSP && !hasNewline && !hasAmp {
				text = trimmed
			} else {
				// Build normalized text in a pooled scratch buffer.
				nb := GetByteBuf()
				for i := 0; i < len(trimmed); i++ {
					c := trimmed[i]
					switch {
					case c == '\n' || c == '\r':
						*nb = append(*nb, ' ')
					case c == 0xC2 && i+1 < len(trimmed) && trimmed[i+1] == 0xA0:
						*nb = append(*nb, ' ')
						i++
					case c == '&':
						replaced, consumed := replaceEntityAt(trimmed, i)
						*nb = append(*nb, replaced...)
						i += consumed - 1
					default:
						*nb = append(*nb, c)
					}
				}
				text = string(*nb)
				PutByteBuf(nb)
			}

			if text != "" {
				needsSpace := prevEndedWithSpace
				if !needsSpace && len(*bp) > 0 {
					needsSpace = startedWithSpace
				}
				if len(*bp) > 0 && needsSpace {
					*bp = append(*bp, ' ')
				}
				*bp = append(*bp, text...)
			}
			prevEndedWithSpace = endedWithSpace
		}
		return true
	})
	return string(*bp)
}

func GetTextLength(node *html.Node) int {
	length := 0
	WalkNodes(node, func(n *html.Node) bool {
		if n.Type == html.TextNode {
			length += len(strings.TrimSpace(normalizeText(n.Data)))
		}
		return true
	})
	return length
}

func GetLinkDensity(node *html.Node) float64 {
	if node == nil {
		return 0.0
	}

	textLength := 0
	linkTextLength := 0

	WalkNodes(node, func(n *html.Node) bool {
		if n.Type == html.TextNode {
			length := len(strings.TrimSpace(normalizeText(n.Data)))
			textLength += length
			for parent := n.Parent; parent != nil; parent = parent.Parent {
				if parent.Type == html.ElementNode && parent.Data == "a" {
					linkTextLength += length
					break
				}
			}
		}
		return true
	})

	if textLength == 0 {
		return 0.0
	}
	return float64(linkTextLength) / float64(textLength)
}

var entityReplacer = strings.NewReplacer(
	// Note: Common entities (&amp;, &nbsp;, &lt;, &gt;, &quot;, &apos;, &copy;, &reg;, &mdash;, &ndash;)
	// are handled in fastReplaceCommonEntities() for better performance.

	// Remaining typographic entities
	"&hellip;", "…",
	"&trade;", "™",
	// Currency symbols
	"&euro;", "€",
	"&pound;", "£",
	"&cent;", "¢",
	"&yen;", "¥",
	"&curren;", "¤",
	// Mathematical symbols
	"&sect;", "§",
	"&para;", "¶",
	"&plusmn;", "±",
	"&times;", "×",
	"&divide;", "÷",
	"&frac12;", "½",
	"&frac14;", "¼",
	"&frac34;", "¾",
	"&deg;", "°",
	"&prime;", "'",
	"&Prime;", "\"",
	"&sup1;", "¹",
	"&sup2;", "²",
	"&sup3;", "³",
	// Additional common entities
	"&middot;", "·",
	"&bull;", "•",
	"&rsquo;", "'",
	"&lsquo;", "'",
	"&rdquo;", "\"",
	"&ldquo;", "\"",
	"&sbquo;", "‚",
	"&bdquo;", "„",
	"&dagger;", "†",
	"&Dagger;", "‡",
	"&permil;", "‰",
	"&micro;", "µ",
)

// ReplaceHTMLEntities replaces HTML entities with their corresponding characters.
// It handles both named entities (like &amp;, &nbsp;) and numeric entities (like &#65;, &#x41;).
// For unknown entities, it falls back to the standard library's html.UnescapeString.
// Optimized with a fast path for the most common entities.
func ReplaceHTMLEntities(text string) string {
	if !strings.ContainsRune(text, '&') {
		return text
	}

	// Fast path: handle the 10 most common entities directly
	// This avoids the overhead of strings.NewReplacer for the majority case
	result := fastReplaceCommonEntities(text)
	if result != text {
		// If we replaced entities, still need to handle numeric ones
		return replaceHTMLEntitiesFull(result)
	}

	// Slow path: an '&' is present but no common entity matched. Every entry in
	// entityReplacer is of the form "&name;", so it cannot match when the text
	// contains no ';'. A lone '&' is extremely common in extracted text (e.g.
	// "Tom & Jerry"), and strings.NewReplacer allocates a full copy even when it
	// replaces nothing — guarding on ';' eliminated ~8.6% of all allocations.
	// When a ';' is present the original behavior is preserved; either way
	// replaceHTMLEntitiesFull (which always runs below) decodes every entity.
	if strings.IndexByte(text, ';') != -1 {
		text = entityReplacer.Replace(text)
	}
	return replaceHTMLEntitiesFull(text)
}

// fastReplaceCommonEntities handles the 10 most common HTML entities with direct scanning.
// This is significantly faster than strings.NewReplacer for these common cases.
// Returns the input string unchanged if no common entities were found.
// Optimized with single-pass detection to avoid multiple scans.
func fastReplaceCommonEntities(text string) string {
	textLen := len(text)

	// Single scan to find first ampersand AND check for common entities
	// This merges two separate loops into one for better cache locality
	firstAmpersand := -1
	hasCommonEntity := false

	for i := 0; i < textLen; i++ {
		if text[i] == '&' {
			if firstAmpersand == -1 {
				firstAmpersand = i
			}
			// Immediately check for common entity at this position
			if !hasCommonEntity {
				remLen := textLen - i
				switch {
				case remLen >= 5 && text[i:i+5] == "&amp;":
					hasCommonEntity = true
				case remLen >= 6 && text[i:i+6] == "&nbsp;":
					hasCommonEntity = true
				case remLen >= 4 && text[i:i+4] == "&lt;":
					hasCommonEntity = true
				case remLen >= 4 && text[i:i+4] == "&gt;":
					hasCommonEntity = true
				case remLen >= 6 && text[i:i+6] == "&quot;":
					hasCommonEntity = true
				case remLen >= 6 && text[i:i+6] == "&apos;":
					hasCommonEntity = true
				case remLen >= 6 && text[i:i+6] == "&copy;":
					hasCommonEntity = true
				case remLen >= 5 && text[i:i+5] == "&reg;":
					hasCommonEntity = true
				case remLen >= 7 && text[i:i+7] == "&mdash;":
					hasCommonEntity = true
				case remLen >= 7 && text[i:i+7] == "&ndash;":
					hasCommonEntity = true
				}
			}
		}
	}

	// Fast path: no ampersands means no entities possible
	if firstAmpersand == -1 {
		return text
	}

	// Fast path: ampersands present but no common entities
	if !hasCommonEntity {
		return text
	}

	// Use pooled builder for better memory efficiency
	sb := GetBuilder()
	defer PutBuilder(sb)

	sb.Grow(textLen)

	// Write prefix unchanged
	if firstAmpersand > 0 {
		sb.WriteString(text[:firstAmpersand])
	}

	i := firstAmpersand
	for i < textLen {
		if text[i] != '&' {
			sb.WriteByte(text[i])
			i++
			continue
		}

		// Check if we have at least 4 characters for the shortest entity (&lt;)
		remainingLen := textLen - i
		if remainingLen < 4 {
			sb.WriteByte(text[i])
			i++
			continue
		}

		// Fast switch for common entities (ordered by frequency)
		// Each case checks bounds before slicing to prevent panic
		switch {
		case remainingLen >= 5 && text[i:i+5] == "&amp;":
			sb.WriteByte('&')
			i += 5
		case remainingLen >= 6 && text[i:i+6] == "&nbsp;":
			sb.WriteByte(' ')
			i += 6
		case remainingLen >= 4 && text[i:i+4] == "&lt;":
			sb.WriteByte('<')
			i += 4
		case remainingLen >= 4 && text[i:i+4] == "&gt;":
			sb.WriteByte('>')
			i += 4
		case remainingLen >= 6 && text[i:i+6] == "&quot;":
			sb.WriteByte('"')
			i += 6
		case remainingLen >= 6 && text[i:i+6] == "&apos;":
			sb.WriteByte('\'')
			i += 6
		case remainingLen >= 6 && text[i:i+6] == "&copy;":
			sb.WriteString("©")
			i += 6
		case remainingLen >= 5 && text[i:i+5] == "&reg;":
			sb.WriteString("®")
			i += 5
		case remainingLen >= 7 && text[i:i+7] == "&mdash;":
			sb.WriteString("—")
			i += 7
		case remainingLen >= 7 && text[i:i+7] == "&ndash;":
			sb.WriteString("–")
			i += 7
		default:
			// Not a common entity, copy as-is
			sb.WriteByte(text[i])
			i++
		}
	}

	return sb.String()
}

// replaceHTMLEntitiesFull handles numeric entities and unknown named entities.
func replaceHTMLEntitiesFull(text string) string {
	// Use pooled builder for better memory efficiency
	sb := GetBuilder()
	defer PutBuilder(sb)

	sb.Grow(len(text))

	i := 0
	for i < len(text) {
		if text[i] != '&' {
			sb.WriteByte(text[i])
			i++
			continue
		}

		// Find the end of the entity (semicolon or end of string)
		end := i + 1
		if end >= len(text) {
			sb.WriteByte(text[i])
			break
		}

		// Check if this is a numeric entity
		if text[end] == '#' {
			replaced, consumed := replaceNumericEntity(text, i)
			sb.WriteString(replaced)
			i += consumed
			continue
		}

		// For non-numeric entities, find the semicolon
		semi := strings.IndexByte(text[i:], ';')
		if semi == -1 {
			// No semicolon found, write the '&' and continue
			sb.WriteByte(text[i])
			i++
			continue
		}
		semi += i

		// Extract entity name
		entityName := text[i+1 : semi]

		// Validate entity name (alphanumeric only)
		if !isValidEntityName(entityName) {
			sb.WriteByte(text[i])
			i++
			continue
		}

		// Try to decode using standard library for unknown entities
		// This handles HTML5 named entities not in our replacer
		decoded := decodeEntityFallback("&" + entityName + ";")
		sb.WriteString(decoded)
		i = semi + 1
	}

	return sb.String()
}

// replaceNumericEntity handles numeric character references like &#65; or &#x41;
// SECURITY: Includes validation to prevent DoS and injection attacks through
// malformed or malicious numeric entities.
func replaceNumericEntity(text string, start int) (string, int) {
	if start+2 >= len(text) || text[start+1] != '#' {
		return string(text[start]), 1
	}

	// Find the semicolon
	semi := strings.IndexByte(text[start:], ';')
	if semi == -1 {
		return string(text[start]), 1
	}
	semi += start

	entity := text[start+2 : semi]
	if len(entity) == 0 {
		return text[start : semi+1], semi - start + 1
	}

	// Security: limit entity length to prevent DoS through extremely long numeric strings.
	// Maximum valid Unicode code point is 0x10FFFF which requires at most 8 decimal digits
	// or 6 hexadecimal digits. We allow up to 10 characters to handle hex prefix + digits.
	const maxEntityLength = 10
	if len(entity) > maxEntityLength {
		return text[start : semi+1], semi - start + 1
	}

	var base int
	if entity[0] == 'x' || entity[0] == 'X' {
		base = 16
		entity = entity[1:]
		// SECURITY: Validate that remaining characters are valid hex digits
		if len(entity) == 0 {
			return text[start : semi+1], semi - start + 1
		}
		for _, c := range entity {
			if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
				return text[start : semi+1], semi - start + 1
			}
		}
	} else {
		base = 10
		// SECURITY: Validate that all characters are valid decimal digits
		for _, c := range entity {
			if c < '0' || c > '9' {
				return text[start : semi+1], semi - start + 1
			}
		}
	}

	// Parse the number with 64-bit to prevent overflow
	num, err := strconv.ParseInt(entity, base, 64)
	if err != nil || num < 0 || num > 0x10FFFF {
		// Invalid numeric entity, return as-is
		return text[start : semi+1], semi - start + 1
	}

	// Check for surrogate pairs and invalid Unicode code points
	if num >= 0xD800 && num <= 0xDFFF {
		// Surrogate pair, not valid as a standalone character
		return "\uFFFD", semi - start + 1
	}

	// Convert to rune and validate it's a valid Unicode code point
	r := rune(num)
	if !utf8.ValidRune(r) {
		return "\uFFFD", semi - start + 1
	}

	// Special handling: convert non-breaking space (0xa0) to regular space (0x20)
	// This ensures consistent behavior with named entity &nbsp; which maps to regular space
	if num == 0xA0 {
		return " ", semi - start + 1
	}

	// Valid Unicode character
	return string(r), semi - start + 1
}

// isValidEntityName checks if an entity name contains only valid characters.
func isValidEntityName(name string) bool {
	if len(name) == 0 {
		return false
	}
	for _, c := range name {
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}

// decodeEntityFallback attempts to decode an entity using the standard library.
// This serves as a fallback for HTML5 named entities not in our fast replacer.
func decodeEntityFallback(entity string) string {
	// The standard library handles all HTML5 named entities
	decoded := htmlstd.UnescapeString(entity)
	if decoded == entity {
		// Entity was not recognized, return as-is
		return entity
	}
	return decoded
}

// IsValidURL checks if a URL is valid and safe for processing.
// This is a centralized URL validation function with size limits for security.
func IsValidURL(url string) bool {
	urlLen := len(url)
	if urlLen == 0 || urlLen > MaxURLLength {
		return false
	}

	// Special handling for data URLs - stricter validation with size limit
	if strings.HasPrefix(url, "data:") {
		if urlLen > MaxDataURILength {
			return false
		}
		for i := 5; i < urlLen; i++ {
			b := url[i]
			if b < 32 || b > 126 || b == '<' || b == '>' || b == '"' || b == '\'' || b == '\\' {
				return false
			}
		}
		return true
	}

	// Validate non-data URLs: check for dangerous characters
	for i := 0; i < urlLen; i++ {
		b := url[i]
		if b < 32 || b == 127 || b == '<' || b == '>' || b == '"' || b == '\'' {
			return false
		}
	}

	// Check for dangerous protocol-relative URL patterns
	// Block //javascript:, //vbscript:, etc.
	// Also handle whitespace before dangerous schemes (e.g., // javascript:)
	if strings.HasPrefix(url, "//") {
		// Trim leading whitespace to prevent bypass attempts
		lowerRest := strings.ToLower(strings.TrimLeft(url[2:], " \t\n\r"))
		if strings.HasPrefix(lowerRest, "javascript:") ||
			strings.HasPrefix(lowerRest, "vbscript:") ||
			strings.HasPrefix(lowerRest, "data:") ||
			strings.HasPrefix(lowerRest, "file:") {
			return false
		}
		return true
	}

	// Accept absolute URLs
	if strings.HasPrefix(url, "https://") || strings.HasPrefix(url, "http://") {
		return true
	}

	// At this point, urlLen > 0 (verified by the urlLen == 0 check at function start)
	// Accept relative URLs and paths (starting with / or .)
	// But reject path traversal patterns
	firstChar := url[0]
	if firstChar == '/' {
		// Block path traversal attempts like /\/, /../
		if urlLen > 1 && (url[1] == '\\' || (url[1] == '.' && (urlLen == 2 || url[2] == '.' || url[2] == '/'))) {
			return false
		}
		return true
	}
	if firstChar == '.' {
		// Block directory traversal
		if strings.HasPrefix(url, "./.") || strings.HasPrefix(url, "../") {
			return false
		}
		return true
	}

	// Accept alphanumeric paths (legitimate filenames like img1.jpg, video.mp4)
	// but reject paths starting with special characters that might be used in injection attacks
	if (firstChar >= 'a' && firstChar <= 'z') ||
		(firstChar >= 'A' && firstChar <= 'Z') ||
		(firstChar >= '0' && firstChar <= '9') {
		return true
	}

	return false
}

// imgHasValidSrc reports whether an <img> node carries a src attribute whose
// value is a valid URL. It mirrors the src validation in parseImageNode
// (extract.go): any src whose value fails IsValidURL yields false, and a node
// with no src at all yields false. The text-extraction pass uses this to decide
// whether to emit an [IMAGE:n] placeholder, keeping its count in lockstep with
// extractImagesAndLinks (which assigns positions only to imgs that
// parseImageNode accepts) so unmatched placeholders cannot leak into output.
func imgHasValidSrc(n *html.Node) bool {
	if n == nil {
		return false
	}
	hasSrc := false
	for _, attr := range n.Attr {
		if attr.Key == "src" {
			if !IsValidURL(attr.Val) {
				return false
			}
			hasSrc = true
		}
	}
	return hasSrc
}

func SelectBestCandidate(candidates map[*html.Node]int) *html.Node {
	var bestNode *html.Node
	bestScore := -1

	for node, score := range candidates {
		if score > bestScore {
			bestScore = score
			bestNode = node
		}
	}
	return bestNode
}

// extractPaddingLeft extracts the padding-left value from an HTML element's style attribute.
// It parses the CSS style attribute and returns the padding-left value in points (pt).
// Returns 0 if padding-left is not found or cannot be parsed.
//
// Examples:
//   - "padding-left:18pt" → 18
//   - "padding-left:63pt;" → 63
//   - "padding-left: 1.5em" → 0 (only pt is supported)
//   - "" → 0
func extractPaddingLeft(node *html.Node) int {
	if node == nil || node.Type != html.ElementNode {
		return 0
	}

	// Get the style attribute
	var styleAttr string
	for _, attr := range node.Attr {
		if attr.Key == "style" {
			styleAttr = attr.Val
			break
		}
	}

	if styleAttr == "" {
		return 0
	}

	matches := paddingLeftRegex.FindStringSubmatch(styleAttr)
	if len(matches) < 2 {
		return 0
	}

	// Parse the numeric value
	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0
	}

	return int(value)
}

// getIndentLevel returns the indentation level (0-3) based on padding-left value.
func getIndentLevel(paddingLeft int) int {
	switch {
	case paddingLeft <= 18:
		return 0
	case paddingLeft <= 40:
		return 1
	case paddingLeft <= 80:
		return 2
	default:
		return 3
	}
}

// getListPrefix returns the Markdown list prefix based on padding-left value.
// This converts CSS padding-left to Markdown list nesting format.
// Example:
//   - 0-18pt   → "" (no prefix, root level)
//   - 19-40pt  → "  - " (level 1, 2 spaces indent)
//   - 41-80pt  → "    - " (level 2, 4 spaces indent)
//   - >80pt    → "      - " (level 3, 6 spaces indent)
func getListPrefix(paddingLeft int) string {
	level := getIndentLevel(paddingLeft)
	switch level {
	case 0:
		return "" // Root level, no bullet
	case 1:
		return "  - " // Level 1: 2 spaces + "- "
	case 2:
		return "    - " // Level 2: 4 spaces + "- "
	case 3:
		return "      - " // Level 3: 6 spaces + "- "
	default:
		return ""
	}
}

// listItemPrefix returns the Markdown marker for a <li> node derived from its
// DOM context rather than CSS padding: "- " for items inside <ul> and "N. "
// (1-based) for items inside <ol>, indented two spaces per enclosing list for
// nesting. Returns "" if node is not a <li> or has no list ancestor.
//
// HTML lists typically rely on the browser's default <ul>/<ol> styling and
// carry no inline padding-left, so getListPrefix alone emitted no marker and
// consecutive items collapsed into a single Markdown paragraph. Using the list
// structure fixes that.
func listItemPrefix(node *html.Node) string {
	if node == nil || node.Type != html.ElementNode || node.Data != "li" {
		return ""
	}

	// Count enclosing lists; the nearest one determines the marker kind.
	depth := 0
	var listParent *html.Node
	for p := node.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && (p.Data == "ul" || p.Data == "ol") {
			depth++
			if listParent == nil {
				listParent = p
			}
		}
	}
	if depth == 0 {
		return "" // malformed <li> with no list ancestor
	}

	indent := strings.Repeat("  ", depth-1)
	if listParent.Data == "ol" {
		// 1-based ordinal among preceding <li> siblings of the same list.
		index := 1
		for sib := listParent.FirstChild; sib != nil; sib = sib.NextSibling {
			if sib == node {
				break
			}
			if sib.Type == html.ElementNode && sib.Data == "li" {
				index++
			}
		}
		return indent + strconv.Itoa(index) + ". "
	}
	return indent + "- "
}

// definitionPrefix returns the Markdown definition marker for a <dd> node:
// ": " indented two spaces per enclosing <dl> for nesting. Returns "" if node
// is not a <dd> or has no <dl> ancestor.
//
// HTML definition lists rely on browser default styling and carry no inline
// padding, so each <dd> is marked explicitly to keep terms and definitions
// visually distinct and prevent them from collapsing into one paragraph.
func definitionPrefix(node *html.Node) string {
	if node == nil || node.Type != html.ElementNode || node.Data != "dd" {
		return ""
	}
	depth := 0
	for p := node.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && p.Data == "dl" {
			depth++
		}
	}
	if depth == 0 {
		return "" // malformed <dd> with no <dl> ancestor
	}
	return strings.Repeat("  ", depth-1) + ": "
}

// blockListPrefix returns the Markdown prefix (marker + indentation) to emit
// before a block element's content. <li> nodes use DOM-based list markers,
// <dd> nodes use the definition marker; other indented blocks fall back to
// the padding-left heuristic. Returns "" when no prefix applies.
func blockListPrefix(node *html.Node) string {
	if node == nil || node.Type != html.ElementNode {
		return ""
	}
	switch node.Data {
	case "li":
		return listItemPrefix(node)
	case "dd":
		return definitionPrefix(node)
	}
	paddingLeft := extractPaddingLeft(node)
	if paddingLeft > 0 {
		return getListPrefix(paddingLeft)
	}
	return ""
}
