package html_test

// output_test.go - Tests for output format functions
// This file tests ExtractToMarkdown, ExtractToJSON, and Result serialization.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/cybergodev/html"
	"github.com/cybergodev/html/internal/testutil"
)

// TestExtractToMarkdown tests ExtractToMarkdown with various HTML inputs.
func TestExtractToMarkdown(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		html       string
		contains   []string
		notContain []string
	}{
		{
			name:     "basic extraction",
			html:     `<html><body><h1>Title</h1><p>Content paragraph.</p></body></html>`,
			contains: []string{"Title", "Content"},
		},
		{
			name:     "markdown with images",
			html:     `<html><body><img src="test.jpg" alt="Test Image"><p>Text</p></body></html>`,
			contains: []string{"![Test Image](test.jpg)"},
		},
		{
			name:     "markdown with links",
			html:     `<html><body><a href="https://example.com">Link</a></body></html>`,
			contains: []string{"Link"},
		},
		{
			name:     "markdown with table",
			html:     `<html><body><table><tr><th>Name</th><th>Value</th></tr><tr><td>Test</td><td>100</td></tr></table></body></html>`,
			contains: []string{"|", "Name", "Value"},
		},
		{
			name:     "complex document",
			html:     `<!DOCTYPE html><html><head><title>Test Article</title></head><body><article><h1>Main Heading</h1><p>First paragraph with <strong>bold</strong> text.</p><h2>Subheading</h2><p>Second paragraph with <em>italic</em> text.</p><ul><li>Item 1</li><li>Item 2</li></ul></article></body></html>`,
			contains: []string{"Main Heading", "First paragraph", "Item 1"},
		},
		{
			name:       "empty HTML",
			html:       ``,
			notContain: []string{"should not appear"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			markdown, err := html.ExtractToMarkdown([]byte(tt.html))
			if err != nil {
				t.Fatalf("ExtractToMarkdown() failed: %v", err)
			}

			for _, substr := range tt.contains {
				if !strings.Contains(markdown, substr) {
					t.Errorf("Markdown should contain %q, got: %s", substr, markdown)
				}
			}

			for _, substr := range tt.notContain {
				if strings.Contains(markdown, substr) {
					t.Errorf("Markdown should not contain %q", substr)
				}
			}
		})
	}
}

// TestExtractToJSON tests ExtractToJSON with various HTML inputs.
func TestExtractToJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		html          string
		expectedTitle string
		expectedKeys  []string
	}{
		{
			name:          "basic JSON extraction",
			html:          `<html><head><title>Test Title</title></head><body><p>Test content.</p></body></html>`,
			expectedTitle: "Test Title",
			expectedKeys:  []string{"title", "text", "word_count"},
		},
		{
			name:         "JSON with all fields",
			html:         `<html><head><title>Title</title></head><body><img src="img.jpg" alt="Image"><a href="https://example.com">Link</a><video src="video.mp4"></video><audio src="audio.mp3"></audio></body></html>`,
			expectedKeys: []string{"title", "text", "word_count"},
		},
		{
			name:         "empty HTML JSON",
			html:         ``,
			expectedKeys: []string{"title", "text"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, err := html.ExtractToJSON([]byte(tt.html))
			if err != nil {
				t.Fatalf("ExtractToJSON() failed: %v", err)
			}

			if len(jsonData) == 0 {
				t.Fatal("JSON data should not be empty")
			}

			var result map[string]interface{}
			if err := json.Unmarshal(jsonData, &result); err != nil {
				t.Fatalf("Invalid JSON: %v", err)
			}

			if tt.expectedTitle != "" {
				if title, ok := result["title"].(string); !ok || title != tt.expectedTitle {
					t.Errorf("title = %v, want %q", result["title"], tt.expectedTitle)
				}
			}

			for _, key := range tt.expectedKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("JSON should have '%s' field", key)
				}
			}
		})
	}
}

// TestResultSerialization tests Result JSON marshaling/unmarshaling.
func TestResultSerialization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		result *html.Result
	}{
		{
			name: "result with all fields",
			result: &html.Result{
				Title:          "Test Title",
				Text:           "Test text content.",
				WordCount:      3,
				ReadingTime:    time.Millisecond * 500,
				ProcessingTime: time.Millisecond * 100,
				Images: []html.ImageInfo{
					{URL: "img.jpg", Alt: "Image", Width: "100", Height: "100"},
				},
				Links: []html.LinkInfo{
					{URL: "https://example.com", Text: "Link", IsExternal: true},
				},
				Videos: []html.VideoInfo{
					{URL: "video.mp4", Type: "video/mp4"},
				},
				Audios: []html.AudioInfo{
					{URL: "audio.mp3", Type: "audio/mp3"},
				},
			},
		},
		{
			name:   "empty result",
			result: &html.Result{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, err := json.Marshal(tt.result)
			if err != nil {
				t.Fatalf("json.Marshal() failed: %v", err)
			}

			if string(jsonData) == "" {
				t.Error("JSON should not be empty")
			}

			var unmarshaled html.Result
			if err := json.Unmarshal(jsonData, &unmarshaled); err != nil {
				t.Fatalf("json.Unmarshal() failed: %v", err)
			}

			if unmarshaled.Title != tt.result.Title {
				t.Errorf("Title = %q, want %q", unmarshaled.Title, tt.result.Title)
			}
			if unmarshaled.WordCount != tt.result.WordCount {
				t.Errorf("WordCount = %d, want %d", unmarshaled.WordCount, tt.result.WordCount)
			}
		})
	}
}

