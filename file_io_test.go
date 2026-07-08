package html_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cybergodev/html"
	"github.com/cybergodev/html/internal/testutil"
)

// ============================================================================
// ExtractFromFile Tests
// ============================================================================

func TestExtractFromFile(t *testing.T) {
	t.Parallel()

	t.Run("convenience function with valid file", func(t *testing.T) {
		htmlContent := testutil.CommonHTMLSnippets.SimpleArticle
		tmpFile := testutil.CreateTempHTML(t, htmlContent)

		result, err := html.ExtractFromFile(tmpFile)
		testutil.AssertNoError(t, err, "ExtractFromFile failed")
		testutil.AssertContains(t, result.Text, "first paragraph", "Text content")
		testutil.AssertEqual(t, result.Title, "Test Article", "Title")
	})

	t.Run("processor method with valid file", func(t *testing.T) {
		p, err := html.New()
		testutil.AssertNoError(t, err, "New() failed")
		defer p.Close()

		htmlContent := testutil.CommonHTMLSnippets.ArticleWithLinks
		tmpFile := testutil.CreateTempHTML(t, htmlContent)

		result, err := p.ExtractFromFile(tmpFile)
		testutil.AssertNoError(t, err, "ExtractFromFile failed")
		testutil.AssertTrue(t, len(result.Links) > 0, "Should extract links")
	})

	t.Run("non-existent file returns error", func(t *testing.T) {
		result, err := html.ExtractFromFile("/non/existent/file.html")
		testutil.AssertError(t, err, "Should return error for non-existent file")
		testutil.AssertTrue(t, result == nil, "Result should be nil on error")
	})

	t.Run("empty path returns error", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		result, err := p.ExtractFromFile("")
		testutil.AssertError(t, err, "Should return error for empty path")
		testutil.AssertTrue(t, result == nil, "Result should be nil")
	})

	t.Run("path traversal blocked", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		result, err := p.ExtractFromFile("../../../etc/passwd")
		testutil.AssertError(t, err, "Should block path traversal")
		testutil.AssertTrue(t, strings.Contains(err.Error(), "path traversal") ||
			strings.Contains(err.Error(), "not found"), "Error should mention path traversal or not found")
		testutil.AssertTrue(t, result == nil, "Result should be nil")
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := html.DefaultConfig()
		cfg.PreserveImages = true
		p, _ := html.New(cfg)
		defer p.Close()

		htmlContent := testutil.CommonHTMLSnippets.ArticleWithImages
		tmpFile := testutil.CreateTempHTML(t, htmlContent)

		result, err := p.ExtractFromFile(tmpFile)
		testutil.AssertNoError(t, err, "ExtractFromFile failed")
		testutil.AssertTrue(t, len(result.Images) > 0, "Should extract images")
	})

	t.Run("processor closed returns error", func(t *testing.T) {
		p, _ := html.New()
		p.Close()

		tmpFile := testutil.CreateTempHTML(t, "<html><body>Test</body></html>")
		result, err := p.ExtractFromFile(tmpFile)
		testutil.AssertError(t, err, "Should return error when processor closed")
		testutil.AssertTrue(t, result == nil, "Result should be nil")
	})

	t.Run("relative path works", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		// Create a temp file in current working directory to ensure relative path works
		htmlContent := `<html><body><p>Relative path test</p></body></html>`

		// Create temp directory in current working directory
		wd, err := os.Getwd()
		if err != nil {
			t.Fatalf("Getwd() failed: %v", err)
		}
		tmpDir := filepath.Join(wd, "testdata_temp")
		if err := os.MkdirAll(tmpDir, 0755); err != nil {
			t.Fatalf("MkdirAll() failed: %v", err)
		}
		defer os.RemoveAll(tmpDir) // Clean up after test

		// Create temp file in the temp directory
		tmpFile := filepath.Join(tmpDir, "relative_test.html")
		if err := os.WriteFile(tmpFile, []byte(htmlContent), 0644); err != nil {
			t.Fatalf("WriteFile() failed: %v", err)
		}

		// Use relative path
		relPath := filepath.Join("testdata_temp", "relative_test.html")

		result, err := p.ExtractFromFile(relPath)
		testutil.AssertNoError(t, err, "ExtractFromFile with relative path failed")
		testutil.AssertContains(t, result.Text, "Relative path test", "Text content")
	})
}

