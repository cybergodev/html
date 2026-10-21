package html_test

// p2_regression_test.go — regression tests for the D-002 P2 fixes visible
// through the public API: ExtractToMarkdown* now forwards audit events and
// statistics to the parent processor instead of dropping them. Also covers
// the post-P2 re-review fix: raw-HTML attribute values are entity-decoded so
// media URLs extracted from sanitizer-removed tags match their DOM-extracted
// counterparts.

import (
	"strings"
	"testing"

	"github.com/cybergodev/html"
)

// TestRawHTMLMediaURLsEntityDecoded: the sanitizer removes <iframe>, so its
// src is extracted from the raw pre-parse HTML; the same &amp; in a surviving
// <video> is decoded by the parser. Both paths must produce identical
// entity handling.
func TestRawHTMLMediaURLsEntityDecoded(t *testing.T) {
	t.Parallel()

	p, err := html.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	res, err := p.Extract([]byte(`<html><body>
<iframe src="https://cdn.example.com/embed/a.mp4?x=1&amp;y=2"></iframe>
</body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range res.Videos {
		if strings.Contains(v.URL, "&amp;") {
			t.Errorf("raw-extracted media URL kept undecoded entity: %q", v.URL)
		}
		if strings.Contains(v.URL, "x=1&y=2") {
			return // found the decoded iframe URL
		}
	}
	t.Errorf("iframe media URL with decoded query not found: %+v", res.Videos)
}

func TestExtractToMarkdownForwardsAudit(t *testing.T) {
	t.Parallel()

	cfg := html.DefaultConfig()
	cfg.Audit.Enabled = true
	p, err := html.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	_, err = p.ExtractToMarkdown([]byte(`<html><body><p>hi</p><script>evil()</script></body></html>`))
	if err != nil {
		t.Fatal(err)
	}

	var sawBlockedTag bool
	for _, e := range p.GetAuditLog() {
		if e.EventType == html.AuditEventBlockedTag && e.Tag == "script" {
			sawBlockedTag = true
		}
	}
	if !sawBlockedTag {
		t.Error("ExtractToMarkdown sanitize events missing from GetAuditLog (audit blind spot)")
	}

	stats := p.GetStatistics()
	if stats.TotalProcessed == 0 {
		t.Error("ExtractToMarkdown run not reflected in GetStatistics")
	}

	// And the plain Extract path still audits, so coverage is now uniform.
	p.ClearAuditLog()
	if _, err := p.Extract([]byte(`<html><body><p>x</p><script>evil()</script></body></html>`)); err != nil {
		t.Fatal(err)
	}
	sawBlockedTag = false
	for _, e := range p.GetAuditLog() {
		if e.EventType == html.AuditEventBlockedTag && strings.EqualFold(e.Tag, "script") {
			sawBlockedTag = true
		}
	}
	if !sawBlockedTag {
		t.Error("Extract sanitize events missing from GetAuditLog")
	}
}
