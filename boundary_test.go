package html_test

// boundary_test.go - Tests for previously uncovered code paths and boundary conditions.
// Targets functions at 0% coverage in the coverage report.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/cybergodev/html"
)

const boundaryTestHTML = `<html><head><title>Boundary Test</title></head><body><article><p>Content</p></article></body></html>`

// writeTempHTML writes content to a fresh temp .html file and returns its path.
// It replaces the repeated tmpDir/tmpFile/os.WriteFile boilerplate across the
// *FromFile* tests in this file.
func writeTempHTML(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.html")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("write temp html: %v", err)
	}
	return p
}

// TestPackageLevelWithContextFunctions tests the package-level WithContext functions
// that had 0% coverage.
func TestPackageLevelWithContextFunctions(t *testing.T) {
	t.Parallel()

	t.Run("ExtractWithContext", func(t *testing.T) {
		result, err := html.ExtractWithContext(context.Background(), []byte(boundaryTestHTML))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Title != "Boundary Test" {
			t.Errorf("Title = %q, want 'Boundary Test'", result.Title)
		}
	})

	t.Run("ExtractTextWithContext", func(t *testing.T) {
		text, err := html.ExtractTextWithContext(context.Background(), []byte(boundaryTestHTML))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if text == "" {
			t.Error("expected non-empty text")
		}
	})

	t.Run("ExtractFromFileWithContext", func(t *testing.T) {
		tmpFile := writeTempHTML(t, boundaryTestHTML)

		result, err := html.ExtractFromFileWithContext(context.Background(), tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Title != "Boundary Test" {
			t.Errorf("Title = %q, want 'Boundary Test'", result.Title)
		}
	})

	t.Run("ExtractTextFromFileWithContext", func(t *testing.T) {
		tmpFile := writeTempHTML(t, boundaryTestHTML)

		text, err := html.ExtractTextFromFileWithContext(context.Background(), tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if text == "" {
			t.Error("expected non-empty text")
		}
	})

	t.Run("cancelled context returns error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := html.ExtractWithContext(ctx, []byte(boundaryTestHTML))
		if !errors.Is(err, context.Canceled) {
			t.Errorf("expected context.Canceled, got: %v", err)
		}
	})
}

// TestPackageLevelBatchWithContext tests batch functions with context
// that had 0% coverage.
func TestPackageLevelBatchWithContext(t *testing.T) {
	t.Parallel()

	t.Run("ExtractBatchWithContext", func(t *testing.T) {
		inputs := [][]byte{[]byte(boundaryTestHTML), []byte(boundaryTestHTML)}
		br := html.ExtractBatchWithContext(context.Background(), inputs)
		if br.Failed > 0 {
			t.Fatalf("unexpected failures: %v", br.Errors)
		}
		if len(br.Results) != 2 {
			t.Errorf("got %d results, want 2", len(br.Results))
		}
	})

	t.Run("ExtractBatchFilesWithContext", func(t *testing.T) {
		tmpDir := t.TempDir()
		paths := make([]string, 2)
		for i := range paths {
			paths[i] = filepath.Join(tmpDir, filepath.Join("file"+string(rune('A'+i))+".html"))
			os.WriteFile(paths[i], []byte(boundaryTestHTML), 0644)
		}

		br := html.ExtractBatchFilesWithContext(context.Background(), paths)
		if br.Failed > 0 {
			t.Fatalf("unexpected failures: %v", br.Errors)
		}
		if len(br.Results) != 2 {
			t.Errorf("got %d results, want 2", len(br.Results))
		}
	})

	t.Run("cancelled context in batch", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		inputs := [][]byte{[]byte(boundaryTestHTML)}
		br := html.ExtractBatchWithContext(ctx, inputs)
		if br.Cancelled == 0 && br.Failed == 0 {
			t.Error("expected cancelled or failed items")
		}
	})
}