// ============================================================================
// ExtractTextFromFile Tests
// ============================================================================

func TestExtractTextFromFile(t *testing.T) {
	t.Parallel()

	t.Run("convenience function returns plain text", func(t *testing.T) {
		htmlContent := `<html><head><title>Text Test</title></head><body>
			<article>
				<h1>Title</h1>
				<p>First paragraph.</p>
				<p>Second paragraph.</p>
			</article>
		</body></html>`
		tmpFile := testutil.CreateTempHTML(t, htmlContent)

		text, err := html.ExtractTextFromFile(tmpFile)
		testutil.AssertNoError(t, err, "ExtractTextFromFile failed")
		testutil.AssertContains(t, text, "First paragraph", "Text should contain content")
		testutil.AssertContains(t, text, "Second paragraph", "Text should contain content")
	})

	t.Run("processor method returns plain text", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		htmlContent := testutil.CommonHTMLSnippets.SimpleArticle
		tmpFile := testutil.CreateTempHTML(t, htmlContent)

		text, err := p.ExtractTextFromFile(tmpFile)
		testutil.AssertNoError(t, err, "ExtractTextFromFile failed")
		testutil.AssertContains(t, text, "first paragraph", "Text content")
	})

	t.Run("non-existent file returns error", func(t *testing.T) {
		text, err := html.ExtractTextFromFile("/non/existent/file.html")
		testutil.AssertError(t, err, "Should return error")
		testutil.AssertEqual(t, text, "", "Text should be empty on error")
	})

	t.Run("empty file returns empty result", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		tmpFile := testutil.CreateTempHTML(t, "")
		text, err := p.ExtractTextFromFile(tmpFile)
		testutil.AssertNoError(t, err, "Empty file should not error")
		testutil.AssertEqual(t, text, "", "Text should be empty")
	})

	t.Run("strips HTML tags", func(t *testing.T) {
		htmlContent := `<html><body><div><span><p>Nested content</p></span></div></body></html>`
		tmpFile := testutil.CreateTempHTML(t, htmlContent)

		text, err := html.ExtractTextFromFile(tmpFile)
		testutil.AssertNoError(t, err, "ExtractTextFromFile failed")
		testutil.AssertNotContains(t, text, "<div>", "Should strip HTML tags")
		testutil.AssertNotContains(t, text, "<span>", "Should strip HTML tags")
		testutil.AssertNotContains(t, text, "<p>", "Should strip HTML tags")
		testutil.AssertContains(t, text, "Nested content", "Should preserve text")
	})
}

// ============================================================================
// ExtractAllLinksFromFile Tests
// ============================================================================

