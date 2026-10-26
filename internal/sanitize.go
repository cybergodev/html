package internal

import (
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/text/unicode/norm"
)

// isRemovedTag reports whether sanitization always strips the element tag.
// It is consulted once per element node on the sanitize walk, so it is a
// switch rather than a map lookup for the same hot-path reason as the
// element classifications in elements.go.
//
// Note: <form> itself is intentionally NOT removed — server frameworks
// (ASP.NET WebForms, JSF, JSP) wrap the entire page body in a single <form>,
// so removing it would discard all visible content. Text extraction never
// renders or submits forms, so the CSRF/UI-redress rationale that justifies
// removing <input>/<button> does not apply to the <form> container itself.
func isRemovedTag(tag string) bool {
	switch tag {
	// Script and style containers
	// Embedded content (potential XSS vectors)
	// Form controls (potential CSRF/UI redress)
	// SVG can contain JavaScript and event handlers
	// MathML can be abused for XSS in some browsers
	case "script", "style", "noscript",
		"iframe", "embed", "object",
		"input", "button",
		"svg",
		"math":
		return true
	}
	return false
}

// isDangerousAttrKey reports whether an attribute key (already lowercased by
// the parser or the caller) is always removed during sanitization. All on*
// event handlers are blocked separately by the prefix check in
// sanitizeNodeInPlace. A switch rather than a map: it runs once per attribute
// per element on the sanitize walk, the same hot-path reason as the element
// classifications in elements.go.
func isDangerousAttrKey(key string) bool {
	switch key {
	case "formaction", // Can override form action
		"autofocus": // Can be used for phishing
		return true
	}
	return false
}

// dangerousCSSPatterns are stripped from style attribute values during
// sanitization as plain substring matches — function-call and scheme-prefix
// shapes, whose spelling cannot legally contain internal whitespace.
var dangerousCSSPatterns = []string{
	"expression(",
	"javascript:",
	"vbscript:",
}

// dangerousCSSProperties are CSS property names whose declaration is always
// dangerous. They are matched as a declaration — the name followed by
// optional CSS whitespace and a ':' — because CSS permits whitespace between
// a property name and its colon, and "behavior :url(…)" slipped past the
// former plain-substring form ("behavior:").
var dangerousCSSProperties = []string{"behavior", "-moz-binding"}

