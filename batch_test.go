package html_test

// batch_test.go - Tests for batch extraction functions
// Tests for Processor.ExtractBatch, ExtractBatchWithContext, ExtractBatchFiles, ExtractBatchFilesWithContext

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cybergodev/html"
	"github.com/cybergodev/html/internal/testutil"
)

// TestExtractBatch tests the ExtractBatch processor method.
func TestExtractBatch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		docs        [][]byte
		wantLen     int
		wantErr     bool
		checkClosed bool
	}{
		{
			name:    "basic batch extraction",
			docs:    [][]byte{[]byte(`<html><body><p>Document 1</p></body></html>`), []byte(`<html><body><p>Document 2</p></body></html>`), []byte(`<html><body><p>Document 3</p></body></html>`)},
			wantLen: 3,
		},
		{
			name:    "empty batch nil",
			docs:    nil,
			wantLen: 0,
		},
		{
			name:    "empty batch slice",
			docs:    [][]byte{},
			wantLen: 0,
		},
		{
			name:    "batch with invalid HTML",
			docs:    [][]byte{[]byte(`<html><body><p>Valid</p></body></html>`), []byte(""), []byte(`<html><body><p>Also valid</p></body></html>`)},
			wantLen: 3,
		},
		{
			name:        "closed processor",
			docs:        [][]byte{[]byte(`<html><body><p>Content</p></body></html>`)},
			wantErr:     true,
			checkClosed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := html.New()
			if err != nil {
				t.Fatalf("New() failed: %v", err)
			}

			if tt.checkClosed {
				_ = p.Close()
			} else {
				defer func() { _ = p.Close() }()
			}

			br := p.ExtractBatch(tt.docs)
			if tt.wantErr {
				if br.Failed == 0 {
					t.Error("Expected error")
				}
				if !errors.Is(br.Errors[0], html.ErrProcessorClosed) {
					t.Errorf("Expected ErrProcessorClosed, got: %v", br.Errors[0])
				}
				return
			}

			if br.Failed > 0 {
				t.Fatalf("ExtractBatch() failed: %v", br.Errors[0])
			}

			if len(br.Results) != tt.wantLen {
				t.Errorf("Expected %d results, got %d", tt.wantLen, len(br.Results))
			}
		})
	}
}

// TestExtractBatchWithConfig tests ExtractBatch with custom configuration.
func TestExtractBatchWithConfig(t *testing.T) {
	t.Parallel()

	cfg := html.DefaultConfig()
	cfg.PreserveImages = false
	p := testutil.NewTestProcessor(t, cfg)

	docs := [][]byte{[]byte(`<html><body><p>Content</p><img src="test.jpg"/></body></html>`)}

	br := p.ExtractBatch(docs)
	if br.Failed > 0 {
		t.Fatalf("ExtractBatch() with config failed: %v", br.Errors[0])
	}

	if len(br.Results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(br.Results))
	}
	if len(br.Results[0].Images) != 0 {
		t.Errorf("Expected no images with PreserveImages=false")
	}
}

// TestExtractBatchWithContext tests the ExtractBatchWithContext processor method.
func TestExtractBatchWithContext(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		docs         [][]byte
		ctx          context.Context
		cancelBefore bool
		wantSuccess  int
		wantFailed   int
	}{
		{
			name:        "successful batch with context",
			docs:        [][]byte{[]byte(`<html><body><p>Doc 1</p></body></html>`), []byte(`<html><body><p>Doc 2</p></body></html>`)},
			ctx:         context.Background(),
			wantSuccess: 2,
		},
		{
			name:         "cancelled context",
			docs:         createNDocs(100),
			cancelBefore: true,
		},
		{
			name:        "empty batch with context",
			docs:        nil,
			ctx:         context.Background(),
			wantSuccess: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := testutil.NewTestProcessor(t)

			ctx := tt.ctx
			if ctx == nil {
				ctx = context.Background()
			}

			if tt.cancelBefore {
				cctx, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cctx
			}

			result := p.ExtractBatchWithContext(ctx, tt.docs)

			if tt.cancelBefore {
				// Every document must be accounted for as cancelled or failed —
				// none may silently succeed or vanish after cancellation.
				if result.Cancelled+result.Failed != len(tt.docs) {
					t.Errorf("Cancelled+Failed = %d+%d, want %d",
						result.Cancelled, result.Failed, len(tt.docs))
				}
				if result.Success != 0 {
					t.Errorf("Success = %d, want 0 after cancellation", result.Success)
				}
			} else {
				if result.Success != tt.wantSuccess {
					t.Errorf("Expected %d successful, got %d", tt.wantSuccess, result.Success)
				}
				if result.Failed != tt.wantFailed {
					t.Errorf("Expected %d failed, got %d", tt.wantFailed, result.Failed)
				}
			}

			if tt.docs == nil {
				if len(result.Results) != 0 || len(result.Errors) != 0 {
					t.Errorf("Expected empty results for nil input")
				}
			}
		})
	}
}

