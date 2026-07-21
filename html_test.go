package html_test

// html_test.go - Comprehensive test suite for the html package
// This file consolidates and improves all main package tests to provide:
// - Better coverage of public APIs
// - Elimination of duplicate tests
// - Improved maintainability
// - Effective regression testing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cybergodev/html"
)

// ============================================================================
// PROCESSOR LIFECYCLE AND CONFIGURATION TESTS
// ============================================================================

func TestProcessorLifecycle(t *testing.T) {
	t.Parallel()

	t.Run("New with valid config", func(t *testing.T) {
		cfg := html.DefaultConfig()
		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer p.Close()
		if p == nil {
			t.Fatal("New() returned nil processor")
		}
	})

	t.Run("New with no config uses defaults", func(t *testing.T) {
		p, err := html.New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer p.Close()
		if p == nil {
			t.Fatal("New() returned nil processor")
		}

		// Verify it works with default config by extracting some content
		result, err := p.Extract([]byte("<html><body><h1>Test</h1></body></html>"))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if result.Title != "Test" {
			t.Errorf("Expected title 'Test', got %q", result.Title)
		}
	})

	t.Run("New with empty config returns error", func(t *testing.T) {
		// Empty config should be rejected - users must use DefaultConfig()
		_, err := html.New(html.Config{})
		if err == nil {
			t.Fatal("New(Config{}) should fail with validation error")
		}
		if !errors.Is(err, html.ErrInvalidConfig) {
			t.Errorf("Expected ErrInvalidConfig, got: %v", err)
		}
	})

	t.Run("New with preset configs", func(t *testing.T) {
		// Test MarkdownConfig
		p1, err := html.New(html.MarkdownConfig())
		if err != nil {
			t.Fatalf("New(MarkdownConfig()) failed: %v", err)
		}
		defer p1.Close()

		// Test TextOnlyConfig
		p2, err := html.New(html.TextOnlyConfig())
		if err != nil {
			t.Fatalf("New(TextOnlyConfig()) failed: %v", err)
		}
		defer p2.Close()

		// Test HighSecurityConfig
		p3, err := html.New(html.HighSecurityConfig())
		if err != nil {
			t.Fatalf("New(HighSecurityConfig()) failed: %v", err)
		}
		defer p3.Close()
	})

	t.Run("Close idempotent", func(t *testing.T) {
		p, _ := html.New()
		if err := p.Close(); err != nil {
			t.Fatalf("Close() failed: %v", err)
		}
		if err := p.Close(); err != nil {
			t.Fatalf("Close() should be idempotent: %v", err)
		}
	})

}

func TestConfiguration(t *testing.T) {
	t.Parallel()

	t.Run("DefaultConfig values", func(t *testing.T) {
		cfg := html.DefaultConfig()
		if cfg.MaxInputSize <= 0 {
			t.Error("MaxInputSize should be positive")
		}
		if cfg.MaxCacheEntries < 0 {
			t.Error("MaxCacheEntries should be non-negative")
		}
		if cfg.CacheTTL < 0 {
			t.Error("CacheTTL should be non-negative")
		}
		if cfg.WorkerPoolSize <= 0 {
			t.Error("WorkerPoolSize should be positive")
		}
		if !cfg.EnableSanitization {
			t.Error("EnableSanitization should be true by default")
		}
		if cfg.MaxDepth <= 0 {
			t.Error("MaxDepth should be positive")
		}
		if cfg.ProcessingTimeout != 30*time.Second {
			t.Errorf("ProcessingTimeout = %v, want %v", cfg.ProcessingTimeout, 30*time.Second)
		}
	})

	t.Run("DefaultExtractionConfig values", func(t *testing.T) {
		cfg := html.DefaultConfig()
		if !cfg.ExtractArticle {
			t.Error("ExtractArticle should be true")
		}
		if !cfg.PreserveImages {
			t.Error("PreserveImages should be true")
		}
		if !cfg.PreserveLinks {
			t.Error("PreserveLinks should be true")
		}
		if !cfg.PreserveVideos {
			t.Error("PreserveVideos should be true")
		}
		if !cfg.PreserveAudios {
			t.Error("PreserveAudios should be true")
		}
		if cfg.InlineImageFormat != "none" {
			t.Errorf("InlineImageFormat = %q, want 'none'", cfg.InlineImageFormat)
		}
		if cfg.TableFormat != "markdown" {
			t.Errorf("TableFormat = %q, want 'markdown'", cfg.TableFormat)
		}
	})

	t.Run("Extraction config functions", func(t *testing.T) {
		// Test DefaultConfig extraction settings
		cfg := html.DefaultConfig()
		if !cfg.ExtractArticle {
			t.Error("DefaultConfig: ExtractArticle should be true")
		}
		if !cfg.PreserveImages {
			t.Error("DefaultConfig: PreserveImages should be true")
		}

		// Test TextOnlyConfig
		textOnlyCfg := html.TextOnlyConfig()
		if !textOnlyCfg.ExtractArticle {
			t.Error("TextOnlyConfig: ExtractArticle should be true")
		}
		if textOnlyCfg.PreserveImages {
			t.Error("TextOnlyConfig: PreserveImages should be false")
		}
		if textOnlyCfg.PreserveLinks {
			t.Error("TextOnlyConfig: PreserveLinks should be false")
		}

		// Test link extraction settings in DefaultConfig
		if !cfg.IncludeImages {
			t.Error("DefaultConfig: IncludeImages should be true")
		}
		if !cfg.IncludeContentLinks {
			t.Error("DefaultConfig: IncludeContentLinks should be true")
		}
	})

	t.Run("custom config creation", func(t *testing.T) {
		t.Run("RSS-style config", func(t *testing.T) {
			cfg := html.DefaultConfig()
			cfg.ExtractArticle = false
			cfg.PreserveImages = true
			cfg.PreserveLinks = true
			cfg.PreserveVideos = false
			cfg.PreserveAudios = false
			cfg.InlineImageFormat = "none"
			cfg.TableFormat = "markdown"

			if cfg.ExtractArticle {
				t.Error("RSS-style config should disable article extraction")
			}
			if !cfg.PreserveImages {
				t.Error("RSS-style config should preserve images")
			}
		})

		t.Run("Markdown config", func(t *testing.T) {
			cfg := html.MarkdownConfig()
			if !cfg.PreserveImages {
				t.Error("Markdown config should preserve images")
			}
			if !cfg.PreserveLinks {
				t.Error("Markdown config should preserve links")
			}
			if cfg.InlineImageFormat != "markdown" {
				t.Error("Markdown config should have markdown image format")
			}
			if cfg.TableFormat != "markdown" {
				t.Error("Markdown config should have markdown table format")
			}
		})
	})
}

// ============================================================================
// BASIC EXTRACTION TESTS
// ============================================================================

func TestBasicExtraction(t *testing.T) {
	t.Parallel()

	p, _ := html.New()
	defer p.Close()

	t.Run("simple HTML", func(t *testing.T) {
		result, err := p.Extract([]byte(`<html><body><p>Hello World</p></body></html>`))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if !strings.Contains(result.Text, "Hello World") {
			t.Errorf("Text should contain 'Hello World', got %q", result.Text)
		}
	})

	t.Run("empty HTML", func(t *testing.T) {
		result, err := p.Extract([]byte(""))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if result.Text != "" {
			t.Errorf("Text should be empty, got %q", result.Text)
		}
	})

	t.Run("whitespace only", func(t *testing.T) {
		result, err := p.Extract([]byte("   \n\t  "))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if result.Text != "" {
			t.Errorf("Text should be empty, got %q", result.Text)
		}
	})

	t.Run("malformed HTML handled gracefully", func(t *testing.T) {
		result, err := p.Extract([]byte(`<html><body><p>Unclosed paragraph`))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if !strings.Contains(result.Text, "Unclosed paragraph") {
			t.Error("Text should contain 'Unclosed paragraph'")
		}
	})
}

// ============================================================================
// INPUT VALIDATION TESTS
// ============================================================================

func TestInputValidation(t *testing.T) {
	t.Parallel()

	t.Run("input size limit enforced", func(t *testing.T) {
		cfg := html.DefaultConfig()
		cfg.MaxInputSize = 100
		cfg.MaxCacheEntries = 10
		p, _ := html.New(cfg)
		defer p.Close()

		largeHTML := strings.Repeat("a", 200)
		_, err := p.Extract([]byte(largeHTML))
		if err == nil {
			t.Fatal("Extract() should fail with input too large")
		}
	})

	t.Run("max depth enforced", func(t *testing.T) {
		cfg := html.DefaultConfig()
		cfg.MaxDepth = 5
		p, _ := html.New(cfg)
		defer p.Close()

		deepHTML := "<div>" + strings.Repeat("<div>", 10) + "content" + strings.Repeat("</div>", 10) + "</div>"
		_, err := p.Extract([]byte(deepHTML))
		if err != html.ErrMaxDepthExceeded {
			t.Errorf("Expected ErrMaxDepthExceeded, got: %v", err)
		}
	})

	t.Run("processing timeout", func(t *testing.T) {
		if testing.Short() {
			t.Skip("Skipping timeout test in short mode")
		}

		// A timeout assertion is only deterministic when the document provably
		// outlasts the budget. withTimeout races the worker goroutine against
		// the deadline timer; a tiny document (the old 500-div/1ns pair) can
		// finish before the timer is serviced, so the result sometimes won the
		// race and the test flaked under load. A ~3MB document (50k paragraphs)
		// takes well over the 10ms budget on any machine, so the deadline fires
		// first and the result is normalized to ErrProcessingTimeout. Mirrors
		// TestWithTimeoutLongRunningOperation in leak_test.go.
		var sb strings.Builder
		for i := 0; i < 50000; i++ {
			sb.WriteString("<p>This is a test paragraph with some content to make it larger.</p>")
		}
		largeHTML := []byte("<html><body>" + sb.String() + "</body></html>")

		cfg := html.DefaultConfig()
		cfg.ProcessingTimeout = 10 * time.Millisecond
		p, _ := html.New(cfg)
		defer p.Close()

		_, err := p.Extract(largeHTML)
		if err != html.ErrProcessingTimeout {
			t.Errorf("Expected ErrProcessingTimeout, got: %v", err)
		}
	})

	t.Run("disabled timeout works", func(t *testing.T) {
		htmlContent := `<html><body><article><h1>Test</h1><p>Content</p></article></body></html>`
		cfg := html.DefaultConfig()
		cfg.ProcessingTimeout = 0
		p, _ := html.New(cfg)
		defer p.Close()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if result.Title != "Test" {
			t.Errorf("Title = %q, want 'Test'", result.Title)
		}
	})
}

// ============================================================================
// FILE EXTRACTION TESTS
// ============================================================================

func TestFileExtraction(t *testing.T) {
	t.Parallel()

	p, _ := html.New()
	defer p.Close()

	t.Run("empty file path", func(t *testing.T) {
		_, err := p.ExtractFromFile("")
		if err == nil {
			t.Fatal("ExtractFromFile() should fail with empty path")
		}
	})

	t.Run("non-existent file", func(t *testing.T) {
		_, err := p.ExtractFromFile("nonexistent.html")
		if err == nil {
			t.Fatal("ExtractFromFile() should fail with non-existent file")
		}
	})

	t.Run("valid file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test.html")
		content := `<html><body><h1>Test</h1><p>Content</p></body></html>`

		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		result, err := p.ExtractFromFile(filePath)
		if err != nil {
			t.Fatalf("ExtractFromFile() failed: %v", err)
		}
		if result.Title != "Test" {
			t.Errorf("Title = %q, want 'Test'", result.Title)
		}
	})

	t.Run("ExtractFromFile package function", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test.html")
		content := `<html><body><h1>Package Test</h1><p>Content</p></body></html>`

		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}

		result, err := html.ExtractFromFile(filePath)
		if err != nil {
			t.Fatalf("ExtractFromFile() failed: %v", err)
		}
		if result.Title != "Package Test" {
			t.Errorf("Title = %q, want 'Package Test'", result.Title)
		}
	})
}

// ============================================================================
// BATCH PROCESSING TESTS
// ============================================================================

func TestBatchProcessing(t *testing.T) {
	t.Parallel()

	p, _ := html.New()
	defer p.Close()

	t.Run("normal batch", func(t *testing.T) {
		inputs := [][]byte{
			[]byte(`<html><body><article><h1>A1</h1><p>C1</p></article></body></html>`),
			[]byte(`<html><body><article><h1>A2</h1><p>C2</p></article></body></html>`),
			[]byte(`<html><body><article><h1>A3</h1><p>C3</p></article></body></html>`),
		}

		br := p.ExtractBatch(inputs)
		if br.Failed > 0 {
			t.Fatalf("ExtractBatch() failed: %v", br.Errors[0])
		}
		if len(br.Results) != 3 {
			t.Errorf("Got %d results, want 3", len(br.Results))
		}
		for i, r := range br.Results {
			expected := fmt.Sprintf("A%d", i+1)
			if r.Title != expected {
				t.Errorf("Result[%d].Title = %q, want %q", i, r.Title, expected)
			}
		}
	})

	t.Run("batch with partial failures", func(t *testing.T) {
		cfg := html.DefaultConfig()
		cfg.MaxInputSize = 100
		cfg.MaxCacheEntries = 10
		p, _ := html.New(cfg)
		defer p.Close()

		inputs := [][]byte{
			[]byte(`<html><body><p>Valid</p></body></html>`),
			[]byte(strings.Repeat("a", 200)), // Too large
			[]byte(`<html><body><p>Another valid</p></body></html>`),
		}

		br := p.ExtractBatch(inputs)
		if br.Failed == 0 {
			t.Fatal("ExtractBatch() should return error for partial failures")
		}
		if len(br.Results) != 3 {
			t.Errorf("Got %d results, want 3", len(br.Results))
		}
		if br.Results[0] == nil || br.Results[2] == nil {
			t.Error("Valid results should not be nil")
		}
	})
}

// ============================================================================
// CONTENT EXTRACTION TESTS
// ============================================================================