func TestExtractAllLinksFromFile(t *testing.T) {
	t.Parallel()

	t.Run("processor method extracts all links", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		htmlContent := testutil.CommonHTMLSnippets.ArticleWithLinks
		tmpFile := testutil.CreateTempHTML(t, htmlContent)

		links, err := p.ExtractAllLinksFromFile(tmpFile)
		testutil.AssertNoError(t, err, "ExtractAllLinksFromFile failed")
		testutil.AssertTrue(t, len(links) > 0, "Should extract links")

		// Check for specific link
		foundExample := false
		for _, link := range links {
			if link.URL == "https://example.com" {
				foundExample = true
				break
			}
		}
		testutil.AssertTrue(t, foundExample, "Should find example.com link")
	})

	t.Run("processor method extracts links with default config", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		htmlContent := `<html><body>
			<a href="https://test.com/page1">Page 1</a>
			<a href="https://test.com/page2">Page 2</a>
			<img src="https://test.com/image.jpg" alt="Test">
		</body></html>`
		tmpFile := testutil.CreateTempHTML(t, htmlContent)

		links, err := p.ExtractAllLinksFromFile(tmpFile)
		testutil.AssertNoError(t, err, "ExtractAllLinksFromFile failed")
		testutil.AssertTrue(t, len(links) >= 2, "Should extract at least 2 links")
	})

	t.Run("non-existent file returns error", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		links, err := p.ExtractAllLinksFromFile("/non/existent/file.html")
		testutil.AssertError(t, err, "Should return error")
		testutil.AssertTrue(t, links == nil, "Links should be nil on error")
	})

	t.Run("empty file returns empty slice", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		tmpFile := testutil.CreateTempHTML(t, "")
		links, err := p.ExtractAllLinksFromFile(tmpFile)
		testutil.AssertNoError(t, err, "Empty file should not error")
		testutil.AssertEqual(t, len(links), 0, "Should have no links")
	})

	t.Run("filters by config", func(t *testing.T) {
		// Create processor with custom link extraction config
		cfg := html.DefaultConfig()
		cfg.IncludeContentLinks = true
		cfg.IncludeExternalLinks = false
		cfg.IncludeImages = false
		p, _ := html.New(cfg)
		defer p.Close()

		htmlContent := `<html><body>
			<a href="https://external.com/page">External</a>
			<a href="/internal">Internal</a>
			<img src="image.jpg" alt="Image">
		</body></html>`
		tmpFile := testutil.CreateTempHTML(t, htmlContent)

		links, err := p.ExtractAllLinksFromFile(tmpFile)
		testutil.AssertNoError(t, err, "ExtractAllLinksFromFile failed")

		// Should only have internal link
		for _, link := range links {
			testutil.AssertFalse(t, link.URL == "https://external.com/page",
				"Should not include external link")
		}
	})

	t.Run("path traversal blocked", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		links, err := p.ExtractAllLinksFromFile("../../../etc/passwd")
		testutil.AssertError(t, err, "Should block path traversal")
		testutil.AssertTrue(t, links == nil, "Links should be nil")
	})
}

// ============================================================================
// ExtractToMarkdownFromFile Tests
// ============================================================================

func TestExtractToMarkdownFromFile(t *testing.T) {
	t.Parallel()

	t.Run("processor method returns markdown", func(t *testing.T) {
		cfg := html.MarkdownConfig()
		p, _ := html.New(cfg)
		defer p.Close()

		htmlContent := `<html><body>
			<h1>Title</h1>
			<p>Paragraph with <strong>bold</strong> text.</p>
			<img src="https://example.com/img.jpg" alt="Test Image">
		</body></html>`
		tmpFile := testutil.CreateTempHTML(t, htmlContent)

		markdown, err := p.ExtractToMarkdownFromFile(tmpFile)
		testutil.AssertNoError(t, err, "ExtractToMarkdownFromFile failed")
		testutil.AssertContains(t, markdown, "![Test Image](https://example.com/img.jpg)",
			"Should contain markdown image")
	})

	t.Run("processor method extracts markdown from article", func(t *testing.T) {
		cfg := html.MarkdownConfig()
		p, _ := html.New(cfg)
		defer p.Close()

		htmlContent := `<html><body>
			<article>
				<h1>Article</h1>
				<img src="photo.png" alt="Photo">
			</article>
		</body></html>`
		tmpFile := testutil.CreateTempHTML(t, htmlContent)

		markdown, err := p.ExtractToMarkdownFromFile(tmpFile)
		testutil.AssertNoError(t, err, "ExtractToMarkdownFromFile failed")
		testutil.AssertContains(t, markdown, "![Photo](photo.png)", "Should have markdown image")
	})

	t.Run("non-existent file returns error", func(t *testing.T) {
		cfg := html.MarkdownConfig()
		p, _ := html.New(cfg)
		defer p.Close()

		markdown, err := p.ExtractToMarkdownFromFile("/non/existent/file.html")
		testutil.AssertError(t, err, "Should return error")
		testutil.AssertEqual(t, markdown, "", "Markdown should be empty on error")
	})

	t.Run("with custom config preserving images", func(t *testing.T) {
		cfg := html.MarkdownConfig()
		cfg.PreserveImages = true
		p, _ := html.New(cfg)
		defer p.Close()

		htmlContent := testutil.CommonHTMLSnippets.ArticleWithImages
		tmpFile := testutil.CreateTempHTML(t, htmlContent)

		markdown, err := p.ExtractToMarkdownFromFile(tmpFile)
		testutil.AssertNoError(t, err, "ExtractToMarkdownFromFile failed")
		testutil.AssertTrue(t, strings.Contains(markdown, "![") || strings.Contains(markdown, "Test Image"),
			"Should process images")
	})
}