// TestExtractBatchWithContextTimeout tests context timeout behavior.
func TestExtractBatchWithContextTimeout(t *testing.T) {
	t.Parallel()

	p := testutil.NewTestProcessor(t)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(10 * time.Millisecond) // Wait for context to expire

	docs := createNDocs(50)
	result := p.ExtractBatchWithContext(ctx, docs)

	// Every document must be accounted for exactly once, even when the context
	// is already expired: no panics, no dropped or double-counted work. Whether a
	// given doc is cancelled or succeeds is timing-dependent, so only the total
	// is a deterministic invariant.
	total := result.Cancelled + result.Success + result.Failed
	if total != len(docs) {
		t.Errorf("accounting mismatch: Cancelled+Success+Failed = %d, want %d (cancelled=%d success=%d failed=%d)",
			total, len(docs), result.Cancelled, result.Success, result.Failed)
	}
}

// TestExtractBatchFiles tests the ExtractBatchFiles processor method.
func TestExtractBatchFiles(t *testing.T) {
	t.Parallel()

	t.Run("batch file extraction", func(t *testing.T) {
		tmpDir := t.TempDir()
		files := createTempHTMLFiles(t, tmpDir, 3)

		p := testutil.NewTestProcessor(t)
		br := p.ExtractBatchFiles(files)
		if br.Failed > 0 {
			t.Fatalf("ExtractBatchFiles() failed: %v", br.Errors[0])
		}

		if len(br.Results) != len(files) {
			t.Errorf("Expected %d results, got %d", len(files), len(br.Results))
		}
	})

	t.Run("batch with non-existent file", func(t *testing.T) {
		p := testutil.NewTestProcessor(t)
		br := p.ExtractBatchFiles([]string{"non-existent-file.html"})
		if br.Failed == 0 {
			t.Error("Expected error for non-existent file")
		}
	})

	t.Run("empty file list", func(t *testing.T) {
		p := testutil.NewTestProcessor(t)
		br := p.ExtractBatchFiles(nil)
		if br.Failed > 0 {
			t.Fatalf("ExtractBatchFiles(nil) failed: %v", br.Errors[0])
		}
		if len(br.Results) != 0 {
			t.Errorf("Expected 0 results for nil input, got %d", len(br.Results))
		}
	})

	t.Run("closed processor", func(t *testing.T) {
		p, _ := html.New()
		_ = p.Close()

		br := p.ExtractBatchFiles([]string{"test.html"})
		if br.Failed == 0 {
			t.Error("Expected error on closed processor")
		}
	})
}

// TestExtractBatchFilesWithContext tests the ExtractBatchFilesWithContext processor method.
func TestExtractBatchFilesWithContext(t *testing.T) {
	t.Parallel()

	t.Run("batch file extraction with context", func(t *testing.T) {
		p := testutil.NewTestProcessor(t)
		tmpDir := t.TempDir()
		files := createTempHTMLFiles(t, tmpDir, 2)

		result := p.ExtractBatchFilesWithContext(context.Background(), files)

		if len(result.Results) != len(files) {
			t.Errorf("Expected %d results, got %d", len(files), len(result.Results))
		}
		if result.Success != len(files) {
			t.Errorf("Expected %d successful, got %d", len(files), result.Success)
		}
	})

	t.Run("batch files with errors", func(t *testing.T) {
		p := testutil.NewTestProcessor(t)
		tmpDir := t.TempDir()

		validFile := tmpDir + "/valid.html"
		_ = os.WriteFile(validFile, []byte(`<html><body><p>Valid</p></body></html>`), 0644)

		files := []string{validFile, "non-existent-file.html"}

		result := p.ExtractBatchFilesWithContext(context.Background(), files)

		if result.Success == 0 {
			t.Error("Expected at least one successful extraction")
		}
		if result.Failed == 0 {
			t.Error("Expected at least one failed extraction")
		}
	})
}

