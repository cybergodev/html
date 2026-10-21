package internal

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// Pre-compiled regex patterns for text processing.
// These are compile-time constants and will panic on initialization if invalid.
var (
	paddingLeftRegex = regexp.MustCompile(`padding-left:\s*(\d+(?:\.\d+)?)\s*pt`)
)

// boundaryCharSets defines different character sets for word boundary detection
type boundaryCharSet int

const (
	// boundaryStandard uses standard word boundary characters (-, _, space, tab)
	// and digits (0-9). Digit boundaries let patterns match CSS tokens with
	// numeric suffixes such as menu3, nav2, content1.
	boundaryStandard boundaryCharSet = iota
	// boundaryCSS uses CSS-specific boundary characters (;, :, space, tab, {, }, ")
	boundaryCSS
)

// hasWordBoundary checks if a pattern appears with proper word boundaries.
// The allowed boundary characters are determined by the boundaryCharSet parameter.
// It scans ALL occurrences of pattern in text, returning true if any occurrence
// has valid boundary characters (or string edges) on both sides. The previous
// implementation checked only the first occurrence, which missed valid matches
// when an earlier non-bounded occurrence existed — e.g. "submenu level-3-menu"
// never matched "menu" because strings.Index found "menu" inside "submenu"
// (no boundary before it) and stopped, ignoring the valid "menu" in "level-3-menu".
func hasWordBoundary(text, pattern string, charSet boundaryCharSet) bool {
	textLen := len(text)
	patLen := len(pattern)
	if patLen == 0 || patLen > textLen {
		return false
	}

	searchStart := 0
	for searchStart <= textLen-patLen {
		idx := strings.Index(text[searchStart:], pattern)
		if idx == -1 {
			return false
		}
		idx += searchStart

		// Check character before the match
		beforeOK := idx == 0
		if !beforeOK {
			beforeOK = isBoundaryChar(text[idx-1], charSet)
		}

		// Check character after the match
		endIdx := idx + patLen
		afterOK := endIdx >= textLen
		if !afterOK {
			afterOK = isBoundaryChar(text[endIdx], charSet)
		}

		if beforeOK && afterOK {
			return true
		}

		// Move past this occurrence to check the next one
		searchStart = idx + 1
	}
	return false
}

// hasWordBoundaryAt reports whether pattern occurs at exactly text[i:] with a
// valid boundary character (or string edge) on both sides of that occurrence.
// It is the positional building block for the single-pass pattern scans: a
// caller iterating over every position i tests the same (position, pattern)
// pairs that hasWordBoundary locates via repeated substring search, so a
// boolean "any occurrence is bounded" computed with this helper is identical to
// hasWordBoundary's result.
func hasWordBoundaryAt(text string, i int, pattern string) bool {
	patLen := len(pattern)
	if patLen == 0 || len(text)-i < patLen {
		return false
	}
	if text[i:i+patLen] != pattern {
		return false
	}
	if i > 0 && !isBoundaryChar(text[i-1], boundaryStandard) {
		return false
	}
	if end := i + patLen; end < len(text) && !isBoundaryChar(text[end], boundaryStandard) {
		return false
	}
	return true
}

// isBoundaryChar checks if a character is a valid boundary character.
// For boundaryStandard, digits (0-9) are treated as boundary characters in
// addition to -, _, space, and tab. CSS class/id naming conventions frequently
// append numeric suffixes to semantic tokens (menu3, nav2, sidebar2, content1),
// and these should still match their base patterns. Without digits as boundaries,
// "nv-menu3-container" escapes the "menu" removal pattern even though it is
// unmistakably a navigation menu element.
func isBoundaryChar(c byte, charSet boundaryCharSet) bool {
	switch charSet {
	case boundaryCSS:
		return c == ';' || c == ':' || c == ' ' || c == '\t' ||
			c == '{' || c == '}' || c == '"'
	case boundaryStandard:
		return c == '-' || c == '_' || c == ' ' || c == '\t' ||
			(c >= '0' && c <= '9')
	default:
		return false
	}
}