// cssDeclPresent reports whether lower (already lowercased) declares name as
// a property: name directly followed by optional CSS whitespace and a ':'.
func cssDeclPresent(lower, name string) bool {
	for i := 0; i+len(name) <= len(lower); i++ {
		if lower[i:i+len(name)] != name {
			continue
		}
		j := i + len(name)
		for j < len(lower) && (lower[j] == ' ' || lower[j] == '\t' || lower[j] == '\n' || lower[j] == '\r' || lower[j] == '\f') {
			j++
		}
		if j < len(lower) && lower[j] == ':' {
			return true
		}
	}
	return false
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

// TruncateAuditURL truncates a URL for safe inclusion in audit log entries.
// It is exported (within the module) so the root package's audit collector can
// apply the identical cap to every AuditEntry.URL as a last line of defense,
// keeping the sanitizer call sites and the central collector on one policy.
func TruncateAuditURL(url string) string {
	if len(url) <= maxAuditURLLength {
		return url
	}
	cut := maxAuditURLLength
	// Back the cut off UTF-8 continuation bytes so the truncated value stays
	// valid UTF-8 — audit logs are routinely re-encoded as JSON downstream,
	// and a split rune would surface as an encoding error there. Mirrors the
	// rune-safe cut in the root package's RawValue truncation.
	for cut > 0 && url[cut]&0xC0 == 0x80 {
		cut--
	}
	return url[:cut] + "...[truncated]"
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
	for _, name := range dangerousCSSProperties {
		if cssDeclPresent(lower, name) {
			return ""
		}
	}
	return style
}

// isURIAttrKey reports whether an attribute key (already lowercased) carries a
// URI value subject to the scheme policy in isSafeURIWithAudit. A switch for
// the same per-attribute hot-path reason as isDangerousAttrKey.
func isURIAttrKey(key string) bool {
	switch key {
	case "href", "src", "cite", "action", "data",
		"poster", "background", "longdesc", "usemap", "profile",
		// Multi-URL attributes: srcset lists image candidates ("a.png 1x,
		// b.png 2x"), ping lists beacon URLs. Neither is a single URL, but the
		// scheme policy still applies to the value as a whole, so a hidden
		// "javascript:" or scriptable data: prefix cannot ride through
		// unchecked.
		"srcset", "ping",
		// SVG attack vectors - xlink:href can execute JavaScript
		"xlink:href":
		// Note: "formaction" is not included here as it's already in
		// isDangerousAttrKey which blocks it completely. Having it here would
		// be redundant.
		return true
	}
	return false
}

// SanitizeDOM sanitizes an already-parsed HTML DOM tree in-place.
// This avoids the overhead of rendering back to string and re-parsing.
// The doc node is modified directly.
func SanitizeDOM(doc *html.Node, audit AuditRecorder) {
	if doc == nil {
		return
	}
	sanitizeNodeWithAuditMode(doc, audit, false)
}

// SanitizeDOMReader sanitizes an already-parsed HTML DOM tree in place with
// reader-mode (ReadPage family) semantics. It differs from SanitizeDOM in one
// way: a <button> element is unwrapped — the element is removed but its child
// text (and safe inline markup) is spliced into the parent — instead of being
// dropped whole with its subtree. ReadPage keeps page-chrome navigation, where
// a menu's top-level labels are routinely <button> dropdown triggers; dropping
// the button also dropped the label. The <button> element itself is still gone
// in both modes (no interactive form control survives), so the unwrap preserves
// no control surface — only the visible label. The Extract path keeps
// SanitizeDOM, whose whole-subtree removal of <button> is unchanged.
func SanitizeDOMReader(doc *html.Node, audit AuditRecorder) {
	if doc == nil {
		return
	}
	sanitizeNodeWithAuditMode(doc, audit, true)
}

// sanitizeNodeWithAuditMode sanitizes n and its entire subtree in place.
//
// The traversal is iterative with an explicit stack rather than recursive:
// SanitizeDOM's callers pre-validate nesting depth, but the traversal must not
// depend on that — a recursive walk on hostile deeply nested input could
// exhaust the call stack before a depth guard ran. Children
// are pushed in reverse so they pop in document order, exactly matching the
// previous pre-order recursion, and sibling pointers saved on the stack are
// unaffected by removals performed deeper in the walk. The stack is pooled,
// but — unlike WalkNodes — deliberately has no node-count cap: sanitization
// must never be silently truncated.
//
// When unwrapButtons is true (reader mode), a <button> is intercepted before
// the generic removed-tag path: its children are queued for sanitization and
// the button is unwrapped, keeping the label text without keeping the control.
func sanitizeNodeWithAuditMode(n *html.Node, audit AuditRecorder, unwrapButtons bool) {
	if n == nil {
		return
	}

	stackPtr := GetNodeSlice()
	defer PutNodeSlice(stackPtr)
	stack := *stackPtr

	stack = append(stack, n)

	for len(stack) > 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]

		if unwrapButtons && node.Type == html.ElementNode && strings.EqualFold(node.Data, "button") {
			audit.RecordBlockedTag(node.Data)
			// Queue the button's children for sanitization before unwrapping:
			// after the unwrap they are ordinary siblings at the button's former
			// position and would otherwise never be reached by the stack walk.
			segStart := len(stack)
			for child := node.FirstChild; child != nil; child = child.NextSibling {
				stack = append(stack, child)
			}
			for i, j := segStart, len(stack)-1; i < j; i, j = i+1, j-1 {
				stack[i], stack[j] = stack[j], stack[i]
			}
			unwrapNode(node)
			continue
		}

		if !sanitizeNodeInPlace(node, audit) {
			continue
		}

		// Push children so the first child pops next (pre-order).
		segStart := len(stack)
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			stack = append(stack, child)
		}
		for i, j := segStart, len(stack)-1; i < j; i, j = i+1, j-1 {
			stack[i], stack[j] = stack[j], stack[i]
		}
	}

	*stackPtr = stack
}

