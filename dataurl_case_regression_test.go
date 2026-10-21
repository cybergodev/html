package html_test

// dataurl_case_regression_test.go — regression tests for RFC 3986
// case-insensitive scheme handling of data: URLs. The prefix check used to be
// case-sensitive in both IsValidURL and isValidDataURLWithAudit, so
// "DATA:text/html,…" bypassed the scriptable-media-type whitelist in
// ExtractAllLinks while valid "DATA:image/png" images were silently dropped
// by the sanitizer without an audit event.

import (
	"strings"
	"testing"

	"github.com/cybergodev/html"
)

// TestScriptableDataURLCaseInsensitive verifies that uppercase/mixed-case
// scriptable data URLs are rejected from link extraction exactly like their
// lowercase form.
func TestScriptableDataURLCaseInsensitive(t *testing.T) {
	t.Parallel()

	p, err := html.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	doc := `<html><body>
<a href="DATA:text/html,%3Cscript%3Ealert(1)%3E/script%3E">evil-upper</a>
<a href="Data:image/svg+xml,%3Csvg%3E">svg-mixed</a>
<a href="data:text/html,%3Cscript%3Ealert(1)%3E/script%3E">evil-lower</a>
<a href="https://go.dev">safe</a>
</body></html>`
	links, err := p.ExtractAllLinks([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(links) != 1 || links[0].URL != "https://go.dev" {
		for _, l := range links {
			low := strings.ToLower(l.URL)
			if strings.HasPrefix(low, "data:text/html") || strings.HasPrefix(low, "data:image/svg") {
				t.Errorf("scriptable data URL returned by ExtractAllLinks: %s", l.URL)
			}
		}
		t.Errorf("expected only the safe link, got %v", links)
	}
}

// TestUppercaseSafeDataURLImageExtracted verifies the functionality half of
// the fix: browsers accept "DATA:" schemes, so a whitelisted media type in
// any case must survive sanitization and appear in Images.
func TestUppercaseSafeDataURLImageExtracted(t *testing.T) {
	t.Parallel()

	cfg := html.DefaultConfig()
	cfg.BaseURL = "https://site.com/page"
	p, err := html.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	res, err := p.Extract([]byte(`<html><body><img src="DATA:image/png;base64,iVBORw0KGgo=" alt="dot"></body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Images) != 1 || res.Images[0].URL != "DATA:image/png;base64,iVBORw0KGgo=" {
		t.Errorf("uppercase safe data-URL image not extracted: %+v", res.Images)
	}
}
