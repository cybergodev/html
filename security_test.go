package html_test

// security_test.go - Comprehensive security and robustness tests
// Tests for XSS prevention, injection attacks, and malformed input handling

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/cybergodev/html"
)

// TestXSSPrevention tests that dangerous scripts are removed from content
// Consolidates basic, SVG, and MathML XSS payloads into a single table-driven test
func TestXSSPrevention(t *testing.T) {
	t.Parallel()

	p, err := html.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	// Consolidated XSS payloads organized by category
	xssPayloads := []struct {
		category string
		name     string
		html     string
	}{
		// Basic XSS payloads
		{"basic", "script tag", `<html><body><script>alert('XSS')</script><p>Content</p></body></html>`},
		{"basic", "script with src", `<html><body><script src="http://evil.com/xss.js"></script><p>Content</p></body></html>`},
		{"basic", "onclick attribute", `<html><body><div onclick="alert('XSS')">Content</div></body></html>`},
		{"basic", "javascript href", `<html><body><a href="javascript:alert('XSS')">Click</a><p>Content</p></body></html>`},
		{"basic", "iframe injection", `<html><body><iframe src="http://evil.com"></iframe><p>Content</p></body></html>`},
		{"basic", "embed tag", `<html><body><embed src="evil.swf"><p>Content</p></body></html>`},
		{"basic", "object tag", `<html><body><object data="evil.swf"></object><p>Content</p></body></html>`},
		{"basic", "onerror attribute", `<html><body><img src=x onerror="alert('XSS')"><p>Content</p></body></html>`},
		{"basic", "onload attribute", `<html><body><body onload="alert('XSS')"><p>Content</p></body></html>`},
		{"basic", "multiple event handlers", `<html><body><div onmouseover="alert('XSS')" onmouseout="alert('XSS2')">Content</div></body></html>`},

		// SVG XSS payloads
		{"svg", "onload event", `<html><body><svg onload="alert('XSS')"><circle cx="50"/></svg><p>Content</p></body></html>`},
		{"svg", "embedded script", `<html><body><svg><script>alert('XSS')</script></svg><p>Content</p></body></html>`},
		{"svg", "foreignObject", `<html><body><svg><foreignObject><body onload="alert('XSS')"></body></foreignObject></svg><p>Content</p></body></html>`},
		{"svg", "animate element", `<html><body><svg><animate onbegin="alert('XSS')" attributeName="x"/></svg><p>Content</p></body></html>`},
		{"svg", "use xlink", `<html><body><svg><use xlink:href="data:image/svg+xml,<svg onload='alert(1)'/>"/></svg><p>Content</p></body></html>`},
		{"svg", "set element", `<html><body><svg><set onbegin="alert('XSS')"/></svg><p>Content</p></body></html>`},

		// MathML XSS payloads
		{"mathml", "annotation-xml", `<html><body><math><annotation-xml encoding="application/xhtml+xml"><script>alert('XSS')</script></annotation-xml></math><p>Content</p></body></html>`},
		{"mathml", "javascript href", `<html><body><math href="javascript:alert('XSS')"><mtext>click</mtext></math><p>Content</p></body></html>`},
	}

	for _, tt := range xssPayloads {
		t.Run(tt.category+"/"+tt.name, func(t *testing.T) {
			result, err := p.Extract([]byte(tt.html))
			if err != nil {
				t.Fatalf("Extract() failed: %v", err)
			}

			// Check that dangerous content is not in the extracted text
			extractedText := strings.ToLower(result.Text)
			if strings.Contains(extractedText, "alert") ||
				strings.Contains(extractedText, "javascript:") ||
				strings.Contains(extractedText, "onerror") ||
				strings.Contains(extractedText, "onclick") ||
				strings.Contains(extractedText, "onload") {
				t.Errorf("XSS payload not sanitized: %s", extractedText)
			}

			// Check that script tags are not in links/images
			for _, link := range result.Links {
				if strings.Contains(strings.ToLower(link.URL), "javascript:") {
					t.Errorf("JavaScript URL not removed from links: %s", link.URL)
				}
			}
		})
	}
}

// hasDangerousSchemePrefix is a test-only check that a URL begins with a
// browser-executable or filesystem-reaching scheme. It mirrors the contract
// IsValidURL now enforces; the test asserts these never reach structured output.
func hasDangerousSchemePrefix(url string) bool {
	return strings.HasPrefix(url, "javascript:") ||
		strings.HasPrefix(url, "vbscript:") ||
		strings.HasPrefix(url, "file:")
}

