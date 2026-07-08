package internal

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/text/unicode/norm"
)

var tagsToRemoveMap = map[string]bool{
	// Script and style containers
	"script": true, "style": true, "noscript": true,
	// Embedded content (potential XSS vectors)
	"iframe": true, "embed": true, "object": true,
	// Form controls (potential CSRF/UI redress). Note: <form> itself is
	// intentionally NOT removed — server frameworks (ASP.NET WebForms, JSF, JSP)
	// wrap the entire page body in a single <form>, so removing it would discard
	// all visible content. Text extraction never renders or submits forms, so the
	// CSRF/UI-redress rationale that justifies removing <input>/<button> does not
	// apply to the <form> container itself.
	"input": true, "button": true,
	// SVG can contain JavaScript and event handlers
	"svg": true,
	// MathML can be abused for XSS in some browsers
	"math": true,
}

// dangerousAttributes are always removed during sanitization.
// All on* event handlers are blocked by prefix check in sanitizeNodeWithAudit.
var dangerousAttributes = map[string]bool{
	// Other dangerous attributes
	"formaction": true, // Can override form action
	"autofocus":  true, // Can be used for phishing
}

// dangerousCSSPatterns are stripped from style attribute values during sanitization.
var dangerousCSSPatterns = []string{
	"expression(",
	"behavior:",
	"-moz-binding:",
	"javascript:",
	"vbscript:",
}

// maxAuditURLLength limits the URL value passed to audit RecordBlockedURL.
// Data URLs can contain large base64 payloads; logging the full content
// wastes disk space and risks leaking embedded sensitive content.
const maxAuditURLLength = 256

// truncateAuditURL truncates a URL for safe inclusion in audit log entries.
func truncateAuditURL(url string) string {
	if len(url) <= maxAuditURLLength {
		return url
	}
	return url[:maxAuditURLLength] + "...[truncated]"
}

// sanitizeStyleValue removes dangerous CSS constructs from a style attribute value.
// Safe properties (text-align, width, etc.) are preserved for metadata extraction.
func sanitizeStyleValue(style string) string {
	lower := strings.ToLower(style)
	for _, pattern := range dangerousCSSPatterns {
		if strings.Contains(lower, pattern) {
			return ""
		}
	}
	return style
}

var uriAttributes = map[string]bool{
	"href":   true,
	"src":    true,
	"cite":   true,
	"action": true,
	"data":   true,
	// Note: "formaction" is not included here as it's already in dangerousAttributes
	// which blocks it completely. Having it here would be redundant.
	"poster":     true,
	"background": true,
	"longdesc":   true,
	"usemap":     true,
	"profile":    true,
	// SVG attack vectors - xlink:href can execute JavaScript
	"xlink:href": true,
}

func SanitizeHTML(htmlContent string) string {
	return SanitizeHTMLWithAudit(htmlContent, NoOpAuditRecorder{})
}

// SanitizeDOM sanitizes an already-parsed HTML DOM tree in-place.
// This avoids the overhead of rendering back to string and re-parsing.
// The doc node is modified directly.
func SanitizeDOM(doc *html.Node, audit AuditRecorder) {
	if doc == nil {
		return
	}
	sanitizeNodeWithAudit(doc, audit)
}

// SanitizeHTMLWithAudit sanitizes HTML content and records security events.
// The audit recorder receives events for blocked tags, attributes, and URLs.
func SanitizeHTMLWithAudit(htmlContent string, audit AuditRecorder) string {
	if htmlContent == "" {
		return ""
	}

	doc, err := html.Parse(strings.NewReader(htmlContent))
	if err != nil {
		return ""
	}

	sanitizeNodeWithAudit(doc, audit)

	// Find body element and extract its content properly
	body := findBodyElement(doc)
	if body == nil {
		// No body element found, render the entire document (fragment case)
		buf := GetBuffer()
		defer PutBuffer(buf)

		if err := html.Render(buf, doc); err != nil {
			return ""
		}
		result := buf.String()
		// Remove the automatic html/head/body wrapper for fragments
		result = strings.ReplaceAll(result, "<html><head></head><body>", "")
		result = strings.ReplaceAll(result, "</body></html>", "")
		return result
	}

	buf := GetBuffer()
	defer PutBuffer(buf)

	for child := body.FirstChild; child != nil; child = child.NextSibling {
		if err := html.Render(buf, child); err != nil {
			continue
		}
	}

	return buf.String()
}

