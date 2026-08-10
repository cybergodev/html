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
// This centralizes the duplicated scheme-parsing logic previously inlined in
// ExtractDomain, ExtractBaseFromURL, NormalizeBaseURL, and asDirectoryBase.
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

// ExtractDomain extracts the host portion of a URL (e.g. "example.com" for
// "https://example.com/path"), stripping the scheme and path. When the input
// has no "://" or leading "//", the scheme is treated as absent and the input
// is returned unchanged (the whole string is the "host").
func ExtractDomain(url string) string {
	start := schemeEnd(url)

	// Find the end of the domain (first slash)
	if pathStart := strings.IndexByte(url[start:], '/'); pathStart >= 0 {
		return url[start : start+pathStart]
	}

	// No path found, return everything after scheme
	return url[start:]
}

// ExtractBaseFromURL extracts the base URL (scheme://domain/) from a URL.
// Returns the base URL including trailing slash, or empty string for invalid URLs.
func ExtractBaseFromURL(url string) string {
	if !IsExternalURL(url) {
		return ""
	}

	start := schemeEnd(url)

	// Find the first slash after the domain
	if pathStart := strings.IndexByte(url[start:], '/'); pathStart >= 0 {
		return url[:start+pathStart+1]
	}

	// No path found, add trailing slash
	return url + "/"
}

// NormalizeBaseURL ensures a base URL ends with a slash.
// Returns empty string for non-HTTP URLs (javascript:, data:, mailto:, etc.).
func NormalizeBaseURL(baseURL string) string {
	if baseURL == "" {
		return ""
	}

	// Skip non-HTTP URLs like data:, javascript:, mailto:, etc.
	// Scheme comparison is case-insensitive (RFC 3986 §3.1).
	if strings.Contains(baseURL, ":") && !hasHTTPScheme(baseURL) {
		return ""
	}

	// For HTTP/HTTPS URLs, find the domain portion and ensure trailing slash
	if IsExternalURL(baseURL) {
		start := schemeEnd(baseURL)

		// Find the first slash after the domain
		if pathStart := strings.IndexByte(baseURL[start:], '/'); pathStart >= 0 {
			// Already has a path, return up to and including the first slash
			return baseURL[:start+pathStart+1]
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

	// Handle absolute paths (/path)
	if len(relativeURL) > 0 && relativeURL[0] == '/' {
		if idx := strings.Index(baseURL, "://"); idx >= 0 {
			domainEnd := strings.IndexByte(baseURL[idx+3:], '/')
			if domainEnd >= 0 {
				return baseURL[:idx+3+domainEnd] + relativeURL
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
		basePath := baseURL
		if i := strings.IndexByte(basePath, '?'); i >= 0 {
			basePath = basePath[:i]
		} else if i := strings.IndexByte(basePath, '#'); i >= 0 {
			basePath = basePath[:i]
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
	// Authority with no path component.
	return baseURL + "/"
}

// IsDifferentDomain checks if two URLs have different domains.
// Returns false if either URL is not external.
func IsDifferentDomain(baseURL, targetURL string) bool {
	if !IsExternalURL(baseURL) || !IsExternalURL(targetURL) {
		return false
	}

	baseDomain := ExtractDomain(baseURL)
	targetDomain := ExtractDomain(targetURL)

	return baseDomain != targetDomain
}
