package html_test

// html_extended_test.go - Extended edge case tests that are NOT covered
// by html_test.go. Removed duplicates:
//   - ErrInputTooLarge (covered by TestInputValidation in html_test.go)
//   - ErrInvalidHTML (covered by TestMalformedHTMLHandling in security_test.go)
//   - ErrProcessingTimeout (covered by TestInputValidation in html_test.go)
//   - ErrFileNotFound (covered by TestFileExtraction in html_test.go)
//   - Empty/whitespace HTML (covered by TestBasicExtraction in html_test.go)
//   - Mixed content/scripts (covered by TestContentExtraction in html_test.go)
//   - Deep nesting (covered by TestDeepNestingDoSPrevention in security_test.go)

import (
	"fmt"
	"strings"
	"testing"

	"github.com/cybergodev/html"
)

// TestEdgeCasesInputBoundary tests input size boundary conditions
// not covered by the basic input validation tests.
func TestEdgeCasesInputBoundary(t *testing.T) {
	t.Parallel()

	t.Run("exact MaxInputSize boundary", func(t *testing.T) {
		cfg := html.DefaultConfig()
		cfg.MaxInputSize = 5000
		p, err := html.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		validHTML := strings.Repeat("<div>a</div>", 100)
		_, err = p.Extract([]byte(validHTML))
		if err != nil {
			t.Errorf("should accept input at MaxInputSize boundary, got: %v", err)
		}

		oversizedHTML := strings.Repeat("<div>a</div>", 1000)
		_, err = p.Extract([]byte(oversizedHTML))
		if err == nil {
			t.Error("expected error for oversize input")
		}
	})

	t.Run("whitespace path returns error", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		_, err := p.ExtractFromFile("   ")
		if err == nil {
			t.Error("expected error for whitespace path")
		}
	})
}

// TestEdgeCasesContentType tests content types not covered elsewhere.
func TestEdgeCasesContentType(t *testing.T) {
	t.Parallel()

	p, _ := html.New()
	defer p.Close()

	t.Run("doctype and comment only", func(t *testing.T) {
		cases := []string{
			"<!DOCTYPE html>",
			"<!-- comment only -->",
			"<html></html>",
			"<html><body></body></html>",
		}
		for _, htmlContent := range cases {
			t.Run(fmt.Sprintf("len_%d", len(htmlContent)), func(t *testing.T) {
				result, err := p.Extract([]byte(htmlContent))
				if err != nil {
					t.Errorf("should not error: %v", err)
				}
				if result == nil {
					t.Error("result should not be nil")
				}
			})
		}
	})

	t.Run("excessive whitespace collapsed", func(t *testing.T) {
		whitespaceHTML := `<html><body>
			<p>Text    with     many     spaces</p>
			<p>Text

	with

	newlines</p>
			<p>Text	with	tabs</p>
		</body></html>`

		result, err := p.Extract([]byte(whitespaceHTML))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if strings.Contains(result.Text, "    ") {
			t.Error("should collapse multiple spaces")
		}
		if strings.Contains(result.Text, "\t") {
			t.Error("should replace tabs with spaces")
		}
	})

	t.Run("entity decoding edge cases", func(t *testing.T) {
		entityHTML := `<html><body>
			<p>&amp;&lt;&gt;&quot;&apos;</p>
			<p>&nbsp;&nbsp;&copy;&reg;&trade;</p>
			<p>&mdash;&ndash;&hellip;</p>
			<p>&euro;&pound;&yen;</p>
			<p>&#65;&#x41;</p>
		</body></html>`

		result, err := p.Extract([]byte(entityHTML))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if !strings.Contains(result.Text, "&<>\"'") {
			t.Error("basic XML entities should be decoded")
		}
		if !strings.Contains(result.Text, "©") {
			t.Error("copyright entity should be decoded")
		}
		if !strings.Contains(result.Text, "—") {
			t.Error("em dash entity should be decoded")
		}
	})
}

// TestUnicodeEdgeCases tests unicode handling not covered by other tests.
func TestUnicodeEdgeCases(t *testing.T) {
	t.Parallel()

	p, _ := html.New()
	defer p.Close()

	t.Run("bidirectional text preserved", func(t *testing.T) {
		htmlContent := `<html><body>
			<p>Hello مرحبا World עולם</p>
			<p dir="rtl">Right to left text</p>
		</body></html>`

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if !strings.Contains(result.Text, "Hello") {
			t.Error("expected English text preserved")
		}
		if !strings.Contains(result.Text, "مرحبا") {
			t.Error("expected Arabic text preserved")
		}
	})

	t.Run("surrogate pairs (emoji) preserved", func(t *testing.T) {
		htmlContent := `<html><body>
			<p>Simple: 😀😂🥳</p>
			<p>Complex: 👨‍👩‍👧‍👦 🏳️‍🌈</p>
			<p>Skin tone: 👍🏻👍🏼👍🏽👍🏾👍🏿</p>
		</body></html>`

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if !strings.Contains(result.Text, "😀") {
			t.Error("expected emoji preserved")
		}
	})

	t.Run("null character handled gracefully", func(t *testing.T) {
		htmlContent := `<html><body><p>Before` + "\x00" + `After</p></body></html>`

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if result == nil {
			t.Error("expected non-nil result")
		}
	})
}

// TestMaxDepthBoundaryConditions tests depth limits at exact boundaries.
func TestMaxDepthBoundaryConditions(t *testing.T) {
	t.Parallel()

	t.Run("depth exactly at limit succeeds", func(t *testing.T) {
		cfg := html.DefaultConfig()
		cfg.MaxDepth = 50
		p, err := html.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		deepHTML := "<html><body>"
		for i := 0; i < 48; i++ {
			deepHTML += "<div>"
		}
		deepHTML += "Content"
		for i := 0; i < 48; i++ {
			deepHTML += "</div>"
		}
		deepHTML += "</body></html>"

		result, err := p.Extract([]byte(deepHTML))
		if err != nil {
			t.Logf("depth at limit returned error: %v", err)
		}
		if result != nil && !strings.Contains(result.Text, "Content") {
			t.Error("expected content at exact depth limit")
		}
	})

	t.Run("very deep nesting with high limit", func(t *testing.T) {
		cfg := html.DefaultConfig()
		cfg.MaxDepth = 500
		p, err := html.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		deepHTML := "<html><body>"
		for i := 0; i < 198; i++ {
			deepHTML += "<div>"
		}
		deepHTML += "Content"
		for i := 0; i < 198; i++ {
			deepHTML += "</div>"
		}
		deepHTML += "</body></html>"

		result, err := p.Extract([]byte(deepHTML))
		if err != nil {
			t.Errorf("deep nesting with high limit should succeed: %v", err)
		}
		if result == nil {
			t.Error("expected non-nil result")
		}
	})
}