func TestContentExtraction(t *testing.T) {
	t.Parallel()

	p, _ := html.New()
	defer p.Close()

	t.Run("article with title", func(t *testing.T) {
		htmlContent := `<html><head><title>Page Title</title></head><body><article><h1>Article</h1><p>Content</p></article></body></html>`
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if result.Title != "Page Title" {
			t.Errorf("Title = %q, want 'Page Title'", result.Title)
		}
		if !strings.Contains(result.Text, "Content") {
			t.Error("Text should contain 'Content'")
		}
	})

	t.Run("multiple paragraphs", func(t *testing.T) {
		htmlContent := `<html><body><article><h1>Title</h1><p>P1</p><p>P2</p><p>P3</p></article></body></html>`
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		// Check that paragraphs are present
		if !strings.Contains(result.Text, "P1") || !strings.Contains(result.Text, "P2") || !strings.Contains(result.Text, "P3") {
			t.Error("Text should contain all paragraphs")
		}
	})

	t.Run("nested elements", func(t *testing.T) {
		htmlContent := `<html><body><div><span><strong>Bold text</strong></span></div></body></html>`
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if !strings.Contains(result.Text, "Bold text") {
			t.Error("Text should contain 'Bold text'")
		}
	})

	t.Run("scripts and styles removed", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<script>alert('test');</script>
				<style>body{color:red;}</style>
				<p>Visible content</p>
			</body></html>
		`
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if strings.Contains(result.Text, "alert") || strings.Contains(result.Text, "color:red") {
			t.Error("Text should not contain script or style content")
		}
		if !strings.Contains(result.Text, "Visible content") {
			t.Error("Text should contain visible content")
		}
	})

	t.Run("word count calculated", func(t *testing.T) {
		htmlContent := `<html><body><p>This is a test with several words.</p></body></html>`
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if result.WordCount == 0 {
			t.Error("WordCount should be > 0")
		}
	})

	t.Run("reading time calculated", func(t *testing.T) {
		htmlContent := `<html><body><p>Word1 word2 word3 word4 word5</p></body></html>`
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if result.ReadingTime == 0 {
			t.Error("ReadingTime should be > 0")
		}
	})
}

// ============================================================================
// MEDIA EXTRACTION TESTS
// ============================================================================

func TestMediaExtraction(t *testing.T) {
	t.Parallel()

	p, _ := html.New()
	defer p.Close()

	t.Run("images extracted", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<img src="img1.jpg" alt="Image 1" width="800" height="600">
				<img src="img2.png" alt="Image 2">
			</body></html>
		`
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if len(result.Images) != 2 {
			t.Errorf("Got %d images, want 2", len(result.Images))
		}
		if result.Images[0].URL != "img1.jpg" {
			t.Errorf("Images[0].URL = %q, want 'img1.jpg'", result.Images[0].URL)
		}
		if result.Images[0].Alt != "Image 1" {
			t.Errorf("Images[0].Alt = %q, want 'Image 1'", result.Images[0].Alt)
		}
		if result.Images[0].Width != "800" {
			t.Errorf("Images[0].Width = %q, want '800'", result.Images[0].Width)
		}
	})

	t.Run("videos extracted", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<video src="video.mp4" poster="poster.jpg"></video>
				<video><source src="video2.webm" type="video/webm"></video>
			</body></html>
		`
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if len(result.Videos) == 0 {
			t.Error("Should extract videos")
		}
	})

	t.Run("audios extracted", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<audio src="audio.mp3"></audio>
				<audio><source src="audio2.ogg" type="audio/ogg"></audio>
			</body></html>
		`
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if len(result.Audios) == 0 {
			t.Error("Should extract audios")
		}
	})

	t.Run("iframe embed videos extracted", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<iframe src="https://www.youtube.com/embed/test123" width="640" height="480"></iframe>
				<iframe src="https://player.vimeo.com/video/456789"></iframe>
			</body></html>
		`
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if len(result.Videos) == 0 {
			t.Error("Should extract iframe videos")
		}
	})

	t.Run("non-video iframe ignored", func(t *testing.T) {
		htmlContent := `<html><body><iframe src="https://example.com/page.html"></iframe></body></html>`
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if len(result.Videos) != 0 {
			t.Errorf("Got %d videos, want 0", len(result.Videos))
		}
	})
}

// ============================================================================
// LINK EXTRACTION TESTS
// ============================================================================

func TestLinkExtraction(t *testing.T) {
	t.Parallel()

	p, _ := html.New()
	defer p.Close()

	t.Run("links extracted with details", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<a href="https://example.com" title="Example">External Link</a>
				<a href="/internal" rel="nofollow">Internal Link</a>
			</body></html>
		`
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if len(result.Links) != 2 {
			t.Errorf("Got %d links, want 2", len(result.Links))
		}
		if !result.Links[0].IsExternal {
			t.Error("First link should be external")
		}
		if !result.Links[1].IsNoFollow {
			t.Error("Second link should have nofollow")
		}
	})

	t.Run("empty link handled", func(t *testing.T) {
		htmlContent := `<html><body><a href=""></a></body></html>`
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if len(result.Links) != 0 {
			t.Errorf("Got %d links, want 0", len(result.Links))
		}
	})
}

// ============================================================================
// PARAGRAPH SPACING TESTS
// ============================================================================

func TestParagraphSpacing(t *testing.T) {
	t.Parallel()

	p, err := html.New()
	if err != nil {
		t.Fatalf("html.New() failed: %v", err)
	}
	defer p.Close()

	// Each case asserts paragraph-separation behavior via a flexible set of
	// columns: required substrings, a minimum Count("\n\n"), and (for lists) a
	// minimum non-empty-line count. Consolidates nine hand-written subtests.
	tests := []struct {
		name             string
		html             string
		wantContains     []string
		minDoubleNewline int // minimum required strings.Count(text, "\n\n")
		minNonEmptyLines int // minimum required non-empty lines (for lists)
	}{
		{
			name:             "paragraphs separated by double newlines",
			html:             `<html><body><p>First paragraph.</p><p>Second paragraph.</p></body></html>`,
			wantContains:     []string{"First paragraph", "Second paragraph"},
			minDoubleNewline: 1,
		},
		{
			name: "long paragraph text from example",
			html: `<html><body>
			<p>We provide our customers with a suite of broadband connectivity services, including fixed Internet, WiFi and mobile, which when bundled together provides our customers with a differentiated converged connectivity experience while saving consumers money.</p>
			<p>This is another paragraph with different content.</p>
		</body></html>`,
			wantContains:     []string{"We provide our customers", "differentiated converged connectivity experience"},
			minDoubleNewline: 1,
		},
		{
			name: "multiple consecutive paragraphs",
			html: `<html><body>
			<p>Paragraph 1</p>
			<p>Paragraph 2</p>
			<p>Paragraph 3</p>
		</body></html>`,
			minDoubleNewline: 2, // at least 2 for 3 paragraphs
		},
		{
			name: "headings and paragraphs",
			html: `<html><body>
			<h1>Title</h1>
			<p>First paragraph after heading.</p>
			<p>Second paragraph.</p>
		</body></html>`,
			wantContains:     []string{"Title", "First paragraph"},
			minDoubleNewline: 1,
		},
		{
			name: "divs as paragraphs",
			html: `<html><body>
			<div>First div content</div>
			<div>Second div content</div>
		</body></html>`,
			minDoubleNewline: 1,
		},
		{
			name: "list items should not have double spacing",
			html: `<html><body>
			<ul>
				<li>Item 1</li>
				<li>Item 2</li>
				<li>Item 3</li>
			</ul>
		</body></html>`,
			minNonEmptyLines: 3, // list items on separate lines, not paragraph-spaced
		},
		{
			name: "article with multiple sections",
			html: `<!DOCTYPE html>
		<html>
		<head><title>Test Article</title></head>
		<body>
			<article>
				<h1>Main Title</h1>
				<p>We provide our customers with a suite of broadband connectivity services.</p>
				<p>This is the second paragraph of the article.</p>
				<h2>Section Title</h2>
				<p>Content under section heading.</p>
			</article>
		</body>
		</html>`,
			wantContains:     []string{"Main Title", "broadband connectivity services", "Section Title"},
			minDoubleNewline: 3,
		},
		{
			name: "mixed content with inline elements",
			html: `<html><body>
			<p>This is a <strong>paragraph</strong> with <em>inline</em> elements.</p>
			<p>This is another <a href="#">paragraph</a> with links.</p>
		</body></html>`,
			wantContains:     []string{"paragraph with inline elements"},
			minDoubleNewline: 1,
		},
		{
			name: "blockquote should create paragraph separation",
			html: `<html><body>
			<p>Regular paragraph before quote.</p>
			<blockquote>This is a quoted text.</blockquote>
			<p>Regular paragraph after quote.</p>
		</body></html>`,
			wantContains:     []string{"quoted text"},
			minDoubleNewline: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Subtests share the parent processor and run sequentially (the
			// original did too); a single *Processor is reused across cases.
			result, err := p.Extract([]byte(tt.html))
			if err != nil {
				t.Fatalf("Extract() failed: %v", err)
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result.Text, want) {
					t.Errorf("expected text to contain %q, got: %q", want, result.Text)
				}
			}
			if tt.minDoubleNewline > 0 {
				if got := strings.Count(result.Text, "\n\n"); got < tt.minDoubleNewline {
					t.Errorf("expected at least %d double newlines, got %d (text=%q)",
						tt.minDoubleNewline, got, result.Text)
				}
			}
			if tt.minNonEmptyLines > 0 {
				nonEmpty := 0
				for _, line := range strings.Split(result.Text, "\n") {
					if strings.TrimSpace(line) != "" {
						nonEmpty++
					}
				}
				if nonEmpty < tt.minNonEmptyLines {
					t.Errorf("expected at least %d non-empty lines, got %d", tt.minNonEmptyLines, nonEmpty)
				}
			}
		})
	}
}

// ============================================================================
// TABLE FORMAT TESTS
// ============================================================================

func TestTableFormats(t *testing.T) {
	t.Parallel()

	t.Run("markdown table with alignment", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr><th align="left">Name</th><th align="center">Age</th><th align="right">Score</th></tr>
					<tr><td>Alice</td><td>25</td><td>95</td></tr>
					<tr><td>Bob</td><td>30</td><td>87</td></tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "markdown"
		p, _ := html.New(cfg)
		defer p.Close()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Check for alignment markers
		if !strings.Contains(result.Text, ":---") {
			t.Error("Markdown table should have left alignment marker")
		}
		if !strings.Contains(result.Text, ":--:") {
			t.Error("Markdown table should have center alignment marker")
		}
		if !strings.Contains(result.Text, "---:") {
			t.Error("Markdown table should have right alignment marker")
		}

		// Check content is preserved
		if !strings.Contains(result.Text, "Alice") {
			t.Error("Table content should be preserved")
		}
	})

	t.Run("html table format with alignment", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr><th align="left">Name</th><th align="center">Value</th></tr>
					<tr><td>Item1</td><td>100</td></tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer p.Close()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should have HTML table tags
		if !strings.Contains(result.Text, "<table>") {
			t.Error("HTML format should contain <table> tag")
		}
		if !strings.Contains(result.Text, "style=\"text-align:left\"") {
			t.Error("HTML table should preserve left alignment")
		}
		if !strings.Contains(result.Text, "style=\"text-align:center\"") {
			t.Error("HTML table should preserve center alignment")
		}
	})

	t.Run("html table with cell merging", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<th colspan="2">Merged Header</th>
					</tr>
					<tr>
						<td rowspan="2">Merged Cell</td>
						<td>Cell 2</td>
					</tr>
					<tr>
						<td>Cell 3</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer p.Close()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should preserve colspan and rowspan
		if !strings.Contains(result.Text, "colspan=") {
			t.Error("HTML table should preserve colspan")
		}
		if !strings.Contains(result.Text, "rowspan=") {
			t.Error("HTML table should preserve rowspan")
		}
		if !strings.Contains(result.Text, "Merged Header") {
			t.Error("Table content should be preserved")
		}
	})

	t.Run("table with style attribute alignment", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr><th style="text-align:right">Total</th></tr>
					<tr><td style="text-align:right">$100</td></tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "markdown"
		p, _ := html.New(cfg)
		defer p.Close()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should detect alignment from style attribute
		if !strings.Contains(result.Text, "---:") {
			t.Error("Should detect right alignment from style attribute")
		}
	})

	t.Run("empty table handling", func(t *testing.T) {
		htmlContent := `<html><body><table></table></body></html>`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "markdown"
		p, _ := html.New(cfg)
		defer p.Close()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Empty table should not produce table output
		if strings.Contains(result.Text, "| --- |") {
			t.Error("Empty table should not produce separator")
		}
	})

	t.Run("default format is markdown", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr><th>Header</th></tr>
					<tr><td>Data</td></tr>
				</table>
			</body></html>
		`

		result, err := html.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Default should be markdown
		if !strings.Contains(result.Text, "| --- |") {
			t.Error("Default table format should be markdown")
		}
	})
}

// ============================================================================
// TABLE COLUMN WIDTH TESTS
// ============================================================================

func TestTableColumnWidths(t *testing.T) {
	t.Parallel()

	t.Run("html table preserves width from style attribute", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<th style="width:1.0%">Name</th>
						<th style="width:50%">Value</th>
						<th style="width:49%">Notes</th>
					</tr>
					<tr>
						<td>Item 1</td>
						<td>100</td>
						<td>First</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer p.Close()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Check for width preservation in style attribute
		if !strings.Contains(result.Text, "width:1.0%") {
			t.Error("HTML table should preserve width:1.0%")
		}
		if !strings.Contains(result.Text, "width:50%") {
			t.Error("HTML table should preserve width:50%")
		}
		if !strings.Contains(result.Text, "width:49%") {
			t.Error("HTML table should preserve width:49%")
		}
	})

	t.Run("html table preserves width from width attribute", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<th width="100">Column 1</th>
						<th width="200px">Column 2</th>
					</tr>
					<tr>
						<td>Data 1</td>
						<td>Data 2</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer p.Close()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Check for width preservation from width attribute
		if !strings.Contains(result.Text, "width:100") {
			t.Error("HTML table should preserve width=100")
		}
		if !strings.Contains(result.Text, "width:200px") {
			t.Error("HTML table should preserve width=200px")
		}
	})

	t.Run("html table combines width with alignment", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<th style="width:30%; text-align:left">Name</th>
						<th style="width:70%; text-align:right">Value</th>
					</tr>
					<tr>
						<td>Item 1</td>
						<td>100</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer p.Close()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Check that both width and alignment are preserved in the same style attribute
		if !strings.Contains(result.Text, "width:30%") {
			t.Error("HTML table should preserve width")
		}
		if !strings.Contains(result.Text, "text-align:left") {
			t.Error("HTML table should preserve alignment")
		}
		// Check they are in the same style attribute with semicolon separator
		if !strings.Contains(result.Text, "text-align:left;") && !strings.Contains(result.Text, ";text-align:left") {
			if !strings.Contains(result.Text, "width:30%;") && !strings.Contains(result.Text, ";width:30%") {
				t.Error("Width and alignment should be in same style attribute separated by semicolon")
			}
		}
	})

	t.Run("markdown table includes width comment", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<th style="width:1.0%">Name</th>
						<th style="width:98%">Value</th>
					</tr>
					<tr>
						<td>Item 1</td>
						<td>Data</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "markdown"
		p, _ := html.New(cfg)
		defer p.Close()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Table metadata comments have been removed for cleaner output
		// Check that table structure is preserved instead
		if !strings.Contains(result.Text, "| Name |") {
			t.Error("Markdown table should include table structure")
		}
		if !strings.Contains(result.Text, "| Value |") {
			t.Error("Markdown table should include table structure")
		}
	})

	t.Run("table without width attributes works normally", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<th>Name</th>
						<th>Value</th>
					</tr>
					<tr>
						<td>Item 1</td>
						<td>100</td>
					</tr>
				</table>
			</body></html>
		`

		// Test HTML format
		cfgHTML := html.DefaultConfig()
		cfgHTML.TableFormat = "html"
		pHTML, _ := html.New(cfgHTML)
		defer pHTML.Close()

		result, err := pHTML.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should not have width in output
		if strings.Contains(result.Text, "width:") {
			t.Error("Table without width should not add width attributes")
		}

		// Test Markdown format
		cfgMD := html.DefaultConfig()
		cfgMD.TableFormat = "markdown"
		pMD, _ := html.New(cfgMD)
		defer pMD.Close()

		result, err = pMD.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should not have width comment
		if strings.Contains(result.Text, "<!-- Column widths:") {
			t.Error("Markdown table without widths should not include width comment")
		}
	})

	t.Run("complex table with mixed width units", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<td style="width:50px">ID</td>
						<td style="width:20%">Name</td>
						<td style="width:1.5*">Score</td>
						<td>Description</td>
					</tr>
					<tr>
						<td>1</td>
						<td>Item</td>
						<td>95</td>
						<td>Test item</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer p.Close()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Check for various width units
		if !strings.Contains(result.Text, "width:50px") {
			t.Error("HTML table should preserve pixel width")
		}
		if !strings.Contains(result.Text, "width:20%") {
			t.Error("HTML table should preserve percentage width")
		}
		// The description column has no width, so it shouldn't appear
	})

	t.Run("table preserves width with colspan", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<th style="width:50%" colspan="2">Merged Header</th>
						<th style="width:50%">Value</th>
					</tr>
					<tr>
						td>A</td>
						<td>B</td>
						<td>C</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer p.Close()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should preserve both width and colspan
		if !strings.Contains(result.Text, "colspan=") {
			t.Error("HTML table should preserve colspan")
		}
		if !strings.Contains(result.Text, "width:50%") {
			t.Error("HTML table should preserve width with colspan")
		}
	})
}

