// url.go provides URL parsing and resolution utilities.
package internal

import "strings"

// IsExternalURL checks if a URL is an external HTTP(S) URL or protocol-relative URL.
func IsExternalURL(url string) bool {
	return hasHTTPScheme(url) || strings.HasPrefix(url, "//")
}

// schemeEnd returns the byte offset in url where the domain/host begins —
// i.e. immediately after the scheme separator. It handles three forms:
//   - "scheme://host/path" → returns the index after "://"
//   - "//host/path"        → returns 2 (after the leading slashes)
//   - "host/path" or other → returns 0 (no scheme)
//
// This centralizes the scheme-parsing logic shared by ExtractBaseFromURL,
// NormalizeBaseURL, ResolveURL, and asDirectoryBase.
func schemeEnd(url string) int {
	if idx := strings.Index(url, "://"); idx >= 0 {
		return idx + 3
	}
	if strings.HasPrefix(url, "//") {
		return 2
	}
	return 0
}

// hasHTTPScheme reports whether url begins with an http:// or https:// scheme,
// compared case-insensitively per RFC 3986 §3.1 (schemes are ASCII
// case-insensitive). Browsers normalize HTTP:///HTTPS:// before any network
// activity, so a case-sensitive prefix test misclassifies such URLs as
// relative/internal. Shared by IsExternalURL, NormalizeBaseURL, and IsValidURL
// so the three stay consistent.
func hasHTTPScheme(url string) bool {
	return (len(url) >= 7 && strings.EqualFold(url[:7], "http://")) ||
		(len(url) >= 8 && strings.EqualFold(url[:8], "https://"))
}

// authorityLen returns the length of the authority component at the start of
// s, where s begins after the scheme (see schemeEnd): the span up to the
// first path '/', query '?', or fragment '#' delimiter.
func authorityLen(s string) int {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '/', '?', '#':
			return i
		}
	}
	return len(s)
}

// ExtractBaseFromURL extracts the base URL (scheme://domain/) from a URL.
// Returns the base URL including trailing slash, or empty string for invalid URLs.
// For an external (http/https or protocol-relative) input, NormalizeBaseURL's
// authority-truncation branch produces exactly this result — the body used to
// be a hand copy of it that had already drifted on protocol-relative inputs.
func ExtractBaseFromURL(url string) string {
	if !IsExternalURL(url) {
		return ""
	}
	return NormalizeBaseURL(url)
}

// NormalizeBaseURL ensures a base URL ends with a slash.
// Returns empty string for non-HTTP URLs (javascript:, data:, mailto:, etc.).
func NormalizeBaseURL(baseURL string) string {
	if baseURL == "" {
		return ""
	}

	// Skip non-HTTP URLs like data:, javascript:, mailto:, etc. Detect the
	// scheme by its RFC 3986 §3.1 shape (hasSchemePrefix, case-insensitive),
	// not by the mere presence of ':': a protocol-relative base with a port
	// or userinfo ("//static.example.com:8080/") contains ':' yet has no
	// scheme, and the Contains check rejected it — disabling URL resolution
	// for every link on such pages — while the port-less form worked.
	if hasSchemePrefix(baseURL) && !hasHTTPScheme(baseURL) {
		return ""
	}

	// For HTTP/HTTPS URLs, find the domain portion and ensure trailing slash
	if IsExternalURL(baseURL) {
		start := schemeEnd(baseURL)

		// Cut at the first '/', '?', or '#' after the authority so an
		// authority directly followed by a query or fragment normalizes to
		// "scheme://host/" rather than "scheme://host?q=1/" (which corrupted
		// every relative URL resolved against it).
		if e := authorityLen(baseURL[start:]); e < len(baseURL)-start {
			if baseURL[start+e] == '/' {
				// Already has a path, return up to and including the first slash
				return baseURL[:start+e+1]
			}
			return baseURL[:start+e] + "/"
		}

		// No path found, add trailing slash
		return baseURL + "/"
	}

	// For relative paths, just ensure trailing slash
	lastSlash := strings.LastIndexByte(baseURL, '/')
	if lastSlash < 0 {
		return baseURL + "/"
	}

	if lastSlash < len(baseURL)-1 {
		return baseURL[:lastSlash+1]
	}

	return baseURL
}

// hasSchemePrefix reports whether s begins with an ASCII scheme prefix such
// as "mailto:", "tel:", "ftp:", or "data:": a letter followed by letters,
// digits, '+', '-', or '.', terminated by ':' before any '/', '?', or '#'
// (RFC 3986 §3.1). Such a reference is absolute by definition even though
// IsExternalURL — which recognizes only http(s) and protocol-relative forms —
// returns false for it.
func hasSchemePrefix(s string) bool {
	if len(s) < 2 || !isASCIIAlphaByte(s[0]) {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ':':
			return true
		case isASCIIAlphaByte(c), c >= '0' && c <= '9', c == '+', c == '-', c == '.':
			// scheme character
		default:
			return false
		}
	}
	return false
}

