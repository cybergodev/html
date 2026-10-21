package html

// external_link_at_regression_test.go — end-to-end regression for the
// D-002 P1 ExtractDomain fix: a literal '@' in a URL path ("/@alice" profile
// URLs) used to be scanned as userinfo, so the link's domain was extracted as
// "alice", IsDifferentDomain flagged it as external, and with
// IncludeExternalLinks=false the same-site link silently vanished from
// Result.Links.

import (
	"testing"
)

func TestSameSitePathUserLinkKeptWhenExternalFiltered(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.BaseURL = "https://example.com/page"
	cfg.IncludeExternalLinks = false

	doc := `<html><body>
		<a href="/@alice">alice profile</a>
		<a href="/about">about</a>
		<a href="https://other.example.net/x">external</a>
	</body></html>`

	res, err := ExtractAllLinks([]byte(doc), cfg)
	if err != nil {
		t.Fatalf("ExtractAllLinks: %v", err)
	}

	has := func(url string) bool {
		for _, l := range res {
			if l.URL == url {
				return true
			}
		}
		return false
	}

	if !has("https://example.com/@alice") {
		t.Errorf("same-site /@alice link was dropped (got %d links)", len(res))
	}
	if !has("https://example.com/about") {
		t.Errorf("same-site /about link was dropped (got %d links)", len(res))
	}
	if has("https://other.example.net/x") {
		t.Error("external link was kept despite IncludeExternalLinks=false")
	}
}