// TestBatchResultStructure tests the BatchResult structure fields.
// Removed TestBatchResultStructure: never-nil assertions on slices that
// ExtractBatchWithContext always allocates, plus the accounting invariant
// (Success+Failed+Cancelled == len(docs)) already asserted identically in
// TestExtractBatchWithContextTimeout and TestExtractBatchWithContext.

// Helper functions

func createNDocs(n int) [][]byte {
	docs := make([][]byte, n)
	content := []byte(`<html><body><p>Document content for testing batch processing.</p></body></html>`)
	for i := range docs {
		docs[i] = content
	}
	return docs
}

func createTempHTMLFiles(t *testing.T, tmpDir string, count int) []string {
	t.Helper()
	files := make([]string, count)
	for i := range count {
		files[i] = tmpDir + "/file" + string(rune('A'+i)) + ".html"
		content := []byte(`<html><body><h1>File ` + string(rune('A'+i)) + `</h1></body></html>`)
		if err := os.WriteFile(files[i], content, 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}
	return files
}

// ---------------------------------------------------------------------------
// Package-level batch wrapper boundaries (from the dissolved boundary_test.go)
// ---------------------------------------------------------------------------

// boundaryTestHTML and writeTempHTML are shared fixtures for the moved
// package-level wrapper tests across this package (links_test.go,
// output_test.go, layer_equivalence_test.go). writeTempHTML mirrors
// testutil.CreateTempHTML.
const boundaryTestHTML = `<html><head><title>Boundary Test</title></head><body><article><p>Content</p></article></body></html>`

// writeTempHTML writes content to a fresh temp .html file and returns its path.
func writeTempHTML(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.html")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatalf("write temp html: %v", err)
	}
	return p
}

// TestPackageLevelBatchWithContext covers the package-level batch WithContext
// entry points (from boundary_test.go).
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
			_ = os.WriteFile(paths[i], []byte(boundaryTestHTML), 0644)
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

// TestPackageLevelBatchFiles_HappyPath covers the package-level
// ExtractBatchFiles happy path. The processor-method variant is tested above
// but the convenience wrapper was only tested on its error branch.
func TestPackageLevelBatchFiles_HappyPath(t *testing.T) {
	t.Parallel()

	tmpDir := t.TempDir()
	paths := make([]string, 3)
	for i := range paths {
		paths[i] = filepath.Join(tmpDir, fmt.Sprintf("file%d.html", i))
		content := fmt.Sprintf(`<html><body><h1>File %d</h1></body></html>`, i)
		_ = os.WriteFile(paths[i], []byte(content), 0644)
	}

	br := html.ExtractBatchFiles(paths)
	if br.Failed > 0 {
		t.Fatalf("unexpected failures: %v", br.Errors)
	}
	if len(br.Results) != 3 {
		t.Errorf("got %d results, want 3", len(br.Results))
	}
}

// TestBatchMultipleConfigsRejected covers the resolveConfig error branch of
// withConfigBatch (processor_pool.go): two variadic configs must fail every
// batch wrapper uniformly, marking all items failed with ErrMultipleConfigs.
func TestBatchMultipleConfigsRejected(t *testing.T) {
	t.Parallel()

	cfg1 := html.DefaultConfig()
	cfg2 := html.DefaultConfig()

	tests := []struct {
		name string
		run  func() *html.BatchResult
	}{
		{"ExtractBatch", func() *html.BatchResult {
			return html.ExtractBatch([][]byte{[]byte(`<p>x</p>`)}, cfg1, cfg2)
		}},
		{"ExtractBatchWithContext", func() *html.BatchResult {
			return html.ExtractBatchWithContext(context.Background(), [][]byte{[]byte(`<p>x</p>`)}, cfg1, cfg2)
		}},
		{"ExtractBatchFiles", func() *html.BatchResult {
			return html.ExtractBatchFiles([]string{"a.html"}, cfg1, cfg2)
		}},
		{"ExtractBatchFilesWithContext", func() *html.BatchResult {
			return html.ExtractBatchFilesWithContext(context.Background(), []string{"a.html"}, cfg1, cfg2)
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			br := tt.run()
			if br.Failed != 1 {
				t.Fatalf("Failed = %d, want 1", br.Failed)
			}
			if !errors.Is(br.Errors[0], html.ErrMultipleConfigs) {
				t.Errorf("error = %v, want ErrMultipleConfigs", br.Errors[0])
			}
		})
	}
}