// trimASCIISpaceAndNBSP trims leading/trailing ASCII whitespace (' ', '\t',
// '\n', '\r') and NBSP (UTF-8 0xC2 0xA0) from s — the exact trim class of
// GetTextContent's general path. The fast path previously used
// strings.TrimSpace, whose wider Unicode White_Space set (\v, \f, U+2028, …)
// made single-child subtrees trim differently than the same text reached
// through the general path.
func trimASCIISpaceAndNBSP(s string) string {
	start := 0
	for start < len(s) {
		c := s[start]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			start++
		} else if c == 0xC2 && start+1 < len(s) && s[start+1] == 0xA0 {
			start += 2
		} else {
			break
		}
	}
	end := len(s)
	for end > start {
		c := s[end-1]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			end--
		} else if c == 0xA0 && end-1 > start && s[end-2] == 0xC2 {
			end -= 2
		} else {
			break
		}
	}
	return s[start:end]
}

// textNeedsNormalization reports whether s contains any byte sequence that
// GetTextContent's normalization pass would rewrite: a newline/CR or an NBSP
// (UTF-8 0xC2 0xA0). '&' is not a trigger: the parser already decoded
// entities once, so the normalization pass never rewrites them. When it
// returns false, the text passes through unchanged apart from edge trimming,
// so GetTextContent can skip its pooled buffer and tree walk entirely.
func textNeedsNormalization(s string) bool {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\n', '\r':
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
// It handles NBSP replacement and line break normalization. HTML entity
// decoding is deliberately NOT performed: every caller feeds text that
// golang.org/x/net/html already decoded once during tokenization, so a
// remaining "&name;" sequence is visible page content (a page showing
// "&amp;lt;"), not an escape to unwrap — decoding here stripped a second
// escape level the source never had.
// This is more efficient than calling the individual functions sequentially.
func normalizeText(s string) string {
	if len(s) == 0 {
		return s
	}

	// First-modification scan via vectorized stdlib searches: firstMod is the
	// earliest position of any trigger — a newline/CR or an NBSP pair (whose
	// trigger position is the start of its 2-byte UTF-8 sequence). The former
	// byte-at-a-time loop was a profiled cost on text-heavy documents. '&' is
	// not a trigger (see the doc comment).
	n := len(s)
	nlIdx := strings.IndexByte(s, '\n')
	crIdx := strings.IndexByte(s, '\r')
	nbspIdx := strings.Index(s, nbspString)
	firstMod := -1
	for _, idx := range [...]int{nlIdx, crIdx, nbspIdx} {
		if idx >= 0 && (firstMod == -1 || idx < firstMod) {
			firstMod = idx
		}
	}

	// Fast path: no processing needed
	if firstMod == -1 {
		return s
	}

	// Single-pass processing for NBSP and newlines.
	// Use a capacity-retaining pooled []byte instead of BuilderPool: a pooled
	// []byte keeps its backing array across calls (reset with [:0]), whereas
	// strings.Builder.Reset() drops the buffer, forcing re-growth on every call.
	bp := GetByteBuf()
	defer PutByteBuf(bp)

	// Copy unchanged prefix
	if firstMod > 0 {
		*bp = append(*bp, s[:firstMod]...)
	}

	// Process from first modification point
	i := firstMod
	for i < n {
		c := s[i]
		switch {
		case c == '\n':
			*bp = append(*bp, ' ')
			i++
		case c == '\r':
			// Skip carriage returns
			i++
		case c == 0xC2 && i+1 < n && s[i+1] == 0xA0:
			// UTF-8 encoding of NBSP (U+00A0) - replace with space
			*bp = append(*bp, ' ')
			i += 2
		default:
			*bp = append(*bp, c)
			i++
		}
	}

	return string(*bp)
}

var unwantedCharReplacer = strings.NewReplacer(
	"☒", "[X]",
	"☐", "[ ]",
	"☑", "[X]",
)

// nbspString and nbspBytes are the UTF-8 encoding of NBSP (U+00A0) — the
// bytes 0xC2 0xA0 — spelled with the \u00a0 escape because a raw U+00A0 in
// source is visually indistinguishable from a plain space. (An ASCII "00a0"
// literal in the scorer shipped exactly that class of bug.) nbspString backs
// the string scans (normalizeText); nbspBytes backs the []byte scans in
// CleanTextBytes.
const nbspString = "\u00a0"

var nbspBytes = []byte(nbspString)

// CleanTextBytes cleans extracted text and returns it as a new string. It
// never modifies b and never aliases it in the returned string — the result is
// always a fresh allocation — so callers may hand in a buffer (e.g. a pooled
// TrackedBuilder's Bytes) that is recycled immediately after the call. The
// byte signature is what lets builder-backed callers skip an intermediate
// String() copy: that copy was ~4% of total allocation bytes on a profiled
// realistic document.
func CleanTextBytes(text []byte) string {
	if len(text) == 0 {
		return ""
	}

	// Classification scan: each flag is a whole-text property expressible as a
	// vectorized stdlib search, so the former byte-at-a-time loop is replaced by
	// a handful of SIMD-backed scans (the loop was a profiled hotspot at document
	// scale). hasMultipleSpaces matches the loop it replaces exactly: a tab
	// anywhere, or two adjacent spaces — the loop's prevSpace tracked only the
	// immediately preceding space, so "a  b" and "x\ty" are precisely the shapes
	// that set it.
	hasNewlines := bytes.IndexByte(text, '\n') >= 0
	hasTabs := bytes.IndexByte(text, '\t') >= 0
	hasMultipleSpaces := hasTabs || bytes.Contains(text, []byte("  "))
	hasNBSP := bytes.Contains(text, nbspBytes)
	hasUnwanted := bytes.Contains(text, []byte("☒")) ||
		bytes.Contains(text, []byte("☐")) ||
		bytes.Contains(text, []byte("☑"))
	n := len(text)

	if !hasNewlines && !hasMultipleSpaces && !hasNBSP && !hasUnwanted {
		// Single-line text with no compression triggers (a newline would be
		// one). The slow path trims each line's trailing spaces/tabs; do the
		// same here so trailing-space behavior does not depend on the presence
		// of unrelated triggers like double spaces.
		return string(bytes.TrimRight(text, " \t"))
	}

	// When the classification pass found no tab and no adjacent spaces, no line
	// below can need compression either — the per-line scans are skipped.
	anyCompressible := hasMultipleSpaces

	// Normalize NBSP (U+00A0, UTF-8 0xC2 0xA0) to a regular space, matching
	// normalizeText and GetTextContent. hasNBSP routed us here, but the body
	// below only matches ' ' and '\t', so without this the NBSP bytes passed
	// through verbatim — inconsistent with the other normalizers.
	if hasNBSP {
		text = bytes.ReplaceAll(text, nbspBytes, []byte(" "))
		n = len(text)
		// The NBSP→space rewrite can create new adjacent-space pairs (NBSP
		// followed by space becomes two spaces), so re-evaluate
		// anyCompressible on the rewritten text.
		anyCompressible = anyCompressible || bytes.Contains(text, []byte("  "))
	}

	// Use a capacity-retaining pooled []byte instead of BuilderPool: a pooled
	// []byte keeps its backing array across calls (reset with [:0]), whereas
	// strings.Builder.Reset() drops the buffer, forcing re-growth on every call.
	bp := GetByteBuf()
	defer PutByteBuf(bp)

	start := 0
	previousWasEmpty := false

	for i := 0; i <= n; i++ {
		if i == n || text[i] == '\n' {
			rawLine := text[start:i]
			isEmpty := true

			if len(rawLine) != 0 {
				firstNonSpace := 0
				lineLen := len(rawLine)
				for firstNonSpace < lineLen && rawLine[firstNonSpace] == ' ' {
					firstNonSpace++
				}

				var indent, contentPart []byte
				if firstNonSpace < lineLen {
					indent = rawLine[:firstNonSpace]
					contentPart = rawLine[firstNonSpace:]
				} else {
					contentPart = rawLine
				}

				contentLen := len(contentPart)
				if contentLen > 0 {
					// Scan for compression need. Skipped entirely when the
					// classification pass proved no tab or space pair exists
					// anywhere in the text (anyCompressible).
					needsCompress := false
					if anyCompressible {
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
					}

					// Trim trailing spaces/tabs
					contentEnd := contentLen
					for contentEnd > 0 && (contentPart[contentEnd-1] == ' ' || contentPart[contentEnd-1] == '\t') {
						contentEnd--
					}

					if contentEnd > 0 {
						if len(*bp) > 0 {
							if previousWasEmpty {
								*bp = append(*bp, '\n')
							}
							*bp = append(*bp, '\n')
						}
						*bp = append(*bp, indent...)
						if needsCompress {
							inSpace := false
							for j := 0; j < contentEnd; j++ {
								c := contentPart[j]
								if c == ' ' || c == '\t' {
									if !inSpace {
										*bp = append(*bp, ' ')
										inSpace = true
									}
								} else {
									*bp = append(*bp, c)
									inSpace = false
								}
							}
						} else {
							*bp = append(*bp, contentPart[:contentEnd]...)
						}
						isEmpty = false
					}
				}
			}

			previousWasEmpty = isEmpty
			start = i + 1
		}
	}

	result := string(*bp)

	if hasUnwanted {
		result = unwantedCharReplacer.Replace(result)
	}
	// Entity decoding is intentionally not performed: both production callers
	// feed text the HTML parser already decoded once, and a second decode
	// would strip an escape level that is visible page content (see
	// normalizeText).
	return result
}

// WalkNodes traverses the HTML node tree iteratively using an explicit stack
// to avoid potential stack overflow on deeply nested documents.
// The fn callback is called for each node. If fn returns false, traversal
// stops for that branch (node's children are not visited).
// Optimized with pooled stack slice to reduce allocations.
//
// The walk is uncapped and visits every node. It used to stop silently at
// 50,000 visited nodes, which lost data with no error and no way for a caller
// to notice: media elements, links, <base>, the h1/h2 title fallback, link
// text built by GetTextContent, table rows, and the image/link placeholder
// counters inside a large <table> were all dropped past that point — while
// other passes over the same document (the text pass, sanitization,
// CleanContentNode) run uncapped, so the results disagreed with each other.
// The cap also contradicted Config.MaxInputSize: a legal 50MB document can
// contain far more than 50,000 nodes, so truncation was guaranteed for legal
// input rather than reserved for hostile input.
//
// What bounds the work instead: input size (MaxInputSize), which bounds the
// parse and therefore the node count, and nesting depth (MaxDepth), which
// bounds the stack's depth. The stack holds one pointer per *pending sibling*,
// so its worst case is a flat document with millions of children directly
// under one parent (measured: 22MB of stack growth for a 7MB, 1M-node
// document, against 61MB for the parsed DOM itself). Bounding this by dropping
// nodes would reintroduce the silent-truncation bug; documents that large are
// better handled by lowering MaxInputSize.
func WalkNodes(node *html.Node, fn func(*html.Node) bool) {
	if node == nil || fn == nil {
		return
	}

	// Use pooled stack to avoid allocation
	stackPtr := GetNodeSlice()
	defer PutNodeSlice(stackPtr)
	stack := *stackPtr

	stack = append(stack, node)

	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if !fn(n) {
			continue
		}

		// Push children so the first child is processed next. The child list is a
		// singly-linked list (FirstChild -> NextSibling), so append them in document
		// order to the end of the stack, then reverse just that segment in place.
		// This yields correct document order when popped without an intermediate
		// buffer allocation. The reverse is skipped for segments of 0 or 1
		// entries (childless and single-child nodes — the common case on
		// text-heavy markup), where it would be a no-op anyway.
		segStart := len(stack)
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			stack = append(stack, child)
		}
		if segEnd := len(stack) - 1; segEnd > segStart {
			for i, j := segStart, segEnd; i < j; i, j = i+1, j-1 {
				stack[i], stack[j] = stack[j], stack[i]
			}
		}
	}

	// Update the pointer for pool return
	*stackPtr = stack
}