// findBodyElement locates the body element in the parsed HTML document
func findBodyElement(doc *html.Node) *html.Node {
	if doc.Type != html.DocumentNode {
		return nil
	}
	for child := doc.FirstChild; child != nil; child = child.NextSibling {
		if child.Type == html.ElementNode && child.Data == "body" {
			return child
		}
	}
	return nil
}

func sanitizeNodeWithAudit(n *html.Node, audit AuditRecorder) {
	if n.Type == html.ElementNode {
		tagName := strings.ToLower(n.Data)
		if tagsToRemoveMap[tagName] {
			audit.RecordBlockedTag(n.Data)
			removeNode(n)
			return
		}

		attrLen := len(n.Attr)
		if attrLen > 0 {
			// Compact attributes in place. Most elements keep every attribute,
			// so this avoids allocating a new slice on the common path; the slice
			// is only resliced when something is actually removed or rewritten.
			out := 0
			modified := false
			for i := 0; i < attrLen; i++ {
				attr := n.Attr[i]
				attrKey := strings.ToLower(attr.Key)
				if len(attrKey) >= 2 && attrKey[0] == 'o' && attrKey[1] == 'n' {
					audit.RecordBlockedAttr(attr.Key, attr.Val)
					modified = true
					continue
				}
				if dangerousAttributes[attrKey] {
					audit.RecordBlockedAttr(attr.Key, attr.Val)
					modified = true
					continue
				}
				if attrKey == "style" {
					sanitized := sanitizeStyleValue(attr.Val)
					if sanitized == "" {
						audit.RecordBlockedAttr(attr.Key, attr.Val)
						modified = true
						continue
					}
					if sanitized != attr.Val {
						attr.Val = sanitized
						modified = true
					}
				}
				if uriAttributes[attrKey] {
					if !isSafeURIWithAudit(attr.Val, audit) {
						modified = true
						continue
					}
				}
				n.Attr[out] = attr
				out++
			}
			if modified {
				// SECURITY: Zero out the dropped slots so stale attribute values
				// (e.g. sanitized style or blocked URI payloads) are not retained
				// in the backing array before reslicing.
				for i := out; i < attrLen; i++ {
					n.Attr[i] = html.Attribute{}
				}
				n.Attr = n.Attr[:out]
			}
		}
	}

	child := n.FirstChild
	for child != nil {
		next := child.NextSibling
		sanitizeNodeWithAudit(child, audit)
		child = next
	}
}

func removeNode(n *html.Node) {
	if n.Parent != nil {
		n.Parent.RemoveChild(n)
	}
}

// RemoveTagContent removes all occurrences of the specified HTML tag and its content.
// This function uses string-based parsing as the primary method to handle edge cases
// like unclosed tags, malformed HTML, and to preserve original character case.
func RemoveTagContent(content, tag string) string {
	if content == "" || tag == "" {
		return content
	}

	// Quick check: if tag is not present, return as-is
	if indexASCIIFold(content, "<"+tag) == -1 {
		return content
	}

	return removeTagContentStringBased(content, tag)
}