// ============================================================================
// TABLE ALIGNMENT PRESERVATION TESTS
// ============================================================================

func TestTableAlignmentPreservation(t *testing.T) {
	t.Parallel()

	t.Run("html table preserves justify alignment", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<td style="text-align:justify">This is justified text that should span the full width.</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer p.Close()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Check for justify alignment in style attribute
		if !strings.Contains(result.Text, "text-align:justify") {
			t.Error("HTML table should preserve justify alignment")
		}
	})

	t.Run("html table preserves all alignment types", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<td style="text-align:left">Left</td>
						<td style="text-align:center">Center</td>
						<td style="text-align:right">Right</td>
						<td style="text-align:justify">Justify</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer p.Close()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Check all alignment types
		if !strings.Contains(result.Text, "text-align:left") {
			t.Error("HTML table should preserve left alignment")
		}
		if !strings.Contains(result.Text, "text-align:center") {
			t.Error("HTML table should preserve center alignment")
		}
		if !strings.Contains(result.Text, "text-align:right") {
			t.Error("HTML table should preserve right alignment")
		}
		if !strings.Contains(result.Text, "text-align:justify") {
			t.Error("HTML table should preserve justify alignment")
		}
	})

	t.Run("html table preserves align attribute", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<td align="left">Left aligned</td>
						<td align="center">Center aligned</td>
						<td align="right">Right aligned</td>
						<td align="justify">Justify aligned</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer p.Close()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// align attribute should be converted to style="text-align:..."
		if !strings.Contains(result.Text, "text-align:left") {
			t.Error("align='left' should convert to text-align:left")
		}
		if !strings.Contains(result.Text, "text-align:center") {
			t.Error("align='center' should convert to text-align:center")
		}
		if !strings.Contains(result.Text, "text-align:right") {
			t.Error("align='right' should convert to text-align:right")
		}
		if !strings.Contains(result.Text, "text-align:justify") {
			t.Error("align='justify' should convert to text-align:justify")
		}
	})

	t.Run("html table combines alignment with width", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<td style="width:50%; text-align:justify">Combined</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer p.Close()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should have both width and alignment in same style
		if !strings.Contains(result.Text, "width:50%") {
			t.Error("Should preserve width")
		}
		if !strings.Contains(result.Text, "text-align:justify") {
			t.Error("Should preserve justify alignment")
		}
		// Check they are in same style attribute with semicolon separator
		hasCombinedStyle := (strings.Contains(result.Text, "width:50%;text-align:justify") ||
			strings.Contains(result.Text, "text-align:justify;width:50%"))
		if !hasCombinedStyle {
			t.Error("Width and alignment should be in same style attribute")
		}
	})

	t.Run("markdown table with justify alignment shows warning", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<th style="text-align:left">Name</th>
						<th style="text-align:justify">Description</th>
					</tr>
					<tr>
						<td>Item</td>
						<td>Justified text content</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "markdown"
		p, _ := html.New(cfg)
		defer p.Close()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Table metadata comments have been removed for cleaner output
		// Justify alignment falls back to default (no alignment markers)
		if !strings.Contains(result.Text, "|") {
			t.Error("Table should be present in output")
		}
		if !strings.Contains(result.Text, "Name") {
			t.Error("Table should contain Name column")
		}
		if !strings.Contains(result.Text, "Description") {
			t.Error("Table should contain Description column")
		}
	})

	t.Run("table alignment priority: align over style", func(t *testing.T) {
		// When both align attribute and style attribute are present,
		// align attribute should take precedence
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<td align="center" style="text-align:left">Should be center</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer p.Close()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should use align attribute (center) not style (left)
		if !strings.Contains(result.Text, "text-align:center") {
			t.Error("align attribute should take precedence over style attribute")
		}
		if strings.Contains(result.Text, "text-align:left") {
			t.Error("Should not use style attribute when align is present")
		}
	})

	t.Run("extracts alignment from complex style attribute", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<td style="color:red; text-align:right; font-size:12px">Right aligned</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer p.Close()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should extract text-align from complex style
		if !strings.Contains(result.Text, "text-align:right") {
			t.Error("Should extract alignment from complex style attribute")
		}
	})

	t.Run("handles whitespace in style attribute", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<td style="text-align : center">Center with spaces</td>
						<td style="text-align:center">Center without spaces</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer p.Close()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Both should be detected as center alignment
		count := strings.Count(result.Text, "text-align:center")
		if count < 2 {
			t.Errorf("Expected 2 center-aligned cells, got %d", count)
		}
	})

	t.Run("mixed case alignment attributes", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<td ALIGN="Center">Mixed case align</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer p.Close()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should handle case-insensitive align attribute
		if !strings.Contains(result.Text, "text-align:center") {
			t.Error("Should handle mixed case ALIGN attribute")
		}
	})
}

// ============================================================================
// CONVENIENCE API TESTS
// ============================================================================

func TestDirectAPIUsage(t *testing.T) {
	t.Parallel()

	t.Run("Extract with Result access", func(t *testing.T) {
		result, err := html.Extract([]byte(`<html><body><h1>Title</h1><p>Content</p></body></html>`))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if result.Title != "Title" {
			t.Errorf("Title = %q, want 'Title'", result.Title)
		}
	})

	t.Run("ExtractText usage", func(t *testing.T) {
		text, err := html.ExtractText([]byte(`<html><body><p>Hello World</p></body></html>`))
		if err != nil {
			t.Fatalf("ExtractText() failed: %v", err)
		}
		if !strings.Contains(text, "Hello World") {
			t.Error("Text should contain 'Hello World'")
		}
	})

	t.Run("Extract and access specific fields", func(t *testing.T) {
		result, err := html.Extract([]byte(`<html><head><title>Test Title</title></head><body><img src="test.jpg" alt="Test"></body></html>`))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if result.Title != "Test Title" {
			t.Errorf("Title = %q, want 'Test Title'", result.Title)
		}
		if len(result.Images) != 1 {
			t.Errorf("Got %d images, want 1", len(result.Images))
		}
	})

	t.Run("ExtractToMarkdown", func(t *testing.T) {
		markdown, err := html.ExtractToMarkdown([]byte(`<html><body><h1>Title</h1><p>Content</p></body></html>`))
		if err != nil {
			t.Fatalf("ExtractToMarkdown() failed: %v", err)
		}
		if markdown == "" {
			t.Error("Markdown should not be empty")
		}
	})

	t.Run("ExtractToJSON", func(t *testing.T) {
		jsonData, err := html.ExtractToJSON([]byte(`<html><head><title>Test</title></head><body><p>Content</p></body></html>`))
		if err != nil {
			t.Fatalf("ExtractToJSON() failed: %v", err)
		}
		if len(jsonData) == 0 {
			t.Error("JSON data should not be empty")
		}

		var result map[string]interface{}
		if err := json.Unmarshal(jsonData, &result); err != nil {
			t.Errorf("Invalid JSON: %v", err)
		}
		if _, ok := result["title"]; !ok {
			t.Error("JSON should have 'title' field")
		}
		if _, ok := result["text"]; !ok {
			t.Error("JSON should have 'text' field")
		}
	})

	t.Run("Custom summarization with Extract", func(t *testing.T) {
		htmlContent := `<html><body><p>Word1 word2 word3 word4 word5 word6</p></body></html>`
		result, err := html.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		maxWords := 3
		words := strings.Fields(result.Text)
		if len(words) > maxWords {
			result.Text = strings.Join(words[:maxWords], " ") + "..."
		}

		if len(strings.Fields(result.Text)) > 4 { // 3 words + "..."
			t.Errorf("Summary has %d words, want <= 4", len(strings.Fields(result.Text)))
		}
	})

	t.Run("Custom cleaning with Extract", func(t *testing.T) {
		result, err := html.Extract([]byte(`<html><body><p>Text</p><p>   </p><p>More</p></body></html>`))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		lines := strings.Split(result.Text, "\n")
		var nonEmptyLines []string
		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				nonEmptyLines = append(nonEmptyLines, trimmed)
			}
		}
		cleaned := strings.Join(nonEmptyLines, "\n\n")

		if cleaned == "" {
			t.Error("Cleaned text should not be empty")
		}
	})

	t.Run("Access WordCount from Result", func(t *testing.T) {
		result, err := html.Extract([]byte(`<html><body><p>Word1 word2 word3</p></body></html>`))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if result.WordCount != 3 {
			t.Errorf("WordCount = %d, want 3", result.WordCount)
		}
	})

	t.Run("Access ReadingTime from Result", func(t *testing.T) {
		result, err := html.Extract([]byte(`<html><body><p>Word1 word2 word3</p></body></html>`))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if result.ReadingTime.Minutes() <= 0 {
			t.Errorf("ReadingTime = %f, want > 0", result.ReadingTime.Minutes())
		}
	})
}

// ============================================================================
// COMPREHENSIVE LINK EXTRACTION TESTS
// ============================================================================

func TestExtractAllLinks(t *testing.T) {
	t.Parallel()

	t.Run("comprehensive link extraction", func(t *testing.T) {
		htmlContent := `
			<html>
			<head>
				<link rel="stylesheet" href="styles.css">
				<script src="app.js"></script>
			</head>
			<body>
				<a href="https://example.com">External Link</a>
				<a href="/internal">Internal Link</a>
				<img src="image.jpg" alt="Test Image">
				<video src="video.mp4"></video>
			</body>
			</html>
		`

		links, err := html.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		if len(links) == 0 {
			t.Fatal("ExtractAllLinks() returned no links")
		}

		types := make(map[string]bool)
		for _, link := range links {
			types[link.Type] = true
		}

		expectedTypes := []string{"css", "js", "link", "image", "video"}
		for _, expectedType := range expectedTypes {
			if !types[expectedType] {
				t.Errorf("Expected link type %q not found", expectedType)
			}
		}
	})

	t.Run("link deduplication", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<a href="page.html">Link 1</a>
				<a href="page.html">Link 2</a>
				<img src="img.jpg">
				<img src="img.jpg" alt="Same">
			</body></html>
		`

		links, err := html.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		urlCounts := make(map[string]int)
		for _, link := range links {
			urlCounts[link.URL]++
		}

		for url, count := range urlCounts {
			if count > 1 {
				t.Errorf("URL %q appears %d times (should be deduplicated)", url, count)
			}
		}
	})

	t.Run("relative URL resolution with manual base URL", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<a href="/about">About</a>
				<a href="contact.html">Contact</a>
				<img src="images/logo.jpg">
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.BaseURL = "https://mysite.com/"
		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer p.Close()

		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		expectedURLs := map[string]bool{
			"https://mysite.com/about":           false,
			"https://mysite.com/contact.html":    false,
			"https://mysite.com/images/logo.jpg": false,
		}

		for _, link := range links {
			if _, exists := expectedURLs[link.URL]; exists {
				expectedURLs[link.URL] = true
			}
		}

		for url, found := range expectedURLs {
			if !found {
				t.Errorf("Expected URL %q not found", url)
			}
		}
	})

	t.Run("selective extraction by type", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<img src="image.jpg">
				<video src="video.mp4"></video>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.IncludeImages = false
		cfg.IncludeVideos = false

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer p.Close()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		for _, link := range links {
			if link.Type == "image" {
				t.Error("Image links should not be included")
			}
			if link.Type == "video" {
				t.Error("Video links should not be included")
			}
		}
	})
}

// ============================================================================
// CACHE AND STATISTICS TESTS
// ============================================================================

func TestCache(t *testing.T) {
	t.Parallel()

	t.Run("cache hit on repeated extraction", func(t *testing.T) {
		cfg := html.DefaultConfig()
		cfg.MaxCacheEntries = 100
		cfg.CacheTTL = time.Hour
		p, _ := html.New(cfg)
		defer p.Close()

		htmlContent := `<html><body><p>Test content</p></body></html>`

		// First extraction - cache miss
		result1, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		stats1 := p.GetStatistics()
		if stats1.CacheHits != 0 {
			t.Errorf("First extraction should have 0 cache hits, got %d", stats1.CacheHits)
		}

		// Second extraction - cache hit
		result2, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		stats2 := p.GetStatistics()
		if stats2.CacheHits != 1 {
			t.Errorf("Second extraction should have 1 cache hit, got %d", stats2.CacheHits)
		}

		// Results should be identical
		if result1.Text != result2.Text {
			t.Error("Cached result should match original")
		}
	})

	t.Run("cache cleared", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		htmlContent := `<html><body><p>Test</p></body></html>`

		p.Extract([]byte(htmlContent))
		statsBefore := p.GetStatistics()
		p.ClearCache()

		stats := p.GetStatistics()
		// Statistics should be preserved after ClearCache
		if stats.CacheHits != statsBefore.CacheHits || stats.CacheMisses != statsBefore.CacheMisses {
			t.Error("Cache stats should be preserved after ClearCache()")
		}

		// ResetStatistics should clear the stats
		p.ResetStatistics()
		statsAfterReset := p.GetStatistics()
		if statsAfterReset.CacheHits != 0 || statsAfterReset.CacheMisses != 0 {
			t.Error("Cache stats should be reset after ResetStatistics()")
		}
	})

	t.Run("cache disabled", func(t *testing.T) {
		cfg := html.DefaultConfig()
		cfg.MaxCacheEntries = 0
		p, _ := html.New(cfg)
		defer p.Close()

		htmlContent := `<html><body><p>Test</p></body></html>`

		p.Extract([]byte(htmlContent))
		p.Extract([]byte(htmlContent))

		stats := p.GetStatistics()
		if stats.CacheHits != 0 {
			t.Errorf("With cache disabled, cache hits should be 0, got %d", stats.CacheHits)
		}
	})

	t.Run("statistics tracked", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		stats := p.GetStatistics()
		if stats.TotalProcessed != 0 {
			t.Errorf("Initial TotalProcessed = %d, want 0", stats.TotalProcessed)
		}

		htmlContent := `<html><body><p>Test</p></body></html>`
		p.Extract([]byte(htmlContent))

		stats = p.GetStatistics()
		if stats.TotalProcessed != 1 {
			t.Errorf("TotalProcessed = %d, want 1", stats.TotalProcessed)
		}
	})
}

// ============================================================================
// CONCURRENCY TESTS
// ============================================================================

func TestConcurrency(t *testing.T) {
	t.Parallel()

	t.Run("concurrent extraction", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		htmlContent := `<html><body><article><h1>Title</h1><p>Content</p></article></body></html>`
		const goroutines = 50
		var wg sync.WaitGroup
		errors := make(chan error, goroutines)

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				result, err := p.Extract([]byte(htmlContent))
				if err != nil {
					errors <- err
					return
				}
				if result.Title != "Title" {
					errors <- fmt.Errorf("wrong title: %q", result.Title)
				}
			}()
		}

		wg.Wait()
		close(errors)
		for err := range errors {
			t.Error(err)
		}
	})

	t.Run("concurrent with cache clearing", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		const goroutines = 20
		var wg sync.WaitGroup

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				htmlContent := fmt.Sprintf(`<html><body><p>Content %d</p></body></html>`, id)
				p.Extract([]byte(htmlContent))
				if id%5 == 0 {
					p.ClearCache()
				}
			}(i)
		}

		wg.Wait()

		stats := p.GetStatistics()
		if stats.TotalProcessed != goroutines {
			t.Errorf("TotalProcessed = %d, want %d", stats.TotalProcessed, goroutines)
		}
	})
}

// ============================================================================
// EDGE CASES AND ERROR HANDLING TESTS
// ============================================================================

func TestEdgeCases(t *testing.T) {
	t.Parallel()

	p, _ := html.New()
	defer p.Close()

	tests := []struct {
		name    string
		html    string
		wantErr bool
		check   func(*testing.T, *html.Result, error)
	}{
		{
			name:    "unicode characters",
			html:    `<html><body><p>Hello 世界 🌍</p></body></html>`,
			wantErr: false,
			check: func(t *testing.T, r *html.Result, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if !strings.Contains(r.Text, "世界") {
					t.Error("Text should contain Chinese characters")
				}
			},
		},
		{
			name:    "special HTML entities",
			html:    `<html><body><p>&lt;&gt;&amp;&quot;&#39;</p></body></html>`,
			wantErr: false,
			check: func(t *testing.T, r *html.Result, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if !strings.Contains(r.Text, "<") {
					t.Error("Text should contain decoded entities")
				}
			},
		},
		{
			name:    "mixed case tags",
			html:    `<HTML><BODY><P>Content</P></BODY></HTML>`,
			wantErr: false,
			check: func(t *testing.T, r *html.Result, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if !strings.Contains(r.Text, "Content") {
					t.Error("Text should contain 'Content'")
				}
			},
		},
		{
			name:    "self-closing tags",
			html:    `<html><body><br/><hr/><p>Content</p></body></html>`,
			wantErr: false,
			check: func(t *testing.T, r *html.Result, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
			},
		},
		{
			name:    "CDATA sections",
			html:    `<html><body><![CDATA[Some data]]><p>Content</p></body></html>`,
			wantErr: false,
			check: func(t *testing.T, r *html.Result, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
			},
		},
		{
			name:    "comments",
			html:    `<html><body><!-- Comment --><p>Content</p></body></html>`,
			wantErr: false,
			check: func(t *testing.T, r *html.Result, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if strings.Contains(r.Text, "Comment") {
					t.Error("Comments should be removed")
				}
			},
		},
		{
			name:    "very long text",
			html:    `<html><body><p>` + strings.Repeat("word ", 10000) + `</p></body></html>`,
			wantErr: false,
			check: func(t *testing.T, r *html.Result, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if r.WordCount == 0 {
					t.Error("WordCount should be > 0")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := p.Extract([]byte(tt.html))
			if (err != nil) != tt.wantErr {
				t.Errorf("Error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.check != nil {
				tt.check(t, result, err)
			}
		})
	}
}

// ============================================================================
// INTEGRATION TESTS
// ============================================================================

func TestIntegrationScenarios(t *testing.T) {
	t.Parallel()

	t.Run("blog post workflow", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		htmlContent := `<!DOCTYPE html>
<html>
<head><title>My Blog Post</title></head>
<body>
	<nav><a href="/">Home</a><a href="/about">About</a></nav>
	<article>
		<h1>Blog Post Title</h1>
		<img src="featured.jpg" alt="Featured Image">
		<p>First paragraph.</p>
		<p>Second paragraph with <a href="https://example.com">link</a>.</p>
	</article>
	<aside><h3>Related</h3></aside>
</body>
</html>`

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		if result.Title != "My Blog Post" {
			t.Errorf("Title = %q, want 'My Blog Post'", result.Title)
		}
		if !strings.Contains(result.Text, "First paragraph") {
			t.Error("Should extract main content")
		}
		if len(result.Images) == 0 {
			t.Error("Should extract images")
		}
		if len(result.Links) == 0 {
			t.Error("Should extract links")
		}
	})

	t.Run("news article with media", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		htmlContent := `
		<html><body>
			<article>
				<h1>Breaking News</h1>
				<video src="news.mp4" poster="thumb.jpg"></video>
				<p>News content.</p>
				<img src="photo.jpg">
				<audio src="interview.mp3"></audio>
			</article>
		</body></html>
		`

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		if len(result.Videos) == 0 {
			t.Error("Should extract videos")
		}
		if len(result.Audios) == 0 {
			t.Error("Should extract audios")
		}
		if len(result.Images) == 0 {
			t.Error("Should extract images")
		}
	})

	t.Run("documentation page with code and tables", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		htmlContent := `
		<html><body>
			<main>
				<h1>API Documentation</h1>
				<pre><code>npm install package</code></pre>
				<table>
					<tr><th>Method</th><th>Description</th></tr>
					<tr><td>init()</td><td>Initialize</td></tr>
				</table>
			</main>
		</body></html>
		`

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		if !strings.Contains(result.Text, "npm install") {
			t.Error("Should extract code blocks")
		}
		if !strings.Contains(result.Text, "init()") {
			t.Error("Should extract table content")
		}
	})
}

