package html_test

// html_test.go - Comprehensive test suite for the html package
// This file consolidates and improves all main package tests to provide:
// - Better coverage of public APIs
// - Elimination of duplicate tests
// - Improved maintainability
// - Effective regression testing

import (
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
		defer func() { _ = p.Close() }()
		if p == nil {
			t.Fatal("New() returned nil processor")
		}
	})

	t.Run("New with no config uses defaults", func(t *testing.T) {
		p, err := html.New()
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer func() { _ = p.Close() }()
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
		defer func() { _ = p1.Close() }()

		// Test TextOnlyConfig
		p2, err := html.New(html.TextOnlyConfig())
		if err != nil {
			t.Fatalf("New(TextOnlyConfig()) failed: %v", err)
		}
		defer func() { _ = p2.Close() }()

		// Test HighSecurityConfig
		p3, err := html.New(html.HighSecurityConfig())
		if err != nil {
			t.Fatalf("New(HighSecurityConfig()) failed: %v", err)
		}
		defer func() { _ = p3.Close() }()
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

	t.Run("custom config creation", func(t *testing.T) {

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
	defer func() { _ = p.Close() }()

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
		defer func() { _ = p.Close() }()

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
		defer func() { _ = p.Close() }()

		deepHTML := "<div>" + strings.Repeat("<div>", 10) + "content" + strings.Repeat("</div>", 10) + "</div>"
		_, err := p.Extract([]byte(deepHTML))
		if err != html.ErrMaxDepthExceeded {
			t.Errorf("Expected ErrMaxDepthExceeded, got: %v", err)
		}
	})

	t.Run("input at exact MaxInputSize boundary accepted", func(t *testing.T) {
		cfg := html.DefaultConfig()
		cfg.MaxInputSize = 5000
		p, err := html.New(cfg)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = p.Close() }()

		validHTML := strings.Repeat("<div>a</div>", 100)
		if _, err := p.Extract([]byte(validHTML)); err != nil {
			t.Errorf("should accept input at MaxInputSize boundary, got: %v", err)
		}

		oversizedHTML := strings.Repeat("<div>a</div>", 1000)
		if _, err := p.Extract([]byte(oversizedHTML)); err == nil {
			t.Error("expected error for oversize input")
		}
	})

	// depthBoundary pins the limit semantics: nesting well under MaxDepth must
	// succeed — only exceeding it errors. Note the depth count includes the
	// html/body ancestors, so divCount stays two levels under the budget.
	t.Run("depth boundary conditions", func(t *testing.T) {
		tests := []struct {
			name     string
			maxDepth int
			divCount int
		}{
			{"nesting under limit", 50, 40},
			{"very deep nesting with high limit", 500, 198},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				cfg := html.DefaultConfig()
				cfg.MaxDepth = tt.maxDepth
				p, err := html.New(cfg)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = p.Close() }()

				deepHTML := "<html><body>" + strings.Repeat("<div>", tt.divCount) +
					"Content" + strings.Repeat("</div>", tt.divCount) + "</body></html>"

				result, err := p.Extract([]byte(deepHTML))
				if err != nil {
					t.Fatalf("nesting depth %d under limit %d should succeed: %v", tt.divCount, tt.maxDepth, err)
				}
				if !strings.Contains(result.Text, "Content") {
					t.Error("expected content at depth under the limit")
				}
			})
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
		defer func() { _ = p.Close() }()

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
		defer func() { _ = p.Close() }()

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
	defer func() { _ = p.Close() }()

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

	t.Run("whitespace-only path returns error", func(t *testing.T) {
		_, err := p.ExtractFromFile("   ")
		if err == nil {
			t.Fatal("ExtractFromFile() should fail with whitespace-only path")
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

}

// ============================================================================
// BATCH PROCESSING TESTS
// ============================================================================

func TestBatchProcessing(t *testing.T) {
	t.Parallel()

	p, _ := html.New()
	defer func() { _ = p.Close() }()

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
		defer func() { _ = p.Close() }()

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
	defer func() { _ = p.Close() }()

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
		htmlContent := `<html><body><p>Word1 word2 word3</p></body></html>`
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		// Exact count pins the counting semantics (whitespace-separated words).
		if result.WordCount != 3 {
			t.Errorf("WordCount = %d, want 3", result.WordCount)
		}
	})

	t.Run("reading time calculated", func(t *testing.T) {
		htmlContent := `<html><body><p>Word1 word2 word3 word4 word5</p></body></html>`
		result, err := p.Extract([]byte(htmlContent))
		if err != nil {
			t.Fatalf("Extract() failed: %v", err)
		}
		if result.ReadingTime <= 0 {
			t.Errorf("ReadingTime = %v, want > 0", result.ReadingTime)
		}
	})
}

// ============================================================================
// LINK EXTRACTION TESTS
// ============================================================================

// ============================================================================
// PARAGRAPH SPACING TESTS
// ============================================================================

func TestParagraphSpacing(t *testing.T) {
	t.Parallel()

	p, err := html.New()
	if err != nil {
		t.Fatalf("html.New() failed: %v", err)
	}
	defer func() { _ = p.Close() }()

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

// ============================================================================
// TABLE COLUMN WIDTH TESTS
// ============================================================================

// ============================================================================
// TABLE ALIGNMENT PRESERVATION TESTS
// ============================================================================

// ============================================================================
// ============================================================================
// COMPREHENSIVE LINK EXTRACTION TESTS
// ============================================================================

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
		defer func() { _ = p.Close() }()

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
		defer func() { _ = p.Close() }()

		htmlContent := `<html><body><p>Test</p></body></html>`

		_, _ = p.Extract([]byte(htmlContent))
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
		defer func() { _ = p.Close() }()

		htmlContent := `<html><body><p>Test</p></body></html>`

		_, _ = p.Extract([]byte(htmlContent))
		_, _ = p.Extract([]byte(htmlContent))

		stats := p.GetStatistics()
		if stats.CacheHits != 0 {
			t.Errorf("With cache disabled, cache hits should be 0, got %d", stats.CacheHits)
		}
	})

	t.Run("statistics tracked", func(t *testing.T) {
		p, _ := html.New()
		defer func() { _ = p.Close() }()

		stats := p.GetStatistics()
		if stats.TotalProcessed != 0 {
			t.Errorf("Initial TotalProcessed = %d, want 0", stats.TotalProcessed)
		}

		htmlContent := `<html><body><p>Test</p></body></html>`
		_, _ = p.Extract([]byte(htmlContent))

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

	t.Run("concurrent with cache clearing", func(t *testing.T) {
		p, _ := html.New()
		defer func() { _ = p.Close() }()

		const goroutines = 20
		var wg sync.WaitGroup

		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				htmlContent := fmt.Sprintf(`<html><body><p>Content %d</p></body></html>`, id)
				_, _ = p.Extract([]byte(htmlContent))
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
	defer func() { _ = p.Close() }()

	tests := []struct {
		name    string
		html    string
		wantErr bool
		check   func(*testing.T, *html.Result, error)
	}{
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
			name:    "doctype only",
			html:    `<!DOCTYPE html>`,
			wantErr: false,
			check: func(t *testing.T, r *html.Result, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if r == nil {
					t.Error("result should not be nil")
				}
			},
		},
		{
			name:    "comment only",
			html:    `<!-- comment only -->`,
			wantErr: false,
			check: func(t *testing.T, r *html.Result, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if r == nil {
					t.Error("result should not be nil")
				}
			},
		},
		{
			name:    "empty html and body",
			html:    `<html><body></body></html>`,
			wantErr: false,
		},
		{
			name: "named and numeric entity decoding",
			html: `<html><body>
				<p>&amp;&lt;&gt;&quot;&apos;</p>
				<p>&nbsp;&nbsp;&copy;&reg;&trade;</p>
				<p>&mdash;&ndash;&hellip;</p>
				<p>&euro;&pound;&yen;</p>
				<p>&#65;&#x41;</p>
			</body></html>`,
			wantErr: false,
			check: func(t *testing.T, r *html.Result, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				for _, want := range []string{"&<>\"'", "©", "—"} {
					if !strings.Contains(r.Text, want) {
						t.Errorf("Text should contain decoded entity %q", want)
					}
				}
			},
		},
		{
			name: "bidirectional text preserved",
			html: `<html><body>
				<p>Hello مرحبا World עולם</p>
				<p dir="rtl">Right to left text</p>
			</body></html>`,
			wantErr: false,
			check: func(t *testing.T, r *html.Result, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				for _, want := range []string{"Hello", "مرحبا"} {
					if !strings.Contains(r.Text, want) {
						t.Errorf("Text should preserve %q", want)
					}
				}
			},
		},
		{
			name: "surrogate pairs (emoji) preserved",
			html: `<html><body>
				<p>Simple: 😀😂🥳</p>
				<p>Complex: 👨‍👩‍👧‍👦 🏳️‍🌈</p>
				<p>Skin tone: 👍🏻👍🏼👍🏽👍🏾👍🏿</p>
			</body></html>`,
			wantErr: false,
			check: func(t *testing.T, r *html.Result, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if !strings.Contains(r.Text, "😀") {
					t.Error("Text should preserve emoji")
				}
			},
		},
		{
			name:    "null character handled gracefully",
			html:    `<html><body><p>Before` + "\x00" + `After</p></body></html>`,
			wantErr: false,
			check: func(t *testing.T, r *html.Result, err error) {
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if r == nil {
					t.Error("expected non-nil result")
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
		defer func() { _ = p.Close() }()

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
		defer func() { _ = p.Close() }()

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
		defer func() { _ = p.Close() }()

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

// ============================================================================
// LINK FORMATTING TESTS
// ============================================================================

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
// JSON OUTPUT TESTS
// ============================================================================

// ============================================================================
// LINK EXTRACTION TESTS
// ============================================================================

// ============================================================================
// URL VALIDATION AND RESOLUTION TESTS
// ============================================================================

// ============================================================================
// LINK TAG EXTRACTION TESTS
// ============================================================================

// ============================================================================
// TABLE MARKDOWN FORMAT TESTS
// ============================================================================

// ============================================================================
// BASE URL AND DOMAIN EXTRACTION TESTS
// ============================================================================

// ============================================================================
// TEXT EXTRACTION EDGE CASES
// ============================================================================

func TestTextExtractionEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("nested block elements", func(t *testing.T) {
		p, _ := html.New()
		defer func() { _ = p.Close() }()

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
		defer func() { _ = p.Close() }()

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

	t.Run("excessive whitespace collapsed", func(t *testing.T) {
		p, _ := html.New()
		defer func() { _ = p.Close() }()

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

}

// ============================================================================
// ADDITIONAL URL AND DOMAIN TESTS FOR COVERAGE
// ============================================================================

// ============================================================================
// SCRIPT AND EMBED LINK EXTRACTION TESTS
// ============================================================================

// ============================================================================
// CONTENT LINK EXTRACTION TESTS
// ============================================================================

// ============================================================================
// TABLE MARKDOWN FORMAT ADDITIONAL TESTS
// ============================================================================

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

	t.Run("TextOnlyConfig disables media preservation", func(t *testing.T) {
		// Use TextOnlyConfig to disable media
		cfg := html.TextOnlyConfig()
		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer func() { _ = p.Close() }()

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

}

// ============================================================================
// Config Tests
// ============================================================================

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
		defer func() { _ = p.Close() }()

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

	t.Run("explicit true overrides default false", func(t *testing.T) {
		// Create config with IncludeVideos=false, IncludeImages=true
		cfg := html.DefaultConfig()
		cfg.IncludeVideos = false
		cfg.IncludeImages = true

		p, err := html.New(cfg)
		if err != nil {
			t.Fatalf("New() failed: %v", err)
		}
		defer func() { _ = p.Close() }()

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
		defer func() { _ = p.Close() }()

		// Generate unique large documents (>64KB to trigger multi-point sampling)
		for i := 0; i < 100; i++ {
			content := fmt.Sprintf(`<html><body><p id="%d">%s</p></body></html>`,
				i, strings.Repeat("x", 70000))
			_, _ = p.Extract([]byte(content))
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
		defer func() { _ = p.Close() }()

		content := `<html><body><p>Identical content</p></body></html>`

		for i := 0; i < 10; i++ {
			_, _ = p.Extract([]byte(content))
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
		defer func() { _ = p.Close() }()

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
		defer func() { _ = p.Close() }()

		htmlContent := `<html><body><article><p>Test content</p></article></body></html>`
		_, _ = p.Extract([]byte(htmlContent))

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

}