// asciiEqualFold checks if two ASCII strings are equal case-insensitively.
func asciiEqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 32
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 32
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// indexASCIIFold finds the case-insensitive index of target in s (ASCII only).
// Optimized to use strings.IndexByte for fast first-character scanning.
func indexASCIIFold(s, target string) int {
	targetLen := len(target)
	sLen := len(s)
	if targetLen == 0 {
		return 0
	}
	if sLen < targetLen {
		return -1
	}
	// Use IndexByte to scan for the first character (SIMD-optimized)
	firstByte := target[0]
	pos := 0
	for pos <= sLen-targetLen {
		idx := strings.IndexByte(s[pos:], firstByte)
		if idx < 0 {
			// Also check uppercase variant
			var upperByte byte
			if firstByte >= 'a' && firstByte <= 'z' {
				upperByte = firstByte - 32
			} else {
				break
			}
			idx = strings.IndexByte(s[pos:], upperByte)
			if idx < 0 {
				break
			}
		}
		idx += pos
		if idx+targetLen > sLen {
			return -1
		}
		if asciiEqualFold(s[idx:idx+targetLen], target) {
			return idx
		}
		pos = idx + 1
	}
	return -1
}

// removeTagContentStringBased removes tags using string operations.
// This approach preserves character case and handles malformed HTML better than DOM parsing.
// Uses BuilderPool for memory efficiency to reduce allocations during string building.
// Optimized to avoid strings.ToLower copy of full content.
func removeTagContentStringBased(content, tag string) string {
	openTag := "<" + strings.ToLower(tag)
	closeTag := "</" + strings.ToLower(tag) + ">"

	// Use pooled builder for better memory efficiency
	sb := GetBuilder()
	defer PutBuilder(sb)

	sb.Grow(len(content))

	pos := 0
	for pos < len(content) {
		// Find the next opening tag (case-insensitive) without creating a lowercase copy
		start := indexASCIIFold(content[pos:], openTag)
		if start == -1 {
			sb.WriteString(content[pos:])
			break
		}
		start += pos

		// Verify this is actually the tag we're looking for
		tagNameLen := len(tag) + 1 // +1 for the '<'
		if start+tagNameLen < len(content) {
			nextChar := content[start+tagNameLen]
			lc := nextChar
			if lc >= 'A' && lc <= 'Z' {
				lc += 32
			}
			if lc != ' ' && lc != '>' && lc != '\t' && lc != '\n' && lc != '/' {
				sb.WriteString(content[pos : start+tagNameLen])
				pos = start + tagNameLen
				continue
			}
		}

		// Write content before the tag
		sb.WriteString(content[pos:start])

		// Find the end of the opening tag
		tagEnd := strings.IndexByte(content[start:], '>')
		if tagEnd == -1 {
			sb.WriteString(content[start:])
			break
		}
		tagEnd += start + 1
		if tagEnd > len(content) {
			tagEnd = len(content)
		}

		// Look for the corresponding closing tag (case-insensitive, no copy)
		if end := indexASCIIFold(content[tagEnd:], closeTag); end != -1 {
			// Found closing tag, skip everything between opening and closing
			pos = tagEnd + end + len(closeTag)
		} else {
			// No closing tag found, this is malformed HTML
			// Skip the opening tag and continue
			pos = tagEnd
		}
	}

	return sb.String()
}

