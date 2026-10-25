// Package html_test provides fuzz tests for the html package.
// These tests verify that the parser handles arbitrary input without panicking.
package html_test

import (
	"strings"
	"testing"

	"github.com/cybergodev/html"
)

// FuzzExtract tests that Extract handles arbitrary input without panicking.
func FuzzExtract(f *testing.F) {
	// Seed corpus with valid and edge-case inputs
	seeds := []string{
		`<html><body><p>Valid HTML</p></body></html>`,
		"",
		"<>",
		"<html><body>",
		"<div>Unclosed",
		"<script>alert(1)</script>",
		strings.Repeat("x", 1024*1024),   // 1MB of data
		"\x00\x01\x02\x03",               // Binary data
		string([]byte{0xFF, 0xFE, 0xFD}), // Invalid UTF-8
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		p, err := html.New()
		if err != nil {
			t.Fatalf("Failed to create processor: %v", err)
		}
		defer func() { _ = p.Close() }()

		// Should never panic
		result, err := p.Extract([]byte(input))

		// If no error, result should be valid
		if err == nil && result == nil {
			t.Error("Nil result with no error")
		}
	})
}

// FuzzEncodingDetection tests encoding detection robustness.
func FuzzEncodingDetection(f *testing.F) {
	seeds := [][]byte{
		[]byte(`<meta charset="utf-8"><body>Test</body>`),
		[]byte(`<meta http-equiv="Content-Type" content="text/html; charset=iso-8859-1">`),
		{0xEF, 0xBB, 0xBF, '<', 'h', 't', 'm', 'l', '>'}, // UTF-8 BOM
		{0xFF, 0xFE, '<', 0, 'h', 0},                     // UTF-16 LE BOM
		[]byte(`<html><body>Simple content</body></html>`),
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input []byte) {
		p, err := html.New()
		if err != nil {
			t.Fatalf("Failed to create processor: %v", err)
		}
		defer func() { _ = p.Close() }()

		// Should handle any byte sequence without panic
		_, _ = p.Extract(input)
	})
}

// FuzzTableParsing tests table extraction robustness.
func FuzzTableParsing(f *testing.F) {
	seeds := []string{
		`<table><tr><td>Cell</td></tr></table>`,
		`<table><tr><td colspan="2">Span</td></tr></table>`,
		`<table><tr><td rowspan="999">Row</td></tr></table>`,
		`<table><tr><td width="1000000px">Wide</td></tr></table>`,
		`<table><tr></tr></table>`,
		`<table></table>`,
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, tableHTML string) {
		cfg := html.DefaultConfig()
		cfg.TableFormat = "markdown"
		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("Failed to create processor: %v", err)
		}
		defer func() { _ = p.Close() }()

		htmlContent := `<html><body>` + tableHTML + `</body></html>`
		result, _ := p.Extract([]byte(htmlContent))

		// Result text should not contain unescaped HTML tags if table was processed
		if result != nil && strings.Contains(result.Text, "<table>") {
			t.Errorf("Markdown output contains HTML tags")
		}
	})
}

// FuzzExtractAllLinks tests link extraction robustness.
func FuzzExtractAllLinks(f *testing.F) {
	seeds := []string{
		`<a href="https://example.com">Link</a>`,
		`<img src="image.jpg">`,
		`<script src="script.js"></script>`,
		`<link rel="stylesheet" href="style.css">`,
		`<a href="javascript:alert(1)">XSS</a>`,
	}

	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		p, err := html.New()
		if err != nil {
			t.Fatalf("Failed to create processor: %v", err)
		}
		defer func() { _ = p.Close() }()

		htmlContent := `<html><body>` + input + `</body></html>`
		_, err = p.ExtractAllLinks([]byte(htmlContent))

		// Should handle any input without panic
		if err != nil {
			// Error is acceptable for malformed input
			t.Logf("Got error (acceptable): %v", err)
		}
	})
}

// FuzzMediaExtraction exercises the raw-HTML media scanner (the regex-free
// ScanMediaURLs path) with arbitrary input: URL-ish fragments, truncated
// schemes, mixed-case extensions, and hostile byte soup. The scanner's
// index arithmetic is hand-rolled, so this target is its empirical
// no-panic/no-index-out-of-range guarantee.
func FuzzMediaExtraction(f *testing.F) {
	seeds := []string{
		`<video src="https://cdn.example.com/v/1.mp4"></video>`,
		`<a href="HTTPS://X.TEST/A.MP4">x</a>`,
		`https://h.test/` + strings.Repeat("a.", 300) + `mp4`,
		`hTtPs://x.test/a.Mp4?t=1#f`,
		`https://x.test/a.mp4.mp3.webm.ogg`,
		`//protocol-relative.test/v.mp3`,
		`https://`,
		`https://x`,
		`https://x.`,
		`h`,
		`ht`,
		`htt`,
		`http`,
		`http:`,
		`http:/`,
		`http://`,
		`https://\backslash.path.mp4`,
		`https://x.test/"quoted',;)}].mp4`,
		strings.Repeat("https://a.mp4", 200),
		"\x00\x01https://\xff\xfe.mp4",
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		cfg := html.DefaultConfig()
		cfg.MaxCacheEntries = 0
		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("Failed to create processor: %v", err)
		}
		defer func() { _ = p.Close() }()

		htmlContent := `<html><body><article>` + input + `</article></body></html>`
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			return
		}
		if result == nil {
			t.Error("Nil result with no error")
		}
	})
}

// FuzzReadPage exercises the reader-mode pipeline — segmentation, kind-aware
// section cleaning, [LINK:n]/[IMAGE:n] token emission, inline substitution,
// and the token strippers behind PlainText — with arbitrary input. The
// strippers (stripLinkTokens/stripImageTokens) and the section merge/dedup
// logic are hand-rolled index arithmetic over attacker-shaped text, so this
// target is their empirical no-panic guarantee, matching the SEC-003 posture
// of the other fuzz targets.
func FuzzReadPage(f *testing.F) {
	seeds := []string{
		`<html><body><header><nav><a href="/a">a</a></nav></header>` +
			`<main><p>hi <a href="/b">b</a> <img src="i.png" alt="pic"></p></main>` +
			`<footer>f</footer></body></html>`,
		`<table><tr><td rowspan="3" colspan="2">s</td></tr><tr></tr></table>`,
		`[LINK:1] literal text [LINK:] more [LINK:abc]`,
		`[IMAGE:999] literal [IMAGE: no digits`,
		`<div role="navigation" class="menu">menu</div>`,
		`<base href="https://x.test/"><a href="rel">r</a>`,
		`<main class="content hidden"><p>x</p></main>`,
		`<aside><a href="/1">1</a> <a href="/2">2</a> <a href="/3">3</a></aside>`,
		`<table><caption>[LINK:1]</caption><tr><td>[IMAGE:2]</td></tr></table>`,
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, input string) {
		p, err := html.New()
		if err != nil {
			t.Fatalf("Failed to create processor: %v", err)
		}
		defer func() { _ = p.Close() }()

		page, err := p.ReadPage([]byte(input))
		if err != nil {
			return
		}
		if page == nil {
			t.Fatal("Nil ReadResult with no error")
		}
		// The flatteners walk whatever section set the pipeline produced
		// (unknown kinds, empty text, links-only sections included).
		_ = page.Render()
		_ = page.RenderText()
	})
}