// ============================================================================
// ExtractToJSONFromFile Tests
// ============================================================================

func TestExtractToJSONFromFile(t *testing.T) {
	t.Parallel()

	t.Run("processor method returns valid JSON", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		htmlContent := testutil.CommonHTMLSnippets.SimpleArticle
		tmpFile := testutil.CreateTempHTML(t, htmlContent)

		jsonData, err := p.ExtractToJSONFromFile(tmpFile)
		testutil.AssertNoError(t, err, "ExtractToJSONFromFile failed")

		// Verify it's valid JSON
		var result map[string]interface{}
		err = json.Unmarshal(jsonData, &result)
		testutil.AssertNoError(t, err, "JSON should be valid")

		testutil.AssertContains(t, string(jsonData), "Test Article", "JSON should contain title")
	})

	t.Run("processor method with links", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		htmlContent := testutil.CommonHTMLSnippets.ArticleWithLinks
		tmpFile := testutil.CreateTempHTML(t, htmlContent)

		jsonData, err := p.ExtractToJSONFromFile(tmpFile)
		testutil.AssertNoError(t, err, "ExtractToJSONFromFile failed")

		// Parse and verify structure
		var result html.Result
		err = json.Unmarshal(jsonData, &result)
		testutil.AssertNoError(t, err, "JSON should parse into Result")
		testutil.AssertTrue(t, len(result.Links) > 0, "Should have links")
	})

	t.Run("non-existent file returns error", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		jsonData, err := p.ExtractToJSONFromFile("/non/existent/file.html")
		testutil.AssertError(t, err, "Should return error")
		testutil.AssertTrue(t, jsonData == nil, "JSON should be nil on error")
	})

	t.Run("JSON contains all fields", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		htmlContent := `<html><head><title>Full Test</title></head><body>
			<article>
				<h1>Article Title</h1>
				<p>Content paragraph.</p>
				<a href="https://example.com">Link</a>
				<img src="img.jpg" alt="Image">
			</article>
		</body></html>`
		tmpFile := testutil.CreateTempHTML(t, htmlContent)

		jsonData, err := p.ExtractToJSONFromFile(tmpFile)
		testutil.AssertNoError(t, err, "ExtractToJSONFromFile failed")

		jsonStr := string(jsonData)
		testutil.AssertContains(t, jsonStr, "title", "JSON should have title field")
		testutil.AssertContains(t, jsonStr, "text", "JSON should have text field")
		testutil.AssertContains(t, jsonStr, "word_count", "JSON should have word_count field")
	})

	t.Run("with custom config preserving images", func(t *testing.T) {
		cfg := html.DefaultConfig()
		cfg.PreserveImages = true
		p, _ := html.New(cfg)
		defer p.Close()

		htmlContent := testutil.CommonHTMLSnippets.ArticleWithImages
		tmpFile := testutil.CreateTempHTML(t, htmlContent)

		jsonData, err := p.ExtractToJSONFromFile(tmpFile)
		testutil.AssertNoError(t, err, "ExtractToJSONFromFile failed")

		var result html.Result
		json.Unmarshal(jsonData, &result)
		testutil.AssertTrue(t, len(result.Images) > 0, "Should have images")
	})
}