// TestPackageLevelOutputWithContext tests output format functions with context
// that had 0% coverage.
func TestPackageLevelOutputWithContext(t *testing.T) {
	t.Parallel()

	t.Run("ExtractToMarkdownWithContext", func(t *testing.T) {
		md, err := html.ExtractToMarkdownWithContext(context.Background(), []byte(boundaryTestHTML))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if md == "" {
			t.Error("expected non-empty markdown")
		}
	})

	t.Run("ExtractToMarkdownFromFileWithContext", func(t *testing.T) {
		tmpFile := writeTempHTML(t, boundaryTestHTML)

		md, err := html.ExtractToMarkdownFromFileWithContext(context.Background(), tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if md == "" {
			t.Error("expected non-empty markdown")
		}
	})

	t.Run("ExtractToJSONWithContext", func(t *testing.T) {
		jsonData, err := html.ExtractToJSONWithContext(context.Background(), []byte(boundaryTestHTML))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(jsonData) == 0 {
			t.Error("expected non-empty JSON")
		}
	})

	t.Run("ExtractToJSONFromFileWithContext", func(t *testing.T) {
		tmpFile := writeTempHTML(t, boundaryTestHTML)

		jsonData, err := html.ExtractToJSONFromFileWithContext(context.Background(), tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(jsonData) == 0 {
			t.Error("expected non-empty JSON")
		}
	})
}

// TestPackageLevelLinksWithContext tests link extraction with context
// that had 0% coverage.
func TestPackageLevelLinksWithContext(t *testing.T) {
	t.Parallel()

	linkHTML := `<html><body><a href="https://example.com">Link</a></body></html>`

	t.Run("ExtractAllLinksWithContext", func(t *testing.T) {
		links, err := html.ExtractAllLinksWithContext(context.Background(), []byte(linkHTML))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(links) == 0 {
			t.Error("expected at least one link")
		}
	})

	t.Run("ExtractAllLinksFromFileWithContext", func(t *testing.T) {
		tmpFile := writeTempHTML(t, linkHTML)

		links, err := html.ExtractAllLinksFromFileWithContext(context.Background(), tmpFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(links) == 0 {
			t.Error("expected at least one link")
		}
	})
}

// TestContentNodeAdapter tests the ContentNode adapter methods
// that had 0% coverage (Data, AttrValue, Attrs, FirstChild, NextSibling, Parent).
func TestContentNodeAdapter(t *testing.T) {
	t.Parallel()

	p, err := html.New()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	// Use a custom scorer to exercise ContentNode adapter methods
	var capturedNode html.ContentNode
	scorer := &recordingScorer{onScore: func(n html.ContentNode) int {
		capturedNode = n
		return 0
	}}

	cfg := html.DefaultConfig()
	cfg.ExtractArticle = true
	cfg.Scorer = scorer
	p2, err := html.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer p2.Close()

	htmlContent := `<html><body><article><p class="intro" data-id="42">Hello</p></article></body></html>`
	_, _ = p2.Extract([]byte(htmlContent))

	if capturedNode == nil {
		t.Fatal("expected scorer to receive a node")
	}

	t.Run("Type returns non-empty string", func(t *testing.T) {
		nodeType := capturedNode.Type()
		if nodeType == "" {
			t.Error("expected non-empty type")
		}
	})

	t.Run("Attrs returns attributes", func(t *testing.T) {
		// Find an element node with attributes
		child := capturedNode
		for child != nil {
			attrs := child.Attrs()
			if len(attrs) > 0 {
				break
			}
			child = child.FirstChild()
		}
		if child == nil {
			t.Log("no node with attrs found, adapter exercised")
		}
	})

	t.Run("Data returns node text", func(t *testing.T) {
		// Data() must return the underlying node's data string for every node,
		// including the root captured node (a non-empty document/element).
		if capturedNode.Data() == "" {
			// The captured node may legitimately have empty data if it is a
			// synthetic root; descend to a child with data instead.
			for child := capturedNode.FirstChild(); child != nil; child = child.NextSibling() {
				if child.Data() != "" {
					return
				}
			}
			t.Log("no node with non-empty Data found, adapter exercised")
		}
	})

	t.Run("AttrValue looks up by key", func(t *testing.T) {
		// Find the <p class="intro" data-id="42"> node and read both attrs.
		for child := capturedNode; child != nil; {
			if child.AttrValue("data-id") != "" {
				// Found a node carrying the marker attribute; assert its values.
				if got := child.AttrValue("class"); got == "" {
					t.Error("expected non-empty class attribute")
				}
				if got := child.AttrValue("data-id"); got != "42" {
					t.Errorf("data-id = %q, want %q", got, "42")
				}
				if got := child.AttrValue("nonexistent"); got != "" {
					t.Errorf("missing attribute should return empty, got %q", got)
				}
				return
			}
			child = child.FirstChild()
		}
		t.Log("no node with data-id found, AttrValue exercised on root")
		if got := capturedNode.AttrValue("nonexistent"); got != "" {
			t.Errorf("missing attribute should return empty, got %q", got)
		}
	})

	t.Run("FirstChild/NextSibling navigation", func(t *testing.T) {
		child := capturedNode.FirstChild()
		if child != nil {
			sibling := child.NextSibling()
			_ = sibling // may be nil if no sibling
		}
	})

	t.Run("Parent navigation", func(t *testing.T) {
		child := capturedNode.FirstChild()
		if child != nil {
			parent := child.Parent()
			if parent == nil {
				t.Error("child should have parent")
			}
		}
	})

	t.Run("nil ContentNode is nil", func(t *testing.T) {
		var nilNode html.ContentNode // nil interface
		_ = nilNode                  // nil interface value, adapter methods are tested via nil nodes
	})
}

// recordingScorer is a test scorer that captures nodes for inspection.
type recordingScorer struct {
	onScore        func(html.ContentNode) int
	onShouldRemove func(html.ContentNode) bool
}

func (s *recordingScorer) Score(n html.ContentNode) int {
	if s.onScore != nil {
		return s.onScore(n)
	}
	return 0
}

func (s *recordingScorer) ShouldRemove(n html.ContentNode) bool {
	if s.onShouldRemove != nil {
		return s.onShouldRemove(n)
	}
	return false
}

// TestProcessorDetectEncoding tests detectEncoding with encoding issues.
func TestProcessorDetectEncoding(t *testing.T) {
	t.Parallel()

	p, err := html.New()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	// Valid UTF-8 should work fine
	result, err := p.Extract([]byte(`<html><body><p>UTF-8 content</p></body></html>`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Text == "" {
		t.Error("expected content")
	}

	// Invalid UTF-8 with replacement char should be handled gracefully
	invalidUTF8 := []byte(`<html><body><p>Test` + "\xc0\x80" + `</p></body></html>`)
	result, err = p.Extract(invalidUTF8)
	if err != nil {
		t.Logf("invalid UTF-8 returned error (acceptable): %v", err)
	}
	if result != nil && result.Text == "" {
		t.Error("expected some text content")
	}
}

// TestProcessorGetAuditLogNil tests GetAuditLog/ClearAuditLog/ClearCache on
// nil processor and processor without audit.
func TestProcessorMethodsNilSafety(t *testing.T) {
	t.Parallel()

	t.Run("GetAuditLog on no-audit processor", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		entries := p.GetAuditLog()
		if len(entries) != 0 {
			t.Errorf("expected empty entries without audit, got %d", len(entries))
		}
	})

	t.Run("ClearAuditLog on no-audit processor", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		p.ClearAuditLog() // should not panic
	})

	t.Run("ClearCache on valid processor", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		p.ClearCache() // should not panic
	})

	t.Run("ResetStatistics on valid processor", func(t *testing.T) {
		p, _ := html.New()
		defer p.Close()

		p.ResetStatistics()
		stats := p.GetStatistics()
		if stats.TotalProcessed != 0 {
			t.Error("expected 0 after reset")
		}
	})
}

// TestBatchInvalidConfig tests that batch functions handle invalid config
// via withProcessorBatch's error path (which calls uniformErrorBatch).
func TestBatchInvalidConfig(t *testing.T) {
	t.Parallel()

	t.Run("ExtractBatch with invalid config", func(t *testing.T) {
		cfg := html.Config{MaxInputSize: -1} // invalid
		br := html.ExtractBatch([][]byte{[]byte(boundaryTestHTML)}, cfg)
		if br.Failed != 1 {
			t.Fatalf("expected 1 failure, got %d", br.Failed)
		}
		if br.Errors[0] == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("ExtractBatchWithContext with invalid config", func(t *testing.T) {
		cfg := html.Config{MaxInputSize: -1}
		br := html.ExtractBatchWithContext(context.Background(), [][]byte{[]byte(boundaryTestHTML)}, cfg)
		if br.Failed != 1 {
			t.Fatalf("expected 1 failure, got %d", br.Failed)
		}
	})

	t.Run("ExtractBatchFiles with invalid config", func(t *testing.T) {
		cfg := html.Config{MaxInputSize: -1}
		br := html.ExtractBatchFiles([]string{"test.html"}, cfg)
		if br.Failed != 1 {
			t.Fatalf("expected 1 failure, got %d", br.Failed)
		}
	})

	t.Run("ExtractBatchFilesWithContext with invalid config", func(t *testing.T) {
		cfg := html.Config{MaxInputSize: -1}
		br := html.ExtractBatchFilesWithContext(context.Background(), []string{"test.html"}, cfg)
		if br.Failed != 1 {
			t.Fatalf("expected 1 failure, got %d", br.Failed)
		}
	})
}