// TestDangerousSchemeNotInStructuredOutput guards the MEDIUM-1/MEDIUM-2 fix.
// ExtractAllLinks skips sanitization by design, and the raw-HTML video/audio
// scan reads pre-sanitization HTML; both reach IsValidURL, which must reject
// dangerous schemes so a consumer iterating Links/Videos never receives a
// javascript:/vbscript:/file: URL it might render unsafely. The ".mp4" suffix
// is the bypass case: IsValidURL accepted it (first byte 'j' is alphanumeric)
// and IsVideoURL matched on the extension.
func TestDangerousSchemeNotInStructuredOutput(t *testing.T) {
	t.Parallel()

	p, err := html.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	// Default config (sanitization ON). The raw-HTML media scan still reads the
	// pre-sanitization string, so these must be filtered by IsValidURL.
	mediaPayloads := []string{
		`<html><body><iframe src="javascript:alert(1).mp4"></iframe><p>ok</p></body></html>`,
		`<html><body><embed src="javascript:alert(1).mp4"><p>ok</p></body></html>`,
		`<html><body><object data="javascript:alert(1).mp4"></object><p>ok</p></body></html>`,
		`<html><body><iframe src="vbscript:msgbox(1).mp4"></iframe><p>ok</p></body></html>`,
	}
	for _, in := range mediaPayloads {
		result, err := p.Extract([]byte(in))
		if err != nil {
			t.Fatalf("Extract failed: %v", err)
		}
		for _, v := range result.Videos {
			if hasDangerousSchemePrefix(strings.ToLower(v.URL)) {
				t.Errorf("dangerous scheme reached Videos: %q (input %q)", v.URL, in)
			}
		}
		for _, a := range result.Audios {
			if hasDangerousSchemePrefix(strings.ToLower(a.URL)) {
				t.Errorf("dangerous scheme reached Audios: %q (input %q)", a.URL, in)
			}
		}
	}

	// ExtractAllLinks deliberately skips sanitization; dangerous schemes must
	// still be filtered by IsValidURL.
	linksHTML := `<html><body>
		<a href="javascript:alert(1)">x</a>
		<a href="vbscript:msgbox(1)">y</a>
		<a href="file:///etc/passwd">z</a>
		<a href="https://example.com/ok">keep</a>
	</body></html>`
	links, err := p.ExtractAllLinks([]byte(linksHTML))
	if err != nil {
		t.Fatalf("ExtractAllLinks failed: %v", err)
	}
	var kept int
	for _, l := range links {
		if hasDangerousSchemePrefix(strings.ToLower(l.URL)) {
			t.Errorf("dangerous scheme reached Links: %q", l.URL)
		}
		if l.URL == "https://example.com/ok" {
			kept++
		}
	}
	if kept == 0 {
		t.Errorf("safe link was filtered out; IsValidURL is too strict")
	}
}

// TestPathTraversalPrevention tests file path validation
func TestPathTraversalPrevention(t *testing.T) {
	t.Parallel()

	p, err := html.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	pathTraversalAttempts := []string{
		"../../../etc/passwd",
		"..\\..\\..\\windows\\system32",
		"../../test.html",
		"../test.html",
		"./../../etc/passwd",
		"test/../../../etc/passwd",
		"..%2F..%2F..%2Fetc%2Fpasswd",
		"..%5c..%5c..%5cwindows%5csystem32",
	}

	for _, path := range pathTraversalAttempts {
		t.Run(path, func(t *testing.T) {
			_, err := p.ExtractFromFile(path)
			if err == nil {
				t.Errorf("Expected error for path traversal attempt: %s", path)
			}
		})
	}
}