// FindElementByTag returns the first element node with the given tag name found
// in a pre-order traversal of doc, or nil if none exists.
//
// Every caller looks for a document-level landmark — <body> (extractArticleNode's
// fallback) or <base> (link base-URL detection). The walk stops at the first
// match and is bounded by the parse that produced the tree.
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

// GetTextContent returns the concatenated text of node and its descendants
// with surrounding whitespace trimmed. It returns "" for a nil node.
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
				return trimASCIISpaceAndNBSP(data)
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
		// Skip raw-text subtrees: their contents are code, not visible text.
		// The main extraction pass skips these via IsNonContentElement; a link
		// title built from GetTextContent used to concatenate script bodies
		// into the title ("evil()click") on the unsanitized ExtractAllLinks
		// path.
		if n.Type == html.ElementNode {
			switch n.Data {
			case "script", "style", "noscript":
				return false
			}
		}
		if n.Type == html.TextNode {
			data := n.Data
			dataLen := len(data)
			if dataLen == 0 {
				return true
			}

			// Single-pass: trim, normalize NBSP+newlines (entities are left
			// alone — the HTML parser already decoded them once).
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

			// textNeedsNormalization checks exactly the two triggers the
			// per-byte loop it replaced tests for (newline/CR, NBSP); a false
			// result proves the normalization pass below is an identity.
			var text string
			if !textNeedsNormalization(trimmed) {
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

// IsValidURL checks if a URL is valid and safe for processing.
// This is a centralized URL validation function with size limits for security.
func IsValidURL(url string) bool {
	urlLen := len(url)
	if urlLen == 0 || urlLen > MaxURLLength {
		return false
	}

	// SECURITY: Reject dangerous schemes (javascript:, vbscript:, file:) and
	// their protocol-relative forms before the format checks below. IsValidURL
	// is a format validator reached by paths that deliberately bypass the DOM
	// sanitizer: ExtractAllLinks skips sanitization, and the raw-HTML
	// video/audio scan reads pre-sanitization HTML. Without this gate a URL
	// such as "javascript:alert(1).mp4" passed because its first byte 'j' is
	// alphanumeric and the char scan below only blocks control bytes and
	// <>"'. containsDangerousScheme applies the same Unicode/C0/whitespace
	// normalization as the DOM sanitizer, so disguised schemes (leading C0
	// controls, embedded tab/LF/CR, fullwidth) are blocked identically here.
	if containsDangerousScheme(url) {
		return false
	}

	// Special handling for data URLs - stricter validation with size limit.
	//
	// SECURITY: the media-type policy must match the DOM sanitizer exactly.
	// IsValidURL gates paths that deliberately bypass sanitization
	// (ExtractAllLinks, the raw-HTML video/audio scan), so without the
	// safeMediaTypes whitelist a percent-encoded data URL declaring a
	// script-executing type — "data:image/svg+xml,%3Csvg onload=…%3E" contains
	// no raw <>"' and passed the charset scan below — would reach results as a
	// clickable link. dataURLMediaType is shared with isValidDataURLWithAudit
	// so the two gates enforce one policy (see isSafeURIWithAudit).
	// Fold case on the scheme prefix: schemes are case-insensitive (RFC 3986
	// §3.1) and isValidDataURLWithAudit folds the same test, so the two gates
	// share one policy. A case-sensitive test here let "DATA:text/html,…" skip
	// the whitelist while "DATA:image/png" images were rejected.
	if len(url) >= 5 && strings.EqualFold(url[:5], "data:") {
		if urlLen > MaxDataURILength {
			return false
		}
		// commaIdx <= 5 covers a missing comma and "data:,…" (empty media part).
		commaIdx := strings.IndexByte(url, ',')
		if commaIdx <= 5 {
			return false
		}
		mediaType, ok := dataURLMediaType(url[5:commaIdx])
		if !ok || !isValidMediaType(mediaType) || !isSafeMediaType(mediaType) {
			return false
		}
		for i := commaIdx + 1; i < urlLen; i++ {
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

	// Accept absolute URLs (scheme is case-insensitive per RFC 3986 §3.1).
	if hasHTTPScheme(url) {
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

// SelectBestCandidate returns the candidate node with the highest score, or nil
// if candidates is empty. Ties are resolved by map iteration order.
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
