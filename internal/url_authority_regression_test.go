package internal

// url_authority_regression_test.go — pins the D-002 P1 authority-handling
// fixes: ExtractDomain no longer scans '@' past the authority (profile URLs
// like "/@alice" were misread as userinfo, extracting "alice" as the domain
// and flagging same-site links as external), and the four base-URL builders
// (ExtractBaseFromURL, NormalizeBaseURL, ResolveURL's rooted-reference
// branch, asDirectoryBase) cut at the first '/', '?', or '#' after the
// authority so a query/fragment-only tail can no longer end up inside the
// base ("https://e.com?x=1/" corrupted every relative URL resolved against
// it).

import "testing"

func TestAuthorityDelimiters(t *testing.T) {
	t.Parallel()

	domainCases := []struct {
		url  string
		want string
	}{
		// '@' in the path is not userinfo.
		{"https://example.com/@alice", "example.com"},
		{"https://example.com/@alice/status/42", "example.com"},
		// userinfo inside the authority still strips.
		{"https://user:pass@example.com/path", "example.com"},
		// query/fragment after the authority no longer leaks into the domain.
		{"https://e.com?x=1", "e.com"},
		{"https://e.com#top", "e.com"},
		{"https://e.com/", "e.com"},
		{"https://e.com/path", "e.com"},
	}
	for _, c := range domainCases {
		if got := ExtractDomain(c.url); got != c.want {
			t.Errorf("ExtractDomain(%q) = %q, want %q", c.url, got, c.want)
		}
	}

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

// TestSameSitePathUserLinkNotExternal pins the user-visible effect: with
// IncludeExternalLinks disabled, a same-site "/@user" profile link must be
// kept, while a genuinely external link must still be filtered.
func TestSameSitePathUserLinkNotExternal(t *testing.T) {
	t.Parallel()

	if IsDifferentDomain("https://example.com/page", "https://example.com/@alice") {
		t.Error("IsDifferentDomain misclassified a same-site /@user link as external")
	}
	if !IsDifferentDomain("https://example.com/page", "https://other.example.net/x") {
		t.Error("IsDifferentDomain failed to classify a cross-site link as external")
	}
}
