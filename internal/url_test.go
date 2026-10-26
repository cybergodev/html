// Package internal provides tests for URL utility functions.
package internal

import "testing"

func TestExtractBaseFromURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "HTTP URL with path",
			url:  "http://example.com/path/to/page",
			want: "http://example.com/",
		},
		{
			name: "HTTPS URL with path",
			url:  "https://example.com/path",
			want: "https://example.com/",
		},
		{
			name: "HTTP URL without path",
			url:  "http://example.com",
			want: "http://example.com/",
		},
		{
			name: "HTTPS URL without path",
			url:  "https://example.com",
			want: "https://example.com/",
		},
		{
			name: "protocol-relative URL with path",
			url:  "//example.com/path",
			want: "//example.com/",
		},
		{
			name: "protocol-relative URL without path",
			url:  "//example.com",
			want: "//example.com/",
		},
		{
			name: "URL with port",
			url:  "http://example.com:8080/path",
			want: "http://example.com:8080/",
		},
		{
			name: "URL with subdomain",
			url:  "https://sub.example.com/path",
			want: "https://sub.example.com/",
		},
		{
			name: "relative path returns empty",
			url:  "/path/to/page",
			want: "",
		},
		{
			name: "empty string returns empty",
			url:  "",
			want: "",
		},
		{
			name: "no scheme and no leading slash returns empty",
			url:  "example.com",
			want: "",
		},
		{
			name: "complex URL with query",
			url:  "https://example.com/path?query=value",
			want: "https://example.com/",
		},
		{
			name: "query only drops to root (from p2_regression_test.go)",
			url:  "https://example.com?ref=x",
			want: "https://example.com/",
		},
		{
			name: "path then query drops to root (from p2_regression_test.go)",
			url:  "https://example.com/p?ref=x",
			want: "https://example.com/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractBaseFromURL(tt.url)
			if got != tt.want {
				t.Errorf("ExtractBaseFromURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "HTTP URL without trailing slash",
			url:  "http://example.com",
			want: "http://example.com/",
		},
		{
			name: "HTTP URL with trailing slash",
			url:  "http://example.com/",
			want: "http://example.com/",
		},
		{
			name: "HTTPS URL without trailing slash",
			url:  "https://example.com",
			want: "https://example.com/",
		},
		{
			name: "HTTPS URL with trailing slash",
			url:  "https://example.com/",
			want: "https://example.com/",
		},
		{
			name: "HTTP URL with path",
			url:  "http://example.com/path",
			want: "http://example.com/",
		},
		{
			name: "protocol-relative URL without trailing slash",
			url:  "//example.com",
			want: "//example.com/",
		},
		{
			name: "protocol-relative URL with trailing slash",
			url:  "//example.com/",
			want: "//example.com/",
		},
		{
			name: "relative path with leading slash - truncates to last slash",
			url:  "/path/to/page",
			want: "/path/to/",
		},
		{
			name: "relative path without leading slash - truncates to last slash",
			url:  "path/to/page",
			want: "path/to/",
		},
		{
			name: "empty string returns empty",
			url:  "",
			want: "",
		},
		{
			name: "javascript URL returns empty",
			url:  "javascript:void(0)",
			want: "",
		},
		{
			name: "data URL returns empty",
			url:  "data:text/html,<html></html>",
			want: "",
		},
		{
			name: "mailto URL returns empty",
			url:  "mailto:test@example.com",
			want: "",
		},
		{
			name: "ftp URL returns empty",
			url:  "ftp://example.com",
			want: "",
		},
		{
			name: "URL with fragment",
			url:  "http://example.com/path#fragment",
			want: "http://example.com/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeBaseURL(tt.url)
			if got != tt.want {
				t.Errorf("NormalizeBaseURL(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

func TestResolveURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		baseURL     string
		relativeURL string
		want        string
	}{
		{
			name:        "absolute HTTP URL",
			baseURL:     "http://example.com/path/",
			relativeURL: "http://other.com/page",
			want:        "http://other.com/page",
		},
		{
			name:        "absolute HTTPS URL",
			baseURL:     "http://example.com/path/",
			relativeURL: "https://other.com/page",
			want:        "https://other.com/page",
		},
		{
			name:        "protocol-relative URL is returned as-is (considered external)",
			baseURL:     "http://example.com/path/",
			relativeURL: "//other.com/page",
			want:        "//other.com/page",
		},
		{
			name:        "absolute path with base",
			baseURL:     "http://example.com/path/to/page/",
			relativeURL: "/other/path",
			want:        "http://example.com/other/path",
		},
		{
			name:        "absolute path with base without trailing slash",
			baseURL:     "http://example.com/path",
			relativeURL: "/other",
			want:        "http://example.com/other",
		},
		{
			name:        "relative path with base",
			baseURL:     "http://example.com/path/",
			relativeURL: "other/page.html",
			want:        "http://example.com/path/other/page.html",
		},
		{
			name:        "relative path with dot slash",
			baseURL:     "http://example.com/path/",
			relativeURL: "./page.html",
			want:        "http://example.com/path/./page.html",
		},
		{
			name:        "relative path with double dot",
			baseURL:     "http://example.com/path/",
			relativeURL: "../page.html",
			want:        "http://example.com/path/../page.html",
		},
		{
			name:        "empty relative URL returns empty",
			baseURL:     "http://example.com/path/",
			relativeURL: "",
			want:        "",
		},
		{
			name:        "empty base URL returns relative URL",
			baseURL:     "",
			relativeURL: "page.html",
			want:        "page.html",
		},
		{
			name:        "both empty returns empty",
			baseURL:     "",
			relativeURL: "",
			want:        "",
		},
		{
			name:        "query string as relative URL",
			baseURL:     "http://example.com/path/",
			relativeURL: "?query=value",
			want:        "http://example.com/path/?query=value",
		},
		{
			name:        "fragment as relative URL",
			baseURL:     "http://example.com/path/",
			relativeURL: "#section",
			want:        "http://example.com/path/#section",
		},
		{
			// RFC 3986 §5.3: a fragment-only reference preserves the base path.
			// A file-style base must NOT be treated as a directory here, or the
			// last segment ("page.html") is dropped.
			name:        "fragment against file-style base preserves path",
			baseURL:     "http://example.com/path/page.html",
			relativeURL: "#top",
			want:        "http://example.com/path/page.html#top",
		},
		{
			name:        "query against file-style base preserves path",
			baseURL:     "http://example.com/path/page.html",
			relativeURL: "?q=1",
			want:        "http://example.com/path/page.html?q=1",
		},
		{
			name:        "query against base with existing query replaces query",
			baseURL:     "http://example.com/path/page.html?old=1",
			relativeURL: "?new=2",
			want:        "http://example.com/path/page.html?new=2",
		},
		{
			name:        "fragment against base with existing fragment replaces fragment",
			baseURL:     "http://example.com/path/page.html#old",
			relativeURL: "#new",
			want:        "http://example.com/path/page.html#new",
		},
		{
			// Absolute path against a base that has a scheme but no path after
			// the domain: the domain has no trailing '/', so the base is reused
			// verbatim and the absolute path appended to it.
			name:        "absolute path, base has scheme but no path after domain",
			baseURL:     "http://example.com",
			relativeURL: "/other/path",
			want:        "http://example.com/other/path",
		},
		{
			// Absolute path against a base with no scheme (no "://"): the base is
			// unusable, so the absolute path is returned on its own.
			name:        "absolute path, base has no scheme",
			baseURL:     "example.com",
			relativeURL: "/other/path",
			want:        "/other/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveURL(tt.baseURL, tt.relativeURL)
			if got != tt.want {
				t.Errorf("ResolveURL(%q, %q) = %q, want %q", tt.baseURL, tt.relativeURL, got, tt.want)
			}
		})
	}
}

// TestIsExternalURL_CaseInsensitiveScheme verifies scheme matching is
// case-insensitive per RFC 3986 §3.1, matching browser behavior. A
// case-sensitive test misclassifies uppercase-scheme URLs as relative/internal.
// TestNormalizeBaseURL_CaseInsensitiveScheme verifies an uppercase scheme is
// still recognized as HTTP and normalized rather than dropped as a non-HTTP URL.
func TestNormalizeBaseURL_CaseInsensitiveScheme(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"HTTP://example.com":       "HTTP://example.com/",
		"HTTPS://example.com/path": "HTTPS://example.com/",
		"HtTp://example.com":       "HtTp://example.com/",
	}
	for in, want := range cases {
		if got := NormalizeBaseURL(in); got != want {
			t.Errorf("NormalizeBaseURL(%q) = %q, want %q", in, got, want)
		}
	}
}
func TestResolveURLSchemeReferencesAreAbsolute(t *testing.T) {
	t.Parallel()
	cases := []struct{ base, ref, want string }{
		// Scheme-bearing references must be returned verbatim.
		{"https://site.com/page", "mailto:user@example.com", "mailto:user@example.com"},
		{"https://site.com/page", "tel:+1-555-0100", "tel:+1-555-0100"},
		{"https://site.com/page", "ftp://files.example.com/x", "ftp://files.example.com/x"},
		{"https://site.com/page", "data:image/png;base64,AAA", "data:image/png;base64,AAA"},
		// ...while plain relative references keep resolving as before.
		{"https://site.com/dir/page", "other.html", "https://site.com/dir/other.html"},
		{"https://site.com/dir/page", "./x.js", "https://site.com/dir/./x.js"},
		{"https://site.com/dir/", "y.css", "https://site.com/dir/y.css"},
		{"https://site.com/page", "file7.html", "https://site.com/file7.html"},
		// Not scheme references: ':' must appear before any '/', '?', '#'.
		{"https://site.com/page", "path/to:file", "https://site.com/path/to:file"},
		{"https://site.com/page", "q?a:b", "https://site.com/q?a:b"},
		{"https://site.com/page", "#frag:x", "https://site.com/page#frag:x"},
		// Digits cannot start a scheme (RFC 3986: scheme = ALPHA ...).
		{"https://site.com/dir/", "2024:report.html", "https://site.com/dir/2024:report.html"},
	}
	for _, c := range cases {
		if got := ResolveURL(c.base, c.ref); got != c.want {
			t.Errorf("ResolveURL(%q, %q) = %q, want %q", c.base, c.ref, got, c.want)
		}
	}
}

func TestResolveURLProtocolRelativeBase(t *testing.T) {
	t.Parallel()
	cases := []struct{ base, ref, want string }{
		// The base's host must survive when resolving /path references.
		{"//cdn.example.com/assets/", "/clip.mp4", "//cdn.example.com/clip.mp4"},
		{"//cdn.example.com", "/clip.mp4", "//cdn.example.com/clip.mp4"},
		// http(s) bases keep their existing behavior.
		{"https://site.com/dir/page", "/root.css", "https://site.com/root.css"},
		{"https://site.com", "/root.css", "https://site.com/root.css"},
		{"http://site.com/a/b", "/x", "http://site.com/x"},
	}
	for _, c := range cases {
		if got := ResolveURL(c.base, c.ref); got != c.want {
			t.Errorf("ResolveURL(%q, %q) = %q, want %q", c.base, c.ref, got, c.want)
		}
	}
}

// url_authority_regression_test.go — pins the D-002 P1 authority-handling
// fixes for the base-URL builders (ExtractBaseFromURL, NormalizeBaseURL,
// ResolveURL's rooted-reference branch, asDirectoryBase): they cut at the
// first '/', '?', or '#' after the authority so a query/fragment-only tail
// can no longer end up inside the base ("https://e.com?x=1/" corrupted every
// relative URL resolved against it). (The ExtractDomain userinfo half of the
// original fix was deleted with that production-dead helper.)

func TestAuthorityDelimiters(t *testing.T) {
	t.Parallel()

	baseCases := []struct {
		name string
		got  string
		want string
	}{
		{"ExtractBaseFromURL fragment", ExtractBaseFromURL("https://cdn.example.com#top"), "https://cdn.example.com/"},
		{"ExtractBaseFromURL query", ExtractBaseFromURL("https://cdn.example.com?ref=x"), "https://cdn.example.com/"},
		{"ExtractBaseFromURL path", ExtractBaseFromURL("https://cdn.example.com/p/page"), "https://cdn.example.com/"},
		{"NormalizeBaseURL query", NormalizeBaseURL("https://e.com?x=1"), "https://e.com/"},
		{"NormalizeBaseURL fragment", NormalizeBaseURL("https://e.com#top"), "https://e.com/"},
		{"NormalizeBaseURL path", NormalizeBaseURL("https://e.com/a/b"), "https://e.com/"},
		{"NormalizeBaseURL trailing slash", NormalizeBaseURL("https://e.com/"), "https://e.com/"},
		{"ResolveURL rooted over query", ResolveURL("https://example.com?x=1", "/img.png"), "https://example.com/img.png"},
		{"ResolveURL rooted over fragment", ResolveURL("https://example.com#top", "/img.png"), "https://example.com/img.png"},
		{"ResolveURL rooted over path", ResolveURL("https://example.com/section/page.html", "/img.png"), "https://example.com/img.png"},
		{"ResolveURL relative into query-only base", ResolveURL("https://host?q=1", "a/b.html"), "https://host/a/b.html"},
		{"ResolveURL relative into plain base", ResolveURL("https://host/section/", "a/b.html"), "https://host/section/a/b.html"},
	}
	for _, c := range baseCases {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, c.got, c.want)
		}
	}
}