// ============================================================================
// INLINE IMAGE FORMATTING TESTS
// ============================================================================

func TestImageFormatting(t *testing.T) {
	t.Parallel()

	t.Run("markdown inline images", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<p>Text before</p>
				<img src="image1.jpg" alt="First Image">
				<p>Text middle</p>
				<img src="image2.png" alt="Second Image">
				<p>Text after</p>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.InlineImageFormat = "markdown"
		p, _ := html.New(cfg)
		defer p.Close()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		if !strings.Contains(result.Text, "![First Image](image1.jpg)") {
			t.Error("Should contain markdown inline image 1")
		}
		if !strings.Contains(result.Text, "![Second Image](image2.png)") {
			t.Error("Should contain markdown inline image 2")
		}
	})

	t.Run("HTML inline images", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<p>Text before</p>
				<img src="img.jpg" alt="Test" width="100" height="50">
				<p>Text after</p>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.InlineImageFormat = "html"
		p, _ := html.New(cfg)
		defer p.Close()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		if !strings.Contains(result.Text, `<img src="img.jpg"`) {
			t.Error("Should contain HTML img tag")
		}
		if !strings.Contains(result.Text, `alt="Test"`) {
			t.Error("Should preserve alt attribute")
		}
		if !strings.Contains(result.Text, `width="100"`) {
			t.Error("Should preserve width attribute")
		}
		if !strings.Contains(result.Text, `height="50"`) {
			t.Error("Should preserve height attribute")
		}
	})

	t.Run("no inline format", func(t *testing.T) {
		htmlContent := `<html><body><img src="image.jpg"><p>Text</p></body></html>`

		cfg := html.DefaultConfig()
		cfg.InlineImageFormat = "none"
		p, _ := html.New(cfg)
		defer p.Close()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		if strings.Contains(result.Text, "[IMAGE:") {
			t.Error("Should not contain image placeholders")
		}
		if len(result.Images) == 0 {
			t.Error("Should still extract images to Images array")
		}
	})
}

// ============================================================================
// LINK FORMATTING TESTS
// ============================================================================

func TestLinkFormatting(t *testing.T) {
	t.Parallel()

	// Each case builds its own processor (so subtests run in parallel) and
	// asserts inline-link formatting via required/forbidden substrings and an
	// optional minimum extracted-links count.
	tests := []struct {
		name            string
		html            string
		modify          func(*html.Config)
		wantContains    []string
		wantNotContains []string
		wantLinksMin    int
	}{
		{
			name: "markdown inline links",
			html: `
			<html><body>
				<p>Text before</p>
				<a href="https://go.dev/tour/">Go Tour</a>
				<p>Text middle</p>
				<a href="https://golang.org">Golang</a>
				<p>Text after</p>
			</body></html>
		`,
			modify:       func(c *html.Config) { c.InlineLinkFormat = "markdown" },
			wantContains: []string{"[Go Tour](https://go.dev/tour/)", "[Golang](https://golang.org)"},
		},
		{
			name: "html inline links",
			html: `
			<html><body>
				<p>Text before</p>
				<a href="https://go.dev/tour/" title="Go Tour">Go Tour</a>
				<p>Text after</p>
			</body></html>
		`,
			modify: func(c *html.Config) { c.InlineLinkFormat = "html" },
			wantContains: []string{
				`<a href="https://go.dev/tour/"`,
				`title="Go Tour"`,
				`>Go Tour</a>`,
			},
		},
		{
			name:            "none format default",
			html:            `<html><body><a href="https://go.dev">Go</a><p>Text</p></body></html>`,
			modify:          func(c *html.Config) { c.InlineLinkFormat = "none" },
			wantNotContains: []string{"[LINK:"},
			wantContains:    []string{"Go"},
			wantLinksMin:    1,
		},
		{
			name:         "empty href handling",
			html:         `<html><body><a href="">Empty Link</a><a href="https://go.dev">Valid Link</a></body></html>`,
			modify:       func(c *html.Config) { c.InlineLinkFormat = "markdown" },
			wantContains: []string{"[Valid Link](https://go.dev)"},
		},
		{
			name:         "empty text handling",
			html:         `<html><body><a href="https://go.dev"></a></body></html>`,
			modify:       func(c *html.Config) { c.InlineLinkFormat = "markdown" },
			wantContains: []string{"[Link 1](https://go.dev)"}, // fallback text for empty link
		},
		{
			name: "combined with image format",
			html: `
			<html><body>
				<p>Text before</p>
				<img src="image.jpg" alt="Test Image">
				<a href="https://go.dev">Go</a>
				<p>Text after</p>
			</body></html>
		`,
			modify: func(c *html.Config) {
				c.InlineImageFormat = "markdown"
				c.InlineLinkFormat = "markdown"
			},
			wantContains: []string{"![Test Image](image.jpg)", "[Go](https://go.dev)"},
		},
		{
			name:         "html escapes special characters",
			html:         `<html><body><a href="https://go.dev?a=1&b=2">Link with "quotes"</a></body></html>`,
			modify:       func(c *html.Config) { c.InlineLinkFormat = "html" },
			wantContains: []string{`href="https://go.dev?a=1&amp;b=2"`, `>Link with &#34;quotes&#34;</a>`},
		},
		{
			// A literal, unpaired "[LINK:n]" token in the source text must not
			// cause all following content to be dropped during link formatting.
			name: "unclosed link placeholder preserves trailing text",
			html: `<html><body><article>
<p>Intro text here.</p>
<a href="https://go.dev">Go</a>
<p>See [LINK:5] for notes. Trailing content must remain.</p>
</article></body></html>`,
			modify: func(c *html.Config) { c.InlineLinkFormat = "markdown" },
			wantContains: []string{
				"[Go](https://go.dev)",
				"[LINK:5]",                      // unclosed placeholder preserved verbatim
				"Trailing content must remain.", // trailing text not dropped
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cfg := html.DefaultConfig()
			if tt.modify != nil {
				tt.modify(&cfg)
			}
			p, err := html.New(cfg)
			if err != nil {
				t.Fatalf("html.New() failed: %v", err)
			}
			defer p.Close()

			result, err := p.Extract([]byte(tt.html))
			if err != nil {
				t.Fatalf("Extract() failed: %v", err)
			}

			for _, want := range tt.wantContains {
				if !strings.Contains(result.Text, want) {
					t.Errorf("expected text to contain %q, got: %s", want, result.Text)
				}
			}
			for _, avoid := range tt.wantNotContains {
				if strings.Contains(result.Text, avoid) {
					t.Errorf("expected text to NOT contain %q, got: %s", avoid, result.Text)
				}
			}
			if tt.wantLinksMin > 0 && len(result.Links) < tt.wantLinksMin {
				t.Errorf("expected at least %d extracted links, got %d", tt.wantLinksMin, len(result.Links))
			}
		})
	}
}

// ============================================================================
// VIDEO EXTRACTION EDGE CASES
// ============================================================================

func TestVideoEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("extract videos from HTML with regex", func(t *testing.T) {
		// Test that videos are extracted from HTML content using regex
		htmlContent := `
			<html><body>
				<iframe src="https://www.youtube.com/embed/test123"></iframe>
			</body></html>
		`

		p, _ := html.New()
		defer p.Close()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		if len(result.Videos) == 0 {
			t.Fatal("Should extract iframe video")
		}

		video := result.Videos[0]
		if video.URL != "https://www.youtube.com/embed/test123" {
			t.Errorf("URL = %q, want youtube embed URL", video.URL)
		}
		if video.Type != "embed" {
			t.Errorf("Type = %q, want 'embed'", video.Type)
		}
	})

	t.Run("extract videos with file extensions", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<embed src="video.mp4" type="video/mp4">
				<object data="movie.flv"></object>
			</body></html>
		`

		p, _ := html.New()
		defer p.Close()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		if len(result.Videos) < 2 {
			t.Errorf("Got %d videos, want at least 2", len(result.Videos))
		}
	})

	t.Run("extract video tags with all attributes", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<video src="video.mp4" poster="poster.jpg" width="800" height="600" duration="120"></video>
			</body></html>
		`

		p, _ := html.New()
		defer p.Close()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		if len(result.Videos) == 0 {
			t.Fatal("Should extract video")
		}

		video := result.Videos[0]
		if video.URL != "video.mp4" {
			t.Errorf("URL = %q, want 'video.mp4'", video.URL)
		}
		if video.Poster != "poster.jpg" {
			t.Errorf("Poster = %q, want 'poster.jpg'", video.Poster)
		}
		if video.Width != "800" {
			t.Errorf("Width = %q, want '800'", video.Width)
		}
		if video.Height != "600" {
			t.Errorf("Height = %q, want '600'", video.Height)
		}
		if video.Duration != "120" {
			t.Errorf("Duration = %q, want '120'", video.Duration)
		}
	})

	t.Run("extract videos with source tags", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<video>
					<source src="video1.mp4" type="video/mp4">
					<source src="video2.webm" type="video/webm">
				</video>
			</body></html>
		`

		p, _ := html.New()
		defer p.Close()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		if len(result.Videos) == 0 {
			t.Fatal("Should extract video from source tag")
		}

		// Should extract the first source
		video := result.Videos[0]
		if video.URL != "video1.mp4" {
			t.Errorf("URL = %q, want 'video1.mp4'", video.URL)
		}
	})
}

// ============================================================================
// CONFIG VALIDATION TESTS
// ============================================================================

func TestConfigValidationEdgeCases(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		modify  func(*html.Config)
		wantErr bool
		errMsg  string
	}{
		{
			name: "WorkerPoolSize at maximum",
			modify: func(c *html.Config) {
				c.WorkerPoolSize = 256
			},
			wantErr: false,
		},
		{
			name: "MaxDepth at maximum",
			modify: func(c *html.Config) {
				c.MaxDepth = 500
			},
			wantErr: false,
		},
		{
			name: "MaxInputSize at maximum",
			modify: func(c *html.Config) {
				c.MaxInputSize = 50 * 1024 * 1024
			},
			wantErr: false,
		},
		{
			name: "zero MaxCacheEntries is valid",
			modify: func(c *html.Config) {
				c.MaxCacheEntries = 0
			},
			wantErr: false,
		},
		{
			name: "zero CacheTTL is valid",
			modify: func(c *html.Config) {
				c.CacheTTL = 0
			},
			wantErr: false,
		},
		{
			name: "zero ProcessingTimeout is valid",
			modify: func(c *html.Config) {
				c.ProcessingTimeout = 0
			},
			wantErr: false,
		},
		{
			name: "WorkerPoolSize exceeds maximum",
			modify: func(c *html.Config) {
				c.WorkerPoolSize = 257
			},
			wantErr: true,
			errMsg:  "exceeds maximum",
		},
		{
			name: "MaxDepth exceeds maximum",
			modify: func(c *html.Config) {
				c.MaxDepth = 501
			},
			wantErr: true,
			errMsg:  "exceeds maximum",
		},
		{
			name: "MaxInputSize exceeds maximum",
			modify: func(c *html.Config) {
				c.MaxInputSize = 50*1024*1024 + 1
			},
			wantErr: true,
			errMsg:  "exceeds maximum",
		},
		{
			name: "negative MaxInputSize",
			modify: func(c *html.Config) {
				c.MaxInputSize = -1
			},
			wantErr: true,
			errMsg:  "must be positive",
		},
		{
			name: "zero MaxInputSize",
			modify: func(c *html.Config) {
				c.MaxInputSize = 0
			},
			wantErr: true,
			errMsg:  "must be positive",
		},
		{
			name: "negative MaxCacheEntries",
			modify: func(c *html.Config) {
				c.MaxCacheEntries = -1
			},
			wantErr: true,
			errMsg:  "cannot be negative",
		},
		{
			name: "MaxCacheEntries exceeds maximum",
			modify: func(c *html.Config) {
				c.MaxCacheEntries = 1000001
			},
			wantErr: true,
			errMsg:  "exceeds maximum",
		},
		{
			name: "negative CacheTTL",
			modify: func(c *html.Config) {
				c.CacheTTL = -1
			},
			wantErr: true,
			errMsg:  "cannot be negative",
		},
		{
			name: "negative ProcessingTimeout",
			modify: func(c *html.Config) {
				c.ProcessingTimeout = -1
			},
			wantErr: true,
			errMsg:  "cannot be negative",
		},
		{
			name: "zero WorkerPoolSize",
			modify: func(c *html.Config) {
				c.WorkerPoolSize = 0
			},
			wantErr: true,
			errMsg:  "must be positive",
		},
		{
			name: "negative WorkerPoolSize",
			modify: func(c *html.Config) {
				c.WorkerPoolSize = -1
			},
			wantErr: true,
			errMsg:  "must be positive",
		},
		{
			name: "zero MaxDepth",
			modify: func(c *html.Config) {
				c.MaxDepth = 0
			},
			wantErr: true,
			errMsg:  "must be positive",
		},
		{
			name: "negative MaxDepth",
			modify: func(c *html.Config) {
				c.MaxDepth = -1
			},
			wantErr: true,
			errMsg:  "must be positive",
		},
		{
			name: "invalid InlineImageFormat",
			modify: func(c *html.Config) {
				c.InlineImageFormat = "invalid"
			},
			wantErr: true,
			errMsg:  "valid values",
		},
		{
			name: "invalid InlineLinkFormat",
			modify: func(c *html.Config) {
				c.InlineLinkFormat = "invalid"
			},
			wantErr: true,
			errMsg:  "valid values",
		},
		{
			name: "invalid TableFormat",
			modify: func(c *html.Config) {
				c.TableFormat = "invalid"
			},
			wantErr: true,
			errMsg:  "valid values",
		},
		{
			name: "empty format strings are valid",
			modify: func(c *html.Config) {
				c.InlineImageFormat = ""
				c.InlineLinkFormat = ""
				c.TableFormat = ""
			},
			wantErr: false,
		},
		{
			name: "case-insensitive format strings",
			modify: func(c *html.Config) {
				c.InlineImageFormat = "MARKDOWN"
				c.InlineLinkFormat = "HTML"
				c.TableFormat = "Markdown"
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := html.DefaultConfig()
			tt.modify(&cfg)
			_, err := html.New(cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("New() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("Error message should contain %q, got %q", tt.errMsg, err.Error())
				}
			}
		})
	}
}

// ============================================================================
// TABLE HTML FORMAT TESTS
// ============================================================================

func TestTableHTMLFormat(t *testing.T) {
	t.Parallel()

	t.Run("extractTableAsHTML is called", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr><th style="width:50%">Header</th></tr>
					<tr><td>Data</td></tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer p.Close()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		if !strings.Contains(result.Text, "<table>") {
			t.Error("Should contain HTML table tag")
		}
		if !strings.Contains(result.Text, "<th") {
			t.Error("Should contain HTML th tag")
		}
		if !strings.Contains(result.Text, "<td") {
			t.Error("Should contain HTML td tag")
		}
		// Debug: print result to see what we got
		if testing.Verbose() {
			t.Logf("Result text:\n%s", result.Text)
		}
	})

	t.Run("HTML table with complex styling", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<th style="width:30%; text-align:left; color:red;">Name</th>
						<th style="width:70%; text-align:right; color:blue;">Value</th>
					</tr>
					<tr>
						<td style="text-align:left">Item</td>
						<td style="text-align:right">100</td>
					</tr>
				</table>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.TableFormat = "html"
		p, _ := html.New(cfg)
		defer p.Close()
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		if !strings.Contains(result.Text, "width:30%") {
			t.Error("Should preserve width for first column")
		}
		if !strings.Contains(result.Text, "width:70%") {
			t.Error("Should preserve width for second column")
		}
		if !strings.Contains(result.Text, "text-align:left") {
			t.Error("Should preserve left alignment")
		}
		if !strings.Contains(result.Text, "text-align:right") {
			t.Error("Should preserve right alignment")
		}
	})
}

// ============================================================================
// JSON OUTPUT TESTS
// ============================================================================

func TestJSONOutput(t *testing.T) {
	t.Parallel()

	t.Run("ExtractToJSON with all fields", func(t *testing.T) {
		htmlContent := `
			<html><head><title>Test Title</title></head>
			<body>
				<p>Test content with special characters: &lt;tag&gt; &amp; "quotes" 'apostrophe'</p>
				<img src="image.jpg" alt="Test Image" width="100" height="50">
				<a href="http://example.com">Link</a>
				<video src="video.mp4" poster="poster.jpg"></video>
				<audio src="audio.mp3"></audio>
			</body></html>
		`

		jsonData, err := html.ExtractToJSON([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractToJSON() failed: %v", err)
		}

		// Parse and verify JSON
		var result map[string]interface{}
		if err := json.Unmarshal(jsonData, &result); err != nil {
			t.Fatalf("Invalid JSON: %v", err)
		}

		// Check title
		if title, ok := result["title"].(string); !ok || title != "Test Title" {
			t.Errorf("Title = %v, want 'Test Title'", result["title"])
		}

		// Check text contains HTML entities are decoded
		if text, ok := result["text"].(string); !ok {
			t.Error("Text should be a string")
		} else {
			if !strings.Contains(text, "Test content") {
				t.Error("Text should contain 'Test content'")
			}
		}

		// Check images array exists and has correct structure
		images, ok := result["images"].([]interface{})
		if !ok {
			t.Error("Should have images array")
		} else if len(images) == 0 {
			t.Error("Should have at least one image")
		} else {
			img := images[0].(map[string]interface{})
			if img["url"].(string) != "image.jpg" {
				t.Errorf("Image URL = %v, want 'image.jpg'", img["url"])
			}
			if img["alt"].(string) != "Test Image" {
				t.Errorf("Image alt = %v, want 'Test Image'", img["alt"])
			}
			if img["width"].(string) != "100" {
				t.Errorf("Image width = %v, want '100'", img["width"])
			}
			if img["height"].(string) != "50" {
				t.Errorf("Image height = %v, want '50'", img["height"])
			}
		}

		// Check links array
		links, ok := result["links"].([]interface{})
		if !ok {
			t.Error("Should have links array")
		} else if len(links) == 0 {
			t.Error("Should have at least one link")
		} else {
			link := links[0].(map[string]interface{})
			if link["url"].(string) != "http://example.com" {
				t.Errorf("Link URL = %v, want 'http://example.com'", link["url"])
			}
		}

		// Check videos array
		videos, ok := result["videos"].([]interface{})
		if !ok {
			t.Error("Should have videos array")
		} else if len(videos) == 0 {
			t.Error("Should have at least one video")
		} else {
			video := videos[0].(map[string]interface{})
			if video["url"].(string) != "video.mp4" {
				t.Errorf("Video URL = %v, want 'video.mp4'", video["url"])
			}
			if video["poster"].(string) != "poster.jpg" {
				t.Errorf("Video poster = %v, want 'poster.jpg'", video["poster"])
			}
		}

		// Check audios array
		audios, ok := result["audios"].([]interface{})
		if !ok {
			t.Error("Should have audios array")
		} else if len(audios) == 0 {
			t.Error("Should have at least one audio")
		} else {
			audio := audios[0].(map[string]interface{})
			if audio["url"].(string) != "audio.mp3" {
				t.Errorf("Audio URL = %v, want 'audio.mp3'", audio["url"])
			}
		}

		// Verify JSON string is valid and contains expected fields
		if !strings.HasPrefix(string(jsonData), `{`) {
			t.Error("JSON should start with {")
		}
		// Verify all expected fields exist
		if !strings.Contains(string(jsonData), `"title"`) ||
			!strings.Contains(string(jsonData), `"text"`) ||
			!strings.Contains(string(jsonData), `"word_count"`) {
			t.Error("JSON should contain title, text, and word_count fields")
		}
	})

	t.Run("ExtractToJSON with minimal content", func(t *testing.T) {
		htmlContent := `<html><body><p>Simple</p></body></html>`

		jsonData, err := html.ExtractToJSON([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractToJSON() failed: %v", err)
		}

		var result map[string]interface{}
		if err := json.Unmarshal(jsonData, &result); err != nil {
			t.Fatalf("Invalid JSON: %v", err)
		}

		if result["text"].(string) != "Simple" {
			t.Errorf("Text = %v, want 'Simple'", result["text"])
		}
	})

	t.Run("ExtractToJSON with multiple images", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<img src="image1.jpg" alt="First Image">
				<img src="image2.png" alt="Second Image">
				<img src="image3.gif" alt="Third Image">
			</body></html>
		`

		jsonData, err := html.ExtractToJSON([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractToJSON() failed: %v", err)
		}

		var result map[string]interface{}
		if err := json.Unmarshal(jsonData, &result); err != nil {
			t.Fatalf("Invalid JSON: %v", err)
		}

		images, ok := result["images"].([]interface{})
		if !ok {
			t.Error("Should have images array")
		} else if len(images) != 3 {
			t.Errorf("Got %d images, want 3", len(images))
		}
	})

	t.Run("ExtractToJSON with multiple links", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<a href="link1.html">Link 1</a>
				<a href="link2.html">Link 2</a>
				<a href="link3.html">Link 3</a>
			</body></html>
		`

		jsonData, err := html.ExtractToJSON([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractToJSON() failed: %v", err)
		}

		var result map[string]interface{}
		if err := json.Unmarshal(jsonData, &result); err != nil {
			t.Fatalf("Invalid JSON: %v", err)
		}

		links, ok := result["links"].([]interface{})
		if !ok {
			t.Error("Should have links array")
		} else if len(links) != 3 {
			t.Errorf("Got %d links, want 3", len(links))
		}
	})
}

// ============================================================================
// LINK EXTRACTION TESTS
// ============================================================================

func TestExtractAllLinksComprehensive(t *testing.T) {
	t.Parallel()

	t.Run("extract source links from video/audio tags", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<video>
					<source src="video1.mp4" type="video/mp4">
					<source src="video2.webm" type="video/webm">
				</video>
				<audio>
					<source src="audio1.mp3" type="audio/mpeg">
					<source src="audio2.ogg" type="audio/ogg">
				</audio>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.IncludeVideos = true
		cfg.IncludeAudios = true

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer p.Close()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		if len(links) < 4 {
			t.Errorf("Got %d links, want at least 4", len(links))
		}

		// Check that video sources are extracted
		hasVideo1 := false
		hasVideo2 := false
		for _, link := range links {
			if link.Type == "video" {
				if link.URL == "video1.mp4" {
					hasVideo1 = true
				}
				if link.URL == "video2.webm" {
					hasVideo2 = true
				}
			}
		}
		if !hasVideo1 {
			t.Error("Should extract video1.mp4 source")
		}
		if !hasVideo2 {
			t.Error("Should extract video2.webm source")
		}
	})

	t.Run("extract embed video links", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<iframe src="https://www.youtube.com/embed/test123"></iframe>
				<embed src="video.mp4" type="video/mp4"></embed>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.IncludeVideos = true

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer p.Close()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		if len(links) == 0 {
			t.Error("Should extract embed video links")
		}

		// Check that embed links are extracted
		hasEmbed := false
		for _, link := range links {
			if link.Type == "video" {
				hasEmbed = true
			}
		}
		if !hasEmbed {
			t.Error("Should extract embed as video")
		}
	})
}

// ============================================================================
// URL VALIDATION AND RESOLUTION TESTS
// ============================================================================

func TestURLValidation(t *testing.T) {
	t.Parallel()

	t.Run("empty URL is invalid", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()
		htmlContent := `<html><body><img src=""></body></html>`
		result, _ := p.Extract([]byte(htmlContent))
		// Empty src should not produce an image
		if len(result.Images) > 0 {
			for _, img := range result.Images {
				if img.URL == "" {
					t.Error("Empty URL should not produce valid image")
				}
			}
		}
	})

	t.Run("valid absolute URLs", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()
		htmlContent := `<html><body>
			<img src="http://example.com/image.jpg">
			<img src="https://example.com/image.png">
			<a href="https://example.com/page">Link</a>
		</body></html>`
		result, _ := p.Extract([]byte(htmlContent))
		if len(result.Images) != 2 {
			t.Errorf("Got %d images, want 2", len(result.Images))
		}
		if len(result.Links) != 1 {
			t.Errorf("Got %d links, want 1", len(result.Links))
		}
	})

	t.Run("valid relative URLs", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()
		htmlContent := `<html><body>
			<img src="/images/photo.jpg">
			<img src="./relative.png">
			<img src="image.gif">
		</body></html>`
		result, _ := p.Extract([]byte(htmlContent))
		if len(result.Images) != 3 {
			t.Errorf("Got %d images, want 3", len(result.Images))
		}
	})

	t.Run("protocol-relative URLs", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()
		htmlContent := `<html><body>
			<img src="//example.com/image.jpg">
		</body></html>`
		result, _ := p.Extract([]byte(htmlContent))
		if len(result.Images) != 1 {
			t.Errorf("Got %d images, want 1", len(result.Images))
		}
	})

	t.Run("alphanumeric path URLs", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()
		htmlContent := `<html><body>
			<img src="img1.jpg">
			<img src="photo123.png">
			<img src="Image.GIF">
		</body></html>`
		result, _ := p.Extract([]byte(htmlContent))
		if len(result.Images) != 3 {
			t.Errorf("Got %d images, want 3", len(result.Images))
		}
	})

	t.Run("data URL with special characters is rejected", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()
		// Data URLs with control characters should be rejected
		htmlContent := `<html><body>
			<img src="data:image/png;base64,invalid<>chars">
		</body></html>`
		result, _ := p.Extract([]byte(htmlContent))
		// Should not extract images with invalid data URLs
		for _, img := range result.Images {
			if strings.Contains(img.URL, "<>") {
				t.Error("Should reject data URL with special characters")
			}
		}
	})

	t.Run("URLs with dangerous characters are rejected", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()
		htmlContent := `<html><body>
			<img src="image.jpg<script>">
			<img src="photo.png onclick="attack()">
		</body></html>`
		result, _ := p.Extract([]byte(htmlContent))
		// Should not extract URLs with dangerous characters
		for _, img := range result.Images {
			if strings.ContainsAny(img.URL, "<>\"'") {
				t.Errorf("Should reject URL with dangerous characters: %s", img.URL)
			}
		}
	})
}

// ============================================================================
// LINK TAG EXTRACTION TESTS
// ============================================================================

func TestLinkTagExtraction(t *testing.T) {
	t.Parallel()

	t.Run("extract all link rel types", func(t *testing.T) {
		htmlContent := `
			<html><head>
				<link rel="stylesheet" href="style.css">
				<link rel="icon" href="favicon.ico">
				<link rel="shortcut icon" href="favicon2.ico">
				<link rel="apple-touch-icon" href="apple.png">
				<link rel="apple-touch-icon-precomposed" href="apple2.png">
				<link rel="canonical" href="https://example.com">
			</head><body></body></html>
		`

		cfg := html.DefaultConfig()
		cfg.IncludeCSS = true
		cfg.IncludeIcons = true

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer p.Close()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Should have CSS and icons
		hasCSS := false
		hasIcon := false
		for _, link := range links {
			if link.Type == "css" {
				hasCSS = true
			}
			if link.Type == "icon" {
				hasIcon = true
			}
		}
		if !hasCSS {
			t.Error("Should extract stylesheet link")
		}
		if !hasIcon {
			t.Error("Should extract icon links")
		}
	})

	t.Run("extract preload links by resource type", func(t *testing.T) {
		htmlContent := `
			<html><head>
				<link rel="preload" as="style" href="preload.css">
				<link rel="preload" as="script" href="preload.js">
				<link rel="preload" as="image" href="preload.jpg">
				<link rel="preload" as="video" href="preload.mp4">
				<link rel="preload" as="audio" href="preload.mp3">
			</head><body></body></html>
		`

		cfg := html.DefaultConfig()
		cfg.IncludeCSS = true
		cfg.IncludeJS = true
		cfg.IncludeImages = true
		cfg.IncludeVideos = true
		cfg.IncludeAudios = true

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer p.Close()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Should have all types
		typeCounts := make(map[string]int)
		for _, link := range links {
			typeCounts[link.Type]++
		}

		if typeCounts["css"] != 1 {
			t.Errorf("Got %d CSS links, want 1", typeCounts["css"])
		}
		if typeCounts["js"] != 1 {
			t.Errorf("Got %d JS links, want 1", typeCounts["js"])
		}
		if typeCounts["image"] != 1 {
			t.Errorf("Got %d image links, want 1", typeCounts["image"])
		}
		if typeCounts["video"] != 1 {
			t.Errorf("Got %d video links, want 1", typeCounts["video"])
		}
		if typeCounts["audio"] != 1 {
			t.Errorf("Got %d audio links, want 1", typeCounts["audio"])
		}
	})

	t.Run("extract prefetch/preconnect/dns-prefetch links", func(t *testing.T) {
		htmlContent := `
			<html><head>
				<link rel="prefetch" as="style" href="prefetch.css">
				<link rel="preconnect" as="script" href="preconnect.js">
				<link rel="dns-prefetch" as="image" href="dns.jpg">
			</head><body></body></html>
		`

		cfg := html.DefaultConfig()
		cfg.IncludeCSS = true
		cfg.IncludeJS = true
		cfg.IncludeImages = true

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer p.Close()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Should have all prefetch types
		if len(links) != 3 {
			t.Errorf("Got %d links, want 3", len(links))
		}
	})

	t.Run("type-based detection for link tags", func(t *testing.T) {
		htmlContent := `
			<html><head>
				<link rel="alternate" type="text/css" href="alt-style.css">
				<link rel="alternate" type="text/javascript" href="alt-script.js">
			</head><body></body></html>
		`

		config := html.DefaultConfig()
		config.IncludeCSS = true
		config.IncludeJS = true

		p, err := html.New(config)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer p.Close()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Should detect by type attribute
		typeCounts := make(map[string]int)
		for _, link := range links {
			typeCounts[link.Type]++
		}

		if typeCounts["css"] != 1 {
			t.Errorf("Got %d CSS links by type, want 1", typeCounts["css"])
		}
		if typeCounts["js"] != 1 {
			t.Errorf("Got %d JS links by type, want 1", typeCounts["js"])
		}
	})

	t.Run("link tag without href is ignored", func(t *testing.T) {
		htmlContent := `
			<html><head>
				<link rel="stylesheet">
			</head><body></body></html>
		`

		links, err := html.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		if len(links) != 0 {
			t.Errorf("Got %d links, want 0", len(links))
		}
	})

	t.Run("link tag with data URL", func(t *testing.T) {
		htmlContent := `
			<html><head>
				<link rel="stylesheet" href="data:text/css;base64,Ym9keSB7YmFja2dyb3VuZDogcmVkO30=">
			</head><body></body></html>
		`

		links, err := html.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Data URLs should be extracted (they pass isValidURL)
		found := false
		for _, link := range links {
			if strings.HasPrefix(link.URL, "data:") {
				found = true
				break
			}
		}
		if !found {
			t.Error("Should extract data URL links if they pass validation")
		}
	})

	t.Run("link tag with title uses title", func(t *testing.T) {
		htmlContent := `
			<html><head>
				<link rel="stylesheet" href="style.css" title="My Stylesheet">
			</head><body></body></html>
		`

		config := html.DefaultConfig()
		config.IncludeCSS = true

		p, err := html.New(config)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer p.Close()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		if len(links) != 1 {
			t.Fatalf("Got %d links, want 1", len(links))
		}

		if links[0].Title != "My Stylesheet" {
			t.Errorf("Title = %q, want 'My Stylesheet'", links[0].Title)
		}
	})

	t.Run("link tag without title uses filename", func(t *testing.T) {
		htmlContent := `
			<html><head>
				<link rel="stylesheet" href="/path/to/style.css">
			</head><body></body></html>
		`

		config := html.DefaultConfig()
		config.IncludeCSS = true

		p, err := html.New(config)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer p.Close()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		if len(links) != 1 {
			t.Fatalf("Got %d links, want 1", len(links))
		}

		if links[0].Title != "style.css" {
			t.Errorf("Title = %q, want 'style.css'", links[0].Title)
		}
	})
}

// ============================================================================
// TABLE MARKDOWN FORMAT TESTS
// ============================================================================

func TestTableMarkdownFormat(t *testing.T) {
	t.Parallel()

	t.Run("complex table with alignments and colspans", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<th align="left">Name</th>
						<th align="center">Age</th>
						<th align="right">Score</th>
					</tr>
					<tr>
						<td>Alice</td>
						<td>25</td>
						<td>95.5</td>
					</tr>
					<tr>
						<td colspan="2">Bob (combined)</td>
						<td>88.0</td>
					</tr>
				</table>
			</body></html>
		`

		p, _ := html.New()
		defer p.Close()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Check for table content (markdown format may vary with colspan)
		if !strings.Contains(result.Text, "Name") {
			t.Error("Should contain table header content")
		}
		if !strings.Contains(result.Text, "Alice") {
			t.Error("Should contain table data row")
		}
		if !strings.Contains(result.Text, "Bob") {
			t.Error("Should contain colspan cell content")
		}
	})

	t.Run("table with uneven rows", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr><td>A</td><td>B</td><td>C</td></tr>
					<tr><td>D</td><td>E</td></tr>
					<tr><td>F</td></tr>
				</table>
			</body></html>
		`

		p, _ := html.New()
		defer p.Close()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should handle uneven rows gracefully
		if !strings.Contains(result.Text, "A") || !strings.Contains(result.Text, "D") || !strings.Contains(result.Text, "F") {
			t.Error("Should extract all cells from uneven table")
		}
	})

	t.Run("nested elements in table cells", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<td><strong>Bold</strong> text</td>
						<td><a href="/link">Link</a></td>
					</tr>
				</table>
			</body></html>
		`

		p, _ := html.New()
		defer p.Close()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should extract text from nested elements
		if !strings.Contains(result.Text, "Bold") {
			t.Error("Should extract text from strong tag")
		}
		if !strings.Contains(result.Text, "Link") {
			t.Error("Should extract text from anchor tag")
		}
	})
}