// TestMalformedHTMLHandling tests tolerance for malformed HTML
func TestMalformedHTMLHandling(t *testing.T) {
	t.Parallel()

	p, err := html.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	malformedCases := []struct {
		name string
		html string
	}{
		{
			name: "unclosed div",
			html: `<html><body><div>Test<p>Content</body></html>`,
		},
		{
			name: "missing closing tags",
			html: `<html><body><h1>Title<p>Content</body></html>`,
		},
		{
			name: "nested improper tags",
			html: `<html><body><b><i>Test</b></i></body></html>`,
		},
		{
			name: "extra closing tags",
			html: `<html><body><p>Test</p></div></body></html>`,
		},
		{
			name: "attribute without value",
			html: `<html><body><input type="text" disabled>Content</body></html>`,
		},
		{
			name: "mismatched quotes",
			html: `<html><body><a href="http://example.com>Link</a>Content</body></html>`,
		},
		{
			name: "empty tag name",
			html: `<html><body><>Test</>Content</body></html>`,
		},
		{
			name: "triple angle brackets",
			html: `<<<>>>Test content<<<>>>`,
		},
	}

	for _, tt := range malformedCases {
		t.Run(tt.name, func(t *testing.T) {
			// Library should be tolerant and extract what it can
			result, err := p.Extract([]byte(tt.html))
			if err != nil && err != html.ErrInvalidHTML {
				t.Fatalf("Extract() failed for malformed HTML: %v", err)
			}
			if result == nil {
				t.Error("Expected non-nil result even for malformed HTML")
			}
		})
	}
}

// TestDataURLInjection tests data URL validation
func TestDataURLInjection(t *testing.T) {
	t.Parallel()

	p, err := html.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	dataURLCases := []struct {
		name          string
		html          string
		shouldExtract bool
	}{
		{
			name:          "safe image data URL",
			html:          `<html><body><img src="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAUAAAAFCAYAAACNbyblAAAAHElEQVQI12P4//8/w38GIAXDIBKE0DHxgljNBAAO9TXL0Y4OHwAAAABJRU5ErkJggg==">Content</body></html>`,
			shouldExtract: true,
		},
		{
			name:          "oversized data URL",
			html:          fmt.Sprintf(`<html><body><img src="data:image/png;base64,%s">Content</body></html>`, strings.Repeat("A", 200000)),
			shouldExtract: false,
		},
		{
			name:          "script data URL",
			html:          `<html><body><script src="data:text/javascript,alert('XSS')"></script>Content</body></html>`,
			shouldExtract: false,
		},
		{
			name:          "html data URL",
			html:          `<html><body><iframe src="data:text/html,<script>alert('XSS')</script>"></iframe>Content</body></html>`,
			shouldExtract: false,
		},
	}

	for _, tt := range dataURLCases {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.Extract([]byte(tt.html))
			if err != nil {
				t.Fatalf("Extract() failed: %v", err)
			}

			hasImage := len(result.Images) > 0
			if tt.shouldExtract && !hasImage {
				t.Error("Expected image to be extracted from data URL")
			}
			if !tt.shouldExtract && hasImage {
				t.Error("Expected data URL to be rejected")
			}
		})
	}
}

// TestInvalidUTF8Handling tests handling of invalid UTF-8 sequences
func TestInvalidUTF8Handling(t *testing.T) {
	t.Parallel()

	p, err := html.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	// Create HTML with invalid UTF-8 sequences
	invalidUTF8 := []byte{
		'<', 'h', 't', 'm', 'l', '>', '<', 'b', 'o', 'd', 'y', '>',
		0xFF, 0xFF, 0xFF, // Invalid UTF-8 bytes
		'C', 'o', 'n', 't', 'e', 'n', 't',
		0xC0, 0x80, // Overlong UTF-8 encoding
		'<', '/', 'b', 'o', 'd', 'y', '>', '<', '/', 'h', 't', 'm', 'l', '>',
	}

	result, err := p.Extract(invalidUTF8)

	// The library should handle invalid UTF-8 gracefully:
	// 1. Either process it (sanitizing/replacing invalid sequences), OR
	// 2. Return a clear error
	if err != nil {
		// Error is acceptable - invalid input was rejected
		if !strings.Contains(err.Error(), "encoding") &&
			!strings.Contains(err.Error(), "utf") &&
			!strings.Contains(err.Error(), "invalid") &&
			err != html.ErrInvalidHTML {
			t.Errorf("Unexpected error for invalid UTF-8: %v", err)
		}
		return
	}

	// If processing succeeded, verify the result is valid
	if result == nil {
		t.Fatal("Expected non-nil result when no error returned")
	}

	// The extracted text must itself be valid UTF-8: invalid input sequences
	// must be sanitized or dropped, never leak through to the output.
	if !utf8.ValidString(result.Text) {
		t.Errorf("extracted text is not valid UTF-8: %q", result.Text)
	}

	// Content should be present (the valid "Content" text)
	if !strings.Contains(result.Text, "Content") {
		t.Errorf("Expected 'Content' in extracted text, got: %q", result.Text)
	}
}

