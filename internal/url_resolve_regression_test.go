package internal

// url_resolve_regression_test.go — regression tests for ResolveURL's handling
// of scheme-bearing references and protocol-relative bases. A reference like
// "mailto:user@example.com" is absolute by definition (RFC 3986 §3.1) but is
// not recognized by IsExternalURL (which knows only http(s) and "//"), so it
// used to fall through to directory concatenation and come out as
// "https://host/mailto:user@example.com". Protocol-relative bases ("//host/")
// used to be dropped entirely when resolving root-relative paths because the
// branch scanned for the literal "://".

import "testing"

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