func isSafeURIWithAudit(uri string, audit AuditRecorder) bool {
	if uri == "" {
		return true
	}

	// SECURITY: Apply NFC normalization first to prevent Unicode-based bypass attacks.
	// Attackers may use different Unicode representations of the same characters
	// to bypass protocol checks. For example:
	// - Using fullwidth characters (ｊａｖａｓｃｒｉｐｔ:)
	// - Using combining characters
	// - Using lookalike characters from different scripts
	//
	// NFC normalization ensures consistent representation before security checks.
	normalized := normalizeURIForSecurity(uri)

	trimmed := strings.TrimSpace(normalized)
	lowerURI := strings.ToLower(trimmed)

	// SECURITY: Reject oversized non-data URIs to bound the work spent parsing a
	// hostile attribute and to keep sanitization consistent with IsValidURL's
	// MaxURLLength ceiling. data: URLs are exempt: they have a separate, larger
	// ceiling (MaxDataURILength) enforced in the data: branch below, and
	// legitimate base64 images routinely exceed MaxURLLength.
	if len(uri) > MaxURLLength && !strings.HasPrefix(lowerURI, "data:") {
		audit.RecordBlockedURL(truncateAuditURL(uri), "URL exceeds size limit")
		return false
	}

	// SECURITY: Check for dangerous schemes with multiple Unicode attack vectors

	// Check for javascript: scheme and its Unicode variants
	if isDangerousScheme(lowerURI, "javascript:") {
		audit.RecordBlockedURL(uri, "javascript scheme")
		return false
	}

	// Check for vbscript: scheme and its Unicode variants
	if isDangerousScheme(lowerURI, "vbscript:") {
		audit.RecordBlockedURL(uri, "vbscript scheme")
		return false
	}

	// Check for file: scheme and its Unicode variants
	if isDangerousScheme(lowerURI, "file:") {
		audit.RecordBlockedURL(uri, "file scheme")
		return false
	}

	// Check for dangerous protocol-relative URL patterns
	// Block //javascript:, //vbscript:, etc. with potential whitespace bypass
	if strings.HasPrefix(trimmed, "//") {
		restLower := strings.ToLower(strings.TrimLeft(trimmed[2:], " \t\n\r"))
		if isDangerousScheme(restLower, "javascript:") ||
			isDangerousScheme(restLower, "vbscript:") ||
			isDangerousScheme(restLower, "data:") ||
			isDangerousScheme(restLower, "file:") {
			audit.RecordBlockedURL(uri, "dangerous protocol-relative URL")
			return false
		}
	}

	if strings.HasPrefix(lowerURI, "data:") {
		// Explicitly block SVG data URLs - they can contain JavaScript
		// This provides defense-in-depth in case SVG tag removal is bypassed
		if strings.Contains(lowerURI, "image/svg+xml") {
			audit.RecordBlockedURL(uri, "svg data url")
			return false
		}
		if !isValidDataURLWithAudit(trimmed, audit) {
			return false
		}
	}

	return true
}

// normalizeURIForSecurity applies security-focused normalization to URIs.
// This helps prevent Unicode-based bypass attacks.
func normalizeURIForSecurity(uri string) string {
	// Apply NFC normalization for consistent character representation so that
	// lookalike Unicode variants (fullwidth, combining marks) cannot disguise
	// dangerous schemes like javascript:.
	return norm.NFC.String(uri)
}

// isDangerousScheme checks if a URI starts with a dangerous scheme,
// accounting for various Unicode attack vectors including fullwidth characters.
func isDangerousScheme(lowerURI, scheme string) bool {
	// Direct match check
	if strings.HasPrefix(lowerURI, scheme) {
		return true
	}

	// SECURITY: Check for fullwidth Unicode characters (U+FF00-U+FFEF) that could
	// disguise dangerous schemes. Fullwidth Latin characters map to ASCII equivalents:
	//   U+FF01(!) through U+FF5E(~) offset by 0xFEE0 from ASCII
	// Some browsers/HTML parsers normalize these, so we must detect them.
	normalized := normalizeFullwidthToASCII(lowerURI)
	return strings.HasPrefix(normalized, scheme)
}