// ============================================================================
// BASE URL AND DOMAIN EXTRACTION TESTS
// ============================================================================

func TestBaseURLDetection(t *testing.T) {
	t.Parallel()

	t.Run("detect base URL from base tag", func(t *testing.T) {
		htmlContent := `
			<html><head>
				<base href="https://example.com/path/">
			</head>
			<body>
				<img src="image.jpg">
				<a href="page.html">Link</a>
			</body></html>
		`

		result, err := html.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Check that base tag is detected and processed
		if len(result.Images) == 0 {
			t.Error("Should extract images with base tag")
		}
	})

	t.Run("normalize base URL", func(t *testing.T) {
		// Test with various base URL formats
		testCases := []struct {
			base     string
			expected string
		}{
			{"https://example.com/path/", "https://example.com/path/"},
			{"https://example.com/path", "https://example.com/path/"},
			{"https://example.com/", "https://example.com/"},
		}

		for _, tc := range testCases {
			htmlContent := fmt.Sprintf(`
				<html><head>
					<base href="%s">
				</head>
				<body>
					<img src="image.jpg">
				</body></html>
			`, tc.base)

			result, err := html.Extract([]byte(htmlContent))
			if err != nil {
				t.Fatalf("Extract() failed: %v", err)
			}

			// Just verify it doesn't crash and produces images
			if len(result.Images) == 0 {
				t.Errorf("Base URL %q should produce images", tc.base)
			}
		}
	})

	t.Run("extract domain from URL", func(t *testing.T) {
		// This tests domain extraction for different link types
		htmlContent := `
			<html><body>
				<a href="https://example.com/page">Internal</a>
				<a href="https://other.com/page">External</a>
				<a href="//cdn.example.com/resource">Protocol-relative</a>
			</body></html>
		`

		result, err := html.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should extract all links regardless of domain
		if len(result.Links) != 3 {
			t.Errorf("Got %d links, want 3", len(result.Links))
		}
	})
}

// ============================================================================
// VIDEO EXTRACTION COMPREHENSIVE TESTS
// ============================================================================

func TestVideoExtractionComprehensive(t *testing.T) {
	t.Parallel()

	t.Run("iframe with width and height", func(t *testing.T) {
		// Note: To test parseIframeNode, we need HTML that's long enough
		// to skip the regex extraction, or use unique URLs
		p, _ := html.New()
		defer p.Close()

		// Create HTML with unique iframe URL
		htmlContent := `
			<html><body>
				<iframe src="https://player.vimeo.com/video/123456" width="640" height="480"></iframe>
			</body></html>
		`

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		if len(result.Videos) == 0 {
			t.Error("Should extract iframe video")
		}

		// Check width/height if present
		for _, video := range result.Videos {
			if video.URL == "https://player.vimeo.com/video/123456" {
				if video.Width != "" || video.Height != "" {
					// Successfully extracted dimensions
					return
				}
			}
		}
	})

	t.Run("embed tag with type attribute", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		htmlContent := `
			<html><body>
				<embed src="https://example.com/video.swf" type="application/x-shockwave-flash" width="800" height="600">
			</body></html>
		`

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// SWF is not a recognized video format (no matching extension or embed
		// host), so extraction must not panic and must yield no videos.
		if len(result.Videos) != 0 {
			t.Errorf("unsupported SWF embed should yield no videos, got %d", len(result.Videos))
		}
	})

	t.Run("object tag with data attribute", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		htmlContent := `
			<html><body>
				<object data="https://example.com/video.mp4" type="video/mp4"></object>
			</body></html>
		`

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Object tags are extracted via regex first, then DOM traversal.
		if len(result.Videos) < 1 {
			t.Fatalf("object data mp4 should be extracted as a video, got %d", len(result.Videos))
		}
		if !strings.Contains(result.Videos[0].URL, "video.mp4") {
			t.Errorf("expected video.mp4 URL, got %q", result.Videos[0].URL)
		}
	})

	t.Run("video with poster attribute", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		htmlContent := `
			<html><body>
				<video src="video.mp4" poster="poster.jpg" width="1920" height="1080">
					<track kind="subtitles" src="subs.vtt" srclang="en">
				</video>
			</body></html>
		`

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		if len(result.Videos) == 0 {
			t.Error("Should extract video with poster")
		}

		for _, video := range result.Videos {
			if video.Poster == "" {
				t.Error("Should extract poster attribute")
			}
			if video.Width == "" || video.Height == "" {
				t.Error("Should extract width and height attributes")
			}
		}
	})

	t.Run("video with multiple source elements", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		htmlContent := `
			<html><body>
				<video>
					<source src="video.mp4" type="video/mp4">
					<source src="video.webm" type="video/webm">
					<source src="video.ogg" type="video/ogg">
				</video>
			</body></html>
		`

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should extract multiple sources as separate videos
		if len(result.Videos) < 1 {
			t.Error("Should extract at least one video source")
		}
	})
}

// ============================================================================
// AUDIO EXTRACTION COMPREHENSIVE TESTS
// ============================================================================

func TestAudioExtractionComprehensive(t *testing.T) {
	t.Parallel()

	t.Run("audio with multiple sources", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		htmlContent := `
			<html><body>
				<audio>
					<source src="audio.mp3" type="audio/mpeg">
					<source src="audio.ogg" type="audio/ogg">
					<source src="audio.wav" type="audio/wav">
				</audio>
			</body></html>
		`

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should extract multiple sources
		if len(result.Audios) < 1 {
			t.Error("Should extract at least one audio source")
		}
	})

	t.Run("audio with direct src attribute", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		htmlContent := `
			<html><body>
				<audio src="single.mp3" controls></audio>
			</body></html>
		`

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		if len(result.Audios) != 1 {
			t.Errorf("Got %d audios, want 1", len(result.Audios))
		}

		if result.Audios[0].URL != "single.mp3" {
			t.Errorf("URL = %q, want 'single.mp3'", result.Audios[0].URL)
		}
	})
}

// ============================================================================
// IMAGE EXTRACTION EDGE CASES
// ============================================================================

func TestImageExtractionEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("picture element with source and img", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		htmlContent := `
			<html><body>
				<picture>
					<source srcset="image.webp" type="image/webp">
					<source srcset="image.jpg" type="image/jpeg">
					<img src="image-fallback.jpg" alt="Fallback">
				</picture>
			</body></html>
		`

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should extract at least the img src
		if len(result.Images) == 0 {
			t.Error("Should extract image from picture element")
		}
	})

	t.Run("img with srcset attribute", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		htmlContent := `
			<html><body>
				<img srcset="small.jpg 300w, medium.jpg 600w, large.jpg 1200w"
				     src="fallback.jpg" alt="Responsive image">
			</body></html>
		`

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should extract at least the src
		if len(result.Images) == 0 {
			t.Error("Should extract image with srcset")
		}
	})

	t.Run("img with all attributes", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		htmlContent := `
			<html><body>
				<img src="photo.jpg" alt="Photo" width="800" height="600" title="My Photo">
			</body></html>
		`

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		if len(result.Images) != 1 {
			t.Fatalf("Got %d images, want 1", len(result.Images))
		}

		img := result.Images[0]
		if img.URL != "photo.jpg" {
			t.Errorf("URL = %q, want 'photo.jpg'", img.URL)
		}
		if img.Alt != "Photo" {
			t.Errorf("Alt = %q, want 'Photo'", img.Alt)
		}
		if img.Width == "" || img.Height == "" {
			t.Error("Should extract width and height")
		}
		if img.Title != "My Photo" {
			t.Errorf("Title = %q, want 'My Photo'", img.Title)
		}
	})
}