// TestControlCharacterHandling tests handling of control characters
func TestControlCharacterHandling(t *testing.T) {
	t.Parallel()

	p, err := html.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	// Create HTML with various control characters
	controlCharHTML := "<html><body>Content\x00\x01\x02\x1F\x7Fwith\x80\x81\x82control</body></html>"

	result, err := p.Extract([]byte(controlCharHTML))
	if err != nil {
		t.Fatalf("Extract() failed: %v", err)
	}

	// Verify the result is valid
	if result == nil {
		t.Fatal("Expected non-nil result")
	}

	// The text should be non-empty
	if result.Text == "" {
		t.Error("Expected non-empty text extraction")
	}

	// Verify that some content was extracted (control chars may be sanitized or preserved)
	// The key requirement is that extraction doesn't crash
	if !strings.Contains(result.Text, "Content") && !strings.Contains(result.Text, "with") {
		t.Errorf("Expected 'Content' or 'with' in extracted text, got: %q", result.Text)
	}

	// NUL bytes must not survive into the extracted text.
	if strings.Contains(result.Text, "\x00") {
		t.Errorf("null byte present in extracted text: %q", result.Text)
	}

	// The output must remain valid UTF-8 despite the \x80-\x82 bytes above.
	if !utf8.ValidString(result.Text) {
		t.Errorf("extracted text is not valid UTF-8: %q", result.Text)
	}
}

// TestNullByteInjection tests null byte handling in URLs and paths
func TestNullByteInjection(t *testing.T) {
	t.Parallel()

	p, err := html.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	nullByteCases := []struct {
		name string
		html string
	}{
		{
			// Interpreted string: "\x00" must be a real NUL byte. (A backtick
			// raw string would embed the four literal characters \x00 and the
			// test below would pass vacuously.)
			name: "null in URL",
			html: "<html><body><a href=\"http://example.com\x00.php\">Link</a></body></html>",
		},
		{
			name: "null in src",
			html: "<html><body><img src=\"http://example.com\x00.jpg\" alt=\"Image\"></body></html>",
		},
		{
			name: "multiple nulls",
			html: "<html><body><a href=\"http://example.com\x00\x00\x00.php\">Link</a></body></html>",
		},
	}

	for _, tt := range nullByteCases {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.Extract([]byte(tt.html))
			if err != nil {
				t.Fatalf("Extract() failed: %v", err)
			}

			// Check that null bytes are not in extracted URLs
			for _, link := range result.Links {
				if strings.Contains(link.URL, "\x00") {
					t.Errorf("Null byte not removed from URL: %q", link.URL)
				}
			}
			for _, img := range result.Images {
				if strings.Contains(img.URL, "\x00") {
					t.Errorf("Null byte not removed from image src: %q", img.URL)
				}
			}
		})
	}
}

// TestProtocolRelativeURLSafety tests safety of protocol-relative URLs
func TestProtocolRelativeURLSafety(t *testing.T) {
	t.Parallel()

	p, err := html.New()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = p.Close() }()

	htmlContent := []byte(`<html>
		<head><base href="//example.com/path/"></head>
		<body>
			<a href="//evil.com/xss.js">Script</a>
			<img src="//evil.com/steal.jpg">
			<p>Content</p>
		</body>
	</html>`)

	result, err := p.Extract(htmlContent)
	if err != nil {
		t.Fatalf("Extract() failed: %v", err)
	}

	// Body content must survive the protocol-relative base tag.
	if !strings.Contains(result.Text, "Content") {
		t.Errorf("expected body content, got %q", result.Text)
	}

	// Protocol-relative URLs must be preserved verbatim or upgraded to https —
	// never rewritten to a plain http:// URL (scheme downgrade).
	for _, link := range result.Links {
		if strings.HasPrefix(link.URL, "http://") {
			t.Errorf("protocol-relative link downgraded to http: %q", link.URL)
		}
	}
	for _, img := range result.Images {
		if strings.HasPrefix(img.URL, "http://") {
			t.Errorf("protocol-relative image downgraded to http: %q", img.URL)
		}
	}
}

// TestBenchmarkDoSPrevention benchmarks DoS prevention overhead
func BenchmarkDoSPreventionChecks(b *testing.B) {
	cfg := html.DefaultConfig()
	p, _ := html.New(cfg)
	defer func() { _ = p.Close() }()

	// Normal HTML content
	htmlContent := []byte(`<html><body><h1>Normal Content</h1><p>Test paragraph</p></body></html>`)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = p.Extract(htmlContent)
	}
}

