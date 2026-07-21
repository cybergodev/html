package html_test

import (
	"os"
	"strings"
	"testing"

	"github.com/cybergodev/html"
)

// TestExtractWithEncodingDetection tests automatic encoding detection in Extract
func TestExtractWithEncodingDetection(t *testing.T) {
	t.Run("Windows-1252 encoded HTML", func(t *testing.T) {
		// Read actual Windows-1252 file
		testFile := "dev_test/Source Files/a2025q310-qexx311.html"
		if _, err := os.Stat(testFile); os.IsNotExist(err) {
			t.Skip("Test file not found")
		}

		// Read file bytes
		htmlBytes, err := os.ReadFile(testFile)
		if err != nil {
			t.Fatalf("Failed to read test file: %v", err)
		}

		// Extract using bytes method (with encoding detection)
		result, err := html.Extract(htmlBytes)
		if err != nil {
			t.Fatalf("Extract failed: %v", err)
		}

		// Verify UTF-8 output
		if !strings.Contains(result.Text, "registrant's") {
			t.Errorf("Expected UTF-8 apostrophes, got: %s", result.Text)
		}

		// Verify no garbled characters
		if strings.Contains(result.Text, "\ufffd") {
			t.Error("Found replacement character, encoding may not be detected correctly")
		}
	})

	t.Run("UTF-8 encoded HTML", func(t *testing.T) {
		htmlContent := `<html><head><meta charset="utf-8"></head><body><p>Hello 世界</p></body></html>`
		htmlBytes := []byte(htmlContent)

		result, err := html.Extract(htmlBytes)
		if err != nil {
			t.Fatalf("Extract failed: %v", err)
		}

		if !strings.Contains(result.Text, "Hello 世界") {
			t.Errorf("Expected 'Hello 世界', got: %s", result.Text)
		}
	})

	t.Run("ASCII HTML", func(t *testing.T) {
		htmlContent := `<html><body><p>Hello World</p></body></html>`
		htmlBytes := []byte(htmlContent)

		result, err := html.Extract(htmlBytes)
		if err != nil {
			t.Fatalf("Extract failed: %v", err)
		}

		expected := "Hello World"
		if !strings.Contains(result.Text, expected) {
			t.Errorf("Expected %q, got: %s", expected, result.Text)
		}
	})
}

// TestExtractToMarkdownEncoding tests automatic encoding detection in ExtractToMarkdown
func TestExtractToMarkdownEncoding(t *testing.T) {
	t.Run("Windows-1252 to Markdown", func(t *testing.T) {
		testFile := "dev_test/Source Files/a2025q310-qexx311.html"
		if _, err := os.Stat(testFile); os.IsNotExist(err) {
			t.Skip("Test file not found")
		}

		htmlBytes, _ := os.ReadFile(testFile)

		cfg := html.MarkdownConfig()
		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer p.Close()

		markdown, err := p.ExtractToMarkdown(htmlBytes)
		if err != nil {
			t.Fatalf("ExtractToMarkdown failed: %v", err)
		}

		// Verify UTF-8 conversion
		if !strings.Contains(markdown, "registrant's") {
			t.Errorf("Expected UTF-8 apostrophes in markdown")
		}

		// Verify no garbled text
		if strings.Contains(markdown, "\ufffd") {
			t.Error("Found replacement character in markdown")
		}
	})

	t.Run("UTF-8 to Markdown", func(t *testing.T) {
		htmlContent := `<html><body><p>Hello 世界 © 2025</p></body></html>`
		htmlBytes := []byte(htmlContent)

		cfg := html.MarkdownConfig()
		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer p.Close()

		markdown, err := p.ExtractToMarkdown(htmlBytes)
		if err != nil {
			t.Fatalf("ExtractToMarkdown failed: %v", err)
		}

		if !strings.Contains(markdown, "Hello 世界") {
			t.Errorf("Expected 'Hello 世界', got: %s", markdown)
		}

		if !strings.Contains(markdown, "©") {
			t.Errorf("Expected copyright symbol")
		}
	})
}

