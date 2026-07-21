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

// c0ControlOrSpace is the byte set browsers strip from the leading and trailing
// edge of a URL before scheme resolution (WHATWG URL Standard: "Remove any
// leading and trailing C0 control or space from input"): the C0 controls
// U+0000–U+001F plus ASCII space U+0020. strings.TrimSpace covers Unicode
// whitespace but not most C0 controls, so a leading byte such as "\x01" must be
// trimmed explicitly or it disguises a dangerous scheme from HasPrefix checks
// ("\x01javascript:…" is not detected) while the browser strips it and executes
// "javascript:".
const c0ControlOrSpace = "\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d\x0e\x0f" +
	"\x10\x11\x12\x13\x14\x15\x16\x17\x18\x19\x1a\x1b\x1c\x1d\x1e\x1f "

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

// findBodyElement locates the body element in the parsed HTML document.
// html.Parse nests <body> under <html> (Document → html → {head, body}), so
// <body> is a grandchild of the document node rather than a direct child; a
// direct-children walk (the old implementation) therefore returned nil for
// every real document and forced SanitizeHTMLWithAudit onto its fragment
// fallback, leaking <html><head>… into output when head carried surviving
// content. FindElementByTag descends the subtree to locate it correctly. The
// DocumentNode guard above is retained so non-document input still returns nil.
func findBodyElement(doc *html.Node) *html.Node {
	if doc.Type != html.DocumentNode {
		return nil
	}
	return FindElementByTag(doc, "body")
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
	// SECURITY: Browsers strip tab/LF/CR anywhere in a URL while parsing
	// (WHATWG URL Standard). A scheme split by those bytes — e.g.
	// "java\tscript:alert(1)" — survives NFC + TrimSpace (which only touch
	// Unicode form and the string's edges) and evades the HasPrefix scheme
	// checks below, yet is reassembled to "javascript:" and executed by every
	// conformant browser. Strip the three bytes before any scheme detection so
	// the lowerURI used by isDangerousScheme and the "data:" prefix test cannot
	// be disguised this way.
	//
	// SECURITY: Browsers also strip leading/trailing C0 control bytes
	// (U+0000–U+001F) and ASCII space before scheme resolution. TrimSpace
	// covers Unicode whitespace but not most C0 controls, so a leading byte
	// such as "\x01" would otherwise survive and disguise a dangerous scheme
	// ("\x01javascript:…" defeats every HasPrefix check below) while the
	// browser strips it and executes "javascript:". schemeStripped feeds scheme
	// detection; trimmed (un-stripped) is still passed to the data-URL body
	// parser below, since bytes there are content, not scheme.
	schemeStripped := strings.Trim(trimmed, c0ControlOrSpace)
	lowerURI := strings.ToLower(stripURLWhitespace(schemeStripped))

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

// stripURLWhitespace removes the bytes that browsers delete while parsing a
// URL — tab (U+0009), LF (U+000A), and CR (U+000D) per the WHATWG URL Standard.
// It is applied before scheme detection so a dangerous scheme cannot be
// disguised by interleaving these bytes (e.g. "java\tscript:"). The fast path
// returns the input unchanged (zero allocation) when none of the bytes are
// present, which is the case for the overwhelming majority of real URLs.
func stripURLWhitespace(s string) string {
	if !strings.ContainsAny(s, "\t\n\r") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\t', '\n', '\r':
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
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

// containsDangerousScheme reports whether uri begins with a scheme — or a
// protocol-relative form — that a browser may execute or that reaches the local
// filesystem: javascript:, vbscript:, file: (and //data:, //javascript:, …).
//
// It is a focused "is the scheme dangerous?" predicate, not a full URL
// allowlist: callers keep accepting http(s), relative paths, fragments,
// mailto/tel, and (via their own branch) safe data: URLs. The point is to close
// the defense-in-depth gap in IsValidURL, which is used by code paths that
// deliberately bypass the DOM sanitizer — ExtractAllLinks skips sanitization,
// and the raw-HTML video/audio scan reads pre-sanitization HTML — so a URL such
// as "javascript:alert(1).mp4" otherwise sailed through merely because its first
// byte 'j' is alphanumeric.
//
// The normalization pipeline mirrors isSafeURIWithAudit exactly (NFC, trim,
// leading/trailing C0+space strip, tab/LF/CR removal, ASCII-case fold, plus
// fullwidth folding) so a disguised scheme cannot pass here while the sanitizer
// blocks it — the two enforce one scheme policy.
func containsDangerousScheme(uri string) bool {
	if uri == "" {
		return false
	}

	normalized := normalizeURIForSecurity(uri)
	trimmed := strings.TrimSpace(normalized)
	schemeStripped := strings.Trim(trimmed, c0ControlOrSpace)
	lowerURI := strings.ToLower(stripURLWhitespace(schemeStripped))

	if isDangerousScheme(lowerURI, "javascript:") ||
		isDangerousScheme(lowerURI, "vbscript:") ||
		isDangerousScheme(lowerURI, "file:") {
		return true
	}

	// Protocol-relative dangerous forms: //javascript:, //vbscript:, //data:,
	// //file:. Match the sanitizer's protocol-relative branch (including the
	// TrimLeft of ASCII whitespace after the //) so the two stay consistent.
	if strings.HasPrefix(trimmed, "//") {
		restLower := strings.ToLower(strings.TrimLeft(trimmed[2:], " \t\n\r"))
		if isDangerousScheme(restLower, "javascript:") ||
			isDangerousScheme(restLower, "vbscript:") ||
			isDangerousScheme(restLower, "data:") ||
			isDangerousScheme(restLower, "file:") {
			return true
		}
	}

	return false
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