// normalizeFullwidthToASCII converts fullwidth Latin characters and digits to their
// ASCII equivalents. Fullwidth characters (U+FF01-U+FF5E) map to ASCII (0x21-0x7E)
// by subtracting 0xFEE0. This prevents scheme bypass using fullwidth characters.
func normalizeFullwidthToASCII(s string) string {
	hasFullwidth := false
	for _, r := range s {
		if r >= 0xFF01 && r <= 0xFF5E {
			hasFullwidth = true
			break
		}
	}
	if !hasFullwidth {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= 0xFF01 && r <= 0xFF5E {
			b.WriteRune(r - 0xFEE0)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isValidDataURLWithAudit(url string, audit AuditRecorder) bool {
	if !strings.HasPrefix(url, "data:") {
		return false
	}

	commaIdx := strings.Index(url, ",")
	if commaIdx == -1 || commaIdx == 5 {
		audit.RecordBlockedURL(truncateAuditURL(url), "malformed data URL")
		return false
	}

	mediaPart := url[5:commaIdx]
	dataPart := url[commaIdx+1:]

	// Enforce maximum data URL size to prevent memory exhaustion
	// Uses the same limit as IsValidURL for consistency
	if len(url) > MaxDataURILength {
		audit.RecordBlockedURL(truncateAuditURL(url), "data URL exceeds size limit")
		return false
	}

	if mediaPart != "" {
		var mediaType string
		if strings.HasSuffix(mediaPart, ";base64") {
			mediaType = strings.TrimSuffix(mediaPart, ";base64")
		} else if strings.Contains(mediaPart, ";") {
			semicolonIdx := strings.Index(mediaPart, ";")
			if semicolonIdx > 0 {
				mediaType = mediaPart[:semicolonIdx]
			}
			// semicolonIdx == 0 leaves mediaType empty; handled by the empty-MIME
			// rejection below.
		} else {
			mediaType = mediaPart
		}

		// A data URL must declare an explicit, whitelisted media type. An empty
		// mediaType — e.g. "data:;base64,<payload>" or "data:;...,..." — used to
		// skip both checks below and bypass the safeMediaTypes whitelist entirely,
		// letting arbitrary base64-encoded content through. Reject it outright.
		if mediaType == "" {
			audit.RecordBlockedURL(truncateAuditURL(url), "missing media type in data URL")
			return false
		}
		// Validate media type and check against whitelist of safe types
		if !isValidMediaType(mediaType) {
			audit.RecordBlockedURL(truncateAuditURL(url), "invalid media type in data URL")
			return false
		}
		if !isSafeMediaType(mediaType) {
			audit.RecordBlockedURL(truncateAuditURL(url), "unsafe media type in data URL: "+mediaType)
			return false
		}
	}

	isBase64 := strings.Contains(mediaPart, ";base64")
	for i := 0; i < len(dataPart); i++ {
		b := dataPart[i]
		if isBase64 {
			if !isBase64Char(b) && b != '=' && b != '\r' && b != '\n' {
				audit.RecordBlockedURL(truncateAuditURL(url), "invalid base64 in data URL")
				return false
			}
		} else {
			if b < 9 || (b >= 11 && b <= 12) || (b >= 14 && b < 32) || b == 127 {
				audit.RecordBlockedURL(truncateAuditURL(url), "invalid character in data URL")
				return false
			}
		}
	}

	return true
}

// safeMediaTypes is the whitelist of safe media types for data URLs.
// Package-level to avoid allocation on every isSafeMediaType call.
var safeMediaTypes = map[string]bool{
	"image/gif": true, "image/jpeg": true, "image/jpg": true,
	"image/png": true, "image/webp": true, "image/bmp": true,
	"image/x-icon": true, "image/vnd.microsoft.icon": true,
	"image/avif": true, "image/apng": true,
	"font/woff": true, "font/woff2": true, "font/ttf": true, "font/otf": true,
	"application/font-woff": true, "application/font-woff2": true,
	"application/pdf": true,
}

// isSafeMediaType checks if the media type is in the whitelist of safe types.
// This prevents XSS through script data URIs and other dangerous content types.
func isSafeMediaType(mediaType string) bool {
	return safeMediaTypes[strings.ToLower(strings.TrimSpace(mediaType))]
}

func isValidMediaType(mediaType string) bool {
	if mediaType == "" {
		return false
	}

	slashIdx := strings.Index(mediaType, "/")
	if slashIdx <= 0 || slashIdx == len(mediaType)-1 {
		return false
	}

	for i := 0; i < len(mediaType); i++ {
		c := mediaType[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') &&
			(c < '0' || c > '9') && c != '-' && c != '+' &&
			c != '/' && c != '.' && c != '_' {
			return false
		}
	}

	return true
}

func isBase64Char(b byte) bool {
	return (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z') ||
		(b >= '0' && b <= '9') ||
		b == '+' || b == '/'
}