// TestNormalizeBaseURLProtocolRelativeWithPort pins the hasSchemePrefix guard:
// a protocol-relative base with a port (or userinfo) contains ':' but has no
// scheme, and the old strings.Contains(":") rejection disabled URL resolution
// for every link on such pages while the port-less form worked.
func TestNormalizeBaseURLProtocolRelativeWithPort(t *testing.T) {
	t.Parallel()

	cases := []struct{ in, want string }{
		{"//static.example.com:8080/", "//static.example.com:8080/"},
		{"//static.example.com:8080/assets/", "//static.example.com:8080/"},
		{"//user:pass@cdn.example.com/", "//user:pass@cdn.example.com/"},
		{"//plain.example.com/", "//plain.example.com/"},
		{"javascript:x", ""},
		{"about:blank", ""},
		{"data:,x", ""},
		{"https://e.com/x", "https://e.com/"},
	}
	for _, c := range cases {
		if got := NormalizeBaseURL(c.in); got != c.want {
			t.Errorf("NormalizeBaseURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestResolveURLQueryOnlyOverFragmentBase pins the RFC 3986 §5.3
// query-reference rule on a fragment-bearing base: the base's fragment is
// dropped even when a '?' appears inside it, instead of the new query being
// appended after the fragment.
func TestResolveURLQueryOnlyOverFragmentBase(t *testing.T) {
	t.Parallel()

	cases := []struct{ base, ref, want string }{
		{"http://e.com/p#f?x", "?q=1", "http://e.com/p?q=1"},
		{"http://e.com/p#f", "?q=1", "http://e.com/p?q=1"},
		{"http://e.com/p?f=1#frag", "?q=2", "http://e.com/p?q=2"},
		{"http://e.com/p", "?q=1", "http://e.com/p?q=1"},
	}
	for _, c := range cases {
		if got := ResolveURL(c.base, c.ref); got != c.want {
			t.Errorf("ResolveURL(%q, %q) = %q, want %q", c.base, c.ref, got, c.want)
		}
	}
}

// TestAsDirectoryBase pins all three branches of asDirectoryBase: an
// already-directory base is unchanged, a file-style base drops its last
// segment, and an authority-only base gains a trailing slash.
func TestAsDirectoryBase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		base string
		want string
	}{
		{"already a directory base", "https://example.com/path/", "https://example.com/path/"},
		{"file-style base drops last segment", "https://example.com/path/page.html", "https://example.com/path/"},
		{"root file drops to root", "https://example.com/page.html", "https://example.com/"},
		{"authority without path gains slash", "http://example.com", "http://example.com/"},
		{"relative directory base unchanged", "dir/sub/", "dir/sub/"},
		{"relative file base drops to its directory", "dir/sub/page.html", "dir/sub/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := asDirectoryBase(tt.base); got != tt.want {
				t.Errorf("asDirectoryBase(%q) = %q, want %q", tt.base, got, tt.want)
			}
		})
	}
}