// sanitizeNodeInPlace applies tag and attribute sanitization to a single
// node, removing it from its parent when its tag is blocklisted. It reports
// whether the node's subtree should still be traversed (false once the node
// has been removed).
func sanitizeNodeInPlace(n *html.Node, audit AuditRecorder) bool {
	if n.Type == html.ElementNode {
		tagName := strings.ToLower(n.Data)
		if isRemovedTag(tagName) {
			audit.RecordBlockedTag(n.Data)
			removeNode(n)
			return false
		}

		attrLen := len(n.Attr)
		if attrLen > 0 {
			// Compact attributes in place. Most elements keep every attribute,
			// so this avoids allocating a new slice on the common path; the slice
			// is only resliced when something is actually removed or rewritten.
			out := 0
			modified := false
			for i := range attrLen {
				attr := n.Attr[i]
				attrKey := strings.ToLower(attr.Key)
				if len(attrKey) >= 2 && attrKey[0] == 'o' && attrKey[1] == 'n' {
					audit.RecordBlockedAttr(attr.Key, attr.Val)
					modified = true
					continue
				}
				if isDangerousAttrKey(attrKey) {
					audit.RecordBlockedAttr(attr.Key, attr.Val)
					modified = true
					continue
				}
				if attrKey == "style" {
					// sanitizeStyleValue returns either "" (blocked) or its
					// input unchanged, so there is no partial-rewrite case to
					// write back here. An empty value ("style=\"\"") is
					// vacuously safe, not blocked: sanitizeStyleValue("")
					// returns "" too, and reading that as "blocked" removed
					// and audit-logged every empty style attribute on
					// otherwise benign markup.
					if attr.Val != "" && sanitizeStyleValue(attr.Val) == "" {
						audit.RecordBlockedAttr(attr.Key, attr.Val)
						modified = true
						continue
					}
				}
				if isURIAttrKey(attrKey) {
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

	return true
}

func removeNode(n *html.Node) {
	if n.Parent != nil {
		n.Parent.RemoveChild(n)
	}
}

// unwrapNode replaces n with its children in place: each child is inserted
// before n in the parent's child list (preserving order), then n is detached.
// It is the reader-mode alternative to removeNode for <button>: the interactive
// control disappears but the label text survives. The re-parented children keep
// their original subtree untouched, so any descendant sanitization still owed to
// them must be done separately (sanitizeNodeWithAuditMode queues them before the
// unwrap). Like removeNode, it is a no-op for a detached node.
func unwrapNode(n *html.Node) {
	if n.Parent == nil {
		return
	}
	for child := n.FirstChild; child != nil; {
		next := child.NextSibling
		n.RemoveChild(child) // detach (clears child.Parent) before re-parenting
		n.Parent.InsertBefore(child, n)
		child = next
	}
	n.Parent.RemoveChild(n)
}

// dangerousSchemePrefixes lists the scheme prefixes blocked everywhere a URI
// is checked, in evaluation order: javascript:, vbscript:, file:. data: is
// deliberately absent — every check site gives it a dedicated branch because
// safe data: image URLs are conditionally allowed. This table is the single
// source of a policy that was previously hand-copied across four call sites
// (the fast and slow paths of both isSafeURIWithAudit and
// containsDangerousScheme) and kept in sync only by comments and a
// differential test; add a scheme here and every site picks it up.
var dangerousSchemePrefixes = []string{"javascript:", "vbscript:", "file:"}

// protocolRelativeBlockedPrefixes lists the scheme prefixes blocked in
// protocol-relative form ("//javascript:…" and friends), evaluated after
// trimming ASCII whitespace following the "//". Unlike the direct list it
// includes data: — no protocol-relative data: URL is safe.
var protocolRelativeBlockedPrefixes = []string{"javascript:", "vbscript:", "data:", "file:"}

// blockedFoldPrefix returns the first policy prefix in list matched by an
// ASCII-case-insensitive prefix comparison against s, or "". Used on the
// printable-ASCII fast paths, where folding on raw bytes is exact.
func blockedFoldPrefix(s string, list []string) string {
	for _, p := range list {
		if asciiFoldHasPrefix(s, p) {
			return p
		}
	}
	return ""
}

// blockedDangerousPrefix returns the first policy prefix in list for which
// isDangerousScheme(s, p) holds, or "". Used on the normalized slow paths,
// which pass an NFC/trim/lowercased URI; isDangerousScheme carries the
// remaining Unicode-disguise comparisons.
func blockedDangerousPrefix(s string, list []string) string {
	for _, p := range list {
		if isDangerousScheme(s, p) {
			return p
		}
	}
	return ""
}

// protocolRelativeRemainder returns the lowercased remainder of s after a
// leading "//" — with leading C0/space removed and the bytes browsers delete
// while parsing a URL (tab, LF, CR) stripped — or "" when s is not
// protocol-relative.
//
// The trailing slice must come from the same C0-stripped string that passed
// the "//" test. Slicing the unstripped text instead was a bypass: with two
// leading control bytes, trimmed[2:] consumed the "//" and the remainder began
// at "javascript:", so the "//javascript:" policy prefix no longer matched.
//
// The ToLower is LOAD-BEARING for the slow-path callers: they feed the result
// to blockedDangerousPrefix → isDangerousScheme, which compares exactly against
// the lowercase policy prefixes (unlike blockedFoldPrefix, which ASCII-folds).
// TestProtocolRelativeC0Bypass pins this — removing the ToLower let
// "\x01\x02//JavaScript:alert(1)" through.
func protocolRelativeRemainder(s string) string {
	rest, ok := strings.CutPrefix(s, "//")
	if !ok {
		return ""
	}
	return strings.ToLower(stripURLWhitespace(strings.TrimLeft(rest, c0ControlOrSpace)))
}

func isSafeURIWithAudit(uri string, audit AuditRecorder) bool {
	if uri == "" {
		return true
	}

	// Fast path: a plain printable-ASCII URI (see isPlainPrintableASCII) is
	// unchanged by every normalization step below — NFC, the whitespace/C0 edge
	// trims, tab/LF/CR stripping — and lowercasing collapses to ASCII folding,
	// so the scheme policy can be applied with fold comparisons directly on
	// uri. sanitizeNodeInPlace reaches here once per href/src/cite/... attribute
	// on the sanitize walk, and the profiler showed the normalization pipeline
	// dominating; non-ASCII or whitespace-bearing URIs fall through to the full
	// pipeline where the disguise vectors this defense exists for live.
	if isPlainPrintableASCII(uri) {
		if len(uri) > MaxURLLength && !asciiFoldHasPrefix(uri, "data:") {
			audit.RecordBlockedURL(TruncateAuditURL(uri), "URL exceeds size limit")
			return false
		}
		if s := blockedFoldPrefix(uri, dangerousSchemePrefixes); s != "" {
			audit.RecordBlockedURL(TruncateAuditURL(uri), strings.TrimSuffix(s, ":")+" scheme")
			return false
		}
		if strings.HasPrefix(uri, "//") {
			// Route through protocolRelativeRemainder — the single
			// implementation of the "//" policy (see containsDangerousSchemeFast)
			// — rather than re-splicing uri[2:] here. On this printable-ASCII
			// fast path its trims are provable no-ops (isPlainPrintableASCII
			// rejects those bytes) and its ToLower is semantically redundant
			// (blockedFoldPrefix ASCII-folds) — at worst one small allocation
			// for an uppercase protocol-relative URL, the price of keeping one
			// spelling of the policy. A hand-rolled copy is how the "//" policy
			// drifted into four spellings once before.
			if blockedFoldPrefix(protocolRelativeRemainder(uri), protocolRelativeBlockedPrefixes) != "" {
				audit.RecordBlockedURL(TruncateAuditURL(uri), "dangerous protocol-relative URL")
				return false
			}
		}
		if asciiFoldHasPrefix(uri, "data:") {
			if asciiFoldContains(uri, "image/svg+xml") {
				audit.RecordBlockedURL(TruncateAuditURL(uri), "svg data url")
				return false
			}
			if !isValidDataURLWithAudit(uri, audit) {
				return false
			}
		}
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
		audit.RecordBlockedURL(TruncateAuditURL(uri), "URL exceeds size limit")
		return false
	}

	// SECURITY: Check for dangerous schemes with multiple Unicode attack
	// vectors, via the shared policy tables. Every RecordBlockedURL below
	// passes TruncateAuditURL(uri): these branches are reached only after
	// the MaxURLLength gate above, but the data: branch is exempt from that
	// gate and can carry megabytes, and a future recorder implementation
	// must never receive an unbounded URL.
	if s := blockedDangerousPrefix(lowerURI, dangerousSchemePrefixes); s != "" {
		audit.RecordBlockedURL(TruncateAuditURL(uri), strings.TrimSuffix(s, ":")+" scheme")
		return false
	}

	// Check for dangerous protocol-relative URL patterns
	// Block //javascript:, //vbscript:, etc. with potential whitespace bypass.
	// The check runs against schemeStripped (C0-stripped) for BOTH the "//"
	// test and the remainder, so a leading control byte cannot disguise the
	// protocol-relative form. Slicing the unstripped `trimmed` here was a
	// hole: with two leading C0 bytes, trimmed[2:] consumed the "//" and the
	// remainder began at "javascript:", which no longer matched the
	// "//javascript:" policy prefix.
	if blockedDangerousPrefix(
		protocolRelativeRemainder(schemeStripped), protocolRelativeBlockedPrefixes) != "" {
		audit.RecordBlockedURL(TruncateAuditURL(uri), "dangerous protocol-relative URL")
		return false
	}

	if strings.HasPrefix(lowerURI, "data:") {
		// Explicitly block SVG data URLs - they can contain JavaScript
		// This provides defense-in-depth in case SVG tag removal is bypassed.
		// SVG data URLs are exempt from the MaxURLLength gate above, so this
		// uri can be as large as MaxInputSize — truncate it for the audit log.
		if strings.Contains(lowerURI, "image/svg+xml") {
			audit.RecordBlockedURL(TruncateAuditURL(uri), "svg data url")
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

// isPlainPrintableASCII reports whether s is non-empty and consists solely of
// printable ASCII bytes (0x21–0x7E): no C0 controls, no space, no DEL, and no
// multi-byte UTF-8. For such a string every step of the scheme-normalization
// pipeline is an identity — NFC (ASCII has no composable sequences), TrimSpace
// and the C0+space edge trims (the edges are > 0x20), tab/LF/CR stripping
// (none present), and fullwidth folding (fullwidth runes are 3-byte UTF-8) —
// and lowercasing collapses to ASCII case folding. The scheme checks can then
// run directly on s with fold comparisons, skipping the pipeline's string
// allocations and rescans. This is the overwhelming majority of real URLs.
func isPlainPrintableASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if b := s[i]; b <= 0x20 || b > 0x7E {
			return false
		}
	}
	return len(s) > 0
}

// asciiFoldContains reports whether substr (assumed lowercase ASCII) is
// contained in s, ignoring ASCII case. For pure-ASCII s it is equivalent to
// strings.Contains(strings.ToLower(s), substr) without the lowercase copy.
func asciiFoldContains(s, substr string) bool {
	substrLen := len(substr)
	sLen := len(s)
	if substrLen > sLen {
		return false
	}
	for i := 0; i <= sLen-substrLen; i++ {
		if asciiFoldHasPrefix(s[i:], substr) {
			return true
		}
	}
	return false
}

// containsDangerousSchemeFast applies the same scheme policy as
// containsDangerousScheme to a plain printable-ASCII uri (see
// isPlainPrintableASCII): direct ASCII-fold prefix checks replace the
// NFC/trim/lowercase pipeline, which is an identity on such input.
func containsDangerousSchemeFast(uri string) bool {
	if blockedFoldPrefix(uri, dangerousSchemePrefixes) != "" {
		return true
	}
	// Protocol-relative dangerous forms, via the same helper the slow paths
	// use. A printable-ASCII uri cannot contain C0 controls or whitespace
	// (isPlainPrintableASCII rejects them), so the trims inside the helper are
	// provable no-ops here — but sharing the helper keeps a single
	// implementation of the "//" policy.
	if rest, ok := strings.CutPrefix(uri, "//"); ok {
		if blockedFoldPrefix(rest, protocolRelativeBlockedPrefixes) != "" {
			return true
		}
	}
	return false
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

	// Fast path: printable-ASCII URLs need none of the normalization below,
	// and isValidURL calls this once per candidate URL on the media and link
	// extraction paths (the profiler showed the NFC/trim/lowercase pipeline as
	// most of IsValidURL's cost). Falls through to the full pipeline for any
	// URL with whitespace, controls, or non-ASCII bytes, where the disguises
	// this function defends against actually live.
	if isPlainPrintableASCII(uri) {
		return containsDangerousSchemeFast(uri)
	}

	normalized := normalizeURIForSecurity(uri)
	trimmed := strings.TrimSpace(normalized)
	schemeStripped := strings.Trim(trimmed, c0ControlOrSpace)
	lowerURI := strings.ToLower(stripURLWhitespace(schemeStripped))

	if blockedDangerousPrefix(lowerURI, dangerousSchemePrefixes) != "" {
		return true
	}

	// Protocol-relative dangerous forms: //javascript:, //vbscript:, //data:,
	// //file:. Shares the sanitizer's helper so both entry points run the same
	// C0-stripped "//" test over the same string they then slice.
	if blockedDangerousPrefix(
		protocolRelativeRemainder(schemeStripped), protocolRelativeBlockedPrefixes) != "" {
		return true
	}

	return false
}

func isValidDataURLWithAudit(url string, audit AuditRecorder) bool {
	// Scheme names are case-insensitive (RFC 3986 §3.1) and browsers accept
	// "DATA:" data URLs. Both callers gate this function behind a
	// case-insensitive "data:" prefix test, so this re-check must fold case
	// too: a case-sensitive test made every uppercase data URL fail here
	// without recording an audit event — valid "DATA:image/png" images were
	// silently dropped, and the same case-sensitive test in IsValidURL let
	// scriptable "DATA:text/html" URLs through instead.
	if len(url) < 5 || !strings.EqualFold(url[:5], "data:") {
		return false
	}

	commaIdx := strings.Index(url, ",")
	if commaIdx == -1 || commaIdx == 5 {
		audit.RecordBlockedURL(TruncateAuditURL(url), "malformed data URL")
		return false
	}

	mediaPart := url[5:commaIdx]
	dataPart := url[commaIdx+1:]

	// Enforce maximum data URL size to prevent memory exhaustion
	// Uses the same limit as IsValidURL for consistency
	if len(url) > MaxDataURILength {
		audit.RecordBlockedURL(TruncateAuditURL(url), "data URL exceeds size limit")
		return false
	}

	// A data URL must declare an explicit, whitelisted media type. An empty
	// mediaType — e.g. "data:;base64,<payload>" or "data:;...,..." — used to
	// skip both checks below and bypass the safeMediaTypes whitelist entirely,
	// letting arbitrary base64-encoded content through. Reject it outright.
	// The extraction lives in dataURLMediaType so IsValidURL (the non-audit
	// gate used by the paths that bypass the DOM sanitizer) enforces the
	// identical policy and the two can never drift apart again.
	mediaType, hasMediaType := dataURLMediaType(mediaPart)
	if !hasMediaType {
		audit.RecordBlockedURL(TruncateAuditURL(url), "missing media type in data URL")
		return false
	}
	// Validate media type and check against whitelist of safe types
	if !isValidMediaType(mediaType) {
		audit.RecordBlockedURL(TruncateAuditURL(url), "invalid media type in data URL")
		return false
	}
	if !isSafeMediaType(mediaType) {
		audit.RecordBlockedURL(TruncateAuditURL(url), "unsafe media type in data URL: "+mediaType)
		return false
	}

	// The data-part policy here is deliberately WIDER than IsValidURL's (see
	// the counterpart comment there): this gate decides whether the attribute
	// may stay in the sanitized DOM — browsers strip \r\n while decoding
	// folded base64 — while that gate decides whether the URL may enter
	// output text, where a raw newline breaks a Markdown link destination. A
	// data: URL can pass here and still be declined at collection.
	isBase64 := strings.Contains(mediaPart, ";base64")
	for i := 0; i < len(dataPart); i++ {
		b := dataPart[i]
		if isBase64 {
			if !isBase64Char(b) && b != '=' && b != '\r' && b != '\n' {
				audit.RecordBlockedURL(TruncateAuditURL(url), "invalid base64 in data URL")
				return false
			}
		} else {
			if b < 9 || (b >= 11 && b <= 12) || (b >= 14 && b < 32) || b == 127 {
				audit.RecordBlockedURL(TruncateAuditURL(url), "invalid character in data URL")
				return false
			}
		}
	}

	return true
}

// dataURLMediaType extracts the declared media type from the part of a data:
// URL between "data:" and the first comma: a trailing ";base64" and any other
// ";parameter" suffix are stripped. ok is false when no explicit media type is
// declared ("data:,…" or "data:;base64,…") — the sanitizer rejects those, and
// so must every path sharing its policy. It is the single source of truth for
// both isValidDataURLWithAudit and IsValidURL so the two gates cannot diverge.
func dataURLMediaType(mediaPart string) (mediaType string, ok bool) {
	if mediaPart == "" {
		return "", false
	}
	if rest, isBase64 := strings.CutSuffix(mediaPart, ";base64"); isBase64 {
		mediaType = rest
	} else if head, _, hasParam := strings.Cut(mediaPart, ";"); hasParam {
		// An empty head (a leading ';') yields an empty mediaType, rejected by
		// the ok=false return below (mirrors the sanitizer's empty-MIME
		// rejection).
		mediaType = head
	} else {
		mediaType = mediaPart
	}
	if mediaType == "" {
		return "", false
	}
	return mediaType, true
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
