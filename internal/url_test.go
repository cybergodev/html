// Package internal provides tests for URL utility functions.
package internal

import "testing"

func TestExtractDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "HTTP URL with path",
			url:  "http://example.com/path",
			want: "example.com",
		},
		{
			name: "HTTPS URL with path",
			url:  "https://example.com/path/to/page",
			want: "example.com",
		},
		{
			name: "HTTP URL without path",
			url:  "http://example.com",
			want: "example.com",
		},
		{
			name: "HTTPS URL without path",
			url:  "https://example.com",
			want: "example.com",
		},
		{
			name: "protocol-relative URL with path",
			url:  "//example.com/path",
			want: "example.com",
		},
		{
			name: "protocol-relative URL without path",
			url:  "//example.com",
			want: "example.com",
		},
		{
			name: "URL with port",
			url:  "http://example.com:8080/path",
			want: "example.com:8080",
		},
		{
			name: "URL with subdomain",
			url:  "https://sub.example.com/path",
			want: "sub.example.com",
		},
		{
			name: "relative path returns empty (no domain)",
			url:  "/path/to/page",
			want: "",
		},
		{
			name: "empty string",
			url:  "",
			want: "",
		},
		{
			name: "no scheme and no leading slash",
			url:  "example.com",
			want: "example.com",
		},
		{
			name: "complex URL with query and fragment",
			url:  "https://example.com/path?query=value#fragment",
			want: "example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractDomain(tt.url)
			if got != tt.want {
				t.Errorf("ExtractDomain(%q) = %q, want %q", tt.url, got, tt.want)
			}
		})
	}
}

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

func TestIsDifferentDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		baseURL   string
		targetURL string
		want      bool
	}{
		{
			name:      "same domain HTTP",
			baseURL:   "http://example.com/path1",
			targetURL: "http://example.com/path2",
			want:      false,
		},
		{
			name:      "same domain HTTPS",
			baseURL:   "https://example.com/path1",
			targetURL: "https://example.com/path2",
			want:      false,
		},
		{
			name:      "different domains",
			baseURL:   "http://example.com/path",
			targetURL: "http://other.com/path",
			want:      true,
		},
		{
			name:      "different subdomains same domain",
			baseURL:   "http://sub1.example.com/path",
			targetURL: "http://sub2.example.com/path",
			want:      true,
		},
		{
			name:      "HTTP vs HTTPS same domain",
			baseURL:   "http://example.com/path",
			targetURL: "https://example.com/path",
			want:      false,
		},
		{
			name:      "same domain with port",
			baseURL:   "http://example.com:8080/path",
			targetURL: "http://example.com:8080/other",
			want:      false,
		},
		{
			name:      "same domain different ports",
			baseURL:   "http://example.com:8080/path",
			targetURL: "http://example.com:9090/other",
			want:      true,
		},
		{
			name:      "base URL is relative path",
			baseURL:   "/path/to/page",
			targetURL: "http://example.com/other",
			want:      false,
		},
		{
			name:      "target URL is relative path",
			baseURL:   "http://example.com/path",
			targetURL: "/other",
			want:      false,
		},
		{
			name:      "both are relative paths",
			baseURL:   "/path1",
			targetURL: "/path2",
			want:      false,
		},
		{
			name:      "protocol-relative URLs same domain",
			baseURL:   "//example.com/path1",
			targetURL: "//example.com/path2",
			want:      false,
		},
		{
			name:      "protocol-relative URLs different domains",
			baseURL:   "//example.com/path",
			targetURL: "//other.com/path",
			want:      true,
		},
		{
			name:      "empty base URL",
			baseURL:   "",
			targetURL: "http://example.com/path",
			want:      false,
		},
		{
			name:      "empty target URL",
			baseURL:   "http://example.com/path",
			targetURL: "",
			want:      false,
		},
		{
			name:      "both empty URLs",
			baseURL:   "",
			targetURL: "",
			want:      false,
		},
		{
			name:      "base is javascript URL",
			baseURL:   "javascript:void(0)",
			targetURL: "http://example.com/path",
			want:      false,
		},
		{
			name:      "target is javascript URL",
			baseURL:   "http://example.com/path",
			targetURL: "javascript:void(0)",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsDifferentDomain(tt.baseURL, tt.targetURL)
			if got != tt.want {
				t.Errorf("IsDifferentDomain(%q, %q) = %v, want %v", tt.baseURL, tt.targetURL, got, tt.want)
			}
		})
	}
}

// TestIsExternalURL_CaseInsensitiveScheme verifies scheme matching is
// case-insensitive per RFC 3986 §3.1, matching browser behavior. A
// case-sensitive test misclassifies uppercase-scheme URLs as relative/internal.
func TestIsExternalURL_CaseInsensitiveScheme(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		url  string
		want bool
	}{
		{"lowercase http", "http://example.com", true},
		{"uppercase http", "HTTP://example.com", true},
		{"mixedcase http", "Http://example.com", true},
		{"uppercase https", "HTTPS://example.com", true},
		{"mixedcase https", "hTtPs://example.com", true},
		{"protocol-relative", "//example.com", true},
		{"relative path", "/path/page", false},
		{"schemeless", "example.com", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsExternalURL(tt.url); got != tt.want {
				t.Errorf("IsExternalURL(%q) = %v, want %v", tt.url, got, tt.want)
			}
		})
	}
}

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