// ============================================================================
// File Encoding Detection Tests
// ============================================================================

func TestFileEncodingDetection(t *testing.T) {
	t.Parallel()

	t.Run("UTF-8 file detected correctly", func(t *testing.T) {
		htmlContent := `<html><head><meta charset="UTF-8"></head><body><p>UTF-8 Content: 你好</p></body></html>`
		tmpFile := testutil.CreateTempHTML(t, htmlContent)

		result, err := html.ExtractFromFile(tmpFile)
		testutil.AssertNoError(t, err, "ExtractFromFile failed")
		testutil.AssertContains(t, result.Text, "你好", "Should preserve Chinese characters")
	})

	t.Run("UTF-8 BOM handled", func(t *testing.T) {
		// UTF-8 BOM is EF BB BF
		bom := []byte{0xEF, 0xBB, 0xBF}
		content := []byte(`<html><body><p>BOM Test</p></body></html>`)
		fullContent := append(bom, content...)

		// Create temp file manually to preserve BOM
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "bom_test.html")
		os.WriteFile(tmpFile, fullContent, 0644)

		result, err := html.ExtractFromFile(tmpFile)
		testutil.AssertNoError(t, err, "ExtractFromFile with BOM failed")
		testutil.AssertContains(t, result.Text, "BOM Test", "Should extract text")
	})

	t.Run("HTML entities decoded", func(t *testing.T) {
		htmlContent := `<html><body><p>&lt;script&gt; &amp; &quot;quotes&quot;</p></body></html>`
		tmpFile := testutil.CreateTempHTML(t, htmlContent)

		result, err := html.ExtractFromFile(tmpFile)
		testutil.AssertNoError(t, err, "ExtractFromFile failed")
		testutil.AssertContains(t, result.Text, "<script>", "Should decode &lt;")
		testutil.AssertContains(t, result.Text, "&", "Should decode &amp;")
	})
}

// ============================================================================
// Edge Cases and Error Handling
// ============================================================================

func TestFileIOEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("large file handled", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		// Create a moderately large HTML file (1MB)
		var sb strings.Builder
		sb.WriteString(`<html><body>`)
		for i := 0; i < 10000; i++ {
			sb.WriteString(`<p>Paragraph number `)
			sb.WriteString(strings.Repeat("x", 50))
			sb.WriteString(`</p>`)
		}
		sb.WriteString(`</body></html>`)

		tmpFile := testutil.CreateTempHTML(t, sb.String())
		result, err := p.ExtractFromFile(tmpFile)
		testutil.AssertNoError(t, err, "Large file extraction failed")
		testutil.AssertTrue(t, len(result.Text) > 0, "Should extract text")
	})

	t.Run("malformed HTML handled", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		// Malformed but parseable HTML
		htmlContent := `<html><body><p>Unclosed paragraph<div>Nested without closing</body></html>`
		tmpFile := testutil.CreateTempHTML(t, htmlContent)

		result, err := p.ExtractFromFile(tmpFile)
		testutil.AssertNoError(t, err, "Malformed HTML should not error")
		testutil.AssertTrue(t, len(result.Text) > 0, "Should extract some text")
	})

	t.Run("empty HTML handled", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		tmpFile := testutil.CreateTempHTML(t, "")
		_, err := p.ExtractFromFile(tmpFile)
		testutil.AssertNoError(t, err, "Empty HTML should not error")
	})

	t.Run("whitespace only HTML", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		tmpFile := testutil.CreateTempHTML(t, "   \n\t  ")
		result, err := p.ExtractFromFile(tmpFile)
		testutil.AssertNoError(t, err, "Whitespace only should not error")
		testutil.AssertEqual(t, result.Text, "", "Text should be empty")
	})

	t.Run("special characters in path", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		// Create file with space in name
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "file with spaces.html")
		content := []byte(`<html><body><p>Space test</p></body></html>`)
		os.WriteFile(tmpFile, content, 0644)

		result, err := p.ExtractFromFile(tmpFile)
		testutil.AssertNoError(t, err, "File with spaces in path failed")
		testutil.AssertContains(t, result.Text, "Space test", "Should extract text")
	})
}