// TestMediaInfoJSONSerialization consolidates JSON serialization tests for all media info types.
func TestMediaInfoJSONSerialization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		data     interface{}
		expected map[string]interface{}
	}{
		{
			name: "image info full",
			data: html.ImageInfo{
				URL: "https://example.com/image.jpg", Alt: "Test image", Title: "Image title",
				Width: "800", Height: "600", IsDecorative: false, Position: 1,
			},
			expected: map[string]interface{}{"url": "https://example.com/image.jpg", "alt": "Test image", "title": "Image title"},
		},
		{
			name:     "image info minimal",
			data:     html.ImageInfo{URL: "https://example.com/minimal.jpg"},
			expected: map[string]interface{}{"url": "https://example.com/minimal.jpg"},
		},
		{
			name: "link info full",
			data: html.LinkInfo{
				URL: "https://example.com", Text: "Example", Title: "Example site",
				IsExternal: true, IsNoFollow: true,
			},
			expected: map[string]interface{}{"url": "https://example.com", "text": "Example", "is_external": true},
		},
		{
			name:     "link info minimal",
			data:     html.LinkInfo{URL: "https://example.com/minimal"},
			expected: map[string]interface{}{"url": "https://example.com/minimal"},
		},
		{
			name:     "video info full",
			data:     html.VideoInfo{URL: "https://example.com/video.mp4", Type: "video/mp4", Poster: "poster.jpg", Width: "640", Height: "480"},
			expected: map[string]interface{}{"url": "https://example.com/video.mp4", "type": "video/mp4"},
		},
		{
			name:     "video info minimal",
			data:     html.VideoInfo{URL: "https://example.com/minimal.mp4"},
			expected: map[string]interface{}{"url": "https://example.com/minimal.mp4"},
		},
		{
			name:     "audio info full",
			data:     html.AudioInfo{URL: "https://example.com/audio.mp3", Type: "audio/mp3"},
			expected: map[string]interface{}{"url": "https://example.com/audio.mp3", "type": "audio/mp3"},
		},
		{
			name:     "audio info minimal",
			data:     html.AudioInfo{URL: "https://example.com/minimal.mp3"},
			expected: map[string]interface{}{"url": "https://example.com/minimal.mp3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			jsonData, err := json.Marshal(tt.data)
			if err != nil {
				t.Fatalf("json.Marshal() failed: %v", err)
			}

			var result map[string]interface{}
			if err := json.Unmarshal(jsonData, &result); err != nil {
				t.Fatalf("json.Unmarshal() failed: %v", err)
			}

			for key, expectedValue := range tt.expected {
				actualValue, ok := result[key]
				if !ok {
					t.Errorf("Missing expected field: %s", key)
					continue
				}
				if actualValue != expectedValue {
					t.Errorf("Field %s = %v, want %v", key, actualValue, expectedValue)
				}
			}
		})
	}
}