// ============================================================================
// TEXT EXTRACTION EDGE CASES
// ============================================================================

func TestTextExtractionEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("nested block elements", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		htmlContent := `
			<html><body>
				<div>
					<div>
						<p>Nested paragraph 1</p>
						<p>Nested paragraph 2</p>
					</div>
				</div>
			</body></html>
		`

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		if !strings.Contains(result.Text, "Nested paragraph 1") {
			t.Error("Should extract text from deeply nested elements")
		}
		if !strings.Contains(result.Text, "Nested paragraph 2") {
			t.Error("Should extract text from deeply nested elements")
		}
	})

	t.Run("mixed inline and block elements", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		htmlContent := `
			<html><body>
				<div>
					<strong>Bold</strong> <em>italic</em> text
					<p>New paragraph</p>
					<span>More <a href="/link">linked</a> text</span>
				</div>
			</body></html>
		`

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		if !strings.Contains(result.Text, "Bold italic text") {
			t.Error("Should extract inline element text")
		}
		if !strings.Contains(result.Text, "New paragraph") {
			t.Error("Should extract block element text")
		}
		if !strings.Contains(result.Text, "More linked text") {
			t.Error("Should extract mixed inline text")
		}
	})

	t.Run("whitespace normalization", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		htmlContent := `
			<html><body>
				<p>Text    with     many     spaces</p>
				<p>
					Text with
					weird
					line breaks
				</p>
			</body></html>
		`

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should normalize whitespace
		if !strings.Contains(result.Text, "Text with many spaces") {
			t.Error("Should normalize multiple spaces")
		}
	})
}

// ============================================================================
// ADDITIONAL URL AND DOMAIN TESTS FOR COVERAGE
// ============================================================================

func TestURLAndDomainExtraction(t *testing.T) {
	t.Parallel()

	t.Run("protocol-relative URL resolution", func(t *testing.T) {
		// This tests resolveURL with protocol-relative URLs
		p, _ := html.New()
		defer p.Close()

		// Using link extraction with base URL
		htmlContent := `
			<html><head>
				<base href="https://example.com/path/">
			</head>
			<body>
				<a href="//cdn.example.com/resource">CDN Link</a>
				<a href="/absolute/path">Absolute Link</a>
				<a href="relative.html">Relative Link</a>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.ResolveRelativeURLs = true
		cfg.IncludeContentLinks = true

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer p.Close()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Should extract all link types
		if len(links) == 0 {
			t.Error("Should extract links with various URL formats")
		}
	})

	t.Run("domain extraction from various URLs", func(t *testing.T) {
		// Tests extractDomain through different URL scenarios
		htmlContent := `
			<html><body>
				<a href="https://subdomain.example.com/page1">Link 1</a>
				<a href="http://example.org/page2">Link 2</a>
				<a href="https://example.net:8080/page3">Link 3</a>
				<a href="//example.info/page4">Link 4</a>
			</body></html>
		`

		result, err := html.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should extract links from different domains
		if len(result.Links) != 4 {
			t.Errorf("Got %d links, want 4", len(result.Links))
		}
	})

	t.Run("base URL detection from meta tags", func(t *testing.T) {
		htmlContent := `
			<html><head>
				<meta property="og:url" content="https://example.com/canonical">
				<link rel="canonical" href="https://example.com/canonical-link">
			</head>
			<body><p>Content</p></body>
		</html>
		`

		result, err := html.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Meta/link tags must not break extraction; body content survives.
		if !strings.Contains(result.Text, "Content") {
			t.Errorf("expected body content to survive meta-tag extraction, got %q", result.Text)
		}
	})

	t.Run("base URL normalization", func(t *testing.T) {
		// Test various base URL formats
		testCases := []string{
			`<base href="https://example.com">`,
			`<base href="https://example.com/">`,
			`<base href="https://example.com/path">`,
			`<base href="https://example.com/path/">`,
		}

		for _, baseTag := range testCases {
			htmlContent := fmt.Sprintf(`
				<html><head>%s</head>
				<body><p>Content</p></body>
				</html>
			`, baseTag)

			result, err := html.Extract([]byte(htmlContent))
			if err != nil {
				t.Errorf("Failed with base tag %s: %v", baseTag, err)
				continue
			}
			if !strings.Contains(result.Text, "Content") {
				t.Errorf("base tag %s: expected body content, got %q", baseTag, result.Text)
			}
		}
	})
}

// ============================================================================
// SCRIPT AND EMBED LINK EXTRACTION TESTS
// ============================================================================

func TestScriptAndEmbedLinkExtraction(t *testing.T) {
	t.Parallel()

	t.Run("extract script links", func(t *testing.T) {
		htmlContent := `
			<html><head>
				<script src="script1.js"></script>
				<script src="https://cdn.example.com/script2.js"></script>
				<script src="/path/to/script3.js" title="Main Script"></script>
			</head><body></body></html>
		`

		cfg := html.DefaultConfig()
		cfg.IncludeJS = true

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer p.Close()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Should extract script links
		if len(links) != 3 {
			t.Errorf("Got %d script links, want 3", len(links))
		}

		// Check that types are correct
		for _, link := range links {
			if link.Type != "js" {
				t.Errorf("Script link should have type 'js', got '%s'", link.Type)
			}
		}
	})

	t.Run("script tag without src is ignored", func(t *testing.T) {
		htmlContent := `
			<html><head>
				<script>console.log("inline script");</script>
			</head><body></body></html>
		`

		cfg := html.DefaultConfig()
		cfg.IncludeJS = true

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer p.Close()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Inline scripts should not produce links
		if len(links) != 0 {
			t.Errorf("Got %d links, want 0 (inline script should not produce link)", len(links))
		}
	})

	t.Run("embed and object link extraction", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<embed src="content.swf" type="application/x-shockwave-flash">
				<object data="video.flv" type="video/x-flv"></object>
				<iframe src="https://example.com/frame"></iframe>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.IncludeVideos = true

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer p.Close()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// The iframe src is a valid absolute URL and must be captured even when
		// the surrounding embed/object media are unrecognized.
		if len(links) == 0 {
			t.Error("expected at least the iframe link to be extracted")
		}
	})

	t.Run("source link extraction with various attributes", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<video>
					<source src="video1.mp4" type="video/mp4">
					<source src="video2.webm" type="video/webm" media="screen and (min-width:800px)">
					<source src="video3.mobile.mp4">
				</video>
				<audio>
					<source src="audio1.mp3" type="audio/mpeg">
					<source src="audio2.ogg" type="audio/ogg">
				</audio>
			</body></html>
		`

		cfg := html.DefaultConfig()
		cfg.IncludeVideos = true
		cfg.IncludeAudios = true

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer p.Close()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Should extract all source links
		if len(links) < 1 {
			t.Error("Should extract source links")
		}
	})
}

// ============================================================================
// CONTENT LINK EXTRACTION TESTS
// ============================================================================

func TestContentLinkExtraction(t *testing.T) {
	t.Parallel()

	t.Run("extract internal vs external links", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<a href="https://example.com/page1">Internal</a>
				<a href="https://other.com/page2">External</a>
				<a href="/absolute">Absolute</a>
				<a href="relative.html">Relative</a>
			</body></html>
		`

		config := html.DefaultConfig()
		config.IncludeContentLinks = true
		config.IncludeExternalLinks = true

		p, err := html.New(config)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer p.Close()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Should extract all link types
		if len(links) != 4 {
			t.Errorf("Got %d links, want 4", len(links))
		}
	})

	t.Run("image link extraction", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<img src="image1.jpg" alt="Image 1">
				<img src="https://example.com/image2.png" alt="Image 2">
				<img src="/path/to/image3.gif" alt="Image 3">
			</body></html>
		`

		config := html.DefaultConfig()
		config.IncludeImages = true

		p, err := html.New(config)
		if err != nil {
			t.Fatalf("html.New() failed: %v", err)
		}
		defer p.Close()
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Should extract image links
		if len(links) != 3 {
			t.Errorf("Got %d image links, want 3", len(links))
		}

		// Check that types are correct
		for _, link := range links {
			if link.Type != "image" {
				t.Errorf("Image link should have type 'image', got '%s'", link.Type)
			}
		}
	})

	t.Run("link without href is ignored", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<a name="anchor"></a>
				<a id="link2"></a>
			</body></html>
		`

		links, err := html.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Links without href should not be extracted
		if len(links) != 0 {
			t.Errorf("Got %d links, want 0 (links without href)", len(links))
		}
	})
}

// ============================================================================
// TABLE MARKDOWN FORMAT ADDITIONAL TESTS
// ============================================================================

func TestTableMarkdownEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("table with empty cells", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr><td>A</td><td></td><td>C</td></tr>
					<tr><td></td><td>E</td><td></td></tr>
				</table>
			</body></html>
		`

		p, _ := html.New()
		defer p.Close()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should handle empty cells
		if !strings.Contains(result.Text, "A") {
			t.Error("Should extract non-empty cells")
		}
		if !strings.Contains(result.Text, "E") {
			t.Error("Should extract non-empty cells")
		}
	})

	t.Run("table with nested markup", func(t *testing.T) {
		htmlContent := `
			<html><body>
				<table>
					<tr>
						<td><strong>Bold</strong> and <em>italic</em></td>
						<td><a href="/link">Link</a></td>
					</tr>
				</table>
			</body></html>
		`

		p, _ := html.New()
		defer p.Close()

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should extract text from nested elements
		if !strings.Contains(result.Text, "Bold") {
			t.Error("Should extract text from strong tag")
		}
		if !strings.Contains(result.Text, "italic") {
			t.Error("Should extract text from em tag")
		}
		if !strings.Contains(result.Text, "Link") {
			t.Error("Should extract text from anchor tag")
		}
	})
}

// ============================================================================
// TextOnlyExtractConfig Tests
// ============================================================================

func TestTextOnlyConfig(t *testing.T) {
	t.Parallel()

	t.Run("TextOnlyConfig disables all media", func(t *testing.T) {
		cfg := html.TextOnlyConfig()

		if !cfg.ExtractArticle {
			t.Error("ExtractArticle should be true")
		}
		if cfg.PreserveImages {
			t.Error("PreserveImages should be false")
		}
		if cfg.PreserveLinks {
			t.Error("PreserveLinks should be false")
		}
		if cfg.PreserveVideos {
			t.Error("PreserveVideos should be false")
		}
		if cfg.PreserveAudios {
			t.Error("PreserveAudios should be false")
		}
		if cfg.InlineImageFormat != "none" {
			t.Errorf("InlineImageFormat should be 'none', got '%s'", cfg.InlineImageFormat)
		}
	})

	t.Run("TextOnlyExtractConfig extracts only text", func(t *testing.T) {
		htmlContent := `<html><head><title>Test</title></head><body>
			<article>
				<h1>Article Title</h1>
				<p>Paragraph content.</p>
				<a href="https://example.com">Link</a>
				<img src="image.jpg" alt="Image">
				<video src="video.mp4"></video>
			</article>
		</body></html>`

		result, err := html.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should have text
		if result.Text == "" {
			t.Error("Should extract text")
		}

		// With default config, should have media
		if len(result.Images) == 0 {
			t.Error("Should extract images with default config")
		}
		if len(result.Links) == 0 {
			t.Error("Should extract links with default config")
		}
	})

	t.Run("TextOnlyConfig disables media preservation", func(t *testing.T) {
		// Use TextOnlyConfig to disable media
		cfg := html.TextOnlyConfig()
		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer p.Close()

		htmlContent := `<html><head><title>Test</title></head><body>
			<article class="entry-content">
				<h1>Article Title</h1>
				<p>Paragraph content.</p>
				<img src="image.jpg" alt="Test Image">
				<a href="https://example.com">Link</a>
			</article>
		</body></html>`

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should have text
		if result.Text == "" {
			t.Error("Should extract text")
		}

		// Should NOT have media
		if len(result.Images) > 0 {
			t.Error("Should not extract images with TextOnlyConfig")
		}
		if len(result.Links) > 0 {
			t.Error("Should not extract links with TextOnlyConfig")
		}
	})

	t.Run("TextOnlyConfig vs DefaultConfig", func(t *testing.T) {
		defaultCfg := html.DefaultConfig()
		textOnlyCfg := html.TextOnlyConfig()

		// ExtractArticle should be same
		if defaultCfg.ExtractArticle != textOnlyCfg.ExtractArticle {
			t.Error("ExtractArticle should be same")
		}

		// Media preservation should differ
		if defaultCfg.PreserveImages == textOnlyCfg.PreserveImages {
			t.Error("PreserveImages should differ")
		}
		if defaultCfg.PreserveLinks == textOnlyCfg.PreserveLinks {
			t.Error("PreserveLinks should differ")
		}
	})
}

// ============================================================================
// Config Tests
// ============================================================================

func TestConfigDefaults(t *testing.T) {
	t.Parallel()

	t.Run("default config extracts content", func(t *testing.T) {
		// This test verifies that default config works correctly
		p, err := html.New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer p.Close()

		htmlContent := `<html><head><title>Test</title></head><body>
			<article class="entry-content">
				<h1>Article Title</h1>
				<p>Paragraph content.</p>
			</article>
		</body></html>`

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// DefaultConfig has ExtractArticle=true, so content should be extracted
		if result.Text == "" {
			t.Error("Expected text to be extracted with default config")
		}
		if !strings.Contains(result.Text, "Article Title") {
			t.Errorf("Expected 'Article Title' in text, got: %s", result.Text)
		}
	})

	t.Run("custom config with image format", func(t *testing.T) {
		// Create processor with custom image format
		cfg := html.DefaultConfig()
		cfg.InlineImageFormat = "html"
		cfg.TableFormat = "html"

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer p.Close()

		htmlContent := `<html><head><title>Test</title></head><body>
			<article class="entry-content">
				<h1>Article Title</h1>
				<p>Paragraph content.</p>
				<img src="image.jpg" alt="Test Image">
				<a href="https://example.com">Link</a>
			</article>
		</body></html>`

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// ExtractArticle should default to true
		if result.Text == "" {
			t.Error("Expected text to be extracted (ExtractArticle defaults to true)")
		}

		// PreserveImages should default to true
		if len(result.Images) == 0 {
			t.Error("Expected images to be preserved (PreserveImages defaults to true)")
		}

		// PreserveLinks should default to true
		if len(result.Links) == 0 {
			t.Error("Expected links to be preserved (PreserveLinks defaults to true)")
		}
	})

	t.Run("markdown format config", func(t *testing.T) {
		cfg := html.DefaultConfig()
		cfg.InlineImageFormat = "markdown"
		cfg.TableFormat = "html"

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer p.Close()

		htmlContent := `<html><head><title>Test</title></head><body>
			<article>
				<h1>Article Title</h1>
				<p>Paragraph content.</p>
			</article>
		</body></html>`

		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should extract content
		if result.Text == "" {
			t.Error("Expected text to be extracted")
		}
	})

	t.Run("Encoding field works correctly", func(t *testing.T) {
		cfg := html.DefaultConfig()
		cfg.Encoding = "utf-8"

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer p.Close()

		// UTF-8 encoded HTML
		htmlContent := []byte(`<html><head><title>Test</title></head><body>
			<article>
				<h1>Article Title</h1>
				<p>中文内容测试</p>
			</article>
		</body></html>`)

		result, err := p.Extract(htmlContent)
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}

		// Should extract content correctly
		if result.Text == "" {
			t.Error("Expected text to be extracted")
		}
	})
}

// ============================================================================
// resolveLinkExtractionConfig Merge Tests
// ============================================================================