// ============================================================================
// Concurrent File Operations
// ============================================================================

func TestConcurrentFileOperations(t *testing.T) {
	t.Parallel()

	t.Run("concurrent reads from same file", func(t *testing.T) {
		tmpFile := testutil.CreateTempHTML(t, testutil.CommonHTMLSnippets.SimpleArticle)

		const numGoroutines = 10
		errCh := make(chan error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				result, err := html.ExtractFromFile(tmpFile)
				if err != nil {
					errCh <- err
					return
				}
				if result.Text == "" {
					errCh <- err
					return
				}
				errCh <- nil
			}()
		}

		for i := 0; i < numGoroutines; i++ {
			err := <-errCh
			testutil.AssertNoError(t, err, "Concurrent read failed")
		}
	})

	t.Run("concurrent reads with shared processor", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		tmpFile := testutil.CreateTempHTML(t, testutil.CommonHTMLSnippets.ArticleWithLinks)

		const numGoroutines = 5
		errCh := make(chan error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			go func() {
				result, err := p.ExtractFromFile(tmpFile)
				if err != nil {
					errCh <- err
					return
				}
				if len(result.Links) == 0 {
					errCh <- err
					return
				}
				errCh <- nil
			}()
		}

		for i := 0; i < numGoroutines; i++ {
			err := <-errCh
			testutil.AssertNoError(t, err, "Concurrent read with shared processor failed")
		}
	})
}