// isASCIIAlphaByte reports whether c is an ASCII letter.
func isASCIIAlphaByte(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// ResolveURL resolves a relative URL against a base URL.
// Handles absolute URLs, protocol-relative URLs, absolute paths, and relative paths.
func ResolveURL(baseURL, relativeURL string) string {
	if relativeURL == "" || baseURL == "" {
		return relativeURL
	}

	// If already absolute, return as-is
	if IsExternalURL(relativeURL) {
		return relativeURL
	}

	// Note: protocol-relative URLs ("//example.com/path") are caught by
	// IsExternalURL above and never reach here, so no dedicated branch is needed.

	// Any other scheme-bearing reference (mailto:, tel:, ftp:, data:, ...) is
	// absolute by definition. Concatenating it onto the base — the previous
	// behavior, since IsExternalURL only knows http(s) — produced garbage such
	// as "https://host/mailto:user@example.com" from href="mailto:...".
	if hasSchemePrefix(relativeURL) {
		return relativeURL
	}

	// Handle absolute paths (/path)
	if len(relativeURL) > 0 && relativeURL[0] == '/' {
		// schemeEnd handles both "scheme://host/…" and protocol-relative
		// "//host/…" bases. The previous raw strings.Index(baseURL, "://")
		// scan never matched a protocol-relative base, silently dropping it
		// and returning the bare path.
		if start := schemeEnd(baseURL); start > 0 {
			// e is the offset of the first '/', '?', or '#' after the
			// authority. Cutting there and appending the rooted reference
			// drops any query or fragment tail: base "https://e.com?x=1" +
			// "/img.png" previously produced "https://e.com?x=1/img.png".
			if e := authorityLen(baseURL[start:]); e < len(baseURL)-start {
				return baseURL[:start+e] + relativeURL
			}
			return baseURL + relativeURL
		}
		return relativeURL
	}

	// Fragment-only ("#frag") and query-only ("?q") references preserve the
	// base path per RFC 3986 §5.3. Treating the base as a directory (as relative
	// paths do below) would wrongly drop its last segment: ".../page.html" +
	// "#top" would become ".../#top" instead of ".../page.html#top". Strip the
	// base's existing query/fragment as appropriate and append the reference.
	switch relativeURL[0] {
	case '#':
		basePath := baseURL
		if i := strings.IndexByte(basePath, '#'); i >= 0 {
			basePath = basePath[:i]
		}
		return basePath + relativeURL
	case '?':
		// RFC 3986 §5.3: a query reference keeps the base path and replaces
		// the query — the base's fragment goes too. Cut at whichever of '?'
		// or '#' comes first: scanning for '?' alone let a '?' inside the
		// fragment win ("http://e.com/p#f?x" + "?q=1" produced
		// "http://e.com/p#f?q=1", a query appended after the fragment).
		basePath := baseURL
		if q := strings.IndexByte(basePath, '?'); q >= 0 {
			basePath = basePath[:q]
		}
		if f := strings.IndexByte(basePath, '#'); f >= 0 {
			basePath = basePath[:f]
		}
		return basePath + relativeURL
	}

	// Handle relative paths (path or ./path).
	//
	// The base is treated as a directory: a relative reference is appended to it
	// verbatim. A base that does not end in '/' (e.g. a file-style base such as
	// "https://host/section/page.html") would otherwise yield a broken URL
	// (".../page.htmlabout.html"), so drop its last path segment first. Bases
	// ending in '/' are the documented contract and are left untouched, which
	// preserves the existing behavior for "./" and "../" references (their dot
	// segments are intentionally NOT collapsed).
	baseURL = asDirectoryBase(baseURL)
	return baseURL + relativeURL
}

// asDirectoryBase ensures baseURL is suitable for appending a relative path to.
// If it already ends in '/', it is returned unchanged. Otherwise the last path
// segment is dropped (file-style base → its directory). For an authority with
// no path (e.g. "http://host") a trailing '/' is appended.
func asDirectoryBase(baseURL string) string {
	if strings.HasSuffix(baseURL, "/") {
		return baseURL
	}
	pathStart := schemeEnd(baseURL)
	if lastSlash := strings.LastIndexByte(baseURL[pathStart:], '/'); lastSlash >= 0 {
		return baseURL[:pathStart+lastSlash+1]
	}
	// Authority with no path component. A query or fragment sitting directly
	// behind the authority is not a path: drop it before appending the root
	// slash ("https://e.com?q=1" -> "https://e.com/", not "https://e.com?q=1/").
	if e := authorityLen(baseURL[pathStart:]); e < len(baseURL)-pathStart {
		return baseURL[:pathStart+e] + "/"
	}
	return baseURL + "/"
}