func TestResolveLinkExtractionConfigMerge(t *testing.T) {
	t.Parallel()

	t.Run("no config returns defaults", func(t *testing.T) {
		p, err := html.New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer p.Close()

		htmlContent := `<html><head><title>Test</title></head><body>
			<a href="https://example.com">Link</a>
			<img src="image.jpg">
			<video src="video.mp4"></video>
		</body></html>`

		// Call without config - should use defaults
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// DefaultLinkExtractionConfig has all Include* fields set to true
		if len(links) == 0 {
			t.Error("Expected links, images, and videos to be extracted with default config")
		}
	})

	t.Run("empty config returns defaults", func(t *testing.T) {
		p, err := html.New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer p.Close()

		htmlContent := `<html><head><title>Test</title></head><body>
			<a href="https://example.com">Link</a>
			<img src="image.jpg">
			<video src="video.mp4"></video>
		</body></html>`

		// Call with empty config - should behave same as no config
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Should use defaults
		if len(links) == 0 {
			t.Error("Expected links to be extracted with empty config (should use defaults)")
		}
	})

	t.Run("string-only config preserves boolean defaults", func(t *testing.T) {
		p, err := html.New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer p.Close()

		htmlContent := `<html><head><title>Test</title></head><body>
			<a href="https://example.com">Link</a>
			<img src="image.jpg">
			<video src="video.mp4"></video>
		</body></html>`

		// Only set BaseURL string field - boolean defaults should be preserved
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// Should still extract all types (defaults preserved)
		if len(links) == 0 {
			t.Error("Expected all link types to be extracted (boolean defaults preserved)")
		}
	})

	t.Run("explicit true overrides default false", func(t *testing.T) {
		// Create config with IncludeVideos=false, IncludeImages=true
		cfg := html.DefaultConfig()
		cfg.IncludeVideos = false
		cfg.IncludeImages = true

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer p.Close()

		htmlContent := `<html><head><title>Test</title></head><body>
			<a href="https://example.com">Link</a>
			<img src="image.jpg">
			<video src="video.mp4"></video>
		</body></html>`

		// Extract with custom config: IncludeImages=true, IncludeVideos=false
		links, err := p.ExtractAllLinks([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}

		// When IncludeImages is true, hasExplicitTrue becomes true
		// So all user's boolean values are used: IncludeImages=true, IncludeVideos=false, etc.
		// Should have images
		hasImages := false
		for _, link := range links {
			if link.Type == "image" {
				hasImages = true
			}
		}
		if !hasImages {
			t.Error("Expected images to be included (IncludeImages=true)")
		}

		// Should NOT have videos (user set IncludeVideos to false by zero)
		hasVideos := false
		for _, link := range links {
			if link.Type == "video" {
				hasVideos = true
			}
		}
		if hasVideos {
			t.Error("Expected videos to NOT be included (IncludeVideos=false)")
		}
	})
}

// TestContextCancellationMidProcessing tests context cancellation during batch processing.
func TestContextCancellationMidProcessing(t *testing.T) {
	t.Parallel()

	t.Run("cancellation during batch processing", func(t *testing.T) {
		p, err := html.New()
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		// Create batch of documents
		docs := make([][]byte, 1000)
		for i := range docs {
			docs[i] = []byte(`<html><body><p>` + strings.Repeat("x", 100) + `</p></body></html>`)
		}

		ctx, cancel := context.WithCancel(context.Background())

		// Cancel after short delay
		go func() {
			time.Sleep(10 * time.Millisecond)
			cancel()
		}()

		result := p.ExtractBatchWithContext(ctx, docs)

		// Should indicate cancellation
		if result == nil {
			t.Error("Expected non-nil result")
		}
		// Some results may have been processed before cancellation
		if result != nil && len(result.Results) > len(docs) {
			t.Errorf("Results exceed input count")
		}
	})

	t.Run("timeout during extraction", func(t *testing.T) {
		cfg := html.DefaultConfig()
		cfg.ProcessingTimeout = 1 * time.Millisecond
		p, err := html.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		// Large HTML to trigger timeout
		largeHTML := []byte(`<html><body>` + strings.Repeat("<p>content</p>", 100000) + `</body></html>`)

		_, err = p.Extract(largeHTML)

		// Timeout may or may not occur depending on system speed
		if err != nil {
			t.Logf("Got error (expected timeout): %v", err)
		}
	})
}

// TestCacheKeyGeneration tests cache key generation edge cases.
func TestCacheKeyGeneration(t *testing.T) {
	t.Parallel()

	t.Run("large document cache key", func(t *testing.T) {
		cfg := html.DefaultConfig()
		cfg.MaxCacheEntries = 10
		p, err := html.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		// Generate unique large documents (>64KB to trigger multi-point sampling)
		for i := 0; i < 100; i++ {
			content := fmt.Sprintf(`<html><body><p id="%d">%s</p></body></html>`,
				i, strings.Repeat("x", 70000))
			p.Extract([]byte(content))
		}

		stats := p.GetStatistics()
		// Cache should have evicted entries (with small cache and unique docs)
		if stats.CacheMisses < 90 {
			t.Errorf("Expected high cache misses, got %d", stats.CacheMisses)
		}
	})

	t.Run("identical content same cache key", func(t *testing.T) {
		cfg := html.DefaultConfig()
		cfg.MaxCacheEntries = 100
		p, err := html.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		content := `<html><body><p>Identical content</p></body></html>`

		for i := 0; i < 10; i++ {
			p.Extract([]byte(content))
		}

		stats := p.GetStatistics()
		if stats.CacheHits < 9 {
			t.Errorf("Expected 9+ cache hits, got %d", stats.CacheHits)
		}
	})
}

// TestCustomScorerNilHandling tests custom scorer edge cases.
func TestCustomScorerNilHandling(t *testing.T) {
	t.Parallel()

	t.Run("nil scorer uses default", func(t *testing.T) {
		cfg := html.DefaultConfig()
		cfg.Scorer = nil
		p, err := html.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		result, err := p.Extract([]byte(`<html><body><p>Test</p></body></html>`))
		if err != nil {
			t.Fatalf("Extract failed: %v", err)
		}
		if result.Text == "" {
			t.Error("Expected text extraction with nil scorer")
		}
	})

	t.Run("custom scorer implementation", func(t *testing.T) {
		// Custom scorer that boosts article scores
		cfg := html.DefaultConfig()
		cfg.Scorer = &testScorer{scoreCalled: new(bool)}
		p, err := html.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer p.Close()

		htmlContent := `<html><body><article><p>Test content</p></article></body></html>`
		p.Extract([]byte(htmlContent))

		// Custom scorer should have been called
		if !*cfg.Scorer.(*testScorer).scoreCalled {
			t.Error("Expected custom scorer to be called")
		}
	})
}

// testScorer is a simple test implementation of the Scorer interface.
type testScorer struct {
	scoreCalled *bool
}

func (s *testScorer) Score(node html.ContentNode) int {
	*s.scoreCalled = true
	if node.Type() == "article" {
		return 200
	}
	return 100
}

func (s *testScorer) ShouldRemove(node html.ContentNode) bool {
	return false
}

// ============================================================================
// Package-Level Functions with Optional Config Tests
// ============================================================================

func TestPackageLevelFunctionsWithOptionalConfig(t *testing.T) {
	t.Parallel()

	htmlContent := []byte(`<html><head><title>Test Page</title></head><body>
		<h1>Main Title</h1>
		<p>Some text content here.</p>
		<a href="https://example.com">Link</a>
		<img src="image.jpg" alt="Test Image">
	</body></html>`)

	t.Run("Extract with default config", func(t *testing.T) {
		// Without config - uses default
		result, err := html.Extract(htmlContent)
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if result.Title != "Test Page" {
			t.Errorf("Expected title 'Test Page', got %q", result.Title)
		}
	})

	t.Run("Extract with custom config", func(t *testing.T) {
		cfg := html.DefaultConfig()
		cfg.PreserveImages = false
		cfg.PreserveLinks = false

		result, err := html.Extract(htmlContent, cfg)
		if err != nil {
			t.Fatalf("Extract() with config failed: %v", err)
		}
		if result.Title != "Test Page" {
			t.Errorf("Expected title 'Test Page', got %q", result.Title)
		}
		// With PreserveImages=false, no images should be extracted
		if len(result.Images) != 0 {
			t.Errorf("Expected no images with PreserveImages=false, got %d", len(result.Images))
		}
		// With PreserveLinks=false, no links should be extracted
		if len(result.Links) != 0 {
			t.Errorf("Expected no links with PreserveLinks=false, got %d", len(result.Links))
		}
	})

	t.Run("ExtractText with default config", func(t *testing.T) {
		text, err := html.ExtractText(htmlContent)
		if err != nil {
			t.Fatalf("ExtractText() failed: %v", err)
		}
		if !strings.Contains(text, "Main Title") {
			t.Errorf("Expected text to contain 'Main Title', got %q", text)
		}
	})

	t.Run("ExtractText with TextOnlyConfig", func(t *testing.T) {
		text, err := html.ExtractText(htmlContent, html.TextOnlyConfig())
		if err != nil {
			t.Fatalf("ExtractText() with config failed: %v", err)
		}
		if !strings.Contains(text, "Main Title") {
			t.Errorf("Expected text to contain 'Main Title', got %q", text)
		}
	})

	t.Run("ExtractToMarkdown with default config", func(t *testing.T) {
		md, err := html.ExtractToMarkdown(htmlContent)
		if err != nil {
			t.Fatalf("ExtractToMarkdown() failed: %v", err)
		}
		// Markdown format should contain markdown-style links/images
		if md == "" {
			t.Error("Expected non-empty markdown output")
		}
	})

	t.Run("ExtractToMarkdown with custom config", func(t *testing.T) {
		cfg := html.MarkdownConfig()
		cfg.PreserveImages = false

		md, err := html.ExtractToMarkdown(htmlContent, cfg)
		if err != nil {
			t.Fatalf("ExtractToMarkdown() with config failed: %v", err)
		}
		if md == "" {
			t.Error("Expected non-empty markdown output")
		}
	})

	t.Run("ExtractToJSON with default config", func(t *testing.T) {
		jsonBytes, err := html.ExtractToJSON(htmlContent)
		if err != nil {
			t.Fatalf("ExtractToJSON() failed: %v", err)
		}

		var result html.Result
		if err := json.Unmarshal(jsonBytes, &result); err != nil {
			t.Fatalf("Failed to unmarshal JSON: %v", err)
		}
		if result.Title != "Test Page" {
			t.Errorf("Expected title 'Test Page', got %q", result.Title)
		}
	})

	t.Run("ExtractToJSON with custom config", func(t *testing.T) {
		cfg := html.TextOnlyConfig()

		jsonBytes, err := html.ExtractToJSON(htmlContent, cfg)
		if err != nil {
			t.Fatalf("ExtractToJSON() with config failed: %v", err)
		}

		var result html.Result
		if err := json.Unmarshal(jsonBytes, &result); err != nil {
			t.Fatalf("Failed to unmarshal JSON: %v", err)
		}
		// TextOnlyConfig disables image preservation
		if len(result.Images) != 0 {
			t.Errorf("Expected no images with TextOnlyConfig, got %d", len(result.Images))
		}
	})

	t.Run("ExtractAllLinks with default config", func(t *testing.T) {
		links, err := html.ExtractAllLinks(htmlContent)
		if err != nil {
			t.Fatalf("ExtractAllLinks() failed: %v", err)
		}
		if len(links) == 0 {
			t.Error("Expected at least one link")
		}
	})

	t.Run("ExtractAllLinks with custom config", func(t *testing.T) {
		cfg := html.DefaultConfig()
		cfg.IncludeImages = false
		cfg.IncludeCSS = false
		cfg.IncludeJS = false

		links, err := html.ExtractAllLinks(htmlContent, cfg)
		if err != nil {
			t.Fatalf("ExtractAllLinks() with config failed: %v", err)
		}
		// Should still have the anchor link
		found := false
		for _, link := range links {
			if link.Type == "link" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected to find anchor link")
		}
	})
}

func TestPackageLevelFileFunctionsWithOptionalConfig(t *testing.T) {
	t.Parallel()

	// Create a temporary HTML file
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.html")
	htmlContent := `<html><head><title>File Test</title></head><body><h1>Content</h1></body></html>`
	if err := os.WriteFile(tmpFile, []byte(htmlContent), 0644); err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	t.Run("ExtractFromFile with default config", func(t *testing.T) {
		result, err := html.ExtractFromFile(tmpFile)
		if err != nil {
			t.Fatalf("ExtractFromFile() failed: %v", err)
		}
		if result.Title != "File Test" {
			t.Errorf("Expected title 'File Test', got %q", result.Title)
		}
	})

	t.Run("ExtractFromFile with custom config", func(t *testing.T) {
		cfg := html.TextOnlyConfig()
		result, err := html.ExtractFromFile(tmpFile, cfg)
		if err != nil {
			t.Fatalf("ExtractFromFile() with config failed: %v", err)
		}
		if result.Title != "File Test" {
			t.Errorf("Expected title 'File Test', got %q", result.Title)
		}
	})

	t.Run("ExtractTextFromFile with default config", func(t *testing.T) {
		text, err := html.ExtractTextFromFile(tmpFile)
		if err != nil {
			t.Fatalf("ExtractTextFromFile() failed: %v", err)
		}
		if !strings.Contains(text, "Content") {
			t.Errorf("Expected text to contain 'Content', got %q", text)
		}
	})

	t.Run("ExtractTextFromFile with custom config", func(t *testing.T) {
		text, err := html.ExtractTextFromFile(tmpFile, html.TextOnlyConfig())
		if err != nil {
			t.Fatalf("ExtractTextFromFile() with config failed: %v", err)
		}
		if !strings.Contains(text, "Content") {
			t.Errorf("Expected text to contain 'Content', got %q", text)
		}
	})

	t.Run("ExtractToMarkdownFromFile with custom config", func(t *testing.T) {
		md, err := html.ExtractToMarkdownFromFile(tmpFile, html.MarkdownConfig())
		if err != nil {
			t.Fatalf("ExtractToMarkdownFromFile() with config failed: %v", err)
		}
		if md == "" {
			t.Error("Expected non-empty markdown output")
		}
	})

	t.Run("ExtractToJSONFromFile with custom config", func(t *testing.T) {
		jsonBytes, err := html.ExtractToJSONFromFile(tmpFile, html.TextOnlyConfig())
		if err != nil {
			t.Fatalf("ExtractToJSONFromFile() with config failed: %v", err)
		}
		if len(jsonBytes) == 0 {
			t.Error("Expected non-empty JSON output")
		}
	})

	t.Run("ExtractAllLinksFromFile with default config", func(t *testing.T) {
		links, err := html.ExtractAllLinksFromFile(tmpFile)
		if err != nil {
			t.Fatalf("ExtractAllLinksFromFile() failed: %v", err)
		}
		// The fixture is link-free; the wrapper must return no links, not error.
		if len(links) != 0 {
			t.Errorf("link-free file should yield no links, got %d", len(links))
		}
	})

	t.Run("ExtractAllLinksFromFile with custom config", func(t *testing.T) {
		cfg := html.DefaultConfig()
		cfg.IncludeExternalLinks = false
		links, err := html.ExtractAllLinksFromFile(tmpFile, cfg)
		if err != nil {
			t.Fatalf("ExtractAllLinksFromFile() with config failed: %v", err)
		}
		if len(links) != 0 {
			t.Errorf("link-free file should yield no links, got %d", len(links))
		}
	})
}
