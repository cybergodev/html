package html

// media_attr_quote_regression_test.go — regression for the D-002 P1 fix in
// extractAttributeValue: the raw-HTML attribute scanner matched an attribute
// name inside another attribute's quoted value and returned the first match,
// so title="video src=here" hijacked the scan and the iframe's real src was
// never read (with a full URL in the decoy, the decoy replaced the real src
// outright). Under the default config the sanitizer removes iframe elements
// before the DOM walk, so this raw scan is the only source of those URLs.
//
// Note: a URL-shaped decoy inside an attribute value may still be reported
// by ScanMediaURLs, the separate raw-text media scanner — that is by design
// (it scans any URL in raw HTML) and independent of this fix. What this test
// pins is that the REAL src attribute always wins the attribute scan.

import (
	"strings"
	"testing"
)

func TestIframeSourceNotHijackedByQuotedTitle(t *testing.T) {
	t.Parallel()

	hasVideo := func(res *Result, substr string) bool {
		for _, v := range res.Videos {
			if strings.Contains(v.URL, substr) {
				return true
			}
		}
		return false
	}

	// Decoy attribute value that reads like an unquoted source; pre-fix, the
	// scanner returned `here"` from inside the title, failed validation, and
	// returned — the real src was never read.
	doc := `<html><body><iframe title="video src=here" src="https://cdn.example.com/embed/real.mp4"></iframe></body></html>`
	res, err := Extract([]byte(doc))
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}
	if !hasVideo(res, "cdn.example.com/embed/real.mp4") {
		t.Errorf("real iframe src not extracted (videos: %v)", res.Videos)
	}

	// Decoy containing a fully valid-looking video URL inside the title:
	// pre-fix the decoy won the first-match race and real2 was lost entirely.
	doc2 := `<html><body><iframe title="demo src=https://evil.example.com/x.mp4 here" src="https://cdn.example.com/embed/real2.mp4"></iframe></body></html>`
	res2, err := Extract([]byte(doc2))
	if err != nil {
		t.Fatalf("Extract(2): %v", err)
	}
	if !hasVideo(res2, "cdn.example.com/embed/real2.mp4") {
		t.Errorf("real iframe src lost to a quoted-value decoy (videos: %v)", res2.Videos)
	}
}