// TestExtractText tests automatic encoding detection in ExtractText
func TestExtractText(t *testing.T) {
	t.Run("Windows-1252 text extraction", func(t *testing.T) {
		testFile := "dev_test/Source Files/a2025q310-qexx311.html"
		if _, err := os.Stat(testFile); os.IsNotExist(err) {
			t.Skip("Test file not found")
		}

		htmlBytes, _ := os.ReadFile(testFile)

		text, err := html.ExtractText(htmlBytes)
		if err != nil {
			t.Fatalf("ExtractText failed: %v", err)
		}

		// Verify UTF-8
		if !strings.Contains(text, "registrant's") {
			t.Errorf("Expected UTF-8 apostrophes")
		}
	})

	t.Run("UTF-8 text extraction", func(t *testing.T) {
		htmlContent := `<html><body><p>Hello 世界 — Test</p></body></html>`
		htmlBytes := []byte(htmlContent)

		text, err := html.ExtractText(htmlBytes)
		if err != nil {
			t.Fatalf("ExtractText failed: %v", err)
		}

		if !strings.Contains(text, "Hello 世界") {
			t.Errorf("Expected 'Hello 世界', got: %s", text)
		}
	})
}

// TestExtractAllLinksWithEncoding tests automatic encoding detection in ExtractAllLinks
func TestExtractAllLinksWithEncoding(t *testing.T) {
	t.Run("UTF-8 HTML with links", func(t *testing.T) {
		htmlContent := `<html><body>
			<a href="https://example.com">Example</a>
			<a href="/relative">Relative Link</a>
		</body></html>`
		htmlBytes := []byte(htmlContent)

		p, err := html.New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer p.Close()

		links, err := p.ExtractAllLinks(htmlBytes)
		if err != nil {
			t.Fatalf("ExtractAllLinks failed: %v", err)
		}

		if len(links) != 2 {
			t.Errorf("Expected 2 links, got %d", len(links))
		}
	})

	t.Run("Windows-1252 HTML with links", func(t *testing.T) {
		// Create Windows-1252 HTML with smart quotes in link text
		htmlBytes := []byte(`<html><head><meta charset="windows-1252"></head>
		<body><a href="/test">Company's Website</a></body></html>`)

		p, err := html.New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer p.Close()

		links, err := p.ExtractAllLinks(htmlBytes)
		if err != nil {
			t.Fatalf("ExtractAllLinks failed: %v", err)
		}

		if len(links) != 1 {
			t.Fatalf("Expected 1 link, got %d", len(links))
		}

		// Verify the apostrophe is correctly decoded
		if !strings.Contains(links[0].Title, "Company's") {
			t.Errorf("Expected 'Company's', got: %s", links[0].Title)
		}
	})
}

// TestExtractWithForcedEncoding tests forced encoding override. The input uses
// Windows-1252 bytes (0x92 = right single quotation mark, U+2019), which are
// invalid UTF-8, so forcing windows-1252 genuinely exercises the override-decode
// path. Previously the input was plain ASCII and the forced-encoding branch was
// never hit.
func TestExtractWithForcedEncoding(t *testing.T) {
	// 0x92 is the Windows-1252 right single quotation mark; as a lone byte it is
	// invalid UTF-8, so auto-detection would fail without the forced charset.
	htmlBytes := []byte("<html><body><p>Company\x92s</p></body></html>")

	cfg := html.DefaultConfig()
	cfg.Encoding = "windows-1252"
	p, err := html.New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	defer p.Close()

	result, err := p.Extract(htmlBytes)
	if err != nil {
		t.Fatalf("Extract failed: %v", err)
	}

	// Forced windows-1252 decoding must turn 0x92 into U+2019.
	if !strings.Contains(result.Text, "Company’s") {
		t.Errorf("Expected forced windows-1252 decode of 0x92 to '’', got: %q", result.Text)
	}
	// And there must be no UTF-8 replacement character (mis-decode marker).
	if strings.Contains(result.Text, "�") {
		t.Errorf("Found replacement character, forced decode may be wrong: %q", result.Text)
	}
}