// TestOutputWithProcessor tests output methods on Processor.
func TestOutputWithProcessor(t *testing.T) {
	t.Parallel()

	htmlContent := `<html><body><h1>Title</h1><p>Content</p></body></html>`

	t.Run("processor ExtractToMarkdown", func(t *testing.T) {
		p := testutil.NewTestProcessor(t)
		markdown, err := p.ExtractToMarkdown([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractToMarkdown() failed: %v", err)
		}
		if markdown == "" {
			t.Error("Markdown should not be empty")
		}
	})

	t.Run("processor ExtractToJSON", func(t *testing.T) {
		p := testutil.NewTestProcessor(t)
		jsonData, err := p.ExtractToJSON([]byte(htmlContent))
		if err != nil {
			t.Fatalf("ExtractToJSON() failed: %v", err)
		}
		if len(jsonData) == 0 {
			t.Error("JSON should not be empty")
		}
	})
}

// TestOutputWrappers_ConfigErrors drives the two shared error branches that all
// package-level ExtractToMarkdown*/ExtractToJSON* convenience wrappers funnel
// through:
//
//  1. resolveConfig rejecting >=2 variadic configs (html.ErrMultipleConfigs) —
//     the "multiple configs" subtests. Covers resolveConfig's default branch and
//     each wrapper's `if err != nil` early return.
//  2. withProcessor failing when New(cfg) rejects an invalid single config —
//     the "invalid config" subtests (the New(cfg) error path). A negative
//     MaxInputSize is rejected by Config.Validate, so the processor is never
//     built.
//
// Both paths return before any I/O or context work, so the file path passed to
// the *FromFile variants need not exist.
func TestOutputWrappers_ConfigErrors(t *testing.T) {
	t.Parallel()

	valid := html.DefaultConfig()
	invalid := html.DefaultConfig()
	invalid.MaxInputSize = -1 // Config.Validate rejects this
	ctx := context.Background()
	htmlBytes := []byte("<p>x</p>")

	wrappers := []struct {
		name       string
		multi      func() error // passes two configs -> ErrMultipleConfigs
		oneInvalid func() error // passes one invalid config -> New() failure
	}{
		{"ExtractToMarkdown", func() error {
			_, err := html.ExtractToMarkdown(htmlBytes, valid, valid)
			return err
		}, func() error {
			_, err := html.ExtractToMarkdown(htmlBytes, invalid)
			return err
		}},
		{"ExtractToMarkdownFromFile", func() error {
			_, err := html.ExtractToMarkdownFromFile("does-not-exist.html", valid, valid)
			return err
		}, func() error {
			_, err := html.ExtractToMarkdownFromFile("does-not-exist.html", invalid)
			return err
		}},
		{"ExtractToJSON", func() error {
			_, err := html.ExtractToJSON(htmlBytes, valid, valid)
			return err
		}, func() error {
			_, err := html.ExtractToJSON(htmlBytes, invalid)
			return err
		}},
		{"ExtractToJSONFromFile", func() error {
			_, err := html.ExtractToJSONFromFile("does-not-exist.html", valid, valid)
			return err
		}, func() error {
			_, err := html.ExtractToJSONFromFile("does-not-exist.html", invalid)
			return err
		}},
		{"ExtractToMarkdownWithContext", func() error {
			_, err := html.ExtractToMarkdownWithContext(ctx, htmlBytes, valid, valid)
			return err
		}, func() error {
			_, err := html.ExtractToMarkdownWithContext(ctx, htmlBytes, invalid)
			return err
		}},
		{"ExtractToMarkdownFromFileWithContext", func() error {
			_, err := html.ExtractToMarkdownFromFileWithContext(ctx, "does-not-exist.html", valid, valid)
			return err
		}, func() error {
			_, err := html.ExtractToMarkdownFromFileWithContext(ctx, "does-not-exist.html", invalid)
			return err
		}},
		{"ExtractToJSONWithContext", func() error {
			_, err := html.ExtractToJSONWithContext(ctx, htmlBytes, valid, valid)
			return err
		}, func() error {
			_, err := html.ExtractToJSONWithContext(ctx, htmlBytes, invalid)
			return err
		}},
		{"ExtractToJSONFromFileWithContext", func() error {
			_, err := html.ExtractToJSONFromFileWithContext(ctx, "does-not-exist.html", valid, valid)
			return err
		}, func() error {
			_, err := html.ExtractToJSONFromFileWithContext(ctx, "does-not-exist.html", invalid)
			return err
		}},
	}

	for _, w := range wrappers {
		t.Run(w.name+"/multiple configs", func(t *testing.T) {
			t.Parallel()
			if err := w.multi(); err == nil {
				t.Errorf("%s with two configs: expected ErrMultipleConfigs, got nil", w.name)
			} else if !errors.Is(err, html.ErrMultipleConfigs) {
				t.Errorf("%s with two configs: expected ErrMultipleConfigs, got %v", w.name, err)
			}
		})
		t.Run(w.name+"/invalid config", func(t *testing.T) {
			t.Parallel()
			if err := w.oneInvalid(); err == nil {
				t.Errorf("%s with invalid config: expected validation error, got nil", w.name)
			}
		})
	}
}

// TestProcessorOutputMethod_ContextError covers the error-return branches of the
// *Processor output methods that wrap ExtractWithContext. An already-canceled
// context makes the underlying extraction return ctx.Err() before doing any
// work, exercising each method's `if err != nil { return ..., err }` path
// (output.go) that the happy-path tests never reach.
func TestProcessorOutputMethod_ContextError(t *testing.T) {
	t.Parallel()

	p := testutil.NewTestProcessor(t)

	htmlBytes := []byte("<html><body><p>content</p></body></html>")

	// ExtractToJSONWithContext: wraps p.ExtractWithContext(ctx, htmlBytes).
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately so the early ctx.Done() check in extractCore fires

	jsonData, err := p.ExtractToJSONWithContext(ctx, htmlBytes)
	if err == nil {
		t.Fatal("ExtractToJSONWithContext: expected context error, got nil")
	}
	if jsonData != nil {
		t.Errorf("ExtractToJSONWithContext: expected nil JSON on error, got %d bytes", len(jsonData))
	}
}