// TestExtractFromFile_AllowedBaseDir covers the AllowedBaseDir directory-jail
// branch of validateAndReadFile (processor.go): a file inside the configured
// base dir is read normally, while a file outside it is rejected as a path
// violation even though the path is otherwise valid. This branch is unreachable
// when AllowedBaseDir is unset (the default), so it needs an explicit test.
func TestExtractFromFile_AllowedBaseDir(t *testing.T) {
	t.Parallel()

	// Two independent temp directories: one is the jail, one is "outside".
	allowedDir := t.TempDir()
	outsideDir := t.TempDir()

	allowedFile := filepath.Join(allowedDir, "inside.html")
	outsideFile := filepath.Join(outsideDir, "outside.html")
	content := []byte("<html><body><p>hello allowed base dir</p></body></html>")
	if err := os.WriteFile(allowedFile, content, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outsideFile, content, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("file inside base dir is allowed", func(t *testing.T) {
		t.Parallel()

		cfg := html.DefaultConfig()
		cfg.AllowedBaseDir = allowedDir
		p, err := html.New(cfg)
		testutil.AssertNoError(t, err, "New() failed")
		defer p.Close()

		result, err := p.ExtractFromFile(allowedFile)
		testutil.AssertNoError(t, err, "file inside AllowedBaseDir should be readable")
		testutil.AssertContains(t, result.Text, "hello allowed base dir", "extracted text")
	})

	t.Run("file outside base dir is blocked", func(t *testing.T) {
		t.Parallel()

		cfg := html.DefaultConfig()
		cfg.AllowedBaseDir = allowedDir
		p, err := html.New(cfg)
		testutil.AssertNoError(t, err, "New() failed")
		defer p.Close()

		_, err = p.ExtractFromFile(outsideFile)
		testutil.AssertError(t, err, "file outside AllowedBaseDir should be rejected")
		testutil.AssertTrue(t,
			strings.Contains(err.Error(), "outside allowed directory"),
			"error should mention 'outside allowed directory'")
	})
}

// TestExtractFromFile_AllowedBaseDir_Symlink covers the reparse-point
// containment branch of validateAndReadFile (processor.go): a symlink or Windows
// directory junction placed inside the allowed tree whose target points outside
// it must NOT be followed, otherwise AllowedBaseDir containment can be bypassed.
// Symlinks require privileges on some hosts (notably Windows without Developer
// Mode / admin), so those cases skip when the harness cannot create one; the
// Windows junction case uses mklink /J, which needs no privilege.
func TestExtractFromFile_AllowedBaseDir_Symlink(t *testing.T) {
	t.Parallel()

	allowedDir := t.TempDir()
	outsideDir := t.TempDir()

	// Secret content that must never reach the caller through the allowed tree.
	secret := []byte("<html><body><p>TOP SECRET outside content</p></body></html>")
	secretFile := filepath.Join(outsideDir, "secret.html")
	if err := os.WriteFile(secretFile, secret, 0o644); err != nil {
		t.Fatal(err)
	}

	newProcessor := func(t *testing.T) *html.Processor {
		t.Helper()
		cfg := html.DefaultConfig()
		cfg.AllowedBaseDir = allowedDir
		p, err := html.New(cfg)
		testutil.AssertNoError(t, err, "New() failed")
		return p
	}

	// assertBlocked reads viaPath and asserts it is rejected as outside the base,
	// and that no secret content is returned.
	assertBlocked := func(t *testing.T, viaPath string) {
		t.Helper()
		p := newProcessor(t)
		defer p.Close()

		result, err := p.ExtractFromFile(viaPath)
		testutil.AssertError(t, err, "path escaping AllowedBaseDir should be rejected")
		testutil.AssertTrue(t,
			strings.Contains(err.Error(), "outside allowed directory"),
			"error should mention 'outside allowed directory'")
		if result != nil {
			testutil.AssertNotContains(t, result.Text, "TOP SECRET",
				"secret outside content must not be returned")
		}
	}

	t.Run("file symlink escaping base is blocked", func(t *testing.T) {
		t.Parallel()

		// allowedDir/escape.html -> outsideDir/secret.html
		link := filepath.Join(allowedDir, "escape.html")
		if err := os.Symlink(secretFile, link); err != nil {
			t.Skipf("cannot create symlink on this host: %v", err)
		}
		assertBlocked(t, link)
	})

	t.Run("directory symlink escaping base is blocked", func(t *testing.T) {
		t.Parallel()

		// allowedDir/escape-dir -> outsideDir, then read a file through it.
		dirLink := filepath.Join(allowedDir, "escape-dir")
		if err := os.Symlink(outsideDir, dirLink); err != nil {
			t.Skipf("cannot create symlink on this host: %v", err)
		}
		assertBlocked(t, filepath.Join(dirLink, "secret.html"))
	})

	t.Run("windows directory junction escaping base is blocked", func(t *testing.T) {
		t.Parallel()

		// allowedDir/escape-dir -> outsideDir via a junction (mklink /J). Junctions
		// need NO privilege on Windows and are NOT resolved by filepath.EvalSymlinks,
		// so this is the primary AllowedBaseDir bypass on that platform. It is the
		// one case that actually executes on a default Windows host.
		if runtime.GOOS != "windows" {
			t.Skip("junctions are a Windows reparse-point feature")
		}
		dirLink := filepath.Join(allowedDir, "escape-dir")
		out, err := exec.Command("cmd", "/c", "mklink", "/J", dirLink, outsideDir).CombinedOutput()
		if err != nil {
			t.Skipf("cannot create junction on this host: %v: %s", err, out)
		}
		assertBlocked(t, filepath.Join(dirLink, "secret.html"))
	})

	t.Run("legitimate file inside base still reads", func(t *testing.T) {
		t.Parallel()

		// Regression guard: the new symlink logic must not break the happy path.
		legit := filepath.Join(allowedDir, "inside.html")
		if err := os.WriteFile(legit, []byte("<p>allowed content</p>"), 0o644); err != nil {
			t.Fatal(err)
		}

		p := newProcessor(t)
		defer p.Close()

		result, err := p.ExtractFromFile(legit)
		testutil.AssertNoError(t, err, "legitimate file inside AllowedBaseDir should be readable")
		testutil.AssertContains(t, result.Text, "allowed content", "extracted text")
	})
}
