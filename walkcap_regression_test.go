package html

// walkcap_regression_test.go — regression for the D-002 P0 fix:
// extractImagesAndLinks used the maxWalkNodes (50,000)-capped WalkNodes
// while the placeholder text pass (extractTextWithStructure) recursed
// uncapped. On documents past the cap the collector silently dropped every
// img/a past node 50,000, but the text pass kept emitting [IMAGE:n]/
// [LINK:n] placeholders for them — the unmatched image tokens leaked into
// Text verbatim and the link URLs were silently discarded. The collector
// now walks uncapped (internal.WalkAllNodes), restoring the documented
// lockstep between the two passes.

import (
	"strings"
	"testing"
)

func TestImagesLinksBeyondWalkCapCollected(t *testing.T) {
	t.Parallel()

	// Each "<p>filler</p>" contributes an element node plus a text node, so
	// 30,000 paragraphs put the DOM well past the old 50,000-node walk cap
	// before the tail paragraph — the image and link below land past the
	// point where the capped collector used to stop.
	var b strings.Builder
	b.WriteString("<html><body>")
	for i := 0; i < 30000; i++ {
		b.WriteString("<p>filler</p>")
	}
	b.WriteString(`<p><img src="/tail.png" alt="tail"><a href="/tail.html">tail link</a></p>`)
	b.WriteString("</body></html>")

	cfg := DefaultConfig()
	cfg.InlineImageFormat = "markdown"
	cfg.InlineLinkFormat = "markdown"
	cfg.MaxCacheEntries = 0

	res, err := Extract([]byte(b.String()), cfg)
	if err != nil {
		t.Fatalf("Extract: %v", err)
	}

	foundImg := false
	for _, img := range res.Images {
		if img.URL == "/tail.png" {
			foundImg = true
			break
		}
	}
	if !foundImg {
		t.Errorf("image past the 50k-node cap was not collected (len(Images)=%d)", len(res.Images))
	}

	foundLink := false
	for _, l := range res.Links {
		if l.URL == "/tail.html" {
			foundLink = true
			break
		}
	}
	if !foundLink {
		t.Errorf("link past the 50k-node cap was not collected (len(Links)=%d)", len(res.Links))
	}

	// No unmatched placeholder token may survive into the final text.
	if strings.Contains(res.Text, "[IMAGE:") {
		t.Error("literal [IMAGE:n] placeholder leaked into Text")
	}
	if strings.Contains(res.Text, "[LINK:") {
		t.Error("literal [LINK:n] placeholder leaked into Text")
	}

	// The tail media must be formatted inline from its collected position.
	if !strings.Contains(res.Text, "![tail](/tail.png)") {
		t.Error("tail image was not formatted inline as markdown")
	}
	if !strings.Contains(res.Text, "[tail link](/tail.html)") {
		t.Error("tail link was not formatted inline as markdown")
	}
}