// TestHighSecurityConfig tests the high-security configuration
func TestHighSecurityConfig(t *testing.T) {
	t.Parallel()

	cfg := html.HighSecurityConfig()

	// Verify security-enhanced settings
	if cfg.MaxInputSize > 10*1024*1024 {
		t.Errorf("HighSecurityConfig MaxInputSize should be <= 10MB, got %d", cfg.MaxInputSize)
	}
	if cfg.MaxDepth > 100 {
		t.Errorf("HighSecurityConfig MaxDepth should be <= 100, got %d", cfg.MaxDepth)
	}
	if cfg.ProcessingTimeout > 15*time.Second {
		t.Errorf("HighSecurityConfig ProcessingTimeout should be <= 15s, got %v", cfg.ProcessingTimeout)
	}
	if !cfg.EnableSanitization {
		t.Error("HighSecurityConfig should always have EnableSanitization=true")
	}

	// Test that high-security config works
	p, err := html.New(cfg)
	if err != nil {
		t.Fatalf("Failed to create processor with HighSecurityConfig: %v", err)
	}
	defer func() { _ = p.Close() }()

	// Test with normal content
	htmlContent := []byte(`<html><body><h1>Test</h1><p>Content</p></body></html>`)
	result, err := p.Extract(htmlContent)
	if err != nil {
		t.Fatalf("Extract() failed: %v", err)
	}
	if result.Text == "" {
		t.Error("Expected non-empty text extraction")
	}

	// Test that oversized input is rejected
	largeHTML := make([]byte, 15*1024*1024) // 15MB, exceeds 10MB limit
	_, err = p.Extract(largeHTML)
	if err == nil {
		t.Error("Expected error for oversized input in high-security mode")
	}
}

// TestHighSecurityConfigStricterThanDefault verifies high-security is more restrictive
func TestHighSecurityConfigStricterThanDefault(t *testing.T) {
	t.Parallel()

	defaultConfig := html.DefaultConfig()
	highSecConfig := html.HighSecurityConfig()

	if highSecConfig.MaxInputSize >= defaultConfig.MaxInputSize {
		t.Error("HighSecurityConfig MaxInputSize should be smaller than default")
	}
	if highSecConfig.MaxDepth >= defaultConfig.MaxDepth {
		t.Error("HighSecurityConfig MaxDepth should be smaller than default")
	}
	if highSecConfig.ProcessingTimeout >= defaultConfig.ProcessingTimeout {
		t.Error("HighSecurityConfig ProcessingTimeout should be shorter than default")
	}
}

// TestExtractAllLinksRejectsScriptableDataURLs covers the GEN-001 finding that
// IsValidURL's data: branch checked only length and charset: percent-encoded
// payloads contain no raw <>"' and passed, so script-executing data URLs
// (image/svg+xml, text/html) reached LinkResource.URL through the
// non-sanitizing ExtractAllLinks path. IsValidURL now enforces the same
// safeMediaTypes whitelist as the DOM sanitizer.
func TestExtractAllLinksRejectsScriptableDataURLs(t *testing.T) {
	t.Parallel()

	const svgPayload = `data:image/svg+xml,%3Csvg%20onload%3Dalert(1)%3E`
	const htmlPayload = `data:text/html,%3Cscript%3Ealert(1)%3C/script%3E`
	const pngPayload = `data:image/png;base64,iVBORw0KGgo=`
	doc := `<html><body>
		<a href="` + svgPayload + `">svg</a>
		<a href="` + htmlPayload + `">html</a>
		<a href="` + pngPayload + `">png</a>
	</body></html>`

	links, err := html.ExtractAllLinks([]byte(doc))
	if err != nil {
		t.Fatalf("ExtractAllLinks() failed: %v", err)
	}

	var sawSVG, sawHTML, sawPNG bool
	for _, link := range links {
		switch {
		case strings.HasPrefix(link.URL, "data:image/svg"):
			sawSVG = true
		case strings.HasPrefix(link.URL, "data:text/html"):
			sawHTML = true
		case strings.HasPrefix(link.URL, "data:image/png"):
			sawPNG = true
		}
	}
	if sawSVG {
		t.Error("SVG data URL must not appear in extracted links")
	}
	if sawHTML {
		t.Error("text/html data URL must not appear in extracted links")
	}
	if !sawPNG {
		t.Error("whitelisted image/png data URL should still be extracted")
	}
}
